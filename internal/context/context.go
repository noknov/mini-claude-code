package context

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/noknov/mini-claude-code/internal/config"
)

// Info holds system and project context gathered at startup.
type Info struct {
	OS        string
	Shell     string
	WorkDir   string
	GitStatus string
	ClaudeMD  string
	Date      string
}

// Gather collects system and project context for the system prompt.
func Gather(workDir string) *Info {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "bash"
	}

	return &Info{
		OS:        fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
		Shell:     shell,
		WorkDir:   workDir,
		Date:      time.Now().Format("Monday Jan 2, 2006"),
		GitStatus: gatherGitStatus(workDir),
		ClaudeMD:  config.FindClaudeMD(workDir),
	}
}

const maxStatusLen = 2000

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
