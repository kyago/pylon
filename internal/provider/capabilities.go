package provider

import (
	"fmt"
	"sort"
)

type Capability string

const (
	CapabilityReadFiles           Capability = "read_files"
	CapabilityEditFiles           Capability = "edit_files"
	CapabilityRunShell            Capability = "run_shell"
	CapabilitySpawnSubagents      Capability = "spawn_subagents"
	CapabilityBackgroundExecution Capability = "background_execution"
	CapabilitySessionResume       Capability = "session_resume"
	CapabilityWorktreeIsolation   Capability = "worktree_isolation"
	CapabilityStructuredOutput    Capability = "structured_output"
	CapabilityToolRestrictions    Capability = "tool_restrictions"
	CapabilityWebResearch         Capability = "web_research"
)

var knownCapabilities = []Capability{
	CapabilityReadFiles,
	CapabilityEditFiles,
	CapabilityRunShell,
	CapabilitySpawnSubagents,
	CapabilityBackgroundExecution,
	CapabilitySessionResume,
	CapabilityWorktreeIsolation,
	CapabilityStructuredOutput,
	CapabilityToolRestrictions,
	CapabilityWebResearch,
}

type CapabilitySet struct {
	ReadFiles           bool `json:"read_files" yaml:"read_files"`
	EditFiles           bool `json:"edit_files" yaml:"edit_files"`
	RunShell            bool `json:"run_shell" yaml:"run_shell"`
	SpawnSubagents      bool `json:"spawn_subagents" yaml:"spawn_subagents"`
	BackgroundExecution bool `json:"background_execution" yaml:"background_execution"`
	SessionResume       bool `json:"session_resume" yaml:"session_resume"`
	WorktreeIsolation   bool `json:"worktree_isolation" yaml:"worktree_isolation"`
	StructuredOutput    bool `json:"structured_output" yaml:"structured_output"`
	ToolRestrictions    bool `json:"tool_restrictions" yaml:"tool_restrictions"`
	WebResearch         bool `json:"web_research" yaml:"web_research"`
}

func ParseCapabilities(names []string) (CapabilitySet, error) {
	var result CapabilitySet
	for _, name := range names {
		capability := Capability(name)
		if !isKnownCapability(capability) {
			return CapabilitySet{}, fmt.Errorf("unknown capability: %s", name)
		}
		result.set(capability, true)
	}
	return result, nil
}

func (capabilities CapabilitySet) Satisfies(required CapabilitySet) bool {
	return len(capabilities.Missing(required)) == 0
}

func (capabilities CapabilitySet) Missing(required CapabilitySet) []Capability {
	missing := make([]Capability, 0)
	for _, capability := range knownCapabilities {
		if required.has(capability) && !capabilities.has(capability) {
			missing = append(missing, capability)
		}
	}
	return missing
}

func (capabilities CapabilitySet) Names() []string {
	names := make([]string, 0)
	for _, capability := range knownCapabilities {
		if capabilities.has(capability) {
			names = append(names, string(capability))
		}
	}
	sort.Strings(names)
	return names
}

func isKnownCapability(capability Capability) bool {
	for _, known := range knownCapabilities {
		if capability == known {
			return true
		}
	}
	return false
}

func (capabilities CapabilitySet) has(capability Capability) bool {
	switch capability {
	case CapabilityReadFiles:
		return capabilities.ReadFiles
	case CapabilityEditFiles:
		return capabilities.EditFiles
	case CapabilityRunShell:
		return capabilities.RunShell
	case CapabilitySpawnSubagents:
		return capabilities.SpawnSubagents
	case CapabilityBackgroundExecution:
		return capabilities.BackgroundExecution
	case CapabilitySessionResume:
		return capabilities.SessionResume
	case CapabilityWorktreeIsolation:
		return capabilities.WorktreeIsolation
	case CapabilityStructuredOutput:
		return capabilities.StructuredOutput
	case CapabilityToolRestrictions:
		return capabilities.ToolRestrictions
	case CapabilityWebResearch:
		return capabilities.WebResearch
	default:
		return false
	}
}

func (capabilities *CapabilitySet) set(capability Capability, value bool) {
	switch capability {
	case CapabilityReadFiles:
		capabilities.ReadFiles = value
	case CapabilityEditFiles:
		capabilities.EditFiles = value
	case CapabilityRunShell:
		capabilities.RunShell = value
	case CapabilitySpawnSubagents:
		capabilities.SpawnSubagents = value
	case CapabilityBackgroundExecution:
		capabilities.BackgroundExecution = value
	case CapabilitySessionResume:
		capabilities.SessionResume = value
	case CapabilityWorktreeIsolation:
		capabilities.WorktreeIsolation = value
	case CapabilityStructuredOutput:
		capabilities.StructuredOutput = value
	case CapabilityToolRestrictions:
		capabilities.ToolRestrictions = value
	case CapabilityWebResearch:
		capabilities.WebResearch = value
	}
}
