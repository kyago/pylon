# Pylon 구현 계획서

> 기준 스펙: `pylon-spec.md` (Draft v0.8)
> 작성일: 2026-03-07

---

## 참조 문서 맵

| 문서 | 경로 | 주요 참조 Phase |
|------|------|----------------|
| 요구사항 명세서 | `pylon-spec.md` | 전체 |
| 에이전트 통신/메모리 리서치 | `claudedocs/research-agent-communication-and-memory.md` | Phase 3, 5 |
| 초기화 패턴 큐레이션 | `claudedocs/curated-patterns-for-init.md` | Phase 1 |
| 갭 분석 | `claudedocs/analysis-best-practice-gap.md` | Phase 4 |
| Claude Code 리서치 | `claudedocs/research-claude-code-best-practice-repo.md` | Phase 4 |
| 수집 스킬/에이전트 | `claudedocs/collected-skills-and-agents.md` | Phase 1 (참고) |

---

## Go 패키지 구조

```
pylon/
├── cmd/pylon/
│   └── main.go                          # 엔트리포인트
├── internal/
│   ├── cli/                             # CLI 명령어 (Cobra)
│   │   ├── root.go                      # 루트 커맨드 + 글로벌 플래그
│   │   ├── doctor.go                    # pylon doctor
│   │   ├── init_cmd.go                  # pylon init
│   │   ├── request.go                   # pylon request
│   │   ├── status.go                    # pylon status
│   │   ├── cancel.go                    # pylon cancel
│   │   ├── resume.go                    # pylon resume
│   │   ├── review.go                    # pylon review
│   │   ├── cleanup.go                   # pylon cleanup
│   │   ├── destroy.go                   # pylon destroy
│   │   ├── add_project.go              # pylon add-project
│   │   └── dashboard.go                # pylon dashboard
│   ├── config/                          # 설정 파싱/관리
│   │   ├── config.go                    # config.yml 구조체 + 파싱
│   │   ├── config_test.go
│   │   ├── agent.go                     # 에이전트 .md 파싱 (frontmatter + body)
│   │   ├── agent_test.go
│   │   ├── verify.go                    # verify.yml 파싱
│   │   └── workspace.go                # 워크스페이스 탐지/관리
│   ├── executor/                        # 프로세스 실행
│   │   ├── executor.go                  # ProcessExecutor 인터페이스
│   │   └── direct.go                    # 직접 실행 (syscall.Exec / exec.Command)
│   ├── orchestrator/                    # 오케스트레이터 코어
│   │   ├── orchestrator.go              # 메인 오케스트레이터 루프
│   │   ├── orchestrator_test.go
│   │   ├── pipeline.go                  # 파이프라인 상태 머신
│   │   ├── pipeline_test.go
│   │   ├── watcher.go                   # fsnotify outbox 감시
│   │   ├── watcher_test.go
│   │   ├── recovery.go                  # SPOF 복구
│   │   └── verify.go                    # 교차 검증 (빌드/테스트/린트)
│   ├── protocol/                        # 통신 프로토콜
│   │   ├── message.go                   # MessageEnvelope 구조체
│   │   ├── message_test.go
│   │   ├── inbox.go                     # inbox 파일 생성 (원자적 쓰기)
│   │   ├── outbox.go                    # outbox 파일 읽기
│   │   ├── topic.go                     # 토픽 라우터 (Pub/Sub)
│   │   └── handoff.go                   # Narrative Casting 핸드오프
│   ├── store/                           # SQLite 저장소
│   │   ├── store.go                     # DB 연결 + 마이그레이션
│   │   ├── store_test.go
│   │   ├── migrations/                  # SQL DDL (go:embed)
│   │   │   └── 001_initial.sql
│   │   ├── message_queue.go             # message_queue CRUD
│   │   ├── pipeline_state.go            # pipeline_state CRUD
│   │   ├── blackboard.go               # blackboard CRUD
│   │   ├── project_memory.go           # project_memory CRUD + BM25 검색
│   │   └── session_archive.go          # session_archive CRUD
│   ├── agent/                           # 에이전트 실행 엔진
│   │   ├── runner.go                    # Claude Code CLI 래퍼
│   │   ├── runner_test.go
│   │   ├── lifecycle.go                 # 에이전트 생명주기 상태 머신
│   │   ├── claudemd.go                  # CLAUDE.md 동적 생성 (200줄 제한)
│   │   ├── claudemd_test.go
│   │   └── env.go                       # 환경변수 해석 (config → agent override)
│   ├── git/                             # Git 유틸리티
│   │   ├── worktree.go                  # Git worktree 생성/정리
│   │   ├── worktree_test.go
│   │   ├── branch.go                    # 브랜치 전략 (task/{date}-{slug})
│   │   ├── submodule.go                # Submodule 관리
│   │   └── pr.go                        # gh pr create 래퍼
│   ├── memory/                          # 메모리 관리
│   │   ├── manager.go                   # 3계층 메모리 매니저
│   │   ├── proactive.go                 # 선제적 메모리 주입 (BM25 검색)
│   │   ├── reactive.go                  # 반응적 메모리 검색 (에이전트 query 처리)
│   │   └── archive.go                   # 세션 아카이빙
│   ├── tui/                             # TUI (Phase 7)
│   │   └── app.go
│   └── web/                             # Web Dashboard (Phase 7)
│       └── server.go
├── testdata/                            # 테스트 픽스처
│   ├── config/                          # 샘플 config.yml
│   ├── agents/                          # 샘플 에이전트 .md
│   └── workspace/                       # 테스트용 워크스페이스
├── go.mod
├── go.sum
└── Makefile
```

