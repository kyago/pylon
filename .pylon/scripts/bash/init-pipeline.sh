#!/bin/bash
set -euo pipefail

# Pre-parse --git-root and --pipeline-dir before sourcing common.sh,
# because common.sh sets GIT_ROOT at source time.
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

source "$(dirname "$0")/common.sh"

# Override GIT_ROOT if --git-root was specified (priority 1)
if [[ -n "$GIT_ROOT_ARG" ]]; then
  GIT_ROOT="$(realpath "$REPO_ROOT/$GIT_ROOT_ARG")"
fi

cd "$GIT_ROOT" || die "GIT_ROOT로 이동 실패: $GIT_ROOT"

require_cmd git jq

REQUIREMENT="${1:?Usage: init-pipeline.sh <requirement> [--git-root <repo-rel-path>] [--pipeline-dir <root-pipeline-dir>]}"

# Generate slug from requirement
SLUG=$(echo "$REQUIREMENT" | tr '[:upper:]' '[:lower:]' | sed 's/[^a-z0-9가-힣]/-/g' | sed 's/--*/-/g' | cut -c1-30 | sed 's/-$//')
PIPELINE_ID="$(date +%Y%m%d)-${SLUG}"
BRANCH="task-${SLUG}"

# --- Sub-pipeline mode: --git-root + --pipeline-dir both provided ---
if [[ -n "$GIT_ROOT_ARG" && -n "$PIPELINE_DIR_ARG" ]]; then
  # Validate: GIT_ROOT must be a valid git repo
  git -C "$GIT_ROOT" rev-parse --git-dir &>/dev/null \
    || die "--git-root '$GIT_ROOT_ARG' is not a valid git repository"

  REPO_BASENAME="$(basename "$GIT_ROOT")"
  SUB_PIPELINE_DIR="${PIPELINE_DIR_ARG}/${REPO_BASENAME}"
  mkdir -p "$SUB_PIPELINE_DIR"

  # Create or checkout branch in target repo
  if ! git -C "$GIT_ROOT" diff --quiet || ! git -C "$GIT_ROOT" diff --cached --quiet; then
    die "$GIT_ROOT_ARG 에 uncommitted changes가 있습니다. commit 또는 stash 후 다시 실행하세요."
  fi
  if git -C "$GIT_ROOT" show-ref --verify --quiet "refs/heads/$BRANCH"; then
    git -C "$GIT_ROOT" checkout "$BRANCH" || die "브랜치 전환 실패: $BRANCH"
  else
    git -C "$GIT_ROOT" checkout -b "$BRANCH" || die "브랜치 생성 실패: $BRANCH"
  fi

  # Initialize sub-pipeline status.json
  jq -cn \
    --arg stage "init" \
    --arg status "running" \
    --arg branch "$BRANCH" \
    --arg started "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
    '{stage: $stage, status: $status, branch: $branch, started_at: $started}' \
    > "$SUB_PIPELINE_DIR/status.json"

  # JSON output: pipeline_dir is the sub-pipeline dir
  jq -cn \
    --arg id "$PIPELINE_ID" \
    --arg branch "$BRANCH" \
    --arg dir "$SUB_PIPELINE_DIR" \
    '{pipeline_id: $id, branch: $branch, pipeline_dir: $dir}'

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
  --arg stage "init" \
  --arg status "running" \
  --arg started "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  '{stage: $stage, status: $status, started_at: $started, sub_pipelines: []}' \
  > "$PIPELINE_DIR/status.json"

# JSON output
jq -cn \
  --arg id "$PIPELINE_ID" \
  --arg dir "$PIPELINE_DIR" \
  '{pipeline_id: $id, pipeline_dir: $dir}'
