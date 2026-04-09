// Package rules loads instruction rules from .claude/rules/*.md.
//
// Rules can be unconditional (always active) or conditional (only active
// when operating on files matching YAML frontmatter "paths:" globs).
package rules

import (
	"os"
	"path/filepath"
	"strings"
)

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

// Rule is a single instruction loaded from a markdown file.
type Rule struct {
	Path    string
	Content string
	Globs   []string // non-empty → conditional rule
}

func (r *Rule) IsConditional() bool { return len(r.Globs) > 0 }

// Matches reports whether this rule applies to the given file path.
func (r *Rule) Matches(target string) bool {
	if !r.IsConditional() {
		return true
	}
	for _, g := range r.Globs {
		if matchAny(g, target) {
			return true
		}
	}
	return false
}

func matchAny(glob, target string) bool {
	if ok, _ := filepath.Match(glob, target); ok {
		return true
	}
	if ok, _ := filepath.Match(glob, filepath.Base(target)); ok {
		return true
	}
	return false
}

// ---------------------------------------------------------------------------
// Loading
// ---------------------------------------------------------------------------

// LoadAll discovers rules from .claude/rules/ in ancestor dirs + user home.
func LoadAll(workDir string) []Rule {
	var rules []Rule
	rules = loadAncestorRules(rules, workDir)
	rules = loadUserRules(rules)
	return rules
}

func loadAncestorRules(rules []Rule, workDir string) []Rule {
	for dir := workDir; ; {
		rules = append(rules, scanDir(filepath.Join(dir, ".claude", "rules"))...)
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return rules
}

func loadUserRules(rules []Rule) []Rule {
	home, _ := os.UserHomeDir()
	if home == "" {
		return rules
	}
	return append(rules, scanDir(filepath.Join(home, ".claude", "rules"))...)
}

func scanDir(dir string) []Rule {
	var rules []Rule
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}
		if r := parseFile(path); r != nil {
			rules = append(rules, *r)
		}
		return nil
	})
	return rules
}

func parseFile(path string) *Rule {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	content := string(data)
	body := stripFrontmatter(content)
	if strings.TrimSpace(body) == "" {
		return nil
	}
	return &Rule{
		Path:    path,
		Content: body,
		Globs:   parseFrontmatterGlobs(content),
	}
}

// ---------------------------------------------------------------------------
// Frontmatter parsing
// ---------------------------------------------------------------------------

// parseFrontmatterGlobs extracts paths: list from YAML frontmatter.
func parseFrontmatterGlobs(content string) []string {
	if !strings.HasPrefix(content, "---\n") {
		return nil
	}
	end := strings.Index(content[4:], "\n---")
	if end < 0 {
		return nil
	}

	var globs []string
	inPaths := false
	for _, line := range strings.Split(content[4:4+end], "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "paths:" {
			inPaths = true
			continue
		}
		if inPaths && strings.HasPrefix(trimmed, "- ") {
			glob := strings.Trim(strings.TrimPrefix(trimmed, "- "), `"'`)
			if glob != "" {
				globs = append(globs, glob)
			}
		} else if inPaths {
			break
		}
	}
	return globs
}

func stripFrontmatter(content string) string {
	if !strings.HasPrefix(content, "---\n") {
		return content
	}
	end := strings.Index(content[4:], "\n---")
	if end < 0 {
		return content
	}
	return strings.TrimSpace(content[4+end+4:])
}

// ---------------------------------------------------------------------------
// Formatting
// ---------------------------------------------------------------------------

// FormatUnconditional returns all unconditional rules joined for the prompt.
func FormatUnconditional(rules []Rule) string {
	var parts []string
	for _, r := range rules {
		if !r.IsConditional() {
			parts = append(parts, r.Content)
		}
	}
	return strings.Join(parts, "\n\n---\n\n")
}

// FormatConditional returns conditional rules matching any of the given paths.
func FormatConditional(rules []Rule, paths []string) string {
	var parts []string
	for _, r := range rules {
		if !r.IsConditional() {
			continue
		}
		for _, p := range paths {
			if r.Matches(p) {
				parts = append(parts, r.Content)
				break
			}
		}
	}
	return strings.Join(parts, "\n\n---\n\n")
}
