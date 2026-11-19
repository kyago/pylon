package cli

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/kyago/pylon/internal/config"
	"github.com/kyago/pylon/internal/layout"
)

// legacyLeftovers returns the paths earlier releases wrote and no current code reads:
// .pylon/{runtime,history,skills}, root *.pylon-bak backups and per-project verify.yml.
// Workspace worktrees are not in projects, so their files are never listed.
func legacyLeftovers(root string, projects []config.ProjectInfo) []string {
	var found []string
	exists := func(p string) bool { _, err := os.Lstat(p); return err == nil }
	for _, d := range []string{"runtime", "history", "skills"} {
		if p := filepath.Join(layout.PylonDir(root), d); exists(p) {
			found = append(found, p)
		}
	}
	for _, base := range []string{"AGENTS.md", "CLAUDE.md"} {
		matches, _ := filepath.Glob(filepath.Join(root, base+rootFileBackupSuffix+"*"))
		found = append(found, matches...)
	}
	for _, p := range projects {
		if v := filepath.Join(p.Path, ".pylon", "verify.yml"); exists(v) {
			found = append(found, v)
		}
	}
	return found
}

// reportLegacy lists leftovers, or deletes them when prune is set.
func reportLegacy(root string, projects []config.ProjectInfo, prune bool) {
	left := legacyLeftovers(root, projects)
	if len(left) == 0 {
		return
	}
	fmt.Println()
	if !prune {
		fmt.Printf("ℹ 이전 버전이 남긴 파일 %d개 (삭제: pylon doctor --prune-legacy):\n", len(left))
		for _, p := range left {
			fmt.Printf("  - %s\n", relToRoot(root, p))
		}
		return
	}
	for _, p := range left {
		if err := removeLegacy(p); err != nil {
			fmt.Printf("⚠ 삭제 실패 %s: %v\n", relToRoot(root, p), err)
			continue
		}
		fmt.Printf("✓ 삭제: %s\n", relToRoot(root, p))
	}
}

// removeLegacy deletes path; .pylon/runtime/ holds read-only entries, so the
// owner-write bit is restored first.
func removeLegacy(path string) error {
	_ = filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		if info, err := d.Info(); err == nil {
			_ = os.Chmod(p, info.Mode().Perm()|0o200)
		}
		return nil
	})
	return os.RemoveAll(path)
}

func relToRoot(root, p string) string {
	if rel, err := filepath.Rel(root, p); err == nil {
		return rel
	}
	return p
}
