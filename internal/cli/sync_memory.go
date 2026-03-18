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
	"github.com/kyago/pylon/internal/store"
)

const (
	// maxWriteContentLen is the maximum length of Write tool content stored in memory.
	maxWriteContentLen = 500
	// maxEditStringLen is the maximum length of Edit tool old/new strings stored in memory.
	maxEditStringLen = 200
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

Claude Code Hook에서 자동 호출되어 세션 종료 시 또는 파일 변경 시
학습 내용을 project_memory(SQLite + BM25 FTS)에 저장합니다.

사용 예:
  pylon sync-memory --from-session --project myapp --agent architect
  pylon sync-memory --incremental --project myapp --file src/main.go --content "리팩토링 완료"`,
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
			return runSyncIncremental(project, agent, filePath, content)
		},
	}

	cmd.Flags().BoolVar(&fromSession, "from-session", false, "세션 종료 시 전체 학습 내용 동기화")
	cmd.Flags().BoolVar(&incremental, "incremental", false, "파일 변경 단위 메모리 갱신")
	cmd.Flags().StringVar(&project, "project", "", "대상 프로젝트 이름")
	cmd.Flags().StringVar(&agent, "agent", "claude", "에이전트 이름")
	cmd.Flags().StringVar(&content, "content", "", "학습 내용 (생략 시 stdin에서 읽음)")
	cmd.Flags().StringVar(&filePath, "file", "", "변경된 파일 경로 (--incremental 시 사용)")

	return cmd
}

// runSyncFromSession handles --from-session: stores session learnings into project memory.
// runSyncFromSession handles --from-session: stores session learnings into project memory.
// Note: Stop hook fires after EVERY Claude response turn, NOT only at session end.
// Pipeline completion is handled by the parent process (RunInteractive path) or
// stale pipeline cleanup at next startup (ExecInteractive path).
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

// runSyncIncremental handles --incremental: records file change context to memory.
// If PYLON_PIPELINE_ID is set, also updates the TUI pipeline's timestamp so the
// dashboard Poller detects activity.
func runSyncIncremental(project, agent, filePath, content string) error {
	root, _, s, err := openWorkspaceStore()
	if err != nil {
		return err
	}
	defer s.Close()

	// Touch pipeline timestamp if TUI session is active
	if pipelineID := os.Getenv("PYLON_PIPELINE_ID"); strings.HasPrefix(pipelineID, "tui-") {
		_ = s.TouchPipelineTimestamp(pipelineID)
	}

	// Resolve project name
	project, err = resolveProject(root, project)
	if err != nil {
		return err
	}

	// Read content from stdin if not provided via flag
	if content == "" {
		content = readStdin()
	}

	// Try to parse Claude Code hook payload from stdin content
	// PostToolUse hooks receive JSON like:
	//   {"tool_name": "Write", "tool_input": {"file_path": "...", "content": "..."}, ...}
	if content != "" {
		parsedFile, parsedContent := tryParseToolUsePayload(content)
		if parsedContent != "" {
			content = parsedContent
		}
		if parsedFile != "" && filePath == "" {
			filePath = parsedFile
		}
	}

	if content == "" {
		if flagJSON {
			fmt.Println(`{"status":"skip","reason":"no content provided"}`)
		} else {
			fmt.Println("저장할 내용이 없습니다")
		}
		return nil
	}

	// Build memory key from file path or timestamp
	key := buildIncrementalKey(filePath)

	entry := &store.MemoryEntry{
		ProjectID:  project,
		Category:   "change",
		Key:        key,
		Content:    content,
		Author:     agent,
		Confidence: 0.7,
	}

	if err := s.InsertMemory(entry); err != nil {
		return fmt.Errorf("변경 내용 저장 실패: %w", err)
	}

	if flagJSON {
		data, _ := json.Marshal(map[string]string{
			"status":  "ok",
			"id":      entry.ID,
			"project": project,
			"key":     key,
		})
		fmt.Println(string(data))
	} else {
		fmt.Printf("✓ 변경 기록 저장: %s/%s\n", project, key)
	}

	return nil
}

// resolveProject determines the project name from flag or workspace context.
func resolveProject(root, project string) (string, error) {
	if project != "" {
		return project, nil
	}

	// Try to infer from discovered projects
	projects, err := config.DiscoverProjects(root)
	if err != nil {
		// If project discovery fails, fall back to workspace directory name
		return filepath.Base(root), nil
	}

	switch len(projects) {
	case 0:
		// No projects discovered: fall back to workspace directory name
		return filepath.Base(root), nil
	case 1:
		// Single project discovered: use its name
		return projects[0].Name, nil
	default:
		// Multiple projects: fall back to workspace directory name.
		// Hooks cannot pass --project dynamically, so erroring here would
		// cause all hook invocations to fail in multi-project workspaces.
		return filepath.Base(root), nil
	}
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

// tryParseToolUsePayload attempts to extract file path and content from a
// Claude Code PostToolUse hook JSON payload. The payload looks like:
//
//	{"tool_name": "Write", "tool_input": {"file_path": "...", "content": "..."}, ...}
//
// Returns the extracted file path and a content summary. If the payload cannot
// be parsed, returns empty strings so the caller can fall through.
func tryParseToolUsePayload(data string) (filePath, content string) {
	type toolInput struct {
		FilePath string `json:"file_path"`
		Content  string `json:"content"`
		// Edit tool may use different fields
		OldString string `json:"old_string"`
		NewString string `json:"new_string"`
	}
	type toolUsePayload struct {
		ToolName  string    `json:"tool_name"`
		ToolInput toolInput `json:"tool_input"`
	}

	var payload toolUsePayload
	if err := json.Unmarshal([]byte(data), &payload); err != nil {
		return "", ""
	}

	filePath = payload.ToolInput.FilePath

	// Build a meaningful content description based on the tool
	switch payload.ToolName {
	case "Write":
		if payload.ToolInput.Content != "" {
			// For large file writes, store a truncated summary
			c := payload.ToolInput.Content
			if len(c) > maxWriteContentLen {
				c = c[:maxWriteContentLen] + "... (truncated)"
			}
			content = fmt.Sprintf("[Write] %s: %s", filePath, c)
		} else {
			content = fmt.Sprintf("[Write] %s", filePath)
		}
	case "Edit":
		if payload.ToolInput.OldString != "" && payload.ToolInput.NewString != "" {
			old := payload.ToolInput.OldString
			if len(old) > maxEditStringLen {
				old = old[:maxEditStringLen] + "..."
			}
			new := payload.ToolInput.NewString
			if len(new) > maxEditStringLen {
				new = new[:maxEditStringLen] + "..."
			}
			content = fmt.Sprintf("[Edit] %s: %q → %q", filePath, old, new)
		} else {
			content = fmt.Sprintf("[Edit] %s", filePath)
		}
	default:
		if filePath != "" {
			content = fmt.Sprintf("[%s] %s", payload.ToolName, filePath)
		}
	}

	return filePath, content
}

// buildIncrementalKey creates a memory key for incremental file change tracking.
// The key does not include the category prefix since that is stored separately.
func buildIncrementalKey(filePath string) string {
	ts := time.Now().Format("20060102-150405.000000000")
	if filePath != "" {
		// Use file path as part of key for traceability
		// Normalize both forward and back slashes for cross-platform compatibility
		clean := strings.ReplaceAll(filePath, "/", "-")
		clean = strings.ReplaceAll(clean, "\\", "-")
		clean = strings.ReplaceAll(clean, ".", "-")
		if len(clean) > 40 {
			clean = clean[len(clean)-40:]
		}
		return fmt.Sprintf("%s/%s", clean, ts)
	}
	return ts
}

