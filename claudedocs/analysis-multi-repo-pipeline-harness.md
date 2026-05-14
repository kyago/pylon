# 다중 Repo 파이프라인 하네스 엔지니어링 — 문제 분석 및 요구사항

## 배경

`.pylon/commands/pl-pipeline.md` 기반의 파이프라인은 단일 git repo를 전제로 설계되었다.
다중 프로젝트 워크스페이스에서 사용할 경우 브랜치 생성, 에이전트 격리, PR 생성 등
여러 계층에서 구조적 결함이 드러난다.

---

## 현재 구조 요약

```
pl-pipeline 실행 흐름
─────────────────────────────────────────────────────────
Step 1  init-pipeline.sh   → 브랜치 1개 생성 (GIT_ROOT 기준)
Step 2  PO 요구사항 분석   → requirement-analysis.md
Step 3  Architect 에이전트 → architecture.md
Step 4  사전조건 검증
Step 5  PM 태스크 분해     → tasks.json (repo 필드 없음)
Step 6  병렬 에이전트 실행 → Agent(isolation="worktree") × N
Step 7  검증               → run-verification.sh
Step 8  PR 생성            → create-pr.sh (단일 PR)
```

---

## 발견된 문제

### 문제 1: GIT_ROOT 가 단일 경로로 고정됨

`common.sh`의 `GIT_ROOT` 결정 로직:

```bash
_GIT_REPO_CFG=$(config_get "git.repo" "")
if [[ -n "$_GIT_REPO_CFG" ]]; then
  GIT_ROOT="$(realpath "$REPO_ROOT/$_GIT_REPO_CFG")"
else
  GIT_ROOT="$(find_git_root)"   # REPO_ROOT 부터 위로 탐색
fi
```

- `GIT_ROOT` 는 항상 하나. 다중 repo 개념 없음
- `git.repo` 미설정 + workspace 루트에 `.git/` 없음 → fallback이 non-git 디렉토리
  → `init-pipeline.sh` 실행 시 `git branch --show-current` 에러로 종료

### 문제 2: tasks.json 에 repo 필드 없음

현재 태스크 스키마:
```json
{
  "id": "T001",
  "title": "...",
  "description": "...",
  "agent": "backend-dev",
  "dependencies": [],
  "status": "pending"
}
```

- 어느 git repo에서 실행할지 지정 불가
- PM이 다중 repo를 의미론적으로 파악해도 실행 계층에 전달할 수단 없음

### 문제 3: isolation="worktree" 범위가 GIT_ROOT 한 곳에만 적용

Step 6에서 모든 에이전트가 동일한 `GIT_ROOT` 기준 worktree로 격리됨:

```
Agent(T001: service-a 작업, isolation="worktree")  → service-a worktree ✅ 격리
Agent(T002: service-b 작업, isolation="worktree")  → service-a worktree ✅ 격리
                                                       └ service-b 접근 시 worktree 밖
                                                         → 격리 없음 ❌
                                                         → 병렬 에이전트 간 충돌 위험
```

- `service-b` 를 두 에이전트가 동시 수정 시 충돌 발생 가능
- `service-b` 에는 task 브랜치 없음 → 어느 브랜치에 커밋되는지 불확실

### 문제 4: PR 생성이 단일 repo 기준

`create-pr.sh` 는 `GIT_ROOT` 하나에서 `gh pr create` 한 번 실행.
다중 repo 작업 결과를 여러 PR로 분리 생성하는 메커니즘 없음.

---

## 시나리오별 현재 동작

| 시나리오 | 브랜치 생성 | 에이전트 격리 | PR 생성 |
|---------|-----------|------------|--------|
| 단일 repo | ✅ 정상 | ✅ 정상 | ✅ 정상 |
| 다중 repo → 단일 서브 프로젝트 작업 (`git.repo` 설정) | ✅ 해당 repo에만 | ✅ 해당 repo 기준 | ✅ 해당 repo |
| 다중 repo → N개 프로젝트에 걸친 작업 | ❌ GIT_ROOT 한 곳만 | ❌ GIT_ROOT 외 격리 없음 | ❌ 단일 PR만 |

---

## 요구사항

### R1: PM이 다중 repo 작업을 인지하고 서브파이프라인을 스폰

