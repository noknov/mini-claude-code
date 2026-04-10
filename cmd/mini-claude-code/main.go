package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/noknov/mini-claude-code/internal/config"
	"github.com/noknov/mini-claude-code/internal/context"
	"github.com/noknov/mini-claude-code/internal/history"
	"github.com/noknov/mini-claude-code/internal/provider"
	"github.com/noknov/mini-claude-code/internal/query"
	"github.com/noknov/mini-claude-code/internal/session"
	"github.com/noknov/mini-claude-code/internal/ui"
)

const version = "0.3.0"

func main() {
	if handleMetaFlags() {
		return
	}

	cfg, err := config.Load()
	if err != nil {
		fatal("Error: %v", err)
	}
	if cfg.APIKey == "" {
		printMissingKeyHelp(cfg.Provider)
		os.Exit(1)
	}

	prov := createProvider(cfg)
	sess := session.New(history.GenerateID())
	ctx := context.Gather(cfg.WorkDir)
	engine := query.NewEngine(prov, sess, ctx, cfg)

	if cfg.PipePrompt != "" {
		runNonInteractive(engine, cfg.PipePrompt)
		return
	}

	terminal := ui.NewTerminal(cfg, ctx.Skills)
	terminal.InitLiner()
	defer terminal.Close()

	terminal.PrintWelcome(version, prov.Name(), prov.Model(), cfg.WorkDir)
	setupSignalHandler(engine, terminal)
	terminal.RunREPL(engine)
}

// ---------------------------------------------------------------------------
// Meta flags (--version, --help)
// ---------------------------------------------------------------------------

func handleMetaFlags() bool {
	if len(os.Args) < 2 {
		return false
	}
	switch os.Args[1] {
	case "--version", "-v":
		fmt.Printf("mini-claude-code v%s\n", version)
		return true
	case "--help", "-h":
		printUsage()
		return true
	}
	return false
}

// ---------------------------------------------------------------------------
// Provider factory
// ---------------------------------------------------------------------------

func createProvider(cfg *config.Config) provider.Provider {
	switch cfg.Provider {
	case "openai":
		return provider.NewOpenAI(cfg.APIKey, cfg.Model, cfg.BaseURL, cfg.ContextWindow)
	default:
		return provider.NewAnthropic(cfg.APIKey, cfg.Model, cfg.BaseURL)
	}
}

// ---------------------------------------------------------------------------
// Non-interactive mode (-p)
// ---------------------------------------------------------------------------

func runNonInteractive(engine *query.Engine, prompt string) {
	terminal := ui.NewTerminal(&config.Config{}, nil)
	engine.Run(prompt, terminal)
}

// ---------------------------------------------------------------------------
// Signal handling
// ---------------------------------------------------------------------------

func setupSignalHandler(engine *query.Engine, terminal *ui.Terminal) {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		for range sig {
			if engine.IsRunning() {
				engine.Cancel()
			} else {
				terminal.Close()
				fmt.Println("\nGoodbye!")
				os.Exit(0)
			}
		}
	}()
}

// ---------------------------------------------------------------------------
// Help
// ---------------------------------------------------------------------------

func printMissingKeyHelp(prov string) {
	fmt.Fprintf(os.Stderr, "API key is not set (provider: %s).\n\n", prov)
	fmt.Fprintln(os.Stderr, "Set the API key for your provider:")
	fmt.Fprintln(os.Stderr, "  ANTHROPIC_API_KEY    for Anthropic (default)")
	fmt.Fprintln(os.Stderr, "  OPENAI_API_KEY       for OpenAI / DeepSeek / compatible")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Switch provider with MINI_CLAUDE_PROVIDER or --provider.")
}

func printUsage() {
	fmt.Println(`mini-claude-code — A minimal Claude Code implementation in Go

Usage:
  mini-claude-code [flags]
  mini-claude-code -p "prompt"    Run non-interactively

Flags:
  -h, --help         Show this help message
  -v, --version      Show version
  -m, --model        Set model (default: claude-sonnet-4-20250514)
  --provider NAME    LLM provider: anthropic (default), openai
  --auto             Auto-approve all tool calls
  --deny             Deny all tool calls
  --plan             Plan mode (read-only)
  -p, --prompt       Non-interactive mode: run prompt and exit

Environment:
  MINI_CLAUDE_PROVIDER   Provider selection (anthropic, openai)
  ANTHROPIC_API_KEY      Anthropic API key
  ANTHROPIC_BASE_URL     Custom Anthropic API endpoint
  OPENAI_API_KEY         OpenAI API key
  OPENAI_BASE_URL        Custom OpenAI-compatible endpoint

REPL Commands:
  /help  /clear  /model  /compact  /skills  /memory  /exit`)
}

func fatal(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
