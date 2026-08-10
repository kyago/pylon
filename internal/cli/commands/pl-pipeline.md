---
description: "Pylon 파이프라인 실행 — 요구사항 → 분석 → 설계 → 구현 → 검증"
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

사용자의 요구사항을 받아 적절한 워크플로우를 자동 선택하고 파이프라인을 실행합니다.

## 워크플로우 선택

요구사항을 분석하여 도메인에 맞는 워크플로우를 자동 선택합니다.
워크플로우를 지정하지 않으면 PO가 요구사항에서 자동 추론합니다.

| 도메인 | 워크플로우 | 파이프라인 |
|--------|-----------|-----------|
| 소프트웨어 개발 | feature/bugfix/hotfix | PO → Architect → PM → Agent → 검증 → PR |
| 리서치/조사 | research | PO → 병렬조사 → 교차검증 → 보고서 → 팩트체크 |
| 콘텐츠 제작 | content | PO → 초안작성 → 편집/리뷰 → (루프) → 최종본 |
| 마케팅 | marketing | PO → 시장조사 → 전략수립 → 콘텐츠생성 → 검증 |

## 실행 단계

### Step 1: 논리 파이프라인 초기화

root pipeline은 요구사항과 repo 목록을 묶는 논리 run입니다. 이 단계에서는 branch를 만들지 않습니다.

```bash
INIT_RESULT=$(.pylon/scripts/bash/init-pipeline.sh "$ARGUMENTS")
PIPELINE_ID=$(echo "$INIT_RESULT" | jq -r '.pipeline_id')
ROOT_PIPELINE_DIR=$(echo "$INIT_RESULT" | jq -r '.pipeline_dir')
PIPELINE_DIR="$ROOT_PIPELINE_DIR"
```

root 결과에는 `branch`가 없습니다. repo가 확정되기 전에 전역 branch를 만들거나 `.branch`를 읽지 않습니다.

### Step 2: PO 요구사항 분석 + 도메인 라우팅

Claude Code가 직접 PO 역할을 수행합니다.

1. `$PIPELINE_DIR/requirement.md`를 읽습니다
2. 위의 **워크플로우 선택** 테이블을 참조하여 도메인을 판단하고, `$PIPELINE_DIR/routing-decision.json`을 작성합니다:
   ```json
   {
     "detected_domain": "software|research|content|marketing",
     "selected_workflow": "feature|bugfix|hotfix|research|content|marketing",
     "reasoning": "도메인 선택 근거를 자연어로 기술",
     "agents": ["사용할 에이전트 목록"]
   }
   ```
3. 요구사항을 분석하여 `$PIPELINE_DIR/requirement-analysis.md`를 작성합니다:
   - 사용자 스토리 (As a... I want... So that...)
   - 수용 기준 (Acceptance Criteria)
   - 기능적/비기능적 요구사항 구분
   - 범위 밖 (Out of Scope) 항목
4. 수용 기준을 모델 독립 JSON으로 `$PIPELINE_DIR/acceptance-criteria.json`에 정규화합니다:
   ```json
   {
     "criteria": [
       {"id": "AC-1", "description": "검사 가능한 완료 조건"}
     ]
   }
   ```
   ID와 설명이 비어 있거나 ID가 중복되면 다음 단계로 진행하지 않습니다.
5. `detected_domain`에 따라 **도메인별 실행 절차**를 따릅니다:
   - `software` → **소프트웨어 파이프라인** (Steps 3-9)
   - `research` → **리서치 파이프라인** (Steps R1-R4)
   - `content` → **콘텐츠 파이프라인** (Steps C1-C4)
   - `marketing` → **마케팅 파이프라인** (Steps M1-M4)

---

## 소프트웨어 파이프라인 (detected_domain: software)

### Step 3: 아키텍처 분석

`.pylon/agents/architect.md`를 읽어 에이전트 정의를 가져온 뒤 Agent 도구로 실행합니다.

