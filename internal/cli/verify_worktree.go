package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/kyago/pylon/internal/fsutil"
	"github.com/kyago/pylon/internal/layout"
)

// treePathspec selects a repository's project content: .pylon/ (pylon-local state) and agent worktrees under
// .claude/worktrees/ are left out. init-pipeline.sh, pl-pr.md and pl-pipeline.md pass the same pathspec to
// `git status` (TestPRCommandsUseTreePathspec keeps them in step).
var treePathspec = []string{":(top)", ":(top,glob,exclude)**/" + pylonDirName + "/**", ":(top,exclude).claude/worktrees"}

// pylonDirName is ".pylon", taken from layout so the directory name lives in one place.
var pylonDirName = filepath.Base(layout.PylonDir(""))

// worktreeState is what verification needs to know about a repository: HEAD's tree and everything that keeps
// the working tree from being exactly that commit. The rules it serves are the verification evidence contract in
// AGENTS.md: never a tree for unchecked code, never a lost user file, everything else fails closed.
type worktreeState struct {
	top       string   // repository root; "" outside a git repository
	head      string   // HEAD commit
	tree      string   // HEAD^{tree}
	changes   []string // `git status` entries for tracked files
	untracked []string // untracked, non-ignored project files relative to top
	problem   string   // anything else that keeps tree from standing for the files on disk
}

func gitIn(dir, stdin string, args ...string) (string, error) {
	// --no-optional-locks: never take index.lock while agents work in the same repository.
	command := exec.Command("git", append([]string{"--no-optional-locks", "-C", dir}, args...)...)
	command.Stdin = strings.NewReader(stdin)
	out, err := command.Output()
	return strings.TrimRight(string(out), "\n"), err
}

func readWorktree(workDir string) worktreeState {
	var s worktreeState
	top, err := gitIn(workDir, "", "rev-parse", "--show-toplevel")
	if err != nil {
		s.problem = "커밋된 HEAD가 없습니다 (git 저장소가 아니거나 첫 커밋 전)"
		return s
	}
	s.top = top
	s.head, err = gitIn(top, "", "rev-parse", "--verify", "-q", "HEAD")
	if err == nil {
		s.tree, err = gitIn(top, "", "rev-parse", "--verify", "-q", "HEAD^{tree}")
	}
	if err != nil {
		s.head, s.tree, s.problem = "", "", "커밋된 HEAD가 없습니다 (git 저장소가 아니거나 첫 커밋 전)"
	}
	if s.changes, s.untracked, err = projectStatus(top); err != nil {
		s.problem = "git status 실패"
		return s
	}
	if s.problem != "" {
		return s
	}
	// A staged .pylon/ edit is outside the pathspec but would ride along with the next commit.
	if _, err := gitIn(top, "", "diff", "--cached", "--quiet", "--", ":(top,glob)**/"+pylonDirName+"/**"); err != nil {
		s.problem = "stage된 .pylon/ 변경이 있습니다"
		return s
	}
	s.problem = markedOnDisk(top)
	return s
}

// projectStatus returns the repository's `git status` entries for tracked project files and its untracked,
// non-ignored project files relative to top, in one scan. --untracked-files is explicit because
// status.showUntrackedFiles=no would hide a forgotten new source file.
func projectStatus(top string) (changes, untracked []string, err error) {
	status, err := gitIn(top, "", append([]string{"status", "--porcelain", "-z", "--untracked-files=all",
		"--ignore-submodules=untracked", "--"}, treePathspec...)...)
	if err != nil {
		return nil, nil, err
	}
	entries := strings.Split(status, "\x00")
	for i := 0; i < len(entries); i++ {
		entry := entries[i]
		if len(entry) < 4 {
			continue
		}
		if path, ok := strings.CutPrefix(entry, "?? "); ok {
			untracked = append(untracked, path)
			continue
		}
		changes = append(changes, entry)
		if strings.ContainsAny(entry[:2], "RC") {
			i++ // -z puts a rename's source path in the next field
		}
	}
	return changes, untracked, nil
}

