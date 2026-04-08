package session

import (
	"github.com/noknov/mini-claude-code/internal/api"
)

// Session holds the conversation state
type Session struct {
	Messages    []api.Message
	InputTokens  int
	OutputTokens int
}

func New() *Session {
	return &Session{}
}

func (s *Session) AddUserMessage(text string) {
	s.Messages = append(s.Messages, api.Message{
		Role: "user",
		Content: []api.ContentBlock{
			{Type: "text", Text: text},
		},
	})
}

func (s *Session) AddAssistantMessage(blocks []api.ContentBlock) {
	s.Messages = append(s.Messages, api.Message{
		Role:    "assistant",
		Content: blocks,
	})
}

func (s *Session) AddToolResult(toolUseID, result string, isError bool) {
	s.Messages = append(s.Messages, api.Message{
		Role: "user",
		Content: []api.ContentBlock{
			{
				Type:      "tool_result",
				ToolUseID: toolUseID,
				Content:   result,
				IsError:   isError,
			},
		},
	})
}

func (s *Session) UpdateUsage(input, output int) {
	s.InputTokens += input
	s.OutputTokens += output
}

func (s *Session) Clear() {
	s.Messages = nil
	s.InputTokens = 0
	s.OutputTokens = 0
}

// EstimateCost returns estimated cost in USD
func (s *Session) EstimateCost() float64 {
	// Approximate pricing for Claude Sonnet
	inputCost := float64(s.InputTokens) / 1_000_000 * 3.0
	outputCost := float64(s.OutputTokens) / 1_000_000 * 15.0
	return inputCost + outputCost
}
