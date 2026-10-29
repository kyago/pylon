---
description: "Pylon 파이프라인 실행 — 요구사항 → 분석 → 설계 → 구현 → 검증"
---

# Pylon Pipeline — 전체 워크플로우 실행

요구사항에 맞는 워크플로우를 골라 파이프라인을 실행합니다.

## 워크플로우 선택

| 도메인 | 워크플로우 | 파이프라인 |
|--------|-----------|-----------|
| 소프트웨어 개발 | feature/bugfix/hotfix | 요구사항 분석 → architect → 태스크 분해 → dev → 검증 → PR |
| 리서치·콘텐츠·마케팅 | research/content/marketing | 조사 → (strategist) → writer → 독립 검토 → 수정 루프 → 최종본 |

## 실행 단계

### Step 1: 논리 파이프라인 초기화

root pipeline은 요구사항과 repo 목록을 묶는 논리 run이며 `branch`가 없습니다(전역 branch를 만들거나
`.branch`를 읽지 않습니다).

이 명령에 붙인 인자(비어 있을 수 있음): $ARGUMENTS

요구사항은 보통 대화에서 이미 정해진 뒤 시작됩니다. 인자와 대화에서 확정된 내용 — 이슈 번호·링크와 본문, 사용자가
답한 결정, 범위 밖으로 정한 것 — 을 합쳐 요구사항 본문을 정리합니다. 본문은 따옴표·백틱·`$`가 들어 있어도 그대로
남도록 따옴표 친 heredoc으로 `requirement.md`에 쓰고, 초기화 스크립트에는 짧은 제목만 넘깁니다. 제목은 run id에
들어가므로 영문 소문자·숫자·하이픈만 씁니다:

```bash
TITLE='<짧은 제목, 예: api-123-add-group-code>'
INIT_RESULT=$(.pylon/builtin/scripts/bash/init-pipeline.sh "$TITLE")
PIPELINE_ID=$(echo "$INIT_RESULT" | jq -r '.pipeline_id')
ROOT_PIPELINE_DIR=$(echo "$INIT_RESULT" | jq -r '.pipeline_dir')
PIPELINE_DIR="$ROOT_PIPELINE_DIR"
cat > "$ROOT_PIPELINE_DIR/requirement.md" <<'REQUIREMENT_EOF'
<요구사항 본문>
REQUIREMENT_EOF
```

### Step 2: 요구사항 분석 + 도메인 라우팅

루트 세션이 직접 PO 역할을 수행합니다.

1. `$PIPELINE_DIR/requirement.md`를 읽고, 세션 컨텍스트의 "pylon 프로젝트 메모리" 목록과
   `pylon mem search --project <프로젝트> --query "<요구사항 키워드>"`로 관련 결정·학습을 확인합니다. 찾은 내용은
   분석과 에이전트 프롬프트에 반영합니다
2. 도메인을 판단해 `$PIPELINE_DIR/routing-decision.json`을 작성합니다:
   ```json
   {"detected_domain": "software|research|content|marketing",
    "selected_workflow": "feature|bugfix|hotfix|research|content|marketing",
    "reasoning": "선택 근거", "agents": ["사용할 에이전트"]}
   ```
3. 사용자 스토리, 수용 기준, 기능·비기능 요구사항, 범위 밖 항목을 담아 `$PIPELINE_DIR/requirement-analysis.md`를
   작성합니다
4. 수용 기준을 모델 독립 JSON으로 `$PIPELINE_DIR/acceptance-criteria.json`에 정규화합니다:
   ```json
   {"criteria": [{"id": "AC-1", "description": "검사 가능한 완료 조건"}]}
   ```
   ID와 설명이 비어 있거나 ID가 중복되면 다음 단계로 진행하지 않습니다.
5. `detected_domain`에 따라 실행 절차를 따릅니다:
   - `software` → **소프트웨어 파이프라인** (Steps 3-9)
   - `research`, `content`, `marketing` → **비소프트웨어 파이프라인** (Steps N1-N5)

