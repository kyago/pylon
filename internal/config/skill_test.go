package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseSkillData_Valid(t *testing.T) {
	data := []byte(`---
name: research-methodology
description: Research methodology guide for multi-source investigation
---

# Research Methodology

## Steps
1. Define research question
2. Search multiple sources
3. Cross-validate findings
`)

	skill, err := ParseSkillData(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if skill.Name != "research-methodology" {
		t.Errorf("expected name 'research-methodology', got %q", skill.Name)
	}
	if skill.Description != "Research methodology guide for multi-source investigation" {
		t.Errorf("unexpected description: %q", skill.Description)
	}
	if skill.Body == "" {
		t.Error("expected non-empty body")
	}
	if !contains(skill.Body, "Research Methodology") {
		t.Errorf("body should contain 'Research Methodology', got %q", skill.Body)
	}
}

func TestParseSkillData_MissingName(t *testing.T) {
	data := []byte(`---
description: some skill
---

body content
`)

	_, err := ParseSkillData(data)
	if err == nil {
		t.Fatal("expected error for missing name")
	}
	if !contains(err.Error(), "'name' is required") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestParseSkillData_NoFrontmatter(t *testing.T) {
	data := []byte("just plain text")

	_, err := ParseSkillData(data)
	if err == nil {
		t.Fatal("expected error for missing frontmatter")
	}
}

func TestParseSkillFile(t *testing.T) {
	dir := t.TempDir()

	// Create skill file
	skillContent := `---
name: test-skill
description: A test skill
---

Test body content.
`
	skillPath := filepath.Join(dir, "test-skill.md")
	os.WriteFile(skillPath, []byte(skillContent), 0644)

	skill, err := ParseSkillFile(skillPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if skill.Name != "test-skill" {
		t.Errorf("expected name 'test-skill', got %q", skill.Name)
	}
	if skill.FilePath != skillPath {
		t.Errorf("expected FilePath %q, got %q", skillPath, skill.FilePath)
	}
}

func TestParseSkillFile_WithReferences(t *testing.T) {
	dir := t.TempDir()

	// Create skill file
	skillContent := `---
name: skill-with-refs
description: A skill with references
---

Skill body.
`
	os.WriteFile(filepath.Join(dir, "skill-with-refs.md"), []byte(skillContent), 0644)

	// Create references directory
	refDir := filepath.Join(dir, "references")
	os.MkdirAll(refDir, 0755)
	os.WriteFile(filepath.Join(refDir, "guide.md"), []byte("# Guide"), 0644)
	os.WriteFile(filepath.Join(refDir, "patterns.md"), []byte("# Patterns"), 0644)
	os.WriteFile(filepath.Join(refDir, "notes.txt"), []byte("not a markdown"), 0644) // should be skipped

	skill, err := ParseSkillFile(filepath.Join(dir, "skill-with-refs.md"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(skill.References) != 2 {
		t.Errorf("expected 2 references, got %d: %v", len(skill.References), skill.References)
	}
}

func TestDiscoverSkills(t *testing.T) {
	dir := t.TempDir()

	// Create valid skill files
	skills := map[string]string{
		"skill-a.md": "---\nname: skill-a\ndescription: A\n---\nBody A",
		"skill-b.md": "---\nname: skill-b\ndescription: B\n---\nBody B",
		"not-a-skill.txt": "plain text",
	}
	for name, content := range skills {
		os.WriteFile(filepath.Join(dir, name), []byte(content), 0644)
	}

	discovered, err := DiscoverSkills(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(discovered) != 2 {
		t.Errorf("expected 2 skills, got %d", len(discovered))
	}
}

func TestDiscoverSkills_NonexistentDir(t *testing.T) {
	skills, err := DiscoverSkills("/nonexistent/dir")
	if err != nil {
		t.Fatalf("expected nil error for nonexistent dir, got: %v", err)
	}
	if skills != nil {
		t.Errorf("expected nil skills, got %d", len(skills))
	}
}

// contains is defined in agent_test.go (same package)
