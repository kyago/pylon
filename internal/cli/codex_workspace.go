package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/kyago/pylon/internal/layout"
)

// codexSkillMarker identifies a SKILL.md that pylon generated, so reconciliation
// can prune stale ones without ever touching a user-authored skill.
const codexSkillMarker = "<!-- pylon-generated-skill -->"

// generateCodexSkills mirrors the pipeline commands under .pylon/commands/ into
// codex's repo-scoped skill discovery directory (.agents/skills/<name>/SKILL.md).
// 각 스킬은 원본 커맨드 문서를 읽어 수행하라는 얇은 포인터라 소스는 하나로
// 유지된다. pylon이 만든(마커 보유) 스킬 중 원본 커맨드가 사라진 것은 제거하고,
// 마커 없는 사용자 스킬은 절대 건드리지 않는다.
func generateCodexSkills(root string) error {
	commandsDir := layout.CommandsDir(root)
	entries, err := os.ReadDir(commandsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // 커맨드가 없는 워크스페이스 — 만들 스킬도 없다
		}
		return err
	}

	skillsDir := layout.CodexSkillsDir(root)
	desired := make(map[string]bool)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), ".md")
		content, err := os.ReadFile(filepath.Join(commandsDir, entry.Name()))
		if err != nil {
			// 읽기 실패를 부재로 취급하면 같은 실행에서 기존 스킬이 pruning된다.
			return fmt.Errorf("커맨드 %s 읽기 실패: %w", entry.Name(), err)
		}
		skill, err := buildCodexSkill(name, string(content))
		if err != nil {
			return fmt.Errorf("codex 스킬 %s 생성 실패: %w", name, err)
		}
		skillDir := filepath.Join(skillsDir, name)
		// 스킬 경로가 디렉토리가 아닌 사용자 파일이면 소유권을 존중해 건너뛴다 —
		// launch 전체를 실패시키지 않는다.
		if info, statErr := os.Stat(skillDir); statErr == nil && !info.IsDir() {
			desired[name] = true
			continue
		}
		if err := os.MkdirAll(skillDir, 0o755); err != nil {
			return err
		}
		skillPath := filepath.Join(skillDir, "SKILL.md")
		if existing, err := os.ReadFile(skillPath); err == nil {
			if string(existing) == skill {
				desired[name] = true
				continue
			}
			// 마커 없는 파일은 사용자 소유 — 덮어쓰지 않는다.
			if !strings.Contains(string(existing), codexSkillMarker) {
				desired[name] = true
				continue
			}
		}
		if err := os.WriteFile(skillPath, []byte(skill), 0o644); err != nil {
			return err
		}
		desired[name] = true
	}

	// 원본 커맨드가 사라진 pylon 생성 스킬을 정리한다. 마커는 SKILL.md에만
	// 있으므로 그 디렉토리 전체가 pylon 소유라는 뜻이다 — 생성 스킬 디렉토리
	// 안에 사용자 파일을 두는 것은 지원하지 않는다.
	for _, dir := range generatedCodexSkillDirs(root, desired) {
		if err := os.RemoveAll(dir); err != nil {
			return err
		}
	}
	return nil
}

// generatedCodexSkillDirs returns the pylon-generated (marker-bearing) skill
// directories under .agents/skills, excluding names in keep. keep이 nil이면
// 생성된 스킬 전부를 반환한다 — uninstall이 이 형태로 사용한다.
func generatedCodexSkillDirs(root string, keep map[string]bool) []string {
	skillsDir := layout.CodexSkillsDir(root)
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		return nil
	}
	var dirs []string
	for _, entry := range entries {
		if !entry.IsDir() || keep[entry.Name()] {
			continue
		}
		data, err := os.ReadFile(filepath.Join(skillsDir, entry.Name(), "SKILL.md"))
		if err != nil || !strings.Contains(string(data), codexSkillMarker) {
			continue
		}
		dirs = append(dirs, filepath.Join(skillsDir, entry.Name()))
	}
	return dirs
}

// buildCodexSkill renders the pointer SKILL.md for one pipeline command.
func buildCodexSkill(name, commandContent string) (string, error) {
	frontmatter := struct {
		Name        string `yaml:"name"`
		Description string `yaml:"description"`
	}{
		Name:        name,
		Description: commandDescription(commandContent),
	}
	if frontmatter.Description == "" {
		frontmatter.Description = "pylon 워크플로우 커맨드 " + name
	}
	encoded, err := yaml.Marshal(frontmatter)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString("---\n")
	b.Write(encoded)
	b.WriteString("---\n")
	b.WriteString(codexSkillMarker + "\n\n")
	fmt.Fprintf(&b, "pylon 워크플로우 커맨드 `%s`입니다. 워크스페이스 루트의\n", name)
	fmt.Fprintf(&b, "`.pylon/commands/%s.md`를 읽고 그 지시를 그대로 수행하세요.\n", name)
	b.WriteString("스킬 호출에 덧붙인 텍스트는 커맨드 문서의 `$ARGUMENTS`로 사용합니다.\n")
	return b.String(), nil
}

// commandDescription extracts the description field from a command file's
// YAML frontmatter; empty when absent or unparseable.
func commandDescription(content string) string {
	rest, ok := strings.CutPrefix(content, "---\n")
	if !ok {
		return ""
	}
	frontmatter, _, ok := strings.Cut(rest, "\n---")
	if !ok {
		return ""
	}
	var meta struct {
		Description string `yaml:"description"`
	}
	if err := yaml.Unmarshal([]byte(frontmatter), &meta); err != nil {
		return ""
	}
	return strings.TrimSpace(meta.Description)
}
