package tools

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

// TodoWriteTool manages a structured task list for the current session.
type TodoWriteTool struct {
	mu    sync.Mutex
	todos []todoItem
}

type todoItem struct {
	ID      string `json:"id"`
	Content string `json:"content"`
	Status  string `json:"status"` // "pending", "in_progress", "completed", "cancelled"
}

type todoWriteInput struct {
	Todos []todoItem `json:"todos"`
	Merge bool       `json:"merge,omitempty"`
}

func (t *TodoWriteTool) Name() string { return "TodoWrite" }

func (t *TodoWriteTool) Description() string {
	return `Create and manage a structured task list. Each todo has an id, content, and status (pending/in_progress/completed/cancelled). Use merge=true to update existing items by id.`
}

func (t *TodoWriteTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"todos": {
				"type": "array",
				"items": {
					"type": "object",
					"properties": {
						"id": { "type": "string" },
						"content": { "type": "string" },
						"status": { "type": "string", "enum": ["pending", "in_progress", "completed", "cancelled"] }
					},
					"required": ["id", "content", "status"]
				}
			},
			"merge": {
				"type": "boolean",
				"description": "If true, merge with existing todos by id. If false, replace all."
			}
		},
		"required": ["todos"]
	}`)
}

func (t *TodoWriteTool) NeedsPermission(_ json.RawMessage) bool { return false }

func (t *TodoWriteTool) FormatPermissionRequest(_ json.RawMessage) string {
	return "Update task list"
}

func (t *TodoWriteTool) Execute(input json.RawMessage, _ string) (string, error) {
	var in todoWriteInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	if !in.Merge {
		t.todos = in.Todos
	} else {
		existing := make(map[string]int)
		for i, item := range t.todos {
			existing[item.ID] = i
		}
		for _, item := range in.Todos {
			if idx, ok := existing[item.ID]; ok {
				t.todos[idx] = item
			} else {
				t.todos = append(t.todos, item)
			}
		}
	}

	return t.formatList(), nil
}

func (t *TodoWriteTool) formatList() string {
	if len(t.todos) == 0 {
		return "Task list is empty."
	}
	var sb strings.Builder
	for _, item := range t.todos {
		icon := statusIcon(item.Status)
		fmt.Fprintf(&sb, "%s %s: %s\n", icon, item.ID, item.Content)
	}
	return sb.String()
}

func statusIcon(status string) string {
	switch status {
	case "completed":
		return "[x]"
	case "in_progress":
		return "[>]"
	case "cancelled":
		return "[-]"
	default:
		return "[ ]"
	}
}