Claude Code에서는 `.claude/agents/`에 등록된 정의를 `subagent_type`으로 호출한다. 그 외 provider에서는 `.pylon/builtin/agents/<name>.md` 본문을 프롬프트 앞에 붙인다. 사용자가 추가한 에이전트는 `.pylon/agents/<name>.md`에 있다.

---

## 소프트웨어 파이프라인 (detected_domain: software)

### Step 3: 아키텍처 분석

```
Agent(subagent_type="architect", prompt="## 태스크\n다음 요구사항의 아키텍처 설계를 작성하세요: [requirement-analysis.md 내용]\n설계는 $PIPELINE_DIR/architecture.md에, 변경 대상 repo 목록은 $PIPELINE_DIR/repos.json에 저장하세요.")
```

### Step 3B: repo sub-pipeline 초기화

`repos.json`의 repo마다 독립 sub-pipeline을 만듭니다. branch, base revision, worktree, verification,
PR은 이 repo sub-pipeline만 소유합니다.

```bash
jq -r '.repos[].path' "$ROOT_PIPELINE_DIR/repos.json" | while IFS= read -r REPO; do
  SUB_RESULT=$(.pylon/builtin/scripts/bash/init-pipeline.sh "$PIPELINE_ID" \
    --git-root "$REPO" \
    --pipeline-dir "$ROOT_PIPELINE_DIR")
  REPO_ID=$(echo "$SUB_RESULT" | jq -r '.repo_id')
  BASE_REVISION=$(echo "$SUB_RESULT" | jq -r '.base_revision')
  REPO_PIPELINE_DIR=$(echo "$SUB_RESULT" | jq -r '.pipeline_dir')
  pylon internal criteria snapshot \
    --run-id "$PIPELINE_ID" \
    --repo-id "$REPO_ID" \
    --base-revision "$BASE_REVISION" \
    --workdir "$REPO" \
    --config "$REPO/.pylon/verify.yml" \
    --acceptance "$ROOT_PIPELINE_DIR/acceptance-criteria.json" \
    --output "$REPO_PIPELINE_DIR/criteria.json" \
    --held-out-output "$REPO_PIPELINE_DIR/evaluator-only/held-out.json" \
    --manifest "$REPO_PIPELINE_DIR/status.json"
done
```

결과는 `$ROOT_PIPELINE_DIR/status.json`의 `sub_pipelines`에 등록됩니다. 이후 단계는 전역 `$BRANCH` 대신
각 항목의 `repo`, `pipeline_dir`, `branch`, `base_revision`을 씁니다. `criteria.json`은 deterministic 명령과
acceptance criteria만 담습니다. held-out 명령(`evaluator-only/held-out.json`)의 경로·내용은 구현 agent
프롬프트나 evaluator bundle에 전달하지 않습니다.

### Step 4: 사전조건 검증

```bash
.pylon/builtin/scripts/bash/check-prerequisites.sh \
  --pipeline-dir "$PIPELINE_DIR" \
  --require-requirement \
  --require-architecture \
  --require-analysis
```

실패 시 누락된 산출물을 재생성합니다.

### Step 5: 태스크 분해

루트 세션이 `requirement-analysis.md`와 `architecture.md`를 기반으로 태스크를 분해합니다.

각 태스크에 ID, 제목, 설명, 담당 에이전트(`subagent_type` 이름), 의존성, `repo_id`, `repo`를 부여해
`$PIPELINE_DIR/tasks.json`에 씁니다. 의존성이 없는 태스크는 병렬로 실행됩니다.
`agent`는 `.pylon/builtin/agents/`의 내장 20개 이름 또는 `.pylon/agents/`의 사용자 에이전트 이름이어야 하며, tasks.json에 옛 이름이 남아 있으면 Step 6에서 `/pl:execute`(`.pylon/builtin/commands/pl-execute.md`)의 옛 에이전트 이름 매핑대로 바꿔 호출합니다:

