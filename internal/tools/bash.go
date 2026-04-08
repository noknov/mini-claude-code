package tools

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

const (
	bashMaxOutput  = 30000
	bashTimeoutSec = 120
)

type BashTool struct{}

type bashInput struct {
	Command   string `json:"command"`
	TimeoutMs int    `json:"timeout_ms,omitempty"`
}

func (t *BashTool) Name() string { return "Bash" }

func (t *BashTool) Description() string {
	return `Execute a shell command. Use for running scripts, installing packages, git operations, file system operations, compiling code, running tests, and any other command-line tasks.
Commands run in the user's shell environment with their PATH and aliases.
Long-running commands will be terminated after the timeout.`
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

func (t *BashTool) NeedsPermission(input json.RawMessage) bool {
	return true
}

func (t *BashTool) FormatPermissionRequest(input json.RawMessage) string {
	var in bashInput
	json.Unmarshal(input, &in)
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

	timeout := time.Duration(bashTimeoutSec) * time.Second
	if in.TimeoutMs > 0 {
		timeout = time.Duration(in.TimeoutMs) * time.Millisecond
	}

	cmd := exec.Command("bash", "-c", in.Command)
	cmd.Dir = workDir

	done := make(chan struct{})
	var output []byte
	var cmdErr error

	go func() {
		output, cmdErr = cmd.CombinedOutput()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(timeout):
		if cmd.Process != nil {
			cmd.Process.Kill()
		}
		return "", fmt.Errorf("command timed out after %v", timeout)
	}

	result := string(output)
	if len(result) > bashMaxOutput {
		half := bashMaxOutput / 2
		result = result[:half] + "\n\n... [output truncated] ...\n\n" + result[len(result)-half:]
	}

	if cmdErr != nil {
		exitErr, ok := cmdErr.(*exec.ExitError)
		if ok {
			return fmt.Sprintf("%s\nExit code: %d", result, exitErr.ExitCode()), nil
		}
		return result, fmt.Errorf("command failed: %w", cmdErr)
	}

	return strings.TrimRight(result, "\n"), nil
}
