package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"unicode"

	"github.com/kyago/pylon/internal/config"
	"github.com/spf13/cobra"
)

var (
	addAgentDomain string
	addAgentRole   string
)

func newAddAgentCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add-agent <name>",
		Short: "Add a custom agent to the workspace",
		Long: `Create a new agent definition file in .pylon/agents/.
The agent will be available for PO to orchestrate in the appropriate domain.`,
		Args: cobra.ExactArgs(1),
		RunE: runAddAgent,
	}

	cmd.Flags().StringVar(&addAgentDomain, "domain", "software", "Agent domain (software/research/content/marketing)")
	cmd.Flags().StringVar(&addAgentRole, "role", "", "Agent role description (required)")

	return cmd
}

func runAddAgent(cmd *cobra.Command, args []string) error {
	name := strings.TrimSpace(args[0])
	if name == "" {
		return fmt.Errorf("에이전트 이름이 비어있습니다")
	}

	// Validate role is provided
	if addAgentRole == "" {
		return fmt.Errorf("--role 플래그가 필요합니다 (예: --role \"Backend Developer\")")
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
	agentPath := filepath.Join(pylonDir, "agents", name+".md")

	// Check if already exists
	if _, err := os.Stat(agentPath); err == nil {
		return fmt.Errorf("에이전트 '%s'가 이미 존재합니다: %s", name, agentPath)
	}

	// Capitalize for display (avoid deprecated strings.Title)
	displayName := toTitleCase(strings.ReplaceAll(name, "-", " "))

	// Generate agent template
	content := fmt.Sprintf(`---
name: %s
role: %s
domain: %s
---

# %s

## Role
%s

## Workflow
1. Receive task from PO
2. Analyze requirements and context
3. Execute task
4. Deliver results
`, name, addAgentRole, addAgentDomain, displayName, addAgentRole)

	if err := os.WriteFile(agentPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("에이전트 파일 생성 실패: %w", err)
	}

	// Sync Claude agent symlinks
	if err := syncClaudeAgentLinks(root, pylonDir); err != nil {
		fmt.Printf("⚠ .claude/agents/ 심링크 갱신 실패: %v\n", err)
	}

	fmt.Printf("✓ 에이전트 '%s' 생성 완료: %s\n", name, agentPath)
	fmt.Printf("  도메인: %s\n", addAgentDomain)
	fmt.Printf("  역할: %s\n", addAgentRole)
	fmt.Println()
	fmt.Println("에이전트 정의를 편집하여 역할과 워크플로우를 구체화하세요.")

	return nil
}

// toTitleCase capitalizes the first letter of each word.
func toTitleCase(s string) string {
	prev := ' '
	return strings.Map(func(r rune) rune {
		if prev == ' ' || prev == '\t' {
			prev = r
			return unicode.ToUpper(r)
		}
		prev = r
		return r
	}, s)
}
