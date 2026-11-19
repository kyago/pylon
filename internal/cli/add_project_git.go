package cli

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func validateProjectName(name string) error {
	if name == "" {
		return fmt.Errorf("프로젝트 이름이 비어 있습니다")
	}
	if name == "." || name == ".." {
		return fmt.Errorf("잘못된 프로젝트 이름: %q", name)
	}
	if strings.ContainsAny(name, `/\`) || filepath.IsAbs(name) {
		return fmt.Errorf("프로젝트 이름에 경로 구분자를 쓸 수 없습니다: %q", name)
	}
	if filepath.Clean(name) != name {
		return fmt.Errorf("잘못된 프로젝트 이름: %q", name)
	}
	return nil
}

// inferProjectName extracts project name from git URL.
func inferProjectName(repoURL string) string {
	// Handle "https://github.com/user/repo.git" or "git@github.com:user/repo.git"
	name := repoURL
	if idx := strings.LastIndex(name, "/"); idx >= 0 {
		name = name[idx+1:]
	}
	if idx := strings.LastIndex(name, ":"); idx >= 0 {
		name = name[idx+1:]
	}
	name = strings.TrimSuffix(name, ".git")
	return name
}

// errNotGitRepo marks a directory git does not recognize as a repository, so callers
// can skip it without matching on message text.
var errNotGitRepo = errors.New("git 저장소가 아닙니다")

// resolveGitExcludePath returns the absolute path to a project's .git/info/exclude file.
// It uses "git rev-parse --git-dir" to correctly resolve non-standard git layouts
// (e.g. worktrees, or legacy submodule clones whose .git is a gitlink file).
func resolveGitExcludePath(projectDir string) (string, error) {
	gitDirCmd := exec.Command("git", "rev-parse", "--git-dir")
	gitDirCmd.Dir = projectDir
	out, err := gitDirCmd.Output()
	if err != nil {
		return "", fmt.Errorf("%w: %v", errNotGitRepo, err)
	}

	gitDir := strings.TrimSpace(string(out))
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(projectDir, gitDir)
	}

	return filepath.Join(gitDir, "info", "exclude"), nil
}

// excludePylonFromRepo adds ".pylon/" to the repo's .git/info/exclude. Works for both submodules and standalone clones.
func excludePylonFromRepo(projectDir string) error {
	return excludeEntryFromRepo(projectDir, ".pylon/")
}

// excludeEntryFromRepo adds one entry to the repo's .git/info/exclude.
// 단일 repo 워크스페이스 루트처럼 .pylon/ 전체가 아니라 특정 파일만
// 제외해야 하는 호출자를 위해 엔트리를 매개변수로 받는다.
func excludeEntryFromRepo(projectDir, entry string) error {
	excludePath, err := resolveGitExcludePath(projectDir)
	if err != nil {
		return err
	}

	// Read existing exclude file if it exists
	existing, err := os.ReadFile(excludePath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("exclude 파일 읽기 실패: %w", err)
	}
	for _, line := range strings.Split(string(existing), "\n") {
		if strings.TrimSpace(line) == entry {
			return nil // already excluded
		}
	}

	// Ensure info/ directory exists
	infoDir := filepath.Dir(excludePath)
	if err := os.MkdirAll(infoDir, 0755); err != nil {
		return fmt.Errorf("git info 디렉터리 생성 실패: %w", err)
	}

	f, err := os.OpenFile(excludePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("exclude 파일 열기 실패: %w", err)
	}
	defer f.Close()

	// Add newline before entry if file doesn't end with one
	if len(existing) > 0 && existing[len(existing)-1] != '\n' {
		if _, err := f.WriteString("\n"); err != nil {
			return err
		}
	}
	if _, err := f.WriteString(entry + "\n"); err != nil {
		return err
	}

	return nil
}

// techStack holds detected technology information.
