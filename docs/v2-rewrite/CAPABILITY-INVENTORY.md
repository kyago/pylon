# Pylon v1 기능 인벤토리 및 v2 전환 매핑

## 범례

- **유지**: v2에서 동일하게 유지
- **이전**: 다른 형태로 전환 (Go → Shell/Slash command)
- **삭제**: v2에서 제거
- **축소**: 기능 범위 축소

---

## 1. 오케스트레이션 (`internal/orchestrator/`)

| 기능 | v1 구현 | v2 처리 | 대체 방식 |
|------|---------|---------|----------|
| Pipeline FSM (12 stages) | `pipeline.go` validTransitions | **이전** | `/pl:pipeline` markdown 단계 |
| Main Loop | `loop.go` Run() | **이전** | `/pl:pipeline` 순차 실행 |
| Headless Agent 실행 | `loop.go` runHeadlessAgent() | **이전** | Claude Code Agent 도구 |
| PO Conversation | `loop.go` runPOConversation() | **이전** | `/pl:pipeline` 내 LLM 직접 수행 |
| PM Task Breakdown | `loop.go` runPMTaskBreakdown() | **이전** | `/pl:breakdown` slash command |
| Agent Wave Execution | `loop.go` runAgentExecution() | **이전** | Agent 도구 병렬 호출 |
| Verification | `loop.go` runVerification() | **이전** | `run-verification.sh` |
| PR Creation | `loop.go` runPRCreation() | **이전** | `create-pr.sh` |
| TaskGraph DAG | `taskgraph.go` | **축소** | tasks.json 내 의존성, LLM이 순서 판단 |
| Scheduler (WIP/capacity) | `scheduler.go` | **삭제** | Agent 도구에 위임 |
| WorkerPool | `capacity.go` | **삭제** | Agent 도구에 위임 |
| OutboxWatcher | `watcher.go` | **삭제** | 동일 프로세스 내 실행 |
| RetryPolicy | `retry.go` | **이전** | `/pl:pipeline` 내 재시도 로직 |
| Crash Recovery | `orchestrator.go` Recover() | **축소** | 파일 기반 (산출물 존재 확인) |
| ConversationManager | `conversation.go` | **축소** | SQLite conversations 테이블 유지 |
| State Persistence | `orchestrator.go` savePipelineState() | **이전** | 파일 + SQLite 이력 |

## 2. 에이전트 (`internal/agent/`)

| 기능 | v1 구현 | v2 처리 | 대체 방식 |
|------|---------|---------|----------|
| ClaudeCodeRunner | `claude_runner.go` | **삭제** | Agent 도구가 대체 |
| GenericCLIRunner | `generic_runner.go` | **삭제** | Agent 도구가 대체 |
| Agent Lifecycle (FSM) | `lifecycle.go` | **삭제** | Agent 도구가 관리 |
| ClaudeMD Builder | `claudemd.go` | **이전** | slash command에서 프롬프트 구성 |
| RunConfig | `runner.go` | **삭제** | Agent 도구 파라미터로 대체 |
| Agent Config Parsing | `config/agent.go` | **유지** | frontmatter 정리 후 유지 |

## 3. 프로세스 실행 (`internal/executor/`)

| 기능 | v1 구현 | v2 처리 | 대체 방식 |
|------|---------|---------|----------|
| ExecInteractive (syscall.Exec) | `direct.go` | **삭제** | 불필요 (프로세스 교체 없음) |
| RunInteractive (child process) | `direct.go` | **삭제** | Bash 도구가 대체 |
| RunHeadless (captured output) | `direct.go` | **삭제** | Agent 도구가 대체 |
| Environment merging | `direct.go` buildEnv() | **삭제** | 불필요 |

## 4. Git 관리 (`internal/git/`)

| 기능 | v1 구현 | v2 처리 | 대체 방식 |
|------|---------|---------|----------|
| WorktreeManager | `worktree.go` | **이전** | Agent `isolation: "worktree"` + shell script |
| Branch creation | `branch.go` | **이전** | `init-pipeline.sh` |
| Branch merging | `worktree.go` MergeBranch() | **이전** | `merge-branches.sh` |
| PR creation | `pr.go` CreatePR() | **이전** | `create-pr.sh` |
| CommandRunner | `runner.go` | **삭제** | shell script에서 직접 git 호출 |

## 5. 저장소 (`internal/store/`)

