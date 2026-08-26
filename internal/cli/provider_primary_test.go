package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kyago/pylon/internal/config"
	"github.com/kyago/pylon/internal/layout"
)

// writeStubExecutable creates an executable stub and returns its path.
func writeStubExecutable(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

// newPrimaryProviderWorkspace builds a workspace whose provider commands point
// at controllable stub paths, so "installed" is fully simulated.
func newPrimaryProviderWorkspace(t *testing.T, claudeCmd, codexCmd string) (string, *config.Config) {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".pylon"), 0o755); err != nil {
		t.Fatal(err)
	}
	content := "version: \"0.2\"\nproviders:\n" +
		"  claude-code:\n    command: " + claudeCmd + "\n" +
		"  codex:\n    command: " + codexCmd + "\n"
	if err := os.WriteFile(layout.ConfigPath(root), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.LoadConfig(layout.ConfigPath(root))
	if err != nil {
		t.Fatal(err)
	}
	return root, cfg
}

func TestEnsurePrimaryProvider_SingleInstalledBecomesPrimary(t *testing.T) {
	binDir := t.TempDir()
	codexCmd := writeStubExecutable(t, binDir, "codex-stub")
	root, cfg := newPrimaryProviderWorkspace(t, filepath.Join(binDir, "missing-claude"), codexCmd)

	updated, changed := ensurePrimaryProvider(root, cfg, false)
	if !changed {
		t.Fatal("single installed provider must be pinned as primary")
	}
	if providerName, _ := updated.Runtime.EffectiveProvider(); providerName != "codex" {
		t.Fatalf("effective provider = %q", providerName)
	}
	reloaded, err := config.LoadConfig(layout.ConfigPath(root))
	if err != nil {
		t.Fatal(err)
	}
	if providerName, _ := reloaded.Runtime.EffectiveProvider(); providerName != "codex" {
		t.Fatalf("pinned provider not persisted: %q", providerName)
	}
}

func TestEnsurePrimaryProvider_PinnedInstalledIsUntouched(t *testing.T) {
	binDir := t.TempDir()
	claudeCmd := writeStubExecutable(t, binDir, "claude-stub")
	codexCmd := writeStubExecutable(t, binDir, "codex-stub")
	root, _ := newPrimaryProviderWorkspace(t, claudeCmd, codexCmd)
	if err := config.SetRuntimeProvider(layout.ConfigPath(root), "claude-code"); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.LoadConfig(layout.ConfigPath(root))
	if err != nil {
		t.Fatal(err)
	}

	before, err := os.ReadFile(layout.ConfigPath(root))
	if err != nil {
		t.Fatal(err)
	}
	if _, changed := ensurePrimaryProvider(root, cfg, false); changed {
		t.Fatal("pinned installed provider must not be rewritten")
	}
	after, err := os.ReadFile(layout.ConfigPath(root))
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("config must be untouched")
	}
}

func TestEnsurePrimaryProvider_MultipleInstalledNonInteractiveKeepsAuto(t *testing.T) {
	binDir := t.TempDir()
	claudeCmd := writeStubExecutable(t, binDir, "claude-stub")
	codexCmd := writeStubExecutable(t, binDir, "codex-stub")
	root, cfg := newPrimaryProviderWorkspace(t, claudeCmd, codexCmd)

	if _, changed := ensurePrimaryProvider(root, cfg, false); changed {
		t.Fatal("non-interactive mode must not pick between multiple providers")
	}
}

func TestEnsurePrimaryProvider_NoneInstalledKeepsConfig(t *testing.T) {
	binDir := t.TempDir()
	root, cfg := newPrimaryProviderWorkspace(t,
		filepath.Join(binDir, "missing-claude"), filepath.Join(binDir, "missing-codex"))

	if _, changed := ensurePrimaryProvider(root, cfg, false); changed {
		t.Fatal("nothing installed — config must stay untouched")
	}
}