---

## Phase 의존성 그래프

```
Phase 0 ──→ Phase 1 ──→ Phase 2 ──→ Phase 3 ──→ Phase 4 ──→ Phase 5
 부트스트랩    기반 명령어   프로세스 실행   오케스트레이터   에이전트 실행   파이프라인
                                        코어          엔진

Phase별 핵심 산출물:
  0: go build 성공, pylon --help
  1: pylon doctor, pylon init, config 파싱
  2: 에이전트 프로세스 직접 실행/종료/감시
  3: SQLite, MessageEnvelope, fsnotify, 파이프라인 상태 머신
  4: Claude CLI 래퍼, CLAUDE.md 빌더, worktree 격리
  5: pylon request 전체 파이프라인 (PO→Architect→PM→에이전트→검증→PR)
```

---

## Phase 0: 프로젝트 부트스트랩

> 목표: Go 모듈 초기화, CLI 프레임워크 뼈대, 빌드 인프라

### 구현 항목

#### 0-1. Go 모듈 초기화

```bash
go mod init github.com/yongjunkang/pylon
```

- Go 1.23+ 타겟
- 초기 의존성: `cobra`, `yaml.v3`, `google/uuid`

#### 0-2. 엔트리포인트 (`cmd/pylon/main.go`)

```go
func main() {
    if err := cli.Execute(); err != nil {
        os.Exit(1)
    }
}
```

#### 0-3. 루트 커맨드 (`internal/cli/root.go`)

- 글로벌 플래그: `--workspace` (워크스페이스 경로 override), `--verbose`, `--json` (JSON 출력)
- 서브커맨드 11개 등록 (각각 빈 RunE 함수)
- 버전 정보 (`pylon version`): 빌드 시 `ldflags`로 주입

#### 0-4. Makefile

```makefile
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")

build:
    go build -ldflags "-X main.version=$(VERSION)" -o bin/pylon ./cmd/pylon

test:
    go test ./... -race -count=1

lint:
    golangci-lint run ./...

install: build
    cp bin/pylon $(GOPATH)/bin/
```

### 의존성

```
github.com/spf13/cobra          v1.9+
gopkg.in/yaml.v3                latest
github.com/google/uuid          latest
```

### 완료 기준

- `make build` 성공
- `pylon --help`가 11개 서브커맨드 목록 표시
- `pylon version`이 빌드 정보 출력
- `make test` / `make lint` 통과

---

## Phase 1: 기반 명령어

> 목표: 워크스페이스 초기화와 설정 파싱의 기반 확립
> 참조: 스펙 Section 5, 7, 16 / `curated-patterns-for-init.md`

### 구현 항목

#### 1-1. config.yml 파서 (`internal/config/config.go`)

> 스펙 참조: Section 16 "config.yml 스키마"

```go
type Config struct {
    Version      string              `yaml:"version"`
    Runtime      RuntimeConfig       `yaml:"runtime"`
    Git          GitConfig           `yaml:"git"`
    Projects     map[string]ProjectConfig `yaml:"projects"`
    Wiki         WikiConfig          `yaml:"wiki"`
    Dashboard    DashboardConfig     `yaml:"dashboard"`
    Memory       MemoryConfig        `yaml:"memory"`
    Conversation ConversationConfig  `yaml:"conversation"`
}

type RuntimeConfig struct {
    Backend        string            `yaml:"backend"`
    MaxConcurrent  int               `yaml:"max_concurrent"`
    TaskTimeout    string            `yaml:"task_timeout"`
    MaxAttempts    int               `yaml:"max_attempts"`
    MaxTurns       int               `yaml:"max_turns"`
    PermissionMode string            `yaml:"permission_mode"`
    Env            map[string]string `yaml:"env"`
}

type GitConfig struct {
    BranchPrefix string          `yaml:"branch_prefix"`
    DefaultBase  string          `yaml:"default_base"`
    AutoPush     bool            `yaml:"auto_push"`
    Worktree     WorktreeConfig  `yaml:"worktree"`
    PR           PRConfig        `yaml:"pr"`
}

type MemoryConfig struct {
    CompactionThreshold float64 `yaml:"compaction_threshold"`
    ProactiveInjection  bool    `yaml:"proactive_injection"`
    ProactiveMaxTokens  int     `yaml:"proactive_max_tokens"`
    SessionArchive      bool    `yaml:"session_archive"`
    RetentionDays       int     `yaml:"retention_days"`
}
```

**구현 세부**:
- `yaml.v3` 기반 파싱 + 기본값 채우기 (Section 16 기본값 적용)
- `version` 필수 검증
- 미지정 필드에 기본값 자동 적용

**테스트**: 최소 config, 풀 config, 잘못된 config, 필수 필드 누락

#### 1-2. 에이전트 .md 파서 (`internal/config/agent.go`)

> 스펙 참조: Section 5 "에이전트 설정 포맷"

```go
type AgentConfig struct {
    // YAML frontmatter (--- 구분자 사이)
    Name           string            `yaml:"name"`
    Role           string            `yaml:"role"`
    Backend        string            `yaml:"backend"`
    Scope          []string          `yaml:"scope"`
    Tools          []string          `yaml:"tools"`
    MaxTurns       int               `yaml:"maxTurns"`
    PermissionMode string            `yaml:"permissionMode"`
    Isolation      string            `yaml:"isolation"`
    Model          string            `yaml:"model"`
    Env            map[string]string `yaml:"env"`

    // Markdown body (--- 이후 전체)
    Body string
    // 파일 경로 (디버깅용)
    FilePath string
}

// ParseAgentFile: .md 파일에서 frontmatter + body 분리 후 파싱
func ParseAgentFile(path string) (*AgentConfig, error)

// ResolveDefaults: config.yml 기본값으로 미지정 필드 채우기
func (a *AgentConfig) ResolveDefaults(cfg *Config)
```

