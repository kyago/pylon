#!/bin/bash
set -euo pipefail
source "$(dirname "$0")/common.sh"

require_cmd go jq

PIPELINE_DIR="${1:?Usage: run-verification.sh <pipeline-dir>}"

cd "$REPO_ROOT"

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
