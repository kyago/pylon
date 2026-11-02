#!/bin/bash
set -euo pipefail

source "$(dirname "$0")/common.sh"

GIT_ROOT_ARG=$(extract_arg "git-root" "$@")
PIPELINE_DIR_ARG=$(extract_arg "pipeline-dir" "$@")
BRANCH_ARG=$(extract_arg "branch" "$@")
resolve_git_root "$GIT_ROOT_ARG"
cd "$GIT_ROOT" || die "GIT_ROOT로 이동 실패: $GIT_ROOT"

require_cmd git jq

REQUIREMENT="${1:?Usage: init-pipeline.sh <requirement> [--git-root <repo-rel-path>] [--pipeline-dir <root-pipeline-dir>] [--branch <name>]}"

# Generate a locale-independent slug from requirement.
SLUG=$(printf '%s\n' "$REQUIREMENT" | LC_ALL=C tr '[:upper:]' '[:lower:]' | LC_ALL=C sed 's/[^a-z0-9]/-/g' | sed 's/--*/-/g' | cut -c1-30 | sed 's/-$//')
[[ -n "$SLUG" ]] || SLUG="run"
PIPELINE_ID="$(date -u +%Y%m%dT%H%M%SZ)-${SLUG}-$$"

# --- Sub-pipeline mode: --git-root + --pipeline-dir both provided ---
if [[ -n "$GIT_ROOT_ARG" && -n "$PIPELINE_DIR_ARG" ]]; then
  ROOT_STATUS="$PIPELINE_DIR_ARG/status.json"
  [[ -f "$ROOT_STATUS" ]] || die "root pipeline status not found: $ROOT_STATUS"
  ROOT_SCOPE=$(jq -r '.scope // "root"' "$ROOT_STATUS")
  [[ "$ROOT_SCOPE" == "root" ]] || die "--pipeline-dir must point to a root pipeline: $PIPELINE_DIR_ARG"

  # Validate: GIT_ROOT must be a valid git repo
  git -C "$GIT_ROOT" rev-parse --git-dir &>/dev/null \
    || die "--git-root '$GIT_ROOT_ARG' is not a valid git repository"

  REPO_ROOT_REAL=$(realpath "$REPO_ROOT")
  REPO_RELATIVE="${GIT_ROOT#"$REPO_ROOT_REAL"/}"
  [[ "$GIT_ROOT" != "$REPO_ROOT_REAL" ]] || REPO_RELATIVE="."
  REPO_ID=$(echo "$REPO_RELATIVE" | sed 's|^\./||; s|/|--|g; s|[^A-Za-z0-9._-]|-|g; s|--*|-|g; s|^-||; s|-$||')
  [[ -n "$REPO_ID" && "$REPO_ID" != "." ]] || REPO_ID="root-repo"
  SUB_PIPELINE_DIR="${PIPELINE_DIR_ARG}/repos/${REPO_ID}"
  mkdir -p "$SUB_PIPELINE_DIR"
  ROOT_PIPELINE_ID=$(jq -r --arg fallback "$(basename "$PIPELINE_DIR_ARG")" '.pipeline_id // $fallback' "$ROOT_STATUS")

  # Create or checkout branch in target repo
  # 검증은 깨끗한 작업 트리의 HEAD만 기록하므로 run도 깨끗하게 시작한다 — 이후 남는 파일은 모두 이 run의 것이다.
  UNCOMMITTED=$(git -C "$GIT_ROOT" status --porcelain --untracked-files=normal --ignore-submodules=untracked \
    -- ':(top)' ':(top,glob,exclude)**/.pylon/**' ':(top,exclude).claude/worktrees')
  if [[ -n "$UNCOMMITTED" ]]; then
    die "$GIT_ROOT_ARG 에 커밋되지 않은 변경·추적 안 된 파일이 있습니다. commit, 'git stash -u', 또는 .git/info/exclude 추가 후 다시 실행하세요 (pylon이 실행 시 만든 .gitignore라면 커밋하세요):
