package provider

import "context"

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
