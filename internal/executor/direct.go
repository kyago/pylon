package executor

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

// buildEnv merges os.Environ() with cfg.Env, ensuring cfg.Env values
// take precedence over inherited environment variables.
func buildEnv(extra map[string]string) []string {
	if len(extra) == 0 {
		return nil
	}
	override := make(map[string]bool, len(extra))
	for k := range extra {
		override[k] = true
	}
	// Keep inherited vars except those being overridden.
	var env []string
	for _, entry := range os.Environ() {
		if k, _, ok := strings.Cut(entry, "="); ok && override[k] {
			continue
		}
		env = append(env, entry)
	}
	for k, v := range extra {
		env = append(env, k+"="+v)
	}
	return env
}

// DirectExecutor implements ProcessExecutor using OS-level process operations.
type DirectExecutor struct{}

// NewDirectExecutor creates a new DirectExecutor.
func NewDirectExecutor() *DirectExecutor {
	return &DirectExecutor{}
}

// ExecInteractive replaces the current process with the given command.
// This uses syscall.Exec so control never returns to the caller on success.
func (d *DirectExecutor) ExecInteractive(cfg ExecConfig) error {
	binPath, err := exec.LookPath(cfg.Command)
	if err != nil {
		return fmt.Errorf("command not found: %s: %w", cfg.Command, err)
	}

	// Change working directory before exec, restoring on failure.
	if cfg.WorkDir != "" {
		origDir, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("failed to get current directory: %w", err)
		}
		if err := os.Chdir(cfg.WorkDir); err != nil {
			return fmt.Errorf("failed to change directory to %s: %w", cfg.WorkDir, err)
		}
		defer func() {
			// Restore cwd if syscall.Exec fails and control returns.
			_ = os.Chdir(origDir)
		}()
	}

	// Build argv: argv[0] is the command name
	argv := append([]string{cfg.Command}, cfg.Args...)

	// Build environment: cfg.Env values override inherited vars.
	env := buildEnv(cfg.Env)
	if env == nil {
		env = os.Environ()
	}

	return syscall.Exec(binPath, argv, env)
}

// RunInteractive runs a child process with inherited stdin/stdout/stderr.
// The parent process remains alive, unlike ExecInteractive which replaces it.
func (d *DirectExecutor) RunInteractive(cfg ExecConfig) error {
	binPath, err := exec.LookPath(cfg.Command)
	if err != nil {
		return fmt.Errorf("command not found: %s: %w", cfg.Command, err)
	}

	cmd := exec.Command(binPath, cfg.Args...)
	if cfg.WorkDir != "" {
		cmd.Dir = cfg.WorkDir
	}
	if env := buildEnv(cfg.Env); env != nil {
		cmd.Env = env
	}

	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

// RunHeadless runs a child process and captures its output.
func (d *DirectExecutor) RunHeadless(cfg ExecConfig) (*ExecResult, error) {
	binPath, err := exec.LookPath(cfg.Command)
	if err != nil {
		return nil, fmt.Errorf("command not found: %s: %w", cfg.Command, err)
	}

	var cmd *exec.Cmd
	if cfg.Ctx != nil {
		cmd = exec.CommandContext(cfg.Ctx, binPath, cfg.Args...)
		// Graceful shutdown: send SIGTERM first, wait 10s, then SIGKILL.
		// Cancel is only invoked after Start() succeeds, so cmd.Process is guaranteed non-nil.
		cmd.Cancel = func() error {
			return cmd.Process.Signal(syscall.SIGTERM)
		}
		cmd.WaitDelay = 10 * time.Second
	} else {
		cmd = exec.Command(binPath, cfg.Args...)
	}

	if cfg.WorkDir != "" {
		cmd.Dir = cfg.WorkDir
	}

	// Set environment: cfg.Env values override inherited vars.
	if env := buildEnv(cfg.Env); env != nil {
		cmd.Env = env
	}

	// Connect stdin if provided.
	if cfg.Stdin != nil {
		cmd.Stdin = cfg.Stdin
	}

	// If callers provide writers, stream directly; otherwise capture into buffers.
	var stdout, stderr bytes.Buffer
	if cfg.Stdout != nil {
		cmd.Stdout = cfg.Stdout
	} else {
		cmd.Stdout = &stdout
	}
	if cfg.Stderr != nil {
		cmd.Stderr = cfg.Stderr
	} else {
		cmd.Stderr = &stderr
	}

	err = cmd.Run()

	result := &ExecResult{
		Stdout: stdout.String(),
		Stderr: stderr.String(),
	}

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		} else {
			return result, fmt.Errorf("failed to run %s: %w", cfg.Command, err)
		}
	}

	return result, nil
}
