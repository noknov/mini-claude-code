package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/noknov/mini-claude-code/internal/api"
	"github.com/noknov/mini-claude-code/internal/config"
	"github.com/noknov/mini-claude-code/internal/context"
	"github.com/noknov/mini-claude-code/internal/query"
	"github.com/noknov/mini-claude-code/internal/session"
	"github.com/noknov/mini-claude-code/internal/ui"
)

const version = "0.1.0"

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
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	if cfg.APIKey == "" {
		fmt.Fprintln(os.Stderr, "ANTHROPIC_API_KEY is not set.")
		fmt.Fprintln(os.Stderr, "Set it via environment variable or run with --help for more info.")
		os.Exit(1)
	}

	client := api.NewClient(cfg.APIKey, cfg.Model, cfg.BaseURL)
	sess := session.New()
	ctx := context.Gather(cfg.WorkDir)

	terminal := ui.NewTerminal(cfg)
	terminal.PrintWelcome(version, cfg.Model, cfg.WorkDir)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	engine := query.NewEngine(client, sess, ctx, cfg)

	go func() {
		<-sigCh
		fmt.Println("\nGoodbye!")
		os.Exit(0)
	}()

	terminal.RunREPL(engine)
}

func printUsage() {
	fmt.Println(`mini-claude-code - A minimal Claude Code implementation in Go

Usage:
  mini-claude-code [flags] [initial prompt]

Flags:
  -h, --help       Show this help message
  -v, --version    Show version
  -m, --model      Set model (default: claude-sonnet-4-20250514)
  -p, --print      Non-interactive mode: print response and exit

Environment:
  ANTHROPIC_API_KEY    API key for Anthropic (required)
  ANTHROPIC_BASE_URL   Custom API base URL

Slash Commands (in REPL):
  /help       Show available commands
  /compact    Compress conversation context
  /clear      Clear conversation history
  /cost       Show token usage and cost
  /model      Switch model
  /exit       Exit the program`)
}
