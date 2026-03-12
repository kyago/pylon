# Pylon

**사람은 요구사항만 전달하고, AI 에이전트 팀이 분석 / 설계 / 구현 / PR까지 수행합니다.**

Pylon은 Claude Code 기반 멀티에이전트 개발팀 오케스트레이터입니다. `pylon`을 실행하면 Claude Code TUI 세션이 직접 시작되고, 루트 에이전트(PO)가 사용자와 대화하며 서브 에이전트 팀을 오케스트레이션합니다.

## 요구 사항

| 도구 | 용도 | 설치 |
|------|------|------|
| **Go** 1.24+ | 빌드 | https://go.dev/dl/ |
| **git** | 버전 관리 | 기본 설치 |
| **gh** | GitHub PR 생성 | `brew install gh` |
| **Claude Code** | AI 에이전트 백엔드 | https://docs.anthropic.com/en/docs/claude-code |

`pylon doctor`로 설치 상태를 확인할 수 있습니다.

## 설치

```bash
go install github.com/kyago/pylon/cmd/pylon@latest
```

> **참고:** `go install`은 바이너리를 `$GOPATH/bin` (기본값: `~/go/bin`)에 설치합니다.
> 해당 디렉토리가 `PATH`에 포함되어 있는지 확인하세요:
>
> ```bash
> export PATH="$PATH:$(go env GOPATH)/bin"
> ```
>
> 영구 적용하려면 위 줄을 `~/.bashrc`, `~/.zshrc` 등 셸 설정 파일에 추가하세요.

또는 소스 빌드:

```bash
git clone https://github.com/kyago/pylon.git
cd pylon
make build    # bin/pylon 생성
make install  # $(go env GOPATH)/bin/pylon 설치
```

## 업데이트

### go install로 설치한 경우

```bash
go install github.com/kyago/pylon/cmd/pylon@latest
```

특정 버전으로 업데이트:

```bash
go install github.com/kyago/pylon/cmd/pylon@v0.2.0
```

### 소스 빌드로 설치한 경우

```bash
cd pylon
git pull origin main
make build
make install
```

### 업데이트 후 확인

```bash
pylon version    # 버전 확인
pylon doctor     # 의존성 상태 확인
```

> **기존 워크스페이스 호환성:** `.pylon/` 디렉토리의 설정 파일은 하위 호환됩니다.
> 업데이트 후 기존 워크스페이스에서 `pylon`을 바로 실행할 수 있습니다.

## 빠른 시작

### 1. 워크스페이스 초기화

```bash
mkdir my-workspace && cd my-workspace
git init
pylon init
```

### 2. 프로젝트 추가

```bash
pylon add-project https://github.com/user/my-app.git
```

### 3. 실행

```bash
pylon
```

Permission Mode를 선택하면 Claude Code TUI 세션이 시작됩니다.

```
Permission Mode 선택
Claude Code 실행 권한을 설정합니다

> default — 매번 권한 확인
  acceptEdits — 파일 편집 자동 허용
  bypassPermissions — 모든 권한 자동 허용
```

### 4. AI와 대화

TUI 세션에서 자연어로 요구사항을 전달합니다:

```
> 로그인 기능 구현해줘

PO: 몇 가지 확인이 필요합니다.
  1. 인증 방식은 JWT와 세션 중 어떤 것을 선호하시나요?
  2. 소셜 로그인도 필요한가요?
  ...
```

루트 에이전트가 요구사항을 분석하고, 서브 에이전트 팀에 작업을 위임하여 구현까지 자동 수행합니다.

## 동작 방식

### 아키텍처

```
사용자 ←→ Claude Code TUI (루트 에이전트 / PO)
              │
              ├── /pl:index ─── 코드베이스 인덱싱
              ├── /pl:status ── 파이프라인 상태 조회
              ├── /pl:verify ── 빌드/테스트/린트 검증
              │
              └── 서브 에이전트 (Claude Code Agent 도구)
                   ├── Analyst ──── 요구사항 분석
                   ├── Architect ── 기술 분석
                   ├── Planner ──── 태스크 분해
                   ├── Developer ── 코드 구현
                   ├── Code Reviewer ─ 코드 리뷰
                   ├── Debugger ──── 디버깅
                   ├── Critic ────── 품질 게이트
                   └── Tech Writer ─ 도메인 문서 갱신
```

