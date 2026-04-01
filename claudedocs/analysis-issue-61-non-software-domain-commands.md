# Issue #61 분석: 비소프트웨어 도메인 슬래시 커맨드 추가

> **이슈**: https://github.com/kyago/pylon/issues/61
> **관련**: PR #58 (멀티도메인 오케스트레이션 — Harness 도메인 영역 흡수)
> **분석일**: 2026-04-01
> **상태**: 분석 완료, 구현 미착수

---

## 1. 이슈 요약

PR #58에서 멀티도메인 오케스트레이션 인프라를 구축했으나, 비소프트웨어 도메인(research, content, marketing)의 **실행 경로가 연결되지 않은** 후속 이슈.

### 이슈에 명시된 작업 항목

1. `/pl:pipeline --workflow=research` 파라미터를 `init-pipeline.sh`에서 실제 처리
2. `routing-decision.json` 생성 로직 구현
3. PO 프롬프트 스킬 목록에 워크플로우 선택 가이드 보강

---

## 2. 현재 상태 (Gap 분석)

### 2.1 구현 완료된 항목 (PR #58)

| 항목 | 위치 | 상태 |
|------|------|------|
| PO 프롬프트 도메인 감지 테이블 | `launch.go:340-350` | 4개 도메인 + 키워드 + 워크플로우 + 에이전트 매핑 완료 |
| 도메인별 파이프라인 설명 | `launch.go:352-361` | research/content/marketing 파이프라인 흐름 기술 완료 |
| 워크플로우 YAML 템플릿 | `internal/workflow/templates/` | `research.yml`, `content.yml`, `marketing.yml` Go binary에 embedded |
| 비소프트웨어 에이전트 15종 | `.pylon/agents/` | research 5종, content 5종, marketing 5종 |
| 스킬 3종 | `.pylon/skills/` | research-methodology, content-writing-guide, marketing-framework |
| 에이전트 도메인 그룹핑 | `launch.go:398-422` | `discoverAgentsByDomain()` 구현 완료 |
| PO 모호성 처리 지시 | `launch.go:349-350` | "확신이 없으면 사용자에게 확인", "혼합 작업은 단계별 전환" |

### 2.2 미구현 (갭)

| 항목 | 위치 | 상태 | 비고 |
|------|------|------|------|
| `init-pipeline.sh` `--workflow` 파싱 | `.pylon/scripts/bash/init-pipeline.sh` | 미구현 | 접근 A에서는 불필요 (아래 참조) |
| `routing-decision.json` 생성 | 없음 | 미구현 | 제안서에만 설계 존재 |
| PO 스킬 목록 비소프트웨어 확장 | `launch.go:380-390` | 미구현 | 소프트웨어 커맨드만 나열 |
| `pl-pipeline.md` 도메인별 실행 분기 | `pl-pipeline.md:34-83` | 미구현 | Step 2에 도메인 라우팅 테이블(lines 25-32)은 존재하나, Steps 3-9가 이를 무시하고 소프트웨어 전용으로 하드코딩 — 파일 내부 불일치 |
| `SuggestWorkflow()` 비소프트웨어 키워드 | `selector.go:12-47` | 미구현 | **프로덕션 데드 코드** (아래 참조) |

### 2.3 발견된 버그/이슈

**`"research"` 키워드 오라우팅** (`selector.go:36`)
- `"research"`, `"조사"`, `"분석"` 키워드가 `explore` 워크플로우에 매핑되어 있음
- 비소프트웨어 리서치 요청이 소프트웨어 `explore` 파이프라인으로 잘못 라우팅됨
- 단, `SuggestWorkflow()`가 프로덕션에서 호출되지 않으므로 현재 실질적 영향 없음

**`SuggestWorkflow()`는 프로덕션 데드 코드**
- `selector_test.go`에서만 호출됨
- 제안서에서 언급된 `request.go`는 실제 파일로 존재하지 않음
- 테스트는 통과하지만 런타임에 아무 영향 없음

---

## 3. 접근 방식 비교

### 3.1 접근 A: PO 중심 라우팅 (LLM 판단) — 채택

PO(LLM)가 요구사항을 의미적으로 분석하여 도메인을 결정하고, `routing-decision.json`을 직접 작성.

```
사용자 → "요청" → PO(LLM)가 도메인 판단 → routing-decision.json 작성 → 도메인별 파이프라인 실행
```

