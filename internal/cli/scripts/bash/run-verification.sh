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

# GIT_ROOT가 워크스페이스 루트나 상위 repo로 잘못 해석된 경우를 조기에 잡는다.
[[ -f "$GIT_ROOT/.pylon/verify.yml" || -f "$GIT_ROOT/go.mod" ]] ||
  die "$GIT_ROOT 에 .pylon/verify.yml이 없습니다. --git-root <프로젝트 상대경로> 지정을 확인하고, 파일이 없으면 pylon doctor로 재생성하세요"

# live verify.yml 경로는 재계산하지 않는다 — snapshot의 Source.Path(절대경로)를 사용해
# snapshot 생성 시점과 검증 시점의 경로 불일치로 인한 오탐을 막는다.
VERIFY_ARGS=(
  internal verify
  --workdir "$GIT_ROOT"
  --snapshot "$CRITERIA_PATH"
  --manifest "$MANIFEST_PATH"
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