```json
{"tasks": [
  {"id": "T001", "title": "...", "description": "...", "agent": "dev",
   "repo_id": "service-a", "repo": "service-a", "dependencies": [], "status": "pending"}
]}
```

계획 산출물이 확정되면 이력 체크포인트를 생성합니다:

```bash
pylon history checkpoint --pipeline "$(basename "$PIPELINE_DIR")" --phase planned
```

### Step 6: 에이전트 병렬 실행

**프롬프트에는 아래 6가지를 모두 내용째로 넣습니다**(경로만 넘기지 않습니다) — 서브 에이전트는 이 대화도
Step 3의 설계도 보지 못합니다.

```
// 의존성 없는 태스크를 한 메시지에서 동시에 실행 (subagent_type = tasks.json의 agent)
Agent(subagent_type="dev", prompt="## 설계 근거
[architecture.md 중 이 태스크의 섹션 전문]

## 수용 기준
[requirement-analysis.md의 해당 AC]

## 태스크
[tasks.json의 T001 항목 전문]

## 대상
- repo: [프로젝트 경로]  base: [base 브랜치]
- 수정 대상 파일: [절대 경로 목록]
- 기존 코드 패턴: [루트 세션이 파악한 규약]

## 검증
완료 후 다음 명령을 직접 실행하고 **출력 전문**을 보고하세요: [빌드/테스트/린트 명령]

## 범위 밖
[건드리지 말 것]", isolation="worktree")
```

`isolation="worktree"`는 **서로 파일이 겹치지 않고 의존성 설치가 필요 없는** 태스크에만 씁니다. 새
워크트리에는 의존성·빌드 산출물이 없어 검증이 돌지 않을 수 있으므로 그런 태스크는 격리 없이 순차 실행합니다.

결과를 `$PIPELINE_DIR/execution-log.json`에 집계하고(태스크별 repo, 상태, 변경 파일, 검증 출력) 실행
체크포인트를 만듭니다. **검증 출력이 첨부되지 않은 "완료" 보고는 완료로 처리하지 않습니다.**

```bash
pylon history checkpoint --pipeline "$(basename "$PIPELINE_DIR")" --phase executed
```

격리 실행한 태스크는 검증 전에 에이전트 브랜치를 **같은 repo의** 태스크 브랜치로 머지합니다(안 하면 Step 7이
에이전트 코드가 없는 트리를 검증합니다):

```bash
REPO="service-a"
TASK_BRANCH=$(jq -r --arg repo "$REPO" '.sub_pipelines[] | select(.repo == $repo) | .branch' "$ROOT_PIPELINE_DIR/status.json")
.pylon/builtin/scripts/bash/merge-branches.sh "$TASK_BRANCH" "$AGENT_BRANCH_1" "$AGENT_BRANCH_2" --git-root "$REPO"
```

다른 repo의 branch를 같은 merge 호출에 섞지 않습니다. merge 실패 시 checkpoint와 runtime을 보존하고
cleanup을 실행하지 않습니다.

### Step 7: 검증

변경된 **프로젝트마다** `--git-root`로 검증합니다(생략하면 루트에서 돌아 하위 프로젝트의 `verify.yml`을 보지
못합니다). 결과는 각 `$REPO_PIPELINE_DIR/verification.json`에 기록됩니다:

```bash
REPO="service-a"
REPO_PIPELINE_DIR=$(jq -r --arg repo "$REPO" '.sub_pipelines[] | select(.repo == $repo) | .pipeline_dir' "$ROOT_PIPELINE_DIR/status.json")
.pylon/builtin/scripts/bash/run-verification.sh "$REPO_PIPELINE_DIR" --git-root "$REPO"
```

- snapshot·manifest digest가 맞지 않으면 명령을 실행하지 않고 fail-closed합니다. snapshot 이후 live
  `verify.yml`이 바뀌면 결과는 실패이고 `criteria-events.jsonl`에 이벤트가 남습니다 — 기준 변경을 승인한 뒤
  새 run을 시작합니다.
- 실패하면 원인을 고쳐 재실행합니다. 실제 출력 없이 다음 단계로 넘어가지 않습니다.

