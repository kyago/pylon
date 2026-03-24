# Pylon v2: spec-kit 패턴 전면 채택 — 아키텍처 재작성

## 1. 배경

### 현재 문제

Pylon v1은 Go 바이너리가 외부 오케스트레이터로 동작하며, Claude Code TUI와의 통합에 구조적 한계가 있다.

핵심 문제:
- `pylon` bare command가 `syscall.Exec("claude")`로 Go 프로세스를 교체하여 오케스트레이션 루프가 실행되지 않음
- `pylon request`는 interactive 스테이지(PO, task review)에서 `syscall.Exec`로 프로세스를 교체, `--continue`로 수동 재개 필요
- Go 오케스트레이터와 Claude Code TUI 간 프로세스 경계 문제가 근본적

### 해결 방향

[github/spec-kit](https://github.com/github/spec-kit) 패턴을 전면 채택:
- **LLM-as-Orchestrator**: Markdown 프롬프트 자체가 프로그램, Claude Code가 실행자
- **Shell script = 원자적 작업 레이어**: git/mkdir/JSON 출력만 담당
- **파일 기반 상태**: 산출물 존재 = 스테이지 완료
- **선언적 handoff 라우팅**: YAML frontmatter에 다음 단계 선언

## 2. v1 vs v2 아키텍처 비교

```
[v1 현재]
┌─ Go 오케스트레이터 (외부 프로세스) ─────────────┐
│  Loop.Run() → validTransitions FSM             │
│  → syscall.Exec("claude") (프로세스 교체)       │
│  → RunHeadless("claude --print") (별도 프로세스) │
│  → SQLite 상태 저장                             │
│  → inbox/outbox 파일 프로토콜                    │
│  ★ Claude Code는 실행만 하는 도구                │
└────────────────────────────────────────────────┘

[v2 목표]
┌─ Claude Code TUI (단일 프로세스) ──────────────┐
│  /pl:pipeline 메타 커맨드                       │
│  → Shell Script(git, mkdir, JSON)              │
│  → Claude Code Agent 도구 (병렬 서브에이전트)    │
│  → 파일 아티팩트 = 상태                         │
│  ★ Claude Code가 오케스트레이터이자 실행자       │
└────────────────────────────────────────────────┘
```

## 3. 의사결정 기록

| # | 질문 | 결정 | 근거 |
|---|------|------|------|
| Q1 | 병렬 실행 방식 | Claude Code native Agent 도구 | worktree 없이 병렬 서브에이전트 실행 가능 |
| Q2 | Dashboard | 포기 | TUI 기반에서 불필요 |
| Q2 | 메모리 | 파일 기반 + SQLite FTS5 BM25 유지 | 검색 성능 필요 |
| Q2 | Pipeline 상태 | SQLite 유지 | crash recovery, 상태 조회에 필요 |
| Q2 | DLQ | 포기 | 단순화 |
| Q3 | pylon CLI 역할 | 최소화 (init, doctor 등만) | 나머지는 slash command |
| Q4 | 워크플로우 정의 | 메타 slash command (/pl:pipeline) | 하나의 command가 전체 흐름 안내 |
| Q5 | 에이전트 정의 | .pylon/agents/*.md 유지 | 불필요 frontmatter 정리 |
| Q6 | 마이그레이션 | 빅뱅 (전면 재작성) | 점진적 전환의 두 트랙 상태 문제 회피 |

## 4. 문서 구조

```
docs/v2-rewrite/
├── OVERVIEW.md              ← 이 파일 (전체 개요)
├── ARCHITECTURE.md          ← 상세 아키텍처 설계
├── MIGRATION.md             ← 삭제/유지/신규 파일 목록
├── DECISIONS.md             ← 의사결정 상세 기록 (ADR)
└── CAPABILITY-INVENTORY.md  ← v1 기능 인벤토리 및 전환 매핑
```
