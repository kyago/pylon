# Pylon 요구사항 명세서 (Draft v0.8)

> 🔷 Pylon — 스타크래프트 프로토스의 파일런처럼, 구조물을 세우고 건물을 소환하는 에너지의 원천.
> AI 에이전트 팀의 기둥이자, 프로젝트를 만들어내는 동력.

## 1. 프로덕트 개요

**한 줄 정의**: 팀 도메인 지식 기반의 AI 멀티에이전트 개발팀 운영 도구

**목표**: 사람은 요구사항만 전달 → AI 에이전트 팀이 분석/설계/구현/PR까지 수행

**플랫폼**: macOS / Linux (TUI + Web Dashboard 기반)

**구현 언어**: Go

## 2. 기술 스택

| 영역 | 기술 | 비고 |
|---|---|---|
| **언어** | Go | 단일 바이너리 배포, 크로스 플랫폼 |
| **TUI** | Bubble Tea + Lipgloss | Charm 생태계 |
| **Web Dashboard** | Templ + HTMX + SSE | Go 바이너리에 임베드, Node 불필요, CLI 동등 기능 제공 |
| **에이전트 프로세스** | 직접 실행 (syscall.Exec / exec.Command) | 프로세스 격리 + git worktree 파일시스템 격리 |
| **에이전트 백엔드 (MVP)** | Claude Code CLI | 오케스트레이터가 직접 프로세스로 실행 |
| **테스트** | Unit + 통합 테스트 | E2E는 추후 |

## 3. 용어 정의

- **워크스페이스**: 루트 git repo. 팀 도메인 지식 + 프로젝트들의 상위 디렉토리
- **프로젝트**: 워크스페이스 하위 git submodule. 독립적인 코드 저장소
- **오케스트레이터**: Go 프로세스. 모든 에이전트의 생명주기를 관리하고, 에이전트 간 통신을 중재하는 핵심 프로세스
- **루트 에이전트**: 워크스페이스 레벨에서 동작하는 에이전트. 기본 구성: PO, PM, Architect, Tech Writer, Reviewer. 사용자가 자유롭게 커스텀 가능
- **프로젝트 에이전트**: 프로젝트 레벨에서 실제 구현하는 에이전트 (개발자/디자이너/QA 등)
- **도메인 지식**: 2계층으로 분리 관리되는 프로젝트 컨텍스트
  - **사람 소유 (AI read-only)**: `.specify/memory/constitution.md` — 프로젝트 헌법, 개발 원칙. 사람이 정의하고 에이전트는 읽기만 가능
  - **AI 소유 (자동 생성/갱신)**: `.pylon/domain/*` — 학습된 컨벤션, 아키텍처 결정, 용어 사전. AI 기반으로 생성/갱신하며 사용자가 직접 수정하지 않음. 수정이 필요하면 루트 에이전트에게 요청
- **대화 (conversation)**: 사용자 ↔ PO 에이전트 간 요구사항 구체화 인터랙션 기록

## 4. 워크스페이스 구조

```
workspace/                              ← git repo (루트)
├── .pylon/
│   ├── config.yml                      ← 도구 설정 (에이전트 백엔드, 동시 실행 수 등)
│   ├── domain/                         ← 팀 도메인 지식 (위키, AI 생성/갱신)
│   │   ├── conventions.md              ← 코딩 컨벤션, 네이밍 규칙
│   │   ├── architecture.md             ← 전체 아키텍처 문서
│   │   └── glossary.md                 ← 비즈니스 용어 사전
│   ├── agents/                         ← 루트 에이전트 설정
│   │   ├── po.md                       ← PO 에이전트 (요구사항 분석, 수용 기준)
│   │   ├── pm.md                       ← PM 에이전트 (태스크 분해, 실행 조율)
│   │   ├── architect.md                ← Architect 에이전트 (기술 방향성, 의존성)
│   │   ├── tech-writer.md              ← Tech Writer 에이전트 (위키/문서 갱신)
│   │   └── reviewer.md                 ← Reviewer 에이전트 (Constitution 검증 전담)
│   ├── skills/                         ← 에이전트 전문 지식 (preload 대상, 선택)
│   │   ├── api-design-guide.md         ← backend-dev에 preload
│   │   └── test-strategy.md            ← qa에 preload
│   ├── runtime/                        ← 에이전트 통신 런타임 (.gitignore 대상)
│   │   ├── inbox/                      ← 오케스트레이터 → 에이전트 태스크 전달
│   │   │   └── {agent-name}/
│   │   │       └── {task-id}.task.json
│   │   ├── outbox/                     ← 에이전트 → 오케스트레이터 결과 보고
│   │   │   └── {agent-name}/
│   │   │       └── {task-id}.result.json
│   │   ├── state.json                  ← ⚠️ 제거됨: SQLite pipeline_state 테이블로 통합
│   │   ├── pylon.db                    ← SQLite (오케스트레이터 내부 전용, 쿼리/히스토리)
│   │   ├── memory/                     ← 프로젝트 메모리 저장소 (장기 지식)
│   │   └── sessions/                   ← 세션 아카이브 (완료된 세션 기록)
│   ├── conversations/                  ← 대화 기록 (.gitignore 대상)
│   │   └── 20260305-143022-user-login/
│   │       ├── thread.md               ← 전체 대화 기록 (사람 ↔ PO)
│   │       └── meta.yml                ← 상태, 시작시간, 관련 프로젝트
│   └── tasks/                          ← 확정된 작업 지시서 (Source of Truth)
│       └── 20260305-user-login.md      ← 최종 작업 지시서
├── project-api/                        ← git submodule
│   ├── .pylon/
│   │   ├── agents/
│   │   │   ├── backend-dev.md
│   │   │   └── qa.md
│   │   ├── verify.yml                  ← 교차 검증 명령어 (빌드/테스트/린트)
│   │   └── context.md                  ← 프로젝트 고유 컨텍스트 (AI 자동 생성)
│   └── .specify/                       ← speckit 산출물 (pylon add-project 시 자동 생성)
│       ├── memory/
│       │   └── constitution.md         ← 프로젝트 헌법 (사람 소유, AI read-only)
│       ├── specs/
│       │   └── {###-feature}/
│       │       ├── spec.md             ← 사용자 스토리 + 수용 기준
│       │       ├── plan.md             ← 기술 접근 + 헌법 검증
│       │       ├── tasks.md            ← Phase별 작업 분해 ([P] = 병렬 가능)
│       │       ├── contracts/          ← API 계약 (YAML)
│       │       └── data-model.md       ← 데이터 모델
│       └── templates/                  ← 산출물 템플릿
├── project-web/                        ← git submodule
│   ├── .pylon/
│   │   └── agents/
│   │       └── frontend-dev.md
│   └── .specify/                       ← speckit 산출물
└── ...
```

### Git 정책

**커밋 대상**: `config.yml`, `domain/`, `agents/`, `skills/`, `tasks/`, 프로젝트별 `context.md`

**`.gitignore` 대상**: `conversations/`, `runtime/`, 런타임 상태 파일, 로그 파일

## 5. 에이전트 설정 포맷

**YAML frontmatter + Markdown body**

```markdown
---
name: backend-dev
role: Backend Developer
backend: claude-code
scope:
  - project-api
tools:
  - git
  - gh
  - docker
capabilities:                  # 에이전트 능력 메타데이터 (PM의 태스크 매칭에 활용)
  accepts:                     # 수용 가능한 입력 유형
    - api_contract
    - schema_definition
    - test_spec
  produces:                    # 생성하는 산출물 유형
    - implementation
    - unit_tests
    - migration
maxTurns: 30                   # 최대 LLM 턴 수 (기본: config.yml의 max_turns)
permissionMode: acceptEdits    # 권한 모드 (기본: config.yml의 permission_mode)
isolation: worktree            # 격리 방식 (기본: worktree)
model: sonnet                  # 모델 지정 (선택, 기본: config.yml의 backend 모델)
env:                           # 에이전트별 환경변수 override (선택)
  CLAUDE_CODE_EFFORT_LEVEL: high
---

# Backend Developer

## 역할
project-api의 백엔드 기능을 구현하는 개발자.

## 컨벤션
- Go 표준 프로젝트 레이아웃 준수
- 에러는 반드시 래핑하여 반환
- 테스트 커버리지 80% 이상 유지

## 컨텍스트
- API 서버: Echo 프레임워크
- DB: PostgreSQL + sqlc
- 인증: JWT 기반
```

### frontmatter 필드 명세

| 필드 | 타입 | 필수 | 기본값 | 설명 |
|------|------|------|--------|------|
| `name` | string | ✅ | - | 에이전트 고유 식별자 |
| `role` | string | ✅ | - | 에이전트 역할 설명 |
| `backend` | string | | config.yml `backend` | 에이전트 백엔드 |
| `scope` | string[] | | - | 담당 프로젝트 목록 |
| `tools` | string[] | | 전체 허용 | 허용 도구 목록 |
| `maxTurns` | int | | config.yml `max_turns` | 에이전트별 LLM 턴 수 제한. 무한 루프/토큰 폭주 방지 |
| `permissionMode` | string | | config.yml `permission_mode` | `default` \| `acceptEdits` \| `bypassPermissions` |
| `isolation` | string | | `worktree` | `none` \| `worktree`. 에이전트 작업 환경 격리 방식 |
| `model` | string | | backend 기본 | `sonnet` \| `opus` \| `haiku` 등. 에이전트별 모델 지정 |
| `capabilities` | object | | - | 에이전트 능력 메타데이터. `accepts`: 수용 가능한 입력 유형 배열, `produces`: 생성하는 산출물 유형 배열. PM의 태스크-에이전트 매칭에 활용. **매칭 방식**: Phase 0에서는 PM 에이전트가 LLM 판단으로 capabilities를 참고하여 매칭. 정형화된 매칭 알고리즘은 Phase 2에서 정의 |
| `env` | map | | config.yml `env` | 에이전트별 환경변수 override |

**permissionMode 상세**:

