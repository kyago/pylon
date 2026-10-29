package cli

import (
	"os"
	"path/filepath"
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
		`--terminal-phase completed`,
		`pylon internal trajectory failure`,
		`--terminal-phase failed`,
	} {
		if !strings.Contains(command, required) {
			t.Fatalf("pipeline command is missing repo-owned lifecycle instruction %q", required)
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
