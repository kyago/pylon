# Dev Agent 병렬 실행 & 기본 에이전트 추가 구현 계획

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Dev 에이전트를 병렬로 실행하고, `pylon init` 시 Claude Code CLI에서 바로 사용할 수 있는 커뮤니티 기반 기본 에이전트를 생성한다.

**Architecture:** errgroup을 사용한 병렬 에이전트 실행 (max_concurrent 설정 존중). init 시 po/pm/architect/tech-writer(기존) + backend-dev/frontend-dev/code-reviewer/security-reviewer/test-engineer(신규) 총 9개 에이전트 생성.

**Tech Stack:** Go, golang.org/x/sync/errgroup (이미 go.mod에 indirect 포함)

---

## Chunk 1: Dev Agent 병렬 실행

### Task 1: errgroup 기반 병렬 실행 테스트

**Files:**
- Modify: `internal/orchestrator/loop_test.go`
- Modify: `internal/orchestrator/loop.go:247-263`

- [ ] **Step 1: 병렬 실행 테스트 작성**

`loop_test.go`에 다중 dev agent 병렬 실행 테스트를 추가한다.

```go
func TestLoop_Run_ParallelDevAgents(t *testing.T) {
	dir := t.TempDir()
	exec := &testExecutor{exitCode: 0}
	lcfg := newTestLoopConfig(dir, exec)
	// 2개의 dev agent 추가
	lcfg.Agents["frontend-dev"] = &config.AgentConfig{
		Name: "frontend-dev", Role: "프론트엔드 개발자",
		PermissionMode: "acceptEdits", MaxTurns: 30,
	}
	loop := NewLoop(lcfg)

	loop.orch.StartPipeline("test-pipeline")
	loop.orch.TransitionTo(StagePOConversation)
	loop.orch.TransitionTo(StageArchitectAnalysis)
	loop.orch.TransitionTo(StagePMTaskBreakdown)
	loop.orch.TransitionTo(StageAgentExecuting)

	err := loop.runAgentExecution(context.Background())

	// Should execute both dev agents (backend-dev + frontend-dev)
	if err != nil {
		// PR creation will fail (no git), but agents should have been called
		t.Logf("error (expected in test): %v", err)
	}
	if len(exec.runCalls) < 2 {
		t.Errorf("expected at least 2 agent calls, got %d", len(exec.runCalls))
	}
}
```

- [ ] **Step 2: 테스트 실행 → 실패 확인**

Run: `go test ./internal/orchestrator/ -run TestLoop_Run_ParallelDevAgents -v`
Expected: PASS (현재 순차 실행으로도 통과, 병렬 변경 후에도 통과해야 함)

- [ ] **Step 3: runAgentExecution을 errgroup 기반 병렬로 변경**

`loop.go`의 `runAgentExecution`을 수정한다:

```go
import "golang.org/x/sync/errgroup"

func (l *Loop) runAgentExecution(ctx context.Context) error {
	devAgents := l.findDevAgents()
	if len(devAgents) == 0 {
		return fmt.Errorf("no dev agents configured (expected agents with name: backend-dev, frontend-dev, or fullstack)")
	}

	// Single agent: no need for goroutine overhead
	if len(devAgents) == 1 {
		if err := l.executeAgent(ctx, devAgents[0]); err != nil {
			return err
		}
		return l.transitionTo(StageVerification)
	}

	// Multiple agents: run in parallel with concurrency limit
	maxConcurrent := l.cfg.Config.Runtime.MaxConcurrent
	if maxConcurrent <= 0 {
		maxConcurrent = 5
	}

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(maxConcurrent)

	for _, name := range devAgents {
		agentName := name // capture loop variable
		g.Go(func() error {
			return l.executeAgent(gctx, agentName)
		})
	}

	if err := g.Wait(); err != nil {
		return err
	}

	return l.transitionTo(StageVerification)
}
```

- [ ] **Step 4: go mod tidy로 errgroup 의존성 정리**

Run: `go mod tidy`

- [ ] **Step 5: 빌드 및 전체 테스트**

Run: `go build ./... && go test ./internal/orchestrator/... -v`
Expected: 전체 PASS

- [ ] **Step 6: 커밋**

```bash
git add internal/orchestrator/loop.go internal/orchestrator/loop_test.go go.mod go.sum
git commit -m "feat: dev agent 병렬 실행 지원 (errgroup 기반)"
```

---

## Chunk 2: 기본 Dev 에이전트 추가

### Task 2: init 시 dev 에이전트 5종 생성

**Files:**
- Modify: `internal/cli/init_cmd.go:186-291` (`writeAgentTemplates`)
- Modify: `internal/cli/init_test.go` (있다면)

커뮤니티에서 널리 사용되는 5개 dev 에이전트를 추가한다. 참고 소스:
- shanraisshan/claude-code-best-practice (72k stars)
- oh-my-claudecode (32 agents)
- obra/superpowers

에이전트 목록:
1. **backend-dev** — 백엔드 개발 (Go, Python, Java 등)
2. **frontend-dev** — 프론트엔드 개발 (React, Vue, Angular 등)
3. **code-reviewer** — 코드 리뷰 (READ-ONLY, disallowedTools 패턴)
4. **security-reviewer** — 보안 분석 (READ-ONLY)
5. **test-engineer** — 테스트 작성 및 검증

