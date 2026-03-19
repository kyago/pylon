# Pylon 고도화 통합 전략

> **작성일**: 2026-03-19
> **입력 문서**:
> - `adaptive-workflow-with-swarm-pattern.md` (이하 **WF 제안**)
> - `orchestration-and-dashboard-improvements.md` (이하 **OD 제안**)
> **목적**: 두 제안을 하나의 일관된 로드맵으로 통합하고, 의존성·충돌을 해소하며, 미결 사항을 명시

---

## 1. 교차 분석: 중복·충돌·의존성

### 1.1 중복 영역

| 영역 | WF 제안 | OD 제안 | 통합 방향 |
|------|---------|---------|-----------|
| **`pipeline.go` 전환 맵** | `BuildTransitions()` 동적 생성 | `StatusPaused` 메타 상태 추가 | 동적 전환 맵 + Paused 상태를 하나의 변경으로 통합 |
| **`loop.go` 메인 루프** | 워크플로우 기반 단계 필터링 | 매 전환 전 Pause/WIP 체크 | 루프 진입부에 `checkPreconditions()` 게이트 추가 (pause 체크 + WIP 체크 + 워크플로우 필터 통합) |
| **`config.go` 확장** | `workflow` 섹션 추가 | `wip_limits`, `backpressure` 섹션 추가 | 동일 Config 구조체에 병합 |
| **멀티 모델/백엔드** | `AgentRunner` 인터페이스 + `GenericCLIRunner` | 워커 용량 추적 `WorkerPool` | `AgentRunner` 인터페이스가 기반 → 그 위에 `WorkerPool`이 용량 관리 |
| **대시보드 변경** | (대시보드 변경 없음) | 사이드바, 메트릭, 제어패널 등 7항목 | OD 제안이 대시보드 전담, WF 제안은 워크플로우 정보를 대시보드에 노출하는 연동만 필요 |

### 1.2 충돌 지점

