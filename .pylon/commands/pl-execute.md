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

```
Agent(prompt="[에이전트 정의]\n\n## 태스크\n[태스크 내용]", isolation="worktree")
```

4. 각 에이전트 완료 후 결과를 `execution-log.json`에 기록합니다
5. 에이전트 브랜치를 task 브랜치로 머지합니다:

```bash
.pylon/scripts/bash/merge-branches.sh "$BRANCH" "$AGENT_BRANCH_1" "$AGENT_BRANCH_2"
```
