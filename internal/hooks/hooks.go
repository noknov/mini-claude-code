// Package hooks implements lifecycle hooks (pre/post tool use, compact, session).
//
// Hooks are configured in .claude/settings.json under the "hooks" key.
// Each hook is a shell command that receives context via environment variables.
package hooks

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/noknov/mini-claude-code/internal/settings"
)

// ---------------------------------------------------------------------------
// Event names
// ---------------------------------------------------------------------------

const (
	PreToolUse   = "PreToolUse"
	PostToolUse  = "PostToolUse"
	PreCompact   = "PreCompact"
	PostCompact  = "PostCompact"
	SessionStart = "SessionStart"
	SessionStop  = "SessionStop"
)

const defaultTimeout = 30 * time.Second

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

// Result is the outcome of a single hook execution.
type Result struct {
	Output   string
	Decision string // "allow", "deny", "ask", or "" (no opinion)
	Error    error
}

// Runner executes hooks defined in settings.
type Runner struct {
	hooks   map[string][]settings.HookEntry
	workDir string
}

// NewRunner creates a hook runner from the given settings.
func NewRunner(s *settings.Settings, workDir string) *Runner {
	h := s.Hooks
	if h == nil {
		h = make(map[string][]settings.HookEntry)
	}
	return &Runner{hooks: h, workDir: workDir}
}

// ---------------------------------------------------------------------------
// Execution
// ---------------------------------------------------------------------------

// Run executes all hooks for the given event.
func (r *Runner) Run(event, toolName string, toolInput json.RawMessage) []Result {
	entries := r.hooks[event]
	if len(entries) == 0 {
		return nil
	}
	var results []Result
	for _, entry := range entries {
		if !matchesCondition(entry.If, toolName) {
			continue
		}
		if entry.Command != "" {
			results = append(results, r.exec(entry.Command, event, toolName, toolInput))
		}
	}
	return results
}

// HasHooks returns true if any hooks are registered for the given event.
func (r *Runner) HasHooks(event string) bool {
	return len(r.hooks[event]) > 0
}

func (r *Runner) exec(command, event, toolName string, toolInput json.RawMessage) Result {
	cmd := exec.Command("bash", "-c", command)
	cmd.Dir = r.workDir
	cmd.Env = append(os.Environ(),
		"HOOK_EVENT="+event,
		"TOOL_NAME="+toolName,
		"TOOL_INPUT="+string(toolInput),
	)

	type cmdResult struct {
		output []byte
		err    error
	}
	ch := make(chan cmdResult, 1)
	go func() {
		out, err := cmd.CombinedOutput()
		ch <- cmdResult{out, err}
	}()

	select {
	case r := <-ch:
		output := strings.TrimSpace(string(r.output))
		return Result{Output: output, Decision: parseDecision(output), Error: r.err}
	case <-time.After(defaultTimeout):
		_ = cmd.Process.Kill()
		return Result{Error: fmt.Errorf("hook timed out after %v", defaultTimeout)}
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func matchesCondition(condition, toolName string) bool {
	condition = strings.TrimSpace(condition)
	if condition == "" {
		return true
	}
	if ok, _ := filepath.Match(condition, toolName); ok {
		return true
	}
	return strings.EqualFold(condition, toolName)
}

func parseDecision(output string) string {
	for _, line := range strings.Split(strings.ToLower(output), "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "decision:allow"), line == "allow":
			return "allow"
		case strings.HasPrefix(line, "decision:deny"), line == "deny":
			return "deny"
		case strings.HasPrefix(line, "decision:ask"), line == "ask":
			return "ask"
		}
	}
	return ""
}

// ResolvePermission returns the first non-empty decision from hook results.
func ResolvePermission(results []Result) string {
	for _, r := range results {
		if r.Decision != "" {
			return r.Decision
		}
	}
	return ""
}
