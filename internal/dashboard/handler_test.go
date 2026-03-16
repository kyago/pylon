package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/kyago/pylon/internal/config"
	"github.com/kyago/pylon/internal/store"
)

// mockStore implements DashboardStore for testing.
type mockStore struct {
	pipelines      []store.PipelineRecord
	messages       []store.QueuedMessage
	queueStats     []store.MessageQueueStat
	statusCounts   map[string]int
	metrics        *store.PipelineMetrics
	projects       []store.ProjectRecord
	memoryEntries  []store.MemoryEntry
	searchResults  []store.MemorySearchResult
	blackboard     []store.BlackboardEntry
}

func (m *mockStore) GetPipeline(id string) (*store.PipelineRecord, error) {
	for _, p := range m.pipelines {
		if p.PipelineID == id {
			return &p, nil
		}
	}
	return nil, nil
}

func (m *mockStore) GetActivePipelines() ([]store.PipelineRecord, error) {
	var active []store.PipelineRecord
	for _, p := range m.pipelines {
		if p.Stage != "completed" && p.Stage != "failed" {
			active = append(active, p)
		}
	}
	return active, nil
}

func (m *mockStore) ListAllPipelines() ([]store.PipelineRecord, error) {
	return m.pipelines, nil
}

func (m *mockStore) UpsertPipeline(rec *store.PipelineRecord) error {
	for i, p := range m.pipelines {
		if p.PipelineID == rec.PipelineID {
			m.pipelines[i] = *rec
			return nil
		}
	}
	m.pipelines = append(m.pipelines, *rec)
	return nil
}

func (m *mockStore) CountMessagesByStatus() (map[string]int, error) {
	if m.statusCounts != nil {
		return m.statusCounts, nil
	}
	return map[string]int{}, nil
}

func (m *mockStore) GetMessageQueueStats() ([]store.MessageQueueStat, error) {
	return m.queueStats, nil
}

func (m *mockStore) GetRecentMessages(limit int) ([]store.QueuedMessage, error) {
	if limit > 0 && limit < len(m.messages) {
		return m.messages[:limit], nil
	}
	return m.messages, nil
}

func (m *mockStore) ListProjectMemory(projectID string) ([]store.MemoryEntry, error) {
	return m.memoryEntries, nil
}

func (m *mockStore) SearchMemory(projectID, query string, limit int) ([]store.MemorySearchResult, error) {
	return m.searchResults, nil
}

func (m *mockStore) GetBlackboardByCategory(projectID, category string) ([]store.BlackboardEntry, error) {
	return m.blackboard, nil
}

func (m *mockStore) ListProjects() ([]store.ProjectRecord, error) {
	return m.projects, nil
}

func (m *mockStore) GetPipelineMetrics() (*store.PipelineMetrics, error) {
	if m.metrics != nil {
		return m.metrics, nil
	}
	return &store.PipelineMetrics{}, nil
}

func newTestServer(t *testing.T, mock *mockStore) *Server {
	t.Helper()
	cfg := &config.DashboardConfig{Host: "localhost", Port: 0}
	runtimeCfg := &config.RuntimeConfig{MaxConcurrent: 5}
	srv, err := NewServer(mock, cfg, runtimeCfg)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	return srv
}

