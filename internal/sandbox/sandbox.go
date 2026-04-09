// Package sandbox provides restricted execution environments for shell commands.
//
// When enabled, Bash commands run with filesystem and network restrictions
// to prevent accidental damage. Currently a stub — OS-level sandboxing
// (e.g. macOS sandbox-exec, Linux seccomp) can be wired in later.
package sandbox

import (
	"os/exec"
	"runtime"
)

// Mode controls the sandbox level.
type Mode string

const (
	ModeOff    Mode = "off"
	ModeBasic  Mode = "basic"  // restrict filesystem writes outside workDir
	ModeStrict Mode = "strict" // restrict network + filesystem
)

// Sandbox wraps command execution with optional restrictions.
type Sandbox struct {
	mode    Mode
	workDir string
}

func New(mode Mode, workDir string) *Sandbox {
	if mode == "" {
		mode = ModeOff
	}
	return &Sandbox{mode: mode, workDir: workDir}
}

// WrapCommand applies sandbox restrictions to a command.
// Returns the (possibly modified) command ready for execution.
func (s *Sandbox) WrapCommand(cmd *exec.Cmd) *exec.Cmd {
	if s.mode == ModeOff {
		return cmd
	}

	switch runtime.GOOS {
	case "darwin":
		return s.wrapDarwin(cmd)
	case "linux":
		return s.wrapLinux(cmd)
	default:
		return cmd
	}
}

// IsEnabled returns true if sandboxing is active.
func (s *Sandbox) IsEnabled() bool {
	return s.mode != ModeOff
}

// wrapDarwin applies macOS sandbox-exec restrictions.
// TODO: implement sandbox-exec profile generation.
func (s *Sandbox) wrapDarwin(cmd *exec.Cmd) *exec.Cmd {
	return cmd
}

// wrapLinux applies Linux seccomp/namespace restrictions.
// TODO: implement seccomp profile or unshare-based sandboxing.
func (s *Sandbox) wrapLinux(cmd *exec.Cmd) *exec.Cmd {
	return cmd
}
