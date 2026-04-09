// Package context gathers system, project, and user context at startup.
//
// This includes OS info, git status, memory files, rules, skills, agents,
// and MCP server configuration — everything needed to build the system prompt.
package context

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/noknov/mini-claude-code/internal/agent"
	"github.com/noknov/mini-claude-code/internal/mcp"
	"github.com/noknov/mini-claude-code/internal/memory"
	"github.com/noknov/mini-claude-code/internal/rules"
	"github.com/noknov/mini-claude-code/internal/settings"
	"github.com/noknov/mini-claude-code/internal/skills"
)

const maxStatusLen = 2000

// Info holds all gathered context.
type Info struct {
	// System
	OS      string
	Shell   string
	WorkDir string
	Date    string

	// Git
	GitStatus string

	// Instruction layers
	MemoryFiles []memory.File
	Rules       []rules.Rule
	Skills      []skills.Skill
	Agents      []agent.Definition

	// MCP
	MCPClient *mcp.Client

	// Settings
	Settings *settings.Settings
}

// Gather collects all context for the given working directory.
func Gather(workDir string) *Info {
	s := settings.Load(workDir)

	return &Info{
		OS:          fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
		Shell:       detectShell(),
		WorkDir:     workDir,
		Date:        time.Now().Format("Monday Jan 2, 2006"),
		GitStatus:   gatherGitStatus(workDir),
		MemoryFiles: memory.LoadAll(workDir),
		Rules:       rules.LoadAll(workDir),
		Skills:      skills.LoadAll(workDir),
		Agents:      agent.LoadAll(workDir),
		MCPClient:   mcp.NewClient(workDir),
		Settings:    s,
	}
}

// ---------------------------------------------------------------------------
// Git
// ---------------------------------------------------------------------------

func gatherGitStatus(workDir string) string {
	if !isGitRepo(workDir) {
		return ""
	}
	var parts []string
	if branch := git(workDir, "rev-parse", "--abbrev-ref", "HEAD"); branch != "" {
		parts = append(parts, "Branch: "+branch)
	}
	if status := git(workDir, "status", "--short"); status != "" {
		if len(status) > maxStatusLen {
			status = status[:maxStatusLen] + "\n... (truncated)"
		}
		parts = append(parts, "Status:\n"+status)
	} else {
		parts = append(parts, "Status: clean")
	}
	if log := git(workDir, "log", "--oneline", "-5"); log != "" {
		parts = append(parts, "Recent commits:\n"+log)
	}
	return strings.Join(parts, "\n\n")
}

func isGitRepo(dir string) bool {
	cmd := exec.Command("git", "rev-parse", "--is-inside-work-tree")
	cmd.Dir = dir
	out, err := cmd.Output()
	return err == nil && strings.TrimSpace(string(out)) == "true"
}

func git(dir string, args ...string) string {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// ---------------------------------------------------------------------------
// Shell detection
// ---------------------------------------------------------------------------

func detectShell() string {
	if shell := os.Getenv("SHELL"); shell != "" {
		return shell
	}
	return "bash"
}
