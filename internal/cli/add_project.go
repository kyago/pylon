package cli

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/kyago/pylon/internal/config"
	"github.com/kyago/pylon/internal/store"
)

func newAddProjectCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add-project [git-url]",
		Short: "Add a project as git submodule",
		Long: `Add a project to the workspace as a git submodule.

Analyzes the codebase and creates project-level .pylon/ configuration
including context.md and default agent definitions.

If the project directory already exists, use --force to re-clone or
--skip-clone to keep the existing directory and only generate .pylon/ config.

Spec Reference: Section 7 "pylon add-project", Section 12`,
		Args: cobra.ExactArgs(1),
		RunE: runAddProject,
	}

	cmd.Flags().String("name", "", "project directory name (default: inferred from URL)")
	cmd.Flags().Bool("force", false, "remove existing directory and re-clone")
	cmd.Flags().Bool("skip-clone", false, "skip git submodule add; use existing directory for .pylon/ setup only")

	return cmd
}

func runAddProject(cmd *cobra.Command, args []string) error {
	repoURL := args[0]

	// Find workspace root
	startDir := flagWorkspace
	if startDir == "" {
		startDir = "."
	}
	root, err := config.FindWorkspaceRoot(startDir)
	if err != nil {
		return fmt.Errorf("not in a pylon workspace — run 'pylon init' first")
	}

	// Determine project name
	projectName, _ := cmd.Flags().GetString("name")
	if projectName == "" {
		projectName = inferProjectName(repoURL)
	}

	force, _ := cmd.Flags().GetBool("force")
	skipClone, _ := cmd.Flags().GetBool("skip-clone")

	if force && skipClone {
		return fmt.Errorf("--force and --skip-clone are mutually exclusive")
	}

	// Validate project name to prevent path traversal
	if err := validateProjectName(projectName); err != nil {
		return err
	}

	projectDir := filepath.Join(root, projectName)

	// Check if project already exists
	info, statErr := os.Stat(projectDir)
	switch {
	case statErr == nil:
		if !info.IsDir() {
			return fmt.Errorf("%s exists but is not a directory", projectName)
		}
		switch {
		case force:
			fmt.Printf("Removing existing directory: %s\n", projectName)
			// Remove submodule registration if it exists
			deregCmd := exec.Command("git", "submodule", "deinit", "-f", projectName)
			deregCmd.Dir = root
			if out, err := deregCmd.CombinedOutput(); err != nil && flagVerbose {
				fmt.Printf("  (submodule deinit skipped: %s)\n", strings.TrimSpace(string(out)))
			}

			rmCmd := exec.Command("git", "rm", "-f", projectName)
			rmCmd.Dir = root
			if out, err := rmCmd.CombinedOutput(); err != nil && flagVerbose {
				fmt.Printf("  (git rm skipped: %s)\n", strings.TrimSpace(string(out)))
			}

			gitModulesDir := filepath.Join(root, ".git", "modules", projectName)
			os.RemoveAll(gitModulesDir) // clean cached module data

			if err := os.RemoveAll(projectDir); err != nil {
				return fmt.Errorf("failed to remove existing directory: %w", err)
			}
		case skipClone:
			fmt.Printf("Using existing directory: %s\n", projectName)
		default:
			return fmt.Errorf("directory %s already exists (use --force to re-clone or --skip-clone to use existing)", projectName)
		}
	case !os.IsNotExist(statErr):
		return fmt.Errorf("cannot access %s: %w", projectName, statErr)
	default:
		// Path does not exist
		if skipClone {
			return fmt.Errorf("directory %s does not exist; cannot use --skip-clone", projectName)
		}
	}

	// Step 1: Add git submodule (skipped when --skip-clone)
	if !skipClone {
		fmt.Printf("Adding git submodule: %s\n", repoURL)
		gitCmd := exec.Command("git", "submodule", "add", repoURL, projectName)
		gitCmd.Dir = root
		if output, err := gitCmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to add submodule: %w\n%s", err, output)
		}
		fmt.Printf("✓ Submodule added: %s\n", projectName)
	}

	// Step 2: Create project .pylon/ structure
	pylonDir := filepath.Join(projectDir, ".pylon")
	agentsDir := filepath.Join(pylonDir, "agents")
	if err := os.MkdirAll(agentsDir, 0755); err != nil {
		return fmt.Errorf("failed to create project .pylon/: %w", err)
	}

	// Step 3: Analyze codebase and generate context.md
	fmt.Println("Analyzing codebase...")
	stack := detectTechStack(projectDir)

	contextContent := generateContextMD(projectName, stack)
	contextPath := filepath.Join(pylonDir, "context.md")
	if err := os.WriteFile(contextPath, []byte(contextContent), 0644); err != nil {
		return fmt.Errorf("failed to create context.md: %w", err)
	}
	fmt.Println("✓ context.md generated")

	// Step 4: Create verify.yml
	verifyContent := generateVerifyYML(stack)
	verifyPath := filepath.Join(pylonDir, "verify.yml")
	if err := os.WriteFile(verifyPath, []byte(verifyContent), 0644); err != nil {
		return fmt.Errorf("failed to create verify.yml: %w", err)
	}
	fmt.Println("✓ verify.yml generated")

	// Step 5: Suggest agents based on tech stack
	agents := suggestAgents(stack)
	fmt.Println()
	fmt.Println("Suggested agents:")
	for _, a := range agents {
		fmt.Printf("  - %-15s %s\n", a.name, a.description)
	}

	fmt.Println()
	reader := bufio.NewReader(os.Stdin)
	fmt.Printf("Create these agents? [Y/n]: ")
	answer, _ := reader.ReadString('\n')
	answer = strings.TrimSpace(strings.ToLower(answer))

	if answer == "" || answer == "y" || answer == "yes" {
		for _, a := range agents {
			agentPath := filepath.Join(agentsDir, a.name+".md")
			if err := os.WriteFile(agentPath, []byte(a.content), 0644); err != nil {
				return fmt.Errorf("failed to create agent %s: %w", a.name, err)
			}
		}
		fmt.Printf("✓ %d agent(s) created\n", len(agents))
	} else {
		fmt.Println("Skipped agent creation. Create manually in .pylon/agents/")
	}

	// Register project in SQLite
	if err := registerProjectInDB(root, projectName, projectDir, stack.Language); err != nil {
		fmt.Printf("⚠ DB 등록 실패: %v\n", err)
	}

	fmt.Println()
	fmt.Printf("Project %s added successfully.\n", projectName)

	// Exclude .pylon/ from submodule git tracking via .git/info/exclude
	if err := excludePylonFromSubmodule(projectDir); err != nil {
		fmt.Printf("⚠ Could not auto-exclude .pylon/ from git tracking: %v\n", err)
		fmt.Println("  Manually add '.pylon/' to the submodule's .git/info/exclude")
	} else {
		fmt.Println("✓ .pylon/ excluded from submodule git tracking")
	}

	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Printf("  1. Review: %s\n", contextPath)
	fmt.Printf("  2. Customize agents: %s\n", agentsDir)
	fmt.Printf("  3. Start working: pylon request \"<requirement>\"\n")

	return nil
}

