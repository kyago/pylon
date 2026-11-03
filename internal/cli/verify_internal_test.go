package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kyago/pylon/internal/config"
)

func TestExecuteVerification_RunsConfiguredStepsInOrder(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "order.log")
	steps := []config.NamedVerifyStep{
		{Name: "build", Command: "printf 'build\\n' >> " + shellQuote(logPath), Timeout: "5s"},
		{Name: "test", Command: "printf 'test\\n' >> " + shellQuote(logPath), Timeout: "5s"},
	}

	result, err := executeVerification(dir, steps, func() time.Time {
		return time.Date(2026, 7, 23, 0, 0, 0, 0, time.UTC)
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.OK || result.Skipped || len(result.Checks) != 2 {
		t.Fatalf("unexpected result: %+v", result)
	}
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(data); got != "build\ntest\n" {
		t.Fatalf("execution order = %q", got)
	}
}

func TestExecuteVerification_ReportsTimeout(t *testing.T) {
	steps := []config.NamedVerifyStep{{Name: "slow", Command: "sleep 1", Timeout: "10ms"}}
	result, err := executeVerification(t.TempDir(), steps, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	if result.OK || len(result.Checks) != 1 || result.Checks[0].OK {
		t.Fatalf("unexpected result: %+v", result)
	}
	if !strings.Contains(result.Checks[0].Output, "timed out") {
		t.Fatalf("timeout output = %q", result.Checks[0].Output)
	}
}

func TestExecuteVerification_HeldOutFailureFailsGate(t *testing.T) {
	steps := []config.NamedVerifyStep{
		{Name: "build", Kind: config.VerifyKindDeterministic, Command: "true", Timeout: "5s"},
		{Name: "acceptance", Kind: config.VerifyKindHeldOut, Command: "false", Timeout: "5s"},
	}
	result, err := executeVerification(t.TempDir(), steps, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	if result.OK || len(result.Checks) != 2 || result.Checks[1].Kind != config.VerifyKindHeldOut || result.Checks[1].OK {
		t.Fatalf("held-out failure did not fail gate: %+v", result)
	}
}

func TestExecuteVerification_TimeoutNotBlockedByOrphanChild(t *testing.T) {
	steps := []config.NamedVerifyStep{{Name: "orphan", Command: "sleep 8 & sleep 8", Timeout: "100ms"}}
	start := time.Now()
	result, err := executeVerification(t.TempDir(), steps, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("verification blocked by orphan child for %v", elapsed)
	}
	if result.OK || len(result.Checks) != 1 || result.Checks[0].OK {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestLoadVerificationSteps_DefaultsAndSkip(t *testing.T) {
	goProject := t.TempDir()
	if err := os.WriteFile(filepath.Join(goProject, "go.mod"), []byte("module example.com/test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	steps, skipped, err := loadVerificationSteps(goProject, filepath.Join(goProject, ".pylon", "verify.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if skipped || len(steps) != 3 || steps[0].Command != "go build ./..." || steps[1].Command != "go vet ./..." {
		t.Fatalf("unexpected Go defaults: skipped=%v steps=%+v", skipped, steps)
	}

	nonGo := t.TempDir()
	steps, skipped, err = loadVerificationSteps(nonGo, filepath.Join(nonGo, ".pylon", "verify.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if !skipped || len(steps) != 0 {
		t.Fatalf("unexpected non-Go fallback: skipped=%v steps=%+v", skipped, steps)
	}
}

func TestGeneratedVerifyConfigIncludesHeldOutSection(t *testing.T) {
	content := generateVerifyYML(techStack{Language: "go"})
	if !strings.Contains(content, "held_out: []") {
		t.Fatalf("generated verify.yml has no held_out section:\n%s", content)
	}
}

func TestInternalVerify_FailsClosedWhenNothingConfigured(t *testing.T) {
	dir := t.TempDir() // Go 프로젝트도 아니고 verify.yml도 없다
	outputPath := filepath.Join(dir, "verification.json")

	cmd := newInternalVerifyCmd()
	var out strings.Builder
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{
		"--workdir", dir,
		"--config", filepath.Join(dir, ".pylon", "verify.yml"),
		"--output", outputPath,
	})

	if err := cmd.Execute(); err == nil {
		t.Fatal("검증 설정이 없으면 실패해야 한다 (fail-closed)")
	}

	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("verification.json이 기록되어야 한다: %v", err)
	}
	var result verificationResult
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	if result.OK {
		t.Fatalf("ok는 false여야 한다: %s", data)
	}
	if !result.Skipped {
		t.Fatalf("skipped는 true여야 한다: %s", data)
	}
	if result.Reason == "" {
		t.Fatalf("reason이 비어 있으면 안 된다: %s", data)
	}
}

func TestInternalVerify_FailsClosedWhenConfigHasNoCommands(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "verify.yml")
	if err := os.WriteFile(configPath, []byte("commands: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := newInternalVerifyCmd()
	var out strings.Builder
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--workdir", dir, "--config", configPath})

	if err := cmd.Execute(); err == nil {
		t.Fatal("실행 가능한 검증 명령이 없으면 실패해야 한다 (fail-closed)")
	}
	if !strings.Contains(out.String(), "\"ok\":false") {
		t.Fatalf("ok는 false여야 한다: %s", out.String())
	}
}

func TestInternalVerify_PassesWhenStepsSucceed(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "verify.yml")
	if err := os.WriteFile(configPath, []byte("build:\n  command: \"true\"\n  timeout: 30s\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := newInternalVerifyCmd()
	var out strings.Builder
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--workdir", dir, "--config", configPath})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("설정된 검증이 성공하면 통과해야 한다: %v (%s)", err, out.String())
	}
	var result verificationResult
	if err := json.Unmarshal([]byte(strings.TrimSpace(out.String())), &result); err != nil {
		t.Fatalf("출력 파싱 실패: %v (%s)", err, out.String())
	}
	if !result.OK || result.Skipped || len(result.Checks) != 1 {
		t.Fatalf("unexpected result: %+v", result)
	}
}

// tree는 "검증한 코드 == 커밋된 HEAD"일 때만 기록된다. 그래야 PR·cleanup이 브랜치·머지 head의 tree와 대조할 수 있다.
func TestInternalVerify_RecordsHeadTreeOnlyWhenClean(t *testing.T) {
	cases := []struct {
		name     string
		setup    func(t *testing.T, dir string, git func(...string) string)
		command  string
		recorded bool
		reason   string // 기록하지 않을 때 tree_reason에 들어갈 문구
	}{
		{name: "깨끗한 작업 트리", recorded: true},
		{name: "추적 안 된 새 파일", setup: func(t *testing.T, dir string, git func(...string) string) {
			writeTreeTestFile(t, dir, "repro_test.go", "package main\n")
		}, reason: "커밋되지 않은 변경"},
		{name: "수정한 추적 파일", setup: func(t *testing.T, dir string, git func(...string) string) {
			writeTreeTestFile(t, dir, "main.go", "package main // edited\n")
		}, reason: "커밋되지 않은 변경"},
		{name: "stage만 된 파일", setup: func(t *testing.T, dir string, git func(...string) string) {
			writeTreeTestFile(t, dir, "new.go", "package main\n")
			git("add", "new.go")
		}, reason: "커밋되지 않은 변경"},
		{name: "showUntrackedFiles=no여도 새 파일은 미커밋", setup: func(t *testing.T, dir string, git func(...string) string) {
			git("config", "status.showUntrackedFiles", "no")
			writeTreeTestFile(t, dir, "repro_test.go", "package main\n")
		}, reason: "커밋되지 않은 변경"},
		{name: "stage된 .pylon 수정", setup: func(t *testing.T, dir string, git func(...string) string) {
			writeTreeTestFile(t, dir, ".pylon/config.yml", "version: 2\n")
			git("add", ".pylon/config.yml")
		}, reason: ".pylon/"},
		// 알려진 한계: pylon이 스스로 쓰는 .pylon 상태 때문에 stage 안 된 추적 .pylon 수정은 보지 않는다.
		{name: "stage 안 된 .pylon 수정", setup: func(t *testing.T, dir string, git func(...string) string) {
			writeTreeTestFile(t, dir, ".pylon/config.yml", "version: 2\n")
		}, recorded: true},
		{name: "에이전트 워크트리와 로컬 verify.yml", setup: func(t *testing.T, dir string, git func(...string) string) {
			git("worktree", "add", "-q", filepath.Join(dir, ".claude", "worktrees", "agent"), "-b", "agent")
		}, recorded: true},
		// 검증이 만든 파일은 HEAD와 고정된 명령으로 다시 만들어진다. 검증이 치우므로 tree가 남는다.
		{name: "검증 단계가 새 파일을 만듦", command: "echo cover > coverage.out", recorded: true},
		{name: "검증 단계가 추적 파일을 고침", command: "echo // formatted >> main.go", reason: "작업 트리를 바꿨습니다"},
		{name: "검증 중 빈 커밋", command: "git -c user.name=t -c user.email=t@t commit -q --allow-empty -m x", reason: "HEAD가 바뀌었습니다"},
		{name: "검증 중 커밋", command: "echo x > extra.go && git add extra.go && git -c user.name=t -c user.email=t@t commit -q -m x", reason: "HEAD가 바뀌었습니다"},
		// 계약: git status가 못 보는 표시된 파일이 디스크에 있으면 내용 비교 없이 tree를 남기지 않는다(fail-closed).
		{name: "assume-unchanged로 숨긴 수정", setup: func(t *testing.T, dir string, git func(...string) string) {
			git("update-index", "--assume-unchanged", "main.go")
			writeTreeTestFile(t, dir, "main.go", "package main // hidden\n")
		}, reason: "표시된 파일"},
		{name: "skip-worktree로 숨긴 수정", setup: func(t *testing.T, dir string, git func(...string) string) {
			git("update-index", "--skip-worktree", "main.go")
			writeTreeTestFile(t, dir, "main.go", "package main // hidden\n")
		}, reason: "표시된 파일"},
		{name: "표시만 있고 내용은 같음", setup: func(t *testing.T, dir string, git func(...string) string) {
			git("update-index", "--assume-unchanged", "main.go")
		}, reason: "표시된 파일"},
		{name: "sparse checkout처럼 디스크에 없는 skip-worktree 파일", setup: func(t *testing.T, dir string, git func(...string) string) {
			git("update-index", "--skip-worktree", "main.go")
			if err := os.Remove(filepath.Join(dir, "main.go")); err != nil {
				t.Fatal(err)
			}
		}, recorded: true},
		{name: "내장 저장소에 추적 안 된 파일만", setup: func(t *testing.T, dir string, git func(...string) string) {
			commitNestedRepo(t, dir, git)
			writeTreeTestFile(t, dir, "lib/build.out", "bin\n")
		}, recorded: true},
		{name: "내장 저장소의 미커밋 수정", setup: func(t *testing.T, dir string, git func(...string) string) {
			commitNestedRepo(t, dir, git)
			writeTreeTestFile(t, dir, "lib/lib.go", "package lib // edited\n")
		}, reason: "커밋되지 않은 변경"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir, git := newTreeTestRepo(t)
			writeTreeTestFile(t, dir, "main.go", "package main\n")
			writeTreeTestFile(t, dir, ".pylon/config.yml", "version: 1\n")
			git("add", "main.go", ".pylon/config.yml")
			git("commit", "-q", "-m", "init")
			if tc.setup != nil {
				tc.setup(t, dir, git)
			}
			head := git("rev-parse", "HEAD^{tree}")
			command := tc.command
			if command == "" {
				command = "true"
			}

			result := runTreeTestVerify(t, dir, command)
			if tc.recorded {
				if result.Tree != head {
					t.Fatalf("tree = %q (%s), HEAD tree %q여야 한다", result.Tree, result.TreeReason, head)
				}
				return
			}
			if result.Tree != "" || !strings.Contains(result.TreeReason, tc.reason) {
				t.Fatalf("tree = %q, tree_reason = %q: tree 없이 %q 사유가 있어야 한다", result.Tree, result.TreeReason, tc.reason)
			}
		})
	}
}

// 깨끗하게 시작한 검증이 만든 추적 안 된 파일은 산출물 폴더로 옮겨 다음 검증이 깨끗한 트리에서 시작하게 한다.
func TestInternalVerify_MovesArtifactsOutOfRepository(t *testing.T) {
	dir, git := newTreeTestRepo(t)
	writeTreeTestFile(t, dir, "main.go", "package main\n")
	git("add", "main.go")
	git("commit", "-q", "-m", "init")

	result := runTreeTestVerify(t, dir, "mkdir -p __pycache__ && echo pyc > __pycache__/m.pyc && echo one > coverage.out")
	realDir, err := filepath.EvalSymlinks(dir) // git이 보고하는 루트는 심볼릭 링크를 푼 경로다(macOS /private/var)
	if err != nil {
		t.Fatal(err)
	}
	artifacts := filepath.Join(realDir, ".pylon", "runtime", "verify-artifacts")
	if result.Tree == "" || result.ArtifactsDir != artifacts || strings.Join(result.Artifacts, ",") != "__pycache__/m.pyc,coverage.out" {
		t.Fatalf("tree = %q (%s), artifacts = %v in %q", result.Tree, result.TreeReason, result.Artifacts, result.ArtifactsDir)
	}
	for _, gone := range []string{"coverage.out", "__pycache__/m.pyc"} {
		if _, err := os.Stat(filepath.Join(dir, gone)); !os.IsNotExist(err) {
			t.Fatalf("%s가 repo에 남았다: %v", gone, err)
		}
	}

	// 같은 경로는 최신본으로 바뀌고, 다른 경로는 지우지 않는다(검증 중 누가 만든 파일일 수도 있다).
	result = runTreeTestVerify(t, dir, "echo two > coverage.out")
	if result.Tree == "" || strings.Join(result.Artifacts, ",") != "coverage.out" {
		t.Fatalf("tree = %q (%s), artifacts = %v", result.Tree, result.TreeReason, result.Artifacts)
	}
	if data, err := os.ReadFile(filepath.Join(artifacts, "coverage.out")); err != nil || string(data) != "two\n" {
		t.Fatalf("같은 경로는 최신본이어야 한다: %q %v", data, err)
	}
	if _, err := os.Stat(filepath.Join(artifacts, "__pycache__", "m.pyc")); err != nil {
		t.Fatalf("이전 검증의 다른 산출물을 지우면 안 된다: %v", err)
	}
}

// 지저분한 트리에서 돈 검증은 어차피 tree를 남기지 않으므로, 커밋하려던 생성 파일(스냅숏 등)을 가져가지 않는다.
func TestInternalVerify_LeavesFilesWhenStartedDirty(t *testing.T) {
	dir, git := newTreeTestRepo(t)
	writeTreeTestFile(t, dir, "main.go", "package main\n")
	git("add", "main.go")
	git("commit", "-q", "-m", "init")
	writeTreeTestFile(t, dir, "notes.txt", "draft\n")

	result := runTreeTestVerify(t, dir, "echo snap > new.snap")
	if result.Tree != "" || len(result.Artifacts) != 0 {
		t.Fatalf("tree = %q, artifacts = %v", result.Tree, result.Artifacts)
	}
	for _, kept := range []string{"notes.txt", "new.snap"} {
		if _, err := os.Stat(filepath.Join(dir, kept)); err != nil {
			t.Fatalf("%s를 옮기면 안 된다: %v", kept, err)
		}
	}
}

// run 잔여물은 지정한 파일만 경로째 run 폴더로 옮긴다. 호출마다 새 폴더라 앞서 옮긴 파일을 덮지 않는다.
func TestInternalMoveUntracked(t *testing.T) {
	dir, git := newTreeTestRepo(t)
	writeTreeTestFile(t, dir, ".gitignore", "*.log\n")
	git("add", ".gitignore")
	git("commit", "-q", "-m", "init")
	writeTreeTestFile(t, dir, "build/out/my file.bin", "bin\n")
	writeTreeTestFile(t, dir, "-n.tmp", "tmp\n")
	writeTreeTestFile(t, dir, "TODO.md", "user\n")
	writeTreeTestFile(t, dir, "debug.log", "ignored\n")

	run := func(to string, paths ...string) (string, error) {
		cmd := newInternalMoveUntrackedCmd()
		var out strings.Builder
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs(append([]string{"--workdir", dir, "--to", to, "--"}, paths...))
		err := cmd.Execute()
		return out.String(), err
	}
	for _, to := range []string{t.TempDir(), filepath.Join(dir, "leftovers")} {
		if _, err := run(to, "-n.tmp"); err == nil {
			t.Fatalf("run 폴더(.pylon/runtime) 밖으로는 옮기면 안 된다: %s", to)
		}
	}
	leftovers := filepath.Join(t.TempDir(), ".pylon", "runtime", "run-1", "repos", "svc", "leftovers")
	out, err := run(leftovers, "build", "build/out/my file.bin", "-n.tmp") // 겹치는 지정은 한 번만 옮긴다
	if err != nil || !strings.Contains(out, `"moved":["-n.tmp","build/out/my file.bin"]`) {
		t.Fatalf("move-untracked: %v (%s)", err, out)
	}
	var first struct{ Dir string }
	if err := json.Unmarshal([]byte(out), &first); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(first.Dir, "build", "out", "my file.bin")); err != nil {
		t.Fatalf("경로째 옮겨야 한다: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "build", "out", "my file.bin")); !os.IsNotExist(err) {
		t.Fatalf("옮긴 파일이 repo에 남았다: %v", err)
	}
	for _, kept := range []string{"TODO.md", "debug.log"} {
		if _, err := os.Stat(filepath.Join(dir, kept)); err != nil {
			t.Fatalf("지정하지 않은 %s는 그대로 둬야 한다: %v", kept, err)
		}
	}

	// 같은 경로를 다시 옮겨도 앞서 옮긴 파일은 남는다.
	writeTreeTestFile(t, dir, "-n.tmp", "second\n")
	if out, err := run(leftovers, "-n.tmp"); err != nil {
		t.Fatalf("두 번째 이동: %v (%s)", err, out)
	}
	if data, err := os.ReadFile(filepath.Join(first.Dir, "-n.tmp")); err != nil || string(data) != "tmp\n" {
		t.Fatalf("앞서 옮긴 잔여물을 덮었다: %q %v", data, err)
	}
	// 추적 안 된 파일이 아닌 경로는 옮기지 않고 실패로 알린다.
	if out, err := run(leftovers, ".gitignore"); err == nil || !strings.Contains(out, `"not_untracked":[".gitignore"]`) {
		t.Fatalf("추적 중인 파일 지정은 실패해야 한다: %v (%s)", err, out)
	}
}

// 옮길 자리가 막히면 파일은 원래 위치로 돌아가고 임시 폴더도 남지 않는다(계약: 사용자 파일을 잃지 않는다).
func TestMoveEntryRestoresSourceOnConflict(t *testing.T) {
	top, root := t.TempDir(), t.TempDir()
	src := writeTreeTestFile(t, top, "notes.md", "user\n")
	dst := writeTreeTestFile(t, root, "notes.md", "earlier\n")

	if err := moveEntry(src, root, dst, false); err == nil {
		t.Fatal("덮어쓰지 않는 이동은 자리가 있으면 실패해야 한다")
	}
	if data, err := os.ReadFile(src); err != nil || string(data) != "user\n" {
		t.Fatalf("원래 위치로 돌아와야 한다: %q %v", data, err)
	}
	if data, err := os.ReadFile(dst); err != nil || string(data) != "earlier\n" {
		t.Fatalf("기존 파일을 건드리면 안 된다: %q %v", data, err)
	}
	if staging, _ := filepath.Glob(filepath.Join(root, ".pylon-moving-*")); len(staging) != 0 {
		t.Fatalf("임시 폴더가 남았다: %v", staging)
	}
}

// 이전 검증이 옮겨 둔 심볼릭 링크를 따라가 산출물 폴더 밖을 지우거나 쓰면 안 된다.
func TestInternalVerify_ArtifactMovesNeverFollowSymlinks(t *testing.T) {
	dir, git := newTreeTestRepo(t)
	git("commit", "-q", "--allow-empty", "-m", "init")
	outside := t.TempDir()
	writeTreeTestFile(t, outside, "a.txt", "keep\n")

	if result := runTreeTestVerify(t, dir, "ln -s "+shellQuote(outside)+" out"); result.Tree == "" {
		t.Fatalf("첫 검증: %s", result.TreeReason)
	}
	result := runTreeTestVerify(t, dir, "mkdir out && echo new > out/a.txt")
	if result.Tree == "" || strings.Join(result.Artifacts, ",") != "out/a.txt" {
		t.Fatalf("tree = %q (%s), artifacts = %v", result.Tree, result.TreeReason, result.Artifacts)
	}
	if data, err := os.ReadFile(filepath.Join(outside, "a.txt")); err != nil || string(data) != "keep\n" {
		t.Fatalf("링크 너머의 파일이 바뀌었다: %q %v", data, err)
	}
	if data, err := os.ReadFile(filepath.Join(result.ArtifactsDir, "out", "a.txt")); err != nil || string(data) != "new\n" {
		t.Fatalf("새 산출물이 산출물 폴더에 있어야 한다: %q %v", data, err)
	}
}

func TestInternalVerify_ArtifactsFollowOutputAndFailedRuns(t *testing.T) {
	dir, git := newTreeTestRepo(t)
	git("commit", "-q", "--allow-empty", "-m", "init")
	runDir := filepath.Join(t.TempDir(), ".pylon", "runtime", "run-1", "repos", "svc")
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatal(err)
	}
	configPath := writeTreeTestFile(t, dir, ".pylon/verify.yml", "build:\n  command: \"echo log > failure.log; false\"\n")
	cmd := newInternalVerifyCmd()
	var out strings.Builder
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--workdir", dir, "--config", configPath, "--output", filepath.Join(runDir, "verification.json")})
	if err := cmd.Execute(); err == nil {
		t.Fatal("실패한 검증은 오류를 내야 한다")
	}
	if _, err := os.Stat(filepath.Join(runDir, "verify-artifacts", "failure.log")); err != nil {
		t.Fatalf("실패한 검증의 산출물도 --output 옆으로 옮겨야 한다: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "failure.log")); !os.IsNotExist(err) {
		t.Fatalf("산출물이 repo에 남았다: %v", err)
	}
}

// 결과 파일을 프로젝트 안에 두면 그 옆 verify-artifacts/는 프로젝트 파일이다. 산출물은 .pylon/runtime으로 보낸다.
func TestInternalVerify_ArtifactsNeverWipeProjectDirectory(t *testing.T) {
	dir, git := newTreeTestRepo(t)
	writeTreeTestFile(t, dir, "verify-artifacts/keep.txt", "user\n")
	git("add", "verify-artifacts")
	git("commit", "-q", "-m", "init")
	configPath := writeTreeTestFile(t, dir, ".pylon/verify.yml", "build:\n  command: \"echo cover > coverage.out\"\n")
	cmd := newInternalVerifyCmd()
	var out strings.Builder
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--workdir", dir, "--config", configPath, "--output", filepath.Join(dir, "verification.json")})
	_ = cmd.Execute() // verification.json 자체가 프로젝트 안의 새 파일이라 결과는 상관없다

	if _, err := os.Stat(filepath.Join(dir, "verify-artifacts", "keep.txt")); err != nil {
		t.Fatalf("프로젝트의 verify-artifacts/를 지웠다: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".pylon", "runtime", "verify-artifacts", "coverage.out")); err != nil {
		t.Fatalf("산출물은 .pylon/runtime으로 가야 한다: %v", err)
	}
}

// 결과 파일이 run 폴더(.pylon/runtime)에 있지 않으면 그 옆 사용자 폴더로 산출물을 옮기지 않는다.
func TestInternalVerify_ArtifactsStayUnderPylonRuntime(t *testing.T) {
	dir, git := newTreeTestRepo(t)
	git("commit", "-q", "--allow-empty", "-m", "init")
	elsewhere := t.TempDir()
	writeTreeTestFile(t, elsewhere, "verify-artifacts/coverage.out", "user\n")
	configPath := writeTreeTestFile(t, dir, ".pylon/verify.yml", "build:\n  command: \"echo cover > coverage.out\"\n")
	cmd := newInternalVerifyCmd()
	var out strings.Builder
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--workdir", dir, "--config", configPath, "--output", filepath.Join(elsewhere, "verification.json")})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("verify: %v (%s)", err, out.String())
	}
	if data, err := os.ReadFile(filepath.Join(elsewhere, "verify-artifacts", "coverage.out")); err != nil || string(data) != "user\n" {
		t.Fatalf(".pylon/runtime 밖의 폴더를 건드렸다: %q %v", data, err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".pylon", "runtime", "verify-artifacts", "coverage.out")); err != nil {
		t.Fatalf("산출물은 repo의 .pylon/runtime으로 가야 한다: %v", err)
	}
}

func TestInternalVerify_NoTreeWithoutCommittedHead(t *testing.T) {
	t.Run("git 저장소가 아님", func(t *testing.T) {
		if result := runTreeTestVerify(t, t.TempDir(), "true"); result.Tree != "" || !strings.Contains(result.TreeReason, "HEAD") {
			t.Fatalf("tree = %q, tree_reason = %q", result.Tree, result.TreeReason)
		}
	})
	t.Run("첫 커밋 전", func(t *testing.T) {
		dir, _ := newTreeTestRepo(t)
		if result := runTreeTestVerify(t, dir, "true"); result.Tree != "" || !strings.Contains(result.TreeReason, "HEAD") {
			t.Fatalf("tree = %q, tree_reason = %q", result.Tree, result.TreeReason)
		}
	})
	t.Run("검증 실패", func(t *testing.T) {
		dir, git := newTreeTestRepo(t)
		git("commit", "-q", "--allow-empty", "-m", "init")
		configPath := writeTreeTestFile(t, dir, ".pylon/verify.yml", "build:\n  command: \"false\"\n")
		cmd := newInternalVerifyCmd()
		var out strings.Builder
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs([]string{"--workdir", dir, "--config", configPath})
		if err := cmd.Execute(); err == nil {
			t.Fatal("실패한 검증은 오류를 내야 한다")
		}
		var result verificationResult
		if err := json.Unmarshal([]byte(strings.Split(out.String(), "\n")[0]), &result); err != nil {
			t.Fatalf("출력 파싱 실패: %v (%s)", err, out.String())
		}
		if result.OK || result.Tree != "" {
			t.Fatalf("실패한 검증은 tree를 남기면 안 된다: %+v", result)
		}
	})
}

// commitNestedRepo commits lib/ as its own repository and records it in dir as a gitlink.
func commitNestedRepo(t *testing.T, dir string, git func(...string) string) {
	t.Helper()
	inner := filepath.Join(dir, "lib")
	git("init", "-q", inner)
	writeTreeTestFile(t, dir, "lib/lib.go", "package lib\n")
	git("-C", inner, "add", "lib.go")
	git("-C", inner, "commit", "-q", "-m", "lib")
	git("add", "lib")
	git("commit", "-q", "-m", "gitlink")
}

func newTreeTestRepo(t *testing.T) (string, func(args ...string) string) {
	t.Helper()
	dir := t.TempDir()
	git := func(args ...string) string {
		t.Helper()
		out, err := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@t"}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v (%s)", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	git("init", "-q")
	return dir, git
}

func writeTreeTestFile(t *testing.T, dir, rel, content string) string {
	t.Helper()
	path := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// runTreeTestVerify runs `pylon internal verify` with one build step; .pylon/verify.yml stays untracked.
func runTreeTestVerify(t *testing.T, dir, command string) verificationResult {
	t.Helper()
	configPath := writeTreeTestFile(t, dir, ".pylon/verify.yml", fmt.Sprintf("build:\n  command: %q\n  timeout: 30s\n", command))
	cmd := newInternalVerifyCmd()
	var out strings.Builder
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--workdir", dir, "--config", configPath})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("verify failed: %v (%s)", err, out.String())
	}
	var result verificationResult
	if err := json.Unmarshal([]byte(strings.TrimSpace(out.String())), &result); err != nil {
		t.Fatalf("출력 파싱 실패: %v (%s)", err, out.String())
	}
	return result
}

func TestNewInternalCmd_IsHidden(t *testing.T) {
	cmd := newInternalCmd()
	if !cmd.Hidden {
		t.Fatal("internal command must be hidden")
	}
	verify, _, err := cmd.Find([]string{"verify"})
	if err != nil || verify == nil || verify.Name() != "verify" {
		t.Fatalf("verify command not registered: cmd=%v err=%v", verify, err)
	}
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

// `pylon internal --help`는 서브커맨드 목록을 보여줘야 한다 (#105: 전부 Hidden이라 빈 help가 출력되던 회귀).
func TestInternalHelpListsSubcommands(t *testing.T) {
	cmd := newInternalCmd()
	var out strings.Builder
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"verify"} {
		if !strings.Contains(out.String(), "\n  "+name+" ") {
			t.Fatalf("internal --help missing subcommand %q:\n%s", name, out.String())
		}
	}
}