| 충돌 | 상세 | 해소 방안 |
|------|------|-----------|
| **Pipeline 구조체 동시 변경** | WF: `WorkflowName string` 추가, OD: `Status PipelineStatus` + `PausedAtStage Stage` 추가 | **한 번에 통합**: 두 필드 세트를 동시에 추가. JSON 직렬화 키 충돌 없음 |
| **`request.go` 하드코딩 전환** | WF: 동적 `NextStageAfter()` 호출로 변경, OD: Pause 시 전환 차단 | **WF 변경이 선행**: 동적 전환이 먼저 적용되어야 Pause 게이트가 그 위에 올라감 |
| **`runner.go` 변경 범위** | WF: `AgentRunner` 인터페이스 추출 + 디스패치, OD: 타임아웃 컨텍스트 적용 | **순서**: 타임아웃(OD)을 먼저 기존 Runner에 적용 → 인터페이스 추출(WF) 시 타임아웃 로직을 인터페이스 계약에 포함 |
| **Task Graph 존재 여부** | WF: `skip_task_graph: true`인 워크플로우는 TaskGraph 미생성, OD: 태스크 테이블 뷰가 TaskGraph 존재 가정 | **[미결 #1]** 참조 |

### 1.3 의존성 그래프

```
Phase 0 (기반)
  ├─ 에이전트 타임아웃 적용 (OD 3.5) ─── 독립, 즉시 가능
  └─ Pipeline 구조체 통합 확장 ────────── WF 1-5 + OD 3.1 병합

Phase 1 (워크플로우 코어)
  ├─ WorkflowTemplate + BuildTransitions (WF 1-1~1-4)
  ├─ request.go 동적 전환 (WF 1-6) ──── Phase 0 Pipeline 확장에 의존
  └─ loop.go 워크플로우 주입 (WF 1-7)

Phase 2 (운영 안정성)
  ├─ Pause/Resume (OD 3.1) ──────────── Phase 1에 의존 (동적 전환 맵 위에 구현)
  ├─ 지능형 재시도 & DLQ (OD 3.2) ──── Phase 0 타임아웃에 의존
  ├─ 고급 메트릭 (OD 4.2) ──────────── 독립
  └─ 워크플로 제어 패널 (OD 4.1) ───── Pause/Resume에 의존

Phase 3 (가시성 & UX)
  ├─ 워크플로우 자동 선택 (WF 2-1~2-3) ── Phase 1에 의존
  ├─ 스테이지 시각화 (OD 4.4) ─────────── Phase 1에 의존 (워크플로우별 단계 다름)
  ├─ 태스크 테이블 뷰 (OD 4.5) ────────── [미결 #1] 해소 후
  └─ DLQ 페이지 (OD 4.6) ─────────────── Phase 2 DLQ에 의존

Phase 4 (확장성)
  ├─ AgentRunner 인터페이스 (WF 3-1~3-3) ── Phase 0 타임아웃 포함
  ├─ 멀티 파이프라인 (OD 3.3) ────────────── Phase 2 Pause/Resume에 의존
  ├─ WIP 제한 & 백프레셔 (OD 3.4) ────────── 멀티 파이프라인에 의존
  ├─ 워커 용량 추적 (OD 3.6) ──────────────── AgentRunner 인터페이스에 의존
  └─ Transport 추상화 (WF 4-1~4-2) ────────── 독립

Phase 5 (고급)
  ├─ 크로스 파이프라인 의존성 (OD 3.7) ──── 멀티 파이프라인에 의존
  ├─ 실시간 에이전트 로그 (OD 4.7) ────────── 독립
  └─ Swarm 템플릿 (WF swarm.yml) ──────────── AgentRunner + 동적 생성에 의존
```

---

## 2. 통합 로드맵

### Phase 완료 현황

| Phase | 상태 | 커밋 | 날짜 |
|-------|------|------|------|
| Phase 0: 기반 강화 | ✅ 완료 | `8e43d9b` | 2026-03-19 |
| Phase 1: 적응형 워크플로우 | ✅ 완료 | `63a8e60` | 2026-03-19 |
| Phase 2: 운영 안정성 | ✅ 완료 | `2654980` | 2026-03-19 |
| Phase 3: 가시성 & UX | ✅ 완료 | - | 2026-03-19 |
| Phase 4: 확장성 | ⬜ 미착수 | - | - |
| Phase 5: 고급 기능 | ⬜ 미착수 | - | - |

---

#### Phase 2 완료 체크리스트

- [x] **2-1 Pause/Resume**
  - [x] `Pipeline.Pause()/Resume()/IsPaused()` 메서드 (`pipeline.go`)
  - [x] `checkPreconditions()` 게이트 — 매 루프 반복 전 Status 체크, Paused면 1초 폴링 대기 (`loop.go`)
  - [x] `POST /api/pipelines/{id}/pause` — 이미 paused면 400 반환 (`handler.go`)
  - [x] `POST /api/pipelines/{id}/resume` — paused가 아니면 400 반환 (`handler.go`)
  - [x] 라우터 등록 (`server.go`)

- [x] **2-2 지능형 재시도**
  - [x] `FailureClass` 타입: Retryable, Terminal, Unknown (`retry.go` 신규)
  - [x] `ClassifyFailure()` — retryable 패턴 16종, terminal 패턴 13종 (`retry.go`)
  - [x] `RetryPolicy` 지수 백오프 — `NextDelay(attempt)`, `DefaultRetryPolicy()` (`retry.go`)
  - [x] `runVerification()` 통합 — Terminal 분류 시 즉시 `StageFailed` 전환 (`loop.go`)
  - [x] 단위 테스트 6종 (Retryable 7 subtests, Terminal 6, Unknown, NilError, NextDelay 7, DefaultPolicy) (`retry_test.go`)

- [x] **2-3 Dead Letter Queue**
  - [x] `006_dlq.sql` 마이그레이션 — dlq 테이블 + 인덱스 2개
  - [x] `DLQEntry` CRUD — `InsertDLQ`, `ListDLQ`, `GetDLQEntry`, `DeleteDLQEntry`, `RequeueDLQ`, `CountDLQ` (`dlq.go` 신규)
  - [x] `RequeueDLQ` 트랜잭션 — pipeline_state 재삽입 + DLQ 삭제 원자적 처리
  - [x] `recordToDLQ()` — terminal 분류 또는 max attempts 초과 시 자동 기록 (`loop.go`)
  - [x] DLQ 중복 기록 방지 — `Run` 루프 catch-all에서 `IsTerminal()` 가드
  - [x] API 엔드포인트: `GET /api/dlq`, `POST /api/dlq/{id}/requeue`, `DELETE /api/dlq/{id}`
  - [x] `DashboardStore` 인터페이스 확장 (`server.go`)

- [x] **2-4 고급 메트릭**
  - [x] `AdvancedMetrics` 구조체 — 7개 필드 (`dashboard_queries.go`)
  - [x] `GetAdvancedMetrics()` — Throughput24h, FailureRate24h, WIPCount, LeadTime P50/P90, RetryCount, DLQCount
  - [x] LeadTime 계산 — `stage_history` JSON 파싱 + Go-side percentile 보간
  - [x] `OverviewData`에 `AdvancedMetrics` 통합 (`handler.go`)

- [x] **2-5 SSE 이벤트**
  - [x] `pipelineSnapshot`에 `Status` 필드 추가 (`poller.go`)
  - [x] `pipeline_paused` SSE 이벤트 — `pipeline_id`, `stage`, `paused_at` 포함
  - [x] `pipeline_resumed` SSE 이벤트 — `pipeline_id`, `stage`, `resumed_at` 포함

**검증 결과**: `go build ./...` ✅ | `go test ./internal/orchestrator/...` ✅ | `go test ./internal/store/...` ✅ | `go test ./internal/dashboard/...` ✅

**알려진 제약** (Phase 4에서 해소 예정):
- Pause race: `saveState`가 in-memory Status를 DB에 쓸 때 외부 pause 신호 덮어쓸 수 있음
- `RequeueDLQ`: 실행 중인 orchestrator에 requeue 알림 없음 → `pylon continue` 필요

---

#### Phase 3 완료 체크리스트

- [x] **3-1 워크플로우 자동 선택**
  - [x] `SuggestWorkflow(requirement) (name, keywords)` 키워드 매칭 엔진 (`selector.go`)
  - [x] 6개 워크플로우 키워드 세트 (hotfix, bugfix, docs, refactor, review, explore) + feature 기본값
  - [x] 우선순위 기반 매칭 (hotfix > bugfix > ...)
  - [x] 한/영 키워드 지원
  - [x] 단위 테스트 14종 (`selector_test.go`)

- [x] **3-2 스테이지 시각화 개선**
  - [x] `PipelineView.WorkflowName`, `WorkflowStages` 필드 추가 (`handler.go`)
  - [x] `PipelineDetailData.WorkflowStages` 필드 추가 (`handler.go`)
  - [x] `handlePipelineDetail`에서 워크플로우 템플릿 로드 → 동적 단계 설정
  - [x] `stage_progress.html` 워크플로우별 단계 표시 + AllStages 폴백
  - [x] `pipeline_card.html` stage-bar-mini 워크플로우별 동적 렌더링
  - [x] 워크플로우 이름 뱃지 표시

- [x] **3-3 태스크 테이블 뷰**
  - [x] `TaskItem` 확장: Status, StartedAt, CompletedAt, ErrorMessage, FileCount (`taskgraph.go`)
  - [x] `TaskItemView` 확장 + Duration 계산 (`handler.go`)
  - [x] 태스크 테이블 Status/Duration/Files/Error 컬럼 (`pipeline.html`)
  - [x] 태스크 상태별 CSS 클래스 (running/completed/failed/pending)

- [x] **3-4 DLQ 페이지**
  - [x] `GET /dlq` HTML 페이지 라우트 (`server.go`)
  - [x] navbar에 DLQ 링크 추가 (`layout.html`)
  - [x] DLQ 테이블: ID, Pipeline, Workflow, Stage, Error, Time, Actions (`dlq.html`)
  - [x] Requeue/Dismiss HTMX 버튼 + after-request 새로고침
  - [x] 에러 output tail 펼치기/접기 (`<details>/<summary>`)
  - [x] 에러 유형별 그룹 카운트 (`classifyError`)
  - [x] 빈 상태 메시지

- [x] **3-5 CLI 워크플로우 선택**
  - [x] `--workflow` 미지정 시 `SuggestWorkflow()` 자동 추천 + 키워드 표시 (`request.go`)
  - [x] `--workflow auto` 자동 선택 모드
  - [x] 사용 가능한 워크플로우 목록 표시

**검증 결과**: `go build ./...` ✅ | `go test ./internal/workflow/...` ✅ | `go test ./internal/orchestrator/...` ✅ | `go test ./internal/dashboard/...` ✅ | `go test ./internal/store/...` ✅

---

### Phase 0: 기반 강화 (1주)

> 두 제안 모두의 전제조건이 되는 최소 변경

| ID | 작업 | 변경 파일 | 출처 |
|----|------|-----------|------|
| 0-1 | **에이전트 타임아웃 강제 적용**: `Runner.RunHeadless()`에 `context.WithTimeout` 적용, 에이전트 YAML `timeout` 오버라이드 지원 | `internal/agent/runner.go` | OD 3.5 |
| 0-2 | **Pipeline 구조체 통합 확장**: `WorkflowName string`, `Status PipelineStatus`, `PausedAtStage Stage` 필드 추가. JSON 직렬화 + SQLite 스키마 마이그레이션 | `internal/orchestrator/pipeline.go`, `internal/store/pipeline_state.go` | WF 1-5 + OD 3.1 |
| 0-3 | **Config 구조체 확장**: `Workflow` 섹션 + `WIPLimits` 섹션 골격 추가 (파싱만, 동작은 이후 Phase) | `internal/config/config.go` | WF 1-8 + OD 3.4 |

### Phase 1: 적응형 워크플로우 (2주)

> 핵심 문제 해결: 12단계 고정 파이프라인 → 작업 유형별 유연한 경로

| ID | 작업 | 변경 파일 | 의존 |
|----|------|-----------|------|
| 1-1 | **WorkflowTemplate 타입 정의** | `internal/workflow/template.go` (신규) | - |
| 1-2 | **YAML 로더 + 내장 템플릿 7종** (feature, bugfix, hotfix, docs, explore, review, refactor) | `internal/workflow/loader.go` (신규), `.pylon/workflows/*.yml` (신규) | 1-1 |
| 1-3 | **`BuildTransitions()` 동적 전환 맵 생성** | `internal/orchestrator/pipeline.go` | 0-2, 1-1 |
| 1-4 | **`request.go` 하드코딩 제거**: `--workflow` 플래그 추가, `TransitionTo(StageArchitectAnalysis)` → `TransitionTo(workflow.NextStageAfter(...))` | `internal/cli/request.go` | 1-3 |
| 1-5 | **loop.go 워크플로우 주입**: `Orchestrator` 생성 시 워크플로우 템플릿 바인딩, 복구 시 `WorkflowName`으로 전환 맵 재구성 | `internal/orchestrator/loop.go`, `orchestrator.go` | 1-3 |
| 1-6 | **하위 호환성 보장**: `--workflow` 미지정 시 기본 `feature` 템플릿 적용, 기존 12단계 동일 동작 확인 통합 테스트 | `internal/orchestrator/pipeline_test.go` | 1-1~1-5 |

**검증 기준**: `pylon request "fix: 로그인 에러" --workflow bugfix`가 PO → Dev → Verify → PR 4단계만 실행.

### Phase 2: 운영 안정성 (2-3주)

> 프로덕션 운영에 필요한 제어·복원·관측 능력

| ID | 작업 | 변경 파일 | 의존 |
|----|------|-----------|------|
| 2-1 | **Pause/Resume**: loop.go에 `checkPreconditions()` 게이트 추가 (매 전환 전 Status 체크), API 엔드포인트 `/pause`, `/resume` | `internal/orchestrator/loop.go`, `internal/dashboard/handler.go` | 0-2 |
| 2-2 | **지능형 재시도**: `FailureClassifier` (retryable/terminal 분류), 지수 백오프 `RetryPolicy`, 재시도 횟수 및 에러 메시지 기반 분류 | `internal/orchestrator/retry.go` (신규) | 0-1 |
| 2-3 | **Dead Letter Queue**: DLQ 테이블 + CRUD, 대시보드 requeue API | `internal/store/dlq.go` (신규), `internal/dashboard/handler.go` | 2-2 |
| 2-4 | **고급 메트릭**: Throughput 24h, Failure Rate, WIP, Lead Time P50/P90, Retry/DLQ 카운트. SQL 윈도우 함수 기반 | `internal/store/metrics.go` (확장), `internal/dashboard/templates/partials/advanced_metrics.html` | - |
| 2-5 | **워크플로 제어 패널**: RUN/PAUSE/STOP 버튼 (개별 + 전역), SSE 이벤트 `pipeline_paused/resumed` | `internal/dashboard/templates/partials/pipeline_controls.html` (신규) | 2-1 |

**검증 기준**: 실행 중 파이프라인을 Pause → 현재 에이전트 완료 후 대기 → Resume → 정상 이어서 실행.

### Phase 3: 가시성 & UX (2주)

> 사용자 경험 및 정보 접근성 개선

| ID | 작업 | 변경 파일 | 의존 |
|----|------|-----------|------|
| 3-1 | **워크플로우 자동 선택**: 키워드 매칭 엔진 + PO 프롬프트에 워크플로우 추천 역할 추가 | `internal/workflow/selector.go` (신규), `.pylon/agents/po.md` (변경) | Phase 1 |
| 3-2 | **스테이지 시각화 개선**: 수평 파이프라인 바, 워크플로우별 표시 단계 동적 조정 | `internal/dashboard/templates/partials/pipeline_stages_bar.html` (신규) | Phase 1 |
| 3-3 | **태스크 테이블 뷰**: 상태·스텝·시간·에러·파일 수 추적, 필터 탭, 개별 재시도 버튼 | `internal/dashboard/handler.go`, `templates/partials/task_table.html` | [미결 #1] |
| 3-4 | **DLQ 페이지**: 실패 원인 그룹핑, output tail, requeue/dismiss UI | `internal/dashboard/templates/dlq.html` (신규) | 2-3 |
| 3-5 | **TUI 워크플로우 선택**: `pylon request` 시 인터랙티브 워크플로우 선택 UI | `internal/cli/launch.go` (변경) | Phase 1 |

### Phase 4: 확장성 (3-4주)

> 멀티 모델, 멀티 파이프라인, 리소스 관리

| ID | 작업 | 변경 파일 | 의존 |
|----|------|-----------|------|
| 4-1 | **AgentRunner 인터페이스 추출**: 기존 Runner → `ClaudeCodeRunner`, `GenericCLIRunner` 추가. 타임아웃(0-1)을 인터페이스 계약에 포함 | `internal/agent/runner.go` (변경), `claude_runner.go` (신규), `generic_runner.go` (신규) | 0-1 |
| 4-2 | **Runner 디스패치**: `cfg.Agent.Backend` 필드에 따라 적절한 Runner 선택 (`runner.go:103`의 하드코딩 `"claude"` 제거) | `internal/agent/runner.go` | 4-1 |
| 4-3 | **환경변수 기반 에이전트 정체성**: `PYLON_AGENT_NAME`, `PYLON_TEAM_NAME` 등 주입 | `internal/agent/claude_runner.go` | 4-1 |
| 4-4 | **멀티 파이프라인 Scheduler**: 독립 Loop 고루틴, 전역 에이전트 세마포어, `runtime.max_pipelines` 설정 | `internal/orchestrator/scheduler.go` (신규) | 2-1 |
| 4-5 | **WIP 제한 & 백프레셔**: 스테이지별 WIP 카운트 체크, 다운스트림 포화 시 업스트림 억제 | `internal/orchestrator/loop.go` | 4-4 |
| 4-6 | **워커 용량 추적 WorkerPool**: 모델별 동시 실행 수·사용률 추적, 대시보드 사이드바 용량 게이지 | `internal/orchestrator/capacity.go` (신규), `internal/dashboard/templates/partials/sidebar.html` | 4-1 |
| 4-7 | **Transport 인터페이스 추출**: 파일 기반 inbox/outbox → `Transport` 인터페이스 + `FileTransport` 구현 | `internal/protocol/transport.go` (신규), `file_transport.go` (신규) | - |

### Phase 5: 고급 기능 (선택적)

| ID | 작업 | 의존 |
|----|------|------|
| 5-1 | 크로스 파이프라인 의존성 (`requires` 절, DAG 검증) | 4-4 |
| 5-2 | 실시간 에이전트 로그 스트리밍 (SSE) | - |
| 5-3 | Swarm 템플릿 (`allow_dynamic_spawn`, 동적 에이전트 생성) | 4-1 + Phase 1 |
| 5-4 | 반응형 의존성 해소 (ClawTeam `blocked_by` 패턴 → TaskGraph 대체) | Phase 1 + 4-4 |

---

## 3. 핵심 아키텍처 의사결정 (ADR 후보)

### ADR-001: 동적 전환 맵 vs Paused 상태의 공존 방식

**맥락**: `BuildTransitions()`가 워크플로우별 전환 맵을 생성하는데, Pause는 어떤 전환 맵에서든 동작해야 함.

**선택지**:
- A) `Paused`를 `Stage`로 추가 → 전환 맵에 모든 단계 → Paused 경로 추가
- B) `Paused`를 `PipelineStatus`로 분리 (Stage와 독립) → loop.go에서 전환 전 Status 체크

