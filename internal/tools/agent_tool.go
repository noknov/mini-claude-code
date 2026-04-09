package tools

import (
	"encoding/json"
	"fmt"
)

// AgentTool spawns a subagent for parallel or specialized tasks.
// The actual agent execution is delegated to the query engine via a callback.
type AgentTool struct {
	// OnSpawn is called by the query engine to actually run the subagent.
	// Set by the engine after registration.
	OnSpawn func(prompt, agentName string) (string, error)
}

type agentToolInput struct {
	Prompt    string `json:"prompt"`
	AgentName string `json:"agent_name,omitempty"`
}

func (t *AgentTool) Name() string { return "Agent" }

func (t *AgentTool) Description() string {
	return `Launch a subagent to handle a complex task autonomously. The subagent has its own conversation context and can use tools independently. Use for parallel research, code review, or tasks that benefit from isolated context.`
}

func (t *AgentTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"prompt": {
				"type": "string",
				"description": "The task description for the subagent"
			},
			"agent_name": {
				"type": "string",
				"description": "Name of a custom agent definition to use (optional)"
			}
		},
		"required": ["prompt"]
	}`)
}

func (t *AgentTool) NeedsPermission(_ json.RawMessage) bool { return true }

func (t *AgentTool) FormatPermissionRequest(input json.RawMessage) string {
	var in agentToolInput
	_ = json.Unmarshal(input, &in)
	prompt := in.Prompt
	if len(prompt) > 100 {
		prompt = prompt[:100] + "..."
	}
	return fmt.Sprintf("Spawn agent: %s", prompt)
}

func (t *AgentTool) Execute(input json.RawMessage, _ string) (string, error) {
	var in agentToolInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}
	if in.Prompt == "" {
		return "", fmt.Errorf("prompt is required")
	}
	if t.OnSpawn == nil {
		return "", fmt.Errorf("agent execution not configured")
	}
	return t.OnSpawn(in.Prompt, in.AgentName)
}
