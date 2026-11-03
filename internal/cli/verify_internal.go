package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/kyago/pylon/internal/config"
	"github.com/kyago/pylon/internal/fsutil"
)

type verificationCheck struct {
	Name       string `json:"name"`
	Kind       string `json:"kind,omitempty"`
	Command    string `json:"command,omitempty"`
	OK         bool   `json:"ok"`
	ExitCode   int    `json:"exit_code"`
	TimedOut   bool   `json:"timed_out,omitempty"`
	DurationMS int64  `json:"duration_ms,omitempty"`
	Output     string `json:"output"`
}

type verificationResult struct {
	OK           bool                `json:"ok"`
	Checks       []verificationCheck `json:"checks"`
	Skipped      bool                `json:"skipped,omitempty"`
	Reason       string              `json:"reason,omitempty"`
	Tree         string              `json:"tree,omitempty"`
	TreeReason   string              `json:"tree_reason,omitempty"`
	Artifacts    []string            `json:"artifacts,omitempty"`
	ArtifactsDir string              `json:"artifacts_dir,omitempty"`
	Timestamp    string              `json:"timestamp"`
}

func newInternalCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "internal", Short: "Helper commands", Hidden: true}
	cmd.AddCommand(newInternalVerifyCmd())
	cmd.AddCommand(newInternalMoveUntrackedCmd())
	return cmd
}

func newInternalVerifyCmd() *cobra.Command {
	var workDir, configPath, outputPath string
	cmd := &cobra.Command{
		Use:          "verify",
		Short:        "Run project verification commands",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			steps, skipped, err := loadVerificationSteps(workDir, configPath)
			if err != nil {
				return err
			}

			// Only committed code can be matched to a PR, so verification vouches for HEAD's tree and only when the
			// checks ran against exactly that: nothing uncommitted before, during or after.
			var before worktreeState
			beforeReason := ""
			if len(steps) > 0 {
				before = readWorktree(workDir)
				beforeReason = before.dirtyReason(before.untracked)
			}
			executed, err := executeVerification(workDir, steps, time.Now)
			if err != nil {
				return err
			}
			if len(steps) > 0 && beforeReason == "" {
				// The run started clean, so every untracked file now came from the steps, which rebuild it from HEAD.
				after := readWorktree(workDir)
				remaining := after.untracked
				if len(after.untracked) > 0 {
					executed.ArtifactsDir = verificationArtifactsDir(after.top, outputPath)
					executed.Artifacts, remaining = moveUntracked(after.top, after.untracked, executed.ArtifactsDir, true)
					if len(executed.Artifacts) == 0 {
						executed.ArtifactsDir = ""
					}
				}
				if executed.OK {
					if reason := after.dirtyReason(remaining); reason != "" {
						executed.TreeReason = "검증 단계가 작업 트리를 바꿨습니다 — " + reason
					} else if after.head != before.head {
						executed.TreeReason = "검증 중 HEAD가 바뀌었습니다"
					} else {
						executed.Tree = before.tree
					}
				}
			} else if executed.OK && len(steps) > 0 {
				executed.TreeReason = beforeReason
			}
			executed.Skipped = skipped
			if len(steps) == 0 {
				executed.OK = false
				if skipped {
					executed.Reason = fmt.Sprintf("검증 설정을 찾을 수 없습니다: %s 를 작성하거나 --config로 올바른 경로를 지정하세요 (workdir: %s)", configPath, workDir)
				} else {
					executed.Reason = fmt.Sprintf("verify.yml에 실행 가능한 검증 명령이 없습니다: %s", configPath)
				}
			}
			if !executed.OK {
				executed.Tree = "" // a tree vouches only for a passing result
			}
			return emitVerificationResult(cmd, outputPath, executed)
		},
	}
	cmd.Flags().StringVar(&workDir, "workdir", "", "project working directory")
	cmd.Flags().StringVar(&configPath, "config", "", "verify.yml path (default: <workdir>/.pylon/verify.yml)")
	cmd.Flags().StringVar(&outputPath, "output", "", "verification result path")
	_ = cmd.MarkFlagRequired("workdir")
	return cmd
}