**권장**: **B** — Paused는 "어떤 단계에 있는가"가 아니라 "실행 중인가"이므로 메타 상태로 분리. OD 제안의 원안과 동일.

### ADR-002: 워크플로우 변경 가능 시점

**맥락**: 파이프라인 실행 중 워크플로우를 변경할 수 있는가?

**선택지**:
- A) 불가 — 시작 시 결정, 변경 불가
- B) PO 대화 단계에서만 변경 가능
- C) 어떤 단계에서든 변경 가능 (단, 현재 단계가 새 워크플로우에 포함되어야 함)

**권장**: **B** — PO가 요구사항 재분석 후 워크플로우 변경 추천 가능. 실행 중 변경은 상태 일관성 위험.

### ADR-003: 타임아웃을 인터페이스 계약에 포함할 것인가

**맥락**: OD 3.5의 타임아웃을 먼저 적용 후, WF Phase 3의 `AgentRunner` 인터페이스 추출 시 타임아웃 처리 위치.

**선택지**:
- A) 각 Runner 구현이 자체적으로 타임아웃 관리
- B) 공통 래퍼가 `context.WithTimeout`을 적용한 뒤 Runner에 전달
- C) 인터페이스 `Start(ctx, task)` 시그니처에서 ctx로 전달, Runner는 ctx 준수 의무

