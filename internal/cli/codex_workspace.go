package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kyago/pylon/internal/layout"
)

// codexSkillMarker identifies a SKILL.md that earlier releases generated, so they can
// be removed without ever touching a user-authored skill.
const codexSkillMarker = "<!-- pylon-generated-skill -->"

// pruneCodexSkills removes the pointer skills earlier releases generated. Codex reads
// the workspace-root AGENTS.md natively, so nothing replaces them.
func pruneCodexSkills(root string) {
	for _, dir := range generatedCodexSkillDirs(root) {
		if err := os.RemoveAll(dir); err != nil {
			fmt.Fprintf(os.Stderr, "경고: Codex 스킬 정리 실패 (%s): %v\n", dir, err)
		}
	}
}

// generatedCodexSkillDirs returns the skill directories under .agents/skills that
// earlier releases generated (marker-bearing).
func generatedCodexSkillDirs(root string) []string {
	skillsDir := layout.CodexSkillsDir(root)
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		return nil
	}
	var dirs []string
	for _, entry := range entries {
		if !entry.IsDir() {
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
