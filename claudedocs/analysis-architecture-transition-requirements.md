# Pylon 아키텍처 전환: 미결 요구사항 분석

> 분석일: 2026-03-08
> 기준 문서: `pylon-spec.md` (Draft v0.8), `IMPLEMENTATION_PLAN.md`
> 전환 방향: Go 오케스트레이터 + tmux 세션 --> Go 런처 + Claude Code TUI 대화형

---

## 전환 개요

### 현재 구현 상태

소스코드 분석 결과, 현재 구현은 Phase 0~4의 핵심 요소를 포함하고 있다:

| 구현 영역 | 파일 | 상태 |
|-----------|------|------|
| CLI 프레임워크 (Cobra, 11개 명령어) | `internal/cli/*.go` | 구현됨 |
| config.yml / agent .md 파싱 | `internal/config/*.go` | 구현 + 테스트 |
| tmux 세션 관리 (인터페이스 추상화) | `internal/tmux/*.go` | 구현 + 테스트 |
| SQLite 저장소 (7개 테이블, FTS5) | `internal/store/*.go` | 구현 + 테스트 |
| MessageEnvelope 통신 프로토콜 | `internal/protocol/*.go` | 구현 + 테스트 |
| 파이프라인 상태 머신 (10단계) | `internal/orchestrator/pipeline.go` | 구현 + 테스트 |
| 오케스트레이터 (상태 저장, SPOF 복구) | `internal/orchestrator/orchestrator.go` | 구현 + 테스트 |
| 교차 검증 (verify.yml 실행) | `internal/orchestrator/verify.go` | 구현 + 테스트 |
| 대화 기록 관리 | `internal/orchestrator/conversation.go` | 구현 + 테스트 |
| Claude CLI 래퍼 + CLAUDE.md 빌더 | `internal/agent/*.go` | 구현 + 테스트 |
| 메모리 매니저 (BM25 검색, 학습 축적) | `internal/memory/manager.go` | 구현 + 테스트 |
| Git worktree / branch / PR | `internal/git/*.go` | 구현 |
| Narrative Casting 핸드오프 | `internal/protocol/handoff.go` | 구현 + 테스트 |

### 전환 후 보존/변경 매핑

| 현재 구성요소 | 전환 후 | 비고 |
|-------------|---------|------|
| `pylon request "..."` | `pylon` (인자 없이) → TUI 대화 | 요구사항을 대화로 전달 |
| Go 오케스트레이터 상태 머신 | CLAUDE.md 규칙 + 파일 기반 체크포인트 | AI가 상태 관리 |
| tmux 세션 (에이전트별) | Claude Code Agent/Team 시스템 | 네이티브 서브에이전트 |
| fsnotify outbox 감시 | SendMessage 기반 통신 | Claude Code 네이티브 |
| SQLite (pylon.db) | CLI 헬퍼 도구 (`pylon mem`) 경유 접근 | 에이전트 직접 접근 불가 |
| inbox/outbox 파일 프로토콜 | Team 통신 + 구조화된 결과 파일 | 하이브리드 |
| Go 바이너리 CLI 명령어들 | `.claude/commands/` 슬래시 커맨드 | 일부 Go CLI 유지 |

---

## 미결 요구사항 1: 상태 관리

### 문제의 본질

현재 파이프라인 상태 머신 (`internal/orchestrator/pipeline.go`)은 Go 코드로 10단계 전이를 엄격하게 관리한다. `validTransitions` 맵이 유효한 전이만 허용하고, `Transition()` 메서드가 무효 전이를 거부한다. 상태는 SQLite `pipeline_state` 테이블과 `state.json` 파일에 이중 저장된다.

Go 오케스트레이터가 사라지면, 이 엄격한 상태 관리의 주체가 없어진다.

### 해결 옵션

**옵션 A: 루트 에이전트가 파일 기반으로 상태 추적**

루트 에이전트(PO)가 `.pylon/runtime/state.json`을 직접 읽고 쓰며 단계 전이를 관리한다.

- 장점: Go 코드 불필요, 단순
- 단점: AI의 상태 관리 신뢰성 보장 불가, 유효 전이 검증 없음
- 위험: 에이전트가 단계를 건너뛰거나 잘못된 전이를 할 수 있음

**옵션 B: Go 런처가 백그라운드 상태 감시자 유지**

Go 바이너리가 완전히 퇴장하지 않고, 상태 파일을 감시하는 경량 프로세스로 남는다.

- 장점: 기존 검증 로직 재활용 가능
- 단점: 축소 목적에 반함, 여전히 Go 프로세스 의존

**옵션 C: Claude Code TaskCreate/TaskUpdate를 파이프라인 상태로 활용**

Claude Code의 네이티브 태스크 시스템을 파이프라인 단계로 사용한다. 각 단계를 태스크로 생성하고, `addBlockedBy`로 의존성을 설정한다.

- 장점: Claude Code TUI에서 시각적으로 진행 상황 확인 가능
- 단점: 태스크 시스템은 상태 머신이 아님, 유효 전이 강제 불가

**옵션 D (권장): 하이브리드 -- Go 런처의 상태 검증 CLI + CLAUDE.md 규칙 + 파일 체크포인트**

