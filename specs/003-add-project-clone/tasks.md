# Tasks: `add-project`를 독립 clone 방식으로 전환

**Input**: Design documents from `/specs/003-add-project-clone/`
**Prerequisites**: spec.md ✅ (commit `314b850`), plan.md ✅
**Branch**: `feat/003-add-project-clone`

**Format**: `[ID] [P?] [Story?] Description`
- `[P]`: 병렬 실행 가능 (다른 파일 + 선행 task 의존성 없음)
- `[Story]`: spec §3의 User Story 매핑 (US1~US4)
- 모든 task는 spec과 plan.md의 결정을 그대로 따른다 (재논의 금지)
- 모든 commit 메시지는 한국어 + 결과물 기반 (CLAUDE.md)

---

## Phase 1: Setup

**Purpose**: 회귀 베이스라인 확보. 변경 전 현재 상태에서 모든 테스트가 통과해야 이후 단계에서 회귀를 감지할 수 있다.

### Task 1: 기존 테스트 베이스라인 확인

**Files**: 없음 (검증만)

- [ ] **Step 1-1**: `internal/cli` 패키지 전체 테스트 실행

```bash
go test ./internal/cli/... -v
```

기대: 모든 테스트 통과. 실패 시 본 변경을 시작하지 말 것.

- [ ] **Step 1-2**: 전체 프로젝트 빌드 확인

```bash
go build ./...
```

기대: 빌드 성공.

- [ ] **Step 1-3**: Phase 1 베이스라인 commit (변경 없음, 작업 시작 표시용 빈 commit)

```bash
git commit --allow-empty -m "chore(003): add-project clone 전환 작업 시작"
```

---

## Phase 2: Foundational — 공통 헬퍼 + 단순 갱신

**Purpose**: 이후 모든 phase가 의존하는 결합 감지 헬퍼와 함수 재명명, `pylon init`의 git init 호출 제거.

**⚠️ CRITICAL**: 이 Phase가 완료되어야 Phase 3 이후 작업이 의미 있음.

### Task 2: `detectProjectCoupling` 헬퍼 신설

**Files**:
- Create: `internal/cli/coupling.go`
- Test: `internal/cli/coupling_test.go`

spec §FR-5 기준:
1. 워크스페이스에 `.git/`이 없으면 → `CouplingClone`
2. 워크스페이스 `.gitmodules`에 해당 path 항목이 존재하면 → `CouplingSubmodule`
3. 그 외 → `CouplingClone`

- [ ] **Step 2-1**: 실패 테스트 작성

```go
// internal/cli/coupling_test.go
package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestDetectProjectCoupling_NoWorkspaceGit(t *testing.T) {
	tmp := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmp, "sub"), 0755); err != nil {
		t.Fatal(err)
	}
	if got := detectProjectCoupling(tmp, "sub"); got != CouplingClone {
		t.Errorf("got %v, want CouplingClone", got)
	}
}

func TestDetectProjectCoupling_WorkspaceGitNoGitmodules(t *testing.T) {
	requireGit(t)
	tmp := t.TempDir()
	if out, err := exec.Command("git", "init", tmp).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	if err := os.MkdirAll(filepath.Join(tmp, "sub"), 0755); err != nil {
		t.Fatal(err)
	}
	if got := detectProjectCoupling(tmp, "sub"); got != CouplingClone {
		t.Errorf("got %v, want CouplingClone", got)
	}
}

func TestDetectProjectCoupling_Submodule(t *testing.T) {
	requireGit(t)
	tmp := t.TempDir()
	if out, err := exec.Command("git", "init", tmp).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	gitmodules := "[submodule \"sub\"]\n\tpath = sub\n\turl = https://example.com/sub.git\n"
	if err := os.WriteFile(filepath.Join(tmp, ".gitmodules"), []byte(gitmodules), 0644); err != nil {
		t.Fatal(err)
	}
	if got := detectProjectCoupling(tmp, "sub"); got != CouplingSubmodule {
		t.Errorf("got %v, want CouplingSubmodule", got)
	}
}
```

- [ ] **Step 2-2**: 실패 확인

```bash
go test ./internal/cli -run TestDetectProjectCoupling -v
```

기대: FAIL (`undefined: detectProjectCoupling`, `undefined: CouplingClone`, `undefined: CouplingSubmodule`).

- [ ] **Step 2-3**: 구현

```go
// internal/cli/coupling.go
package cli

import (
	"os"
	"path/filepath"
	"strings"
)

// Coupling represents how a project is attached to the workspace.
type Coupling int

const (
	// CouplingClone means the project is a standalone git clone in the workspace.
	CouplingClone Coupling = iota
	// CouplingSubmodule means the project is registered as a git submodule of the workspace.
	CouplingSubmodule
)

func (c Coupling) String() string {
	switch c {
	case CouplingSubmodule:
		return "submodule"
	case CouplingClone:
		return "clone"
	default:
		return "unknown"
	}
}

// detectProjectCoupling determines how a project is attached to the workspace.
// Priority (per spec 003 §FR-5):
//  1. Workspace has no .git/ -> CouplingClone
//  2. Workspace .gitmodules has a [submodule] entry whose path equals projectName -> CouplingSubmodule
//  3. Otherwise -> CouplingClone
func detectProjectCoupling(workspaceRoot, projectName string) Coupling {
	if _, err := os.Stat(filepath.Join(workspaceRoot, ".git")); err != nil {
		return CouplingClone
	}
	data, err := os.ReadFile(filepath.Join(workspaceRoot, ".gitmodules"))
	if err != nil {
		return CouplingClone
	}
	if hasSubmodulePath(string(data), projectName) {
		return CouplingSubmodule
	}
	return CouplingClone
}

// hasSubmodulePath returns true if the .gitmodules contents declare a submodule
// whose path = projectName. Uses a simple line scanner that tolerates leading
// whitespace and either tab or space separators.
func hasSubmodulePath(gitmodules, projectName string) bool {
	for _, line := range strings.Split(gitmodules, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "path") {
			continue
		}
		// path = <value>
		idx := strings.Index(trimmed, "=")
		if idx < 0 {
			continue
		}
		val := strings.TrimSpace(trimmed[idx+1:])
		if val == projectName {
			return true
		}
	}
	return false
}
```

- [ ] **Step 2-4**: 통과 확인

```bash
go test ./internal/cli -run TestDetectProjectCoupling -v
```

기대: 3개 테스트 모두 PASS.

- [ ] **Step 2-5**: commit

```bash
git add internal/cli/coupling.go internal/cli/coupling_test.go
git commit -m "feat(003): detectProjectCoupling 헬퍼 추가

워크스페이스 .git/ 존재 여부와 .gitmodules 항목을 기준으로
프로젝트의 결합 방식(clone/submodule)을 단일 진실원으로 판정한다.
add-project --force, uninstall, doctor, migrate-project가
이 함수를 공유한다."
```

---

### Task 3: `excludePylonFromSubmodule` → `excludePylonFromRepo` 재명명

**Files**:
- Modify: `internal/cli/add_project.go:257` (함수 정의), `:189` (호출 사이트), `:193` (메시지)
- Modify: `internal/cli/doctor.go:171` (호출 사이트)
- Modify: `internal/cli/add_project_test.go:39, 49, 64, 73, 76, 92, 95` (테스트 함수명 및 호출)

- [ ] **Step 3-1**: 모든 사용 사이트 grep으로 열거

```bash
grep -rn "excludePylonFromSubmodule" internal/
```

기대 출력 (현 시점):
- `internal/cli/add_project.go:189, 193, 255, 257`
- `internal/cli/add_project_test.go:39, 49, 64, 73, 76, 92, 95`
- `internal/cli/doctor.go:171`

- [ ] **Step 3-2**: 모든 사이트를 일괄 재명명 (이름 변경만)

```bash
# add_project.go
# - 함수 정의: func excludePylonFromSubmodule(projectDir string) error → func excludePylonFromRepo(projectDir string) error
# - 호출: if err := excludePylonFromSubmodule(projectDir); err != nil {
#   → if err := excludePylonFromRepo(projectDir); err != nil {
# - 메시지: "✓ .pylon/ excluded from submodule git tracking"
#   → "✓ .pylon/ excluded from project git tracking"
# - 주석 헤더 (line 255): "excludePylonFromSubmodule adds .pylon/ to the submodule's git exclude file"
#   → "excludePylonFromRepo adds .pylon/ to the repo's git exclude file (works for both submodules and standalone clones)"
```

```bash
# doctor.go:171
# excludePylonFromSubmodule(p.Path) → excludePylonFromRepo(p.Path)
```

```bash
# add_project_test.go
# - 테스트 함수: TestExcludePylonFromSubmodule → TestExcludePylonFromRepo
#               TestExcludePylonFromSubmodule_Idempotent → TestExcludePylonFromRepo_Idempotent
#               TestExcludePylonFromSubmodule_NotGitRepo → TestExcludePylonFromRepo_NotGitRepo
# - 호출 사이트: excludePylonFromSubmodule(tmpDir) → excludePylonFromRepo(tmpDir)
```

- [ ] **Step 3-3**: 컴파일 + 테스트

```bash
go build ./... && go test ./internal/cli -run TestExcludePylonFromRepo -v
```

기대: 빌드 성공, 3개 테스트 모두 PASS.

- [ ] **Step 3-4**: 잔존 사용처가 없는지 재확인

```bash
grep -rn "excludePylonFromSubmodule" internal/
```

기대: 출력 없음.

- [ ] **Step 3-5**: commit

