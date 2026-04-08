package tools

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

type FileEditTool struct{}

type fileEditInput struct {
	Path       string `json:"path"`
	OldString  string `json:"old_string"`
	NewString  string `json:"new_string"`
	ReplaceAll bool   `json:"replace_all,omitempty"`
}

func (t *FileEditTool) Name() string { return "Edit" }

func (t *FileEditTool) Description() string {
	return `Edit a file by replacing an exact string match with new content.
The old_string must uniquely identify the text to replace (unless replace_all is true).
Use this for surgical edits rather than rewriting entire files.`
}

func (t *FileEditTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {
				"type": "string",
				"description": "Absolute path to the file to edit"
			},
			"old_string": {
				"type": "string",
				"description": "The exact text to find and replace"
			},
			"new_string": {
				"type": "string",
				"description": "The replacement text"
			},
			"replace_all": {
				"type": "boolean",
				"description": "Replace all occurrences (default: false)"
			}
		},
		"required": ["path", "old_string", "new_string"]
	}`)
}

func (t *FileEditTool) NeedsPermission(_ json.RawMessage) bool {
	return true
}

func (t *FileEditTool) FormatPermissionRequest(input json.RawMessage) string {
	var in fileEditInput
	json.Unmarshal(input, &in)
	return fmt.Sprintf("Edit file: %s", in.Path)
}

func (t *FileEditTool) Execute(input json.RawMessage, workDir string) (string, error) {
	var in fileEditInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}

	path := resolvePath(in.Path, workDir)

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("file not found: %s", path)
		}
		return "", fmt.Errorf("read file: %w", err)
	}

	content := string(data)

	if in.OldString == in.NewString {
		return "", fmt.Errorf("old_string and new_string are identical")
	}

	count := strings.Count(content, in.OldString)
	if count == 0 {
		return "", fmt.Errorf("old_string not found in file")
	}

	if count > 1 && !in.ReplaceAll {
		return "", fmt.Errorf("old_string found %d times; use replace_all or provide more context to make it unique", count)
	}

	var newContent string
	if in.ReplaceAll {
		newContent = strings.ReplaceAll(content, in.OldString, in.NewString)
	} else {
		newContent = strings.Replace(content, in.OldString, in.NewString, 1)
	}

	if err := os.WriteFile(path, []byte(newContent), 0644); err != nil {
		return "", fmt.Errorf("write file: %w", err)
	}

	replacements := count
	if !in.ReplaceAll {
		replacements = 1
	}
	return fmt.Sprintf("Replaced %d occurrence(s) in %s", replacements, path), nil
}