deterministic gate가 통과하면 worker 자기 보고가 아니라 `execution-log.json`·변경 파일·verification
evidence를 근거로 repo 단위 task report를 확정합니다:

```bash
cat > "$REPO_PIPELINE_DIR/task-report-input.json" <<JSON
{
  "run_id": "$PIPELINE_ID",
  "task_id": "repo-final-$REPO_ID",
  "repo_id": "$REPO_ID",
  "attempt": 1,
  "status": "succeeded",
  "summary": "repo 구현과 deterministic verification 완료",
  "changed_files": ["repo-relative/path"],
  "evidence_refs": ["verification.json", "change.diff"],
  "verification": {
    "criteria_digest": "$(jq -r '.criteria_digest' "$REPO_PIPELINE_DIR/verification.json")",
    "deterministic_passed": true,
    "evaluator_status": "pending",
    "evidence_refs": ["verification.json"]
  },
  "hypotheses_rejected": [],
  "remaining_unknowns": []
}
JSON

pylon internal trajectory task-report \
  --input "$REPO_PIPELINE_DIR/task-report-input.json" \
  --output "$REPO_PIPELINE_DIR/task-report.json"
```

### Step 7B: 격리 evaluator

deterministic gate가 통과한 repo만 새 evaluator context로 검토합니다. 구현 agent의 대화·memory·prompt 없이
Pylon이 허용한 증거만 read-only bundle로 복사합니다:

```bash
BASE_REVISION=$(jq -r '.base_revision' "$REPO_PIPELINE_DIR/status.json")
git -C "$REPO" diff --binary "$BASE_REVISION" > "$REPO_PIPELINE_DIR/change.diff"

pylon internal evaluator prepare \
  --task-id "repo-final" \
  --implementation-root "$REPO" \
  --requirement "$ROOT_PIPELINE_DIR/requirement.md" \
  --criteria "$REPO_PIPELINE_DIR/criteria.json" \
  --verification "$REPO_PIPELINE_DIR/verification.json" \
  --diff "$REPO_PIPELINE_DIR/change.diff" \
  --task-report "$REPO_PIPELINE_DIR/task-report.json" \
  --output "$REPO_PIPELINE_DIR/evaluator-input" \
  --manifest "$REPO_PIPELINE_DIR/status.json"
```

새 `verifier` agent에는 bundle 경로만 전달합니다. 출력 JSON schema는 정의
파일에 있습니다(도구 제한은 정의 파일이 강제한다).

```
Agent(subagent_type="verifier", prompt="evaluator bundle: [$REPO_PIPELINE_DIR/evaluator-input 절대 경로]")
```

루트 세션이 agent의 JSON 응답을 `$REPO_PIPELINE_DIR/evaluator-raw.json`에 저장한 뒤 Pylon이 검증·기록합니다:

```bash
pylon internal evaluator record \
  --bundle "$REPO_PIPELINE_DIR/evaluator-input" \
  --manifest "$REPO_PIPELINE_DIR/status.json" \
  --input "$REPO_PIPELINE_DIR/evaluator-raw.json" \
  --output "$REPO_PIPELINE_DIR/evaluator-result.json"
```

criterion이 빠졌거나 `partial/missing`이 있는데 `pass`면 기록이 거부됩니다. deterministic gate 실패는
evaluator PASS로 덮을 수 없습니다.

두 gate가 모두 통과해야 해당 repo 태스크가 verified이며, 자연어 Agent 보고만으로 성공 처리하지 않습니다:

```bash
DETERMINISTIC_PASSED=$(jq -r '.passed == true' "$REPO_PIPELINE_DIR/verification.json")
EVALUATOR_PASSED=$(jq -r '.status == "pass"' "$REPO_PIPELINE_DIR/evaluator-result.json")
VERIFIED=false
[[ "$DETERMINISTIC_PASSED" == "true" && "$EVALUATOR_PASSED" == "true" ]] && VERIFIED=true
```