1. CLAUDE.md에 파이프라인 단계 전이 규칙을 명시적으로 포함
2. 루트 에이전트가 단계 전환 시 `pylon stage transition <from> <to>` CLI를 호출
3. Go CLI가 `validTransitions` 맵으로 전이를 검증하고, `state.json`에 기록
4. Claude Code TaskCreate로 시각적 진행 추적 병행

```
루트 에이전트 → `pylon stage transition po_conversation architect_analysis`
                     ↓
              Go CLI가 유효 전이 검증 → state.json 갱신 → 성공/실패 반환
                     ↓
              루트 에이전트가 다음 단계 진행
```

- 장점: 유효 전이의 프로그래밍적 보장, 기존 코드 재활용, AI 부담 최소화
- 단점: Go CLI에 `stage` 서브커맨드 추가 필요

### 권장 방향

**옵션 D**. Go 런처가 "상태 전이 검증기" 역할을 겸한다. 이는 기존 `internal/orchestrator/pipeline.go`의 `validTransitions` 로직을 CLI 서브커맨드로 노출하는 것에 불과하므로 구현 비용이 낮다.

### 우선순위: P0 (필수)

---

## 미결 요구사항 2: 세션 지속성

### 문제의 본질

현재 아키텍처에서 tmux 세션은 Go 오케스트레이터와 독립적이다. 오케스트레이터가 죽어도 에이전트의 tmux 세션은 살아있다 (`pylon-spec.md` Section 8 "SPOF 복구" 참조). 이것이 현재 설계의 핵심 안전장치다.

새 아키텍처에서 Claude Code TUI 세션이 종료되면:
- 루트 에이전트(PO) 사라짐
- 서브에이전트들도 함께 종료됨 (Claude Code Agent 도구로 생성된 에이전트는 부모 세션에 종속)
- 진행 중인 파이프라인의 컨텍스트 유실

### 해결 옵션

**옵션 A: tmux 안에서 Claude Code 실행**

```
pylon 실행 → tmux new-session -s pylon-main → claude (TUI)
```

터미널이 닫혀도 tmux 세션(과 그 안의 Claude Code)은 생존한다. `pylon resume`은 `tmux attach -t pylon-main`으로 재연결한다.

- 장점: tmux의 세션 지속성 그대로 활용, 현재 스펙의 철학과 일치
- 단점: 사용자가 tmux를 알아야 할 수 있음 (런처가 추상화하면 해결)

**옵션 B: 체크포인트 기반 콜드 재시작**

루트 에이전트가 주기적으로 상태를 파일에 저장하고, 재시작 시 체크포인트에서 복원한다.

- 장점: tmux 의존성 제거
- 단점: 체크포인트와 크래시 사이의 작업 유실, 서브에이전트의 진행 상태 복구 불가

**옵션 C: Claude Code `--resume` 활용**

Claude Code CLI에는 `--resume` 플래그가 있어 이전 대화를 재개할 수 있다. 대화 ID를 저장해두면 재시작이 가능하다.

- 장점: Claude Code 네이티브 기능
- 단점: 서브에이전트 상태는 복구 불가, resume 시 컨텍스트가 압축될 수 있음

**옵션 D (권장): tmux 래퍼 + 체크포인트 + Claude Code resume 다중 방어**

```
pylon (인자 없이)
  ├── [1] tmux 세션 pylon-main 존재 확인
  │     ├── 존재 → tmux attach -t pylon-main (재연결)
  │     └── 미존재 → 아래 진행
  ├── [2] state.json 확인
  │     ├── 진행 중인 파이프라인 존재 → 복구 모드
  │     │     ├── Claude Code session ID 있으면 → claude --resume {id}
  │     │     └── 없으면 → 새 세션 + 상태 복원 컨텍스트 주입
  │     └── 없음 → 새 세션
  └── [3] tmux new-session -s pylon-main → claude
```

- 장점: 3중 방어 (tmux 재연결 > Claude resume > 상태 파일 복원)
- 단점: 복구 로직의 복잡도

### 권장 방향

**옵션 D**. 핵심은 **Claude Code TUI를 반드시 tmux 안에서 실행**하는 것이다. 이는 현재 스펙의 "tmux 세션은 오케스트레이터와 독립적으로 생존" 원칙을 그대로 승계한다. Go 런처의 기본 동작:

1. `pylon` → tmux 세션 확인 → 없으면 생성 + claude 실행
2. `pylon` → tmux 세션 존재 → 재연결
3. Claude Code 세션 ID를 `state.json`에 기록하여 `--resume` 활용 가능하게 함

Go 런처에 필요한 구현:
- tmux 세션 존재 확인 및 생성 (기존 `internal/tmux/` 코드 재활용)
- Claude Code 세션 ID 기록/복원
- 복구 모드 진입 로직

### 우선순위: P0 (필수)

---

## 미결 요구사항 3: 서브 에이전트 통신

### 문제의 본질

현재 스펙은 inbox/outbox 파일 기반 통신 + fsnotify 감시 + SQLite 기록이라는 정교한 하이브리드 프로토콜을 정의한다 (`pylon-spec.md` Section 8). 이 프로토콜의 핵심 가치:

