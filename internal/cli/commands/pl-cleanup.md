---
description: "세션 마무리 — 파이프라인 사용 여부와 무관. 검증 근거 정리, 메모리·도메인 지식 갱신, 파이프라인 run 마감, 머지된 브랜치·워크트리 정리"
---

# Pylon Cleanup — 세션 마무리

이번 세션의 작업을 마무리합니다. 대화로만 진행했든 `/pl:pipeline`으로 시작했든 같은 명령으로 끝냅니다.
보통 PR이 머지된 뒤 실행되므로 작업 트리가 깨끗해도 일이 없었던 것이 아닙니다 — 이번 세션에 한 일은
대화 기록, PR, 파이프라인 산출물에서 찾습니다.

단계는 순서대로 진행합니다. 메모리와 도메인 지식은 파이프라인 산출물을 읽어 만들므로 run 정리(삭제)보다
먼저 끝내야 하고, 브랜치는 머지가 확인된 뒤에만 지웁니다. 이미 없어진 브랜치·워크트리·run은 "이미 정리됨"으로
보고하고 넘어갑니다.

## Step 1: 세션 범위 파악

각 repo에서 먼저 `git fetch -q origin`을 실행한 뒤, 확인한 근거로만 정리합니다:
- 대화에서 커밋·PR·이슈를 만든 repo와 번호·URL, 사용한 브랜치·워크트리
- 미커밋 변경(`git status`)과 미푸시 커밋(upstream이 있으면 `git log @{u}..HEAD`, 없으면 `git log origin/<base>..HEAD`)
- `.pylon/runtime/` 아래의 run 전부: root run(`status.json`의 `scope`가 `root`), repo-scope run, `status.json`이 없는
  디렉토리. root run 안의 `repos/<id>/`는 그 run의 일부이고, `pylon init`이 만드는 `memory/`·`sessions/`는 run이
  아닙니다. 대화에 pipeline id가 나온 run이 **이번 세션의 run**입니다. `status`가 `awaiting_cleanup`이면
  `/pl:pipeline`이 끝나 마감을 기다리는 run입니다

메모리는 프로젝트별로 저장되므로 대상은 정식 프로젝트 이름(`/pl:project-list`, `.pylon/config.yml`의
`projects`)을 그대로 씁니다. 다른 이름으로 저장하면 `pylon mem search --project <정식이름>`에 잡히지 않습니다.
- 세션이 여러 프로젝트를 다뤘으면 항목마다 해당 프로젝트에 나눠 저장합니다.
- 정식 프로젝트가 없으면(단일 repo 워크스페이스) 워크스페이스 디렉토리 이름을 쓰고 보고에 밝힙니다.
- 근거가 전혀 없을 때만 사용자에게 대상을 묻습니다.

## Step 2: 검증과 결과 확인

검증은 아직 검증되지 않은 변경에만 실행합니다. 머지된 base 브랜치에서 다시 돌리는 것은 이번 변경에 대해
아무것도 증명하지 않습니다.

- 미커밋·미푸시 변경이 있는 repo(워크트리 포함)는 그 루트에서 실행합니다. `.pylon/`은 git에서 제외되어
  워크트리에는 `verify.yml`이 없으므로, 설정은 그 프로젝트 메인 체크아웃의 것을 씁니다:

  ```bash
  pylon internal verify --workdir <git-root> --config <project>/.pylon/verify.yml
  ```

  - `"skipped": true` — `verify.yml`도 `go.mod`도 없어 검증이 **미설정**인 경우(종료 코드 1과 함께 나옵니다).
    실패가 아니므로 "검증 미설정"으로 보고하고 계속합니다.
  - `skipped` 없이 `"ok": false` + `checks` — 실제 검증이 돌아 실패한 경우입니다. `verify.yml`이 없어도 `go.mod`가
    있으면 기본 `go build/vet/test`가 돌므로 **진짜 실패**입니다. 실패 항목을 보고하고, 그 repo의 run 마감과
    브랜치 정리는 하지 않습니다.
