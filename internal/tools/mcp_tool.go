package tools

import (
	"encoding/json"
	"fmt"

	"github.com/noknov/mini-claude-code/internal/mcp"
)

// MCPTool calls a tool on a connected MCP server.
type MCPTool struct {
	Client *mcp.Client
}

type mcpToolInput struct {
	Server string          `json:"server"`
	Tool   string          `json:"tool"`
	Args   json.RawMessage `json:"args,omitempty"`
}

func (t *MCPTool) Name() string { return "MCPTool" }

func (t *MCPTool) Description() string {
	return `Call a tool on a connected MCP (Model Context Protocol) server. MCP servers extend the available tools with external capabilities.`
}

func (t *MCPTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"server": {
				"type": "string",
				"description": "Name of the MCP server"
			},
			"tool": {
				"type": "string",
				"description": "Name of the tool to call"
			},
			"args": {
				"type": "object",
				"description": "Arguments to pass to the tool"
			}
		},
		"required": ["server", "tool"]
	}`)
}

func (t *MCPTool) NeedsPermission(_ json.RawMessage) bool { return true }

func (t *MCPTool) FormatPermissionRequest(input json.RawMessage) string {
	var in mcpToolInput
	_ = json.Unmarshal(input, &in)
	return fmt.Sprintf("MCP call: %s/%s", in.Server, in.Tool)
}

func (t *MCPTool) Execute(input json.RawMessage, _ string) (string, error) {
	var in mcpToolInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}
	if t.Client == nil {
		return "", fmt.Errorf("MCP client not configured")
	}
	return t.Client.CallTool(in.Server, in.Tool, in.Args)
}

// MCPResourceTool reads a resource from a connected MCP server.
type MCPResourceTool struct {
	Client *mcp.Client
}

type mcpResourceInput struct {
	Server string `json:"server"`
	URI    string `json:"uri"`
}

func (t *MCPResourceTool) Name() string { return "ReadMCPResource" }

func (t *MCPResourceTool) Description() string {
	return `Read a resource from a connected MCP server by URI.`
}

func (t *MCPResourceTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"server": {
				"type": "string",
				"description": "Name of the MCP server"
			},
			"uri": {
				"type": "string",
				"description": "Resource URI to read"
			}
		},
		"required": ["server", "uri"]
	}`)
}

func (t *MCPResourceTool) NeedsPermission(_ json.RawMessage) bool { return false }

func (t *MCPResourceTool) FormatPermissionRequest(input json.RawMessage) string {
	var in mcpResourceInput
	_ = json.Unmarshal(input, &in)
	return fmt.Sprintf("Read MCP resource: %s/%s", in.Server, in.URI)
}

func (t *MCPResourceTool) Execute(input json.RawMessage, _ string) (string, error) {
	var in mcpResourceInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}
	if t.Client == nil {
		return "", fmt.Errorf("MCP client not configured")
	}
	return t.Client.ReadResource(in.Server, in.URI)
}
