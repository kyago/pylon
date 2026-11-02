package cli

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kyago/pylon/internal/history"
	"github.com/kyago/pylon/internal/layout"
)

type pipelineInitResult struct {
	PipelineID   string `json:"pipeline_id"`
	Scope        string `json:"scope"`
	RepoID       string `json:"repo_id"`
	Repo         string `json:"repo"`
	Branch       string `json:"branch"`
	BaseRevision string `json:"base_revision"`
	PipelineDir  string `json:"pipeline_dir"`
}

func TestInitPipelineRootOwnsNoBranchAndRegistersRepoPipelines(t *testing.T) {
	workspace := setupPipelineScriptWorkspace(t)
	repoA, baseA := initGitRepo(t, workspace, "service-a")
	repoB, baseB := initGitRepo(t, workspace, filepath.Join("nested", "service-b"))

	root := runInitPipeline(t, workspace, "update two services")
	if root.Scope != "root" {
		t.Fatalf("root scope = %q, want root", root.Scope)
	}
	if root.Branch != "" {
		t.Fatalf("root pipeline must not own a branch, got %q", root.Branch)
	}

	subA := runInitPipeline(t, workspace, "update two services", "--git-root", "service-a", "--pipeline-dir", root.PipelineDir)
	subB := runInitPipeline(t, workspace, "update two services", "--git-root", filepath.Join("nested", "service-b"), "--pipeline-dir", root.PipelineDir)

	if subA.Scope != "repo" || subB.Scope != "repo" {
		t.Fatalf("sub-pipeline scopes = %q, %q; want repo", subA.Scope, subB.Scope)
	}
	if subA.RepoID == subB.RepoID {
		t.Fatalf("repo IDs must be unique: %q", subA.RepoID)
	}
	if subA.BaseRevision != baseA || subB.BaseRevision != baseB {
		t.Fatalf("base revisions = %q, %q; want %q, %q", subA.BaseRevision, subB.BaseRevision, baseA, baseB)
	}
	assertCurrentBranch(t, repoA, subA.Branch)
	assertCurrentBranch(t, repoB, subB.Branch)
	repeatedA := runInitPipeline(t, workspace, "update two services", "--git-root", "service-a", "--pipeline-dir", root.PipelineDir)
	if repeatedA.BaseRevision != baseA {
		t.Fatalf("reinitialization changed base revision from %q to %q", baseA, repeatedA.BaseRevision)
	}

	status := readJSONMap(t, filepath.Join(root.PipelineDir, "status.json"))
	if _, exists := status["branch"]; exists {
		t.Fatal("root status must not contain branch")
	}
	registered, ok := status["sub_pipelines"].([]any)
	if !ok || len(registered) != 2 {
		t.Fatalf("root sub_pipelines = %#v, want two entries", status["sub_pipelines"])
	}
	for _, item := range registered {
		entry := item.(map[string]any)
		if entry["repo_id"] == "" || entry["repo"] == "" || entry["branch"] == "" || entry["base_revision"] == "" {
			t.Fatalf("incomplete sub-pipeline registration: %#v", entry)
		}
	}
}

func TestInitPipelineNamespacesBranchesByLogicalRun(t *testing.T) {
	workspace := setupPipelineScriptWorkspace(t)
	repo, _ := initGitRepo(t, workspace, "service-a")

	firstRoot := runInitPipeline(t, workspace, "repeat requirement")
	firstSub := runInitPipeline(t, workspace, "repeat requirement", "--git-root", "service-a", "--pipeline-dir", firstRoot.PipelineDir)
	runPipelineGit(t, repo, "checkout", "main")
	secondRoot := runInitPipeline(t, workspace, "repeat requirement")
	secondSub := runInitPipeline(t, workspace, "repeat requirement", "--git-root", "service-a", "--pipeline-dir", secondRoot.PipelineDir)

	if firstRoot.PipelineID == secondRoot.PipelineID {
		t.Fatalf("logical runs reused pipeline id %q", firstRoot.PipelineID)
	}
	if firstSub.Branch == secondSub.Branch {
		t.Fatalf("logical runs reused task branch %q", firstSub.Branch)
	}
	if !strings.Contains(firstSub.Branch, firstRoot.PipelineID) || !strings.Contains(secondSub.Branch, secondRoot.PipelineID) {
		t.Fatalf("branches are not namespaced by run: %q %q", firstSub.Branch, secondSub.Branch)
	}
	if want := "task-" + firstRoot.PipelineID + "-service-a"; firstSub.Branch != want {
		t.Fatalf("default branch = %q, want %q", firstSub.Branch, want)
	}

	// /pl:pipeline은 규칙이 없는 repo에 빈 --branch를 넘긴다.
	runPipelineGit(t, repo, "checkout", "main")
	thirdRoot := runInitPipeline(t, workspace, "repeat requirement")
	thirdSub := runInitPipeline(t, workspace, "repeat requirement", "--git-root", "service-a", "--pipeline-dir", thirdRoot.PipelineDir, "--branch", "")
	if want := "task-" + thirdRoot.PipelineID + "-service-a"; thirdSub.Branch != want {
		t.Fatalf("empty --branch = %q, want %q", thirdSub.Branch, want)
	}
}

