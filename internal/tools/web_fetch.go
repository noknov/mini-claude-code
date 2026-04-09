package tools

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const maxFetchSize = 512 * 1024 // 512 KB

type WebFetchTool struct{}

type webFetchInput struct {
	URL string `json:"url"`
}

func (t *WebFetchTool) Name() string { return "WebFetch" }

func (t *WebFetchTool) Description() string {
	return `Fetch content from a URL and return it as text. Useful for reading documentation, API responses, or web pages. Returns raw text content (HTML tags stripped where possible).`
}

func (t *WebFetchTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"url": {
				"type": "string",
				"description": "The URL to fetch"
			}
		},
		"required": ["url"]
	}`)
}

func (t *WebFetchTool) NeedsPermission(_ json.RawMessage) bool { return true }

func (t *WebFetchTool) FormatPermissionRequest(input json.RawMessage) string {
	var in webFetchInput
	_ = json.Unmarshal(input, &in)
	return fmt.Sprintf("Fetch URL: %s", in.URL)
}

func (t *WebFetchTool) Execute(input json.RawMessage, _ string) (string, error) {
	var in webFetchInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}
	if in.URL == "" {
		return "", fmt.Errorf("url is required")
	}
	if !strings.HasPrefix(in.URL, "http://") && !strings.HasPrefix(in.URL, "https://") {
		in.URL = "https://" + in.URL
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(in.URL)
	if err != nil {
		return "", fmt.Errorf("fetch failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxFetchSize))
	if err != nil {
		return "", fmt.Errorf("read body: %w", err)
	}

	return string(body), nil
}