1. **구조화된 메시지**: `MessageEnvelope`의 6가지 타입 (task_assign, result, query, query_result, broadcast, heartbeat)
2. **원자적 쓰기**: tmp→mv 패턴으로 메시지 손실 방지
3. **이력 관리**: SQLite `message_queue` 테이블에 전체 히스토리
4. **토픽 구독**: Pub/Sub 패턴으로 관심사 분리
5. **Narrative Casting**: 핸드오프 시 서사적 컨텍스트 재구성

Claude Code의 네이티브 Team 시스템은 SendMessage, TaskCreate/Update, broadcast 등을 지원하지만, 위 기능 전부를 대체하지는 못한다.

### 해결 옵션

**옵션 A: Claude Code Team 시스템 전면 채택 (inbox/outbox 포기)**

- 장점: 단순, Anthropic이 유지보수
- 단점: 구조화된 메시지 타입 없음, SQLite 히스토리 없음, BM25 메모리 연동 없음, Narrative Casting 불가

**옵션 B: inbox/outbox 프로토콜 유지 (루트 에이전트가 감시)**

- 장점: 기존 스펙 완전 보존
- 단점: 루트 에이전트가 fsnotify를 할 수 없음, 폴링 방식 필요 (비효율)

**옵션 C (권장): 하이브리드 -- 제어 흐름은 Claude Code Team, 구조화된 데이터는 파일**

```
[제어 흐름 (Claude Code Team 시스템)]
  루트 에이전트 → SendMessage("pm", "태스크 분해 완료, 에이전트 실행 시작") → PM 서브에이전트
  PM 서브에이전트 → SendMessage("root", "backend-dev 완료, 검증 필요") → 루트 에이전트

[구조화된 데이터 (파일)]
  PM → .pylon/runtime/results/20260305-user-login/backend-dev.result.json
  루트 에이전트가 결과 파일 읽기 → 학습 추출 → pylon mem store 호출
```

- 장점: 각 채널의 강점 활용 -- 제어는 실시간, 데이터는 구조화/영속화
- 단점: 두 채널 관리 필요

**구현 세부**:
- 서브에이전트의 CLAUDE.md (`.claude/agents/*.md`)에 결과 파일 작성 규칙 포함
- 결과 파일은 기존 `ResultBody` 포맷의 간소화 버전 사용
- `learnings` 필드는 유지하여 프로젝트 메모리 축적 지원
- 토픽 구독과 브로드캐스트는 Claude Code의 broadcast 메시지로 대체
- Narrative Casting은 루트 에이전트가 서브에이전트 생성 시 컨텍스트 요약을 프롬프트에 포함하는 방식으로 구현

### 권장 방향

**옵션 C**. 핵심 원칙은 "제어는 네이티브, 데이터는 파일"이다. 이를 통해:
- Claude Code의 강점 (실시간 통신, TUI 가시성, 에이전트 생명주기 관리) 활용
- Pylon의 차별화 가치 (구조화된 결과, 학습 축적, 프로젝트 메모리) 보존

### 우선순위: P0 (필수)

---

## 미결 요구사항 4: CLAUDE.md 동적 생성

### 문제의 본질

현재 `ClaudeMDBuilder` (`internal/agent/claudemd.go`)는 200줄 제한으로 개별 에이전트용 CLAUDE.md를 생성한다. 새 아키텍처에서는 다른 역할을 해야 한다:

- **현재**: 에이전트별 통신 규칙 + 태스크 컨텍스트 주입 (200줄)
- **새 역할**: 루트 에이전트의 전체 오케스트레이션 플레이북 (프로젝트 수준 CLAUDE.md)

Claude Code TUI에서 CLAUDE.md는 프로젝트 루트에 위치하며, 세션 시작 시 자동 로드된다. 크기 제한은 200줄이 아니라 합리적 범위 내에서 유연하다.

### Go 런처가 생성해야 하는 파일 구조

```
{workspace}/
├── .claude/
│   ├── CLAUDE.md                    ← [생성] 루트 에이전트 플레이북
│   ├── agents/                      ← [생성] Claude Code 네이티브 에이전트 정의
│   │   ├── pm.md                    ← .pylon/agents/pm.md 에서 변환
│   │   ├── architect.md             ← .pylon/agents/architect.md 에서 변환
│   │   ├── tech-writer.md           ← .pylon/agents/tech-writer.md 에서 변환
│   │   ├── backend-dev.md           ← .pylon/agents/backend-dev.md 에서 변환
│   │   └── ...
│   └── commands/                    ← [생성] 슬래시 커맨드
│       ├── status.md
│       ├── cancel.md
│       ├── verify.md
│       └── ...
└── .pylon/                          ← [기존] Pylon 설정 (Source of Truth)
    ├── config.yml
    ├── agents/
    ├── domain/
    └── ...
```

### CLAUDE.md에 포함되어야 할 내용

