package cli

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/kyago/pylon/internal/config"
	"github.com/spf13/cobra"
)

func newProjectListCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "project-list",
		Aliases: []string{"list-projects"},
		Short:   "워크스페이스의 프로젝트 목록 출력",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runProjectList(cmd.OutOrStdout())
		},
	}
}

func runProjectList(w io.Writer) error {
	root, err := resolveRoot()
	if err != nil {
		return err
	}
	projects, err := config.DiscoverProjects(root)
	if err != nil {
		return fmt.Errorf("프로젝트 탐색 실패: %w", err)
	}

	if flagJSON {
		out := make([]map[string]string, 0, len(projects)) // 비어 있어도 null이 아닌 []
		for _, p := range projects {
			out = append(out, map[string]string{"name": p.Name, "path": p.Path})
		}
		return json.NewEncoder(w).Encode(out)
	}
	if len(projects) == 0 {
		fmt.Fprintln(w, "등록된 프로젝트가 없습니다 — 'pylon add-project <git-url>'로 추가하세요")
		return nil
	}
	for _, p := range projects {
		fmt.Fprintf(w, "%s\t%s\n", p.Name, p.Path)
	}
	return nil
}
