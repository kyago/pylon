package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kyago/pylon/internal/config"
	"github.com/kyago/pylon/internal/layout"
	"github.com/kyago/pylon/internal/memory"
	"github.com/spf13/cobra"
)

func newDeleteProjectCmd() *cobra.Command {
	var purge, force bool

	cmd := &cobra.Command{
		Use:   "delete-project <name>",
		Short: "프로젝트를 워크스페이스에서 제거",
		Long: `'add-project'로 추가한 프로젝트를 제거합니다.

기본은 프로젝트 메모리(.pylon/memory/<name>/)와 프로젝트의 .pylon/ 표시만
지웁니다. clone한 디렉터리는 --purge를 줄 때만 지웁니다.

사용 예:
  pylon delete-project myapp
  pylon delete-project myapp --purge
  pylon delete-project myapp --force`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDeleteProject(args[0], purge, force)
		},
	}

	cmd.Flags().BoolVar(&purge, "purge", false, "clone한 프로젝트 디렉터리도 삭제")
	cmd.Flags().BoolVarP(&force, "force", "f", false, "확인 프롬프트 생략")

	return cmd
}

func runDeleteProject(name string, purge, force bool) error {
	if err := validateProjectName(name); err != nil {
		return err
	}

	root, err := resolveRoot()
	if err != nil {
		return err
	}

	// 레지스트리 대신 워크스페이스 스캔으로 프로젝트를 찾는다.
	discovered, err := config.DiscoverProjects(root)
	if err != nil {
		return fmt.Errorf("프로젝트 탐색 실패: %w", err)
	}
	projPath := ""
	for _, p := range discovered {
		if p.Name == name {
			projPath = p.Path
			break
		}
	}
	if projPath == "" {
		return fmt.Errorf("프로젝트 %q은(는) 등록되어 있지 않습니다", name)
	}

	removeTarget, dirSafe := projectRemovalTarget(root, projPath, purge)

	if !force && !flagJSON && !confirmProjectDeletion(name, projPath, removeTarget, purge) {
		fmt.Println("취소되었습니다")
		return nil
	}

	memDeleted, err := memory.NewStore(root).DeleteProject(name)
	if err != nil {
		return fmt.Errorf("프로젝트 메모리 삭제 실패: %w", err)
	}

	removed := false
	var rmErr error
	if removeTarget != "" {
		if rmErr = os.RemoveAll(removeTarget); rmErr == nil {
			removed = true
		}
	}
	refreshProjectList(root)

	if flagJSON {
		out := map[string]any{
			"status":   "ok",
			"project":  name,
			"projects": 1,
			"memory":   memDeleted,
			"purged":   purge,
			"removed":  removed,
		}
		if rmErr != nil {
			out["remove_error"] = rmErr.Error()
		}
		return printJSON(out)
	}
	printProjectDeletion(name, projPath, removeTarget, purge, dirSafe, removed, rmErr, memDeleted)
	return nil
}

func projectRemovalTarget(root, projectPath string, purge bool) (string, bool) {
	projectDir, safe := resolveProjectDir(root, projectPath)
	if !safe {
		return "", false
	}
	if purge {
		return projectDir, true
	}
	marker := layout.PylonDir(projectDir)
	if dirExists(marker) {
		return marker, true
	}
	return "", true
}

func confirmProjectDeletion(name, projectPath, removeTarget string, purge bool) bool {
	fmt.Printf("프로젝트 %q 삭제:\n", name)
	fmt.Println("  - 메모리 (.pylon/memory/" + name + "/)")
	if purge && removeTarget != "" {
		fmt.Printf("  - 디렉터리: %s\n", removeTarget)
	} else if purge {
		fmt.Printf("  ⚠ --purge를 지정했지만 디렉터리 %q이(가) 워크스페이스 밖이거나 없어 그대로 둡니다\n", projectPath)
	} else if removeTarget != "" {
		fmt.Printf("  - .pylon/ 표시: %s\n", removeTarget)
	}
	fmt.Print("계속하시겠습니까? [y/N]: ")
	var answer string
	_, _ = fmt.Scanln(&answer) // 입력이 없으면(EOF) 빈 문자열 = 거부
	answer = strings.ToLower(strings.TrimSpace(answer))
	return answer == "y" || answer == "yes"
}

func printProjectDeletion(name, projectPath, removeTarget string, purge, dirSafe, removed bool, rmErr error, memoryCount int64) {
	fmt.Printf("✓ 프로젝트 %q 등록 해제 (memory %d건 정리)\n", name, memoryCount)
	switch {
	case removed && purge:
		fmt.Printf("✓ 디렉터리 삭제: %s\n", removeTarget)
	case removed:
		fmt.Printf("✓ .pylon/ 마커 제거 (소스 보존): %s\n", removeTarget)
	case rmErr != nil:
		fmt.Printf("⚠ 삭제 실패 (레지스트리는 정리됨): %v\n", rmErr)
	case purge:
		fmt.Printf("⚠ 디렉터리 %q가 워크스페이스 밖이거나 없어 보존됨\n", projectPath)
	case !dirSafe:
		fmt.Printf("⚠ 디렉터리 %q가 워크스페이스 밖이라 .pylon/ 마커를 제거하지 못함\n", projectPath)
	}
}

// resolveProjectDir returns the absolute project directory, and whether it is
// safe to modify: the path must resolve to a location strictly under the
// workspace root and must currently exist as a directory.
func resolveProjectDir(root, projPath string) (string, bool) {
	if projPath == "" {
		return "", false
	}

	dir := projPath
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(root, dir)
	}
	dir = filepath.Clean(dir)

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", false
	}
	absRoot = filepath.Clean(absRoot)

	rel, err := filepath.Rel(absRoot, dir)
	if err != nil {
		return "", false
	}
	// Must be strictly under root (not root itself, not an ancestor).
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}

	if !dirExists(dir) {
		return "", false
	}
	return dir, true
}
