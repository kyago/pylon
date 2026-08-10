#!/bin/bash
set -euo pipefail

source "$(dirname "$0")/common.sh"

GIT_ROOT_ARG=$(extract_arg "git-root" "$@")
resolve_git_root "$GIT_ROOT_ARG"

PIPELINE_DIR="${1:?Usage: run-verification.sh <pipeline-dir> [--git-root <repo-rel-path>]}"
CRITERIA_PATH="$PIPELINE_DIR/criteria.json"
MANIFEST_PATH="$PIPELINE_DIR/status.json"

[[ -f "$CRITERIA_PATH" ]] || die "criteria snapshot not found: $CRITERIA_PATH (create it before worker execution)"
[[ -f "$MANIFEST_PATH" ]] || die "repo pipeline manifest not found: $MANIFEST_PATH"

cd "$GIT_ROOT" || die "프로젝트 경로로 이동 실패: $GIT_ROOT"
require_cmd pylon jq

VERIFY_ARGS=(
  internal verify
  --workdir "$GIT_ROOT"
  --snapshot "$CRITERIA_PATH"
  --manifest "$MANIFEST_PATH"
  --live-config "$GIT_ROOT/.pylon/verify.yml"
)
HELD_OUT_REL=$(jq -r '.held_out.path // empty' "$MANIFEST_PATH")
if [[ -n "$HELD_OUT_REL" ]]; then
  HELD_OUT_PATH="$PIPELINE_DIR/$HELD_OUT_REL"
  [[ -f "$HELD_OUT_PATH" ]] || die "held-out evaluator snapshot not found: $HELD_OUT_PATH"
  VERIFY_ARGS+=(--held-out "$HELD_OUT_PATH")
fi
if [[ -d "$PIPELINE_DIR" ]]; then
  VERIFY_ARGS+=(--output "$PIPELINE_DIR/verification.json")
fi
pylon "${VERIFY_ARGS[@]}"
