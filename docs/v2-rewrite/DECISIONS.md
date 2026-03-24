# Pylon v2 아키텍처 의사결정 기록 (ADR)

## ADR-001: spec-kit 패턴 전면 채택

**상태**: 승인
**일시**: 2026-03-24

### 맥락

Pylon v1의 Go 오케스트레이터는 `syscall.Exec`로 Claude Code TUI 프로세스를 교체하여
파이프라인이 TUI 내에서 실행되지 않는 구조적 한계가 있었다.

`--headless --background` 플래그로 우회 해결했으나 (feat/headless-background-request 브랜치),
이는 외부 프로세스 방식이라 TUI 통합이 간접적이었다.

### 결정

[github/spec-kit](https://github.com/github/spec-kit) 패턴을 전면 채택하여 아키텍처를 재작성한다.

- **LLM-as-Orchestrator**: Markdown slash command가 프로그램, Claude Code가 실행자
- **Shell script**: 원자적 작업(git, mkdir, JSON 출력)만 담당
- **파일 기반 상태**: 산출물 존재 = 스테이지 완료

### 근거

1. TUI 통합 문제의 근본 원인은 프로세스 경계 — spec-kit은 프로세스 경계가 없음
2. spec-kit이 81K+ 스타로 검증된 패턴
3. Claude Code의 Agent 도구로 병렬 실행이 가능해져 Go 스케줄러 불필요

### 포기하는 것

- Dashboard (웹 UI)
- DLQ (Dead Letter Queue)
- 정교한 crash recovery (SQLite + outbox 스캔 → 파일 기반 약한 복구)
- 프로세스 수준 격리 (Go 컨텍스트 기반 타임아웃 → LLM 자율 관리)

---

## ADR-002: 병렬 실행을 Claude Code Agent 도구로 대체

**상태**: 승인
**일시**: 2026-03-24

### 맥락

v1은 `errgroup` + TaskGraph DAG로 에이전트를 wave 기반 병렬 실행.
spec-kit에는 병렬 실행 기능이 없음.

### 결정

Claude Code의 내장 Agent 도구를 사용하여 병렬 서브에이전트를 실행한다.

```
// 단일 메시지에서 여러 Agent 호출 → 자동 병렬
Agent(prompt="T001 구현", isolation="worktree")
Agent(prompt="T002 구현", isolation="worktree")
Agent(prompt="T003 구현", isolation="worktree")
```

### 근거

1. Claude Code Agent 도구는 단일 메시지 내 복수 호출 시 자동 병렬 실행
2. `isolation: "worktree"` 옵션으로 git worktree 격리 지원
3. Go `errgroup` 수준의 정교함은 없지만, 대부분의 사용 사례에 충분

### 트레이드오프

| v1 Go errgroup | v2 Agent 도구 |
|----------------|--------------|
| DAG 기반 wave 실행 | 단순 병렬 (의존성 없는 태스크만) |
| WIP 한도, 모델별 용량 관리 | Claude Code에 위임 |
| 정교한 에러 복구 | Agent 도구의 자동 처리 |
| Go 컨텍스트 기반 타임아웃 | Claude Code 자체 관리 |

---

## ADR-003: SQLite를 메모리 검색 + 이력 조회 전용으로 축소

**상태**: 승인
**일시**: 2026-03-24

### 맥락

v1은 SQLite 6개 테이블로 전체 상태 관리.
v2에서는 파일 기반 상태가 primary.

### 결정

SQLite를 유지하되, 역할을 축소:

**유지**:
- `project_memory` + FTS5 → BM25 검색 (핵심)
- `pipeline_state` → 이력 조회 (보조)
- `conversations` → 대화 이력 (보조)

**삭제**:
- `message_queue` → inbox/outbox 불필요
- `blackboard` → project_memory로 통합
- `dlq` → 포기
- `topic_subscriptions` → 불필요

### 근거

BM25 전문 검색은 SQLite FTS5가 가장 효율적.
파일 기반 검색(grep)으로는 랭킹/가중치 검색 불가.
`pylon mem search` CLI가 이를 활용.

---

## ADR-004: pylon CLI를 최소화하고 나머지는 slash command로

**상태**: 승인
**일시**: 2026-03-24

### 맥락

v1의 `pylon` CLI는 20+ 명령어를 가진 Go 바이너리.
대부분은 Claude Code TUI에서 slash command로 실행 가능.

### 결정

`pylon` CLI는 워크스페이스 관리 명령만 유지:
- `init`, `doctor`, `version`, `uninstall`
- `mem search/store/list` (SQLite 접근)
- `status` (파이프라인 조회)
- `add-project`, `sync-*`

파이프라인 실행/제어는 slash command:
- `/pl:pipeline` (= `pylon request`)
- `/pl:cancel` (= `pylon cancel`)
- `/pl:status` (= `pylon status`의 TUI 버전)

### 근거

- TUI 내에서 실행해야 하는 기능은 slash command가 자연스러움
- SQLite 접근, 워크스페이스 초기화 등은 CLI가 적합
- 두 경로가 충돌하지 않음 (CLI = 관리, slash command = 실행)

---

## ADR-005: 빅뱅 마이그레이션

**상태**: 승인
**일시**: 2026-03-24

### 맥락

점진적 전환 vs 빅뱅 전환.

### 결정

빅뱅 (전면 재작성).

### 근거

1. **두 트랙 상태 문제**: 점진적 전환 시 Go FSM 상태와 파일 기반 상태가 공존,
   crash recovery에서 불일치 발생 가능 (Architect 검증에서 지적된 리스크)
2. **Go FSM이 전역적**: 한 스테이지만 slash command로 이전하면
   나머지 Go 루프와의 전이(transition) 호환성 유지 필요
3. **코드 규모**: 삭제 대상(~60%)이 유지 대상(~40%)보다 큼 — 새로 쓰는 게 빠름

### 리스크

- 기존 기능의 일시적 후퇴 (dashboard, DLQ 등)
- shell script + slash command의 안정성이 Go 코드보다 낮을 수 있음
- 테스트 커버리지 재구축 필요

---

## ADR-006: 에이전트 정의 형식 유지

**상태**: 승인
**일시**: 2026-03-24

### 맥락

`.pylon/agents/*.md` (YAML frontmatter + Markdown body).

### 결정

형식을 유지하되 불필요한 frontmatter 필드를 정리.

**유지할 필드**:
```yaml
name: backend-dev
role: Backend Developer
scope: [project-api]
tools: [git, gh]
model: sonnet  # optional
```

**제거할 필드**:
```yaml
backend: claude-code      # 항상 claude-code
maxTurns: 30              # Agent 도구가 관리
permissionMode: acceptEdits # Agent 도구가 관리
isolation: worktree       # Agent 호출 시 지정
timeout: 30m              # Agent 도구가 관리
capabilities:             # 사용되지 않음
  accepts: [...]
  produces: [...]
env:                      # 환경변수 주입 불필요
  KEY: value
```

### 근거

- v2에서 에이전트는 Claude Code의 Agent 도구로 실행
- Agent 도구가 프로세스 수명, 권한, 격리를 자체 관리
- 에이전트 정의는 역할/범위/프롬프트만 담당하면 충분

---

## ADR-007: 메타 워크플로우 커맨드 (/pl:pipeline)

**상태**: 승인
**일시**: 2026-03-24

### 맥락

개별 스테이지 command만 둘 것인지, 전체 흐름을 안내하는 메타 command를 둘 것인지.

### 결정

메타 slash command `/pl:pipeline`을 두되, 개별 스테이지 command도 제공.

- `/pl:pipeline` — 전체 흐름을 순차적으로 안내 (Primary)
- `/pl:architect`, `/pl:breakdown` 등 — 개별 스테이지 단독 실행 (Secondary)

### 근거

- spec-kit의 `/speckit.implement`가 전체 구현 흐름을 안내하는 것과 동일 패턴
- 개별 command는 디버깅, 재실행, 부분 실행에 유용
- YAML frontmatter의 `handoffs:`로 메타 → 개별 라우팅 선언
