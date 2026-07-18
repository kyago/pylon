package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kyago/pylon/internal/config"
	"github.com/kyago/pylon/internal/layout"
)

// generateClaudeAgentsWithSkills generates .claude/agents/ files with skill injection.
// For agents with skills: generates a file with skill content appended to the body.
// For agents without skills: creates a symlink to .pylon/agents/ (default behavior).
// Respects SkillsConfig flags: Enabled, PreloadToAgents, ProgressiveDisclosure.
func generateClaudeAgentsWithSkills(root string, cfg *config.Config) error {
	pylonDir := layout.PylonDir(root)
	pylonAgentsDir := filepath.Join(pylonDir, "agents")
	claudeAgentsDir := layout.ClaudeAgentsDir(root)
	skillsDir := filepath.Join(pylonDir, "skills")

	if err := os.MkdirAll(claudeAgentsDir, 0755); err != nil {
		return err
	}

	entries, err := os.ReadDir(pylonAgentsDir)
	if err != nil {
		return nil // no agents directory, nothing to do
	}

	// Preload skill map for efficient lookup
	skillMap := make(map[string]*config.SkillConfig)
	if cfg.Skills.Enabled && cfg.Skills.PreloadToAgents {
		skills, err := config.DiscoverSkills(skillsDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "경고: 스킬 로드 실패: %v\n", err)
		}
		for _, s := range skills {
			skillMap[s.Name] = s
		}
	}

	// Track which agent files are expected so we can clean up stale entries
	expectedAgents := make(map[string]bool)

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		expectedAgents[entry.Name()] = true
		agentPath := filepath.Join(pylonAgentsDir, entry.Name())
		linkPath := filepath.Join(claudeAgentsDir, entry.Name())

		// Parse agent to check for skills
		agent, err := config.ParseAgentFile(agentPath)
		if err != nil {
			// Unparseable agent — fall back to symlink
			ensureSymlink(linkPath, layout.AgentLinkTarget(entry.Name()))
			continue
		}

		// If skills disabled or agent has no skills, use symlink
		if !cfg.Skills.Enabled || !cfg.Skills.PreloadToAgents || len(agent.Skills) == 0 {
			ensureSymlink(linkPath, layout.AgentLinkTarget(entry.Name()))
			continue
		}

		// Agent has skills — generate file with skill content injected
		content, err := os.ReadFile(agentPath)
		if err != nil {
			continue
		}

		injected := buildSkillInjection(agent.Skills, skillMap, cfg.Skills.ProgressiveDisclosure)
		if injected == "" {
			// No matching skills found, use symlink
			ensureSymlink(linkPath, layout.AgentLinkTarget(entry.Name()))
			continue
		}

		// Append skill section to agent content
		combined := string(content) + "\n\n" + injected

		// linkPath의 이름은 .pylon/agents/ 기반이므로 pylon 관리 에이전트다.
		// 심링크는 제거하고 최신 주입 내용으로 재생성한다. 일반 파일은 pylon이
		// 생성한 주입 파일(주입 마커 포함)만 갱신하고, 마커가 없는(사용자가 직접
		// 작성한) 파일은 보존한다. 이미 최신 내용이면 다시 쓰지 않는다.
		//
		// 계약: 주입 마커를 포함한 .claude/agents/ 파일은 pylon 소유로 간주되며,
		// 사용자가 이 파일을 직접 수정하더라도(마커가 남아 있는 한) update 시
		// 최신 주입 내용으로 덮어써진다. 에이전트 커스터마이징은 .claude/agents/가
		// 아니라 .pylon/agents/ 또는 .pylon/skills/에서 해야 한다. 마커가 없는
		// 파일(사용자가 처음부터 작성한 에이전트)은 절대 덮어쓰지 않는다.
		if info, err := os.Lstat(linkPath); err == nil {
			if info.Mode()&os.ModeSymlink != 0 {
				os.Remove(linkPath)
			} else {
				existing, rerr := os.ReadFile(linkPath)
				if rerr != nil || !strings.Contains(string(existing), skillInjectionMarker) {
					continue // 사용자 작성 파일(주입 마커 없음) — 보존
				}
				if string(existing) == combined {
					continue // 이미 최신 주입 내용 — 갱신 불필요
				}
			}
		}
		if err := os.WriteFile(linkPath, []byte(combined), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "경고: %s 에이전트 파일 생성 실패: %v\n", entry.Name(), err)
		}
	}

	// Clean up stale entries in .claude/agents/ that no longer exist in .pylon/agents/.
	// Only symlinks are removed — pylon-generated regular files (from skill injection) are
	// left in place because they cannot be reliably distinguished from user-created files
	// without a manifest. This is a known limitation; a manifest-based approach can be added later.
	//
	// Related known gap: if an agent that still exists in .pylon/agents/ loses all of its
	// skills between versions, it routes to the ensureSymlink path above, which does not
	// touch existing regular files — so a previously injected (marker-bearing) regular file
	// is left with stale content until manually removed. Not data loss; same manifest limitation.
	claudeEntries, err := os.ReadDir(claudeAgentsDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "경고: .claude/agents/ 읽기 실패: %v\n", err)
	}
	for _, ce := range claudeEntries {
		if ce.IsDir() || !strings.HasSuffix(ce.Name(), ".md") {
			continue
		}
		if expectedAgents[ce.Name()] {
			continue
		}
		stale := filepath.Join(claudeAgentsDir, ce.Name())
		info, err := os.Lstat(stale)
		if err != nil {
			continue
		}
		if info.Mode()&os.ModeSymlink != 0 {
			if err := os.Remove(stale); err != nil {
				fmt.Fprintf(os.Stderr, "경고: 스테일 심링크 제거 실패 %s: %v\n", ce.Name(), err)
			}
		}
	}

	return nil
}

// skillInjectionMarker is the header that marks a .claude/agents/ file as
// pylon-generated with injected skill content. It is used to distinguish
// pylon-managed injected files (safe to refresh on upgrade) from files a user
// authored by hand (preserved).
const skillInjectionMarker = "## 주입된 스킬"

// buildSkillInjection generates the skill content to inject into an agent file.
// If progressiveDisclosure is true, only metadata (name + description) is injected.
// If false, the full skill body is injected.
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
func syncClaudeAgentLinks(workDir, pylonDir string) error {
	claudeAgentsDir := layout.ClaudeAgentsDir(workDir)
	if err := os.MkdirAll(claudeAgentsDir, 0755); err != nil {
		return err
	}

	pylonAgentsDir := filepath.Join(pylonDir, "agents")
	entries, err := os.ReadDir(pylonAgentsDir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		// 기존 심링크는 제거 후 재생성, 일반 파일은 보존 (ensureSymlink 계약)
		ensureSymlink(filepath.Join(claudeAgentsDir, entry.Name()), layout.AgentLinkTarget(entry.Name()))
	}

	return nil
}

// ensureSymlink creates a symlink at linkPath pointing to target.
// If a symlink already exists, it is removed and recreated.
// Regular files are left untouched.
func ensureSymlink(linkPath, target string) {
	if info, err := os.Lstat(linkPath); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			os.Remove(linkPath)
		} else {
			return // regular file, don't touch
		}
	}
	if err := os.Symlink(target, linkPath); err != nil && !os.IsExist(err) {
		fmt.Fprintf(os.Stderr, "경고: 심링크 생성 실패 %s: %v\n", filepath.Base(linkPath), err)
	}
}