// loadVerificationSteps reads verify.yml, falls back to the Go defaults when a go.mod
// exists, and reports skipped when neither is there.
func loadVerificationSteps(workDir, configPath string) ([]config.NamedVerifyStep, bool, error) {
	if configPath == "" {
		configPath = filepath.Join(workDir, ".pylon", "verify.yml")
	}
	if _, err := os.Stat(configPath); err == nil {
		verifyConfig, err := config.LoadVerifyConfig(configPath)
		if err != nil {
			return nil, false, err
		}
		return append(verifyConfig.OrderedSteps(), verifyConfig.OrderedHeldOutSteps()...), false, nil
	} else if !os.IsNotExist(err) {
		return nil, false, fmt.Errorf("failed to inspect verify config: %w", err)
	}
	if _, err := os.Stat(filepath.Join(workDir, "go.mod")); err == nil {
		return []config.NamedVerifyStep{
			{Name: "build", Kind: config.VerifyKindDeterministic, Command: "go build ./...", Timeout: "5m"},
			{Name: "vet", Kind: config.VerifyKindDeterministic, Command: "go vet ./...", Timeout: "5m"},
			{Name: "test", Kind: config.VerifyKindDeterministic, Command: "go test ./...", Timeout: "10m"},
		}, false, nil
	} else if !os.IsNotExist(err) {
		return nil, false, fmt.Errorf("failed to inspect Go project: %w", err)
	}
	return nil, true, nil
}

func executeVerification(workDir string, steps []config.NamedVerifyStep, now func() time.Time) (verificationResult, error) {
	result := verificationResult{
		OK:        true,
		Checks:    make([]verificationCheck, 0, len(steps)),
		Timestamp: now().UTC().Format(time.RFC3339),
	}
	for _, step := range steps {
		timeout, err := time.ParseDuration(step.Timeout)
		if err != nil {
			return verificationResult{}, fmt.Errorf("invalid timeout for %s: %w", step.Name, err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		command := exec.CommandContext(ctx, "bash", "-lc", step.Command)
		command.Dir = workDir
		command.WaitDelay = 2 * time.Second
		startedAt := time.Now()
		output, runErr := command.CombinedOutput()
		duration := time.Since(startedAt)
		timedOut := errors.Is(ctx.Err(), context.DeadlineExceeded)
		cancel()

		outputText := strings.TrimSpace(string(output))
		if timedOut {
			if outputText != "" {
				outputText += "\n"
			}
			outputText += "timed out after " + timeout.String()
		}
		exitCode := 0
		if runErr != nil {
			exitCode = 1
			var exitErr *exec.ExitError
			if errors.As(runErr, &exitErr) {
				exitCode = exitErr.ExitCode()
			}
		}
		kind := step.Kind
		if kind == "" {
			kind = config.VerifyKindDeterministic
		}
		check := verificationCheck{
			Name:       step.Name,
			Kind:       kind,
			Command:    step.Command,
			OK:         runErr == nil && !timedOut,
			ExitCode:   exitCode,
			TimedOut:   timedOut,
			DurationMS: duration.Milliseconds(),
			Output:     outputText,
		}
		if !check.OK {
			result.OK = false
		}
		result.Checks = append(result.Checks, check)
	}
	return result, nil
}

func emitVerificationResult(cmd *cobra.Command, outputPath string, result verificationResult) error {
	if err := writeVerificationResult(cmd, outputPath, result); err != nil {
		return err
	}
	if !result.OK {
		if result.Reason != "" {
			return errors.New(result.Reason)
		}
		return errors.New("verification failed")
	}
	return nil
}

func writeVerificationResult(cmd *cobra.Command, outputPath string, result verificationResult) error {
	data, err := json.Marshal(result)
	if err != nil {
		return err
	}
	if outputPath != "" {
		if err := fsutil.WriteFileAtomic(outputPath, append(data, '\n'), 0644); err != nil {
			return fmt.Errorf("failed to write verification result: %w", err)
		}
	}
	fmt.Fprintln(cmd.OutOrStdout(), string(data))
	return nil
}
