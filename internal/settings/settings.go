// Package settings implements multi-layer settings merge.
//
// Load order (later overrides earlier):
//  1. Managed  (/etc/claude-code/settings.json)
//  2. User     (~/.claude/settings.json)
//  3. Project  (.claude/settings.json in each ancestor dir)
//  4. Local    (.claude/settings.local.json — not committed to git)
package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

// Settings is the merged result from all sources.
type Settings struct {
	Hooks          map[string][]HookEntry `json:"hooks,omitempty"`
	Permissions    PermissionSettings     `json:"permissions,omitempty"`
	AutoMemory     *bool                  `json:"autoMemoryEnabled,omitempty"`
	AutoCompact    *bool                  `json:"autoCompactEnabled,omitempty"`
	OutputLanguage string                 `json:"outputLanguage,omitempty"`
	OutputStyle    string                 `json:"outputStyle,omitempty"`
	SandboxMode    string                 `json:"sandboxMode,omitempty"`
}

// PermissionSettings holds allow/deny rules for tool execution.
type PermissionSettings struct {
	Allow []PermissionRule `json:"allow,omitempty"`
	Deny  []PermissionRule `json:"deny,omitempty"`
}

// PermissionRule matches a tool name + optional argument glob.
type PermissionRule struct {
	Tool    string `json:"tool"`
	Pattern string `json:"pattern,omitempty"`
}

// HookEntry is a single hook definition within a lifecycle event.
type HookEntry struct {
	Command string `json:"command,omitempty"`
	If      string `json:"if,omitempty"`
}

// ---------------------------------------------------------------------------
// Loading
// ---------------------------------------------------------------------------

// Load discovers and merges settings from all sources.
func Load(workDir string) *Settings {
	merged := &Settings{Hooks: make(map[string][]HookEntry)}
	for _, path := range discoverPaths(workDir) {
		if s := readFile(path); s != nil {
			merge(merged, s)
		}
	}
	return merged
}

// discoverPaths returns settings file paths in load order (low → high priority).
func discoverPaths(workDir string) []string {
	home, _ := os.UserHomeDir()
	var paths []string

	// 1. Managed
	paths = append(paths, "/etc/claude-code/settings.json")

	// 2. User
	if home != "" {
		paths = append(paths, filepath.Join(home, ".claude", "settings.json"))
	}

	// 3. Project — walk up (root-level first = lower priority)
	var ancestors []string
	for dir := workDir; ; {
		ancestors = append(ancestors, filepath.Join(dir, ".claude", "settings.json"))
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	for i := len(ancestors) - 1; i >= 0; i-- {
		paths = append(paths, ancestors[i])
	}

	// 4. Local (highest priority)
	paths = append(paths, filepath.Join(workDir, ".claude", "settings.local.json"))
	return paths
}

func readFile(path string) *Settings {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var s Settings
	if json.Unmarshal(data, &s) != nil {
		return nil
	}
	return &s
}

func merge(dst, src *Settings) {
	for event, hooks := range src.Hooks {
		dst.Hooks[event] = append(dst.Hooks[event], hooks...)
	}
	dst.Permissions.Allow = append(dst.Permissions.Allow, src.Permissions.Allow...)
	dst.Permissions.Deny = append(dst.Permissions.Deny, src.Permissions.Deny...)
	if src.AutoMemory != nil {
		dst.AutoMemory = src.AutoMemory
	}
	if src.AutoCompact != nil {
		dst.AutoCompact = src.AutoCompact
	}
	if src.OutputLanguage != "" {
		dst.OutputLanguage = src.OutputLanguage
	}
	if src.OutputStyle != "" {
		dst.OutputStyle = src.OutputStyle
	}
	if src.SandboxMode != "" {
		dst.SandboxMode = src.SandboxMode
	}
}

// ---------------------------------------------------------------------------
// Convenience accessors
// ---------------------------------------------------------------------------

func (s *Settings) IsAutoMemoryEnabled() bool  { return s.AutoMemory == nil || *s.AutoMemory }
func (s *Settings) IsAutoCompactEnabled() bool { return s.AutoCompact == nil || *s.AutoCompact }