**권장**: **C** — Go의 관용적 패턴. Runner는 ctx cancellation을 존중하면 됨. 공통 래퍼는 불필요한 복잡도.

---

## 4. 미결 사항 (Open Questions)

### ~~미결 #1: TaskGraph 없는 워크플로우에서 태스크 테이블 뷰~~ ✅ 해소

**결정**: A) 암시적 단일 TaskGraph 자동 생성. `skip_task_graph` 워크플로우에서도 에이전트 이름을 태스크로 하는 단일 `TaskItem`을 자동 생성하여 대시보드 코드 분기 없이 통일된 렌더링.

### 미결 #2: 멀티 파이프라인 간 WIP 제한의 범위

**문제**: OD 제안의 WIP 제한은 스테이지별 수를 제한하지만, 워크플로우 템플릿 도입 후에는 같은 스테이지라도 워크플로우마다 의미가 다를 수 있음.

**질문**: WIP 제한은 전역(모든 파이프라인의 `agent_executing` 합산) vs 워크플로우별 vs 파이프라인별 중 어느 수준인가?

**후보 해법**:
- A) 전역 WIP만 (단순, OD 제안 원안)
- B) `config.yml`에서 워크플로우별 WIP 오버라이드 지원
- C) 런타임에서 자동 조정 (워크플로우 유형별 가중치)

