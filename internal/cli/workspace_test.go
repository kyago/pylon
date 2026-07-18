package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveRoot(t *testing.T) {
	t.Run("워크스페이스가 아니면 한글 에러를 반환한다", func(t *testing.T) {
		dir := t.TempDir()
		orig := flagWorkspace
		flagWorkspace = dir
		defer func() { flagWorkspace = orig }()

		_, err := resolveRoot()
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		want := "pylon 워크스페이스가 아닙니다 — 'pylon init'을 먼저 실행하세요"
		if err.Error() != want {
			t.Errorf("error = %q, want %q", err.Error(), want)
		}
	})

	t.Run("워크스페이스 루트를 찾는다", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(dir, ".pylon"), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, ".pylon", "config.yml"), []byte("version: \"0.1\"\n"), 0644); err != nil {
			t.Fatal(err)
		}
		sub := filepath.Join(dir, "sub")
		if err := os.MkdirAll(sub, 0755); err != nil {
			t.Fatal(err)
		}
		orig := flagWorkspace
		flagWorkspace = sub
		defer func() { flagWorkspace = orig }()

		root, err := resolveRoot()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		resolved, _ := filepath.EvalSymlinks(dir)
		got, _ := filepath.EvalSymlinks(root)
		if got != resolved {
			t.Errorf("root = %q, want %q", got, resolved)
		}
	})
}
