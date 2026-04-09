package permission

import "strings"

// Asker is the interface for prompting the user for permission.
type Asker interface {
	AskPermission(toolName, description string) string
}

// Manager handles tool execution permission checks.
type Manager struct {
	mode     string          // "ask", "auto", "deny"
	alwaysOK map[string]bool // tools the user has permanently approved
}

func NewManager(mode string) *Manager {
	return &Manager{
		mode:     mode,
		alwaysOK: make(map[string]bool),
	}
}

// Check returns true if the tool is allowed to execute.
// In "ask" mode it prompts the user via the Asker interface.
func (m *Manager) Check(toolName, description string, asker Asker) bool {
	switch m.mode {
	case "auto":
		return true
	case "deny":
		return false
	}

	if m.alwaysOK[toolName] {
		return true
	}

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

func (m *Manager) SetMode(mode string) { m.mode = mode }
