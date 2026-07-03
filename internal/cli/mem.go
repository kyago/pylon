package cli

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/kyago/pylon/internal/store"
)

func newMemCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mem",
		Short: "Manage project memory (knowledge store)",
		Long: `프로젝트 메모리(지식 저장소)를 관리합니다.

루트 에이전트(PO)가 프로젝트 지식을 저장/검색할 때 사용합니다.

사용 가능한 하위 명령:
  search  BM25 검색
  store   메모리 저장
  list    메모리 목록
  delete  메모리 삭제 (키/카테고리 단위)
  prune   오염 메모리 정리 (카테고리 일괄/중복 제거)`,
	}

	cmd.AddCommand(newMemSearchCmd())
	cmd.AddCommand(newMemStoreCmd())
	cmd.AddCommand(newMemListCmd())
	cmd.AddCommand(newMemDeleteCmd())
	cmd.AddCommand(newMemPruneCmd())

	return cmd
}

func newMemSearchCmd() *cobra.Command {
	var project string
	var query string
	var limit int

	cmd := &cobra.Command{
		Use:   "search",
		Short: "Search project memory using BM25",
		RunE: func(cmd *cobra.Command, args []string) error {
			if project == "" || query == "" {
				return fmt.Errorf("--project and --query are required")
			}
			return runMemSearch(project, query, limit)
		},
	}

	cmd.Flags().StringVar(&project, "project", "", "project name")
	cmd.Flags().StringVar(&query, "query", "", "search query")
	cmd.Flags().IntVar(&limit, "limit", 10, "max results")

	return cmd
}

func newMemStoreCmd() *cobra.Command {
	var project string
	var key string
	var content string
	var category string
	var author string
	var confidence float64

	cmd := &cobra.Command{
		Use:   "store",
		Short: "Store a memory entry",
		RunE: func(cmd *cobra.Command, args []string) error {
			if project == "" || key == "" || content == "" {
				return fmt.Errorf("--project, --key, and --content are required")
			}
			return runMemStore(project, key, content, category, author, confidence)
		},
	}

	cmd.Flags().StringVar(&project, "project", "", "project name")
	cmd.Flags().StringVar(&key, "key", "", "memory key")
	cmd.Flags().StringVar(&content, "content", "", "memory content")
	cmd.Flags().StringVar(&category, "category", "general", "memory category")
	cmd.Flags().StringVar(&author, "author", "agent", "author identifier")
	cmd.Flags().Float64Var(&confidence, "confidence", 0.8, "confidence score (0-1)")

	return cmd
}

func newMemListCmd() *cobra.Command {
	var project string
	var category string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List memory entries for a project",
		RunE: func(cmd *cobra.Command, args []string) error {
			if project == "" {
				return fmt.Errorf("--project is required")
			}
			return runMemList(project, category)
		},
	}

	cmd.Flags().StringVar(&project, "project", "", "project name")
	cmd.Flags().StringVar(&category, "category", "", "filter by category (optional)")

	return cmd
}

func newMemDeleteCmd() *cobra.Command {
	var project, key, category string
	var dryRun, yes bool

	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete memory entries by key or category",
		Long: `메모리 항목을 삭제합니다.

사용 예:
  pylon mem delete --project myapp --key some-key
  pylon mem delete --project myapp --category change --dry-run`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if project == "" {
				return fmt.Errorf("--project is required")
			}
			if key == "" && category == "" {
				return fmt.Errorf("--key or --category is required")
			}
			return runMemDelete(project, category, key, dryRun, yes)
		},
	}

	cmd.Flags().StringVar(&project, "project", "", "project name")
	cmd.Flags().StringVar(&key, "key", "", "memory key to delete")
	cmd.Flags().StringVar(&category, "category", "", "delete all entries in this category")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show affected count without deleting")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip confirmation prompt")

	return cmd
}

func newMemPruneCmd() *cobra.Command {
	var project, category string
	var dedup, dryRun, yes bool

	cmd := &cobra.Command{
		Use:   "prune",
		Short: "Prune polluted memory entries (bulk delete / dedup)",
		Long: `오염된 메모리를 정리합니다. 삭제 후 VACUUM으로 저장 공간을 회수합니다.

사용 예:
  pylon mem prune --category change                # 전체 프로젝트의 change 일괄 삭제
  pylon mem prune --category change --project app  # 특정 프로젝트만
  pylon mem prune --dedup                          # 동일 파일 중복 change 중 최신 1건만 유지
  pylon mem prune --category change --dry-run      # 영향 건수 미리보기`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if !dedup && category == "" {
				return fmt.Errorf("--category or --dedup is required")
			}
			return runMemPrune(project, category, dedup, dryRun, yes)
		},
	}

	cmd.Flags().StringVar(&project, "project", "", "project name (empty = all projects)")
	cmd.Flags().StringVar(&category, "category", "", "delete all entries in this category")
	cmd.Flags().BoolVar(&dedup, "dedup", false, "keep only the latest change entry per file")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show affected count without deleting")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip confirmation prompt")

	return cmd
}

// confirmDeletion asks the user to confirm a destructive operation.
func confirmDeletion(count int64) bool {
	fmt.Printf("%d건이 삭제됩니다. 계속하시겠습니까? [y/N]: ", count)
	var answer string
	fmt.Scanln(&answer)
	answer = strings.ToLower(strings.TrimSpace(answer))
	return answer == "y" || answer == "yes"
}

func printPruneResult(action string, count int64, dryRun bool) {
	if flagJSON {
		data, _ := json.Marshal(map[string]any{
			"status":  "ok",
			"action":  action,
			"count":   count,
			"dry_run": dryRun,
		})
		fmt.Println(string(data))
	} else if dryRun {
		fmt.Printf("%d건이 삭제 대상입니다 (dry-run, 실제 삭제되지 않음)\n", count)
	} else {
		fmt.Printf("✓ %d건 삭제 완료\n", count)
	}
}