Pylon은 Go CLI가 워크스페이스를 준비한 뒤, Claude Code를 직접 실행합니다. 서브 에이전트는 Claude Code 네이티브 기능(Agent tool, TeamCreate)으로 생성되어 별도의 프로세스 관리 계층 없이 동작합니다.

### 핵심 개념

| 개념 | 설명 |
|------|------|
| **`.pylon/`** | 워크스페이스 소스 오브 트루스 (설정, 에이전트 정의, 도메인 지식) |
| **`.claude/`** | `pylon` 실행 시 `.pylon/`에서 동적 생성되는 Claude Code 설정 |
| **CLAUDE.md** | 루트 에이전트의 시스템 프롬프트 (실행마다 자동 갱신) |
| **슬래시 커맨드** | AI가 사용하는 내부 스킬 (`.claude/commands/`) |
| **파이프라인** | 요구사항 → 코드 변환 과정의 상태 기계 |
| **프로젝트 메모리** | SQLite + BM25 기반 지식 저장소 |

### 파이프라인 흐름

```
요구사항 전달
    │
    ├─ [1] PO 대화 ─── 요구사항 구체화, 역질문
    ├─ [2] Architect 분석 ─── 기술 방향성 결정
    ├─ [3] PM 태스크 분해 ─── 작업 할당, 실행 순서
    ├─ [4] 에이전트 실행 ─── 프로젝트별 병렬 구현
    ├─ [5] 교차 검증 ─── 빌드/테스트/린트
    ├─ [6] PR 생성 ─── GitHub PR
    ├─ [7] PO 검증 ─── 수용 기준 확인
    └─ [8] 위키 갱신 ─── 도메인 지식 업데이트
```

## 슬래시 커맨드

TUI 세션 내에서 AI가 사용하는 내부 스킬입니다:

| 커맨드 | 설명 |
|--------|------|
| `/pl:index` | 프로젝트 코드베이스를 분석하여 도메인 위키 갱신 |
| `/pl:status` | 파이프라인 및 에이전트 상태 조회 |
| `/pl:verify` | 빌드/테스트/린트 교차 검증 실행 |
| `/pl:add-project` | 새 프로젝트를 git submodule로 추가 |
| `/pl:cancel` | 진행 중인 파이프라인 취소 |
| `/pl:review` | PR 코드 리뷰 |

## CLI 명령어

Go 바이너리가 제공하는 유틸리티 명령입니다. 루트 에이전트가 파이프라인 제어와 메모리 접근에 사용합니다.

### 기본 명령

| 명령어 | 설명 |
|--------|------|
| `pylon` | Claude Code TUI 세션 실행 (기본 동작) |
| `pylon init` | 워크스페이스 초기화 |
| `pylon doctor` | 필수 도구 설치 확인 |
| `pylon add-project <url>` | 프로젝트 서브모듈 추가 |
| `pylon index` | 프로젝트 코드베이스 인덱싱 |
| `pylon destroy` | 워크스페이스 완전 제거 |
| `pylon version` | 버전 정보 |

### 파이프라인 상태 관리

에이전트가 파이프라인 진행을 제어할 때 사용합니다:

```bash
pylon stage list                                      # 파이프라인 목록
pylon stage status --pipeline <id>                     # 상태 조회
pylon stage transition --pipeline <id> --to <stage>    # 상태 전이
```

### 프로젝트 메모리

에이전트가 프로젝트 지식을 저장/검색할 때 사용합니다:

```bash
pylon mem list --project <name>                        # 메모리 목록
pylon mem search --project <name> --query "검색어"      # BM25 검색
pylon mem store --project <name> --key "키" --content "내용"  # 저장
```

모든 명령에 `--json` 플래그를 추가하면 JSON 형식으로 출력됩니다.

## 워크스페이스 구조