- PM(Step 5)이 태스크 분해 시점에 repo별로 태스크를 그룹화
- 각 repo 그룹을 독립 실행 단위(서브파이프라인)로 분리하여 스폰
- 스폰 주체가 PM인 이유: 아키텍처 산출물(architecture.md)을 이미 가진 시점이며,
  태스크 레벨에서 repo 경계를 가장 정밀하게 파악할 수 있는 위치이기 때문

```
pl-pipeline (root)
 ├─ Step 2: PO  → 요구사항 분석 (다중 repo 가능성 인지, 산출물에 기록)
 ├─ Step 3: Architect → 영향 받는 repo 목록 파악 → architecture.md 에 기록
 ├─ Step 5: PM  → tasks.json 생성 + repo별 그룹화 → 서브파이프라인 스폰
 │    ├─ 서브파이프라인 → service-a
 │    │    ├─ init-pipeline.sh (service-a 브랜치 생성)
 │    │    ├─ 에이전트 실행 (service-a 워크트리 격리)
 │    │    ├─ run-verification.sh (service-a 검증)
 │    │    └─ (PR 생성 권한 없음 — PM이 통제)
 │    └─ 서브파이프라인 → service-b
 │         ├─ init-pipeline.sh (service-b 브랜치 생성)
 │         ├─ 에이전트 실행 (service-b 워크트리 격리)
 │         ├─ run-verification.sh (service-b 검증)
 │         └─ (PR 생성 권한 없음 — PM이 통제)
 └─ Step 8: PM  → 모든 서브파이프라인 완료 확인 후 per-repo PR 생성
```

### R2: tasks.json 과 status.json 역할 분리

**tasks.json** — PM이 생성하는 정적 명세, 이후 불변

```json
{
  "tasks": [
    {
      "id": "T001",
      "title": "...",
      "description": "...",
      "agent": "backend-dev",
      "repo": "services/service-a",
      "dependencies": []
    }
  ]
}
```

- `repo` 는 `REPO_ROOT` 기준 상대경로
- `status` 필드 제거 — 런타임 상태는 tasks.json에 두지 않음
- PM이 생성 후 수정하지 않음 (계획 스냅샷)

**status.json** — 오케스트레이터(스크립트)가 관리하는 런타임 상태, `sub_pipelines` 배열 확장

```json
{
  "stage": "executing",
  "status": "running",
  "branch": "task/feat-summary",
  "started_at": "2026-04-24T10:00:00Z",

  "sub_pipelines": [
    {
      "repo": "services/service-a",
      "branch": "task/feat-summary",
      "pipeline_dir": ".pylon/runtime/20260424-feat/service-a",
      "status": "success"
    },
    {
      "repo": "services/service-b",
      "branch": "task/feat-summary",
      "pipeline_dir": ".pylon/runtime/20260424-feat/service-b",
      "status": "failed"
    }
  ]
}
```

**결정 근거**:
- tasks.json에 runtime 상태(`status`)를 섞으면 병렬 에이전트의 동시 쓰기 충돌 위험
- 새 파일(`sub-pipelines.json`) 추가 대신 기존 `status.json` 확장 → 파이프라인 단일 진실 원천 유지
- 각 서브파이프라인은 자신의 독립 디렉토리(`service-a/`, `service-b/`)에서 동작하므로 병렬 쓰기 충돌 없음
- 단일 repo 환경: `sub_pipelines` 배열이 1개 항목이거나 생략 → 코드 분기 없음

### R3: init-pipeline.sh 가 per-repo 브랜치 생성을 지원

- 단일 repo: 기존 동작 유지
- 서브파이프라인 컨텍스트: 지정된 `GIT_ROOT` 에서 브랜치 생성
- 동일 브랜치명을 여러 repo에 동시 생성 가능해야 함

### R4: 에이전트 격리가 올바른 repo 기준으로 동작

- 서브파이프라인 에이전트는 해당 서브파이프라인의 `GIT_ROOT` worktree 기준으로 격리
- 다른 repo 작업 에이전트와 파일시스템 충돌 없음

### R5: PR 생성 주체는 PM, 서브에이전트는 PR 생성 금지

서브에이전트가 PR을 직접 생성하는 것은 부적절하다. 이유:

1. **기술적 제약**: `isolation="worktree"` 에이전트는 임시 worktree 안에서 동작한다.
   PR은 worktree 정리 후 task 브랜치가 확정된 시점에만 생성 가능하므로,
   에이전트 실행 중에는 PR을 만들 수 있는 상태 자체가 아니다.

