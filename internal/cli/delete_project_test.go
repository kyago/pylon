package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kyago/pylon/internal/store"
)

// setupDeleteWorkspace creates a minimal workspace with a registered project
// (plus one project_memory row) and returns the workspace root.
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
		if err := os.MkdirAll(projectDir, 0755); err != nil {
			t.Fatal(err)
		}
	}

	s, err := store.NewStore(filepath.Join(root, ".pylon", "pylon.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.Migrate(); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertProject(&store.ProjectRecord{
		ProjectID: projectName,
		Path:      projectDir,
		Stack:     "go",
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.InsertMemory(&store.MemoryEntry{
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

func openTestStore(t *testing.T, root string) *store.Store {
	t.Helper()
	s, err := store.NewStore(filepath.Join(root, ".pylon", "pylon.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Migrate(); err != nil {
		s.Close()
		t.Fatal(err)
	}
	return s
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

func TestRunDeleteProject_Default_KeepsDirectory(t *testing.T) {
	root := setupDeleteWorkspace(t, "myapp", true)
	projectDir := filepath.Join(root, "myapp")

	oldWorkspace := flagWorkspace
	flagWorkspace = root
	defer func() { flagWorkspace = oldWorkspace }()

	if err := runDeleteProject("myapp", false, true); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Directory preserved
	if !dirExists(projectDir) {
		t.Error("project directory should be preserved without --purge")
	}

	// DB registration and memory removed
	s := openTestStore(t, root)
	defer s.Close()
	if _, err := s.GetProject("myapp"); err == nil {
		t.Error("expected project to be unregistered")
	}
	mem, err := s.ListProjectMemory("myapp")
	if err != nil {
		t.Fatal(err)
	}
	if len(mem) != 0 {
		t.Errorf("expected project_memory to be cleared, got %d rows", len(mem))
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
	s := openTestStore(t, root)
	defer s.Close()
	if _, err := s.GetProject("myapp"); err == nil {
		t.Error("expected project to be unregistered")
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

	// Project should still be registered.
	s := openTestStore(t, root)
	defer s.Close()
	if _, err := s.GetProject("myapp"); err != nil {
		t.Errorf("project should remain registered after decline: %v", err)
	}
}

func TestResolvePurgeDir(t *testing.T) {
	root := t.TempDir()
	inside := filepath.Join(root, "proj")
	if err := os.MkdirAll(inside, 0755); err != nil {
		t.Fatal(err)
	}

	// Inside the workspace and existing → safe.
	if dir, ok := resolvePurgeDir(root, inside); !ok || dir != filepath.Clean(inside) {
		t.Errorf("expected inside path to be purgeable, got %q ok=%v", dir, ok)
	}

	// Root itself → not safe.
	if _, ok := resolvePurgeDir(root, root); ok {
		t.Error("workspace root should not be purgeable")
	}

	// Outside the workspace → not safe.
	outside := t.TempDir()
	if _, ok := resolvePurgeDir(root, outside); ok {
		t.Error("path outside workspace should not be purgeable")
	}

	// Nonexistent path inside workspace → not safe.
	if _, ok := resolvePurgeDir(root, filepath.Join(root, "missing")); ok {
		t.Error("nonexistent directory should not be purgeable")
	}
}