**파싱 로직**:
1. 파일 첫 줄 `---` 확인
2. 두 번째 `---`까지 YAML 파싱 → frontmatter
3. 두 번째 `---` 이후 전체 → body (원본 보존)
4. 필수 필드 검증: `name`, `role`

**기본값 상속**:
- `backend` ← config.yml `runtime.backend`
- `maxTurns` ← config.yml `runtime.max_turns`
- `permissionMode` ← config.yml `runtime.permission_mode`
- `isolation` ← `"worktree"` (하드코딩 기본값)
- `env` ← config.yml `runtime.env` 위에 에이전트 env merge

**테스트**: frontmatter만, body 포함, 기본값 상속, 다양한 필드 조합

#### 1-3. 워크스페이스 탐지 (`internal/config/workspace.go`)

```go
// 현재 디렉토리부터 상위로 올라가며 .pylon/ 탐색
func FindWorkspaceRoot(startDir string) (string, error)

// .pylon/ 내부 프로젝트 목록 (git submodule 기반)
func DiscoverProjects(root string) ([]ProjectInfo, error)

// 워크스페이스 내 모든 에이전트 설정 로드
func LoadAllAgents(root string) (map[string]*AgentConfig, error)
```

#### 1-4. `pylon doctor` (`internal/cli/doctor.go`)

> 스펙 참조: Section 7 "pylon doctor"

```go
type Check struct {
    Name     string
    Required bool
    Verify   func() (version string, err error)
}

var checks = []Check{
    {"git",    true,  verifyGit},     // git --version
    {"gh",     true,  verifyGH},      // gh --version + gh auth status
    {"claude", true,  verifyClaude},  // claude --version
}
```

**출력**:
```
Pylon Doctor
─────────────────────────
✓ git      2.44.0   [required]
✓ gh       2.65.0   [required, authenticated]
✓ claude   2.3.1    [required]

All checks passed.
```

**에러 시**: 미설치 도구의 설치 안내 URL 표시

#### 1-5. `pylon init` (`internal/cli/init.go`)

> 스펙 참조: Section 7 "pylon init" / `curated-patterns-for-init.md`

**실행 흐름**:

```
1. doctor 검증 (내부 호출)
2. .pylon/ 존재 시 에러 + 안내
3. 대화형 입력:
   a. 에이전트 백엔드 (MVP: claude-code 고정)
   b. PR reviewer GitHub 유저명
4. 디렉토리/파일 생성:

.pylon/
├── config.yml                    ← Section 16 "최소 config" 기반
├── domain/
│   ├── conventions.md            ← 빈 템플릿 (헤더만)
│   ├── architecture.md           ← 빈 템플릿
│   └── glossary.md               ← 빈 템플릿
├── agents/
│   ├── po.md                     ← permissionMode: default
│   ├── pm.md                     ← permissionMode: acceptEdits
│   ├── architect.md              ← permissionMode: acceptEdits
│   └── tech-writer.md            ← permissionMode: acceptEdits, 자기진화 규칙
├── skills/
│   └── .gitkeep
├── runtime/                      ← .gitignore 대상
│   ├── inbox/
│   ├── outbox/
│   ├── memory/
│   └── sessions/
├── conversations/                ← .gitignore 대상
└── tasks/

5. .gitignore에 runtime/, conversations/ 추가
6. git init (이미 있으면 스킵)
```

**에이전트 템플릿 기본 구조** (4종 각각 역할에 맞게):

```yaml
---
name: po
role: Product Owner
backend: claude-code
maxTurns: 50
permissionMode: default
---

# Product Owner

## 역할
사용자의 요구사항을 분석하고, 역질문을 통해 구체화하며,
수용 기준을 정의하는 프로덕트 오너.

## 워크플로우
1. 요구사항 수신 → 위키 기반 분석
2. 불명확한 부분 역질문
3. 수용 기준 확정 → outbox에 결과 전달
```

### 완료 기준

- `pylon doctor`가 4개 도구 검증 후 결과 표시
- `pylon init`이 워크스페이스 생성 후 `pylon doctor`와 config 파싱 모두 통과
- config.yml + agent .md 파싱 단위 테스트 통과
- 워크스페이스 탐지 (`FindWorkspaceRoot`) 동작

---

## Phase 2: 프로세스 실행 레이어

> 목표: 에이전트 프로세스의 생성, 관리, 감시, 종료를 Go에서 안전하게 제어
> 참조: 스펙 Section 8 "프로세스 관리: 직접 실행"

### 구현 항목

#### 2-1. 프로세스 실행 인터페이스 (`internal/executor/executor.go`)

```go
// 인터페이스로 추상화 (테스트 모킹용)
type ProcessExecutor interface {
    ExecInteractive(cfg ExecConfig) error          // syscall.Exec로 현재 프로세스 대체 (인터랙티브 에이전트용)
    RunHeadless(ctx context.Context, cfg ExecConfig) (*ExecResult, error)  // exec.Command로 자식 프로세스 실행 (비인터랙티브 에이전트용)
}

type ExecConfig struct {
    Name       string            // 에이전트 이름
    Command    string            // 실행할 바이너리
    Args       []string          // CLI 인자
    WorkDir    string            // 작업 디렉토리
    Env        map[string]string // 환경변수
    Stdout     io.Writer         // 출력 스트림 (선택)
    Stderr     io.Writer         // 에러 스트림 (선택)
}

type ExecResult struct {
    ExitCode int
    Output   string
}
```

