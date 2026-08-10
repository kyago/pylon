package cli

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
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

func TestCleanupPipelineRequiresMatchingTerminalCheckpoint(t *testing.T) {
	workspace := setupPipelineScriptWorkspace(t)
	repo, _ := initGitRepo(t, workspace, "service-a")
	root := runInitPipeline(t, workspace, "safe cleanup")
	sub := runInitPipeline(t, workspace, "safe cleanup", "--git-root", "service-a", "--pipeline-dir", root.PipelineDir)

	agentBranch := sub.Branch + "--worker"
	runPipelineGit(t, repo, "branch", agentBranch)

	cleanup := filepath.Join(workspace, ".pylon", "scripts", "bash", "cleanup-pipeline.sh")
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

	manifestDir := filepath.Join(workspace, ".pylon", "history", "pipelines", root.PipelineID, "completed")
	if err := os.MkdirAll(manifestDir, 0755); err != nil {
		t.Fatal(err)
	}
	manifest := `{"pipeline_id":"` + root.PipelineID + `","phase":"completed"}`
	if err := os.WriteFile(filepath.Join(manifestDir, "manifest.json"), []byte(manifest), 0644); err != nil {
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
	if output, err = cmd.CombinedOutput(); err != nil {
		t.Fatalf("cleanup with checkpoint failed: %v\n%s", err, output)
	}
	if _, err := os.Stat(sub.PipelineDir); !os.IsNotExist(err) {
		t.Fatalf("runtime still exists after checkpoint cleanup: %v", err)
	}
	assertBranchMissing(t, repo, agentBranch)
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

	merge := filepath.Join(workspace, ".pylon", "scripts", "bash", "merge-branches.sh")
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

	merge := filepath.Join(workspace, ".pylon", "scripts", "bash", "merge-branches.sh")
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
	scriptsDir := filepath.Join(workspace, ".pylon", "scripts", "bash")
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
	cmd := exec.Command(filepath.Join(workspace, ".pylon", "scripts", "bash", "init-pipeline.sh"), commandArgs...)
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
