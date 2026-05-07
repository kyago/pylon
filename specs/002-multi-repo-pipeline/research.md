# Research: Multi-Repo Pipeline Harness

**Branch**: `002-multi-repo-pipeline` | **Date**: 2026-04-25

## 결정 1: `--git-root` 인자 파싱 패턴

**Decision**: `source common.sh` 이전에 각 스크립트에서 `--git-root`를 사전 파싱하고, source 후 GIT_ROOT를 override

**Rationale**: `common.sh`가 source될 때 `GIT_ROOT`가 이미 결정된다. 사전 파싱 없이는 override가 불가능하다.

**Pattern**:
```bash
# 1. --git-root 및 --pipeline-dir 사전 파싱
GIT_ROOT_ARG=""
PIPELINE_DIR_ARG=""
_args=()
_next=""
for arg in "$@"; do
  if [[ "$_next" == "git-root" ]]; then
    GIT_ROOT_ARG="$arg"
    _next=""
  elif [[ "$_next" == "pipeline-dir" ]]; then
    PIPELINE_DIR_ARG="$arg"
    _next=""
  elif [[ "$arg" == "--git-root" ]]; then
    _next="git-root"
  elif [[ "$arg" == "--pipeline-dir" ]]; then
    _next="pipeline-dir"
  else
    _args+=("$arg")
  fi
done
set -- "${_args[@]+"${_args[@]}"}"

# 2. common.sh source
source "$(dirname "$0")/common.sh"

# 3. override (우선순위 1)
if [[ -n "$GIT_ROOT_ARG" ]]; then
  GIT_ROOT="$(realpath "$REPO_ROOT/$GIT_ROOT_ARG")"
fi
```

**Alternatives considered**: common.sh 내부에서 `$@`를 파싱하는 방식 — 스크립트별 positional 인자 순서가 다르므로 부적합.

---

## 결정 2: `find_git_root()` 대체

**Decision**: `git rev-parse --show-toplevel 2>/dev/null || echo "$REPO_ROOT"` 로 대체

**Rationale**: 기존 `find_git_root()`는 `.git`이 디렉토리인지 확인하지만, git worktree에서 `.git`은 파일(포인터)이다. `git rev-parse --show-toplevel`은 양쪽 모두 올바르게 처리한다.

**Alternatives considered**: worktree 감지 로직 추가 (`-f "$dir/.git"` 조건 병합) — 불필요한 복잡성.

---

## 결정 3: sub-pipeline 모드 인식 방법

**Decision**: `--git-root` + `--pipeline-dir` 동시 제공 시 서브파이프라인 모드

- `--git-root` 없음 → 루트 파이프라인 모드 (Step 1, 브랜치 생성 없음)
- `--git-root` + `--pipeline-dir` → 서브파이프라인 모드 (repo-Agent, 브랜치 생성)

**루트 모드 변경사항**: 기존 `init-pipeline.sh`는 브랜치도 생성했으나, Step 1에서는 어느 repo에 브랜치를 만들지 아직 알 수 없다. 루트 모드는 PIPELINE_DIR + status.json 초기화만 수행한다.

**서브파이프라인 dir**: `{pipeline-dir}/{repo-basename}/`
- 예: `--pipeline-dir .pylon/runtime/20260425-feat --git-root services/service-a`
  → sub dir: `.pylon/runtime/20260425-feat/service-a/`

---

## 결정 4: `run-verification.sh`에서 비-Go repo 처리

**Decision**: `--git-root`로 지정된 경로에 `go.mod`가 없으면 검증 스킵 후 성공 반환

```bash
if [[ -n "$GIT_ROOT_ARG" ]]; then
  cd "$GIT_ROOT" || die "..."
  if [[ ! -f "go.mod" ]]; then
    # go.mod 없는 repo: 언어별 검증은 향후 확장
    jq -cn '{ok: true, checks: [], skipped: true, timestamp: now | todate}' > ...
    exit 0
  fi
fi
```

**Rationale**: 다중 repo 워크스페이스에서 모든 repo가 Go일 수 없다. 현 단계에서 Go만 지원하되 스킵 처리로 파이프라인이 중단되지 않게 한다.

---

## 결정 5: 단일 repo 하위 호환성 보장 전략

**Decision**: `--git-root` 인자가 없으면 모든 스크립트는 기존 로직을 그대로 실행한다.

- `common.sh`의 GIT_ROOT 결정 로직 (config.yml → git rev-parse)은 유지
- `init-pipeline.sh` 루트 모드에서 브랜치 생성 로직은 제거 (아래 참고)
- `pl-pipeline.md`에서 단일 repo = `affected_repos: ["."]`로 처리

**루트 모드 브랜치 생성 제거 영향**: 기존 `pl-pipeline.md`의 Step 1에서 브랜치를 생성했으나, 새 설계에서는 Step 5의 repo-Agent가 담당한다. 이는 `pl-pipeline.md` 변경으로 흡수되며 스크립트 인터페이스 호환성에 영향 없다.

---

## 결정 6: architect.md 에이전트 출력 요구사항

**Decision**: architect.md에 `affected_repos` 섹션을 명시적으로 요구하는 지시 추가

**형식**:
```markdown
## Affected Repositories
- `<REPO_ROOT 기준 상대경로>`: <변경 이유>
```

**단일 repo**: `- ".": <변경 이유>`

**PM 사용 방법**: Step 5에서 PM이 architecture.md의 `## Affected Repositories` 섹션을 파싱하여 repo 목록 추출 → 각 repo별 Agent 스폰
