# Pylon v2 상세 아키텍처

## 1. 실행 모델

### 1.1 워크플로우 흐름

```
사용자 → /pl:pipeline "로그인 기능 구현"
           │
           ├─ Shell: init-pipeline.sh (git branch, 디렉토리 생성, JSON 출력)
           │    → .pylon/runtime/pipeline-{id}/requirement.md 생성
           │
           ├─ LLM: PO 분석 (Claude Code가 직접 수행)
           │    → requirement-analysis.md 생성
           │
           ├─ LLM: Architect 분석 (Agent 도구 또는 직접)
           │    → architecture.md 생성
           │
           ├─ Shell: check-prerequisites.sh (architecture.md 존재 검증)
           │
           ├─ LLM: PM 태스크 분해
           │    → tasks.json 생성
           │
           ├─ LLM: 태스크 실행 (Agent 도구로 병렬 서브에이전트)
           │    → 각 에이전트가 코드 작성
           │
           ├─ Shell: run-verification.sh (build, test, lint)
           │    → verification.json 출력
           │
           ├─ Shell: create-pr.sh (gh pr create)
           │    → pr.json 출력
           │
           └─ 완료 보고
```

### 1.2 프로세스 모델

```
┌─ Claude Code TUI (PID 1234) ─────────────────────────────┐
│                                                           │
│  /pl:pipeline 실행 중                                      │
│                                                           │
│  ┌─ Bash 도구 ──────────────┐  ┌─ Agent 도구 ───────────┐ │
│  │ init-pipeline.sh         │  │ subagent: backend-dev  │ │
│  │ check-prerequisites.sh   │  │ subagent: frontend-dev │ │
│  │ run-verification.sh      │  │ subagent: test-eng     │ │
│  │ create-pr.sh             │  │ (병렬 실행)             │ │
│  └──────────────────────────┘  └─────────────────────────┘ │
│                                                           │
│  ★ 모든 것이 단일 Claude Code 프로세스 안에서 실행          │
│  ★ 외부 Go 오케스트레이터 없음                              │
│  ★ syscall.Exec 없음                                      │
└───────────────────────────────────────────────────────────┘
```

## 2. Slash Command 설계

### 2.1 메타 커맨드: /pl:pipeline

전체 워크플로우를 순차적으로 안내하는 단일 진입점.

YAML frontmatter 구조:
```yaml
---
description: Pylon 파이프라인 실행 — 요구사항 → 구현 → PR
handoffs:
  - label: 아키텍처 분석만
    agent: pl.architect
  - label: 태스크 분해만
    agent: pl.breakdown
  - label: 검증만
    agent: pl.verify
---
```

Markdown 본문은 Claude Code에 대한 실행 지시서:
```
1. init-pipeline.sh 실행 → pipeline-id, branch 획득
2. 요구사항 분석 (PO 역할 수행)
3. 아키텍처 분석 (architect agent 또는 직접)
4. check-prerequisites.sh --require-architecture 실행
5. 태스크 분해 (PM 역할 수행)
6. 에이전트 실행 (Agent 도구로 병렬)
7. run-verification.sh 실행
8. create-pr.sh 실행
9. 결과 보고
```

### 2.2 개별 스테이지 커맨드

| 커맨드 | 역할 | 산출물 |
|--------|------|--------|
| `/pl:pipeline` | 전체 흐름 메타 커맨드 | 모든 산출물 |
| `/pl:architect` | 아키텍처 분석 단독 실행 | `architecture.md` |
| `/pl:breakdown` | PM 태스크 분해 단독 실행 | `tasks.json` |
| `/pl:execute` | 에이전트 실행 단독 실행 | 코드 변경 |
| `/pl:verify` | 검증 단독 실행 | `verification.json` |
| `/pl:pr` | PR 생성 단독 실행 | `pr.json` |
| `/pl:status` | 파이프라인 상태 조회 | stdout |
| `/pl:index` | 코드베이스 인덱싱 | 메모리 갱신 |
| `/pl:cancel` | 파이프라인 취소 | 상태 갱신 |

## 3. Shell Script 설계

### 3.1 스크립트 목록

```
.pylon/scripts/bash/
├── common.sh               ← 공통 유틸 (repo root, feature detection)
├── init-pipeline.sh         ← git branch + 디렉토리 초기화
├── check-prerequisites.sh   ← 스테이지 진입 전 사전조건 검증
├── run-verification.sh      ← build/test/lint 실행
├── create-pr.sh             ← gh pr create 래퍼
├── merge-branches.sh        ← 에이전트 브랜치 머지
└── cleanup-pipeline.sh      ← worktree/branch 정리
```

### 3.2 스크립트 설계 원칙

1. **JSON 출력**: 모든 스크립트는 `--json` 플래그로 JSON 출력 지원
2. **원자적 작업**: git, mkdir, mv 등 단일 작업만 수행
3. **판단 없음**: 분기/결정은 LLM이 담당, 스크립트는 실행만
4. **에러 처리**: 실패 시 stderr에 에러, exit code 1

