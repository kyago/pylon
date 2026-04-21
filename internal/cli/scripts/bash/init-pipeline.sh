#!/bin/bash
set -euo pipefail
source "$(dirname "$0")/common.sh"

require_cmd git jq

REQUIREMENT="${1:?Usage: init-pipeline.sh <requirement>}"

# Generate slug from requirement
SLUG=$(echo "$REQUIREMENT" | tr '[:upper:]' '[:lower:]' | sed 's/[^a-z0-9가-힣]/-/g' | sed 's/--*/-/g' | cut -c1-30 | sed 's/-$//')
PIPELINE_ID="$(date +%Y%m%d)-${SLUG}"
BRANCH="task-${SLUG}"

# Branch management:
#   - protected branch → must leave; create or switch to task branch
#   - already on the task branch → continuation, no-op
#   - other feature branch → new task; create task branch
CURRENT_BRANCH=$(git branch --show-current)

if is_protected_branch "$CURRENT_BRANCH"; then
  if git show-ref --verify --quiet "refs/heads/$BRANCH"; then
    git checkout "$BRANCH" || die "브랜치 전환 실패: $BRANCH"
  else
    git checkout -b "$BRANCH" || die "브랜치 생성 실패: $BRANCH"
  fi
elif [[ "$CURRENT_BRANCH" != "$BRANCH" ]]; then
  echo "INFO: '$CURRENT_BRANCH'에서 파이프라인 브랜치 '$BRANCH'로 전환합니다." >&2
  if git show-ref --verify --quiet "refs/heads/$BRANCH"; then
    git checkout "$BRANCH" || die "브랜치 전환 실패: $BRANCH"
  else
    git checkout -b "$BRANCH" || die "브랜치 생성 실패: $BRANCH"
  fi
fi

# Create pipeline runtime directory
PIPELINE_DIR="$RUNTIME_DIR/${PIPELINE_ID}"
mkdir -p "$PIPELINE_DIR"

# Write requirement
echo "$REQUIREMENT" > "$PIPELINE_DIR/requirement.md"

# Write initial status (branch 필드 포함)
jq -cn \
  --arg stage "init" \
  --arg status "running" \
  --arg branch "$BRANCH" \
  --arg started "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  '{stage: $stage, status: $status, branch: $branch, started_at: $started}' \
  > "$PIPELINE_DIR/status.json"

# JSON output
jq -cn \
  --arg id "$PIPELINE_ID" \
  --arg branch "$BRANCH" \
  --arg dir "$PIPELINE_DIR" \
  '{pipeline_id: $id, branch: $branch, pipeline_dir: $dir}'
