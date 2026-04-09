package tool

import (
	"encoding/json"
	"fmt"
)

// Tool defines the interface every executable tool must implement.
type Tool interface {
	Name() string
	Description() string
	InputSchema() json.RawMessage
	Execute(input json.RawMessage, workDir string) (string, error)

	// NeedsPermission reports whether this tool call requires user approval.
	NeedsPermission(input json.RawMessage) bool
	// FormatPermissionRequest returns a human-readable description of the
	// action that needs approval.
	FormatPermissionRequest(input json.RawMessage) string
}

// Registry is a name-indexed collection of tools.
type Registry struct {
	tools map[string]Tool
	order []string // preserves registration order
}

func NewRegistry() *Registry {
	return &Registry{tools: make(map[string]Tool)}
}

func (r *Registry) Register(t Tool) {
	name := t.Name()
	if _, exists := r.tools[name]; !exists {
		r.order = append(r.order, name)
	}
	r.tools[name] = t
}

func (r *Registry) Get(name string) (Tool, bool) {
	t, ok := r.tools[name]
	return t, ok
}

// All returns every registered tool in registration order.
func (r *Registry) All() []Tool {
	result := make([]Tool, 0, len(r.order))
	for _, name := range r.order {
		result = append(result, r.tools[name])
	}
	return result
}

// Execute looks up a tool by name and runs it.
func (r *Registry) Execute(name string, input json.RawMessage, workDir string) (string, error) {
	t, ok := r.Get(name)
	if !ok {
		return "", fmt.Errorf("unknown tool: %s", name)
	}
	return t.Execute(input, workDir)
}