**장점**:
- 의미 파악은 LLM의 핵심 역량 — 키워드 매칭으로 불가능한 의도 파악 가능
- "AI 트렌드 분석해서 블로그 글 써줘" → research → content 멀티도메인 처리 가능
- 기존 PO 프롬프트 인프라의 90%가 이미 구축됨
- 변경 범위 최소 (2-3개 파일, Markdown 중심)
- OMC Ralph의 "단일진입점 자동라우팅" 철학과 일치
- 키워드 추가마다 코드 배포 불필요 — 프롬프트 수정으로 대응

**단점**:
- LLM 판단은 비결정론적 — 동일 요청에 다른 라우팅 가능
- 디버깅 시 LLM 추론 과정 추적 필요
- 모델 버전 변경 시 라우팅 동작 변화 가능

### 3.2 접근 B: Go CLI 중심 라우팅 (키워드 매칭) — 미채택

`SuggestWorkflow()`를 확장하여 코드 레벨에서 결정론적으로 라우팅.

```
사용자 → "요청" → SuggestWorkflow() 키워드 매칭 → init-pipeline.sh --workflow=X → 파이프라인 실행
```

**장점**:
- 결정론적, 테스트 가능 (`selector_test.go` 기반)
- 잘못된 라우팅을 키워드 diff로 디버깅 가능
- 모델 비의존적

**단점**:
- 단어를 보지만 의도를 이해하지 못함
- `explore` 키워드 버킷이 비소프트웨어 요청을 가로채는 문제 이미 존재
- 혼합 도메인/모호한 요청 처리 불가
- `SuggestWorkflow()`가 프로덕션 데드 코드 — 연결 작업 추가 필요
- 키워드 테이블 유지보수 부담 증가

### 3.3 채택 근거

1. **도메인 라우팅은 의미 파악 태스크** — 키워드 매칭이 아닌 LLM 판단이 본질적으로 적합
2. **기존 인프라 정합성** — PO 프롬프트에 도메인 감지 로직이 이미 완비
3. **`SuggestWorkflow()` 실체** — 프로덕션 데드 코드이므로 확장 기반이 약함
4. **Pylon 설계 철학** — OMC Ralph 패턴 (단일진입점 → LLM 자동 판단 → 적절한 도구 호출)과 일치
5. **변경 최소화** — Markdown 수정 중심으로 2-3개 파일 변경

---

## 4. 구현 계획

### 4.1 P0: `pl-pipeline.md` 도메인별 실행 분기 추가

> **선행 조건**: §5.2 비소프트웨어 파이프라인 Steps 상세 설계가 완료되어야 함.
> P0를 §5.2 해결 없이 진행하면 "라우팅 테이블은 있지만 실행 경로가 없는" 상태가 반복됨.

**파일**: `internal/cli/commands/pl-pipeline.md`
**변경 내용**:
- Step 2에 `routing-decision.json` 생성 지시 추가
- Steps 3-9를 도메인별 조건부 분기로 확장
- 비소프트웨어 도메인별 실행 절차 정의

**현재 Step 2** (PO 요구사항 분석):
```
1. requirement.md를 읽습니다
2. requirement-analysis.md를 작성합니다
```

**변경 후 Step 2** (PO 요구사항 분석 + 도메인 라우팅):
```
1. requirement.md를 읽습니다
2. 도메인을 판단하여 routing-decision.json을 작성합니다:
   {"detected_domain": "...", "selected_workflow": "...", "reasoning": "...", "agents": [...]}
3. requirement-analysis.md를 작성합니다
4. detected_domain에 따라 도메인별 실행 절차를 따릅니다
```

**도메인별 Steps 3+ 분기**:
- `software` → 기존 Steps 3-9 유지 (아키텍처 → PM 분해 → 에이전트 실행 → 검증 → PR)
- `research` → 병렬 조사 에이전트 실행 → 교차 검증 → 보고서 작성 → 팩트 체크
- `content` → 초안 작성 → 편집/리뷰 루프 → 최종본
- `marketing` → 시장 조사 → 전략 수립 → 콘텐츠 생성 → 검증

### 4.2 P1: `launch.go` PO 프롬프트 스킬 목록 보강

**파일**: `internal/cli/launch.go:380-390`
**변경 내용**:
- 현재 소프트웨어 전용 스킬 목록에 도메인 라우팅 안내 추가
- PO가 도메인 라우팅의 주체임을 명시
- `/pl:pipeline`이 모든 도메인의 범용 진입점임을 강조

### 4.3 P2: `SuggestWorkflow()` 처리 결정

