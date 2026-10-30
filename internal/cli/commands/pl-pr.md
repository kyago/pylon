---
description: "Pull Request 생성"
---

# PR Creation

검증 완료 후 Pull Request를 생성합니다.

## 입력
- `$ARGUMENTS`: repo sub-pipeline 디렉토리(`<root run>/repos/<repo_id>`, root `status.json`의
  `sub_pipelines[].pipeline_dir`). 여러 repo를 바꾼 run은 repo마다 실행합니다.

## 실행

1. root run(`$ARGUMENTS/../..`)의 `requirement-analysis.md`, `architecture.md`와 `$ARGUMENTS/verification.json`을
   읽습니다
2. 검증한 코드가 PR에 올라갈 브랜치 코드와 같은지 확인합니다. 검증은 커밋 전 작업 트리에서도 돌기 때문에 커밋이
   아니라 tree로 대조합니다:

   ```bash
   REPO=$(jq -r '.repo' "$ARGUMENTS/status.json")
   BRANCH=$(jq -r '.branch' "$ARGUMENTS/status.json")
   git -C "$REPO" branch --show-current
   jq -c '{ok, tree}' "$ARGUMENTS/verification.json"
   git -C "$REPO" rev-parse "$BRANCH^{tree}"
   git -C "$REPO" status --porcelain -- ':(top)' ':(top,glob,exclude)**/.pylon/**' ':(top,exclude).claude/worktrees'
   ```

   - 현재 브랜치가 `$BRANCH`가 아니면 멈추고 알립니다. 검증과 status가 다른 브랜치를 보게 됩니다.
   - `ok`가 true이고 두 tree가 같으면 진행합니다.
   - 커밋되지 않은 변경이 있으면 커밋을 안내하고 멈춥니다. 자동 커밋하지 않습니다. 커밋하지 않을 파일(검증이 만든
     커버리지 파일, 개인 메모 등)이면 커밋 대신 `.git/info/exclude`·`.gitignore` 추가나 삭제를 안내합니다 — tree는
     무시되지 않은 파일을 모두 담으므로 그대로 두면 일치하지 않습니다.
   - 그 밖에는(검증 실패, 검증 뒤 커밋이 추가됨, `tree`가 없는 결과) 다시 검증하고, 새 결과의 `ok`가 true이고
     `tree`가 `$BRANCH^{tree}`와 같을 때만 진행합니다. 그래도 맞지 않으면 멈추고 이유를 보고합니다:
     `.pylon/builtin/scripts/bash/run-verification.sh "$ARGUMENTS" --git-root "$REPO"`
3. PR 제목과 본문을 작성합니다
4. PR을 생성합니다:

```bash
.pylon/builtin/scripts/bash/create-pr.sh "$ARGUMENTS" --git-root "$REPO" --branch "$BRANCH" --title "feat: [요약]" --body "[본문]"
```

## 산출물
`$PIPELINE_DIR/pr.json`
