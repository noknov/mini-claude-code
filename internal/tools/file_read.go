package tools

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const maxReadSize = 1 << 20 // 1 MB

type FileReadTool struct{}

type fileReadInput struct {
	Path   string `json:"path"`
	Offset int    `json:"offset,omitempty"`
	Limit  int    `json:"limit,omitempty"`
}

func (t *FileReadTool) Name() string { return "Read" }

func (t *FileReadTool) Description() string {
	return `Read a file or list a directory.
For files: returns content with line numbers (1-indexed).
Use offset and limit to read a specific range of lines.`
}

func (t *FileReadTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {
				"type": "string",
				"description": "Absolute path to the file or directory"
			},
			"offset": {
				"type": "integer",
				"description": "Starting line number (1-indexed)"
			},
			"limit": {
				"type": "integer",
				"description": "Maximum number of lines to read"
			}
		},
		"required": ["path"]
	}`)
}

func (t *FileReadTool) NeedsPermission(_ json.RawMessage) bool { return false }

func (t *FileReadTool) FormatPermissionRequest(input json.RawMessage) string {
	var in fileReadInput
	_ = json.Unmarshal(input, &in)
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
		return "", fmt.Errorf("stat: %w", err)
	}

	if info.IsDir() {
		return readDirectory(path)
	}
	if info.Size() > maxReadSize {
		return "", fmt.Errorf("file too large (%d bytes, max %d)", info.Size(), maxReadSize)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read file: %w", err)
	}

	lines := strings.Split(string(data), "\n")
	start, end := lineRange(lines, in.Offset, in.Limit)

	var sb strings.Builder
	for i := start; i < end; i++ {
		fmt.Fprintf(&sb, "%6d|%s\n", i+1, lines[i])
	}
	return sb.String(), nil
}

// lineRange converts 1-indexed offset/limit into 0-indexed [start, end).
func lineRange(lines []string, offset, limit int) (start, end int) {
	start = 0
	if offset > 0 {
		start = offset - 1
	}
	if start > len(lines) {
		start = len(lines)
	}
	end = len(lines)
	if limit > 0 && start+limit < end {
		end = start + limit
	}
	return start, end
}

func readDirectory(path string) (string, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return "", fmt.Errorf("read directory: %w", err)
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "Directory: %s\n\n", path)
	for _, e := range entries {
		indicator := "  "
		if e.IsDir() {
			indicator = "d "
		}
		sb.WriteString(indicator + e.Name() + "\n")
	}
	return sb.String(), nil
}

func resolvePath(path, workDir string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Clean(filepath.Join(workDir, path))
}
