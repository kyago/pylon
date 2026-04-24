# Tasks: Multi-Repo Pipeline Harness

**Input**: Design documents from `/specs/002-multi-repo-pipeline/`
**Prerequisites**: plan.md ✅, spec.md ✅, research.md ✅, data-model.md ✅, contracts/ ✅

**Organization**: 태스크는 User Story 단위로 그룹화되어 각 스토리를 독립적으로 구현·검증 가능

## Format: `[ID] [P?] [Story?] Description`

- **[P]**: 병렬 실행 가능 (다른 파일, 완료되지 않은 태스크 의존성 없음)
- **[Story]**: 해당 User Story (US1~US5, spec.md 기준)
- 각 태스크에 정확한 파일 경로 포함

---

## Phase 1: Setup

**Purpose**: 변경 대상 파일 검증 및 두 경로(.pylon/ vs internal/cli/)가 동일한지 확인

- [ ] T001 두 스크립트 경로의 대상 파일 diff 비교 및 불일치 기록 — `diff .pylon/scripts/bash/common.sh internal/cli/scripts/bash/common.sh` 등 4개 파일

---

## Phase 2: Foundational — common.sh `find_git_root()` 제거

**Purpose**: 모든 스크립트가 의존하는 공통 라이브러리 변경. 이 Phase가 완료되어야 이후 스크립트 변경이 의미 있음.

**⚠️ CRITICAL**: 이 Phase가 완료되기 전까지 US별 스크립트 변경을 시작하지 않음

- [ ] T002 [P] `.pylon/scripts/bash/common.sh` — `find_git_root()` 함수 전체 제거, `GIT_ROOT` 결정 로직을 `git -C "$REPO_ROOT" rev-parse --show-toplevel 2>/dev/null || echo "$REPO_ROOT"` 한 줄로 대체
- [ ] T003 [P] `internal/cli/scripts/bash/common.sh` — T002 동일 변경 적용

**Checkpoint**: `git rev-parse --show-toplevel` 이 worktree 환경에서도 올바른 경로를 반환하는지 수동 확인

---

## Phase 3: User Story 5 — 스크립트 `--git-root` 인자 인터페이스 (Priority: P2)

**Goal**: `init-pipeline.sh`, `run-verification.sh`, `create-pr.sh` 3개 스크립트 모두 `--git-root` 인자를 지원

**Independent Test**: `init-pipeline.sh "test" --git-root services/service-a --pipeline-dir /tmp/test-pipeline` 실행 시 `services/service-a` repo에 브랜치가 생성되는지 확인

### init-pipeline.sh 수정

- [ ] T004 [P] [US5] `.pylon/scripts/bash/init-pipeline.sh` — `source common.sh` 이전에 `--git-root`, `--pipeline-dir` 사전 파싱 코드 삽입 (research.md의 파싱 패턴 적용); source 후 `GIT_ROOT` override 로직 추가; 루트 모드(--git-root 없음)에서 브랜치 생성 로직 제거 — 루트 모드는 PIPELINE_DIR + status.json 초기화만 수행
- [ ] T005 [US5] `.pylon/scripts/bash/init-pipeline.sh` — 서브파이프라인 모드(--git-root + --pipeline-dir 모두 있음) 구현: `{pipeline-dir}/{repo-basename}/` sub-dir 생성, 지정 repo에 브랜치 생성 (기존 있으면 checkout), 서브파이프라인 `status.json` 초기화, JSON 출력에 `pipeline_dir` 필드를 sub-dir 경로로 출력 (depends on T004)
- [ ] T006 [P] [US5] `internal/cli/scripts/bash/init-pipeline.sh` — T004+T005 동일 변경 적용

### run-verification.sh 수정

- [ ] T007 [P] [US5] `.pylon/scripts/bash/run-verification.sh` — `source common.sh` 이전에 `--git-root` 사전 파싱; source 후 GIT_ROOT override; `--git-root` 있을 때 해당 경로에 `go.mod` 없으면 `{ok:true, checks:[], skipped:true}` JSON 출력 후 exit 0
- [ ] T008 [P] [US5] `internal/cli/scripts/bash/run-verification.sh` — T007 동일 변경 적용

### create-pr.sh 수정

- [ ] T009 [P] [US5] `.pylon/scripts/bash/create-pr.sh` — `source common.sh` 이전에 `--git-root` 사전 파싱; source 후 GIT_ROOT override (기존 `--branch`, `--title` 등 인자 처리 로직 유지)
- [ ] T010 [P] [US5] `internal/cli/scripts/bash/create-pr.sh` — T009 동일 변경 적용