2. **일관성**: 서브파이프라인 일부가 검증 실패할 경우, 이미 생성된 PR이 원격에
   노출된 채로 불일치 상태가 발생한다.

3. **정보 비대칭**: 개별 서브에이전트는 자신의 repo 범위 태스크만 알고,
   전체 `requirement-analysis.md` / `architecture.md` 맥락을 갖지 않는다.
   PR 본문의 품질을 보장할 수 없다.

```
서브에이전트 권한 경계
  허용: 코드 변경, 커밋, git push (브랜치)
  금지: gh pr create

PM 통제 하에 수행
  모든 서브파이프라인 완료 확인
  → per-repo create-pr.sh 호출 (PM이 직접)
  → PR 목록 집계 후 최종 보고
```

### R6: 스크립트 기반 하네스 유지 — `--git-root` 인자로 repo 강제 지정

- 서브파이프라인 실행도 기존 bash 스크립트 계층을 그대로 활용
- LLM 레이어(PO/PM/에이전트)가 달라져도 스크립트 계층은 동일한 인터페이스 제공
- 모든 git 관련 스크립트에 `--git-root <path>` 인자를 추가하여 대상 repo를 명시 지정

**결정 근거**: 프롬프트 기반(에이전트가 cd 후 실행)은 AI 동작에 의존하므로 실행 보장 불가.
`--git-root` 인자 방식은 스크립트가 직접 `cd "$GIT_ROOT"` 를 강제하므로 AI 동작과 무관하게 올바른 repo에서 실행됨.

```bash
# PM이 서브파이프라인 스폰 시 스크립트 호출 예시
init-pipeline.sh "요구사항" --git-root services/service-a
run-verification.sh "$PIPELINE_DIR" --git-root services/service-a
create-pr.sh "$PIPELINE_DIR" --git-root services/service-a --branch task/feat
```

**GIT_ROOT 결정 우선순위** (common.sh):
1. `--git-root` 인자 (스크립트별 파싱)
2. `config.yml` 의 `git.repo` 설정 (단일 repo 기존 방식)
3. `git rev-parse --show-toplevel` 자동 탐지 (fallback)

**`find_git_root()` 제거**: `git rev-parse --show-toplevel` 로 대체.
worktree(`.git`이 파일인 경우)에서도 정확하게 동작하며, 커스텀 bash 재구현이 불필요.

---

## 설계 결정 사항

| 항목 | 결정 |
|------|------|
| 서브파이프라인 스폰 주체 | **PM (Step 5)** — 태스크 분해 시점, architecture.md 기반으로 repo 경계 확정 |
| 서브파이프라인 단위 | **repo 1개당 Agent 1개** — PM이 repo별 Agent를 병렬 스폰, Agent가 자율 완결 |
| Agent 내부 구조 | **init → 구현 → verify 자율 처리** — PM은 스폰과 결과 수신만 담당 |
| PR 생성 주체 | **PM** — 서브에이전트 PR 생성 금지 |
| 브랜치명 형식 | **`task/{feature-summary}`** — 모든 서브파이프라인이 동일한 브랜치명 공유 |
| 서브파이프라인 부분 실패 | **PM이 성공/실패 결과를 집계하여 사용자에게 보고, 중단하지 않음** |
| GIT_ROOT 전달 방식 | **`--git-root` 인자** — 스크립트가 repo를 강제 지정, AI 동작 의존 없음 |
| `find_git_root()` | **제거** — `git rev-parse --show-toplevel` 으로 대체 |
| GIT_ROOT 결정 우선순위 | `--git-root` 인자 → `config.yml git.repo` → `git rev-parse` 자동탐지 |
| tasks.json 역할 | 정적 명세 (PM 생성, 불변) — `status` 필드 제거, `repo` 필드 추가 |
| tasks.json `dependencies` | **순수 비즈니스 의존성만** — 기술적 직렬화 제거, 동일 repo 충돌은 repo-Agent 격리로 자연 해결 |
| status.json 역할 | 런타임 상태 (오케스트레이터 관리) — `sub_pipelines[]` 배열 확장 |
| sub-pipelines.json | **미생성** — 기존 status.json 확장으로 대체 |
| 단일/다중 repo 분기 | **없음** — 단일 repo도 sub_pipeline 1개(`repo: "."`)로 처리, 코드 경로 통일 |
| 서브파이프라인 완료 신호 | **Agent 도구 반환값** — PM이 Agent() 결과로 성공/실패 판단, 파일 폴링 없음 |
| config.yml 확장 | **불필요** — PM이 architecture.md 기반으로 repo 목록 자율 판단 |

