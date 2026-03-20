# Pylon 오케스트레이션 & 대시보드 개선 제안서

> **작성일**: 2026-03-19
> **참조**: [Frequency Autonomous Software Factory](https://www.frequency.sh/blog/autonomous-software-factory/)
> **목적**: Frequency의 프로덕션 검증된 패턴을 참고하여 Pylon의 오케스트레이션과 대시보드를 고도화하기 위한 구체적 개선안

---

## 목차

1. [현재 상태 요약](#1-현재-상태-요약)
2. [Frequency vs Pylon 비교 분석](#2-frequency-vs-pylon-비교-분석)
3. [오케스트레이션 개선 제안](#3-오케스트레이션-개선-제안)
4. [대시보드 개선 제안](#4-대시보드-개선-제안)
5. [구현 우선순위 로드맵](#5-구현-우선순위-로드맵)

---

## 1. 현재 상태 요약

### Pylon 오케스트레이션 (Phase 0)

- **파이프라인**: 12단계 고정 state machine (init → completed/failed)
- **동시성**: `errgroup.SetLimit(maxConcurrent)` 기반 wave 병렬 실행
- **재시도**: `MaxAttempts` (기본 2회) 단순 재시도, 백오프 없음
- **상태 관리**: SQLite 단일 테이블 (`pipeline_state`)에 JSON 스냅샷 저장
- **에이전트 통신**: 파일 기반 inbox/outbox (`{taskID}.task.json` / `{taskID}.result.json`)
- **실패 처리**: verification 실패 시 agent_executing으로 루프백, terminal 도달 시 복구 불가

### Pylon 대시보드

- **스택**: HTMX + SSE + Go 템플릿, `go:embed`
- **페이지**: Overview, Pipeline Detail, Messages, Memory (4개)
- **실시간**: 1초 SQLite 폴링 → SSE broadcast
- **제어**: 파이프라인 강제 취소 (best-effort DB 업데이트)만 가능
- **메트릭**: Total/Active/Completed/Failed 카운트, Success Rate

### Frequency (참조 시스템)

- **파이프라인**: YAML 기반 config-driven state machine, 복수 workflow 동시 운영
- **동시성**: WIP 제한 (글로벌/스텝별/상태별), greedy scheduler
- **재시도**: retryable/terminal 분류, 스텝별 15-90초 백오프
- **상태 관리**: Git-backed (파일 기반, 리포 내 상태 저장)
- **워커**: 멀티모델 (Claude + Codex), 용량 퍼센트 추적
- **대시보드**: 워크플로 RUN/PAUSE/STOP, 서브젝트별 상태 추적, 메트릭 (throughput, failure rate, WIP, lead time, DLQ)

---

## 2. Frequency vs Pylon 비교 분석

| 영역 | Frequency | Pylon (현재) | 갭 |
|------|-----------|-------------|-----|
| **워크플로 정의** | YAML state machine, 복수 워크플로 | 12단계 하드코딩 | 유연한 워크플로 정의 필요 |
| **복수 파이프라인** | 10개 동시 운영, 카테고리별 그룹 | 단일 파이프라인 포커스 | 멀티 파이프라인 관리 |
| **워커 용량** | Claude 57%, Codex 52% 실시간 추적 | Active Agents 카운트만 | 모델별 용량 추적 |
| **스케줄링** | Greedy scheduler + WIP limits | 단순 errgroup limit | 큐 기반 스케줄링 |
| **재시도/DLQ** | 스텝별 백오프 + DLQ + 자동 분류 | 고정 MaxAttempts | 지능형 재시도 |
| **파이프라인 제어** | RUN/PAUSE/STOP per workflow | Cancel만 가능 | Pause/Resume 필요 |
| **서브젝트 추적** | 서브젝트별 상태/스텝/시간/에러 | 파이프라인 레벨만 | 세분화된 추적 |
| **메트릭** | Throughput, Failure Rate, WIP, Lead Time P50 | 기본 카운트만 | 고급 메트릭 |
| **크로스 파이프라인** | `requires` 절로 의존성 선언 | 없음 | 파이프라인 간 조율 |
| **용량 게이팅** | 다운스트림 큐 깊이 체크 | 없음 | 백프레셔 |

---

## 3. 오케스트레이션 개선 제안

### 3.1 파이프라인 Pause/Resume 지원

**현재 상태**: `handleAPIPipelineCancel`로 강제 failed 전환만 가능. 일시 중지 후 재개 불가.

**목표 상태**: 파이프라인을 일시 중지하면 현재 실행 중인 에이전트 완료 후 대기, Resume 시 이어서 실행.

**구현 방향**:
```go
// domain/stage.go에 Paused 상태 추가 (메타 상태)
type PipelineStatus string
const (
    StatusRunning PipelineStatus = "running"
    StatusPaused  PipelineStatus = "paused"
)

// Pipeline 구조체에 Status 필드 추가
type Pipeline struct {
    // ...existing fields
    Status PipelineStatus
    PausedAtStage Stage  // pause 시점의 stage 기록
}
```

- `loop.go`의 메인 루프에서 매 스테이지 전환 전 `pipeline.Status == StatusPaused` 체크
- Pause 시 현재 실행 중인 에이전트는 완료까지 대기 (graceful)
- Resume 시 `PausedAtStage`부터 재개
- 대시보드 API: `POST /api/pipelines/{id}/pause`, `POST /api/pipelines/{id}/resume`

**Frequency 참고**: 워크플로별 PAUSE 버튼으로 개별 워크플로 일시 중지 가능

---

### 3.2 지능형 재시도 & Dead Letter Queue (DLQ)

**현재 상태**: `MaxAttempts` (기본 2)까지 단순 재시도. 실패 원인 구분 없음. 실패 히스토리 추적 불가.

**목표 상태**: 실패를 retryable/terminal로 분류하고, 스텝별 백오프 적용, DLQ에 terminal 실패 기록.

**구현 방향**:

```go
// orchestrator/retry.go (신규)
type FailureClass string
const (
    FailureRetryable FailureClass = "retryable"  // 타임아웃, 일시적 에러
    FailureTerminal  FailureClass = "terminal"   // 구문 오류, 불가능한 작업
)

type RetryPolicy struct {
    MaxAttempts    int
    InitialBackoff time.Duration  // 기본 15초
    MaxBackoff     time.Duration  // 기본 90초
    BackoffFactor  float64        // 기본 2.0
}

type DLQEntry struct {
    PipelineID  string
    TaskID      string
    Stage       string
    Error       string
    OutputTail  string    // 마지막 500줄
    FailedAt    time.Time
    Requeued    bool
}
```

- verification 실패 시 에러 메시지 분석으로 retryable/terminal 자동 분류
- retryable: 지수 백오프로 재시도 (15s → 30s → 60s → 90s cap)
- terminal: DLQ 테이블에 기록, 대시보드에서 수동 requeue 가능
- `store/dlq.go`: DLQ CRUD 쿼리

**Frequency 참고**: 스텝별 15-90초 백오프, append-only DLQ 감사 추적, 대시보드에서 RETRY/DLQ 카운트 표시

---

### 3.3 멀티 파이프라인 동시 관리

**현재 상태**: 단일 파이프라인 포커스. `ListAllPipelines()`로 과거 파이프라인 조회 가능하나, 동시 실행 설계 아님.

**목표 상태**: 복수 파이프라인 동시 실행, 파이프라인 간 리소스 조율.

**구현 방향**:

```go
// orchestrator/scheduler.go (신규)
type Scheduler struct {
    store         DashboardStore
    maxGlobal     int              // 전체 동시 에이전트 수 제한
    maxPerPipeline int             // 파이프라인당 에이전트 수 제한
    activeAgents  atomic.Int32
    pipelines     map[string]*Loop // pipelineID → Loop
}

func (s *Scheduler) Submit(pipeline *Pipeline) error
func (s *Scheduler) PauseAll()
func (s *Scheduler) ResumeAll()
func (s *Scheduler) ActiveCount() int
```

- 각 파이프라인은 독립 Loop 고루틴으로 실행
- Scheduler가 전역 에이전트 수 제한 관리 (세마포어)
- 파이프라인 간 리소스 경쟁 방지: worktree 격리 이미 존재
- config.yml에 `runtime.max_pipelines` 추가

**Frequency 참고**: 10개 파이프라인 동시 운영, CREATE/SHIP 카테고리로 그룹핑

---

### 3.4 WIP 제한 & 백프레셔

**현재 상태**: `max_concurrent`로 에이전트 수만 제한. 스테이지별/파이프라인별 WIP 제한 없음.

**목표 상태**: 다단계 WIP 제한으로 시스템 과부하 방지.

**구현 방향**:

```yaml
# config.yml 확장
runtime:
  max_concurrent: 5          # 전역 에이전트 수 제한 (기존)
  max_pipelines: 3            # 동시 파이프라인 수 제한 (신규)
  wip_limits:                 # 스테이지별 WIP 제한 (신규)
    agent_executing: 3
    verification: 2
    pr_creation: 1
  backpressure:               # 백프레셔 (신규)
    enabled: true
    queue_depth_threshold: 10 # 큐 깊이 임계치
```

- 스테이지 전환 전 해당 스테이지의 WIP 카운트 확인
- 임계치 초과 시 대기 (ticker + context 기반)
- 백프레셔: 다운스트림 스테이지가 포화되면 업스트림 진행 억제

**Frequency 참고**: 글로벌/스텝별/상태별 WIP 제한, ideas 파이프라인이 build 큐 깊이 체크 후 생성 중단

---

### 3.5 에이전트 타임아웃 강제 적용

**현재 상태**: `config.runtime.task_timeout: 30m` 설정 존재하나 실제 적용 안 됨 (`runner.go`에 타임아웃 로직 없음).

**목표 상태**: 에이전트 실행에 컨텍스트 기반 타임아웃 적용, 초과 시 강제 종료.

**구현 방향**:

```go
// agent/runner.go - RunHeadless에 타임아웃 컨텍스트 적용
func (r *Runner) RunHeadless(ctx context.Context, agentCfg AgentConfig, ...) (*ExecResult, error) {
    timeout := r.config.Runtime.TaskTimeout
    if agentCfg.Timeout > 0 {
        timeout = agentCfg.Timeout  // 에이전트별 오버라이드
    }

    ctx, cancel := context.WithTimeout(ctx, timeout)
    defer cancel()

    // cmd.Process.Kill() on context cancellation
    // ...
}
```

- 에이전트 YAML에 `timeout: 15m` 오버라이드 지원
- 타임아웃 발생 시 retryable 실패로 분류 (DLQ 아닌 재시도 대상)
- Stale worktree 자동 정리: 타임아웃 + 버퍼 후 삭제

**Frequency 참고**: "stale worktrees older than their step's timeout plus a buffer" 자동 정리

---

### 3.6 워커 용량 추적 & 멀티모델 지원

**현재 상태**: 모든 에이전트가 단일 모델 사용 (`config.runtime.backend`). 모델별 사용량 추적 없음.

**목표 상태**: Claude/Codex 등 멀티모델 워커풀, 모델별 용량 퍼센트 실시간 추적.

**구현 방향**:

```go
// orchestrator/capacity.go (신규)
type WorkerPool struct {
    mu       sync.RWMutex
    capacity map[string]*ModelCapacity  // "claude" → capacity
}

type ModelCapacity struct {
    Model       string
    MaxWorkers  int
    Active      int
    Utilization float64  // 0.0 ~ 1.0
}

func (wp *WorkerPool) Acquire(model string) (release func(), err error)
func (wp *WorkerPool) Utilization() map[string]float64
```

- 에이전트 YAML의 `model` 필드로 모델 선택 (기존)
- 모델별 동시 실행 수 제한 및 사용률 추적
- 대시보드에 모델별 용량 게이지 표시 (Frequency의 Claude 57%, Codex 52%)

**Frequency 참고**: Worker Capacity 섹션에 모델별 사용률 바 표시

---

### 3.7 크로스 파이프라인 의존성

**현재 상태**: 파이프라인 간 의존성 표현 수단 없음.

**목표 상태**: 파이프라인 A의 특정 스테이지 완료를 파이프라인 B의 전제조건으로 선언.

**구현 방향**:

```yaml
# 파이프라인 정의에 requires 절 추가
pipeline:
  name: deploy
  requires:
    - pipeline: build
      stage: completed
    - pipeline: test
      stage: verification
```

- Scheduler가 requires 절 평가 후 파이프라인 시작 결정
- 순환 의존성 감지 (DAG 검증)
- 대시보드에 파이프라인 의존성 그래프 시각화

**Frequency 참고**: `requires` 절로 `subject_state` 및 `states: [promoted]` 조건 선언

---

## 4. 대시보드 개선 제안

### 4.1 워크플로 제어 패널 (RUN/PAUSE/STOP)

**현재 상태**: Cancel 버튼만 존재 (Pipeline Detail 페이지). 전체 제어 불가.

**목표 상태**: Frequency처럼 개별 파이프라인 RUN/PAUSE/STOP + 전체 RUN ALL/PAUSE ALL/STOP ALL.

**구현 방향**:

```html
<!-- templates/partials/pipeline_controls.html -->
<div class="pipeline-controls">
    {{if .IsActive}}
    <button class="btn btn-warning" hx-post="/api/pipelines/{{.ID}}/pause">PAUSE</button>
    <button class="btn btn-danger" hx-post="/api/pipelines/{{.ID}}/stop">STOP</button>
    {{else if .IsPaused}}
    <button class="btn btn-success" hx-post="/api/pipelines/{{.ID}}/resume">RUN</button>
    <button class="btn btn-danger" hx-post="/api/pipelines/{{.ID}}/stop">STOP</button>
    {{end}}
</div>

<!-- overview.html 상단 -->
<div class="global-controls">
    <button class="btn btn-success" hx-post="/api/pipelines/run-all">▶ RUN ALL</button>
    <button class="btn btn-warning" hx-post="/api/pipelines/pause-all">⏸ PAUSE ALL</button>
    <button class="btn btn-danger" hx-post="/api/pipelines/stop-all">■ STOP ALL</button>
</div>
```

- 새 API 엔드포인트: `POST /api/pipelines/{id}/pause`, `/resume`, `/stop`
- 전역 제어: `POST /api/pipelines/run-all`, `/pause-all`, `/stop-all`
- SSE 이벤트: `pipeline_paused`, `pipeline_resumed` 추가

**Frequency 참고**: 사이드바에 RUN ALL / PAUSE ALL / STOP ALL, 개별 워크플로에 RUN/STOP 버튼

---

### 4.2 고급 메트릭 대시보드

**현재 상태**: Total/Active/Completed/Failed 카운트 + Success Rate만 표시.

**목표 상태**: Throughput, Failure Rate, WIP, Lead Time P50, Retry/DLQ 실시간 추적.

**구현 방향**:

```go
// store/metrics.go (확장)
type AdvancedMetrics struct {
    // 기존
    TotalPipelines     int
    CompletedPipelines int
    FailedPipelines    int
    ActivePipelines    int
    SuccessRate        float64

    // 신규
    Throughput24h    int           // 최근 24시간 완료 수
    FailureRate      float64       // 실패율 (%)
    WIP              int           // 현재 진행 중인 작업 수
    LeadTimeP50      time.Duration // 완료까지 중앙값 소요시간
    LeadTimeP90      time.Duration // 90th percentile
    RetryCount       int           // 재시도 중인 작업 수
    DLQCount         int           // DLQ 대기 항목 수
    AvgStageTime     map[string]time.Duration // 스테이지별 평균 소요시간
}
```

```html
<!-- templates/partials/advanced_metrics.html -->
<div class="card-grid metrics-row">
    <div class="metric-card">
        <div class="metric-label">THROUGHPUT 24H</div>
        <div class="metric-value">{{.Metrics.Throughput24h}}</div>
    </div>
    <div class="metric-card">
        <div class="metric-label">FAILURE RATE</div>
        <div class="metric-value">{{printf "%.0f" .Metrics.FailureRate}}%</div>
    </div>
    <div class="metric-card">
        <div class="metric-label">WIP</div>
        <div class="metric-value">{{.Metrics.WIP}}</div>
    </div>
    <div class="metric-card">
        <div class="metric-label">LEAD TIME P50</div>
        <div class="metric-value">{{.Metrics.LeadTimeP50}}</div>
    </div>
    <div class="metric-card">
        <div class="metric-label">RETRY / DLQ</div>
        <div class="metric-value">{{.Metrics.RetryCount}} / {{.Metrics.DLQCount}}</div>
    </div>
</div>
```

- `pipeline_state` 테이블에 `created_at` 컬럼 추가 (Lead Time 계산용)
- SQL 윈도우 함수로 P50/P90 계산
- SSE로 메트릭 주기적 업데이트 (5초 간격)

**Frequency 참고**: THROUGHPUT 24H, FAILURE RATE, WIP, LEAD TIME P50, TERMINAL, RETRY/DLQ 6개 메트릭 카드

---

### 4.3 워커 용량 사이드바

**현재 상태**: Active Agents 카운트만 표시 (단일 숫자).

**목표 상태**: 모델별 사용률 프로그레스 바, 워크스페이스 정보 표시.

**구현 방향**:

```html
<!-- templates/partials/sidebar.html (신규) -->
<aside class="sidebar">
    <div class="sidebar-section">
        <h3>WORKER CAPACITY</h3>
        {{range .Workers}}
        <div class="worker-row">
            <span class="worker-model">{{.Model}}</span>
            <div class="capacity-bar">
                <div class="capacity-fill" style="width: {{.Utilization}}%"></div>
            </div>
            <span class="capacity-pct {{if gt .Utilization 80.0}}capacity-high{{end}}">
                {{printf "%.0f" .Utilization}}%
            </span>
        </div>
        {{end}}
    </div>

    <div class="sidebar-section">
        <span class="workspace-path">{{workspaceName}}</span>
    </div>

    <div class="sidebar-section">
        <h3>PIPELINES</h3>
        <div class="filter-tabs">
            <button class="tab active">ALL</button>
            <button class="tab">RUNNING</button>
            <button class="tab">PAUSED</button>
        </div>
        {{range .Pipelines}}
        <div class="pipeline-sidebar-item">
            <span class="status-dot {{statusClass .CurrentStage}}"></span>
            <span>{{truncate .TaskSpec 30}}</span>
            <button class="btn-sm">{{if .IsActive}}STOP{{else}}RUN{{end}}</button>
        </div>
        {{end}}
    </div>
</aside>
```

- 레이아웃 변경: 사이드바 + 메인 콘텐츠 2컬럼 구조
- 사이드바에 워커 용량, 파이프라인 목록, 글로벌 제어 배치
- 메인 영역에 선택된 파이프라인 상세 표시

**Frequency 참고**: 좌측 사이드바에 Worker Capacity (Claude/Codex 프로그레스 바), 워크플로 목록, 글로벌 제어

---

### 4.4 파이프라인 스테이지 시각화 개선

**현재 상태**: `stage_progress.html`에 스테이지 점(dot) 나열. 각 스테이지의 서브젝트 수 표시 없음.

**목표 상태**: Frequency처럼 수평 파이프라인 바에 각 스테이지별 항목 수 표시.

**구현 방향**:

```html
<!-- templates/partials/pipeline_stages_bar.html (신규) -->
<div class="pipeline-bar">
    {{range .Stages}}
    <div class="stage-column {{if eq .Name $.CurrentStage}}stage-active{{end}}
                             {{if .IsTerminal}}stage-terminal{{end}}">
        <div class="stage-count {{if gt .Count 0}}has-items{{end}}">
            {{.Count}}
        </div>
        <div class="stage-connector"></div>
        <div class="stage-label">{{stageLabel .Name}}</div>
    </div>
    {{end}}
</div>
```

- 멀티 파이프라인 시 각 파이프라인 내 태스크가 어떤 스테이지에 있는지 시각화
- 스테이지 간 진행 방향 화살표/커넥터
- 활성 스테이지 강조 (초록 하이라이트)
- 터미널 스테이지 (completed/failed) 카운트 표시

**Frequency 참고**: 수평 PIPELINE 바에 SELECTED(0) → IMPLEMENTED(0) → ... → PROMOTED(30) 카운트 표시

---

### 4.5 서브젝트/태스크 테이블 뷰

**현재 상태**: Pipeline Detail에 Task Graph 테이블 있으나, 실행 상태/시간/에러 정보 없음.

**목표 상태**: 각 태스크의 상태, 현재 스텝, 소요 시간, 에러, 변경 파일 수 추적.

**구현 방향**:

```go
// handler.go - TaskItemView 확장
type TaskItemView struct {
    ID          string
    Description string
    AgentName   string
    DependsOn   []string
    // 신규 필드
    State       string        // queued, running, completed, failed
    CurrentStep string        // 현재 실행 중인 스텝
    Duration    time.Duration // 소요 시간
    Error       string        // 에러 메시지 (실패 시)
    FilesCount  int           // 변경된 파일 수
    Score       float64       // 품질 점수 (verification 결과)
}
```

```html
<!-- templates/partials/task_table.html (확장) -->
<table class="table task-table">
    <thead>
        <tr>
            <th>TASK</th>
            <th>STATE</th>
            <th>STEP</th>
            <th>SCORE</th>
            <th>TIME</th>
            <th>ERROR</th>
            <th>FILES</th>
            <th>ACTION</th>
        </tr>
    </thead>
    <tbody>
        {{range .Pipeline.TaskGraph}}
        <tr>
            <td>{{.Description}}</td>
            <td><span class="badge {{statusClass .State}}">{{.State}}</span></td>
            <td><em>{{.CurrentStep}}</em></td>
            <td>{{if .Score}}{{printf "%.1f" .Score}}{{else}}–{{end}}</td>
            <td>{{.Duration}}</td>
            <td>{{if .Error}}<span class="error-text">{{truncate .Error 50}}</span>{{else}}–{{end}}</td>
            <td>{{.FilesCount}}</td>
            <td>
                {{if eq .State "failed"}}
                <button class="btn-sm btn-warning" hx-post="/api/tasks/{{.ID}}/retry">Retry</button>
                {{end}}
            </td>
        </tr>
        {{end}}
    </tbody>
</table>
```

- 필터 탭: ALL / FAILED / COMPLETED + 검색
- 실패 태스크 개별 재시도 버튼
- SSE로 태스크 상태 변경 실시간 반영

**Frequency 참고**: SUBJECT, STATE, STEP, SCORE, TIME, ERROR, FILES, ACTION 8컬럼 테이블, 필터 탭 (ALL/FAILED/PROMOTED)

---

### 4.6 DLQ (Dead Letter Queue) 페이지

**현재 상태**: 실패한 파이프라인은 terminal 상태로 전환되어 조회만 가능. 재처리 수단 없음.

**목표 상태**: DLQ 전용 페이지에서 실패 원인 분석, 수동 requeue 가능.

**구현 방향**:

```html
<!-- templates/dlq.html (신규) -->
<div class="container">
    <h1>Dead Letter Queue</h1>
    <div class="card-grid">
        <div class="metric-card">
            <div class="metric-label">DLQ Items</div>
            <div class="metric-value failed-text">{{len .DLQEntries}}</div>
        </div>
        <div class="metric-card">
            <div class="metric-label">Requeued Today</div>
            <div class="metric-value">{{.RequeuedToday}}</div>
        </div>
    </div>

    {{range .DLQEntries}}
    <div class="card dlq-card">
        <div class="dlq-header">
            <span class="badge badge-failed">{{.Stage}}</span>
            <span>{{.PipelineID}} / {{.TaskID}}</span>
            <span class="timeline-time">{{timeAgo .FailedAt}}</span>
        </div>
        <div class="dlq-error">
            <pre>{{.Error}}</pre>
        </div>
        <div class="dlq-output">
            <details>
                <summary>Output Tail (last 500 lines)</summary>
                <pre>{{.OutputTail}}</pre>
            </details>
        </div>
        <div class="dlq-actions">
            <button class="btn btn-primary" hx-post="/api/dlq/{{.ID}}/requeue">Requeue</button>
            <button class="btn btn-secondary" hx-delete="/api/dlq/{{.ID}}">Dismiss</button>
        </div>
    </div>
    {{end}}
</div>
```

- 네비게이션에 DLQ 링크 추가 (배지로 카운트 표시)
- 실패 원인별 그룹핑 (타임아웃, 빌드 실패, 테스트 실패 등)
- Requeue 시 해당 스테이지부터 재시작

**Frequency 참고**: DLQ에 실패 원인 + output tail 저장, 대시보드에서 requeue 가능, RETRY/DLQ 메트릭 카드

---

### 4.7 실시간 에이전트 로그 스트리밍

**현재 상태**: 에이전트 실행 결과만 outbox에서 확인. 실행 중 로그 조회 불가.

**목표 상태**: 실행 중인 에이전트의 stdout/stderr 실시간 스트리밍.

**구현 방향**:

- 에이전트 실행 시 로그를 `.pylon/runtime/logs/{agentName}-{taskID}.log`에 기록 (tee)
- 대시보드 API: `GET /api/agents/{name}/logs?follow=true` (SSE 스트림)
- Pipeline Detail 페이지에서 에이전트 카드 클릭 → 로그 패널 슬라이드

```go
// dashboard/handler.go
func (srv *Server) handleAgentLogs(w http.ResponseWriter, r *http.Request) {
    agentName := chi.URLParam(r, "name")
    logPath := filepath.Join(runtimeDir, "logs", agentName+".log")

    if r.URL.Query().Get("follow") == "true" {
        // tail -f 스타일 SSE 스트리밍
        srv.streamLogFile(w, r, logPath)
    } else {
        // 최근 N줄 반환
        srv.readLogTail(w, logPath, 200)
    }
}
```

**Frequency 참고**: "per-step logs and failure root cause analysis" 기능

---

## 5. 구현 우선순위 로드맵

### Phase 1: 핵심 제어 & 가시성 (2-3주)

| 우선순위 | 항목 | 영향도 | 난이도 |
|---------|------|-------|-------|
| P0 | 3.5 에이전트 타임아웃 강제 적용 | 높음 (안정성) | 낮음 |
| P0 | 4.2 고급 메트릭 대시보드 | 높음 (가시성) | 중간 |
| P1 | 3.1 파이프라인 Pause/Resume | 높음 (제어) | 중간 |
| P1 | 4.1 워크플로 제어 패널 | 높음 (UX) | 중간 |
| P1 | 4.4 파이프라인 스테이지 시각화 | 중간 (가시성) | 낮음 |

### Phase 2: 안정성 & 운영 (3-4주)

| 우선순위 | 항목 | 영향도 | 난이도 |
|---------|------|-------|-------|
| P1 | 3.2 지능형 재시도 & DLQ | 높음 (안정성) | 높음 |
| P1 | 4.5 서브젝트/태스크 테이블 뷰 | 중간 (가시성) | 중간 |
| P1 | 4.6 DLQ 페이지 | 중간 (운영) | 중간 |
| P2 | 4.7 실시간 에이전트 로그 | 중간 (디버깅) | 중간 |

### Phase 3: 확장성 (4-6주)

| 우선순위 | 항목 | 영향도 | 난이도 |
|---------|------|-------|-------|
| P2 | 3.3 멀티 파이프라인 동시 관리 | 높음 (확장) | 높음 |
| P2 | 3.4 WIP 제한 & 백프레셔 | 중간 (안정성) | 중간 |
| P2 | 3.6 워커 용량 추적 & 멀티모델 | 중간 (운영) | 중간 |
| P2 | 4.3 워커 용량 사이드바 | 중간 (UX) | 중간 |
| P3 | 3.7 크로스 파이프라인 의존성 | 낮음 (고급) | 높음 |

---

## 부록: Frequency 대시보드 UI 분석 (스크린샷 기반)

스크린샷에서 관찰된 Frequency 대시보드의 구체적 UI 요소:

### 좌측 사이드바
- **Worker Capacity**: Claude (57%, 빨간 바), Codex (52%, 빨간 바)
- **워크스페이스 경로**: `~/app-factory`
- **라인 수**: "10 lines" + "+ ADD REPO" 버튼
- **글로벌 제어**: ▶ RUN ALL / ⏸ PAUSE ALL / ■ STOP ALL
- **필터**: Filter lines 입력, ALL/RUNNING/PAUSED 탭
- **워크플로 그룹**:
  - CREATE 2: `build` (● 실행중, 30/30), `ideas` (○ 정지, 0/60)
  - SHIP 4: `deploy` (● 실행중, 29/30), `marketing` (● 실행중, 27/30), `release` (● 실행중, 0/30), `release-shared` (● 실행중, 0/30)
- 각 워크플로에 프로그레스 바 + RUN/STOP 버튼

### 메인 영역 (build 워크플로 선택 시)
- **헤더**: `build` workflow / build, ▶ RUN / ⏸ PAUSE / ■ STOP
- **설명**: "Implement selected ideas, run review/build/SEO checks, and promote to implemented."
- **메타**: YAML 경로, PID, 시작 시간
- **PIPELINE 바**: 10개 스테이지 수평 나열, 각 스테이지에 숫자 카운트
  - SELECTED(0) → IMPLEMENTED(0) → FAILED(0) → READY_FOR_BUILD(0) → BUILT(0) → BUILD_FIX_NEEDED(0) → SEO_READY(0) → PROMOTED_CANDID..(0) → PROMOTED_CANDID..(0) → SHIP_PENDING(0) → PROMOTED(30, 초록 하이라이트)
- **메트릭 카드**: THROUGHPUT 24H(3), FAILURE RATE(0%), WIP(—), LEAD TIME P50(9.2h), TERMINAL(30), RETRY/DLQ(0/0)
- **필터 탭**: ALL 30 / FAILED 0 / PROMOTED 30 + Search subjects
- **서브젝트 테이블**: SUBJECT, STATE, STEP, SCORE, TIME, ERROR, FILES, ACTION 8컬럼
  - 모든 서브젝트가 `promoted` 상태, `mark_ready_to_ship` 또는 `depth_rebuild` 스텝
  - 소요 시간: 5m ~ 8.6h 범위

이 UI 패턴들을 Pylon 대시보드에 선택적으로 적용하되, Pylon의 12단계 파이프라인 모델에 맞게 조정해야 합니다.
