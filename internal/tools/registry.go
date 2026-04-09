package tools

import (
	"github.com/noknov/mini-claude-code/internal/mcp"
	"github.com/noknov/mini-claude-code/internal/skills"
	"github.com/noknov/mini-claude-code/internal/tool"
)

// NewDefaultRegistry creates a registry with all built-in tools.
// Dynamic tools (Agent, Skill, MCP) are configured with their dependencies.
func NewDefaultRegistry(sk []skills.Skill, mcpClient *mcp.Client) *tool.Registry {
	r := tool.NewRegistry()

	// Core file/shell tools
	r.Register(&BashTool{})
	r.Register(&FileReadTool{})
	r.Register(&FileWriteTool{})
	r.Register(&FileEditTool{})
	r.Register(&GlobTool{})
	r.Register(&GrepTool{})

	// Web tools
	r.Register(&WebFetchTool{})
	r.Register(&WebSearchTool{})

	// Notebook
	r.Register(&NotebookEditTool{})

	// Task management
	r.Register(&TodoWriteTool{})

	// Agent (callback set by query engine)
	r.Register(&AgentTool{})

	// Skill
	r.Register(&SkillTool{Skills: sk})

	// MCP tools (only if servers are configured)
	if mcpClient != nil && mcpClient.HasServers() {
		r.Register(&MCPTool{Client: mcpClient})
		r.Register(&MCPResourceTool{Client: mcpClient})
	}

	return r
}
