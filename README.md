# Pylon

> 스타크래프트 프로토스의 파일런처럼, AI 에이전트 팀을 구동하는 에너지의 원천.

**사람은 요구사항만 전달하고, AI 에이전트 팀이 분석 / 설계 / 구현 / PR까지 수행합니다.**

Pylon은 Claude Code CLI 기반 멀티에이전트 개발팀 오케스트레이션 도구입니다. PO, PM, Architect 등 역할별 에이전트가 tmux 세션에서 병렬로 동작하며, 파이프라인 기반으로 요구사항을 코드로 전환합니다.

## 요구 사항

| 도구 | 용도 | 설치 |
|------|------|------|
| **Go** 1.24+ | 빌드 | https://go.dev/dl/ |
| **tmux** | 에이전트 프로세스 격리 | `brew install tmux` |
| **git** | 버전 관리, worktree | 기본 설치 |
| **gh** | GitHub PR 생성 | `brew install gh` |
| **Claude Code** | 에이전트 백엔드 | https://docs.anthropic.com/en/docs/claude-code |

`pylon doctor` 명령으로 설치 상태를 확인할 수 있습니다.

## 설치

### 소스 빌드

```bash
git clone https://github.com/yongjunkang/pylon.git
cd pylon
make build
```

`bin/pylon` 바이너리가 생성됩니다.

### PATH에 설치

```bash
make install
```

`$GOPATH/bin/pylon`으로 설치됩니다.

### go install

```bash
go install github.com/yongjunkang/pylon/cmd/pylon@latest
```

## 빠른 시작

### 1. 워크스페이스 초기화

```bash
mkdir my-workspace && cd my-workspace
git init
pylon init
```

`.pylon/` 디렉토리가 생성되며, 기본 config와 에이전트 설정이 포함됩니다.

### 2. 도구 검증

```bash
pylon doctor
```

```
Pylon Doctor
────────────────────────────────────────
✓ tmux       3.4        [required]
✓ git        2.50.1     [required]
✓ gh         2.86.0     [required]
✓ claude     2.1.71     [required]

All checks passed!
```

### 3. 프로젝트 추가

```bash
pylon add-project https://github.com/user/my-app.git
```

### 4. 요구사항 전달

```bash
pylon request "로그인 기능 구현해줘"
```

파이프라인이 시작되며 PO 에이전트가 요구사항을 분석합니다.

### 5. 상태 확인

```bash
pylon status
```

```
Pylon Status
─────────────────────────────────────
Active Sessions:
  ● pylon-po          alive     (created 5m ago)
  ● pylon-architect   alive     (created 3m ago)

Pipeline: 20260307-로그인-기능-구현해줘
  Stage:    architect_analysis
  Attempts: 0/3
```

## 명령어

| 명령어 | 설명 |
|--------|------|
| `pylon init` | 워크스페이스 초기화 |
| `pylon doctor` | 필수 도구 설치 확인 |
| `pylon request "<요구사항>"` | 요구사항 전달 및 파이프라인 시작 |
| `pylon status` | 현재 작업 상태 확인 |
| `pylon cancel <pipeline-id>` | 진행 중인 파이프라인 취소 |
| `pylon resume <pipeline-id>` | 중단된 파이프라인 재개 |
| `pylon review <pr-url>` | PR 리뷰 코멘트 처리 |
| `pylon add-project <git-url>` | 프로젝트 서브모듈 추가 |
| `pylon cleanup` | 좀비 tmux 세션 정리 |
| `pylon destroy` | 워크스페이스 완전 제거 |
| `pylon dashboard` | 웹 대시보드 실행 |
| `pylon version` | 버전 정보 출력 |

## 파이프라인 흐름

```
pylon request "요구사항"
    │
    ├─ [1] PO 대화 ─── 사용자와 요구사항 구체화
    │
    ├─ [2] Architect 분석 ─── 기술 방향성 결정
    │
    ├─ [3] PM 태스크 분해 ─── 작업 할당
    │
    ├─ [4] 에이전트 실행 ─── 병렬 구현
    │
    ├─ [5] 교차 검증 ─── 빌드/테스트/린트
    │
    ├─ [6] PR 생성 ─── GitHub PR + 리뷰어 지정
    │
    ├─ [7] PO 검증 ─── 수용 기준 확인
    │
    └─ [8] 완료
```

## 워크스페이스 구조

```
workspace/
├── .pylon/
│   ├── config.yml          # 도구 설정
│   ├── agents/             # 루트 에이전트 정의 (PO, PM, Architect ...)
│   ├── domain/             # 팀 도메인 지식 (위키)
│   ├── skills/             # 에이전트 전문 지식
│   ├── runtime/            # 에이전트 통신 (inbox/outbox)
│   ├── conversations/      # 대화 기록
│   └── pylon.db            # SQLite 상태 저장소
├── project-a/              # git submodule
└── project-b/              # git submodule
```

## 설정

`.pylon/config.yml`에서 주요 설정을 관리합니다:

```yaml
runtime:
  max_concurrent: 3
  max_attempts: 2

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

## 개발

```bash
# 빌드
make build

# 테스트
make test

# 린트
make lint

# 정리
make clean
```

## 기술 스택

| 영역 | 기술 |
|------|------|
| 언어 | Go |
| CLI | Cobra |
| 에이전트 프로세스 | tmux 세션 |
| 에이전트 백엔드 | Claude Code CLI |
| 저장소 | SQLite (CGO-free, WAL 모드) |
| 메모리 검색 | FTS5 BM25 |
| 통신 프로토콜 | JSON 파일 기반 inbox/outbox |

## 라이선스

MIT
