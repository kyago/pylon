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
	Tree           string              `json:"tree,omitempty"`
	Timestamp      string              `json:"timestamp"`
}

func newInternalCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "internal", Short: "Pipeline helper commands used by /pl:* commands", Hidden: true}
	cmd.AddCommand(newInternalVerifyCmd())
	cmd.AddCommand(newInternalCriteriaCmd())
	cmd.AddCommand(newInternalEvaluatorCmd())
	cmd.AddCommand(newInternalTrajectoryCmd())
	return cmd
}

func newInternalVerifyCmd() *cobra.Command {
	var workDir, configPath, outputPath string
	var snapshotPath, heldOutPath, manifestPath, liveConfigPath string
	cmd := &cobra.Command{
		Use:          "verify",
		Short:        "Run project verification commands",
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
				changed, actualDigest, err := criteria.LiveSourceChanged(snapshot, resolveLiveConfigPath(workDir, liveConfigPath, snapshot.Source))
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

			snapshots, closeSnapshots := newTreeSnapshots(workDir)
			defer closeSnapshots()
			treeBefore := snapshots.tree()
			executed, err := executeVerification(workDir, steps, time.Now)
			if err != nil {
				return err
			}
			// A step that rewrites files (formatter) leaves no single tree the checks ran against; files a step
			// only adds (coverage.out, build output) were not inputs, so the tree from before still holds.
			if treeBefore != "" && snapshots.onlyAdditionsSince(treeBefore) {
				executed.Tree = treeBefore
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

// resolveLiveConfigPath returns the live verify.yml path used for change
// detection. 구버전 binary가 만든 snapshot은 Source.Path를 상대경로로 기록했는데,
// 검증 프로세스의 cwd에서는 그 경로가 해석되지 않아 존재하는 파일을 "missing"으로
// 오판한다. 그 경우에만 workdir의 표준 위치로 대체하고, 그 외에는
// LiveSourceChanged의 Source.Path fallback에 맡긴다.
func resolveLiveConfigPath(workDir, liveConfigPath string, source criteria.Source) string {
	if liveConfigPath != "" {
		return liveConfigPath
	}
	if source.Path != "" && !filepath.IsAbs(source.Path) {
		return filepath.Join(workDir, ".pylon", "verify.yml")
	}
	return ""
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

// treePathspec selects a repository's project content: .pylon/ (pylon-local state) and agent worktrees under
// .claude/worktrees/ are left out. pl-pr.md and pl-pipeline.md pass the same pathspec to `git status`.
var treePathspec = []string{":(top)", ":(top,glob,exclude)**/.pylon/**", ":(top,exclude).claude/worktrees"}

// treeSnapshots hashes workDir's working tree, untracked files included, so a verdict on uncommitted changes
// matches the commit that later lands them. It works on a private copy of the index, leaving the real one
// untouched; later snapshots reuse that copy so only files changed since are rehashed.
type treeSnapshots struct {
	workDir string
	index   string // "" when workDir is not a git repository
}

func newTreeSnapshots(workDir string) (*treeSnapshots, func()) {
	s := &treeSnapshots{workDir: workDir}
	tmpDir, err := os.MkdirTemp("", "pylon-tree-")
	if err != nil {
		return s, func() {}
	}
	cleanup := func() { _ = os.RemoveAll(tmpDir) }
	// --path-format=absolute needs git 2.31; --git-path prints a path relative to workDir instead.
	out, err := exec.Command("git", "-C", workDir, "rev-parse", "--git-path", "index").Output()
	if err != nil {
		return s, cleanup
	}
	realIndexPath := strings.TrimSpace(string(out))
	if !filepath.IsAbs(realIndexPath) {
		realIndexPath = filepath.Join(workDir, realIndexPath)
	}
	index := filepath.Join(tmpDir, "index")
	// The copied stat cache keeps unchanged files from being rehashed; a missing index just starts empty.
	// The copy keeps the original mtime because git rechecks content only for entries not older than the
	// index file (racy git); a fresh mtime would let a same-size edit in that window pass as unchanged.
	if info, err := os.Stat(realIndexPath); err == nil {
		data, err := os.ReadFile(realIndexPath)
		if err != nil {
			return s, cleanup
		}
		if err := os.WriteFile(index, data, 0o600); err != nil {
			return s, cleanup
		}
		if err := os.Chtimes(index, info.ModTime(), info.ModTime()); err != nil {
			return s, cleanup
		}
	}
	s.index = index
	return s, cleanup
}

func (s *treeSnapshots) git(stdin string, args ...string) (string, error) {
	// A split temp index would leave orphan sharedindex files in the real .git.
	command := exec.Command("git", append([]string{"-C", s.workDir, "-c", "core.splitIndex=false"}, args...)...)
	command.Env = append(os.Environ(), "GIT_INDEX_FILE="+s.index)
	command.Stdin = strings.NewReader(stdin)
	out, err := command.Output()
	return strings.TrimSpace(string(out)), err
}

// tree returns the hash of the current working tree, or "" when no tree can carry it.
func (s *treeSnapshots) tree() string {
	if s.index == "" {
		return ""
	}
	// assume-unchanged and skip-worktree hide edits from `add`. The checks ran against what is on disk, so drop
	// both marks in the private index; a skip-worktree file absent from disk (sparse checkout) keeps its blob.
	entries, err := s.git("", append([]string{"ls-files", "-v", "-z"}, treePathspec...)...)
	if err != nil {
		return ""
	}
	var assumed, skipped strings.Builder
	for _, entry := range strings.Split(entries, "\x00") {
		if len(entry) < 3 {
			continue
		}
		tag, path := entry[0], entry[2:]
		if tag >= 'a' && tag <= 'z' {
			assumed.WriteString(path + "\x00")
		}
		if tag == 'S' || tag == 's' {
			if _, err := os.Lstat(filepath.Join(s.workDir, path)); err == nil {
				skipped.WriteString(path + "\x00")
			}
		}
	}
	// update-index applies only the first mark option per path, so each mark needs its own call.
	if assumed.Len() > 0 {
		if _, err := s.git(assumed.String(), "update-index", "--no-assume-unchanged", "-z", "--stdin"); err != nil {
			return ""
		}
	}
	if skipped.Len() > 0 {
		if _, err := s.git(skipped.String(), "update-index", "--no-skip-worktree", "-z", "--stdin"); err != nil {
			return ""
		}
	}
	if _, err := s.git("", append([]string{"add", "-A"}, treePathspec...)...); err != nil {
		return ""
	}
	// Tracked .pylon/ files take their committed blobs, so pylon writing its own state never moves the tree.
	if _, err := s.git("", "rev-parse", "--verify", "-q", "HEAD"); err == nil {
		if _, err := s.git("", "reset", "-q", "HEAD", "--", ":(top,glob)**/.pylon/**"); err != nil {
			return ""
		}
	}
	// A tree cannot carry edits inside submodules or nested repositories; their untracked files are not edits.
	if _, err := s.git("", append([]string{"diff-files", "--quiet", "--ignore-submodules=untracked"}, treePathspec...)...); err != nil {
		return ""
	}
	tree, err := s.git("", "write-tree")
	if err != nil {
		return ""
	}
	return tree
}

// onlyAdditionsSince reports whether the working tree still matches before apart from newly added files.
func (s *treeSnapshots) onlyAdditionsSince(before string) bool {
	after := s.tree()
	if after == "" {
		return false
	}
	if after == before {
		return true
	}
	changed, err := s.git("", "diff-tree", "-r", "--no-renames", "--name-only", "--diff-filter=a", before, after)
	return err == nil && changed == ""
}

// workingTree returns a single snapshot of workDir's working tree; see treeSnapshots.
func workingTree(workDir string) string {
	snapshots, closeSnapshots := newTreeSnapshots(workDir)
	defer closeSnapshots()
	return snapshots.tree()
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
