# Pylon v1 → v2 마이그레이션 가이드

## 개요

v2는 spec-kit 패턴을 전면 채택하여 아키텍처를 재작성했습니다.

- **v1**: Go 오케스트레이터가 `syscall.Exec()`로 Claude Code TUI와 통합, `pylon request` CLI로 파이프라인 실행
- **v2**: Claude Code TUI가 직접 오케스트레이터 역할, Markdown slash command (`/pl:pipeline`) + shell script 기반

핵심 변화는 **프로세스 경계 제거**입니다. v1은 Go 프로세스와 Claude Code 프로세스가 분리되어 있었고, v2는 slash command를 통해 Claude Code 내에서 직접 파이프라인을 관리합니다.

## 주요 변경사항

### 아키텍처 변경

| 영역 | v1 | v2 |
|------|----|----|
| **오케스트레이션** | Go FSM (loop.go) | Claude Code + slash commands |
| **에이전트 실행** | `RunHeadless("claude --print")` | Claude Code Agent 도구 (네이티브) |
| **병렬 실행** | errgroup + TaskGraph DAG | Agent 도구 복수 호출 (자동 병렬) |
| **상태 관리** | SQLite (6개 테이블) | 파일 기반 (산출물 존재 = 완료) + SQLite (메모리 검색만) |
| **대시보드** | 웹 UI (localhost:7777) | 제거 |
| **프로세스 통신** | inbox/outbox 파일 프로토콜 | 없음 (동일 프로세스) |

### 삭제된 CLI 커맨드

| v1 커맨드 | v2 대체 | 비고 |
|-----------|---------|------|
| `pylon request "..."` | `/pl:pipeline "..."` | TUI 내 slash command로 전환 |
| `pylon resume` | — | 파일 기반 자동 재개 (불필요) |
| `pylon stage list/status` | `/pl:status` 또는 `pylon status` | 파일 기반 상태 조회 |
| `pylon cancel` | `/pl:cancel` | slash command로 전환 |
| `pylon review` | `/pl:pr` | 개명됨 |
| `pylon dashboard` | — | 기능 제거 |
| `pylon index` | `/pl:index` | slash command로 전환 |

### 유지되는 CLI 커맨드

```bash
pylon              # Claude Code TUI 실행 (기존과 동일)
pylon init         # 워크스페이스 초기화
pylon doctor       # 진단
pylon version      # 버전 확인
pylon status       # 파이프라인 상태 조회
pylon add-project  # 프로젝트 추가
pylon mem search   # 메모리 검색
pylon mem store    # 메모리 저장
pylon mem list     # 메모리 목록
pylon sync-agents  # 에이전트 정의 동기화
pylon sync-memory  # 메모리 동기화
pylon sync-projects # 프로젝트 동기화
pylon uninstall    # 워크스페이스 제거
```

## 업그레이드 절차

### Step 1: 바이너리 업데이트

```bash
go install github.com/kyago/pylon/cmd/pylon@latest
```

또는 로컬 빌드:

```bash
cd /path/to/pylon
git pull origin main
make install
```

### Step 2: SQLite 자동 마이그레이션

v2 바이너리를 처음 실행하면 `007_v2_cleanup.sql` 마이그레이션이 자동 적용됩니다.

**삭제되는 테이블**:
- `message_queue` — inbox/outbox 메시지 큐 (불필요)
- `blackboard` — 프로젝트 메모리 (project_memory로 통합)
- `dlq` — Dead Letter Queue (기능 제거)
- `topic_subscriptions` — 구독 시스템 (불필요)
- `session_archive` — 세션 아카이브 (선택적)

**경고**: 이 마이그레이션은 **비가역적**입니다. 위 테이블의 데이터가 필요하면 업그레이드 전에 백업하세요.

```bash
# 업그레이드 전 백업
cp .pylon/pylon.db .pylon/pylon.db.v1-backup
```

