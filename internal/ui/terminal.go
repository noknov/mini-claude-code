// Package ui handles all terminal I/O: the REPL loop, streaming output,
// tool display, permission prompts, and slash commands.
package ui

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/peterh/liner"

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
	cfg            *config.Config
	liner          *liner.State
	streaming      bool
	lastStreamChar byte
	skills         []skills.Skill
	lastCtrlC      int64 // unix timestamp of last Ctrl+C
}

func NewTerminal(cfg *config.Config, sk []skills.Skill) *Terminal {
	return &Terminal{
		cfg:    cfg,
		skills: sk,
	}
}

// InitLiner creates the liner instance (enters raw mode).
// Must call Close before exit.
func (t *Terminal) InitLiner() {
	t.liner = liner.NewLiner()
	t.liner.SetCtrlCAborts(true)
	t.liner.SetBeep(false)
}

// Close restores the terminal to its original state.
func (t *Terminal) Close() {
	if t.liner != nil {
		t.liner.Close()
	}
}

// ---------------------------------------------------------------------------
// Display — all output goes through writeln / writeRaw to handle raw mode.
// ---------------------------------------------------------------------------

func (t *Terminal) PrintWelcome(version, providerName, model, workDir string) {
	t.writeln("")
	t.writeln(bold + cyan + " mini-claude-code" + reset + " v" + version)
	t.writeln("  Provider: " + providerName)
	t.writeln("  Model:    " + bold + model + reset)
	t.writeln("  Dir:      " + workDir)
	t.writeln("  " + dim + "Tip: /help for commands, Ctrl+C to exit" + reset)
	t.writeln("")
}

// writeln writes a line, converting \n to \r\n when liner is active (raw mode).
func (t *Terminal) writeln(s string) {
	if t.liner != nil {
		s = strings.ReplaceAll(s, "\n", "\r\n")
		os.Stdout.WriteString(s + "\r\n")
	} else {
		fmt.Println(s)
	}
}

// writeRaw writes text directly, converting \n to \r\n when liner is active.
func (t *Terminal) writeRaw(s string) {
	if t.liner != nil {
		s = strings.ReplaceAll(s, "\n", "\r\n")
	}
	os.Stdout.WriteString(s)
}

func (t *Terminal) ReadInput() (string, error) {
	if t.liner == nil {
		return "", fmt.Errorf("terminal not initialized")
	}
	drainStdin()
	input, err := t.liner.Prompt("> ")
	if err != nil {
		return "", err
	}
	input = strings.TrimSpace(input)
	if input != "" {
		t.liner.AppendHistory(input)
	}
	return input, nil
}

// drainStdin discards any buffered bytes in the kernel's terminal input
// queue using TIOCFLUSH. This is safe to call while liner's goroutine is
// running because it operates at the kernel tty layer, not the fd flags.
func drainStdin() {
	fd := os.Stdin.Fd()
	what := 1 // FREAD: flush the read queue only
	syscall.Syscall(syscall.SYS_IOCTL, fd, _TIOCFLUSH, uintptr(unsafe.Pointer(&what)))
}

// TIOCFLUSH on darwin: _IOW('t', 16, int) = 0x80047410
const _TIOCFLUSH = 0x80047410

func (t *Terminal) StartStreaming() { t.streaming = true; t.lastStreamChar = 0 }
func (t *Terminal) StreamText(text string) {
	if len(text) > 0 {
		t.lastStreamChar = text[len(text)-1]
	}
	t.writeRaw(text)
}
func (t *Terminal) StopStreaming() {
	if t.streaming {
		if t.lastStreamChar != '\n' {
			t.writeRaw("\n")
		}
		t.writeRaw("\n")
		t.streaming = false
	}
}

func (t *Terminal) PrintToolUse(name string, input interface{}) {
	t.writeln("")
	t.writeln(yellow + "> " + name + reset + " " + dim + formatToolInput(input) + reset)
}

func (t *Terminal) PrintToolResult(name, result string) {
	lines := strings.Split(result, "\n")
	if len(lines) > 10 {
		for _, l := range lines[:5] {
			t.writeln("  " + dim + l + reset)
		}
		t.writeln(fmt.Sprintf("  %s... (%d more lines)%s", dim, len(lines)-5, reset))
	} else if result != "" {
		for _, l := range lines {
			t.writeln("  " + dim + l + reset)
		}
	}
}

