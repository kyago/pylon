package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/kyago/pylon/internal/config"
	"github.com/kyago/pylon/internal/layout"
	"github.com/kyago/pylon/internal/memory"
)

func newMemCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mem",
		Short: "프로젝트 메모리 관리",
		Long: `프로젝트 메모리(지식 저장소)를 관리합니다.

루트 에이전트가 프로젝트 지식을 저장/검색할 때 사용합니다.`,
	}

	cmd.AddCommand(newMemSearchCmd())
	cmd.AddCommand(newMemStoreCmd())
	cmd.AddCommand(newMemListCmd())
	cmd.AddCommand(newMemDeleteCmd())
	cmd.AddCommand(newMemPruneCmd())
	cmd.AddCommand(newMemInjectCmd())

	return cmd
}

func newMemSearchCmd() *cobra.Command {
	var project string
	var query string
	var limit int

	cmd := &cobra.Command{
		Use:   "search",
		Short: "프로젝트 메모리를 토큰 매칭으로 검색",
		RunE: func(cmd *cobra.Command, args []string) error {
			if project == "" || query == "" {
				return fmt.Errorf("--project와 --query는 필수입니다")
			}
			return runMemSearch(project, query, limit)
		},
	}

	cmd.Flags().StringVar(&project, "project", "", "프로젝트 이름")
	cmd.Flags().StringVar(&query, "query", "", "검색어")
	cmd.Flags().IntVar(&limit, "limit", 10, "최대 결과 수")

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
		Short: "메모리 항목 저장",
		RunE: func(cmd *cobra.Command, args []string) error {
			if project == "" || key == "" || content == "" {
				return fmt.Errorf("--project, --key, --content는 필수입니다")
			}
			return runMemStore(project, key, content, category, author, confidence)
		},
	}

	cmd.Flags().StringVar(&project, "project", "", "프로젝트 이름")
	cmd.Flags().StringVar(&key, "key", "", "메모리 키")
	cmd.Flags().StringVar(&content, "content", "", "메모리 내용")
	cmd.Flags().StringVar(&category, "category", "general", "메모리 카테고리")
	cmd.Flags().StringVar(&author, "author", "agent", "작성자")
	cmd.Flags().Float64Var(&confidence, "confidence", 0.8, "신뢰도 (0-1)")

	return cmd
}

func newMemListCmd() *cobra.Command {
	var project string
	var category string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "프로젝트의 메모리 목록",
		RunE: func(cmd *cobra.Command, args []string) error {
			if project == "" {
				return fmt.Errorf("--project는 필수입니다")
			}
			return runMemList(project, category)
		},
	}

	cmd.Flags().StringVar(&project, "project", "", "프로젝트 이름")
	cmd.Flags().StringVar(&category, "category", "", "카테고리로 필터 (선택)")

	return cmd
}

func newMemDeleteCmd() *cobra.Command {
	var project, key, category string
	var dryRun, yes bool

	cmd := &cobra.Command{
		Use:   "delete",
		Short: "키 또는 카테고리로 메모리 삭제",
		Long: `메모리 항목을 삭제합니다.

사용 예:
  pylon mem delete --project myapp --key some-key
  pylon mem delete --project myapp --category change --dry-run`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if project == "" {
				return fmt.Errorf("--project는 필수입니다")
			}
			if key == "" && category == "" {
				return fmt.Errorf("--key 또는 --category가 필요합니다")
			}
			return runMemDelete(project, category, key, dryRun, yes)
		},
	}

	cmd.Flags().StringVar(&project, "project", "", "프로젝트 이름")
	cmd.Flags().StringVar(&key, "key", "", "삭제할 메모리 키")
	cmd.Flags().StringVar(&category, "category", "", "이 카테고리의 항목을 모두 삭제")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "삭제하지 않고 대상 건수만 표시")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "확인 프롬프트 생략")

	return cmd
}

