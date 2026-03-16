package cli

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/kyago/pylon/internal/config"
	"github.com/kyago/pylon/internal/store"
)

func newSyncProjectsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "sync-projects",
		Short: "Sync project list to database",
		Long: `워크스페이스의 프로젝트 목록을 SQLite에 동기화합니다.

config.yml의 projects 설정과 파일시스템의 .pylon/ 디렉토리를
스캔하여 projects 테이블을 갱신합니다.

기존 사용자나 DB 누락 시 강제 갱신 용도로 사용합니다.`,
		RunE: runSyncProjects,
	}
}

func runSyncProjects(cmd *cobra.Command, args []string) error {
	root, cfg, s, err := openWorkspaceStore()
	if err != nil {
		return err
	}
	defer s.Close()

	// 1. config.yml의 projects 맵에서 등록
	for name, proj := range cfg.Projects {
		projectPath := filepath.Join(root, name)
		if err := s.UpsertProject(&store.ProjectRecord{
			ProjectID: name,
			Path:      projectPath,
			Stack:     proj.Stack,
		}); err != nil {
			fmt.Printf("⚠ %s 등록 실패: %v\n", name, err)
		}
	}

	// 2. 파일시스템 스캔 (.pylon/ 디렉토리를 가진 서브디렉토리)
	discovered, err := config.DiscoverProjects(root)
	if err != nil {
		fmt.Printf("⚠ 프로젝트 탐색 실패: %v\n", err)
	}

	for _, p := range discovered {
		stack := ""
		if proj, ok := cfg.Projects[p.Name]; ok {
			stack = proj.Stack
		}
		if err := s.UpsertProject(&store.ProjectRecord{
			ProjectID: p.Name,
			Path:      p.Path,
			Stack:     stack,
		}); err != nil {
			fmt.Printf("⚠ %s 등록 실패: %v\n", p.Name, err)
		}
	}

	// 중복 제거된 최종 결과 표시
	projects, err := s.ListProjects()
	if err != nil {
		return fmt.Errorf("프로젝트 조회 실패: %w", err)
	}

	if len(projects) == 0 {
		fmt.Println("등록된 프로젝트가 없습니다")
		return nil
	}

	fmt.Printf("%-20s %-40s %s\n", "PROJECT", "PATH", "STACK")
	for _, p := range projects {
		fmt.Printf("%-20s %-40s %s\n", p.ProjectID, p.Path, p.Stack)
	}
	fmt.Printf("\n✓ %d개 프로젝트 동기화 완료\n", len(projects))

	return nil
}