**구현 (`internal/executor/direct.go`)**:
- `ExecInteractive`: 바이너리 경로 해석 → 작업 디렉토리 변경 → 환경변수 빌드 → `syscall.Exec` 호출
- `RunHeadless`: `exec.Command` 생성 → stdout/stderr 캡처 또는 스트리밍 → 종료 코드 반환

**에러 처리**:
- 바이너리 미설치 → `pylon doctor` 안내
- 프로세스 생성 실패 → 시스템 에러 전파

#### 2-2. `pylon cleanup` (`internal/cli/cleanup.go`)

> 스펙 참조: Section 7 "pylon cleanup"

```
1. state.json에서 활성 에이전트 목록 조회
2. PID 기반으로 프로세스 생존 여부 확인
3. 좀비 프로세스 목록 표시 + 사용자 확인 (y/n)
4. 확인 후 프로세스 종료 + worktree 정리
```

#### 2-3. `pylon status` 기초 (`internal/cli/status.go`)

> 스펙 참조: Section 7 "pylon status"

```
출력:
Pylon Status
─────────────────────────
Active Agents:
  ● PO              running   (started 5m ago)
  ● PM              running   (started 3m ago)

No active pipeline.
```

Phase 3에서 파이프라인 정보 추가, Phase 5에서 전체 상태 표시.

#### 2-4. `pylon destroy` (`internal/cli/destroy.go`)

> 스펙 참조: Section 7 "pylon destroy"

```
1. 활성 에이전트 프로세스 전체 종료
2. .pylon/ 디렉토리 삭제 (확인 프롬프트)
3. .gitignore에서 pylon 관련 항목 제거
```

### 테스트 전략

- `ProcessExecutor` 인터페이스 → 모킹으로 단위 테스트
- 실제 프로세스 실행 통합 테스트 → `//go:build integration` 태그 분리
- `testdata/` 에 mock 프로세스 출력 데이터

### 완료 기준

- 에이전트 프로세스 직접 실행/종료/상태 확인 동작
- `pylon cleanup`이 좀비 프로세스 정리
- `pylon status`가 활성 에이전트 표시
- 인터페이스 모킹 기반 단위 테스트 통과

---

## Phase 3: 오케스트레이터 코어

> 목표: SQLite 저장소, 통신 프로토콜, 파일 감시, 파이프라인 상태 머신
> 참조: 스펙 Section 8 전체 / `research-agent-communication-and-memory.md`

### 구현 항목

#### 3-1. SQLite 저장소 (`internal/store/`)

> 스펙 참조: Section 8 "SQLite 스키마"

**의존성**: `modernc.org/sqlite` (CGO-free pure Go SQLite)
- 크로스 컴파일 용이, CGO 불필요
- WAL 모드 활성화: `PRAGMA journal_mode=WAL;`

**마이그레이션 (`internal/store/migrations/001_initial.sql`)**:
- 스펙 Section 8의 DDL 그대로 사용
- 7개 테이블: `message_queue`, `pipeline_state`, `blackboard`, `topic_subscriptions`, `project_memory`, `project_memory_fts` (FTS5), `session_archive`
- `go:embed` 디렉티브로 바이너리에 포함

```go
type Store struct {
    db *sql.DB
}

func NewStore(dbPath string) (*Store, error)  // 연결 + WAL 모드
func (s *Store) Migrate() error               // embed SQL 실행
func (s *Store) Close() error
```

**각 테이블 CRUD**:

| 파일 | 핵심 메서드 |
|------|-----------|
| `message_queue.go` | Enqueue, Dequeue, Ack, GetByTaskID, GetPending |
| `pipeline_state.go` | Upsert, Get, GetHistory, GetActive |
| `blackboard.go` | Put, Get, GetByCategory, GetByOwner |
| `project_memory.go` | Insert, Search (BM25 via FTS5), GetByCategory, IncrementAccessCount |
| `session_archive.go` | Archive, GetByAgent, GetByTask, GetRecent |

**테스트**: 각 CRUD 함수에 인메모리 SQLite (`:memory:`)로 단위 테스트

#### 3-2. 통신 프로토콜 (`internal/protocol/`)

> 스펙 참조: Section 8 "하이브리드 통신 프로토콜"
> 리서치 참조: `research-agent-communication-and-memory.md` MessageEnvelope 구조체

```go
type MessageType string
const (
    MsgTaskAssign  MessageType = "task_assign"
    MsgResult      MessageType = "result"
    MsgQuery       MessageType = "query"
    MsgQueryResult MessageType = "query_result"
    MsgBroadcast   MessageType = "broadcast"
    MsgHeartbeat   MessageType = "heartbeat"
)

type Priority int
const (
    PriorityCritical Priority = 0
    PriorityHigh     Priority = 1
    PriorityNormal   Priority = 2
    PriorityLow      Priority = 3
)

type MessageEnvelope struct {
    ID        string      `json:"id"`         // UUID v7
    Type      MessageType `json:"type"`
    Priority  Priority    `json:"priority"`
    From      string      `json:"from"`       // 발신 에이전트명 또는 "orchestrator"
    To        string      `json:"to"`         // 수신 에이전트명 또는 "orchestrator"
    ReplyTo   string      `json:"reply_to,omitempty"`
    Subject   string      `json:"subject"`
    Body      any         `json:"body"`       // 타입별 payload
    Context   *MsgContext `json:"context,omitempty"`
    TTL       string      `json:"ttl,omitempty"`
    Trace     []string    `json:"trace,omitempty"`
    Timestamp string      `json:"timestamp"`  // RFC3339
}

type MsgContext struct {
    TaskID        string `json:"task_id"`
    PipelineID    string `json:"pipeline_id"`
    ParentTaskID  string `json:"parent_task_id,omitempty"`
    Summary       string `json:"summary,omitempty"`       // Narrative Casting 서사 요약
}
```