$UNCOMMITTED"
  fi
  if [[ -f "$SUB_PIPELINE_DIR/status.json" ]]; then
    STORED_REPO=$(jq -r '.repo // ""' "$SUB_PIPELINE_DIR/status.json")
    [[ "$STORED_REPO" == "$REPO_RELATIVE" ]] || die "sub-pipeline repo ownership mismatch: $SUB_PIPELINE_DIR"
    STORED_ROOT_PIPELINE_ID=$(jq -r '.root_pipeline_id // ""' "$SUB_PIPELINE_DIR/status.json")
    [[ "$STORED_ROOT_PIPELINE_ID" == "$ROOT_PIPELINE_ID" ]] || die "sub-pipeline root ownership mismatch: $SUB_PIPELINE_DIR"
    BRANCH=$(jq -r '.branch // ""' "$SUB_PIPELINE_DIR/status.json")
    # 이름은 run을 시작할 때 정해지므로 설정·인자로 다시 만들지 않고 root 등록과 대조한다. 등록 전에 죽었으면 root 항목이 없다.
    REGISTERED_BRANCH=$(jq -r --arg id "$REPO_ID" '.sub_pipelines[]? | select(.repo_id == $id) | .branch' "$ROOT_STATUS")
    [[ -z "$REGISTERED_BRANCH" || "$BRANCH" == "$REGISTERED_BRANCH" ]] \
      || die "sub-pipeline branch ownership mismatch: registered '$REGISTERED_BRANCH', found '$BRANCH'"
    [[ -z "$BRANCH_ARG" || "$BRANCH" == "$BRANCH_ARG" ]] \
      || die "sub-pipeline branch ownership mismatch: run owns '$BRANCH', --branch requested '$BRANCH_ARG'"
    BASE_REVISION=$(jq -r '.base_revision // ""' "$SUB_PIPELINE_DIR/status.json")
    BASE_BRANCH=$(jq -r '.base_branch // ""' "$SUB_PIPELINE_DIR/status.json")
    STARTED_AT=$(jq -r '.started_at // ""' "$SUB_PIPELINE_DIR/status.json")
  else
    # --branch(세션이 사용자 규칙으로 정한 이름) > git.branch_prefix > task. 접두사가 /로 끝나면 구분자 없이 잇는다.
    # shortcut: yq·PyYAML이 모두 없으면 config_get이 빈 값을 돌려 기본 접두사가 쓰인다, 그런 환경이 보고되면 pylon이 값을 넘기게 바꾼다.
    BRANCH_PREFIX=$(config_get "git.branch_prefix" "task")
    [[ "$BRANCH_PREFIX" == */ ]] || BRANCH_PREFIX="${BRANCH_PREFIX}-"
    BRANCH="${BRANCH_ARG:-${BRANCH_PREFIX}${ROOT_PIPELINE_ID}-${REPO_ID}}"
    git check-ref-format "refs/heads/$BRANCH" || die "브랜치 이름으로 쓸 수 없습니다: $BRANCH"
    ! is_protected_branch "$BRANCH" || die "protected branch는 작업 브랜치로 쓸 수 없습니다: $BRANCH"
    BASE_REVISION=$(git -C "$GIT_ROOT" rev-parse HEAD)
    BASE_BRANCH=$(git -C "$GIT_ROOT" branch --show-current)
    STARTED_AT="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  fi
  [[ -n "$BASE_REVISION" ]] || die "sub-pipeline base revision is missing: $SUB_PIPELINE_DIR"
  if git -C "$GIT_ROOT" show-ref --verify --quiet "refs/heads/$BRANCH"; then
    [[ -f "$SUB_PIPELINE_DIR/status.json" ]] || die "branch already exists without sub-pipeline ownership: $BRANCH"
    git -C "$GIT_ROOT" merge-base --is-ancestor "$BASE_REVISION" "$BRANCH" \
      || die "owned branch no longer descends from recorded base revision: $BRANCH ($BASE_REVISION)"
    git -C "$GIT_ROOT" checkout "$BRANCH" || die "브랜치 전환 실패: $BRANCH"
  else
    git -C "$GIT_ROOT" checkout -b "$BRANCH" "$BASE_REVISION" || die "브랜치 생성 실패: $BRANCH"
  fi

  # Initialize sub-pipeline status.json
  jq -cn \
    --arg scope "repo" \
    --arg root_pipeline_id "$ROOT_PIPELINE_ID" \
    --arg repo_id "$REPO_ID" \
    --arg repo "$REPO_RELATIVE" \
    --arg stage "init" \
    --arg status "running" \
    --arg branch "$BRANCH" \
    --arg base_branch "$BASE_BRANCH" \
    --arg base_revision "$BASE_REVISION" \
    --arg pipeline_dir "$SUB_PIPELINE_DIR" \
    --arg started "$STARTED_AT" \
    '{scope: $scope, root_pipeline_id: $root_pipeline_id, repo_id: $repo_id, repo: $repo,
      stage: $stage, status: $status, branch: $branch, base_branch: $base_branch,
      base_revision: $base_revision, pipeline_dir: $pipeline_dir, started_at: $started}' \
    > "$SUB_PIPELINE_DIR/status.json"

  # Register this repo-owned pipeline in the logical root pipeline.
  LOCK_DIR="$PIPELINE_DIR_ARG/.sub-pipelines.lock"
  LOCK_ACQUIRED=false
  for _ in $(seq 1 100); do
    if mkdir "$LOCK_DIR" 2>/dev/null; then
      LOCK_ACQUIRED=true
      break
    fi
    sleep 0.05
  done
  [[ "$LOCK_ACQUIRED" == "true" ]] || die "root pipeline registration lock timeout: $PIPELINE_DIR_ARG"
  trap 'rmdir "$LOCK_DIR" 2>/dev/null || true' EXIT

  ENTRY=$(jq -cn \
    --arg repo_id "$REPO_ID" \
    --arg repo "$REPO_RELATIVE" \
    --arg branch "$BRANCH" \
    --arg base_revision "$BASE_REVISION" \
    --arg pipeline_dir "$SUB_PIPELINE_DIR" \
    '{repo_id: $repo_id, repo: $repo, branch: $branch, base_revision: $base_revision,
      pipeline_dir: $pipeline_dir, status: "running"}')
  ROOT_STATUS_TMP=$(mktemp "$PIPELINE_DIR_ARG/.status.XXXXXX")
  jq --argjson entry "$ENTRY" \
    '.sub_pipelines = (((.sub_pipelines // []) | map(select(.repo_id != $entry.repo_id))) + [$entry])' \
    "$ROOT_STATUS" > "$ROOT_STATUS_TMP"
  mv "$ROOT_STATUS_TMP" "$ROOT_STATUS"
  rmdir "$LOCK_DIR"
  trap - EXIT

  # JSON output: pipeline_dir is the sub-pipeline dir
  jq -cn \
    --arg id "$ROOT_PIPELINE_ID" \
    --arg scope "repo" \
    --arg repo_id "$REPO_ID" \
    --arg repo "$REPO_RELATIVE" \
    --arg branch "$BRANCH" \
    --arg base_revision "$BASE_REVISION" \
    --arg dir "$SUB_PIPELINE_DIR" \
    '{pipeline_id: $id, scope: $scope, repo_id: $repo_id, repo: $repo,
      branch: $branch, base_revision: $base_revision, pipeline_dir: $dir}'

  exit 0
