#!/bin/bash
set -euo pipefail
source "$(dirname "$0")/common.sh"

require_cmd git jq

REQUIREMENT="${1:?Usage: init-pipeline.sh <requirement>}"

# Generate slug from requirement
SLUG=$(echo "$REQUIREMENT" | tr '[:upper:]' '[:lower:]' | sed 's/[^a-z0-9가-힣]/-/g' | sed 's/--*/-/g' | cut -c1-30 | sed 's/-$//')
PIPELINE_ID="$(date +%Y%m%d)-${SLUG}"
BRANCH="task-${SLUG}"

# Create or switch to branch
git checkout -b "$BRANCH" 2>/dev/null || git checkout "$BRANCH"

# Create pipeline runtime directory
PIPELINE_DIR="$RUNTIME_DIR/${PIPELINE_ID}"
mkdir -p "$PIPELINE_DIR"

# Write requirement
echo "$REQUIREMENT" > "$PIPELINE_DIR/requirement.md"

# Write initial status
jq -cn \
  --arg stage "init" \
  --arg status "running" \
  --arg started "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  '{stage: $stage, status: $status, started_at: $started}' \
  > "$PIPELINE_DIR/status.json"

# JSON output
jq -cn \
  --arg id "$PIPELINE_ID" \
  --arg branch "$BRANCH" \
  --arg dir "$PIPELINE_DIR" \
  '{pipeline_id: $id, branch: $branch, pipeline_dir: $dir}'