| 모드 | 설명 | 권장 대상 |
|------|------|-----------|
| `default` | 모든 작업에 사용자 승인 필요 | PO (사용자 인터랙션) |
| `acceptEdits` | 파일 편집 자동 승인, bash는 승인 필요 | 개발 에이전트 (기본 권장) |
| `bypassPermissions` | 모든 작업 자동 승인 | 신뢰도 높은 자동화 에이전트 |

## 6. 루트 에이전트 기본 구성

기본 5종의 루트 에이전트를 제공하며, 사용자가 자유롭게 추가/수정/삭제 가능.

| 에이전트 | 역할 | 활성 시점 |
|----------|------|-----------|
| **PO** (Product Owner) | 요구사항 분석, 사용자 대화 (역질문), 수용 기준 정의, 비즈니스 검증 | request 초반 + 완료 검증 |
| **PM** (Project Manager) | 태스크 분해, 에이전트 조율, 실행 관리, 에러 처리/에스컬레이션 | 요구사항 확정 후 |
| **Architect** | 크로스 프로젝트 아키텍처 결정, 기술 방향성, 프로젝트 간 의존성 분석 | 태스크 분해 시 PM이 호출 |
| **Tech Writer** | 위키/도메인 지식 갱신, 프로젝트 context.md 관리, 문서 품질 | 작업 완료 후 |
| **Reviewer** | Constitution 검증 전담. 산출물의 constitution.md 준수 여부를 독립적/객관적으로 검증. speckit CLI의 `speckit analyze` 활용 | speckit 산출물 생성/수정 완료 시 |

### speckit 전용 모드 에이전트 역할

speckit 산출물(`.specify/`)이 존재하는 프로젝트에서는 에이전트 역할이 재정의된다. 산출물이 이미 존재하므로 에이전트는 "생성"이 아닌 "검증/보완/실행"에 집중한다.

| 에이전트 | speckit 모드 역할 | speckit 산출물 수정 권한 |
|---------|------------------|------------------------|
| **PO** | 보완자 (Enricher) | spec.md: 제안 → PO 대화 중 사람 승인 후 write. speckit CLI의 `speckit clarify` + `speckit analyze` 활용 |
| **Architect** | 검증 + 조정자 | plan.md (write 직접), contracts/ (write 직접). constitution.md 대비 기술 적합성 검증 |
| **PM** | 할당 + 조율자 | tasks.md (read-only). [P] 마커 파싱 → 병렬/직렬 결정 → Agent Card 매칭 → 태스크 할당. 실행 모니터링 + 에러 처리 + 에스컬레이션 |
| **Developer** | 구현자 | contracts/ 제한적 write (사소한 조정: 필드 타입 변경, 엔드포인트 세부 조정, 응답 필드 추가). 구조적 변경은 Architect에 에스컬레이션 |
| **Tech Writer** | 도메인 지식 전담 | `.pylon/domain/*` (write), context.md (write), constitution.md (read-only). 작업 완료 후 변경사항 분석 → 위키 갱신 |
| **Reviewer** | Constitution 검증 전담 | 전체 산출물 read (검증만). speckit CLI의 `speckit analyze` 활용하여 산출물 간 교차 일관성 분석 + constitution.md 준수 여부 검증 |

**Developer의 구조적 변경 에스컬레이션 기준**: 엔드포인트 삭제/추가, 인증 방식 변경, DB 스키마 구조 변경 등 contracts/의 구조를 바꾸는 작업은 Architect에게 에스컬레이션한다.

> **참고: speckit CLI 커맨드** — 위 표에서 언급되는 `speckit analyze`, `speckit clarify` 등은 pylon CLI 서브커맨드가 아니라 **speckit CLI의 커맨드**이다. 에이전트가 CLAUDE.md의 지시에 따라 셸에서 직접 실행하는 외부 도구이며, pylon의 Section 7 "명령 인터페이스"에는 포함되지 않는다.

## 7. 명령 인터페이스

### 글로벌 플래그

모든 명령에 적용되는 공통 플래그:

| 플래그 | 설명 |
|--------|------|
| `--workspace <path>` | 워크스페이스 경로 오버라이드 |
| `--verbose`, `-v` | 상세 출력 활성화 |
| `--json` | JSON 포맷 출력 |

---

### 기본 명령

#### `pylon init`

- 현재 디렉토리를 워크스페이스로 초기화
- **필수 도구 검증**: git, gh, claude CLI 설치 여부 확인. 미설치 시 안내 후 중단
- 대화형으로 입력받는 것:
  - 에이전트 백엔드 선택 (MVP: Claude Code)
  - PR reviewer GitHub 사용자명 (선택)
- DB 초기화: `.pylon/pylon.db` 생성 + 마이그레이션 실행
- 기존 프로젝트 자동 발견: `.pylon/` 디렉토리를 가진 서브디렉토리를 `projects` 테이블에 등록
- 결과: `.pylon/` 디렉토리 + `.gitignore` 설정 + git init (이미 있으면 스킵)

#### `pylon add-project [git-url]`

- git submodule로 프로젝트 추가
- **플래그**: `--name <이름>` (기본: URL에서 추론), `--force` (기존 디렉토리 재복제), `--skip-clone` (기존 디렉토리 사용)
- 코드베이스 분석하여 tech stack 감지 + 에이전트 제안
- `.pylon/context.md`, `.pylon/verify.yml` 자동 생성
- SQLite `projects` 테이블에 프로젝트 등록 (stack 정보 포함)
- 결과: submodule 추가 + 프로젝트별 `.pylon/` 생성

#### `pylon doctor`

- 필수 도구 설치 여부 검증 (git, gh, claude CLI)
- 버전 호환성 확인
- 워크스페이스 상태 진단

#### `pylon version`

- 빌드 버전 정보 출력

#### `pylon uninstall`

- 워크스페이스 및 pylon 관련 리소스 완전 제거
- **플래그**: `--force` (확인 생략), `--dry-run` (삭제 예정 항목만 표시), `--remove-projects` (서브모듈도 제거), `--remove-binary` (바이너리도 제거)

---

### 파이프라인 실행

#### `pylon request [requirement]`

- 사람이 자연어로 요구사항 전달
- PO 에이전트와 인터랙션 모드로 요구사항 구체화
- **플래그**: `--continue <pipeline-id>` (PO 대화 재개)
- **동시 request 정책**: PO 대화는 한 번에 하나만 가능. 실행 단계의 태스크는 큐에 쌓여서 리소스 여유 시 순차 실행
- 실행 흐름:
  1. PO 에이전트가 위키 기반으로 요구사항 분석
  2. 불명확한 부분은 역질문 → 사용자와 대화로 구체화
  3. 대화 기록: `.pylon/conversations/{id}/thread.md`
  4. PO가 수용 기준 확정
  5. Architect가 기술 방향성 + 프로젝트 간 의존성 분석
  6. PM이 작업 분해 → 관련 프로젝트/에이전트 지정 (병렬 또는 직렬)
  7. 프로젝트별 에이전트에게 태스크 할당
  8. 에이전트가 브랜치 생성 → 구현
  9. **오케스트레이터가 교차 검증** (빌드/테스트/린트) → 실패 시 에이전트에게 수정 요청
  10. 검증 통과 시 PR 생성 (사람을 reviewer로 지정)
  11. 에러 발생 시 PM 에이전트에게 보고 → PM이 재시도 or 사람 에스컬레이션 판단
  12. PO가 수용 기준 대비 검증
  13. Tech Writer가 위키(도메인 지식) 자동 업데이트
- 결과: `.pylon/tasks/`에 작업 기록 + 각 프로젝트에 PR

#### `pylon status`

- 현재 진행 중인 작업 현황 조회
- 활성 에이전트 프로세스, 태스크 진행 상태, 큐 대기 중인 작업 표시

#### `pylon cancel [pipeline-id]`

- 진행 중인 파이프라인 취소
- 관련 에이전트 프로세스 종료 + 작업 브랜치 정리

#### `pylon resume <conversation-id>`

- 중단된 대화 재개
- **conversation-id**: `conversations/` 하위 디렉토리명 (예: `20260305-143022-user-login`)
- **재개 방식**: Claude Code CLI의 `--resume {session_id}` 활용
  - `meta.yml`에 `session_id` 필드를 저장하여 CLI 세션과 연결
  - CLI가 세션 상태를 자체 복원하므로 thread.md 재주입 불필요
- **전제 조건**: `meta.yml`의 `status`가 `active`인 대화만 재개 가능

#### `pylon review <pr-url>`

- PR의 리뷰 코멘트를 읽고 에이전트가 코드 수정
- 사용자가 명시적으로 실행하여 PR 피드백 루프 처리
- 향후 GitHub webhook 자동 감지로 고도화

---

### 파이프라인 상태 관리

#### `pylon stage`

- 파이프라인 단계 상태 관리 CLI
- `pylon stage list` — 전체 파이프라인 목록 조회
- `pylon stage status --pipeline <id>` — 특정 파이프라인 단계 상태
- `pylon stage transition --pipeline <id> --to <stage>` — 단계 전환

---

### 프로젝트 메모리

#### `pylon mem`

- 프로젝트 메모리 CRUD + BM25 전문 검색
- `pylon mem list --project <name> [--category <cat>]` — 메모리 목록 (카테고리 필터 선택)
- `pylon mem search --project <name> --query "..." [--limit <n>]` — 메모리 검색 (기본 10건)
- `pylon mem store --project <name> --key "..." --content "..." [--category <cat>] [--author <name>] [--confidence <0-1>]` — 메모리 저장

#### `pylon sync-memory`

- 세션 아카이브에서 학습 내용을 프로젝트 메모리로 동기화
- **플래그**:
  - `--from-session` — 세션 종료 시 전체 학습 내용 동기화
  - `--incremental` — 파일 변경 단위 메모리 갱신
  - `--project <name>` — 대상 프로젝트
  - `--agent <name>` — 에이전트 이름 (기본: claude)
  - `--content <text>` — 학습 내용 (생략 시 stdin에서 읽음)
  - `--file <path>` — 변경된 파일 경로 (`--incremental` 시 사용)