// markedOnDisk reports a file marked assume-unchanged or skip-worktree that is present on disk: git status cannot
// see its edits, so the tree fails closed instead of comparing contents. A skip-worktree file absent from disk
// (sparse checkout) is not checked out at all and does not count.
func markedOnDisk(top string) string {
	entries, err := gitIn(top, "", append([]string{"ls-files", "-v", "-z", "--"}, treePathspec...)...)
	if err != nil {
		return "git ls-files 실패"
	}
	for _, entry := range strings.Split(entries, "\x00") {
		if len(entry) < 3 {
			continue
		}
		tag, path := entry[0], entry[2:]
		if (tag < 'a' || tag > 'z') && tag != 'S' {
			continue
		}
		if _, err := os.Lstat(filepath.Join(top, path)); err == nil {
			return "assume-unchanged·skip-worktree 표시된 파일이 있어 수정 여부를 알 수 없습니다: " + path +
				" (git update-index --no-assume-unchanged / --no-skip-worktree로 해제)"
		}
	}
	return ""
}

// dirtyReason explains why the working tree is not exactly HEAD, counting only the given untracked files;
// "" means clean.
func (s worktreeState) dirtyReason(untracked []string) string {
	if s.problem != "" {
		return s.problem
	}
	dirty := append([]string{}, s.changes...)
	for _, path := range untracked {
		dirty = append(dirty, "?? "+path)
	}
	if len(dirty) == 0 {
		return ""
	}
	if len(dirty) > 3 {
		dirty = append(dirty[:3], "...")
	}
	return "커밋되지 않은 변경: " + strings.Join(dirty, ", ")
}

// resolvePath returns path made absolute with symlinks resolved as far as it exists, so it compares with the
// resolved root git reports (/private/var on macOS) even when its tail is not created yet.
func resolvePath(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	var tail []string
	for dir := abs; ; dir = filepath.Dir(dir) {
		if real, err := filepath.EvalSymlinks(dir); err == nil {
			return filepath.Join(append([]string{real}, tail...)...)
		}
		if filepath.Dir(dir) == dir {
			return abs
		}
		tail = append([]string{filepath.Base(dir)}, tail...)
	}
}

// insideProject reports whether path lies in the repository's project content: under top and outside every
// .pylon/ directory. Moving files there would put them right back into the tree they were taken out of.
func insideProject(top, path string) bool {
	resolved := resolvePath(path)
	within, err := fsutil.IsWithin(top, resolved)
	if err != nil || !within {
		return err != nil
	}
	rel, err := filepath.Rel(top, resolved)
	if err != nil {
		return true
	}
	for _, part := range strings.Split(filepath.ToSlash(rel), "/") {
		if part == pylonDirName {
			return false
		}
	}
	return true
}

// underPylonRuntime reports whether path lies in some .pylon/runtime/ directory, the only place moved files may go.
func underPylonRuntime(path string) bool {
	runtime := filepath.Base(layout.RuntimeDir(""))
	parts := strings.Split(filepath.ToSlash(resolvePath(path)), "/")
	for i := 0; i+1 < len(parts); i++ {
		if parts[i] == pylonDirName && parts[i+1] == runtime {
			return true
		}
	}
	return false
}

// verificationArtifactsDir returns the absolute directory for verification artifacts: beside the result file when
// that is a run directory under .pylon/runtime/, otherwise <repo>/.pylon/runtime/verify-artifacts.
func verificationArtifactsDir(top, outputPath string) string {
	if outputPath != "" {
		dir := resolvePath(filepath.Join(filepath.Dir(outputPath), layout.VerifyArtifactsName))
		if underPylonRuntime(dir) && !insideProject(top, dir) {
			return dir
		}
	}
	return layout.VerifyArtifactsDir(top)
}

// moveUntracked moves untracked project files into dir under the same relative paths. With replace, an earlier
// artifact at the destination is replaced once the new file is in place, so recurring artifacts stay bounded;
// without it, an existing destination keeps the file in the repository. Emptied directories are left alone: git
// cannot say which ones a step created. Files it cannot move are returned as kept.
func moveUntracked(top string, paths []string, dir string, replace bool) (moved, kept []string) {
	sorted := append([]string{}, paths...)
	sort.Strings(sorted)
	for _, path := range sorted {
		if err := moveEntry(filepath.Join(top, path), dir, filepath.Join(dir, path), replace); err != nil {
			kept = append(kept, path)
			continue
		}
		moved = append(moved, path)
	}
	return moved, kept
}

