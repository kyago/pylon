package dashboard

import (
	"embed"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"time"

	"github.com/kyago/pylon/internal/domain"
)

//go:embed templates/*.html templates/partials/*.html
var templateFS embed.FS

//go:embed static/*
var staticFS embed.FS

// staticSubFS returns a sub-filesystem rooted at "static/".
func staticSubFS() (fs.FS, error) {
	return fs.Sub(staticFS, "static")
}

// TemplateRenderer caches parsed templates.
type TemplateRenderer struct {
	templates *template.Template
}

// NewTemplateRenderer parses all embedded templates with helper functions.
func NewTemplateRenderer() (*TemplateRenderer, error) {
	funcMap := template.FuncMap{
		"stageIndex": stageIndex,
		"stageLabel": stageLabel,
		"timeAgo":    timeAgo,
		"isTerminal": isTerminal,
		"allStages":  allStages,
		"sub":        func(a, b int) int { return a - b },
		"add":        func(a, b int) int { return a + b },
		"pct": func(a, b int) float64 {
			if b == 0 {
				return 0
			}
			return float64(a) / float64(b) * 100
		},
		"formatTime": func(t time.Time) string {
			return t.Format("2006-01-02 15:04:05")
		},
		"truncate": func(s string, n int) string {
			if len(s) <= n {
				return s
			}
			return s[:n] + "..."
		},
		"statusClass": func(status string) string {
			switch status {
			case "running":
				return "status-running"
			case "completed":
				return "status-completed"
			case "failed":
				return "status-failed"
			default:
				return "status-default"
			}
		},
	}

	tmpl, err := template.New("").Funcs(funcMap).ParseFS(
		templateFS, "templates/*.html", "templates/partials/*.html",
	)
	if err != nil {
		return nil, fmt.Errorf("failed to parse templates: %w", err)
	}

	return &TemplateRenderer{templates: tmpl}, nil
}

// Render executes a named template.
func (tr *TemplateRenderer) Render(w io.Writer, name string, data any) error {
	return tr.templates.ExecuteTemplate(w, name, data)
}

func stageIndex(stage string) int {
	for i, s := range domain.AllStages() {
		if string(s) == stage {
			return i
		}
	}
	return -1
}

func stageLabel(stage string) string {
	labels := map[string]string{
		"init":               "Init",
		"po_conversation":    "PO Conv",
		"architect_analysis": "Architect",
		"pm_task_breakdown":  "PM Tasks",
		"agent_executing":    "Executing",
		"verification":       "Verify",
		"pr_creation":        "PR Create",
		"po_validation":      "PO Valid",
		"wiki_update":        "Wiki",
		"completed":          "Done",
		"failed":             "Failed",
	}
	if l, ok := labels[stage]; ok {
		return l
	}
	return stage
}

func timeAgo(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}

func isTerminal(stage string) bool {
	return stage == "completed" || stage == "failed"
}

func allStages() []domain.Stage {
	return domain.AllStages()
}
