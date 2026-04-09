// Package session manages conversation state: message history, token
// accounting, and session identity.
package session

import (
	"time"

	"github.com/noknov/mini-claude-code/internal/cost"
	"github.com/noknov/mini-claude-code/internal/provider"
)

// Session holds the conversation state.
type Session struct {
	ID        string
	Title     string
	CreatedAt time.Time
	Messages  []provider.Message
	Cost      *cost.Tracker
}

func New(id string) *Session {
	return &Session{
		ID:        id,
		CreatedAt: time.Now(),
		Cost:      cost.NewTracker(),
	}
}

// ---------------------------------------------------------------------------
// Message management
// ---------------------------------------------------------------------------

func (s *Session) AddUserMessage(text string) {
	s.Messages = append(s.Messages, provider.Message{
		Role:    "user",
		Content: []provider.ContentBlock{{Type: "text", Text: text}},
	})
}

func (s *Session) AddAssistantMessage(blocks []provider.ContentBlock) {
	s.Messages = append(s.Messages, provider.Message{
		Role:    "assistant",
		Content: blocks,
	})
}

func (s *Session) AddToolResult(toolUseID, content string, isError bool) {
	s.Messages = append(s.Messages, provider.Message{
		Role: "user",
		Content: []provider.ContentBlock{{
			Type:      "tool_result",
			ToolUseID: toolUseID,
			Content:   content,
			IsError:   isError,
		}},
	})
}

// SetMessages replaces the message history (used after compaction).
func (s *Session) SetMessages(msgs []provider.Message) {
	s.Messages = msgs
}

// ---------------------------------------------------------------------------
// Usage tracking
// ---------------------------------------------------------------------------

func (s *Session) UpdateUsage(model string, inputTokens, outputTokens int) {
	s.Cost.Add(model, inputTokens, outputTokens)
}

func (s *Session) TotalTokens() (input, output int) {
	return s.Cost.TotalTokens()
}

func (s *Session) EstimateCost() float64 {
	return s.Cost.EstimateCost()
}

// ---------------------------------------------------------------------------
// Reset
// ---------------------------------------------------------------------------

func (s *Session) Clear() {
	s.Messages = nil
	s.Cost.Clear()
}
