package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kyago/pylon/internal/config"
)

func TestBuildSkillInjection_EmptySkills(t *testing.T) {
	skillMap := map[string]*config.SkillConfig{}
	result := buildSkillInjection(nil, skillMap, false)
	if result != "" {
		t.Errorf("nil skills should return empty string, got %q", result)
	}

	result = buildSkillInjection([]string{}, skillMap, false)
	if result != "" {
		t.Errorf("empty skills should return empty string, got %q", result)
	}
}

func TestBuildSkillInjection_NoMatchingSkill(t *testing.T) {
	skillMap := map[string]*config.SkillConfig{
		"other-skill": {Name: "other-skill", Description: "Other", Body: "body"},
	}
	result := buildSkillInjection([]string{"nonexistent"}, skillMap, false)
	if result != "" {
		t.Errorf("no matching skill should return empty string, got %q", result)
	}
}

func TestBuildSkillInjection_FullInjection(t *testing.T) {
	skillMap := map[string]*config.SkillConfig{
		"research-methodology": {
			Name:        "research-methodology",
			Description: "Research methodology guide",
			Body:        "## Step 1\nDo research\n\n## Step 2\nAnalyze results",
		},
	}

	result := buildSkillInjection([]string{"research-methodology"}, skillMap, false)

	if !strings.Contains(result, "## 주입된 스킬") {
		t.Error("should contain skill injection header")
	}
	if !strings.Contains(result, "### research-methodology") {
		t.Error("should contain skill name as heading")
	}
	if !strings.Contains(result, "_Research methodology guide_") {
		t.Error("should contain skill description in italics")
	}
	if !strings.Contains(result, "## Step 1") {
		t.Error("should contain full skill body")
	}
}

func TestBuildSkillInjection_ProgressiveDisclosure(t *testing.T) {
	skillMap := map[string]*config.SkillConfig{
		"research-methodology": {
			Name:        "research-methodology",
			Description: "Research methodology guide",
			Body:        "## Step 1\nDo research",
		},
	}

	result := buildSkillInjection([]string{"research-methodology"}, skillMap, true)

	if !strings.Contains(result, "## 주입된 스킬") {
		t.Error("should contain skill injection header")
	}
	if !strings.Contains(result, "### research-methodology") {
		t.Error("should contain skill name")
	}
	if !strings.Contains(result, "Research methodology guide") {
		t.Error("should contain description")
	}
	if !strings.Contains(result, ".pylon/skills/research-methodology.md") {
		t.Error("should contain reference to full skill file")
	}
	// Should NOT contain the full body
	if strings.Contains(result, "## Step 1") {
		t.Error("progressive disclosure should not include full body")
	}
}

func TestBuildSkillInjection_MultipleSkills(t *testing.T) {
	skillMap := map[string]*config.SkillConfig{
		"skill-a": {Name: "skill-a", Description: "Skill A", Body: "Body A"},
		"skill-b": {Name: "skill-b", Description: "Skill B", Body: "Body B"},
	}

	result := buildSkillInjection([]string{"skill-a", "skill-b"}, skillMap, false)

	if strings.Count(result, "## 주입된 스킬") != 1 {
		t.Error("should have exactly one injection header")
	}
	if !strings.Contains(result, "### skill-a") {
		t.Error("should contain skill-a")
	}
	if !strings.Contains(result, "### skill-b") {
		t.Error("should contain skill-b")
	}
	if !strings.Contains(result, "Body A") {
		t.Error("should contain skill-a body")
	}
	if !strings.Contains(result, "Body B") {
		t.Error("should contain skill-b body")
	}
}