| 섹션 | 내용 | 예상 줄 수 |
|------|------|-----------|
| **아이덴티티** | "당신은 Pylon PO 에이전트입니다" + 역할 정의 | ~20줄 |
| **파이프라인 프로토콜** | 10단계 전이 규칙, 각 단계의 행동 지침 | ~60줄 |
| **에이전트 스폰 규칙** | 서브에이전트 생성 방법, maxTurns, 모델 지정 | ~30줄 |
| **결과 파일 작성 규칙** | 서브에이전트에게 전달할 결과 파일 포맷 | ~20줄 |
| **메모리 접근 규칙** | `pylon mem` CLI 사용법 | ~15줄 |
| **안전 규칙** | max_concurrent, max_attempts, 에스컬레이션 기준 | ~20줄 |
| **워크스페이스 컨텍스트** | 프로젝트 목록, 기술 스택, 도메인 지식 경로 | ~30줄 |
| **현재 상태** | 진행 중인 파이프라인이 있으면 복구 컨텍스트 | ~20줄 |
| 합계 | | ~215줄 |

### .pylon/agents/*.md → .claude/agents/*.md 변환 규칙

Pylon 에이전트 포맷 (frontmatter + markdown body)을 Claude Code 에이전트 포맷으로 변환한다:

```markdown
# Pylon 포맷 (.pylon/agents/backend-dev.md)
---
name: backend-dev
role: Backend Developer
backend: claude-code
scope: [project-api]
tools: [git, gh, docker]
maxTurns: 30
permissionMode: acceptEdits
isolation: worktree
model: sonnet
---
# Backend Developer
[역할 설명 markdown]

# Claude Code 포맷 (.claude/agents/backend-dev.md)
---
name: backend-dev
description: project-api의 백엔드 기능을 구현하는 개발자
tools: [Read, Write, Edit, Bash, Grep, Glob]
model: sonnet
maxTurns: 30
---
# Backend Developer
[역할 설명 markdown]
## Pylon 규칙
- 작업 완료 후 결과를 .pylon/runtime/results/{task-id}/backend-dev.result.json에 작성
- verify.yml의 검증 명령어를 실행하여 자체 검증
- learnings 필드에 발견한 교훈 기록
```

### 권장 방향

Go 런처의 핵심 기능:
1. `.pylon/config.yml` 읽기
2. `.pylon/agents/*.md` 읽기
3. `.pylon/domain/` 파일 목록 확인
4. `.pylon/runtime/state.json` 확인 (복구 컨텍스트)
5. 위 정보를 조합하여 `.claude/CLAUDE.md`, `.claude/agents/*.md`, `.claude/commands/*.md` 생성
6. `exec("claude")` (또는 tmux 안에서)

`.claude/` 디렉토리는 `.gitignore` 대상이며, 매 실행 시 재생성된다. Source of Truth는 항상 `.pylon/`이다.

### 우선순위: P0 (필수)

---

## 미결 요구사항 5: 슬래시 커맨드(스킬) 설계

### 문제의 본질

현재 11개 CLI 명령어 중 일부는 Go CLI로 유지하고, 일부는 `.claude/commands/` 슬래시 커맨드로 전환해야 한다.

### 명령어별 전환 결정

| 현재 명령어 | 전환 방향 | 근거 |
|------------|----------|------|
| `pylon` (인자 없이) | Go 런처 | 워크스페이스 탐색 → .claude/ 생성 → claude 실행 |
| `pylon init` | Go CLI 유지 | 대화형 입력, .pylon/ 초기 구조 생성 |
| `pylon doctor` | Go CLI 유지 | 사전 검증, claude 실행 전에 필요 |
| `pylon request` | 제거 (TUI 대화로 대체) | 루트 에이전트와 직접 대화 |
| `pylon status` | `/status` 슬래시 커맨드 | TUI 내에서 호출 |
| `pylon cancel` | `/cancel` 슬래시 커맨드 | TUI 내에서 호출 |
| `pylon resume` | Go 런처 통합 | tmux 재연결 또는 --resume 로직 |
| `pylon review` | `/review` 슬래시 커맨드 | TUI 내에서 PR URL 전달 |
| `pylon cleanup` | `/cleanup` 슬래시 커맨드 | TUI 내에서 호출 |
| `pylon destroy` | Go CLI 유지 | .pylon/ 삭제는 claude 외부에서 |
| `pylon add-project` | `/add-project` 슬래시 커맨드 | AI 기반 코드 분석 포함 |
| `pylon dashboard` | 별도 결정 필요 | 웹 대시보드는 별도 프로세스 |

### 슬래시 커맨드 포맷 설계

Claude Code의 커맨드 파일 형식을 따른다:

```markdown
# .claude/commands/status.md

현재 Pylon 파이프라인의 진행 상황을 보여줍니다.

## 실행 단계

1. `.pylon/runtime/state.json` 파일을 읽습니다
2. 파이프라인 ID, 현재 단계, 단계 히스토리를 표시합니다
3. 활성 에이전트가 있으면 해당 상태도 표시합니다
4. 결과 파일들 (.pylon/runtime/results/) 을 스캔하여 완료된 태스크를 표시합니다
```

```markdown
# .claude/commands/verify.md

프로젝트의 교차 검증을 실행합니다.

$PROJECT_NAME

## 실행 단계

1. `$PROJECT_NAME/.pylon/verify.yml` 파일을 읽습니다
2. 정의된 각 검증 명령어를 순서대로 실행합니다
3. 모든 검증 결과를 요약하여 보고합니다
4. 실패한 검증이 있으면 상세 출력을 포함합니다
```

