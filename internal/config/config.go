package config

import (
	"os"
	"path/filepath"
	"strings"
)

const DefaultModel = "claude-sonnet-4-20250514"

// Config holds all application settings.
// Priority (highest wins): CLI flags > environment variables > defaults.
type Config struct {
	APIKey         string
	Model          string
	BaseURL        string
	WorkDir        string
	PermissionMode string // "ask" (default), "auto", "deny"
}

// Load reads configuration from environment variables and CLI arguments.
func Load() (*Config, error) {
	workDir, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	model := DefaultModel
	if m := os.Getenv("ANTHROPIC_MODEL"); m != "" {
		model = m
	}
	for i, arg := range os.Args {
		if (arg == "-m" || arg == "--model") && i+1 < len(os.Args) {
			model = os.Args[i+1]
		}
	}

	return &Config{
		APIKey:         os.Getenv("ANTHROPIC_API_KEY"),
		Model:          model,
		BaseURL:        os.Getenv("ANTHROPIC_BASE_URL"),
		WorkDir:        workDir,
		PermissionMode: "ask",
	}, nil
}

// FindClaudeMD searches for CLAUDE.md files from workDir up to the filesystem
// root, then checks ~/.claude/CLAUDE.md. Files closer to workDir appear later
// (higher priority for the model).
func FindClaudeMD(workDir string) string {
	var parts []string
	dir := workDir
	for {
		path := filepath.Join(dir, "CLAUDE.md")
		if data, err := os.ReadFile(path); err == nil {
			parts = append(parts, string(data))
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	if home, err := os.UserHomeDir(); err == nil {
		path := filepath.Join(home, ".claude", "CLAUDE.md")
		if data, err := os.ReadFile(path); err == nil {
			parts = append(parts, string(data))
		}
	}

	return strings.Join(parts, "\n\n---\n\n")
}