// validateProjectName ensures the project name is a safe single directory name
// without path separators or traversal components.
func validateProjectName(name string) error {
	if name == "" {
		return fmt.Errorf("project name cannot be empty")
	}
	if name == "." || name == ".." {
		return fmt.Errorf("invalid project name: %q", name)
	}
	if strings.ContainsAny(name, `/\`) || filepath.IsAbs(name) {
		return fmt.Errorf("project name must not contain path separators: %q", name)
	}
	if filepath.Clean(name) != name {
		return fmt.Errorf("invalid project name: %q", name)
	}
	return nil
}

// inferProjectName extracts project name from git URL.
func inferProjectName(repoURL string) string {
	// Handle "https://github.com/user/repo.git" or "git@github.com:user/repo.git"
	name := repoURL
	if idx := strings.LastIndex(name, "/"); idx >= 0 {
		name = name[idx+1:]
	}
	if idx := strings.LastIndex(name, ":"); idx >= 0 {
		name = name[idx+1:]
	}
	name = strings.TrimSuffix(name, ".git")
	return name
}

// resolveGitExcludePath returns the absolute path to a project's .git/info/exclude file.
// It uses "git rev-parse --git-dir" to correctly resolve submodule git directories.
func resolveGitExcludePath(projectDir string) (string, error) {
	gitDirCmd := exec.Command("git", "rev-parse", "--git-dir")
	gitDirCmd.Dir = projectDir
	out, err := gitDirCmd.Output()
	if err != nil {
		return "", fmt.Errorf("not a git repository: %w", err)
	}

	gitDir := strings.TrimSpace(string(out))
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(projectDir, gitDir)
	}

	return filepath.Join(gitDir, "info", "exclude"), nil
}

// excludePylonFromSubmodule adds ".pylon/" to the submodule's git exclude file
// so that generated .pylon/ files do not show up as untracked changes.
func excludePylonFromSubmodule(projectDir string) error {
	excludePath, err := resolveGitExcludePath(projectDir)
	if err != nil {
		return err
	}

	// Read existing exclude file if it exists
	existing, err := os.ReadFile(excludePath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to read exclude file: %w", err)
	}
	for _, line := range strings.Split(string(existing), "\n") {
		if strings.TrimSpace(line) == ".pylon/" {
			return nil // already excluded
		}
	}

	// Ensure info/ directory exists
	infoDir := filepath.Dir(excludePath)
	if err := os.MkdirAll(infoDir, 0755); err != nil {
		return fmt.Errorf("failed to create git info directory: %w", err)
	}

	f, err := os.OpenFile(excludePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open exclude file: %w", err)
	}
	defer f.Close()

	// Add newline before entry if file doesn't end with one
	if len(existing) > 0 && existing[len(existing)-1] != '\n' {
		if _, err := f.WriteString("\n"); err != nil {
			return err
		}
	}
	if _, err := f.WriteString(".pylon/\n"); err != nil {
		return err
	}

	return nil
}

// techStack holds detected technology information.
type techStack struct {
	Language   string
	Framework  string
	HasTests   bool
	BuildTool  string
	LintTool   string
}

// detectTechStack analyzes the project directory to identify technologies.
func detectTechStack(projectDir string) techStack {
	stack := techStack{}

	// Go
	if fileExists(filepath.Join(projectDir, "go.mod")) {
		stack.Language = "go"
		stack.BuildTool = "go build ./..."
		stack.LintTool = "golangci-lint run ./..."
		if dirExists(filepath.Join(projectDir, "internal")) || dirExists(filepath.Join(projectDir, "cmd")) {
			stack.Framework = "standard-layout"
		}
		stack.HasTests = hasFilesWithSuffix(projectDir, "_test.go")
	}

	// Node.js / TypeScript
	if fileExists(filepath.Join(projectDir, "package.json")) {
		stack.Language = "typescript"
		stack.BuildTool = "npm run build"
		stack.LintTool = "npm run lint"
		if fileExists(filepath.Join(projectDir, "tsconfig.json")) {
			stack.Language = "typescript"
		} else {
			stack.Language = "javascript"
		}
		if fileExists(filepath.Join(projectDir, "next.config.js")) || fileExists(filepath.Join(projectDir, "next.config.mjs")) || fileExists(filepath.Join(projectDir, "next.config.ts")) {
			stack.Framework = "nextjs"
		} else if fileExists(filepath.Join(projectDir, "vite.config.ts")) || fileExists(filepath.Join(projectDir, "vite.config.js")) {
			stack.Framework = "vite"
		}
		stack.HasTests = dirExists(filepath.Join(projectDir, "__tests__")) ||
			dirExists(filepath.Join(projectDir, "tests")) ||
			dirExists(filepath.Join(projectDir, "test"))
	}

	// Python
	if fileExists(filepath.Join(projectDir, "pyproject.toml")) || fileExists(filepath.Join(projectDir, "setup.py")) || fileExists(filepath.Join(projectDir, "requirements.txt")) {
		stack.Language = "python"
		stack.BuildTool = "python -m pytest"
		stack.LintTool = "ruff check ."
		if fileExists(filepath.Join(projectDir, "manage.py")) {
			stack.Framework = "django"
		} else if fileExists(filepath.Join(projectDir, "app.py")) || fileExists(filepath.Join(projectDir, "main.py")) {
			stack.Framework = "fastapi"
		}
		stack.HasTests = dirExists(filepath.Join(projectDir, "tests")) || dirExists(filepath.Join(projectDir, "test"))
	}

	// Rust
	if fileExists(filepath.Join(projectDir, "Cargo.toml")) {
		stack.Language = "rust"
		stack.BuildTool = "cargo build"
		stack.LintTool = "cargo clippy"
		stack.HasTests = true // Rust tests are inline
	}

	// Fallback
	if stack.Language == "" {
		stack.Language = "unknown"
		stack.BuildTool = "echo 'no build configured'"
	}

	return stack
}

// agentSuggestion holds a suggested agent definition.
type agentSuggestion struct {
	name        string
	description string
	content     string
}

// suggestAgents returns agent suggestions based on tech stack.
func suggestAgents(stack techStack) []agentSuggestion {
	var agents []agentSuggestion

	switch stack.Language {
	case "go":
		agents = append(agents, agentSuggestion{
			name:        "backend-dev",
			description: "Go backend developer",
			content: `---
name: backend-dev
role: Backend Developer
backend: claude-code
maxTurns: 50
permissionMode: acceptEdits
---

# Backend Developer

## Role
Implement backend features, APIs, and business logic in Go.

## Tech Stack
- Language: Go
- Testing: go test
- Linting: golangci-lint
`,
		})

	case "typescript", "javascript":
		if stack.Framework == "nextjs" || stack.Framework == "vite" {
			agents = append(agents, agentSuggestion{
				name:        "frontend-dev",
				description: "Frontend developer (" + stack.Framework + ")",
				content: fmt.Sprintf(`---
name: frontend-dev
role: Frontend Developer
backend: claude-code
maxTurns: 50
permissionMode: acceptEdits
---

# Frontend Developer

## Role
Implement UI components, pages, and frontend logic.

## Tech Stack
- Language: %s
- Framework: %s
`, stack.Language, stack.Framework),
			})
		}
		agents = append(agents, agentSuggestion{
			name:        "fullstack-dev",
			description: stack.Language + " developer",
			content: fmt.Sprintf(`---
name: fullstack-dev
role: Fullstack Developer
backend: claude-code
maxTurns: 50
permissionMode: acceptEdits
---

# Fullstack Developer

## Role
Implement features across the full stack.

## Tech Stack
- Language: %s
`, stack.Language),
		})

	case "python":
		agents = append(agents, agentSuggestion{
			name:        "backend-dev",
			description: "Python backend developer",
			content: fmt.Sprintf(`---
name: backend-dev
role: Backend Developer
backend: claude-code
maxTurns: 50
permissionMode: acceptEdits
---

# Backend Developer

## Role
Implement backend features, APIs, and business logic in Python.

## Tech Stack
- Language: Python
- Framework: %s
- Testing: pytest
- Linting: ruff
`, stack.Framework),
		})

	case "rust":
		agents = append(agents, agentSuggestion{
			name:        "backend-dev",
			description: "Rust developer",
			content: `---
name: backend-dev
role: Backend Developer
backend: claude-code
maxTurns: 50
permissionMode: acceptEdits
---

# Backend Developer

## Role
Implement features and business logic in Rust.

## Tech Stack
- Language: Rust
- Build: cargo
- Linting: clippy
`,
		})

	default:
		agents = append(agents, agentSuggestion{
			name:        "developer",
			description: "General developer",
			content: `---
name: developer
role: Developer
backend: claude-code
maxTurns: 50
permissionMode: acceptEdits
---

# Developer

## Role
Implement features and business logic.
`,
		})
	}

	// Always suggest QA agent if tests exist
	if stack.HasTests {
		agents = append(agents, agentSuggestion{
			name:        "qa",
			description: "QA engineer",
			content: `---
name: qa
role: QA Engineer
backend: claude-code
maxTurns: 30
permissionMode: acceptEdits
---

# QA Engineer

## Role
Write and maintain tests, ensure code quality,
and validate acceptance criteria.
`,
		})
	}

	return agents
}

// generateContextMD creates a context.md for the project.
func generateContextMD(projectName string, stack techStack) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("# %s\n\n", projectName))
	b.WriteString("> This file is auto-generated by pylon. Tech Writer agent will keep it updated.\n\n")
	b.WriteString("## Tech Stack\n\n")
	b.WriteString(fmt.Sprintf("- **Language**: %s\n", stack.Language))
	if stack.Framework != "" {
		b.WriteString(fmt.Sprintf("- **Framework**: %s\n", stack.Framework))
	}
	if stack.BuildTool != "" {
		b.WriteString(fmt.Sprintf("- **Build**: `%s`\n", stack.BuildTool))
	}
	if stack.LintTool != "" {
		b.WriteString(fmt.Sprintf("- **Lint**: `%s`\n", stack.LintTool))
	}
	b.WriteString("\n## Architecture\n\n_To be analyzed by Architect agent._\n")
	b.WriteString("\n## Key Conventions\n\n_To be documented by Tech Writer agent._\n")
	return b.String()
}

// generateVerifyYML creates a verify.yml for cross-validation.
func generateVerifyYML(stack techStack) string {
	var b strings.Builder
	b.WriteString("commands:\n")

	if stack.BuildTool != "" {
		b.WriteString(fmt.Sprintf("  - name: build\n    command: %s\n    timeout: 5m\n", stack.BuildTool))
	}

	switch stack.Language {
	case "go":
		b.WriteString("  - name: test\n    command: go test ./... -race\n    timeout: 10m\n")
	case "typescript", "javascript":
		b.WriteString("  - name: test\n    command: npm test\n    timeout: 10m\n")
	case "python":
		b.WriteString("  - name: test\n    command: python -m pytest\n    timeout: 10m\n")
	case "rust":
		b.WriteString("  - name: test\n    command: cargo test\n    timeout: 10m\n")
	}

	if stack.LintTool != "" {
		b.WriteString(fmt.Sprintf("  - name: lint\n    command: %s\n    timeout: 3m\n", stack.LintTool))
	}

	return b.String()
}

func registerProjectInDB(root, projectName, projectDir, stackLang string) error {
	dbPath := filepath.Join(root, ".pylon", "pylon.db")
	s, err := store.NewStore(dbPath)
	if err != nil {
		return fmt.Errorf("store open: %w", err)
	}
	defer s.Close()

	if err := s.Migrate(); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	if err := s.UpsertProject(&store.ProjectRecord{
		ProjectID: projectName,
		Path:      projectDir,
		Stack:     stackLang,
	}); err != nil {
		return fmt.Errorf("upsert: %w", err)
	}
	return nil
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func hasFilesWithSuffix(dir, suffix string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), suffix) {
			return true
		}
		if e.IsDir() && e.Name() != "." && e.Name() != ".." && e.Name() != ".git" {
			if hasFilesWithSuffix(filepath.Join(dir, e.Name()), suffix) {
				return true
			}
		}
	}
	return false
}
