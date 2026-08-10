package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/kyago/pylon/internal/config"
	"github.com/kyago/pylon/internal/criteria"
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
	OK             bool                `json:"ok"`
	Checks         []verificationCheck `json:"checks"`
	Skipped        bool                `json:"skipped,omitempty"`
	Reason         string              `json:"reason,omitempty"`
	CriteriaDigest string              `json:"criteria_digest,omitempty"`
	IntegrityOK    bool                `json:"integrity_ok,omitempty"`
	SourceChanged  bool                `json:"source_changed,omitempty"`
	Timestamp      string              `json:"timestamp"`
}

func newInternalCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "internal", Hidden: true}
	cmd.AddCommand(newInternalVerifyCmd())
	cmd.AddCommand(newInternalCriteriaCmd())
	cmd.AddCommand(newInternalEvaluatorCmd())
	cmd.AddCommand(newInternalStateCmd())
	cmd.AddCommand(newInternalTrajectoryCmd())
	cmd.AddCommand(newInternalCorpusCmd())
	cmd.AddCommand(newInternalCuratorCmd())
	return cmd
}

func newInternalVerifyCmd() *cobra.Command {
	var workDir, configPath, outputPath string
	var snapshotPath, heldOutPath, manifestPath, liveConfigPath string
	cmd := &cobra.Command{
		Use:          "verify",
		Short:        "Run project verification commands",
		Hidden:       true,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			var steps []config.NamedVerifyStep
			var skipped bool
			result := verificationResult{Checks: []verificationCheck{}, Timestamp: time.Now().UTC().Format(time.RFC3339)}

			if snapshotPath != "" {
				snapshot, err := criteria.Load(snapshotPath)
				if err != nil {
					return failVerification(cmd, outputPath, manifestPath, result, "criteria_integrity_failed", err)
				}
				result.CriteriaDigest = snapshot.Digest
				if manifestPath == "" {
					return failVerification(cmd, outputPath, manifestPath, result, "criteria_integrity_failed", errors.New("--manifest is required with --snapshot"))
				}
				if err := criteria.ValidateManifest(manifestPath, snapshotPath, snapshot); err != nil {
					return failVerification(cmd, outputPath, manifestPath, result, "criteria_integrity_failed", err)
				}
				result.IntegrityOK = true
				steps = append(steps, snapshot.Verification...)
				expectedHeldOutPath, hasHeldOut, err := criteria.ManifestHeldOutPath(manifestPath)
				if err != nil {
					return failVerification(cmd, outputPath, manifestPath, result, "criteria_integrity_failed", err)
				}
				if hasHeldOut && heldOutPath == "" {
					return failVerification(cmd, outputPath, manifestPath, result, "criteria_integrity_failed", errors.New("manifest requires --held-out evaluator snapshot"))
				}
				if heldOutPath != "" {
					if !hasHeldOut {
						return failVerification(cmd, outputPath, manifestPath, result, "criteria_integrity_failed", errors.New("manifest has no held-out reference"))
					}
					resolvedHeldOutPath, resolveErr := filepath.Abs(heldOutPath)
					if resolveErr != nil {
						return failVerification(cmd, outputPath, manifestPath, result, "criteria_integrity_failed", resolveErr)
					}
					expectedHeldOutPath, resolveErr = filepath.Abs(expectedHeldOutPath)
					if resolveErr != nil || resolvedHeldOutPath != expectedHeldOutPath {
						return failVerification(cmd, outputPath, manifestPath, result, "criteria_integrity_failed", errors.New("held-out path does not match manifest"))
					}
					heldOut, loadErr := criteria.LoadHeldOut(heldOutPath)
					if loadErr != nil {
						return failVerification(cmd, outputPath, manifestPath, result, "criteria_integrity_failed", loadErr)
					}
					if heldOut.CriteriaDigest != snapshot.Digest {
						return failVerification(cmd, outputPath, manifestPath, result, "criteria_integrity_failed", errors.New("held-out criteria digest mismatch"))
					}
					if err := criteria.ValidateHeldOutManifest(manifestPath, heldOutPath, heldOut); err != nil {
						return failVerification(cmd, outputPath, manifestPath, result, "criteria_integrity_failed", err)
					}
					steps = append(steps, heldOut.Verification...)
				}
				if len(steps) == 0 {
					return failVerification(cmd, outputPath, manifestPath, result, "criteria_integrity_failed", errors.New("criteria contains no verification commands"))
				}
				changed, actualDigest, err := criteria.LiveSourceChanged(snapshot, liveConfigPath)
				if err != nil {
					return failVerification(cmd, outputPath, manifestPath, result, "criteria_source_check_failed", err)
				}
				result.SourceChanged = changed
				if changed {
					if err := recordCriteriaEvent(manifestPath, criteria.Event{
						Type:     "criteria_source_changed",
						Expected: snapshot.Source.Digest,
						Actual:   actualDigest,
						Message:  "live verification config changed after criteria snapshot",
					}); err != nil {
						return failVerification(cmd, outputPath, manifestPath, result, "criteria_event_write_failed", err)
					}
				}
			} else {
				var err error
				steps, skipped, err = loadVerificationSteps(workDir, configPath)
				if err != nil {
					return err
				}
			}

			executed, err := executeVerification(workDir, steps, time.Now)
			if err != nil {
				return err
			}
			executed.Skipped = skipped
			executed.CriteriaDigest = result.CriteriaDigest
			executed.IntegrityOK = result.IntegrityOK
			executed.SourceChanged = result.SourceChanged
			if len(steps) == 0 {
				executed.OK = false
				if skipped {
					executed.Reason = fmt.Sprintf("검증 설정을 찾을 수 없습니다: %s 를 작성하거나 --config로 올바른 경로를 지정하세요 (workdir: %s)", configPath, workDir)
				} else {
					executed.Reason = fmt.Sprintf("verify.yml에 실행 가능한 검증 명령이 없습니다: %s", configPath)
				}
			}
			if executed.SourceChanged {
				executed.OK = false
				executed.Reason = "live verification config changed after criteria snapshot; snapshot commands were executed but the run must be re-approved"
				executed.Checks = append(executed.Checks, verificationCheck{
					Name:     "criteria_source_unchanged",
					Kind:     "integrity",
					OK:       false,
					ExitCode: 1,
					Output:   executed.Reason,
				})
			}
			return emitVerificationResult(cmd, outputPath, executed)
		},
	}
	cmd.Flags().StringVar(&workDir, "workdir", "", "project working directory")
	cmd.Flags().StringVar(&configPath, "config", "", "legacy live verify.yml path")
	cmd.Flags().StringVar(&snapshotPath, "snapshot", "", "criteria snapshot path")
	cmd.Flags().StringVar(&heldOutPath, "held-out", "", "evaluator-only held-out snapshot path")
	cmd.Flags().StringVar(&manifestPath, "manifest", "", "run manifest bound to the criteria snapshot")
	cmd.Flags().StringVar(&liveConfigPath, "live-config", "", "live verify.yml path used only for change detection")
	cmd.Flags().StringVar(&outputPath, "output", "", "verification result path")
	_ = cmd.MarkFlagRequired("workdir")
	return cmd
}

func loadVerificationSteps(workDir, configPath string) ([]config.NamedVerifyStep, bool, error) {
	steps, skipped, _, err := criteria.ResolveVerification(workDir, configPath)
	return steps, skipped, err
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

func failVerification(cmd *cobra.Command, outputPath, manifestPath string, result verificationResult, eventType string, cause error) error {
	result.OK = false
	result.IntegrityOK = false
	result.Reason = cause.Error()
	if result.Timestamp == "" {
		result.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}
	if manifestPath != "" {
		if err := recordCriteriaEvent(manifestPath, criteria.Event{Type: eventType, Message: cause.Error()}); err != nil {
			cause = errors.Join(cause, fmt.Errorf("failed to record criteria event: %w", err))
			result.Reason = cause.Error()
		}
	}
	if err := writeVerificationResult(cmd, outputPath, result); err != nil {
		return errors.Join(cause, err)
	}
	return cause
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

func recordCriteriaEvent(manifestPath string, event criteria.Event) error {
	return criteria.AppendEvent(filepath.Join(filepath.Dir(manifestPath), "criteria-events.jsonl"), event)
}
