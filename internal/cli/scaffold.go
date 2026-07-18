package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/kyago/pylon/internal/layout"
)

// scaffoldMarkdownResource creates .pylon/<subdir>/<name>.md with the given
// template content, shared by add-agent and add-skill. kind is the user-facing
// resource label ("에이전트", "스킬") used in error messages. The subdirectory is
// created if missing, and an existing file is never overwritten. Returns the
// created file path.
func scaffoldMarkdownResource(kind, subdir, name, content string) (string, error) {
	root, err := resolveRoot()
	if err != nil {
		return "", err
	}

	dir := filepath.Join(layout.PylonDir(root), subdir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("%s 디렉토리 생성 실패: %w", subdir, err)
	}

	path := filepath.Join(dir, name+".md")
	if _, err := os.Stat(path); err == nil {
		return "", fmt.Errorf("%s '%s'가 이미 존재합니다: %s", kind, name, path)
	}

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("%s 파일 생성 실패: %w", kind, err)
	}

	return path, nil
}
