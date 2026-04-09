// Package memory implements the 5-layer instruction/memory system.
//
// Load order (later = higher priority for the model):
//  1. Managed  (/etc/claude-code/CLAUDE.md)
//  2. User     (~/.claude/CLAUDE.md)
//  3. Project  (CLAUDE.md + .claude/CLAUDE.md in ancestor dirs)
//  4. Local    (CLAUDE.local.md — not committed to git)
//  5. AutoMem  (~/.claude/projects/<slug>/MEMORY.md)
package memory

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

// Type indicates the source layer of a memory file.
type Type int

const (
	Managed Type = iota
	User
	Project
	Local
	AutoMem
)

// File is a single loaded instruction/memory file.
type File struct {
	Path    string
	Type    Type
	Content string
}

// ---------------------------------------------------------------------------
// Loading
// ---------------------------------------------------------------------------

// LoadAll discovers and loads all memory files in priority order.
func LoadAll(workDir string) []File {
	var files []File
	files = loadManaged(files)
	files = loadUser(files)
	files = loadProject(files, workDir)
	files = loadLocal(files, workDir)
	files = loadAutoMemory(files, workDir)
	return files
}

func loadManaged(files []File) []File {
	return appendIfReadable(files, "/etc/claude-code/CLAUDE.md", Managed)
}

func loadUser(files []File) []File {
	home, _ := os.UserHomeDir()
	if home == "" {
		return files
	}
	return appendIfReadable(files, filepath.Join(home, ".claude", "CLAUDE.md"), User)
}

func loadProject(files []File, workDir string) []File {
	// Walk up, collect, then reverse so root-level = lower priority.
	var found []File
	for dir := workDir; ; {
		found = appendIfReadable(found, filepath.Join(dir, "CLAUDE.md"), Project)
		found = appendIfReadable(found, filepath.Join(dir, ".claude", "CLAUDE.md"), Project)
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	for i := len(found) - 1; i >= 0; i-- {
		files = append(files, found[i])
	}
	return files
}

func loadLocal(files []File, workDir string) []File {
	return appendIfReadable(files, filepath.Join(workDir, "CLAUDE.local.md"), Local)
}

func loadAutoMemory(files []File, workDir string) []File {
	path := AutoMemoryPath(workDir)
	if path == "" {
		return files
	}
	return appendIfReadable(files, path, AutoMem)
}

func appendIfReadable(files []File, path string, typ Type) []File {
	data, err := os.ReadFile(path)
	if err != nil {
		return files
	}
	content := strings.TrimSpace(string(data))
	if content == "" {
		return files
	}
	return append(files, File{Path: path, Type: typ, Content: content})
}

// ---------------------------------------------------------------------------
// Formatting
// ---------------------------------------------------------------------------

// FormatForPrompt concatenates all memory files into a single string.
func FormatForPrompt(files []File) string {
	parts := make([]string, 0, len(files))
	for _, f := range files {
		parts = append(parts, f.Content)
	}
	return strings.Join(parts, "\n\n---\n\n")
}

// ---------------------------------------------------------------------------
// Auto-memory persistence
// ---------------------------------------------------------------------------

// AutoMemoryPath returns the path to the project's auto-memory file.
func AutoMemoryPath(workDir string) string {
	home, _ := os.UserHomeDir()
	if home == "" {
		return ""
	}
	return filepath.Join(home, ".claude", "projects", projectSlug(workDir), "MEMORY.md")
}

// AutoMemoryDir returns the directory for project-specific auto-memory.
func AutoMemoryDir(workDir string) string {
	p := AutoMemoryPath(workDir)
	if p == "" {
		return ""
	}
	return filepath.Dir(p)
}

// ReadAutoMemory reads the current auto-memory content.
func ReadAutoMemory(workDir string) string {
	path := AutoMemoryPath(workDir)
	if path == "" {
		return ""
	}
	data, _ := os.ReadFile(path)
	return string(data)
}

// WriteAutoMemory persists content to the auto-memory file.
func WriteAutoMemory(workDir, content string) error {
	path := AutoMemoryPath(workDir)
	if path == "" {
		return fmt.Errorf("cannot determine auto-memory path")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0644)
}

func projectSlug(workDir string) string {
	h := sha256.Sum256([]byte(workDir))
	return fmt.Sprintf("%s-%x", filepath.Base(workDir), h[:4])
}
