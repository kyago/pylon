package cli

import (
	"fmt"
	"os"
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

// 루트 프롬프트는 무조건 위임을 지시하면 안 된다 — 사용자가 보고한 과잉 위임의 직접 원인이었다.
func TestBuildRootCLAUDEMDPrefersDirectExecution(t *testing.T) {
	out := buildRootCLAUDEMD(&config.Config{}, nil, t.TempDir())

	if strings.Contains(out, "코드를 직접 작성하지 말고") {
		t.Error("무조건 위임 지시가 남아 있으면 안 된다")
	}
	for _, want := range []string{
		"기본값은 직접 수행입니다",
		"애매하면 직접 합니다",
		"위임할 때 프롬프트에 반드시 넣을 것",
		"안티패턴",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("위임 판단 기준이 누락됨: %q", want)
		}
	}
}

// 38개 에이전트 나열(45줄)은 Claude Code가 이미 자동 노출하므로 중복이다.
// 루트 프롬프트에는 선택 기준만 남긴다.
func TestBuildRootCLAUDEMDReplacesAgentRosterWithCriteria(t *testing.T) {
	root := t.TempDir()
	agentsDir := filepath.Join(root, ".pylon", "agents")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	agent := "---\nname: brand-strategist\nrole: Brand Strategist\ndomain: marketing\n---\n\n# Brand Strategist\n"
	if err := os.WriteFile(filepath.Join(agentsDir, "brand-strategist.md"), []byte(agent), 0o644); err != nil {
		t.Fatal(err)
	}

	out := buildRootCLAUDEMD(&config.Config{}, nil, root)

	if strings.Contains(out, "brand-strategist") {
		t.Error("에이전트 전체 나열은 제거되어야 한다 (.pylon/agents/ 및 Claude Code가 이미 노출)")
	}
	if !strings.Contains(out, "서브 에이전트 선택 기준") {
		t.Error("선택 기준표가 있어야 한다")
	}
}

// 검증 명령이 프롬프트에 없으면 루트 에이전트는 "완료" 보고를 확인할 수단이 없다.
func TestBuildRootCLAUDEMDRendersProjectVerifyCommands(t *testing.T) {
	root := t.TempDir()
	withVerify := filepath.Join(root, "api")
	if err := os.MkdirAll(filepath.Join(withVerify, ".pylon"), 0o755); err != nil {
		t.Fatal(err)
	}
	verifyYML := "build:\n  command: \"go build ./...\"\ntest:\n  command: \"go test ./...\"\n"
	if err := os.WriteFile(filepath.Join(withVerify, ".pylon", "verify.yml"), []byte(verifyYML), 0o644); err != nil {
		t.Fatal(err)
	}

	projects := []config.ProjectInfo{
		{Name: "api", Path: withVerify},
		{Name: "web", Path: filepath.Join(root, "web")}, // verify.yml 없음
	}
	out := buildRootCLAUDEMD(&config.Config{}, projects, root)

	if !strings.Contains(out, "go build ./...") || !strings.Contains(out, "go test ./...") {
		t.Error("프로젝트 verify.yml의 명령이 렌더링되어야 한다")
	}
	if !strings.Contains(out, "`.pylon/verify.yml` 없음") {
		t.Error("검증 미설정 프로젝트는 미설정으로 표시되어야 한다")
	}
}

// 문법이 깨진 verify.yml을 "없음"으로 안내하면 사용자는 파일을 새로 만들려 하고
// 진짜 원인(파싱 오류)은 남는다. 부재와 오류는 구분해서 보고해야 한다.
func TestBuildRootCLAUDEMDDistinguishesBrokenVerifyConfig(t *testing.T) {
	root := t.TempDir()
	projectDir := filepath.Join(root, "api")
	if err := os.MkdirAll(filepath.Join(projectDir, ".pylon"), 0o755); err != nil {
		t.Fatal(err)
	}
	broken := "commands:\n  - name: build\n   command: oops\n" // 들여쓰기 깨짐
	if err := os.WriteFile(filepath.Join(projectDir, ".pylon", "verify.yml"), []byte(broken), 0o644); err != nil {
		t.Fatal(err)
	}

	out := buildRootCLAUDEMD(&config.Config{}, []config.ProjectInfo{{Name: "api", Path: projectDir}}, root)

	if strings.Contains(out, "`.pylon/verify.yml` 없음") {
		t.Error("존재하지만 깨진 파일을 '없음'으로 보고하면 안 된다")
	}
	if !strings.Contains(out, "읽을 수 없습니다") {
		t.Errorf("파싱 실패가 보고되어야 한다:\n%s", out)
	}
}

// 프로젝트 서브디렉토리가 없는 단일 저장소 워크스페이스에서는 루트 verify.yml이 검증 대상이다.
// (run-verification.sh도 --git-root 없이 루트에서 돈다.)
func TestBuildRootCLAUDEMDRendersRootVerifyCommandsWithoutProjects(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".pylon"), 0o755); err != nil {
		t.Fatal(err)
	}
	verifyYML := "build:\n  command: \"make build\"\n"
	if err := os.WriteFile(filepath.Join(root, ".pylon", "verify.yml"), []byte(verifyYML), 0o644); err != nil {
		t.Fatal(err)
	}

	out := buildRootCLAUDEMD(&config.Config{}, nil, root)

	if !strings.Contains(out, "make build") {
		t.Error("프로젝트가 없어도 루트 verify.yml의 명령이 렌더링되어야 한다")
	}
	if strings.Contains(out, "--git-root") {
		t.Error("단일 저장소 워크스페이스에는 --git-root 안내가 붙으면 안 된다")
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