- [ ] **Step 1: 기존 init_test.go 확인**

Run: `go test ./internal/cli/ -run TestInit -v` (테스트 존재 여부 확인)

- [ ] **Step 2: writeAgentTemplates에 5개 dev 에이전트 추가**

`init_cmd.go`의 `writeAgentTemplates`에 추가:

```go
"backend-dev.md": `---
name: backend-dev
role: Backend Developer
backend: claude-code
maxTurns: 30
permissionMode: acceptEdits
isolation: worktree
---

# Backend Developer

## Role
Implement backend features, APIs, and data layers.
Follow project conventions and maintain test coverage.

## Guidelines
- Read existing code patterns before implementing
- Write tests alongside implementation
- Wrap all errors with context before returning
- Follow the project's established architecture
`,
"frontend-dev.md": `---
name: frontend-dev
role: Frontend Developer
backend: claude-code
maxTurns: 30
permissionMode: acceptEdits
isolation: worktree
---

# Frontend Developer

## Role
Implement UI components, pages, and client-side logic.
Follow design system conventions and ensure accessibility.

## Guidelines
- Match existing component patterns and naming
- Write unit tests for components and hooks
- Ensure responsive design and accessibility (WCAG 2.1 AA)
- Handle loading, error, and empty states
`,
"code-reviewer.md": `---
name: code-reviewer
role: Code Reviewer
backend: claude-code
maxTurns: 10
permissionMode: default
disallowedTools:
  - Edit
  - Write
  - NotebookEdit
---

# Code Reviewer

## Role
Review code changes for bugs, security vulnerabilities,
performance issues, and adherence to project conventions.
This agent is READ-ONLY — it cannot modify files.

## Review Checklist
1. Logic errors and edge cases
2. Security vulnerabilities (OWASP Top 10)
3. Performance implications
4. Test coverage adequacy
5. Naming and code style consistency
6. Error handling completeness

## Output Format
Report issues with confidence levels (HIGH/MEDIUM/LOW).
Only report HIGH confidence issues by default.
`,
"security-reviewer.md": `---
name: security-reviewer
role: Security Reviewer
backend: claude-code
maxTurns: 10
permissionMode: default
disallowedTools:
  - Edit
  - Write
  - NotebookEdit
---

# Security Reviewer

## Role
Analyze code for security vulnerabilities, authentication
flaws, injection risks, and data exposure issues.
This agent is READ-ONLY — it cannot modify files.

## Focus Areas
1. Input validation and sanitization
2. Authentication and authorization
3. SQL/NoSQL injection
4. XSS and CSRF vulnerabilities
5. Sensitive data exposure (secrets, PII)
6. Dependency vulnerabilities
7. Access control and privilege escalation

## Output Format
Report findings with severity (CRITICAL/HIGH/MEDIUM/LOW)
and remediation suggestions.
`,
"test-engineer.md": `---
name: test-engineer
role: Test Engineer
backend: claude-code
maxTurns: 30
permissionMode: acceptEdits
isolation: worktree
---

# Test Engineer

## Role
Write and maintain tests — unit, integration, and E2E.
Ensure adequate coverage for new and modified code.

## Guidelines
- Follow existing test patterns and frameworks
- Use table-driven tests where appropriate
- Test both happy paths and error cases
- Mock external dependencies, not internal logic
- Aim for meaningful coverage, not percentage targets
`,
```

- [ ] **Step 3: init 출력 메시지 업데이트**

`init_cmd.go`의 "Created:" 출력에서 에이전트 목록을 업데이트:

```go
fmt.Println("  .pylon/agents/             - agent definitions (9 agents)")
```

- [ ] **Step 4: findDevAgents에 test-engineer 추가 검토**

`loop.go`의 `findDevAgents`의 `devRoles` 맵에 `test-engineer`는 추가하지 않는다.
test-engineer는 dev agent가 아닌 독립적 역할이며, 별도 스테이지(verification)에서 활용될 수 있다.

- [ ] **Step 5: 빌드 및 테스트**

Run: `go build ./... && go test ./internal/cli/... -v`
Expected: PASS

- [ ] **Step 6: 커밋**

```bash
git add internal/cli/init_cmd.go
git commit -m "feat: pylon init 시 dev 에이전트 5종 기본 생성"
```

---

## Chunk 3: 통합 테스트 및 마무리

### Task 3: 통합 검증

- [ ] **Step 1: 전체 빌드 및 테스트**

Run: `make build && make test`
Expected: 전체 PASS

- [ ] **Step 2: init으로 생성되는 에이전트 수동 확인**

Run: `cd /tmp && mkdir pylon-test && cd pylon-test && git init && pylon init`
Expected: `.pylon/agents/`에 9개 `.md` 파일 생성 확인

- [ ] **Step 3: 최종 커밋 (필요 시)**

변경사항이 있으면 커밋.

---

## 파일 변경 요약

| 파일 | 액션 | 목적 |
|------|------|------|
| `internal/orchestrator/loop.go` | MODIFY | runAgentExecution을 errgroup 병렬로 변경 |
| `internal/orchestrator/loop_test.go` | MODIFY | 병렬 실행 테스트 추가 |
| `internal/cli/init_cmd.go` | MODIFY | dev 에이전트 5종 템플릿 추가 |
| `go.mod` / `go.sum` | MODIFY | errgroup direct 의존성 |