**유지되는 테이블**:
- `project_memory`, `project_memory_fts` — BM25 메모리 검색
- `pipeline_state` — 파이프라인 이력 조회
- `conversations` — 대화 이력

### Step 3: 새 slash command 자동 생성

`pylon` 또는 `pylon init`을 실행하면 v2 slash command가 자동으로 생성됩니다.

생성되는 파일:
- `.pylon/commands/pl-pipeline.md` — 전체 파이프라인
- `.pylon/commands/pl-architect.md` — 아키텍처 분석
- `.pylon/commands/pl-breakdown.md` — 태스크 분해
- `.pylon/commands/pl-execute.md` — 에이전트 실행
- `.pylon/commands/pl-verify.md` — 검증
- `.pylon/commands/pl-pr.md` — PR 생성
- `.pylon/commands/pl-status.md` — 상태 조회
- `.pylon/commands/pl-cancel.md` — 파이프라인 취소
- `.pylon/commands/pl-index.md` — 코드베이스 인덱싱

### Step 4: Shell script 자동 생성

`.pylon/scripts/bash/` 디렉토리가 자동으로 생성됩니다.

```bash
.pylon/scripts/bash/
├── common.sh                # 공통 유틸리티
├── init-pipeline.sh         # 파이프라인 초기화
├── check-prerequisites.sh   # 사전조건 검증
├── run-verification.sh      # 빌드/테스트/린트
├── create-pr.sh             # PR 생성
├── merge-branches.sh        # 브랜치 머지
└── cleanup-pipeline.sh      # 정리
```

### Step 5: 디렉토리 정리 (선택사항)

더 이상 사용하지 않는 v1 런타임 디렉토리를 제거할 수 있습니다.

```bash
# v1 파이프라인 데이터 (불필요)
rm -rf .pylon/runtime/inbox
rm -rf .pylon/runtime/outbox

# 대시보드 관련 디렉토리 (선택)
rm -rf .pylon/dashboard
```

### Step 6: config.yml 정리 (선택사항)

v2에서 제거된 설정 섹션을 수동으로 정리할 수 있습니다. 이전 설정은 하위 호환되므로 제거할 필요는 없습니다.

```yaml
# 제거 가능 (불필요)
dashboard:
  host: localhost
  port: 7777

protocol:
  inbox_dir: .pylon/runtime/inbox
  outbox_dir: .pylon/runtime/outbox
```

### Step 7: 에이전트 정의 정리 (선택사항)

`.pylon/agents/*.md` 파일의 frontmatter에서 불필요한 필드를 정리할 수 있습니다.

**제거 가능한 필드**:
```yaml
backend: claude-code          # ← 항상 claude-code
maxTurns: 30                  # ← Agent 도구가 관리
permissionMode: acceptEdits   # ← Agent 도구가 관리
isolation: worktree           # ← Agent 호출 시 지정
timeout: 30m                  # ← Agent 도구가 관리
capabilities:                 # ← 사용되지 않음
  accepts: [...]
  produces: [...]
env:                          # ← 환경변수 주입 불필요
  KEY: value
```

**필수/추천 필드**:
```yaml
name: backend-dev
role: Backend Developer
scope: [project-api]
tools: [git, gh]
model: sonnet  # optional
```

변경 후 동기화:
```bash
pylon sync-agents
```

## 새로운 기능

### Slash Command 워크플로우

#### 전체 파이프라인 실행

```bash
pylon   # Claude Code TUI 실행
> /pl:pipeline 로그인 기능 구현
```

단계별 흐름:
1. 파이프라인 초기화 (git branch, 디렉토리)
2. PO 요구사항 분석
3. 아키텍처 설계 (Agent 도구)
4. 사전조건 검증
5. PM 태스크 분해
6. 에이전트 병렬 실행
7. 검증 (빌드/테스트/린트)
8. PR 생성
9. 완료 보고

#### 개별 스테이지 실행

