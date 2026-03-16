package dashboard

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
)

// SSEEvent represents a server-sent event.
type SSEEvent struct {
	Type string `json:"type"`
	Data any    `json:"data"`
}

// SSEHub manages SSE client connections and event broadcasting.
type SSEHub struct {
	mu         sync.RWMutex
	clients    map[chan SSEEvent]struct{}
	register   chan chan SSEEvent
	unregister chan chan SSEEvent
	broadcast  chan SSEEvent
}

// NewSSEHub creates a new SSE hub.
func NewSSEHub() *SSEHub {
	return &SSEHub{
		clients:    make(map[chan SSEEvent]struct{}),
		register:   make(chan chan SSEEvent),
		unregister: make(chan chan SSEEvent),
		broadcast:  make(chan SSEEvent, 64),
	}
}

// Run starts the hub event loop. It blocks until ctx is cancelled.
func (h *SSEHub) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			h.mu.Lock()
			for ch := range h.clients {
				close(ch)
				delete(h.clients, ch)
			}
			h.mu.Unlock()
			return
		case ch := <-h.register:
			h.mu.Lock()
			h.clients[ch] = struct{}{}
			h.mu.Unlock()
		case ch := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[ch]; ok {
				close(ch)
				delete(h.clients, ch)
			}
			h.mu.Unlock()
		case event := <-h.broadcast:
			h.mu.RLock()
			for ch := range h.clients {
				select {
				case ch <- event:
				default:
					// Client too slow, skip this event
				}
			}
			h.mu.RUnlock()
		}
	}
}

// Subscribe returns an event channel and an unsubscribe function.
func (h *SSEHub) Subscribe() (chan SSEEvent, func()) {
	ch := make(chan SSEEvent, 16)
	h.register <- ch
	return ch, func() {
		h.unregister <- ch
	}
}

// Publish sends an event to all connected clients (non-blocking).
func (h *SSEHub) Publish(event SSEEvent) {
	select {
	case h.broadcast <- event:
	default:
		// Broadcast buffer full, drop event
	}
}

// ClientCount returns the number of connected SSE clients.
func (h *SSEHub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// handleSSEStream handles the SSE HTTP endpoint.
func (srv *Server) handleSSEStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	ch, unsubscribe := srv.hub.Subscribe()
	defer unsubscribe()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-ch:
			if !ok {
				return
			}
			data, err := json.Marshal(event.Data)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event.Type, data)
			flusher.Flush()
		}
	}
}