**파일**: `internal/workflow/selector.go`
**옵션**:
- A) 현재 상태 유지 (데드 코드, 향후 CLI `--workflow` 오버라이드에 활용 가능)
- B) 제거 (데드 코드 정리)
- C) `--workflow` CLI 힌트로 연결하여 PO에게 제안값 제공 (하이브리드)

---

## 5. 미결 사항 (Open Questions)

### 5.1 routing-decision.json 소비자 문제

**문제**: PO가 `routing-decision.json`을 작성해도, 이를 읽어서 파이프라인 동작을 변경하는 코드가 없다. 제안서(`proposal-harness-domain-absorption.md:340-353`)에서 설계한 `loadWorkflowForPipeline()` 함수가 구현되지 않았다.

**선택지**:
- A) `routing-decision.json`은 순수 기록용 산출물로 사용 — PO가 자신이 작성한 파일을 참조하여 분기 (자기참조적이지만 LLM 컨텍스트에서 유효)
- B) 오케스트레이터(`pipeline.go`)에 `loadWorkflowForPipeline()` 구현하여 프로그래매틱하게 소비
- C) `pylon status` CLI에서만 읽어서 현재 워크플로우 표시에 사용

**현재 판단**: 접근 A에서는 **PO가 pl-pipeline.md의 지시에 따라 도메인별 분기를 직접 실행**하므로, routing-decision.json은 (A) 기록용 + (C) 상태 조회용으로 충분할 수 있다. 다만 오케스트레이터 연동(B)은 향후 자동화 수준 향상 시 필요.

### 5.2 비소프트웨어 파이프라인 Steps 상세 설계

**문제**: `pl-pipeline.md`에 도메인별 분기를 추가하려면, research/content/marketing 각각의 구체적 실행 단계가 정의되어야 한다.

**현재 상태**:
- 워크플로우 YAML(`research.yml`, `content.yml`, `marketing.yml`)에 스테이지 순서는 정의됨
- 그러나 각 스테이지에서 **어떤 에이전트를 어떤 프롬프트로 호출하는지**는 정의되지 않음
- 소프트웨어 도메인의 Steps 3-9처럼 구체적인 실행 지시가 필요

**필요 결정**:
- research 파이프라인: `fan_out` 스테이지에서 어떤 에이전트를 병렬 호출? 산출물 형식은?
- content 파이프라인: `validate` → `generate` 루프의 종료 조건은?
- marketing 파이프라인: `fan_out`/`fan_in` 패턴의 구체적 에이전트 배치는?

### 5.3 워크플로우 YAML과 실행의 불일치

**문제**: 워크플로우 YAML 템플릿은 Go 오케스트레이터가 소비하는 형식이지만, 접근 A에서 PO가 직접 분기하는 경우 YAML 파일은 사실상 참조되지 않는다.

**선택지**:
- A) YAML은 오케스트레이터 레이어용으로 유지, PO 레이어는 pl-pipeline.md의 자연어 지시로 운영 — 이중 정의
- B) YAML을 PO가 읽을 수 있는 형태로 변환하여 pl-pipeline.md에서 참조
- C) 장기적으로 오케스트레이터가 routing-decision.json + YAML로 자동 실행 (접근 B와 수렴)

### 5.4 사용자 오버라이드 메커니즘

**문제**: PO가 잘못된 도메인을 선택했을 때 사용자가 교정하는 방법이 불명확.

**선택지**:
- A) 사용자가 자연어로 "아니, 이건 리서치가 아니라 마케팅이야"라고 말하면 PO가 수정
- B) `/pl:pipeline --workflow=marketing` 구문으로 명시적 오버라이드 (이 경우 init-pipeline.sh 변경 필요)
- C) routing-decision.json을 사용자가 직접 수정 후 재실행

**현재 판단**: (A)가 접근 A의 철학과 일치. (B)는 하이브리드 접근 시 추가.

### 5.5 `SuggestWorkflow()`의 장기 운명

**문제**: 프로덕션 데드 코드를 유지할 것인지.

- 제거하면 코드 정리 효과
- 유지하면 향후 CLI `--workflow` 힌트 경로에 활용 가능 (하이브리드 접근)
- `explore` 워크플로우의 `"research"` 키워드 오매핑은 데드 코드이므로 현재 무해

### 5.6 PR #63 멀티도메인 템플릿과의 관계

**문제**: 최근 머지된 PR #63("도메인 지식 템플릿 멀티도메인 대응")에서 도메인 지식 파일(`overview.md`, `practices.md`, `glossary.md`)을 범용화했다. 이 변경이 비소프트웨어 파이프라인 구현에 어떤 영향을 주는지 명확히 해야 한다.

