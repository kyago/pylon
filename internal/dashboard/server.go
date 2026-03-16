// Package dashboard provides a web-based monitoring UI for pylon pipelines.
package dashboard

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/kyago/pylon/internal/config"
	"github.com/kyago/pylon/internal/store"
)

// DashboardStore defines the store interface used by the dashboard.
type DashboardStore interface {
	// Pipeline methods
	GetPipeline(pipelineID string) (*store.PipelineRecord, error)
	GetActivePipelines() ([]store.PipelineRecord, error)
	ListAllPipelines() ([]store.PipelineRecord, error)
	UpsertPipeline(rec *store.PipelineRecord) error

	// Message methods
	CountMessagesByStatus() (map[string]int, error)
	GetMessageQueueStats() ([]store.MessageQueueStat, error)
	GetRecentMessages(limit int) ([]store.QueuedMessage, error)

	// Memory methods
	ListProjectMemory(projectID string) ([]store.MemoryEntry, error)
	SearchMemory(projectID, query string, limit int) ([]store.MemorySearchResult, error)

	// Blackboard methods
	GetBlackboardByCategory(projectID, category string) ([]store.BlackboardEntry, error)

	// Project registry
	ListProjects() ([]store.ProjectRecord, error)

	// Metrics
	GetPipelineMetrics() (*store.PipelineMetrics, error)
}

// Server is the dashboard HTTP server.
type Server struct {
	router     chi.Router
	store      DashboardStore
	cfg        *config.DashboardConfig
	hub        *SSEHub
	templates  *TemplateRenderer
	runtimeCfg *config.RuntimeConfig
}

// NewServer creates a new dashboard server with all routes registered.
func NewServer(s DashboardStore, cfg *config.DashboardConfig, runtimeCfg *config.RuntimeConfig) (*Server, error) {
	tmpl, err := NewTemplateRenderer()
	if err != nil {
		return nil, fmt.Errorf("failed to load templates: %w", err)
	}

	srv := &Server{
		store:      s,
		cfg:        cfg,
		hub:        NewSSEHub(),
		templates:  tmpl,
		runtimeCfg: runtimeCfg,
	}

	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(middleware.Logger)

	// SSE endpoint — long-lived connection, no timeout
	r.Get("/api/events", srv.handleSSEStream)

	// All other routes get a 30s timeout
	r.Group(func(r chi.Router) {
		r.Use(middleware.Timeout(30 * time.Second))

		// Pages
		r.Get("/", srv.handleOverview)
		r.Get("/pipelines/{id}", srv.handlePipelineDetail)
		r.Get("/messages", srv.handleMessages)
		r.Get("/memory", srv.handleMemory)

		// JSON API
		r.Route("/api", func(r chi.Router) {
			r.Get("/overview", srv.handleAPIOverview)
			r.Get("/pipelines", srv.handleAPIPipelines)
			r.Get("/pipelines/{id}", srv.handleAPIPipelineDetail)
			r.Post("/pipelines/{id}/cancel", srv.handleAPIPipelineCancel)
			r.Get("/messages", srv.handleAPIMessages)
			r.Get("/memory", srv.handleAPIMemory)
		})
	})

	// Static files
	staticSub, err := staticSubFS()
	if err != nil {
		return nil, fmt.Errorf("failed to load static files: %w", err)
	}
	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.FS(staticSub))))

	srv.router = r
	return srv, nil
}

// Start runs the HTTP server with graceful shutdown.
func (srv *Server) Start(ctx context.Context) error {
	go srv.hub.Run(ctx)

	poller := NewPoller(srv.store, srv.hub, srv.runtimeCfg)
	go poller.Run(ctx)

	addr := fmt.Sprintf("%s:%d", srv.cfg.Host, srv.cfg.Port)
	httpSrv := &http.Server{
		Addr:    addr,
		Handler: srv.router,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		httpSrv.Shutdown(shutdownCtx)
	}()

	log.Printf("Dashboard: http://%s", addr)
	if err := httpSrv.ListenAndServe(); err != http.ErrServerClosed {
		return fmt.Errorf("server error: %w", err)
	}
	return nil
}
