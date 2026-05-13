# Feature Specification: `add-project`를 독립 clone 방식으로 전환

**Feature Branch**: `003-add-project-clone`
**Status**: Draft
**Date**: 2026-05-12
**Related**: `specs/002-multi-repo-pipeline/spec.md` (결합 방식 중립적 — 본 변경의 근거)

---

## 1. Background

### 1.1 현재 상태

`internal/cli/add_project.go`의 `pylon add-project`는 `git submodule add <url> <name>`을 통해 하위 repo를 워크스페이스에 등록한다. 이 결정에 따라 다음의 부수 동작이 코드 전반에 분산되어 있다:

- `internal/cli/init_cmd.go:189–196`: 워크스페이스를 `git init`으로 슈퍼프로젝트화
- `internal/cli/add_project.go:255–297`: submodule의 `.git/info/exclude`에 `.pylon/` 자동 등록
- `internal/cli/add_project.go:82–98`: `--force` 시 `git submodule deinit` + `.git/modules/<name>` 정리
- `internal/cli/uninstall.go:122, 167, 211–220, 263–266`: `--remove-projects`가 submodule 제거 경로
- `internal/cli/doctor.go:94, 125`: `checkSubmoduleExcludes`가 submodule 가정

### 1.2 spec 002와의 정합성

`specs/002-multi-repo-pipeline/spec.md`는 결합 방식에 중립적이다:

- FR-003: "각 task는 `repo` 필드를 포함하며, 워크스페이스 루트 기준 상대 경로" — submodule/clone 모두 동일하게 만족
- 라인 141: "각 repo는 독립적인 git 저장소를 가진다" — submodule 강제 아님
- `research.md:52`에서 이미 `git rev-parse --show-toplevel`로 worktree/submodule/clone 모두 정상 처리

즉 submodule 채택은 `add-project` 한 곳에서만 강제되며, 파이프라인 로직과는 무관하다.

### 1.3 변경 동기

워크스페이스가 개인 로컬 작업공간으로만 쓰이고 공유되지 않는 사용 시나리오에서 submodule의 핵심 이점(슈퍼프로젝트 SHA 핀, `--recursive` clone, 팀 재현성)은 발휘되지 않는다. 반면 다음 비용은 그대로 부담된다:

- 하위 repo 변경 시마다 슈퍼 워크스페이스가 dirty 상태 표시
- submodule이 기본 detached HEAD로 clone되어 spec 002의 브랜치 생성 흐름과 마찰
- `.git/info/exclude` 우회 로직, `.git/modules/` 정리 로직 등 코드 복잡도
- 사용자가 submodule mental model(deinit, gitlink 파일, `.gitmodules` 편집) 학습 부담

본 변경은 `add-project`를 spec 002의 결합 중립성에 정렬하는 **fix**이며, 새 기능 추가가 아니다.

---

## 2. 결정사항

| 항목 | 결정 |
|---|---|
| 신규 `add-project` 결합 방식 | `git clone <url> <dir>` (submodule 미사용) |
| 기존 submodule 프로젝트 | 동작 유지 (파이프라인은 결합 중립이므로 그대로 작동) |
| 마이그레이션 도구 | `pylon migrate-project <name>` 신규 명령 — 명시적 사용자 호출만 |
| 하위 repo `.pylon/` 처리 | `.git/info/exclude`에 자동 추가 (현행과 동일, 함수명만 일반화) |
| `pylon init`의 `git init <workspace>` | **완전 제거** — 개인 로컬 워크스페이스 시나리오에서 슈퍼 git 불필요 |

---

## 3. User Stories

### Story 1 — 신규 프로젝트 추가 (P1)

개인 워크스페이스에서 외부 git repo를 추가하는 사용자는 슈퍼프로젝트(`git init`된 워크스페이스), `.gitmodules`, submodule 개념을 의식하지 않고 단순한 clone으로 프로젝트를 붙일 수 있어야 한다.

**Acceptance**:
1. **Given** `pylon init`으로 새로 만든 워크스페이스에서, **When** `pylon add-project <git-url>`을 실행하면, **Then** 워크스페이스에는 `.gitmodules`가 생성되지 않고, 하위 디렉토리는 일반 `git clone` 결과이며, 기본 브랜치에 체크아웃된 상태다.
2. **Given** 워크스페이스에 `.git/`이 존재하지 않을 때, **When** `add-project`를 실행하면, **Then** 별도 의무 없이 정상 완료된다.

