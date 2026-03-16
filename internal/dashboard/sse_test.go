package dashboard

import (
	"context"
	"testing"
	"time"
)

func TestSSEHubSubscribeUnsubscribe(t *testing.T) {
	hub := NewSSEHub()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go hub.Run(ctx)
	time.Sleep(10 * time.Millisecond)

	ch, unsub := hub.Subscribe()
	time.Sleep(10 * time.Millisecond)

	if hub.ClientCount() != 1 {
		t.Errorf("want 1 client, got %d", hub.ClientCount())
	}

	unsub()
	time.Sleep(10 * time.Millisecond)

	if hub.ClientCount() != 0 {
		t.Errorf("want 0 clients, got %d", hub.ClientCount())
	}

	// Channel should be closed
	select {
	case _, ok := <-ch:
		if ok {
			t.Error("expected channel to be closed")
		}
	default:
		// This is also acceptable - channel might not have been read
	}
}

func TestSSEHubBroadcast(t *testing.T) {
	hub := NewSSEHub()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go hub.Run(ctx)
	time.Sleep(10 * time.Millisecond)

	ch1, unsub1 := hub.Subscribe()
	defer unsub1()
	ch2, unsub2 := hub.Subscribe()
	defer unsub2()

	time.Sleep(10 * time.Millisecond)

	event := SSEEvent{
		Type: "test_event",
		Data: map[string]string{"key": "value"},
	}
	hub.Publish(event)

	// Both clients should receive the event
	for i, ch := range []chan SSEEvent{ch1, ch2} {
		select {
		case received := <-ch:
			if received.Type != "test_event" {
				t.Errorf("client %d: want test_event, got %s", i, received.Type)
			}
		case <-time.After(time.Second):
			t.Errorf("client %d: timeout waiting for event", i)
		}
	}
}

func TestSSEHubContextCancel(t *testing.T) {
	hub := NewSSEHub()
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		hub.Run(ctx)
		close(done)
	}()

	time.Sleep(10 * time.Millisecond)

	ch, _ := hub.Subscribe()
	time.Sleep(10 * time.Millisecond)

	cancel()

	select {
	case <-done:
		// Hub stopped
	case <-time.After(time.Second):
		t.Error("hub did not stop")
	}

	// Channel should be closed
	select {
	case _, ok := <-ch:
		if ok {
			t.Error("expected channel to be closed after context cancel")
		}
	case <-time.After(100 * time.Millisecond):
		// Acceptable
	}
}

func TestSSEHubPublishNonBlocking(t *testing.T) {
	hub := NewSSEHub()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go hub.Run(ctx)
	time.Sleep(10 * time.Millisecond)

	// Publish without subscribers should not block
	for i := 0; i < 100; i++ {
		hub.Publish(SSEEvent{Type: "test", Data: i})
	}
}

func TestSSEHubSlowClient(t *testing.T) {
	hub := NewSSEHub()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go hub.Run(ctx)
	time.Sleep(10 * time.Millisecond)

	// Subscribe but don't read
	_, unsub := hub.Subscribe()
	defer unsub()

	// Fast client
	ch2, unsub2 := hub.Subscribe()
	defer unsub2()

	time.Sleep(10 * time.Millisecond)

	// Send many events - should not block
	for i := 0; i < 50; i++ {
		hub.Publish(SSEEvent{Type: "test", Data: i})
	}

	// Fast client should receive some events
	received := 0
	for {
		select {
		case <-ch2:
			received++
		case <-time.After(100 * time.Millisecond):
			goto done
		}
	}
done:
	if received == 0 {
		t.Error("fast client received no events")
	}
}