func TestHandleAPIOverview(t *testing.T) {
	mock := &mockStore{
		metrics:      &store.PipelineMetrics{TotalPipelines: 3, ActivePipelines: 1},
		statusCounts: map[string]int{"queued": 5},
	}
	srv := newTestServer(t, mock)

	req := httptest.NewRequest("GET", "/api/overview", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("want 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("want application/json, got %s", ct)
	}

	var data OverviewData
	if err := json.NewDecoder(w.Body).Decode(&data); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if data.Metrics.TotalPipelines != 3 {
		t.Errorf("total: want 3, got %d", data.Metrics.TotalPipelines)
	}
}

func TestHandleAPIPipelines(t *testing.T) {
	mock := &mockStore{
		pipelines: []store.PipelineRecord{
			{PipelineID: "p1", Stage: "agent_executing", StateJSON: `{"pipeline_id":"p1","current_stage":"agent_executing","created_at":"2025-01-01T00:00:00Z"}`, UpdatedAt: time.Now()},
			{PipelineID: "p2", Stage: "completed", StateJSON: `{"pipeline_id":"p2","current_stage":"completed","created_at":"2025-01-01T00:00:00Z"}`, UpdatedAt: time.Now()},
		},
	}
	srv := newTestServer(t, mock)

	// All pipelines
	req := httptest.NewRequest("GET", "/api/pipelines", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("want 200, got %d", w.Code)
	}

	var views []PipelineView
	json.NewDecoder(w.Body).Decode(&views)
	if len(views) != 2 {
		t.Errorf("want 2 pipelines, got %d", len(views))
	}

	// Active filter
	req = httptest.NewRequest("GET", "/api/pipelines?status=active", nil)
	w = httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	json.NewDecoder(w.Body).Decode(&views)
	if len(views) != 1 {
		t.Errorf("want 1 active, got %d", len(views))
	}
}

func TestHandleAPIPipelineDetail(t *testing.T) {
	mock := &mockStore{
		pipelines: []store.PipelineRecord{
			{PipelineID: "p1", Stage: "verification", StateJSON: `{"pipeline_id":"p1","current_stage":"verification","task_spec":"test task","created_at":"2025-01-01T00:00:00Z"}`, UpdatedAt: time.Now()},
		},
	}
	srv := newTestServer(t, mock)

	// Existing pipeline
	r := chi.NewRouter()
	r.Get("/api/pipelines/{id}", srv.handleAPIPipelineDetail)

	req := httptest.NewRequest("GET", "/api/pipelines/p1", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("want 200, got %d", w.Code)
	}

	var view PipelineView
	json.NewDecoder(w.Body).Decode(&view)
	if view.TaskSpec != "test task" {
		t.Errorf("want 'test task', got '%s'", view.TaskSpec)
	}

	// Non-existent pipeline
	req = httptest.NewRequest("GET", "/api/pipelines/missing", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("want 404, got %d", w.Code)
	}
}

func TestHandleAPIPipelineCancel(t *testing.T) {
	mock := &mockStore{
		pipelines: []store.PipelineRecord{
			{PipelineID: "p1", Stage: "agent_executing", StateJSON: `{"pipeline_id":"p1","current_stage":"agent_executing","created_at":"2025-01-01T00:00:00Z"}`, UpdatedAt: time.Now()},
			{PipelineID: "p2", Stage: "completed", StateJSON: `{"pipeline_id":"p2","current_stage":"completed","created_at":"2025-01-01T00:00:00Z"}`, UpdatedAt: time.Now()},
		},
	}
	srv := newTestServer(t, mock)

	r := chi.NewRouter()
	r.Post("/api/pipelines/{id}/cancel", srv.handleAPIPipelineCancel)

	// Cancel active pipeline
	req := httptest.NewRequest("POST", "/api/pipelines/p1/cancel", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("want 200, got %d", w.Code)
	}

	var result map[string]string
	json.NewDecoder(w.Body).Decode(&result)
	if result["status"] != "cancelled" {
		t.Errorf("want cancelled, got %s", result["status"])
	}

	// Verify pipeline was updated
	rec, _ := mock.GetPipeline("p1")
	if rec.Stage != "failed" {
		t.Errorf("want failed, got %s", rec.Stage)
	}

	// Cancel already terminal pipeline
	req = httptest.NewRequest("POST", "/api/pipelines/p2/cancel", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", w.Code)
	}
}

func TestHandleAPIMessages(t *testing.T) {
	mock := &mockStore{
		messages: []store.QueuedMessage{
			{ID: "m1", Type: "task_assign", FromAgent: "po", ToAgent: "dev", Status: "queued", CreatedAt: time.Now()},
			{ID: "m2", Type: "result", FromAgent: "dev", ToAgent: "po", Status: "acked", CreatedAt: time.Now()},
		},
	}
	srv := newTestServer(t, mock)

	req := httptest.NewRequest("GET", "/api/messages", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("want 200, got %d", w.Code)
	}

	var data MessagesData
	json.NewDecoder(w.Body).Decode(&data)
	if len(data.Messages) != 2 {
		t.Errorf("want 2, got %d", len(data.Messages))
	}

	// With filters
	req = httptest.NewRequest("GET", "/api/messages?agent=dev&status=acked", nil)
	w = httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	json.NewDecoder(w.Body).Decode(&data)
	if len(data.Messages) != 1 {
		t.Errorf("want 1 filtered, got %d", len(data.Messages))
	}
}

func TestHandleOverviewHTMX(t *testing.T) {
	mock := &mockStore{
		metrics: &store.PipelineMetrics{},
	}
	srv := newTestServer(t, mock)

	// Full page request
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	srv.router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("want 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Errorf("want text/html, got %s", ct)
	}

	body := w.Body.String()
	if len(body) == 0 {
		t.Error("empty body")
	}
}
