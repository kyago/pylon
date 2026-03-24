# Pylon v2 마이그레이션 계획

## 1. 파일 처리 계획

### 1.1 삭제 대상 (Go 코드)

| 디렉토리/파일 | 이유 |
|--------------|------|
| `internal/dashboard/` 전체 | Dashboard 포기 결정 |
| `internal/executor/` 전체 | syscall.Exec/RunHeadless 불필요 (Claude Code가 직접 실행) |
| `internal/protocol/` 전체 | inbox/outbox 메시지 프로토콜 불필요 (동일 프로세스) |
| `internal/orchestrator/loop.go` | Go FSM → slash command 기반으로 전환 |
| `internal/orchestrator/scheduler.go` | 병렬 스케줄링 → Claude Code Agent 도구로 대체 |
| `internal/orchestrator/capacity.go` | WorkerPool → 불필요 |
| `internal/orchestrator/watcher.go` | OutboxWatcher → 불필요 |
| `internal/orchestrator/retry.go` | RetryPolicy → slash command 내 재시도 로직으로 |
| `internal/orchestrator/verify.go` | → shell script `run-verification.sh`로 이전 |
| `internal/orchestrator/conversation.go` | → 단순화 또는 제거 |
| `internal/cli/launch.go` | 대폭 축소 — `generateClaudeDir()` 등 유지, 나머지 제거 |
| `internal/cli/request.go` | → slash command `/pl:pipeline`으로 이전 |
| `internal/cli/resume.go` | → 불필요 (파일 기반 상태로 자동 재개) |
| `internal/cli/review.go` | → slash command `/pl:review`로 유지 |
| `internal/cli/stage.go` | → `pylon status`로 통합 |
| `internal/cli/hooks.json` | → 유지 (메모리 sync hook) |
| `internal/agent/claude_runner.go` | → 불필요 (Claude Code Agent 도구 사용) |
| `internal/agent/generic_runner.go` | → 불필요 |
| `internal/agent/lifecycle.go` | → 불필요 (Agent 도구가 수명 관리) |
| `internal/git/worktree.go` | → shell script `merge-branches.sh`로 이전 |
| `internal/git/pr.go` | → shell script `create-pr.sh`로 이전 |

### 1.2 유지 대상

| 디렉토리/파일 | 역할 | 변경 |
|--------------|------|------|
| `internal/store/store.go` | SQLite 연결 관리 | 유지 |
| `internal/store/project_memory.go` | BM25 메모리 | 유지 |
| `internal/store/pipeline_state.go` | 파이프라인 이력 | 유지 (조회 전용으로 축소) |
| `internal/store/migrations/` | DB 스키마 | 불필요 테이블 제거하는 신규 마이그레이션 추가 |
| `internal/memory/manager.go` | 메모리 Manager | 유지 |
| `internal/config/config.go` | config.yml 파싱 | 유지 (Dashboard, Protocol 관련 필드 제거) |
| `internal/config/agent.go` | 에이전트 정의 파싱 | 유지 (불필요 필드 정리) |
| `internal/config/verify.go` | 설정 검증 | 유지 |
| `internal/cli/init.go` | 워크스페이스 초기화 | 유지 |
| `internal/cli/doctor.go` | 진단 | 유지 |
| `internal/cli/mem.go` | 메모리 CLI | 유지 |
| `internal/cli/status.go` | 상태 조회 | 유지 (파일 기반 + SQLite 조회) |
| `internal/cli/sync_memory.go` | 메모리 동기화 | 유지 |
| `internal/cli/add_project.go` | 프로젝트 추가 | 유지 |
| `internal/cli/uninstall.go` | 워크스페이스 제거 | 유지 |
| `internal/slug/` | slug 생성 | 유지 |
| `internal/domain/stage.go` | Stage 상수 | 리팩토링 (파일 기반으로 단순화) |

### 1.3 신규 작성

