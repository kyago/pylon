package cli

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

func newInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Initialize a new pylon workspace",
		Long: `Initialize the current directory as a pylon workspace.

Creates the .pylon/ directory structure with default configuration,
agent definitions, and runtime directories.

Spec Reference: Section 7 "pylon init"`,
		RunE: runInit,
	}
}

func runInit(cmd *cobra.Command, args []string) error {
	// Step 1: Run doctor checks
	fmt.Println("Running dependency checks...")
	passed, err := RunDoctorChecks()
	if err != nil {
		return fmt.Errorf("doctor check failed: %w", err)
	}
	if !passed {
		return fmt.Errorf("required tools are missing. Run 'pylon doctor' for details")
	}
	fmt.Println("All dependency checks passed.")
	fmt.Println()

	// Determine workspace directory
	workDir := flagWorkspace
	if workDir == "" {
		workDir, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("failed to get working directory: %w", err)
		}
	}

	// Step 2: Check if .pylon/ already exists
	pylonDir := filepath.Join(workDir, ".pylon")
	if _, err := os.Stat(pylonDir); err == nil {
		return fmt.Errorf(".pylon/ already exists in %s. Use 'pylon destroy' to remove it first", workDir)
	}

	// Step 3: Interactive input
	reader := bufio.NewReader(os.Stdin)

	fmt.Println("Pylon Workspace Initialization")
	fmt.Println(strings.Repeat("\u2500", 40))

	// Backend selection (MVP: claude-code only)
	fmt.Printf("Agent backend [claude-code]: ")
	backendInput, _ := reader.ReadString('\n')
	backendInput = strings.TrimSpace(backendInput)
	if backendInput == "" {
		backendInput = "claude-code"
	}

	// PR reviewer
	fmt.Printf("PR reviewer GitHub username: ")
	reviewerInput, _ := reader.ReadString('\n')
	reviewerInput = strings.TrimSpace(reviewerInput)
	if reviewerInput == "" {
		reviewerInput = "your-github-username"
	}

	// Step 4: Create directory structure
	// Spec Reference: Section 4 "Workspace Structure", Section 7 "pylon init"
	dirs := []string{
		filepath.Join(pylonDir, "domain"),
		filepath.Join(pylonDir, "agents"),
		filepath.Join(pylonDir, "skills"),
		filepath.Join(pylonDir, "runtime", "inbox"),
		filepath.Join(pylonDir, "runtime", "outbox"),
		filepath.Join(pylonDir, "runtime", "memory"),
		filepath.Join(pylonDir, "runtime", "sessions"),
		filepath.Join(pylonDir, "conversations"),
		filepath.Join(pylonDir, "tasks"),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	// Create config.yml (Spec Section 16: minimal config after init)
	configContent := fmt.Sprintf(`version: "0.1"

runtime:
  backend: %s
  max_concurrent: 5
  max_turns: 50
  permission_mode: acceptEdits

git:
  pr:
    reviewers:
      - %s
`, backendInput, reviewerInput)

	if err := os.WriteFile(filepath.Join(pylonDir, "config.yml"), []byte(configContent), 0644); err != nil {
		return fmt.Errorf("failed to create config.yml: %w", err)
	}

	// Create domain knowledge templates
	domainFiles := map[string]string{
		"conventions.md":  "# Coding Conventions\n\n> This file is managed by AI agents. To modify, request changes through a root agent.\n\n## Naming Rules\n\n## Code Style\n\n## Error Handling\n",
		"architecture.md": "# Architecture\n\n> This file is managed by AI agents. To modify, request changes through a root agent.\n\n## System Overview\n\n## Component Diagram\n\n## Key Decisions\n",
		"glossary.md":     "# Glossary\n\n> This file is managed by AI agents. To modify, request changes through a root agent.\n\n| Term | Definition |\n|------|------------|\n",
	}

	for name, content := range domainFiles {
		path := filepath.Join(pylonDir, "domain", name)
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			return fmt.Errorf("failed to create %s: %w", name, err)
		}
	}

	// Create agent templates (Spec Section 6: default root agents)
	if err := writeAgentTemplates(pylonDir); err != nil {
		return err
	}

	// Create .gitkeep in skills/
	if err := os.WriteFile(filepath.Join(pylonDir, "skills", ".gitkeep"), []byte(""), 0644); err != nil {
		return fmt.Errorf("failed to create skills/.gitkeep: %w", err)
	}

	// Step 5: Update .gitignore
	gitignorePath := filepath.Join(workDir, ".gitignore")
	gitignoreEntries := []string{
		"# Pylon runtime (agent communication, state)",
		".pylon/runtime/",
		".pylon/conversations/",
		"",
	}
	gitignoreContent := strings.Join(gitignoreEntries, "\n")

	f, err := os.OpenFile(gitignorePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to update .gitignore: %w", err)
	}
	defer f.Close()

	if _, err := f.WriteString("\n" + gitignoreContent); err != nil {
		return fmt.Errorf("failed to write .gitignore: %w", err)
	}

	// Step 6: git init (skip if already a git repo)
	if _, err := os.Stat(filepath.Join(workDir, ".git")); os.IsNotExist(err) {
		fmt.Println("Initializing git repository...")
		gitInit := exec.Command("git", "init", workDir)
		if out, err := gitInit.CombinedOutput(); err != nil {
			fmt.Printf("Warning: git init failed: %s\n", string(out))
		}
	}

	fmt.Println()
	fmt.Printf("Pylon workspace initialized at %s\n", workDir)
	fmt.Println()
	fmt.Println("Created:")
	fmt.Println("  .pylon/config.yml          - workspace configuration")
	fmt.Println("  .pylon/domain/             - team domain knowledge (wiki)")
	fmt.Println("  .pylon/agents/             - root agent definitions (po, pm, architect, tech-writer)")
	fmt.Println("  .pylon/skills/             - agent skills")
	fmt.Println("  .pylon/runtime/            - agent communication runtime")
	fmt.Println("  .pylon/conversations/      - conversation history")
	fmt.Println("  .pylon/tasks/              - confirmed task specs")
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Println("  1. Edit .pylon/config.yml to customize settings")
	fmt.Println("  2. Add projects: pylon add-project <name>")
	fmt.Println("  3. Start working: pylon request \"<requirement>\"")

	return nil
}

