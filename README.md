# Pylon

**사람은 요구사항만 전달하고, AI 에이전트 팀이 알아서 수행합니다.**

Pylon은 Claude Code 기반 멀티도메인 AI 오케스트레이터입니다. `pylon`을 실행하면 Claude Code TUI 세션이 직접 시작되고, 루트 에이전트(PO)가 사용자의 요구사항을 분석하여 적절한 도메인(소프트웨어 개발, 리서치, 콘텐츠 제작, 마케팅)과 워크플로우를 자동 선택하고 전문 에이전트 팀을 오케스트레이션합니다.

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

### 4. 파이프라인 실행

TUI 세션에서 슬래시 커맨드 또는 자연어로 요구사항을 전달합니다:

```
> /pl:pipeline 로그인 기능 구현해줘
```

또는 직접 대화:

```
> 로그인 기능 구현해줘

PO: 몇 가지 확인이 필요합니다.
  1. 인증 방식은 JWT와 세션 중 어떤 것을 선호하시나요?
  2. 소셜 로그인도 필요한가요?
  ...
```

Claude Code TUI가 LLM-as-Orchestrator로 동작하며, 슬래시 커맨드와 셸 스크립트를 조합하여 전체 파이프라인(분석 → 설계 → 구현 → PR)을 자동 수행합니다.

## 동작 방식

### 아키텍처

```
사용자 ←→ Claude Code TUI (LLM-as-Orchestrator)
              │
              ├── 슬래시 커맨드 (.pylon/commands/)
              │   ├── /pl:pipeline ─ 전체 파이프라인
              │   ├── /pl:architect ─ 아키텍처 분석
              │   ├── /pl:breakdown ─ PM 태스크 분해
              │   ├── /pl:execute ─── 에이전트 병렬 실행
              │   ├── /pl:verify ─── 빌드/테스트/린트 검증
              │   ├── /pl:pr ─────── PR 생성
              │   ├── /pl:status ─── 파이프라인 상태 조회
              │   ├── /pl:cancel ─── 취소 및 정리
              │   └── /pl:index ─── 코드베이스 인덱싱
              │
              └── 셸 스크립트 (.pylon/scripts/bash/)
                   ├── init-pipeline.sh
                   ├── run-verification.sh
                   ├── create-pr.sh
                   └── ... (원자적 작업)
```

Pylon은 Go CLI가 워크스페이스를 준비한 뒤, Claude Code를 직접 실행(`syscall.Exec`)합니다. Claude Code TUI는 LLM-as-Orchestrator 패턴으로 동작하며, 슬래시 커맨드와 셸 스크립트를 조합하여 파이프라인을 실행합니다. 파일 기반 상태(artifact 존재 = 단계 완료)로 진행을 추적합니다.

### 핵심 개념

| 개념 | 설명 |
|------|------|
| **`.pylon/`** | 워크스페이스 소스 오브 트루스 (설정, 에이전트 정의, 도메인 지식) |
| **`.claude/`** | `pylon` 실행 시 `.pylon/`에서 동적 생성되는 Claude Code 설정 (agents 심링크, commands, hooks) |
| **CLAUDE.md** | 루트 에이전트의 시스템 프롬프트 (실행마다 자동 갱신) |
| **슬래시 커맨드** | AI가 사용하는 내부 스킬 (`.pylon/commands/`) |
| **파이프라인** | 파일 기반 워크플로우 (산출물 존재 = 단계 완료) |
| **프로젝트 메모리** | SQLite + BM25 기반 지식 저장소 |

### 파이프라인 흐름

```
/pl:pipeline "로그인 기능 구현"
    │
    ├─ [1] init-pipeline.sh ────── requirement.md
    ├─ [2] PO 분석 ────────────── requirement-analysis.md
    ├─ [3] Architect 분석 ──────── architecture.md
    ├─ [4] PM 태스크 분해 ──────── tasks.json
    ├─ [5] Agent 병렬 실행 ─────── execution-log.json
    ├─ [6] run-verification.sh ─── verification.json
    ├─ [7] create-pr.sh ────────── pr.json
    └─ 완료 보고
```

## 슬래시 커맨드

TUI 세션 내에서 AI가 사용하는 내부 스킬입니다:

| 커맨드 | 설명 |
|--------|------|
| `/pl:pipeline` | 전체 파이프라인 실행 (요구사항 → 분석 → 설계 → 구현 → PR) |
| `/pl:architect` | 아키텍처 분석 단독 실행 |
| `/pl:breakdown` | PM 태스크 분해 |
| `/pl:execute` | 에이전트 병렬 실행 |
| `/pl:verify` | 빌드/테스트/린트 교차 검증 |
| `/pl:pr` | PR 생성 |
| `/pl:status` | 파이프라인 상태 조회 (파일 기반) |
| `/pl:cancel` | 파이프라인 취소 및 정리 |
| `/pl:index` | 프로젝트 코드베이스 인덱싱 |

## 내장 에이전트 (38종)

`pylon init` 시 `.pylon/agents/`에 설치됩니다. `pylon sync-agents`로 최신 버전으로 갱신할 수 있습니다. PO가 요구사항을 분석하여 적절한 도메인의 에이전트를 자동 선택합니다.

### 소프트웨어 개발 (23종)

