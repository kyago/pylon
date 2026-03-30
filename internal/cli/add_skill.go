package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kyago/pylon/internal/config"
	"github.com/spf13/cobra"
)

func newAddSkillCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "add-skill <name>",
		Short: "Add a custom skill to the workspace",
		Long: `Create a new skill definition file in .pylon/skills/.
Skills provide domain knowledge that agents can reference during execution.`,
		Args: cobra.ExactArgs(1),
		RunE: runAddSkill,
	}
}

func runAddSkill(cmd *cobra.Command, args []string) error {
	name := strings.TrimSpace(args[0])
	if name == "" {
		return fmt.Errorf("스킬 이름이 비어있습니다")
	}

	// Find workspace
	startDir := flagWorkspace
	if startDir == "" {
		startDir = "."
	}
	root, err := config.FindWorkspaceRoot(startDir)
	if err != nil {
		return fmt.Errorf("pylon 워크스페이스가 아닙니다 — 'pylon init'을 먼저 실행하세요")
	}

	pylonDir := filepath.Join(root, ".pylon")
	skillPath := filepath.Join(pylonDir, "skills", name+".md")

	// Ensure skills directory exists
	skillsDir := filepath.Join(pylonDir, "skills")
	if err := os.MkdirAll(skillsDir, 0755); err != nil {
		return fmt.Errorf("skills 디렉토리 생성 실패: %w", err)
	}

	// Check if already exists
	if _, err := os.Stat(skillPath); err == nil {
		return fmt.Errorf("스킬 '%s'가 이미 존재합니다: %s", name, skillPath)
	}

	// Generate skill template
	displayName := toTitleCase(strings.ReplaceAll(name, "-", " "))

	content := fmt.Sprintf(`---
name: %s
description: "%s 스킬 — 설명을 작성하세요"
---

# %s

## 개요

이 스킬의 목적과 사용 시나리오를 설명하세요.

## 가이드

1. 첫 번째 단계
2. 두 번째 단계
3. 세 번째 단계
`, name, displayName, displayName)

	if err := os.WriteFile(skillPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("스킬 파일 생성 실패: %w", err)
	}

	fmt.Printf("✓ 스킬 '%s' 생성 완료: %s\n", name, skillPath)
	fmt.Println()
	fmt.Println("스킬 파일을 편집하여 내용을 구체화하세요.")
	fmt.Println("에이전트에 연결하려면 에이전트 .md의 skills 필드에 추가하세요:")
	fmt.Printf("  skills:\n    - %s\n", name)

	return nil
}
