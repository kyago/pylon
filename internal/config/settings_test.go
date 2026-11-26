package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSettings(t *testing.T) {
	cases := []struct {
		name, content string
		want, wantErr bool
	}{
		{"on", "version: \"0.2\"\nauto_review: true\n", true, false},
		{"off", "auto_review: false\n", false, false},
		{"absent key with old keys", "version: \"0.1\"\nruntime:\n  provider: claude\n", false, false},
		{"not a bool", "auto_review: maybe\n", false, true},
		{"broken yaml", "auto_review: [\n", false, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			root := t.TempDir()
			if err := os.MkdirAll(filepath.Join(root, ".pylon"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, ".pylon", "config.yml"), []byte(c.content), 0o644); err != nil {
				t.Fatal(err)
			}
			got, err := LoadSettings(root)
			if got.AutoReview != c.want || (err != nil) != c.wantErr {
				t.Errorf("AutoReview = %v, err = %v", got.AutoReview, err)
			}
		})
	}
	if got, err := LoadSettings(t.TempDir()); got.AutoReview || err != nil {
		t.Errorf("missing file: %v, %v", got, err)
	}
}
