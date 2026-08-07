package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteReferenceTemplatesWritesManual(t *testing.T) {
	pylonDir := t.TempDir()
	if err := writeReferenceTemplates(pylonDir); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(pylonDir, "reference", "pylon-usage.md"))
	if err != nil {
		t.Fatalf("manual not written: %v", err)
	}
	if !strings.Contains(string(got), "pylon 워크스페이스 운영 매뉴얼") {
		t.Errorf("manual content missing, got: %.60q", got)
	}
}
