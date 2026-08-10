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

func (available CapabilitySet) Satisfies(required CapabilitySet) bool {
	return len(available.Missing(required)) == 0
}

func (available CapabilitySet) Missing(required CapabilitySet) []Capability {
	missing := make([]Capability, 0)
	for _, capability := range knownCapabilities {
		if required.has(capability) && !available.has(capability) {
			missing = append(missing, capability)
		}
	}
	return missing
}

func (set CapabilitySet) Names() []string {
	names := make([]string, 0)
	for _, capability := range knownCapabilities {
		if set.has(capability) {
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

func (set CapabilitySet) has(capability Capability) bool {
	switch capability {
	case CapabilityReadFiles:
		return set.ReadFiles
	case CapabilityEditFiles:
		return set.EditFiles
	case CapabilityRunShell:
		return set.RunShell
	case CapabilitySpawnSubagents:
		return set.SpawnSubagents
	case CapabilityBackgroundExecution:
		return set.BackgroundExecution
	case CapabilitySessionResume:
		return set.SessionResume
	case CapabilityWorktreeIsolation:
		return set.WorktreeIsolation
	case CapabilityStructuredOutput:
		return set.StructuredOutput
	case CapabilityToolRestrictions:
		return set.ToolRestrictions
	case CapabilityWebResearch:
		return set.WebResearch
	default:
		return false
	}
}

func (set *CapabilitySet) set(capability Capability, value bool) {
	switch capability {
	case CapabilityReadFiles:
		set.ReadFiles = value
	case CapabilityEditFiles:
		set.EditFiles = value
	case CapabilityRunShell:
		set.RunShell = value
	case CapabilitySpawnSubagents:
		set.SpawnSubagents = value
	case CapabilityBackgroundExecution:
		set.BackgroundExecution = value
	case CapabilitySessionResume:
		set.SessionResume = value
	case CapabilityWorktreeIsolation:
		set.WorktreeIsolation = value
	case CapabilityStructuredOutput:
		set.StructuredOutput = value
	case CapabilityToolRestrictions:
		set.ToolRestrictions = value
	case CapabilityWebResearch:
		set.WebResearch = value
	}
}