func TestInitPipelineRejectsOwnedBranchThatLostBaseAncestry(t *testing.T) {
	workspace := setupPipelineScriptWorkspace(t)
	repo, _ := initGitRepo(t, workspace, "service-a")
	root := runInitPipeline(t, workspace, "stale branch")
	sub := runInitPipeline(t, workspace, "stale branch", "--git-root", "service-a", "--pipeline-dir", root.PipelineDir)

	runPipelineGit(t, repo, "checkout", "--orphan", "unrelated")
	runPipelineGit(t, repo, "rm", "-rf", ".")
	if err := os.WriteFile(filepath.Join(repo, "unrelated.txt"), []byte("unrelated\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runPipelineGit(t, repo, "add", "unrelated.txt")
	runPipelineGit(t, repo, "commit", "-m", "unrelated root")
	unrelated := strings.TrimSpace(runPipelineGit(t, repo, "rev-parse", "HEAD"))
	runPipelineGit(t, repo, "checkout", "main")
	runPipelineGit(t, repo, "branch", "-f", sub.Branch, unrelated)

	cmd := exec.Command(filepath.Join(layout.ScriptsDir(workspace), "init-pipeline.sh"),
		"stale branch", "--git-root", "service-a", "--pipeline-dir", root.PipelineDir)
	cmd.Dir = workspace
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("stale owned branch was accepted: %s", output)
	}
	if !strings.Contains(string(output), "recorded base revision") {
		t.Fatalf("unexpected stale branch error: %s", output)
	}
}

// 사용자 규칙으로 정한 이름은 체크아웃·status·root 등록에 그대로 쓰이고, 재진입은 저장된 이름을 유지한다.
func TestInitPipelineUsesExplicitBranch(t *testing.T) {
	workspace := setupPipelineScriptWorkspace(t)
	repo, _ := initGitRepo(t, workspace, "service-a")
	root := runInitPipeline(t, workspace, "named branch")
	repoArgs := []string{"--git-root", "service-a", "--pipeline-dir", root.PipelineDir}
	const branch = "feature/API-123-add-group-code"

	sub := runInitPipeline(t, workspace, "named branch", append(repoArgs, "--branch", branch)...)
	if sub.Branch != branch {
		t.Fatalf("branch = %q, want %q", sub.Branch, branch)
	}
	assertCurrentBranch(t, repo, branch)
	statusPath := filepath.Join(sub.PipelineDir, "status.json")
	if got := readJSONMap(t, statusPath)["branch"]; got != branch {
		t.Fatalf("status branch = %v, want %q", got, branch)
	}
	entries := readJSONMap(t, filepath.Join(root.PipelineDir, "status.json"))["sub_pipelines"].([]any)
	if got := entries[0].(map[string]any)["branch"]; got != branch {
		t.Fatalf("root entry branch = %v, want %q", got, branch)
	}

	if again := runInitPipeline(t, workspace, "named branch", repoArgs...); again.Branch != branch {
		t.Fatalf("re-entry branch = %q, want stored %q", again.Branch, branch)
	}
	output := runInitPipelineFailure(t, workspace, append(repoArgs, "--branch", "feature/other")...)
	if !strings.Contains(output, "branch ownership mismatch") {
		t.Fatalf("re-entry with another name: %s", output)
	}
	assertBranchMissing(t, repo, "feature/other")

	// status.json이 root 등록과 어긋나면 그 브랜치를 이 run의 것으로 믿지 않는다 — 정리가 브랜치를 지우는 근거다.
	status, err := os.ReadFile(statusPath)
	if err != nil {
		t.Fatal(err)
	}
	tampered := strings.Replace(string(status), `"branch":"`+branch+`"`, `"branch":"someone-elses"`, 1)
	if err := os.WriteFile(statusPath, []byte(tampered), 0644); err != nil {
		t.Fatal(err)
	}
	output = runInitPipelineFailure(t, workspace, repoArgs...)
	if !strings.Contains(output, "branch ownership mismatch") {
		t.Fatalf("tampered status: %s", output)
	}
}

func TestInitPipelineRejectsUnusableBranchNames(t *testing.T) {
	workspace := setupPipelineScriptWorkspace(t)
	repo, _ := initGitRepo(t, workspace, "service-a")
	runPipelineGit(t, repo, "branch", "taken")
	// 직전 브랜치가 지워진 상태에서는 @{-1}이 그 이름으로 풀려 git이 새 브랜치를 만들어 버린다.
	runPipelineGit(t, repo, "checkout", "-b", "previous")
	runPipelineGit(t, repo, "checkout", "main")
	runPipelineGit(t, repo, "branch", "-D", "previous")
	runPipelineGit(t, repo, "tag", "v1")
	root := runInitPipeline(t, workspace, "bad names")
	before := runPipelineGit(t, repo, "for-each-ref", "refs/heads")

	for _, name := range []string{"-x", "-", "a..b", "@{-1}", "@", "heads/main", "refs/heads/main", "v1", "bad name", "main", "develop", "DEVELOP", "taken"} {
		runInitPipelineFailure(t, workspace, "--git-root", "service-a", "--pipeline-dir", root.PipelineDir, "--branch", name)
		if after := runPipelineGit(t, repo, "for-each-ref", "refs/heads"); after != before {
			t.Fatalf("--branch %q changed branches:\n%s", name, after)
		}
		assertCurrentBranch(t, repo, "main")
	}
	if entries := readJSONMap(t, filepath.Join(root.PipelineDir, "status.json"))["sub_pipelines"].([]any); len(entries) != 0 {
		t.Fatalf("rejected names were registered: %v", entries)
	}

	// root pipeline은 브랜치를 소유하지 않는다.
	if output := runInitPipelineFailure(t, workspace, "--branch", "feature/x"); !strings.Contains(output, "--branch") {
		t.Fatalf("root mode must explain --branch: %s", output)
	}
}

func TestInitPipelineUsesConfiguredBranchPrefix(t *testing.T) {
	if _, err := exec.LookPath("yq"); err != nil {
		t.Skip("yq not installed: config_get needs a YAML parser")
	}
	for _, tc := range []struct{ prefix, wantStart string }{
		{"Feature/", "Feature/"},
		{"wip", "wip-"},
	} {
		t.Run(tc.prefix, func(t *testing.T) {
			workspace := setupPipelineScriptWorkspace(t)
			initGitRepo(t, workspace, "service-a")
			writeBranchPrefix(t, workspace, tc.prefix)
			root := runInitPipeline(t, workspace, "prefixed")
			repoArgs := []string{"--git-root", "service-a", "--pipeline-dir", root.PipelineDir}

			sub := runInitPipeline(t, workspace, "prefixed", repoArgs...)
			if want := tc.wantStart + root.PipelineID + "-service-a"; sub.Branch != want {
				t.Fatalf("branch = %q, want %q", sub.Branch, want)
			}

			// 실행 중 설정이 바뀌어도 재진입은 만들어 둔 브랜치를 쓴다.
			writeBranchPrefix(t, workspace, "changed")
			if again := runInitPipeline(t, workspace, "prefixed", repoArgs...); again.Branch != sub.Branch {
				t.Fatalf("re-entry branch = %q, want stored %q", again.Branch, sub.Branch)
			}
		})
	}
}

// kislyuk yq는 문자열을 JSON으로(따옴표째) 출력한다. 따옴표는 git 참조에 허용되므로 그대로 두면 브랜치 이름에 들어간다.
func TestInitPipelineStripsQuotesFromBranchPrefix(t *testing.T) {
	binDir := t.TempDir()
	fake := "#!/bin/sh\ncase \"$1\" in .git.branch_prefix) echo '\"wip/\"' ;; *) echo null ;; esac\n"
	if err := os.WriteFile(filepath.Join(binDir, "yq"), []byte(fake), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	workspace := setupPipelineScriptWorkspace(t)
	initGitRepo(t, workspace, "service-a")
	root := runInitPipeline(t, workspace, "quoted")
	sub := runInitPipeline(t, workspace, "quoted", "--git-root", "service-a", "--pipeline-dir", root.PipelineDir)
	if want := "wip/" + root.PipelineID + "-service-a"; sub.Branch != want {
		t.Fatalf("branch = %q, want %q", sub.Branch, want)
	}
}

// 동료가 쓰는 이름을 받으면 push가 그 브랜치에 커밋을 얹는다. fetch하지 않은 브랜치도 막아야 한다.
func TestInitPipelineRefusesNamedBranchOnOrigin(t *testing.T) {
	workspace := setupPipelineScriptWorkspace(t)
	repo, _ := initGitRepo(t, workspace, "service-a")
	origin := filepath.Join(t.TempDir(), "origin.git")
	runPipelineGit(t, repo, "init", "--bare", origin)
	runPipelineGit(t, repo, "remote", "add", "origin", origin)
	runPipelineGit(t, repo, "push", "origin", "main:refs/heads/feature/colleague")
	root := runInitPipeline(t, workspace, "colleague")
	repoArgs := []string{"--git-root", "service-a", "--pipeline-dir", root.PipelineDir}

	output := runInitPipelineFailure(t, workspace, append(repoArgs, "--branch", "feature/colleague")...)
	if !strings.Contains(output, "branch already exists") {
		t.Fatalf("name on origin: %s", output)
	}
	assertBranchMissing(t, repo, "feature/colleague")

	if sub := runInitPipeline(t, workspace, "colleague", append(repoArgs, "--branch", "feature/mine")...); sub.Branch != "feature/mine" {
		t.Fatalf("free name = %q", sub.Branch)
	}
}

// 확인할 수 없으면 쓰지 않는다. run id가 든 기본 이름은 겹칠 수 없으므로 네트워크 없이 예전처럼 동작한다.
func TestInitPipelineRefusesNamedBranchWhenOriginUnreachable(t *testing.T) {
	workspace := setupPipelineScriptWorkspace(t)
	repo, _ := initGitRepo(t, workspace, "service-a")
	runPipelineGit(t, repo, "remote", "add", "origin", filepath.Join(t.TempDir(), "missing.git"))
	root := runInitPipeline(t, workspace, "offline")
	repoArgs := []string{"--git-root", "service-a", "--pipeline-dir", root.PipelineDir}

	runInitPipelineFailure(t, workspace, append(repoArgs, "--branch", "feature/offline")...)
	assertBranchMissing(t, repo, "feature/offline")

	sub := runInitPipeline(t, workspace, "offline", repoArgs...)
	if want := "task-" + root.PipelineID + "-service-a"; sub.Branch != want {
		t.Fatalf("default branch = %q, want %q", sub.Branch, want)
	}
}

// sub status는 root 등록보다 먼저 쓰인다. 등록만 남았는데 새 이름으로 시작하면 원래 브랜치가 run에서 떨어져 나간다.
func TestInitPipelineRefusesRegisteredRepoWithoutStatus(t *testing.T) {
	workspace := setupPipelineScriptWorkspace(t)
	repo, _ := initGitRepo(t, workspace, "service-a")
	root := runInitPipeline(t, workspace, "lost status")
	repoArgs := []string{"--git-root", "service-a", "--pipeline-dir", root.PipelineDir}
	sub := runInitPipeline(t, workspace, "lost status", append(repoArgs, "--branch", "feature/first")...)
	if err := os.Remove(filepath.Join(sub.PipelineDir, "status.json")); err != nil {
		t.Fatal(err)
	}

	output := runInitPipelineFailure(t, workspace, append(repoArgs, "--branch", "feature/second")...)
	if !strings.Contains(output, "branch ownership mismatch") {
		t.Fatalf("registered repo without status: %s", output)
	}
	assertBranchMissing(t, repo, "feature/second")
}

func TestInitPipelineRefusesProtectedBranchOnResume(t *testing.T) {
	workspace := setupPipelineScriptWorkspace(t)
	repo, _ := initGitRepo(t, workspace, "service-a")
	root := runInitPipeline(t, workspace, "resume main")
	repoArgs := []string{"--git-root", "service-a", "--pipeline-dir", root.PipelineDir}
	sub := runInitPipeline(t, workspace, "resume main", append(repoArgs, "--branch", "feature/x")...)

	writeJSONField(t, filepath.Join(sub.PipelineDir, "status.json"), "branch", "main")
	writeJSONField(t, filepath.Join(root.PipelineDir, "status.json"), "sub_pipelines", []any{})

	runInitPipelineFailure(t, workspace, repoArgs...)
	assertCurrentBranch(t, repo, "feature/x")
}

// 검증은 깨끗한 트리의 HEAD만 기록하므로 run도 깨끗하게 시작해야 한다. .pylon/과 에이전트 워크트리는 프로젝트 변경이 아니다.
func TestInitPipelineRejectsUntrackedFiles(t *testing.T) {
	workspace := setupPipelineScriptWorkspace(t)
	repo, _ := initGitRepo(t, workspace, "service-a")
	if err := os.WriteFile(filepath.Join(repo, ".pylon", "verify.yml"), []byte("build: {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "notes.txt"), []byte("draft\n"), 0644); err != nil {
		t.Fatal(err)
	}
	root := runInitPipeline(t, workspace, "dirty start")

	cmd := exec.Command(filepath.Join(layout.ScriptsDir(workspace), "init-pipeline.sh"),
		"dirty start", "--git-root", "service-a", "--pipeline-dir", root.PipelineDir)
	cmd.Dir = workspace
	output, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(output), "notes.txt") || strings.Contains(string(output), "verify.yml") {
		t.Fatalf("추적 안 된 notes.txt만 거부 사유여야 한다 (err=%v): %s", err, output)
	}

	if err := os.Remove(filepath.Join(repo, "notes.txt")); err != nil {
		t.Fatal(err)
	}
	runInitPipeline(t, workspace, "dirty start", "--git-root", "service-a", "--pipeline-dir", root.PipelineDir)
}

func TestCleanupPipelineRequiresMatchingTerminalCheckpoint(t *testing.T) {
	workspace := setupPipelineScriptWorkspace(t)
	repo, _ := initGitRepo(t, workspace, "service-a")
	root := runInitPipeline(t, workspace, "safe cleanup")
	sub := runInitPipeline(t, workspace, "safe cleanup", "--git-root", "service-a", "--pipeline-dir", root.PipelineDir)

	agentBranch := sub.Branch + "--worker"
	runPipelineGit(t, repo, "branch", agentBranch)

	cleanup := filepath.Join(layout.ScriptsDir(workspace), "cleanup-pipeline.sh")
	cmd := exec.Command(cleanup, sub.PipelineDir, "--terminal-phase", "completed")
	cmd.Dir = workspace
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("cleanup without checkpoint succeeded: %s", output)
	}
	if !strings.Contains(string(output), "terminal checkpoint") {
		t.Fatalf("cleanup error must explain checkpoint requirement: %s", output)
	}
	if _, err := os.Stat(sub.PipelineDir); err != nil {
		t.Fatalf("runtime removed before checkpoint: %v", err)
	}
	assertBranchExists(t, repo, agentBranch)

	if _, err := history.NewManager(workspace).Checkpoint(root.PipelineID, history.PhaseCompleted); err != nil {
		t.Fatal(err)
	}
	leftover := filepath.Join(sub.PipelineDir, "leftovers", "20260101T000000Z-1", "notes.md")
	if err := os.MkdirAll(filepath.Dir(leftover), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(leftover, []byte("user\n"), 0644); err != nil {
		t.Fatal(err)
	}
	readOnlyDir := filepath.Join(sub.PipelineDir, "evaluator-input")
	if err := os.MkdirAll(readOnlyDir, 0755); err != nil {
		t.Fatal(err)
	}
	readOnlyFile := filepath.Join(readOnlyDir, "request.json")
	if err := os.WriteFile(readOnlyFile, []byte("{}\n"), 0444); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(readOnlyDir, 0555); err != nil {
		t.Fatal(err)
	}

	cmd = exec.Command(cleanup, sub.PipelineDir, "--terminal-phase", "completed")
	cmd.Dir = workspace
	binDir := filepath.Join(workspace, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatal(err)
	}
	repoRoot, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	pylonBin := filepath.Join(binDir, "pylon")
	build := exec.Command("go", "build", "-o", pylonBin, "./cmd/pylon")
	build.Dir = repoRoot
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build pylon: %v\n%s", err, output)
	}
	cmd.Env = append(os.Environ(), "PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	if output, err = cmd.CombinedOutput(); err != nil {
		t.Fatalf("cleanup with checkpoint failed: %v\n%s", err, output)
	}
	if _, err := os.Stat(sub.PipelineDir); !os.IsNotExist(err) {
		t.Fatalf("runtime still exists after checkpoint cleanup: %v", err)
	}
	kept := filepath.Join(workspace, ".pylon", "runtime", "leftovers", root.PipelineID, "repos", filepath.Base(sub.PipelineDir),
		"leftovers", "20260101T000000Z-1", "notes.md")
	if data, err := os.ReadFile(kept); err != nil || string(data) != "user\n" {
		t.Fatalf("run 잔여물은 정리 뒤에도 보존해야 한다: %q %v\n%s", data, err, output)
	}
	assertBranchMissing(t, repo, agentBranch)
}

// 정리 스크립트는 run 브랜치의 워크트리를 강제로 지우지 않는다. 미커밋·추적 안 된 파일이 있는 워크트리는
// git이 거부하므로 남기고 kept로 보고한다 — 머지 후 남은 작업이 사라지지 않게.
func TestCleanupPipelineKeepsDirtyRunWorktrees(t *testing.T) {
	workspace := setupPipelineScriptWorkspace(t)
	repo, base := initGitRepo(t, workspace, "service-a")
	root := runInitPipeline(t, workspace, "keep dirty worktree")
	sub := runInitPipeline(t, workspace, "keep dirty worktree", "--git-root", "service-a", "--pipeline-dir", root.PipelineDir)
	runPipelineGit(t, repo, "checkout", base) // detached 메인 체크아웃도 스크립트를 멈추지 않아야 한다

	dirty := filepath.Join(workspace, "wt-dirty")
	runPipelineGit(t, repo, "worktree", "add", dirty, sub.Branch)
	if err := os.WriteFile(filepath.Join(dirty, "scratch.txt"), []byte("wip\n"), 0644); err != nil {
		t.Fatal(err)
	}
	clean := filepath.Join(workspace, "wt-clean")
	runPipelineGit(t, repo, "worktree", "add", "-b", sub.Branch+"--worker", clean, base)

	if _, err := history.NewManager(workspace).Checkpoint(root.PipelineID, history.PhaseCompleted); err != nil {
		t.Fatal(err)
	}
	output := runCleanupPipeline(t, workspace, sub.PipelineDir)
	if got, err := os.ReadFile(filepath.Join(dirty, "scratch.txt")); err != nil || string(got) != "wip\n" {
		t.Errorf("dirty worktree must be kept: %q, %v\n%s", got, err, output)
	}
	if !strings.Contains(string(output), "kept") || !strings.Contains(string(output), "wt-dirty") {
		t.Errorf("kept worktree must be reported: %s", output)
	}
	if _, err := os.Stat(clean); !os.IsNotExist(err) {
		t.Errorf("clean worktree must be removed: %v\n%s", err, output)
	}
}

// --branch로 정한 이름에는 run id가 없어 `<이름>--*`가 사용자 브랜치일 수 있다 — 정리가 지우면 커밋을 잃는다.
func TestCleanupPipelineKeepsUserBranchesBesideNamedBranch(t *testing.T) {
	workspace := setupPipelineScriptWorkspace(t)
	repo, base := initGitRepo(t, workspace, "service-a")
	root := runInitPipeline(t, workspace, "user branches")
	sub := runInitPipeline(t, workspace, "user branches", "--git-root", "service-a", "--pipeline-dir", root.PipelineDir, "--branch", "feature/login")

	userBranch := "feature/login--wip"
	userWorktree := filepath.Join(workspace, "wt-user")
	runPipelineGit(t, repo, "worktree", "add", "-b", userBranch, userWorktree, base)
	// 같은 이름의 워크트리도 사람이 다시 만든 브랜치일 수 있다. git worktree remove는 무시된 파일을 지우므로 건드리지 않는다.
	runPipelineGit(t, repo, "checkout", "main")
	sameName := filepath.Join(workspace, "wt-same-name")
	runPipelineGit(t, repo, "worktree", "add", sameName, "feature/login")
	if _, err := history.NewManager(workspace).Checkpoint(root.PipelineID, history.PhaseCompleted); err != nil {
		t.Fatal(err)
	}

	output := runCleanupPipeline(t, workspace, sub.PipelineDir)
	assertBranchExists(t, repo, userBranch)
	for _, kept := range []string{userWorktree, sameName} {
		if _, err := os.Stat(kept); err != nil {
			t.Errorf("worktree must be kept: %v\n%s", err, output)
		}
	}
	if !strings.Contains(string(output), "wt-same-name") {
		t.Errorf("kept worktree must be reported: %s", output)
	}
}

func TestMergeBranchesUsesOwningRepo(t *testing.T) {
	workspace := setupPipelineScriptWorkspace(t)
	repoA, _ := initGitRepo(t, workspace, "service-a")
	repoB, baseB := initGitRepo(t, workspace, "service-b")
	root := runInitPipeline(t, workspace, "merge owned repo")
	subA := runInitPipeline(t, workspace, "merge owned repo", "--git-root", "service-a", "--pipeline-dir", root.PipelineDir)
	runInitPipeline(t, workspace, "merge owned repo", "--git-root", "service-b", "--pipeline-dir", root.PipelineDir)

	runPipelineGit(t, repoA, "checkout", "-b", "worker-a")
	if err := os.WriteFile(filepath.Join(repoA, "change.txt"), []byte("service-a only\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runPipelineGit(t, repoA, "add", "change.txt")
	runPipelineGit(t, repoA, "commit", "-m", "service-a change")
	runPipelineGit(t, repoA, "checkout", subA.Branch)

	merge := filepath.Join(layout.ScriptsDir(workspace), "merge-branches.sh")
	cmd := exec.Command(merge, subA.Branch, "worker-a", "--git-root", "service-a")
	cmd.Dir = workspace
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("merge failed: %v\n%s", err, output)
	}
	if _, err := os.Stat(filepath.Join(repoA, "change.txt")); err != nil {
		t.Fatalf("change was not merged into owning repo: %v", err)
	}
	if got := strings.TrimSpace(runPipelineGit(t, repoB, "rev-parse", "HEAD")); got != baseB {
		t.Fatalf("non-owning repo moved from %s to %s", baseB, got)
	}
}

func TestMergeBranchesConflictReturnsFailureAndPreservesRuntime(t *testing.T) {
	workspace := setupPipelineScriptWorkspace(t)
	repo, _ := initGitRepo(t, workspace, "service-a")
	root := runInitPipeline(t, workspace, "conflicting merge")
	sub := runInitPipeline(t, workspace, "conflicting merge", "--git-root", "service-a", "--pipeline-dir", root.PipelineDir)

	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("task branch\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runPipelineGit(t, repo, "add", "README.md")
	runPipelineGit(t, repo, "commit", "-m", "task change")
	runPipelineGit(t, repo, "checkout", "main")
	runPipelineGit(t, repo, "checkout", "-b", "worker-conflict")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("worker branch\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runPipelineGit(t, repo, "add", "README.md")
	runPipelineGit(t, repo, "commit", "-m", "worker change")
	runPipelineGit(t, repo, "checkout", sub.Branch)

	merge := filepath.Join(layout.ScriptsDir(workspace), "merge-branches.sh")
	cmd := exec.Command(merge, sub.Branch, "worker-conflict", "--git-root", "service-a")
	cmd.Dir = workspace
	output, err := cmd.Output()
	if err == nil {
		t.Fatalf("conflicting merge succeeded: %s", output)
	}
	var result struct {
		OK     bool     `json:"ok"`
		Failed []string `json:"failed"`
	}
	if jsonErr := json.Unmarshal(output, &result); jsonErr != nil {
		t.Fatalf("merge failure output is not JSON: %v\n%s", jsonErr, output)
	}
	if result.OK || len(result.Failed) != 1 || result.Failed[0] != "worker-conflict" {
		t.Fatalf("unexpected merge failure result: %+v", result)
	}
	if _, statErr := os.Stat(root.PipelineDir); statErr != nil {
		t.Fatalf("root runtime was not preserved: %v", statErr)
	}
	if got := strings.TrimSpace(runPipelineGit(t, repo, "branch", "--show-current")); got != sub.Branch {
		t.Fatalf("merge failure left repo on %q, want %q", got, sub.Branch)
	}
	if _, statErr := os.Stat(filepath.Join(repo, ".git", "MERGE_HEAD")); !os.IsNotExist(statErr) {
		t.Fatalf("merge state was not aborted: %v", statErr)
	}
}

func setupPipelineScriptWorkspace(t *testing.T) string {
	t.Helper()
	workspace := t.TempDir()
	scriptsDir := layout.ScriptsDir(workspace)
	if err := os.MkdirAll(scriptsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, ".pylon", "config.yml"), []byte("version: \"0.1\"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"common.sh", "init-pipeline.sh", "cleanup-pipeline.sh", "merge-branches.sh"} {
		content, err := embeddedScripts.ReadFile("scripts/bash/" + name)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(scriptsDir, name), content, 0755); err != nil {
			t.Fatal(err)
		}
	}
	return workspace
}

func initGitRepo(t *testing.T, workspace, relative string) (string, string) {
	t.Helper()
	repo := filepath.Join(workspace, relative)
	if err := os.MkdirAll(filepath.Join(repo, ".pylon"), 0755); err != nil {
		t.Fatal(err)
	}
	runPipelineGit(t, repo, "init", "-b", "main")
	runPipelineGit(t, repo, "config", "user.email", "pylon-test@example.com")
	runPipelineGit(t, repo, "config", "user.name", "Pylon Test")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte(relative+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runPipelineGit(t, repo, "add", "README.md")
	runPipelineGit(t, repo, "commit", "-m", "initial")
	return repo, strings.TrimSpace(runPipelineGit(t, repo, "rev-parse", "HEAD"))
}

func runInitPipeline(t *testing.T, workspace, requirement string, args ...string) pipelineInitResult {
	t.Helper()
	commandArgs := append([]string{requirement}, args...)
	cmd := exec.Command(filepath.Join(layout.ScriptsDir(workspace), "init-pipeline.sh"), commandArgs...)
	cmd.Dir = workspace
	output, err := cmd.Output()
	if err != nil {
		stderr := ""
		if exitErr, ok := err.(*exec.ExitError); ok {
			stderr = string(exitErr.Stderr)
		}
		t.Fatalf("init-pipeline failed: %v\n%s%s", err, output, stderr)
	}
	var result pipelineInitResult
	if err := json.Unmarshal(output, &result); err != nil {
		t.Fatalf("init-pipeline output is not JSON: %v\n%s", err, output)
	}
	return result
}

func runInitPipelineFailure(t *testing.T, workspace string, args ...string) string {
	t.Helper()
	cmd := exec.Command(filepath.Join(layout.ScriptsDir(workspace), "init-pipeline.sh"), append([]string{"run"}, args...)...)
	cmd.Dir = workspace
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("init-pipeline accepted %q: %s", args, output)
	}
	return string(output)
}

func writeBranchPrefix(t *testing.T, workspace, prefix string) {
	t.Helper()
	config := "version: \"0.1\"\ngit:\n  branch_prefix: " + prefix + "\n"
	if err := os.WriteFile(filepath.Join(workspace, ".pylon", "config.yml"), []byte(config), 0644); err != nil {
		t.Fatal(err)
	}
}

func writeJSONField(t *testing.T, path, field string, value any) {
	t.Helper()
	status := readJSONMap(t, path)
	status[field] = value
	data, err := json.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
}

func runCleanupPipeline(t *testing.T, workspace, pipelineDir string) []byte {
	t.Helper()
	binDir := filepath.Join(workspace, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatal(err)
	}
	repoRoot, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	build := exec.Command("go", "build", "-o", filepath.Join(binDir, "pylon"), "./cmd/pylon")
	build.Dir = repoRoot
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build pylon: %v\n%s", err, output)
	}
	cmd := exec.Command(filepath.Join(layout.ScriptsDir(workspace), "cleanup-pipeline.sh"), pipelineDir, "--terminal-phase", "completed")
	cmd.Dir = workspace
	cmd.Env = append(os.Environ(), "PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("cleanup failed: %v\n%s", err, output)
	}
	return output
}

func readJSONMap(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if err := json.Unmarshal(data, &value); err != nil {
		t.Fatal(err)
	}
	return value
}

func runPipelineGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, output)
	}
	return string(output)
}

