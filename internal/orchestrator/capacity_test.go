package orchestrator

import (
	"context"
	"testing"
	"time"
)

func TestWorkerPool_AcquireRelease(t *testing.T) {
	wp := NewWorkerPool(map[string]int{"sonnet": 2})

	ctx := context.Background()

	// Acquire 2 slots
	if err := wp.Acquire(ctx, "sonnet"); err != nil {
		t.Fatalf("acquire 1: %v", err)
	}
	if err := wp.Acquire(ctx, "sonnet"); err != nil {
		t.Fatalf("acquire 2: %v", err)
	}

	// Third should block — test with timeout
	ctx2, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
	defer cancel()
	err := wp.Acquire(ctx2, "sonnet")
	if err == nil {
		t.Fatal("expected timeout for third acquire")
	}

	// Release one and retry
	wp.Release("sonnet")
	if err := wp.Acquire(ctx, "sonnet"); err != nil {
		t.Fatalf("acquire after release: %v", err)
	}

	if wp.TotalActive() != 2 {
		t.Fatalf("expected 2 active, got %d", wp.TotalActive())
	}
}

func TestWorkerPool_UnlimitedModel(t *testing.T) {
	wp := NewWorkerPool(map[string]int{"sonnet": 1})

	ctx := context.Background()

	// Unlimited model (not in limits map) should always succeed
	for i := 0; i < 10; i++ {
		if err := wp.Acquire(ctx, "opus"); err != nil {
			t.Fatalf("unlimited acquire %d: %v", i, err)
		}
	}

	if wp.TotalActive() != 10 {
		t.Fatalf("expected 10 active, got %d", wp.TotalActive())
	}
}

func TestWorkerPool_Status(t *testing.T) {
	wp := NewWorkerPool(map[string]int{"sonnet": 3, "haiku": 5})

	ctx := context.Background()
	_ = wp.Acquire(ctx, "sonnet")
	_ = wp.Acquire(ctx, "sonnet")
	_ = wp.Acquire(ctx, "haiku")

	status := wp.Status()

	if s, ok := status["sonnet"]; !ok {
		t.Fatal("missing sonnet status")
	} else if s.Active != 2 || s.MaxSlot != 3 {
		t.Fatalf("sonnet: active=%d max=%d, want 2/3", s.Active, s.MaxSlot)
	}

	if s, ok := status["haiku"]; !ok {
		t.Fatal("missing haiku status")
	} else if s.Active != 1 || s.MaxSlot != 5 {
		t.Fatalf("haiku: active=%d max=%d, want 1/5", s.Active, s.MaxSlot)
	}
}

func TestWorkerPool_ReleaseUnderflow(t *testing.T) {
	wp := NewWorkerPool(nil)

	// Release without acquire should not go negative
	wp.Release("opus")
	status := wp.Status()
	if s, ok := status["opus"]; ok && s.Active < 0 {
		t.Fatal("active count went negative")
	}
}