**inbox.go** — 오케스트레이터 → 에이전트 태스크 전달:
```go
// .pylon/runtime/inbox/{agent-name}/{task-id}.task.json
func WriteTask(inboxDir, agentName string, msg *MessageEnvelope) error
```

**outbox.go** — 에이전트 → 오케스트레이터 결과 수집:
```go
// .pylon/runtime/outbox/{agent-name}/{task-id}.result.json
func ReadResult(path string) (*MessageEnvelope, error)
```

**원자적 쓰기**:
```go
func writeAtomically(path string, data []byte) error {
    tmp := path + ".tmp"
    if err := os.WriteFile(tmp, data, 0644); err != nil {
        return err
    }
    return os.Rename(tmp, path) // POSIX rename 원자성
}
```

#### 3-3. 파일 감시 (`internal/orchestrator/watcher.go`)

```go
type OutboxWatcher struct {
    watcher  *fsnotify.Watcher
    outboxDir string
    handler   func(agentName, resultFile string) error
}

func (w *OutboxWatcher) Start(ctx context.Context) error
```

**동작**:
- `fsnotify`로 `.pylon/runtime/outbox/` 하위 전체 감시
- `.result.json` 확장자의 `Create` 이벤트만 처리
- `.tmp` 파일 무시 (원자적 쓰기의 중간 파일)
- 디바운싱: 동일 파일 이벤트 100ms 내 중복 제거
- 결과 읽기 → SQLite에 저장 → 파이프라인 다음 단계 결정

#### 3-4. 파이프라인 상태 머신 (`internal/orchestrator/pipeline.go`)

> 스펙 참조: Section 7 "pylon request" 실행 흐름 (10단계)

```go
type Stage string
const (
    StageInit              Stage = "init"
    StagePOConversation    Stage = "po_conversation"
    StageArchitectAnalysis Stage = "architect_analysis"
    StagePMTaskBreakdown   Stage = "pm_task_breakdown"
    StageAgentExecuting    Stage = "agent_executing"
    StageVerification      Stage = "verification"
    StagePRCreation        Stage = "pr_creation"
    StagePOValidation      Stage = "po_validation"
    StageWikiUpdate        Stage = "wiki_update"
    StageCompleted         Stage = "completed"
    StageFailed            Stage = "failed"
)

type Pipeline struct {
    ID           string
    CurrentStage Stage
    TaskSpec     string              // tasks/ 경로
    Agents       map[string]AgentStatus
    History      []StageTransition
    CreatedAt    time.Time
}

func (p *Pipeline) Transition(to Stage) error     // 유효성 검증 + 기록
func (p *Pipeline) CanTransition(to Stage) bool
func (p *Pipeline) Snapshot() []byte               // state.json 직렬화
```

**유효 상태 전이**:
```
init → po_conversation → architect_analysis → pm_task_breakdown
→ agent_executing → verification
→ (실패 시) agent_executing  (max_attempts까지)
→ pr_creation → po_validation → wiki_update → completed
→ (어느 단계든) failed
```

#### 3-5. Git Worktree 관리 (`internal/git/worktree.go`)

> 스펙 참조: Section 8 "Git Worktree 격리"

```go
type WorktreeManager struct {
    enabled     bool
    autoCleanup bool
}

func (w *WorktreeManager) Create(projectDir, agentName, taskBranch string) (path string, err error)
func (w *WorktreeManager) Remove(path string) error
func (w *WorktreeManager) Cleanup(projectDir string) error  // 완료된 worktree 일괄 정리
```

**경로**: `{project}/.git/pylon-worktrees/{agent}-{task-slug}`

#### 3-6. SPOF 복구 (`internal/orchestrator/recovery.go`)

> 스펙 참조: Section 8 "장애 복구 (SPOF 대응)"

```go
func (o *Orchestrator) Recover() error {
    // 1. state.json 로드 → 마지막 파이프라인 상태 복원
    // 2. 에이전트 프로세스 생존 여부 확인 (PID 기반)
    // 3. outbox 스캔 → 미처리 결과 수집
    // 4. SQLite 히스토리와 교차 검증
    // 5. 에이전트 상태 재구성 + 파이프라인 재개
}
```

### 의존성 추가

```
modernc.org/sqlite               latest   # CGO-free SQLite
github.com/fsnotify/fsnotify     v1.8+    # 파일시스템 감시
```

### 완료 기준

- SQLite 마이그레이션 + 7개 테이블 CRUD 테스트 통과
- MessageEnvelope JSON 직렬화/역직렬화 테스트 통과
- inbox/outbox 원자적 파일 쓰기 테스트 통과
- fsnotify가 outbox 파일 생성 이벤트 감지
- 파이프라인 상태 전이 (유효/무효) 테스트 통과
- state.json 스냅샷 저장/복원 동작

---

## Phase 4: 에이전트 실행 엔진

> 목표: Claude Code CLI를 직접 프로세스로 실행하고, 에이전트 생명주기 관리
> 참조: 스펙 Section 5, 8 / `analysis-best-practice-gap.md` / `research-claude-code-best-practice-repo.md`

### 구현 항목

#### 4-1. Claude Code CLI 래퍼 (`internal/agent/runner.go`)

