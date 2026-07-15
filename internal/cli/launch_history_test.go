package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kyago/pylon/internal/config"
)

func TestPrepareHistoryInitializesMissingRepository(t *testing.T) {
	installFakeFossil(t)
	root := t.TempDir()

	if err := prepareHistory(root, config.HistoryConfig{}); err != nil {
		t.Fatalf("prepareHistory failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".pylon", "history", "checkout", ".fslckout")); err != nil {
		t.Fatalf("history checkout was not initialized: %v", err)
	}
}

func TestPrepareHistoryMissingFossilSuggestsDoctor(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	err := prepareHistory(t.TempDir(), config.HistoryConfig{})
	if err == nil || !strings.Contains(err.Error(), "pylon doctor") {
		t.Fatalf("missing Fossil error must suggest doctor: %v", err)
	}
}
