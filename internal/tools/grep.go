package tools

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

const maxGrepLines = 5000

type GrepTool struct{}

type grepInput struct {
	Pattern string `json:"pattern"`
	Path    string `json:"path,omitempty"`
	Include string `json:"glob,omitempty"`
}

func (t *GrepTool) Name() string { return "Grep" }

func (t *GrepTool) Description() string {
	return `Search file contents using ripgrep (rg). Supports full regex syntax.
Falls back to grep -rn if ripgrep is not installed.
Use the glob parameter to filter by file type (e.g. "*.go").`
}

func (t *GrepTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"pattern": {
				"type": "string",
				"description": "Regex pattern to search for"
			},
			"path": {
				"type": "string",
				"description": "File or directory to search in"
			},
			"glob": {
				"type": "string",
				"description": "Glob pattern to filter files (e.g. \"*.go\")"
			}
		},
		"required": ["pattern"]
	}`)
}

func (t *GrepTool) NeedsPermission(_ json.RawMessage) bool { return false }

func (t *GrepTool) FormatPermissionRequest(input json.RawMessage) string {
	var in grepInput
	_ = json.Unmarshal(input, &in)
	return fmt.Sprintf("Search for: %s", in.Pattern)
}

func (t *GrepTool) Execute(input json.RawMessage, workDir string) (string, error) {
	var in grepInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}

	searchPath := workDir
	if in.Path != "" {
		searchPath = resolvePath(in.Path, workDir)
	}

	if _, err := exec.LookPath("rg"); err != nil {
		return runGrep(in.Pattern, searchPath, workDir)
	}
	return runRipgrep(in.Pattern, in.Include, searchPath, workDir)
}

func runRipgrep(pattern, include, searchPath, workDir string) (string, error) {
	args := []string{"-n", "--color=never", "--no-heading"}
	if include != "" {
		args = append(args, "--glob", include)
	}
	args = append(args, pattern, searchPath)

	cmd := exec.Command("rg", args...)
	cmd.Dir = workDir
	output, err := cmd.CombinedOutput()
	return handleSearchResult(string(output), err)
}

func runGrep(pattern, searchPath, workDir string) (string, error) {
	cmd := exec.Command("grep", "-rn", "--color=never", pattern, searchPath)
	cmd.Dir = workDir
	output, err := cmd.CombinedOutput()
	return handleSearchResult(string(output), err)
}

func handleSearchResult(output string, err error) (string, error) {
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return "No matches found", nil
		}
		if output != "" {
			return output, nil
		}
		return "", fmt.Errorf("search failed: %w", err)
	}

	lines := strings.Split(output, "\n")
	if len(lines) > maxGrepLines {
		lines = lines[:maxGrepLines]
		output = strings.Join(lines, "\n") +
			fmt.Sprintf("\n... (truncated, showing first %d matches)", maxGrepLines)
	}
	return output, nil
}
