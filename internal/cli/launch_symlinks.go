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

// syncClaudeAgentLinks ensures .claude/agents/ has symlinks for every workspace agent
// (built-in and user-added) and prunes managed entries for agents that no longer exist.
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
