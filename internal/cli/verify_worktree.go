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

	"github.com/spf13/cobra"

	"github.com/kyago/pylon/internal/fsutil"
	"github.com/kyago/pylon/internal/layout"
)

// treePathspec selects a repository's project content: .pylon/ (pylon-local state) and agent worktrees under
// .claude/worktrees/ are left out. init-pipeline.sh, pl-pr.md and pl-pipeline.md pass the same pathspec to
// `git status` (TestPRCommandsUseTreePathspec keeps them in step).
var treePathspec = []string{":(top)", ":(top,glob,exclude)**/.pylon/**", ":(top,exclude).claude/worktrees"}

// worktreeState is what verification needs to know about a repository: HEAD's tree and everything that keeps
// the working tree from being exactly that commit. Ignored files are local environment and never count;
// unstaged edits to tracked .pylon/ files are not seen either, since pylon writes its own state there.
type worktreeState struct {
	top       string   // repository root; "" outside a git repository
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
	if s.tree, err = gitIn(top, "", "rev-parse", "--verify", "-q", "HEAD^{tree}"); err != nil {
		s.tree, s.problem = "", "커밋된 HEAD가 없습니다 (git 저장소가 아니거나 첫 커밋 전)"
	}
	// One scan gives both the clean check and the untracked set. --untracked-files is explicit because
	// status.showUntrackedFiles=no would hide a forgotten new source file.
	status, err := gitIn(top, "", append([]string{"status", "--porcelain", "-z", "--untracked-files=all",
		"--ignore-submodules=untracked", "--"}, treePathspec...)...)
	if err != nil {
		s.problem = "git status 실패"
		return s
	}
	entries := strings.Split(status, "\x00")
	for i := 0; i < len(entries); i++ {
		entry := entries[i]
		if len(entry) < 4 {
			continue
		}
		if path, ok := strings.CutPrefix(entry, "?? "); ok {
			s.untracked = append(s.untracked, path)
			continue
		}
		s.changes = append(s.changes, entry)
		if strings.ContainsAny(entry[:2], "RC") {
			i++ // -z puts a rename's source path in the next field
		}
	}
	if s.problem != "" {
		return s
	}
	// A staged .pylon/ edit is outside the pathspec but would ride along with the next commit.
	if _, err := gitIn(top, "", "diff", "--cached", "--quiet", "--", ":(top,glob)**/.pylon/**"); err != nil {
		s.problem = "stage된 .pylon/ 변경이 있습니다"
		return s
	}
	s.problem = hiddenEdit(top)
	return s
}