| 에이전트 | 역할 |
|----------|------|
| po | 프로덕트 오너 (요구사항 분석, 도메인 라우팅) |
| pm | 프로젝트 매니저 (태스크 분해, 조율) |
| architect | 아키텍트 (기술 방향성, 의존성 분석) |
| analyst | 분석가 (요구사항 분석) |
| backend-dev | 백엔드 개발자 |
| frontend-dev | 프론트엔드 개발자 |
| designer | UI/UX 디자이너 |
| test-engineer | 테스트 엔지니어 |
| code-reviewer | 코드 리뷰어 |
| code-simplifier | 코드 단순화 |
| debugger | 디버거 |
| devops | DevOps 엔지니어 |
| explorer | 코드베이스 탐색 |
| git-master | Git 워크플로 |
| perf-engineer | 성능 엔지니어 |
| refactorer | 리팩토링 |
| researcher | 리서치 |
| security-reviewer | 보안 리뷰 |
| tech-writer | 기술 문서 작성 |
| doc-specialist | 외부 문서 전문가 |
| tracer | 추적/디버깅 |
| verifier | 검증 |
| critic | 비평/리뷰 |

### 리서치/조사 (5종)

| 에이전트 | 역할 |
|----------|------|
| lead-researcher | 리서치 리더 (조사 계획, 팀 조율) |
| web-searcher | 웹 검색 전문가 |
| academic-analyst | 학술 자료 분석가 |
| fact-checker | 팩트 체커 (교차 검증) |
| report-writer | 보고서 작성자 |

### 콘텐츠 제작 (5종)

| 에이전트 | 역할 |
|----------|------|
| content-strategist | 콘텐츠 전략가 |
| writer | 콘텐츠 작가 |
| editor | 편집자 |
| seo-specialist | SEO 전문가 |
| content-reviewer | 콘텐츠 QA 리뷰어 |

### 마케팅 (5종)

| 에이전트 | 역할 |
|----------|------|
| market-researcher | 시장 조사 분석가 |
| copywriter | 카피라이터 |
| campaign-planner | 캠페인 기획자 |
| data-analyst | 마케팅 데이터 분석가 |
| brand-strategist | 브랜드 전략가 |

## CLI 명령어

Go 바이너리가 제공하는 유틸리티 명령입니다. 루트 에이전트가 파이프라인 제어와 메모리 접근에 사용합니다.

### 기본 명령

| 명령어 | 설명 |
|--------|------|
| `pylon` | Claude Code TUI 세션 실행 (기본 동작) |
| `pylon init` | 워크스페이스 초기화 |
| `pylon doctor` | 필수 도구 설치 확인 |
| `pylon version` | 버전 정보 |
| `pylon add-project <url>` | 프로젝트 서브모듈 추가 |
| `pylon add-agent <name>` | 커스텀 에이전트 추가 (`--domain`, `--role`) |
| `pylon add-skill <name>` | 커스텀 스킬 추가 |
| `pylon status` | 파이프라인 및 에이전트 상태 조회 |
| `pylon cancel [pipeline-id]` | 진행 중인 파이프라인 취소 |
| `pylon uninstall` | 워크스페이스 완전 제거 |

### 프로젝트 메모리

에이전트가 프로젝트 지식을 저장/검색할 때 사용합니다:

```bash
pylon mem list --project <name>                        # 메모리 목록
pylon mem search --project <name> --query "검색어"      # BM25 검색
pylon mem store --project <name> --key "키" --content "내용"  # 저장
```

### 동기화

| 명령어 | 설명 |
|--------|------|
| `pylon sync-agents` | 내장 에이전트 정의를 워크스페이스에 동기화 (`--force`로 덮어쓰기) |
| `pylon sync-projects` | 프로젝트 목록을 SQLite에 동기화 |
| `pylon sync-memory` | 세션 학습 내용을 프로젝트 메모리에 동기화 |

모든 명령에 `--json` 플래그를 추가하면 JSON 형식으로 출력됩니다.

## 워크스페이스 구조

```
workspace/
├── .pylon/                    # 소스 오브 트루스 (git 추적)
│   ├── config.yml             # 워크스페이스 설정
│   ├── agents/                # 에이전트 정의 (23종)
│   ├── skills/                # 에이전트 스킬
│   ├── domain/                # 팀 도메인 지식 (위키)
│   ├── commands/              # 파이프라인 슬래시 커맨드
│   │   ├── pl-pipeline.md
│   │   ├── pl-architect.md
│   │   └── ...
│   ├── scripts/bash/          # 파이프라인 셸 스크립트
│   │   ├── common.sh
│   │   ├── init-pipeline.sh
│   │   └── ...
│   ├── tasks/                 # 확정된 태스크 스펙
│   ├── runtime/               # (git 무시)
│   │   ├── {pipeline-id}/     # 파이프라인별 산출물
│   │   ├── memory/            # 세션 메모리
│   │   └── sessions/          # 세션 상태
│   ├── conversations/         # 대화 이력 (git 무시)
│   └── pylon.db               # SQLite (파이프라인 상태 + 프로젝트 메모리)
│
├── .claude/                   # 동적 생성 (git 무시)
│   ├── agents/                # .pylon/agents/ 심링크
│   ├── commands/              # 슬래시 커맨드 심링크
│   └── settings.json          # Claude Code hooks 설정
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

memory:
  compaction_threshold: 0.7
  proactive_injection: true
  proactive_max_tokens: 2000
  session_archive: true

conversation:
  retention_days: 90
```

모든 설정에는 기본값이 있으므로 `version` 필드만 필수입니다. 기본값은 `pylon init`이 생성하는 템플릿을 참고하세요.

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
| 프로세스 실행 | syscall.Exec (최소 런처) + Claude Code Agent 도구 (서브에이전트) |
| 파이프라인 작업 | Shell Scripts (원자적 작업) |
| AI 백엔드 | Claude Code CLI |
| 저장소 | SQLite (CGO-free, WAL 모드) |
| 메모리 검색 | FTS5 BM25 |

## 라이선스

MIT
