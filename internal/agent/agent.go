// Package agent loads and manages custom agent definitions from .claude/agents/.
//
// Agents are markdown files with optional YAML frontmatter specifying model,
// tools, permissions, and skills. They can be spawned as subagents (fork or fresh).
package agent

import (
	"os"
	"path/filepath"
	"strings"
)

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

// Definition describes a custom agent.
type Definition struct {
	Name           string
	Path           string
	Description    string
	Model          string
	Tools          []string // allowed tool names (empty = all)
	PermissionMode string   // "ask", "auto", "deny", "bubble"
	Skills         []string // skill names to preload
	Content        string   // the full prompt/instructions
}

// Mode controls how a subagent is spawned.
type Mode string

const (
	ModeFork  Mode = "fork"  // inherits parent conversation prefix
	ModeFresh Mode = "fresh" // starts with clean context
)

// ---------------------------------------------------------------------------
// Loading
// ---------------------------------------------------------------------------

// LoadAll discovers agent definitions from .claude/agents/ directories.
func LoadAll(workDir string) []Definition {
	var agents []Definition
	agents = loadAncestorAgents(agents, workDir)
	agents = loadUserAgents(agents)
	return agents
}

func loadAncestorAgents(agents []Definition, workDir string) []Definition {
	for dir := workDir; ; {
		agents = append(agents, scanDir(filepath.Join(dir, ".claude", "agents"))...)
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return agents
}

func loadUserAgents(agents []Definition) []Definition {
	home, _ := os.UserHomeDir()
	if home == "" {
		return agents
	}
	return append(agents, scanDir(filepath.Join(home, ".claude", "agents"))...)
}

func scanDir(dir string) []Definition {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var agents []Definition
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		def := parseFrontmatter(string(data))
		def.Name = strings.TrimSuffix(e.Name(), ".md")
		def.Path = path
		agents = append(agents, def)
	}
	return agents
}

// ---------------------------------------------------------------------------
// Frontmatter parsing
// ---------------------------------------------------------------------------

func parseFrontmatter(content string) Definition {
	def := Definition{PermissionMode: "ask"}

	if !strings.HasPrefix(content, "---\n") {
		def.Content = content
		return def
	}
	end := strings.Index(content[4:], "\n---")
	if end < 0 {
		def.Content = content
		return def
	}

	def.Content = strings.TrimSpace(content[4+end+4:])

	var currentList *[]string
	for _, line := range strings.Split(content[4:4+end], "\n") {
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "- ") && currentList != nil {
			val := strings.Trim(strings.TrimPrefix(trimmed, "- "), `"'`)
			*currentList = append(*currentList, val)
			continue
		}

		if strings.HasSuffix(trimmed, ":") && !strings.Contains(trimmed, " ") {
			switch strings.TrimSuffix(trimmed, ":") {
			case "tools":
				currentList = &def.Tools
			case "skills":
				currentList = &def.Skills
			default:
				currentList = nil
			}
			continue
		}

		currentList = nil
		if k, v := parseKV(trimmed); k != "" {
			switch k {
			case "description":
				def.Description = v
			case "model":
				def.Model = v
			case "permission_mode":
				def.PermissionMode = v
			}
		}
	}
	return def
}

func parseKV(line string) (string, string) {
	idx := strings.Index(line, ": ")
	if idx < 0 {
		return "", ""
	}
	return strings.TrimSpace(line[:idx]), strings.Trim(strings.TrimSpace(line[idx+2:]), `"'`)
}

// ---------------------------------------------------------------------------
// Lookup
// ---------------------------------------------------------------------------

// Find returns an agent definition by name, or nil if not found.
func Find(agents []Definition, name string) *Definition {
	for i := range agents {
		if agents[i].Name == name {
			return &agents[i]
		}
	}
	return nil
}

// Names returns all agent names.
func Names(agents []Definition) []string {
	names := make([]string, len(agents))
	for i, a := range agents {
		names[i] = a.Name
	}
	return names
}
