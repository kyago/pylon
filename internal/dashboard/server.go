// Package dashboard provides a web-based monitoring UI for pylon pipelines.
package dashboard

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/kyago/pylon/internal/config"
	"github.com/kyago/pylon/internal/store"
)

// requestLogger returns a Chi middleware that logs HTTP requests to the given logger.
func requestLogger(logger *log.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			start := time.Now()
			next.ServeHTTP(ww, r)
			status := ww.Status()
			if status == 0 {
				status = http.StatusOK
			}
			logger.Printf("%s %s %d %s", r.Method, r.URL.Path, status, time.Since(start))
		})
	}
}

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
	GetAdvancedMetrics() (*store.AdvancedMetrics, error)

	// DLQ
	ListDLQ() ([]store.DLQEntry, error)
	GetDLQEntry(id int) (*store.DLQEntry, error)
	DeleteDLQEntry(id int) error
	RequeueDLQ(id int) error
	CountDLQ() (int, error)
}

// Server is the dashboard HTTP server.
type Server struct {
	router        chi.Router
	store         DashboardStore
	cfg           *config.DashboardConfig
	hub           *SSEHub
	templates     *TemplateRenderer
	logger        *log.Logger
	WorkspaceName string // displayed in dashboard header
}

// NewServer creates a new dashboard server with all routes registered.
// workspaceName is displayed in the dashboard header to identify the workspace.
// logger controls where dashboard log output is written; if nil, logs are discarded.
func NewServer(s DashboardStore, cfg *config.DashboardConfig, workspaceName string, logger *log.Logger) (*Server, error) {
	if logger == nil {
		logger = log.New(io.Discard, "", 0)
	}

	tmpl, err := NewTemplateRenderer(workspaceName)
	if err != nil {
		return nil, fmt.Errorf("failed to load templates: %w", err)
	}

	srv := &Server{
		store:         s,
		cfg:           cfg,
		hub:           NewSSEHub(),
		templates:     tmpl,
		logger:        logger,
		WorkspaceName: workspaceName,
	}

	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(requestLogger(logger))

	// SSE endpoints — long-lived connections, no timeout
	r.Get("/api/events", srv.handleSSEStream)
	r.Get("/api/pipelines/{id}/logs", srv.handlePipelineLogs)

	// All other routes get a 30s timeout
	r.Group(func(r chi.Router) {
		r.Use(middleware.Timeout(30 * time.Second))

		// Pages
		r.Get("/", srv.handleOverview)
		r.Get("/pipelines/{id}", srv.handlePipelineDetail)
		r.Get("/messages", srv.handleMessages)
		r.Get("/memory", srv.handleMemory)
		r.Get("/dlq", srv.handleDLQ)

		// JSON API
		r.Route("/api", func(r chi.Router) {
			r.Get("/overview", srv.handleAPIOverview)
			r.Get("/pipelines", srv.handleAPIPipelines)
			r.Get("/pipelines/{id}", srv.handleAPIPipelineDetail)
			r.Post("/pipelines/{id}/cancel", srv.handleAPIPipelineCancel)
			r.Post("/pipelines/{id}/pause", srv.handleAPIPipelinePause)
			r.Post("/pipelines/{id}/resume", srv.handleAPIPipelineResume)
			r.Get("/messages", srv.handleAPIMessages)
			r.Get("/memory", srv.handleAPIMemory)
			r.Get("/dlq", srv.handleAPIDLQ)
			r.Post("/dlq/{id}/requeue", srv.handleAPIDLQRequeue)
			r.Delete("/dlq/{id}", srv.handleAPIDLQDelete)
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
		srv.logger.Printf("port %d unavailable (%v), falling back to random port", srv.cfg.Port, err)
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

	poller := NewPoller(srv.store, srv.hub, srv.logger)
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
	srv.logger.Printf("http://%s", ln.Addr().String())
	return srv.Serve(ctx, ln)
}
