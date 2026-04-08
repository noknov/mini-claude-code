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

// Info holds gathered system and project context
type Info struct {
	OS        string
	Shell     string
	WorkDir   string
	GitStatus string
	ClaudeMD  string
	Date      string
}

// Gather collects system and project context for the system prompt
func Gather(workDir string) *Info {
	info := &Info{
		OS:      fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
		Shell:   os.Getenv("SHELL"),
		WorkDir: workDir,
		Date:    time.Now().Format("Monday Jan 2, 2006"),
	}

	if info.Shell == "" {
		info.Shell = "bash"
	}

	info.GitStatus = getGitStatus(workDir)
	info.ClaudeMD = config.FindClaudeMD(workDir)

	return info
}

func getGitStatus(workDir string) string {
	if !isGitRepo(workDir) {
		return ""
	}

	var parts []string

	if branch := gitCmd(workDir, "rev-parse", "--abbrev-ref", "HEAD"); branch != "" {
		parts = append(parts, fmt.Sprintf("Branch: %s", branch))
	}

	if status := gitCmd(workDir, "status", "--short"); status != "" {
		if len(status) > 2000 {
			status = status[:2000] + "\n... (truncated)"
		}
		parts = append(parts, fmt.Sprintf("Status:\n%s", status))
	} else {
		parts = append(parts, "Status: clean")
	}

	if log := gitCmd(workDir, "log", "--oneline", "-5"); log != "" {
		parts = append(parts, fmt.Sprintf("Recent commits:\n%s", log))
	}

	if len(parts) == 0 {
		return ""
	}

	return strings.Join(parts, "\n\n")
}

func isGitRepo(dir string) bool {
	cmd := exec.Command("git", "rev-parse", "--is-inside-work-tree")
	cmd.Dir = dir
	output, err := cmd.Output()
	return err == nil && strings.TrimSpace(string(output)) == "true"
}

func gitCmd(dir string, args ...string) string {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}