- 이미 PR로 머지된 변경은 다음을 근거로 씁니다:
  - 이 대화에서 돌린 검증 출력. 검증을 돌린 커밋이 머지된 PR의 head(`gh pr view <번호> --json headRefOid`)와 같을
    때만 근거가 됩니다.
  - PR의 체크 결과(`gh pr checks <번호>`). "no checks reported"는 실패가 아니라 "PR 체크 없음"으로 보고합니다.
  - 파이프라인 run의 `verification.json`·`evaluator-result.json`. 그 뒤에 바뀐 코드는 위 두 근거로 확인합니다.

세션의 결과물마다 확인한 것과 확인하지 않은 것을 나눕니다. 배포 결과, 마이그레이션 적용, 운영 데이터 반영처럼
코드 밖의 결과는 읽기 전용으로 확인할 수단(배포 run 로그, 조회 쿼리, 상태 API)이 있으면 실제로 조회해 확인하고,
수단이 없으면 "미확인"으로 남깁니다. Step 8 보고에 그대로 씁니다.

## Step 3: 메모리 저장

다음 세션이 모르면 시간을 쓰게 될 것만 저장합니다:
- 방향을 정한 결정과 근거 → `decision`
- 이번에 시간을 쓴 함정·교훈 → `learning`
- 재사용할 패턴 → `pattern`

코드·커밋·PR 본문에서 바로 알 수 있는 사실과 변경 파일 목록 같은 이력은 저장하지 않습니다(검색을 오염시킵니다).
파이프라인 run이 있으면 `requirement-analysis.md`, `architecture.md`, 리뷰·검증 산출물에서 결정 근거를 가져옵니다.

먼저 기존 항목을 확인합니다 — 세션 컨텍스트의 "pylon 프로젝트 메모리" 목록과 검색:

```bash
pylon mem search --project <프로젝트> --query "<주제 키워드>"
```

같은 주제가 있으면 **같은 `--category`와 `--key`**로 저장해 갱신합니다(갱신은 카테고리와 key가 모두 같아야
일어납니다). 본문 첫 줄은 한 줄 요약으로 씁니다 — 인덱스와 세션 주입에 그 줄이 보입니다.

```bash
pylon mem store --project <프로젝트> --category decision --key <슬러그> --content "결정 요약
근거와 맥락"
```

저장할 것이 없으면 건너뜁니다.

## Step 4: 도메인 지식 갱신

세션이 바꾼 사실이 `.pylon/domain/*.md`나 `<project>/.pylon/context.md`에 적혀 있으면(스펙 버전, 테이블·필드
목록, 흐름, 운영 절차) 머지된 코드에서 확인한 값으로 그 줄을 고칩니다. 새 사실이 그 문서가 다루는 범위면
추가합니다. 코드에서 확인하지 않은 내용은 적지 않습니다. 아직 머지 전이면 건너뛰고 "머지 후 갱신 필요"로
보고합니다.

## Step 5: 파이프라인 run 마감

**대상.** 이번 세션의 run만 자동으로 마감합니다. 다른 run은 상태(`status`, terminal 체크포인트 유무, 머지 여부)를
목록으로 보고하고, 사용자가 마감을 요청할 때 같은 절차로 마감합니다. `failed` run은 조사용으로 보존된 것이므로
사용자가 요청할 때만 정리합니다. repo-scope run과 `status.json`이 없는 디렉토리도 목록에 넣습니다.

**마감 조건.** 하나라도 맞지 않으면 run을 그대로 두고 이유를 보고합니다(머지 전이면 "머지 후 다시 `/pl:cleanup`").
- repo가 있는 run: 모든 `sub_pipelines[]`의 작업이 머지되었습니다(판정은 Step 6의 머지 확인 규칙). sub의 `branch`에
  `base_revision` 이후 커밋이 없으면(`git rev-list --count <base_revision>..<branch>`가 0) 대화에서 그 repo에 쓴
  브랜치나 PR head로 판정합니다. 대화에도 그 repo에 연결된 브랜치·PR이 없으면 그 sub는 "변경 없음"이며 마감을
  막지 않습니다. sub의 `repo`가 이미 지운 워크트리 경로면 그 워크트리를 만든 프로젝트 repo(대화 기록, 워크트리
  이름)에서 판정합니다. 판정할 repo를 찾지 못하면 run을 남깁니다.
