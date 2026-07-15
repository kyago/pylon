---
description: "Pylon 파이프라인 실행 — 요구사항 → 분석 → 설계 → 구현 → PR (다중 repo 지원)"
handoffs:
  - label: 아키텍처 분석만
    command: pl-architect
  - label: 태스크 분해만
    command: pl-breakdown
  - label: 에이전트 실행만
    command: pl-execute
  - label: 검증만
    command: pl-verify
  - label: PR 생성만
    command: pl-pr
---

# Pylon Pipeline — 전체 워크플로우 실행

사용자의 요구사항을 받아 분석 → 설계 → 구현 → PR까지 전체 파이프라인을 실행합니다.
단일 repo와 다중 repo 워크스페이스 모두 지원합니다.

## 실행 단계

### Step 1: 파이프라인 초기화

PIPELINE_DIR와 status.json만 생성합니다. 브랜치 생성은 Step 5의 repo-Agent가 담당합니다.

```bash
INIT_RESULT=$(.pylon/scripts/bash/init-pipeline.sh "$ARGUMENTS")
PIPELINE_ID=$(echo "$INIT_RESULT" | jq -r '.pipeline_id')
PIPELINE_DIR=$(echo "$INIT_RESULT" | jq -r '.pipeline_dir')
```

`$INIT_RESULT`에서 `pipeline_id`, `pipeline_dir`을 추출합니다.
(`branch` 필드는 루트 모드에서 출력되지 않습니다.)

### Step 2: PO 요구사항 분석

Claude Code가 직접 PO 역할을 수행합니다.

1. `$PIPELINE_DIR/requirement.md`를 읽습니다
2. 요구사항을 분석하여 다음을 포함하는 `requirement-analysis.md`를 작성합니다:
   - 사용자 스토리 (As a... I want... So that...)
   - 수용 기준 (Acceptance Criteria)
   - 기능적/비기능적 요구사항 구분
   - **다중 repo 변경 가능성** — 요구사항이 여러 repo에 걸쳐 있는지 판단
   - 범위 밖 (Out of Scope) 항목
3. `$PIPELINE_DIR/requirement-analysis.md`에 저장합니다

### Step 3: 아키텍처 분석

`.pylon/agents/architect.md`를 읽어 에이전트 정의를 가져온 뒤 Agent 도구로 실행합니다.

```
// 1. 에이전트 정의 로드
ARCHITECT_DEF=$(cat .pylon/agents/architect.md)

// 2. 아키텍트 에이전트 실행
Agent(prompt="$ARCHITECT_DEF\n\n## 태스크\n다음 요구사항을 분석하고 아키텍처 설계를 작성하세요: [requirement-analysis.md 내용]\n코드베이스 구조를 파악하고 영향 받는 파일, 변경 사항, 새로 생성할 파일을 명시하세요.\n**반드시 ## Affected Repositories 섹션을 포함하세요.**\n결과를 $PIPELINE_DIR/architecture.md에 저장하세요.")
```

에이전트 정의는 `.pylon/agents/architect.md`에서 읽어 프롬프트에 주입합니다.

`architecture.md`에는 반드시 `## Affected Repositories` 섹션이 포함되어야 합니다:
```markdown
## Affected Repositories
- `services/service-a`: REST API 변경으로 엔드포인트 추가 필요
- `services/service-b`: service-a 클라이언트 업데이트 필요
```
단일 repo인 경우: `- ".": [변경 이유]`

### Step 4: 사전조건 검증

```bash
.pylon/scripts/bash/check-prerequisites.sh \
  --pipeline-dir "$PIPELINE_DIR" \
  --require-requirement \
  --require-architecture \
  --require-analysis
```

실패 시 누락된 산출물을 재생성합니다.

### Step 5: PM 태스크 분해 + repo-Agent 병렬 스폰

**태스크 생성:**

1. `architecture.md`의 `## Affected Repositories` 섹션을 파싱하여 repo 목록을 추출하고 `AFFECTED_REPOS` 배열을 구성합니다:

```bash
AFFECTED_REPOS=()
while IFS= read -r line; do
  [[ "$line" =~ ^\-[[:space:]]+\`([^\`]+)\` ]] && AFFECTED_REPOS+=("${BASH_REMATCH[1]}")
done < <(awk '/^## Affected Repositories/{f=1;next} f&&/^##/{exit} f{print}' "$PIPELINE_DIR/architecture.md")
```

2. `requirement-analysis.md`와 `architecture.md`를 기반으로 태스크를 분해합니다
3. 각 태스크에 `repo` 필드를 포함합니다 (REPO_ROOT 기준 상대경로, 단일 repo는 `"."`)
4. `$PIPELINE_DIR/tasks.json` 형식:

```json
{
  "tasks": [
    {
      "id": "T001",
      "title": "서비스 A API 엔드포인트 추가",
      "description": "POST /users 엔드포인트 구현",
      "agent": "backend-dev",
      "repo": "services/service-a",
      "dependencies": []
    },
    {
      "id": "T002",
      "title": "서비스 B 클라이언트 업데이트",
      "description": "service-a의 새 API 호출 클라이언트 추가",
      "agent": "backend-dev",
      "repo": "services/service-b",
      "dependencies": []
    }
  ]
}
```

**루트 status.json 업데이트 (sub_pipelines 초기화):**

PM이 `AFFECTED_REPOS` 목록을 기반으로 루트 `status.json`의 `sub_pipelines` 배열을 채웁니다:

```bash
# AFFECTED_REPOS의 각 repo에 대해 sub_pipelines 항목 추가
for REPO in "${AFFECTED_REPOS[@]}"; do
  REPO_BASENAME=$(basename "$(realpath "$REPO_ROOT/$REPO")")
  jq --arg repo "$REPO" \
     --arg dir "$PIPELINE_DIR/$REPO_BASENAME" \
     '.sub_pipelines += [{repo: $repo, pipeline_dir: $dir, status: "running"}]' \
     "$PIPELINE_DIR/status.json" > "$PIPELINE_DIR/status.json.tmp" \
     && mv "$PIPELINE_DIR/status.json.tmp" "$PIPELINE_DIR/status.json"
done
```

**계획 이력 체크포인트:**

```bash
pylon history checkpoint \
  --pipeline "$(basename "$PIPELINE_DIR")" \
  --phase planned
```

**repo-Agent 병렬 스폰:**

`AFFECTED_REPOS`의 각 repo에 대해 아래 템플릿을 인스턴스화하여 **단일 응답에서 모두 병렬로** 스폰합니다.
`{REPO}`, `{REPO_BASENAME}` 자리를 실제 값으로 치환합니다.
(`{REPO_BASENAME}` = `basename "$(realpath "$REPO_ROOT/{REPO}")"` — 단일 repo(`"."`)일 때도 실제 디렉토리명을 반환합니다.)

```
// 템플릿: AFFECTED_REPOS의 각 {REPO}에 대해 아래를 반복 스폰 (단일 응답, 병렬 실행)
Agent(
  description="repo-Agent: {REPO}",
  prompt="""
## repo-Agent 지시사항

대상 repo: {REPO} (basename: {REPO_BASENAME})

다음 순서로 실행하세요:

### a. 서브파이프라인 초기화
INIT_RESULT=$(.pylon/scripts/bash/init-pipeline.sh "$REQUIREMENT" \
  --git-root {REPO} \
  --pipeline-dir $PIPELINE_DIR)
SUB_PIPELINE_DIR=$(echo "$INIT_RESULT" | jq -r '.pipeline_dir')
BRANCH=$(echo "$INIT_RESULT" | jq -r '.branch')

### b. 해당 repo 태스크 필터링
$PIPELINE_DIR/tasks.json에서 repo == "{REPO}" 태스크만 선택합니다.

### c. 태스크 순서대로 구현
- 담당 에이전트(`.pylon/agents/<task.agent>.md`)를 로드하여 각 태스크를 실행합니다
  (각 태스크의 `agent` 필드 값을 파일명으로 사용. 예: `backend-dev.md`)
- 의존성 없는 태스크는 병렬로 실행합니다
- 각 태스크 완료 후 코드를 커밋합니다

### d. 검증
.pylon/scripts/bash/run-verification.sh "$SUB_PIPELINE_DIR" --git-root {REPO}

### e. 결과를 sub-pipeline status.json에 기록 후 반환
성공 시:
  jq '.status = "success"' "$SUB_PIPELINE_DIR/status.json" \
    > "$SUB_PIPELINE_DIR/status.json.tmp" \
    && mv "$SUB_PIPELINE_DIR/status.json.tmp" "$SUB_PIPELINE_DIR/status.json"
  반환: {success: true, branch: "$BRANCH", pipeline_dir: "$SUB_PIPELINE_DIR"}
실패 시:
  jq --arg err "[오류 원인]" '.status = "failed" | .error = $err' \
    "$SUB_PIPELINE_DIR/status.json" \
    > "$SUB_PIPELINE_DIR/status.json.tmp" \
    && mv "$SUB_PIPELINE_DIR/status.json.tmp" "$SUB_PIPELINE_DIR/status.json"
  반환: {success: false, error: "[오류 원인]"}
"""
)
```

**구형 tasks.json 호환성**: `repo` 필드가 없는 태스크는 `"."` (REPO_ROOT)를 기본값으로 사용합니다.

**repo-Agent 결과 수집:**

모든 repo-Agent 완료 후, PM이 각 서브파이프라인의 `status.json`을 확인하여 성공/실패를 분류합니다:

