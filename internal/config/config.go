package config

import (
	"os"
	"path/filepath"
	"strings"
)

const (
	DefaultModel = "claude-sonnet-4-20250514"
)

type Config struct {
	APIKey  string
	Model   string
	BaseURL string
	WorkDir string

	// Permission mode: "ask" (default), "auto" (accept all), "deny" (reject all)
	PermissionMode string
}

func Load() (*Config, error) {
	workDir, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	model := DefaultModel
	if m := os.Getenv("ANTHROPIC_MODEL"); m != "" {
		model = m
	}

	// Check CLI args for model override
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

// FindClaudeMD searches for CLAUDE.md files up the directory tree
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

	// Also check ~/.claude/CLAUDE.md
	if home, err := os.UserHomeDir(); err == nil {
		path := filepath.Join(home, ".claude", "CLAUDE.md")
		if data, err := os.ReadFile(path); err == nil {
			parts = append(parts, string(data))
		}
	}

	return strings.Join(parts, "\n\n---\n\n")
}
