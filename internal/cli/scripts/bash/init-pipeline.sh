#!/bin/bash
set -euo pipefail

source "$(dirname "$0")/common.sh"

GIT_ROOT_ARG=$(extract_arg "git-root" "$@")
PIPELINE_DIR_ARG=$(extract_arg "pipeline-dir" "$@")
resolve_git_root "$GIT_ROOT_ARG"
cd "$GIT_ROOT" || die "GIT_ROOT로 이동 실패: $GIT_ROOT"

require_cmd git jq

REQUIREMENT="${1:?Usage: init-pipeline.sh <requirement> [--git-root <repo-rel-path>] [--pipeline-dir <root-pipeline-dir>]}"

# Generate slug from requirement
SLUG=$(echo "$REQUIREMENT" | tr '[:upper:]' '[:lower:]' | sed 's/[^a-z0-9가-힣]/-/g' | sed 's/--*/-/g' | cut -c1-30 | sed 's/-$//')
[[ -n "$SLUG" ]] || SLUG="run"
PIPELINE_ID="$(date +%Y%m%d)-${SLUG}"
BRANCH="task-${SLUG}"

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

  # Create or checkout branch in target repo
  if ! git -C "$GIT_ROOT" diff --quiet || ! git -C "$GIT_ROOT" diff --cached --quiet; then
    die "$GIT_ROOT_ARG 에 uncommitted changes가 있습니다. commit 또는 stash 후 다시 실행하세요."
  fi
  if [[ -f "$SUB_PIPELINE_DIR/status.json" ]]; then
    STORED_REPO=$(jq -r '.repo // ""' "$SUB_PIPELINE_DIR/status.json")
    [[ "$STORED_REPO" == "$REPO_RELATIVE" ]] || die "sub-pipeline repo ownership mismatch: $SUB_PIPELINE_DIR"
    BASE_REVISION=$(jq -r '.base_revision // ""' "$SUB_PIPELINE_DIR/status.json")
    BASE_BRANCH=$(jq -r '.base_branch // ""' "$SUB_PIPELINE_DIR/status.json")
    STARTED_AT=$(jq -r '.started_at // ""' "$SUB_PIPELINE_DIR/status.json")
  else
    BASE_REVISION=$(git -C "$GIT_ROOT" rev-parse HEAD)
    BASE_BRANCH=$(git -C "$GIT_ROOT" branch --show-current)
    STARTED_AT="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  fi
  [[ -n "$BASE_REVISION" ]] || die "sub-pipeline base revision is missing: $SUB_PIPELINE_DIR"
  if git -C "$GIT_ROOT" show-ref --verify --quiet "refs/heads/$BRANCH"; then
    git -C "$GIT_ROOT" checkout "$BRANCH" || die "브랜치 전환 실패: $BRANCH"
  else
    git -C "$GIT_ROOT" checkout -b "$BRANCH" || die "브랜치 생성 실패: $BRANCH"
  fi

  # Initialize sub-pipeline status.json
  ROOT_PIPELINE_ID=$(jq -r --arg fallback "$(basename "$PIPELINE_DIR_ARG")" '.pipeline_id // $fallback' "$ROOT_STATUS")
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