`$VERIFIED` 값을 `$PIPELINE_DIR/execution-log.json`의 해당 repo 태스크 항목마다 `verified: true|false`로
기록합니다.

### Step 8: PR 생성 (선택)

`config.yml`의 `git.pr.auto_pr`이 `true`일 때만 실행합니다(기본 비활성, 수동 생성은 `/pl:pr`):

```bash
source .pylon/builtin/scripts/bash/common.sh
AUTO_PR=$(config_get "git.pr.auto_pr" "false")
if [[ "$AUTO_PR" == "true" ]]; then
  jq -c '.sub_pipelines[]' "$ROOT_PIPELINE_DIR/status.json" | while IFS= read -r SUB; do
    REPO=$(echo "$SUB" | jq -r '.repo')
    REPO_PIPELINE_DIR=$(echo "$SUB" | jq -r '.pipeline_dir')
    TASK_BRANCH=$(echo "$SUB" | jq -r '.branch')
    .pylon/builtin/scripts/bash/create-pr.sh "$REPO_PIPELINE_DIR" --git-root "$REPO" \
      --branch "$TASK_BRANCH" --title "feat: [요구사항 요약]"
  done
fi
```

### Step 9: 완료 보고와 인계

여기서 run을 정리하지 않습니다. PR 생성·리뷰 반영·머지는 이 run의 브랜치와 워크트리에서 이어지므로, 머지 뒤
`/pl:cleanup`이 검증 근거 정리 → 메모리·도메인 지식 갱신 → 완료 체크포인트와 runtime·워크트리 정리 → 머지된 브랜치
정리를 한 번에 합니다. 메모리 저장도 그때 합니다.

root `status.json`을 인계 상태로 바꿉니다:

```bash
STATUS_TMP=$(mktemp "$ROOT_PIPELINE_DIR/.status.XXXXXX")
jq '.stage = "verified" | .status = "awaiting_cleanup"
    | if .sub_pipelines then .sub_pipelines |= map(.status = "awaiting_cleanup") else . end' \
  "$ROOT_PIPELINE_DIR/status.json" > "$STATUS_TMP"
mv "$STATUS_TMP" "$ROOT_PIPELINE_DIR/status.json"
```

실행 결과(생성/변경 파일, 검증 결과, PR URL, 소요 시간)를 요약하고, 남은 일(PR 생성·머지)과 마감 방법(머지 후
`/pl:cleanup`)을 알립니다.

---

## 비소프트웨어 파이프라인 (detected_domain: research, content, marketing)

각 단계 프롬프트에는 이전 산출물 내용을 붙여넣는다(경로만 넘기지 않는다). 독자와 수용 기준도 함께
넣습니다. 읽기 전용 에이전트(`researcher`, `market-researcher`, `fact-checker`, `editor`, `critic`)는 파일을
쓸 수 없으므로 루트 세션이 결과(최종 메시지)를 지정된 경로에 저장합니다.

### Step N1: 조사

루트 세션이 요구사항에서 조사 질문을 도출해 `$PIPELINE_DIR/research-plan.md`에 쓰고, 출처 유형마다
`researcher`를 **병렬·백그라운드**로 실행합니다. 출처 수는 질문이 정합니다. 마케팅은 `market-researcher`를
쓰고 성과 데이터가 있으면 함께 넣습니다. 모두 도착하면 각 결과를 `$PIPELINE_DIR/research-<source>.md`로
저장합니다.

```
Agent(subagent_type="researcher", run_in_background=true, prompt="sources: web\n\n[research-plan.md 내용]")
Agent(subagent_type="researcher", run_in_background=true, prompt="sources: academic\n\n[research-plan.md 내용]")
```

### Step N2: 종합

전략이 필요한 콘텐츠·마케팅이면 `strategist`(`domain: content|marketing`, 요구사항 + 조사 결과)가 먼저
`$PIPELINE_DIR/strategy.md`를 씁니다. 이어서 `writer`가 요구사항·전략·조사 결과로 초안
`$PIPELINE_DIR/draft.md`를 씁니다. `brief type:`은 리서치 `research report`, 콘텐츠 `article`, 마케팅
`marketing copy`입니다.

