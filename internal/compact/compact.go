// Package compact implements conversation compaction (auto, full, micro).
//
// When the conversation approaches the context window limit, older messages
// are summarized to free space while preserving key context.
package compact

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/noknov/mini-claude-code/internal/provider"
)

const (
	charsPerToken        = 4
	defaultContextWindow = 200000
	summaryReserve       = 20000
)

// ---------------------------------------------------------------------------
// Compactor
// ---------------------------------------------------------------------------

// Compactor handles conversation compaction at multiple levels.
type Compactor struct {
	provider      provider.Provider
	contextWindow int
}

func New(prov provider.Provider) *Compactor {
	return &Compactor{provider: prov, contextWindow: defaultContextWindow}
}

// ---------------------------------------------------------------------------
// Auto-compact check
// ---------------------------------------------------------------------------

// ShouldCompact estimates whether the conversation needs compaction.
func (c *Compactor) ShouldCompact(messages []provider.Message) bool {
	return c.estimateTokens(messages) > c.contextWindow-summaryReserve
}

// ---------------------------------------------------------------------------
// Full compact
// ---------------------------------------------------------------------------

// Compact summarizes older messages, keeping recent ones intact.
func (c *Compactor) Compact(ctx context.Context, messages []provider.Message) ([]provider.Message, error) {
	if len(messages) < 4 {
		return messages, nil
	}

	keepCount := min(4, len(messages)/2)
	toSummarize := messages[:len(messages)-keepCount]
	toKeep := messages[len(messages)-keepCount:]

	summary, err := c.summarize(ctx, toSummarize)
	if err != nil {
		return messages, fmt.Errorf("compact: %w", err)
	}

	return buildCompactedMessages(summary, toKeep), nil
}

func buildCompactedMessages(summary string, recent []provider.Message) []provider.Message {
	result := []provider.Message{
		{Role: "user", Content: []provider.ContentBlock{
			{Type: "text", Text: "[Previous conversation summarized]\n\n" + summary},
		}},
		{Role: "assistant", Content: []provider.ContentBlock{
			{Type: "text", Text: "Understood. I have the context from our previous conversation. How can I help you next?"},
		}},
	}
	return append(result, recent...)
}

// ---------------------------------------------------------------------------
// Micro-compact (trim old tool results without full summarization)
// ---------------------------------------------------------------------------

// MicroCompact trims old tool results to save tokens.
func (c *Compactor) MicroCompact(messages []provider.Message) []provider.Message {
	if len(messages) < 10 {
		return messages
	}

	result := make([]provider.Message, len(messages))
	copy(result, messages)

	boundary := len(result) / 2
	for i := 0; i < boundary; i++ {
		trimToolResults(&result[i])
	}
	return result
}

func trimToolResults(msg *provider.Message) {
	const maxLen = 500
	for j := range msg.Content {
		block := &msg.Content[j]
		if block.Type == "tool_result" && len(block.Content) > maxLen {
			half := maxLen / 2
			block.Content = block.Content[:half] + "\n... [trimmed] ...\n" + block.Content[len(block.Content)-half:]
		}
	}
}

// ---------------------------------------------------------------------------
// Summarization (calls the LLM)
// ---------------------------------------------------------------------------

func (c *Compactor) summarize(ctx context.Context, messages []provider.Message) (string, error) {
	conversation := formatConversation(messages)

	req := provider.Request{
		SystemPrompt: summarySystemPrompt,
		Messages: []provider.Message{{
			Role:    "user",
			Content: []provider.ContentBlock{{Type: "text", Text: "Summarize this conversation:\n\n" + conversation}},
		}},
		MaxTokens: 4096,
	}

	return collectStreamText(ctx, c.provider, req)
}

func formatConversation(messages []provider.Message) string {
	var sb strings.Builder
	for _, m := range messages {
		fmt.Fprintf(&sb, "[%s]: ", m.Role)
		for _, b := range m.Content {
			switch b.Type {
			case "text":
				sb.WriteString(b.Text)
			case "tool_use":
				fmt.Fprintf(&sb, "[Called tool: %s]", b.Name)
			case "tool_result":
				content := b.Content
				if len(content) > 200 {
					content = content[:200] + "..."
				}
				fmt.Fprintf(&sb, "[Tool result: %s]", content)
			}
			sb.WriteByte(' ')
		}
		sb.WriteByte('\n')
	}
	return sb.String()
}

func collectStreamText(ctx context.Context, prov provider.Provider, req provider.Request) (string, error) {
	eventCh, errCh := prov.SendStream(ctx, req)
	var result strings.Builder
	for {
		select {
		case evt, ok := <-eventCh:
			if !ok {
				return result.String(), nil
			}
			if evt.Type == "text" && evt.Text != "" {
				result.WriteString(evt.Text)
			}
		case err, ok := <-errCh:
			if ok && err != nil {
				return "", err
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Token estimation
// ---------------------------------------------------------------------------

func (c *Compactor) estimateTokens(messages []provider.Message) int {
	total := 0
	for _, m := range messages {
		data, _ := json.Marshal(m)
		total += len(data) / charsPerToken
	}
	return total
}

const summarySystemPrompt = `You are a conversation summarizer. Produce a concise summary that preserves:
1. Key decisions and their rationale
2. Important code changes and file paths
3. Current task state and next steps
4. Any errors encountered and how they were resolved

Be factual and specific. Include file paths, function names, and technical details.`
