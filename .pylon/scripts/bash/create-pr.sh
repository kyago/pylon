#!/bin/bash
set -euo pipefail
source "$(dirname "$0")/common.sh"

require_cmd git gh jq

PIPELINE_DIR="${1:?Usage: create-pr.sh <pipeline-dir> [--branch <branch>] [--title <title>] [--body <body>] [--draft]}"
shift

BRANCH=""
TITLE=""
BODY=""
DRAFT=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --branch) BRANCH="$2"; shift 2 ;;
    --title)  TITLE="$2";  shift 2 ;;
    --body)   BODY="$2";   shift 2 ;;
    --draft)  DRAFT="--draft"; shift ;;
    *) shift ;;
  esac
done

# Switch to the specified branch if provided and not already on it
CURRENT_BRANCH=$(git branch --show-current)

if [[ -n "$BRANCH" && "$CURRENT_BRANCH" != "$BRANCH" ]]; then
  if git show-ref --verify --quiet "refs/heads/$BRANCH"; then
    git checkout "$BRANCH" || die "브랜치 전환 실패: $BRANCH"
  else
    die "브랜치 '$BRANCH'가 존재하지 않습니다. init-pipeline.sh를 먼저 실행하세요."
  fi
  CURRENT_BRANCH="$BRANCH"
fi

# Guard: cannot create PR from a protected branch
if is_protected_branch "$CURRENT_BRANCH"; then
  die "protected branch '$CURRENT_BRANCH'에서 PR을 생성할 수 없습니다. --branch <브랜치명>을 지정하세요."
fi

# Push branch to remote if not already pushed
if ! git ls-remote --exit-code --heads origin "$CURRENT_BRANCH" &>/dev/null; then
  git push -u origin "$CURRENT_BRANCH" || die "브랜치 push 실패: $CURRENT_BRANCH"
fi

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
PR_URL=$(gh pr create --title "$TITLE" --body "$(printf '%b' "$BODY")" ${DRAFT:+"$DRAFT"})
PR_NUMBER=$(basename "$PR_URL")

OUTPUT=$(jq -cn \
  --arg url "$PR_URL" \
  --arg number "$PR_NUMBER" \
  --arg title "$TITLE" \
  '{url: $url, number: $number, title: $title}')

if [[ -d "$PIPELINE_DIR" ]]; then
  echo "$OUTPUT" > "$PIPELINE_DIR/pr.json"
fi

echo "$OUTPUT" | jq .
