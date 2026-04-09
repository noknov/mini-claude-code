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
	openaiDefaultURL  = "https://api.openai.com"
	openaiDefaultToks = 16384
)

// OpenAI implements Provider for the OpenAI Chat Completions API.
// Also works with any OpenAI-compatible endpoint (Ollama, vLLM, Together, etc.).
type OpenAI struct {
	apiKey  string
	model   string
	baseURL string
	client  *http.Client
}

func NewOpenAI(apiKey, model, baseURL string) *OpenAI {
	if baseURL == "" {
		baseURL = openaiDefaultURL
	}
	return &OpenAI{
		apiKey:  apiKey,
		model:   model,
		baseURL: strings.TrimRight(baseURL, "/"),
		client:  &http.Client{Timeout: 10 * time.Minute},
	}
}

func (o *OpenAI) Name() string      { return "openai" }
func (o *OpenAI) Model() string     { return o.model }
func (o *OpenAI) SetModel(m string) { o.model = m }

func (o *OpenAI) SendStream(ctx context.Context, req Request) (<-chan StreamEvent, <-chan error) {
	eventCh := make(chan StreamEvent, 64)
	errCh := make(chan error, 1)

	maxToks := req.MaxTokens
	if maxToks == 0 {
		maxToks = openaiDefaultToks
	}

	apiReq := o.buildRequest(req, maxToks)

	go func() {
		defer close(eventCh)
		defer close(errCh)

		body, err := json.Marshal(apiReq)
		if err != nil {
			errCh <- fmt.Errorf("marshal: %w", err)
			return
		}

		httpReq, err := http.NewRequestWithContext(ctx, "POST", o.baseURL+"/v1/chat/completions", bytes.NewReader(body))
		if err != nil {
			errCh <- fmt.Errorf("create request: %w", err)
			return
		}
		httpReq.Header.Set("Content-Type", "application/json")
		if o.apiKey != "" {
			httpReq.Header.Set("Authorization", "Bearer "+o.apiKey)
		}

		resp, err := o.client.Do(httpReq)
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

		o.parseSSE(resp.Body, eventCh, errCh)
	}()

	return eventCh, errCh
}

// buildRequest converts our generic Request into an OpenAI-shaped payload.
func (o *OpenAI) buildRequest(req Request, maxToks int) map[string]interface{} {
	msgs := make([]map[string]interface{}, 0, len(req.Messages)+1)

	if req.SystemPrompt != "" {
		msgs = append(msgs, map[string]interface{}{
			"role":    "system",
			"content": req.SystemPrompt,
		})
	}

	for _, m := range req.Messages {
		msgs = append(msgs, convertMessage(m))
	}

	result := map[string]interface{}{
		"model":      o.model,
		"max_tokens": maxToks,
		"messages":   msgs,
		"stream":     true,
		"stream_options": map[string]interface{}{
			"include_usage": true,
		},
	}

	if len(req.Tools) > 0 {
		oaiTools := make([]map[string]interface{}, 0, len(req.Tools))
		for _, t := range req.Tools {
			oaiTools = append(oaiTools, map[string]interface{}{
				"type": "function",
				"function": map[string]interface{}{
					"name":        t.Name,
					"description": t.Description,
					"parameters":  t.InputSchema,
				},
			})
		}
		result["tools"] = oaiTools
	}

	return result
}

func convertMessage(m Message) map[string]interface{} {
	if m.Role == "assistant" {
		return convertAssistantMessage(m)
	}

	// Check for tool_result blocks (mapped to role "tool" in OpenAI).
	for _, b := range m.Content {
		if b.Type == "tool_result" {
			return map[string]interface{}{
				"role":         "tool",
				"tool_call_id": b.ToolUseID,
				"content":      b.Content,
			}
		}
	}

	// Regular user message — concatenate text blocks.
	var texts []string
	for _, b := range m.Content {
		if b.Type == "text" {
			texts = append(texts, b.Text)
		}
	}
	return map[string]interface{}{
		"role":    m.Role,
		"content": strings.Join(texts, "\n"),
	}
}

