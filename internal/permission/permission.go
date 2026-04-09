// Package permission handles tool execution authorization.
//
// Supports multiple modes (ask/auto/deny/plan), glob-based allow/deny rules
// from settings, hook-based decisions, and a classifier stub for auto-mode.
package permission

import (
	"path/filepath"
	"strings"

	"github.com/noknov/mini-claude-code/internal/settings"
)

// ---------------------------------------------------------------------------
// Interfaces
// ---------------------------------------------------------------------------

// Asker prompts the user for permission.
type Asker interface {
	AskPermission(toolName, description string) string
}

// ---------------------------------------------------------------------------
// Manager
// ---------------------------------------------------------------------------

// Manager handles tool execution permission checks.
type Manager struct {
	mode     string          // "ask", "auto", "deny", "plan"
	alwaysOK map[string]bool // tools the user has permanently approved this session
	rules    settings.PermissionSettings
}

func NewManager(mode string, rules settings.PermissionSettings) *Manager {
	return &Manager{
		mode:     mode,
		alwaysOK: make(map[string]bool),
		rules:    rules,
	}
}

// ---------------------------------------------------------------------------
// Check
// ---------------------------------------------------------------------------

// Check returns true if the tool is allowed to execute.
func (m *Manager) Check(toolName, description string, asker Asker) bool {
	switch m.mode {
	case "auto":
		return true
	case "deny", "plan":
		return false
	}

	// Check deny rules first
	if m.matchesAnyRule(m.rules.Deny, toolName, description) {
		return false
	}

	// Check allow rules
	if m.matchesAnyRule(m.rules.Allow, toolName, description) {
		return true
	}

	// Per-tool session approval
	if m.alwaysOK[toolName] {
		return true
	}

	// Classify command safety (stub — always returns "ask" for now)
	if decision := classifyCommand(toolName, description); decision != "" {
		return decision == "allow"
	}

	return m.askUser(toolName, description, asker)
}

// CheckWithHookDecision incorporates a hook's permission decision.
func (m *Manager) CheckWithHookDecision(hookDecision, toolName, description string, asker Asker) bool {
	switch hookDecision {
	case "allow":
		return true
	case "deny":
		return false
	case "ask":
		return m.askUser(toolName, description, asker)
	default:
		return m.Check(toolName, description, asker)
	}
}

func (m *Manager) askUser(toolName, description string, asker Asker) bool {
	resp := strings.TrimSpace(strings.ToLower(asker.AskPermission(toolName, description)))
	switch resp {
	case "y", "yes", "":
		return true
	case "a", "always":
		m.alwaysOK[toolName] = true
		return true
	default:
		return false
	}
}

// SetMode changes the permission mode.
func (m *Manager) SetMode(mode string) { m.mode = mode }

// Mode returns the current permission mode.
func (m *Manager) Mode() string { return m.mode }

// ---------------------------------------------------------------------------
// Rule matching
// ---------------------------------------------------------------------------

func (m *Manager) matchesAnyRule(rules []settings.PermissionRule, toolName, description string) bool {
	for _, rule := range rules {
		if matchesRule(rule, toolName, description) {
			return true
		}
	}
	return false
}

func matchesRule(rule settings.PermissionRule, toolName, description string) bool {
	if !matchToolName(rule.Tool, toolName) {
		return false
	}
	if rule.Pattern == "" {
		return true
	}
	if ok, _ := filepath.Match(rule.Pattern, description); ok {
		return true
	}
	return strings.Contains(description, rule.Pattern)
}

func matchToolName(pattern, name string) bool {
	if pattern == "*" || pattern == name {
		return true
	}
	ok, _ := filepath.Match(pattern, name)
	return ok
}

// ---------------------------------------------------------------------------
// Classifier stub
// ---------------------------------------------------------------------------

// classifyCommand is a placeholder for a classifier model that determines
// whether a command is safe to auto-approve. Returns "" to defer to user.
func classifyCommand(_, _ string) string {
	// TODO: implement lightweight classifier (e.g. regex-based for common
	// safe patterns like "git status", "ls", "cat", read-only commands).
	return ""
}
