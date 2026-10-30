package cli

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/kyago/pylon/internal/config"
	"github.com/kyago/pylon/internal/layout"
)

func TestPipelineCommandUsesRepoOwnedBranches(t *testing.T) {
	content, err := embeddedCommands.ReadFile("commands/pl-pipeline.md")
	if err != nil {
		t.Fatal(err)
	}
	command := string(content)
	if strings.Contains(command, `BRANCH=$(echo "$INIT_RESULT" | jq -r '.branch')`) {
		t.Fatal("root pipeline still reads a global branch from init output")
	}
	for _, required := range []string{
		`--pipeline-dir "$ROOT_PIPELINE_DIR"`,
		`--held-out-output "$REPO_PIPELINE_DIR/evaluator-only/held-out.json"`,
		`.sub_pipelines[] | select(.repo == $repo) | .branch`,
		`.status = "awaiting_cleanup"`,
		`pylon internal trajectory failure`,
		`--terminal-phase failed`,
	} {
		if !strings.Contains(command, required) {
			t.Fatalf("pipeline command is missing repo-owned lifecycle instruction %q", required)
		}
	}
}

// 파이프라인은 완료 시 run을 정리하지 않고 /pl:cleanup에 넘긴다. 정리는 cleanup이 완료 체크포인트를 만든 뒤에만
// 한다 — 체크포인트 없이 runtime·워크트리를 지우지 않기 위해.
func TestCleanupClosesPipelineRunAfterCheckpoint(t *testing.T) {
	pipeline, err := embeddedCommands.ReadFile("commands/pl-pipeline.md")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(pipeline), `cleanup-pipeline.sh "$ROOT_PIPELINE_DIR" --terminal-phase completed`) {
		t.Fatal("pipeline must hand the completed run to /pl:cleanup instead of tearing it down")
	}
	content, err := embeddedCommands.ReadFile("commands/pl-cleanup.md")
	if err != nil {
		t.Fatal(err)
	}
	cleanup := string(content)
	checkpoint := strings.Index(cleanup, `pylon history checkpoint --pipeline "$PIPELINE_ID" --phase "$PHASE"`)
	teardown := strings.Index(cleanup, `cleanup-pipeline.sh "$ROOT_PIPELINE_DIR" --terminal-phase "$PHASE"`)
	if checkpoint < 0 || teardown < checkpoint {
		t.Fatalf("cleanup must checkpoint before teardown: checkpoint=%d teardown=%d", checkpoint, teardown)
	}
	for _, required := range []string{"awaiting_cleanup", "git merge-base --is-ancestor", "headRefOid", "^{tree}", "git worktree remove <path>"} {
		if !strings.Contains(cleanup, required) {
			t.Errorf("cleanup command is missing %q", required)
		}
	}
	if strings.Contains(cleanup, "git worktree remove --force") {
		t.Error("cleanup must not force-remove user worktrees")
	}
}