**Checkpoint**: `--git-root services/service-a`로 각 스크립트를 호출하여 올바른 디렉토리에서 git 명령이 실행되는지 확인

---

## Phase 4: User Story 2 — 다중 Repo 서브파이프라인 워크플로우 (Priority: P1)

**Goal**: `pl-pipeline.md`가 PM 주도 서브파이프라인 스폰 방식으로 재작성됨; 아키텍트가 `affected_repos` 섹션을 출력

**Independent Test**: 두 repo 포함 워크스페이스에서 pl-pipeline 실행 시 각 repo에 독립 브랜치 생성 및 병렬 repo-Agent 스폰 확인

- [ ] T011 [P] [US2] `.pylon/commands/pl-pipeline.md` — Step 1~7 전체 재작성: Step 1(브랜치 생성 제거, PIPELINE_DIR + status.json만), Step 2(requirement-analysis.md 다중 repo 가능성 항목 추가), Step 3(architecture.md에 affected_repos 섹션 필수 명시), Step 4(변경 없음), Step 5(tasks.json repo 필드 포함, status 필드 제거, architecture.md affected_repos 기반 repo별 Agent 병렬 스폰 + 각 repo-Agent 내부 실행 순서 명세), Step 6(PM이 성공 repo만 per-repo create-pr.sh 호출), Step 7(repo별 성공/실패 + PR URL 목록 완료 보고)
- [ ] T012 [P] [US2] `internal/cli/agents/architect.md` — 에이전트 프롬프트에 `## Affected Repositories` 섹션을 architecture.md에 반드시 포함하도록 지시 추가 (`- services/service-a: [변경 이유]` 형식, 단일 repo는 `"."` 사용)

**Checkpoint**: pl-pipeline.md의 Step 5가 repo-Agent 병렬 스폰 예시와 함께 명확히 작성되었는지 검토

---

## Phase 5: User Story 3 — per-repo PR 생성 완성 (Priority: P2)

**Goal**: 각 서브파이프라인 완료 후 PM이 성공 repo에만 per-repo PR을 생성하고 URL 목록을 집계

**Independent Test**: 다중 repo 파이프라인 완료 후 각 repo에 별도 `pr.json`이 생성되고 최종 보고에 PR URL 목록이 포함되는지 확인

> **Note**: T009~T010(create-pr.sh --git-root)과 T011(pl-pipeline.md Step 6 재작성)으로 대부분 커버됨. 이 Phase는 데이터 흐름 검증에 집중.

- [ ] T013 [US3] `.pylon/scripts/bash/init-pipeline.sh` 서브파이프라인 모드 출력 검증 — `{pipeline_dir}` 값이 sub-dir 경로임을 확인; pl-pipeline.md Step 5의 PM이 이 값을 `create-pr.sh --pipeline-dir`로 전달하는 흐름이 data-model.md의 PIPELINE_DIR 계층구조와 일치하는지 교차 검토 및 불일치 수정
- [ ] T014 [US3] `status.json` sub_pipelines 배열 업데이트 책임 명확화 — init-pipeline.sh 서브파이프라인 모드가 루트 status.json의 sub_pipelines에 항목을 추가하는지, 또는 PM이 직접 업데이트하는지 결정하고 pl-pipeline.md에 명시 (`.pylon/commands/pl-pipeline.md`)

---

## Phase 6: User Story 1 — 단일 Repo 호환성 검증 (Priority: P1)

**Goal**: 모든 변경 후에도 기존 단일 repo 워크플로우가 회귀 없이 동작

**Independent Test**: `--git-root` 없이 기존 방식대로 init-pipeline.sh를 실행하여 기존과 동일한 JSON 출력 확인

> **Note**: US4(파일시스템 격리)는 repo별 Agent 분리 설계로 달성되며 별도 코드 없음. T011(pl-pipeline.md) 재작성으로 US4도 함께 커버.

- [ ] T015 [US1] 하위 호환성 최종 검토 — `--git-root` 없는 init-pipeline.sh/run-verification.sh/create-pr.sh 호출 시 기존 동작과 동일한지 각 스크립트의 조건 분기 검토; `tasks.json` repo 필드 없는 구형 형식 처리 방침을 pl-pipeline.md에 명시 (없으면 `"."` 기본값 사용)

---

## Phase 7: Polish

**Purpose**: 코드 품질 및 일관성 최종 점검