#### `pylon sync-projects`

- 프로젝트 목록을 SQLite `projects` 테이블에 강제 동기화
- `config.yml`의 projects 맵 + 파일시스템 `.pylon/` 디렉토리 스캔
- 기존 사용자나 DB 누락 시 수동 갱신 용도

---

### 코드베이스 인덱싱

#### `pylon index`

- 코드베이스를 분석하여 도메인 위키(`.pylon/domain/`) 인덱싱
- `pylon add-project` 이후 프로젝트 컨텍스트 갱신에 사용

---

### 대시보드

#### `pylon dashboard`

- 웹 대시보드 로컬 서버 실행 (Chi v5 + HTMX + SSE)
- **플래그**: `--port <포트>` (기본: config의 `dashboard.port`, 기본값 7777)
- **기술 스택**: Chi v5 라우터, `html/template` + HTMX, SSE 실시간 스트림, `go:embed` 단일 바이너리
- **페이지 구성**:
  - **Overview**: 동시성 게이지, 파이프라인 메트릭스 (전체/활성/완료/실패/성공률), 파이프라인 카드 그리드
  - **Pipeline Detail**: 11단계 스테이지 프로그레스 바, 에이전트 상태 카드, 전환 히스토리 타임라인, Cancel 버튼
  - **Messages**: 메시지 큐 테이블, 에이전트/상태 필터링, 에이전트별 큐 깊이
  - **Memory**: 프로젝트 드롭다운 (DB 기반), 메모리 테이블 + FTS5 검색, 블랙보드 뷰어
- **JSON API**: `/api/overview`, `/api/pipelines`, `/api/pipelines/{id}`, `/api/pipelines/{id}/cancel`, `/api/messages`, `/api/memory`
- **SSE 스트림**: `/api/events` — 1초 SQLite 폴링, 파이프라인/에이전트 변경 실시간 push

---

### 미구현 (향후 추가 예정)

#### `pylon cleanup`

- 좀비 에이전트 프로세스 정리
- 비정상 종료된 에이전트 프로세스 일괄 제거
- Phase 0에서는 `pylon cancel`로 정상 종료 처리

> **⚠️ 구현 시 참고 (스펙 대비 변경사항)**:
> - **파이프라인 상태**: 스펙의 `state.json` 파일 대신 SQLite `pipeline_state` 테이블에서 조회해야 함 (아키텍처 변경, 아래 Section 8 참조)
> - **파이프라인 단계**: 스펙 작성 시 7단계 → 현재 `init` 포함 11단계 (Section 8 참조)
> - **CLI 명령어 목록**: 스펙 원안 11개 → 현재 18개 (`version`, `stage`, `mem`, `sync-memory`, `sync-projects`, `uninstall`, `index` 추가)

## 8. 에이전트 실행 모델

### 아키텍처: Go 오케스트레이터 + 직접 프로세스 실행

```
사용자 ↔ [Go 오케스트레이터] ↔ 프로세스(PO)
                              ↔ 프로세스(PM)
                              ↔ 프로세스(Architect)
                              ↔ 프로세스(Tech Writer)
                              ↔ 프로세스(backend-dev)
                              ↔ 프로세스(frontend-dev)
```

- **Go 오케스트레이터**: 모든 에이전트의 생명주기 관리, 에이전트 간 통신 중재, 상태 감시
- **직접 프로세스 실행**: 각 에이전트를 `syscall.Exec` (인터랙티브) 또는 `exec.Command` (헤드리스)로 실행. git worktree로 파일시스템 격리
- 에이전트의 outbox 결과 파일을 오케스트레이터가 읽어 다음 단계 결정
- 모든 태스크 지시/결과는 `.pylon/tasks/`에 파일로도 기록 (디버깅 + 영속성)

### 프로세스 관리: 직접 실행

- 각 에이전트 = 독립 프로세스 1개
- 인터랙티브 에이전트 (PO): `syscall.Exec`로 현재 프로세스를 대체하여 실행
- 비인터랙티브 에이전트: `exec.Command`로 자식 프로세스 실행, stdout/stderr 캡처
- 디버깅: Claude Code 자체 로그 / `--output-format stream-json`으로 실시간 확인
- 크래시 시 캡처된 출력 보존 → PM이 에러 분석 가능

### 동시 실행

- **PM 자율 판단**: 슬롯 수를 고정하지 않음. PM이 태스크 복잡도와 의존성을 분석하여 필요한 만큼 에이전트를 생성
- **퀄리티 우선**: 병렬 처리 속도보다 작업 품질을 우선. PM이 에이전트 수를 과도하게 늘리지 않도록 판단
- `config.yml`의 `max_concurrent`는 시스템 리소스 보호를 위한 상한선일 뿐, 목표치가 아님
- 역할 예시: 프론트엔드, 백엔드, 데브옵스, QA

### 실행 흐름

```
[pylon request "로그인 기능 구현해줘"]
    ↓
[Go 오케스트레이터] → 에이전트 프로세스 생성
    ↓
[프로세스: PO] ← 위키 + 요구사항
    ↓ "소셜 로그인도 포함? 세션 방식은?" (역질문)
    ↑ 사용자 응답 (TUI/Dashboard 인터랙션)
    ↓ 요구사항 + 수용 기준 확정
    ↓
[Go 오케스트레이터] → conversations/ 기록 + tasks/ 생성
    ↓
[프로세스: Architect] ← 기술 방향성 + 의존성 분석
    ↓
[프로세스: PM] ← 태스크 분해 + 에이전트 지정 + 직렬/병렬 결정
    ↓
[프로세스: api-backend-dev] → task/20260305-user-login 브랜치 → 구현
[프로세스: web-frontend-dev] → (의존성 있으면 대기 후) 브랜치 → 구현
    ↓ (에이전트 완료 시)
[Go 오케스트레이터] → 교차 검증 (빌드/테스트/린트)
    ↓ (검증 실패 시) → 에이전트에게 수정 요청
    ↓ (검증 통과 시) → PR 생성
    ↓ (에러 시)
[프로세스: PM] ← 에러 분석 → 재시도 or 사람 에스컬레이션
    ↓ (완료 시)
[프로세스: PO] ← 수용 기준 대비 검증
    ↓
[프로세스: Tech Writer] ← 위키 + context.md 자동 업데이트
```

### Claude Code CLI 실행 명세

오케스트레이터가 각 에이전트를 실행할 때 다음 명세를 따른다.

#### 기본 실행 형식 (비인터랙티브 에이전트)

```bash
claude \
  --print \                                    # 비인터랙티브 출력
  --output-format stream-json \                # 구조화된 스트림 출력
  --max-turns {maxTurns} \                     # 턴 수 제한 (안전장치)
  --permission-mode {permissionMode} \         # 권한 모드
  --model {model} \                            # 모델 지정 (선택)
  --append-system-prompt "$(cat claude.md)" \  # 통신 규칙 + 컨텍스트 주입
  --prompt "{task_prompt}"                     # 태스크 프롬프트 (아래 조립 규칙 참조)
```

#### task_prompt 조립 규칙

오케스트레이터가 에이전트 실행 시 `--prompt`에 넣는 내용은 **짧은 지시**만 포함한다. 태스크 상세는 inbox 파일에 위임한다.

**--prompt 내용 (3~5줄)**:
```
당신은 {role}입니다.
inbox 파일을 읽고 태스크를 수행하세요.
inbox: .pylon/runtime/inbox/{agent-name}/{task-id}.task.json
완료 후 outbox에 결과를 작성하세요.
```

- **inbox 파일**: 기존 MessageEnvelope (`body` + `context`) 그대로 사용
- **CLAUDE.md**: 통신 규칙 상세 + 컨벤션 (200줄 제한 유지, `--append-system-prompt`로 주입)

#### 인터랙티브 에이전트 (PO)

PO 에이전트는 사용자와 대화해야 하므로 `--print` 없이 인터랙티브 모드로 실행:

```bash
claude \
  --max-turns {maxTurns} \
  --permission-mode default \
  --append-system-prompt "$(cat claude.md)"
```

#### 환경변수 설정

오케스트레이터가 에이전트 프로세스 생성 시 다음 환경변수를 설정한다. `config.yml`의 `runtime.env`를 기본값으로, 에이전트 frontmatter의 `env`로 override한다.

```bash
export CLAUDE_AUTOCOMPACT_PCT_OVERRIDE=80     # 자동 컴팩트 임계값
export CLAUDE_CODE_EFFORT_LEVEL=high          # 사고 깊이 (기본 high)
export CLAUDE_CODE_MAX_TURNS={maxTurns}       # 최대 턴 수 (이중 안전장치)
```

#### Git Worktree 격리

에이전트가 같은 프로젝트에서 병렬로 코드를 수정할 때, git 충돌을 물리적으로 방지하기 위해 worktree 격리를 사용한다.

```
에이전트 태스크 할당 시 (isolation: worktree):
1. git worktree add {project}/.git/pylon-worktrees/{agent}-{task} -b {branch}
2. 에이전트 프로세스의 작업 디렉토리를 worktree 경로로 설정
3. 에이전트 실행 (worktree 내에서 독립 작업)
4. 태스크 완료 후:
   a. 변경사항 커밋 + push
   b. git worktree remove (auto_cleanup=true 시)
```

**isolation 모드**:

| 모드 | 설명 | 사용 시나리오 |
|------|------|-------------|
| `worktree` (기본) | 에이전트별 독립 git worktree 생성 | 같은 프로젝트의 병렬 에이전트 |
| `none` | worktree 없이 프로젝트 디렉토리에서 직접 작업 | 단일 에이전트 또는 루트 에이전트 |

**핵심**: 직접 프로세스 실행이 프로세스 격리를, git worktree가 파일시스템 격리를 담당한다. 두 격리가 결합되어 에이전트 간 간섭을 완전히 방지한다.

### 하이브리드 통신 프로토콜

에이전트와 오케스트레이터 간 통신은 **하이브리드 방식**을 채택한다.

