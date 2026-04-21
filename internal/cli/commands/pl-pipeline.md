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

### Step 1: 파이프라인 초기화

Shell script로 git branch와 런타임 디렉토리를 생성합니다.

```bash
INIT_RESULT=$(.pylon/scripts/bash/init-pipeline.sh "$ARGUMENTS")
PIPELINE_ID=$(echo "$INIT_RESULT" | jq -r '.pipeline_id')
PIPELINE_DIR=$(echo "$INIT_RESULT" | jq -r '.pipeline_dir')
BRANCH=$(echo "$INIT_RESULT" | jq -r '.branch')
```

`$INIT_RESULT`에서 `pipeline_id`, `branch`, `pipeline_dir`을 추출합니다.

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
4. `detected_domain`에 따라 **도메인별 실행 절차**를 따릅니다:
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

`config.yml`에서 `git.pr.auto_pr`이 `true`인 경우에만 실행합니다:

```bash
.pylon/scripts/bash/create-pr.sh "$PIPELINE_DIR" --branch "$BRANCH" --title "feat: [요구사항 요약]"
```

### Step 9: 완료 보고

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
- 3회 연속 실패 시 사용자에게 보고합니다
- `status.json`에 현재 단계와 에러를 기록합니다

## Crash Recovery

파이프라인 재실행 시 기존 산출물을 확인합니다:
- 이미 존재하는 산출물은 건너뜁니다
- `status.json`의 마지막 단계부터 재개합니다
