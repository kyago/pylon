package cli

import (
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
	if !strings.Contains(got, "--config\n"+filepath.Join(resolvedProjectDir, ".pylon", "verify.yml")) {
		t.Fatalf("verify config not forwarded:\n%s", got)
	}
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
