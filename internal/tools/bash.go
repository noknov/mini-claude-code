package tools

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

const (
	bashDefaultTimeout = 120 * time.Second
	bashMaxOutput      = 30000
)

type BashTool struct{}

type bashInput struct {
	Command   string `json:"command"`
	TimeoutMs int    `json:"timeout_ms,omitempty"`
}

func (t *BashTool) Name() string { return "Bash" }

func (t *BashTool) Description() string {
	return `Execute a shell command in bash.
Use for running scripts, installing packages, git operations, compiling code, running tests, and any other CLI tasks.
Long-running commands are terminated after the timeout (default 120 s).`
}

func (t *BashTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"command": {
				"type": "string",
				"description": "The shell command to execute"
			},
			"timeout_ms": {
				"type": "integer",
				"description": "Timeout in milliseconds (default: 120000)"
			}
		},
		"required": ["command"]
	}`)
}

func (t *BashTool) NeedsPermission(_ json.RawMessage) bool { return true }

func (t *BashTool) FormatPermissionRequest(input json.RawMessage) string {
	var in bashInput
	_ = json.Unmarshal(input, &in)
	return fmt.Sprintf("Run command: %s", in.Command)
}

func (t *BashTool) Execute(input json.RawMessage, workDir string) (string, error) {
	var in bashInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}
	if in.Command == "" {
		return "", fmt.Errorf("command is required")
	}

	timeout := bashDefaultTimeout
	if in.TimeoutMs > 0 {
		timeout = time.Duration(in.TimeoutMs) * time.Millisecond
	}

	cmd := exec.Command("bash", "-c", in.Command)
	cmd.Dir = workDir

	type result struct {
		output []byte
		err    error
	}
	ch := make(chan result, 1)
	go func() {
		out, err := cmd.CombinedOutput()
		ch <- result{out, err}
	}()

	select {
	case r := <-ch:
		return formatBashOutput(r.output, r.err)
	case <-time.After(timeout):
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		return "", fmt.Errorf("command timed out after %v", timeout)
	}
}

func formatBashOutput(output []byte, cmdErr error) (string, error) {
	text := string(output)
	if len(text) > bashMaxOutput {
		half := bashMaxOutput / 2
		text = text[:half] + "\n\n... [output truncated] ...\n\n" + text[len(text)-half:]
	}

	if cmdErr == nil {
		return strings.TrimRight(text, "\n"), nil
	}
	if exitErr, ok := cmdErr.(*exec.ExitError); ok {
		return fmt.Sprintf("%s\nExit code: %d", text, exitErr.ExitCode()), nil
	}
	return text, fmt.Errorf("command failed: %w", cmdErr)
}
