# Implementation Plan: Multi-Repo Pipeline Harness

**Branch**: `002-multi-repo-pipeline` | **Date**: 2026-04-25 | **Spec**: [spec.md](./spec.md)
**Input**: Feature specification from `/specs/002-multi-repo-pipeline/spec.md`

## Summary

현재 `pl-pipeline` 명령은 단일 git repo를 전제로 설계되어, 다중 repo 워크스페이스에서 브랜치 생성·에이전트 격리·PR 생성이 모두 실패한다. 이 기능은 bash 스크립트 계층에 `--git-root <path>` 인자를 추가하고, `pl-pipeline.md` 워크플로우를 재작성하여 PM이 repo별 서브파이프라인을 병렬 스폰하도록 한다. 단일 repo 하위 호환성은 `repo: "."` 하나의 서브파이프라인 경로로 보장한다.

## Technical Context

**Language/Version**: Go 1.24+ (메인 CLI), Bash (파이프라인 스크립트)
**Primary Dependencies**: Cobra v1.10.2, gopkg.in/yaml.v3, jq, git, gh CLI
**Storage**: 파일시스템 (PIPELINE_DIR 내 JSON 파일들)
**Testing**: Go `testing` + Bash 스크립트 수동 검증
**Target Platform**: Linux/macOS (darwin)
**Project Type**: CLI 도구
**Performance Goals**: 병렬 서브파이프라인 간 충돌 없는 독립 실행
**Constraints**: `--git-root` 인자가 없을 때 기존 단일 repo 동작 100% 유지
**Scale/Scope**: 스크립트 7개, 명령 파일 1개, 에이전트 파일 1개 수정

## Constitution Check

*constitution.md가 미작성 상태이므로 프로젝트 관찰 기반 원칙 적용*

| 원칙 | 적용 여부 | 판정 |
|------|----------|------|
| 하위 호환성 — 기존 단일 repo 동작 보존 | `--git-root` 없으면 기존 로직 그대로 | ✅ PASS |
| 스크립트 계층 유지 — AI 동작에 의존하지 않음 | `--git-root` 인자로 repo 강제 지정 | ✅ PASS |
| 단순성 — 중첩 Agent 없음 | repo-Agent가 직접 구현 (스폰 없음) | ✅ PASS |
| 파일 분리 — tasks.json 정적/status.json 동적 | `status` 필드 제거, `sub_pipelines` 추가 | ✅ PASS |

**Gates**: 모든 원칙 통과. Phase 0 진행 가능.

## Project Structure

### Documentation (this feature)

```text
specs/002-multi-repo-pipeline/
├── plan.md              # 이 파일
├── research.md          # Phase 0 결과
├── data-model.md        # Phase 1 결과
├── contracts/
│   └── script-interface.md   # --git-root 인자 계약
└── tasks.md             # /speckit.tasks 명령 출력
```

### Source Code (수정 대상 파일)

```text
.pylon/scripts/bash/
├── common.sh            # find_git_root() 제거, git rev-parse 대체
├── init-pipeline.sh     # --git-root 파싱 추가, sub-pipeline dir 분기
├── run-verification.sh  # --git-root 파싱 추가
└── create-pr.sh         # --git-root 파싱 추가

internal/cli/scripts/bash/    (동일 변경 ×2)
├── common.sh
├── init-pipeline.sh
├── run-verification.sh
└── create-pr.sh

.pylon/commands/
└── pl-pipeline.md       # Step 전체 재작성

internal/cli/agents/
└── architect.md         # affected_repos 섹션 출력 지시 추가
```

**Structure Decision**: 단일 프로젝트 구조. 신규 파일 없음, 기존 파일 수정만.

## Complexity Tracking

해당 없음 — 모든 변경이 기존 파일 수정이며 신규 아키텍처 레이어 없음.

---

## Phase 0: Research

### research.md 결과 요약

주요 결정사항 모두 분석 문서(`claudedocs/analysis-multi-repo-pipeline-harness.md`)에 이미 확정되어 있다. 추가 조사가 필요한 항목만 정리한다.

**결정 1: `--git-root` 인자 파싱 패턴**