### ~~미결 #3: PO 워크플로우 오분류 시 복구 경로~~ ✅ 해소 (ADR-002)

**결정**: B) PO 대화 단계에서만 워크플로우 변경 허용. PO가 요구사항 재분석 후 "이 작업은 feature급입니다" 추천 가능. `agent_executing` 이후에는 변경 불가 — 잘못 선택 시 cancel 후 새 파이프라인으로 재시작.

### 미결 #4: DLQ 재시도와 워크플로우의 관계

**문제**: DLQ에서 requeue 할 때, 원래 워크플로우의 특정 단계에서 재시작해야 함. 그러나 현재 DLQ 설계는 `Stage` 정보만 저장하고 `WorkflowName`은 저장하지 않음.

**질문**: `DLQEntry`에 `WorkflowName`을 추가해야 하는가? requeue 시 원래 워크플로우의 전환 맵을 복원해야 하는가?

**결론**: 추가해야 함 — DLQEntry 스키마에 `WorkflowName string` 필드 포함 필요.

### 미결 #5: 대시보드 레이아웃 전환 시점

**문제**: OD 제안은 사이드바 + 메인 2컬럼 레이아웃을 제안하지만, 현재는 단일 컬럼. 이 변경은 모든 기존 페이지에 영향을 미치는 대규모 프론트엔드 리팩토링.

