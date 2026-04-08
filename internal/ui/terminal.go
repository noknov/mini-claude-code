package ui

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/noknov/mini-claude-code/internal/config"
)

const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorBlue   = "\033[34m"
	colorCyan   = "\033[36m"
	colorDim    = "\033[2m"
	colorBold   = "\033[1m"
)

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

func (t *Terminal) PrintWelcome(version, model, workDir string) {
	fmt.Printf("\n%s╭─ mini-claude-code v%s ─╮%s\n", colorCyan, version, colorReset)
	fmt.Printf("%s│%s Model: %s%s%s\n", colorCyan, colorReset, colorBold, model, colorReset)
	fmt.Printf("%s│%s Dir:   %s\n", colorCyan, colorReset, workDir)
	fmt.Printf("%s╰────────────────────────╯%s\n", colorCyan, colorReset)
	fmt.Printf("%sTip: /help for commands, Ctrl+C to exit%s\n\n", colorDim, colorReset)
}

func (t *Terminal) ReadInput() (string, error) {
	fmt.Printf("%s> %s", colorGreen, colorReset)
	input, err := t.reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(input), nil
}

func (t *Terminal) StartStreaming() {
	t.streaming = true
	fmt.Printf("\n%s", colorReset)
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
	fmt.Printf("\n%s⚡ %s%s ", colorYellow, name, colorReset)
	switch v := input.(type) {
	case []byte:
		compact := strings.ReplaceAll(string(v), "\n", " ")
		if len(compact) > 100 {
			compact = compact[:100] + "..."
		}
		fmt.Printf("%s%s%s\n", colorDim, compact, colorReset)
	default:
		fmt.Printf("%s%v%s\n", colorDim, v, colorReset)
	}
}

func (t *Terminal) PrintToolResult(name, result string) {
	lines := strings.Split(result, "\n")
	if len(lines) > 10 {
		for _, line := range lines[:5] {
			fmt.Printf("  %s%s%s\n", colorDim, line, colorReset)
		}
		fmt.Printf("  %s... (%d more lines)%s\n", colorDim, len(lines)-5, colorReset)
	} else if len(result) > 0 {
		for _, line := range lines {
			fmt.Printf("  %s%s%s\n", colorDim, line, colorReset)
		}
	}
}

func (t *Terminal) PrintToolError(name string, err error) {
	fmt.Printf("  %s✗ %s: %v%s\n", colorRed, name, err, colorReset)
}

func (t *Terminal) PrintToolDenied(name string) {
	fmt.Printf("  %s⊘ %s: permission denied%s\n", colorYellow, name, colorReset)
}

func (t *Terminal) PrintError(err error) {
	fmt.Printf("%s✗ Error: %v%s\n", colorRed, err, colorReset)
}

func (t *Terminal) PrintInfo(msg string) {
	fmt.Printf("%s%s%s\n", colorDim, msg, colorReset)
}

func (t *Terminal) PrintSuccess(msg string) {
	fmt.Printf("%s✓ %s%s\n", colorGreen, msg, colorReset)
}

// AskPermission implements the permission.Asker interface
func (t *Terminal) AskPermission(toolName, description string) string {
	fmt.Printf("\n%s🔒 Permission needed: %s%s\n", colorYellow, description, colorReset)
	fmt.Printf("   %sAllow? [Y/n/a(lways)] %s", colorDim, colorReset)
	input, _ := t.reader.ReadString('\n')
	return strings.TrimSpace(input)
}

// REPLEngine is the interface the REPL needs from the query engine
type REPLEngine interface {
	Run(input string, terminal *Terminal)
	SessionInfo() (inputTokens, outputTokens int, cost float64)
	ClearSession()
	SetModel(model string)
	GetModel() string
}

// RunREPL starts the interactive read-eval-print loop
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
	cmd := parts[0]

	switch cmd {
	case "/help":
		t.printHelp()
		return true
	case "/clear":
		engine.ClearSession()
		t.PrintSuccess("Conversation cleared")
		return true
	case "/cost":
		inputTokens, outputTokens, cost := engine.SessionInfo()
		fmt.Printf("\n%sToken Usage:%s\n", colorBold, colorReset)
		fmt.Printf("  Input:  %d tokens\n", inputTokens)
		fmt.Printf("  Output: %d tokens\n", outputTokens)
		fmt.Printf("  Cost:   ~$%.4f\n\n", cost)
		return true
	case "/model":
		if len(parts) > 1 {
			engine.SetModel(parts[1])
			t.PrintSuccess(fmt.Sprintf("Model set to %s", parts[1]))
		} else {
			fmt.Printf("Current model: %s\n", engine.GetModel())
		}
		return true
	case "/exit", "/quit":
		fmt.Println("Goodbye!")
		os.Exit(0)
		return true
	case "/compact":
		t.PrintInfo("Compact not yet implemented in mini version")
		return true
	default:
		t.PrintError(fmt.Errorf("unknown command: %s (try /help)", cmd))
		return true
	}
}

func (t *Terminal) printHelp() {
	fmt.Printf(`
%sAvailable Commands:%s
  /help       Show this help message
  /clear      Clear conversation history
  /cost       Show token usage and estimated cost
  /model      Show or change the current model
  /compact    Compress conversation context (TODO)
  /exit       Exit the program

%sKeyboard:%s
  Ctrl+C      Interrupt / Exit

`, colorBold, colorReset, colorBold, colorReset)
}
