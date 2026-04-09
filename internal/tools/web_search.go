package tools

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type WebSearchTool struct{}

type webSearchInput struct {
	Query string `json:"query"`
}

func (t *WebSearchTool) Name() string { return "WebSearch" }

func (t *WebSearchTool) Description() string {
	return `Search the web for real-time information. Returns search results with titles, URLs, and snippets. Use when you need up-to-date information not available in your training data.`
}

func (t *WebSearchTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"query": {
				"type": "string",
				"description": "The search query"
			}
		},
		"required": ["query"]
	}`)
}

func (t *WebSearchTool) NeedsPermission(_ json.RawMessage) bool { return true }

func (t *WebSearchTool) FormatPermissionRequest(input json.RawMessage) string {
	var in webSearchInput
	_ = json.Unmarshal(input, &in)
	return fmt.Sprintf("Web search: %s", in.Query)
}

func (t *WebSearchTool) Execute(input json.RawMessage, _ string) (string, error) {
	var in webSearchInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}
	if in.Query == "" {
		return "", fmt.Errorf("query is required")
	}

	// Use DuckDuckGo HTML search as a simple no-API-key search backend.
	searchURL := "https://html.duckduckgo.com/html/?q=" + url.QueryEscape(in.Query)

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(searchURL)
	if err != nil {
		return "", fmt.Errorf("search failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	results := extractSearchResults(string(body))
	if results == "" {
		return "No results found for: " + in.Query, nil
	}
	return results, nil
}

// extractSearchResults does a basic extraction from DuckDuckGo HTML.
func extractSearchResults(html string) string {
	var sb strings.Builder
	count := 0

	for _, chunk := range strings.Split(html, "result__a") {
		if count >= 10 {
			break
		}
		if !strings.Contains(chunk, "href=") {
			continue
		}

		href := extractAttr(chunk, "href=\"", "\"")
		title := extractTextContent(chunk)
		if href == "" || title == "" {
			continue
		}

		count++
		fmt.Fprintf(&sb, "%d. %s\n   %s\n\n", count, title, href)
	}

	return sb.String()
}

func extractAttr(s, prefix, suffix string) string {
	start := strings.Index(s, prefix)
	if start < 0 {
		return ""
	}
	s = s[start+len(prefix):]
	end := strings.Index(s, suffix)
	if end < 0 {
		return ""
	}
	return s[:end]
}

func extractTextContent(s string) string {
	// Strip HTML tags for a rough text extraction.
	var result strings.Builder
	inTag := false
	for _, r := range s {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
		case !inTag:
			result.WriteRune(r)
		}
	}
	text := strings.TrimSpace(result.String())
	if len(text) > 200 {
		text = text[:200]
	}
	return text
}