### Story 2 — 기존 submodule 프로젝트 동작 유지 (P1)

현재 submodule로 추가된 프로젝트를 가진 사용자는 본 변경 이후에도 기존 워크스페이스에서 파이프라인을 변경 없이 사용할 수 있어야 한다.

**Acceptance**:
1. **Given** `.gitmodules`와 submodule로 등록된 service-a가 있는 워크스페이스에서, **When** `pylon request <요구사항>`을 실행하면, **Then** spec 002의 다중 repo 파이프라인이 변경 없이 동작한다.
2. **Given** 기존 submodule 프로젝트가 등록된 상태에서, **When** `pylon doctor`를 실행하면, **Then** submodule과 clone 양쪽의 `.pylon/` exclude 상태가 동일한 기준으로 보고된다.

### Story 3 — 명시적 마이그레이션 (P2)

기존 submodule 프로젝트를 clone 방식으로 전환하고자 하는 사용자는 `pylon migrate-project <name>`을 명시적으로 실행하여 안전하게 전환할 수 있어야 한다. 사용자 동의 없이 git 구조가 변경되어서는 안 된다.

**Acceptance**:
1. **Given** 워킹 트리가 dirty한 submodule에 대해, **When** `migrate-project`를 실행하면, **Then** 마이그레이션이 차단되고 어떤 git 상태도 변경되지 않는다.
2. **Given** origin에 push되지 않은 로컬 커밋이 있는 submodule에 대해, **When** `migrate-project`를 실행하면, **Then** 차단된다.
3. **Given** 모든 안전 조건을 만족하는 submodule에 대해, **When** `migrate-project <name>`을 실행하면, **Then** submodule 등록이 해제되고, 동일 위치에 같은 origin에서 clone되며, `.pylon/` 내용이 보존된다.
4. **Given** `--dry-run`을 사용하면, **When** 실행되면, **Then** 실제 변경 없이 차단 조건 점검 결과만 출력된다.

### Story 4 — `--force` 사용 시 submodule 잔재 보호 (P2)

사용자가 `--force`로 기존 디렉토리를 재clone하려 할 때 그 디렉토리가 기존 submodule이라면 실수로 submodule 잔재(.gitmodules 항목 등)가 어색하게 남거나 잘못 정리되지 않아야 한다.

**Acceptance**:
1. **Given** 기존 submodule 디렉토리에 대해, **When** `pylon add-project <url> --force`를 실행하면, **Then** 진행이 중단되고 "이 프로젝트는 submodule로 등록되어 있습니다. `pylon migrate-project <name>`을 먼저 사용하거나 `--force --migrate`를 명시하세요" 같은 안내가 출력된다.

---

## 4. Functional Requirements

- **FR-1**: `pylon add-project <url> [--name <n>]`은 `git clone <url> <name>`을 호출한다. `git submodule add` 호출 경로는 제거한다.
- **FR-2**: `pylon add-project --force`는 기존 디렉토리가 submodule로 등록되어 있다면(상위 워크스페이스의 `.gitmodules`에 항목 존재 OR `.git/modules/<name>` 존재) 중단하고 `migrate-project` 안내를 출력한다. `--migrate` 플래그를 함께 명시한 경우에만 진행하며, 이때의 동작은 다음과 같다: (a) §5.1의 차단 조건을 동일하게 점검하고, (b) 통과 시 §5.2의 1–5단계(submodule 해제 + commit 안내)를 수행한 뒤, (c) `--force`의 원래 의도대로 **새 URL/브랜치로 재clone** (§5.2의 6–9단계 대신, 사용자가 `add-project`에 넘긴 URL 사용).
- **FR-3**: `pylon add-project --skip-clone`은 현행대로 유지 — 이미 존재하는 디렉토리에 `.pylon/`만 생성한다.
- **FR-4**: 신설 명령 `pylon migrate-project <name> [--dry-run] [--force]`는 §5의 안전성 의미론에 따라 submodule을 일반 clone으로 전환한다.
- **FR-5**: `pylon uninstall --remove-projects`는 각 등록된 프로젝트를 검사하여:
  - submodule이면 기존 경로(`git submodule deinit` + `git rm` + `.git/modules/<name>` 정리)로 진행
  - 일반 clone이면 디렉토리 삭제로 진행
  결합 방식 감지 기준 (이 우선순위로 평가):
  1. 워크스페이스에 `.git/`이 없으면 → **clone**
  2. 워크스페이스의 `.gitmodules`에 해당 프로젝트 path 항목이 존재하면 → **submodule**
  3. 그 외(워크스페이스는 git repo이지만 해당 항목 없음) → **clone**
  이 기준은 다른 코드 경로(`migrate-project`, `add-project --force`, `doctor`)에서 동일하게 사용한다.
