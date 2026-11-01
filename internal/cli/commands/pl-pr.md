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
2. 검증한 코드가 PR에 올라갈 브랜치 코드와 같은지 확인합니다. `verification.json`의 `tree`는 깨끗한 작업 트리에서
   검증한 HEAD의 tree입니다(미커밋 변경이 있으면 기록되지 않고 `tree_reason`에 이유가 남습니다). 커밋이 아니라
   tree로 대조해야 rebase·merge 뒤에도 같은 코드면 일치합니다:

   ```bash
   REPO=$(jq -r '.repo' "$ARGUMENTS/status.json")
   BRANCH=$(jq -r '.branch' "$ARGUMENTS/status.json")
   git -C "$REPO" branch --show-current
   jq -c '{ok, tree, tree_reason}' "$ARGUMENTS/verification.json"
   git -C "$REPO" rev-parse "$BRANCH^{tree}"
   git -C "$REPO" status --porcelain --untracked-files=normal -- ':(top)' ':(top,glob,exclude)**/.pylon/**' ':(top,exclude).claude/worktrees'
   ```

   - 현재 브랜치가 `$BRANCH`가 아니면 멈추고 알립니다. 검증과 status가 다른 브랜치를 보게 됩니다.
   - status에 추적 중인 파일의 변경(`??`가 아닌 줄)이 있으면 커밋을 안내하고 멈춥니다. 자동 커밋하지 않습니다.
   - `ok`가 true이고 두 tree가 같으면 진행합니다. `??` 줄(추적 안 된 파일)은 PR에 들어가지 않는다고 보고에
     알립니다 — 검증은 그 파일 없이 커밋된 코드로 돌았습니다.
   - 그 밖에는(검증 실패, 검증 뒤 커밋이 추가됨, `tree`가 없는 결과) 다시 검증하고, 새 결과의 `ok`가 true이고
     `tree`가 `$BRANCH^{tree}`와 같을 때만 진행합니다. 검증은 추적 안 된 파일이 없을 때만 `tree`를 남기므로 `??`
     파일은 먼저 커밋하거나, 커밋하지 않을 파일이면 경로째 run 폴더로 옮기고 출력의 `moved` 목록을 보고에 남깁니다
     (`pylon internal move-untracked --workdir "$REPO" --to "$ARGUMENTS/leftovers" -- <파일>...` — 지정한 파일만
     옮기며 사용자가 남긴 파일은 지정하지 않습니다. 검증 명령이 만든 파일은 검증이
     `verify-artifacts/`로 직접 옮깁니다). 그래도 맞지
     않으면 `tree_reason`과 함께 멈추고 보고합니다:
     `.pylon/builtin/scripts/bash/run-verification.sh "$ARGUMENTS" --git-root "$REPO"`
3. PR 제목과 본문을 작성합니다
4. PR을 생성합니다. `create-pr.sh`는 브랜치를 항상 push하므로 PR head는 2단계에서 대조한 로컬 브랜치입니다 — 원격이
   앞서 있으면 push가 거부되고 멈춥니다(강제 push하지 않습니다). 그때는 원격 커밋을 받아 2단계부터 다시 합니다:

```bash
.pylon/builtin/scripts/bash/create-pr.sh "$ARGUMENTS" --git-root "$REPO" --branch "$BRANCH" --title "feat: [요약]" --body "[본문]"
```

## 산출물
`$PIPELINE_DIR/pr.json`
