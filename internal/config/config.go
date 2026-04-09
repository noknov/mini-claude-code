// Package config handles CLI argument parsing and environment variable loading.
//
// Priority (highest wins): CLI flags > environment variables > defaults.
package config

import (
	"os"
	"strings"
)

const (
	DefaultAnthropicModel = "claude-sonnet-4-20250514"
	DefaultOpenAIModel    = "gpt-4o"
)

// Config holds all application settings.
type Config struct {
	Provider       string // "anthropic" (default), "openai"
	APIKey         string
	Model          string
	BaseURL        string
	WorkDir        string
	PermissionMode string // "ask" (default), "auto", "deny", "plan"

	// Non-interactive mode
	PipePrompt string // if set, run this prompt and exit
	PrintOnly  bool   // print response without tool use
}

// Load reads configuration from environment variables and CLI arguments.
func Load() (*Config, error) {
	workDir, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	cfg := &Config{
		Provider:       detectProvider(),
		WorkDir:        workDir,
		PermissionMode: "ask",
	}

	cfg.APIKey, cfg.BaseURL, cfg.Model = loadProviderConfig(cfg.Provider)
	cfg.applyFlags()

	return cfg, nil
}

// ---------------------------------------------------------------------------
// Provider detection
// ---------------------------------------------------------------------------

func detectProvider() string {
	if p := os.Getenv("MINI_CLAUDE_PROVIDER"); p != "" {
		return p
	}
	return "anthropic"
}

func loadProviderConfig(prov string) (apiKey, baseURL, model string) {
	switch prov {
	case "openai":
		apiKey = os.Getenv("OPENAI_API_KEY")
		baseURL = os.Getenv("OPENAI_BASE_URL")
		model = envOrDefault("OPENAI_MODEL", DefaultOpenAIModel)
	default:
		apiKey = os.Getenv("ANTHROPIC_API_KEY")
		baseURL = os.Getenv("ANTHROPIC_BASE_URL")
		model = envOrDefault("ANTHROPIC_MODEL", DefaultAnthropicModel)
	}
	return
}

// ---------------------------------------------------------------------------
// CLI flag parsing
// ---------------------------------------------------------------------------

func (cfg *Config) applyFlags() {
	args := os.Args[1:]
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-m", "--model":
			if i+1 < len(args) {
				cfg.Model = args[i+1]
				i++
			}
		case "--provider":
			if i+1 < len(args) {
				cfg.Provider = args[i+1]
				cfg.APIKey, cfg.BaseURL, cfg.Model = loadProviderConfig(cfg.Provider)
				i++
			}
		case "--auto":
			cfg.PermissionMode = "auto"
		case "--deny":
			cfg.PermissionMode = "deny"
		case "--plan":
			cfg.PermissionMode = "plan"
		case "-p", "--prompt":
			if i+1 < len(args) {
				cfg.PipePrompt = strings.Join(args[i+1:], " ")
				return
			}
		case "--print":
			cfg.PrintOnly = true
		}
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func envOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}
