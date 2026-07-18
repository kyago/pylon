#!/bin/bash
set -euo pipefail
source "$(dirname "$0")/common.sh"

require_cmd git jq

TARGET_BRANCH="${1:?Usage: merge-branches.sh <target-branch> <source-branch>...}"
shift
SOURCE_BRANCHES=("$@")

[[ ${#SOURCE_BRANCHES[@]} -eq 0 ]] && die "No source branches to merge"

git checkout "$TARGET_BRANCH"

MERGED=()
FAILED=()

for branch in "${SOURCE_BRANCHES[@]}"; do
  if git merge "$branch" --no-edit 2>/dev/null; then
    MERGED+=("$branch")
  else
    git merge --abort 2>/dev/null || true
    FAILED+=("$branch")
  fi
done

MERGED_JSON=$(array_to_json ${MERGED[@]+"${MERGED[@]}"})
FAILED_JSON=$(array_to_json ${FAILED[@]+"${FAILED[@]}"})

jq -cn \
  --arg target "$TARGET_BRANCH" \
  --argjson merged "$MERGED_JSON" \
  --argjson failed "$FAILED_JSON" \
  '{target: $target, merged: $merged, failed: $failed, ok: (($failed | length) == 0)}'
