package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kyago/pylon/internal/layout"
	"github.com/spf13/cobra"
)

func newDeleteProjectCmd() *cobra.Command {
	var purge, force bool

	cmd := &cobra.Command{
		Use:   "delete-project <name>",
		Short: "Remove a project from the workspace registry",
		Long: `Remove a project registration added by 'add-project'.

By default only the DB registration is removed (projects, project_memory).
The cloned directory on disk is preserved unless --purge is given.

사용 예:
  pylon delete-project myapp
  pylon delete-project myapp --purge
  pylon delete-project myapp --force`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDeleteProject(args[0], purge, force)
		},
	}

	cmd.Flags().BoolVar(&purge, "purge", false, "also delete the cloned project directory on disk")
	cmd.Flags().BoolVarP(&force, "force", "f", false, "skip confirmation prompt")

	return cmd
}

func runDeleteProject(name string, purge, force bool) error {
	if err := validateProjectName(name); err != nil {
		return err
	}

	root, _, s, err := openWorkspaceStore()
	if err != nil {
		return err
	}
	defer s.Close()

	proj, err := s.GetProject(name)
	if err != nil {
		return err
	}

	removeTarget, dirSafe := projectRemovalTarget(root, proj.Path, purge)

	if !force && !flagJSON && !confirmProjectDeletion(name, proj.Path, removeTarget, purge) {
		fmt.Println("취소되었습니다")
		return nil
	}

	res, err := s.DeleteProject(name)
	if err != nil {
		return fmt.Errorf("failed to delete project: %w", err)
	}

	removed := false
	var rmErr error
	if removeTarget != "" {
		if rmErr = os.RemoveAll(removeTarget); rmErr == nil {
			removed = true
		}
	}

	if flagJSON {
		out := map[string]any{
			"status":   "ok",
			"project":  name,
			"projects": res.Projects,
			"memory":   res.Memory,
			"purged":   purge,
			"removed":  removed,
		}
		if rmErr != nil {
			out["remove_error"] = rmErr.Error()
		}
		return printJSON(out)
	}
	printProjectDeletion(name, proj.Path, removeTarget, purge, dirSafe, removed, rmErr, res.Memory)
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
	fmt.Printf("Delete project %q:\n", name)
	fmt.Println("  - registry + memory records")
	if purge && removeTarget != "" {
		fmt.Printf("  - directory: %s\n", removeTarget)
	} else if purge {
		fmt.Printf("  ⚠ --purge requested but directory %q is outside the workspace or missing; it will be kept\n", projectPath)
	} else if removeTarget != "" {
		fmt.Printf("  - .pylon/ marker: %s\n", removeTarget)
	}
	fmt.Print("계속하시겠습니까? [y/N]: ")
	var answer string
	fmt.Scanln(&answer)
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
		fmt.Printf("⚠ 디렉터리 %q가 워크스페이스 밖이라 .pylon/ 마커를 제거하지 못함; sync-projects가 재등록할 수 있음\n", projectPath)
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