```bash
git add internal/cli/add_project.go internal/cli/add_project_test.go internal/cli/doctor.go
git commit -m "refactor(003): excludePylonFromSubmodule을 excludePylonFromRepo로 재명명

함수 동작은 submodule과 standalone clone 양쪽 모두에 동일하게 적용된다.
이름이 동작 의미와 일치하도록 일반화한다."
```

---

### Task 4: `pylon init`의 워크스페이스 `git init` 호출 제거

**Files**:
- Modify: `internal/cli/init_cmd.go:189-196` (블록 제거)
- Modify: `internal/cli/init_cmd.go:221` (이미 init된 워크스페이스의 submodule exclude 처리 — 검토 후 잔존 결정)
- Modify: `internal/cli/init_cmd_test.go` (해당 동작을 검증하는 테스트가 있다면)

- [ ] **Step 4-1**: `init_cmd.go`에서 git init 관련 동작 검증을 위한 테스트 작성

```go
// internal/cli/init_cmd_test.go 에 추가
func TestRunInit_DoesNotGitInitWorkspace(t *testing.T) {
	requireGit(t)
	tmp := t.TempDir()
	cmd := newInitCmd()
	cmd.SetArgs([]string{tmp})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("init failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmp, ".git")); err == nil {
		t.Errorf("expected workspace to NOT be a git repo, but .git/ exists")
	}
}
```

(파일 상단 import에 `"os"`, `"path/filepath"`, `"testing"`이 있는지 확인 후 추가)

- [ ] **Step 4-2**: 실패 확인

```bash
go test ./internal/cli -run TestRunInit_DoesNotGitInitWorkspace -v
```

기대: FAIL (현재는 init이 git init을 수행).

- [ ] **Step 4-3**: `init_cmd.go:189-196`의 블록 제거

```go
// 제거 대상:
// Step 6: git init (skip if already a git repo)
if _, err := os.Stat(filepath.Join(workDir, ".git")); os.IsNotExist(err) {
    fmt.Println("Initializing git repository...")
    gitInit := exec.Command("git", "init", workDir)
    if out, err := gitInit.CombinedOutput(); err != nil {
        fmt.Printf("Warning: git init failed: %s\n", string(out))
    }
}
```

해당 블록을 완전히 삭제. 이후 단계 번호(`Step 7: ...`)도 갱신.

- [ ] **Step 4-4**: 라인 221 근처의 "Ensure .pylon/ is excluded from submodule git tracking" 주석 일반화

해당 블록(`internal/cli/init_cmd.go:221-227`)은 `DiscoverProjects`가 찾은 **하위 프로젝트들**에 대해 `excludePylonFromRepo(p.Path)`를 호출한다(워크스페이스 자체가 아님). Task 3에서 함수가 이미 재명명되었으므로 동작은 그대로 유지하면 된다. 주석만 일반화한다:

```go
// 변경 전:
// Ensure .pylon/ is excluded from submodule git tracking (skip non-git dirs)
// 변경 후:
// Ensure .pylon/ is excluded from project git tracking (skip non-git dirs)
```

- [ ] **Step 4-5**: 빌드 + 새 테스트 통과 확인

```bash
go build ./... && go test ./internal/cli -run TestRunInit -v
```

기대: 빌드 성공, 새 테스트 포함 모든 init 테스트 PASS.

- [ ] **Step 4-6**: 전체 회귀

```bash
go test ./internal/cli/... -v
```

기대: 모든 테스트 PASS.

- [ ] **Step 4-7**: commit

```bash
git add internal/cli/init_cmd.go internal/cli/init_cmd_test.go
git commit -m "feat(003): pylon init이 워크스페이스를 git init하지 않도록 변경

워크스페이스 자체는 더 이상 슈퍼프로젝트로 사용되지 않는다.
사용자가 .pylon/config.yml 등을 git으로 추적하려면
직접 git init을 실행해야 한다."
```

---

### Task 5: `internal/config/workspace.go` 주석 갱신

**Files**: Modify: `internal/config/workspace.go:58`

- [ ] **Step 5-1**: 현재 주석 확인

```bash
sed -n '55,62p' internal/config/workspace.go
```

- [ ] **Step 5-2**: "git submodule" 언급을 일반화

기존: `// or being registered as a git submodule.`
변경 후: `// or being added as a standalone git clone in the workspace.`

(전체 주석 문맥에 맞게 한 줄 수정)

- [ ] **Step 5-3**: 빌드 확인 + commit

```bash
go build ./...
git add internal/config/workspace.go
git commit -m "docs(003): workspace.go 주석에서 submodule 가정 제거"
```

---

## Phase 3: User Story 1 — 신규 `add-project`는 `git clone`

**Goal**: `pylon add-project <url>`이 `git submodule add` 대신 `git clone`을 호출한다 (spec FR-1).

### Task 6: `add-project`의 결합 방식을 `git clone`으로 교체

**Files**:
- Modify: `internal/cli/add_project.go:117-126` (Step 1: Add git submodule 블록)
- Modify: `internal/cli/add_project.go:19-22` (Use/Short/Long 갱신)
- Modify: `internal/cli/add_project.go:34-36` (--skip-clone Help 텍스트)
- Modify: `internal/cli/add_project.go:82-98` (`--force` 분기는 Task 7에서 더 손봄. 일단 submodule 잔재 정리 코드는 그대로 두되 호출 조건만 정리)
- Test: `internal/cli/add_project_test.go` 신규 테스트

- [ ] **Step 6-1**: 신규 테스트 추가 — 새 프로젝트 추가 시 `.gitmodules`가 워크스페이스에 생기지 않고, 하위 디렉토리가 일반 clone 결과인지 확인

```go
// internal/cli/add_project_test.go 에 추가
func TestRunAddProject_UsesPlainClone(t *testing.T) {
	requireGit(t)

	// 워크스페이스(.pylon/만 있음, git 없음)
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, ".pylon"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, ".pylon", "config.yml"), []byte("version: \"0.1\"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// 더미 origin repo (bare가 아닌 일반 repo면 충분)
	origin := t.TempDir()
	if out, err := exec.Command("git", "init", origin).CombinedOutput(); err != nil {
		t.Fatalf("git init origin: %v\n%s", err, out)
	}
	if err := os.WriteFile(filepath.Join(origin, "README.md"), []byte("hi\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, origin, "add", ".")
	runGit(t, origin, "-c", "user.email=t@t", "-c", "user.name=t", "commit", "-m", "init")

	addCmd := newAddProjectCmd()
	addCmd.SetArgs([]string{"file://" + origin, "--name", "myproj"})

	oldWorkspace := flagWorkspace
	flagWorkspace = workspace
	defer func() { flagWorkspace = oldWorkspace }()

	// agent prompt 답 자동화
	withStdin(t, "n\n", func() {
		if err := addCmd.Execute(); err != nil {
			t.Fatalf("add-project failed: %v", err)
		}
	})

	// 1) 워크스페이스에 .gitmodules가 생기지 않아야 함
	if _, err := os.Stat(filepath.Join(workspace, ".gitmodules")); err == nil {
		t.Errorf(".gitmodules should not be created in workspace")
	}
	// 2) 워크스페이스에 .git/이 자동 생성되지 않아야 함
	if _, err := os.Stat(filepath.Join(workspace, ".git")); err == nil {
		t.Errorf("workspace .git/ should not be created")
	}
	// 3) 하위 디렉토리는 일반 git clone 결과 (정상 디렉토리 .git)
	subGit := filepath.Join(workspace, "myproj", ".git")
	info, err := os.Stat(subGit)
	if err != nil {
		t.Fatalf("sub project .git missing: %v", err)
	}
	if !info.IsDir() {
		t.Errorf("sub project .git should be a directory (clone), got gitlink file")
	}
	// 4) .pylon/이 sub 디렉토리에 생성됨
	if _, err := os.Stat(filepath.Join(workspace, "myproj", ".pylon", "context.md")); err != nil {
		t.Errorf("expected sub .pylon/context.md, got %v", err)
	}
}
```

테스트 헬퍼 `runGit`과 `withStdin`이 패키지 내 다른 곳에 있다면 재사용. 없다면 동일 파일에 추가:

```go
func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func withStdin(t *testing.T, input string, fn func()) {
	t.Helper()
	oldStdin := os.Stdin
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	if _, err := w.WriteString(input); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	os.Stdin = r
	defer func() {
		os.Stdin = oldStdin
		_ = r.Close()
	}()
	fn()
}
```

- [ ] **Step 6-2**: 실패 확인

```bash
go test ./internal/cli -run TestRunAddProject_UsesPlainClone -v
```

기대: FAIL — 현재 코드는 `git submodule add` 호출 시 워크스페이스가 git repo가 아니어서 에러. 또는 `.gitmodules`가 생기는 경로.

- [ ] **Step 6-3**: `add_project.go:117-126`의 submodule add 블록을 clone으로 교체

```go
// 변경 전:
// Step 1: Add git submodule (skipped when --skip-clone)
if !skipClone {
    fmt.Printf("Adding git submodule: %s\n", repoURL)
    gitCmd := exec.Command("git", "submodule", "add", repoURL, projectName)
    gitCmd.Dir = root
    if output, err := gitCmd.CombinedOutput(); err != nil {
        return fmt.Errorf("failed to add submodule: %w\n%s", err, output)
    }
    fmt.Printf("✓ Submodule added: %s\n", projectName)
}

// 변경 후:
// Step 1: Clone the project (skipped when --skip-clone)
if !skipClone {
    fmt.Printf("Cloning: %s\n", repoURL)
    gitCmd := exec.Command("git", "clone", repoURL, projectName)
    gitCmd.Dir = root
    if output, err := gitCmd.CombinedOutput(); err != nil {
        return fmt.Errorf("failed to clone: %w\n%s", err, output)
    }
    fmt.Printf("✓ Cloned: %s\n", projectName)
}
```