필요에 따라 특정 단계만 실행할 수 있습니다:

```bash
/pl:architect         # 아키텍처 분석만
/pl:breakdown         # 태스크 분해만
/pl:execute           # 에이전트 실행만
/pl:verify            # 검증만
/pl:pr                # PR 생성만
/pl:status            # 파이프라인 상태 조회
/pl:cancel            # 파이프라인 취소
/pl:index             # 코드베이스 인덱싱
```

### 파일 기반 상태 추적

파이프라인 진행 상태는 `.pylon/runtime/{pipeline-id}/` 디렉토리의 산출물로 추적됩니다.

| 파일 | 의미 |
|------|------|
| `requirement.md` | 파이프라인 초기화 완료 |
| `requirement-analysis.md` | PO 분석 완료 |
| `architecture.md` | 아키텍처 분석 완료 |
| `tasks.json` | 태스크 분해 완료 |
| `execution-log.json` | 에이전트 실행 완료 |
| `verification.json` | 검증 완료 |
| `pr.json` | PR 생성 완료 |
| `status.json` | 현재 상태 메타데이터 |

**재개 원리**: 파이프라인 재실행 시 기존 산출물을 확인하고, 이미 존재하는 단계는 건너뜁니다. 마지막 완료된 단계 이후부터 자동 재개됩니다.

### Shell Script 계층

`.pylon/scripts/bash/`의 스크립트는 다음과 같은 원자적 작업을 수행합니다:

- **git 작업**: 브랜치 생성/체크아웃
- **디렉토리 관리**: 파이프라인 디렉토리 생성
- **JSON 출력**: 결과를 구조화된 형식으로 반환
- **빌드/테스트**: 검증 스크립트 실행

모든 스크립트는 `--json` 플래그로 JSON 형식 출력을 지원합니다.

**예: 파이프라인 초기화**
```bash
.pylon/scripts/bash/init-pipeline.sh "로그인 기능 구현"
# 출력:
# {
#   "pipeline_id": "20260324-login",
#   "branch": "task-login",
#   "pipeline_dir": ".pylon/runtime/20260324-login"
# }
```

### 메모리 시스템 개선

메모리 검색이 더 강력해졌습니다:

```bash
pylon mem search --project project-api "인증 처리"
```

SQLite FTS5 BM25 검색으로 랭킹/가중치 기반 결과 제공. slash command에서 활용:

```bash
# /pl:pipeline 실행 시 내부적으로
pylon mem search --project <project> "<요구사항 키워드>"
```

검색된 도메인 지식이 에이전트 프롬프트에 자동 주입됩니다.

## v1에서 v2 워크플로우 비교

### v1 워크플로우

```bash
# CLI에서 요청
pylon request "로그인 기능 구현"

# Go 오케스트레이터가 다음을 순차 실행:
# 1. PO 분석
# 2. 아키텍처 분석
# 3. 태스크 분해
# 4. RunHeadless("claude --print")로 각 에이전트 실행
# 5. 검증
# 6. PR 생성

# 외부 프로세스로 실행되므로 interactive 스테이지에서
# syscall.Exec로 프로세스 교체 후 --continue로 수동 재개
```

### v2 워크플로우

```bash
# Claude Code TUI에서 직접 실행
pylon   # TUI 실행
> /pl:pipeline 로그인 기능 구현

# Claude Code가 직접 오케스트레이션:
# 1. init-pipeline.sh 실행
# 2. PO 분석 (Claude Code가 직접 수행)
# 3. Agent 도구로 아키텍트 서브에이전트 실행
# 4. 태스크 분해
# 5. Agent 도구로 백엔드/프론트엔드/테스트 에이전트 병렬 실행
# 6. run-verification.sh 실행
# 7. create-pr.sh 실행
# 8. 완료 보고

# 모든 것이 Claude Code 프로세스 내에서 실행
# interactive한 대화 필요 시 TUI에서 직접 진행
```

## FAQ

