#!/bin/bash
set -euo pipefail

source "$(dirname "$0")/common.sh"

GIT_ROOT_ARG=$(extract_arg "git-root" "$@")
resolve_git_root "$GIT_ROOT_ARG"

require_cmd jq

PIPELINE_DIR="${1:?Usage: run-verification.sh <pipeline-dir> [--git-root <repo-rel-path>]}"

# If --git-root specified, run verification in that repo
if [[ -n "$GIT_ROOT_ARG" ]]; then
  cd "$GIT_ROOT" || die "--git-root 경로로 이동 실패: $GIT_ROOT"

  # Skip verification if no go.mod (non-Go repo)
  if [[ ! -f "go.mod" ]]; then
    echo "INFO: go.mod not found in $GIT_ROOT, skipping Go verification" >&2
    OUTPUT=$(jq -cn \
      --arg timestamp "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
      '{ok: true, checks: [], skipped: true, timestamp: $timestamp}')
    if [[ -d "$PIPELINE_DIR" ]]; then
      echo "$OUTPUT" > "$PIPELINE_DIR/verification.json"
    fi
    echo "$OUTPUT" | jq .
    exit 0
  fi
else
  cd "$REPO_ROOT"
fi

require_cmd go

RESULTS=()
OVERALL_OK=true

# Run go build
BUILD_OUTPUT=$(go build ./... 2>&1) && BUILD_OK=true || BUILD_OK=false
RESULTS+=("$(jq -cn --arg name "build" --arg ok "$BUILD_OK" --arg output "$BUILD_OUTPUT" '{name: $name, ok: ($ok == "true"), output: $output}')")
[[ "$BUILD_OK" == "false" ]] && OVERALL_OK=false

# Run go vet
VET_OUTPUT=$(go vet ./... 2>&1) && VET_OK=true || VET_OK=false
RESULTS+=("$(jq -cn --arg name "vet" --arg ok "$VET_OK" --arg output "$VET_OUTPUT" '{name: $name, ok: ($ok == "true"), output: $output}')")
[[ "$VET_OK" == "false" ]] && OVERALL_OK=false

# Run go test
TEST_OUTPUT=$(go test ./... 2>&1) && TEST_OK=true || TEST_OK=false
RESULTS+=("$(jq -cn --arg name "test" --arg ok "$TEST_OK" --arg output "$TEST_OUTPUT" '{name: $name, ok: ($ok == "true"), output: $output}')")
[[ "$TEST_OK" == "false" ]] && OVERALL_OK=false

# Combine results
RESULTS_JSON=$(printf '%s\n' "${RESULTS[@]}" | jq -s .)

OUTPUT=$(jq -cn \
  --arg ok "$OVERALL_OK" \
  --argjson checks "$RESULTS_JSON" \
  --arg timestamp "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  '{ok: ($ok == "true"), checks: $checks, timestamp: $timestamp}')

# Write to pipeline dir if it exists
if [[ -d "$PIPELINE_DIR" ]]; then
  echo "$OUTPUT" > "$PIPELINE_DIR/verification.json"
fi

echo "$OUTPUT" | jq .
[[ "$OVERALL_OK" == "true" ]] || exit 1