```
// 1. 에이전트 정의 로드
ARCHITECT_DEF=$(cat .pylon/agents/architect.md)

// 2. 아키텍트 에이전트 실행
Agent(prompt="$ARCHITECT_DEF\n\n## 태스크\n다음 요구사항을 분석하고 아키텍처 설계를 작성하세요: [requirement-analysis.md 내용]\n코드베이스 구조를 파악하고 영향 받는 repo, 파일, 변경 사항, 새로 생성할 파일을 명시하세요.\n결과를 $PIPELINE_DIR/architecture.md에 저장하고, 변경 대상 repo를 {\"repos\":[{\"path\":\"workspace 상대경로\"}]} 형식으로 $PIPELINE_DIR/repos.json에 저장하세요.")
```

에이전트 정의는 `.pylon/agents/architect.md`에서 읽어 프롬프트에 주입합니다.

### Step 3B: repo sub-pipeline 초기화

`repos.json`의 각 repo에 대해 독립 sub-pipeline을 만듭니다. branch, base revision, worktree, verification,
PR은 이 repo sub-pipeline만 소유합니다.

```bash
jq -r '.repos[].path' "$ROOT_PIPELINE_DIR/repos.json" | while IFS= read -r REPO; do
  SUB_RESULT=$(.pylon/scripts/bash/init-pipeline.sh "$ARGUMENTS" \
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

생성 결과는 `$ROOT_PIPELINE_DIR/status.json`의 `sub_pipelines`에 등록됩니다. 이후 단계에서는 전역
`$BRANCH`를 만들지 말고 각 항목의 `repo`, `pipeline_dir`, `branch`, `base_revision`을 함께 사용합니다.
`criteria.json`은 worker 실행 전에 만들어지며 deterministic 명령과 acceptance criteria만 포함합니다.
held-out 명령은 manifest에 binding된 `$REPO_PIPELINE_DIR/evaluator-only/held-out.json`에 별도 저장하고
구현 agent 프롬프트나 evaluator input bundle에 경로·내용을 전달하지 않습니다.

### Step 4: 사전조건 검증

```bash
.pylon/scripts/bash/check-prerequisites.sh \
  --pipeline-dir "$PIPELINE_DIR" \
  --require-requirement \
  --require-architecture \
  --require-analysis
