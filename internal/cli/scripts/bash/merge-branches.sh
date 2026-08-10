#!/bin/bash
set -euo pipefail
source "$(dirname "$0")/common.sh"

require_cmd git jq

GIT_ROOT_ARG=$(extract_arg "git-root" "$@")
ARGS=()
while [[ $# -gt 0 ]]; do
  case "$1" in
    --git-root) shift 2 ;;
    *) ARGS+=("$1"); shift ;;
  esac
done

TARGET_BRANCH="${ARGS[0]:?Usage: merge-branches.sh <target-branch> <source-branch>... [--git-root <repo-rel-path>]}"
SOURCE_BRANCHES=("${ARGS[@]:1}")

resolve_git_root "$GIT_ROOT_ARG"
cd "$GIT_ROOT" || die "GIT_ROOT로 이동 실패: $GIT_ROOT"

[[ ${#SOURCE_BRANCHES[@]} -eq 0 ]] && die "No source branches to merge"

git checkout "$TARGET_BRANCH" >&2

MERGED=()
FAILED=()

for branch in "${SOURCE_BRANCHES[@]}"; do
  if git merge "$branch" --no-edit >&2; then
    MERGED+=("$branch")
  else
    git merge --abort 2>/dev/null || true
    FAILED+=("$branch")
  fi
done

MERGED_JSON=$(array_to_json ${MERGED[@]+"${MERGED[@]}"})
FAILED_JSON=$(array_to_json ${FAILED[@]+"${FAILED[@]}"})

RESULT=$(jq -cn \
  --arg target "$TARGET_BRANCH" \
  --argjson merged "$MERGED_JSON" \
  --argjson failed "$FAILED_JSON" \
  '{target: $target, merged: $merged, failed: $failed, ok: (($failed | length) == 0)}')
echo "$RESULT"

[[ ${#FAILED[@]} -eq 0 ]]
