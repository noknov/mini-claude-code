package permission

import (
	"strings"
)

// Manager handles tool permission checks
type Manager struct {
	mode      string // "ask", "auto", "deny"
	allowOnce map[string]bool
	allowAll  map[string]bool
}

// Asker is the interface for asking user permission
type Asker interface {
	AskPermission(toolName, description string) string
}

func NewManager(mode string) *Manager {
	return &Manager{
		mode:      mode,
		allowOnce: make(map[string]bool),
		allowAll:  make(map[string]bool),
	}
}

// Check returns true if the tool is allowed to execute
func (m *Manager) Check(toolName, description string, asker Asker) bool {
	if m.mode == "auto" {
		return true
	}
	if m.mode == "deny" {
		return false
	}

	if m.allowAll[toolName] {
		return true
	}

	response := asker.AskPermission(toolName, description)
	response = strings.TrimSpace(strings.ToLower(response))

	switch response {
	case "y", "yes", "":
		return true
	case "a", "always":
		m.allowAll[toolName] = true
		return true
	default:
		return false
	}
}

func (m *Manager) SetMode(mode string) {
	m.mode = mode
}
