# Implementation Plan: `add-project`를 독립 clone 방식으로 전환

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking in `tasks.md`.

**Branch**: `feat/003-add-project-clone` | **Date**: 2026-05-12 | **Spec**: [spec.md](./spec.md)

**Goal**: `pylon add-project`를 git submodule 기반에서 독립 `git clone` 기반으로 전환하고, 기존 submodule 프로젝트는 명시적인 `pylon migrate-project` 명령으로만 전환하도록 한다.

**Architecture**: 결합 방식 감지(`detectProjectCoupling`)를 단일 헬퍼로 추상화하여 `add-project --force`, `uninstall`, `doctor`, `migrate-project` 네 명령이 동일한 기준으로 분기한다. `pylon init`의 슈퍼프로젝트 `git init` 호출은 제거한다. 기존 코드 경로(submodule deinit)는 호환을 위해 보존하되, 신규 경로(`git clone`)와 명확히 분리한다.

**Tech Stack**: Go 1.24+, Cobra v1.10.2, Go `testing` 표준 패키지, `git`/`gh` CLI (외부 의존).

---

## 1. Summary

본 구현은 `internal/cli/add_project.go`의 단일 `git submodule add` 호출 경로를 `git clone`으로 교체하고, 결합 방식이 혼재하는 시점(전환기)을 대비해 결합 감지 헬퍼를 도입한다. 기존 submodule 프로젝트가 등록된 워크스페이스에서는 spec 002의 다중 repo 파이프라인이 변경 없이 동작한다(결합 중립). 마이그레이션은 `pylon migrate-project` 명령으로만 명시적으로 수행되며, spec §5의 안전성 의미론(dirty tree/unpushed commit/local branch/SHA mismatch 차단)을 그대로 적용한다.

## 2. Technical Context

- **언어**: Go 1.24+
- **주요 의존성**: Cobra v1.10.2 (CLI), `os/exec` (git 호출), `internal/store` (SQLite)
- **테스트**: Go `testing` 표준 패키지 (`go test ./...`). 기존 `add_project_test.go`, `uninstall_test.go`, `doctor_test.go` 패턴 준수 — 임시 디렉토리(`t.TempDir()`)에 `git init` 후 실제 git 명령 실행, `requireGit(t)` 헬퍼로 git 부재 시 스킵.
- **빌드**: `go build ./...`
- **린트**: `golangci-lint run ./...` (현재 CI에서 실행)
- **플랫폼**: Linux/macOS (darwin)
- **현재 브랜치**: `feat/003-add-project-clone` (spec.md commit `314b850`)

## 3. Constitution Check

| 원칙 | 적용 여부 | 판정 |
|------|----------|------|
| 하위 호환성 — spec 002 파이프라인 변경 없음 | 결합 중립, 코드 변경 없음 | ✅ PASS |
| 하위 호환성 — 기존 submodule 프로젝트 동작 유지 | 감지+분기로 보존 | ✅ PASS |
| 단순성 — Non-goals 준수 | spec §7 명시 (매니페스트, shallow, .pylon/projects 이동 등 제외) | ✅ PASS |
| 결정의 명시성 — 안전성 의미론은 spec §5에 결정 완료 | 재논의 없이 그대로 구현 | ✅ PASS |
| 사용자 데이터 보호 — 마이그레이션은 차단 우선, atomic | spec §5.1, §5.2 준수 | ✅ PASS |

**Gates**: 통과. 구현 진행 가능.

## 4. Project Structure

### Documentation (this feature)

```text
specs/003-add-project-clone/
├── spec.md              # 이미 commit (314b850)
├── plan.md              # 이 파일
└── tasks.md             # 단계별 TDD 체크리스트
```

### Source Code (변경 대상)

```text
internal/cli/
├── add_project.go        # 핵심: submodule add → git clone, --force 분기, 함수 재명명
├── add_project_test.go   # 신규 테스트 + 기존 테스트 함수명 갱신
├── migrate_project.go    # 신규: pylon migrate-project 명령 전체
├── migrate_project_test.go  # 신규: 안전성 차단 조건 + 마이그레이션 절차
├── coupling.go           # 신규: detectProjectCoupling 헬퍼 (단일 진실원)
├── coupling_test.go      # 신규: 3가지 케이스 (no .git / submodule / clone)
├── uninstall.go          # buildUninstallPlan/executeUninstall 분기 적용
├── uninstall_test.go     # 분기 케이스 추가
├── doctor.go             # checkSubmoduleExcludes → checkRepoExcludes 일반화
├── doctor_test.go        # 함수명 갱신
├── destroy.go            # 안내 문구 일반화
├── init_cmd.go           # 워크스페이스 git init 호출 블록 제거
└── root.go               # migrate-project 명령 등록

internal/config/
└── workspace.go          # 라인 58 주석 갱신

cmd/pylon/main.go         # 변경 없음 (cli.Execute에서 처리됨)

docs / 루트 문서:
├── README.md
├── pylon-spec.md         # Section 7
├── docs/MIGRATION-V2.md
├── docs/v2-rewrite/MIGRATION.md
└── IMPLEMENTATION_PLAN.md
```

### 새 파일의 책임 분리