- [ ] **Step 6-4**: 명령 메타 갱신

```go
// add_project.go:18-30
cmd := &cobra.Command{
    Use:   "add-project [git-url]",
    Short: "Add a project as a standalone git clone in the workspace",
    Long: `Add a project to the workspace as a standalone git clone.

Analyzes the codebase and creates project-level .pylon/ configuration
including context.md and default agent definitions.

If the project directory already exists, use --force to re-clone or
--skip-clone to keep the existing directory and only generate .pylon/ config.

If the project directory exists as a legacy git submodule, --force is blocked
to prevent accidental data loss. Use 'pylon migrate-project <name>' first,
or pass --migrate together with --force to convert and re-clone.

Spec Reference: spec 003.`,
    Args: cobra.ExactArgs(1),
    RunE: runAddProject,
}
```

- [ ] **Step 6-5**: `--skip-clone` 플래그 Help 텍스트 갱신

```go
cmd.Flags().Bool("skip-clone", false, "skip git clone; use existing directory for .pylon/ setup only")
```

- [ ] **Step 6-6**: `--force` 경로의 submodule 정리 코드(`add_project.go:82-98`)는 Task 7에서 분기로 다시 손본다. 이번 step에서는 **그대로 둔다** — 워크스페이스가 git repo가 아닌 경우 `git submodule deinit -f`는 silent fail 처리되므로 일단 문제 없음(`flagVerbose` 분기에서 메시지만 출력). 단, Step 6-1 테스트는 이 경로를 거치지 않는다(디렉토리가 존재하지 않으므로).

- [ ] **Step 6-7**: 새 테스트 통과 확인

```bash
go test ./internal/cli -run TestRunAddProject_UsesPlainClone -v
```

기대: PASS.

- [ ] **Step 6-8**: 기존 add-project 테스트 회귀

```bash
go test ./internal/cli -run TestRunAddProject -v
```

기대: 모든 기존 add-project 테스트 PASS.

- [ ] **Step 6-9**: commit

```bash
git add internal/cli/add_project.go internal/cli/add_project_test.go
git commit -m "feat(003): add-project가 git submodule 대신 git clone 사용

워크스페이스가 git repo일 필요가 없어졌으며 .gitmodules가 생성되지 않는다.
하위 프로젝트는 default 브랜치에 체크아웃된 일반 clone 상태로 추가된다.
spec 003 §FR-1, US-1."
```

---

## Phase 4: User Story 4 — `--force`에서 submodule 잔재 보호 + `--migrate`

**Goal**: 사용자가 기존 submodule 디렉토리에 대해 `--force`를 잘못 사용하지 않도록 차단하고, 명시적으로 `--migrate`를 함께 쓴 경우에만 마이그레이션 후 재clone (spec FR-2, US-4).

### Task 7: `--force` 분기에 결합 방식 감지 적용 + `--migrate` 플래그

**Files**:
- Modify: `internal/cli/add_project.go:34-37` (`--migrate` 플래그 추가)
- Modify: `internal/cli/add_project.go:82-98` (--force 분기 재작성)
- Test: `internal/cli/add_project_test.go` 신규 테스트 2개

- [ ] **Step 7-1**: 실패 테스트 작성 — submodule 잔재가 있는 디렉토리에 `--force`만 줬을 때 차단

```go
// internal/cli/add_project_test.go 에 추가
func TestRunAddProject_ForceBlockedOnSubmoduleRemnant(t *testing.T) {
	requireGit(t)
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, ".pylon"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, ".pylon", "config.yml"), []byte("version: \"0.1\"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	// 워크스페이스를 git repo로 만들고 .gitmodules에 항목 추가 (잔재 모사)
	runGit(t, workspace, "init")
	gitmodules := "[submodule \"myproj\"]\n\tpath = myproj\n\turl = https://example.com/myproj.git\n"
	if err := os.WriteFile(filepath.Join(workspace, ".gitmodules"), []byte(gitmodules), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(workspace, "myproj"), 0755); err != nil {
		t.Fatal(err)
	}

	cmd := newAddProjectCmd()
	cmd.SetArgs([]string{"https://example.com/myproj.git", "--name", "myproj", "--force"})

	oldWorkspace := flagWorkspace
	flagWorkspace = workspace
	defer func() { flagWorkspace = oldWorkspace }()

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected --force on submodule to be blocked, got nil")
	}
	if !strings.Contains(err.Error(), "migrate-project") {
		t.Errorf("error should suggest migrate-project: %v", err)
	}
}
```

- [ ] **Step 7-2**: 실패 확인

```bash
go test ./internal/cli -run TestRunAddProject_ForceBlockedOnSubmoduleRemnant -v
```

기대: FAIL — 현재는 차단 없이 진행.

- [ ] **Step 7-3**: `--migrate` 플래그 추가

```go
// add_project.go:34-37 직후에 추가
cmd.Flags().Bool("migrate", false, "convert legacy submodule to clone (must be combined with --force)")
```

- [ ] **Step 7-4**: `--force` 분기 재작성

```go
// 변경 전 (add_project.go:81-98 안의 force 케이스):
switch {
case force:
    fmt.Printf("Removing existing directory: %s\n", projectName)
    // Remove submodule registration if it exists
    deregCmd := exec.Command("git", "submodule", "deinit", "-f", projectName)
    deregCmd.Dir = root
    if out, err := deregCmd.CombinedOutput(); err != nil && flagVerbose {
        fmt.Printf("  (submodule deinit skipped: %s)\n", strings.TrimSpace(string(out)))
    }
    rmCmd := exec.Command("git", "rm", "-f", projectName)
    rmCmd.Dir = root
    if out, err := rmCmd.CombinedOutput(); err != nil && flagVerbose {
        fmt.Printf("  (git rm skipped: %s)\n", strings.TrimSpace(string(out)))
    }
    gitModulesDir := filepath.Join(root, ".git", "modules", projectName)
    os.RemoveAll(gitModulesDir)
    if err := os.RemoveAll(projectDir); err != nil {
        return fmt.Errorf("failed to remove existing directory: %w", err)
    }

// 변경 후:
case force:
    migrate, _ := cmd.Flags().GetBool("migrate")
    coupling := detectProjectCoupling(root, projectName)
    if coupling == CouplingSubmodule {
        if !migrate {
            return fmt.Errorf("%s is registered as a git submodule. Run 'pylon migrate-project %s' first, or pass --migrate together with --force to convert and re-clone", projectName, projectName)
        }
        // --force --migrate: spec 003 §5.1 차단 조건을 점검한 뒤 §5.2-1~5 (submodule 해제)를 수행
        if err := runSubmoduleSafetyChecks(root, projectName, false); err != nil {
            return fmt.Errorf("migration blocked: %w (use 'pylon migrate-project --force' to override)", err)
        }
        if err := teardownSubmodule(root, projectName); err != nil {
            return fmt.Errorf("submodule teardown failed: %w", err)
        }
        fmt.Printf("✓ Submodule '%s' deregistered; ready for re-clone\n", projectName)
        fmt.Println("  Note: workspace .gitmodules was modified. Commit manually if you track the workspace in git.")
    }
    fmt.Printf("Removing existing directory: %s\n", projectName)
    if err := os.RemoveAll(projectDir); err != nil {
        return fmt.Errorf("failed to remove existing directory: %w", err)
    }
```

`runSubmoduleSafetyChecks`와 `teardownSubmodule`은 Task 9에서 `migrate_project.go`에 작성한다. Task 7 시점에서는 컴파일을 위해 빈 stub만 같은 파일(또는 신규 빈 `migrate_project.go`)에 둔다:

```go
// 임시 stub — Task 9에서 본 구현으로 교체된다.
func runSubmoduleSafetyChecks(workspaceRoot, projectName string, forceOverride bool) error {
    return nil
}
func teardownSubmodule(workspaceRoot, projectName string) error {
    // 임시: 기존 add_project.go의 submodule 정리 코드를 그대로 옮긴 단순 버전
    deregCmd := exec.Command("git", "submodule", "deinit", "-f", projectName)
    deregCmd.Dir = workspaceRoot
    _, _ = deregCmd.CombinedOutput()
    rmCmd := exec.Command("git", "rm", "-f", projectName)
    rmCmd.Dir = workspaceRoot
    _, _ = rmCmd.CombinedOutput()
    return os.RemoveAll(filepath.Join(workspaceRoot, ".git", "modules", projectName))
}
```

위 stub은 신규 파일 `internal/cli/migrate_project.go`에 둔다. Task 9에서 이 파일을 확장한다.

- [ ] **Step 7-5**: 컴파일 + 새 테스트 통과 확인

```bash
go build ./...
go test ./internal/cli -run TestRunAddProject_ForceBlockedOnSubmoduleRemnant -v
```

기대: 빌드 성공, 테스트 PASS.

- [ ] **Step 7-6**: 기존 `--force` 테스트 회귀 (`TestRunAddProject_ForceAndSkipCloneMutuallyExclusive` 등)

```bash
go test ./internal/cli -run TestRunAddProject -v
```

기대: 모든 PASS.

- [ ] **Step 7-7**: commit

