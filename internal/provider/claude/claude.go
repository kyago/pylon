// Package claude implements the Claude CLI provider adapter.
package claude

import (
	"context"
	"fmt"
	"os/exec"
	"sort"
	"strings"

	"github.com/kyago/pylon/internal/provider"
)

const Name = "claude-code"

type lookPathFunc func(string) (string, error)
type versionCommandFunc func(context.Context, string) ([]byte, error)

type Option func(*Adapter)

type Adapter struct {
	command        string
	lookPath       lookPathFunc
	versionCommand versionCommandFunc
}

var _ provider.InteractiveAdapter = (*Adapter)(nil)
var _ provider.VersionedAdapter = (*Adapter)(nil)

func New(command string, options ...Option) *Adapter {
	if strings.TrimSpace(command) == "" {
		command = "claude"
	}
	adapter := &Adapter{
		command:  command,
		lookPath: exec.LookPath,
		versionCommand: func(ctx context.Context, path string) ([]byte, error) {
			return exec.CommandContext(ctx, path, "--version").CombinedOutput()
		},
	}
	for _, option := range options {
		option(adapter)
	}
	return adapter
}

func WithLookPath(lookPath func(string) (string, error)) Option {
	return func(adapter *Adapter) {
		adapter.lookPath = lookPath
	}
}

func WithVersionCommand(command func(context.Context, string) ([]byte, error)) Option {
	return func(adapter *Adapter) {
		adapter.versionCommand = command
	}
}

func (adapter *Adapter) Name() string {
	return Name
}

func (adapter *Adapter) Probe(context.Context) (provider.CapabilitySet, error) {
	if _, err := adapter.lookPath(adapter.command); err != nil {
		return provider.CapabilitySet{}, fmt.Errorf("%s executable not found: %w", adapter.command, err)
	}
	return provider.CapabilitySet{
		ReadFiles:           true,
		EditFiles:           true,
		RunShell:            true,
		SpawnSubagents:      true,
		BackgroundExecution: true,
		SessionResume:       true,
		WorktreeIsolation:   true,
		StructuredOutput:    true,
		ToolRestrictions:    true,
		WebResearch:         true,
	}, nil
}

func (adapter *Adapter) Version(ctx context.Context) (string, error) {
	path, err := adapter.lookPath(adapter.command)
	if err != nil {
		return "", fmt.Errorf("%s executable not found: %w", adapter.command, err)
	}
	out, err := adapter.versionCommand(ctx, path)
	if err != nil {
		return "", fmt.Errorf("%s version check failed: %w", adapter.command, err)
	}
	version := strings.TrimSpace(string(out))
	if version == "" {
		return "", fmt.Errorf("%s version output is empty", adapter.command)
	}
	return version, nil
}

func (adapter *Adapter) PrepareInteractive(_ context.Context, spec provider.InteractiveSpec) (provider.ProcessSpec, error) {
	path, err := adapter.lookPath(adapter.command)
	if err != nil {
		return provider.ProcessSpec{}, fmt.Errorf("%s executable not found: %w", adapter.command, err)
	}

	args := []string{adapter.command}
	if spec.MaxTurns > 0 {
		args = append(args, "--max-turns", fmt.Sprintf("%d", spec.MaxTurns))
	}
	if spec.PermissionMode != "" {
		args = append(args, "--permission-mode", spec.PermissionMode)
	}

	return provider.ProcessSpec{
		Executable:  path,
		Args:        args,
		Environment: mergeEnvironment(spec.Environment, spec.EnvironmentOverride),
		DisplayName: "Claude Code",
	}, nil
}

func (adapter *Adapter) Start(context.Context, provider.TaskSpec) (provider.WorkerHandle, error) {
	return provider.WorkerHandle{}, provider.ErrUnsupportedOperation
}

func (adapter *Adapter) Poll(context.Context, provider.WorkerHandle) (provider.WorkerStatus, error) {
	return provider.WorkerStatus{}, provider.ErrUnsupportedOperation
}

func (adapter *Adapter) Resume(context.Context, provider.WorkerHandle) (provider.WorkerHandle, error) {
	return provider.WorkerHandle{}, provider.ErrUnsupportedOperation
}

func (adapter *Adapter) Cancel(context.Context, provider.WorkerHandle) error {
	return provider.ErrUnsupportedOperation
}

func (adapter *Adapter) Collect(context.Context, provider.WorkerHandle) (provider.TaskResult, error) {
	return provider.TaskResult{}, provider.ErrUnsupportedOperation
}

func mergeEnvironment(base []string, overrides map[string]string) []string {
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
