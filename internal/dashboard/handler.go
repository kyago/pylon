package dashboard

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/kyago/pylon/internal/domain"
	"github.com/kyago/pylon/internal/orchestrator"
	"github.com/kyago/pylon/internal/store"
)

// --- View Models ---

// TaskItemView represents a task within a task graph for template rendering.
type TaskItemView struct {
	ID          string
	Description string
	AgentName   string
	DependsOn   []string
}

// PipelineView is the template-friendly representation of a pipeline.
type PipelineView struct {
	ID           string
	CurrentStage string
	StageIndex   int
	TotalStages  int
	TaskSpec     string
	Agents       map[string]AgentView
	History      []TransitionView
	TaskGraph    []TaskItemView
	Attempts     int
	MaxAttempts  int
	CreatedAt    time.Time
	UpdatedAt    time.Time
	IsActive     bool
}

// AgentView represents an agent within a pipeline.
type AgentView struct {
	Name   string
	TaskID string
	Status string
}

// TransitionView represents a stage transition event.
type TransitionView struct {
	From        string
	To          string
	CompletedAt time.Time
}

// ConcurrencyView holds concurrency metrics.
type ConcurrencyView struct {
	RunningAgents   int
	TotalPipelines  int
	ActivePipelines int
}

// OverviewData is the view model for the overview page.
type OverviewData struct {
	Pipelines       []PipelineView
	Concurrency     ConcurrencyView
	QueueStats      map[string]int
	Metrics         *store.PipelineMetrics
	AdvancedMetrics *store.AdvancedMetrics
}

// PipelineDetailData is the view model for the pipeline detail page.
type PipelineDetailData struct {
	Pipeline PipelineView
	Stages   []domain.Stage
}

// MessagesData is the view model for the messages page.
type MessagesData struct {
	Messages     []store.QueuedMessage
	QueueStats   []store.MessageQueueStat
	FilterAgent  string
	FilterStatus string
}

// MemoryData is the view model for the memory page.
type MemoryData struct {
	Projects      []store.ProjectRecord
	Entries       []store.MemoryEntry
	SearchResults []store.MemorySearchResult
	Blackboard    []store.BlackboardEntry
	ProjectID     string
	Query         string
}

// --- Data conversion ---

func pipelineRecordToView(rec store.PipelineRecord) PipelineView {
	view := PipelineView{
		ID:           rec.PipelineID,
		CurrentStage: rec.Stage,
		StageIndex:   stageIndex(rec.Stage),
		TotalStages:  len(domain.AllStages()),
		UpdatedAt:    rec.UpdatedAt,
		IsActive:     !isTerminal(rec.Stage),
		Agents:       make(map[string]AgentView),
	}

	pipeline, err := orchestrator.LoadPipeline([]byte(rec.StateJSON))
	if err == nil {
		view.TaskSpec = pipeline.TaskSpec
		view.Attempts = pipeline.Attempts
		view.MaxAttempts = pipeline.MaxAttempts
		view.CreatedAt = pipeline.CreatedAt
		for name, agent := range pipeline.Agents {
			view.Agents[name] = AgentView{
				Name:   name,
				TaskID: agent.TaskID,
				Status: agent.Status,
			}
		}
		for _, h := range pipeline.History {
			view.History = append(view.History, TransitionView{
				From:        string(h.From),
				To:          string(h.To),
				CompletedAt: h.CompletedAt,
			})
		}
		if pipeline.TaskGraph != nil {
			for _, t := range pipeline.TaskGraph.Tasks {
				view.TaskGraph = append(view.TaskGraph, TaskItemView{
					ID:          t.ID,
					Description: t.Description,
					AgentName:   t.AgentName,
					DependsOn:   t.DependsOn,
				})
			}
		}
	}

	return view
}

// --- Page Handlers ---