```bash
git add internal/cli/add_project.go internal/cli/add_project_test.go internal/cli/migrate_project.go
git commit -m "feat(003): add-project --force가 submodule 잔재 감지 시 차단

기존 submodule 디렉토리에 대해 --force만 사용하면
'pylon migrate-project' 안내를 출력하고 중단한다.
--migrate 플래그를 함께 사용한 경우에만 안전성 검사 후 변환을 수행한다.
spec 003 §FR-2, US-4."
```

---

## Phase 5: User Story 3 — `pylon migrate-project`

**Goal**: spec §5의 전체 안전성 의미론을 구현 (FR-4, US-3).

### Task 8: 명령 골격 + `coupling` 사전 검증 + 등록

**Files**:
- Modify: `internal/cli/migrate_project.go` (Task 7의 stub 확장)
- Modify: `internal/cli/root.go:69` 직후 (`AddCommand(newMigrateProjectCmd())` 등록)
- Test: `internal/cli/migrate_project_test.go` 신규

- [ ] **Step 8-1**: 실패 테스트 — clone 프로젝트 대상으로 `migrate-project` 호출 시 거절

```go
// internal/cli/migrate_project_test.go
package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunMigrateProject_RejectsCloneProject(t *testing.T) {
	requireGit(t)
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, ".pylon"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, ".pylon", "config.yml"), []byte("version: \"0.1\"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	// 일반 clone 흉내: 하위 디렉토리에 git init, 워크스페이스는 git 아님
	projectDir := filepath.Join(workspace, "myproj")
	if out, err := exec.Command("git", "init", projectDir).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}

	cmd := newMigrateProjectCmd()
	cmd.SetArgs([]string{"myproj"})

	oldWorkspace := flagWorkspace
	flagWorkspace = workspace
	defer func() { flagWorkspace = oldWorkspace }()

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for non-submodule project")
	}
	if !strings.Contains(err.Error(), "not a submodule") && !strings.Contains(err.Error(), "submodule") {
		t.Errorf("unexpected error: %v", err)
	}
}
```

- [ ] **Step 8-2**: `newMigrateProjectCmd` 구현 + 등록

```go
// internal/cli/migrate_project.go — Task 7의 stub을 다음으로 교체
package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/kyago/pylon/internal/config"
)

func newMigrateProjectCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "migrate-project [name]",
		Short: "Convert a legacy git submodule project to a standalone clone",
		Long: `Convert a project that was previously added as a git submodule
into a standalone git clone in the workspace.

Safety checks (spec 003 §5.1) block migration when:
  - the submodule working tree is dirty
  - there are local commits not pushed to origin
  - there are local-only branches
  - the pinned SHA differs from origin's default branch tip

Use --dry-run to preview check results without changing any state.
Use --force to override safety checks (data loss possible).`,
		Args: cobra.ExactArgs(1),
		RunE: runMigrateProject,
	}
	cmd.Flags().Bool("dry-run", false, "report safety check results without changing state")
	cmd.Flags().Bool("force", false, "override safety checks (may discard local changes)")
	return cmd
}

func runMigrateProject(cmd *cobra.Command, args []string) error {
	projectName := args[0]

	startDir := flagWorkspace
	if startDir == "" {
		startDir = "."
	}
	root, err := config.FindWorkspaceRoot(startDir)
	if err != nil {
		return fmt.Errorf("not in a pylon workspace — run 'pylon init' first")
	}

	if err := validateProjectName(projectName); err != nil {
		return err
	}

	if detectProjectCoupling(root, projectName) != CouplingSubmodule {
		return fmt.Errorf("%s is not a submodule (nothing to migrate)", projectName)
	}

	dryRun, _ := cmd.Flags().GetBool("dry-run")
	force, _ := cmd.Flags().GetBool("force")

	if err := runSubmoduleSafetyChecks(root, projectName, force); err != nil {
		return fmt.Errorf("migration blocked: %w", err)
	}
	if dryRun {
		fmt.Println("✓ All safety checks passed (dry-run; no changes were made)")
		return nil
	}

	return performMigration(root, projectName)
}

// runSubmoduleSafetyChecks is implemented in Task 9.
// teardownSubmodule and performMigration are implemented in Tasks 9-10.
```

`root.go`에 등록:

```go
// internal/cli/root.go:69 직후
rootCmd.AddCommand(newMigrateProjectCmd())
```

Task 7에서 만든 stub `runSubmoduleSafetyChecks`/`teardownSubmodule`은 그대로 유지 (다음 task에서 본 구현으로 교체). `performMigration`은 새 stub:

```go
func performMigration(workspaceRoot, projectName string) error {
	return fmt.Errorf("not implemented (Task 10)")
}
```

- [ ] **Step 8-3**: 테스트 통과 확인

```bash
go build ./... && go test ./internal/cli -run TestRunMigrateProject_RejectsCloneProject -v
```

기대: PASS.

- [ ] **Step 8-4**: commit

```bash
git add internal/cli/migrate_project.go internal/cli/migrate_project_test.go internal/cli/root.go
git commit -m "feat(003): pylon migrate-project 명령 등록

submodule이 아닌 프로젝트에 대해서는 즉시 거절한다.
안전성 검사와 마이그레이션 절차는 후속 task에서 구현한다."
```

---

### Task 9: `runSubmoduleSafetyChecks` 본 구현 + 각 차단 조건 테스트

**Files**:
- Modify: `internal/cli/migrate_project.go`
- Modify: `internal/cli/migrate_project_test.go`

spec §5.1의 4개 차단 조건:

| 조건 | 검출 방식 |
|---|---|
| dirty working tree | `git -C <submodule> status --porcelain` 출력이 비어있지 않으면 dirty |
| unpushed commits | 모든 로컬 브랜치에 대해 `git -C <submodule> log @{u}..HEAD --oneline`이 비어있지 않거나, upstream 미설정 |
| local-only branches | `git -C <submodule> for-each-ref refs/heads/`로 로컬 브랜치 열거 후, 각 브랜치의 upstream이 없거나 origin에 동일 이름 ref가 없으면 local-only |
| SHA mismatch | 슈퍼프로젝트의 핀 SHA(`git -C <workspace> ls-tree HEAD <name>`로 추출)와 `git -C <submodule> rev-parse origin/HEAD`의 SHA 비교 |

각 검사는 함수로 분리하여 테스트 가능하게 만든다.

- [ ] **Step 9-1**: 차단 조건 1 (dirty) 실패 테스트

```go
func TestSafetyCheck_DirtyWorkingTree(t *testing.T) {
	requireGit(t)
	ws, name, sub := setupSubmoduleFixture(t)
	// submodule 안에 untracked 파일 생성
	if err := os.WriteFile(filepath.Join(sub, "dirty.txt"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	err := runSubmoduleSafetyChecks(ws, name, false)
	if err == nil || !strings.Contains(err.Error(), "dirty") {
		t.Fatalf("expected dirty error, got %v", err)
	}
	// --force는 통과
	if err := runSubmoduleSafetyChecks(ws, name, true); err != nil {
		t.Errorf("--force should bypass, got %v", err)
	}
}
```

`setupSubmoduleFixture`는 헬퍼:

```go
// internal/cli/migrate_project_test.go 상단에 헬퍼
func setupSubmoduleFixture(t *testing.T) (workspace, name, subPath string) {
	t.Helper()
	requireGit(t)
	name = "sub"

	// origin repo (bare 아니어도 됨)
	origin := t.TempDir()
	runGit(t, origin, "init")
	if err := os.WriteFile(filepath.Join(origin, "README.md"), []byte("hi\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, origin, "add", ".")
	runGit(t, origin, "-c", "user.email=t@t", "-c", "user.name=t", "commit", "-m", "init")

	// workspace as superproject
	workspace = t.TempDir()
	runGit(t, workspace, "init")
	runGit(t, workspace, "-c", "user.email=t@t", "-c", "user.name=t", "commit", "--allow-empty", "-m", "ws init")
	runGit(t, workspace, "-c", "protocol.file.allow=always", "submodule", "add", "file://"+origin, name)
	runGit(t, workspace, "-c", "user.email=t@t", "-c", "user.name=t", "commit", "-m", "add submodule")

	subPath = filepath.Join(workspace, name)
	return workspace, name, subPath
}
```

- [ ] **Step 9-2**: 실패 확인 (`runSubmoduleSafetyChecks`가 stub이라 에러 없음)

```bash
go test ./internal/cli -run TestSafetyCheck_DirtyWorkingTree -v
```

기대: FAIL.

- [ ] **Step 9-3**: dirty 검사 구현 + 통과

```go
// migrate_project.go
func runSubmoduleSafetyChecks(workspaceRoot, projectName string, force bool) error {
	subDir := filepath.Join(workspaceRoot, projectName)
	if force {
		return nil
	}
	if err := checkWorkingTreeClean(subDir); err != nil {
		return err
	}
	// 추가 검사는 후속 step에서 차례로
	return nil
}

func checkWorkingTreeClean(subDir string) error {
	out, err := exec.Command("git", "-C", subDir, "status", "--porcelain").Output()
	if err != nil {
		return fmt.Errorf("git status failed: %w", err)
	}
	if len(out) > 0 {
		return fmt.Errorf("working tree is dirty (untracked or modified files); commit or stash, or rerun with --force to discard")
	}
	return nil
}
```

- [ ] **Step 9-4**: 통과 확인

```bash
go test ./internal/cli -run TestSafetyCheck_DirtyWorkingTree -v
```

기대: PASS.

- [ ] **Step 9-5**: 차단 조건 2 (unpushed commits) 실패 테스트 + 구현

