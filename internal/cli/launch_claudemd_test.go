package cli

import (
	"fmt"
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

// 여러 프로젝트가 예산을 공유할 때, 앞 프로젝트의 큰 인덱스가 예산을
// 독식해 뒤 프로젝트가 0바이트를 받는 굶주림이 없어야 한다.
func TestBuildRootCLAUDEMDFairShareAcrossProjects(t *testing.T) {
	root := t.TempDir()
	s := memory.NewStore(root)
	// aaa: 기본 예산(2000토큰×4=8000바이트)을 혼자 초과할 만큼의 필러 항목.
	// 근사 중복 감지에 걸리지 않도록 항목마다 뚜렷이 다른 토큰을 반복한다.
	// (동률 confidence는 recency desc로 정렬되므로 필러의 삽입 순서에 의존하는
	// 단정은 하지 않는다 — 상위 노출 검증은 아래 고확신 항목이 담당한다.)
	for i := 0; i < 60; i++ {
		e := &memory.Entry{ProjectID: "aaa", Category: "learning",
			Key:        fmt.Sprintf("항목-%02d", i),
			Content:    strings.Repeat(fmt.Sprintf("학습토큰%02d ", i), 12),
			Confidence: 0.5}
		if err := s.Insert(e); err != nil {
			t.Fatalf("Insert(%d) 실패: %v", i, err)
		}
	}
	// aaa의 고확신 항목: 절단 후에도 반드시 살아남아야 한다 (confidence 우선 정렬)
	if err := s.Insert(&memory.Entry{ProjectID: "aaa", Category: "learning",
		Key: "최상위 항목", Content: "확신도가 가장 높아 절단 후에도 살아남아야 하는 핵심 지식",
		Confidence: 0.95}); err != nil {
		t.Fatalf("최상위 항목 Insert 실패: %v", err)
	}
	if err := s.Insert(&memory.Entry{ProjectID: "bbb", Category: "decision",
		Key: "굶주림 방지 확인용", Content: "뒤 프로젝트도 예산을 배정받아 주입되어야 한다",
		Confidence: 0.9}); err != nil {
		t.Fatalf("bbb Insert 실패: %v", err)
	}

	cfg := &config.Config{}
	cfg.Memory.ProactiveInjection = true
	projects := []config.ProjectInfo{
		{Name: "aaa", Path: filepath.Join(root, "aaa")},
		{Name: "bbb", Path: filepath.Join(root, "bbb")},
	}

	out := buildRootCLAUDEMD(cfg, projects, root)
	if !strings.Contains(out, "굶주림 방지 확인용") {
		t.Error("뒤 프로젝트가 예산에서 굶으면 안 된다 (공정 분배)")
	}
	if !strings.Contains(out, "최상위 항목") {
		t.Error("앞 프로젝트의 고확신 항목은 절단 후에도 주입되어야 한다")
	}
}
