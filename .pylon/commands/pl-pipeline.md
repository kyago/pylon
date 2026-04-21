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

사용자의 요구사항을 받아 분석 → 설계 → 구현 → PR까지 전체 파이프라인을 실행합니다.

## 실행 단계

### Step 1: 파이프라인 초기화

Shell script로 git branch와 런타임 디렉토리를 생성합니다.

```bash
INIT_RESULT=$(.pylon/scripts/bash/init-pipeline.sh "$ARGUMENTS")
PIPELINE_ID=$(echo "$INIT_RESULT" | jq -r '.pipeline_id')
PIPELINE_DIR=$(echo "$INIT_RESULT" | jq -r '.pipeline_dir')
BRANCH=$(echo "$INIT_RESULT" | jq -r '.branch')
```

`$INIT_RESULT`에서 `pipeline_id`, `branch`, `pipeline_dir`을 추출합니다.

### Step 2: PO 요구사항 분석

Claude Code가 직접 PO 역할을 수행합니다.

1. `$PIPELINE_DIR/requirement.md`를 읽습니다
2. 요구사항을 분석하여 다음을 포함하는 `requirement-analysis.md`를 작성합니다:
   - 사용자 스토리 (As a... I want... So that...)
   - 수용 기준 (Acceptance Criteria)
   - 기능적/비기능적 요구사항 구분
   - 범위 밖 (Out of Scope) 항목
3. `$PIPELINE_DIR/requirement-analysis.md`에 저장합니다

### Step 3: 아키텍처 분석

`.pylon/agents/architect.md`를 읽어 에이전트 정의를 가져온 뒤 Agent 도구로 실행합니다.

```
// 1. 에이전트 정의 로드
ARCHITECT_DEF=$(cat .pylon/agents/architect.md)

// 2. 아키텍트 에이전트 실행
Agent(prompt="$ARCHITECT_DEF\n\n## 태스크\n다음 요구사항을 분석하고 아키텍처 설계를 작성하세요: [requirement-analysis.md 내용]\n코드베이스 구조를 파악하고 영향 받는 파일, 변경 사항, 새로 생성할 파일을 명시하세요.\n결과를 $PIPELINE_DIR/architecture.md에 저장하세요.")
```

에이전트 정의는 `.pylon/agents/architect.md`에서 읽어 프롬프트에 주입합니다.

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

1. 각 태스크에 ID, 제목, 설명, 담당 에이전트, 의존성을 부여합니다
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
      "dependencies": [],
      "status": "pending"
    }
  ]
}
```

### Step 6: 에이전트 병렬 실행

독립 태스크는 Agent 도구로 병렬 실행합니다.

```
// 의존성 없는 태스크를 동시에 실행
Agent(prompt="[에이전트 정의]\n\n## 태스크\n[T001 내용]", isolation="worktree")
Agent(prompt="[에이전트 정의]\n\n## 태스크\n[T002 내용]", isolation="worktree")
Agent(prompt="[에이전트 정의]\n\n## 태스크\n[T003 내용]", isolation="worktree")
```

에이전트 정의는 `.pylon/agents/{agent-name}.md`에서 읽어 프롬프트에 주입합니다.

### Step 7: 검증

```bash
.pylon/scripts/bash/run-verification.sh "$PIPELINE_DIR"
```

실패 시 에러를 분석하고 수정 후 재실행합니다.

### Step 8: PR 생성 (선택)

> PR 생성은 기본적으로 비활성화되어 있습니다.
> `config.yml`의 `git.pr.auto_pr: true` 설정 시에만 자동 실행됩니다.
> 수동으로 PR을 생성하려면 `/pl:pr` 커맨드를 사용하세요.

`config.yml`에서 `git.pr.auto_pr` 값을 읽어 `true`인 경우에만 실행합니다:

```bash
source .pylon/scripts/bash/common.sh
AUTO_PR=$(config_get "git.pr.auto_pr" "false")
if [[ "$AUTO_PR" == "true" ]]; then
  .pylon/scripts/bash/create-pr.sh "$PIPELINE_DIR" --branch "$BRANCH" --title "feat: [요구사항 요약]"
fi
```

### Step 9: 완료 보고

실행 결과를 요약합니다:
- 생성/변경된 파일 목록
- 테스트 결과
- PR URL (auto_pr 활성화 시)
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

## Crash Recovery

파이프라인 재실행 시 기존 산출물을 확인합니다:
- 이미 존재하는 산출물은 건너뜁니다
- `status.json`의 마지막 단계부터 재개합니다
