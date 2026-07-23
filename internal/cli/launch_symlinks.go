package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kyago/pylon/internal/config"
	"github.com/kyago/pylon/internal/layout"
)

func generateClaudeAgentsWithSkills(root string, cfg *config.Config) error {
	pylonAgentsDir := filepath.Join(layout.PylonDir(root), "agents")
	claudeAgentsDir := layout.ClaudeAgentsDir(root)
	if err := os.MkdirAll(claudeAgentsDir, 0o755); err != nil {
		return err
	}
	entries, err := os.ReadDir(pylonAgentsDir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}

	skillMap, err := loadAgentSkillMap(cfg, filepath.Join(layout.PylonDir(root), "skills"))
	if err != nil {
		return err
	}
	expected := make(map[string]bool)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		expected[entry.Name()] = true
		if err := materializeClaudeAgent(
			filepath.Join(pylonAgentsDir, entry.Name()),
			filepath.Join(claudeAgentsDir, entry.Name()),
			layout.AgentLinkTarget(entry.Name()), cfg, skillMap,
		); err != nil {
			return fmt.Errorf("%s 에이전트 동기화 실패: %w", entry.Name(), err)
		}
	}
	return removeStaleAgentLinks(claudeAgentsDir, expected)
}

func loadAgentSkillMap(cfg *config.Config, skillsDir string) (map[string]*config.SkillConfig, error) {
	skillMap := make(map[string]*config.SkillConfig)
	if !cfg.Skills.Enabled || !cfg.Skills.PreloadToAgents {
		return skillMap, nil
	}
	skills, err := config.DiscoverSkills(skillsDir)
	if err != nil {
		return nil, fmt.Errorf("스킬 로드 실패: %w", err)
	}
	for _, skill := range skills {
		skillMap[skill.Name] = skill
	}
	return skillMap, nil
}

func materializeClaudeAgent(agentPath, linkPath, target string, cfg *config.Config, skillMap map[string]*config.SkillConfig) error {
	agent, err := config.ParseAgentFile(agentPath)
	if err != nil || !cfg.Skills.Enabled || !cfg.Skills.PreloadToAgents || len(agent.Skills) == 0 {
		return ensureSymlink(linkPath, target)
	}
	content, err := os.ReadFile(agentPath)
	if err != nil {
		return err
	}
	injected := buildSkillInjection(agent.Skills, skillMap, cfg.Skills.ProgressiveDisclosure)
	if injected == "" {
		return ensureSymlink(linkPath, target)
	}
	return writeInjectedAgent(linkPath, string(content)+"\n\n"+injected)
}

func writeInjectedAgent(path, content string) error {
	info, err := os.Lstat(path)
	switch {
	case err == nil && info.Mode()&os.ModeSymlink != 0:
		if err := os.Remove(path); err != nil {
			return err
		}
	case err == nil:
		existing, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !strings.Contains(string(existing), skillInjectionMarker) || string(existing) == content {
			return nil
		}
	case !os.IsNotExist(err):
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

func removeStaleAgentLinks(claudeAgentsDir string, expected map[string]bool) error {
	entries, err := os.ReadDir(claudeAgentsDir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") || expected[entry.Name()] {
			continue
		}
		path := filepath.Join(claudeAgentsDir, entry.Name())
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			if err := os.Remove(path); err != nil {
				return err
			}
		}
	}
	return nil
}

const skillInjectionMarker = "## 주입된 스킬"

func buildSkillInjection(skillNames []string, skillMap map[string]*config.SkillConfig, progressiveDisclosure bool) string {
	var b strings.Builder
	injected := false

	for _, name := range skillNames {
		skill, ok := skillMap[name]
		if !ok {
			continue
		}

		if !injected {
			b.WriteString(skillInjectionMarker + "\n\n")
			injected = true
		}

		if progressiveDisclosure {
			// Metadata only: name + description (plain text — full body is deferred to file reference)
			b.WriteString(fmt.Sprintf("### %s\n", skill.Name))
			if skill.Description != "" {
				b.WriteString(fmt.Sprintf("%s\n", skill.Description))
			}
			b.WriteString(fmt.Sprintf("\n> 전체 내용은 `.pylon/skills/%s.md`를 참조하세요.\n\n", skill.Name))
		} else {
			// Full body injection (italic description to visually separate from body content)
			b.WriteString(fmt.Sprintf("### %s\n", skill.Name))
			if skill.Description != "" {
				b.WriteString(fmt.Sprintf("_%s_\n\n", skill.Description))
			}
			if skill.Body != "" {
				b.WriteString(skill.Body)
				b.WriteString("\n\n")
			}
		}
	}

	return b.String()
}

// syncClaudeAgentLinks ensures .claude/agents/ has symlinks for all .pylon/agents/*.md files.

// syncClaudeAgentLinks ensures .claude/agents/ has symlinks for all .pylon/agents/*.md files.
func syncClaudeAgentLinks(workDir, pylonDir string) error {
	claudeAgentsDir := layout.ClaudeAgentsDir(workDir)
	if err := os.MkdirAll(claudeAgentsDir, 0o755); err != nil {
		return err
	}
	entries, err := os.ReadDir(filepath.Join(pylonDir, "agents"))
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		if err := ensureSymlink(filepath.Join(claudeAgentsDir, entry.Name()), layout.AgentLinkTarget(entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

// ensureSymlink creates a managed symlink while preserving regular user files.
func ensureSymlink(linkPath, target string) error {
	info, err := os.Lstat(linkPath)
	switch {
	case err == nil && info.Mode()&os.ModeSymlink != 0:
		if err := os.Remove(linkPath); err != nil {
			return err
		}
	case err == nil:
		return nil
	case !os.IsNotExist(err):
		return err
	}
	if err := os.Symlink(target, linkPath); err != nil && !os.IsExist(err) {
		return err
	}
	return nil
}
