package config

import (
	"os"
	"path/filepath"
	"strings"
)

const (
	DefaultAnthropicModel = "claude-sonnet-4-20250514"
	DefaultOpenAIModel    = "gpt-4o"
)

// Config holds all application settings.
// Priority (highest wins): CLI flags > environment variables > defaults.
type Config struct {
	Provider       string // "anthropic" (default), "openai"
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

	prov := "anthropic"
	if p := os.Getenv("MINI_CLAUDE_PROVIDER"); p != "" {
		prov = p
	}

	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	baseURL := os.Getenv("ANTHROPIC_BASE_URL")
	model := DefaultAnthropicModel
	if m := os.Getenv("ANTHROPIC_MODEL"); m != "" {
		model = m
	}

	if prov == "openai" {
		apiKey = os.Getenv("OPENAI_API_KEY")
		baseURL = os.Getenv("OPENAI_BASE_URL")
		model = DefaultOpenAIModel
		if m := os.Getenv("OPENAI_MODEL"); m != "" {
			model = m
		}
	}

	permMode := "ask"

	for i, arg := range os.Args {
		switch arg {
		case "-m", "--model":
			if i+1 < len(os.Args) {
				model = os.Args[i+1]
			}
		case "--provider":
			if i+1 < len(os.Args) {
				prov = os.Args[i+1]
			}
		case "--auto":
			permMode = "auto"
		case "--deny":
			permMode = "deny"
		}
	}

	return &Config{
		Provider:       prov,
		APIKey:         apiKey,
		Model:          model,
		BaseURL:        baseURL,
		WorkDir:        workDir,
		PermissionMode: permMode,
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
