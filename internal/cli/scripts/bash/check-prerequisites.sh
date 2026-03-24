#!/bin/bash
set -euo pipefail
source "$(dirname "$0")/common.sh"

usage() {
  echo "Usage: check-prerequisites.sh --pipeline-dir <dir> [--require-requirement] [--require-architecture] [--require-tasks]"
  exit 1
}

PIPELINE_DIR=""
REQUIREMENTS=()

while [[ $# -gt 0 ]]; do
  case "$1" in
    --pipeline-dir) PIPELINE_DIR="$2"; shift 2 ;;
    --require-requirement) REQUIREMENTS+=("requirement.md"); shift ;;
    --require-architecture) REQUIREMENTS+=("architecture.md"); shift ;;
    --require-tasks) REQUIREMENTS+=("tasks.json"); shift ;;
    --require-analysis) REQUIREMENTS+=("requirement-analysis.md"); shift ;;
    *) usage ;;
  esac
done

[[ -z "$PIPELINE_DIR" ]] && die "Missing --pipeline-dir"
[[ -d "$PIPELINE_DIR" ]] || die "Pipeline directory not found: $PIPELINE_DIR"

MISSING=()
FOUND=()

for file in "${REQUIREMENTS[@]}"; do
  if [[ -f "$PIPELINE_DIR/$file" ]]; then
    FOUND+=("$file")
  else
    MISSING+=("$file")
  fi
done

# Build JSON output
FOUND_JSON=$(printf '%s\n' "${FOUND[@]}" | jq -R . | jq -s .)
MISSING_JSON=$(printf '%s\n' "${MISSING[@]}" 2>/dev/null | jq -R . | jq -s . 2>/dev/null || echo "[]")

if [[ ${#MISSING[@]} -gt 0 ]]; then
  jq -cn \
    --argjson found "$FOUND_JSON" \
    --argjson missing "$MISSING_JSON" \
    '{ok: false, found: $found, missing: $missing}'
  exit 1
else
  jq -cn \
    --argjson found "$FOUND_JSON" \
    '{ok: true, found: $found, missing: []}'
fi
