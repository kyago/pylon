package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// SkillConfig represents a skill definition parsed from a .md file.
type SkillConfig struct {
	// YAML frontmatter fields
	Name        string `yaml:"name"`
	Description string `yaml:"description"`

	// Markdown body (everything after the second ---)
	Body string `yaml:"-"`
	// Source file path
	FilePath string `yaml:"-"`
	// Discovered reference file paths (populated by DiscoverSkills)
	References []string `yaml:"-"`
}

// ParseSkillFile reads a .md skill file and extracts YAML frontmatter + markdown body.
func ParseSkillFile(path string) (*SkillConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read skill file %s: %w", path, err)
	}

	skill, err := ParseSkillData(data)
	if err != nil {
		return nil, fmt.Errorf("failed to parse skill file %s: %w", path, err)
	}
	skill.FilePath = path

	// Discover references/ directory next to the skill file
	refDir := filepath.Join(filepath.Dir(path), "references")
	if entries, err := os.ReadDir(refDir); err == nil {
		for _, entry := range entries {
			if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".md") {
				skill.References = append(skill.References, filepath.Join(refDir, entry.Name()))
			}
		}
	}

	return skill, nil
}

// ParseSkillData parses skill configuration from raw bytes.
func ParseSkillData(data []byte) (*SkillConfig, error) {
	content := string(data)
	frontmatter, body, err := splitFrontmatter(content)
	if err != nil {
		return nil, err
	}

	skill := &SkillConfig{}
	if err := yaml.Unmarshal([]byte(frontmatter), skill); err != nil {
		return nil, fmt.Errorf("failed to parse skill frontmatter YAML: %w", err)
	}

	skill.Body = body

	if skill.Name == "" {
		return nil, fmt.Errorf("skill validation error: 'name' is required")
	}

	return skill, nil
}

// DiscoverSkills reads all .md skill files from a directory.
func DiscoverSkills(dir string) ([]*SkillConfig, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read skills directory %s: %w", dir, err)
	}

	var skills []*SkillConfig
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		skill, err := ParseSkillFile(path)
		if err != nil {
			continue // skip unparseable skill files
		}
		skills = append(skills, skill)
	}
	return skills, nil
}