- **FR-6**: `pylon doctor`의 `checkSubmoduleExcludes`를 `checkRepoExcludes`로 일반화한다. 두 결합 방식 모두 `.git/info/exclude`에 `.pylon/` 존재 여부를 확인한다. `--fix-excludes` 플래그 동작은 양쪽 모두에 적용된다.
- **FR-7**: `pylon init`은 워크스페이스에 대해 `git init`을 호출하지 않는다. 워크스페이스 메타데이터(`.pylon/config.yml` 등)를 git으로 추적하고 싶은 사용자는 직접 `git init` 후 `git add`한다.
- **FR-8**: spec 002의 파이프라인 로직(브랜치 생성, 병렬 실행, PR 생성)은 변경하지 않는다.

---

## 5. `pylon migrate-project` 안전성 의미론

### 5.1 차단 조건 (각 조건 위반 시 기본은 중단, `--force`로만 우회)

| 조건 | 기본 동작 | `--force` 동작 |
|---|---|---|
| submodule 워킹 트리에 untracked / modified / staged 파일 존재 | 차단 | 변경 폐기 후 진행 |
| origin에 push되지 않은 로컬 커밋 존재 (모든 로컬 브랜치 기준) | 차단 (데이터 손실 위험) | 폐기 후 진행 |
| submodule에만 존재하고 origin에 없는 로컬 브랜치 존재 | 차단 (브랜치 목록 출력) | 폐기 |
| 슈퍼프로젝트가 핀한 SHA가 origin의 기본 브랜치 tip과 다름 | 경고 + 차단 | 기본 브랜치 tip으로 재clone (핀 손실 명시 안내) |

### 5.2 마이그레이션 절차 (성공 경로, atomic 보장)

1. **사전 검증**: §5.1의 차단 조건 모두 점검. 실패 시 어떤 git 상태도 변경하지 않고 종료.
2. **메타데이터 수집**: submodule의 origin URL, 현재 체크아웃 상태, 슈퍼프로젝트의 핀 SHA를 기록. 현재 상태가 detached HEAD라면 origin의 기본 브랜치(`git symbolic-ref refs/remotes/origin/HEAD` 또는 `origin/HEAD` 미설정 시 `main`/`master` 순으로 fallback)를 §5.2-6에서 사용할 체크아웃 대상으로 결정.
3. **`.pylon/` 임시 보관**: 하위 repo의 `.pylon/` 디렉토리 전체를 워크스페이스의 임시 위치(예: `<workspace>/.pylon/migrate-tmp/<name>/`)로 이동.
4. **submodule 해제**:
   - `git submodule deinit -f -- <name>` (워크스페이스에서)
   - `git rm -f <name>` (워크스페이스에서)
   - `rm -rf <workspace>/.git/modules/<name>`
5. **사용자에게 commit 안내 출력**: `.gitmodules`가 변경되었음을 알리고, 사용자가 직접 `git -C <workspace> add .gitmodules && git -C <workspace> commit`을 하도록 안내. (워크스페이스 자체에 대한 자동 commit은 하지 않음 — FR-7에 따라 워크스페이스 git 자체가 deprecated 방향.)
6. **재clone**: 동일 위치에 `git clone <origin-url> <name>`. 원래 체크아웃 브랜치(§5.2-2 기록)로 `git checkout`.
7. **`.pylon/` 복원**: 임시 위치에서 새 디렉토리로 이동. 임시 위치 정리.
8. **exclude 재설정**: 새 `.git/info/exclude`에 `.pylon/` 추가 (`excludePylonFromRepo`).
9. **SQLite 레코드**: 경로 동일하므로 변경 없음. 단, stack 재탐지 후 변경된 경우만 upsert.

