// Package executor provides process execution abstractions for pylon agents.
// It provides direct process execution for agent management.
package executor

import (
	"context"
	"io"
)

// ExecConfig holds parameters for launching a process.
type ExecConfig struct {
	Name    string            // descriptive name for logging
	Command string            // binary path or name (resolved via LookPath)
	Args    []string          // command-line arguments (not including argv[0])
	WorkDir string            // working directory
	Env     map[string]string // additional environment variables
	Ctx     context.Context   // if set, process is killed when context is cancelled
	Stdin   io.Reader         // if set, stdin is connected to this reader
	Stdout  io.Writer         // if set, stdout is streamed here instead of captured
	Stderr  io.Writer         // if set, stderr is streamed here instead of captured
}

// ExecResult holds the output from a headless process execution.
type ExecResult struct {
	ExitCode int
	Stdout   string
	Stderr   string
}

// ProcessExecutor abstracts how pylon launches processes.
type ProcessExecutor interface {
	// ExecInteractive replaces the current process (syscall.Exec).
	// Used for the root agent where pylon hands off to claude CLI.
	ExecInteractive(cfg ExecConfig) error

	// RunInteractive runs a child process with inherited stdin/stdout/stderr.
	// Unlike ExecInteractive, the parent process stays alive (for background tasks like dashboard).
	RunInteractive(cfg ExecConfig) error

	// RunHeadless runs a child process, captures output, and returns the result.
	// Used for CLI-triggered tasks like `pylon index`.
	RunHeadless(cfg ExecConfig) (*ExecResult, error)
}
