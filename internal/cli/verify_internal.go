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
)

type verificationCheck struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Output string `json:"output"`
}

type verificationResult struct {
	OK        bool                `json:"ok"`
	Checks    []verificationCheck `json:"checks"`
	Skipped   bool                `json:"skipped,omitempty"`
	Reason    string              `json:"reason,omitempty"`
	Timestamp string              `json:"timestamp"`
}

func newInternalCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "internal", Hidden: true}
	cmd.AddCommand(newInternalVerifyCmd())
	return cmd
}

func newInternalVerifyCmd() *cobra.Command {
	var workDir, configPath, outputPath string
	cmd := &cobra.Command{
		Use:          "verify",
		Short:        "Run project verification commands",
		Hidden:       true,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			steps, skipped, err := loadVerificationSteps(workDir, configPath)
			if err != nil {
				return err
			}
			result, err := executeVerification(workDir, steps, time.Now)
			if err != nil {
				return err
			}
			result.Skipped = skipped
			// 검증할 것이 하나도 없으면 통과로 보고하지 않는다. 조용한 초록불은
			// 검증 실패보다 나쁘다 — 호출자가 잘못된 디렉토리를 가리켜도 알 수 없다.
			if len(steps) == 0 {
				result.OK = false
				if skipped {
					result.Reason = fmt.Sprintf("검증 설정을 찾을 수 없습니다: %s 를 작성하거나 --config로 올바른 경로를 지정하세요 (workdir: %s)", configPath, workDir)
				} else {
					result.Reason = fmt.Sprintf("verify.yml에 실행 가능한 검증 명령이 없습니다: %s", configPath)
				}
			}
			data, err := json.Marshal(result)
			if err != nil {
				return err
			}
			if outputPath != "" {
				if err := os.WriteFile(outputPath, append(data, '\n'), 0o644); err != nil {
					return fmt.Errorf("failed to write verification result: %w", err)
				}
			}
			fmt.Fprintln(cmd.OutOrStdout(), string(data))
			if !result.OK {
				if result.Reason != "" {
					return errors.New(result.Reason)
				}
				return errors.New("verification failed")
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&workDir, "workdir", "", "project working directory")
	cmd.Flags().StringVar(&configPath, "config", "", "verify.yml path")
	cmd.Flags().StringVar(&outputPath, "output", "", "verification result path")
	_ = cmd.MarkFlagRequired("workdir")
	return cmd
}

func loadVerificationSteps(workDir, configPath string) ([]config.NamedVerifyStep, bool, error) {
	if configPath == "" {
		configPath = filepath.Join(workDir, ".pylon", "verify.yml")
	}
	if _, err := os.Stat(configPath); err == nil {
		verifyConfig, err := config.LoadVerifyConfig(configPath)
		if err != nil {
			return nil, false, err
		}
		return verifyConfig.OrderedSteps(), false, nil
	} else if !os.IsNotExist(err) {
		return nil, false, fmt.Errorf("failed to inspect verify config: %w", err)
	}

	if _, err := os.Stat(filepath.Join(workDir, "go.mod")); err == nil {
		return []config.NamedVerifyStep{
			{Name: "build", Command: "go build ./...", Timeout: "5m"},
			{Name: "vet", Command: "go vet ./...", Timeout: "5m"},
			{Name: "test", Command: "go test ./...", Timeout: "10m"},
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
		// 살아남은 자식 프로세스가 출력 파이프를 물고 있어도 타임아웃 후 무한정 대기하지 않도록 한다.
		command.WaitDelay = 2 * time.Second
		output, runErr := command.CombinedOutput()
		timedOut := errors.Is(ctx.Err(), context.DeadlineExceeded)
		cancel()

		outputText := strings.TrimSpace(string(output))
		if timedOut {
			if outputText != "" {
				outputText += "\n"
			}
			outputText += "timed out after " + timeout.String()
		}
		check := verificationCheck{Name: step.Name, OK: runErr == nil && !timedOut, Output: outputText}
		if !check.OK {
			result.OK = false
		}
		result.Checks = append(result.Checks, check)
	}
	return result, nil
}