func (srv *Server) handleOverview(w http.ResponseWriter, r *http.Request) {
	data, err := srv.buildOverviewData(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	tmplName := "overview.html"
	if r.Header.Get("HX-Request") == "true" {
		tmplName = "overview_content"
	}

	srv.renderHTML(w, tmplName, data)
}

func (srv *Server) handlePipelineDetail(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	rec, err := srv.store.GetPipeline(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if rec == nil {
		http.Error(w, "pipeline not found", http.StatusNotFound)
		return
	}

	data := PipelineDetailData{
		Pipeline: pipelineRecordToView(*rec),
		Stages:   domain.AllStages(),
	}

	tmplName := "pipeline.html"
	if r.Header.Get("HX-Request") == "true" {
		tmplName = "pipeline_content"
	}

	srv.renderHTML(w, tmplName, data)
}

func (srv *Server) handleMessages(w http.ResponseWriter, r *http.Request) {
	data, err := srv.buildMessagesData(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	tmplName := "messages.html"
	if r.Header.Get("HX-Request") == "true" {
		tmplName = "messages_content"
	}

	srv.renderHTML(w, tmplName, data)
}

func (srv *Server) handleMemory(w http.ResponseWriter, r *http.Request) {
	data, err := srv.buildMemoryData(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	tmplName := "memory.html"
	if r.Header.Get("HX-Request") == "true" {
		tmplName = "memory_content"
	}

	srv.renderHTML(w, tmplName, data)
}

// --- JSON API Handlers ---

func (srv *Server) handleAPIOverview(w http.ResponseWriter, r *http.Request) {
	data, err := srv.buildOverviewData(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	srv.writeJSON(w, data)
}

func (srv *Server) handleAPIPipelines(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")

	var records []store.PipelineRecord
	var err error

	switch status {
	case "active":
		records, err = srv.store.GetActivePipelines()
	case "":
		records, err = srv.store.ListAllPipelines()
	default:
		records, err = srv.store.ListAllPipelines()
		if err == nil {
			var filtered []store.PipelineRecord
			for _, rec := range records {
				if rec.Stage == status {
					filtered = append(filtered, rec)
				}
			}
			records = filtered
		}
	}

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	views := make([]PipelineView, len(records))
	for i, rec := range records {
		views[i] = pipelineRecordToView(rec)
	}
	srv.writeJSON(w, views)
}

func (srv *Server) handleAPIPipelineDetail(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	rec, err := srv.store.GetPipeline(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if rec == nil {
		http.Error(w, "pipeline not found", http.StatusNotFound)
		return
	}
	srv.writeJSON(w, pipelineRecordToView(*rec))
}

// handleAPIPipelineCancel forces a pipeline into the failed state.
//
// NOTE: 대시보드는 별도 프로세스로 실행되므로 오케스트레이터의 컨텍스트/채널에
// 직접 접근할 수 없다. DB 상태를 직접 변경하는 "best-effort" 강제 취소이며,
// 오케스트레이터가 동시에 같은 파이프라인을 실행 중이면 상태를 덮어쓸 수 있다.
// 실행 중인 에이전트 프로세스는 이 API로 중단되지 않는다.
func (srv *Server) handleAPIPipelineCancel(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	rec, err := srv.store.GetPipeline(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if rec == nil {
		http.Error(w, "pipeline not found", http.StatusNotFound)
		return
	}

	pipeline, err := orchestrator.LoadPipeline([]byte(rec.StateJSON))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if pipeline.IsTerminal() {
		http.Error(w, "pipeline already in terminal state", http.StatusBadRequest)
		return
	}

	// Force transition to failed (bypasses state machine validation)
	pipeline.History = append(pipeline.History, orchestrator.StageTransition{
		From:        pipeline.CurrentStage,
		To:          orchestrator.StageFailed,
		CompletedAt: time.Now(),
	})
	pipeline.CurrentStage = orchestrator.StageFailed
	pipeline.Status = orchestrator.StatusFailed

	snapshot, err := pipeline.Snapshot()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	err = srv.store.UpsertPipeline(&store.PipelineRecord{
		PipelineID: id,
		Stage:      string(orchestrator.StageFailed),
		StateJSON:  string(snapshot),
		UpdatedAt:  time.Now(),
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	srv.writeJSON(w, map[string]string{"status": "cancelled", "pipeline_id": id})
}

// handleAPIPipelinePause pauses a running pipeline.
func (srv *Server) handleAPIPipelinePause(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	rec, err := srv.store.GetPipeline(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if rec == nil {
		http.Error(w, "pipeline not found", http.StatusNotFound)
		return
	}

	pipeline, err := orchestrator.LoadPipeline([]byte(rec.StateJSON))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err := pipeline.Pause(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	snapshot, err := pipeline.Snapshot()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	err = srv.store.UpsertPipeline(&store.PipelineRecord{
		PipelineID:    id,
		Stage:         string(pipeline.CurrentStage),
		StateJSON:     string(snapshot),
		WorkflowName:  pipeline.WorkflowName,
		Status:        string(pipeline.Status),
		PausedAtStage: string(pipeline.PausedAtStage),
		UpdatedAt:     time.Now(),
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	srv.writeJSON(w, map[string]string{"status": "paused", "pipeline_id": id})
}

// handleAPIPipelineResume resumes a paused pipeline.
func (srv *Server) handleAPIPipelineResume(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	rec, err := srv.store.GetPipeline(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if rec == nil {
		http.Error(w, "pipeline not found", http.StatusNotFound)
		return
	}

	pipeline, err := orchestrator.LoadPipeline([]byte(rec.StateJSON))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err := pipeline.Resume(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	snapshot, err := pipeline.Snapshot()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	err = srv.store.UpsertPipeline(&store.PipelineRecord{
		PipelineID:    id,
		Stage:         string(pipeline.CurrentStage),
		StateJSON:     string(snapshot),
		WorkflowName:  pipeline.WorkflowName,
		Status:        string(pipeline.Status),
		PausedAtStage: string(pipeline.PausedAtStage),
		UpdatedAt:     time.Now(),
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	srv.writeJSON(w, map[string]string{"status": "resumed", "pipeline_id": id})
}

// handleAPIDLQ returns all DLQ entries.
func (srv *Server) handleAPIDLQ(w http.ResponseWriter, r *http.Request) {
	entries, err := srv.store.ListDLQ()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	srv.writeJSON(w, entries)
}

// handleAPIDLQRequeue moves a DLQ entry back to the pipeline for retry.
func (srv *Server) handleAPIDLQRequeue(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id := 0
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil {
		http.Error(w, "invalid DLQ entry ID", http.StatusBadRequest)
		return
	}

	if err := srv.store.RequeueDLQ(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	srv.writeJSON(w, map[string]string{"status": "requeued", "dlq_id": idStr})
}

// handleAPIDLQDelete removes a DLQ entry (dismiss).
func (srv *Server) handleAPIDLQDelete(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id := 0
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil {
		http.Error(w, "invalid DLQ entry ID", http.StatusBadRequest)
		return
	}

	if err := srv.store.DeleteDLQEntry(id); err != nil {
		if strings.Contains(err.Error(), "not found") {
			http.Error(w, err.Error(), http.StatusNotFound)
		} else {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}

	srv.writeJSON(w, map[string]string{"status": "deleted", "dlq_id": idStr})
}

func (srv *Server) handleAPIMessages(w http.ResponseWriter, r *http.Request) {
	data, err := srv.buildMessagesData(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	srv.writeJSON(w, data)
}

func (srv *Server) handleAPIMemory(w http.ResponseWriter, r *http.Request) {
	data, err := srv.buildMemoryData(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	srv.writeJSON(w, data)
}

// --- Data builders ---

func (srv *Server) buildOverviewData(r *http.Request) (*OverviewData, error) {
	records, err := srv.store.ListAllPipelines()
	if err != nil {
		return nil, err
	}

	views := make([]PipelineView, len(records))
	var runningAgents int
	for i, rec := range records {
		views[i] = pipelineRecordToView(rec)
		for _, agent := range views[i].Agents {
			if agent.Status == "running" {
				runningAgents++
			}
		}
	}

	queueStats, err := srv.store.CountMessagesByStatus()
	if err != nil {
		return nil, err
	}

	metrics, err := srv.store.GetPipelineMetrics()
	if err != nil {
		return nil, err
	}

	advMetrics, err := srv.store.GetAdvancedMetrics()
	if err != nil {
		// Non-fatal: advanced metrics may fail during migration transition
		advMetrics = &store.AdvancedMetrics{}
	}

	return &OverviewData{
		Pipelines: views,
		Concurrency: ConcurrencyView{
			RunningAgents:   runningAgents,
			TotalPipelines:  metrics.TotalPipelines,
			ActivePipelines: metrics.ActivePipelines,
		},
		QueueStats:      queueStats,
		Metrics:         metrics,
		AdvancedMetrics: advMetrics,
	}, nil
}

func (srv *Server) buildMessagesData(r *http.Request) (*MessagesData, error) {
	messages, err := srv.store.GetRecentMessages(100)
	if err != nil {
		return nil, err
	}

	agentFilter := r.URL.Query().Get("agent")
	statusFilter := r.URL.Query().Get("status")

	if agentFilter != "" || statusFilter != "" {
		var filtered []store.QueuedMessage
		for _, msg := range messages {
			if agentFilter != "" && msg.ToAgent != agentFilter && msg.FromAgent != agentFilter {
				continue
			}
			if statusFilter != "" && msg.Status != statusFilter {
				continue
			}
			filtered = append(filtered, msg)
		}
		messages = filtered
	}

	stats, err := srv.store.GetMessageQueueStats()
	if err != nil {
		return nil, err
	}

	return &MessagesData{
		Messages:     messages,
		QueueStats:   stats,
		FilterAgent:  agentFilter,
		FilterStatus: statusFilter,
	}, nil
}

func (srv *Server) buildMemoryData(r *http.Request) (*MemoryData, error) {
	projectID := r.URL.Query().Get("project")
	query := r.URL.Query().Get("query")

	projects, err := srv.store.ListProjects()
	if err != nil {
		return nil, err
	}

	data := &MemoryData{
		Projects:  projects,
		ProjectID: projectID,
		Query:     query,
	}

	if projectID == "" {
		return data, nil
	}

	if query != "" {
		results, err := srv.store.SearchMemory(projectID, query, 50)
		if err != nil {
			return nil, err
		}
		data.SearchResults = results
	} else {
		entries, err := srv.store.ListProjectMemory(projectID)
		if err != nil {
			return nil, err
		}
		data.Entries = entries
	}

	// Get blackboard entries for this project
	categories := []string{"hypothesis", "evidence", "decision", "constraint", "result"}
	for _, cat := range categories {
		entries, err := srv.store.GetBlackboardByCategory(projectID, cat)
		if err != nil {
			return nil, err
		}
		data.Blackboard = append(data.Blackboard, entries...)
	}

	return data, nil
}

// renderHTML renders a template to a buffer first, then writes to w.
// This prevents partial HTML output if the template has an error.
func (srv *Server) renderHTML(w http.ResponseWriter, tmplName string, data any) {
	var buf bytes.Buffer
	if err := srv.templates.Render(&buf, tmplName, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = buf.WriteTo(w)
}

func (srv *Server) writeJSON(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(data); err != nil {
		srv.logger.Printf("writeJSON encode error: %v", err)
	}
}
