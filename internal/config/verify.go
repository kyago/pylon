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
	Build    *VerifyStep      `yaml:"build"`
	Test     *VerifyStep      `yaml:"test"`
	Lint     *VerifyStep      `yaml:"lint"`
	Commands NamedVerifySteps `yaml:"commands"`
	HeldOut  NamedVerifySteps `yaml:"held_out"`
}

const (
	VerifyKindDeterministic = "deterministic"
	VerifyKindHeldOut       = "held_out"
)

// NamedVerifyStep is a verify step with its category name.
type NamedVerifyStep struct {
	Name    string `yaml:"name" json:"name"`
	Kind    string `yaml:"-" json:"kind"`
	Command string `yaml:"command" json:"command"`
	Timeout string `yaml:"timeout" json:"timeout"`
}

// NamedVerifySteps accepts both authoring forms for commands/held_out:
//
//	commands:              commands:
//	  - name: build          build:
//	    command: ...           command: ...
//
// 맵 형태는 사람이나 AI 세션이 자연스럽게 쓰는 변형이라 거부하는 대신
// 키를 name으로 삼아 받아들인다. 문서 순서는 두 형태 모두 유지된다.
type NamedVerifySteps []NamedVerifyStep

func (s *NamedVerifySteps) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.SequenceNode:
		var list []NamedVerifyStep
		if err := node.Decode(&list); err != nil {
			return err
		}
		*s = list
		return nil
	case yaml.MappingNode:
		steps := make([]NamedVerifyStep, 0, len(node.Content)/2)
		for i := 0; i+1 < len(node.Content); i += 2 {
			var step NamedVerifyStep
			if err := node.Content[i+1].Decode(&step); err != nil {
				return err
			}
			if step.Name == "" {
				step.Name = node.Content[i].Value
			}
			steps = append(steps, step)
		}
		*s = steps
		return nil
	default:
		return fmt.Errorf("verify steps must be a list or a map of name to step (line %d)", node.Line)
	}
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
	if len(vc.Commands) > 0 {
		steps := make([]NamedVerifyStep, 0, len(vc.Commands))
		for _, step := range vc.Commands {
			if step.Command == "" {
				continue
			}
			if step.Timeout == "" {
				step.Timeout = "60s"
			}
			step.Kind = VerifyKindDeterministic
			steps = append(steps, step)
		}
		return steps
	}

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
			Kind:    VerifyKindDeterministic,
			Command: s.step.Command,
			Timeout: timeout,
		})
	}

	return steps
}

func (vc *VerifyConfig) OrderedHeldOutSteps() []NamedVerifyStep {
	steps := make([]NamedVerifyStep, 0, len(vc.HeldOut))
	for _, step := range vc.HeldOut {
		if step.Command == "" {
			continue
		}
		if step.Timeout == "" {
			step.Timeout = "60s"
		}
		step.Kind = VerifyKindHeldOut
		steps = append(steps, step)
	}
	return steps
}