fi

# --- Error: --git-root without --pipeline-dir ---
if [[ -n "$GIT_ROOT_ARG" && -z "$PIPELINE_DIR_ARG" ]]; then
  die "--git-root requires --pipeline-dir to be specified as well"
fi

[[ -z "$BRANCH_ARG" ]] || die "--branch는 --git-root, --pipeline-dir와 함께만 쓸 수 있습니다 (root pipeline은 브랜치를 소유하지 않습니다)"

# --- Root mode: no --git-root ---
# Step 1: PIPELINE_DIR + status.json initialization only. No branch creation.
PIPELINE_DIR="$RUNTIME_DIR/${PIPELINE_ID}"
mkdir -p "$PIPELINE_DIR"

# Write requirement
echo "$REQUIREMENT" > "$PIPELINE_DIR/requirement.md"

# Initialize root status.json with empty sub_pipelines array
jq -cn \
  --arg pipeline_id "$PIPELINE_ID" \
  --arg scope "root" \
  --arg stage "init" \
  --arg status "running" \
  --arg started "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  '{pipeline_id: $pipeline_id, scope: $scope, stage: $stage, status: $status,
    started_at: $started, sub_pipelines: []}' \
  > "$PIPELINE_DIR/status.json"

# JSON output
jq -cn \
  --arg id "$PIPELINE_ID" \
  --arg scope "root" \
  --arg dir "$PIPELINE_DIR" \
  '{pipeline_id: $id, scope: $scope, pipeline_dir: $dir}'
