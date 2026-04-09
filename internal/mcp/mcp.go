// Package mcp implements MCP (Model Context Protocol) client integration.
//
// Discovers MCP servers from .mcp.json configs, communicates via JSON-RPC
// over stdio, and exposes discovered tools/resources to the query engine.
package mcp

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const rpcTimeout = 30 * time.Second

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

// ServerConfig describes an MCP server from .mcp.json.
type ServerConfig struct {
	Command string            `json:"command"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
}

// Config is the top-level .mcp.json structure.
type Config struct {
	MCPServers map[string]ServerConfig `json:"mcpServers"`
}

// ToolInfo describes a tool exposed by an MCP server.
type ToolInfo struct {
	Server      string
	Name        string
	Description string
	InputSchema json.RawMessage
}

// ResourceInfo describes a resource exposed by an MCP server.
type ResourceInfo struct {
	Server      string
	URI         string
	Name        string
	Description string
	MimeType    string
}

// ---------------------------------------------------------------------------
// Client
// ---------------------------------------------------------------------------

// Client manages connections to MCP servers.
type Client struct {
	servers map[string]ServerConfig
	workDir string
}

// NewClient creates an MCP client from discovered configuration.
func NewClient(workDir string) *Client {
	return &Client{
		servers: discoverServers(workDir),
		workDir: workDir,
	}
}

func (c *Client) HasServers() bool { return len(c.servers) > 0 }
func (c *Client) ServerNames() []string {
	names := make([]string, 0, len(c.servers))
	for name := range c.servers {
		names = append(names, name)
	}
	return names
}

// ---------------------------------------------------------------------------
// Tool operations
// ---------------------------------------------------------------------------

// ListTools queries an MCP server for its available tools.
func (c *Client) ListTools(serverName string) ([]ToolInfo, error) {
	result, err := c.rpc(serverName, "tools/list", nil)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Tools []struct {
			Name        string          `json:"name"`
			Description string          `json:"description"`
			InputSchema json.RawMessage `json:"inputSchema"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(result, &resp); err != nil {
		return nil, fmt.Errorf("parse tools: %w", err)
	}
	tools := make([]ToolInfo, len(resp.Tools))
	for i, t := range resp.Tools {
		tools[i] = ToolInfo{Server: serverName, Name: t.Name, Description: t.Description, InputSchema: t.InputSchema}
	}
	return tools, nil
}

// CallTool invokes a tool on an MCP server.
func (c *Client) CallTool(serverName, toolName string, args json.RawMessage) (string, error) {
	params := map[string]interface{}{"name": toolName, "arguments": json.RawMessage(args)}
	result, err := c.rpc(serverName, "tools/call", params)
	if err != nil {
		return "", err
	}
	return extractTextContent(result), nil
}

// ---------------------------------------------------------------------------
// Resource operations
// ---------------------------------------------------------------------------

// ListResources queries an MCP server for its available resources.
func (c *Client) ListResources(serverName string) ([]ResourceInfo, error) {
	result, err := c.rpc(serverName, "resources/list", nil)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Resources []struct {
			URI         string `json:"uri"`
			Name        string `json:"name"`
			Description string `json:"description"`
			MimeType    string `json:"mimeType"`
		} `json:"resources"`
	}
	if err := json.Unmarshal(result, &resp); err != nil {
		return nil, err
	}
	resources := make([]ResourceInfo, len(resp.Resources))
	for i, r := range resp.Resources {
		resources[i] = ResourceInfo{Server: serverName, URI: r.URI, Name: r.Name, Description: r.Description, MimeType: r.MimeType}
	}
	return resources, nil
}

// ReadResource fetches a resource's content from an MCP server.
func (c *Client) ReadResource(serverName, uri string) (string, error) {
	params := map[string]interface{}{"uri": uri}
	result, err := c.rpc(serverName, "resources/read", params)
	if err != nil {
		return "", err
	}
	return extractTextContent(result), nil
}

// ---------------------------------------------------------------------------
// Prompt formatting
// ---------------------------------------------------------------------------

// FormatInstructions returns MCP server info for the system prompt.
func (c *Client) FormatInstructions() string {
	if !c.HasServers() {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("Connected MCP servers:\n")
	for name, srv := range c.servers {
		fmt.Fprintf(&sb, "  - %s (command: %s)\n", name, srv.Command)
	}
	return sb.String()
}

// ---------------------------------------------------------------------------
// JSON-RPC over stdio
// ---------------------------------------------------------------------------

func (c *Client) rpc(serverName, method string, params interface{}) (json.RawMessage, error) {
	srv, ok := c.servers[serverName]
	if !ok {
		return nil, fmt.Errorf("unknown MCP server: %s", serverName)
	}

	reqBody := map[string]interface{}{"jsonrpc": "2.0", "id": 1, "method": method}
	if params != nil {
		reqBody["params"] = params
	}
	input, _ := json.Marshal(reqBody)

	cmd := exec.Command(srv.Command, srv.Args...)
	cmd.Dir = c.workDir
	cmd.Stdin = strings.NewReader(string(input) + "\n")
	cmd.Env = buildEnv(srv.Env)

	output, err := runWithTimeout(cmd, rpcTimeout)
	if err != nil {
		return nil, fmt.Errorf("MCP %s/%s: %w", serverName, method, err)
	}

	return parseRPCResponse(output)
}

func buildEnv(extra map[string]string) []string {
	env := os.Environ()
	for k, v := range extra {
		env = append(env, k+"="+v)
	}
	return env
}

func runWithTimeout(cmd *exec.Cmd, timeout time.Duration) ([]byte, error) {
	type result struct {
		output []byte
		err    error
	}
	ch := make(chan result, 1)
	go func() {
		out, err := cmd.Output()
		ch <- result{out, err}
	}()

	select {
	case r := <-ch:
		return r.output, r.err
	case <-time.After(timeout):
		_ = cmd.Process.Kill()
		return nil, fmt.Errorf("timed out after %v", timeout)
	}
}

func parseRPCResponse(data []byte) (json.RawMessage, error) {
	var resp struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("parse RPC response: %w", err)
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("RPC error: %s", resp.Error.Message)
	}
	return resp.Result, nil
}

func extractTextContent(data json.RawMessage) string {
	var resp struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if json.Unmarshal(data, &resp) != nil {
		return string(data)
	}
	var texts []string
	for _, c := range resp.Content {
		if c.Type == "text" {
			texts = append(texts, c.Text)
		}
	}
	return strings.Join(texts, "\n")
}

// ---------------------------------------------------------------------------
// Config discovery
// ---------------------------------------------------------------------------

func discoverServers(workDir string) map[string]ServerConfig {
	servers := make(map[string]ServerConfig)
	for _, cfg := range discoverConfigs(workDir) {
		for name, srv := range cfg.MCPServers {
			servers[name] = srv
		}
	}
	return servers
}

func discoverConfigs(workDir string) []Config {
	var configs []Config

	for dir := workDir; ; {
		if cfg := loadConfig(filepath.Join(dir, ".mcp.json")); cfg != nil {
			configs = append(configs, *cfg)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	if home, _ := os.UserHomeDir(); home != "" {
		if cfg := loadConfig(filepath.Join(home, ".claude", "mcp.json")); cfg != nil {
			configs = append(configs, *cfg)
		}
	}
	return configs
}

func loadConfig(path string) *Config {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var cfg Config
	if json.Unmarshal(data, &cfg) != nil || len(cfg.MCPServers) == 0 {
		return nil
	}
	return &cfg
}
