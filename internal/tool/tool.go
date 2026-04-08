package tool

import (
	"encoding/json"
	"fmt"
)

// Tool defines the interface all tools must implement
type Tool interface {
	Name() string
	Description() string
	InputSchema() json.RawMessage
	Execute(input json.RawMessage, workDir string) (string, error)
	NeedsPermission(input json.RawMessage) bool
	FormatPermissionRequest(input json.RawMessage) string
}

// Registry holds all registered tools
type Registry struct {
	tools map[string]Tool
}

func NewRegistry() *Registry {
	return &Registry{tools: make(map[string]Tool)}
}

func (r *Registry) Register(t Tool) {
	r.tools[t.Name()] = t
}

func (r *Registry) Get(name string) (Tool, bool) {
	t, ok := r.tools[name]
	return t, ok
}

func (r *Registry) All() []Tool {
	result := make([]Tool, 0, len(r.tools))
	for _, t := range r.tools {
		result = append(result, t)
	}
	return result
}

func (r *Registry) APIDefs() []json.RawMessage {
	defs := make([]json.RawMessage, 0, len(r.tools))
	for _, t := range r.tools {
		def := map[string]interface{}{
			"name":         t.Name(),
			"description":  t.Description(),
			"input_schema": json.RawMessage(t.InputSchema()),
		}
		b, _ := json.Marshal(def)
		defs = append(defs, b)
	}
	return defs
}

// Execute runs a tool by name with the given input
func (r *Registry) Execute(name string, input json.RawMessage, workDir string) (string, error) {
	t, ok := r.Get(name)
	if !ok {
		return "", fmt.Errorf("unknown tool: %s", name)
	}
	return t.Execute(input, workDir)
}