```go
func TestSafetyCheck_UnpushedCommits(t *testing.T) {
	requireGit(t)
	ws, name, sub := setupSubmoduleFixture(t)
	// submodule 내부에서 새 커밋 (origin push 없음)
	if err := os.WriteFile(filepath.Join(sub, "x.txt"), []byte("y"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, sub, "add", ".")
	runGit(t, sub, "-c", "user.email=t@t", "-c", "user.name=t", "commit", "-m", "local only")
	err := runSubmoduleSafetyChecks(ws, name, false)
	if err == nil || !strings.Contains(err.Error(), "unpushed") {
		t.Fatalf("expected unpushed error, got %v", err)
	}
}
```

```go
// migrate_project.go에 추가
func checkAllCommitsPushed(subDir string) error {
	// 모든 로컬 브랜치 열거
	out, err := exec.Command("git", "-C", subDir, "for-each-ref", "--format=%(refname:short)", "refs/heads/").Output()
	if err != nil {
		return fmt.Errorf("for-each-ref failed: %w", err)
	}
	branches := strings.Fields(strings.TrimSpace(string(out)))
	if len(branches) == 0 {
		// detached HEAD or empty repo; check HEAD against origin/HEAD
		return checkHEADAgainstOrigin(subDir)
	}
	for _, b := range branches {
		// upstream이 있는가?
		upstream, err := exec.Command("git", "-C", subDir, "rev-parse", "--abbrev-ref", b+"@{upstream}").Output()
		if err != nil || strings.TrimSpace(string(upstream)) == "" {
			return fmt.Errorf("branch %q has no upstream (potentially unpushed commits)", b)
		}
		// upstream과 비교: 로컬에만 있는 커밋이 있는가?
		ahead, err := exec.Command("git", "-C", subDir, "rev-list", "--count", b+"@{upstream}.."+b).Output()
		if err != nil {
			return fmt.Errorf("rev-list failed on %q: %w", b, err)
		}
		if strings.TrimSpace(string(ahead)) != "0" {
			return fmt.Errorf("branch %q has unpushed commits (%s ahead of upstream)", b, strings.TrimSpace(string(ahead)))
		}
	}
	return nil
}

func checkHEADAgainstOrigin(subDir string) error {
	headSHA, err := exec.Command("git", "-C", subDir, "rev-parse", "HEAD").Output()
	if err != nil {
		return nil // empty repo
	}
	originHead, err := exec.Command("git", "-C", subDir, "rev-parse", "origin/HEAD").Output()
	if err != nil {
		return nil // origin/HEAD 미설정 — SHA mismatch 검사로 위임
	}
	if strings.TrimSpace(string(headSHA)) != strings.TrimSpace(string(originHead)) {
		return fmt.Errorf("HEAD is not at origin's default branch tip (detached or diverged)")
	}
	return nil
}
```

`runSubmoduleSafetyChecks`에 추가:

```go
if err := checkAllCommitsPushed(subDir); err != nil {
    return err
}
```

빌드 + 테스트:

```bash
go build ./... && go test ./internal/cli -run TestSafetyCheck_UnpushedCommits -v
```

- [ ] **Step 9-6**: 차단 조건 3 (local-only branches) 테스트 + 구현

```go
func TestSafetyCheck_LocalOnlyBranch(t *testing.T) {
	requireGit(t)
	ws, name, sub := setupSubmoduleFixture(t)
	// 새 로컬 브랜치 (upstream 없음)
	runGit(t, sub, "checkout", "-b", "feature/local")
	if err := os.WriteFile(filepath.Join(sub, "x.txt"), []byte("y"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, sub, "add", ".")
	runGit(t, sub, "-c", "user.email=t@t", "-c", "user.name=t", "commit", "-m", "wip")
	err := runSubmoduleSafetyChecks(ws, name, false)
	if err == nil {
		t.Fatal("expected error for local-only branch")
	}
	// 에러 메시지가 unpushed 또는 local-only 중 하나를 포함해야 함
	if !strings.Contains(err.Error(), "upstream") && !strings.Contains(err.Error(), "local") {
		t.Errorf("unexpected error: %v", err)
	}
}
```

위 `checkAllCommitsPushed`가 이미 upstream 부재를 잡으므로 별도 함수 불필요. 테스트만 추가하여 동작 확인.

```bash
go test ./internal/cli -run TestSafetyCheck_LocalOnlyBranch -v
```

기대: PASS (이미 step 9-5에서 구현됨).

- [ ] **Step 9-7**: 차단 조건 4 (SHA mismatch) 테스트 + 구현

```go
func TestSafetyCheck_SHAMatchesOrigin(t *testing.T) {
	requireGit(t)
	ws, name, sub := setupSubmoduleFixture(t)
	// origin에 새 커밋 후 submodule이 그것을 fetch 안 한 상태로 두면 워크스페이스 핀 SHA == origin/HEAD가 됨 (fetch가 안 됐기 때문)
	// 반대로: submodule이 fetch했지만 슈퍼프로젝트가 핀을 갱신하지 않은 경우를 모사하기 위해
	// origin에 새 커밋 추가 후 submodule에서 fetch만 수행 (체크아웃 안 함)
	originDir := strings.TrimSuffix(getSubmoduleURL(t, sub), "/") // 헬퍼
	if err := os.WriteFile(filepath.Join(originDir, "new.txt"), []byte("z"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, originDir, "add", ".")
	runGit(t, originDir, "-c", "user.email=t@t", "-c", "user.name=t", "commit", "-m", "origin advance")
	runGit(t, sub, "fetch")

	// 슈퍼프로젝트 핀 SHA != origin/HEAD가 되었지만, submodule HEAD는 여전히 옛 SHA
	// 현재 구현(`checkAllCommitsPushed` 안의 detached HEAD 분기 또는 일반 분기)에서
	// 어떤 메시지로 잡힐지에 따라 어서션 조정.
	// (spec 5.1의 SHA mismatch는 핀 SHA와 origin/HEAD 비교 — 별도 함수가 필요)
	err := runSubmoduleSafetyChecks(ws, name, false)
	if err == nil {
		t.Fatal("expected SHA mismatch error")
	}
}

// 헬퍼
func getSubmoduleURL(t *testing.T, subDir string) string {
	t.Helper()
	out, err := exec.Command("git", "-C", subDir, "config", "--get", "remote.origin.url").Output()
	if err != nil {
		t.Fatalf("get url: %v", err)
	}
	url := strings.TrimSpace(string(out))
	return strings.TrimPrefix(url, "file://")
}
```

```go
// migrate_project.go에 추가
func checkSHAMatchesOrigin(workspaceRoot, projectName, subDir string) error {
	pinSHA, err := exec.Command("git", "-C", workspaceRoot, "ls-tree", "HEAD", projectName).Output()
	if err != nil {
		return nil // 슈퍼프로젝트가 핀을 갱신 안 했어도 통과 (조건 외)
	}
	// 출력 형식: "160000 commit <sha>\t<path>"
	parts := strings.Fields(string(pinSHA))
	if len(parts) < 3 {
		return nil
	}
	pinned := parts[2]
	originSHA, err := exec.Command("git", "-C", subDir, "rev-parse", "origin/HEAD").Output()
	if err != nil {
		// origin/HEAD 미설정: main → master 순으로 fallback
		for _, ref := range []string{"origin/main", "origin/master"} {
			if out, err := exec.Command("git", "-C", subDir, "rev-parse", ref).Output(); err == nil {
				originSHA = out
				break
			}
		}
		if len(originSHA) == 0 {
			return nil
		}
	}
	tip := strings.TrimSpace(string(originSHA))
	if pinned != tip {
		return fmt.Errorf("pinned SHA %s differs from origin default branch tip %s (pin will be lost; rerun with --force to proceed)", pinned[:8], tip[:8])
	}
	return nil
}
```

`runSubmoduleSafetyChecks`에 추가:

```go
if err := checkSHAMatchesOrigin(workspaceRoot, projectName, subDir); err != nil {
    return err
}
```

```bash
go build ./... && go test ./internal/cli -run TestSafetyCheck -v
```

기대: 모든 SafetyCheck 테스트 PASS.

- [ ] **Step 9-8**: commit

```bash
git add internal/cli/migrate_project.go internal/cli/migrate_project_test.go
git commit -m "feat(003): migrate-project 안전성 검사 4개 구현

- working tree dirty 검출
- 로컬 브랜치의 unpushed 커밋 검출
- upstream 미설정 브랜치 검출
- 슈퍼프로젝트 핀 SHA와 origin tip 불일치 검출

각 조건은 별도 함수로 분리되며 --force로 우회 가능하다.
spec 003 §5.1."
```

---

### Task 10: 마이그레이션 절차 (`performMigration`) 및 `--dry-run` 검증

**Files**:
- Modify: `internal/cli/migrate_project.go` (`performMigration` 본 구현, `teardownSubmodule` 본 구현)
- Modify: `internal/cli/migrate_project_test.go` (통합 테스트)

spec §5.2의 9단계 순서를 그대로 구현한다.

- [ ] **Step 10-1**: 통합 테스트 작성

