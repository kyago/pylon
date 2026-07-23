package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type techStack struct {
	Language  string
	Framework string
	HasTests  bool
	BuildTool string
	LintTool  string
}

// detectTechStack analyzes the project directory to identify technologies.
func detectTechStack(projectDir string) techStack {
	stack := techStack{}
	detectGoStack(projectDir, &stack)
	detectNodeStack(projectDir, &stack)
	detectPythonStack(projectDir, &stack)
	detectRustStack(projectDir, &stack)
	if stack.Language == "" {
		stack.Language = "unknown"
		stack.BuildTool = "echo 'no build configured'"
	}
	return stack
}

func detectGoStack(projectDir string, stack *techStack) {
	if !fileExists(filepath.Join(projectDir, "go.mod")) {
		return
	}
	stack.Language = "go"
	stack.BuildTool = "go build ./..."
	stack.LintTool = "golangci-lint run ./..."
	if dirExists(filepath.Join(projectDir, "internal")) || dirExists(filepath.Join(projectDir, "cmd")) {
		stack.Framework = "standard-layout"
	}
	stack.HasTests = hasFilesWithSuffix(projectDir, "_test.go")
}

func detectNodeStack(projectDir string, stack *techStack) {
	if !fileExists(filepath.Join(projectDir, "package.json")) {
		return
	}
	stack.Language = "typescript"
	stack.BuildTool = "npm run build"
	stack.LintTool = "npm run lint"
	if !fileExists(filepath.Join(projectDir, "tsconfig.json")) {
		stack.Language = "javascript"
	}
	if fileExists(filepath.Join(projectDir, "next.config.js")) || fileExists(filepath.Join(projectDir, "next.config.mjs")) || fileExists(filepath.Join(projectDir, "next.config.ts")) {
		stack.Framework = "nextjs"
	} else if fileExists(filepath.Join(projectDir, "vite.config.ts")) || fileExists(filepath.Join(projectDir, "vite.config.js")) {
		stack.Framework = "vite"
	}
	stack.HasTests = dirExists(filepath.Join(projectDir, "__tests__")) || dirExists(filepath.Join(projectDir, "tests")) || dirExists(filepath.Join(projectDir, "test"))
}

func detectPythonStack(projectDir string, stack *techStack) {
	if !fileExists(filepath.Join(projectDir, "pyproject.toml")) && !fileExists(filepath.Join(projectDir, "setup.py")) && !fileExists(filepath.Join(projectDir, "requirements.txt")) {
		return
	}
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

func detectRustStack(projectDir string, stack *techStack) {
	if !fileExists(filepath.Join(projectDir, "Cargo.toml")) {
		return
	}
	stack.Language = "rust"
	stack.BuildTool = "cargo build"
	stack.LintTool = "cargo clippy"
	stack.HasTests = true
}

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
