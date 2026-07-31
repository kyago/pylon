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
   검증 출력이 첨부되지 않은 "완료" 보고는 완료로 처리하지 않습니다
5. 에이전트 브랜치를 task 브랜치로 머지합니다:

```bash
.pylon/scripts/bash/merge-branches.sh "$BRANCH" "$AGENT_BRANCH_1" "$AGENT_BRANCH_2"
```