- 사용자가 정리를 요청한 포기 run(`PHASE=cancelled`)과 실패 run(`PHASE=failed`)은 머지 조건을 적용하지 않습니다.
  머지되지 않은 브랜치는 Step 6에서 지우지 않고 보고에 남깁니다.
- 그 sub의 워크트리(`git worktree list`에서 run 브랜치에 있는 것)에 미커밋·추적 안 된 파일이 없습니다(`git status
  --porcelain`). 정리 스크립트도 그런 워크트리는 지우지 않고 출력의 `kept`에 남깁니다. `kept`가 있으면 보고합니다.
- repo가 없는 비소프트웨어 run: 체크포인트는 `final.md`·`seo.md` 같은 산출물을 보존하지 않으므로, 사용자가 정한
  위치에 저장했거나 커밋한 것을 확인했을 때만 마감합니다. 확인하지 못하면 runtime 경로를 알려 주고 남깁니다.

**정리 전 준비.**
- 각 sub의 `repo`, `branch`, `base_revision`(root `status.json`)과 `base_branch`(sub의 `status.json`)를 기록해 둡니다.
  정리하면 `status.json`이 사라지는데 Step 6에 필요합니다.
- 메인 체크아웃이 run 브랜치에 있고(`init-pipeline.sh`가 task 브랜치를 체크아웃한다) 미커밋 변경이 없으면 base
  브랜치로 전환하고 `git pull --ff-only`로 맞춥니다.

**절차.** 정리는 terminal 체크포인트 → 정리 순서만 허용됩니다. 체크포인트가 실패하면 아무것도 지우지 않습니다.
마감할 run마다 변수를 정합니다:

```bash
ROOT_PIPELINE_DIR=.pylon/runtime/<pipeline-id>
PIPELINE_ID=$(basename "$ROOT_PIPELINE_DIR")
PHASE=completed   # 이미 terminal 체크포인트가 있는 run은 그 phase, 포기한 run은 cancelled, 실패 run은 failed
```

`awaiting_cleanup` run이나 사용자가 포기한 run은 상태를 바꾼 뒤 체크포인트를 만듭니다. 같은 phase를 다시
만들면 최신 상태로 교체됩니다. history에 그 phase의 체크포인트가 이미 있는 run(실패 run은 실패 기록 때 `failed`
체크포인트가 만들어진다)은 이 블록을 건너뜁니다:

```bash
STATUS_TMP=$(mktemp "$ROOT_PIPELINE_DIR/.status.XXXXXX")
jq --arg phase "$PHASE" '.stage = $phase | .status = $phase' "$ROOT_PIPELINE_DIR/status.json" > "$STATUS_TMP"
mv "$STATUS_TMP" "$ROOT_PIPELINE_DIR/status.json"
pylon history checkpoint --pipeline "$PIPELINE_ID" --phase "$PHASE"
```

체크포인트가 성공하면 repo sub-pipeline, root runtime 순으로 정리합니다. sub의 `repo` 경로가 이미 없으면(워크트리를
먼저 지운 경우) 그 sub는 건너뜁니다 — sub-pipeline 디렉토리는 root runtime 안에 있어 root 정리가 함께 지웁니다:

```bash
jq -c '.sub_pipelines[]?' "$ROOT_PIPELINE_DIR/status.json" | while IFS= read -r SUB; do
  REPO=$(echo "$SUB" | jq -r '.repo')
  REPO_PIPELINE_DIR=$(echo "$SUB" | jq -r '.pipeline_dir')
  [[ -d "$REPO" ]] || continue
  .pylon/builtin/scripts/bash/cleanup-pipeline.sh "$REPO_PIPELINE_DIR" --terminal-phase "$PHASE"
done
.pylon/builtin/scripts/bash/cleanup-pipeline.sh "$ROOT_PIPELINE_DIR" --terminal-phase "$PHASE"
```