### Q: 기존 파이프라인 이력은 유지되나요?

**A:** 네. SQLite의 `pipeline_state` 테이블은 유지됩니다. 다음 명령어로 과거 파이프라인을 조회할 수 있습니다:

```bash
pylon status
```

### Q: 프로젝트 메모리는 유지되나요?

**A:** 네. `project_memory` 테이블과 FTS5 인덱스는 그대로 유지됩니다. 메모리 검색도 동일합니다:

```bash
pylon mem search --project <name> <query>
```

### Q: v1에서 작성한 커스텀 에이전트 정의는?

**A:** `.pylon/agents/*.md` 파일은 그대로 사용 가능합니다. v2에서는 일부 frontmatter 필드만 단순화되었습니다:

```bash
pylon sync-agents   # 정의 갱신 (자동)
```

v1에서 사용 중인 에이전트는 v2도 동일하게 작동합니다.

### Q: v1으로 롤백할 수 있나요?

**A:** SQLite 마이그레이션은 **비가역적**이므로, v1 바이너리로 돌아갈 수 없습니다.

롤백이 필요하면:
1. 업그레이드 전 `pylon.db` 백업 복원
2. v1 바이너리 재설치

```bash
# 준비 단계에서 백업을 했다면
cp .pylon/pylon.db.v1-backup .pylon/pylon.db
go install github.com/kyago/pylon/cmd/pylon@v1.x.x
```

### Q: 대시보드 기능은?

**A:** v2에서는 대시보드(웹 UI)가 제거되었습니다. 기능을 대체하는 것:

- **파이프라인 상태 조회**: `/pl:status` 또는 `pylon status`
- **메모리 검색**: `pylon mem search`
- **실시간 모니터링**: Claude Code TUI의 네이티브 기능

대시보드의 시각화가 필요하면 향후 버전 검토 대상입니다.

### Q: 에러 발생 시 어떻게 복구하나요?

**A:** 파이프라인 실패 시:

1. 에러 메시지를 읽고 원인 파악
2. `/pl:status`로 현재 단계 확인
3. 필요한 단계부터 재실행 (자동으로 건너뛴 단계는 다시 실행하지 않음)

예를 들어 검증에서 테스트 실패 시:

```bash
> /pl:verify   # 검증만 재실행
# 테스트 수정 후
> /pl:verify   # 다시 실행
# 성공 시
> /pl:pr       # PR 생성으로 진행
```

### Q: 팀 협업 시 어떻게 하나요?

**A:** v2에서는 모든 것이 git branch와 파일 기반이므로:

1. feature branch에서 `/pl:pipeline` 실행
2. PR이 생성됨
3. 팀이 PR을 리뷰하고 merge

메모리는 SQLite (공유 `pylon.db`)를 사용하므로 프로젝트 메모리도 팀원과 공유됩니다:

```bash
pylon sync-memory  # 최신 메모리 동기화
```

## 마이그레이션 체크리스트

- [ ] v2 바이너리 설치 완료
- [ ] SQLite 마이그레이션 확인 (v2 바이너리 실행)
- [ ] 새 slash command 생성 확인 (`.pylon/commands/pl-*.md`)
- [ ] Shell script 생성 확인 (`.pylon/scripts/bash/`)
- [ ] `pylon status` 실행 하여 기존 데이터 확인
- [ ] `/pl:pipeline` 명령어로 새 파이프라인 실행 테스트
- [ ] 메모리 검색 테스트: `pylon mem search --project <name> <query>`
- [ ] 선택: 불필요한 v1 디렉토리 정리
- [ ] 선택: config.yml 정리

## 추가 자료

- **v2 아키텍처 설계**: `docs/v2-rewrite/ARCHITECTURE.md`
- **의사결정 기록**: `docs/v2-rewrite/DECISIONS.md`
- **기능 인벤토리**: `docs/v2-rewrite/CAPABILITY-INVENTORY.md`
