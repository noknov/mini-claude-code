package tools

import (
	"encoding/json"
	"fmt"

	"github.com/noknov/mini-claude-code/internal/skills"
)

// SkillTool lets the model invoke a registered skill programmatically.
type SkillTool struct {
	Skills []skills.Skill
}

type skillToolInput struct {
	Name string `json:"name"`
}

func (t *SkillTool) Name() string { return "Skill" }

func (t *SkillTool) Description() string {
	return `Invoke a registered skill (prompt template) by name. Skills are loaded from .claude/commands/ directories. Use this to apply reusable workflows.`
}

func (t *SkillTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"name": {
				"type": "string",
				"description": "The skill name to invoke"
			}
		},
		"required": ["name"]
	}`)
}

func (t *SkillTool) NeedsPermission(_ json.RawMessage) bool { return false }

func (t *SkillTool) FormatPermissionRequest(input json.RawMessage) string {
	var in skillToolInput
	_ = json.Unmarshal(input, &in)
	return fmt.Sprintf("Invoke skill: %s", in.Name)
}

func (t *SkillTool) Execute(input json.RawMessage, _ string) (string, error) {
	var in skillToolInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}
	skill := skills.Find(t.Skills, in.Name)
	if skill == nil {
		available := skills.Names(t.Skills)
		return "", fmt.Errorf("skill %q not found. Available: %v", in.Name, available)
	}
	return skill.Content, nil
}
