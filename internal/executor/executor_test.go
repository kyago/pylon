package executor

import (
	"bytes"
	"strings"
	"testing"
)

func TestDirectExecutor_RunHeadless_Echo(t *testing.T) {
	exec := NewDirectExecutor()

	result, err := exec.RunHeadless(ExecConfig{
		Command: "echo",
		Args:    []string{"hello", "world"},
	})
	if err != nil {
		t.Fatalf("RunHeadless failed: %v", err)
	}

	if result.ExitCode != 0 {
		t.Errorf("exit code = %d, want 0", result.ExitCode)
	}

	expected := "hello world"
	if got := strings.TrimSpace(result.Stdout); got != expected {
		t.Errorf("stdout = %q, want %q", got, expected)
	}
}

func TestDirectExecutor_RunHeadless_WorkDir(t *testing.T) {
	exec := NewDirectExecutor()

	result, err := exec.RunHeadless(ExecConfig{
		Command: "pwd",
		WorkDir: "/tmp",
	})
	if err != nil {
		t.Fatalf("RunHeadless failed: %v", err)
	}

	// /tmp might be a symlink to /private/tmp on macOS
	got := strings.TrimSpace(result.Stdout)
	if got != "/tmp" && got != "/private/tmp" {
		t.Errorf("pwd = %q, want /tmp or /private/tmp", got)
	}
}

func TestDirectExecutor_RunHeadless_NonZeroExit(t *testing.T) {
	exec := NewDirectExecutor()

	result, err := exec.RunHeadless(ExecConfig{
		Command: "sh",
		Args:    []string{"-c", "exit 42"},
	})
	if err != nil {
		t.Fatalf("RunHeadless failed unexpectedly: %v", err)
	}

	if result.ExitCode != 42 {
		t.Errorf("exit code = %d, want 42", result.ExitCode)
	}
}

func TestDirectExecutor_RunHeadless_CommandNotFound(t *testing.T) {
	exec := NewDirectExecutor()

	_, err := exec.RunHeadless(ExecConfig{
		Command: "nonexistent-command-xyz",
	})
	if err == nil {
		t.Fatal("expected error for nonexistent command")
	}
}

func TestDirectExecutor_RunHeadless_Env(t *testing.T) {
	exec := NewDirectExecutor()

	result, err := exec.RunHeadless(ExecConfig{
		Command: "sh",
		Args:    []string{"-c", "echo $TEST_PYLON_VAR"},
		Env:     map[string]string{"TEST_PYLON_VAR": "hello_pylon"},
	})
	if err != nil {
		t.Fatalf("RunHeadless failed: %v", err)
	}

	expected := "hello_pylon"
	if got := strings.TrimSpace(result.Stdout); got != expected {
		t.Errorf("stdout = %q, want %q", got, expected)
	}
}

func TestDirectExecutor_RunHeadless_StdoutStream(t *testing.T) {
	exec := NewDirectExecutor()

	var buf bytes.Buffer
	result, err := exec.RunHeadless(ExecConfig{
		Command: "echo",
		Args:    []string{"streamed"},
		Stdout:  &buf,
	})
	if err != nil {
		t.Fatalf("RunHeadless failed: %v", err)
	}

	// Output should go to the provided writer, not result.Stdout.
	if got := strings.TrimSpace(buf.String()); got != "streamed" {
		t.Errorf("writer got %q, want %q", got, "streamed")
	}
	if result.Stdout != "" {
		t.Errorf("result.Stdout = %q, want empty when streaming", result.Stdout)
	}
}

func TestDirectExecutor_RunHeadless_EnvOverride(t *testing.T) {
	exec := NewDirectExecutor()

	result, err := exec.RunHeadless(ExecConfig{
		Command: "sh",
		Args:    []string{"-c", "echo $HOME"},
		Env:     map[string]string{"HOME": "/override/home"},
	})
	if err != nil {
		t.Fatalf("RunHeadless failed: %v", err)
	}

	if got := strings.TrimSpace(result.Stdout); got != "/override/home" {
		t.Errorf("HOME = %q, want %q (env override should take precedence)", got, "/override/home")
	}
}

func TestDirectExecutor_ExecInteractive_CommandNotFound(t *testing.T) {
	exec := NewDirectExecutor()

	// ExecInteractive with a nonexistent command should return error
	// (without actually replacing the process)
	err := exec.ExecInteractive(ExecConfig{
		Command: "nonexistent-command-xyz",
	})
	if err == nil {
		t.Fatal("expected error for nonexistent command")
	}
}