```go
func TestPerformMigration_HappyPath(t *testing.T) {
	requireGit(t)
	ws, name, sub := setupSubmoduleFixture(t)
	// .pylon/ 더미 데이터
	if err := os.MkdirAll(filepath.Join(sub, ".pylon", "agents"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, ".pylon", "context.md"), []byte("ctx\n"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := performMigration(ws, name); err != nil {
		t.Fatalf("performMigration failed: %v", err)
	}

	// 1) .gitmodules에서 항목 제거됨
	data, _ := os.ReadFile(filepath.Join(ws, ".gitmodules"))
	if strings.Contains(string(data), "submodule \""+name+"\"") {
		t.Errorf(".gitmodules still contains [submodule %q] entry", name)
	}
	// 2) detectProjectCoupling이 CouplingClone 반환
	if got := detectProjectCoupling(ws, name); got != CouplingClone {
		t.Errorf("after migration coupling = %v, want CouplingClone", got)
	}
	// 3) 하위 .git이 디렉토리 (clone 결과)
	info, err := os.Stat(filepath.Join(sub, ".git"))
	if err != nil {
		t.Fatalf(".git missing: %v", err)
	}
	if !info.IsDir() {
		t.Errorf(".git should be directory after re-clone, got gitlink file")
	}
	// 4) .pylon/이 복원됨
	if _, err := os.Stat(filepath.Join(sub, ".pylon", "context.md")); err != nil {
		t.Errorf(".pylon/context.md missing after migration: %v", err)
	}
	// 5) .git/info/exclude에 .pylon/ 등록
	excl, err := os.ReadFile(filepath.Join(sub, ".git", "info", "exclude"))
	if err != nil || !strings.Contains(string(excl), ".pylon/") {
		t.Errorf(".pylon/ not in .git/info/exclude (err=%v)", err)
	}
}
```

- [ ] **Step 10-2**: 실패 확인 (`performMigration`이 stub이라 에러)

```bash
go test ./internal/cli -run TestPerformMigration_HappyPath -v
```

기대: FAIL.

- [ ] **Step 10-3**: `performMigration` 본 구현

```go
// migrate_project.go
import "io"  // 필요한 추가 import

func performMigration(workspaceRoot, projectName string) error {
	subDir := filepath.Join(workspaceRoot, projectName)

	// §5.2-2: metadata 수집
	originURL, err := exec.Command("git", "-C", subDir, "config", "--get", "remote.origin.url").Output()
	if err != nil {
		return fmt.Errorf("cannot read origin URL: %w", err)
	}
	url := strings.TrimSpace(string(originURL))

	// 현재 체크아웃 상태
	branch, _ := exec.Command("git", "-C", subDir, "symbolic-ref", "--short", "HEAD").Output()
	checkoutTarget := strings.TrimSpace(string(branch))
	if checkoutTarget == "" {
		// detached HEAD: origin/HEAD → main → master fallback
		for _, ref := range []string{"refs/remotes/origin/HEAD", "refs/remotes/origin/main", "refs/remotes/origin/master"} {
			if _, err := exec.Command("git", "-C", subDir, "rev-parse", ref).Output(); err == nil {
				switch ref {
				case "refs/remotes/origin/HEAD":
					if out, err := exec.Command("git", "-C", subDir, "symbolic-ref", "refs/remotes/origin/HEAD").Output(); err == nil {
						s := strings.TrimSpace(string(out))
						checkoutTarget = strings.TrimPrefix(s, "refs/remotes/origin/")
					}
				case "refs/remotes/origin/main":
					checkoutTarget = "main"
				case "refs/remotes/origin/master":
					checkoutTarget = "master"
				}
				break
			}
		}
	}

	// §5.2-3: .pylon/ 임시 보관
	tmpHome := filepath.Join(workspaceRoot, ".pylon", "migrate-tmp", projectName)
	if err := os.MkdirAll(filepath.Dir(tmpHome), 0755); err != nil {
		return fmt.Errorf("create tmp parent: %w", err)
	}
	pylonSrc := filepath.Join(subDir, ".pylon")
	if _, err := os.Stat(pylonSrc); err == nil {
		_ = os.RemoveAll(tmpHome)
		if err := os.Rename(pylonSrc, tmpHome); err != nil {
			// rename이 cross-device로 실패하면 copy로 fallback
			if err := copyDir(pylonSrc, tmpHome); err != nil {
				return fmt.Errorf("preserve .pylon/: %w", err)
			}
			_ = os.RemoveAll(pylonSrc)
		}
	}

	// §5.2-4: submodule 해제
	if err := teardownSubmodule(workspaceRoot, projectName); err != nil {
		return fmt.Errorf("submodule teardown: %w (manual cleanup may be required)", err)
	}

	// §5.2-5: commit 안내 (자동 commit 안 함)
	fmt.Println("✓ Submodule deregistered.")
	fmt.Println("  Note: workspace .gitmodules was modified. Run 'git -C", workspaceRoot, "add .gitmodules && git commit' to record this change if you track the workspace in git.")

	// §5.2-6: 재clone
	cloneCmd := exec.Command("git", "clone", url, projectName)
	cloneCmd.Dir = workspaceRoot
	if out, err := cloneCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("re-clone failed: %w\n%s\n.pylon/ preserved at %s — rerun 'git clone %s %s' and move the directory back", err, out, tmpHome, url, projectName)
	}
	if checkoutTarget != "" {
		coCmd := exec.Command("git", "-C", subDir, "checkout", checkoutTarget)
		if out, err := coCmd.CombinedOutput(); err != nil {
			fmt.Printf("⚠ checkout %s failed; staying on default branch: %s\n", checkoutTarget, out)
		}
	}

	// §5.2-7: .pylon/ 복원
	if _, err := os.Stat(tmpHome); err == nil {
		if err := os.Rename(tmpHome, pylonSrc); err != nil {
			if err := copyDir(tmpHome, pylonSrc); err != nil {
				return fmt.Errorf("restore .pylon/: %w (preserved at %s)", err, tmpHome)
			}
		}
		_ = os.RemoveAll(filepath.Dir(tmpHome)) // .pylon/migrate-tmp/ 정리 (비어있을 때만)
	}

	// §5.2-8: exclude 재설정
	if err := excludePylonFromRepo(subDir); err != nil {
		fmt.Printf("⚠ Could not exclude .pylon/ from new repo: %v\n", err)
	}

	fmt.Printf("✓ Migrated '%s' to standalone clone.\n", projectName)
	return nil
}

func teardownSubmodule(workspaceRoot, projectName string) error {
	if out, err := exec.Command("git", "-C", workspaceRoot, "submodule", "deinit", "-f", "--", projectName).CombinedOutput(); err != nil {
		return fmt.Errorf("submodule deinit: %w\n%s", err, out)
	}
	if out, err := exec.Command("git", "-C", workspaceRoot, "rm", "-f", "--", projectName).CombinedOutput(); err != nil {
		return fmt.Errorf("git rm: %w\n%s", err, out)
	}
	if err := os.RemoveAll(filepath.Join(workspaceRoot, ".git", "modules", projectName)); err != nil {
		return fmt.Errorf("remove cached modules: %w", err)
	}
	return nil
}

func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0755)
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.Create(target)
		if err != nil {
			return err
		}
		defer out.Close()
		_, err = io.Copy(out, in)
		return err
	})
}
```

- [ ] **Step 10-4**: 통합 테스트 통과 확인

```bash
go build ./... && go test ./internal/cli -run TestPerformMigration_HappyPath -v
```

기대: PASS.

- [ ] **Step 10-5**: `--dry-run` 테스트 추가

```go
func TestRunMigrateProject_DryRunNoChanges(t *testing.T) {
	requireGit(t)
	ws, name, sub := setupSubmoduleFixture(t)
	_ = sub

	cmd := newMigrateProjectCmd()
	cmd.SetArgs([]string{name, "--dry-run"})

	oldWorkspace := flagWorkspace
	flagWorkspace = ws
	defer func() { flagWorkspace = oldWorkspace }()

	if err := cmd.Execute(); err != nil {
		t.Fatalf("dry-run failed: %v", err)
	}
	// .gitmodules 그대로
	data, _ := os.ReadFile(filepath.Join(ws, ".gitmodules"))
	if !strings.Contains(string(data), "submodule \""+name+"\"") {
		t.Errorf("dry-run should not modify .gitmodules")
	}
	// coupling 그대로
	if got := detectProjectCoupling(ws, name); got != CouplingSubmodule {
		t.Errorf("after dry-run coupling = %v, want CouplingSubmodule", got)
	}
}
```

```bash
go test ./internal/cli -run TestRunMigrateProject_DryRunNoChanges -v
```

기대: PASS.

- [ ] **Step 10-6**: 전체 회귀

```bash
go test ./internal/cli/... -v
```

기대: 모든 테스트 PASS.

- [ ] **Step 10-7**: commit

```bash
git add internal/cli/migrate_project.go internal/cli/migrate_project_test.go
git commit -m "feat(003): pylon migrate-project 마이그레이션 절차 구현

spec 003 §5.2의 9단계를 모두 구현한다:
- 메타데이터 수집 (origin URL, 체크아웃 브랜치/HEAD 상태)
- .pylon/ 임시 보관
- submodule 해제 (deinit + git rm + .git/modules 정리)
- 사용자에게 .gitmodules commit 안내 출력
- 동일 origin에서 재clone 후 원래 브랜치 체크아웃
- .pylon/ 복원 및 .git/info/exclude 재설정

detached HEAD인 경우 origin/HEAD → main → master 순으로 fallback.
--dry-run은 어떤 상태도 변경하지 않는다.
spec 003 §5.1, §5.2, §5.4."
```

---

## Phase 6: User Story 2 — `uninstall` / `doctor` 분기

**Goal**: 두 결합 방식이 혼재된 워크스페이스에서 두 명령이 동일한 기준으로 분기하도록 한다 (US-2, FR-5, FR-6).

### Task 11: `uninstall --remove-projects`의 결합 방식 분기