> 스펙 참조: Section 8 "Claude Code CLI 실행 명세"

```go
type RunConfig struct {
    Agent       *config.AgentConfig
    Global      *config.Config
    TaskPrompt  string
    WorkDir     string    // worktree 경로 또는 프로젝트 경로
    ClaudeMD    string    // 동적 생성된 CLAUDE.md 내용
    Interactive bool      // PO용 인터랙티브 모드
}

type Runner struct {
    executor executor.ProcessExecutor
}

func (r *Runner) BuildCommand(cfg RunConfig) (cmd string, args []string)
func (r *Runner) Start(ctx context.Context, cfg RunConfig) error
```

**비인터랙티브 에이전트 (PM, Architect, 개발자 등)**:
```bash
claude \
  --print \
  --output-format stream-json \
  --max-turns {maxTurns} \
  --permission-mode {permissionMode} \
  --model {model} \
  --append-system-prompt "$(cat claude.md)" \
  --prompt "{task_prompt}"
```

**인터랙티브 에이전트 (PO)**:
```bash
claude \
  --max-turns {maxTurns} \
  --permission-mode default \
  --append-system-prompt "$(cat claude.md)"
```

**환경변수 우선순위**:
```
에이전트 frontmatter env > config.yml runtime.env > 시스템 기본값
```

**주요 환경변수** (스펙 Section 8):
- `CLAUDE_AUTOCOMPACT_PCT_OVERRIDE`: Compaction 트리거 비율
- `CLAUDE_CODE_EFFORT_LEVEL`: 에이전트 노력 수준 (high/medium/low)
- `CLAUDE_CODE_MAX_TURNS`: 런타임 턴 제한 override

#### 4-2. CLAUDE.md 동적 생성 (`internal/agent/claudemd.go`)

> 스펙 참조: Section 8 "에이전트 CLAUDE.md 주입 규칙" (200줄 제한)

```go
type ClaudeMDBuilder struct {
    maxLines int  // 200
}

type BuildInput struct {
    // 우선순위 순서 (1 = 최고)
    CommunicationRules string   // 1. inbox/outbox 통신 절차
    TaskContext        string   // 2. 수용 기준, 제약 조건
    CompactionRules    string   // 3. 컨텍스트 관리 지침
    ProjectMemory      string   // 4. 선제적 주입 메모리 요약
    DomainPaths        []string // 5. 도메인 지식 참조 경로 (파일 경로만)
}

func (b *ClaudeMDBuilder) Build(input BuildInput) (string, error)
```

**5레벨 주입 우선순위** (스펙 Section 8 표):

| 우선순위 | 내용 | 줄 수 (예산) |
|---------|------|-------------|
| 1 | 통신 규칙 (inbox/outbox 절차) | ~30줄 |
| 2 | 태스크 컨텍스트 (수용 기준, 제약 조건) | ~50줄 |
| 3 | 컨텍스트 관리 규칙 (Compaction) | ~20줄 |
| 4 | 프로젝트 메모리 요약 | ~80줄 |
| 5 | 도메인 지식 참조 경로 | ~20줄 |

**핵심 원칙**: 도메인 지식 본문은 포함하지 않고 파일 경로만 안내 → 에이전트가 필요 시 직접 읽기

#### 4-3. 에이전트 생명주기 (`internal/agent/lifecycle.go`)

```go
type State string
const (
    StateIdle      State = "idle"
    StateStarting  State = "starting"
    StateRunning   State = "running"
    StateCompleted State = "completed"
    StateFailed    State = "failed"
    StateCancelled State = "cancelled"
)

type AgentLifecycle struct {
    AgentName    string
    State        State
    TaskID       string
    PID          int
    StartedAt    time.Time
    Timeout      time.Duration
}

func (l *AgentLifecycle) Transition(to State) error
func (l *AgentLifecycle) CheckTimeout() bool
func (l *AgentLifecycle) IsTerminal() bool   // completed/failed/cancelled
```

**유효 상태 전이**:
```
idle → starting → running → completed
                          → failed
                          → cancelled
```

#### 4-4. 환경변수 해석 (`internal/agent/env.go`)

```go
// config.yml runtime.env + agent frontmatter env 머지
func ResolveEnv(globalEnv, agentEnv map[string]string) map[string]string

// 에이전트 프로세스에 환경변수 주입
func InjectEnv(env map[string]string) []string
```

#### 4-5. Worktree 격리 통합 (`internal/agent/` 내)

```go
// 에이전트 실행 전 worktree 준비
func PrepareWorkDir(
    gitMgr *git.WorktreeManager,
    agent *config.AgentConfig,
    taskBranch string,
    projectDir string,
) (workDir string, cleanup func() error, err error)
```

- `isolation: worktree` → git worktree 생성 후 해당 경로에서 실행
- `isolation: none` → 프로젝트 디렉토리에서 직접 실행
- `cleanup` 함수: `auto_cleanup: true` 시 태스크 완료 후 worktree 제거

### 완료 기준

- `BuildCommand`가 스펙 Section 8의 CLI 형식과 일치하는 명령어 생성
- CLAUDE.md 빌더가 200줄 제한 준수 (초과 시 우선순위 낮은 항목부터 제거)
- 환경변수 우선순위 (agent > config > system) 해석 정확
- 에이전트 상태 전이 유효성 테스트 통과
- worktree 생성/정리 통합 테스트 통과 (`//go:build integration`)

---

## Phase 5: 핵심 파이프라인 (`pylon request`)