// moveEntry moves src to dst inside root. The file first lands in a private staging directory, so nothing in root
// is removed unless the move itself succeeded, and no symlink an earlier move left in root is ever followed.
// Rename only: across filesystems it fails and the file stays where it is (fail-closed).
func moveEntry(src, root, dst string, replace bool) error {
	rel, err := filepath.Rel(root, filepath.Dir(dst))
	if err != nil {
		return err
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	staging, err := os.MkdirTemp(root, ".pylon-moving-")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(staging) }() // only when empty: a file that could not go back stays here
	staged := filepath.Join(staging, filepath.Base(dst))
	if err := os.Rename(src, staged); err != nil {
		return err
	}
	restore := func(cause error) error {
		if err := os.Rename(staged, src); err != nil {
			return fmt.Errorf("%w (원래 위치로 되돌리지 못함, %s에 있음: %v)", cause, staged, err)
		}
		return cause
	}
	parent := root
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		if part == "." {
			continue
		}
		parent = filepath.Join(parent, part)
		info, err := os.Lstat(parent)
		if err == nil && info.IsDir() {
			continue
		}
		if err == nil { // an earlier artifact (file or symlink) sits where a directory is needed
			if !replace {
				return restore(fmt.Errorf("%s exists", parent))
			}
			if err := os.Remove(parent); err != nil {
				return restore(err)
			}
		}
		if err := os.Mkdir(parent, 0o755); err != nil {
			return restore(err)
		}
	}
	if _, err := os.Lstat(dst); err == nil {
		if !replace {
			return restore(fmt.Errorf("%s exists", dst))
		}
		if err := os.RemoveAll(dst); err != nil {
			return restore(err)
		}
	}
	if err := os.Rename(staged, dst); err != nil {
		return restore(err)
	}
	return nil
}

// newInternalMoveUntrackedCmd moves the listed untracked files out of a repository into a fresh directory under a
// run's .pylon/runtime/, so the working tree is clean for verification and the files stay recoverable.
func newInternalMoveUntrackedCmd() *cobra.Command {
	var workDir, to string
	cmd := &cobra.Command{
		Use:          "move-untracked -- <path>...",
		Short:        "Move listed untracked project files under a run directory, keeping their paths",
		Args:         cobra.MinimumNArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			top, err := gitIn(workDir, "", "rev-parse", "--show-toplevel")
			if err != nil {
				return fmt.Errorf("git 저장소가 아닙니다: %s", workDir)
			}
			if to == "" || !underPylonRuntime(to) || insideProject(top, to) {
				return fmt.Errorf("--to는 .pylon/runtime/ 아래(run 폴더)여야 합니다: %q", to)
			}
			_, untracked, err := projectStatus(top)
			if err != nil {
				return fmt.Errorf("git status 실패: %w", err)
			}
			// Only listed paths that git reports as untracked move; a directory selects the files under it.
			var selected, missing []string
			seen := map[string]bool{}
			for _, arg := range args {
				want := strings.TrimSuffix(filepath.ToSlash(filepath.Clean(arg)), "/")
				found := false
				for _, path := range untracked {
					if trimmed := strings.TrimSuffix(path, "/"); trimmed == want || strings.HasPrefix(trimmed, want+"/") {
						found = true
						if !seen[path] {
							seen[path], selected = true, append(selected, path)
						}
					}
				}
				if !found {
					missing = append(missing, arg)
				}
			}
			// Each call gets its own directory, so a second move never meets an earlier one's files.
			if err := os.MkdirAll(to, 0o755); err != nil {
				return err
			}
			dir, err := os.MkdirTemp(to, time.Now().UTC().Format("20060102T150405Z")+"-")
			if err != nil {
				return err
			}
			moved, kept := moveUntracked(top, selected, dir, false)
			if len(moved) == 0 {
				_ = os.Remove(dir)
			}
			data, err := json.Marshal(struct {
				Dir     string   `json:"dir"`
				Moved   []string `json:"moved"`
				Kept    []string `json:"kept"`
				Missing []string `json:"not_untracked"`
			}{dir, append([]string{}, moved...), append([]string{}, kept...), append([]string{}, missing...)})
			if err != nil {
				return err
			}
			if _, err := fmt.Fprintln(cmd.OutOrStdout(), string(data)); err != nil {
				return err
			}
			if len(kept) > 0 || len(missing) > 0 {
				return errors.New("옮기지 못했거나 추적 안 된 파일이 아닌 경로가 있습니다")
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&workDir, "workdir", ".", "repository to clean")
	cmd.Flags().StringVar(&to, "to", "", "run directory under .pylon/runtime/ to move into (paths are kept)")
	return cmd
}