**질문**: 사이드바 레이아웃은 Phase 4(멀티 파이프라인)에서 "필수"가 되는데, 더 일찍(Phase 2) 도입해야 하는가? 점진적 도입(Phase 2에서 접이식 사이드바 → Phase 4에서 상시 사이드바)이 가능한가?

### ~~미결 #6: `feature` 워크플로우와 기존 동작의 100% 호환성 검증 방법~~ ✅ 해소

**결정**: C) 자동 추론 — `BuildTransitions()`에서 verification이 stages에 포함되면, stages 순서상 verification 바로 앞 단계(e.g., `agent_executing`)를 자동으로 루프백 대상에 추가. 명시적 `loops` 섹션 불필요.

**검증 방법**: `TestFeatureWorkflow_MatchesHardcoded` 테스트로 feature 템플릿의 `BuildTransitions()` 결과가 현재 하드코딩 `validTransitions`와 정확히 일치하는지 자동 검증. `ForceStage()` 롤백은 전환 검증을 우회하므로 영향 없음.

### ~~미결 #7: Swarm 템플릿의 구현 전제조건 범위~~ ✅ 해소

**결정**: B) 최소 확장점만 확보. `WorkflowTemplate`에 `AllowDynamicSpawn bool` 필드를 예약(Phase 5용, 현재 미사용). `loop.go`의 `runAgentExecution()`에서 TaskGraph 순회 로직을 별도 함수로 추출하여 나중에 교체 가능한 구조만 확보. 실제 Swarm 로직은 미구현.

