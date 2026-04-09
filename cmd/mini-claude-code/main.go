package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/noknov/mini-claude-code/internal/config"
	"github.com/noknov/mini-claude-code/internal/context"
	"github.com/noknov/mini-claude-code/internal/provider"
	"github.com/noknov/mini-claude-code/internal/query"
	"github.com/noknov/mini-claude-code/internal/session"
	"github.com/noknov/mini-claude-code/internal/ui"
)

const version = "0.2.0"

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "--version", "-v":
			fmt.Printf("mini-claude-code v%s\n", version)
			return
		case "--help", "-h":
			printUsage()
			return
		}
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	if cfg.APIKey == "" {
		fmt.Fprintf(os.Stderr, "API key is not set.\n")
		if cfg.Provider == "openai" {
			fmt.Fprintln(os.Stderr, "Set OPENAI_API_KEY environment variable.")
		} else {
			fmt.Fprintln(os.Stderr, "Set ANTHROPIC_API_KEY environment variable.")
		}
		os.Exit(1)
	}

	prov := createProvider(cfg)
	sess := session.New()
	ctx := context.Gather(cfg.WorkDir)
	terminal := ui.NewTerminal(cfg)
	engine := query.NewEngine(prov, sess, ctx, cfg)

	terminal.PrintWelcome(version, prov.Name(), prov.Model(), cfg.WorkDir)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sig
		fmt.Println("\nGoodbye!")
		os.Exit(0)
	}()

	terminal.RunREPL(engine)
}

func createProvider(cfg *config.Config) provider.Provider {
	switch cfg.Provider {
	case "openai":
		return provider.NewOpenAI(cfg.APIKey, cfg.Model, cfg.BaseURL)
	default:
		return provider.NewAnthropic(cfg.APIKey, cfg.Model, cfg.BaseURL)
	}
}

func printUsage() {
	fmt.Println(`mini-claude-code — A minimal Claude Code implementation in Go

Usage:
  mini-claude-code [flags]

Flags:
  -h, --help         Show this help message
  -v, --version      Show version
  -m, --model        Set model (default: claude-sonnet-4-20250514)
  --provider NAME    LLM provider: anthropic (default), openai
  --auto             Auto-approve all tool calls
  --deny             Deny all tool calls

Environment:
  MINI_CLAUDE_PROVIDER   Provider selection (anthropic, openai)
  ANTHROPIC_API_KEY      Anthropic API key
  ANTHROPIC_BASE_URL     Custom Anthropic API endpoint
  OPENAI_API_KEY         OpenAI API key
  OPENAI_BASE_URL        Custom OpenAI-compatible endpoint

Commands (in REPL):
  /help    /clear    /cost    /model    /compact    /exit`)
}
