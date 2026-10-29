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
// created if missing, an existing file is never overwritten, and a built-in name is
// refused — the builtin would shadow it and the next sync would clear it. Returns the
// created file path.
func scaffoldMarkdownResource(kind, subdir, name, content string) (string, error) {
	root, err := resolveRoot()
	if err != nil {
		return "", err
	}

	if isBuiltinResource(subdir + "/" + name + ".md") {
		return "", fmt.Errorf("%s '%s'는 내장 리소스 이름입니다 — 다른 이름을 사용하세요", kind, name)
	}
	// 은퇴한 내장 이름의 파일은 다음 sync가 .pylon-bak으로 비켜 놓는다 — syncBuiltinResources 참고.
	if _, retired := retiredBuiltins[subdir+"/"+name+".md"]; retired {
		return "", fmt.Errorf("%s '%s'는 은퇴한 내장 리소스 이름입니다 — 다른 이름을 사용하세요", kind, name)
	}

	dir := layout.ResourceDir(root, subdir)
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

// isBuiltinResource reports whether rel (e.g. "agents/architect.md") names an embedded resource.
func isBuiltinResource(rel string) bool {
	files, err := embeddedBuiltinFiles()
	if err != nil {
		return false
	}
	for _, f := range files {
		if f.rel == rel {
			return true
		}
	}
	return false
}
