package tools

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

const maxGrepResults = 5000

type GrepTool struct{}

type grepInput struct {
	Pattern string `json:"pattern"`
	Path    string `json:"path,omitempty"`
	Include string `json:"glob,omitempty"`
}

func (t *GrepTool) Name() string { return "Grep" }

func (t *GrepTool) Description() string {
	return `Search file contents using ripgrep. Supports regex patterns.
Falls back to grep if ripgrep is not installed.`
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
	json.Unmarshal(input, &in)
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

	args := []string{"-n", "--color=never", "--no-heading"}
	if in.Include != "" {
		args = append(args, "--glob", in.Include)
	}
	args = append(args, in.Pattern, searchPath)

	rgPath, err := exec.LookPath("rg")
	if err != nil {
		return grepFallback(in.Pattern, searchPath, workDir)
	}

	cmd := exec.Command(rgPath, args...)
	cmd.Dir = workDir
	output, err := cmd.CombinedOutput()
	result := string(output)

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return "No matches found", nil
		}
		if result != "" {
			return result, nil
		}
		return "", fmt.Errorf("grep failed: %w", err)
	}

	lines := strings.Split(result, "\n")
	if len(lines) > maxGrepResults {
		lines = lines[:maxGrepResults]
		result = strings.Join(lines, "\n") + fmt.Sprintf("\n... (truncated, showing first %d matches)", maxGrepResults)
	}

	return result, nil
}

func grepFallback(pattern, searchPath, workDir string) (string, error) {
	cmd := exec.Command("grep", "-rn", "--color=never", pattern, searchPath)
	cmd.Dir = workDir
	output, err := cmd.CombinedOutput()
	result := string(output)

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return "No matches found", nil
		}
		return result, nil
	}

	return result, nil
}
