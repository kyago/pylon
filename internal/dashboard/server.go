// Package dashboard provides a web-based monitoring UI for pylon pipelines.
package dashboard

import (
	"context"
	"fmt"
	"log"
	"net"
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
	router        chi.Router
	store         DashboardStore
	cfg           *config.DashboardConfig
	hub           *SSEHub
	templates     *TemplateRenderer
	runtimeCfg    *config.RuntimeConfig
	WorkspaceName string // displayed in dashboard header
}

// NewServer creates a new dashboard server with all routes registered.
// workspaceName is displayed in the dashboard header to identify the workspace.
func NewServer(s DashboardStore, cfg *config.DashboardConfig, runtimeCfg *config.RuntimeConfig, workspaceName ...string) (*Server, error) {
	wsName := ""
	if len(workspaceName) > 0 {
		wsName = workspaceName[0]
	}

	tmpl, err := NewTemplateRenderer(wsName)
	if err != nil {
		return nil, fmt.Errorf("failed to load templates: %w", err)
	}

	srv := &Server{
		store:         s,
		cfg:           cfg,
		hub:           NewSSEHub(),
		templates:     tmpl,
		runtimeCfg:    runtimeCfg,
		WorkspaceName: wsName,
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

// Listen creates a TCP listener on the configured port.
// If the port is already in use, it falls back to an OS-assigned free port.
func (srv *Server) Listen() (net.Listener, error) {
	addr := fmt.Sprintf("%s:%d", srv.cfg.Host, srv.cfg.Port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		// Port busy — try OS-assigned free port
		ln, err = net.Listen("tcp", fmt.Sprintf("%s:0", srv.cfg.Host))
		if err != nil {
			return nil, fmt.Errorf("failed to listen: %w", err)
		}
	}
	return ln, nil
}

// Serve starts the SSE hub, poller, and HTTP server on the given listener.
func (srv *Server) Serve(ctx context.Context, ln net.Listener) error {
	go srv.hub.Run(ctx)

	poller := NewPoller(srv.store, srv.hub, srv.runtimeCfg)
	go poller.Run(ctx)

	httpSrv := &http.Server{
		Handler: srv.router,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		httpSrv.Shutdown(shutdownCtx)
	}()

	if err := httpSrv.Serve(ln); err != http.ErrServerClosed {
		return fmt.Errorf("server error: %w", err)
	}
	return nil
}

// Start runs the HTTP server with graceful shutdown (backward compatible).
func (srv *Server) Start(ctx context.Context) error {
	ln, err := srv.Listen()
	if err != nil {
		return err
	}
	log.Printf("Dashboard: http://%s", ln.Addr().String())
	return srv.Serve(ctx, ln)
}
