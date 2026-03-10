package executor

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

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

	// Build environment: inherit current + overlay extras
	env := os.Environ()
	for k, v := range cfg.Env {
		env = append(env, k+"="+v)
	}

	return syscall.Exec(binPath, argv, env)
}

// RunHeadless runs a child process and captures its output.
func (d *DirectExecutor) RunHeadless(cfg ExecConfig) (*ExecResult, error) {
	binPath, err := exec.LookPath(cfg.Command)
	if err != nil {
		return nil, fmt.Errorf("command not found: %s: %w", cfg.Command, err)
	}

	cmd := exec.Command(binPath, cfg.Args...)

	if cfg.WorkDir != "" {
		cmd.Dir = cfg.WorkDir
	}

	// Set environment
	if len(cfg.Env) > 0 {
		cmd.Env = os.Environ()
		for k, v := range cfg.Env {
			cmd.Env = append(cmd.Env, k+"="+v)
		}
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
