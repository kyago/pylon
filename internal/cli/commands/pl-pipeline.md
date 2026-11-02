---
description: "Pylon 파이프라인 실행 — 요구사항 → 분석 → 설계 → 구현 → 검증"
---

# Pylon Pipeline — 전체 워크플로우 실행

요구사항에 맞는 워크플로우를 골라 파이프라인을 실행합니다.

## 워크플로우 선택

feature/bugfix/hotfix: 요구사항 분석 → architect → 태스크 분해 → dev → 검증 → PR

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

### Step 2: 요구사항 분석 + 워크플로우 선택

루트 세션이 직접 PO 역할을 수행합니다.

1. `$PIPELINE_DIR/requirement.md`를 읽고, 세션 컨텍스트의 "pylon 프로젝트 메모리" 목록과
   `pylon mem search --project <프로젝트> --query "<요구사항 키워드>"`로 관련 결정·학습을 확인합니다. 찾은 내용은
   분석과 에이전트 프롬프트에 반영합니다
2. 워크플로우를 판단해 `$PIPELINE_DIR/routing-decision.json`을 작성합니다:
   ```json
   {"selected_workflow": "feature|bugfix|hotfix",
    "reasoning": "선택 근거", "agents": ["사용할 에이전트"]}
   ```
3. 사용자 스토리, 수용 기준, 기능·비기능 요구사항, 범위 밖 항목을 담아 `$PIPELINE_DIR/requirement-analysis.md`를
   작성합니다
4. 수용 기준을 모델 독립 JSON으로 `$PIPELINE_DIR/acceptance-criteria.json`에 정규화합니다:
   ```json
   {"criteria": [{"id": "AC-1", "description": "검사 가능한 완료 조건"}]}
   ```
   ID와 설명이 비어 있거나 ID가 중복되면 다음 단계로 진행하지 않습니다.
   `selected_workflow`가 `bugfix`·`hotfix`면 첫 기준을 결함 재현으로 둡니다 — 재현 명령이나 절차, 지금 나오는
   잘못된 결과, 수정 후 기대 결과. 수정은 이 재현이 기대 결과를 낼 때만 완료입니다.

Claude Code에서는 `.claude/agents/`에 등록된 정의를 `subagent_type`으로 호출한다. 그 외 provider에서는 `.pylon/builtin/agents/<name>.md` 본문을 프롬프트 앞에 붙인다. 사용자가 추가한 에이전트는 `.pylon/agents/<name>.md`에 있다.

---

## 구현 단계 (Steps 3-9)

### Step 3: 아키텍처 분석

```
Agent(subagent_type="architect", prompt="## 태스크\n다음 요구사항의 아키텍처 설계를 작성하세요: [requirement-analysis.md 내용]\n설계는 $PIPELINE_DIR/architecture.md에, 변경 대상 repo 목록은 $PIPELINE_DIR/repos.json에 저장하세요.")
```

### Step 3B: repo sub-pipeline 초기화

`repos.json`의 repo마다 독립 sub-pipeline을 만듭니다. branch, base revision, worktree, verification,
PR은 이 repo sub-pipeline만 소유합니다.

**브랜치 이름은 사용자 규칙이 먼저입니다.** repo마다 아래 순서로 브랜치 명명 규칙을 찾아, 먼저 찾은 규칙으로 이름을
정하고 `repos.json`의 그 항목에 `branch`로 적습니다(`{"path":"services/api","branch":"feature/api-123-add-group-code"}`):

1. 대상 repo의 `CLAUDE.md`, `AGENTS.md`, `CONTRIBUTING.md`
2. 워크스페이스 `AGENTS.md`의 pylon 블록 밖, 세션에 로드된 사용자 지침(`~/.claude/CLAUDE.md` 등)

규칙이 설명 부분을 요구하면 Step 1의 `$TITLE`을 쓰고, 이슈 번호처럼 요구사항에 없는 값이 필요하면 지어내지 말고 사용자에게
묻습니다. 규칙이 없으면 `branch`를 적지 않습니다 — 스크립트가 `.pylon/config.yml`의 `git.branch_prefix`(기본 `task`)로
`<접두사>-<run-id>-<repo-id>`를 만듭니다(`feature/`처럼 `/`로 끝나면 `<접두사><run-id>-<repo-id>`).