```markdown
# .claude/commands/cancel.md

진행 중인 파이프라인을 취소합니다.

$PIPELINE_ID

## 실행 단계

1. `.pylon/runtime/state.json`에서 $PIPELINE_ID 파이프라인을 찾습니다
2. 활성 서브에이전트가 있으면 종료를 요청합니다
3. `pylon stage transition {current} failed`를 실행하여 파이프라인을 failed 상태로 전환합니다
4. Git worktree가 있으면 정리합니다
```

### 새로 추가할 슬래시 커맨드

| 커맨드 | 용도 |
|--------|------|
| `/mem-search <query>` | 프로젝트 메모리 BM25 검색 |
| `/mem-store <content>` | 프로젝트 메모리에 지식 저장 |
| `/verify <project>` | 프로젝트별 교차 검증 실행 |
| `/wiki-update` | 도메인 지식 수동 갱신 트리거 |

### 우선순위: P1 (중요, P0 이후 순차 구현)

---

## 미결 요구사항 6: 메모리 접근

### 문제의 본질

현재 프로젝트 메모리는 SQLite `project_memory` 테이블 + FTS5 `project_memory_fts` 인덱스에 저장된다 (`internal/store/project_memory.go`). BM25 풀텍스트 검색을 지원하며, 5개 카테고리(architecture, pattern, decision, learning, codebase)로 분류된다.

AI 에이전트는 SQLite에 직접 접근할 수 없다. 현재 스펙에서는 오케스트레이터가 중재했지만, 오케스트레이터가 없어지면 다른 접근 방식이 필요하다.

### 해결 옵션

**옵션 A: 파일 기반 메모리로 전환**

SQLite를 포기하고, `.pylon/runtime/memory/` 디렉토리에 카테고리별 마크다운 파일로 저장한다.

- 장점: 에이전트가 직접 읽기/쓰기 가능, 단순
- 단점: BM25 검색 불가, 구조화된 쿼리 불가, 접근 횟수 추적 불가

**옵션 B (권장): Go CLI 헬퍼 도구 (`pylon mem`)**

기존 SQLite + BM25 인프라를 CLI 서브커맨드로 노출한다.

```bash
# 검색
pylon mem search "인증 아키텍처 결정" --project project-api --limit 5

# 저장
pylon mem store --project project-api --category learning --key "jwt-nullable" \
  "sqlc에서 nullable 필드 처리 시 sql.NullString 필요"

# 카테고리별 조회
pylon mem list --project project-api --category architecture

# 전체 요약
pylon mem summary --project project-api
```

- 장점: 기존 `internal/store/project_memory.go`와 `internal/memory/manager.go` 코드 완전 재활용, BM25 검색 유지
- 단점: 에이전트가 bash로 CLI를 호출해야 함

**옵션 C: Pylon MCP 서버**

SQLite를 래핑하는 MCP 서버를 제공하여 Claude Code에서 도구로 사용한다.

- 장점: Claude Code 네이티브 통합, 깔끔한 도구 인터페이스
- 단점: MCP 서버 구현 복잡도, 별도 프로세스 관리

### 권장 방향

**MVP: 옵션 B** (`pylon mem` CLI 서브커맨드). 기존 코드를 최대한 재활용하면서 에이전트에게 메모리 접근을 제공한다. CLAUDE.md에 사용법을 명시한다.

**v2: 옵션 C** (MCP 서버)로 고도화. `pylon mem-server` 명령어로 MCP 서버를 시작하고, `.claude/settings.json`에 MCP 서버 설정을 포함시킨다.

### 필요한 Go 구현

`pylon mem` 서브커맨드를 `internal/cli/mem.go`에 추가:
- `search`, `store`, `list`, `summary` 서브커맨드
- 기존 `internal/store/` 및 `internal/memory/` 패키지 재활용
- JSON 또는 텍스트 출력 지원 (`--json` 플래그)

### 선제적 메모리 주입

Go 런처가 CLAUDE.md 생성 시, `pylon mem summary`의 결과를 "프로젝트 메모리 요약" 섹션으로 포함시킨다. 이는 현재 `ClaudeMDBuilder`의 Priority 4 ("프로젝트 메모리 요약")와 동일한 역할이다.

### 우선순위: P1 (중요, 파일 기반 워크어라운드로 MVP 가능)

---

## 미결 요구사항 7: 교차 검증

### 문제의 본질

현재 스펙에서 교차 검증은 Go 오케스트레이터가 수행한다 (`internal/orchestrator/verify.go`). 에이전트 작업 완료 → 오케스트레이터가 `verify.yml` 명령어 실행 → 실패 시 에이전트에게 수정 요청.

Go 오케스트레이터가 없으면, 검증의 트리거와 실행 주체가 불명확해진다.

### 해결 옵션

**옵션 A: 개발 에이전트가 자체 검증**

개발 에이전트의 CLAUDE.md에 검증 규칙을 포함시킨다.

```markdown
## 작업 완료 전 필수 단계
1. .pylon/verify.yml 파일을 읽습니다
2. 정의된 모든 명령어를 실행합니다
3. 실패하면 수정하고 재실행합니다
4. 모든 검증 통과 후에만 result.json에 "completed" 작성
```

