package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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

	// Determine whether the cloned directory is safe to purge: it must live
	// under the workspace root and actually exist on disk.
	purgeDir := ""
	if purge {
		if dir, ok := resolvePurgeDir(root, proj.Path); ok {
			purgeDir = dir
		}
	}

	if !force && !flagJSON {
		fmt.Printf("Delete project %q:\n", name)
		fmt.Println("  - registry + memory records")
		if purge {
			if purgeDir != "" {
				fmt.Printf("  - directory: %s\n", purgeDir)
			} else {
				fmt.Printf("  ⚠ --purge requested but directory %q is outside the workspace or missing; it will be kept\n", proj.Path)
			}
		}
		fmt.Printf("계속하시겠습니까? [y/N]: ")
		var answer string
		fmt.Scanln(&answer)
		if a := strings.ToLower(strings.TrimSpace(answer)); a != "y" && a != "yes" {
			fmt.Println("취소되었습니다")
			return nil
		}
	}

	res, err := s.DeleteProject(name)
	if err != nil {
		return fmt.Errorf("failed to delete project: %w", err)
	}

	dirRemoved := false
	var dirErr error
	if purgeDir != "" {
		if dirErr = os.RemoveAll(purgeDir); dirErr == nil {
			dirRemoved = true
		}
	}

	if flagJSON {
		out := map[string]any{
			"status":      "ok",
			"project":     name,
			"projects":    res.Projects,
			"memory":      res.Memory,
			"dir_removed": dirRemoved,
		}
		if dirErr != nil {
			out["dir_error"] = dirErr.Error()
		}
		data, _ := json.Marshal(out)
		fmt.Println(string(data))
		return nil
	}

	fmt.Printf("✓ 프로젝트 %q 등록 해제 (memory %d건 정리)\n", name, res.Memory)
	if dirRemoved {
		fmt.Printf("✓ 디렉터리 삭제: %s\n", purgeDir)
	} else if dirErr != nil {
		fmt.Printf("⚠ 디렉터리 삭제 실패 (레지스트리는 정리됨): %v\n", dirErr)
	}
	return nil
}

// resolvePurgeDir returns the absolute clone directory to purge, and whether it
// is safe: the path must resolve to a location strictly under the workspace
// root and must currently exist as a directory.
func resolvePurgeDir(root, projPath string) (string, bool) {
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
