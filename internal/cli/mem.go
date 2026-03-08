package cli

import (
	"encoding/json"
	"fmt"

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
  list    메모리 목록`,
	}

	cmd.AddCommand(newMemSearchCmd())
	cmd.AddCommand(newMemStoreCmd())
	cmd.AddCommand(newMemListCmd())

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