- 장점: 단순, 즉시 피드백
- 단점: 자체 코드를 자체 검증 (독립성 부족)

**옵션 B (권장): 이중 검증 -- 자체 검증 + PM 독립 검증**

1. **자체 검증**: 개발 에이전트가 코드 작성 후 verify.yml 실행 (1차)
2. **독립 검증**: PM 서브에이전트가 결과 수신 후 독립적으로 verify.yml 재실행 (2차)
3. 실패 시 PM이 개발 에이전트에게 수정 요청 (max_attempts까지)

```
개발 에이전트: verify.yml 실행 (자체 검증) → 통과 → 결과 보고
                                                       ↓
PM 에이전트: 결과 수신 → verify.yml 독립 실행 → 통과 → 다음 단계
                                               → 실패 → 수정 요청
```

- 장점: 이중 방어, 독립적 검증
- 단점: 동일 검증을 2회 실행 (비용)

**옵션 C: 슬래시 커맨드로 수동 검증**

`/verify <project>` 커맨드를 제공하여 루트 에이전트나 사용자가 수동으로 호출한다.

- 장점: 유연성
- 단점: 자동화되지 않음

### 권장 방향

**옵션 B (이중 검증)**를 기본으로 하되, 비용 최적화를 위해 1차 자체 검증을 필수로, 2차 PM 검증을 설정으로 제어한다 (`config.yml`의 `verify.independent: true/false`).

기존 `RunVerification` 함수는 에이전트가 bash에서 직접 호출할 수 있으므로, 별도의 Go CLI 래핑(`pylon verify <project>`)을 추가하면 된다.

### 우선순위: P1 (중요, 자체 검증만으로 MVP 가능)

---

## 미결 요구사항 8: 도메인 지식 (위키) 갱신

### 문제의 본질

현재 스펙에서 위키 갱신은:
- 담당: Tech Writer 에이전트
- 트리거: `config.yml`의 `wiki.update_on` (task_complete, pr_merged)
- 대상: `.pylon/domain/` 파일들 및 프로젝트별 `context.md`

Go 오케스트레이터가 없으면, 트리거 메커니즘이 필요하다.

### 권장 방향

파이프라인 프로토콜에서 `wiki_update` 단계를 명시적으로 유지한다. CLAUDE.md의 파이프라인 규칙에:

```markdown
## 파이프라인 단계 9: wiki_update
PO 검증(po_validation) 완료 후:
1. Tech Writer 에이전트를 스폰합니다
2. Tech Writer에게 전달할 정보:
   - git diff (태스크 브랜치 vs main)
   - 변경된 파일 목록
   - 태스크의 수용 기준
   - 에이전트들의 learnings
3. Tech Writer가 다음을 갱신합니다:
   - .pylon/domain/architecture.md (아키텍처 변경 시)
   - .pylon/domain/conventions.md (새 패턴 도입 시)
   - {project}/.pylon/context.md (프로젝트 구조 변경 시)
4. 갱신 완료 후 pylon stage transition wiki_update completed
```

### Self-Evolution 패턴 적용

수집된 스킬 연구 (`claudedocs/collected-skills-and-agents.md`)의 `presentation-curator` 에이전트에서 발견한 "Self-Evolution" 패턴을 Tech Writer에 적용한다:

- Tech Writer 에이전트의 `.claude/agents/tech-writer.md`에 자기진화 규칙 포함
- 실행 후 자신의 Learnings 섹션을 업데이트
- 도메인 지식과 에이전트 정의 간의 일관성 유지

### 우선순위: P2 (개선, 핵심 개발 흐름에는 영향 없음)

---

## 미결 요구사항 9: 멀티 프로젝트 조율

### 문제의 본질

Pylon의 핵심 차별화 중 하나는 워크스페이스 내 여러 프로젝트(git submodule)에 걸친 작업 조율이다. 예: "로그인 기능"이 project-api (백엔드)와 project-web (프론트엔드) 모두에 영향.

현재 스펙에서는 PM이 프로젝트 간 의존성을 분석하고, 병렬/직렬 실행을 결정한다.

### 권장 방향

Claude Code의 Team 시스템과 태스크 의존성을 활용한다:

```markdown
## CLAUDE.md: 멀티 프로젝트 조율 규칙

### Architect 분석 단계
Architect 에이전트는 다음을 결정합니다:
- 영향받는 프로젝트 목록
- 프로젝트 간 의존성 (예: API 변경 → 프론트엔드 대기)
- 각 프로젝트에서 필요한 작업 범위

### PM 태스크 분해 단계
PM 에이전트는 다음을 수행합니다:
1. TeamCreate로 실행 팀 생성
2. 프로젝트별 에이전트 생성 (Agent 도구 사용)
3. TaskCreate로 태스크 생성
4. 의존성이 있는 태스크는 TaskUpdate(addBlockedBy)로 연결
5. 독립 태스크는 병렬 실행 허용

### 실행 규칙
- 동시 실행 에이전트 수 ≤ {max_concurrent}
- 각 에이전트는 자신의 git worktree에서 작업
- 동일 프로젝트 내 다수 에이전트 시 반드시 worktree 격리
```

### Git Worktree 활용