func newMemPruneCmd() *cobra.Command {
	var project, category string
	var dryRun, yes bool

	cmd := &cobra.Command{
		Use:   "prune",
		Short: "오염된 메모리 정리 (카테고리 일괄 삭제)",
		Long: `오염된 메모리를 정리합니다.

사용 예:
  pylon mem prune --category change                # 전체 프로젝트의 change 일괄 삭제
  pylon mem prune --category change --project app  # 특정 프로젝트만
  pylon mem prune --category change --dry-run      # 영향 건수 미리보기`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if category == "" {
				return fmt.Errorf("--category는 필수입니다")
			}
			return runMemPrune(project, category, dryRun, yes)
		},
	}

	cmd.Flags().StringVar(&project, "project", "", "프로젝트 이름 (비우면 전체 프로젝트)")
	cmd.Flags().StringVar(&category, "category", "", "이 카테고리의 항목을 모두 삭제")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "삭제하지 않고 대상 건수만 표시")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "확인 프롬프트 생략")

	return cmd
}

// newMemInjectCmd stays as a hidden command for workspaces that still have the old
// SessionStart hook. Running it updates the workspace, which removes that hook.
// shortcut: 옛 훅을 지우는 전환용이다, 사용 중인 워크스페이스에서 훅이 모두 사라지면 이 명령과 PruneLegacyHookEntries를 지운다.
func newMemInjectCmd() *cobra.Command {
	return &cobra.Command{
		Use:    "inject",
		Short:  "(deprecated) 이전 릴리스의 SessionStart 훅이 호출합니다",
		Hidden: true,
		Args:   cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMemInject()
		},
	}
}

// runMemInject writes nothing to stdout: a hook's stdout becomes session context, and
// memory is now looked up on demand. 경고와 알림은 stderr로만 쓴다.
func runMemInject() error {
	root, err := resolveRoot()
	if err != nil {
		return err
	}
	refreshWorkspaceAfterUpgrade(root)
	pruneLegacyHookMemory(root)
	return nil
}

// pruneLegacyHookMemory deletes old Stop-hook payloads from every project's memory. It
// reports to stderr only, because runMemInject's stdout must stay empty.
func pruneLegacyHookMemory(root string) {
	memStore := memory.NewStore(root)
	projects, err := memStore.ListProjects()
	if err != nil {
		fmt.Fprintf(os.Stderr, "경고: 구버전 훅 항목 정리 실패: %v\n", err)
		return
	}
	for _, project := range projects {
		if n, err := memStore.PruneLegacyHookEntries(project); err != nil {
			fmt.Fprintf(os.Stderr, "경고: %s 구버전 훅 항목 정리 실패: %v\n", project, err)
		} else if n > 0 {
			fmt.Fprintf(os.Stderr, "%s: 구버전 Stop 훅이 저장한 항목 %d건 삭제\n", project, n)
		}
	}
}

// refreshWorkspaceAfterUpgrade brings a workspace up to date, which also removes the old hook.
// 내장 리소스가 임베드와 같으면 아무것도 쓰지 않는다. 세션 시작을 막지 않도록 실패는 stderr
// 경고로 끝내고, stdout은 세션 컨텍스트이므로 쓰지 않는다.
func refreshWorkspaceAfterUpgrade(root string) {
	changed, err := syncBuiltinResources(root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "경고: 내장 리소스 갱신 실패: %v\n", err)
		return
	}
	if !changed {
		return
	}
	projects, err := config.DiscoverProjects(root)
	if err != nil {
		projects = nil // 탐색 실패 시 프로젝트 목록 없이라도 갱신한다
	}
	var genErr error
	if dirExists(layout.ClaudeDir(root)) {
		genErr = generateClaudeDir(root, projects)
	} else {
		_, _, genErr = ensureRootAgentFiles(root, projects)
	}
	pruneCodexSkills(root)
	if genErr != nil {
		fmt.Fprintf(os.Stderr, "경고: 워크스페이스 파일 갱신 실패 (다음 세션에서 다시 시도합니다): %v\n", genErr)
		// 내장 리소스가 최신이면 다음 세션이 갱신을 건너뛰므로 하나를 지워 재시도하게 한다.
		_ = os.Remove(filepath.Join(layout.BuiltinResourceDir(root, "reference"), "pylon-usage.md"))
	}
}

func printJSON(value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}

func printJSONIndent(value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}

// confirmDeletion asks the user to confirm a destructive operation.
func confirmDeletion(count int64) bool {
	fmt.Printf("%d건이 삭제됩니다. 계속하시겠습니까? [y/N]: ", count)
	var answer string
	_, _ = fmt.Scanln(&answer) // 입력이 없으면(EOF) 빈 문자열 = 거부
	answer = strings.ToLower(strings.TrimSpace(answer))
	return answer == "y" || answer == "yes"
}