이미 있는 브랜치는 로컬이든 origin이든 재사용하지 않습니다. 이름이 거부되면 반복문이 멈춥니다. `branch already exists`면
`repos.json`의 이름을 고치거나 사용자에게 묻고, origin을 확인하지 못했다면 사용자에게 묻습니다. 고친 뒤 반복문을 처음부터
다시 실행합니다. 이미 초기화한 repo는 기록된 브랜치로 그대로 이어집니다.

```bash
jq -r '.repos[] | [.path, .branch // ""] | @tsv' "$ROOT_PIPELINE_DIR/repos.json" | while IFS=$'\t' read -r REPO REPO_BRANCH; do
  SUB_RESULT=$(.pylon/builtin/scripts/bash/init-pipeline.sh "$PIPELINE_ID" \
    --git-root "$REPO" \
    --pipeline-dir "$ROOT_PIPELINE_DIR" \
    --branch "$REPO_BRANCH") || exit 1
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

`bugfix`·`hotfix`는 재현 테스트를 첫 태스크로 두고 수정 태스크가 그것에 의존하게 합니다 — 함께 병렬로 돌면
"수정 전에 실패했다"는 증거가 남지 않습니다. 두 태스크는 격리 없이 순서대로 실행합니다(격리하면 수정 태스크의
워크트리에 재현 테스트가 없습니다).

- 재현 태스크는 테스트만 추가하고 제품 코드는 고치지 않습니다. Step 6 프롬프트의 범위 밖에 그렇게 적고, `## 검증`에
  기대 결과로 "새 테스트가 결함의 지금 잘못된 결과로 실패한다"를 적습니다.
- 루트 세션은 그 실패 출력이 첨부되면 성공으로 기록하고 재시도하지 않습니다. 새 테스트가 통과하면(재현 안 됨)
  테스트를 지금 동작에 맞추지 않고, 수정 태스크로 넘어가지 않은 채 사용자에게 보고합니다.
- 재현 실패 출력과 수정 뒤 통과 출력은 Step 7 task report의 `summary`에 원문으로 넣습니다(Step 7의 `task-summary.txt`).
  evaluator는 `execution-log.json`을 보지 못합니다.
- 로컬 테스트로 재현하기 비싸면(통합 환경·운영 데이터 필요) 그 이유를 `requirement-analysis.md`에 적고 재현
  태스크를 두지 않습니다. 그때는 수정 뒤 루트 세션이 Step 2의 재현 절차를 직접 실행하고 그 출력을 같은 `summary`에
  넣습니다.

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

### Step 6B: 커밋

검증은 깨끗한 작업 트리의 커밋된 HEAD만 기록합니다(`verification.json`의 `tree`). 그래서 검증 전에 repo마다 이번
run의 변경을 task 브랜치에 커밋합니다:

```bash
REPO="service-a"
SUB=$(jq -c --arg repo "$REPO" '.sub_pipelines[] | select(.repo == $repo)' "$ROOT_PIPELINE_DIR/status.json")
TASK_BRANCH=$(echo "$SUB" | jq -r '.branch')
REPO_PIPELINE_DIR=$(echo "$SUB" | jq -r '.pipeline_dir')
git -C "$REPO" branch --show-current   # $TASK_BRANCH여야 합니다
git -C "$REPO" status --porcelain --untracked-files=normal --ignore-submodules=untracked -- ':(top)' ':(top,glob,exclude)**/.pylon/**' ':(top,exclude).claude/worktrees'
git -C "$REPO" log --oneline -5        # 커밋 메시지 형식
```

