// Package codex implements the OpenAI Codex CLI provider adapter.
package codex

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/kyago/pylon/internal/provider"
)

const Name = "codex"

// 유효한 codex --sandbox 값 (공식 문서: codex-rs/protocol/src/config_types.rs)
var sandboxModes = map[string]bool{
	"read-only":          true,
	"workspace-write":    true,
	"danger-full-access": true,
}

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
		command = "codex"
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
		ReadFiles:     true,
		EditFiles:     true,
		RunShell:      true,
		SessionResume: true,
		WebResearch:   true,
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
	// codex의 실행 권한은 --sandbox로 제어한다. claude 전용 permission mode 등
	// 알 수 없는 값은 codex 기본 동작(config.toml)에 맡기고 플래그를 생략한다.
	if sandboxModes[spec.PermissionMode] {
		args = append(args, "--sandbox", spec.PermissionMode)
	}

	return provider.ProcessSpec{
		Executable:  path,
		Args:        args,
		Environment: provider.MergeEnvironment(spec.Environment, spec.EnvironmentOverride),
		DisplayName: "Codex",
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
