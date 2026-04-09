package provider

import (
	"context"
	"encoding/json"
)

// Provider is the abstraction every LLM backend must implement.
type Provider interface {
	Name() string
	Model() string
	SetModel(model string)
	ContextWindow() int

	// SendStream sends a conversation and streams back events.
	// The caller ranges over the returned channel; it is closed when the
	// response is complete or an error occurs.
	SendStream(ctx context.Context, req Request) (<-chan StreamEvent, <-chan error)
}

// Request is what the query engine sends to a provider.
type Request struct {
	SystemPrompt string
	Messages     []Message
	Tools        []ToolDef
	MaxTokens    int
}

// Message is a single conversation turn.
type Message struct {
	Role    string         `json:"role"`
	Content []ContentBlock `json:"content"`
}

// ContentBlock is a polymorphic content element inside a Message.
type ContentBlock struct {
	Type string `json:"type"`

	// text
	Text string `json:"text,omitempty"`

	// tool_use (assistant requests a tool call)
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`

	// tool_result (user returns tool output)
	ToolUseID string `json:"tool_use_id,omitempty"`
	Content   string `json:"content,omitempty"`
	IsError   bool   `json:"is_error,omitempty"`
}

// ToolDef describes a tool the model can call.
type ToolDef struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema interface{} `json:"input_schema"`
}

// StreamEvent is a single incremental update from the provider.
type StreamEvent struct {
	Type string // "text", "tool_use_start", "tool_input_delta", "tool_use_end", "done"

	Text string // for "text"

	ToolID   string // for "tool_use_start"
	ToolName string

	PartialJSON string // for "tool_input_delta"

	Response *Response // for "done"
}

// Response is the fully-assembled result after the stream ends.
type Response struct {
	ID            string
	Model         string
	StopReason    string
	ContentBlocks []ContentBlock
	InputTokens   int
	OutputTokens  int
}