---

## pl-pipeline.md Step 변경 명세

### 변경 개요

```
Step 1  init-pipeline.sh   → pipeline dir 생성만 (branch 생성 제거)
Step 2  PO 요구사항 분석   → requirement-analysis.md (다중 repo 가능성 명시 추가)
Step 3  Architect          → architecture.md (affected_repos[] 목록 명시 추가)
Step 4  사전조건 검증      → 변경 없음
Step 5  PM                 → tasks.json 생성 + repo별 Agent 병렬 스폰
Step 6  (제거)             → repo-Agent 내부로 이동
Step 7  (제거)             → repo-Agent 내부로 이동
Step 8  PR 생성            → PM이 성공 repo별 per-repo create-pr.sh 호출
Step 9  완료 보고          → repo별 성공/실패 집계 + PR URL 목록
```

### Step별 상세

**Step 1 — 파이프라인 초기화**
- `init-pipeline.sh`는 pipeline dir과 초기 status.json만 생성
- git branch 생성 제거 — 각 repo-Agent가 `init-pipeline.sh --git-root <path>`로 담당

**Step 2 — PO 요구사항 분석**
- requirement-analysis.md에 다중 repo 가능성 항목 추가:
  - "이 요구사항이 여러 repo에 걸쳐 있는가?"
  - "영향 가능성이 있는 repo는 어디인가?"

**Step 3 — 아키텍처 분석**
- architecture.md에 `affected_repos` 섹션 명시 추가:
  ```
  ## Affected Repositories
  - services/service-a: [변경 이유]
  - services/service-b: [변경 이유]
  ```
- PM이 이 목록을 기준으로 repo-Agent 수를 결정

**Step 5 — PM 태스크 분해 + repo-Agent 스폰**
1. tasks.json 생성 (`repo` 필드 포함, `status` 필드 없음)
2. architecture.md에서 `affected_repos` 읽기
3. repo별 Agent 병렬 스폰 (단일 응답에서 동시 호출):
   ```
   Agent(service-a: init → 해당 repo 태스크 구현 → verify)  ─┐ 병렬
   Agent(service-b: init → 해당 repo 태스크 구현 → verify)  ─┘
   ```

**각 repo-Agent 내부 실행 순서**
1. `init-pipeline.sh --git-root <repo-path>` — branch 생성
2. tasks.json에서 자신의 repo에 해당하는 태스크 추출
3. 태스크 구현 (코드 변경, 커밋)
4. `run-verification.sh --git-root <repo-path>`
5. 결과(success/failure) 반환

**Step 8 — PR 생성**
- PM이 Agent 반환값으로 성공 repo 목록 확인
- 성공한 repo에 대해서만 per-repo PR 생성:
  ```bash
  create-pr.sh "$PIPELINE_DIR" --git-root services/service-a --branch task/feat
  create-pr.sh "$PIPELINE_DIR" --git-root services/service-b --branch task/feat
  ```

**Step 9 — 완료 보고**
- repo별 성공/실패 상태
- 생성된 PR URL 목록
- 실패 repo가 있는 경우 원인 요약

---

## 구현 보완 명세

### PIPELINE_DIR 계층 구조

Step 1과 repo-Agent가 각각 다른 디렉토리를 생성한다.

```
.pylon/runtime/
└── 20260425-feat-login/          ← Step 1 (root init-pipeline.sh)이 생성
    ├── requirement.md
    ├── requirement-analysis.md
    ├── architecture.md
    ├── tasks.json
    ├── status.json
    ├── service-a/                ← repo-Agent가 init-pipeline.sh --git-root로 생성
    │   ├── status.json
    │   └── pr.json
    └── service-b/                ← repo-Agent가 init-pipeline.sh --git-root로 생성
        ├── status.json
        └── pr.json
```

