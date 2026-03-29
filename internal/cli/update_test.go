package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestValidateVersion(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		// CalVer valid
		{"2026.3.1", true},
		{"2026.12.15", true},
		{"2025.1.1", true},
		{"2030.6.100", true},
		{"latest", true},
		{"Latest", true},
		{"LATEST", true},

		// Invalid
		{"abc", false},
		{"", false},
		{"v1.2.3", false},
		{"2026", false},
		{"2026.3", false},
		{"hello-world", false},
		{"2026.3.1.4", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := validateVersion(tt.input)
			if got != tt.want {
				t.Errorf("validateVersion(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestResolveUpdateTarget(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    string
		wantErr bool
	}{
		{"no args", nil, "latest", false},
		{"empty args", []string{}, "latest", false},
		{"empty string", []string{""}, "latest", false},
		{"whitespace only", []string{"  "}, "latest", false},
		{"latest keyword", []string{"latest"}, "latest", false},
		{"latest uppercase", []string{"LATEST"}, "latest", false},
		{"latest mixed case", []string{"Latest"}, "latest", false},
		{"calver version", []string{"2026.3.1"}, "2026.3.1", false},
		{"calver high seq", []string{"2026.12.100"}, "2026.12.100", false},
		{"invalid version", []string{"abc"}, "", true},
		{"semver rejected", []string{"v1.2.3"}, "", true},
		{"partial version", []string{"2026.3"}, "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveUpdateTarget(tt.args)
			if (err != nil) != tt.wantErr {
				t.Errorf("resolveUpdateTarget(%v) error = %v, wantErr %v", tt.args, err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("resolveUpdateTarget(%v) = %q, want %q", tt.args, got, tt.want)
			}
		})
	}
}

func TestBuildAssetName(t *testing.T) {
	name := buildAssetName("2026.3.1")

	wantPrefix := "pylon_2026.3.1_" + runtime.GOOS + "_" + runtime.GOARCH
	if runtime.GOOS == "windows" {
		want := wantPrefix + ".zip"
		if name != want {
			t.Errorf("buildAssetName(\"2026.3.1\") = %q, want %q", name, want)
		}
	} else {
		want := wantPrefix + ".tar.gz"
		if name != want {
			t.Errorf("buildAssetName(\"2026.3.1\") = %q, want %q", name, want)
		}
	}
}

func TestReplaceBinary(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("replaceBinary is not supported on Windows")
	}

	t.Run("successful replacement", func(t *testing.T) {
		dir := t.TempDir()

		// Create a "current" binary
		dst := filepath.Join(dir, "pylon")
		if err := os.WriteFile(dst, []byte("old-binary"), 0o755); err != nil {
			t.Fatal(err)
		}

		// Create a "new" binary
		src := filepath.Join(dir, "pylon-new")
		if err := os.WriteFile(src, []byte("new-binary"), 0o755); err != nil {
			t.Fatal(err)
		}

		if err := replaceBinary(dst, src); err != nil {
			t.Fatalf("replaceBinary() error = %v", err)
		}

		// Verify new content
		data, err := os.ReadFile(dst)
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != "new-binary" {
			t.Errorf("replaced binary content = %q, want %q", string(data), "new-binary")
		}

		// Verify no .bak file remains
		if _, err := os.Stat(dst + ".bak"); !os.IsNotExist(err) {
			t.Error("backup file should not exist after successful replacement")
		}

		// Verify no temp files remain
		entries, _ := os.ReadDir(dir)
		for _, e := range entries {
			if e.Name() != "pylon" && e.Name() != "pylon-new" {
				t.Errorf("unexpected file remaining: %s", e.Name())
			}
		}
	})

	t.Run("preserves permissions", func(t *testing.T) {
		dir := t.TempDir()

		dst := filepath.Join(dir, "pylon")
		if err := os.WriteFile(dst, []byte("old"), 0o755); err != nil {
			t.Fatal(err)
		}

		src := filepath.Join(dir, "pylon-new")
		if err := os.WriteFile(src, []byte("new"), 0o644); err != nil {
			t.Fatal(err)
		}

		if err := replaceBinary(dst, src); err != nil {
			t.Fatalf("replaceBinary() error = %v", err)
		}

		info, err := os.Stat(dst)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o755 {
			t.Errorf("replaced binary permissions = %o, want 0755", info.Mode().Perm())
		}
	})

	t.Run("fails if src does not exist", func(t *testing.T) {
		dir := t.TempDir()

		dst := filepath.Join(dir, "pylon")
		if err := os.WriteFile(dst, []byte("old"), 0o755); err != nil {
			t.Fatal(err)
		}

		err := replaceBinary(dst, filepath.Join(dir, "nonexistent"))
		if err == nil {
			t.Error("replaceBinary() should fail when src does not exist")
		}

		// Original binary should still exist
		data, readErr := os.ReadFile(dst)
		if readErr != nil {
			t.Fatalf("original binary should still exist: %v", readErr)
		}
		if string(data) != "old" {
			t.Error("original binary content should be preserved on failure")
		}
	})
}
