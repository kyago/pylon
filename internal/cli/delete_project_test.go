package cli

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kyago/pylon/internal/config"
	"github.com/kyago/pylon/internal/memory"
)

// setupDeleteWorkspace creates a minimal workspace with a discoverable project
// (a subdirectory carrying its own .pylon/ marker) plus one memory entry, and
// returns the workspace root.
func setupDeleteWorkspace(t *testing.T, projectName string, registerDir bool) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".pylon"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".pylon", "config.yml"), []byte("version: \"0.1\"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	projectDir := filepath.Join(root, projectName)
	if registerDir {
		// Create the clone directory with a source file and a .pylon/ marker,
		// mirroring what add-project leaves on disk — this is how the project is
		// discovered now that the SQLite registry is gone.
		if err := os.MkdirAll(filepath.Join(projectDir, ".pylon"), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(projectDir, "main.go"), []byte("package main\n"), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(projectDir, ".pylon", "context.md"), []byte("# ctx\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	if err := memory.NewStore(root).Insert(&memory.Entry{
		ProjectID:  projectName,
		Category:   "learning",
		Key:        "k1",
		Content:    "some content",
		Confidence: 0.8,
	}); err != nil {
		t.Fatal(err)
	}

	return root
}

// isDiscovered reports whether DiscoverProjects finds a project by name.
func isDiscovered(t *testing.T, root, name string) bool {
	t.Helper()
	projects, err := config.DiscoverProjects(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range projects {
		if p.Name == name {
			return true
		}
	}
	return false
}

func TestNewDeleteProjectCmd_Flags(t *testing.T) {
	cmd := newDeleteProjectCmd()
	for _, name := range []string{"purge", "force"} {
		if cmd.Flags().Lookup(name) == nil {
			t.Errorf("--%s flag not found", name)
		}
	}
}

func TestRunDeleteProject_NotRegistered(t *testing.T) {
	root := setupDeleteWorkspace(t, "myapp", true)

	oldWorkspace := flagWorkspace
	flagWorkspace = root
	defer func() { flagWorkspace = oldWorkspace }()

	err := runDeleteProject("ghost", false, true)
	if err == nil {
		t.Fatal("expected error for unregistered project, got nil")
	}
	if !strings.Contains(err.Error(), "not registered") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRunDeleteProject_Default_KeepsSourceRemovesMarker(t *testing.T) {
	root := setupDeleteWorkspace(t, "myapp", true)
	projectDir := filepath.Join(root, "myapp")

	oldWorkspace := flagWorkspace
	flagWorkspace = root
	defer func() { flagWorkspace = oldWorkspace }()

	if err := runDeleteProject("myapp", false, true); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Source directory and files preserved
	if !dirExists(projectDir) {
		t.Error("project directory should be preserved without --purge")
	}
	if !fileExists(filepath.Join(projectDir, "main.go")) {
		t.Error("source files should be preserved without --purge")
	}
	// .pylon/ marker removed so the project is no longer discovered
	if dirExists(filepath.Join(projectDir, ".pylon")) {
		t.Error(".pylon/ marker should be removed on default delete")
	}
	if isDiscovered(t, root, "myapp") {
		t.Error("project should no longer be discovered after marker removal")
	}

	// Project memory removed
	mem, err := memory.NewStore(root).List("myapp")
	if err != nil {
		t.Fatal(err)
	}
	if len(mem) != 0 {
		t.Errorf("expected project memory to be cleared, got %d entries", len(mem))
	}
}

func TestRunDeleteProject_Purge_RemovesDirectory(t *testing.T) {
	root := setupDeleteWorkspace(t, "myapp", true)
	projectDir := filepath.Join(root, "myapp")

	oldWorkspace := flagWorkspace
	flagWorkspace = root
	defer func() { flagWorkspace = oldWorkspace }()

	if err := runDeleteProject("myapp", true, true); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if dirExists(projectDir) {
		t.Error("project directory should be removed with --purge")
	}
	mem, err := memory.NewStore(root).List("myapp")
	if err != nil {
		t.Fatal(err)
	}
	if len(mem) != 0 {
		t.Errorf("expected project memory to be cleared, got %d entries", len(mem))
	}
}

func TestRunDeleteProject_ConfirmDecline(t *testing.T) {
	root := setupDeleteWorkspace(t, "myapp", true)

	oldWorkspace := flagWorkspace
	flagWorkspace = root
	defer func() { flagWorkspace = oldWorkspace }()

	// Decline the confirmation prompt.
	withStdin(t, "n\n", func() {
		if err := runDeleteProject("myapp", false, false); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	// Project should still be discoverable and its memory intact.
	if !isDiscovered(t, root, "myapp") {
		t.Error("project should remain discoverable after decline")
	}
	mem, err := memory.NewStore(root).List("myapp")
	if err != nil {
		t.Fatal(err)
	}
	if len(mem) != 1 {
		t.Errorf("memory should be intact after decline, got %d entries", len(mem))
	}
}

// TestRunDeleteProject_JSONOutput exercises the --json output path.
func TestRunDeleteProject_JSONOutput(t *testing.T) {
	root := setupDeleteWorkspace(t, "myapp", true)

	oldWorkspace := flagWorkspace
	flagWorkspace = root
	oldJSON := flagJSON
	flagJSON = true
	defer func() {
		flagWorkspace = oldWorkspace
		flagJSON = oldJSON
	}()

	out := captureStdout(t, func() {
		if err := runDeleteProject("myapp", false, false); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	for _, want := range []string{`"status":"ok"`, `"project":"myapp"`, `"purged":false`, `"removed":true`} {
		if !strings.Contains(out, want) {
			t.Errorf("JSON output missing %q; got: %s", want, out)
		}
	}
}

// captureStdout redirects os.Stdout for the duration of fn and returns what was
// written.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	oldStdout := os.Stdout
	os.Stdout = w
	fn()
	os.Stdout = oldStdout
	if err := w.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	return string(data)
}

func TestResolveProjectDir(t *testing.T) {
	root := t.TempDir()
	inside := filepath.Join(root, "proj")
	if err := os.MkdirAll(inside, 0755); err != nil {
		t.Fatal(err)
	}

	// Inside the workspace and existing → safe.
	if dir, ok := resolveProjectDir(root, inside); !ok || dir != filepath.Clean(inside) {
		t.Errorf("expected inside path to be purgeable, got %q ok=%v", dir, ok)
	}

	// Root itself → not safe.
	if _, ok := resolveProjectDir(root, root); ok {
		t.Error("workspace root should not be purgeable")
	}

	// Outside the workspace → not safe.
	outside := t.TempDir()
	if _, ok := resolveProjectDir(root, outside); ok {
		t.Error("path outside workspace should not be purgeable")
	}

	// Nonexistent path inside workspace → not safe.
	if _, ok := resolveProjectDir(root, filepath.Join(root, "missing")); ok {
		t.Error("nonexistent directory should not be purgeable")
	}
}
