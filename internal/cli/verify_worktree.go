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
	if s.changes, s.untracked, err = projectStatus(top); err != nil {
		s.problem = "git status 실패"
		return s
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
			if _, err := os.Lstat(filepath.Join(full, ".git")); err != nil {
				continue // not initialized: rev-parse would walk up and report the superproject's HEAD
			}
			if head, err := gitIn(full, "", "rev-parse", "HEAD"); err != nil || head != blob {
				return reason(path)
			}
		case "120000": // symlink: the blob is the link text, which hash-object on the path would not read
			target, err := symlinkText(full)
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

// symlinkText returns a symlink's target, or the file content when core.symlinks=false checked it out as a plain
// file holding the link text.
func symlinkText(path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return os.Readlink(path)
	}
	data, err := os.ReadFile(path)
	return string(data), err
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
	rel, err := filepath.Rel(top, resolvePath(path))
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return err != nil
	}
	for _, part := range strings.Split(filepath.ToSlash(rel), "/") {
		if part == filepath.Base(layout.PylonDir("")) {
			return false
		}
	}
	return true
}

// underPylonRuntime reports whether path lies in some .pylon/runtime/ directory, the only place leftovers may go.
func underPylonRuntime(path string) bool {
	pylon, runtime := filepath.Base(layout.PylonDir("")), filepath.Base(layout.RuntimeDir(""))
	parts := strings.Split(filepath.ToSlash(resolvePath(path)), "/")
	for i := 0; i+1 < len(parts); i++ {
		if parts[i] == pylon && parts[i+1] == runtime {
			return true
		}
	}
	return false
}

// verificationArtifactsDir returns the absolute directory for verification artifacts: beside the result file
// (the pipeline's run directory), or <repo>/.pylon/runtime/verify-artifacts when there is none or it sits in
// the project itself.
func verificationArtifactsDir(top, outputPath string) string {
	if outputPath != "" {
		if dir := resolvePath(filepath.Join(filepath.Dir(outputPath), "verify-artifacts")); !insideProject(top, dir) {
			return dir
		}
	}
	return filepath.Join(layout.RuntimeDir(top), "verify-artifacts")
}

// renameFile is os.Rename; tests swap it to exercise the cross-filesystem copy.
var renameFile = os.Rename

// moveUntracked moves untracked project files into dir under the same relative paths and removes directories
// they leave empty. With replace, a file already at the destination is replaced — only that path, and only once
// the new file is in place — so recurring artifacts stay bounded; without it, an existing destination keeps the
// file in the repository. Files it cannot move are returned as kept.
func moveUntracked(top string, paths []string, dir string, replace bool) (moved, kept []string) {
	sorted := append([]string{}, paths...)
	sort.Strings(sorted)
	for _, path := range sorted {
		src, dst := filepath.Join(top, path), filepath.Join(dir, path)
		if err := moveEntry(src, dir, dst, replace); err != nil {
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

// moveEntry moves src to dst inside root without ever following a symlink an earlier move left there.
func moveEntry(src, root, dst string, replace bool) error {
	rel, err := filepath.Rel(root, filepath.Dir(dst))
	if err != nil {
		return err
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
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
		if err == nil { // a file or symlink from an earlier move sits where a directory is needed
			if !replace {
				return fmt.Errorf("%s exists", parent)
			}
			if err := os.Remove(parent); err != nil {
				return err
			}
		}
		if err := os.Mkdir(parent, 0o755); err != nil {
			return err
		}
	}
	if _, err := os.Lstat(dst); err == nil && !replace {
		return fmt.Errorf("%s exists", dst)
	}
	// Stage beside dst first so the previous artifact is removed only after the new one fully arrived.
	staged := dst + ".pylon-moving"
	if err := os.RemoveAll(staged); err != nil {
		return err
	}
	if err := renameFile(src, staged); err != nil {
		if err := copyEntry(src, staged); err != nil {
			_ = os.RemoveAll(staged)
			return err
		}
		if err := os.RemoveAll(src); err != nil {
			_ = os.RemoveAll(staged)
			return err
		}
	}
	if err := os.RemoveAll(dst); err != nil {
		return err
	}
	return os.Rename(staged, dst)
}

// copyEntry copies a file, symlink or directory tree for a move across filesystems, where rename fails.
func copyEntry(src, dst string) error {
	return filepath.WalkDir(src, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		switch {
		case info.Mode()&os.ModeSymlink != 0:
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(link, target)
		case info.IsDir():
			return os.Mkdir(target, info.Mode().Perm())
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode().Perm())
	})
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
			for _, arg := range args {
				want := strings.TrimSuffix(filepath.ToSlash(filepath.Clean(arg)), "/")
				found := false
				for _, path := range untracked {
					if trimmed := strings.TrimSuffix(path, "/"); trimmed == want || strings.HasPrefix(trimmed, want+"/") {
						selected, found = append(selected, path), true
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
