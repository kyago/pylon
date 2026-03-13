package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// VerifyStep defines a single verification step from verify.yml.
type VerifyStep struct {
	Command string `yaml:"command"`
	Timeout string `yaml:"timeout"`
}

// VerifyConfig holds the parsed verify.yml configuration.
// Spec Reference: Section 7 "pylon request" step 9 (cross-validation)
type VerifyConfig struct {
	Build *VerifyStep `yaml:"build"`
	Test  *VerifyStep `yaml:"test"`
	Lint  *VerifyStep `yaml:"lint"`
}

// NamedVerifyStep is a verify step with its category name.
type NamedVerifyStep struct {
	Name    string
	Command string
	Timeout string
}

// LoadVerifyConfig reads and parses a verify.yml file.
func LoadVerifyConfig(path string) (*VerifyConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read verify config: %w", err)
	}

	vc := &VerifyConfig{}
	if err := yaml.Unmarshal(data, vc); err != nil {
		return nil, fmt.Errorf("failed to parse verify config: %w", err)
	}

	return vc, nil
}

// OrderedSteps returns the verify steps in execution order: build → test → lint.
// Missing categories are skipped. Empty commands are skipped.
// Default timeout "60s" is applied when not specified.
func (vc *VerifyConfig) OrderedSteps() []NamedVerifyStep {
	var steps []NamedVerifyStep

	sources := []struct {
		name string
		step *VerifyStep
	}{
		{"build", vc.Build},
		{"test", vc.Test},
		{"lint", vc.Lint},
	}

	for _, s := range sources {
		if s.step == nil || s.step.Command == "" {
			continue
		}
		timeout := s.step.Timeout
		if timeout == "" {
			timeout = "60s"
		}
		steps = append(steps, NamedVerifyStep{
			Name:    s.name,
			Command: s.step.Command,
			Timeout: timeout,
		})
	}

	return steps
}