## Step 6: 머지된 브랜치·워크트리 정리

이번 세션이 만들거나 작업한 브랜치·워크트리와, Step 5에서 마감한 run의 브랜치(기록해 둔 `branch`)만 다룹니다.
그 밖의 브랜치는 후보로만 보고합니다.

**머지 확인 규칙**(Step 5도 같은 규칙을 씁니다). 다음 중 하나여야 합니다:
- run 브랜치처럼 `base_revision`이 기록된 브랜치: `git rev-list --count <base_revision>..<branch>`가 1 이상이고
  `git merge-base --is-ancestor <branch> origin/<base>`가 성공합니다(커밋이 있는 브랜치가 base에 들어감).
- PR 머지(머지 방식 무관): `gh pr list --head <branch> --state merged --json number,headRefOid`의 PR 중 `headRefOid`가
  지금 브랜치 끝(`git rev-parse <branch>`, 로컬이 없으면 `origin/<branch>`)과 같고, 같은 head로 열린 PR이 없습니다.
  이름만 같고 커밋이 다르면 머지되지 않은 것으로 봅니다. `base_revision`이 없는 브랜치(대화에서 만든 브랜치)는 이
  규칙으로만 판정합니다.

`base_revision` 이후 커밋이 없는 run 브랜치(작업을 다른 브랜치에서 한 경우)는 잃을 커밋이 없으므로 머지 확인 없이
지웁니다.

**정리.**
- 워크트리: `git worktree remove <path>` — `--force`를 쓰지 않습니다. 미커밋 변경이 있으면 git이 거부하고, 그
  워크트리와 브랜치는 남깁니다.
- 브랜치가 메인 체크아웃에 체크아웃되어 있으면 Step 5의 준비처럼 base로 전환한 뒤 지웁니다.
- 로컬 브랜치: `git branch -D <branch>`.
- 원격 브랜치: `git ls-remote --heads origin <branch>`에 있고, `origin/<branch>`도 머지 확인 규칙을 통과할 때만
  `git push origin --delete <branch>`. 원격에 로컬보다 앞선 커밋이 있으면 남깁니다.
- 기본 브랜치(`git.default_base`, 원격 기본 브랜치 `git symbolic-ref refs/remotes/origin/HEAD`)와 보호 브랜치(`main`,
  `master`, `develop`, `dev`)는 건드리지 않습니다. 열린 PR의 base인 브랜치(`gh pr list --base <branch> --state open`이
  비어 있지 않음)도 지우지 않습니다.

머지되지 않았거나 머지를 확인할 수 없는 브랜치는 남기고 이유를 보고합니다.

## Step 7: 커밋 안내

워크스페이스가 git 저장소이고 `.pylon/memory/`, `.pylon/domain/`에 커밋되지 않은 변경이 있거나, 프로젝트에 커밋되지
않은 변경이 남았으면 무엇이 남았는지 알리고 커밋을 안내합니다. 자동 커밋하지 않습니다.

만료 메모리 정리(`memory.retention_days`)는 세션 시작 시 `pylon mem inject`가 수행합니다(Claude Code 훅;
Codex는 수동 `pylon mem inject`) — 여기서 호출하지 않습니다.

## Step 8: 완료 보고

- 결과 확인: 확인한 것(근거)과 미확인 항목
- 저장·갱신한 메모리(프로젝트 · 카테고리 · key, 신규/갱신)
- 고친 도메인 문서와 바뀐 사실, 또는 머지 후로 미룬 것
- 파이프라인 run: 마감한 것(id · phase), 남긴 것(이유), 다른 run 목록과 마감 가능 여부
- 브랜치·워크트리: 지운 것, 남긴 것(이유)
- 커밋 안내 여부
