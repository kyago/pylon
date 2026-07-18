package cli

import (
	"fmt"
	"strings"

	"unicode"

	"github.com/kyago/pylon/internal/layout"
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

	agentPath, err := scaffoldMarkdownResource("에이전트", "agents", name, content)
	if err != nil {
		return err
	}

	// Sync Claude agent symlinks
	if root, rerr := resolveRoot(); rerr == nil {
		if err := syncClaudeAgentLinks(root, layout.PylonDir(root)); err != nil {
			fmt.Printf("⚠ .claude/agents/ 심링크 갱신 실패: %v\n", err)
		}
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
