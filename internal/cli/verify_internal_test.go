package cli

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kyago/pylon/internal/config"
	"github.com/kyago/pylon/internal/criteria"
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

func TestInternalVerify_UsesFrozenSnapshotAndRejectsLiveConfigChange(t *testing.T) {
	workDir, runDir, configPath, snapshotPath, manifestPath := createCriteriaFixture(t)
	markerPath := filepath.Join(workDir, "marker.txt")
	initialConfig := "build:\n  command: \"printf snapshot > " + markerPath + "\"\n"
	if err := os.WriteFile(configPath, []byte(initialConfig), 0644); err != nil {
		t.Fatal(err)
	}
	createCriteriaSnapshot(t, workDir, configPath, snapshotPath, manifestPath)
	changedConfig := "build:\n  command: \"printf live > " + markerPath + "\"\n"
	if err := os.WriteFile(configPath, []byte(changedConfig), 0644); err != nil {
		t.Fatal(err)
	}

	outputPath := filepath.Join(runDir, "verification.json")
	cmd := newInternalVerifyCmd()
	var output strings.Builder
	cmd.SetOut(&output)
	cmd.SetErr(&output)
	cmd.SetArgs([]string{
		"--workdir", workDir,
		"--snapshot", snapshotPath,
		"--manifest", manifestPath,
		"--live-config", configPath,
		"--output", outputPath,
	})
	if err := cmd.Execute(); err == nil {
		t.Fatal("live config change must fail closed")
	}
	marker, err := os.ReadFile(markerPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(marker) != "snapshot" {
		t.Fatalf("executed command = %q, want frozen snapshot command", marker)
	}
	var result verificationResult
	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	if result.OK || !result.IntegrityOK || !result.SourceChanged || result.CriteriaDigest == "" {
		t.Fatalf("unexpected snapshot result: %+v", result)
	}
	events, err := os.ReadFile(filepath.Join(runDir, "criteria-events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(events), "criteria_source_changed") {
		t.Fatalf("missing criteria source change event: %s", events)
	}
}

func TestInternalVerify_LoadsHeldOutOnlyFromEvaluatorSnapshot(t *testing.T) {
	workDir, runDir, configPath, snapshotPath, manifestPath := createCriteriaFixture(t)
	publicMarker := filepath.Join(workDir, "public.txt")
	heldOutMarker := filepath.Join(workDir, "held-out.txt")
	configData := "build:\n  command: \"printf public > " + publicMarker + "\"\nheld_out:\n  - name: secret\n    command: \"printf held-out > " + heldOutMarker + "\"\n"
	if err := os.WriteFile(configPath, []byte(configData), 0644); err != nil {
		t.Fatal(err)
	}
	createCriteriaSnapshot(t, workDir, configPath, snapshotPath, manifestPath)
	heldOutPath := filepath.Join(runDir, "evaluator-only", "held-out.json")

	publicData, err := os.ReadFile(snapshotPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(publicData), "held-out.txt") || strings.Contains(string(publicData), "secret") {
		t.Fatalf("public criteria exposed held-out command: %s", publicData)
	}

	missingHeldOut := newInternalVerifyCmd()
	missingHeldOut.SetOut(&strings.Builder{})
	missingHeldOut.SetErr(&strings.Builder{})
	missingHeldOut.SetArgs([]string{
		"--workdir", workDir,
		"--snapshot", snapshotPath,
		"--manifest", manifestPath,
		"--live-config", configPath,
	})
	if err := missingHeldOut.Execute(); err == nil {
		t.Fatal("manifest-bound held-out snapshot was silently skipped")
	}

	cmd := newInternalVerifyCmd()
	cmd.SetOut(&strings.Builder{})
	cmd.SetErr(&strings.Builder{})
	cmd.SetArgs([]string{
		"--workdir", workDir,
		"--snapshot", snapshotPath,
		"--held-out", heldOutPath,
		"--manifest", manifestPath,
		"--live-config", configPath,
	})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{publicMarker, heldOutMarker} {
		if _, err := os.Stat(marker); err != nil {
			t.Fatalf("verification marker missing %s: %v", marker, err)
		}
	}
}

func TestInternalVerify_RejectsTamperedSnapshotBeforeExecution(t *testing.T) {
	workDir, runDir, configPath, snapshotPath, manifestPath := createCriteriaFixture(t)
	markerPath := filepath.Join(workDir, "marker.txt")
	configData := "build:\n  command: \"printf ran > " + markerPath + "\"\n"
	if err := os.WriteFile(configPath, []byte(configData), 0644); err != nil {
		t.Fatal(err)
	}
	createCriteriaSnapshot(t, workDir, configPath, snapshotPath, manifestPath)

	data, err := os.ReadFile(snapshotPath)
	if err != nil {
		t.Fatal(err)
	}
	var snapshot map[string]any
	if err := json.Unmarshal(data, &snapshot); err != nil {
		t.Fatal(err)
	}
	steps := snapshot["verification"].([]any)
	steps[0].(map[string]any)["command"] = "printf tampered > " + markerPath
	tampered, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(snapshotPath, append(tampered, '\n'), 0644); err != nil {
		t.Fatal(err)
	}

	outputPath := filepath.Join(runDir, "verification.json")
	cmd := newInternalVerifyCmd()
	cmd.SetOut(&strings.Builder{})
	cmd.SetErr(&strings.Builder{})
	cmd.SetArgs([]string{
		"--workdir", workDir,
		"--snapshot", snapshotPath,
		"--manifest", manifestPath,
		"--live-config", configPath,
		"--output", outputPath,
	})
	if err := cmd.Execute(); err == nil {
		t.Fatal("tampered criteria snapshot must fail closed")
	}
	if _, err := os.Stat(markerPath); !os.IsNotExist(err) {
		t.Fatalf("tampered verification command executed: %v", err)
	}
	resultData, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	var result verificationResult
	if err := json.Unmarshal(resultData, &result); err != nil {
		t.Fatal(err)
	}
	if result.OK || result.IntegrityOK || !strings.Contains(result.Reason, "integrity") {
		t.Fatalf("unexpected tamper result: %+v", result)
	}
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
	criteriaCmd, _, err := cmd.Find([]string{"criteria", "snapshot"})
	if err != nil || criteriaCmd == nil || criteriaCmd.Name() != "snapshot" {
		t.Fatalf("criteria snapshot command not registered: cmd=%v err=%v", criteriaCmd, err)
	}
	evaluatorCmd, _, err := cmd.Find([]string{"evaluator", "prepare"})
	if err != nil || evaluatorCmd == nil || evaluatorCmd.Name() != "prepare" {
		t.Fatalf("evaluator prepare command not registered: cmd=%v err=%v", evaluatorCmd, err)
	}
	recordCmd, _, err := cmd.Find([]string{"evaluator", "record"})
	if err != nil || recordCmd == nil || recordCmd.Name() != "record" {
		t.Fatalf("evaluator record command not registered: cmd=%v err=%v", recordCmd, err)
	}
}

func TestBuiltInEvaluatorAndInvestigatorPolicies(t *testing.T) {
	decision := []string{"verifier", "critic", "code-reviewer", "security-reviewer", "fact-checker", "content-reviewer"}
	for _, name := range decision {
		data, err := embeddedAgents.ReadFile("agents/" + name + ".md")
		if err != nil {
			t.Fatal(err)
		}
		agent, err := config.ParseAgentData(data)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		if agent.EvaluationRole != config.EvaluationRoleDecision || agent.AccessMode != config.AccessModeReadOnly || agent.InputPolicy != config.InputPolicyIsolatedEvidence {
			t.Fatalf("decision agent %s policy = %+v", name, agent)
		}
		if !containsString(agent.DisallowedTools, "Bash") || containsString(agent.Tools, "Bash") {
			t.Fatalf("decision agent %s tools=%v disallowed=%v", name, agent.Tools, agent.DisallowedTools)
		}
	}
	for _, name := range []string{"explorer", "tracer", "analyst", "researcher", "doc-specialist"} {
		data, err := embeddedAgents.ReadFile("agents/" + name + ".md")
		if err != nil {
			t.Fatal(err)
		}
		agent, err := config.ParseAgentData(data)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		if agent.EvaluationRole != config.EvaluationRoleInvestigation || agent.CanIssueFinalVerdict() {
			t.Fatalf("investigation agent %s policy = %+v", name, agent)
		}
	}
}

func TestRunVerificationScript_DelegatesFromResolvedGitRoot(t *testing.T) {
	root := t.TempDir()
	pylonDir := filepath.Join(root, ".pylon")
	scriptsDir := filepath.Join(pylonDir, "scripts", "bash")
	projectDir := filepath.Join(root, "project")
	if err := os.MkdirAll(scriptsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(projectDir, ".pylon"), 0o755); err != nil {
		t.Fatal(err)
	}
	verifyPath := filepath.Join(projectDir, ".pylon", "verify.yml")
	if err := os.WriteFile(verifyPath, []byte("build:\n  command: true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pylonDir, "config.yml"), []byte("version: \"0.1\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"common.sh", "run-verification.sh"} {
		data, err := os.ReadFile(filepath.Join("scripts", "bash", name))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(scriptsDir, name), data, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	binDir := filepath.Join(root, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	capturePath := filepath.Join(root, "capture.txt")
	fakePylon := `#!/bin/bash
printf '%s\n' "$PWD" > "$CAPTURE_PATH"
printf '%s\n' "$@" >> "$CAPTURE_PATH"
while [[ $# -gt 0 ]]; do
  if [[ "$1" == "--output" ]]; then
    shift
    printf '{"ok":true,"checks":[],"timestamp":"2026-07-23T00:00:00Z"}\n' > "$1"
    break
  fi
  shift
done
printf '{"ok":true,"checks":[],"timestamp":"2026-07-23T00:00:00Z"}\n'
`
	if err := os.WriteFile(filepath.Join(binDir, "pylon"), []byte(fakePylon), 0o755); err != nil {
		t.Fatal(err)
	}
	pipelineDir := filepath.Join(pylonDir, "runtime", "test")
	if err := os.MkdirAll(pipelineDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pipelineDir, "criteria.json"), []byte("{}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pipelineDir, "status.json"), []byte("{}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	heldOutPath := filepath.Join(pipelineDir, "evaluator-only", "held-out.json")
	if err := os.MkdirAll(filepath.Dir(heldOutPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(heldOutPath, []byte("{}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pipelineDir, "status.json"), []byte(`{"held_out":{"path":"evaluator-only/held-out.json"}}`), 0644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(filepath.Join(scriptsDir, "run-verification.sh"), pipelineDir, "--git-root", "project")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "PATH="+binDir+":"+os.Getenv("PATH"), "CAPTURE_PATH="+capturePath)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("script failed: %v\n%s", err, output)
	}
	capture, err := os.ReadFile(capturePath)
	if err != nil {
		t.Fatal(err)
	}
	got := string(capture)
	resolvedProjectDir, err := filepath.EvalSymlinks(projectDir)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got, resolvedProjectDir+"\ninternal\nverify\n") {
		t.Fatalf("unexpected delegation:\n%s", got)
	}
	if !strings.Contains(got, "--snapshot\n"+filepath.Join(pipelineDir, "criteria.json")) {
		t.Fatalf("criteria snapshot not forwarded:\n%s", got)
	}
	if !strings.Contains(got, "--manifest\n"+filepath.Join(pipelineDir, "status.json")) {
		t.Fatalf("criteria manifest not forwarded:\n%s", got)
	}
	if !strings.Contains(got, "--held-out\n"+heldOutPath) {
		t.Fatalf("held-out snapshot not forwarded:\n%s", got)
	}
	// live 경로는 재계산해 넘기지 않는다 — snapshot의 Source.Path(절대경로)가 기준이다.
	if strings.Contains(got, "--live-config") {
		t.Fatalf("live config must not be recomputed by the script:\n%s", got)
	}

	// verify.yml도 go.mod도 없는 GIT_ROOT는 잘못된 해석으로 보고 실행 전에 실패해야 한다.
	if err := os.Remove(verifyPath); err != nil {
		t.Fatal(err)
	}
	cmd = exec.Command(filepath.Join(scriptsDir, "run-verification.sh"), pipelineDir, "--git-root", "project")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "PATH="+binDir+":"+os.Getenv("PATH"), "CAPTURE_PATH="+capturePath)
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected guard failure without verify.yml/go.mod:\n%s", output)
	}
	if !strings.Contains(string(output), "verify.yml") {
		t.Fatalf("guard message must mention verify.yml:\n%s", output)
	}
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func createCriteriaFixture(t *testing.T) (workDir, runDir, configPath, snapshotPath, manifestPath string) {
	t.Helper()
	root := t.TempDir()
	workDir = filepath.Join(root, "repo")
	runDir = filepath.Join(root, "runtime", "run", "repos", "repo")
	if err := os.MkdirAll(filepath.Join(workDir, ".pylon"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(runDir, 0755); err != nil {
		t.Fatal(err)
	}
	configPath = filepath.Join(workDir, ".pylon", "verify.yml")
	snapshotPath = filepath.Join(runDir, "criteria.json")
	manifestPath = filepath.Join(runDir, "status.json")
	if err := os.WriteFile(manifestPath, []byte(`{"repo_id":"repo","base_revision":"base"}`), 0644); err != nil {
		t.Fatal(err)
	}
	return workDir, runDir, configPath, snapshotPath, manifestPath
}

func createCriteriaSnapshot(t *testing.T, workDir, configPath, snapshotPath, manifestPath string) {
	t.Helper()
	acceptancePath := filepath.Join(filepath.Dir(manifestPath), "acceptance-input.json")
	if err := os.WriteFile(acceptancePath, []byte(`{"criteria":[{"id":"AC-1","description":"verification succeeds"}]}`), 0644); err != nil {
		t.Fatal(err)
	}
	cmd := newInternalCriteriaSnapshotCmd()
	var output strings.Builder
	cmd.SetOut(&output)
	cmd.SetErr(&output)
	cmd.SetArgs([]string{
		"--run-id", "run",
		"--repo-id", "repo",
		"--base-revision", "base",
		"--workdir", workDir,
		"--config", configPath,
		"--acceptance", acceptancePath,
		"--output", snapshotPath,
		"--held-out-output", filepath.Join(filepath.Dir(snapshotPath), "evaluator-only", "held-out.json"),
		"--manifest", manifestPath,
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("criteria snapshot failed: %v (%s)", err, output.String())
	}
}

func TestResolveLiveConfigPath(t *testing.T) {
	workDir := "/work/repo"
	abs := filepath.Join(workDir, ".pylon", "verify.yml")
	// 명시 지정이 항상 우선한다.
	if got := resolveLiveConfigPath(workDir, "/explicit/verify.yml", criteria.Source{Path: "rel/.pylon/verify.yml"}); got != "/explicit/verify.yml" {
		t.Fatalf("explicit path ignored: %q", got)
	}
	// 구버전 snapshot의 상대 Source.Path는 workdir 표준 위치로 대체된다.
	if got := resolveLiveConfigPath(workDir, "", criteria.Source{Path: "repo/.pylon/verify.yml"}); got != abs {
		t.Fatalf("legacy relative path not remapped: %q", got)
	}
	// 절대 Source.Path는 LiveSourceChanged의 fallback에 맡긴다.
	if got := resolveLiveConfigPath(workDir, "", criteria.Source{Path: abs}); got != "" {
		t.Fatalf("absolute source must use fallback: %q", got)
	}
}
