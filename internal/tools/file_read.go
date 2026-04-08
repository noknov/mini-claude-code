package tools

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const maxFileSize = 1024 * 1024 // 1MB

type FileReadTool struct{}

type fileReadInput struct {
	Path   string `json:"path"`
	Offset int    `json:"offset,omitempty"`
	Limit  int    `json:"limit,omitempty"`
}

func (t *FileReadTool) Name() string { return "Read" }

func (t *FileReadTool) Description() string {
	return `Read the contents of a file. Supports text files with optional line range selection.
Use offset and limit to read specific portions of large files.
Lines are 1-indexed in the output.`
}

func (t *FileReadTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {
				"type": "string",
				"description": "Absolute path to the file to read"
			},
			"offset": {
				"type": "integer",
				"description": "Line number to start reading from (1-indexed)"
			},
			"limit": {
				"type": "integer",
				"description": "Number of lines to read"
			}
		},
		"required": ["path"]
	}`)
}

func (t *FileReadTool) NeedsPermission(_ json.RawMessage) bool {
	return false
}

func (t *FileReadTool) FormatPermissionRequest(input json.RawMessage) string {
	var in fileReadInput
	json.Unmarshal(input, &in)
	return fmt.Sprintf("Read file: %s", in.Path)
}

func (t *FileReadTool) Execute(input json.RawMessage, workDir string) (string, error) {
	var in fileReadInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}

	path := resolvePath(in.Path, workDir)

	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("file not found: %s", path)
		}
		return "", fmt.Errorf("stat file: %w", err)
	}

	if info.IsDir() {
		return readDirectory(path)
	}

	if info.Size() > maxFileSize {
		return "", fmt.Errorf("file too large (%d bytes, max %d)", info.Size(), maxFileSize)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read file: %w", err)
	}

	lines := strings.Split(string(data), "\n")

	if in.Offset > 0 || in.Limit > 0 {
		start := 0
		if in.Offset > 0 {
			start = in.Offset - 1
		}
		if start >= len(lines) {
			return "", fmt.Errorf("offset %d exceeds file length (%d lines)", in.Offset, len(lines))
		}

		end := len(lines)
		if in.Limit > 0 {
			end = start + in.Limit
			if end > len(lines) {
				end = len(lines)
			}
		}
		lines = lines[start:end]

		var sb strings.Builder
		for i, line := range lines {
			lineNum := start + i + 1
			sb.WriteString(fmt.Sprintf("%6d|%s\n", lineNum, line))
		}
		return sb.String(), nil
	}

	var sb strings.Builder
	for i, line := range lines {
		sb.WriteString(fmt.Sprintf("%6d|%s\n", i+1, line))
	}
	return sb.String(), nil
}

func readDirectory(path string) (string, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return "", fmt.Errorf("read directory: %w", err)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Directory: %s\n\n", path))
	for _, entry := range entries {
		prefix := "  "
		if entry.IsDir() {
			prefix = "📁"
		}
		sb.WriteString(fmt.Sprintf("%s %s\n", prefix, entry.Name()))
	}
	return sb.String(), nil
}

func resolvePath(path, workDir string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Clean(filepath.Join(workDir, path))
}
