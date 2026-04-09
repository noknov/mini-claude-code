package tools

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const maxGlobResults = 100

var globSkipDirs = map[string]bool{
	".git": true, "node_modules": true, "__pycache__": true,
	".next": true, "vendor": true, ".venv": true,
}

type GlobTool struct{}

type globInput struct {
	Pattern string `json:"pattern"`
	Path    string `json:"path,omitempty"`
}

func (t *GlobTool) Name() string { return "Glob" }

func (t *GlobTool) Description() string {
	return `Find files matching a glob pattern, sorted by modification time (newest first).
Supports "**/" for recursive matching (e.g. "**/*.go").
Automatically skips .git, node_modules, __pycache__, vendor, .venv.`
}

func (t *GlobTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"pattern": {
				"type": "string",
				"description": "Glob pattern (e.g. \"*.go\", \"**/*.ts\", \"src/**/*.jsx\")"
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
	_ = json.Unmarshal(input, &in)
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
	if !strings.Contains(pattern, "/") && !strings.HasPrefix(pattern, "**/") {
		pattern = "**/" + pattern
	}

	type entry struct {
		path    string
		modTime int64
	}
	var matches []entry

	_ = filepath.Walk(searchDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			if globSkipDirs[filepath.Base(path)] {
				return filepath.SkipDir
			}
			return nil
		}

		relPath, _ := filepath.Rel(searchDir, path)
		if matchGlob(pattern, relPath) {
			matches = append(matches, entry{path: relPath, modTime: info.ModTime().Unix()})
		}
		return nil
	})

	sort.Slice(matches, func(i, j int) bool {
		return matches[i].modTime > matches[j].modTime
	})

	if len(matches) == 0 {
		return "No files found matching pattern", nil
	}

	total := len(matches)
	truncated := total > maxGlobResults
	if truncated {
		matches = matches[:maxGlobResults]
	}

	var sb strings.Builder
	for _, m := range matches {
		sb.WriteString(m.path + "\n")
	}
	header := fmt.Sprintf("Found %d file(s):\n", total)
	if truncated {
		header = fmt.Sprintf("Found %d file(s), showing first %d:\n", total, maxGlobResults)
	}
	return header + sb.String(), nil
}

// matchGlob handles "**/" recursive patterns by splitting on "**/" and
// matching each segment. For simple patterns without "**/" it falls back
// to filepath.Match on the full relative path.
func matchGlob(pattern, path string) bool {
	if !strings.Contains(pattern, "**/") {
		ok, _ := filepath.Match(pattern, path)
		return ok
	}

	parts := strings.SplitN(pattern, "**/", 2)
	prefix := parts[0]
	suffix := parts[1]

	if prefix != "" {
		ok, _ := filepath.Match(strings.TrimSuffix(prefix, "/"), pathHead(path, strings.Count(prefix, "/")))
		if !ok {
			return false
		}
	}

	if suffix == "" {
		return true
	}
	if strings.Contains(suffix, "**/") {
		// Nested **/ — match suffix recursively against every sub-path.
		for i := 0; i < len(path); i++ {
			if path[i] == '/' || i == 0 {
				sub := path
				if i > 0 {
					sub = path[i+1:]
				}
				if matchGlob(suffix, sub) {
					return true
				}
			}
		}
		return false
	}

	// suffix is a simple pattern like "*.go" or "foo/*.go"
	// Match against every possible tail of path.
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' || i == 0 {
			tail := path[i:]
			if i > 0 {
				tail = path[i+1:]
			}
			if ok, _ := filepath.Match(suffix, tail); ok {
				return true
			}
		}
	}
	return false
}

func pathHead(path string, depth int) string {
	if depth <= 0 {
		return ""
	}
	parts := strings.SplitN(path, "/", depth+1)
	if len(parts) <= depth {
		return path
	}
	return strings.Join(parts[:depth], "/")
}
