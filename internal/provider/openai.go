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

const openaiDefaultURL = "https://api.openai.com"

// OpenAI implements Provider for the OpenAI Chat Completions API.
// Also works with any OpenAI-compatible endpoint (Ollama, vLLM, Together, etc.).
type OpenAI struct {
	apiKey        string
	model         string
	baseURL       string
	contextWindow int
	client        *http.Client
}

func NewOpenAI(apiKey, model, baseURL string, contextWindow int) *OpenAI {
	if baseURL == "" {
		baseURL = openaiDefaultURL
	}
	if contextWindow <= 0 {
		contextWindow = 128000
	}
	return &OpenAI{
		apiKey:        apiKey,
		model:         model,
		baseURL:       strings.TrimRight(baseURL, "/"),
		contextWindow: contextWindow,
		client:        &http.Client{Timeout: 10 * time.Minute},
	}
}

func (o *OpenAI) Name() string       { return "openai" }
func (o *OpenAI) Model() string      { return o.model }
func (o *OpenAI) SetModel(m string)  { o.model = m }
func (o *OpenAI) ContextWindow() int { return o.contextWindow }

func (o *OpenAI) SendStream(ctx context.Context, req Request) (<-chan StreamEvent, <-chan error) {
	eventCh := make(chan StreamEvent, 64)
	errCh := make(chan error, 1)

	apiReq := o.buildRequest(req)

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
func (o *OpenAI) buildRequest(req Request) map[string]interface{} {
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
		"model":    o.model,
		"messages": msgs,
		"stream":   true,
		"stream_options": map[string]interface{}{
			"include_usage": true,
		},
	}

	if req.MaxTokens > 0 {
		result["max_tokens"] = req.MaxTokens
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

	switch {
	case len(textParts) > 0:
		msg["content"] = strings.Join(textParts, "\n")
	case len(toolCalls) == 0:
		msg["content"] = ""
	}
	if len(toolCalls) > 0 {
		msg["tool_calls"] = toolCalls
	}
	return msg
}

// ---------------------------------------------------------------------------
// SSE stream parsing
// ---------------------------------------------------------------------------

// sseChunk maps the JSON structure of a single SSE data line.
type sseChunk struct {
	ID      string           `json:"id"`
	Model   string           `json:"model"`
	Choices []sseChunkChoice `json:"choices"`
	Usage   *sseUsage        `json:"usage"`
}

type sseChunkChoice struct {
	Delta        sseChunkDelta `json:"delta"`
	FinishReason *string       `json:"finish_reason"`
}

type sseChunkDelta struct {
	Content   *string            `json:"content"`
	ToolCalls []sseChunkToolCall `json:"tool_calls,omitempty"`
}

type sseChunkToolCall struct {
	Index    int    `json:"index"`
	ID       string `json:"id,omitempty"`
	Function struct {
		Name      string `json:"name,omitempty"`
		Arguments string `json:"arguments,omitempty"`
	} `json:"function"`
}

type sseUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}

// pendingToolCall accumulates streamed fragments of a single tool call.
type pendingToolCall struct {
	id   string
	name string
	args strings.Builder
}

// streamAccumulator collects streamed content into a final Response.
type streamAccumulator struct {
	result  Response
	textBuf strings.Builder
	tools   map[int]*pendingToolCall
}

func newStreamAccumulator() *streamAccumulator {
	return &streamAccumulator{tools: make(map[int]*pendingToolCall)}
}

func (a *streamAccumulator) flushText() {
	if a.textBuf.Len() == 0 {
		return
	}
	a.result.ContentBlocks = append(a.result.ContentBlocks, ContentBlock{
		Type: "text",
		Text: a.textBuf.String(),
	})
	a.textBuf.Reset()
}

func (a *streamAccumulator) flushTools(eventCh chan<- StreamEvent) {
	for _, tc := range a.tools {
		a.result.ContentBlocks = append(a.result.ContentBlocks, ContentBlock{
			Type:  "tool_use",
			ID:    tc.id,
			Name:  tc.name,
			Input: json.RawMessage(tc.args.String()),
		})
		eventCh <- StreamEvent{Type: "tool_use_end"}
	}
}

func (a *streamAccumulator) finalize(eventCh chan<- StreamEvent) {
	a.flushText()
	eventCh <- StreamEvent{Type: "done", Response: &a.result}
}

func (o *OpenAI) parseSSE(r io.Reader, eventCh chan<- StreamEvent, errCh chan<- error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 1<<20), 1<<20)
	acc := newStreamAccumulator()

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			acc.finalize(eventCh)
			return
		}

		var chunk sseChunk
		if json.Unmarshal([]byte(data), &chunk) != nil {
			continue
		}

		acc.updateMetadata(&chunk)

		if len(chunk.Choices) == 0 {
			continue
		}
		choice := chunk.Choices[0]

		acc.processTextDelta(choice.Delta.Content, eventCh)
		acc.processToolDeltas(choice.Delta.ToolCalls, eventCh)

		if choice.FinishReason != nil {
			acc.result.StopReason = *choice.FinishReason
			acc.flushText()
			acc.flushTools(eventCh)
		}
	}

	if err := scanner.Err(); err != nil {
		errCh <- fmt.Errorf("read stream: %w", err)
	}
}

func (a *streamAccumulator) updateMetadata(chunk *sseChunk) {
	if chunk.ID != "" {
		a.result.ID = chunk.ID
	}
	if chunk.Model != "" {
		a.result.Model = chunk.Model
	}
	if chunk.Usage != nil {
		a.result.InputTokens = chunk.Usage.PromptTokens
		a.result.OutputTokens = chunk.Usage.CompletionTokens
	}
}

func (a *streamAccumulator) processTextDelta(content *string, eventCh chan<- StreamEvent) {
	if content == nil || *content == "" {
		return
	}
	a.textBuf.WriteString(*content)
	eventCh <- StreamEvent{Type: "text", Text: *content}
}

func (a *streamAccumulator) processToolDeltas(deltas []sseChunkToolCall, eventCh chan<- StreamEvent) {
	for _, tc := range deltas {
		pending := a.getOrCreateTool(tc.Index)

		if tc.ID != "" {
			pending.id = tc.ID
		}
		if tc.Function.Name != "" {
			pending.name = tc.Function.Name
			eventCh <- StreamEvent{Type: "tool_use_start", ToolID: pending.id, ToolName: pending.name}
		}
		if tc.Function.Arguments != "" {
			pending.args.WriteString(tc.Function.Arguments)
			eventCh <- StreamEvent{Type: "tool_input_delta", PartialJSON: tc.Function.Arguments}
		}
	}
}

func (a *streamAccumulator) getOrCreateTool(index int) *pendingToolCall {
	if tc, ok := a.tools[index]; ok {
		return tc
	}
	tc := &pendingToolCall{}
	a.tools[index] = tc
	return tc
}