func (t *Terminal) PrintToolError(name string, err error) {
	t.writeln(fmt.Sprintf("  %s✗ %s: %v%s", red, name, err, reset))
}

func (t *Terminal) PrintToolDenied(name string) {
	t.writeln(fmt.Sprintf("  %s⊘ %s: denied%s", yellow, name, reset))
}

func (t *Terminal) PrintError(err error) {
	t.writeln(fmt.Sprintf("%s✗ Error: %v%s", red, err, reset))
}

func (t *Terminal) PrintInfo(msg string) {
	t.writeln(dim + msg + reset)
}

func (t *Terminal) PrintSuccess(msg string) {
	t.writeln(green + "✓ " + msg + reset)
}

// AskPermission implements the permission.Asker interface.
func (t *Terminal) AskPermission(toolName, description string) string {
	t.writeln("")
	t.writeln(yellow + "? " + description + reset)
	if t.liner == nil {
		return "y"
	}
	input, err := t.liner.Prompt("  Allow? [Y/n/a(lways)] ")
	if err != nil {
		return "y"
	}
	return strings.TrimSpace(input)
}

// ---------------------------------------------------------------------------
// REPL
// ---------------------------------------------------------------------------

// REPLEngine is the interface the REPL loop needs from the query engine.
type REPLEngine interface {
	Run(input string, terminal *Terminal)
	SessionInfo() (inputTokens, outputTokens int)
	ClearSession()
	SetModel(model string)
	GetModel() string
	Cancel()
}

func (t *Terminal) RunREPL(engine REPLEngine) {
	for {
		input, err := t.ReadInput()
		if err != nil {
			if err == liner.ErrPromptAborted {
				now := time.Now().UnixMilli()
				if now-t.lastCtrlC < 800 {
					t.writeln("Goodbye!")
					return
				}
				t.lastCtrlC = now
				t.writeln(dim + "  Press Ctrl-C again to exit" + reset)
				continue
			}
			break
		}
		t.lastCtrlC = 0
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
		t.printTokenUsage(engine)
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
		t.writeln("Goodbye!")
		os.Exit(0)
	default:
		if t.trySkillInvocation(cmd, engine) {
			return true
		}
		t.PrintError(fmt.Errorf("unknown command: %s (try /help)", cmd))
	}
	return true
}

func (t *Terminal) printTokenUsage(engine REPLEngine) {
	in, out := engine.SessionInfo()
	t.writeln("")
	t.writeln(bold + "Token Usage:" + reset)
	t.writeln(fmt.Sprintf("  Input:  %d tokens", in))
	t.writeln(fmt.Sprintf("  Output: %d tokens", out))
	t.writeln("")
}

func (t *Terminal) handleModelCommand(parts []string, engine REPLEngine) {
	if len(parts) > 1 {
		engine.SetModel(parts[1])
		t.PrintSuccess(fmt.Sprintf("Model set to %s", parts[1]))
	} else {
		t.writeln("Current model: " + engine.GetModel())
	}
}

func (t *Terminal) printSkills() {
	if len(t.skills) == 0 {
		t.PrintInfo("No skills loaded. Add .md files to .claude/commands/ to create skills.")
		return
	}
	t.writeln("")
	t.writeln(bold + "Available skills:" + reset)
	for _, s := range t.skills {
		t.writeln("  /" + s.Name)
	}
	t.writeln("")
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
	t.writeln("")
	t.writeln(bold + "Commands:" + reset)
	t.writeln("  /help          Show this help")
	t.writeln("  /clear         Clear conversation history")
	t.writeln("  /cost          Show token usage")
	t.writeln("  /model [name]  Show or change the current model")
	t.writeln("  /compact       Compress conversation context")
	t.writeln("  /memory        Show memory file info")
	t.writeln("  /skills        List available skills")
	t.writeln("  /permissions   Show permission mode")
	t.writeln("  /resume        Resume a previous session")
	t.writeln("  /exit          Exit the program")
	t.writeln("")
	t.writeln(bold + "Skills:" + reset)
	t.writeln("  /skill-name    Run a skill from .claude/commands/")
	t.writeln("")
	t.writeln(bold + "Keyboard:" + reset)
	t.writeln("  Ctrl+C         Interrupt current operation / Exit")
	t.writeln("")
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func formatToolInput(input interface{}) string {
	var s string
	switch v := input.(type) {
	case json.RawMessage:
		s = string(v)
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
