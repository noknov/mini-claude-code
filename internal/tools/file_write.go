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
	return `Write content to a file. Creates the file if it doesn't exist, or overwrites if it does.
Parent directories are created automatically.`
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
				"description": "The content to write to the file"
			}
		},
		"required": ["path", "contents"]
	}`)
}

func (t *FileWriteTool) NeedsPermission(_ json.RawMessage) bool {
	return true
}

func (t *FileWriteTool) FormatPermissionRequest(input json.RawMessage) string {
	var in fileWriteInput
	json.Unmarshal(input, &in)
	return fmt.Sprintf("Write to file: %s", in.Path)
}

func (t *FileWriteTool) Execute(input json.RawMessage, workDir string) (string, error) {
	var in fileWriteInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}

	path := resolvePath(in.Path, workDir)

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("create directory: %w", err)
	}

	if err := os.WriteFile(path, []byte(in.Content), 0644); err != nil {
		return "", fmt.Errorf("write file: %w", err)
	}

	return fmt.Sprintf("Successfully wrote %d bytes to %s", len(in.Content), path), nil
}
