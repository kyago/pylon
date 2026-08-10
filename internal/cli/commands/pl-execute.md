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

4. 각 에이전트 완료 후 결과를 `execution-log.json`에 기록합니다.
   검증 출력이 첨부되지 않은 "완료" 보고는 완료로 처리하지 않습니다. 또한 provider의 자연어 응답을
   정본으로 사용하지 않고, task/attempt별 구조화 보고서를 Pylon으로 검증해 기록합니다:

```bash
TASK_REPORT_INPUT="$PIPELINE_DIR/tasks/$TASK_ID/attempts/$ATTEMPT/task-report-input.json"
TASK_REPORT="$PIPELINE_DIR/tasks/$TASK_ID/attempts/$ATTEMPT/task-report.json"

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

`task-report.json`은 create-once이며 digest가 포함됩니다. 같은 attempt의 내용을 나중에 덮어쓰지 말고,
재시도는 새 attempt 디렉토리에 기록합니다.
5. 에이전트 브랜치를 **같은 repo의** task 브랜치로 머지합니다. `$ARGUMENTS`가 root pipeline이면
   `status.json.sub_pipelines`에서 태스크의 `repo`와 일치하는 branch를 조회합니다:

```bash
.pylon/scripts/bash/merge-branches.sh "$TASK_BRANCH" "$AGENT_BRANCH_1" "$AGENT_BRANCH_2" --git-root "$REPO"
```

root pipeline에는 branch가 없으며, 서로 다른 repo의 branch를 한 merge 호출에 섞지 않습니다.