> 목표: 사용자 요구사항 입력 → PO 대화 → Architect → PM → 에이전트 실행 → 검증 → PR
> 참조: 스펙 Section 7 "pylon request" 13단계 흐름, Section 8, 9, 10

### 구현 항목

#### 5-1. `pylon request` 전체 흐름 (`internal/cli/request.go`)

> 스펙 참조: Section 7 "pylon request" 실행 흐름

```
사용자: pylon request "로그인 기능 구현해줘"
    ↓
[1] 오케스트레이터 시작 (이미 실행 중이면 연결)
    ↓
[2] 파이프라인 생성: ID = "{YYYYMMDD}-{slug}" (예: 20260305-user-login)
    ↓
[3] Stage: po_conversation
    - PO 에이전트 프로세스 생성 (인터랙티브 모드)
    - 위키 기반 요구사항 분석
    - 사용자와 역질문 대화로 구체화
    - 대화 기록 → .pylon/conversations/{id}/thread.md
    - 수용 기준 확정 → outbox에 result
    ↓
[4] Stage: architect_analysis
    - Architect 에이전트 실행 (비인터랙티브)
    - 기술 방향성 + 프로젝트 간 의존성 분석
    - 결과 → outbox
    ↓
[5] Stage: pm_task_breakdown
    - PM 에이전트 실행 (비인터랙티브)
    - 태스크 분해 → 에이전트 지정 (직렬/병렬)
    - 최종 작업 지시서 → .pylon/tasks/{id}.md
    ↓
[6] Stage: agent_executing
    - 프로젝트 에이전트에게 태스크 할당 (inbox)
    - Worktree 생성 → 브랜치 생성
    - 병렬 실행 (PM 판단 기반, max_concurrent 이내)
    - 완료 시 outbox에 result
    ↓
[7] Stage: verification
    - 교차 검증 (verify.yml: 빌드/테스트/린트)
    - 실패 시 → agent_executing으로 복귀 (max_attempts까지)
    - 성공 시 → 다음 단계
    ↓
[8] Stage: pr_creation
    - gh pr create (reviewer 지정)
    ↓
[9] Stage: po_validation
    - PO가 수용 기준 대비 최종 검증
    ↓
[10] Stage: wiki_update
    - Tech Writer가 위키 자동 업데이트
    ↓
[11] Stage: completed
```

#### 5-2. 대화 기록 관리

> 스펙 참조: Section 9

```go
type ConversationManager struct {
    baseDir string  // .pylon/conversations/
}

func (c *ConversationManager) Create(id string) (*Conversation, error)
func (c *ConversationManager) AppendMessage(id string, role, content string) error
func (c *ConversationManager) SaveMeta(id string, meta ConversationMeta) error
func (c *ConversationManager) Load(id string) (*Conversation, error)

type ConversationMeta struct {
    Status    string   `yaml:"status"`
    StartedAt string   `yaml:"started_at"`
    Projects  []string `yaml:"projects"`
    TaskID    string   `yaml:"task_id"`
}
```

**thread.md 형식** (스펙 Section 9 예시):
```markdown
# 대화: {제목}

## [{timestamp}] 사용자
{메시지}

## [{timestamp}] PO
{응답}
```

#### 5-3. 교차 검증 (`internal/orchestrator/verify.go`)

> 스펙 참조: Section 7 "pylon request" 9단계

```go
type VerifyResult struct {
    Name    string
    Command string
    Success bool
    Output  string
    Elapsed time.Duration
}

func RunVerification(projectDir string, commands []VerifyCommand) ([]VerifyResult, error)
```

**verify.yml 형식**:
```yaml
commands:
  - name: build
    command: go build ./...
    timeout: 5m
  - name: test
    command: go test ./... -race
    timeout: 10m
  - name: lint
    command: golangci-lint run
    timeout: 3m
```

**실패 처리**: 실패 시 에이전트에게 수정 요청 (outbox의 에러 내용 포함). `max_attempts` (기본 2회) 초과 시 `StageFailed` → 사람 에스컬레이션.

#### 5-4. PR 생성 (`internal/git/pr.go`)

> 스펙 참조: Section 13 "Git 브랜치 전략", config.yml `git.pr`

```go
type PRConfig struct {
    Title     string
    Body      string
    Branch    string
    Base      string   // config.yml git.default_base
    Reviewers []string // config.yml git.pr.reviewers
    Draft     bool     // config.yml git.pr.draft
}

func CreatePR(projectDir string, cfg PRConfig) (prURL string, err error)
```

**구현**: `exec.Command("gh", "pr", "create", ...)` 래핑

#### 5-5. 메모리 관리 통합 (`internal/memory/`)

> 스펙 참조: Section 8 "에이전트 메모리 아키텍처", "선제적 + 반응적 메모리 접근"

```go
type MemoryManager struct {
    store *store.Store
    cfg   config.MemoryConfig
}

// 선제적 주입: 태스크 시작 전 관련 메모리 검색
func (m *MemoryManager) GetProactiveContext(taskDesc string, maxTokens int) (string, error)

// 반응적 검색: 에이전트 query 메시지 처리
func (m *MemoryManager) HandleQuery(query string, categories []string) ([]MemoryEntry, error)

// 학습 축적: 에이전트 result의 learnings 처리
func (m *MemoryManager) StoreLearnings(taskID, agent string, learnings []string) error
```

**BM25 검색**: SQLite FTS5의 `project_memory_fts` 테이블 활용
```sql
SELECT pm.* FROM project_memory pm
JOIN project_memory_fts fts ON pm.id = fts.rowid
WHERE project_memory_fts MATCH ?
ORDER BY rank
LIMIT ?
```

