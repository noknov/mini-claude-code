package tools

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const maxGlobResults = 200

type GlobTool struct{}

type globInput struct {
	Pattern string `json:"pattern"`
	Path    string `json:"path,omitempty"`
}

func (t *GlobTool) Name() string { return "Glob" }

func (t *GlobTool) Description() string {
	return `Find files matching a glob pattern. Returns matching file paths sorted by modification time.
Patterns not starting with "**/" are automatically prepended with "**/" for recursive search.`
}

func (t *GlobTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"pattern": {
				"type": "string",
				"description": "Glob pattern to match files (e.g. \"*.go\", \"**/*.ts\")"
			},
			"path": {
				"type": "string",
				"description": "Directory to search in (defaults to working directory)"
			}
		},
		"required": ["pattern"]
	}`)
}

func (t *GlobTool) NeedsPermission(_ json.RawMessage) bool { return false }

func (t *GlobTool) FormatPermissionRequest(input json.RawMessage) string {
	var in globInput
	json.Unmarshal(input, &in)
	return fmt.Sprintf("Search files: %s", in.Pattern)
}

func (t *GlobTool) Execute(input json.RawMessage, workDir string) (string, error) {
	var in globInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}

	searchDir := workDir
	if in.Path != "" {
		searchDir = resolvePath(in.Path, workDir)
	}

	pattern := in.Pattern
	if !strings.HasPrefix(pattern, "**/") && !strings.HasPrefix(pattern, "/") {
		pattern = "**/" + pattern
	}

	type fileEntry struct {
		path    string
		modTime int64
	}

	var matches []fileEntry

	filepath.Walk(searchDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			base := filepath.Base(path)
			if base == ".git" || base == "node_modules" || base == "__pycache__" || base == ".next" {
				return filepath.SkipDir
			}
			return nil
		}

		relPath, _ := filepath.Rel(searchDir, path)
		matched, _ := filepath.Match(filepath.Base(pattern), filepath.Base(relPath))
		if matched {
			matches = append(matches, fileEntry{path: relPath, modTime: info.ModTime().Unix()})
		}
		return nil
	})

	sort.Slice(matches, func(i, j int) bool {
		return matches[i].modTime > matches[j].modTime
	})

	if len(matches) > maxGlobResults {
		matches = matches[:maxGlobResults]
	}

	var sb strings.Builder
	for _, m := range matches {
		sb.WriteString(m.path)
		sb.WriteString("\n")
	}

	if len(matches) == 0 {
		return "No files found matching pattern", nil
	}

	return fmt.Sprintf("Found %d files:\n%s", len(matches), sb.String()), nil
}