func runMemDelete(project, category, key string, dryRun, yes bool) error {
	_, _, s, err := openWorkspaceStore()
	if err != nil {
		return err
	}
	defer s.Close()

	count, err := s.DeleteMemory(project, category, key, true)
	if err != nil {
		return fmt.Errorf("failed to count memory: %w", err)
	}

	if dryRun || count == 0 {
		printPruneResult("delete", count, true)
		return nil
	}

	if !yes && !flagJSON && !confirmDeletion(count) {
		fmt.Println("취소되었습니다")
		return nil
	}

	deleted, err := s.DeleteMemory(project, category, key, false)
	if err != nil {
		return fmt.Errorf("failed to delete memory: %w", err)
	}

	printPruneResult("delete", deleted, false)
	return nil
}

func runMemPrune(project, category string, dedup, dryRun, yes bool) error {
	_, _, s, err := openWorkspaceStore()
	if err != nil {
		return err
	}
	defer s.Close()

	countFn := func(dry bool) (int64, error) {
		if dedup {
			return s.DedupMemoryChanges(project, dry)
		}
		return s.DeleteMemory(project, category, "", dry)
	}

	action := "prune"
	if dedup {
		action = "dedup"
	}

	count, err := countFn(true)
	if err != nil {
		return fmt.Errorf("failed to count memory: %w", err)
	}

	if dryRun || count == 0 {
		printPruneResult(action, count, true)
		return nil
	}

	if !yes && !flagJSON && !confirmDeletion(count) {
		fmt.Println("취소되었습니다")
		return nil
	}

	deleted, err := countFn(false)
	if err != nil {
		return fmt.Errorf("failed to prune memory: %w", err)
	}

	if err := s.Vacuum(); err != nil {
		return err
	}

	printPruneResult(action, deleted, false)
	return nil
}

func runMemSearch(project, query string, limit int) error {
	_, _, s, err := openWorkspaceStore()
	if err != nil {
		return err
	}
	defer s.Close()

	results, err := s.SearchMemory(project, query, limit)
	if err != nil {
		return fmt.Errorf("search failed: %w", err)
	}

	if len(results) == 0 {
		if flagJSON {
			fmt.Println("[]")
		} else {
			fmt.Println("검색 결과가 없습니다")
		}
		return nil
	}

	if flagJSON {
		type resultOut struct {
			Key        string  `json:"key"`
			Category   string  `json:"category"`
			Content    string  `json:"content"`
			Confidence float64 `json:"confidence"`
			Rank       float64 `json:"rank"`
		}
		out := make([]resultOut, len(results))
		for i, r := range results {
			out[i] = resultOut{
				Key:        r.Key,
				Category:   r.Category,
				Content:    r.Content,
				Confidence: r.Confidence,
				Rank:       r.Rank,
			}
		}
		data, _ := json.MarshalIndent(out, "", "  ")
		fmt.Println(string(data))
	} else {
		for _, r := range results {
			fmt.Printf("[%s/%s] (rank: %.2f, confidence: %.1f)\n", r.Category, r.Key, r.Rank, r.Confidence)
			fmt.Printf("  %s\n\n", r.Content)
		}
	}

	return nil
}

func runMemStore(project, key, content, category, author string, confidence float64) error {
	_, _, s, err := openWorkspaceStore()
	if err != nil {
		return err
	}
	defer s.Close()

	entry := &store.MemoryEntry{
		ProjectID:  project,
		Category:   category,
		Key:        key,
		Content:    content,
		Author:     author,
		Confidence: confidence,
	}

	if err := s.InsertMemory(entry); err != nil {
		return fmt.Errorf("failed to store memory: %w", err)
	}

	if flagJSON {
		data, _ := json.Marshal(map[string]string{
			"id":      entry.ID,
			"project": project,
			"key":     key,
			"status":  "ok",
		})
		fmt.Println(string(data))
	} else {
		fmt.Printf("✓ 메모리 저장: %s/%s/%s\n", project, category, key)
	}

	return nil
}

func runMemList(project, category string) error {
	_, _, s, err := openWorkspaceStore()
	if err != nil {
		return err
	}
	defer s.Close()

	var entries []store.MemoryEntry

	if category != "" {
		entries, err = s.GetMemoryByCategory(project, category)
	} else {
		entries, err = s.ListProjectMemory(project)
	}
	if err != nil {
		return fmt.Errorf("failed to list memory: %w", err)
	}

	if len(entries) == 0 {
		if flagJSON {
			fmt.Println("[]")
		} else {
			fmt.Println("메모리 항목이 없습니다")
		}
		return nil
	}

	if flagJSON {
		type entryOut struct {
			Key        string  `json:"key"`
			Category   string  `json:"category"`
			Content    string  `json:"content"`
			Author     string  `json:"author"`
			Confidence float64 `json:"confidence"`
		}
		out := make([]entryOut, len(entries))
		for i, e := range entries {
			out[i] = entryOut{
				Key:        e.Key,
				Category:   e.Category,
				Content:    e.Content,
				Author:     e.Author,
				Confidence: e.Confidence,
			}
		}
		data, _ := json.MarshalIndent(out, "", "  ")
		fmt.Println(string(data))
	} else {
		fmt.Printf("%-15s %-15s %-40s %s\n", "CATEGORY", "KEY", "CONTENT", "AUTHOR")
		for _, e := range entries {
			content := e.Content
			if len(content) > 40 {
				content = content[:37] + "..."
			}
			fmt.Printf("%-15s %-15s %-40s %s\n", e.Category, e.Key, content, e.Author)
		}
	}

	return nil
}