### 5.3 실패 시 롤백

§5.2-4 이후 실패가 발생하면 다음 순서로 부분 복구를 시도한다:
- `.pylon/`은 임시 위치에 보관되어 있으므로 자동 손실 없음
- submodule 해제가 완료된 상태에서 clone이 실패하면 사용자에게 명확한 다음 액션 안내(같은 명령 재시도 또는 수동 clone)
- 완전 롤백(원래 submodule 상태로 복원)은 지원하지 않는다 (§5.2-4에서 슈퍼프로젝트 SHA 핀이 이미 분리됨). 이 비대칭성은 사전 검증(§5.1) 강도로 보완한다.

### 5.4 `--dry-run`

§5.1의 모든 차단 조건만 점검하여 결과를 출력하고 종료한다. 어떤 파일 시스템/git 상태도 변경하지 않는다.

---

## 6. 변경되는 파일

| 파일 | 변경 내용 |
|---|---|
| `internal/cli/add_project.go` | submodule 분기 제거, `excludePylonFromSubmodule` → `excludePylonFromRepo` 재명명, submodule 잔재 감지(FR-2) 추가, Short/Long 설명 갱신 |
| `internal/cli/migrate_project.go` | **신규** — §5 전체 구현 |
| `internal/cli/uninstall.go` | 프로젝트별 결합 방식 감지 + 분기 (FR-5) |
| `internal/cli/doctor.go` | `checkSubmoduleExcludes` → `checkRepoExcludes` 일반화 |
| `internal/cli/destroy.go` | "Git submodules are preserved" 문구 일반화 |
| `internal/cli/init_cmd.go` | `git init` 호출 블록(189–196) 및 관련 메시지 제거 |
| `internal/config/workspace.go` | 라인 58 주석 갱신 |
| `cmd/pylon/...` | `migrate-project` 명령 루트 등록 |
| `README.md` (라인 88, 272) | `add-project` 설명 문구 갱신, `migrate-project` 문서화 |
| `pylon-spec.md` Section 7 | 결합 방식 설명 갱신 |
| `docs/v2-rewrite/MIGRATION.md`, `docs/MIGRATION-V2.md` | 참조 갱신 |
| `IMPLEMENTATION_PLAN.md` | 해당 항목 갱신 |

---

## 7. Non-goals

다음 항목은 본 spec의 범위에 포함하지 않는다. 필요해질 경우 별도 spec으로 분리한다.

- `.pylon/projects/<name>/`로 하위 repo의 `.pylon/` 메타데이터를 이동하는 구조 개편
- 매니페스트 파일(repo 목록 + 핀 SHA)을 통한 팀 공유 모델
- `--shallow`, `--branch`, `--depth` 같은 clone 옵션 신설
- google-repo, jj workspace 등 다른 다중-repo 관리 도구 검토
- spec 002의 파이프라인 로직 수정
- 워크스페이스 자체의 협업/공유 모델 정의

---

## 8. Acceptance Criteria (요약)

- `pylon add-project <url>`이 `.gitmodules`를 생성하지 않고, 워크스페이스 `.git/` 존재를 요구하지 않는다.
- 기존 submodule 프로젝트가 등록된 워크스페이스에서 `pylon request`가 변경 없이 동작한다.
- `pylon migrate-project <name>`은 §5.1의 모든 차단 조건을 통과해야 진행되며, 차단 시 워크스페이스/하위 repo 상태를 변경하지 않는다.
- `pylon migrate-project <name> --dry-run`이 어떤 상태 변경 없이 점검 결과만 출력한다.
- `pylon doctor`가 submodule과 clone 양쪽 프로젝트의 `.pylon/` exclude 상태를 동일한 기준으로 보고한다.
- `pylon uninstall --remove-projects`가 두 결합 방식 모두 정리한다.
- `pylon init`을 실행한 워크스페이스에 `.git/` 디렉토리가 생성되지 않는다.

---

## 9. Open Questions

없음. 모든 결정사항은 §2에 명시되었다.
