package tools

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// NotebookEditTool edits Jupyter notebook cells.
type NotebookEditTool struct{}

type notebookEditInput struct {
	Path      string `json:"path"`
	CellIndex int    `json:"cell_index"`
	NewSource string `json:"new_source"`
	CellType  string `json:"cell_type,omitempty"` // "code", "markdown", "raw"
	IsNewCell bool   `json:"is_new_cell,omitempty"`
}

func (t *NotebookEditTool) Name() string { return "NotebookEdit" }

func (t *NotebookEditTool) Description() string {
	return `Edit a Jupyter notebook cell. Can modify existing cells or insert new ones. Supports code, markdown, and raw cell types.`
}

func (t *NotebookEditTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {
				"type": "string",
				"description": "Path to the .ipynb notebook file"
			},
			"cell_index": {
				"type": "integer",
				"description": "0-based index of the cell to edit or insert at"
			},
			"new_source": {
				"type": "string",
				"description": "New cell content"
			},
			"cell_type": {
				"type": "string",
				"description": "Cell type: code, markdown, or raw (default: code)"
			},
			"is_new_cell": {
				"type": "boolean",
				"description": "If true, insert a new cell at the index"
			}
		},
		"required": ["path", "cell_index", "new_source"]
	}`)
}

func (t *NotebookEditTool) NeedsPermission(_ json.RawMessage) bool { return true }

func (t *NotebookEditTool) FormatPermissionRequest(input json.RawMessage) string {
	var in notebookEditInput
	_ = json.Unmarshal(input, &in)
	return fmt.Sprintf("Edit notebook: %s (cell %d)", in.Path, in.CellIndex)
}

func (t *NotebookEditTool) Execute(input json.RawMessage, workDir string) (string, error) {
	var in notebookEditInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("invalid input: %w", err)
	}

	path := resolvePath(in.Path, workDir)
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read notebook: %w", err)
	}

	var notebook map[string]interface{}
	if err := json.Unmarshal(data, &notebook); err != nil {
		return "", fmt.Errorf("parse notebook: %w", err)
	}

	cells, ok := notebook["cells"].([]interface{})
	if !ok {
		return "", fmt.Errorf("invalid notebook format: missing cells array")
	}

	cellType := in.CellType
	if cellType == "" {
		cellType = "code"
	}

	newCell := map[string]interface{}{
		"cell_type": cellType,
		"source":    splitSource(in.NewSource),
		"metadata":  map[string]interface{}{},
	}
	if cellType == "code" {
		newCell["outputs"] = []interface{}{}
		newCell["execution_count"] = nil
	}

	if in.IsNewCell {
		if in.CellIndex > len(cells) {
			in.CellIndex = len(cells)
		}
		cells = append(cells[:in.CellIndex], append([]interface{}{newCell}, cells[in.CellIndex:]...)...)
	} else {
		if in.CellIndex >= len(cells) {
			return "", fmt.Errorf("cell index %d out of range (notebook has %d cells)", in.CellIndex, len(cells))
		}
		cells[in.CellIndex] = newCell
	}

	notebook["cells"] = cells
	output, err := json.MarshalIndent(notebook, "", " ")
	if err != nil {
		return "", fmt.Errorf("marshal notebook: %w", err)
	}

	if err := os.WriteFile(path, output, 0644); err != nil {
		return "", fmt.Errorf("write notebook: %w", err)
	}

	action := "Updated"
	if in.IsNewCell {
		action = "Inserted"
	}
	return fmt.Sprintf("%s cell %d in %s", action, in.CellIndex, path), nil
}

func splitSource(s string) []string {
	lines := strings.Split(s, "\n")
	result := make([]string, len(lines))
	for i, line := range lines {
		if i < len(lines)-1 {
			result[i] = line + "\n"
		} else {
			result[i] = line
		}
	}
	return result
}
