# Pylon

> 스타크래프트 프로토스의 파일런처럼, AI 에이전트 팀을 구동하는 에너지의 원천.

**사람은 요구사항만 전달하고, AI 에이전트 팀이 분석 / 설계 / 구현 / PR까지 수행합니다.**

Pylon은 Claude Code 기반 멀티에이전트 개발팀 오케스트레이터입니다. `pylon`을 실행하면 대화형 AI 세션이 시작되고, 루트 에이전트(PO)가 사용자와 대화하며 서브 에이전트 팀을 오케스트레이션합니다.

## 요구 사항

| 도구 | 용도 | 설치 |
|------|------|------|
| **Go** 1.24+ | 빌드 | https://go.dev/dl/ |
| **tmux** | 세션 지속성 | `brew install tmux` |
| **git** | 버전 관리 | 기본 설치 |
| **gh** | GitHub PR 생성 | `brew install gh` |
| **Claude Code** | AI 에이전트 백엔드 | https://docs.anthropic.com/en/docs/claude-code |

`pylon doctor`로 설치 상태를 확인할 수 있습니다.

## 설치

```bash
go install github.com/kyago/pylon/cmd/pylon@latest
```

또는 소스 빌드:

```bash
git clone https://github.com/kyago/pylon.git
cd pylon
make build    # bin/pylon 생성
make install  # $GOPATH/bin/pylon 설치
```

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

Permission Mode를 선택하면 Claude Code TUI 세션이 tmux에서 시작됩니다.

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
              ├── /index ─── 코드베이스 인덱싱
              ├── /status ── 파이프라인 상태 조회
              ├── /verify ── 빌드/테스트/린트 검증
              │
              └── 서브 에이전트 (Claude Code Agent 도구)
                   ├── Architect ── 기술 분석
                   ├── PM ───────── 태스크 분해
                   ├── Developer ── 코드 구현
                   └── Tech Writer ─ 도메인 문서 갱신
```

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
| `/index` | 프로젝트 코드베이스를 분석하여 도메인 위키 갱신 |
| `/status` | 파이프라인 및 에이전트 상태 조회 |
| `/verify` | 빌드/테스트/린트 교차 검증 실행 |
| `/add-project` | 새 프로젝트를 git submodule로 추가 |
| `/cancel` | 진행 중인 파이프라인 취소 |
| `/review` | PR 코드 리뷰 |

## CLI 명령어

Go 바이너리가 제공하는 유틸리티 명령입니다. 루트 에이전트가 파이프라인 제어와 메모리 접근에 사용합니다.

### 기본 명령

| 명령어 | 설명 |
|--------|------|
| `pylon` | Claude Code TUI 세션 실행 (기본 동작) |
| `pylon init` | 워크스페이스 초기화 |
| `pylon doctor` | 필수 도구 설치 확인 |
| `pylon add-project <url>` | 프로젝트 서브모듈 추가 |
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
│   ├── commands/              # 슬래시 커맨드 (index, status, verify ...)
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
runtime:
  max_concurrent: 3           # 동시 에이전트 수
  max_turns: 200              # Claude 최대 턴 수
  max_attempts: 2             # 검증 재시도 횟수
  permission_mode: default    # default | acceptEdits | bypassPermissions

tmux:
  session_prefix: "pylon"
  history_limit: 50000

git:
  branch_prefix: "pylon"
  default_base: "main"
  pr:
    draft: true
    reviewers: []

memory:
  proactive_injection: true
  proactive_max_tokens: 500
```

## 세션 관리

```bash
pylon                  # 새 세션 시작 또는 기존 세션에 재연결
tmux attach -t pylon-root   # 수동 재연결
pylon cleanup          # 좀비 세션 정리
pylon destroy          # 워크스페이스 완전 제거
```

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
| 세션 관리 | tmux |
| AI 백엔드 | Claude Code CLI |
| 저장소 | SQLite (CGO-free, WAL 모드) |
| 메모리 검색 | FTS5 BM25 |

## 라이선스

MIT
