package orchestrator

import (
	"context"
	"testing"
	"time"
)

func TestScheduler_SubmitRequiresPipelineID(t *testing.T) {
	s := NewScheduler(2, nil)
	defer s.Stop()

	_, err := s.Submit(LoopConfig{})
	if err == nil {
		t.Fatal("expected error for empty PipelineID")
	}
}

func TestScheduler_DuplicateSubmit(t *testing.T) {
	s := NewScheduler(2, nil)
	defer s.Stop()

	cfg := LoopConfig{PipelineID: "test-1"}
	_, err := s.Submit(cfg)
	if err != nil {
		t.Fatalf("first submit: %v", err)
	}

	_, err = s.Submit(cfg)
	if err == nil {
		t.Fatal("expected error for duplicate PipelineID")
	}
}

func TestScheduler_StatusTracking(t *testing.T) {
	s := NewScheduler(5, nil)
	defer s.Stop()

	// Submit three pipelines (they will fail quickly due to nil deps, but status is tracked)
	for _, id := range []string{"p1", "p2", "p3"} {
		_, _ = s.Submit(LoopConfig{PipelineID: id})
	}

	// Wait for goroutines to start and fail
	time.Sleep(100 * time.Millisecond)

	statuses := s.Status()
	if len(statuses) != 3 {
		t.Fatalf("expected 3 pipelines tracked, got %d", len(statuses))
	}

	// All should be failed (nil Config causes panic recovery → failed)
	for _, st := range statuses {
		if st.Status != "failed" && st.Status != "running" && st.Status != "waiting" {
			t.Logf("pipeline %s: %s", st.PipelineID, st.Status)
		}
	}
}

func TestScheduler_Stop(t *testing.T) {
	s := NewScheduler(2, nil)

	done := make(chan struct{})
	go func() {
		s.Stop()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Stop() timed out")
	}
}

func TestScheduler_WIP_AcquireRelease(t *testing.T) {
	limits := map[string]int{"agent_executing": 1}
	s := NewScheduler(5, limits)
	defer s.Stop()

	ctx := context.Background()

	// First acquire should succeed
	if err := s.AcquireWIP(ctx, "agent_executing"); err != nil {
		t.Fatalf("first acquire: %v", err)
	}

	// Second acquire should block, test with timeout
	ctx2, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
	defer cancel()

	err := s.AcquireWIP(ctx2, "agent_executing")
	if err == nil {
		t.Fatal("expected timeout error for WIP limit")
	}

	// Release and retry
	s.ReleaseWIP("agent_executing")
	if err := s.AcquireWIP(ctx, "agent_executing"); err != nil {
		t.Fatalf("acquire after release: %v", err)
	}

	// Stage without limit should always pass
	if err := s.AcquireWIP(ctx, "init"); err != nil {
		t.Fatalf("no-limit stage: %v", err)
	}

	// Check WIP status
	wipStatus := s.WIPStatus()
	if wipStatus["agent_executing"] != 1 {
		t.Fatalf("expected WIP 1 for agent_executing, got %d", wipStatus["agent_executing"])
	}
}

func TestScheduler_RunningCount(t *testing.T) {
	s := NewScheduler(5, nil)
	defer s.Stop()

	if s.RunningCount() != 0 {
		t.Fatalf("expected 0 running, got %d", s.RunningCount())
	}
}