| 기능 | v1 구현 | v2 처리 | 비고 |
|------|---------|---------|------|
| Store 초기화/마이그레이션 | `store.go` | **유지** | |
| project_memory + FTS5 | `project_memory.go` | **유지** | BM25 검색 핵심 |
| pipeline_state | `pipeline_state.go` | **축소** | 이력 조회 전용 |
| conversations | `conversations.go` | **유지** | |
| message_queue | `message_queue.go` | **삭제** | inbox/outbox 불필요 |
| blackboard | `blackboard.go` | **삭제** | project_memory로 통합 |
| dlq | `dlq.go` | **삭제** | DLQ 포기 |
| dashboard_queries | `dashboard_queries.go` | **삭제** | Dashboard 포기 |
| session_archive | `session_archive.go` | **삭제** | 선택적 |

## 6. 메모리 (`internal/memory/`)

| 기능 | v1 구현 | v2 처리 | 비고 |
|------|---------|---------|------|
| BM25 Search | `manager.go` | **유지** | `pylon mem search`로 접근 |
| Proactive Injection | `manager.go` GetProactiveContext() | **이전** | slash command에서 `pylon mem search` 호출 |
| Learning Storage | `manager.go` StoreLearnings() | **유지** | hook으로 자동 수집 |

## 7. 프로토콜 (`internal/protocol/`)

| 기능 | v1 구현 | v2 처리 | 비고 |
|------|---------|---------|------|
| MessageEnvelope | `message.go` | **삭제** | 동일 프로세스 내 통신 |
| FileTransport (inbox/outbox) | `outbox.go`, `inbox.go` | **삭제** | 불필요 |
| OutboxWatcher | `watcher.go` | **삭제** | 불필요 |

## 8. Dashboard (`internal/dashboard/`)

| 기능 | v1 구현 | v2 처리 | 비고 |
|------|---------|---------|------|
| HTTP Server | `server.go` | **삭제** | Dashboard 포기 |
| SSE Hub | `sse.go` | **삭제** | |
| Poller | `poller.go` | **삭제** | |
| Templates | `templates/` | **삭제** | |
| Handlers | `handler.go` | **삭제** | |

## 9. CLI 명령어 (`internal/cli/`)

| 명령어 | v1 구현 | v2 처리 | 대체 방식 |
|--------|---------|---------|----------|
| `pylon` (launch) | `launch.go` | **축소** | `.claude/` 생성 + TUI 실행만 |
| `pylon init` | `init.go` | **유지** | scripts/ 디렉토리도 생성하도록 확장 |
| `pylon request` | `request.go` | **이전** | `/pl:pipeline` |
| `pylon resume` | `resume.go` | **삭제** | 파일 기반 자동 재개 |
| `pylon status` | `status.go` | **유지** | 파일 + SQLite 조회 |
| `pylon cancel` | `cancel.go` | **이전** | `/pl:cancel` |
| `pylon stage` | `stage.go` | **삭제** | `pylon status`로 통합 |
| `pylon review` | `review.go` | **이전** | `/pl:review` |
| `pylon dashboard` | `dashboard.go` | **삭제** | Dashboard 포기 |
| `pylon index` | `index.go` | **이전** | `/pl:index` |
| `pylon doctor` | `doctor.go` | **유지** | |
| `pylon mem *` | `mem.go` | **유지** | |
| `pylon add-project` | `add_project.go` | **유지** | |
| `pylon sync-*` | `sync_*.go` | **유지** | |
| `pylon uninstall` | `uninstall.go` | **유지** | |

## 10. 워크플로우 템플릿 (`internal/workflow/`)

| 기능 | v1 구현 | v2 처리 | 대체 방식 |
|------|---------|---------|----------|
| YAML 워크플로우 정의 | `templates/*.yml` | **이전** | slash command + shell script 조합 |
| Template Loader | `loader.go` | **삭제** | 불필요 |
| Workflow Selector | `selector.go` | **이전** | `/pl:pipeline`에서 LLM이 판단 |
| Stage transitions | `template.go` BuildTransitions() | **삭제** | YAML handoffs로 대체 |

---

## 요약 통계

| 처리 | 항목 수 | 비율 |
|------|---------|------|
| 유지 | 15 | ~20% |
| 이전 (형태 변환) | 25 | ~33% |
| 축소 | 5 | ~7% |
| 삭제 | 30 | ~40% |
| **합계** | **75** | **100%** |
