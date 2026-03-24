package cli

import (
	"bufio"
	"embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/kyago/pylon/internal/config"
	"github.com/kyago/pylon/internal/store"
)

//go:embed agents/*.md
var embeddedAgents embed.FS

//go:embed scripts/bash/*.sh
var embeddedScripts embed.FS

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
	passed, err := RunDoctorChecks()
	if err != nil {
		return fmt.Errorf("doctor check failed: %w", err)
	}
	if !passed {
		return fmt.Errorf("required tools are missing — install them and retry 'pylon init'")
	}
	fmt.Println("All checks passed.")
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

	// Backend is fixed to claude-code in MVP
	backendInput := "claude-code"
	fmt.Printf("Agent backend: %s\n", backendInput)

	// PR reviewer
	fmt.Printf("PR reviewer GitHub username (Enter to skip): ")
	reviewerInput, _ := reader.ReadString('\n')
	reviewerInput = strings.TrimSpace(reviewerInput)

	// Step 4: Create directory structure
	// Spec Reference: Section 4 "Workspace Structure", Section 7 "pylon init"
	dirs := []string{
		filepath.Join(pylonDir, "domain"),
		filepath.Join(pylonDir, "agents"),
		filepath.Join(pylonDir, "skills"),
		filepath.Join(pylonDir, "scripts", "bash"),
		filepath.Join(pylonDir, "commands"),
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
	reviewerSection := "    reviewers: []"
	if reviewerInput != "" {
		reviewerSection = fmt.Sprintf("    reviewers:\n      - %s", reviewerInput)
	}

	configContent := fmt.Sprintf(`version: "0.1"

runtime:
  backend: %s
  max_concurrent: 5
  max_turns: 50
  permission_mode: acceptEdits

git:
  pr:
%s
`, backendInput, reviewerSection)

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

	// Create pipeline script templates
	if err := writeScriptTemplates(pylonDir); err != nil {
		return err
	}

	// Create symlinks in .claude/agents/ for Claude CLI native discovery
	if err := syncClaudeAgentLinks(workDir, pylonDir); err != nil {
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
		"# Claude CLI agent symlinks (managed by pylon)",
		".claude/agents/",
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

	// Step 7: Initialize DB and sync discovered projects
	dbPath := filepath.Join(pylonDir, "pylon.db")
	s, err := store.NewStore(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open store: %w", err)
	}
	defer s.Close()

	if err := s.Migrate(); err != nil {
		return fmt.Errorf("failed to migrate: %w", err)
	}

	projects, err := config.DiscoverProjects(workDir)
	if err != nil {
		fmt.Printf("⚠ 프로젝트 탐색 실패: %v\n", err)
	}
	for _, p := range projects {
		if err := s.UpsertProject(&store.ProjectRecord{
			ProjectID: p.Name,
			Path:      p.Path,
		}); err != nil {
			fmt.Printf("⚠ %s 등록 실패: %v\n", p.Name, err)
		}
	}
	if len(projects) > 0 {
		fmt.Printf("✓ %d project(s) registered in DB\n", len(projects))
	}

	fmt.Println()
	fmt.Printf("Pylon workspace initialized at %s\n", workDir)
	fmt.Println()
	fmt.Println("Created:")
	fmt.Println("  .pylon/config.yml          - workspace configuration")
	fmt.Println("  .pylon/domain/             - team domain knowledge (wiki)")
	fmt.Println("  .pylon/agents/             - agent definitions (23 agents)")
	fmt.Println("  .pylon/skills/             - agent skills")
	fmt.Println("  .pylon/scripts/bash/       - pipeline shell scripts")
	fmt.Println("  .pylon/commands/           - pipeline slash commands")
	fmt.Println("  .pylon/runtime/            - agent communication runtime")
	fmt.Println("  .pylon/conversations/      - conversation history")
	fmt.Println("  .pylon/tasks/              - confirmed task specs")
	fmt.Println("  .claude/agents/            - Claude CLI symlinks (-> .pylon/agents/)")
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Println("  1. Edit .pylon/config.yml to customize settings")
	fmt.Println("  2. Add projects: pylon add-project <name>")
	fmt.Println("  3. Start working: /pl:pipeline in Claude Code TUI")

	return nil
}


func writeAgentTemplates(pylonDir string) error {
	entries, err := embeddedAgents.ReadDir("agents")
	if err != nil {
		return fmt.Errorf("failed to read embedded agents: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		content, err := embeddedAgents.ReadFile("agents/" + entry.Name())
		if err != nil {
			return fmt.Errorf("failed to read agent %s: %w", entry.Name(), err)
		}
		path := filepath.Join(pylonDir, "agents", entry.Name())
		if err := os.WriteFile(path, content, 0644); err != nil {
			return fmt.Errorf("failed to create agent %s: %w", entry.Name(), err)
		}
	}
	return nil
}

func writeScriptTemplates(pylonDir string) error {
	entries, err := embeddedScripts.ReadDir("scripts/bash")
	if err != nil {
		return fmt.Errorf("failed to read embedded scripts: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sh") {
			continue
		}
		content, err := embeddedScripts.ReadFile("scripts/bash/" + entry.Name())
		if err != nil {
			return fmt.Errorf("failed to read script %s: %w", entry.Name(), err)
		}
		path := filepath.Join(pylonDir, "scripts", "bash", entry.Name())
		if err := os.WriteFile(path, content, 0755); err != nil {
			return fmt.Errorf("failed to create script %s: %w", entry.Name(), err)
		}
	}
	return nil
}
