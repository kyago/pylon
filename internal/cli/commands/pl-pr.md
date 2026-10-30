---
description: "Pull Request 생성"
---

# PR Creation

검증 완료 후 Pull Request를 생성합니다.

## 입력
- `$ARGUMENTS`: 파이프라인 디렉토리 경로

## 실행

1. `requirement-analysis.md`, `architecture.md`, `verification.json`을 읽습니다
2. 검증한 코드가 PR에 올라갈 브랜치 코드와 같은지 확인합니다. 검증은 커밋 전 작업 트리에서도 돌기 때문에 커밋이
   아니라 tree로 대조합니다:

   ```bash
   REPO=$(jq -r '.repo' "$ARGUMENTS/status.json")
   BRANCH=$(jq -r '.branch' "$ARGUMENTS/status.json")
   jq -r '.tree // empty' "$ARGUMENTS/verification.json"
   git -C "$REPO" rev-parse "$BRANCH^{tree}"
   git -C "$REPO" status --porcelain -- ':(exclude).pylon'
   ```

   - 두 tree가 같으면 진행합니다.
   - 다르고 커밋되지 않은 변경이 있으면 커밋을 안내하고 멈춥니다. 자동 커밋하지 않습니다.
   - 다르고 커밋되지 않은 변경이 없으면(검증 뒤 커밋이 추가됨, `tree`가 없는 이전 버전 결과, 검증 중 파일을 고친
     단계) 다시 검증하고 통과해야 진행합니다:
     `.pylon/builtin/scripts/bash/run-verification.sh "$ARGUMENTS" --git-root "$REPO"`
3. PR 제목과 본문을 작성합니다
4. PR을 생성합니다:

```bash
.pylon/builtin/scripts/bash/create-pr.sh "$ARGUMENTS" --git-root "$REPO" --branch "$BRANCH" --title "feat: [요약]" --body "[본문]"
```

## 산출물
`$PIPELINE_DIR/pr.json`