**핵심 원칙**: 에이전트는 파일만 읽고 쓴다. 오케스트레이터가 파일을 수집하여 내부 SQLite에 기록하고, 상태 관리/쿼리/히스토리를 담당한다.

```
에이전트 (Claude Code CLI)           Go 오케스트레이터
─────────────────────              ─────────────────────
outbox/{agent}/{id}.result.json →  fsnotify 감지 → SQLite INSERT
                                   → 다음 단계 결정
inbox/{agent}/{id}.task.json    ←  SQLite 조회 → 파일 생성
```

#### 통신 디렉토리 구조

```
.pylon/runtime/
├── inbox/{agent-name}/                ← 오케스트레이터 → 에이전트 (태스크 할당)
│   └── {task-id}.task.json
├── outbox/{agent-name}/               ← 에이전트 → 오케스트레이터 (결과 보고)
│   └── {task-id}.result.json
├── state.json                         ← ⚠️ 구현에서 제거됨 (아래 참고)
└── pylon.db                           ← SQLite (오케스트레이터 전용)

> **⚠️ state.json → SQLite 전환**: 구현 과정에서 `state.json` 파일은 제거되고, 파이프라인 상태가 `pylon.db`의 `pipeline_state` 테이블로 통합되었다. ACID 보장과 원자적 갱신을 위한 아키텍처 결정. 대시보드 등 상태 조회는 SQLite 쿼리를 사용해야 한다.
```

#### 메시지 스키마

모든 메시지는 공통 봉투(MessageEnvelope) 구조를 따른다. 에이전트가 읽고 쓰는 파일도 이 스키마를 따르되, 에이전트는 `context`와 `trace` 등 오케스트레이터 전용 필드를 무시해도 된다.

```
MessageEnvelope
├── id          : UUID v7 (시간순 정렬 가능)
├── type        : task_assign | result | query | query_result | broadcast | heartbeat
├── priority    : critical | high | normal | low
├── from        : 발신 에이전트 (또는 "orchestrator")
├── to          : 수신 에이전트 (또는 "*" = 브로드캐스트)
├── reply_to    : 원본 메시지 ID (응답 시)
├── subject     : 메시지 제목/요약
├── body        : 타입별 구조화된 본문 (JSON)
├── context     : 관련 컨텍스트 (선택)
│   ├── task_id, project_id
│   ├── references      : 관련 파일/문서 경로
│   ├── summary         : 이전 대화/결정 요약
│   ├── decisions       : 관련 결정 이력
│   └── constraints     : 제약 조건
├── ttl         : 메시지 유효 기간 (선택)
├── trace       : 메시지 전달 경로 추적
└── timestamp   : ISO 8601
```

#### Inbox 메시지 포맷 (오케스트레이터 → 에이전트)

```json
{
  "id": "01961a2b-3c4d-7e8f-9012-abcdef123456",
  "type": "task_assign",
  "priority": "normal",
  "from": "orchestrator",
  "to": "backend-dev",
  "subject": "JWT 기반 로그인 API 구현",
  "body": {
    "task_id": "20260305-user-login",
    "description": "JWT 기반 로그인 API 구현",
    "branch": "task/20260305-user-login",
    "acceptance_criteria": [
      "POST /auth/login 엔드포인트 구현",
      "JWT 토큰 발급 및 검증",
      "기존 테스트 통과"
    ],
    "context_files": [".pylon/tasks/20260305-user-login.md"]
  },
  "context": {
    "task_id": "20260305-user-login",
    "project_id": "project-api",
    "summary": "PO 확정: 이메일+카카오 로그인, JWT 세션. Architect: Echo 미들웨어 기반 인증 레이어 권장",
    "decisions": ["JWT 기반 인증", "카카오 OAuth 2.0", "Echo 미들웨어 구조"],
    "references": [".pylon/domain/architecture.md", ".pylon/domain/conventions.md"]
  },
  "ttl": "30m",
  "timestamp": "2026-03-05T14:35:00Z"
}
```

#### Outbox 메시지 포맷 (에이전트 → 오케스트레이터)

에이전트가 작성하는 결과 파일. 에이전트는 최소 필수 필드만 작성하면 된다.

```json
{
  "id": "01961a2b-4d5e-7f00-1234-fedcba654321",
  "type": "result",
  "from": "backend-dev",
  "to": "orchestrator",
  "reply_to": "01961a2b-3c4d-7e8f-9012-abcdef123456",
  "subject": "JWT 로그인 API 구현 완료",
  "body": {
    "task_id": "20260305-user-login",
    "status": "completed",
    "files_changed": ["internal/auth/handler.go", "internal/auth/service.go"],
    "commits": ["abc1234"],
    "summary": "JWT 로그인 API 구현 완료. Echo 미들웨어 기반 인증 레이어 추가.",
    "learnings": ["sqlc에서 nullable 필드 처리 시 sql.NullString 필요"]
  },
  "timestamp": "2026-03-05T15:12:00Z"
}
```

`body.status` 값: `completed` | `failed` | `blocked`

에이전트의 `body.learnings` 필드는 선택적이며, 작성 시 프로젝트 메모리에 자동 축적된다.

#### ~~State.json 포맷~~ → SQLite `pipeline_state` 테이블 (아키텍처 변경)

> **⚠️ 변경**: 아래 state.json 파일 포맷은 초기 설계안이며, 구현에서는 SQLite `pipeline_state` 테이블로 통합되었다. `state_json` 컬럼에 동일한 구조의 JSON이 저장된다.

```sql
-- 실제 구현 스키마 (migrations/001_initial.sql)
CREATE TABLE IF NOT EXISTS pipeline_state (
    pipeline_id TEXT PRIMARY KEY,
    stage       TEXT NOT NULL,
    state_json  TEXT NOT NULL,  -- 아래 JSON 구조 저장
    updated_at  DATETIME NOT NULL
);
```

`state_json` 컬럼에 저장되는 JSON 구조 (참고용):

```json
{
  "pipeline_id": "20260305-user-login",
  "current_stage": "agent_executing",
  "stage_history": [
    {"stage": "po_conversation", "completed_at": "2026-03-05T14:32:00Z"},
    {"stage": "architect_analysis", "completed_at": "2026-03-05T14:34:00Z"},
    {"stage": "pm_task_breakdown", "completed_at": "2026-03-05T14:35:00Z"}
  ],
  "active_agents": {
    "backend-dev": {
      "task_id": "20260305-user-login",
      "pid": 12345,
      "status": "running"
    }
  }
}
```

#### 원자적 파일 쓰기 (Atomic Write)

에이전트가 파일을 쓸 때 POSIX `rename(2)` 원자성을 보장하기 위해 tmp → mv 패턴을 사용한다.

```
1. 결과를 {task-id}.tmp.json 에 작성
2. mv {task-id}.tmp.json → {task-id}.result.json
```

오케스트레이터는 `fsnotify`로 `.result.json` 파일 생성 이벤트만 감지하므로, 쓰기 중간 상태의 파일을 읽지 않는다.

#### 에이전트 CLAUDE.md 주입 규칙

각 에이전트 프로세스 실행 시, 오케스트레이터가 `--append-system-prompt`로 규칙을 주입한다.

**크기 제한**: 주입되는 CLAUDE.md는 **200줄 이하**를 유지한다. 200줄을 초과하면 에이전트의 규칙 준수율이 저하된다.

**주입 내용 우선순위** (상위일수록 필수):

| 우선순위 | 내용 | 필수 여부 |
|---------|------|----------|
| 1 | 통신 규칙 (inbox/outbox 절차) | 필수 |
| 2 | 태스크 컨텍스트 (수용 기준, 제약 조건) | 필수 |
| 3 | 컨텍스트 관리 규칙 (Compaction) | 필수 |
| 4 | 프로젝트 메모리 요약 (선제적 주입) | proactive_max_tokens 이내 |
| 5 | 도메인 지식 참조 경로 | 선택 |

**핵심 원칙**: 도메인 지식 본문은 CLAUDE.md에 직접 포함하지 않고, 파일 경로만 안내하여 에이전트가 필요 시 읽도록 유도한다.

**통신 규칙 예시**:

```markdown
## Pylon 통신 규칙
태스크를 완료하면 반드시 아래 절차를 따르세요:
1. `.pylon/runtime/inbox/{에이전트명}/` 에서 태스크 파일을 읽어 작업을 수행합니다
2. 작업 완료 후 결과를 `.pylon/runtime/outbox/{에이전트명}/{task-id}.tmp.json`에 JSON으로 작성합니다
3. 작성 완료 후 mv 명령을 실행합니다:
   mv .pylon/runtime/outbox/{에이전트명}/{task-id}.tmp.json \
      .pylon/runtime/outbox/{에이전트명}/{task-id}.result.json
4. 절대로 SQLite(pylon.db)에 직접 접근하지 마세요
```

#### 오케스트레이터 내부 SQLite

오케스트레이터만 접근하는 내부 DB. 에이전트는 SQLite에 절대 접근하지 않는다.

**용도**:
- 통신 히스토리 누적 (inbox/outbox 메시지 전체 기록)
- 태스크 상태 쿼리 (`SELECT * FROM tasks WHERE status = 'running'`)
- 에이전트 성과 통계 (완료율, 평균 소요시간)
- Dashboard/TUI에 구조화된 데이터 제공

**테이블 구조**:

```sql
-- ─── 메시지 큐 (ack 기반 전달 보장) ────────────────
CREATE TABLE message_queue (
    id          TEXT PRIMARY KEY,          -- UUID v7
    type        TEXT NOT NULL,             -- task_assign, result, query, broadcast, heartbeat
    priority    INTEGER DEFAULT 2,         -- 0=critical, 1=high, 2=normal, 3=low
    from_agent  TEXT NOT NULL,
    to_agent    TEXT NOT NULL,
    subject     TEXT,
    body        TEXT NOT NULL,             -- JSON
    context     TEXT,                      -- JSON (컨텍스트)
    status      TEXT DEFAULT 'queued',     -- queued → delivered → acked → (expired | failed)
    reply_to    TEXT,
    ttl_seconds INTEGER,
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    delivered_at DATETIME,
    acked_at    DATETIME
);

CREATE INDEX idx_mq_to_status ON message_queue(to_agent, status);
CREATE INDEX idx_mq_priority ON message_queue(priority, created_at);

-- ─── 파이프라인 상태 ───────────────────────────────
CREATE TABLE pipeline_state (
    pipeline_id TEXT PRIMARY KEY,
    stage       TEXT NOT NULL,
    state_json  TEXT NOT NULL,
    updated_at  DATETIME NOT NULL
);

-- ─── 블랙보드 (프로젝트 공유 지식) ─────────────────
CREATE TABLE blackboard (
    id          TEXT PRIMARY KEY,
    project_id  TEXT NOT NULL,
    category    TEXT NOT NULL,             -- hypothesis, evidence, decision, constraint, result
    key         TEXT NOT NULL,
    value       TEXT,                      -- JSON
    confidence  REAL DEFAULT 0.5,          -- 0.0 ~ 1.0
    author      TEXT NOT NULL,             -- 작성 에이전트
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME,
    superseded_by TEXT,                    -- 이후 업데이트된 항목 ID
    UNIQUE(project_id, category, key)
);

-- ─── 토픽 구독 ─────────────────────────────────────
CREATE TABLE topic_subscriptions (
    agent_id    TEXT NOT NULL,
    topic       TEXT NOT NULL,             -- e.g., "task.completed", "decision.architecture"
    filter      TEXT,                      -- 선택적 필터 (JSON)
    PRIMARY KEY (agent_id, topic)
);

-- ─── 프로젝트 메모리 (장기 지식) ────────────────────
CREATE TABLE project_memory (
    id          TEXT PRIMARY KEY,
    project_id  TEXT NOT NULL,
    category    TEXT NOT NULL,             -- architecture, pattern, decision, learning, codebase
    key         TEXT NOT NULL,
    content     TEXT NOT NULL,
    metadata    TEXT,                      -- JSON
    author      TEXT,
    confidence  REAL DEFAULT 0.8,
    access_count INTEGER DEFAULT 0,
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME,
    expires_at  DATETIME
);

-- BM25 풀텍스트 검색 인덱스
CREATE VIRTUAL TABLE project_memory_fts USING fts5(
    key, content, category,
    content='project_memory',
    content_rowid='rowid'
);

CREATE INDEX idx_pm_project ON project_memory(project_id, category);

-- ─── 세션 아카이브 ──────────────────────────────────
CREATE TABLE session_archive (
    id          TEXT PRIMARY KEY,
    agent_name  TEXT NOT NULL,
    task_id     TEXT NOT NULL,
    summary     TEXT NOT NULL,             -- 압축된 세션 요약
    raw_path    TEXT,                      -- 원본 파일 경로 (runtime/sessions/)
    token_count INTEGER,
    created_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

**데이터 흐름**:
```
에이전트가 outbox에 .result.json 작성
    ↓
오케스트레이터 fsnotify 감지
    ↓
파일 읽기 → SQLite INSERT (message_queue, status='delivered')
    ↓
learnings 필드 존재 시 → project_memory INSERT
    ↓
pipeline_state UPDATE (⚠️ state.json은 제거됨, SQLite로 통합)
    ↓
토픽 매칭 → 구독 에이전트에게 알림
    ↓
블랙보드 갱신 (결정/결과 기록)
    ↓
다음 단계 결정 → inbox에 새 .task.json 생성 (컨텍스트 + 관련 메모리 주입)
```

#### SPOF 복구 (오케스트레이터 장애 복구)

오케스트레이터가 비정상 종료되더라도, 통신 히스토리와 에이전트 프로세스 상태를 기반으로 파이프라인을 복구할 수 있다.

**복구 알고리즘**:

```
1. pipeline_state 테이블 조회 → 마지막 파이프라인 상태 확인 (⚠️ state.json → SQLite 전환)
2. 에이전트 프로세스 생존 여부 확인 (PID 기반)
3. runtime/outbox/ 스캔 → 오케스트레이터 종료 중 도착한 결과 수집
4. SQLite 히스토리와 교차 검증 → 누락된 메시지 복원
5. 각 에이전트의 현재 상태 판단:
   - outbox에 result 있음 → 완료 처리 진행
   - 프로세스 살아있음 + result 없음 → 작업 진행 중, 감시 재개
   - 프로세스 없음 + result 없음 → 비정상 종료, PM에게 에스컬레이션
6. 파이프라인 재개
```

**핵심**: 에이전트 프로세스의 상태와 outbox 결과 파일을 교차 검증하여, 오케스트레이터 크래시 이후에도 파이프라인을 안전하게 복구한다.

#### 블랙보드 (프로젝트 공유 지식 저장소)

블랙보드는 에이전트들이 직접 통신하지 않고도 비동기적으로 지식을 공유하는 중앙 저장소다.

```
┌─────────────────────────────────────────────────┐
│              블랙보드 (pylon.db)                   │
│  ┌──────────┬──────────┬──────────┬──────────┐  │
│  │ decision │ evidence │ result   │constraint│  │
│  │(아키텍처 │(분석결과)│(검증완료)│(제약조건)│  │
│  │ 결정)    │          │          │          │  │
│  └──────────┴──────────┴──────────┴──────────┘  │
└───────┬──────────┬──────────┬──────────┬────────┘
        │          │          │          │
   ┌────▼───┐ ┌───▼────┐ ┌──▼─────┐ ┌──▼─────┐
   │   PO   │ │Architect│ │backend │ │   QA   │
   │(읽기)  │ │(읽기/쓰기)│(읽기)  │ │(읽기/쓰기)│
   └────────┘ └────────┘ └────────┘ └────────┘
```

**사용 시나리오**:
- Architect가 기술 결정을 블랙보드에 기록 → 프로젝트 에이전트가 자동으로 인식
- QA 에이전트가 발견한 버그를 게시 → 관련 개발 에이전트에게 알림
- PM이 우선순위 변경을 반영 → 모든 에이전트에게 전파

**에이전트의 블랙보드 접근**: 에이전트는 블랙보드에 직접 접근하지 않는다. 오케스트레이터가 태스크 할당 시 관련 블랙보드 항목을 inbox 메시지의 `context`에 포함하여 전달하고, 에이전트의 outbox 결과에서 결정/발견 사항을 추출하여 블랙보드에 기록한다.

#### 토픽 기반 구독 (Pub/Sub)

오케스트레이터가 관리하는 토픽 라우터. 에이전트가 관심 토픽을 구독하면, 해당 이벤트 발생 시 오케스트레이터가 자동으로 알림을 전달한다.

**토픽 계층**:
```
task.*                    -- 모든 태스크 이벤트
task.assigned             -- 태스크 할당
task.completed            -- 태스크 완료
task.failed               -- 태스크 실패
decision.*                -- 모든 결정
decision.architecture     -- 아키텍처 결정
decision.requirement      -- 요구사항 결정
bug.*                     -- 버그 관련
code.review.requested     -- 코드 리뷰 요청
wiki.updated              -- 위키 갱신
```

**기본 구독**: 에이전트 역할에 따라 자동 구독 설정
- **PM**: `task.*`, `bug.critical`, `decision.*`
- **Architect**: `decision.architecture`, `task.completed`
- **Tech Writer**: `task.completed`, `wiki.*`, `decision.*`
- **프로젝트 에이전트**: `task.assigned.{자기이름}`, `decision.architecture`

**동작 방식**: 이벤트 발생 → 오케스트레이터가 구독 테이블 조회 → 매칭되는 에이전트의 inbox에 알림 메시지 생성. 에이전트는 파일 기반 통신만 사용하므로 Pub/Sub 메커니즘을 알 필요 없다.

#### 핸드오프 프로토콜 (Narrative Casting)

에이전트 간 태스크 위임 시, 이전 에이전트의 작업 컨텍스트를 서사적으로 재구성하여 전달한다. 단순 데이터 전달이 아닌, 새 에이전트가 즉시 이해할 수 있는 "이야기" 형태로 변환한다.

```
에이전트 A (Architect) 완료 → outbox에 결과 작성
    ↓
오케스트레이터: Narrative Casting 수행
    ├── A의 결과에서 핵심 결정 사항 추출
    ├── 관련 블랙보드 항목 수집
    ├── 프로젝트 메모리에서 관련 지식 검색
    └── 서사적 컨텍스트(narrative_context) 생성
    ↓
에이전트 B (backend-dev)의 inbox 메시지:
    ├── body: 태스크 상세
    └── context:
        ├── summary: "Architect가 Echo 미들웨어 기반 인증 레이어를 결정함..."
        ├── decisions: ["JWT 방식", "미들웨어 패턴"]
        ├── references: [관련 파일 목록]
        └── constraints: ["기존 API 호환성 유지"]
```

**핵심 원칙**: 이전 에이전트의 원시 로그나 전체 대화를 전달하지 않는다. 오케스트레이터가 핵심만 추출하여 압축된 서사적 맥락으로 변환한다. 이를 통해 새 에이전트의 컨텍스트 윈도우를 절약하면서도 충분한 맥락을 제공한다.

### 코드 품질 교차 검증

에이전트가 코드 작성을 완료하면, Go 오케스트레이터가 프로젝트의 `.pylon/verify.yml`에 정의된 검증 기준에 따라 교차 검증을 수행한다.

```
에이전트 코드 작성 완료
    ↓
[Go 오케스트레이터] 교차 검증 실행
    ├── 빌드 성공 확인
    ├── 기존 테스트 통과 확인
    └── 린트/포맷 검사
    ↓ (실패 시)
    에이전트에게 검증 결과 전달 → 수정 요청 → 재검증
    ↓ (성공 시)
    PR 생성 진행
