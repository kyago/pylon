#!/bin/bash
set -euo pipefail
source "$(dirname "$0")/common.sh"

require_cmd git jq

PIPELINE_DIR="${1:-}"
BRANCH="${2:-}"

CLEANED=()

# Remove worktrees associated with the branch (exact branch match via porcelain output)
if [[ -n "$BRANCH" ]]; then
  while IFS= read -r wt; do
    [[ -z "$wt" ]] && continue
    wt_branch=$(git worktree list --porcelain | grep -A2 "^worktree ${wt}$" | grep "^branch " | sed 's|^branch refs/heads/||')
    if [[ "$wt_branch" == "$BRANCH" || "$wt_branch" == "$BRANCH/"* ]]; then
      git worktree remove "$wt" --force 2>/dev/null && CLEANED+=("worktree:$wt")
    fi
  done < <(git worktree list --porcelain | grep "^worktree " | sed 's/^worktree //')

  # Delete agent sub-branches
  while IFS= read -r b; do
    if [[ -n "$b" && "$b" == *"$BRANCH/"* ]]; then
      git branch -D "$b" 2>/dev/null && CLEANED+=("branch:$b")
    fi
  done < <(git branch --list "${BRANCH}/*" 2>/dev/null | sed 's/^[* ]*//')
fi

# Clean pipeline runtime directory
if [[ -n "$PIPELINE_DIR" && -d "$PIPELINE_DIR" ]]; then
  # Mark as cleaned, don't delete (keep for history)
  jq -cn --arg status "cleaned" --arg cleaned_at "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
    '{status: $status, cleaned_at: $cleaned_at}' > "$PIPELINE_DIR/status.json"
  CLEANED+=("runtime:$PIPELINE_DIR")
fi

CLEANED_JSON=$(printf '%s\n' "${CLEANED[@]}" 2>/dev/null | jq -R . | jq -s . 2>/dev/null || echo "[]")
jq -cn --argjson cleaned "$CLEANED_JSON" '{ok: true, cleaned: $cleaned}'