**Files**:
- Modify: `internal/cli/uninstall.go:43-50` (`uninstallPlan` 구조체 — 필드 추가)
- Modify: `internal/cli/uninstall.go:116-124` (`buildUninstallPlan`)
- Modify: `internal/cli/uninstall.go:167-172, 211-221` (`printUninstallPlan`, `executeUninstall`)
- Modify: `internal/cli/uninstall.go:29` (Long 설명에서 "submodules" 언급 일반화)
- Test: `internal/cli/uninstall_test.go`

- [ ] **Step 11-1**: 신규 테스트 — clone 프로젝트의 `--remove-projects`는 디렉토리 삭제로 처리

```go
// internal/cli/uninstall_test.go 에 추가
func TestUninstall_RemoveProjects_ClonePath(t *testing.T) {
	requireGit(t)
	ws := t.TempDir()
	if err := os.MkdirAll(filepath.Join(ws, ".pylon"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, ".pylon", "config.yml"), []byte("version: \"0.1\"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	// 워크스페이스는 git이 아님; 하위에 clone (git init) 흉내
	projDir := filepath.Join(ws, "myproj")
	if out, err := exec.Command("git", "init", projDir).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	// DiscoverProjects가 인식하도록 프로젝트에 .pylon/ 생성
	// (config/workspace.go:60 DiscoverProjects는 하위 디렉토리의 .pylon/ 존재만 확인)
	if err := os.MkdirAll(filepath.Join(projDir, ".pylon"), 0755); err != nil {
		t.Fatal(err)
	}

	plan, err := buildUninstallPlan(ws, true, false)
	if err != nil {
		t.Fatalf("buildUninstallPlan: %v", err)
	}
	// submodules 리스트는 비어야 함, cloneProjects 리스트에 들어가야 함
	if len(plan.submodules) > 0 {
		t.Errorf("clone project should not be in submodules list, got %v", plan.submodules)
	}
	if len(plan.cloneProjects) != 1 || plan.cloneProjects[0] != "myproj" {
		t.Errorf("expected cloneProjects=[myproj], got %v", plan.cloneProjects)
	}
}
```

`registerForTest` 헬퍼는 기존 uninstall_test에서 사용하는 setup을 따른다. 없다면 `config.DiscoverProjects`가 디렉토리 스캔만으로 찾아내는지 확인 후 (`.pylon/` 존재로 식별), 디렉토리만 만들어주면 됨.

- [ ] **Step 11-2**: 실패 확인

```bash
go test ./internal/cli -run TestUninstall_RemoveProjects_ClonePath -v
```

기대: FAIL — 현재는 `plan.submodules`로 모두 들어감.

- [ ] **Step 11-3**: `uninstallPlan`에 필드 추가 + `buildUninstallPlan` 분기

```go
// uninstall.go:43
type uninstallPlan struct {
	runtimeFiles   []string
	projectPylons  []string
	submodules     []string // legacy submodule projects (only if --remove-projects)
	cloneProjects  []string // standalone clone projects (only if --remove-projects)
	workspacePylon string
	gitignorePath  string
	binaryPath     string
}
```

```go
// uninstall.go:116-124의 for loop 안에서:
if removeProjects {
    switch detectProjectCoupling(root, p.Name) {
    case CouplingSubmodule:
        plan.submodules = append(plan.submodules, p.Name)
    case CouplingClone:
        plan.cloneProjects = append(plan.cloneProjects, p.Name)
    }
}
```

- [ ] **Step 11-4**: `printUninstallPlan`에 cloneProjects 출력 추가

```go
// uninstall.go:167-172 직전에 추가
if len(plan.cloneProjects) > 0 {
    fmt.Println("  [Clone project directories]")
    for _, c := range plan.cloneProjects {
        fmt.Printf("    - %s/\n", c)
    }
}
```

- [ ] **Step 11-5**: `executeUninstall`에서 clone 디렉토리 삭제 처리

```go
// uninstall.go:211 (Step 3 직전 또는 직후)에 추가
if len(plan.cloneProjects) > 0 {
    for _, name := range plan.cloneProjects {
        projDir := filepath.Join(root, name)
        if err := os.RemoveAll(projDir); err != nil {
            errors = append(errors, fmt.Sprintf("failed to remove clone project (%s): %v", name, err))
        } else {
            fmt.Printf("✓ Removed clone project: %s\n", name)
        }
    }
}
```

- [ ] **Step 11-6**: `Long` 설명 일반화

```go
// uninstall.go:21-30
Long: `Completely remove pylon from the current workspace.

This removes:
  1. Runtime artifacts (.claude/, CLAUDE.md)
  2. Project-level .pylon/ directories
  3. Workspace .pylon/ directory (config, domain, agents, database)
  4. Pylon entries from .gitignore

Project source code is preserved by default.

Use --remove-projects to also remove project directories (clones) and
deregister legacy git submodules. Use --remove-binary to also delete
the pylon binary from $GOPATH/bin.`,
```

`--remove-projects` 플래그 Help 갱신:

```go
cmd.Flags().Bool("remove-projects", false, "also remove project directories (clones) and submodule registrations")
```

- [ ] **Step 11-7**: 새 테스트 + 기존 회귀

```bash
go test ./internal/cli -run TestUninstall -v
```

기대: 모든 PASS.

- [ ] **Step 11-8**: commit

```bash
git add internal/cli/uninstall.go internal/cli/uninstall_test.go
git commit -m "feat(003): uninstall --remove-projects가 결합 방식별로 분기

clone 프로젝트는 디렉토리 삭제, submodule 프로젝트는 기존 deinit 경로.
detectProjectCoupling 헬퍼를 공유하여 일관된 감지 기준 사용.
spec 003 §FR-5, US-2."
```

---

### Task 12: `doctor`의 `checkSubmoduleExcludes` 일반화

**Files**:
- Modify: `internal/cli/doctor.go:45-58` (명령 Long/Flag Help)
- Modify: `internal/cli/doctor.go:94-110` (호출부 메시지)
- Modify: `internal/cli/doctor.go:125-192` (함수 본체 + 메시지)
- Test: `internal/cli/doctor_test.go`

- [ ] **Step 12-1**: 함수 재명명 및 메시지 일반화

```go
// doctor.go:125
// checkSubmoduleExcludes → checkRepoExcludes
func checkRepoExcludes(fix bool) bool {
    // 본문은 그대로 유지하되, 메시지에서 "서브모듈" → "프로젝트" 일반화
    // 예: "✓ 모든 서브모듈에 .pylon/ exclude 설정됨" → "✓ 모든 프로젝트에 .pylon/ exclude 설정됨"
    // "⚠ .pylon/ exclude 미설정 서브모듈 %d개:" → "⚠ .pylon/ exclude 미설정 프로젝트 %d개:"
}
```

호출부도 갱신:

```go
// doctor.go:104
if !checkRepoExcludes(fixExcludes) {
    allPassed = false
}
```

```go
// doctor.go:109
fmt.Println("⚠ git 미설치로 프로젝트 exclude 검사 건너뜀")
```

```go
// doctor.go:51-52 (Long)
Use --fix-excludes to automatically add .pylon/ to project .git/info/exclude
for any projects that are missing the local-scope ignore entry.
```

```go
// doctor.go:56 (flag help)
cmd.Flags().Bool("fix-excludes", false, "auto-fix missing .pylon/ exclude entries in project repos")
```

- [ ] **Step 12-2**: 테스트 갱신 (`doctor_test.go`에서 `checkSubmoduleExcludes` 호출이 있다면 `checkRepoExcludes`로)

```bash
grep -n "checkSubmoduleExcludes" internal/cli/doctor_test.go
```

발견되는 모든 사용을 일괄 갱신. 테스트 함수명도 `TestCheckSubmoduleExcludes*` → `TestCheckRepoExcludes*`.

- [ ] **Step 12-3**: 빌드 + 테스트

```bash
go build ./... && go test ./internal/cli -run TestCheckRepoExcludes -v
```

기대: 모든 PASS.

- [ ] **Step 12-4**: commit

```bash
git add internal/cli/doctor.go internal/cli/doctor_test.go
git commit -m "refactor(003): doctor의 submodule exclude 검사를 일반화

checkSubmoduleExcludes를 checkRepoExcludes로 재명명.
메시지와 Help 문구에서 'submodule' 가정을 제거하여
clone과 submodule 양쪽 프로젝트를 동일한 기준으로 검사한다.
spec 003 §FR-6."
```

---

### Task 13: `destroy.go` 메시지 일반화

**Files**: Modify: `internal/cli/destroy.go:19`

- [ ] **Step 13-1**: 현재 메시지 확인

```bash
sed -n '15,25p' internal/cli/destroy.go
```

- [ ] **Step 13-2**: "Git submodules are preserved" → "Project repositories are preserved"

```go
// destroy.go:19
Long: `... Project repositories are preserved.`,
```

- [ ] **Step 13-3**: 빌드 + commit

```bash
go build ./...
git add internal/cli/destroy.go
git commit -m "docs(003): destroy 명령의 submodule 메시지 일반화"
```

---

## Phase 7: 문서 갱신

각 문서 변경은 독립적으로 commit 가능. `[P]` 표시.

### Task 14 [P]: `README.md` 갱신

**Files**: Modify: `README.md:88, 272`

- [ ] **Step 14-1**: 변경 위치 확인

```bash
grep -n "add-project\|서브모듈\|submodule" README.md
```

- [ ] **Step 14-2**: 변경
  - 라인 88 근처: `pylon add-project https://github.com/user/my-app.git`의 결과 설명에서 "submodule" → "standalone clone"
  - 라인 272 표: `pylon add-project <url>`의 설명을 "프로젝트 서브모듈 추가" → "프로젝트를 워크스페이스에 clone하여 추가"
  - `pylon migrate-project <name>` 한 줄을 표에 추가: "기존 submodule 프로젝트를 clone으로 전환"