**현재 판단**: PR #63은 `.pylon/domain/` 템플릿의 범용화로, 파이프라인 실행 경로와는 직교(orthogonal). 다만 비소프트웨어 도메인 파이프라인 실행 시 해당 도메인 지식 파일을 에이전트 프롬프트에 주입하는 흐름은 PR #63 결과물과 자연스럽게 연결됨.

### 5.7 LLM 라우팅 테스트 전략

**문제**: 접근 A(PO 중심 라우팅)의 최대 약점인 비결정론에 대한 품질 보증 방안이 부재.

**필요 사항**:
- 라우팅 정확도를 어떻게 측정할 것인가?
- 모델 버전 업데이트 시 라우팅 동작 변경을 어떻게 감지할 것인가?
- 최소한 골든 테스트 셋(예: "AI 트렌드 조사" → research, "로그인 버그 수정" → software)이 필요

**선택지**:
- A) routing-decision.json 산출물을 수집하여 사후적으로 정확도 모니터링
- B) 골든 테스트 셋을 정의하고 모델 버전 변경 시 수동 검증
- C) `SuggestWorkflow()`를 PO 판단의 사전 힌트(sanity check)로 활용 — PO와 키워드 매칭이 불일치하면 경고

**현재 판단**: (A)+(B) 조합이 현실적. routing-decision.json이 자연스러운 감사 로그 역할을 하며, 골든 테스트 셋으로 주기적 검증 가능.

### 5.8 Step 2 기존 라우팅 테이블과의 관계

**문제**: `pl-pipeline.md`의 Step 2(lines 25-32)에는 이미 도메인별 라우팅 테이블과 `--workflow=research` 예시가 포함되어 있다. 4.1항에서 제안한 `routing-decision.json` 작성 지시를 Step 2에 추가할 때, 기존 라우팅 테이블과의 관계를 정리해야 한다.

**선택지**:
- A) 기존 테이블을 유지하고, `routing-decision.json` 작성을 추가 지시로 삽입 — 테이블은 참조용, JSON은 실행용
- B) 기존 테이블을 `routing-decision.json` 생성 지시로 대체 — 중복 제거
- C) 기존 테이블을 PO의 판단 기준으로 유지하되, 이를 기반으로 JSON을 출력하도록 흐름 명시

**현재 판단**: (C)가 가장 자연스러움. 기존 테이블이 PO의 "참조 가이드"이고 routing-decision.json이 "판단 결과 산출물"이라는 역할 구분.

---

## 6. 변경 영향 범위

### 확정 변경

| 파일 | 변경 유형 | 영향도 |
|------|----------|--------|
| `internal/cli/commands/pl-pipeline.md` | Markdown 수정 — Step 2 확장 + 도메인별 분기 추가 | 높음 |
| `internal/cli/launch.go:380-390` | Go 코드 수정 — PO 스킬 목록 보강 | 중간 |

### 잠재적 변경 (미결 사항 해소 시)

| 파일 | 조건 | 변경 유형 |
|------|------|----------|
| `internal/workflow/selector.go` | 5.5항 결정 시 | 데드 코드 제거 또는 explore 키워드 정리 |
| `internal/orchestrator/pipeline.go` | 5.1항에서 B 선택 시 | `loadWorkflowForPipeline()` 추가 |
| `.pylon/scripts/bash/init-pipeline.sh` | 5.4항에서 B 선택 시 | `--workflow` 파라미터 파싱 추가 |

---

## 7. 참조

### 소스 코드
- `internal/cli/launch.go:308-430` — `buildRootCLAUDEMD()` PO 프롬프트 생성
- `internal/cli/launch.go:512-592` — `buildSlashCommands()` 슬래시 커맨드 생성
- `internal/cli/commands/pl-pipeline.md` — 파이프라인 슬래시 커맨드 정의
- `internal/workflow/selector.go` — `SuggestWorkflow()` 키워드 매칭 (프로덕션 데드 코드)
- `internal/workflow/templates/*.yml` — 워크플로우 YAML 템플릿 (Go binary embedded)
- `.pylon/scripts/bash/init-pipeline.sh` — 파이프라인 초기화 셸 스크립트

### 설계 문서
- `claudedocs/proposal-harness-domain-absorption.md` §3.4 — routing-decision.json 설계, init-pipeline.sh 변경 설계, loadWorkflowForPipeline() 설계
- `docs/proposals/pylon-enhancement-strategy.md` — 워크플로우 선택 전략
- `docs/proposals/adaptive-workflow-with-swarm-pattern.md` — 적응형 워크플로우 패턴
