package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/kyago/pylon/internal/config"
	"github.com/kyago/pylon/internal/memory"
	"github.com/kyago/pylon/internal/store"
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
func runSyncIncremental(project, agent, filePath, content string) error {
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

	if content == "" {
		if flagJSON {
			fmt.Println(`{"status":"skip","reason":"no content provided"}`)
		} else {
			fmt.Println("저장할 내용이 없습니다")
		}
		return nil
	}

	_ = cfg // cfg used for memory manager creation if needed in future

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
	if err == nil && len(projects) == 1 {
		return projects[0].Name, nil
	}

	// Fall back to workspace directory name
	return filepath.Base(root), nil
}

// parseLearnings splits content into individual learning entries.
func parseLearnings(content string) ([]string, error) {
	if content == "" {
		// Try reading from stdin (non-blocking check)
		stat, _ := os.Stdin.Stat()
		if stat != nil && (stat.Mode()&os.ModeCharDevice) == 0 {
			data, err := os.ReadFile("/dev/stdin")
			if err != nil {
				return nil, fmt.Errorf("stdin 읽기 실패: %w", err)
			}
			content = string(data)
		}
	}

	if content == "" {
		return nil, nil
	}

	// Split by newlines, filter empty lines
	lines := strings.Split(content, "\n")
	var learnings []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		// Remove common list prefixes
		trimmed = strings.TrimPrefix(trimmed, "- ")
		trimmed = strings.TrimPrefix(trimmed, "* ")
		if trimmed != "" {
			learnings = append(learnings, trimmed)
		}
	}

	return learnings, nil
}

// buildIncrementalKey creates a memory key for incremental file change tracking.
func buildIncrementalKey(filePath string) string {
	ts := time.Now().Format("20060102-150405")
	if filePath != "" {
		// Use file path as part of key for traceability
		clean := strings.ReplaceAll(filePath, string(os.PathSeparator), "-")
		clean = strings.ReplaceAll(clean, ".", "-")
		if len(clean) > 40 {
			clean = clean[len(clean)-40:]
		}
		return fmt.Sprintf("change/%s/%s", clean, ts)
	}
	return fmt.Sprintf("change/%s", ts)
}
