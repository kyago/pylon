#!/bin/bash
set -euo pipefail
source "$(dirname "$0")/common.sh"

require_cmd git jq

PIPELINE_DIR="${1:-}"
[[ -n "$PIPELINE_DIR" ]] || die "Usage: cleanup-pipeline.sh <pipeline-dir> [--terminal-phase completed|cancelled|failed] [--git-root <repo>] [--branch <branch>]"
shift || true

BRANCH_ARG=""
GIT_ROOT_ARG=""
TERMINAL_PHASE=""

# Keep the old positional branch/true form readable, but no longer trust the
# boolean alone: a matching history manifest must exist before deletion.
if [[ $# -gt 0 && "$1" != --* ]]; then
  BRANCH_ARG="$1"
  shift
fi
if [[ $# -gt 0 && "$1" == "true" ]]; then
  TERMINAL_PHASE="completed"
  shift
fi
while [[ $# -gt 0 ]]; do
  case "$1" in
    --branch) BRANCH_ARG="${2:-}"; shift 2 ;;
    --git-root) GIT_ROOT_ARG="${2:-}"; shift 2 ;;
    --terminal-phase) TERMINAL_PHASE="${2:-}"; shift 2 ;;
    *) die "unknown cleanup argument: $1" ;;
  esac
done

[[ -f "$PIPELINE_DIR/status.json" ]] || die "pipeline status not found: $PIPELINE_DIR/status.json"

SCOPE=$(jq -r 'if .scope then .scope elif (.branch // "") != "" then "repo" else "root" end' "$PIPELINE_DIR/status.json")
ROOT_PIPELINE_ID=$(jq -r --arg fallback "$(basename "$PIPELINE_DIR")" '.root_pipeline_id // .pipeline_id // $fallback' "$PIPELINE_DIR/status.json")
BRANCH=$(jq -r '.branch // ""' "$PIPELINE_DIR/status.json")
REPO=$(jq -r '.repo // ""' "$PIPELINE_DIR/status.json")

if [[ -n "$BRANCH_ARG" && "$BRANCH_ARG" != "$BRANCH" ]]; then
  die "branch ownership mismatch: status owns '$BRANCH', argument requested '$BRANCH_ARG'"
fi
if [[ "$SCOPE" == "root" && -n "$BRANCH_ARG" ]]; then
  die "root pipeline does not own a branch"
fi
if [[ -n "$GIT_ROOT_ARG" && -n "$REPO" && "$GIT_ROOT_ARG" != "$REPO" ]]; then
  die "repo ownership mismatch: status owns '$REPO', argument requested '$GIT_ROOT_ARG'"
fi

case "$TERMINAL_PHASE" in
  "")
    STATUS_TMP=$(mktemp "$PIPELINE_DIR/.status.XXXXXX")
    jq --arg cleanup_status "preserved" --arg cleanup_reason "terminal_checkpoint_required" \
      '.cleanup = {status: $cleanup_status, reason: $cleanup_reason}' \
      "$PIPELINE_DIR/status.json" > "$STATUS_TMP"
    mv "$STATUS_TMP" "$PIPELINE_DIR/status.json"
    jq -cn --arg pipeline_dir "$PIPELINE_DIR" \
      '{ok: false, preserved: [$pipeline_dir], reason: "terminal_checkpoint_required"}'
    exit 0
    ;;
  completed|cancelled|failed) ;;
  *) die "invalid terminal phase: $TERMINAL_PHASE" ;;
esac

MANIFEST="$PYLON_DIR/history/pipelines/$ROOT_PIPELINE_ID/$TERMINAL_PHASE/manifest.json"
[[ -f "$MANIFEST" ]] || die "matching terminal checkpoint not found: $MANIFEST"
MANIFEST_PIPELINE=$(jq -r '.pipeline_id // ""' "$MANIFEST")
MANIFEST_PHASE=$(jq -r '.phase // ""' "$MANIFEST")
[[ "$MANIFEST_PIPELINE" == "$ROOT_PIPELINE_ID" && "$MANIFEST_PHASE" == "$TERMINAL_PHASE" ]] \
  || die "terminal checkpoint manifest does not match pipeline/phase: $MANIFEST"

CLEANED=()

# Remove worktrees associated with the branch (exact branch match via porcelain output)
if [[ "$SCOPE" == "repo" ]]; then
  [[ -n "$REPO" ]] || die "repo pipeline status is missing repo"
  resolve_git_root "${GIT_ROOT_ARG:-$REPO}"
  git -C "$GIT_ROOT" rev-parse --git-dir &>/dev/null || die "repo is not a git repository: $REPO"

  while IFS= read -r wt; do
    [[ -z "$wt" ]] && continue
    wt_branch=$(git -C "$GIT_ROOT" worktree list --porcelain | grep -A2 "^worktree ${wt}$" | grep "^branch " | sed 's|^branch refs/heads/||')
    if [[ "$wt_branch" == "$BRANCH" || "$wt_branch" == "$BRANCH/"* || "$wt_branch" == "$BRANCH--"* ]]; then
      git -C "$GIT_ROOT" worktree remove "$wt" --force 2>/dev/null && CLEANED+=("worktree:$wt")
    fi
  done < <(git -C "$GIT_ROOT" worktree list --porcelain | grep "^worktree " | sed 's/^worktree //')

  # Delete agent sub-branches
  while IFS= read -r b; do
    if [[ -n "$b" && ( "$b" == "$BRANCH/"* || "$b" == "$BRANCH--"* ) ]]; then
      git -C "$GIT_ROOT" branch -D "$b" 2>/dev/null && CLEANED+=("branch:$b")
    fi
  done < <({
    git -C "$GIT_ROOT" branch --list "${BRANCH}/*" 2>/dev/null
    git -C "$GIT_ROOT" branch --list "${BRANCH}--*" 2>/dev/null
  } | sed 's/^[* ]*//')
fi

# Clean pipeline runtime directory
if [[ -n "$PIPELINE_DIR" && -d "$PIPELINE_DIR" ]]; then
  chmod -R u+w "$PIPELINE_DIR" || die "runtime 권한 복구 실패: $PIPELINE_DIR"
  rm -rf "$PIPELINE_DIR"
  CLEANED+=("runtime-deleted:$PIPELINE_DIR")
fi

CLEANED_JSON=$(array_to_json ${CLEANED[@]+"${CLEANED[@]}"})
jq -cn --argjson cleaned "$CLEANED_JSON" '{ok: true, cleaned: $cleaned}'
