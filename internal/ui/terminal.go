// Package ui handles all terminal I/O: the REPL loop, streaming output,
// tool display, permission prompts, and slash commands.
package ui

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/noknov/mini-claude-code/internal/config"
	"github.com/noknov/mini-claude-code/internal/skills"
)

// ANSI escape sequences.
const (
	reset  = "\033[0m"
	red    = "\033[31m"
	green  = "\033[32m"
	yellow = "\033[33m"
	cyan   = "\033[36m"
	dim    = "\033[2m"
	bold   = "\033[1m"
)

// ---------------------------------------------------------------------------
// Terminal
// ---------------------------------------------------------------------------

// Terminal handles all user-facing I/O.
type Terminal struct {
	cfg       *config.Config
	reader    *bufio.Reader
	streaming bool
	skills    []skills.Skill
}

func NewTerminal(cfg *config.Config, sk []skills.Skill) *Terminal {
	return &Terminal{
		cfg:    cfg,
		reader: bufio.NewReader(os.Stdin),
		skills: sk,
	}
}

// ---------------------------------------------------------------------------
// Display
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

func (t *Terminal) StartStreaming()        { t.streaming = true; fmt.Print("\n") }
func (t *Terminal) StreamText(text string) { fmt.Print(text) }
func (t *Terminal) StopStreaming() {
	if t.streaming {
		fmt.Println()
		t.streaming = false
	}
}

func (t *Terminal) PrintToolUse(name string, input interface{}) {
	fmt.Printf("\n%s> %s%s %s%s%s\n", yellow, name, reset, dim, formatToolInput(input), reset)
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
	Cancel()
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

// ---------------------------------------------------------------------------
// Slash commands
// ---------------------------------------------------------------------------

func (t *Terminal) handleCommand(input string, engine REPLEngine) bool {
	parts := strings.Fields(input)
	cmd := parts[0]

	switch cmd {
	case "/help":
		t.printHelp()
	case "/clear":
		engine.ClearSession()
		t.PrintSuccess("Conversation cleared")
	case "/cost":
		t.printCost(engine)
	case "/model":
		t.handleModelCommand(parts, engine)
	case "/compact":
		t.PrintInfo("Triggering manual compaction...")
		// TODO: wire to compactor directly
	case "/memory":
		t.PrintInfo("Memory files are loaded at startup from CLAUDE.md and .claude/ directories")
	case "/skills":
		t.printSkills()
	case "/permissions":
		t.PrintInfo("Permission mode: ask | Use /permissions auto or /permissions deny to change")
	case "/resume":
		t.PrintInfo("Session resume: not yet implemented")
	case "/exit", "/quit":
		fmt.Println("Goodbye!")
		os.Exit(0)
	default:
		// Check if it's a skill invocation
		if t.trySkillInvocation(cmd, engine) {
			return true
		}
		t.PrintError(fmt.Errorf("unknown command: %s (try /help)", cmd))
	}
	return true
}

func (t *Terminal) printCost(engine REPLEngine) {
	in, out, cost := engine.SessionInfo()
	fmt.Printf("\n%sToken Usage:%s\n", bold, reset)
	fmt.Printf("  Input:  %d tokens\n", in)
	fmt.Printf("  Output: %d tokens\n", out)
	fmt.Printf("  Cost:   ~$%.4f\n\n", cost)
}

func (t *Terminal) handleModelCommand(parts []string, engine REPLEngine) {
	if len(parts) > 1 {
		engine.SetModel(parts[1])
		t.PrintSuccess(fmt.Sprintf("Model set to %s", parts[1]))
	} else {
		fmt.Printf("Current model: %s\n", engine.GetModel())
	}
}

func (t *Terminal) printSkills() {
	if len(t.skills) == 0 {
		t.PrintInfo("No skills loaded. Add .md files to .claude/commands/ to create skills.")
		return
	}
	fmt.Printf("\n%sAvailable skills:%s\n", bold, reset)
	for _, s := range t.skills {
		fmt.Printf("  /%s\n", s.Name)
	}
	fmt.Println()
}

func (t *Terminal) trySkillInvocation(cmd string, engine REPLEngine) bool {
	name := strings.TrimPrefix(cmd, "/")
	skill := skills.Find(t.skills, name)
	if skill == nil {
		return false
	}
	t.PrintInfo(fmt.Sprintf("Running skill: %s", name))
	engine.Run(skill.Content, t)
	return true
}

func (t *Terminal) printHelp() {
	fmt.Printf(`
%sCommands:%s
  /help          Show this help
  /clear         Clear conversation history
  /cost          Show token usage and estimated cost
  /model [name]  Show or change the current model
  /compact       Compress conversation context
  /memory        Show memory file info
  /skills        List available skills
  /permissions   Show permission mode
  /resume        Resume a previous session
  /exit          Exit the program

%sSkills:%s
  /skill-name    Run a skill from .claude/commands/

%sKeyboard:%s
  Ctrl+C         Interrupt current operation / Exit

`, bold, reset, bold, reset, bold, reset)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

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