func convertAssistantMessage(m Message) map[string]interface{} {
	msg := map[string]interface{}{"role": "assistant"}

	var textParts []string
	var toolCalls []map[string]interface{}

	for _, b := range m.Content {
		switch b.Type {
		case "text":
			textParts = append(textParts, b.Text)
		case "tool_use":
			toolCalls = append(toolCalls, map[string]interface{}{
				"id":   b.ID,
				"type": "function",
				"function": map[string]interface{}{
					"name":      b.Name,
					"arguments": string(b.Input),
				},
			})
		}
	}

	if len(textParts) > 0 {
		msg["content"] = strings.Join(textParts, "\n")
	}
	if len(toolCalls) > 0 {
		msg["tool_calls"] = toolCalls
	}
	return msg
}

func (o *OpenAI) parseSSE(r io.Reader, eventCh chan<- StreamEvent, errCh chan<- error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 1<<20), 1<<20)

	var result Response
	// Track active tool calls by index.
	activeTools := make(map[int]*struct {
		id   string
		name string
		args strings.Builder
	})

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			eventCh <- StreamEvent{Type: "done", Response: &result}
			return
		}

		var chunk struct {
			ID      string `json:"id"`
			Model   string `json:"model"`
			Choices []struct {
				Delta struct {
					Content   *string `json:"content"`
					ToolCalls []struct {
						Index    int    `json:"index"`
						ID       string `json:"id,omitempty"`
						Function struct {
							Name      string `json:"name,omitempty"`
							Arguments string `json:"arguments,omitempty"`
						} `json:"function"`
					} `json:"tool_calls,omitempty"`
				} `json:"delta"`
				FinishReason *string `json:"finish_reason"`
			} `json:"choices"`
			Usage *struct {
				PromptTokens     int `json:"prompt_tokens"`
				CompletionTokens int `json:"completion_tokens"`
			} `json:"usage"`
		}
		if json.Unmarshal([]byte(data), &chunk) != nil {
			continue
		}

		if chunk.ID != "" {
			result.ID = chunk.ID
		}
		if chunk.Model != "" {
			result.Model = chunk.Model
		}
		if chunk.Usage != nil {
			result.InputTokens = chunk.Usage.PromptTokens
			result.OutputTokens = chunk.Usage.CompletionTokens
		}

		if len(chunk.Choices) == 0 {
			continue
		}
		choice := chunk.Choices[0]

		if choice.Delta.Content != nil && *choice.Delta.Content != "" {
			eventCh <- StreamEvent{Type: "text", Text: *choice.Delta.Content}
		}

		for _, tc := range choice.Delta.ToolCalls {
			at, exists := activeTools[tc.Index]
			if !exists {
				at = &struct {
					id   string
					name string
					args strings.Builder
				}{}
				activeTools[tc.Index] = at
			}
			if tc.ID != "" {
				at.id = tc.ID
			}
			if tc.Function.Name != "" {
				at.name = tc.Function.Name
				eventCh <- StreamEvent{
					Type:     "tool_use_start",
					ToolID:   at.id,
					ToolName: at.name,
				}
			}
			if tc.Function.Arguments != "" {
				at.args.WriteString(tc.Function.Arguments)
				eventCh <- StreamEvent{Type: "tool_input_delta", PartialJSON: tc.Function.Arguments}
			}
		}

		if choice.FinishReason != nil {
			result.StopReason = *choice.FinishReason

			// Finalize all tool call blocks.
			for _, at := range activeTools {
				result.ContentBlocks = append(result.ContentBlocks, ContentBlock{
					Type:  "tool_use",
					ID:    at.id,
					Name:  at.name,
					Input: json.RawMessage(at.args.String()),
				})
				eventCh <- StreamEvent{Type: "tool_use_end"}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		errCh <- fmt.Errorf("read stream: %w", err)
	}
}
