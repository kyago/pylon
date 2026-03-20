# Pylon 적응형 워크플로우 & 스웜 패턴 결합 제안서

> **작성일**: 2026-03-19
> **참조**: [ClawTeam (HKUDS)](https://github.com/HKUDS/ClawTeam) — Agent Swarm Intelligence Framework
> **목적**: Pylon의 12단계 고정 파이프라인을 작업 복잡도에 따라 적응하는 유연한 워크플로우로 개선하고, ClawTeam의 에이전트 무관 스웜 패턴을 결합하여 확장성을 확보

---

## 목차

1. [문제 정의](#1-문제-정의)
2. [ClawTeam 핵심 패턴 분석](#2-clawteam-핵심-패턴-분석)
3. [제안: 적응형 워크플로우 시스템](#3-제안-적응형-워크플로우-시스템)
4. [제안: 에이전트 무관 스웜 레이어](#4-제안-에이전트-무관-스웜-레이어)
5. [아키텍처 변경안](#5-아키텍처-변경안)
6. [구현 우선순위 로드맵](#6-구현-우선순위-로드맵)
7. [리스크 및 트레이드오프](#7-리스크-및-트레이드오프)

---

## 1. 문제 정의

### 현재 상태

Pylon의 12단계 파이프라인은 **모든 작업에 동일한 경로**를 강제한다:

```
init → po_conversation → architect_analysis → pm_task_breakdown → task_review
     → agent_executing → verification → pr_creation → po_validation
     → wiki_update → completed
```

`validTransitions` 맵이 단계 건너뛰기를 차단하므로, 단순 버그 수정도 architect 분석과 PM 작업 분해를 거쳐야 한다.

### 문제 시나리오

| 작업 유형 | 실제 필요 단계 | 현재 강제 단계 | 낭비 |
|-----------|---------------|---------------|------|
| 단순 버그 수정 | PO → Dev → Verify → PR | 12단계 전체 | Architect, PM, Task Review, Wiki 불필요 |
| 문서화 | PO → Tech Writer → PR | 12단계 전체 | Architect, PM, Verify 불필요 |
| 이슈 파악/탐색 | PO → Explorer/Researcher | 12단계 전체 | 대부분 불필요 (코드 변경 없음) |
| 코드 리뷰 | Reviewer → 결과 보고 | 12단계 전체 | 전체 불필요 |
| 대규모 기능 개발 | 12단계 전체 | 12단계 전체 | 적합 |

**핵심 문제**: 파이프라인이 "대규모 기능 개발"에 최적화되어 있어, 일상적 개발 작업의 80%에서 불필요한 오버헤드가 발생한다.

### 현재 유연성 메커니즘의 한계

- `auto_approve_task_review: true` → 하나의 게이트만 생략
- `wiki.auto_update: false` → 마지막 단계만 생략
- `verify.yml` 미설정 → 검증 단계만 생략
- `pylon stage transition` → `validTransitions` 그래프 내에서만 이동 가능
- `ForceStage()` → 롤백 전용, 순방향 스킵 불가

**이 메커니즘들은 단계를 하나씩 끄는 것이지, 근본적으로 다른 워크플로우를 지원하지 않는다.**

---

## 2. ClawTeam 핵심 패턴 분석

ClawTeam에서 Pylon에 적용할 수 있는 핵심 패턴들:

### 패턴 1: 리더가 워크플로우를 동적으로 결정

ClawTeam에서는 리더 에이전트가 작업 복잡도를 판단하고 팀 구성을 자율적으로 결정한다. 고정된 파이프라인이 아니라 **리더의 판단**이 워크플로우를 결정한다.

**Pylon 적용**: PO 에이전트가 요구사항을 분석한 뒤, 적절한 워크플로우 템플릿을 선택하도록 한다.

### 패턴 2: 반응형 의존성 해소 (Reactive Dependency Unblocking)

ClawTeam의 태스크는 완료 시 자동으로 `blocked_by` 목록을 업데이트하고, 차단이 해제된 태스크를 `pending`으로 승격한다. 중앙 스케줄러 없이 태스크 자체가 의존성을 관리한다.

**Pylon 적용**: 현재 wave 기반 순차 실행을 반응형 태스크 그래프로 대체할 수 있다.

### 패턴 3: 에이전트 무관 프로토콜 (CLI-Based Coordination)

ClawTeam에서 에이전트는 환경변수(`CLAWTEAM_AGENT_NAME`, `CLAWTEAM_TEAM_NAME`)로 정체성을 받고, CLI 명령어로 조율한다. 이로 인해 어떤 LLM 기반 도구든 참여 가능하다.

**Pylon 적용**: Claude Code 외에 Codex, Gemini 등도 워커로 참여할 수 있는 프로토콜 레이어를 추가한다.

### 패턴 4: 템플릿 기반 팀 아키타입

ClawTeam의 TOML 템플릿은 역할, 태스크 소유권, 프롬프트를 사전 정의한다. `clawteam launch hedge-fund`처럼 한 명령으로 전체 팀을 구성한다.

**Pylon 적용**: 작업 유형별 워크플로우 템플릿(bugfix, docs, feature 등)을 정의한다.

---

## 3. 제안: 적응형 워크플로우 시스템

### 3.1 핵심 아이디어: 워크플로우 템플릿

12단계 고정 파이프라인을 유지하되, **워크플로우 템플릿**이 어떤 단계를 실행할지 결정한다.

```yaml
# .pylon/workflows/bugfix.yml
name: bugfix
description: "단순 버그 수정 워크플로우"
triggers:
  keywords: ["fix", "bug", "수정", "오류", "에러"]

stages:
  - po_conversation        # 요구사항 확인
  - agent_executing        # 바로 개발 진입
  - verification           # 빌드/테스트 검증
  - pr_creation            # PR 생성

agents:
  required: ["backend-dev"]  # 또는 프로젝트 기반 자동 선택
  optional: ["debugger"]

config:
  skip_task_graph: true      # PM 분해 없이 단일 태스크로 실행
  auto_approve: true         # PO 검증 자동 승인
```

### 3.2 워크플로우 템플릿 목록

| 템플릿 | 포함 단계 | 예상 에이전트 | 사용 시나리오 |
|--------|----------|-------------|-------------|
| **`feature`** | 12단계 전체 | PO, Architect, PM, Dev×N, Reviewer | 대규모 기능 개발 (현재와 동일) |
| **`bugfix`** | PO → Dev → Verify → PR | Dev, Debugger | 단순 버그 수정 |
| **`hotfix`** | Dev → Verify → PR | Dev | 긴급 수정 (PO 대화 생략) |
| **`docs`** | PO → Tech Writer → PR | Tech Writer | 문서 작성/수정 |
| **`explore`** | PO → Explorer → 결과 보고 | Explorer, Researcher | 이슈 파악, 코드 탐색 |
| **`review`** | Reviewer → 결과 보고 | Code Reviewer, Security Reviewer | 코드 리뷰 |
| **`refactor`** | PO → Architect → Dev → Verify → PR | Architect, Dev, Reviewer | 리팩토링 |
| **`swarm`** (Phase 5, 미구현) | PO → 동적 팀 구성 → 결과 수렴 | 리더 결정 | ClawTeam 스타일 자율 협업 — `allow_dynamic_spawn` 구현 필요 |

### 3.3 워크플로우 선택 메커니즘

워크플로우 선택은 3가지 방식으로 동작한다:

```
(1) 명시적 지정
    $ pylon request --workflow bugfix "로그인 페이지 500 에러 수정"

(2) PO 자동 분류 (기본)
    $ pylon request "로그인 페이지 500 에러 수정"
    → PO 에이전트가 요구사항을 분석하여 workflow 추천
    → 사용자 확인 후 실행

(3) 키워드 매칭 (빠른 경로)
    $ pylon request "fix: 로그인 500 에러"
    → triggers.keywords 매칭 → bugfix 워크플로우 자동 선택
```

### 3.4 구현 방안: 파이프라인 레이어에 워크플로우 레이어 추가

기존 12단계 파이프라인은 **그대로 유지**하되, 워크플로우 템플릿이 실행할 단계를 필터링한다.

```
현재 아키텍처:
  Pipeline (12 stages) → Loop → Agent Execution

제안 아키텍처:
  Workflow Template → Pipeline (N stages, N ≤ 12) → Loop → Agent Execution
                  ↓
           Stage Filter: 워크플로우에 정의된 단계만 validTransitions에 포함
```

#### 핵심 변경 포인트

**`internal/orchestrator/pipeline.go`**:

```go
// 현재: 고정된 validTransitions 맵
var validTransitions = map[Stage][]Stage{
    StageInit:               {StagePOConversation, StageFailed},
    StagePOConversation:     {StageArchitectAnalysis, StageFailed},
    // ...12단계 모두 하드코딩
}

// 제안: 워크플로우 기반 동적 전환 맵 생성
func BuildTransitions(workflow *WorkflowTemplate) map[Stage][]Stage {
    stages := workflow.Stages  // e.g., [po_conversation, agent_executing, verification, pr_creation]
    transitions := make(map[Stage][]Stage)

    // init은 항상 첫 번째 단계로 연결
    transitions[StageInit] = []Stage{stages[0], StageFailed}

    // 각 단계를 순차적으로 연결
    for i := 0; i < len(stages)-1; i++ {
        transitions[stages[i]] = []Stage{stages[i+1], StageFailed}
    }

    // 마지막 단계는 completed로 연결
    transitions[stages[len(stages)-1]] = []Stage{StageCompleted, StageFailed}

    return transitions
}
```

**`internal/orchestrator/loop.go`**:

```go
// 현재: 모든 단계에 대한 switch-case
switch l.orch.Pipeline.CurrentStage {
    case StageArchitectAnalysis: l.runHeadlessAgent(ctx, "architect", ...)
    case StagePMTaskBreakdown: l.runPMTaskBreakdown(ctx)
    // ...

// 제안: switch-case는 유지하되, 워크플로우에 없는 단계는 자동 스킵
// BuildTransitions()이 해당 단계를 전환 맵에서 제외하므로
// 도달 자체가 불가능 → 기존 핸들러 코드 변경 불필요
```

**`internal/cli/request.go`** (중요 — hardcoded 전환 수정):

```go
// 현재: request.go:240에서 PO 대화 후 StageArchitectAnalysis로 하드코딩
if err := orch.TransitionTo(orchestrator.StageArchitectAnalysis); err != nil {

// 현재: request.go:188에서 Task Review 후 StageAgentExecuting으로 하드코딩
if err := orch.TransitionTo(orchestrator.StageAgentExecuting); err != nil {

// 제안: 워크플로우 템플릿에서 다음 단계를 동적으로 조회
// NextStage()는 현재 단계 다음에 올 단계를 워크플로우 전환 맵에서 조회한다
nextStage := workflow.NextStageAfter(orch.Pipeline.CurrentStage)
if err := orch.TransitionTo(nextStage); err != nil {
```

> **주의**: `request.go`는 PO 인터랙티브 세션과 Task Review 세션에서 `validTransitions`를 우회하여
> 직접 단계를 전환한다. 이 하드코딩된 전환을 워크플로우 기반으로 변경하지 않으면,
> `feature` 이외의 워크플로우에서 PO 핸드오프 시점에 잘못된 단계로 이동하거나 에러가 발생한다.

### 3.5 `pylon request` CLI 변경

```go
// internal/cli/request.go 변경
func newRequestCmd() *cobra.Command {
    cmd := &cobra.Command{
        Use:   "request [requirement]",
        Short: "Submit a requirement",
    }

    // 새 플래그 추가
    cmd.Flags().StringP("workflow", "w", "", "워크플로우 템플릿 (feature|bugfix|hotfix|docs|explore|review|refactor)")

    return cmd
}
```

---

## 4. 제안: 에이전트 무관 스웜 레이어

### 4.1 핵심 아이디어

현재 Pylon은 Claude Code만 에이전트 백엔드로 지원한다. 에이전트 무관 레이어를 추가하여 다양한 LLM 기반 도구가 워커로 참여할 수 있게 한다.

### 4.2 Runner 인터페이스 추상화

```go
// internal/agent/runner.go — 현재
type Runner struct {
    // Claude Code 전용 실행 로직
}

// 제안: 인터페이스 추출
type AgentRunner interface {
    // Start는 에이전트를 시작하고 결과를 반환한다
    Start(ctx context.Context, task AgentTask) (*AgentResult, error)

    // IsAlive는 에이전트의 생존 여부를 확인한다
    IsAlive() bool

    // Stop은 에이전트를 중지한다
    Stop() error

    // Backend는 백엔드 이름을 반환한다 ("claude-code", "codex", "custom")
    Backend() string
}

// Claude Code 구현 (기존 코드 리팩토링)
type ClaudeCodeRunner struct { /* 기존 Runner 내용 */ }

// Codex 구현 (신규)
type CodexRunner struct { /* codex CLI 호출 */ }

// 범용 CLI 구현 (ClawTeam 스타일)
type GenericCLIRunner struct {
    Command  string   // 실행할 CLI 명령 (예: "codex", "aider")
    Args     []string
    EnvVars  map[string]string  // PYLON_AGENT_NAME, PYLON_TEAM_NAME 등
}
```

### 4.3 에이전트 설정 확장

```yaml
# .pylon/agents/codex-dev.yml
---
name: codex-dev
role: "Backend developer using Codex"
type: dev
backend: codex            # 기존 필드 (agent.go:18에 이미 존재) — "claude-code" (기본값) | "codex" | "custom"
command: "codex"           # backend=custom일 때 실행할 명령
args: ["--model", "codex-mini-latest", "--quiet"]
isolation: worktree
---
You are a backend developer...
```

### 4.4 환경변수 기반 에이전트 정체성 (ClawTeam 패턴 적용)

에이전트가 자신의 역할과 팀을 인식할 수 있도록 환경변수를 주입한다:

```go
// 에이전트 실행 시 주입되는 환경변수
env := map[string]string{
    "PYLON_AGENT_NAME":   agentName,       // "backend-dev"
    "PYLON_TEAM_NAME":    pipelineID,      // 파이프라인 ID = 팀 식별자
    "PYLON_AGENT_ROLE":   agentConfig.Role,
    "PYLON_WORKSPACE":    workspacePath,
    "PYLON_INBOX":        inboxPath,       // 메시지 수신 경로
    "PYLON_OUTBOX":       outboxPath,      // 결과 전송 경로
    "PYLON_TASK_ID":      taskID,
}
```

### 4.5 Transport 인터페이스 (ClawTeam 패턴 적용)

현재 파일시스템 기반 inbox/outbox를 인터페이스로 추상화:

```go
// internal/protocol/transport.go
type Transport interface {
    // Deliver는 메시지를 수신자에게 전달한다
    Deliver(ctx context.Context, msg MessageEnvelope) error

    // Fetch는 수신자의 메시지를 가져온다 (소비형)
    Fetch(ctx context.Context, recipient string) ([]MessageEnvelope, error)

    // Count는 대기 중인 메시지 수를 반환한다
    Count(ctx context.Context, recipient string) (int, error)

    // Watch는 새 메시지를 실시간으로 감시한다
    Watch(ctx context.Context, recipient string) (<-chan MessageEnvelope, error)
}

// 파일시스템 구현 (기존 코드 리팩토링)
type FileTransport struct { /* 현재 inbox/outbox 로직 */ }

// 향후: gRPC, NATS 등 네트워크 기반 구현 가능
```

---

## 5. 아키텍처 변경안

### 5.1 변경 전후 비교

```
=== 현재 ===

User → pylon request → Pipeline(12 stages) → Loop → ClaudeCodeRunner
                              ↓
                        고정 전환 맵
                              ↓
                    모든 작업이 동일 경로

=== 제안 ===

User → pylon request → WorkflowSelector → Pipeline(N stages) → Loop → AgentRunner
            ↓                                    ↓                        ↓
     --workflow bugfix              동적 전환 맵 생성          ClaudeCode | Codex | Custom
     또는 PO 자동 분류              (워크플로우 기반)          (Runner 인터페이스)
     또는 키워드 매칭
```

### 5.2 패키지 구조 변경

```
internal/
├── workflow/                    # [신규] 워크플로우 템플릿 시스템
│   ├── template.go             # WorkflowTemplate 타입 정의
│   ├── loader.go               # YAML 템플릿 로더
│   ├── selector.go             # 워크플로우 선택 로직 (키워드, PO, 명시적)
│   └── builtin.go              # 기본 내장 템플릿 (feature, bugfix, docs 등)
│
├── agent/
│   ├── runner.go               # [변경] AgentRunner 인터페이스 추출
│   ├── claude_runner.go        # [리팩토링] 기존 Runner → ClaudeCodeRunner
│   ├── codex_runner.go         # [신규] Codex CLI 래퍼
│   └── generic_runner.go       # [신규] 범용 CLI 래퍼
│
├── protocol/
│   ├── transport.go            # [변경] Transport 인터페이스 추출
│   ├── file_transport.go       # [리팩토링] 기존 파일 기반 transport
│   └── message.go              # [기존 유지] MessageEnvelope
│
├── orchestrator/
│   ├── pipeline.go             # [변경] BuildTransitions() 추가
│   ├── loop.go                 # [변경] 워크플로우 인식 루프
│   └── taskgraph.go            # [기존 유지]
│
├── config/
│   ├── config.go               # [변경] workflow 필드 추가
│   └── agent.go                # [변경] backend 필드 추가
│
└── cli/
    └── request.go              # [변경] --workflow 플래그 추가
```

### 5.3 워크플로우 템플릿 저장 위치

```
.pylon/
├── workflows/                  # [신규] 워크플로우 템플릿
│   ├── feature.yml             # 기본 내장 (12단계 전체)
│   ├── bugfix.yml              # 단순 버그 수정
│   ├── hotfix.yml              # 긴급 수정
│   ├── docs.yml                # 문서화
│   ├── explore.yml             # 탐색/조사
│   ├── review.yml              # 코드 리뷰
│   └── refactor.yml            # 리팩토링
│   # swarm.yml은 Phase 5에서 추가 (allow_dynamic_spawn 구현 후)
├── agents/                     # 기존 에이전트 정의
└── config.yml                  # 기존 설정
```

### 5.4 config.yml 변경

```yaml
# 현재
runtime:
  backend: claude-code

# 제안: 추가
workflow:
  default: feature              # 기본 워크플로우 (명시적 지정 없을 때)
  auto_detect: true             # PO 기반 자동 분류 활성화
  keyword_matching: true        # 키워드 기반 빠른 매칭 활성화
  custom_dir: ".pylon/workflows"  # 커스텀 워크플로우 디렉토리

# 에이전트별 백엔드 설정 (기본값은 runtime.backend)
runtime:
  backend: claude-code          # 기본 백엔드
  backends:                     # [신규] 다중 백엔드 설정
    codex:
      command: "codex"
      args: ["--model", "codex-mini-latest"]
    custom-agent:
      command: "/path/to/my-agent"
      args: ["--flag"]
```

---

## 6. 구현 우선순위 로드맵

### Phase 1: 워크플로우 템플릿 시스템 (핵심, 최우선)

> **목표**: 작업 유형에 따라 다른 파이프라인 경로를 실행할 수 있게 한다

| 항목 | 작업 내용 | 변경 파일 | 우선순위 |
|------|----------|----------|---------|
| 1-1 | `WorkflowTemplate` 타입 정의 | `internal/workflow/template.go` (신규) | P0 |
| 1-2 | YAML 템플릿 로더 | `internal/workflow/loader.go` (신규) | P0 |
| 1-3 | 내장 템플릿 7종 작성 | `.pylon/workflows/*.yml` (신규) | P0 |
| 1-4 | `BuildTransitions()` 동적 전환 맵 | `internal/orchestrator/pipeline.go` (변경) | P0 |
| 1-5 | `Pipeline` 구조체에 `WorkflowName` 필드 추가 (crash recovery 시 전환 맵 복원용) | `internal/orchestrator/pipeline.go` (변경) | P0 |
| 1-6 | `--workflow` 플래그 추가 + `request.go:240`, `request.go:188`의 하드코딩된 단계 전환을 워크플로우 기반으로 변경 | `internal/cli/request.go` (변경) | P0 |
| 1-7 | 오케스트레이션 루프에 워크플로우 주입 | `internal/orchestrator/loop.go` (변경) | P0 |
| 1-8 | config.yml workflow 섹션 | `internal/config/config.go` (변경) | P1 |

**예상 영향**: 기존 12단계 파이프라인은 `feature` 워크플로우로 매핑되어 하위 호환성 유지.

> **Pipeline 상태 영속성 (1-5 상세)**:
> 현재 `Pipeline` 구조체(`pipeline.go:68-78`)는 `CurrentStage`만 저장하고 어떤 워크플로우로 실행 중인지 기록하지 않는다.
> 파이프라인이 중단 후 `pylon request --continue`로 복구될 때, 워크플로우 이름 없이는 기본 `validTransitions`를 사용하게 되어
> 비-feature 워크플로우에서 잘못된 전환이 발생한다.
> `WorkflowName string json:"workflow_name,omitempty"`를 `Pipeline` 구조체에 추가하고,
> 복구 시 해당 이름으로 템플릿을 로드하여 `BuildTransitions()`를 재호출해야 한다.

### Phase 2: 워크플로우 자동 선택 (UX 개선)

| 항목 | 작업 내용 | 변경 파일 | 우선순위 |
|------|----------|----------|---------|
| 2-1 | 키워드 매칭 엔진 | `internal/workflow/selector.go` (신규) | P1 |
| 2-2 | PO 에이전트 워크플로우 추천 프롬프트 | `.pylon/agents/po.md` (변경) | P1 |
| 2-3 | TUI에서 워크플로우 선택 UI | `internal/cli/launch.go` (변경) | P2 |

### Phase 3: 에이전트 무관 레이어 (확장성)

| 항목 | 작업 내용 | 변경 파일 | 우선순위 |
|------|----------|----------|---------|
| 3-1 | `AgentRunner` 인터페이스 추출 | `internal/agent/runner.go` (변경) | P1 |
| 3-2 | 기존 Runner → `ClaudeCodeRunner` 리팩토링 | `internal/agent/claude_runner.go` (신규) | P1 |
| 3-3 | `Runner.Start()`가 `cfg.Agent.Backend`에 따라 디스패치하도록 변경 (현재 `runner.go:103`에서 항상 `"claude"` 호출) | `internal/agent/runner.go` (변경) | P1 |
| 3-4 | `GenericCLIRunner` 구현 | `internal/agent/generic_runner.go` (신규) | P2 |
| 3-5 | 환경변수 기반 에이전트 정체성 주입 | `internal/agent/claude_runner.go` (변경) | P2 |

> **참고**: `backend` 필드는 `internal/config/agent.go:18`에 이미 존재하며 `ResolveDefaults()`에서 상속된다.
> 실제 gap은 `Runner.Start()`(`runner.go:87-111`)가 이 필드를 무시하고 항상 `"claude"`를 호출하는 것이다.

### Phase 4: Transport 추상화 (선택적)

| 항목 | 작업 내용 | 변경 파일 | 우선순위 |
|------|----------|----------|---------|
| 4-1 | `Transport` 인터페이스 추출 | `internal/protocol/transport.go` (신규) | P2 |
| 4-2 | 기존 코드 → `FileTransport` 리팩토링 | `internal/protocol/file_transport.go` (신규) | P2 |

---

## 7. 리스크 및 트레이드오프

### 리스크

| 리스크 | 영향 | 완화 방안 |
|--------|------|----------|
| 워크플로우 선택 오류 | 부적절한 워크플로우로 작업 품질 저하 | PO 확인 단계 + `--workflow` 명시적 지정 옵션 |
| 에이전트 무관 레이어 복잡도 | 다중 백엔드 테스트/유지보수 부담 | Phase 3를 선택적으로 진행, Claude Code 우선 |
| 기존 파이프라인 호환성 | `feature` 워크플로우가 기존과 동일하게 동작하지 않을 수 있음 | `feature.yml`을 12단계 전체로 정의 + 통합 테스트 |
| 워크플로우 폭증 | 너무 많은 워크플로우가 오히려 선택 부담 | 내장 6종 + 커스텀 디렉토리 분리 |
| 파이프라인 상태 복구 실패 | crash 후 `--continue`로 복구 시 워크플로우 이름 미보존으로 잘못된 전환 발생 | `Pipeline` 구조체에 `WorkflowName` 필드 추가 (Phase 1-5) |
| `swarm` 템플릿 구현 부재 | `allow_dynamic_spawn`은 `loop.go:396-423`의 고정 에이전트 목록 구조 변경 필요 — Phase 1-4에 미포함 | `swarm.yml`은 Phase 5로 분리, Phase 1에서는 미구현으로 표기 |

### 트레이드오프

| 관점 | 현재 (고정 파이프라인) | 제안 (적응형 워크플로우) |
|------|---------------------|----------------------|
| **예측 가능성** | 항상 동일한 경로 → 높은 예측 가능성 | 워크플로우마다 다른 경로 → 약간 낮은 예측 가능성 |
| **감사/추적** | 모든 파이프라인이 동일한 히스토리 구조 | 워크플로우별로 히스토리 길이가 다름 |
| **학습 곡선** | 하나의 모델만 이해하면 됨 | 워크플로우 개념 추가 학습 필요 |
| **유연성** | 낮음 | 높음 |
| **효율성** | 단순 작업에서 낮음 | 작업에 비례하는 효율성 |

### 핵심 원칙

1. **하위 호환성**: `--workflow` 미지정 시 기본 워크플로우(`feature`)가 현재 12단계와 동일하게 동작
2. **점진적 도입**: Phase 1만 구현해도 핵심 문제 해결. Phase 2-4는 선택적
3. **단순함 우선**: 워크플로우는 "어떤 단계를 실행할지"만 결정. 새로운 실행 엔진을 만들지 않음
4. **기존 코드 최소 변경**: 파이프라인 핸들러(loop.go의 각 stage 처리)는 변경 없이 전환 맵만 동적으로 생성

---

## 부록: 워크플로우 템플릿 상세 예시

### bugfix.yml

```yaml
name: bugfix
description: "단순 버그 수정 — Architect/PM 단계 생략, 바로 개발 진입"
triggers:
  keywords: ["fix", "bug", "수정", "오류", "에러", "hotfix", "patch"]

stages:
  - po_conversation
  - agent_executing
  - verification
  - pr_creation

agents:
  required: ["backend-dev"]
  optional: ["debugger"]

config:
  skip_task_graph: true
  auto_approve: true
```

### explore.yml

```yaml
name: explore
description: "코드 탐색/이슈 파악 — 코드 변경 없음, 보고서만 생성"
triggers:
  keywords: ["탐색", "조사", "파악", "분석", "explore", "investigate", "research"]

stages:
  - po_conversation
  - agent_executing

agents:
  required: ["explorer"]
  optional: ["researcher"]

config:
  skip_task_graph: true
  skip_worktree: true
  skip_pr: true
  auto_approve: true
```

### swarm.yml (Phase 5 — 미구현, ClawTeam 스타일)

> **주의**: 이 템플릿은 `allow_dynamic_spawn: true` 기능이 필요하며,
> 이를 위해 `loop.go:396-423`의 `runAgentExecution()`이 고정 에이전트 목록 대신
> 동적 에이전트 생성을 지원하도록 변경해야 한다. Phase 1-4에서는 구현하지 않으며,
> 별도 Phase 5로 진행한다.

```yaml
name: swarm
description: "자율 스웜 — PO가 팀 구성을 동적으로 결정"
triggers:
  keywords: ["swarm", "자율", "팀"]

stages:
  - po_conversation
  - agent_executing
  - verification
  - pr_creation

agents:
  required: []                  # PO가 동적으로 결정
  dynamic: true                 # 에이전트 동적 생성 허용

config:
  skip_task_graph: false         # PO가 자체적으로 태스크 그래프 생성
  allow_dynamic_spawn: true      # 실행 중 에이전트 추가 생성 허용 (Phase 5 구현 필요)
  auto_approve: false            # PO가 최종 검증
```