#### 5-6. Narrative Casting 핸드오프 (`internal/protocol/handoff.go`)

> 스펙 참조: Section 8 "핸드오프 프로토콜"

```go
func BuildHandoffContext(
    prevResult *MessageEnvelope,
    blackboard []store.BlackboardEntry,
    memories   []store.MemoryEntry,
) *MsgContext
```

**역할**: 이전 에이전트의 결과 + 블랙보드 + 관련 메모리를 다음 에이전트의 `MsgContext.Summary`로 합성

#### 5-7. `pylon cancel` / `pylon resume` / `pylon review`

```go
// cancel: 진행 중인 파이프라인 중단 + 에이전트 프로세스 종료 + worktree 정리
func cancelPipeline(pipelineID string) error

// resume: 중단된 대화 재개 (conversations/ 로드 → PO 재시작)
func resumeConversation(conversationID string) error

// review: PR URL에서 코멘트 읽기 → 에이전트에게 수정 요청
func reviewPR(prURL string) error
```

### 완료 기준

- `pylon request "테스트"` 실행 시 전체 파이프라인 (PO→Architect→PM→에이전트→검증→PR) 동작
- 대화 기록이 `.pylon/conversations/` 에 저장
- 교차 검증 실패 시 에이전트 재시도 + 최대 횟수 초과 시 실패 처리
- PR 생성 + reviewer 자동 지정
- 프로젝트 메모리 BM25 검색 동작
- `pylon cancel`로 진행 중 작업 취소 가능
- `pylon status`가 파이프라인 전체 상태 표시

---

## 테스트 전략 (전 Phase 공통)

### 테스트 계층

| 계층 | 대상 | 방법 | 태그 |
|------|------|------|------|
| 단위 테스트 | config 파싱, 프로토콜, 상태 머신 | 인터페이스 모킹, 테이블 드리븐 | (없음, 기본) |
| SQLite 테스트 | store CRUD, BM25 검색 | 인메모리 DB (`:memory:`) | (없음, 기본) |
| 통합 테스트 | 프로세스 실행, git worktree, CLI | 실제 바이너리 실행 | `//go:build integration` |
| E2E 테스트 | 전체 파이프라인 | (Phase 5 이후) | `//go:build e2e` |

### 테스트 원칙

- **인터페이스 기반 모킹**: executor, git, claude CLI는 인터페이스로 추상화
- **테이블 드리븐 테스트**: Go 관례 `[]struct{ name, input, expected }` 패턴
- **테스트 픽스처**: `testdata/` 에 샘플 config.yml, agent .md, verify.yml
- **커버리지 목표**: 핵심 로직 (config 파싱, 프로토콜, 상태 머신) 80%+

---

## 의존성 목록

| 라이브러리 | 용도 | 도입 Phase |
|-----------|------|-----------|
| `github.com/spf13/cobra` | CLI 프레임워크 | 0 |
| `gopkg.in/yaml.v3` | YAML 파싱 | 0 |
| `github.com/google/uuid` | UUID v7 (메시지 ID) | 0 |
| `modernc.org/sqlite` | CGO-free SQLite | 3 |
| `github.com/fsnotify/fsnotify` | 파일시스템 감시 | 3 |
| `github.com/charmbracelet/bubbletea` | TUI 프레임워크 | 7 |
| `github.com/charmbracelet/lipgloss` | TUI 스타일링 | 7 |

---

## Phase별 완료 체크리스트 요약

### Phase 0 ✅ 조건
- [ ] `make build` 성공
- [ ] `pylon --help` 서브커맨드 11개 표시
- [ ] `pylon version` 빌드 정보 출력
- [ ] `make test && make lint` 통과

### Phase 1 ✅ 조건
- [ ] config.yml 풀/최소/에러 케이스 파싱 테스트 통과
- [ ] agent .md frontmatter+body 파싱 테스트 통과
- [ ] 기본값 상속 (config → agent) 정확
- [ ] `pylon doctor` 4개 도구 검증 동작
- [ ] `pylon init` 워크스페이스 생성 + 파일 구조 검증

### Phase 2 ✅ 조건
- [ ] 에이전트 프로세스 직접 실행/종료/상태 확인 동작
- [ ] `pylon cleanup` 좀비 프로세스 정리
- [ ] `pylon status` 활성 에이전트 표시
- [ ] ProcessExecutor 인터페이스 모킹 테스트 통과

### Phase 3 ✅ 조건
- [ ] SQLite 마이그레이션 + 7개 테이블 CRUD 테스트 통과
- [ ] MessageEnvelope JSON 직렬화/역직렬화 정확
- [ ] inbox/outbox 원자적 파일 쓰기 동작
- [ ] fsnotify outbox 이벤트 감지 동작
- [ ] 파이프라인 상태 전이 유효성 테스트 통과
- [ ] state.json 스냅샷 저장/복원 동작

### Phase 4 ✅ 조건
- [ ] BuildCommand 출력이 스펙 CLI 형식과 일치
- [ ] CLAUDE.md 빌더 200줄 제한 준수
- [ ] 환경변수 우선순위 해석 정확
- [ ] 에이전트 상태 전이 테스트 통과
- [ ] worktree 생성/정리 동작

### Phase 5 ✅ 조건
- [ ] `pylon request` 전체 파이프라인 동작
- [ ] 대화 기록 저장 (thread.md + meta.yml)
- [ ] 교차 검증 성공/실패/재시도 처리
- [ ] PR 생성 + reviewer 지정
- [ ] BM25 메모리 검색 동작
- [ ] `pylon cancel` 작업 취소
- [ ] `pylon status` 파이프라인 전체 상태 표시
