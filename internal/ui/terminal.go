package ui

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/noknov/mini-claude-code/internal/config"
)

// ANSI escape sequences for terminal styling.
const (
	reset  = "\033[0m"
	red    = "\033[31m"
	green  = "\033[32m"
	yellow = "\033[33m"
	cyan   = "\033[36m"
	dim    = "\033[2m"
	bold   = "\033[1m"
)

// Terminal handles all user-facing I/O.
type Terminal struct {
	cfg       *config.Config
	reader    *bufio.Reader
	streaming bool
}

func NewTerminal(cfg *config.Config) *Terminal {
	return &Terminal{
		cfg:    cfg,
		reader: bufio.NewReader(os.Stdin),
	}
}

// ---------------------------------------------------------------------------
// Display helpers
// ---------------------------------------------------------------------------

func (t *Terminal) PrintWelcome(version, providerName, model, workDir string) {
	fmt.Printf("\n%s%s mini-claude-code%s v%s\n", bold, cyan, reset, version)
	fmt.Printf("  Provider: %s\n", providerName)
	fmt.Printf("  Model:    %s%s%s\n", bold, model, reset)
	fmt.Printf("  Dir:      %s\n", workDir)
	fmt.Printf("  %sTip: /help for commands, Ctrl+C to exit%s\n\n", dim, reset)
}

func (t *Terminal) ReadInput() (string, error) {
	fmt.Printf("%s> %s", green, reset)
	line, err := t.reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

func (t *Terminal) StartStreaming() {
	t.streaming = true
	fmt.Print("\n")
}

func (t *Terminal) StreamText(text string) {
	fmt.Print(text)
}

func (t *Terminal) StopStreaming() {
	if t.streaming {
		fmt.Println()
		t.streaming = false
	}
}

func (t *Terminal) PrintToolUse(name string, input interface{}) {
	summary := formatToolInput(input)
	fmt.Printf("\n%s> %s%s %s%s%s\n", yellow, name, reset, dim, summary, reset)
}

func (t *Terminal) PrintToolResult(name, result string) {
	lines := strings.Split(result, "\n")
	if len(lines) > 10 {
		for _, l := range lines[:5] {
			fmt.Printf("  %s%s%s\n", dim, l, reset)
		}
		fmt.Printf("  %s... (%d more lines)%s\n", dim, len(lines)-5, reset)
	} else if result != "" {
		for _, l := range lines {
			fmt.Printf("  %s%s%s\n", dim, l, reset)
		}
	}
}

func (t *Terminal) PrintToolError(name string, err error) {
	fmt.Printf("  %s✗ %s: %v%s\n", red, name, err, reset)
}

func (t *Terminal) PrintToolDenied(name string) {
	fmt.Printf("  %s⊘ %s: denied%s\n", yellow, name, reset)
}

func (t *Terminal) PrintError(err error) {
	fmt.Printf("%s✗ Error: %v%s\n", red, err, reset)
}

func (t *Terminal) PrintInfo(msg string) {
	fmt.Printf("%s%s%s\n", dim, msg, reset)
}

func (t *Terminal) PrintSuccess(msg string) {
	fmt.Printf("%s✓ %s%s\n", green, msg, reset)
}

// AskPermission implements the permission.Asker interface.
func (t *Terminal) AskPermission(toolName, description string) string {
	fmt.Printf("\n%s? %s%s\n", yellow, description, reset)
	fmt.Printf("  %sAllow? [Y/n/a(lways)] %s", dim, reset)
	line, _ := t.reader.ReadString('\n')
	return strings.TrimSpace(line)
}

// ---------------------------------------------------------------------------
// REPL
// ---------------------------------------------------------------------------

// REPLEngine is the interface the REPL loop needs from the query engine.
type REPLEngine interface {
	Run(input string, terminal *Terminal)
	SessionInfo() (inputTokens, outputTokens int, cost float64)
	ClearSession()
	SetModel(model string)
	GetModel() string
}

func (t *Terminal) RunREPL(engine REPLEngine) {
	for {
		input, err := t.ReadInput()
		if err != nil {
			break
		}
		if input == "" {
			continue
		}
		if strings.HasPrefix(input, "/") {
			if t.handleCommand(input, engine) {
				continue
			}
		}
		engine.Run(input, t)
	}
}

func (t *Terminal) handleCommand(input string, engine REPLEngine) bool {
	parts := strings.Fields(input)
	switch parts[0] {
	case "/help":
		t.printHelp()
	case "/clear":
		engine.ClearSession()
		t.PrintSuccess("Conversation cleared")
	case "/cost":
		in, out, cost := engine.SessionInfo()
		fmt.Printf("\n%sToken Usage:%s\n", bold, reset)
		fmt.Printf("  Input:  %d tokens\n", in)
		fmt.Printf("  Output: %d tokens\n", out)
		fmt.Printf("  Cost:   ~$%.4f\n\n", cost)
	case "/model":
		if len(parts) > 1 {
			engine.SetModel(parts[1])
			t.PrintSuccess(fmt.Sprintf("Model set to %s", parts[1]))
		} else {
			fmt.Printf("Current model: %s\n", engine.GetModel())
		}
	case "/compact":
		t.PrintInfo("Compact is not yet implemented")
	case "/exit", "/quit":
		fmt.Println("Goodbye!")
		os.Exit(0)
	default:
		t.PrintError(fmt.Errorf("unknown command: %s (try /help)", parts[0]))
	}
	return true
}

func (t *Terminal) printHelp() {
	fmt.Printf(`
%sCommands:%s
  /help       Show this help
  /clear      Clear conversation history
  /cost       Show token usage and estimated cost
  /model      Show or change the current model
  /compact    Compress conversation context (TODO)
  /exit       Exit the program

%sKeyboard:%s
  Ctrl+C      Exit

`, bold, reset, bold, reset)
}

// formatToolInput produces a short one-line summary of the tool input.
func formatToolInput(input interface{}) string {
	var s string
	switch v := input.(type) {
	case []byte:
		s = string(v)
	case string:
		s = v
	default:
		s = fmt.Sprintf("%v", v)
	}
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > 120 {
		return s[:120] + "..."
	}
	return s
}