- 현재 브랜치가 `$TASK_BRANCH`가 아니면 멈추고 보고합니다.
- `execution-log.json`의 변경 파일과 새 테스트만 `git add -- <파일>`로 stage합니다(`git add -A` 금지).
- 메시지는 repo의 최근 커밋 형식을 따릅니다. 새 커밋만 만듭니다(amend·강제 push 금지 — `create-pr.sh`는 강제 없이
  push합니다). hook 실패나 git 사용자 정보 누락이면 `--no-verify`로 우회하지 않고 멈춰 보고합니다.
- 커밋할 변경이 없으면(status가 이미 비어 있음) 커밋을 건너뜁니다.
- 커밋 뒤에도 남은 파일:
  - 추적 중인 파일의 변경은 태스크 결과인지 확인해 맞으면 커밋하고, 판단이 서지 않으면 되돌리지 말고 멈춰 보고합니다.
  - 추적 안 된 소스·테스트 파일은 태스크 결과에서 빠졌는지 확인해 맞으면 커밋합니다.
  - 나머지(커버리지, 빌드 결과, 임시 파일)는 run이 만든 잔여물입니다 — run은 깨끗한 트리에서 시작합니다
    (`init-pipeline.sh`가 확인). 지우지 말고 그 파일들을 지정해 경로째 run 폴더로 옮기고(호출마다 새
    `leftovers/<시각>/` 폴더, 디렉토리를 지정하면 그 아래 추적 안 된 파일 전부) 출력의 `moved` 목록을 완료 보고에
    남깁니다. 지정하지 않은 파일은 옮기지 않습니다. `/pl:cleanup`은 이 폴더를 지우지 않고
    `.pylon/runtime/leftovers/<run>/`으로 옮겨 보존합니다:

    ```bash
    pylon internal move-untracked --workdir "$REPO" --to "$REPO_PIPELINE_DIR/leftovers" -- coverage.out build/
    ```
- 위 status가 비어 있어야 Step 7로 갑니다.

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
- 실패하면 원인을 고쳐 Step 6B대로 커밋한 뒤 재실행합니다. 실제 출력 없이 다음 단계로 넘어가지 않습니다.
- 통과했는데 `tree`가 없으면 `tree_reason`을 봅니다. 미커밋 변경이 남았으면 Step 6B대로 정리하고 재실행합니다.
  깨끗한 트리에서 시작한 검증이 새로 만든 파일(coverage 등)은 검증이 `$REPO_PIPELINE_DIR/verify-artifacts/`로 직접
  옮기므로(같은 경로는 최신본으로 덮어씀) 따로 처리하지 않습니다.

deterministic gate가 통과하면 worker 자기 보고가 아니라 `execution-log.json`·변경 파일·verification
evidence를 근거로 repo 단위 task report를 확정합니다. `summary`에는 테스트 출력 원문이 들어가므로 따옴표 친 heredoc으로 파일에 쓰고 JSON은 `jq`로 만듭니다(따옴표·줄바꿈·`$`가
그대로 남습니다):

```bash
cat > "$REPO_PIPELINE_DIR/task-summary.txt" <<'SUMMARY_EOF'
repo 구현과 deterministic verification 완료
<bugfix·hotfix면 재현 실패 출력과 수정 뒤 통과 출력 원문>
SUMMARY_EOF
jq -n \
  --arg run_id "$PIPELINE_ID" \
  --arg repo_id "$REPO_ID" \
  --rawfile summary "$REPO_PIPELINE_DIR/task-summary.txt" \
  --arg criteria_digest "$(jq -r '.criteria_digest' "$REPO_PIPELINE_DIR/verification.json")" \
  '{run_id: $run_id, task_id: ("repo-final-" + $repo_id), repo_id: $repo_id, attempt: 1, status: "succeeded",
    summary: $summary, changed_files: ["repo-relative/path"], evidence_refs: ["verification.json", "change.diff"],
    verification: {criteria_digest: $criteria_digest, deterministic_passed: true, evaluator_status: "pending",
                   evidence_refs: ["verification.json"]},
    hypotheses_rejected: [], remaining_unknowns: []}' > "$REPO_PIPELINE_DIR/task-report-input.json"

pylon internal trajectory task-report \
  --input "$REPO_PIPELINE_DIR/task-report-input.json" \
  --output "$REPO_PIPELINE_DIR/task-report.json"
```

