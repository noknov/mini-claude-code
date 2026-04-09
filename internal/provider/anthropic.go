package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	anthropicDefaultURL  = "https://api.anthropic.com"
	anthropicAPIVersion  = "2023-06-01"
	anthropicDefaultToks = 16384
)

// Anthropic implements Provider for the Anthropic Messages API.
type Anthropic struct {
	apiKey  string
	model   string
	baseURL string
	client  *http.Client
}

func NewAnthropic(apiKey, model, baseURL string) *Anthropic {
	if baseURL == "" {
		baseURL = anthropicDefaultURL
	}
	return &Anthropic{
		apiKey:  apiKey,
		model:   model,
		baseURL: strings.TrimRight(baseURL, "/"),
		client:  &http.Client{Timeout: 10 * time.Minute},
	}
}

func (a *Anthropic) Name() string      { return "anthropic" }
func (a *Anthropic) Model() string     { return a.model }
func (a *Anthropic) SetModel(m string) { a.model = m }

func (a *Anthropic) SendStream(ctx context.Context, req Request) (<-chan StreamEvent, <-chan error) {
	eventCh := make(chan StreamEvent, 64)
	errCh := make(chan error, 1)

	maxToks := req.MaxTokens
	if maxToks == 0 {
		maxToks = anthropicDefaultToks
	}

	apiReq := anthropicRequest{
		Model:     a.model,
		MaxTokens: maxToks,
		System:    req.SystemPrompt,
		Stream:    true,
	}
	for _, m := range req.Messages {
		apiReq.Messages = append(apiReq.Messages, m)
	}
	for _, t := range req.Tools {
		apiReq.Tools = append(apiReq.Tools, t)
	}

	go func() {
		defer close(eventCh)
		defer close(errCh)

		body, err := json.Marshal(apiReq)
		if err != nil {
			errCh <- fmt.Errorf("marshal: %w", err)
			return
		}

		httpReq, err := http.NewRequestWithContext(ctx, "POST", a.baseURL+"/v1/messages", bytes.NewReader(body))
		if err != nil {
			errCh <- fmt.Errorf("create request: %w", err)
			return
		}
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("X-API-Key", a.apiKey)
		httpReq.Header.Set("anthropic-version", anthropicAPIVersion)

		resp, err := a.client.Do(httpReq)
		if err != nil {
			errCh <- fmt.Errorf("send request: %w", err)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			respBody, _ := io.ReadAll(resp.Body)
			errCh <- fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(respBody))
			return
		}

		a.parseSSE(resp.Body, eventCh, errCh)
	}()

	return eventCh, errCh
}

func (a *Anthropic) parseSSE(r io.Reader, eventCh chan<- StreamEvent, errCh chan<- error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 1<<20), 1<<20)

	var (
		sseType  string
		result   Response
		curBlock *ContentBlock
		jsonBuf  strings.Builder
	)

	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "event: ") {
			sseType = strings.TrimPrefix(line, "event: ")
			continue
		}
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := []byte(strings.TrimPrefix(line, "data: "))

		switch sseType {
		case "message_start":
			var d struct {
				Message struct {
					ID    string `json:"id"`
					Model string `json:"model"`
					Usage *struct {
						InputTokens int `json:"input_tokens"`
					} `json:"usage"`
				} `json:"message"`
			}
			if json.Unmarshal(data, &d) == nil {
				result.ID = d.Message.ID
				result.Model = d.Message.Model
				if d.Message.Usage != nil {
					result.InputTokens = d.Message.Usage.InputTokens
				}
			}

		case "content_block_start":
			var d struct {
				Index int          `json:"index"`
				Block ContentBlock `json:"content_block"`
			}
			if json.Unmarshal(data, &d) == nil {
				block := d.Block
				curBlock = &block
				jsonBuf.Reset()
				if block.Type == "text" {
					eventCh <- StreamEvent{Type: "text"}
				} else if block.Type == "tool_use" {
					eventCh <- StreamEvent{
						Type:     "tool_use_start",
						ToolID:   block.ID,
						ToolName: block.Name,
					}
				}
			}

		case "content_block_delta":
			var d struct {
				Delta struct {
					Type        string `json:"type"`
					Text        string `json:"text,omitempty"`
					PartialJSON string `json:"partial_json,omitempty"`
				} `json:"delta"`
			}
			if json.Unmarshal(data, &d) != nil {
				continue
			}
			switch d.Delta.Type {
			case "text_delta":
				if curBlock != nil {
					curBlock.Text += d.Delta.Text
				}
				eventCh <- StreamEvent{Type: "text", Text: d.Delta.Text}
			case "input_json_delta":
				jsonBuf.WriteString(d.Delta.PartialJSON)
				eventCh <- StreamEvent{Type: "tool_input_delta", PartialJSON: d.Delta.PartialJSON}
			}

		case "content_block_stop":
			if curBlock == nil {
				continue
			}
			if curBlock.Type == "tool_use" && jsonBuf.Len() > 0 {
				curBlock.Input = json.RawMessage(jsonBuf.String())
			}
			result.ContentBlocks = append(result.ContentBlocks, *curBlock)
			if curBlock.Type == "tool_use" {
				eventCh <- StreamEvent{Type: "tool_use_end"}
			}
			curBlock = nil

		case "message_delta":
			var d struct {
				Delta struct {
					StopReason string `json:"stop_reason"`
				} `json:"delta"`
				Usage *struct {
					OutputTokens int `json:"output_tokens"`
				} `json:"usage"`
			}
			if json.Unmarshal(data, &d) == nil {
				result.StopReason = d.Delta.StopReason
				if d.Usage != nil {
					result.OutputTokens = d.Usage.OutputTokens
				}
			}

		case "message_stop":
			eventCh <- StreamEvent{Type: "done", Response: &result}
		}
	}

	if err := scanner.Err(); err != nil {
		errCh <- fmt.Errorf("read stream: %w", err)
	}
}

type anthropicRequest struct {
	Model     string    `json:"model"`
	MaxTokens int       `json:"max_tokens"`
	System    string    `json:"system,omitempty"`
	Messages  []Message `json:"messages"`
	Tools     []ToolDef `json:"tools,omitempty"`
	Stream    bool      `json:"stream"`
}
