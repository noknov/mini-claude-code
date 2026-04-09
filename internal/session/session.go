package session

import "github.com/noknov/mini-claude-code/internal/provider"

// Session holds the conversation state: message history and token accounting.
type Session struct {
	Messages     []provider.Message
	InputTokens  int
	OutputTokens int
}

func New() *Session {
	return &Session{}
}

func (s *Session) AddUserMessage(text string) {
	s.Messages = append(s.Messages, provider.Message{
		Role: "user",
		Content: []provider.ContentBlock{
			{Type: "text", Text: text},
		},
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
		Content: []provider.ContentBlock{
			{
				Type:      "tool_result",
				ToolUseID: toolUseID,
				Content:   content,
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

// EstimateCost returns an approximate cost in USD based on Claude Sonnet pricing.
func (s *Session) EstimateCost() float64 {
	const (
		inputPricePerMTok  = 3.0
		outputPricePerMTok = 15.0
	)
	return float64(s.InputTokens)/1e6*inputPricePerMTok +
		float64(s.OutputTokens)/1e6*outputPricePerMTok
}
