package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

const (
	maxGrepMatches    = 250
	maxGrepResultSize = 20000 // chars, aligned with Claude Code's persistence threshold
	grepTimeout       = 20 * time.Second
)

var vcsDirectoriesToExclude = []string{".git", ".svn", ".hg", ".bzr", ".jj"}

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
	args := buildRipgrepArgs(pattern, include)
	args = append(args, searchPath)

	ctx, cancel := context.WithTimeout(context.Background(), grepTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "rg", args...)
	cmd.Dir = workDir
	output, err := cmd.CombinedOutput()

	if ctx.Err() == context.DeadlineExceeded {
		result := truncateSearchOutput(string(output))
		return result + "\n\n(search timed out, results may be incomplete)", nil
	}
	return handleSearchResult(string(output), err)
}

func buildRipgrepArgs(pattern, include string) []string {
	args := []string{
		"--hidden",
		"--no-binary",
		"-n", "--color=never", "--no-heading",
		"--max-columns", "500",
		"--max-columns-preview",
	}
	for _, dir := range vcsDirectoriesToExclude {
		args = append(args, "--glob", "!"+dir)
	}
	if include != "" {
		args = append(args, "--glob", include)
	}
	args = append(args, pattern)
	return args
}

func runGrep(pattern, searchPath, workDir string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), grepTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "grep", "-rn", "--color=never",
		"--binary-files=without-match", pattern, searchPath)
	cmd.Dir = workDir
	output, err := cmd.CombinedOutput()

	if ctx.Err() == context.DeadlineExceeded {
		result := truncateSearchOutput(string(output))
		return result + "\n\n(search timed out, results may be incomplete)", nil
	}
	return handleSearchResult(string(output), err)
}

func handleSearchResult(output string, err error) (string, error) {
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return "No matches found", nil
		}
		if output != "" {
			return truncateSearchOutput(output), nil
		}
		return "", fmt.Errorf("search failed: %w", err)
	}
	return truncateSearchOutput(output), nil
}

func truncateSearchOutput(output string) string {
	lines := strings.Split(output, "\n")
	truncated := false

	if len(lines) > maxGrepMatches {
		lines = lines[:maxGrepMatches]
		truncated = true
	}

	result := strings.Join(lines, "\n")
	if len(result) > maxGrepResultSize {
		result = result[:maxGrepResultSize]
		truncated = true
	}

	if truncated {
		result += fmt.Sprintf("\n\n... (truncated, showing first %d matches out of more)", maxGrepMatches)
	}
	return result
}
