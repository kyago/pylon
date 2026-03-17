package dashboard

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kyago/pylon/internal/config"
)

func TestNewServer(t *testing.T) {
	mock := &mockStore{}
	cfg := &config.DashboardConfig{Host: "localhost", Port: 0}
	runtimeCfg := &config.RuntimeConfig{MaxConcurrent: 5}

	srv, err := NewServer(mock, cfg, runtimeCfg, "test-workspace")
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	if srv.router == nil {
		t.Error("router is nil")
	}
	if srv.hub == nil {
		t.Error("hub is nil")
	}
	if srv.templates == nil {
		t.Error("templates is nil")
	}
}

func TestServerGracefulShutdown(t *testing.T) {
	mock := &mockStore{}
	port := freePort(t)
	cfg := &config.DashboardConfig{Host: "127.0.0.1", Port: port}
	runtimeCfg := &config.RuntimeConfig{MaxConcurrent: 5}

	srv, err := NewServer(mock, cfg, runtimeCfg, "test-workspace")
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Start(ctx)
	}()

	time.Sleep(100 * time.Millisecond)

	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/", port))
	if err != nil {
		t.Fatalf("server not responding: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("Start returned error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Error("shutdown timed out")
	}
}

func TestServerRoutes(t *testing.T) {
	mock := &mockStore{}
	cfg := &config.DashboardConfig{Host: "127.0.0.1", Port: 0}
	runtimeCfg := &config.RuntimeConfig{MaxConcurrent: 5}

	srv, err := NewServer(mock, cfg, runtimeCfg, "test-workspace")
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	ts := httptest.NewServer(srv.router)
	defer ts.Close()

	routes := []struct {
		method string
		path   string
		status int
	}{
		{"GET", "/", http.StatusOK},
		{"GET", "/messages", http.StatusOK},
		{"GET", "/memory", http.StatusOK},
		{"GET", "/api/overview", http.StatusOK},
		{"GET", "/api/pipelines", http.StatusOK},
		{"GET", "/api/messages", http.StatusOK},
		{"GET", "/api/memory", http.StatusOK},
		{"GET", "/api/pipelines/nonexistent", http.StatusNotFound},
		{"GET", "/static/style.css", http.StatusOK},
	}

	for _, tt := range routes {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			req, _ := http.NewRequest(tt.method, ts.URL+tt.path, nil)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			resp.Body.Close()
			if resp.StatusCode != tt.status {
				t.Errorf("want %d, got %d", tt.status, resp.StatusCode)
			}
		})
	}
}

// freePort finds an available port by briefly listening on :0.
func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("freePort: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return port
}