- PM은 root `PIPELINE_DIR` 경로를 각 repo-Agent 프롬프트에 전달
- repo-Agent는 `{PIPELINE_DIR}/{repo_name}/` 을 sub-pipeline dir로 사용
- `init-pipeline.sh --git-root <path>`는 branch를 생성하고 sub-pipeline dir을 반환

### `--git-root` 인자 파싱 방식

각 스크립트에서 `--git-root`를 파싱한 뒤 `common.sh` source 전에 환경변수로 설정:

```bash
#!/bin/bash
set -euo pipefail

# --git-root 인자 사전 파싱
GIT_ROOT_ARG=""
_args=()
for arg in "$@"; do
  if [[ "$arg" == "--git-root" ]]; then
    _next=true
  elif [[ "${_next:-}" == "true" ]]; then
    GIT_ROOT_ARG="$arg"
    _next=false
  else
    _args+=("$arg")
  fi
done
set -- "${_args[@]+"${_args[@]}"}"

source "$(dirname "$0")/common.sh"

# --git-root 인자가 있으면 common.sh 설정값을 덮어씀
if [[ -n "$GIT_ROOT_ARG" ]]; then
  GIT_ROOT="$(realpath "$REPO_ROOT/$GIT_ROOT_ARG")"
fi
```

`common.sh`의 GIT_ROOT 결정 로직은 유지하되, 스크립트 레벨에서 override.

### R4 에이전트 격리 — 구현 방식 변경

R4 요구사항("에이전트 격리가 올바른 repo 기준으로 동작")은 기존 `isolation="worktree"` 방식 대신 **repo 단위 Agent 분리**로 달성한다.

- 기존: 모든 태스크 Agent가 동일 GIT_ROOT worktree 공유 → 충돌 위험
- 변경: 각 repo에 Agent 1개 → Agent 간 파일 접근 범위가 자연히 분리

`isolation="worktree"` 파라미터는 다중 repo 환경에서 사용하지 않는다.  
단일 repo 환경(sub_pipeline 1개)에서도 동일하게 적용.

### repo-Agent 내부 구현 방식

repo-Agent는 추가 Agent를 스폰하지 않고 직접 구현한다:

```
repo-Agent (service-a 담당)
 ├─ Bash: init-pipeline.sh --git-root service-a
 ├─ tasks.json 읽기 → repo == "service-a" 태스크 필터링
 ├─ 태스크별 순서대로 직접 코드 변경 (Edit/Write 도구 사용)
 ├─ git add, git commit (Bash 도구)
 ├─ Bash: run-verification.sh "$SUB_PIPELINE_DIR" --git-root service-a
 └─ 결과 반환 (success / failure + 원인)
```

내부에서 병렬 Agent 스폰 없음 — repo-Agent 자체가 격리 단위이므로 추가 격리 불필요.

### tasks.json `repo` 필드 기본값 (단일 repo 하위 호환)

단일 repo 환경에서 PM이 tasks.json 생성 시 `repo: "."` 사용:

```json
{
  "tasks": [
    {
      "id": "T001",
      "title": "...",
      "description": "...",
      "agent": "backend-dev",
      "repo": ".",
      "dependencies": []
    }
  ]
}
```

`repo: "."` 이면 `--git-root`를 생략하거나 `REPO_ROOT`를 그대로 사용.  
기존 단일 repo 워크플로우는 sub_pipeline 1개(`repo: "."`)로 동일하게 처리되어 코드 경로 분기 없음.

### 수정 대상 파일 목록

| 파일 | 변경 내용 |
|------|----------|
| `common.sh` (×2) | `find_git_root()` 제거, `git rev-parse --show-toplevel` fallback으로 대체 |
| `init-pipeline.sh` (×2) | `--git-root` 파싱 추가, Step 1 호출 시 branch 생성 스킵 옵션 또는 sub-pipeline dir 분기 |
| `run-verification.sh` (×2) | `--git-root` 파싱 추가 |
| `create-pr.sh` (×2) | `--git-root` 파싱 추가 (`cd "$GIT_ROOT"` 이미 있음 — override만 추가) |
| `.pylon/commands/pl-pipeline.md` | Step 전체 재작성 (본 문서 Step 변경 명세 기준) |
| `.pylon/agents/architect.md` | `affected_repos` 섹션 출력 지시 추가 |

> `(×2)` = `.pylon/scripts/bash/` 와 `internal/cli/scripts/bash/` 동일 변경