- [ ] **Step 14-3**: commit

```bash
git add README.md
git commit -m "docs(003): README의 add-project 동작 설명 갱신 및 migrate-project 문서화"
```

---

### Task 15 [P]: `pylon-spec.md` Section 7 갱신

**Files**: Modify: `pylon-spec.md` Section 7 ("pylon add-project"), Section 12

- [ ] **Step 15-1**: 변경 위치 확인

```bash
grep -n "add-project\|submodule\|서브모듈" pylon-spec.md | head -30
```

- [ ] **Step 15-2**: Section 7에서 `add-project` 동작 기술을 "git clone" 기반으로 재작성. submodule 관련 옵션 설명은 제거하거나 `migrate-project` 섹션으로 이동.

- [ ] **Step 15-3**: commit

```bash
git add pylon-spec.md
git commit -m "docs(003): pylon-spec.md Section 7을 clone 기반으로 갱신"
```

---

### Task 16 [P]: 마이그레이션 문서 갱신

**Files**: Modify: `docs/MIGRATION-V2.md`, `docs/v2-rewrite/MIGRATION.md`, `docs/v2-rewrite/CAPABILITY-INVENTORY.md`

- [ ] **Step 16-1**: 변경 위치 확인

```bash
grep -n "add-project\|submodule\|서브모듈" docs/MIGRATION-V2.md docs/v2-rewrite/MIGRATION.md docs/v2-rewrite/CAPABILITY-INVENTORY.md
```

- [ ] **Step 16-2**: 각 파일에서 add-project가 submodule을 사용한다는 가정을 갱신, `migrate-project` 명령을 신규 항목으로 추가.

- [ ] **Step 16-3**: commit

```bash
git add docs/MIGRATION-V2.md docs/v2-rewrite/MIGRATION.md docs/v2-rewrite/CAPABILITY-INVENTORY.md
git commit -m "docs(003): v2 마이그레이션 문서에 clone 전환 반영"
```

---

### Task 17 [P]: `IMPLEMENTATION_PLAN.md` 갱신

**Files**: Modify: `IMPLEMENTATION_PLAN.md`

- [ ] **Step 17-1**: `add_project.go` 항목 (라인 39 부근) 설명을 "프로젝트 git clone"으로 갱신, `migrate_project.go` 신규 항목 추가.

- [ ] **Step 17-2**: commit

```bash
git add IMPLEMENTATION_PLAN.md
git commit -m "docs(003): IMPLEMENTATION_PLAN.md에 clone 전환 반영"
```

---

## Phase 8: 회귀 검증 + Acceptance

### Task 18: 전체 회귀

- [ ] **Step 18-1**: 전체 테스트

```bash
go test ./... -race -timeout 5m
```

기대: 모든 PASS.

- [ ] **Step 18-2**: 빌드 검증

```bash
go build ./...
```

- [ ] **Step 18-3**: 린트 (해당 환경에서 사용 가능한 경우)

```bash
golangci-lint run ./... 2>/dev/null || echo "lint skipped (not installed)"
```

---

### Task 19: Acceptance 시나리오 수동 검증

spec §8의 7가지 Acceptance Criteria를 임시 디렉토리에서 수동 검증한다.

- [ ] **Step 19-1**: A1 — 신규 `add-project`가 `.gitmodules` 미생성 + 워크스페이스 `git init` 불필요

```bash
mkdir -p /tmp/pylon-test-ws && cd /tmp/pylon-test-ws
~/go/bin/pylon init .
~/go/bin/pylon add-project https://github.com/octocat/Hello-World.git --name hello
ls -la .gitmodules 2>&1 | grep -q "No such" && echo "✓ A1 part 1"
ls -la .git 2>&1 | grep -q "No such" && echo "✓ A1 part 2"
ls hello/.git | head -2  # directory, not gitlink
cd - && rm -rf /tmp/pylon-test-ws
```

- [ ] **Step 19-2**: A2 — 기존 submodule 워크스페이스에서 spec 002 파이프라인 회귀 (수동)

(spec 002 파이프라인은 외부 LLM이 필요하므로 자동화 어려움. 결합 감지가 올바른지만 검증):

```bash
# 기존 submodule 워크스페이스 (실 보유분 또는 fixture)에서
~/go/bin/pylon doctor
# 출력에 "✓ 모든 프로젝트에 .pylon/ exclude 설정됨" 또는 동등한 메시지가 나오는지 확인
```

- [ ] **Step 19-3**: A3 — `migrate-project`의 차단 동작 (dirty tree)

```bash
# 임시 submodule 워크스페이스 만들고
~/go/bin/pylon migrate-project <name>  # 정상 시 통과
# submodule 안에 dirty 파일 만들고 다시
~/go/bin/pylon migrate-project <name>  # "working tree is dirty"로 차단되는지
```

- [ ] **Step 19-4**: A4 — `--dry-run`

```bash
~/go/bin/pylon migrate-project <name> --dry-run
# 출력에 "✓ All safety checks passed (dry-run; no changes were made)"
# .gitmodules 변경 없음 확인
```

- [ ] **Step 19-5**: A5 — `doctor`가 양쪽 모두 동일 기준 보고
- [ ] **Step 19-6**: A6 — `uninstall --remove-projects`가 두 결합 방식 모두 정리
- [ ] **Step 19-7**: A7 — `pylon init` 후 워크스페이스에 `.git/` 미생성

```bash
mkdir -p /tmp/pylon-test-init && cd /tmp/pylon-test-init
~/go/bin/pylon init .
ls -la .git 2>&1 | grep -q "No such" && echo "✓ A7"
cd - && rm -rf /tmp/pylon-test-init
```

- [ ] **Step 19-8**: 결과 요약 출력 (체크리스트 형태)

```text
A1 ✓  A2 ✓  A3 ✓  A4 ✓  A5 ✓  A6 ✓  A7 ✓
```

---

### Task 20: PR 생성

- [ ] **Step 20-1**: 변경 사항 push

```bash
git push origin feat/003-add-project-clone
```

- [ ] **Step 20-2**: PR 생성 (한국어, CLAUDE.md 준수)

```bash
gh pr create --title "feat(003): add-project를 독립 clone 방식으로 전환" \
  --body "$(cat <<'EOF'
## 개요

`pylon add-project`를 git submodule 기반에서 독립 `git clone` 기반으로 전환한다.
spec 002 다중 repo 파이프라인은 결합 방식에 중립이므로 변경 없이 동작한다.

## 주요 변경

- `pylon add-project`가 `git clone`을 사용. `.gitmodules`를 생성하지 않으며 워크스페이스 `git init`도 요구하지 않음
- `pylon init`이 워크스페이스를 `git init`하지 않음
- `pylon migrate-project <name>` 신규 명령으로 기존 submodule을 안전하게 clone으로 전환
- `detectProjectCoupling` 헬퍼로 `add-project --force`, `uninstall`, `doctor`, `migrate-project`가 동일한 감지 기준 공유
- 기존 submodule 프로젝트는 동작 그대로 유지 (회귀 없음)

## 안전성

`migrate-project`는 spec 003 §5.1의 차단 조건을 모두 점검한다:
- working tree dirty
- unpushed local commits
- local-only branches
- 핀 SHA와 origin tip 불일치

`--dry-run`은 어떤 상태도 변경하지 않는다.

## 테스트

- 단위/통합 테스트 추가: coupling, add-project clone 경로, migrate-project 차단 4종 + happy path + dry-run, uninstall 분기, doctor 일반화
- 전체 회귀 `go test ./... -race` 통과

## 호환성

- 기존 submodule 프로젝트가 등록된 워크스페이스는 그대로 동작
- `pylon add-project --force`가 submodule 디렉토리에 대해 차단되어 실수 방지
- `--force --migrate` 조합으로 명시적 변환 + 재clone 가능

Spec: `specs/003-add-project-clone/spec.md`
Plan: `specs/003-add-project-clone/plan.md`
EOF
)"
```

- [ ] **Step 20-3**: PR 생성 확인 — 머지는 `--merge` 사용 (사용자 메모 [[no-squash-merge]]), 사용자의 명시적 승인 후에만.

---

## 부록: Task ↔ Spec FR/Story 매핑

| Task | Spec 참조 |
|---|---|
| T2 (`detectProjectCoupling`) | §FR-5의 우선순위 규칙 |
| T3 (재명명) | §FR-6 (doctor 일반화의 전제) |
| T4 (`init` git init 제거) | §FR-7 |
| T5 (주석) | (clean-up) |
| T6 (clone) | §FR-1, §FR-3, US-1 |
| T7 (force 분기) | §FR-2, US-4 |
| T8 (migrate 명령 골격) | §FR-4의 일부 |
| T9 (안전 검사) | §5.1, §FR-4, US-3 |
| T10 (절차 + dry-run) | §5.2, §5.4, §FR-4, US-3 |
| T11 (uninstall) | §FR-5, US-2 |
| T12 (doctor) | §FR-6, US-2 |
| T13 (destroy 문구) | (cleanup) |
| T14–T17 (문서) | — |
| T18 (회귀) | §8 자동 검증 |
| T19 (수동 acceptance) | §8 acceptance criteria 1–7 |
| T20 (PR) | — |

§FR-8 (파이프라인 로직 미변경)은 별도 task 없음 — 본 변경은 spec 002 코드를 건드리지 않으며, T19에서 회귀가 없음을 수동 확인한다.
