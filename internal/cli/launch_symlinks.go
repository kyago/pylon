package cli

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/kyago/pylon/internal/layout"
)

// agentSource is one agent definition and the .claude/agents/ link target for it.
type agentSource struct {
	path   string
	target string
}

// workspaceAgents lists the user's agent definitions in .pylon/agents/ keyed by file
// name. Pylon ships no agents of its own.
func workspaceAgents(root string) (map[string]agentSource, error) {
	agents := make(map[string]agentSource)
	dir := layout.ResourceDir(root, "agents")
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return agents, nil
	}
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		agents[entry.Name()] = agentSource{filepath.Join(dir, entry.Name()), layout.AgentLinkTarget(entry.Name())}
	}
	return agents, nil
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

// skillInjectionMarker identifies agent files that releases with skill injection wrote into .claude/agents/.
const skillInjectionMarker = "## 주입된 스킬"

// syncClaudeAgentLinks ensures .claude/agents/ has a symlink for every user agent and
// prunes managed entries for agents that no longer exist, including the links earlier
// releases made for built-in agents.
func syncClaudeAgentLinks(root string) error {
	claudeAgentsDir := layout.ClaudeAgentsDir(root)
	if err := os.MkdirAll(claudeAgentsDir, 0o755); err != nil {
		return err
	}
	agents, err := workspaceAgents(root)
	if err != nil {
		return err
	}
	expected := make(map[string]bool, len(agents))
	for name, agent := range agents {
		expected[name] = true
		if err := ensureSymlink(filepath.Join(claudeAgentsDir, name), agent.target); err != nil {
			return err
		}
	}
	return removeStaleAgentLinks(claudeAgentsDir, expected)
}

// syncClaudeSkillLinks links every user skill (.pylon/skills/<name>/SKILL.md) into
// .claude/skills/ and removes links into .pylon/skills/ whose skill is gone. Entries
// that are not such links belong to the user and stay.
func syncClaudeSkillLinks(root string) error {
	claudeSkillsDir := layout.ClaudeSkillsDir(root)
	// 사용자가 .claude/skills 자체를 다른 곳으로 링크했으면 그 디렉터리는 건드리지 않는다.
	if info, err := os.Lstat(claudeSkillsDir); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return nil
	}
	skillsDir := layout.ResourceDir(root, "skills")
	entries, err := os.ReadDir(skillsDir)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	expected := make(map[string]bool)
	for _, entry := range entries {
		if _, err := os.Stat(filepath.Join(skillsDir, entry.Name(), "SKILL.md")); err != nil {
			continue
		}
		if err := os.MkdirAll(claudeSkillsDir, 0o755); err != nil {
			return err
		}
		expected[entry.Name()] = true
		linkPath, target := filepath.Join(claudeSkillsDir, entry.Name()), layout.SkillLinkTarget(entry.Name())
		if cur, err := os.Readlink(linkPath); err == nil && cur != target {
			continue // 같은 이름의 사용자 링크는 바꾸지 않는다
		}
		if err := ensureSymlink(linkPath, target); err != nil {
			return err
		}
	}
	links, err := os.ReadDir(claudeSkillsDir)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	for _, link := range links {
		path := filepath.Join(claudeSkillsDir, link.Name())
		target, err := os.Readlink(path)
		if err != nil || expected[link.Name()] || target != layout.SkillLinkTarget(link.Name()) {
			continue
		}
		if err := os.Remove(path); err != nil {
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