- [ ] T016 [P] `.pylon/scripts/bash/` 4개 파일과 `internal/cli/scripts/bash/` 4개 파일이 동일한지 최종 diff 검증 — 불일치 수정
- [ ] T017 [P] `specs/002-multi-repo-pipeline/contracts/script-interface.md` 계약과 실제 구현된 스크립트 인터페이스 일치 여부 확인 — 불일치 발견 시 contracts 문서 또는 코드 수정

---

## Dependencies & Execution Order

### Phase Dependencies

- **Phase 1 (Setup)**: 즉시 시작 가능
- **Phase 2 (Foundational)**: Phase 1 완료 후 시작, Phase 3-7을 블로킹
- **Phase 3 (US5)**: Phase 2 완료 후 시작 — T004/T007 병렬, T005는 T004 완료 후
- **Phase 4 (US2)**: Phase 3 완료 후 시작 — T011, T012 병렬 실행 가능
- **Phase 5 (US3)**: Phase 4 완료 후 시작
- **Phase 6 (US1)**: Phase 3~5 완료 후 시작
- **Phase 7 (Polish)**: 모든 US 구현 완료 후 시작

### User Story Dependencies

- **US5 (Phase 3)**: Phase 2 완료 후 시작 — US2, US3의 스크립트 전제조건
- **US2 (Phase 4)**: US5 완료 후 시작
- **US3 (Phase 5)**: US5 완료 후 시작 (US2와 병렬 가능)
- **US1 (Phase 6)**: US2, US3, US5 완료 후 통합 검증
- **US4**: US2 Phase 4의 T011(pl-pipeline.md 재작성)으로 달성, 별도 태스크 없음

### 내부 의존성 (Phase 3)

```
T002, T003 [P]                   # Phase 2: common.sh 양쪽 복사본
    ↓
T004, T007, T009 [P 가능]        # init-pipeline, run-verification, create-pr .pylon/
T005 (depends T004)              # init-pipeline.sh 서브파이프라인 모드
T006, T008, T010 [P]             # internal/cli/ 복사본들
    ↓
T011, T012 [P]                   # pl-pipeline.md + architect.md
```

---

## Parallel Execution Examples

### Phase 2 — common.sh (전체 병렬)

```
Task: T002 — .pylon/scripts/bash/common.sh find_git_root 제거
Task: T003 — internal/cli/scripts/bash/common.sh find_git_root 제거
```

### Phase 3 — Script Interface (그룹별 병렬)

```
# 그룹 A: 루트 모드 변경 (T004, T007, T009 병렬)
Task: T004 — .pylon/ init-pipeline.sh 수정
Task: T007 — .pylon/ run-verification.sh 수정
Task: T009 — .pylon/ create-pr.sh 수정

# 그룹 B: internal/ 복사본 (A 이후 또는 동시, T006, T008, T010 병렬)
Task: T006 — internal/ init-pipeline.sh 수정
Task: T008 — internal/ run-verification.sh 수정
Task: T010 — internal/ create-pr.sh 수정
```

### Phase 4 — Workflow 재작성 (전체 병렬)

```
Task: T011 — pl-pipeline.md 전체 재작성
Task: T012 — architect.md affected_repos 추가
```

---

## Implementation Strategy

### MVP First (US1 + US5 Only)

1. Phase 1: Setup
2. Phase 2: Foundational (common.sh)
3. Phase 3: US5 (--git-root 스크립트 인터페이스)
4. Phase 6: US1 호환성 검증
5. **STOP and VALIDATE**: 단일 repo 워크플로우가 기존과 동일하게 동작하는지 확인

### Incremental Delivery

1. Setup + Foundational → 공통 기반 완성
2. US5 추가 → 스크립트 --git-root 인터페이스 완성 (독립 검증 가능)
3. US2 추가 → 다중 repo 파이프라인 워크플로우 완성
4. US3 추가 → per-repo PR 생성 완성
5. US1 검증 → 단일 repo 하위 호환성 확인
6. Polish → 최종 점검

---

## Notes

- **[P] 태스크 = 다른 파일 대상, 미완료 태스크에 대한 의존성 없음**
- `.pylon/scripts/bash/`와 `internal/cli/scripts/bash/` 양쪽 모두 수정 필수 (×2 정책)
- T005(서브파이프라인 모드)는 T004(루트 모드 변경 + 파싱) 완료 후 같은 파일에 추가 구현
- `find_git_root()` 제거 시 common.sh를 source하는 모든 스크립트에서 해당 함수 직접 호출 여부 확인
- 각 태스크 완료 후 커밋 권장 (롤백 포인트 확보)