- **Decision**: 각 스크립트에서 `source common.sh` 이전에 `--git-root` 인자를 사전 파싱하여 환경변수로 보관, source 후 GIT_ROOT를 override
- **Rationale**: common.sh가 source될 때 GIT_ROOT가 이미 결정되므로, 이전에 값을 포착해야 함. 기존 `while case`루프 패턴과 일관성 유지
- **Alternatives considered**: common.sh 내부에서 `$@`를 재파싱하는 방식 — 스크립트별 인자 순서가 다르므로 적합하지 않음

**결정 2: `find_git_root()` 대체 방법**

- **Decision**: `git -C "$dir" rev-parse --show-toplevel 2>/dev/null` 대신 `git rev-parse --show-toplevel 2>/dev/null`로 대체
- **Rationale**: git worktree에서 `.git`이 파일(심볼릭 포인터)인 경우 `find_git_root()`의 `-d "$dir/.git"` 검사가 실패하지만, `git rev-parse --show-toplevel`은 올바르게 처리함
- **Alternatives considered**: 커스텀 탐색 함수 유지 — worktree 버그가 있으며 불필요한 복잡성

**결정 3: sub-pipeline dir 결정 방식**

- **Decision**: `init-pipeline.sh --git-root <path>`를 sub-pipeline 모드로 인식. 이 경우 `PIPELINE_DIR` 인자도 별도로 받아 `{PIPELINE_DIR}/{repo_basename}/`을 sub-pipeline dir로 생성
- **Rationale**: 루트 파이프라인(Step 1)과 서브파이프라인(repo-Agent)이 같은 스크립트를 공유하면서 디렉토리 계층이 달라야 함
- **인자 설계**: `init-pipeline.sh <requirement> [--git-root <path>] [--pipeline-dir <dir>]`
  - `--git-root` 없음 → 루트 모드 (기존 동작)
  - `--git-root` + `--pipeline-dir` → 서브파이프라인 모드

**결정 4: `run-verification.sh`에서 Go 프로젝트 여부 판별**

- **Decision**: `--git-root`로 지정된 repo에 `go.mod`가 있으면 Go 검증 실행, 없으면 `echo "no go.mod found, skipping Go checks"` 후 성공 반환
- **Rationale**: 다중 repo 워크스페이스에서 일부 repo가 Go가 아닐 수 있음 (예: Node.js, 순수 bash 프로젝트)
- **Alternatives considered**: Go 전용으로 유지 — 단일 언어 스택으로만 사용하는 경우 충분하지만 확장성 없음

**결정 5: pl-pipeline.md Step 1 — 브랜치 생성 분리**

- **Decision**: Step 1 (`init-pipeline.sh`)은 PIPELINE_DIR와 status.json만 생성, 브랜치 생성 안 함. 브랜치는 각 repo-Agent가 `init-pipeline.sh --git-root <repo> --pipeline-dir <root-dir>` 호출 시 생성
- **Rationale**: 루트 파이프라인이 어느 repo에 브랜치를 만들어야 하는지 Step 1 시점에는 알 수 없음 (아키텍처 분석 전)

---

## Phase 1: Design & Contracts

### data-model.md

**tasks.json — 정적 태스크 명세 (PM 생성, 이후 불변)**

```json
{
  "tasks": [
    {
      "id": "T001",
      "title": "서비스 A API 엔드포인트 추가",
      "description": "...",
      "agent": "backend-dev",
      "repo": "services/service-a",
      "dependencies": []
    },
    {
      "id": "T002",
      "title": "서비스 B 클라이언트 업데이트",
      "description": "...",
      "agent": "backend-dev",
      "repo": "services/service-b",
      "dependencies": []
    }
  ]
}
```

변경 사항:
- `repo` 필드 추가 (REPO_ROOT 기준 상대경로, 단일 repo는 `"."`)
- `status` 필드 제거

**status.json — 런타임 상태 (오케스트레이터 관리)**

```json
{
  "stage": "executing",
  "status": "running",
  "branch": "task-feat-login",
  "started_at": "2026-04-25T10:00:00Z",
  "sub_pipelines": [
    {
      "repo": "services/service-a",
      "branch": "task-feat-login",
      "pipeline_dir": ".pylon/runtime/20260425-feat-login/service-a",
      "status": "success"
    },
    {
      "repo": "services/service-b",
      "branch": "task-feat-login",
      "pipeline_dir": ".pylon/runtime/20260425-feat-login/service-b",
      "status": "running"
    }
  ]
}
```

