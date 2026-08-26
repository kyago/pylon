package provider

import (
	"context"
	"sort"
	"strings"
)

type InteractiveSpec struct {
	MaxTurns            int
	PermissionMode      string
	Environment         []string
	EnvironmentOverride map[string]string
}

type ProcessSpec struct {
	Executable  string
	Args        []string
	Environment []string
	DisplayName string
}

type InteractiveAdapter interface {
	Adapter
	PrepareInteractive(context.Context, InteractiveSpec) (ProcessSpec, error)
}

type VersionedAdapter interface {
	Version(context.Context) (string, error)
}

// MergeEnvironment merges override key=value pairs into a base environment,
// returning a sorted, deduplicated environment slice.
func MergeEnvironment(base []string, overrides map[string]string) []string {
	values := make(map[string]string, len(base)+len(overrides))
	for _, entry := range base {
		if key, value, ok := strings.Cut(entry, "="); ok {
			values[key] = value
		}
	}
	for key, value := range overrides {
		values[key] = value
	}

	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	environment := make([]string, 0, len(keys))
	for _, key := range keys {
		environment = append(environment, key+"="+values[key])
	}
	return environment
}
