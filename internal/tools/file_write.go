package tools

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type FileWriteTool struct{}

type fileWriteInput struct {
	Path    string `json:"path"`
	Content string `json:"contents"`
}

func (t *FileWriteTool) Name() string { return "Write" }

func (t *FileWriteTool) Description() string {
	return `Write content to a file, creating it if it doesn't exist or overwriting if it does.
Parent directories are created automatically.
Prefer the Edit tool for modifying existing files.`
}

func (t *FileWriteTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {
				"type": "string",
				"description": "Absolute path to the file to write"
			},
			"contents": {
				"type": "string",
				"description": "The content to write"
			}
		},
		"required": ["path", "contents"]
	}`)
}

func (t *FileWriteTool) NeedsPermission(_ json.RawMessage) bool { return true }

func (t *FileWriteTool) FormatPermissionRequest(input json.RawMessage) string {
	var in fileWriteInput
	_ = json.Unmarshal(input, &in)
	return fmt.Sprintf("Write to file: %s", in.Path)
}

func (t *FileWriteTool) Execute(input json.RawMessage, workDir string) (string, error) {
	var in fileWriteInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}

	path := resolvePath(in.Path, workDir)

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return "", fmt.Errorf("create directory: %w", err)
	}
	if err := os.WriteFile(path, []byte(in.Content), 0644); err != nil {
		return "", fmt.Errorf("write file: %w", err)
	}
	return fmt.Sprintf("Wrote %d bytes to %s", len(in.Content), path), nil
}
