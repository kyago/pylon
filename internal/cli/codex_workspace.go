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

// generateCodexSkills mirrors the pipeline commands (.pylon/builtin/commands/, then
// user-added .pylon/commands/) into codex's repo-scoped skill discovery directory
// (.agents/skills/<name>/SKILL.md).
// 각 스킬은 원본 커맨드 문서를 읽어 수행하라는 얇은 포인터라 소스는 하나로
// 유지된다. pylon이 만든(마커 보유) 스킬 중 원본 커맨드가 사라진 것은 제거하고,
// 마커 없는 사용자 스킬은 절대 건드리지 않는다.
func generateCodexSkills(root string) error {
	skillsDir := layout.CodexSkillsDir(root)
	desired := make(map[string]bool)
	for _, commandsDir := range []string{
		layout.BuiltinResourceDir(root, "commands"),
		layout.ResourceDir(root, "commands"),
	} {
		entries, err := os.ReadDir(commandsDir)
		if os.IsNotExist(err) {
			continue // 커맨드가 없는 디렉토리 — 만들 스킬도 없다
		}
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
				continue
			}
			name := strings.TrimSuffix(entry.Name(), ".md")
			if desired[name] {
				continue // 내장 이름이 우선한다 — syncBuiltinResources 참고
			}
			commandPath := filepath.Join(commandsDir, entry.Name())
			content, err := os.ReadFile(commandPath)
			if err != nil {
				// 읽기 실패를 부재로 취급하면 같은 실행에서 기존 스킬이 pruning된다.
				return fmt.Errorf("커맨드 %s 읽기 실패: %w", entry.Name(), err)
			}
			rel, err := filepath.Rel(root, commandPath)
			if err != nil {
				return err
			}
			skill, err := buildCodexSkill(name, filepath.ToSlash(rel), string(content))
			if err != nil {
				return fmt.Errorf("codex 스킬 %s 생성 실패: %w", name, err)
			}
			desired[name] = true
			if err := writeCodexSkill(filepath.Join(skillsDir, name), skill); err != nil {
				return err
			}
		}
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

// writeCodexSkill writes one pointer skill, leaving user-owned paths alone: a plain
// file at the skill path or a SKILL.md without the pylon marker is never overwritten,
// and neither fails the launch.
func writeCodexSkill(skillDir, skill string) error {
	if info, err := os.Stat(skillDir); err == nil && !info.IsDir() {
		return nil
	}
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		return err
	}
	skillPath := filepath.Join(skillDir, "SKILL.md")
	if existing, err := os.ReadFile(skillPath); err == nil {
		if string(existing) == skill || !strings.Contains(string(existing), codexSkillMarker) {
			return nil
		}
	}
	return os.WriteFile(skillPath, []byte(skill), 0o644)
}

// buildCodexSkill renders the pointer SKILL.md for one pipeline command. commandRel is
// the command file's slash path relative to the workspace root.
func buildCodexSkill(name, commandRel, commandContent string) (string, error) {
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
	fmt.Fprintf(&b, "`%s`를 읽고 그 지시를 그대로 수행하세요.\n", commandRel)
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