| 파일 | 역할 |
|------|------|
| `.pylon/commands/pl-pipeline.md` | 메타 워크플로우 slash command |
| `.pylon/commands/pl-architect.md` | 아키텍처 분석 slash command |
| `.pylon/commands/pl-breakdown.md` | PM 태스크 분해 slash command |
| `.pylon/commands/pl-execute.md` | 에이전트 실행 slash command |
| `.pylon/commands/pl-verify.md` | 검증 slash command |
| `.pylon/commands/pl-pr.md` | PR 생성 slash command |
| `.pylon/scripts/bash/common.sh` | 공통 유틸리티 |
| `.pylon/scripts/bash/init-pipeline.sh` | 파이프라인 초기화 |
| `.pylon/scripts/bash/check-prerequisites.sh` | 사전조건 검증 |
| `.pylon/scripts/bash/run-verification.sh` | build/test/lint 실행 |
| `.pylon/scripts/bash/create-pr.sh` | PR 생성 |
| `.pylon/scripts/bash/merge-branches.sh` | 브랜치 머지 |
| `.pylon/scripts/bash/cleanup-pipeline.sh` | 정리 |
| `internal/cli/launch.go` (재작성) | 최소화된 런처 |

## 2. 마이그레이션 순서

### Phase 1: Shell Script 계층 구축
1. `common.sh` 작성 (repo root 감지, 유틸리티)
2. `init-pipeline.sh` 작성 (git branch + 디렉토리)
3. `check-prerequisites.sh` 작성 (파일 존재 검증)
4. `run-verification.sh` 작성 (build/test/lint)
5. `create-pr.sh` 작성 (gh pr create)
6. `merge-branches.sh` 작성 (agent branch merge)
7. `cleanup-pipeline.sh` 작성 (worktree/branch 정리)

### Phase 2: Slash Command 작성
1. `/pl:pipeline` 메타 커맨드 (핵심)
2. `/pl:architect` 아키텍처 분석
3. `/pl:breakdown` 태스크 분해
4. `/pl:execute` 에이전트 실행
5. `/pl:verify` 검증
6. `/pl:pr` PR 생성
7. 기존 `/pl:status`, `/pl:cancel` 업데이트

### Phase 3: Go 코드 정리
1. `internal/dashboard/` 삭제
2. `internal/executor/` 삭제
3. `internal/protocol/` 삭제
4. `internal/orchestrator/` 대폭 축소 (loop, scheduler, capacity, watcher, retry 삭제)
5. `internal/agent/` 축소 (runner, lifecycle 삭제)
6. `internal/cli/` 축소 (request, resume, stage, review 삭제)
7. `internal/git/` 축소 (worktree, pr 삭제)
8. `generateClaudeDir()` 업데이트 — 새 slash command 생성

### Phase 4: 설정 정리
1. `config.yml` 스키마 업데이트 (dashboard, protocol 관련 제거)
2. SQLite 마이그레이션 추가 (불필요 테이블 DROP)
3. 에이전트 정의 frontmatter 정리

## 3. 하위 호환성

### 유지되는 CLI
```bash
pylon init              # 워크스페이스 초기화
pylon doctor            # 진단
pylon mem search        # 메모리 검색
pylon mem store         # 메모리 저장
pylon mem list          # 메모리 목록
pylon status            # 파이프라인 상태
pylon add-project       # 프로젝트 추가
pylon sync-memory       # 메모리 동기화
pylon sync-agents       # 에이전트 동기화
pylon sync-projects     # 프로젝트 동기화
pylon uninstall         # 워크스페이스 제거
pylon version           # 버전
```

### 제거되는 CLI
```bash
pylon request           # → /pl:pipeline
pylon resume            # → 불필요 (파일 기반 자동 재개)
pylon cancel            # → /pl:cancel
pylon stage             # → pylon status로 통합
pylon review            # → /pl:review
pylon dashboard         # → 포기
pylon index             # → /pl:index
```
