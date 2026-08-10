---
description: "에이전트 병렬 실행"
---

# Agent Execution

분해된 태스크를 에이전트에 배정하여 병렬 실행합니다.

## 입력
- `$ARGUMENTS`: 파이프라인 디렉토리 경로

## 실행

1. `tasks.json`을 읽습니다
2. `.pylon/agents/` 에이전트 정의를 읽습니다
3. 의존성 그래프를 분석하여 wave 단위로 실행합니다:
   - Wave 1: 의존성 없는 태스크 (병렬 Agent 호출)
   - Wave 2: Wave 1에 의존하는 태스크
   - ...

실행 전에 durable state store를 초기화하고, 재시작한 실행이면 먼저 event log와 만료 lease를 복구합니다.
상태 명령은 반드시 workspace root에서 실행합니다:

```bash
RUN_ID=$(jq -r '.root_pipeline_id // .pipeline_id // empty' "$PIPELINE_DIR/status.json")
[[ -n "$RUN_ID" ]] || RUN_ID=$(basename "$PIPELINE_DIR")

pylon internal state create-run "$RUN_ID" --requirement "$(cat "$PIPELINE_DIR/requirement.md")"
pylon internal state recover "$RUN_ID"
```

각 태스크는 `Agent(...)`를 호출하기 **전에** state store에 생성·ready·claim·start되어야 합니다. provider 응답을
받은 뒤에 뒤늦게 상태를 만드는 것은 금지합니다. 태스크별 입력과 lease/fencing token을 다음처럼 준비합니다:

```bash
TASK_STATE_DIR="$PIPELINE_DIR/tasks/$TASK_ID/state"
mkdir -p "$TASK_STATE_DIR"
TASK_SPEC_SOURCE="$TASK_STATE_DIR/task.json"
jq --arg task_id "$TASK_ID" '.tasks[] | select(.id == $task_id)' \
  "$PIPELINE_DIR/tasks.json" > "$TASK_SPEC_SOURCE"

jq --arg run_id "$RUN_ID" \
   --arg task_id "$TASK_ID" \
   --arg repo_id "$REPO_ID" \
   --arg worktree_path "$REPO" \
   --arg base_revision "$BASE_REVISION" \
   --arg artifact_dir "$PIPELINE_DIR/tasks/$TASK_ID" \
   '{
      schema_version: 1,
      run_id: $run_id,
      task_id: $task_id,
      repo_id: $repo_id,
      worktree_path: $worktree_path,
      base_revision: $base_revision,
      acceptance_criteria: (.acceptance_criteria // []),
      required_capabilities: {
        read_files: true, edit_files: true, run_shell: true,
        structured_output: true, worktree_isolation: false
      },
      artifact_dir: $artifact_dir,
      dependencies: (.dependencies // [])
    }' "$TASK_SPEC_SOURCE" > "$TASK_STATE_DIR/spec.json"

if ! TASK_STATE=$(pylon internal state show "$RUN_ID" "$TASK_ID" 2>/dev/null); then
  pylon internal state create-task --spec "$TASK_STATE_DIR/spec.json"
  pylon internal state ready "$RUN_ID" "$TASK_ID"
  TASK_STATE=$(pylon internal state show "$RUN_ID" "$TASK_ID")
fi

case $(printf '%s' "$TASK_STATE" | jq -r '.status') in
  ready)
    CLAIM_JSON=$(pylon internal state claim "$RUN_ID" "$TASK_ID" --owner "orchestrator-$$" --lease 10m)
    ;;
  interrupted)
    CLAIM_JSON=$(pylon internal state retry "$RUN_ID" "$TASK_ID" --owner "orchestrator-$$" --lease 10m)
    ;;
  *)
    echo "task is not claimable; inspect state before launching provider: $TASK_ID" >&2
    exit 1
    ;;
esac
FENCE=$(printf '%s' "$CLAIM_JSON" | jq -r '.lease.fencing_token')
ATTEMPT=$(printf '%s' "$CLAIM_JSON" | jq -r '.lease.attempt')

cat > "$TASK_STATE_DIR/worker-handle.json" <<JSON
{"provider":"session-native","external_id":"$RUN_ID-$TASK_ID-$ATTEMPT","attempt":$ATTEMPT}
JSON
pylon internal state start "$RUN_ID" "$TASK_ID" --fence "$FENCE" --handle "$TASK_STATE_DIR/worker-handle.json"
```

async provider는 실행 중 5분 이내 간격으로 아래 heartbeat를 갱신합니다. 프로세스가 재시작되면 먼저
`state recover`를 실행하고, provider가 resume token을 지원할 때만 `state resume`를 사용합니다. 그 외에는
`state retry`로 새 attempt/fencing token을 발급받아 다시 시작합니다.

