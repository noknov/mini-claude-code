// Package skills loads reusable prompt templates from .claude/commands/.
//
// Skills are markdown files that can be invoked as slash commands (/skill-name)
// or loaded by the SkillTool for the model to use programmatically.
package skills

import (
	"os"
	"path/filepath"
	"strings"
)

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

// Skill is a reusable prompt template.
type Skill struct {
	Name    string // e.g. "review" or "deploy:staging"
	Path    string
	Content string
}

// ---------------------------------------------------------------------------
// Loading
// ---------------------------------------------------------------------------

// LoadAll discovers skills from project + user command directories.
func LoadAll(workDir string) []Skill {
	var skills []Skill
	skills = loadAncestorSkills(skills, workDir)
	skills = loadUserSkills(skills)
	return dedup(skills)
}

func loadAncestorSkills(skills []Skill, workDir string) []Skill {
	for dir := workDir; ; {
		skills = append(skills, scanDir(filepath.Join(dir, ".claude", "commands"))...)
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return skills
}

func loadUserSkills(skills []Skill) []Skill {
	home, _ := os.UserHomeDir()
	if home == "" {
		return skills
	}
	return append(skills, scanDir(filepath.Join(home, ".claude", "commands"))...)
}

func scanDir(dir string) []Skill {
	var skills []Skill
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		content := strings.TrimSpace(string(data))
		if content == "" {
			return nil
		}
		rel, _ := filepath.Rel(dir, path)
		name := strings.TrimSuffix(rel, ".md")
		name = strings.ReplaceAll(name, string(filepath.Separator), ":")
		skills = append(skills, Skill{Name: name, Path: path, Content: content})
		return nil
	})
	return skills
}

func dedup(skills []Skill) []Skill {
	seen := make(map[string]bool)
	var result []Skill
	for _, s := range skills {
		if !seen[s.Name] {
			seen[s.Name] = true
			result = append(result, s)
		}
	}
	return result
}

// ---------------------------------------------------------------------------
// Lookup
// ---------------------------------------------------------------------------

// Find returns a skill by name, or nil if not found.
func Find(skills []Skill, name string) *Skill {
	for i := range skills {
		if skills[i].Name == name {
			return &skills[i]
		}
	}
	return nil
}

// Names returns all skill names.
func Names(skills []Skill) []string {
	names := make([]string, len(skills))
	for i, s := range skills {
		names[i] = s.Name
	}
	return names
}

// ---------------------------------------------------------------------------
// Formatting
// ---------------------------------------------------------------------------

// FormatListing returns a formatted list for the system prompt.
func FormatListing(skills []Skill) string {
	if len(skills) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("Available skills (invoke with /skill-name):\n")
	for _, s := range skills {
		sb.WriteString("  - " + s.Name)
		if first := firstLine(s.Content); first != "" {
			sb.WriteString(": " + first)
		}
		sb.WriteByte('\n')
	}
	return sb.String()
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return strings.TrimSpace(s)
}