func printPruneResult(action string, count int64, dryRun bool) {
	if flagJSON {
		_ = printJSON(map[string]any{
			"status":  "ok",
			"action":  action,
			"count":   count,
			"dry_run": dryRun,
		})
	} else if dryRun {
		fmt.Printf("%d건이 삭제 대상입니다 (dry-run, 실제 삭제되지 않음)\n", count)
	} else {
		fmt.Printf("✓ %d건 삭제 완료\n", count)
	}
}

func runMemDelete(project, category, key string, dryRun, yes bool) error {
	root, err := resolveRoot()
	if err != nil {
		return err
	}
	memStore := memory.NewStore(root)

	count, err := memStore.Delete(project, category, key, true)
	if err != nil {
		return fmt.Errorf("메모리 건수 확인 실패: %w", err)
	}

	if dryRun || count == 0 {
		printPruneResult("delete", count, true)
		return nil
	}

	if !yes && !flagJSON && !confirmDeletion(count) {
		fmt.Println("취소되었습니다")
		return nil
	}

	deleted, err := memStore.Delete(project, category, key, false)
	if err != nil {
		return fmt.Errorf("메모리 삭제 실패: %w", err)
	}

	printPruneResult("delete", deleted, false)
	return nil
}

func runMemPrune(project, category string, dryRun, yes bool) error {
	root, err := resolveRoot()
	if err != nil {
		return err
	}
	memStore := memory.NewStore(root)

	count, err := memStore.Delete(project, category, "", true)
	if err != nil {
		return fmt.Errorf("메모리 건수 확인 실패: %w", err)
	}

	if dryRun || count == 0 {
		printPruneResult("prune", count, true)
		return nil
	}

	if !yes && !flagJSON && !confirmDeletion(count) {
		fmt.Println("취소되었습니다")
		return nil
	}

	deleted, err := memStore.Delete(project, category, "", false)
	if err != nil {
		return fmt.Errorf("메모리 정리 실패: %w", err)
	}

	printPruneResult("prune", deleted, false)
	return nil
}

func runMemSearch(project, query string, limit int) error {
	root, err := resolveRoot()
	if err != nil {
		return err
	}
	memStore := memory.NewStore(root)

	results, err := memStore.Search(project, query, limit)
	if err != nil {
		return fmt.Errorf("검색 실패: %w", err)
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
		return printJSONIndent(out)
	} else {
		for _, r := range results {
			fmt.Printf("[%s/%s] (rank: %.2f, confidence: %.1f)\n", r.Category, r.Key, r.Rank, r.Confidence)
			fmt.Printf("  %s\n\n", r.Content)
		}
	}

	return nil
}

func runMemStore(project, key, content, category, author string, confidence float64) error {
	root, err := resolveRoot()
	if err != nil {
		return err
	}
	memStore := memory.NewStore(root)

	entry := &memory.Entry{
		ProjectID:  project,
		Category:   category,
		Key:        key,
		Content:    content,
		Author:     author,
		Confidence: confidence,
	}

	if err := memStore.Insert(entry); err != nil {
		if errors.Is(err, memory.ErrDuplicate) {
			if flagJSON {
				_ = printJSON(map[string]string{
					"status": "skip",
					"reason": "duplicate content",
					"path":   entry.Path,
				})
			} else {
				fmt.Printf("동일한 내용이 이미 저장되어 있어 건너뜁니다: %s\n", entry.Path)
			}
			return nil
		}
		return fmt.Errorf("메모리 저장 실패: %w", err)
	}

	if flagJSON {
		return printJSON(map[string]any{
			"path":    entry.Path,
			"project": project,
			"key":     key,
			"status":  "ok",
			"updated": entry.Updated,
		})
	} else if entry.Updated {
		fmt.Printf("✓ 메모리 갱신됨: %s\n", entry.Path)
	} else {
		fmt.Printf("✓ 메모리 저장됨: %s\n", entry.Path)
	}

	return nil
}

func runMemList(project, category string) error {
	root, err := resolveRoot()
	if err != nil {
		return err
	}
	memStore := memory.NewStore(root)

	var entries []memory.Entry

	if category != "" {
		entries, err = memStore.ListByCategory(project, category)
	} else {
		entries, err = memStore.List(project)
	}
	if err != nil {
		return fmt.Errorf("메모리 목록 조회 실패: %w", err)
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
		return printJSONIndent(out)
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
