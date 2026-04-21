---
description: "Pull Request 생성"
---

# PR Creation

검증 완료 후 Pull Request를 생성합니다.

## 입력
- `$ARGUMENTS`: 파이프라인 디렉토리 경로

## 실행

1. `requirement-analysis.md`, `architecture.md`, `verification.json`을 읽습니다
2. PR 제목과 본문을 작성합니다
3. `status.json`에서 브랜치명을 읽어 PR을 생성합니다:

```bash
BRANCH=$(jq -r '.branch' "$ARGUMENTS/status.json")
.pylon/scripts/bash/create-pr.sh "$ARGUMENTS" --branch "$BRANCH" --title "feat: [요약]" --body "[본문]"
```

## 산출물
`$PIPELINE_DIR/pr.json`