```

- 검증 명령어는 프로젝트별 `.pylon/verify.yml`에 정의
- 검증 실패 시 에이전트에게 실패 로그를 전달하여 수정 기회 부여
- 검증도 `max_attempts` 횟수에 포함

#### verify.yml 스키마

```yaml
# .pylon/verify.yml
build:
  command: "go build ./..."
  timeout: 120s           # 선택, 기본 60s

test:
  command: "go test ./..."
  timeout: 300s

lint:
  command: "golangci-lint run"
```

| 필드 | 필수 | 기본값 | 설명 |
|------|------|--------|------|
| `{category}.command` | 필수 | - | 실행할 검증 명령어 |
| `{category}.timeout` | 선택 | `60s` | 명령어 타임아웃 |

- **카테고리**: `build`, `test`, `lint`를 최상위 키로 사용
- **실행 순서**: build → test → lint (위에서 아래로 순차 실행)
- **실패 정책**: 하나라도 실패 시 전체 실패 → 에이전트에게 수정 요청

### 에러 처리

- 프로젝트 에이전트 실패 → PM 에이전트에게 보고
- PM 에이전트가 에러 분석:
  - 자동 재시도 (최대 `max_attempts`회, 기본 2회 — 최초 1회 + 재시도 1회)
  - 모든 시도 실패 시 자동으로 사람에게 에스컬레이션

### 도메인 지식 업데이트

**2계층 분리 원칙**:
- **사람 소유 (AI read-only)**: `.specify/memory/constitution.md` — 프로젝트 헌법, 개발 원칙. 에이전트는 읽기만 가능
- **AI 소유 (자동 생성/갱신)**: `.pylon/domain/*` — 학습된 컨벤션, 아키텍처 결정, 용어 사전. 사용자가 직접 수정하지 않음. 수정이 필요하면 루트 에이전트에게 요청

- **갱신 담당**: Tech Writer 에이전트 (`.pylon/domain/*`와 `context.md` 갱신. `constitution.md`는 수정 불가)
- **갱신 트리거**: `config.yml`의 `wiki.update_on`으로 설정 (`task_complete`, `pr_merged` 중 선택)
- 변경된 코드를 분석하여 domain/ 파일 및 프로젝트 context.md 갱신

### 에이전트 메모리 아키텍처

에이전트의 컨텍스트와 지식을 체계적으로 관리하기 위한 3계층 메모리 모델.

#### 3계층 메모리 모델

```
┌──────────────────────────────────────────────────────┐
│        Layer 1: Working Context (작업 메모리)           │
│  = 에이전트의 현재 Claude Code 세션 컨텍스트 윈도우      │
│  = 매 LLM 호출마다 재구성                              │
│  = 관리: 에이전트 자체 (Claude Code 내장)               │
│  = 수명: 단일 LLM 호출                                 │
├──────────────────────────────────────────────────────┤
│        Layer 2: Session Memory (세션 메모리)            │
│  = 현재 태스크 실행 동안의 대화/결정 이력                │
│  = Compaction 전략으로 압축 관리                        │
│  = 관리: 오케스트레이터 + 에이전트 협력                  │
│  = 수명: 태스크 완료까지                                │
├──────────────────────────────────────────────────────┤
│        Layer 3: Project Memory (프로젝트 메모리)         │
│  = 프로젝트 수준의 장기 지식 저장소                      │
│  = 아키텍처 결정, 학습된 패턴, 코드베이스 이해           │
│  = 관리: 오케스트레이터 (SQLite + BM25 인덱스)          │
│  = 수명: 프로젝트 전체                                  │
└──────────────────────────────────────────────────────┘
```

**Layer 1 (Working Context)**: Claude Code의 내장 컨텍스트 윈도우. Pylon이 직접 관리하지 않으나, Compaction 트리거 시 오케스트레이터가 개입한다.

**Layer 2 (Session Memory)**: 에이전트가 현재 태스크를 수행하면서 축적하는 중기 기억. 태스크 완료 시 핵심 내용이 Layer 3으로 승격되고, 나머지는 세션 아카이브에 저장된다.

**Layer 3 (Project Memory)**: 프로젝트 수준에서 영속적으로 유지되는 장기 지식. 새 태스크 시작 시 관련 메모리를 검색하여 에이전트에게 선제적으로 주입한다.

#### Compaction 전략 (컨텍스트 압축)

에이전트의 컨텍스트 윈도우 관리는 **에이전트 주도 방식**을 채택한다. 오케스트레이터가 외부에서 컨텍스트 사용량을 모니터링하지 않으며, 에이전트 자체의 내장 기능(Claude Code의 auto-compact)과 CLAUDE.md 규칙 주입을 통해 처리한다.

**에이전트 CLAUDE.md에 주입되는 Compaction 규칙**:
```markdown
## 컨텍스트 관리 규칙
- 작업 중 컨텍스트가 부족해지면 Claude Code의 내장 compact 기능을 활용하세요
- compact 시 반드시 보존할 항목:
  - 현재 태스크 목표 및 수용 기준
  - 미해결 이슈/버그
  - 내린 아키텍처 결정 사항
- 태스크 완료 시 결과 파일(outbox)에 learnings 필드로 핵심 교훈을 기록하세요
```

**오케스트레이터의 역할**:
- Compaction 자체에 개입하지 않음
- 대신, 태스크를 적절한 크기로 분해하여 단일 세션이 과도하게 길어지지 않도록 PM이 관리
- 에이전트 완료 후 outbox의 `learnings` 필드를 프로젝트 메모리에 축적하여 간접적으로 지식 보존

#### 프로젝트 메모리

프로젝트 수준의 장기 지식을 SQLite + BM25 풀텍스트 검색으로 관리한다.

**메모리 카테고리**:

| 카테고리 | 설명 | 예시 |
|---------|------|------|
| `architecture` | 아키텍처 결정 및 근거 | "REST API 대신 gRPC 선택 근거: 마이크로서비스 간 저지연 필요" |
| `pattern` | 코드 패턴 및 컨벤션 | "에러 핸들링은 sentinel error 패턴 사용" |
| `decision` | 기술/비즈니스 결정 | "PostgreSQL 선택: JSON 컬럼 지원 + 팀 경험" |
| `learning` | 실패/성공에서 학습한 교훈 | "sqlc에서 nullable 필드는 sql.NullString 필요" |
| `codebase` | 코드베이스 구조 이해 | "internal/auth/는 인증 도메인, handler→service→repo 계층" |

**메모리 축적 방식**:
1. **자동 추출**: 에이전트의 outbox 결과에서 `learnings` 필드를 project_memory에 저장
2. **블랙보드 승격**: 블랙보드의 검증된 결정(confidence ≥ 0.8)을 project_memory로 이동
3. **Tech Writer 갱신**: 도메인 위키 갱신 시 관련 메모리도 함께 업데이트

#### 선제적 + 반응적 메모리 접근

**선제적 접근 (Proactive)**: 태스크 할당 시 오케스트레이터가 자동으로 관련 메모리를 검색하여 inbox 메시지의 `context`에 주입한다.

```
새 태스크 할당 시:
1. 태스크 설명에서 키워드 추출
2. project_memory_fts에서 BM25 검색
3. 관련성 높은 항목 (상위 N개, 토큰 예산 내) 선별
4. inbox 메시지의 context.references와 context.summary에 포함
```

**반응적 접근 (Reactive)**: 에이전트가 작업 중 추가 정보가 필요할 때, outbox에 `query` 타입 메시지를 작성하면 오케스트레이터가 검색 결과를 inbox으로 응답한다.

```json
// 에이전트가 메모리 검색 요청
{
  "type": "query",
  "from": "backend-dev",
  "to": "orchestrator",
  "body": {
    "query": "인증 관련 아키텍처 결정",
    "categories": ["architecture", "decision"]
  }
}

// 오케스트레이터가 검색 결과 응답
{
  "type": "query_result",
  "from": "orchestrator",
  "to": "backend-dev",
  "body": {
    "results": [
      {"key": "auth-architecture", "content": "JWT + 미들웨어 패턴...", "confidence": 0.9}
    ]
  }
}
```

#### 세션 자동 아카이빙

에이전트 세션 종료 시 자동으로 아카이빙하여 프로젝트 메모리를 축적한다.

**아카이브 트리거**: 태스크 완료 시 / 에이전트 종료 시 / Compaction 발생 시

**프로세스**:
```
에이전트 세션 종료
    ↓
오케스트레이터가 세션 내용 수집 (에이전트 출력 캡처 또는 outbox 기록)
    ↓
핵심 정보 추출:
    ├── 내린 결정들 → project_memory (decision)
    ├── 발견한 패턴 → project_memory (pattern)
    ├── 학습한 교훈 → project_memory (learning)
    └── 코드베이스 이해 → project_memory (codebase)
    ↓
세션 요약 생성 → session_archive 테이블 저장
    ↓
원본 데이터 → runtime/sessions/{agent}-{task-id}.jsonl
```

**BM25 검색 연동**: 아카이브된 세션의 핵심 정보가 `project_memory_fts`에 자동 인덱싱되어, 향후 유사 태스크에서 선제적으로 검색된다.

#### 향후 확장 방향

- **벡터 임베딩 시맨틱 검색**: BM25 풀텍스트 검색을 보완하는 벡터 기반 유사도 검색. 로컬 임베딩 모델 또는 API 연동. 구조화된 노트의 80%는 BM25로 충분하며, 비정형 지식 검색 시 벡터 검색으로 보완하는 하이브리드 방식 (참고: QMD 프로젝트의 하이브리드 검색 전략)

### speckit 연동

Pylon은 speckit을 내장하며, `pylon add-project` 시 `specify init`이 자동 실행되어 모든 프로젝트에 `.specify/` 존재가 보장된다.

#### speckit 산출물 소비 방식

에이전트는 speckit 산출물(spec.md, plan.md, tasks.md, contracts/ 등)을 **직접 읽어서** 네이티브로 이해한다. 별도의 파싱/변환 레이어를 두지 않는다.

- **오케스트레이터 역할**: inbox 메시지에 speckit 산출물 **파일 경로만** 전달
- **에이전트 역할**: Read 도구로 마크다운/YAML을 직접 읽고 해석
- **[P] 마커 처리**: PM 에이전트가 tasks.md를 읽고 스케줄링 계획(JSON)을 오케스트레이터에 보고. 오케스트레이터는 PM의 계획대로 에이전트 디스패치

```
오케스트레이터:
  inbox 메시지에 speckit 파일 경로 포함
    → { "context": { "references": [".specify/specs/001/spec.md", ...] } }

에이전트:
  Read 도구로 직접 읽기
    → spec.md, plan.md, tasks.md, contracts/*.yml 을 네이티브 이해

PM 에이전트 (tasks.md 소비):
  tasks.md 읽기 → [P] 마커 파싱 → 스케줄링 계획 JSON 작성 → outbox 보고
    → 오케스트레이터가 PM의 계획대로 에이전트 디스패치
```

#### Constitution 검증 흐름

speckit의 `constitution.md`(프로젝트 헌법)에 대한 준수 여부를 **별도 검증 에이전트 (reviewer)**가 독립적/객관적으로 검증한다.

**검증 주체**: reviewer 에이전트 — 산출물을 작성한 에이전트와 분리하여 객관성 확보, 결과 추적 가능

**트리거 조건**: reviewer는 **speckit 산출물(spec.md, plan.md, tasks.md, contracts/, data-model.md)을 생성하거나 수정하는 에이전트의 작업 완료 시** 호출된다. 구체적으로:
- **Architect**가 plan.md, contracts/, data-model.md를 작성/수정한 후
- **Developer**가 contracts/를 제한적으로 수정한 후
- **PO**가 spec.md 변경을 제안한 후 (사람 승인 전 사전 검증)

일반 코드 구현(소스 파일 수정)에 대해서는 reviewer가 호출되지 않으며, verify.yml의 빌드/테스트/린트 검증이 대신 수행된다.

**실패 정책**: 재시도 1회 → 사람 에스컬레이션

```
에이전트가 speckit 산출물 작성/수정 완료
    ↓
reviewer 에이전트: constitution.md 대비 검증
    ↓ (통과)
    다음 단계 진행
    ↓ (실패)
    위반 항목 + 사유를 원래 에이전트에 전달
    ↓
원래 에이전트: 수정 후 재제출
    ↓
reviewer 에이전트: 재검증 (1회)
    ↓ (통과)
    다음 단계 진행
    ↓ (재검증 실패)
    파이프라인 일시정지 + 사람에게 알림 (에스컬레이션)
```

#### 셸 step 에러 처리

`pipeline.yml`의 `type: shell` step에 대한 에러 처리는 **즉시 중단 (halt)**을 기본 동작으로 하되, step별 `on_error` 속성으로 override할 수 있다.

| on_error 값 | 동작 | 비고 |
|-------------|------|------|
| `halt` (기본) | 즉시 중단 | `set -e` 스타일. 별도 지정 없으면 기본 |
| `retry` | 재시도 | `max_retries` 필수, `timeout` 선택 |
| `continue` | 경고만 남기고 계속 | 비필수 step에 사용 |
| `escalate` | 사람에게 알림 + 일시정지 | 사람 판단이 필요한 경우 |

```yaml
# pipeline.yml 셸 step 에러 처리 예시
stages:
  - name: check_prerequisites
    type: shell
    command: ".specify/scripts/bash/check-prerequisites.sh {{feature_number}}"
    output: json
    on_error: retry
    max_retries: 2
    timeout: 300s

  - name: lint_check
    type: shell
    command: "golangci-lint run"
    on_error: halt           # 기본값과 동일, 명시적 선언

  - name: optional_metrics
    type: shell
    command: "collect-metrics.sh"
    on_error: continue       # 실패해도 파이프라인 계속
```

#### 에이전트별 speckit 산출물 수정 권한 매트릭스

| 산출물 | PO | Architect | PM | Developer | Tech Writer | Reviewer |
|-------|----|-----------|----|-----------|-------------|----------|
| spec.md | 제안→사람승인 | read | read | read | read | read |
| plan.md | read | **write** | read | read | read | read |
| tasks.md | read | read | read ⚠️ | read | read | read |
| contracts/ | read | **write** | read | **제한적 write** | read | read |
| data-model.md | read | **write** | read | read | read | read |
| constitution.md | read | read | read | read | read ⚠️ | read |
| `.pylon/domain/*` | read | read | read | read | **write** | read |
| context.md | read | read | read | read | **write** | read |

> ⚠️ 표시: 해당 에이전트의 핵심 입력 산출물이지만 수정 권한 없음 (아래 설명 참조)

**Developer의 "제한적 write"**: 사소한 조정(필드 타입 변경, 엔드포인트 세부 조정, 응답 필드 추가)만 가능. 구조적 변경(엔드포인트 삭제/추가, 인증 방식 변경, DB 스키마 구조 변경)은 Architect에 에스컬레이션.

**PM의 tasks.md (read ⚠️)**: PM은 tasks.md를 읽어서 [P] 마커 파싱 → 스케줄링 계획을 수립하지만, tasks.md 자체를 수정하지 않는다. 핵심 소비자이므로 ⚠️로 표시.

**Tech Writer의 constitution.md (read ⚠️)**: Tech Writer는 constitution.md를 참조하여 문서 품질 기준을 준수하지만 수정 권한은 없다. 사람만 constitution.md를 수정할 수 있다.

**PO의 spec.md "사람 승인" 메커니즘**: PO가 spec.md 변경을 제안하면 파이프라인이 `StagePOConversation`으로 전환되어 일시정지된다. 사람은 PO와의 인터랙티브 대화(Claude Code CLI)를 통해 변경 내용을 검토하고 승인/거부한다. PO 대화가 종료되면 승인으로 간주하고 다음 단계로 진행한다. 이는 `ErrInteractiveRequired` 패턴과 동일한 메커니즘이다.

## 9. 대화 기록 관리

### 구조

```
.pylon/conversations/                   ← .gitignore 대상
└── 20260305-143022-user-login/
    ├── thread.md       ← 전체 대화 기록 (사람 ↔ PO)
    └── meta.yml        ← 대화 메타데이터

.pylon/tasks/                           ← git 커밋 대상 (Source of Truth)
└── 20260305-user-login.md              ← 최종 작업 지시서
```

- `conversations/`는 대화 과정 아카이브 (git 제외, 로컬 보관)
- `tasks/`는 확정된 작업 지시서 (git 포함, Source of Truth)
- `meta.yml`의 `task_id`로 conversations ↔ tasks 연결

#### meta.yml 스키마

```yaml
# .pylon/conversations/{id}/meta.yml
status: active                           # active | completed | cancelled
started_at: "2026-03-05T14:30:22Z"
completed_at: null                       # 완료/취소 시 ISO 8601
projects:                                # 관련 프로젝트 목록
  - project-api
task_id: null                            # 요구사항 확정 시 tasks/와 연결
session_id: null                         # Claude Code CLI 세션 ID (resume용)
```

| 필드 | 필수 | 설명 |
|------|------|------|
| `status` | 필수 | `active` \| `completed` \| `cancelled` |
| `started_at` | 필수 | 대화 시작 시각 (ISO 8601) |
| `completed_at` | 선택 | 완료/취소 시각 (ISO 8601) |
| `projects` | 선택 | 관련 프로젝트 목록 |
| `task_id` | 선택 | 요구사항 확정 시 `tasks/`의 파일과 연결 |
| `session_id` | 선택 | Claude Code CLI 세션 ID (`pylon resume`용) |

**status 열거형**:
- `active`: PO와 대화 진행 중
- `completed`: 요구사항 확정 → 파이프라인 실행으로 전환
- `cancelled`: 사용자가 `pylon cancel`로 취소

### thread.md 형식

```markdown
# 대화: 로그인 기능 구현

## [2026-03-05 14:30] 사용자
로그인 기능 구현해줘

## [2026-03-05 14:30] PO
소셜 로그인도 포함할까요? 현재 project-api에 OAuth 관련 의존성이 없는데,
Google/Kakao 등 소셜 로그인도 스코프에 포함시킬지 확인이 필요합니다.

## [2026-03-05 14:31] 사용자
카카오 로그인만 포함해줘

## [2026-03-05 14:31] PO
확인했습니다. 요구사항 정리:
- 이메일/비밀번호 기본 로그인
- 카카오 소셜 로그인
- JWT 기반 세션 관리 (기존 아키텍처 준수)

이대로 진행할까요?

## [2026-03-05 14:32] 사용자
ㅇㅇ 진행해

## [2026-03-05 14:32] PO
작업을 시작합니다. → task/20260305-user-login
```

## 10. 사람 개입 포인트

### MVP (v1)

- 요구사항 전달 → `pylon request` 실행
- PO의 역질문에 응답 (TUI/Dashboard 인터랙션)
- PR 리뷰 + 머지 승인 (PR에 reviewer로 지정)
- PR 피드백 반영 → `pylon review` 실행

### 고도화 (v2)

- AI가 이슈/채팅에서 요구사항 자동 감지
- PR 코멘트 webhook으로 자동 피드백 루프
- 사람은 머지만 승인 (또는 자동 머지 옵션)

## 11. 알림

- **PR 생성 시**: GitHub PR의 reviewer로 사람을 지정 → GitHub 기본 알림 활용
- **에스컬레이션 시**: TUI/Dashboard에서 알림 표시
- **별도 알림 채널 없음** (GitHub 알림으로 충분)

## 12. 프로젝트 에이전트 자동 구성

- `pylon add-project` 시 **AI 기반 자동 구성**:
  - 코드베이스 전체를 분석하여 기술 스택, 프레임워크, 아키텍처 파악
  - AI가 적절한 에이전트 구성을 제안 (역할, 도구, 컨벤션 포함)
  - 사용자 확인 후 에이전트 .md 파일 생성
- 코드베이스 분석하여 `context.md` 자동 생성
- 사용자가 추가 에이전트 커스텀 가능

## 13. Git 브랜치 전략

**GitHub Flow + 태스크 prefix**

```
main                                    ← 항상 배포 가능 상태
├── task/20260305-user-login            ← 태스크 단위 브랜치
├── task/20260306-api-pagination
└── task/20260307-dashboard-redesign
```

**네이밍 컨벤션**: `task/{YYYYMMDD}-{태스크-slug}`

- PM이 태스크 생성 시 브랜치명 결정
- 날짜 prefix로 자연 정렬
- 각 submodule에서 동일한 브랜치명 사용

**멀티 프로젝트 태스크**:
- 의존성 없음 → 병렬 (각 프로젝트에서 동시에 같은 브랜치명)
- 의존성 있음 → PM이 순서 지정 (api PR 머지 후 web 착수)

## 14. 보안

### 시크릿 관리

- **pylon은 시크릿을 직접 저장하지 않음**
- 각 도구의 기존 인증을 그대로 사용:
  - Claude Code CLI → `~/.claude/` (Anthropic 인증)
  - gh CLI → `gh auth login` (GitHub 인증)
  - git → 기존 git credential 설정
- `config.yml`에 API 키, 토큰 등 민감 정보를 포함하지 않음

### 에이전트 실행 샌드박싱

- Claude Code CLI 네이티브 permission 시스템 활용
- 에이전트별 `permissionMode`로 권한 수준 제어 (Section 5 frontmatter 참조)
- PO 에이전트: `default` (사용자 승인 필요) — 사용자 인터랙션 모드
- 프로젝트 에이전트: `acceptEdits` (파일 편집 자동 승인) — 무인 자동화 기본
- `bypassPermissions`는 신뢰도가 검증된 에이전트에만 사용 권장

## 15. 기존 도구 비교

| | MetaGPT | CrewAI | AutoGen | Auto-Claude | **Pylon** |
|---|---|---|---|---|---|
| **조직 구조** | 단일 프로젝트 | 단일 태스크 | 플랫 대화 | 마스터/워커 | **루트→프로젝트 계층** |
| **도메인 지식 축적** | ❌ | ❌ | ❌ | ❌ | **✅ 코드 기반 위키** |
| **멀티 프로젝트** | ❌ | ❌ | ❌ | ❌ | **✅ submodule** |
| **사용자 역할** | 요구사항+개입 | 코드 정의 | 대화 참여 | 태스크 정의 | **요구사항만** |
| **팀 특화** | 범용 | 범용 | 범용 | Claude 특화 | **✅ 팀 지식** |
| **에이전트 페르소나** | ✅ | ✅ | △ | ❌ | **✅** |
| **도구 선택** | 자체 LLM | 다양 | 다양 | Claude Code | **선택 가능** |
| **에러 복구** | △ | ❌ | ❌ | ✅ | **✅ PM 판단** |
| **대화 기록** | ❌ | ❌ | △ | ❌ | **✅ 영속 관리** |
| **코드 교차 검증** | ❌ | ❌ | ❌ | ❌ | **✅ 오케스트레이터** |
| **에이전트 메모리** | ❌ | △ 단기만 | △ 대화만 | ❌ | **✅ 3계층 (작업/세션/프로젝트)** |
| **지식 전이** | ❌ | ❌ | ❌ | ❌ | **✅ 프로젝트 메모리 + BM25** |

## 16. config.yml 스키마

### 풀 스키마

```yaml
# .pylon/config.yml
version: "0.1"                          # 스키마 버전 (마이그레이션용)

# ─── 런타임 ───────────────────────────────────
runtime:
  backend: claude-code          # 기본 백엔드 (에이전트별 override 가능)
  max_concurrent: 5             # 동시 실행 최대 에이전트 수 (시스템 리소스 보호용 상한선)
  task_timeout: 30m             # 단일 태스크 타임아웃 (stuck 방지)
  max_attempts: 2               # 최대 시도 횟수 (최초 1회 + 재시도 1회). 모두 실패 시 자동 에스컬레이션
  max_turns: 50                 # 에이전트당 최대 LLM 턴 수 (기본 50, 에이전트별 override 가능)
  permission_mode: acceptEdits  # 기본 권한 모드 (default | acceptEdits | bypassPermissions)
  env:                          # 에이전트 환경변수 기본값 (에이전트별 override 가능)
    CLAUDE_AUTOCOMPACT_PCT_OVERRIDE: "80"
    CLAUDE_CODE_EFFORT_LEVEL: "high"

# ─── Git ──────────────────────────────────────
git:
  branch_prefix: task           # → task/20260305-user-login
  default_base: main            # PR 대상 베이스 브랜치
  auto_push: true               # 작업 브랜치 자동 push
  worktree:
    enabled: true               # 에이전트별 git worktree 생성 (기본 true)
    auto_cleanup: true          # 태스크 완료 후 worktree 자동 정리
  pr:
    reviewers:                  # PR reviewer GitHub 유저명
      - keiyjay
    draft: false                # draft PR 여부
    template: null              # PR 본문 템플릿 경로 (null이면 기본)

# ─── 프로젝트 (자동 감지 + override) ──────────
projects:
  project-api:
    stack: go                   # 자동 감지 override
    # agents: [backend-dev, qa] # 자동 구성 override
  project-web:
    stack: react

# ─── 위키 ─────────────────────────────────────
wiki:
  auto_update: true
  update_on:                    # 자동 업데이트 트리거
    - task_complete
    - pr_merged

# ─── 대시보드 ─────────────────────────────────
dashboard:
  host: localhost
  port: 7777

# ─── 메모리 ──────────────────────────────────
memory:
  compaction_threshold: 0.7     # 컨텍스트 사용량 70% 도달 시 압축 트리거
  proactive_injection: true     # 태스크 시작 시 관련 메모리 자동 주입
  proactive_max_tokens: 2000    # 선제적 주입 최대 토큰 수
  session_archive: true         # 세션 종료 시 자동 아카이빙
  retention_days: 0             # 프로젝트 메모리 보관 기간 (0이면 무제한)

# ─── 대화 기록 ────────────────────────────────
conversation:
  retention_days: 90            # 보관 기간 (0이면 무제한)
```

### pylon init 직후 최소 config

```yaml
version: "0.1"

runtime:
  backend: claude-code
  max_concurrent: 5
  max_turns: 50
  permission_mode: acceptEdits

git:
  pr:
    reviewers:
      - keiyjay
```

나머지는 전부 기본값으로 동작.

## 17. 미결 사항

- [ ] `pylon` 패키지명 충돌 확인 (배포 시 확인, 지금은 그대로)
- [x] 에이전트 간 통신의 구체적 메시지 포맷 정의 → Section 8 "하이브리드 통신 프로토콜"에 정의 완료
- [x] 에이전트 메모리/컨텍스트 관리 전략 → Section 8 "에이전트 메모리 아키텍처"에 정의 완료
- [x] 에이전트 CLI 실행 명세 → Section 8 "Claude Code CLI 실행 명세"에 정의 완료
- [x] 에이전트 권한 모드 및 안전장치 → Section 5 frontmatter `permissionMode`, `maxTurns` 정의 완료
- [x] 에이전트 파일시스템 격리 → Section 8 "Git Worktree 격리" 정의 완료
- [x] speckit 전용 모드 에이전트 역할 재정의 → Section 6 "speckit 전용 모드 에이전트 역할" + Section 8 "에이전트별 speckit 산출물 수정 권한 매트릭스"에 정의 완료
- [x] speckit 산출물 소비 방식 → Section 8 "speckit 산출물 소비 방식"에 정의 완료 (에이전트 직접 읽기, 별도 파싱 레이어 불필요)
- [x] Constitution 검증 실패 경로 → Section 8 "Constitution 검증 흐름"에 정의 완료 (별도 reviewer 에이전트 + 재시도 1회 → 사람 에스컬레이션)
- [x] speckit 위치 및 접근 방식 → Section 7 `pylon add-project`에 정의 완료 (pylon 내장, add-project 시 `specify init` 자동 실행)
- [x] 셸 step 에러 처리 → Section 8 "셸 step 에러 처리"에 정의 완료 (기본 halt + step별 on_error override)
- [x] verify.yml 스키마 → Section 8 "코드 품질 교차 검증"에 정의 완료
- [x] task_prompt 조립 방식 → Section 8 "Claude Code CLI 실행 명세"에 정의 완료
- [x] meta.yml 스키마 및 status 열거형 → Section 9 "대화 기록 관리"에 정의 완료
- [x] pylon resume 상세 동작 → Section 7에 정의 완료
- [ ] pylon cleanup 상세 설계 (좀비 프로세스 판단, pipeline 상태 처리, worktree 정리)
- [ ] TUI 대화 인터페이스 상세 UX 설계 (화면 구성, 키바인딩 등)
- [ ] Dashboard SSE 이벤트 타입 및 데이터 포맷 정의
  - 데이터 소스: SQLite `pipeline_state`, `message_queue`, `session_archive` 테이블 (state.json 제거됨)
  - CLI 동등 기능 대상: 스펙 원안 11개 + 추가 5개(index, stage, mem, sync-memory, uninstall) = 16개 명령어
- [ ] 에이전트 프롬프트 상세 설계 (PO/PM/Architect/Tech Writer 각각)
- [ ] Compaction 트리거의 구체적 구현 방식 (에이전트 측 토큰 카운팅 메커니즘)
- [ ] 블랙보드 항목의 confidence 점수 산정 기준 정의 (현재 Go 레벨에서 0.0~1.0 범위 검증 구현됨)
- [ ] 벡터 임베딩 검색 도입 시점 및 임베딩 모델 선정 (향후 확장)
- [ ] Hooks 시스템 설계 (speckit Extension/Hook 연계 — `before_implement`, `after_tasks` 등. Phase 1 로드맵)
- [ ] 설정 계층화 (`config.local.yml` 도입 여부)
- [ ] `.pylon/tasks/` 디렉토리 활용 범위 재정의 (현재 참조만 있고 실질적 사용 미미)
- [ ] Skills 패턴 상세 설계 (에이전트별 전문 지식 preload 메커니즘)