func writeAgentTemplates(pylonDir string) error {
	agentTemplates := map[string]string{
		"po.md": `---
name: po
role: Product Owner
backend: claude-code
maxTurns: 50
permissionMode: default
---

# Product Owner

## Role
Analyze user requirements through clarifying questions,
define acceptance criteria, and validate final deliverables
against business expectations.

## Workflow
1. Receive requirement -> wiki-based analysis
2. Clarify ambiguous points through questions
3. Confirm acceptance criteria -> deliver result via outbox
`,
		"pm.md": `---
name: pm
role: Project Manager
backend: claude-code
maxTurns: 50
permissionMode: acceptEdits
---

# Project Manager

## Role
Break down confirmed requirements into tasks,
assign agents, manage execution order (serial/parallel),
and handle error escalation.

## Workflow
1. Receive confirmed requirements from PO
2. Analyze technical dependencies with Architect
3. Break down into tasks -> assign to project agents
4. Monitor execution -> handle failures and retries
5. Report completion via outbox
`,
		"architect.md": `---
name: architect
role: Architect
backend: claude-code
maxTurns: 50
permissionMode: acceptEdits
---

# Architect

## Role
Make cross-project architectural decisions,
analyze technical direction and inter-project dependencies,
ensure consistency across the codebase.

## Workflow
1. Receive analysis request from PM
2. Review domain knowledge and existing architecture
3. Analyze technical direction and dependencies
4. Record decisions -> deliver result via outbox
`,
		"tech-writer.md": `---
name: tech-writer
role: Tech Writer
backend: claude-code
maxTurns: 50
permissionMode: acceptEdits
---

# Tech Writer

## Role
Maintain and update domain knowledge (wiki),
project context files, and ensure documentation
stays in sync with the codebase.

## Workflow
1. Receive update trigger (task_complete or pr_merged)
2. Analyze code changes and their impact
3. Update domain/ files and project context.md
4. Verify cross-document consistency
5. Record learnings -> deliver result via outbox

### Self-Evolution Rules
After completing a task:
1. Sync modified domain documents with related skills
2. Verify cross-document consistency (conventions <-> architecture <-> glossary)
3. Record learnings in the Learnings section below

## Learnings
_Findings from previous executions are recorded here._
`,
	}

	for name, content := range agentTemplates {
		path := filepath.Join(pylonDir, "agents", name)
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			return fmt.Errorf("failed to create agent %s: %w", name, err)
		}
	}
	return nil
}