### ~~미결 #8: SQLite 스키마 마이그레이션 전략~~ ✅ 해소 (Phase 0 완료)

**결정**: 기존 `store.Migrate()` 프레임워크(`schema_migrations` 테이블 + 파일명 순차 실행) 활용. `005_pipeline_extensions.sql`로 `ALTER TABLE` 3건 추가. 별도 마이그레이션 프레임워크 도입 불필요 — 기존 패턴이 충분히 기능함.

### 미결 #9: SSE 아키텍처 확장성

**문제**: 현재 SSE는 1초 SQLite 폴링 → broadcast 구조. Phase 2에서 `pipeline_paused/resumed`, Phase 3에서 태스크 상태 변경, Phase 5에서 실시간 로그 스트리밍이 추가되면 이벤트 볼륨이 크게 증가. 특히 멀티 파이프라인(Phase 4) 환경에서 폴링 기반 SSE가 성능 병목이 될 수 있음.

**질문**: 폴링 주기 조정으로 충분한가, 아니면 이벤트 기반 pub-sub(채널 기반 내부 이벤트 버스)로 전환이 필요한가?

---

## 5. 변경 파일 총 영향도

| 파일 | Phase | 변경 유형 | 영향도 |
|------|-------|-----------|--------|
| `internal/orchestrator/pipeline.go` | 0, 1 | 구조체 확장 + `BuildTransitions()` | **높음** |
| `internal/orchestrator/loop.go` | 1, 2, 4 | 워크플로우 주입 + Pause 게이트 + WIP 체크 | **높음** |
| `internal/cli/request.go` | 1 | 하드코딩 제거 + `--workflow` 플래그 | **높음** |
| `internal/agent/runner.go` | 0, 4 | 타임아웃 + 인터페이스 추출 | **높음** |
| `internal/config/config.go` | 0 | Workflow + WIP 섹션 추가 | 중간 |
| `internal/store/pipeline_state.go` | 0 | 스키마 마이그레이션 (WorkflowName, Status) | 중간 |
| `internal/dashboard/handler.go` | 2, 3 | API 엔드포인트 + 뷰 모델 확장 | 중간 |
| `internal/orchestrator/orchestrator.go` | 1 | 복구 시 워크플로우 로드 | 중간 |
| `internal/domain/stage.go` | 0 | PipelineStatus 타입 추가 (Stage와 별도) | 낮음 |

---

## 6. 핵심 원칙

1. **기존 동작 무결성**: `--workflow` 미지정 시 현재 12단계와 100% 동일 동작 (미결 #6 해소 필수)
2. **점진적 가치 전달**: Phase 1만으로 핵심 문제(80% 작업에서의 오버헤드) 해결. 이후 Phase는 선택적
3. **단일 변경 포인트**: 같은 파일을 여러 Phase에서 반복 수정하지 않도록, 구조체 확장(Phase 0)을 선행
4. **운영 우선**: 타임아웃(Phase 0) → Pause/Resume(Phase 2) → DLQ(Phase 2) 순으로 안정성 먼저 확보
5. **대시보드는 백엔드 따라가기**: 백엔드 기능이 구현된 후 대시보드에 노출 (동시 변경 최소화)