- **`coupling.go`** — `type Coupling int` (CouplingNone/CouplingSubmodule/CouplingClone)과 `func detectProjectCoupling(workspaceRoot, projectName string) Coupling`만 export. 다른 명령들이 이 단일 함수를 호출하여 결합 방식을 일관되게 감지. **이유**: spec §FR-5의 우선순위 규칙을 한 곳에 두지 않으면 `migrate-project`, `add-project --force`, `uninstall`, `doctor`에 같은 규칙이 4번 중복된다.
- **`migrate_project.go`** — `newMigrateProjectCmd`, `runMigrateProject`, 그리고 spec §5.1 차단 조건 점검 함수 6개(`checkWorkingTreeClean`, `checkAllCommitsPushed`, `checkNoLocalOnlyBranches`, `checkSHAMatchesOrigin`, `collectMigrationMetadata`, `performMigration`). 한 파일 안에 응집.

## 5. Test Strategy

### TDD 사이클

각 작업은 다음 순서를 엄격히 따른다:

1. **실패 테스트 작성** → 함수/타입이 아직 없거나 동작이 변경됨
2. **실행 → 실패 확인** (`go test -run TestX ./internal/cli -v`)
3. **최소 구현**
4. **실행 → 통과 확인**
5. **회귀 테스트** (`go test ./...`)
6. **commit** (CLAUDE.md 규칙: 한국어, 결과물에만 기반)

### 테스트 패턴

기존 `add_project_test.go`의 패턴을 그대로 따른다:

```go
func TestX(t *testing.T) {
    requireGit(t)
    tmpDir := t.TempDir()
    // git init / submodule add 등 실제 git 호출로 fixture 구성
    // 함수 실행 후 파일 시스템 상태로 assert
}
```

### 결합 방식 fixture 구성

`migrate_project_test.go`와 `coupling_test.go`는 실제 submodule fixture를 만들어야 한다:

```go
func setupSubmoduleFixture(t *testing.T) (workspace, projectName string) {
    t.Helper()
    requireGit(t)
    workspace = t.TempDir()
    // 1. 워크스페이스 git init + 더미 commit (submodule add는 commit이 있어야 함)
    // 2. 별도 origin repo 생성 (file:// URL)
    // 3. git submodule add file://<origin> <name>
    // 4. 워크스페이스에서 submodule 초기 commit
    return workspace, "sub"
}
```

## 6. Phase Roadmap

각 Phase는 commit 단위. Phase 내부의 step은 `tasks.md`에서 체크박스로 추적.

| Phase | 내용 | 대상 Story |
|---|---|---|
| 1 | Setup — 회귀 베이스라인 확보 | — |
| 2 | Foundational — `detectProjectCoupling` 헬퍼, 함수 재명명, `init`의 git init 제거 | (기반) |
| 3 | US-1 — `add-project`가 `git clone` 사용 | Story 1 |
| 4 | US-4 — `--force`에서 submodule 잔재 보호 + `--migrate` | Story 4 |
| 5 | US-3 — `pylon migrate-project` 신설 (차단 조건 + 절차 + dry-run) | Story 3 |
| 6 | US-2 — `uninstall`/`doctor` 분기, 기존 submodule 동작 회귀 확인 | Story 2 |
| 7 | 문서 갱신 | — |
| 8 | 회귀 검증 (전체 테스트 + acceptance 시나리오) | — |

## 7. Risks and Rollback

| 위험 | 영향 | 완화 |
|---|---|---|
| `git submodule add`를 사용하던 사용자가 변경을 인지 못해 기존 워크스페이스가 혼란 | 사용자 혼동 | README + MIGRATION 문서에 명시, `add-project --force`가 submodule 잔재 감지 시 명확한 안내 출력 |
| `migrate-project`의 부분 실패(§5.2-4 이후 clone 실패) | 사용자가 수동 복구 필요 | spec §5.3 명시: `.pylon/` 임시 보관으로 데이터 손실 없음. 명확한 다음 액션 안내 |
| `coupling.go` 헬퍼의 오감지 | 잘못된 분기로 git 손상 가능 | 3 케이스 모두에 대한 단위 테스트 + 우선순위 규칙을 spec §FR-5에서 일자로 인용 |
| 기존 `excludePylonFromSubmodule` 사용처 누락 | 빌드 실패 | grep으로 모든 호출 사이트 찾고 일괄 갱신 |

**Rollback**: 전체 변경은 단일 feature 브랜치(`feat/003-add-project-clone`)에서 진행. PR 머지 전이라면 브랜치 폐기로 즉시 롤백. 머지 후에는 `git revert <merge-sha>` 한 번으로 전체 되돌릴 수 있도록 task별 commit이 누적된다(squash 금지 — 사용자 메모 [[no-squash-merge]] 준수).

## 8. Self-Review Checklist (작성자용)

- [ ] spec.md의 모든 FR(FR-1~FR-8)이 하나 이상의 task에 매핑되는가?
- [ ] spec §5.1의 4가지 차단 조건이 각각 별도 테스트로 다뤄지는가?
- [ ] `--dry-run`(spec FR-4)이 별도 task로 다뤄지는가?
- [ ] Acceptance Criteria(spec §8) 7가지가 모두 수동/자동 검증에 포함되는가?
- [ ] Non-goals(spec §7)에 명시된 항목이 task에 침투하지 않았는가?
- [ ] 모든 commit 메시지가 한국어 + 결과물 기반(CLAUDE.md 준수)인가?
