package cli

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/kyago/pylon/internal/config"
	"github.com/kyago/pylon/internal/memory"
)

func TestBuildRootCLAUDEMDInjectsMemoryIndex(t *testing.T) {
	root := t.TempDir()
	memStore := memory.NewStore(root)
	if err := memStore.Insert(&memory.Entry{ProjectID: "app", Category: "decision",
		Key: "저장소는 md 파일", Content: "SQLite 대신 md 파일을 쓴다", Confidence: 0.9}); err != nil {
		t.Fatalf("Insert 실패: %v", err)
	}

	cfg := &config.Config{}
	cfg.Memory.ProactiveInjection = true
	projects := []config.ProjectInfo{{Name: "app", Path: filepath.Join(root, "app")}}

	out := buildRootCLAUDEMD(cfg, projects, root)
	if !strings.Contains(out, "저장소는 md 파일") {
		t.Error("메모리 인덱스가 주입되어야 한다")
	}

	cfg.Memory.ProactiveInjection = false
	out = buildRootCLAUDEMD(cfg, projects, root)
	if strings.Contains(out, "저장소는 md 파일") {
		t.Error("비활성화 시 주입되면 안 된다")
	}
}
