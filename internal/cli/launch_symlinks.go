package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kyago/pylon/internal/config"
	"github.com/kyago/pylon/internal/layout"
)

// agentSource is one agent definition and the .claude/agents/ link target for it.
type agentSource struct {
	path   string
	target string
}

// workspaceAgents lists the agent definitions keyed by file name: built-ins from
// .pylon/builtin/agents/, then user-added ones from .pylon/agents/. A built-in name
// always wins — see syncBuiltinResources.
func workspaceAgents(root string) (map[string]agentSource, error) {
	agents := make(map[string]agentSource)
	for _, src := range []struct {
		dir    string
		target func(string) string
	}{
		{layout.BuiltinResourceDir(root, "agents"), layout.BuiltinAgentLinkTarget},
		{layout.ResourceDir(root, "agents"), layout.AgentLinkTarget},
	} {
		entries, err := os.ReadDir(src.dir)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
				continue
			}
			if _, taken := agents[entry.Name()]; !taken {
				agents[entry.Name()] = agentSource{filepath.Join(src.dir, entry.Name()), src.target(entry.Name())}
			}
		}
	}
	return agents, nil
}

func generateClaudeAgentsWithSkills(root string, cfg *config.Config) error {
	claudeAgentsDir := layout.ClaudeAgentsDir(root)
	if err := os.MkdirAll(claudeAgentsDir, 0o755); err != nil {
		return err
	}
	agents, err := workspaceAgents(root)
	if err != nil {
		return err
	}
	skillMap, err := loadAgentSkillMap(root, cfg)
	if err != nil {
		return err
	}
	expected := make(map[string]bool)
	for name, agent := range agents {
		expected[name] = true
		if err := materializeClaudeAgent(
			agent.path, filepath.Join(claudeAgentsDir, name), agent.target, cfg, skillMap,
		); err != nil {
			return fmt.Errorf("%s 에이전트 동기화 실패: %w", name, err)
		}
	}
	return removeStaleAgentLinks(claudeAgentsDir, expected)
}

// loadAgentSkillMap discovers skills from .pylon/builtin/skills/, then user-added
// .pylon/skills/ (a built-in name wins). FilePath is rewritten relative to root so
// progressive disclosure can point the agent at the file.
func loadAgentSkillMap(root string, cfg *config.Config) (map[string]*config.SkillConfig, error) {
	skillMap := make(map[string]*config.SkillConfig)
	if !cfg.Skills.Enabled || !cfg.Skills.PreloadToAgents {
		return skillMap, nil
	}
	for _, dir := range []string{layout.BuiltinResourceDir(root, "skills"), layout.ResourceDir(root, "skills")} {
		skills, err := config.DiscoverSkills(dir)
		if err != nil {
			return nil, fmt.Errorf("스킬 로드 실패: %w", err)
		}
		for _, skill := range skills {
			if _, taken := skillMap[skill.Name]; taken {
				continue
			}
			if rel, err := filepath.Rel(root, skill.FilePath); err == nil {
				skill.FilePath = filepath.ToSlash(rel)
			}
			skillMap[skill.Name] = skill
		}
	}
	return skillMap, nil
}

func materializeClaudeAgent(agentPath, linkPath, target string, cfg *config.Config, skillMap map[string]*config.SkillConfig) error {
	agent, err := config.ParseAgentFile(agentPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "경고: 에이전트 정의 파싱 실패 %s: %v\n", agentPath, err)
		return ensureSymlink(linkPath, target)
	}
	if !cfg.Skills.Enabled || !cfg.Skills.PreloadToAgents || len(agent.Skills) == 0 {
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
		// 관리 대상은 심링크와 스킬을 주입해 만든 파일이다 — 그 밖의 사용자 파일은 남긴다.
		managed := info.Mode()&os.ModeSymlink != 0
		if !managed && info.Mode().IsRegular() {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			managed = strings.Contains(string(data), skillInjectionMarker)
		}
		if managed {
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
			b.WriteString(fmt.Sprintf("\n> 전체 내용은 `%s`를 참조하세요.\n\n", skill.FilePath))
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

// syncClaudeAgentLinks ensures .claude/agents/ has symlinks for every workspace agent
// (built-in and user-added).
func syncClaudeAgentLinks(root string) error {
	claudeAgentsDir := layout.ClaudeAgentsDir(root)
	if err := os.MkdirAll(claudeAgentsDir, 0o755); err != nil {
		return err
	}
	agents, err := workspaceAgents(root)
	if err != nil {
		return err
	}
	for name, agent := range agents {
		if err := ensureSymlink(filepath.Join(claudeAgentsDir, name), agent.target); err != nil {
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