### Step N3: 독립 검토

`fact-checker`(초안 + 조사 자료)와 `editor`(초안 + 요구사항·전략)를 병렬로 실행하고 결과를
`$PIPELINE_DIR/review-facts.md`, `$PIPELINE_DIR/review-edit.md`로 저장합니다. 리서치 보고서는 `critic`도
추가할 수 있습니다(`review-critic.md`, 지적은 재작성 때 함께 넘깁니다).

### Step N4: 수정 루프

`editor`에 must-fix가 있거나 `fact-checker`에 `contradicted`가 하나라도 있으면 `writer`에 현재 초안과 해당
항목 전문을 넘겨 `draft.md`를 고쳐 쓰게 하고 Step N3를 다시 실행합니다. 상한은 3회입니다 — 라운드마다 검토
비용이 들기 때문입니다. 루프가 끝나거나 3회 상한에 도달하면 `draft.md`를 `$PIPELINE_DIR/final.md`로 복사하고
Step N5로 진행합니다. 상한에 도달했다면 남은 must-fix·`contradicted` 항목을 완료 보고에 미해결로 적습니다.

### Step N5: 완료

요구사항에 SEO가 있으면 `seo-specialist`에 최종본을 한 번 넘겨 제안을 `$PIPELINE_DIR/seo.md`에 저장하게
합니다. 완료 보고에는 산출물 목록, 미해결 must-fix·`contradicted` 요약, 남은 `unverified` 주장을 넣습니다.
root `status.json`은 Step 9와 같이 `awaiting_cleanup`으로 바꾸고, 마감(체크포인트·정리·메모리)은 `/pl:cleanup`에
맡깁니다.

---

## 에러 처리

- 실패하면 원인을 분석해 재시도하고, attempt마다 명령 출력·provider handle·변경 파일·배제한 가설을 task
  report에 남깁니다
- 3회 연속 실패하면 아래 failure record를 기록해 `failed` checkpoint를 만든 뒤 사용자에게 보고합니다
- terminal checkpoint 성공 전에는 `cleanup-pipeline.sh`를 호출하지 않고, merge·verification·PR·checkpoint
  중 하나라도 실패하면 root·repo runtime과 worktree를 보존합니다

```bash
cat > "$ROOT_PIPELINE_DIR/failure-input.json" <<JSON
{
  "run_id": "$PIPELINE_ID",
  "task_id": "$FAILED_TASK_ID",
  "repo_id": "$FAILED_REPO_ID",
  "attempt": $FAILED_ATTEMPT,
  "phase": "$FAILED_PHASE",
  "terminal_cause": "$TERMINAL_CAUSE",
  "evidence_refs": ["tasks/$FAILED_TASK_ID/attempts/$FAILED_ATTEMPT/stderr.log"],
  "hypotheses_rejected": [
    {"hypothesis": "배제한 원인", "probe": "확인 방법", "result": "배제 근거"}
  ],
  "remaining_unknowns": ["아직 확인하지 못한 사항"],
  "abandoned_reason": "최대 attempt 소진"
}
JSON

pylon internal trajectory failure \
  --pipeline "$PIPELINE_ID" \
  --input "$ROOT_PIPELINE_DIR/failure-input.json" \
  --output "$ROOT_PIPELINE_DIR/failure-record.json" \
  --manifest "$ROOT_PIPELINE_DIR/status.json"
```

이 명령은 failure record digest를 `status.json`에 결합한 뒤 `failed` 체크포인트를 만들고, 체크포인트가
실패하면 `cleanup.status=preserved`로 runtime/worktree/log를 보존합니다. 정리는 체크포인트 성공 뒤
`cleanup-pipeline.sh ... --terminal-phase failed`로만 합니다.

## Crash Recovery

재실행 시 이미 있는 산출물은 건너뛰고 `status.json`의 마지막 단계부터 재개합니다.