```

실패 시 누락된 산출물을 재생성합니다.

### Step 5: PM 태스크 분해

`requirement-analysis.md`와 `architecture.md`를 기반으로 태스크를 분해합니다.

1. 각 태스크에 ID, 제목, 설명, 담당 에이전트, 의존성, `repo_id`, `repo`를 부여합니다
2. 의존성이 없는 태스크는 병렬 실행 가능하도록 표시합니다
3. `$PIPELINE_DIR/tasks.json` 형식:

```json
{
  "tasks": [
    {
      "id": "T001",
      "title": "...",
      "description": "...",
      "agent": "backend-dev",
      "repo_id": "service-a",
      "repo": "service-a",
      "dependencies": [],
      "status": "pending"
    }
  ]
}
```

계획 산출물이 확정되면 이력 체크포인트를 생성합니다:

```bash
pylon history checkpoint --pipeline "$(basename "$PIPELINE_DIR")" --phase planned
```

### Step 6: 에이전트 병렬 실행

독립 태스크는 Agent 도구로 병렬 실행합니다.

각 Agent 호출은 `/pl:execute`의 durable state lifecycle을 따라야 합니다. root run에 대해
`pylon internal state create-run` 또는 재시작 시 `state recover`를 먼저 수행하고, 모든 태스크를
`create-task -> ready -> claim -> start`로 전이한 뒤 Agent를 시작합니다. Agent 결과는 동일 attempt의
fencing token으로 `state complete`가 성공한 경우에만 trajectory와 `execution-log.json`에 반영합니다.

**프롬프트에는 아래 6가지를 모두 넣습니다.** 서브 에이전트는 이 대화도, Step 3에서 만든 설계도 보지
못합니다 — 프롬프트에 넣지 않은 것은 존재하지 않는 것과 같습니다. 경로만 넘기지 말고 **내용을 붙여넣습니다**.

```
// 의존성 없는 태스크를 동시에 실행 (T002, T003도 동일한 형식)
Agent(prompt="[에이전트 정의 — .pylon/agents/{agent}.md]

## 설계 근거
[architecture.md 중 이 태스크에 해당하는 섹션 전문]

## 수용 기준
[requirement-analysis.md의 해당 Acceptance Criteria]

## 태스크
[tasks.json의 T001 항목 전문 — title, description 포함]

## 대상
- repo: [프로젝트 경로]  base: [base 브랜치]
- 수정 대상 파일: [절대 경로 목록]
- 기존 코드 패턴: [메인 루프가 이미 파악한 규약]

## 검증
완료 후 다음 명령을 직접 실행하고 **출력 전문**을 보고하세요: [빌드/테스트/린트 명령]
출력 없이 '통과했습니다'라고 보고하지 않습니다.

## 범위 밖
[건드리지 말 것]", isolation="worktree")
```

`isolation="worktree"`는 **의존성 설치가 필요 없고 서로 파일이 겹치지 않는** 태스크에만 씁니다.
새 워크트리에는 의존성·빌드 산출물이 없어 검증 명령이 실행되지 않을 수 있습니다. 검증 실행이 필요한
태스크는 격리 없이 순차 실행합니다.

모든 에이전트 결과를 `$PIPELINE_DIR/execution-log.json`에 집계합니다(태스크별 repo, 상태, 변경 파일, 검증 출력).
**검증 출력이 첨부되지 않은 "완료" 보고는 완료로 처리하지 않습니다.** 그 다음 실행 체크포인트를 생성합니다:

```bash
pylon history checkpoint --pipeline "$(basename "$PIPELINE_DIR")" --phase executed
```

에이전트 정의는 `.pylon/agents/{agent-name}.md`에서 읽어 프롬프트에 주입합니다.

격리 실행한 태스크가 있으면 검증 전에 **같은 repo의** 에이전트 브랜치를 해당 repo의 태스크 브랜치로 머지합니다.
머지하지 않으면 Step 7은 에이전트가 쓴 코드가 없는 트리를 검증하게 됩니다:

```bash
REPO="service-a"
TASK_BRANCH=$(jq -r --arg repo "$REPO" '.sub_pipelines[] | select(.repo == $repo) | .branch' "$ROOT_PIPELINE_DIR/status.json")
.pylon/scripts/bash/merge-branches.sh "$TASK_BRANCH" "$AGENT_BRANCH_1" "$AGENT_BRANCH_2" --git-root "$REPO"
```

다른 repo의 branch를 같은 merge 호출에 섞지 않습니다. merge 실패 시 checkpoint와 runtime을 보존하고
cleanup을 실행하지 않습니다.

### Step 7: 검증

변경이 일어난 **프로젝트를 대상으로** 검증합니다. `--git-root`를 생략하면 워크스페이스 루트에서 실행되어
하위 프로젝트의 `.pylon/verify.yml`을 보지 못합니다:

```bash
REPO="service-a"
REPO_PIPELINE_DIR=$(jq -r --arg repo "$REPO" '.sub_pipelines[] | select(.repo == $repo) | .pipeline_dir' "$ROOT_PIPELINE_DIR/status.json")
.pylon/scripts/bash/run-verification.sh "$REPO_PIPELINE_DIR" --git-root "$REPO"
```

여러 프로젝트를 수정했다면 프로젝트마다 실행합니다.

결과는 각 `$REPO_PIPELINE_DIR/verification.json`에 기록됩니다.

- criteria snapshot 또는 manifest digest가 일치하지 않으면 검증 명령을 실행하지 않고 fail-closed합니다.
- live `.pylon/verify.yml`이 snapshot 이후 변경되면 frozen snapshot 명령을 실행하되 전체 결과는 실패하며
  repo pipeline의 `criteria-events.jsonl`에 변경 이벤트를 남깁니다. 기준 변경을 승인한 뒤 새 run을 시작합니다.
- 검증 실패 시 에러를 분석하고 수정 후 재실행합니다.
- 검증 명령의 실제 출력 없이 다음 단계로 넘어가지 않습니다.

deterministic gate가 통과하면 repo 단위 구현 결과를 동일한 task report schema로 확정합니다. worker의
자기 보고가 아니라 `execution-log.json`, 변경 파일, verification evidence를 근거로 작성합니다:

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

deterministic gate가 통과한 repo만 새 evaluator context로 검토합니다. 구현 agent의 대화, memory,
worker prompt를 전달하지 않습니다. Pylon이 허용한 증거만 read-only bundle로 복사합니다:

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

새 `verifier` agent에는 `$REPO_PIPELINE_DIR/evaluator-input` 경로와 아래 출력 schema만 전달합니다.
이 agent는 `Read/Grep/Glob`만 사용하며 Bash/Edit/Write를 사용할 수 없습니다.

```json
{
  "schema_version": 1,
  "request_digest": "request.json의 digest",
  "status": "pass|fail|incomplete",
  "summary": "판정 요약",
  "criteria": [
    {"id": "AC-1", "status": "verified|partial|missing", "evidence": "bundle 내부 근거"}
  ],
  "risks": [],
  "evidence_refs": ["change.diff"],
  "evaluator": "verifier"
}
```

agent의 JSON 응답을 `$REPO_PIPELINE_DIR/evaluator-raw.json`에 저장한 뒤 Pylon이 검증·기록합니다:

```bash
pylon internal evaluator record \
  --bundle "$REPO_PIPELINE_DIR/evaluator-input" \
  --manifest "$REPO_PIPELINE_DIR/status.json" \
  --input "$REPO_PIPELINE_DIR/evaluator-raw.json" \
  --output "$REPO_PIPELINE_DIR/evaluator-result.json"
```

모든 acceptance criterion이 포함되지 않았거나 하나라도 `partial/missing`인데 `pass`를 반환하면 기록이
거부됩니다. deterministic gate 실패는 evaluator PASS로 덮어쓸 수 없습니다.

evaluator 결과를 기록한 뒤 해당 repo에서 `verifying` 상태인 모든 태스크를 최종 확정합니다. 두 gate 중
하나라도 실패하면 해당 플래그를 생략하여 task를 `failed`로 만들며, 자연어 Agent 보고만으로 성공 처리하지
않습니다:

```bash
DETERMINISTIC_PASSED=$(jq -r '.passed == true' "$REPO_PIPELINE_DIR/verification.json")
EVALUATOR_PASSED=$(jq -r '.status == "pass"' "$REPO_PIPELINE_DIR/evaluator-result.json")

jq -r --arg repo_id "$REPO_ID" '.tasks[] | select(.repo_id == $repo_id) | .id' "$PIPELINE_DIR/tasks.json" |
while read -r TASK_ID; do
  [[ $(pylon internal state show "$PIPELINE_ID" "$TASK_ID" | jq -r '.status') == "verifying" ]] || continue
  VERIFY_ARGS=()
  [[ "$DETERMINISTIC_PASSED" == "true" ]] && VERIFY_ARGS+=(--deterministic)
  [[ "$EVALUATOR_PASSED" == "true" ]] && VERIFY_ARGS+=(--evaluator)
  pylon internal state verify "$PIPELINE_ID" "$TASK_ID" \
    "${VERIFY_ARGS[@]}" \
    --evidence "$REPO_PIPELINE_DIR/verification.json,$REPO_PIPELINE_DIR/evaluator-result.json"
done
```

### Step 8: PR 생성 (선택)

> PR 생성은 기본적으로 비활성화되어 있습니다.
> `config.yml`의 `git.pr.auto_pr: true` 설정 시에만 자동 실행됩니다.
> 수동으로 PR을 생성하려면 `/pl:pr` 커맨드를 사용하세요.

`config.yml`에서 `git.pr.auto_pr` 값을 읽어 `true`인 경우에만 실행합니다:

```bash
source .pylon/scripts/bash/common.sh
AUTO_PR=$(config_get "git.pr.auto_pr" "false")
if [[ "$AUTO_PR" == "true" ]]; then
  jq -c '.sub_pipelines[]' "$ROOT_PIPELINE_DIR/status.json" | while IFS= read -r SUB; do
    REPO=$(echo "$SUB" | jq -r '.repo')
    REPO_PIPELINE_DIR=$(echo "$SUB" | jq -r '.pipeline_dir')
    TASK_BRANCH=$(echo "$SUB" | jq -r '.branch')
    .pylon/scripts/bash/create-pr.sh "$REPO_PIPELINE_DIR" --git-root "$REPO" \
      --branch "$TASK_BRANCH" --title "feat: [요구사항 요약]"
  done
fi
```

### Step 9: 완료 보고

`status.json`을 최종 상태로 갱신한 뒤 완료 체크포인트를 생성합니다:

```bash
pylon history checkpoint --pipeline "$PIPELINE_ID" --phase completed
```

체크포인트가 성공하면 repo sub-pipeline을 각각 정리한 뒤 root runtime을 정리합니다. cleanup script도
terminal checkpoint manifest를 다시 확인합니다. 체크포인트가 실패하면 어떤 cleanup도 실행하지 않습니다:

```bash
jq -c '.sub_pipelines[]' "$ROOT_PIPELINE_DIR/status.json" | while IFS= read -r SUB; do
  REPO_PIPELINE_DIR=$(echo "$SUB" | jq -r '.pipeline_dir')
  .pylon/scripts/bash/cleanup-pipeline.sh "$REPO_PIPELINE_DIR" --terminal-phase completed
done
.pylon/scripts/bash/cleanup-pipeline.sh "$ROOT_PIPELINE_DIR" --terminal-phase completed
```

완료 checkpoint가 확정된 뒤에만 학습 후보를 만들 수 있습니다. 실행 중 대화나 단순 세션 종료를 trigger로
사용하지 않습니다. 후보 생성은 선택 사항이며 active memory/skill을 자동 수정하지 않습니다:

```bash
cat > curator-proposal.json <<JSON
{
  "type": "memory|pitfall|skill|agent_prompt|pipeline_rule|acceptance_corpus",
  "title": "후보 제목",
  "summary": "제안 변경 요약",
  "rationale": "finalized evidence에 기반한 이유",
  "target_files": [".pylon/memory/<project>/learning/example.md"],
  "evidence_refs": ["task-reports-summary.json", "failure-records-summary.json"],
  "regression_fixtures": ["fixture-id"]
}
JSON

pylon internal curator propose \
  --checkpoint "$PIPELINE_ID/completed" \
  --input curator-proposal.json
```

사람이 candidate의 `proposal.md`, `evidence.json`, `source-runs.json`, `target-files.json`을 검토한 뒤에만
`pylon internal curator review --decision approve|reject`를 실행합니다. 승인 후보도 acceptance corpus가
통과한 report를 `pylon internal curator gate`로 기록하기 전에는 적용 대상이 아닙니다. Pylon은 gate 이후에도
active 파일을 자동 수정하지 않으며 실제 적용은 별도 commit/PR로 수행합니다.

실행 결과를 요약합니다:
- 생성/변경된 파일 목록
- 테스트 결과
- PR URL (auto_pr 활성화 시)
- 총 소요 시간

---

## 리서치 파이프라인 (detected_domain: research)

> 에이전트 정의는 `.pylon/agents/{agent-name}.md`에서 읽어 프롬프트에 주입합니다.
> 각 단계 실행 전, 이전 단계의 산출물이 `$PIPELINE_DIR/`에 존재하는지 확인합니다.

### Step R1: 병렬 조사 (fan_out)

Agent 도구로 조사 에이전트를 **병렬** 실행합니다.

```
Agent(prompt="[lead-researcher 에이전트 정의]\n\n## 조사 요구사항\n[requirement-analysis.md 내용]\n\n조사 계획을 수립하고 핵심 질문을 도출하세요.\n결과를 $PIPELINE_DIR/research-plan.md에 저장하세요.")

// 아래 2개를 단일 메시지에서 병렬 실행
Agent(prompt="[web-searcher 에이전트 정의]\n\n## 조사 요구사항\n[research-plan.md 내용]\n\n웹 소스를 조사하세요.\n결과를 $PIPELINE_DIR/web-research.md에 저장하세요.")
Agent(prompt="[academic-analyst 에이전트 정의]\n\n## 조사 요구사항\n[research-plan.md 내용]\n\n학술 자료를 조사하세요.\n결과를 $PIPELINE_DIR/academic-research.md에 저장하세요.")
```

### Step R2: 교차 검증 (fan_in)

```
Agent(prompt="[fact-checker 에이전트 정의]\n\n## 검증 대상\n[web-research.md + academic-research.md 내용]\n\n출처 간 교차 검증을 수행하세요:\n- 상충하는 정보 식별\n- 출처 신뢰도 평가\n- 검증된 사실과 미확인 주장 구분\n결과를 $PIPELINE_DIR/fact-check.md에 저장하세요.")
```

### Step R3: 보고서 작성 (generate)

```
Agent(prompt="[report-writer 에이전트 정의]\n\n## 참조 자료\n[research-plan.md + web-research.md + academic-research.md + fact-check.md 내용]\n\n종합 보고서를 작성하세요:\n- 핵심 발견 사항 요약\n- 근거 자료 인용\n- 결론 및 권고사항\n결과를 $PIPELINE_DIR/report.md에 저장하세요.")
```

### Step R4: 최종 검증 (validate) + 완료

PO가 직접 보고서를 검토합니다:
1. `$PIPELINE_DIR/report.md`를 읽고 품질을 검증합니다:
   - 요구사항의 질문에 답변되었는가?
   - 근거가 충분한가?
   - 논리적 비약이 없는가?
2. 부족하면 Step R3로 돌아가 보완을 지시합니다 (최대 3회)
3. 완료 시 실행 결과를 요약합니다

---

## 콘텐츠 파이프라인 (detected_domain: content)

> 에이전트 정의는 `.pylon/agents/{agent-name}.md`에서 읽어 프롬프트에 주입합니다.
> 각 단계 실행 전, 이전 단계의 산출물이 `$PIPELINE_DIR/`에 존재하는지 확인합니다.

### Step C1: 콘텐츠 전략 수립

```
Agent(prompt="[content-strategist 에이전트 정의]\n\n## 요구사항\n[requirement-analysis.md 내용]\n\n콘텐츠 전략을 수립하세요:\n- 타겟 독자 정의\n- 톤/스타일 가이드\n- 구성 개요 (아웃라인)\n- SEO 키워드 (해당 시)\n결과를 $PIPELINE_DIR/content-strategy.md에 저장하세요.")
```

### Step C2: 초안 작성 (generate)

```
Agent(prompt="[writer 에이전트 정의]\n\n## 전략\n[content-strategy.md 내용]\n\n전략에 따라 콘텐츠 초안을 작성하세요.\n결과를 $PIPELINE_DIR/draft.md에 저장하세요.")
```

### Step C3: 편집 및 리뷰 (validate → generate 루프)

아래를 **최대 3회** 반복합니다. PO가 승인하면 루프를 종료합니다.

```
// 편집자와 QA 리뷰어를 병렬 실행
Agent(prompt="[editor 에이전트 정의]\n\n## 초안\n[draft.md 내용]\n\n문법, 스타일, 가독성을 편집하세요.\n편집 피드백을 $PIPELINE_DIR/edit-feedback.md에 저장하세요.")
Agent(prompt="[content-reviewer 에이전트 정의]\n\n## 초안\n[draft.md 내용]\n\n품질 기준 충족 여부를 검토하세요:\n- 정확성, 완전성, 일관성\n검토 결과를 $PIPELINE_DIR/review-feedback.md에 저장하세요.")
```

PO가 피드백을 종합하여:
- 수정이 필요하면 → writer에게 피드백과 함께 재작성 지시 (Step C2로 복귀)
- 승인이면 → Step C4로 진행

### Step C4: SEO 최적화 + 완료

```
Agent(prompt="[seo-specialist 에이전트 정의]\n\n## 최종 콘텐츠\n[draft.md 내용]\n[content-strategy.md의 SEO 키워드]\n\nSEO 최적화를 적용하세요:\n- 메타 설명, 제목 태그 제안\n- 키워드 밀도 검토\n- 내부/외부 링크 제안\n결과를 $PIPELINE_DIR/final-content.md에 저장하세요.")
```

완료 시 실행 결과를 요약합니다.

---

## 마케팅 파이프라인 (detected_domain: marketing)

> 에이전트 정의는 `.pylon/agents/{agent-name}.md`에서 읽어 프롬프트에 주입합니다.
> 각 단계 실행 전, 이전 단계의 산출물이 `$PIPELINE_DIR/`에 존재하는지 확인합니다.

### Step M1: 시장 조사 (fan_out)

Agent 도구로 조사 에이전트를 **병렬** 실행합니다.

```
// 병렬 실행
Agent(prompt="[market-researcher 에이전트 정의]\n\n## 요구사항\n[requirement-analysis.md 내용]\n\n시장 조사를 수행하세요:\n- 타겟 시장 분석\n- 경쟁사 분석\n- 시장 트렌드\n결과를 $PIPELINE_DIR/market-research.md에 저장하세요.")
Agent(prompt="[data-analyst 에이전트 정의]\n\n## 요구사항\n[requirement-analysis.md 내용]\n\n데이터 기반 분석을 수행하세요:\n- 기존 마케팅 성과 데이터 분석\n- 타겟 고객 세그먼트 정의\n- KPI 벤치마크\n결과를 $PIPELINE_DIR/data-analysis.md에 저장하세요.")
```

### Step M2: 전략 수립 (fan_in)

```
Agent(prompt="[brand-strategist 에이전트 정의]\n\n## 참조 자료\n[market-research.md + data-analysis.md 내용]\n\n마케팅 전략을 수립하세요:\n- 포지셔닝 전략\n- 메시지 프레임워크\n- 채널 전략\n- 예산 배분 제안\n결과를 $PIPELINE_DIR/marketing-strategy.md에 저장하세요.")
```

### Step M3: 콘텐츠 생성 (generate)

```
// 병렬 실행
Agent(prompt="[copywriter 에이전트 정의]\n\n## 전략\n[marketing-strategy.md 내용]\n\n마케팅 카피를 작성하세요:\n- 헤드라인, 서브헤드\n- 본문 카피\n- CTA (Call to Action)\n결과를 $PIPELINE_DIR/marketing-copy.md에 저장하세요.")
Agent(prompt="[campaign-planner 에이전트 정의]\n\n## 전략\n[marketing-strategy.md 내용]\n\n캠페인 실행 계획을 수립하세요:\n- 타임라인\n- 채널별 실행 계획\n- 성과 측정 기준\n결과를 $PIPELINE_DIR/campaign-plan.md에 저장하세요.")
```

### Step M4: 검증 + 완료

PO가 마케팅 산출물을 종합 검증합니다:
1. `marketing-strategy.md`, `marketing-copy.md`, `campaign-plan.md`를 검토합니다
2. 전략과 실행 계획의 일관성을 확인합니다
3. 부족한 산출물에 따라 Step M2 또는 M3의 해당 에이전트에 수정을 지시합니다 (최대 3회)
4. 완료 시 실행 결과를 요약합니다

---

## 메모리 활용

파이프라인 시작 시 `pylon mem search` CLI로 관련 도메인 지식을 검색하여
에이전트 프롬프트에 주입합니다:

```bash
pylon mem search --project <project-name> "<요구사항 키워드>"
```

## 에러 처리

- 각 단계 실패 시 에러를 분석하고 재시도합니다
- 각 실패 attempt의 명령 출력, provider handle, 변경 파일, 배제한 가설을 task report에 기록합니다
- 3회 연속 실패 시 아래 failure record를 먼저 기록하고 `failed` checkpoint를 생성한 뒤 사용자에게 보고합니다
- terminal checkpoint가 성공하기 전에는 `cleanup-pipeline.sh`를 호출하지 않습니다
- merge, verification, PR, checkpoint 중 하나라도 실패하면 root와 repo runtime 및 worktree를 보존합니다

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

이 명령은 failure record를 원자적으로 기록하고 `status.json`에 digest를 결합한 후에만
`pylon history checkpoint --phase failed`와 동등한 체크포인트를 생성합니다. 체크포인트가 실패하면
`cleanup.status=preserved`를 기록하고 오류를 반환하므로 runtime/worktree/log를 그대로 둡니다. 체크포인트가
성공한 뒤 명시적으로 정리할 때만 `cleanup-pipeline.sh ... --terminal-phase failed`를 사용합니다.

## Crash Recovery

파이프라인 재실행 시 기존 산출물을 확인합니다:
- 이미 존재하는 산출물은 건너뜁니다
- `status.json`의 마지막 단계부터 재개합니다