### Step 7B: 격리 evaluator

deterministic gate가 통과한 repo만 새 evaluator context로 검토합니다. 구현 agent의 대화·memory·prompt 없이
Pylon이 허용한 증거만 read-only bundle로 복사합니다:

`verification.json`에 `tree`가 있어야 evaluator가 커밋된 변경 전체를 봅니다. 없으면 `tree_reason`이 미커밋 변경일
때 Step 6B대로 정리하고 Step 7을 한 번 더 실행합니다(포매터가 고친 파일은 커밋하면 두 번째 실행에서 그대로
남습니다). 그래도 `tree`가 없으면 evaluator로 넘어가지 않고 `tree_reason`과 함께 멈춰 보고합니다:

```bash
BASE_REVISION=$(jq -r '.base_revision' "$REPO_PIPELINE_DIR/status.json")
VERIFIED_TREE=$(jq -r '.tree // empty' "$REPO_PIPELINE_DIR/verification.json")
git -C "$REPO" diff --binary "$BASE_REVISION" "$VERIFIED_TREE" > "$REPO_PIPELINE_DIR/change.diff"

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
DETERMINISTIC_PASSED=$(jq -r '.ok == true' "$REPO_PIPELINE_DIR/verification.json")
EVALUATOR_PASSED=$(jq -r '.status == "pass"' "$REPO_PIPELINE_DIR/evaluator-result.json")
VERIFIED=false
[[ "$DETERMINISTIC_PASSED" == "true" && "$EVALUATOR_PASSED" == "true" ]] && VERIFIED=true
```

`$VERIFIED` 값을 `$PIPELINE_DIR/execution-log.json`의 해당 repo 태스크 항목마다 `verified: true|false`로
기록합니다.

### Step 8: PR 생성 (선택)

`config.yml`의 `git.pr.auto_pr`이 `true`일 때만 실행합니다(기본 비활성, 수동 생성은 `/pl:pr`). repo마다
`/pl:pr`(`.pylon/builtin/commands/pl-pr.md`) 2단계와 같은 대조 — 현재 브랜치, 검증 통과, 검증 tree와 브랜치 tree 일치,
추적 중인 파일의 미커밋 변경 없음 — 를 통과한 repo만 PR을 만듭니다. 통과하지 못한 repo는 건너뛰고 커밋·재검증
안내를 보고에 남깁니다:

```bash
source .pylon/builtin/scripts/bash/common.sh
AUTO_PR=$(config_get "git.pr.auto_pr" "false")
if [[ "$AUTO_PR" == "true" ]]; then
  jq -c '.sub_pipelines[]' "$ROOT_PIPELINE_DIR/status.json" | while IFS= read -r SUB; do
    REPO=$(echo "$SUB" | jq -r '.repo')
    REPO_PIPELINE_DIR=$(echo "$SUB" | jq -r '.pipeline_dir')
    TASK_BRANCH=$(echo "$SUB" | jq -r '.branch')
    VERIFIED_TREE=$(jq -r 'select(.ok == true) | .tree // empty' "$REPO_PIPELINE_DIR/verification.json")
    BRANCH_TREE=$(git -C "$REPO" rev-parse "$TASK_BRANCH^{tree}")
    UNCOMMITTED=$(git -C "$REPO" status --porcelain --untracked-files=no --ignore-submodules=untracked -- ':(top)' ':(top,glob,exclude)**/.pylon/**' ':(top,exclude).claude/worktrees')
    if [[ "$(git -C "$REPO" branch --show-current)" != "$TASK_BRANCH" || -z "$VERIFIED_TREE" \
          || "$VERIFIED_TREE" != "$BRANCH_TREE" || -n "$UNCOMMITTED" ]]; then
      echo "PR 보류: $REPO — 미커밋 변경이 있거나 검증한 tree가 $TASK_BRANCH와 다릅니다. /pl:pr로 진행하세요" >&2
      continue
    fi
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
