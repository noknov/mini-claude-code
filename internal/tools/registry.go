package tools

import (
	"github.com/noknov/mini-claude-code/internal/tool"
)

// NewDefaultRegistry creates a registry with all built-in tools
func NewDefaultRegistry() *tool.Registry {
	r := tool.NewRegistry()
	r.Register(&BashTool{})
	r.Register(&FileReadTool{})
	r.Register(&FileWriteTool{})
	r.Register(&FileEditTool{})
	r.Register(&GlobTool{})
	r.Register(&GrepTool{})
	return r
}
