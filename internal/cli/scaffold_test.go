package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func newScaffoldWorkspace(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".pylon"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".pylon", "config.yml"), []byte("version: \"0.1\"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	orig := flagWorkspace
	flagWorkspace = dir
	t.Cleanup(func() { flagWorkspace = orig })
	return dir
}

func TestScaffoldMarkdownResource(t *testing.T) {
	t.Run("파일을 생성하고 내용이 byte-동일하다", func(t *testing.T) {
		dir := newScaffoldWorkspace(t)
		content := "---\nname: x\n---\n\n# X\n"

		path, err := scaffoldMarkdownResource("에이전트", "agents", "x", content)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := filepath.Join(dir, ".pylon", "agents", "x.md")
		if path != want {
			t.Errorf("path = %q, want %q", path, want)
		}
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != content {
			t.Errorf("content mismatch:\ngot:  %q\nwant: %q", got, content)
		}
	})

	t.Run("하위 디렉토리가 없으면 생성한다", func(t *testing.T) {
		dir := newScaffoldWorkspace(t)
		if _, err := os.Stat(filepath.Join(dir, ".pylon", "skills")); !os.IsNotExist(err) {
			t.Fatal("precondition: skills dir must not exist")
		}
		if _, err := scaffoldMarkdownResource("스킬", "skills", "s", "body"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("이미 존재하면 kind 가 포함된 에러를 반환한다", func(t *testing.T) {
		dir := newScaffoldWorkspace(t)
		existing := filepath.Join(dir, ".pylon", "skills")
		if err := os.MkdirAll(existing, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(existing, "dup.md"), []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}

		_, err := scaffoldMarkdownResource("스킬", "skills", "dup", "new")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		wantPrefix := "스킬 'dup'가 이미 존재합니다"
		if len(err.Error()) < len(wantPrefix) || err.Error()[:len(wantPrefix)] != wantPrefix {
			t.Errorf("error = %q, want prefix %q", err.Error(), wantPrefix)
		}
	})
}