```
workspace/
├── .pylon/                    # 소스 오브 트루스 (git 추적)
│   ├── config.yml             # 워크스페이스 설정
│   ├── agents/                # 에이전트 정의 (PO, PM, Architect, Tech Writer)
│   ├── domain/                # 팀 도메인 지식 (위키)
│   ├── runtime/               # 파이프라인 상태, 에이전트 결과
│   └── pylon.db               # SQLite (파이프라인 상태 + 프로젝트 메모리)
│
├── .claude/                   # 동적 생성 (git 무시)
│   ├── commands/pl/           # 슬래시 커맨드 (pl:index, pl:status, pl:verify ...)
│   └── ...
│
├── CLAUDE.md                  # 루트 에이전트 시스템 프롬프트 (동적 생성)
│
├── project-a/                 # git submodule
│   └── .pylon/context.md      # 프로젝트 컨텍스트
└── project-b/                 # git submodule
```

## 설정

`.pylon/config.yml`:

```yaml
version: "0.1"

runtime:
  backend: claude-code         # AI 백엔드
  max_concurrent: 5            # 동시 에이전트 수
  max_turns: 50                # Claude 최대 턴 수
  max_attempts: 2              # 검증 재시도 횟수
  task_timeout: 30m            # 태스크 타임아웃
  permission_mode: acceptEdits # default | acceptEdits | bypassPermissions

git:
  branch_prefix: task          # 작업 브랜치 접두사
  default_base: main           # 기본 베이스 브랜치
  auto_push: true              # 자동 푸시
  worktree:
    enabled: true              # git worktree 격리
    auto_cleanup: true         # 완료 후 자동 정리
  pr:
    draft: false
    reviewers: []

wiki:
  auto_update: true            # 위키 자동 갱신
  update_on:
    - task_complete
    - pr_merged

dashboard:
  host: localhost
  port: 7777

memory:
  compaction_threshold: 0.7
  proactive_injection: true
  proactive_max_tokens: 2000
  session_archive: true

conversation:
  retention_days: 90
```

모든 설정에는 기본값이 있으므로 `version` 필드만 필수입니다. 기본값은 `pylon init`이 생성하는 템플릿을 참고하세요.

## Default Agent Pack

Running `pylon init` automatically creates the following agents in `.pylon/agents/`.

### Agent Definition Format

Each agent consists of YAML frontmatter + Markdown body. Compatible with Claude Code frontmatter fields.

```yaml
---
# Claude Code compatible fields
name: analyst
description: "Read-only analysis agent that systematically analyzes requirements and derives clear acceptance criteria"
model: opus
tools: [Read, Grep, Glob]
disallowedTools: [Write, Edit]
maxTurns: 20
permissionMode: default
isolation: worktree

# Pylon-only fields
role: Requirements Analyst
backend: claude-code
scope: [project-api]
env:
  CLAUDE_CODE_EFFORT_LEVEL: high
---
(system prompt body)
```

### Agent List

| Agent | Role | Model | Description |
|-------|------|-------|-------------|
| **po** | Product Owner | — | Analyzes user requirements, computes ambiguity scores, defines acceptance criteria |
| **pm** | Project Manager | — | Decomposes tasks, assigns agents, manages execution order |
| **architect** | Architect | — | Cross-project architecture decisions, technical direction analysis |
| **tech-writer** | Tech Writer | — | Maintains domain knowledge and project documentation |
| **analyst** | Requirements Analyst | opus | Requirement analysis and acceptance criteria derivation (read-only) |
| **planner** | Execution Planner | opus | Execution planning and task decomposition |
| **code-reviewer** | Code Reviewer | opus | Severity-classified code review, SOLID principles validation (read-only) |
| **debugger** | Debugger | sonnet | Root cause analysis, build error resolution |
| **critic** | Quality Critic | opus | Final quality gate for plans and code (read-only) |

## 개발

```bash
make build       # 빌드
make test        # 테스트
make lint        # 린트
make clean       # 정리
```

## 기술 스택

| 영역 | 기술 |
|------|------|
| 언어 | Go |
| CLI 프레임워크 | Cobra |
| TUI 컴포넌트 | charmbracelet/huh |
| 프로세스 실행 | syscall.Exec (직접 실행) |
| AI 백엔드 | Claude Code CLI |
| 저장소 | SQLite (CGO-free, WAL 모드) |
| 메모리 검색 | FTS5 BM25 |

## 라이선스

MIT