```bash
pylon internal state heartbeat "$RUN_ID" "$TASK_ID" --fence "$FENCE" --lease 10m
```

서브 에이전트는 이 대화도 파이프라인 산출물도 보지 못합니다. 프롬프트에 **설계 근거·수용 기준·대상
파일·검증 명령·범위 밖**을 모두 내용째로 넣습니다(경로만 넘기지 않습니다):

```
Agent(prompt="[에이전트 정의]

## 설계 근거
[architecture.md 중 해당 섹션 전문]

## 수용 기준
[requirement-analysis.md의 해당 AC]

## 태스크
[tasks.json 항목 전문]

## 대상
repo / base 브랜치 / 수정 대상 파일 절대 경로

## 검증
완료 후 다음을 직접 실행하고 출력 전문을 보고하세요: [빌드/테스트/린트 명령]

## 범위 밖
[건드리지 말 것]", isolation="worktree")
```

`isolation="worktree"`는 의존성 설치가 필요 없고 파일이 겹치지 않는 태스크에만 사용합니다.

4. 각 에이전트 완료 후 provider 결과를 fencing token과 함께 먼저 확정한 다음 `execution-log.json`에 기록합니다.
   검증 출력이 첨부되지 않은 "완료" 보고는 완료로 처리하지 않습니다. 또한 provider의 자연어 응답을
   정본으로 사용하지 않고, task/attempt별 구조화 보고서를 Pylon으로 검증해 기록합니다:

```bash
TASK_RESULT="$PIPELINE_DIR/tasks/$TASK_ID/attempts/$ATTEMPT/provider-result.json"
TASK_REPORT_INPUT="$PIPELINE_DIR/tasks/$TASK_ID/attempts/$ATTEMPT/task-report-input.json"
TASK_REPORT="$PIPELINE_DIR/tasks/$TASK_ID/attempts/$ATTEMPT/task-report.json"

cat > "$TASK_RESULT" <<JSON
{
  "state": "succeeded|failed|cancelled|interrupted",
  "exit_code": 0,
  "changed_files": ["repo-relative/path"],
  "evidence": {"test_output": "tasks/$TASK_ID/attempts/$ATTEMPT/test-output.log"}
}
JSON

pylon internal state complete "$RUN_ID" "$TASK_ID" --fence "$FENCE" --result "$TASK_RESULT"

cat > "$TASK_REPORT_INPUT" <<JSON
{
  "run_id": "$(basename "$PIPELINE_DIR")",
  "task_id": "$TASK_ID",
  "repo_id": "$REPO_ID",
  "attempt": $ATTEMPT,
  "status": "succeeded|failed|cancelled|interrupted",
  "summary": "작업 결과 요약",
  "provider": {"name": "$PROVIDER", "capabilities": ["read_files", "edit_files", "run_shell"]},
  "required_capabilities": ["read_files", "edit_files", "run_shell"],
  "changed_files": ["repo-relative/path"],
  "evidence_refs": ["tasks/$TASK_ID/attempts/$ATTEMPT/test-output.log"],
  "hypotheses_rejected": [
    {"hypothesis": "배제한 원인", "probe": "확인 방법", "result": "배제 근거"}
  ],
  "remaining_unknowns": []
}
JSON

pylon internal trajectory task-report --input "$TASK_REPORT_INPUT" --output "$TASK_REPORT"
```

`state complete`가 stale fencing token, 만료 lease, 잘못된 transition으로 실패하면 trajectory 보고서를 쓰지
않습니다. 성공한 worker는 `verifying` 상태에 남으며, deterministic gate와 격리 evaluator가 모두 끝난 뒤
상위 pipeline이 `state verify`로만 `succeeded`를 확정합니다.

`task-report.json`은 create-once이며 digest가 포함됩니다. 같은 attempt의 내용을 나중에 덮어쓰지 말고,
재시도는 새 attempt 디렉토리에 기록합니다.
5. 에이전트 브랜치를 **같은 repo의** task 브랜치로 머지합니다. `$ARGUMENTS`가 root pipeline이면
   `status.json.sub_pipelines`에서 태스크의 `repo`와 일치하는 branch를 조회합니다:

```bash
.pylon/scripts/bash/merge-branches.sh "$TASK_BRANCH" "$AGENT_BRANCH_1" "$AGENT_BRANCH_2" --git-root "$REPO"
```

root pipeline에는 branch가 없으며, 서로 다른 repo의 branch를 한 merge 호출에 섞지 않습니다.