func assertCurrentBranch(t *testing.T, repo, want string) {
	t.Helper()
	if got := strings.TrimSpace(runPipelineGit(t, repo, "branch", "--show-current")); got != want {
		t.Fatalf("current branch in %s = %q, want %q", repo, got, want)
	}
}

func assertBranchExists(t *testing.T, repo, branch string) {
	t.Helper()
	cmd := exec.Command("git", "show-ref", "--verify", "--quiet", "refs/heads/"+branch)
	cmd.Dir = repo
	if err := cmd.Run(); err != nil {
		t.Fatalf("branch %q does not exist in %s", branch, repo)
	}
}

func assertBranchMissing(t *testing.T, repo, branch string) {
	t.Helper()
	cmd := exec.Command("git", "show-ref", "--verify", "--quiet", "refs/heads/"+branch)
	cmd.Dir = repo
	if err := cmd.Run(); err == nil {
		t.Fatalf("branch %q still exists in %s", branch, repo)
	}
}

// cleanup-pipeline.sh의 runtime 정리 분기를 직접 검증한다:
// 일치하는 terminal checkpoint가 있으면 삭제하고, 없으면 보존한다.
func TestCleanupPipelineScript_RuntimeBranches(t *testing.T) {
	for _, tool := range []string{"bash", "jq", "git"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s not available", tool)
		}
	}
	// 스크립트는 저장소 소스 트리에서 직접 가져와 임시 위치에 복사한다
	// (common.sh를 함께 두어 source가 동작하도록).
	repoRoot, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	scriptsSrc := filepath.Join(repoRoot, "internal", "cli", "scripts", "bash")
	tmp := t.TempDir()
	for _, name := range []string{"cleanup-pipeline.sh", "common.sh"} {
		data, err := os.ReadFile(filepath.Join(scriptsSrc, name))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(tmp, name), data, 0755); err != nil {
			t.Fatal(err)
		}
	}
	script := filepath.Join(tmp, "cleanup-pipeline.sh")
	binDir := filepath.Join(tmp, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		t.Fatal(err)
	}
	pylonBin := filepath.Join(binDir, "pylon")
	build := exec.Command("go", "build", "-o", pylonBin, "./cmd/pylon")
	build.Dir = repoRoot
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build pylon: %v\n%s", err, out)
	}

	// common.sh의 find_repo_root()가 .pylon/ 을 찾을 수 있도록 작업 디렉토리를 준비한다.
	pylonDir := filepath.Join(tmp, ".pylon")
	if err := os.MkdirAll(pylonDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pylonDir, "config.yml"), []byte("version: \"0.1\"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	runScript := func(pipelineDir string, terminalPhase ...string) ([]byte, error) {
		t.Helper()
		args := []string{script, pipelineDir}
		if len(terminalPhase) > 0 {
			args = append(args, "--terminal-phase", terminalPhase[0])
		}
		cmd := exec.Command("bash", args...)
		cmd.Dir = tmp
		cmd.Env = append(os.Environ(), "PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
		return cmd.CombinedOutput()
	}

	// 분기 1: terminal checkpoint 확인 → 디렉토리 삭제
	deleted := filepath.Join(pylonDir, "runtime", "runtime-deleted")
	if err := os.MkdirAll(deleted, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(deleted, "status.json"), []byte(`{"pipeline_id":"runtime-deleted","scope":"root"}`), 0644); err != nil {
		t.Fatal(err)
	}
	checkpoint := exec.Command(pylonBin, "history", "checkpoint", "--pipeline", "runtime-deleted", "--phase", "completed")
	checkpoint.Dir = tmp
	if out, err := checkpoint.CombinedOutput(); err != nil {
		t.Fatalf("create checkpoint: %v\n%s", err, out)
	}
	if out, err := runScript(deleted, "completed"); err != nil {
		t.Fatalf("script failed: %v\n%s", err, out)
	}
	if _, err := os.Stat(deleted); !os.IsNotExist(err) {
		t.Fatalf("runtime dir must be deleted after checkpoint: %v", err)
	}

	// 분기 2: 형식만 맞춘 가짜 manifest는 runtime 삭제를 승인하지 않는다.
	forged := filepath.Join(pylonDir, "runtime", "runtime-forged")
	if err := os.MkdirAll(forged, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(forged, "status.json"), []byte(`{"pipeline_id":"runtime-forged","scope":"root"}`), 0644); err != nil {
		t.Fatal(err)
	}
	forgedManifestDir := filepath.Join(pylonDir, "history", "pipelines", "runtime-forged", "completed")
	if err := os.MkdirAll(forgedManifestDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(forgedManifestDir, "manifest.json"), []byte(`{"pipeline_id":"runtime-forged","phase":"completed"}`), 0644); err != nil {
		t.Fatal(err)
	}
	if out, err := runScript(forged, "completed"); err == nil {
		t.Fatalf("forged checkpoint must fail cleanup:\n%s", out)
	}
	if _, err := os.Stat(forged); err != nil {
		t.Fatalf("runtime must survive forged checkpoint: %v", err)
	}

	// 분기 3: checkpoint 미지정 → preserved 마킹 후 보존
	kept := filepath.Join(tmp, "runtime-kept")
	if err := os.MkdirAll(kept, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(kept, "status.json"), []byte(`{"pipeline_id":"runtime-kept","scope":"root"}`), 0644); err != nil {
		t.Fatal(err)
	}
	if out, err := runScript(kept); err != nil {
		t.Fatalf("script failed: %v\n%s", err, out)
	}
	data, err := os.ReadFile(filepath.Join(kept, "status.json"))
	if err != nil {
		t.Fatalf("status.json must exist when runtime is kept: %v", err)
	}
	var sj struct {
		Cleanup struct {
			Status string `json:"status"`
			Reason string `json:"reason"`
		} `json:"cleanup"`
	}
	if err := json.Unmarshal(data, &sj); err != nil {
		t.Fatal(err)
	}
	if sj.Cleanup.Status != "preserved" || sj.Cleanup.Reason != "terminal_checkpoint_required" {
		t.Fatalf("cleanup = %#v, want preserved checkpoint requirement", sj.Cleanup)
	}

	// 분기 3: 상태가 없는 임의 경로는 정리 대상으로 인정하지 않는다.
	if out, err := runScript(filepath.Join(tmp, "no-such-dir")); err == nil {
		t.Fatalf("script accepted a directory without pipeline state: %s", out)
	}
}