// hiddenEdit finds edits that git status cannot see: entries marked assume-unchanged or skip-worktree whose
// disk content differs from the index (equal to HEAD once status is clean). A skip-worktree entry absent from
// disk (sparse checkout) is not an edit.
func hiddenEdit(top string) string {
	entries, err := gitIn(top, "", append([]string{"ls-files", "-s", "-v", "-z", "--"}, treePathspec...)...)
	if err != nil {
		return "git ls-files 실패"
	}
	reason := func(path string) string {
		return "assume-unchanged·skip-worktree로 숨긴 수정이 있습니다: " + path
	}
	var paths, blobs []string
	for _, entry := range strings.Split(entries, "\x00") {
		meta, path, ok := strings.Cut(entry, "\t")
		fields := strings.Fields(meta)
		if !ok || len(fields) != 4 {
			continue
		}
		tag, mode, blob := fields[0][0], fields[1], fields[2]
		assumeUnchanged, skipWorktree := tag >= 'a' && tag <= 'z', tag == 'S' || tag == 's'
		if !assumeUnchanged && !skipWorktree {
			continue
		}
		full := filepath.Join(top, path)
		if _, err := os.Lstat(full); err != nil {
			if skipWorktree {
				continue
			}
			return reason(path)
		}
		switch mode {
		case "160000": // submodule: the index holds a commit, not file content
			if head, err := gitIn(full, "", "rev-parse", "HEAD"); err != nil || head != blob {
				return reason(path)
			}
		case "120000": // symlink: the blob is the link text, which hash-object on the path would not read
			target, err := os.Readlink(full)
			if err != nil {
				return reason(path)
			}
			if hash, err := gitIn(top, target, "hash-object", "--stdin"); err != nil || hash != blob {
				return reason(path)
			}
		default:
			paths, blobs = append(paths, path), append(blobs, blob)
		}
	}
	if len(paths) > 0 {
		hashes, err := gitIn(top, strings.Join(paths, "\n")+"\n", "hash-object", "--stdin-paths")
		if err != nil || hashes != strings.Join(blobs, "\n") {
			return reason(strings.Join(paths, ", "))
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

// insideProject reports whether path lies in the repository's project content: under top and outside every
// .pylon/ directory. Moving files there would put them right back into the tree they were taken out of.
func insideProject(top, path string) bool {
	abs, err := filepath.Abs(path)
	if err != nil {
		return true
	}
	// git reports top with symlinks resolved (/private/var on macOS), so resolve the existing parent too.
	if parent, err := filepath.EvalSymlinks(filepath.Dir(abs)); err == nil {
		abs = filepath.Join(parent, filepath.Base(abs))
	}
	if within, err := fsutil.IsWithin(top, abs); err != nil || !within {
		return err != nil
	}
	rel, err := filepath.Rel(top, abs)
	if err != nil {
		return true
	}
	for _, part := range strings.Split(filepath.ToSlash(rel), "/") {
		if part == filepath.Base(layout.PylonDir("")) {
			return false
		}
	}
	return true
}

// verificationArtifactsDir returns where verification artifacts go: beside the result file (the pipeline's run
// directory), or <repo>/.pylon/runtime/verify-artifacts when there is none or it sits in the project itself.
func verificationArtifactsDir(top, outputPath string) string {
	fallback := filepath.Join(layout.RuntimeDir(top), "verify-artifacts")
	if outputPath == "" {
		return fallback
	}
	if dir := filepath.Join(filepath.Dir(outputPath), "verify-artifacts"); !insideProject(top, dir) {
		return dir
	}
	return fallback
}

// moveUntracked moves untracked project files into dir under the same relative paths and removes directories
// they leave empty. Only a file already at the same destination path is replaced — never the whole directory —
// so recurring artifacts stay bounded and nothing else is ever deleted. Files it cannot move are returned as kept.
func moveUntracked(top string, paths []string, dir string) (moved, kept []string) {
	sorted := append([]string{}, paths...)
	sort.Strings(sorted)
	for _, path := range sorted {
		src, dst := filepath.Join(top, path), filepath.Join(dir, path)
		// shortcut: rename only; across filesystems the file stays put and is reported as kept (fail-closed).
		if os.RemoveAll(dst) != nil || os.MkdirAll(filepath.Dir(dst), 0o755) != nil || os.Rename(src, dst) != nil {
			kept = append(kept, path)
			continue
		}
		moved = append(moved, path)
		for parent := filepath.Dir(src); parent != top && strings.HasPrefix(parent, top+string(filepath.Separator)); parent = filepath.Dir(parent) {
			if os.Remove(parent) != nil {
				break // not empty
			}
		}
	}
	return moved, kept
}

// newInternalMoveUntrackedCmd moves a run's leftover untracked files out of a repository so its working tree is
// clean for verification, keeping them recoverable under the run directory.
func newInternalMoveUntrackedCmd() *cobra.Command {
	var workDir, to string
	cmd := &cobra.Command{
		Use:          "move-untracked",
		Short:        "Move untracked project files out of a repository, keeping their paths",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			state := readWorktree(workDir)
			if state.top == "" {
				return fmt.Errorf("git 저장소가 아닙니다: %s", workDir)
			}
			if to == "" || insideProject(state.top, to) {
				return fmt.Errorf("--to는 프로젝트 밖(예: run 폴더)이어야 합니다: %q", to)
			}
			moved, kept := moveUntracked(state.top, state.untracked, to)
			data, err := json.Marshal(struct {
				Moved []string `json:"moved"`
				Kept  []string `json:"kept"`
			}{append([]string{}, moved...), append([]string{}, kept...)})
			if err != nil {
				return err
			}
			if _, err := fmt.Fprintln(cmd.OutOrStdout(), string(data)); err != nil {
				return err
			}
			if len(kept) > 0 {
				return errors.New("옮기지 못한 파일이 있습니다")
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&workDir, "workdir", ".", "repository to clean")
	cmd.Flags().StringVar(&to, "to", "", "destination directory outside the project (paths are kept)")
	return cmd
}
