package fake

import (
	"context"
	"testing"

	"github.com/kyago/pylon/internal/provider"
)

func TestAdapterRecordsLifecycleCalls(t *testing.T) {
	adapter := New("fake", provider.CapabilitySet{ReadFiles: true})
	handle, err := adapter.Start(context.Background(), provider.TaskSpec{RunID: "run-1", TaskID: "T001"})
	if err != nil {
		t.Fatal(err)
	}
	if handle.Provider != "fake" || handle.Attempt != 1 {
		t.Fatalf("handle = %+v", handle)
	}
	if _, err := adapter.Poll(context.Background(), handle); err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.Resume(context.Background(), handle); err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.Collect(context.Background(), handle); err != nil {
		t.Fatal(err)
	}
	if err := adapter.Cancel(context.Background(), handle); err != nil {
		t.Fatal(err)
	}

	calls := adapter.Calls()
	if calls.Start != 1 || calls.Poll != 1 || calls.Resume != 1 || calls.Collect != 1 || calls.Cancel != 1 {
		t.Fatalf("calls = %+v", calls)
	}
}