```bash
SUCCESSFUL_REPOS=()
FAILED_REPOS=()
for REPO in "${AFFECTED_REPOS[@]}"; do
  REPO_BASENAME=$(basename "$(realpath "$REPO_ROOT/$REPO")")
  SUB_STATUS=$(jq -r '.status' "$PIPELINE_DIR/$REPO_BASENAME/status.json" 2>/dev/null || echo "failed")
  if [[ "$SUB_STATUS" == "success" ]]; then
    SUCCESSFUL_REPOS+=("$REPO")
  else
    FAILED_REPOS+=("$REPO")
    # 루트 status.json의 해당 sub_pipeline 항목을 "failed"로 업데이트
    jq --arg repo "$REPO" '.sub_pipelines |= map(if .repo == $repo then .status = "failed" else . end)' \
      "$PIPELINE_DIR/status.json" > "$PIPELINE_DIR/status.json.tmp" \
      && mv "$PIPELINE_DIR/status.json.tmp" "$PIPELINE_DIR/status.json"
  fi
done
```

**실행 이력 체크포인트:**

```bash
pylon history checkpoint \
  --pipeline "$(basename "$PIPELINE_DIR")" \
  --phase executed
```

### Step 6: PR 생성

성공한 repo에 대해서만 PM이 직접 per-repo PR을 생성합니다.
repo-Agent는 PR 생성을 수행하지 않습니다.

```bash
# auto_pr 설정 확인 후 성공 repo에 대해서만 PR 생성
source .pylon/scripts/bash/common.sh
AUTO_PR=$(config_get "git.pr.auto_pr" "false")

for REPO in "${SUCCESSFUL_REPOS[@]}"; do
  REPO_BASENAME=$(basename "$(realpath "$REPO_ROOT/$REPO")")
  SUB_PIPELINE_DIR="$PIPELINE_DIR/$REPO_BASENAME"
  BRANCH=$(jq -r '.branch' "$SUB_PIPELINE_DIR/status.json")

  if [[ "$AUTO_PR" == "true" ]]; then
    .pylon/scripts/bash/create-pr.sh "$SUB_PIPELINE_DIR" \
      --git-root "$REPO" \
      --branch "$BRANCH" \
      --title "feat: [요구사항 요약] ($REPO_BASENAME)"
  fi

  # PR 생성 여부와 무관하게 루트 status.json의 해당 항목을 "success"로 업데이트
  jq --arg repo "$REPO" '.sub_pipelines |= map(if .repo == $repo then .status = "success" else . end)' \
    "$PIPELINE_DIR/status.json" > "$PIPELINE_DIR/status.json.tmp" \
    && mv "$PIPELINE_DIR/status.json.tmp" "$PIPELINE_DIR/status.json"
done
```

**최종 상태 및 완료 이력 체크포인트:**

```bash
FINAL_STATUS="success"
[[ ${#FAILED_REPOS[@]} -gt 0 ]] && FINAL_STATUS="failed"
jq --arg status "$FINAL_STATUS" \
   '.stage = "done" | .status = $status' \
   "$PIPELINE_DIR/status.json" > "$PIPELINE_DIR/status.json.tmp" \
  && mv "$PIPELINE_DIR/status.json.tmp" "$PIPELINE_DIR/status.json"

pylon history checkpoint \
  --pipeline "$(basename "$PIPELINE_DIR")" \
  --phase completed
```

> `config.yml`의 `git.pr.auto_pr: true` 설정 시에만 자동 실행됩니다.
> 수동으로 PR을 생성하려면 `/pl:pr` 커맨드를 사용하세요.

### Step 7: 완료 보고

실행 결과를 repo별로 요약합니다:

**성공 repo:**
- repo 경로
- 생성된 PR URL
- 변경된 파일 목록

**실패 repo:**
- repo 경로
- 실패 원인 요약

**전체 요약:**
- 총 repo 수, 성공/실패 수
- PR URL 목록
- 총 소요 시간

## 메모리 활용

파이프라인 시작 시 `pylon mem search` CLI로 관련 도메인 지식을 검색하여
에이전트 프롬프트에 주입합니다:

```bash
pylon mem search --project <project-name> "<요구사항 키워드>"
```

## 에러 처리

- 각 단계 실패 시 에러를 분석하고 재시도합니다
- 3회 연속 실패 시 사용자에게 보고합니다
- `status.json`에 현재 단계와 에러를 기록합니다
- **일부 repo 실패**: 성공한 repo만 PR 생성, 실패 repo는 원인과 함께 보고합니다

## Crash Recovery

파이프라인 재실행 시 기존 산출물을 확인합니다:
- 이미 존재하는 산출물은 건너뜁니다
- `status.json`의 마지막 단계부터 재개합니다
- sub_pipelines 중 `status: "success"`인 항목은 재실행하지 않습니다
