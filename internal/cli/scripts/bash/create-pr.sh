#!/bin/bash
set -euo pipefail
source "$(dirname "$0")/common.sh"

require_cmd gh jq

PIPELINE_DIR="${1:?Usage: create-pr.sh <pipeline-dir> [--title <title>] [--body <body>] [--draft]}"
shift

TITLE=""
BODY=""
DRAFT=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --title) TITLE="$2"; shift 2 ;;
    --body) BODY="$2"; shift 2 ;;
    --draft) DRAFT="--draft"; shift ;;
    *) shift ;;
  esac
done

# Read requirement for default title
if [[ -z "$TITLE" && -f "$PIPELINE_DIR/requirement.md" ]]; then
  TITLE=$(head -1 "$PIPELINE_DIR/requirement.md")
fi
[[ -z "$TITLE" ]] && die "PR title required (--title or requirement.md)"

# Build PR body from artifacts if not provided
if [[ -z "$BODY" ]]; then
  BODY="## Requirement\n"
  [[ -f "$PIPELINE_DIR/requirement.md" ]] && BODY+="$(cat "$PIPELINE_DIR/requirement.md")\n\n"
  [[ -f "$PIPELINE_DIR/requirement-analysis.md" ]] && BODY+="## Analysis\n$(cat "$PIPELINE_DIR/requirement-analysis.md")\n\n"
fi

# Create PR
PR_URL=$(gh pr create --title "$TITLE" --body "$(echo -e "$BODY")" $DRAFT 2>&1)
PR_NUMBER=$(echo "$PR_URL" | grep -oE '[0-9]+$' || echo "")

OUTPUT=$(jq -cn \
  --arg url "$PR_URL" \
  --arg number "$PR_NUMBER" \
  --arg title "$TITLE" \
  '{url: $url, number: $number, title: $title}')

if [[ -d "$PIPELINE_DIR" ]]; then
  echo "$OUTPUT" > "$PIPELINE_DIR/pr.json"
fi

echo "$OUTPUT" | jq .
