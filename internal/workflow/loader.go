package workflow

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"gopkg.in/yaml.v3"
)

// validWorkflowName restricts workflow names to safe characters only.
var validWorkflowName = regexp.MustCompile(`^[a-z0-9_-]+$`)

//go:embed templates/*.yml
var embeddedTemplates embed.FS

// LoadWorkflow loads a workflow template by name.
// It first checks the custom templateDir, then falls back to embedded templates.
func LoadWorkflow(name string, templateDir string) (*WorkflowTemplate, error) {
	// Validate workflow name to prevent path traversal
	if !validWorkflowName.MatchString(name) {
		return nil, fmt.Errorf("invalid workflow name %q: must match [a-z0-9_-]+", name)
	}

	// Try custom template directory first
	if templateDir != "" {
		path := filepath.Join(templateDir, name+".yml")
		data, err := os.ReadFile(path)
		if err == nil {
			return parseTemplate(data)
		}
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to read workflow template %s: %w", path, err)
		}
	}

	// Fall back to embedded templates
	data, err := embeddedTemplates.ReadFile("templates/" + name + ".yml")
	if err != nil {
		return nil, fmt.Errorf("workflow template %q not found", name)
	}
	return parseTemplate(data)
}

// AvailableWorkflows returns the names of all built-in workflow templates.
func AvailableWorkflows() []string {
	entries, err := embeddedTemplates.ReadDir("templates")
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		name := e.Name()
		if filepath.Ext(name) == ".yml" {
			names = append(names, name[:len(name)-4])
		}
	}
	return names
}

func parseTemplate(data []byte) (*WorkflowTemplate, error) {
	var t WorkflowTemplate
	if err := yaml.Unmarshal(data, &t); err != nil {
		return nil, fmt.Errorf("failed to parse workflow template: %w", err)
	}
	if len(t.Stages) == 0 {
		return nil, fmt.Errorf("workflow template %q has no stages", t.Name)
	}
	return &t, nil
}