func TestGenerateClaudeAgentsWithSkills_SkillsDisabled(t *testing.T) {
	root := t.TempDir()
	pylonDir := filepath.Join(root, ".pylon")
	agentsDir := filepath.Join(pylonDir, "agents")
	claudeAgentsDir := filepath.Join(root, ".claude", "agents")
	os.MkdirAll(agentsDir, 0755)
	os.MkdirAll(claudeAgentsDir, 0755)

	// Write a test agent with skills
	agentContent := `---
name: test-agent
role: Test Agent
skills:
  - some-skill
---
# Test Agent`
	os.WriteFile(filepath.Join(agentsDir, "test-agent.md"), []byte(agentContent), 0644)

	// Write a test skill
	skillsDir := filepath.Join(pylonDir, "skills")
	os.MkdirAll(skillsDir, 0755)
	skillContent := `---
name: some-skill
description: A test skill
---
Skill body content`
	os.WriteFile(filepath.Join(skillsDir, "some-skill.md"), []byte(skillContent), 0644)

	cfg := &config.Config{
		Skills: config.SkillsConfig{
			Enabled:               false,
			PreloadToAgents:       true,
			ProgressiveDisclosure: false,
		},
	}

	err := generateClaudeAgentsWithSkills(root, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should be a symlink, not a file with injected content
	linkPath := filepath.Join(claudeAgentsDir, "test-agent.md")
	info, err := os.Lstat(linkPath)
	if err != nil {
		t.Fatalf("link should exist: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Error("when skills disabled, agent should be a symlink")
	}
}

func TestGenerateClaudeAgentsWithSkills_WithInjection(t *testing.T) {
	root := t.TempDir()
	pylonDir := filepath.Join(root, ".pylon")
	agentsDir := filepath.Join(pylonDir, "agents")
	claudeAgentsDir := filepath.Join(root, ".claude", "agents")
	os.MkdirAll(agentsDir, 0755)
	os.MkdirAll(claudeAgentsDir, 0755)

	// Write a test agent with skills
	agentContent := `---
name: test-agent
role: Test Agent
skills:
  - some-skill
---
# Test Agent

Do agent things.`
	os.WriteFile(filepath.Join(agentsDir, "test-agent.md"), []byte(agentContent), 0644)

	// Write a test skill
	skillsDir := filepath.Join(pylonDir, "skills")
	os.MkdirAll(skillsDir, 0755)
	skillContent := `---
name: some-skill
description: A test skill
---
Skill body content here`
	os.WriteFile(filepath.Join(skillsDir, "some-skill.md"), []byte(skillContent), 0644)

	cfg := &config.Config{
		Skills: config.SkillsConfig{
			Enabled:               true,
			PreloadToAgents:       true,
			ProgressiveDisclosure: false,
		},
	}

	err := generateClaudeAgentsWithSkills(root, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should be a regular file, not a symlink
	linkPath := filepath.Join(claudeAgentsDir, "test-agent.md")
	info, err := os.Lstat(linkPath)
	if err != nil {
		t.Fatalf("file should exist: %v", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Error("with skill injection, agent should be a regular file, not symlink")
	}

	// Check content has both agent and skill
	content, _ := os.ReadFile(linkPath)
	if !strings.Contains(string(content), "# Test Agent") {
		t.Error("should contain original agent content")
	}
	if !strings.Contains(string(content), "## 주입된 스킬") {
		t.Error("should contain injected skill header")
	}
	if !strings.Contains(string(content), "Skill body content here") {
		t.Error("should contain skill body")
	}
}

func TestGenerateClaudeAgentsWithSkills_UserFileProtected(t *testing.T) {
	root := t.TempDir()
	pylonDir := filepath.Join(root, ".pylon")
	agentsDir := filepath.Join(pylonDir, "agents")
	claudeAgentsDir := filepath.Join(root, ".claude", "agents")
	os.MkdirAll(agentsDir, 0755)
	os.MkdirAll(claudeAgentsDir, 0755)

	// Write a test agent with skills
	agentContent := `---
name: test-agent
role: Test Agent
skills:
  - some-skill
---
# Test Agent`
	os.WriteFile(filepath.Join(agentsDir, "test-agent.md"), []byte(agentContent), 0644)

	// Write a test skill
	skillsDir := filepath.Join(pylonDir, "skills")
	os.MkdirAll(skillsDir, 0755)
	skillContent := `---
name: some-skill
description: A test skill
---
Skill body`
	os.WriteFile(filepath.Join(skillsDir, "some-skill.md"), []byte(skillContent), 0644)

	// Pre-create a regular file (user-created) in .claude/agents/
	userContent := "user custom agent content"
	os.WriteFile(filepath.Join(claudeAgentsDir, "test-agent.md"), []byte(userContent), 0644)

	cfg := &config.Config{
		Skills: config.SkillsConfig{
			Enabled:               true,
			PreloadToAgents:       true,
			ProgressiveDisclosure: false,
		},
	}

	err := generateClaudeAgentsWithSkills(root, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// User file should be preserved, not overwritten
	content, _ := os.ReadFile(filepath.Join(claudeAgentsDir, "test-agent.md"))
	if string(content) != userContent {
		t.Errorf("user file should be preserved, got %q", string(content))
	}
}

func TestGenerateClaudeAgentsWithSkills_StaleCleanup(t *testing.T) {
	root := t.TempDir()
	pylonDir := filepath.Join(root, ".pylon")
	agentsDir := filepath.Join(pylonDir, "agents")
	claudeAgentsDir := filepath.Join(root, ".claude", "agents")
	os.MkdirAll(agentsDir, 0755)
	os.MkdirAll(claudeAgentsDir, 0755)

	// Write one agent in .pylon/agents/
	agentContent := `---
name: active-agent
role: Active Agent
---
# Active Agent`
	os.WriteFile(filepath.Join(agentsDir, "active-agent.md"), []byte(agentContent), 0644)

	// Create a stale symlink in .claude/agents/ (agent was deleted from .pylon/agents/)
	staleLink := filepath.Join(claudeAgentsDir, "deleted-agent.md")
	os.Symlink(filepath.Join("..", "..", ".pylon", "agents", "deleted-agent.md"), staleLink)

	cfg := &config.Config{
		Skills: config.SkillsConfig{
			Enabled:               true,
			PreloadToAgents:       true,
			ProgressiveDisclosure: false,
		},
	}

	err := generateClaudeAgentsWithSkills(root, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Stale symlink should be removed
	if _, err := os.Lstat(staleLink); !os.IsNotExist(err) {
		t.Error("stale symlink should be removed")
	}

	// Active agent should still exist
	if _, err := os.Lstat(filepath.Join(claudeAgentsDir, "active-agent.md")); err != nil {
		t.Error("active agent should still exist")
	}
}

func TestGenerateClaudeAgentsWithSkills_PreloadDisabled(t *testing.T) {
	root := t.TempDir()
	pylonDir := filepath.Join(root, ".pylon")
	agentsDir := filepath.Join(pylonDir, "agents")
	claudeAgentsDir := filepath.Join(root, ".claude", "agents")
	os.MkdirAll(agentsDir, 0755)
	os.MkdirAll(claudeAgentsDir, 0755)

	agentContent := `---
name: test-agent
role: Test Agent
skills:
  - some-skill
---
# Test Agent`
	os.WriteFile(filepath.Join(agentsDir, "test-agent.md"), []byte(agentContent), 0644)

	skillsDir := filepath.Join(pylonDir, "skills")
	os.MkdirAll(skillsDir, 0755)
	skillContent := `---
name: some-skill
description: A test skill
---
Skill body`
	os.WriteFile(filepath.Join(skillsDir, "some-skill.md"), []byte(skillContent), 0644)

	cfg := &config.Config{
		Skills: config.SkillsConfig{
			Enabled:               true,
			PreloadToAgents:       false,
			ProgressiveDisclosure: false,
		},
	}

	err := generateClaudeAgentsWithSkills(root, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	linkPath := filepath.Join(claudeAgentsDir, "test-agent.md")
	info, err := os.Lstat(linkPath)
	if err != nil {
		t.Fatalf("link should exist: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Error("when preload disabled, agent should be a symlink")
	}
}
