#!/bin/bash
set -euo pipefail

source "$(dirname "$0")/common.sh"

GIT_ROOT_ARG=$(extract_arg "git-root" "$@")
resolve_git_root "$GIT_ROOT_ARG"

PIPELINE_DIR="${1:?Usage: run-verification.sh <pipeline-dir> [--git-root <repo-rel-path>]}"

cd "$GIT_ROOT" || die "프로젝트 경로로 이동 실패: $GIT_ROOT"
require_cmd pylon

VERIFY_ARGS=(
  internal verify
  --workdir "$GIT_ROOT"
  --config "$GIT_ROOT/.pylon/verify.yml"
)
if [[ -d "$PIPELINE_DIR" ]]; then
  VERIFY_ARGS+=(--output "$PIPELINE_DIR/verification.json")
fi
pylon "${VERIFY_ARGS[@]}"