멀티 프로젝트 병렬 실행에서 git worktree 격리는 여전히 필수다. 에이전트의 CLAUDE.md에 worktree 생성 규칙을 포함시킨다:

```markdown
## 작업 환경 격리
1. 태스크 시작 시 worktree를 생성합니다:
   git worktree add {project}/.git/pylon-worktrees/{agent}-{task} -b {branch}
2. worktree 경로에서 모든 작업을 수행합니다
3. 완료 후 커밋 + push
4. worktree 제거: git worktree remove {path}
```

### 우선순위: P1 (중요, 단일 프로젝트로 MVP 가능하지만 핵심 가치)

---

## 미결 요구사항 10: 비용 제어

### 문제의 본질

AI 에이전트는 무한 루프, 과도한 재시도, 불필요한 서브에이전트 생성 등으로 비용이 폭주할 수 있다. 현재 스펙의 안전장치:

| 장치 | 현재 값 | 구현 위치 |
|------|---------|----------|
| maxTurns | 50 (기본) | Claude CLI `--max-turns` |
| max_attempts | 2 (최초+재시도1) | Pipeline.MaxAttempts |
| task_timeout | 30m | config.yml |
| max_concurrent | 5 | config.yml |
| CLAUDE_CODE_MAX_TURNS | maxTurns | 환경변수 |

### 권장 방향: 다층 방어

**Layer 1: Go 런처 (환경변수 설정)**
```bash
# Go 런처가 claude 실행 전 설정
export CLAUDE_CODE_MAX_TURNS=200       # 루트 에이전트 전체 턴 상한
export CLAUDE_AUTOCOMPACT_PCT_OVERRIDE=80
```

**Layer 2: CLAUDE.md (AI 수준 규칙)**
```markdown
## 비용 관리 규칙
- 서브에이전트 동시 실행: 최대 {max_concurrent}개
- 태스크 재시도: 최대 {max_attempts}회 (실패 시 사용자에게 보고)
- 서브에이전트 maxTurns: 에이전트별 설정 따름 (기본 {max_turns})
- 파이프라인 전체 목표 시간: {task_timeout} 이내
- 에스컬레이션: 재시도 초과, 타임아웃 임박 시 즉시 사용자에게 보고하고 중단
```

**Layer 3: Agent 도구 파라미터**
```
루트 에이전트가 서브에이전트 생성 시:
Agent(subagent_type: "backend-dev", maxTurns: 30, ...)
```

**Layer 4: 슬래시 커맨드 감시 (선택)**
```markdown
# .claude/commands/budget.md
현재 파이프라인의 비용 추정을 보여줍니다.
1. state.json에서 단계별 소요 시간 확인
2. 생성된 서브에이전트 수 확인
3. 재시도 횟수 확인
4. 예상 잔여 비용 표시
```

### 추가 안전장치

1. **무한 루프 방지**: CLAUDE.md에 "동일 검증 실패가 2회 연속 발생하면 사용자에게 에스컬레이션" 규칙
2. **서브에이전트 폭주 방지**: CLAUDE.md에 "전체 파이프라인에서 생성한 서브에이전트 총 수가 {max_concurrent * 3}을 초과하면 중단" 규칙
3. **타임아웃**: Go 런처가 tmux 세션 생성 시 watchdog 타이머 설정 가능 (v2)

### 우선순위: P0 (필수)

---

## 우선순위 종합 및 구현 순서

### P0 (필수 -- 시스템 동작의 전제 조건)

| # | 요구사항 | 핵심 구현물 | 의존성 |
|---|---------|-----------|--------|
| 4 | CLAUDE.md 동적 생성 | Go 런처의 `.claude/` 생성기 | 없음 (최우선) |
| 2 | 세션 지속성 | Go 런처의 tmux 래퍼 | #4 |
| 1 | 상태 관리 | `pylon stage` CLI + state.json | #4 |
| 3 | 서브에이전트 통신 | CLAUDE.md 통신 규칙 + 결과 파일 포맷 | #4 |
| 10 | 비용 제어 | 환경변수 + CLAUDE.md 규칙 + maxTurns | #4 |

**P0 구현 순서**: #4 → #2 → #1 → #3 → #10

### P1 (중요 -- 핵심 가치 실현)

| # | 요구사항 | 핵심 구현물 |
|---|---------|-----------|
| 5 | 슬래시 커맨드 설계 | `.claude/commands/` 파일 5~7개 |
| 7 | 교차 검증 | `pylon verify` CLI + CLAUDE.md 검증 규칙 |
| 6 | 메모리 접근 | `pylon mem` CLI 서브커맨드 |
| 9 | 멀티 프로젝트 조율 | CLAUDE.md 조율 규칙 + worktree 지침 |

### P2 (개선 -- 차후 고도화)

| # | 요구사항 | 핵심 구현물 |
|---|---------|-----------|
| 8 | 도메인 지식 갱신 | Tech Writer 에이전트 정의 + 자기진화 규칙 |

---

## Go 바이너리의 새로운 역할 정리

전환 후 Go 바이너리(`pylon`)의 역할은 "런처 + 유틸리티"로 축소된다:

### 유지되는 CLI 명령어