// 커맨드의 subagent_type과 매뉴얼 서브 에이전트 표가 부르는 이름은 모두 내장 에이전트 파일로 존재하고
// 그 name: 과 같아야 한다 — 에이전트를 통합·삭제하면서 남은 옛 이름이 호출되지 않도록.
func TestAgentReferencesResolveToBuiltins(t *testing.T) {
	refs := map[string]string{} // 이름 → 처음 발견한 위치
	entries, err := embeddedCommands.ReadDir("commands")
	if err != nil {
		t.Fatal(err)
	}
	subagent := regexp.MustCompile(`subagent_type="([^"]+)"`)
	found := 0
	for _, entry := range entries {
		data, err := embeddedCommands.ReadFile("commands/" + entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range subagent.FindAllStringSubmatch(string(data), -1) {
			refs[m[1]] = "commands/" + entry.Name()
			found++
		}
	}
	if found < 4 {
		t.Fatalf("subagent_type references found = %d, parser likely broken", found)
	}

	manual, err := embeddedReference.ReadFile("reference/pylon-usage.md")
	if err != nil {
		t.Fatal(err)
	}
	section := string(manual)
	start := strings.Index(section, "### 서브 에이전트 선택 기준")
	if start < 0 {
		t.Fatal("manual agent selection section missing")
	}
	section = section[start:]
	if end := strings.Index(section, "\n## "); end >= 0 {
		section = section[:end]
	}
	backtick := regexp.MustCompile("`([^`]+)`")
	found = 0
	for _, line := range strings.Split(section, "\n") {
		cells := strings.Split(line, "|")
		if !strings.HasPrefix(line, "|") || len(cells) < 3 {
			continue
		}
		for _, m := range backtick.FindAllStringSubmatch(cells[2], -1) {
			refs[m[1]] = "reference/pylon-usage.md"
			found++
		}
	}
	if found < 6 {
		t.Fatalf("manual table agent names found = %d, parser likely broken", found)
	}

	for name, where := range refs {
		data, err := embeddedAgents.ReadFile("agents/" + name + ".md")
		if err != nil {
			t.Errorf("%s references agent %q with no agents/%s.md", where, name, name)
			continue
		}
		agent, err := config.ParseAgentData(data)
		if err != nil {
			t.Errorf("parse agents/%s.md: %v", name, err)
			continue
		}
		if agent.Name != name {
			t.Errorf("agents/%s.md has name %q, referenced as %q in %s", name, agent.Name, name, where)
		}
	}
}

func TestExecuteCommandRequiresStructuredTaskReports(t *testing.T) {
	content, err := embeddedCommands.ReadFile("commands/pl-execute.md")
	if err != nil {
		t.Fatal(err)
	}
	command := string(content)
	for _, required := range []string{
		`pylon internal trajectory task-report`,
		`hypotheses_rejected`,
		`remaining_unknowns`,
		`tasks/$TASK_ID/task-report.json`,
	} {
		if !strings.Contains(command, required) {
			t.Fatalf("execute command is missing trajectory instruction %q", required)
		}
	}
}

func TestPipelineRecordsVerifiedFlagAfterEvaluator(t *testing.T) {
	content, err := embeddedCommands.ReadFile("commands/pl-pipeline.md")
	if err != nil {
		t.Fatal(err)
	}
	command := string(content)
	record := strings.Index(command, `pylon internal evaluator record`)
	verified := strings.Index(command, `verified: true|false`)
	if record < 0 || verified < record {
		t.Fatalf("task verification must follow evaluator recording: evaluator=%d verified=%d", record, verified)
	}
	// verificationResult의 통과 필드는 ok다. 없는 필드를 읽으면 verified가 영원히 false가 된다.
	if !strings.Contains(command, `jq -r '.ok == true' "$REPO_PIPELINE_DIR/verification.json"`) {
		t.Fatal("pipeline must read the deterministic gate from verification.json's ok field")
	}
	for _, removed := range []string{"pylon internal state", "pylon internal corpus", "pylon internal curator"} {
		if strings.Contains(command, removed) {
			t.Fatalf("pipeline command still references removed command %q", removed)
		}
	}
}

func TestExecuteDoesNotReferenceRemovedStateStore(t *testing.T) {
	content, err := embeddedCommands.ReadFile("commands/pl-execute.md")
	if err != nil {
		t.Fatal(err)
	}
	command := string(content)
	for _, removed := range []string{"pylon internal state", "fencing", "lease"} {
		if strings.Contains(command, removed) {
			t.Fatalf("execute command still references removed state store %q", removed)
		}
	}
}

// 내장 이름의 사용자 커맨드(이전 릴리스가 남긴 낡은 사본)는 내장 커맨드를 가리지 못하고,
// 같은 실행에서 .pylon-bak으로 비켜난다.
func TestGenerateClaudeDir_LegacyCommandCopyNeverShadowsBuiltin(t *testing.T) {
	root := t.TempDir()
	pylonCmds := layout.ResourceDir(root, "commands")
	if err := os.MkdirAll(pylonCmds, 0755); err != nil {
		t.Fatal(err)
	}
	legacy := filepath.Join(pylonCmds, "pl-pipeline.md")
	if err := os.WriteFile(legacy, []byte("OLD VERSION\n"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := generateClaudeDir(root, &config.Config{}, nil); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(filepath.Join(root, ".claude", "commands", "pl", "pipeline.md"))
	if err != nil {
		t.Fatal(err)
	}
	want, err := embeddedCommands.ReadFile("commands/pl-pipeline.md")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatal("낡은 사본이 내장 커맨드를 가렸다")
	}
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Fatal("내장 이름의 낡은 사본이 사용자 디렉토리에 남았다")
	}
	if bak, err := os.ReadFile(legacy + rootFileBackupSuffix); err != nil || string(bak) != "OLD VERSION\n" {
		t.Fatalf("내용이 다른 사본은 백업되어야 한다: %q, %v", bak, err)
	}
}

// pl-cleanup 명령이 임베드되고 같은 실행에서 .claude/commands/pl/cleanup.md로 생성되어야 한다.
func TestGenerateClaudeDir_InstallsCleanupCommand(t *testing.T) {
	root := t.TempDir()

	if err := generateClaudeDir(root, &config.Config{}, nil); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(filepath.Join(root, ".claude", "commands", "pl", "cleanup.md"))
	if err != nil {
		t.Fatalf("pl:cleanup 명령이 생성되지 않았다: %v", err)
	}
	want, err := embeddedCommands.ReadFile("commands/pl-cleanup.md")
	if err != nil {
		t.Fatalf("pl-cleanup.md가 임베드되지 않았다: %v", err)
	}
	if string(got) != string(want) {
		t.Fatal("생성된 cleanup 명령이 임베드 원본과 일치하지 않는다")
	}
}
