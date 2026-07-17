package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/kyago/pylon/internal/config"
	"github.com/kyago/pylon/internal/memory"
)

func newSyncMemoryCmd() *cobra.Command {
	var (
		fromSession bool
		incremental bool
		project     string
		agent       string
		content     string
		filePath    string
	)

	cmd := &cobra.Command{
		Use:   "sync-memory",
		Short: "Synchronize session learnings to project memory",
		Long: `세션 학습 내용을 프로젝트 메모리에 동기화합니다.

Claude Code Hook에서 자동 호출되어 세션 종료 시
학습 내용을 project_memory(SQLite + BM25 FTS)에 저장합니다.

사용 예:
  pylon sync-memory --from-session --project myapp --agent architect`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if !fromSession && !incremental {
				return fmt.Errorf("--from-session 또는 --incremental 중 하나를 지정하세요")
			}
			if fromSession && incremental {
				return fmt.Errorf("--from-session과 --incremental은 동시에 사용할 수 없습니다")
			}
			if fromSession {
				return runSyncFromSession(project, agent, content)
			}
			return runSyncIncremental()
		},
	}

	cmd.Flags().BoolVar(&fromSession, "from-session", false, "세션 종료 시 전체 학습 내용 동기화")
	cmd.Flags().BoolVar(&incremental, "incremental", false, "(deprecated) 저장하지 않음 — 파일 변경 이력은 Fossil history가 담당")
	cmd.Flags().StringVar(&project, "project", "", "대상 프로젝트 이름")
	cmd.Flags().StringVar(&agent, "agent", "claude", "에이전트 이름")
	cmd.Flags().StringVar(&content, "content", "", "학습 내용 (생략 시 stdin에서 읽음)")
	cmd.Flags().StringVar(&filePath, "file", "", "(deprecated) 사용되지 않음")
	_ = cmd.Flags().MarkDeprecated("incremental", "파일 변경 이력은 Fossil history가 담당합니다")
	_ = cmd.Flags().MarkDeprecated("file", "파일 변경 이력은 Fossil history가 담당합니다")

	return cmd
}

// runSyncFromSession handles --from-session: stores session learnings into project memory.
// Note: Stop hook fires after EVERY Claude response turn, NOT only at session end.
func runSyncFromSession(project, agent, content string) error {
	root, cfg, s, err := openWorkspaceStore()
	if err != nil {
		return err
	}
	defer s.Close()

	// Resolve project name
	project, err = resolveProject(root, project)
	if err != nil {
		return err
	}

	// Read content from stdin if not provided
	learnings, err := parseLearnings(content)
	if err != nil {
		return err
	}

	if len(learnings) == 0 {
		if flagJSON {
			fmt.Println(`{"status":"skip","reason":"no learnings to store"}`)
		} else {
			fmt.Println("저장할 학습 내용이 없습니다")
		}
		return nil
	}

	// Use memory manager to store learnings
	mgr := memory.NewManager(s, cfg.Memory)
	taskID := fmt.Sprintf("session-%s", time.Now().Format("20060102-150405"))
	if err := mgr.StoreLearnings(project, taskID, agent, learnings); err != nil {
		return fmt.Errorf("학습 내용 저장 실패: %w", err)
	}

	if flagJSON {
		data, _ := json.Marshal(map[string]any{
			"status":   "ok",
			"project":  project,
			"agent":    agent,
			"task_id":  taskID,
			"count":    len(learnings),
			"category": "learning",
		})
		fmt.Println(string(data))
	} else {
		fmt.Printf("✓ %d개 학습 내용을 %s 프로젝트에 저장했습니다 (task: %s)\n", len(learnings), project, taskID)
	}

	return nil
}

// runSyncIncremental is a deprecated no-op. File-change history is recorded
// by Fossil history checkpoints (execution-summary.json의 changed_files);
// storing raw changes in project_memory polluted BM25 search (issue #76).
func runSyncIncremental() error {
	// Drain piped stdin so PostToolUse hooks do not hit a broken pipe.
	_ = readStdin()
	if flagJSON {
		fmt.Println(`{"status":"skip","reason":"change tracking moved to fossil history"}`)
	} else {
		fmt.Println("파일 변경 이력은 Fossil history가 담당합니다 — 저장을 건너뜁니다")
	}
	return nil
}

// resolveProject determines the project name from flag or workspace context.
func resolveProject(root, project string) (string, error) {
	if project != "" {
		return project, nil
	}
	projects, err := config.DiscoverProjects(root)
	if err != nil || len(projects) == 0 {
		return filepath.Base(root), nil
	}
	if len(projects) == 1 {
		return projects[0].Name, nil
	}
	// Hooks cannot pass --project dynamically, so erroring here would
	// cause all hook invocations to fail in multi-project workspaces.
	return filepath.Base(root), nil
}

// readStdin reads stdin if data is piped (non-interactive).
func readStdin() string {
	stat, _ := os.Stdin.Stat()
	if stat != nil && (stat.Mode()&os.ModeCharDevice) == 0 {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(data))
	}
	return ""
}

// parseLearnings splits content into individual learning entries.
// Handles both JSON payloads (from Claude Code hooks) and plain text.
func parseLearnings(content string) ([]string, error) {
	if content == "" {
		// Try reading from stdin (non-blocking check)
		content = readStdin()
	}

	if content == "" {
		return nil, nil
	}

	trimmed := strings.TrimSpace(content)

	// If the content looks like JSON (typical for Claude Code hooks), try to decode it
	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		learnings := tryParseJSONLearnings(trimmed)
		if len(learnings) > 0 {
			return learnings, nil
		}
		// If JSON decoding succeeded but yielded no usable fields,
		// fall through to the plain-text splitting logic below.
	}

	// Split by newlines, filter empty lines
	lines := strings.Split(content, "\n")
	var learnings []string
	for _, line := range lines {
		t := strings.TrimSpace(line)
		if t == "" {
			continue
		}
		// Remove common list prefixes
		t = strings.TrimPrefix(t, "- ")
		t = strings.TrimPrefix(t, "* ")
		if t != "" {
			learnings = append(learnings, t)
		}
	}

	return learnings, nil
}

// tryParseJSONLearnings attempts to extract learnings from JSON hook payload.
func tryParseJSONLearnings(data string) []string {
	type hookPayload struct {
		Summary    string   `json:"summary"`
		Content    string   `json:"content"`
		Learnings  []string `json:"learnings"`
		ToolOutput string   `json:"tool_output"`
	}

	var payload hookPayload
	if err := json.Unmarshal([]byte(data), &payload); err != nil {
		return nil
	}

	var learnings []string

	// Prefer explicitly provided learnings array
	if len(payload.Learnings) > 0 {
		learnings = append(learnings, payload.Learnings...)
	}

	// Also consider common text fields that may contain summaries
	for _, field := range []string{payload.Summary, payload.Content, payload.ToolOutput} {
		t := strings.TrimSpace(field)
		if t != "" {
			learnings = append(learnings, t)
		}
	}

	return learnings
}

