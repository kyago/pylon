package fake

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/kyago/pylon/internal/provider"
)

type CallCounts struct {
	Probe   int
	Start   int
	Poll    int
	Resume  int
	Cancel  int
	Collect int
}

type Adapter struct {
	ProviderName string
	Capabilities provider.CapabilitySet
	ProbeError   error
	StartError   error
	PollError    error
	ResumeError  error
	CancelError  error
	CollectError error
	Status       provider.WorkerStatus
	Result       provider.TaskResult

	mu    sync.Mutex
	calls CallCounts
}

func New(name string, capabilities provider.CapabilitySet) *Adapter {
	return &Adapter{
		ProviderName: name,
		Capabilities: capabilities,
		Status: provider.WorkerStatus{
			State:     provider.WorkerStateRunning,
			UpdatedAt: time.Unix(0, 0).UTC(),
		},
		Result: provider.TaskResult{State: provider.WorkerStateSucceeded},
	}
}

func (adapter *Adapter) Name() string {
	return adapter.ProviderName
}

func (adapter *Adapter) Probe(context.Context) (provider.CapabilitySet, error) {
	adapter.increment(func(calls *CallCounts) { calls.Probe++ })
	return adapter.Capabilities, adapter.ProbeError
}

func (adapter *Adapter) Start(_ context.Context, spec provider.TaskSpec) (provider.WorkerHandle, error) {
	adapter.increment(func(calls *CallCounts) { calls.Start++ })
	if adapter.StartError != nil {
		return provider.WorkerHandle{}, adapter.StartError
	}
	return provider.WorkerHandle{
		Provider:   adapter.ProviderName,
		ExternalID: fmt.Sprintf("%s-%s", spec.RunID, spec.TaskID),
		Attempt:    1,
	}, nil
}

func (adapter *Adapter) Poll(context.Context, provider.WorkerHandle) (provider.WorkerStatus, error) {
	adapter.increment(func(calls *CallCounts) { calls.Poll++ })
	return adapter.Status, adapter.PollError
}

func (adapter *Adapter) Resume(_ context.Context, handle provider.WorkerHandle) (provider.WorkerHandle, error) {
	adapter.increment(func(calls *CallCounts) { calls.Resume++ })
	if adapter.ResumeError != nil {
		return provider.WorkerHandle{}, adapter.ResumeError
	}
	return handle, nil
}

func (adapter *Adapter) Cancel(context.Context, provider.WorkerHandle) error {
	adapter.increment(func(calls *CallCounts) { calls.Cancel++ })
	return adapter.CancelError
}

func (adapter *Adapter) Collect(context.Context, provider.WorkerHandle) (provider.TaskResult, error) {
	adapter.increment(func(calls *CallCounts) { calls.Collect++ })
	return adapter.Result, adapter.CollectError
}

func (adapter *Adapter) Calls() CallCounts {
	adapter.mu.Lock()
	defer adapter.mu.Unlock()
	return adapter.calls
}

func (adapter *Adapter) increment(update func(*CallCounts)) {
	adapter.mu.Lock()
	defer adapter.mu.Unlock()
	update(&adapter.calls)
}