예시 — `init-pipeline.sh`:
```bash
#!/bin/bash
set -euo pipefail
source "$(dirname "$0")/common.sh"

REQUIREMENT="$1"
SLUG=$(echo "$REQUIREMENT" | tr '[:upper:]' '[:lower:]' | sed 's/[^a-z0-9가-힣]/-/g' | head -c 30)
PIPELINE_ID="$(date +%Y%m%d)-${SLUG}"
BRANCH="task-${SLUG}"

git checkout -b "$BRANCH" 2>/dev/null || git checkout "$BRANCH"

PIPELINE_DIR=".pylon/runtime/${PIPELINE_ID}"
mkdir -p "$PIPELINE_DIR"
echo "$REQUIREMENT" > "$PIPELINE_DIR/requirement.md"

# JSON 출력
jq -cn \
  --arg id "$PIPELINE_ID" \
  --arg branch "$BRANCH" \
  --arg dir "$PIPELINE_DIR" \
  '{pipeline_id: $id, branch: $branch, pipeline_dir: $dir}'
```

## 4. 상태 관리

### 4.1 파일 기반 상태 (Primary)

각 스테이지의 산출물 존재 = 해당 스테이지 완료:

```
.pylon/runtime/{pipeline-id}/
├── requirement.md          → init 완료
├── requirement-analysis.md → PO 분석 완료
├── architecture.md         → architect 분석 완료
├── tasks.json              → PM 태스크 분해 완료
├── execution-log.json      → 에이전트 실행 완료
├── verification.json       → 검증 완료
├── pr.json                 → PR 생성 완료
└── status.json             → 현재 상태 메타데이터
```

`check-prerequisites.sh`가 이 파일들의 존재를 검증:
```bash
check-prerequisites.sh --require-architecture --require-tasks
# → architecture.md, tasks.json 존재 확인 후 JSON 출력
```

### 4.2 SQLite (Secondary — 검색/조회용)

SQLite는 메모리 검색(BM25)과 파이프라인 이력 조회에만 사용:

유지할 테이블:
- `project_memory` + `project_memory_fts` — BM25 검색
- `pipeline_state` — 파이프라인 이력 (조회용)
- `conversations` — 대화 이력

삭제할 테이블:
- `message_queue` — inbox/outbox 불필요
- `blackboard` — project_memory로 통합
- `dlq` — 포기
- `topic_subscriptions` — 불필요
- `session_archive` — 선택적

### 4.3 `pylon` CLI의 SQLite 접근

`pylon mem search`, `pylon status` 등 CLI 명령어가 SQLite를 조회.
slash command에서는 `pylon mem search` CLI를 Bash 도구로 호출하여 접근.

## 5. 에이전트 실행 모델

### 5.1 Claude Code Agent 도구 활용

v1의 `RunHeadless("claude --print")` 대신, Claude Code의 내장 Agent 도구 사용:

```
// /pl:pipeline 내부에서
Agent(subagent_type="general-purpose",
      prompt="backend-dev 에이전트로서 다음 태스크를 구현하세요: ...",
      isolation="worktree")
```

### 5.2 병렬 실행

독립 태스크는 단일 메시지에서 여러 Agent 호출로 병렬 실행:

```
// 동시에 3개 Agent 실행
Agent(prompt="backend 태스크 T001 구현", isolation="worktree")
Agent(prompt="frontend 태스크 T002 구현", isolation="worktree")
Agent(prompt="test 태스크 T003 구현", isolation="worktree")
```

### 5.3 에이전트 정의 활용

`.pylon/agents/*.md`의 frontmatter에서 역할/scope/tools를 읽어
Agent 프롬프트에 주입:

```markdown
---
name: backend-dev
role: Backend Developer
scope: [project-api]
tools: [git, gh]
---
# Backend Developer
## 역할
...
```

→ Agent 프롬프트에 포함:
```
당신은 backend-dev 에이전트입니다.
역할: Backend Developer
범위: project-api
사용 가능 도구: git, gh
[에이전트 정의 본문]

## 태스크
...
```

## 6. 메모리 시스템

### 6.1 파일 기반 메모리

각 프로젝트의 도메인 지식을 파일로 관리:
```
.pylon/domain/
├── architecture.md    ← 아키텍처 개요
├── conventions.md     ← 코딩 컨벤션
├── glossary.md        ← 비즈니스 용어
└── patterns.md        ← 코드 패턴
```

### 6.2 BM25 검색

`pylon mem search --project <name> <query>` CLI로 SQLite FTS5 검색.
slash command에서 Bash 도구로 호출:
```bash
pylon mem search --project project-api "인증 처리"
```

### 6.3 프로액티브 주입

`/pl:pipeline` 실행 시 요구사항 키워드로 메모리 검색 후
에이전트 프롬프트에 관련 지식 주입 (v1의 `GetProactiveContext()` 역할).

## 7. Git 워크플로우

### 7.1 브랜치 전략

```
main
  └── task-{slug}                    ← 파이프라인 브랜치
        ├── task-{slug}/backend-dev  ← 에이전트 worktree 브랜치
        ├── task-{slug}/frontend-dev
        └── task-{slug}/test-eng
```

### 7.2 Shell Script로 관리

- `init-pipeline.sh` — task branch 생성
- `merge-branches.sh` — 에이전트 브랜치 → task branch 머지
- `create-pr.sh` — task branch → main PR 생성
- `cleanup-pipeline.sh` — worktree/branch 정리

Agent 도구의 `isolation: "worktree"` 옵션으로 자동 worktree 생성.