변경 사항:
- `sub_pipelines` 배열 추가 (단일 repo는 배열 1개, `repo: "."`)

**PIPELINE_DIR 계층구조**

```
.pylon/runtime/
└── 20260425-feat-login/          ← Step 1 (루트 init-pipeline.sh) 생성
    ├── requirement.md
    ├── requirement-analysis.md
    ├── architecture.md
    ├── tasks.json
    ├── status.json
    ├── service-a/                ← repo-Agent (init-pipeline.sh --git-root) 생성
    │   ├── status.json
    │   └── pr.json
    └── service-b/
        ├── status.json
        └── pr.json
```

### contracts/script-interface.md

**`init-pipeline.sh` 인터페이스**

```bash
# 루트 모드 (기존 — Step 1)
init-pipeline.sh "<requirement>"
# → PIPELINE_DIR 생성, status.json 초기화, 브랜치 생성 없음
# → JSON 출력: {pipeline_id, pipeline_dir}

# 서브파이프라인 모드 (신규 — repo-Agent)
init-pipeline.sh "<requirement>" --git-root <repo-rel-path> --pipeline-dir <root-pipeline-dir>
# → <root-pipeline-dir>/<repo-basename>/ 생성
# → <repo>에 브랜치 생성
# → JSON 출력: {pipeline_id, branch, pipeline_dir: <sub-dir>}
```

**`run-verification.sh` 인터페이스**

```bash
# 기존
run-verification.sh "<pipeline-dir>"

# 신규: --git-root 추가
run-verification.sh "<pipeline-dir>" --git-root <repo-rel-path>
# → <repo>에서 검증 실행
# → go.mod 없으면 검증 스킵 (성공 반환)
```

**`create-pr.sh` 인터페이스**

```bash
# 기존
create-pr.sh "<pipeline-dir>" --branch <branch> --title <title>

# 신규: --git-root 추가
create-pr.sh "<pipeline-dir>" --git-root <repo-rel-path> --branch <branch> --title <title>
# → <repo>에서 gh pr create 실행
```

**GIT_ROOT 결정 우선순위 (모든 스크립트 공통)**

```
1. --git-root <path>  (스크립트 인자, REPO_ROOT 기준 realpath)
2. config.yml git.repo  (기존 설정)
3. git rev-parse --show-toplevel  (자동 탐지, find_git_root 대체)
```

### pl-pipeline.md 워크플로우 재설계

```
Step 1  파이프라인 초기화
        → init-pipeline.sh: PIPELINE_DIR + status.json만 생성 (브랜치 없음)

Step 2  PO 요구사항 분석
        → requirement-analysis.md (다중 repo 가능성 항목 추가)

Step 3  아키텍처 분석
        → architecture.md (affected_repos 섹션 필수 포함)

Step 4  사전조건 검증
        → check-prerequisites.sh (변경 없음)

Step 5  PM 태스크 분해 + repo-Agent 병렬 스폰
        → tasks.json 생성 (repo 필드 포함, status 필드 없음)
        → architecture.md에서 affected_repos 읽기
        → repo별 Agent 단일 응답에서 병렬 스폰

        [각 repo-Agent 내부]
        a. init-pipeline.sh --git-root <repo> --pipeline-dir <root-dir>
        b. tasks.json에서 해당 repo 태스크 필터링
        c. 태스크 순서대로 구현 (코드 변경, 커밋)
        d. run-verification.sh <sub-dir> --git-root <repo>
        e. 결과 반환 (success / failure + 원인)

Step 6  PR 생성 (PM 통제)
        → 성공 repo만 per-repo create-pr.sh 호출
        → PR URL 목록 수집

Step 7  완료 보고
        → repo별 성공/실패 상태
        → 생성된 PR URL 목록
        → 실패 repo 원인 요약
```

### architect.md 변경사항

`affected_repos` 섹션을 architecture.md 산출물에 포함하도록 지시 추가:

```markdown
## Affected Repositories

다음 형식으로 영향 받는 repo를 명시하세요:

- `services/service-a`: [변경 이유]
- `services/service-b`: [변경 이유]

단일 repo 프로젝트라면:

- `.`: [변경 이유]
```

---

## Agent Context Update

에이전트 컨텍스트 업데이트 스크립트를 실행합니다.