| 명령어 | 역할 |
|--------|------|
| `pylon` (인자 없이) | 런처: .pylon/ 탐색 → .claude/ 생성 → tmux+claude 실행 |
| `pylon init` | 워크스페이스 초기화 |
| `pylon doctor` | 필수 도구 검증 |
| `pylon destroy` | 워크스페이스 해체 |

### 추가할 CLI 명령어

| 명령어 | 역할 |
|--------|------|
| `pylon stage transition <from> <to>` | 파이프라인 상태 전이 검증 + 기록 |
| `pylon stage status` | 현재 파이프라인 상태 조회 |
| `pylon mem search <query>` | 프로젝트 메모리 BM25 검색 |
| `pylon mem store ...` | 프로젝트 메모리 저장 |
| `pylon mem list [--category]` | 프로젝트 메모리 조회 |
| `pylon verify <project>` | 교차 검증 실행 |

### 제거되는 CLI 명령어

| 명령어 | 대체 방안 |
|--------|----------|
| `pylon request` | TUI 대화로 대체 |
| `pylon status` | `/status` 슬래시 커맨드 또는 `pylon stage status` |
| `pylon cancel` | `/cancel` 슬래시 커맨드 |
| `pylon resume` | `pylon` (런처가 자동 판단) |
| `pylon review` | `/review` 슬래시 커맨드 |
| `pylon cleanup` | `/cleanup` 슬래시 커맨드 |
| `pylon dashboard` | 별도 결정 필요 |

---

## 기존 코드 재활용도

| 패키지 | 재활용 여부 | 비고 |
|--------|-----------|------|
| `internal/config/` | **전체 재활용** | config.yml, agent.md 파싱은 그대로 필요 |
| `internal/store/` | **전체 재활용** | SQLite, BM25 검색은 pylon mem으로 노출 |
| `internal/memory/` | **전체 재활용** | 선제적/반응적 메모리 로직 유지 |
| `internal/protocol/` | **부분 재활용** | MessageEnvelope은 결과 파일에 사용, inbox/outbox는 축소 |
| `internal/orchestrator/pipeline.go` | **전체 재활용** | pylon stage에서 상태 전이 검증 |
| `internal/orchestrator/verify.go` | **전체 재활용** | pylon verify에서 교차 검증 실행 |
| `internal/orchestrator/conversation.go` | **전체 재활용** | 대화 기록 관리 유지 |
| `internal/orchestrator/orchestrator.go` | **부분 재활용** | 복구 로직은 유지, 실행 루프는 제거 |
| `internal/tmux/` | **부분 재활용** | 세션 생성/확인은 유지, 에이전트별 관리는 축소 |
| `internal/agent/runner.go` | **축소** | Claude CLI 래퍼는 불필요 (Claude Code가 직접 실행) |
| `internal/agent/claudemd.go` | **대폭 변경** | 개별 에이전트용 → 프로젝트 수준 CLAUDE.md 생성기로 전환 |
| `internal/agent/lifecycle.go` | **제거 가능** | Claude Code Team 시스템이 대체 |
| `internal/git/` | **전체 재활용** | worktree, branch, PR 로직 유지 |
| `internal/slug/` | **전체 재활용** | 파이프라인 ID 생성에 계속 사용 |
| `internal/cli/` | **대폭 변경** | 새 서브커맨드 구조로 재편 |

**재활용률 추정**: 기존 Go 코드의 약 70%가 새 아키텍처에서 재활용 가능하다. 핵심 변경은 "실행 루프"의 제거와 "런처 + CLI 유틸리티" 패턴으로의 전환이다.

---

## 위험 요소 정리

### 높은 위험

| 위험 | 원인 | 완화 방안 |
|------|------|----------|
| AI 상태 관리 신뢰성 | AI가 파이프라인 규칙을 안 따를 수 있음 | `pylon stage` CLI로 유효 전이 프로그래밍적 보장 |
| 세션 끊김 시 작업 유실 | Claude Code 서브에이전트가 부모와 함께 종료됨 | tmux 래퍼로 세션 생존, 결과 파일로 중간 상태 보존 |
| 비용 폭주 | 무한 루프, 과도한 재시도 | 다층 방어 (환경변수 + CLAUDE.md 규칙 + maxTurns) |

### 중간 위험

| 위험 | 원인 | 완화 방안 |
|------|------|----------|
| 메모리 접근 지연 | CLI 호출 오버헤드 | pylon mem 응답 최적화, 선제적 주입으로 호출 최소화 |
| 서브에이전트 조율 복잡도 | Claude Code Team 시스템의 한계 | 점진적 복잡도 증가 (단일 프로젝트 → 멀티 프로젝트) |
| CLAUDE.md 크기 vs 준수율 | 너무 길면 AI가 규칙 무시 | 핵심 규칙만 CLAUDE.md에, 세부 사항은 참조 파일로 |

### 낮은 위험

| 위험 | 원인 | 완화 방안 |
|------|------|----------|
| 도메인 지식 미갱신 | Tech Writer 스폰을 잊을 수 있음 | 파이프라인 규칙에 필수 단계로 명시 |
| 교차 검증 우회 | 에이전트가 검증을 건너뛸 수 있음 | PM 독립 검증으로 이중 방어 |
