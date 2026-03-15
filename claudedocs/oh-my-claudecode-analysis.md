# oh-my-claudecode 분석 보고서 — Pylon 프로젝트 관점

> 분석 일자: 2026-03-10
> 대상: [oh-my-claudecode](https://github.com/Yeachan-Heo/oh-my-claudecode) v4.7.9
> 목적: Pylon 프로젝트에서 참조/도입할 패턴 및 접근법 평가

---

## 1. 프로젝트 개요

### 1.1 oh-my-claudecode (OMC) 기본 정보

| 항목 | 내용 |
|------|------|
| **이름** | oh-my-claudecode (OMC) |
| **npm 패키지** | `oh-my-claude-sisyphus` |
| **언어** | TypeScript (82%) |
| **라이선스** | MIT |
| **GitHub Stars** | 9,044 |
| **커밋 수** | 1,818 |
| **버전** | 4.7.9 |
| **형태** | Claude Code 플러그인 (마켓플레이스 배포) |

### 1.2 OMC 핵심 비전

"Teams-first Multi-agent orchestration for Claude Code" — 제로 설정으로 동작하는 멀티 에이전트 오케스트레이션 시스템. Claude Code의 네이티브 기능(Team, Agent Tool, hooks)을 극대화하는 플러그인 형태로 제공.

### 1.3 Pylon 기본 정보 (비교 기준)

| 항목 | 내용 |
|------|------|
| **이름** | Pylon |
| **언어** | Go 1.24+ |
| **형태** | 독립 CLI 바이너리 (오케스트레이터) |
| **핵심 아키텍처** | 외부 프로세스로 Claude Code CLI 인스턴스를 관리하는 메타-오케스트레이터 |
| **상태 관리** | SQLite (WAL) + 파일 기반 inbox/outbox |
| **에이전트 격리** | git worktree + tmux 세션 |
| **파이프라인** | PO → Architect → PM → Developer → QA → PR |

---

## 2. 근본적 설계 철학 비교

### 2.1 아키텍처 패러다임

| 측면 | Pylon | OMC |
|------|-------|-----|
| **관계** | Claude Code의 **외부 오케스트레이터** | Claude Code의 **내부 플러그인** |
| **언어** | Go (단일 바이너리) | TypeScript (npm 패키지) |
| **에이전트 실행** | 별도 tmux 세션에서 Claude Code CLI 프로세스 실행 | Claude Code 내부의 Agent Tool/Team 기능 활용 |
| **통신** | 파일 기반 inbox/outbox + SQLite 큐 | Claude Code 네이티브 Team/Agent 도구 |
| **격리** | git worktree per agent (OS 레벨) | Claude Code 세션 내부 논리적 분리 |
| **상태 영속** | SQLite WAL + SPOF 복구 | 파일 시스템 (.omc/state/) |
| **프로젝트 구조** | 모노레포 + 서브모듈 지원 | 단일 프로젝트 |

**핵심 차이**: Pylon은 Claude Code 위에 있는 **메타-오케스트레이터**이고, OMC는 Claude Code 안에 있는 **플러그인**이다. Pylon은 여러 프로젝트를 동시에 다루는 팀 전체를 관리하지만, OMC는 단일 Claude Code 세션 내에서의 에이전트 협업에 집중한다.

### 2.2 파이프라인 비교

**Pylon 파이프라인**:
```
User Requirement
  → [1] PO Dialog (요구사항 분석/명확화)
  → [2] Architect Analysis (기술 방향/크로스 프로젝트 의존성)
  → [3] PM Decomposition (작업 분해/에이전트 배정)
  → [4] Parallel Implementation (프로젝트별 에이전트 실행)
  → [5] Cross-Validation (빌드/테스트/린트)
  → [6] PR Creation (GitHub PR + 리뷰어 지정)
  → [7] PO Validation (수용 기준 검증)
  → [8] Wiki Update (도메인 지식 자동 갱신)
```

**OMC Autopilot 파이프라인**:
```
Idea
  → Phase 0: Expansion (아이디어 → 상세 스펙)
  → Phase 1: Planning (스펙 → 구현 계획, 아키텍트+크리틱 검증)
  → Phase 2: Execution (Ralph + Ultrawork로 병렬 구현)
  → Phase 3: QA (빌드/린트/테스트 최대 5회 반복)
  → Phase 4: Validation (아키텍트/보안/코드리뷰 병렬 검증)
  → Phase 5: Cleanup (상태 파일 정리)
```

**비교 분석**:
- Pylon은 PO-중심 대화형 요구사항 발굴 + 멀티 프로젝트 지원이 고유
- OMC는 실행 단계에서 Ralph(완료 보장) + Ultrawork(최대 병렬성) 결합이 고유
- 양쪽 모두 "분석 → 계획 → 실행 → 검증" 구조이나, Pylon은 더 상위 수준(팀/프로젝트 관리), OMC는 더 실행 수준(코드 작성/검증)

---

## 3. OMC 핵심 기능 상세 (Pylon에 참조 가치 있는 것 중심)

### 3.1 에이전트 모델 티어링

OMC는 32개 에이전트를 3단계 모델 티어로 분류:

| 도메인 | LOW (Haiku) | MEDIUM (Sonnet) | HIGH (Opus) |
|--------|-------------|-----------------|-------------|
| 분석 | architect-low | architect-medium | architect |
| 실행 | executor-low | executor | executor-high |
| 보안 | security-reviewer-low | - | security-reviewer |
| 테스트 | test-engineer (haiku) | test-engineer | - |

**Pylon 참조 포인트**: Pylon의 `config.yml`에 에이전트별 `model` 필드가 이미 있지만, **작업 복잡도에 따른 동적 모델 라우팅**은 구현되지 않음. OMC의 위임 카테고리 시스템(`quick` → Haiku, `ultrabrain` → Opus)을 참조할 수 있음.

### 3.2 Deep Interview (모호성 게이팅)

OMC의 가장 혁신적인 스킬. 소크라틱 질문 루프에 수학적 측정을 결합:

```
초기화 → 소크라틱 질문 루프 → 모호성 점수 계산 → 실행 게이팅

- 6개 명확도 차원에 가중치 부여:
  scope(범위), behavior(행동), constraints(제약),
  integration(통합), edge_cases(경계조건), acceptance(수용기준)
- 가장 약한 차원을 대상으로 질문 생성
- 매 답변 후 모호성 점수 계산 및 표시
- 모호성 <= 20%까지 실행 차단
- 브라운필드/그린필드 자동 감지
```

**Pylon 참조 포인트**: Pylon의 PO Dialog 단계(`internal/orchestrator/conversation.go`)에서 이 모호성 점수링 시스템을 도입하면, PO 에이전트가 요구사항을 충분히 명확화했는지 정량적으로 판단할 수 있음.

### 3.3 Ralph (완료 보장 루프)

PRD 기반 반복 검증으로 작업 완료를 보장하는 메커니즘:

- `prd.json`으로 각 사용자 스토리의 수용 기준 관리
- `progress.txt`로 반복 간 학습 내용 축적
- 모든 스토리 `passes: true` + 아키텍트 검증까지 반복
- 3회 동일 에러 시 circuit breaker 발동

**Pylon 참조 포인트**: Pylon은 `max_attempts: 2`로 검증 실패 시 재시도하지만, Ralph처럼 PRD 기반으로 각 수용 기준을 개별 추적하고, 학습 내용을 반복 간 축적하는 구조는 없음. `internal/orchestrator/verify.go`에서 참조 가능.

### 3.4 Circuit Breaker 패턴

여러 에이전트/스킬에서 공통 적용되는 안전장치:

- 3회 동일 에러 반복 시 접근법 자체를 의문시
- 아키텍처 수준에서 재검토 트리거
- 무한 루프 방지

**Pylon 참조 포인트**: Pylon의 `max_attempts`는 단순 카운터인데, circuit breaker는 **동일 에러 패턴**을 감지하여 단순 재시도가 아닌 접근 자체를 변경하도록 유도함. 더 지능적인 재시도 전략.

### 3.5 7단계 검증 프로토콜

```
BUILD → TEST → LINT → FUNCTIONALITY → ARCHITECT → TODO → ERROR_FREE
```

- 5분 이내 신선도 요구 (캐시된 결과 불허)
- 실제 명령 출력 증거 포함 필수
- 각 단계 통과 시에만 다음 단계 진행

**Pylon 참조 포인트**: Pylon의 `verify.yml`은 build/test/lint 3단계인데, OMC는 기능 검증, 아키텍트 리뷰, TODO 검사, 에러프리 검증까지 7단계로 확장. 특히 **FUNCTIONALITY**와 **ARCHITECT** 단계가 Pylon에 없는 영역.

### 3.6 에이전트 READ-ONLY 패턴

Architect 에이전트에 Write/Edit 도구를 차단하여 순수 분석 전용으로 운용:

```markdown
---
name: architect
model: claude-opus-4-6
disallowedTools: Write, Edit
---
```

- 모든 발견 사항에 `file:line` 참조 필수
- 증상이 아닌 근본 원인 식별 요구
- "일반적인 조언" 차단, 코드 근거 필수

**Pylon 참조 포인트**: Pylon의 Architect 에이전트 정의(`.pylon/agents/architect.md`)에 `disallowedTools` 필드를 도입하여, 분석과 실행의 분리를 강제할 수 있음. 이미 `tools` 필드가 있으므로 `disallowedTools` 추가는 자연스러움.

### 3.7 훅 시스템 (31개)

Claude Code의 라이프사이클 이벤트에 개입:

| 이벤트 | 훅 | 용도 |
|--------|-----|------|
| **PreToolUse** | pre-tool-enforcer | 도구 사용 전 검증 (예: architect의 Write 차단) |
| **PostToolUse** | post-tool-verifier | 도구 사용 후 결과 검증 |
| **SubagentStart/Stop** | subagent-tracker | 서브에이전트 생명주기 추적 |
| **PreCompact** | pre-compact | 컨텍스트 컴팩트 전 상태 보존 |

**Pylon 참조 포인트**: Pylon은 외부 오케스트레이터이므로 Claude Code 내부 훅에 직접 개입하기 어렵지만, 에이전트의 `.claude/settings.json`에 훅을 주입하여 간접적으로 활용할 수 있음. 특히 `SubagentStart/Stop` 추적은 Pylon의 에이전트 모니터링에 유용.

### 3.8 Notepad Wisdom 시스템

계획별 지식을 4개 카테고리로 구조화:

```
.omc/notepads/{plan-name}/
├── learnings.md   # 패턴, 관습, 성공적 접근법
├── decisions.md   # 아키텍처 선택과 근거
├── issues.md      # 문제와 차단 요소
└── problems.md    # 기술 부채와 함정
```

**Pylon 참조 포인트**: Pylon의 `.pylon/runtime/memory/`와 개념적으로 유사하나, OMC의 카테고리 분류가 더 구조화됨. Pylon의 memory 시스템(`internal/memory/manager.go`)에서 카테고리별 분류 체계를 참조할 수 있음.

### 3.9 Learner 스킬 (학습 추출)

대화에서 재사용 가능한 원칙/휴리스틱을 추출하는 체계:

- "5분 구글링으로 해결 가능?" → 추출 불가
- "이 코드베이스에 특정적?" → 추출 불가 시 중단
- "진짜 디버깅 노력이 필요했나?" → 아니면 중단
- 코드 스니펫이 아닌 **사고 원칙** 추출

**Pylon 참조 포인트**: Pylon의 Wiki 자동 업데이트(`wiki.auto_update: true`)와 결합하면, 단순 사실 기록을 넘어 프로젝트에서 학습한 원칙을 자동으로 축적할 수 있음.

### 3.10 AI Slop Cleaner

AI 생성 코드의 체계적 정리:
- 테스트 우선: 행동 변경 전 회귀 테스트 확보
- 삭제 우선: 추가보다 삭제 선호
- 5개 슬롭 카테고리: 중복코드, 죽은코드, 불필요한 추상화, 경계 침범, 약한 테스트

**Pylon 참조 포인트**: 멀티 에이전트가 코드를 생성하면 품질 불균일 문제가 필연적. Pylon의 Cross-Validation 단계 후에 AI Slop Cleaner 패턴의 전용 에이전트를 추가하는 것을 검토할 수 있음.

### 3.11 벤치마크 시스템

에이전트 품질 측정을 위한 자체 벤치마크:

```
benchmarks/
├── code-reviewer/   # SQL 인젝션, 결제 환불 등 시나리오
├── debugger/        # Redis 장애, TS 빌드 에러 등 시나리오
├── executor/        # 타임스탬프 추가, 입력 검증 등 시나리오
└── harsh-critic/    # 계획/코드/분석 비평 시나리오
```

ground-truth JSON과 비교하는 자동 채점.

**Pylon 참조 포인트**: Pylon은 아직 에이전트 프롬프트 품질을 측정하는 벤치마크가 없음. 에이전트 정의를 반복 개선할 때 필수적인 피드백 루프.

### 3.12 합의 계획 (Ralplan)

Planner → Architect → Critic의 삼각 검증:
- 최대 5회 반복
- Architect는 가장 강력한 반론(steelman antithesis) 제시 필수
- Critic은 APPROVE/ITERATE/REJECT 판정

**Pylon 참조 포인트**: Pylon의 `PO → Architect → PM` 흐름과 유사하나, OMC는 Critic(비평가) 역할이 독립적. Pylon에서도 Architect의 분석 결과를 검증하는 별도 Critic 에이전트를 고려할 수 있음.

---

## 4. 도입 권장사항 (Pylon 프로젝트 관점, 우선순위별)

### P0: 즉시 도입 — 높은 가치, 낮은 노력

#### 4.1 PO Dialog에 모호성 점수 도입

**OMC 출처**: `skills/deep-interview/SKILL.md`

**현재 Pylon**: PO 에이전트가 사용자와 대화하여 요구사항을 명확화하지만, "충분히 명확해졌는지"의 판단 기준이 정성적.

**개선안**: PO 에이전트의 프롬프트에 6개 명확도 차원과 모호성 점수를 포함시켜, 점수가 임계값(예: 모호성 ≤ 20%) 이하일 때만 다음 단계(Architect)로 진행하도록 게이팅.

**적용 위치**: `.pylon/agents/po.md` 프롬프트 강화 + `internal/orchestrator/conversation.go`에 모호성 점수 파싱 로직 추가

#### 4.2 Circuit Breaker 패턴

**OMC 출처**: 여러 에이전트/스킬에서 공통 적용

**현재 Pylon**: `max_attempts: 2`로 단순 재시도 카운터만 존재.

**개선안**: 동일 에러 패턴 감지 → 3회 반복 시 접근법 변경을 유도. 에이전트에게 "이전과 다른 접근법을 시도하라"는 컨텍스트 전달.

**적용 위치**: `internal/orchestrator/verify.go`에 에러 패턴 비교 로직 추가

#### 4.3 에이전트 `disallowedTools` 필드 추가

**OMC 출처**: `agents/architect.md`

**현재 Pylon**: 에이전트 정의에 `tools` (허용 도구 목록)만 있음.

**개선안**: `disallowedTools` 필드를 추가하여 분석 전용 에이전트(Architect, PO)에서 Write/Edit를 명시적으로 차단. Claude Code CLI의 `--disallowedTools` 플래그로 전달.

**적용 위치**: `internal/config/agent.go`에 필드 추가 + `internal/agent/runner.go`에서 CLI 인수 생성 시 반영

### P1: 중기 도입 — 높은 가치, 중간 노력

#### 4.4 검증 프로토콜 확장 (7단계)

**OMC 출처**: `docs/ARCHITECTURE.md`

**현재 Pylon**: `verify.yml`로 build/test/lint 3단계 검증.

**개선안**:
```
BUILD → TEST → LINT → FUNCTIONALITY → ARCHITECT_REVIEW → TODO_CHECK → ERROR_FREE
```
특히 **FUNCTIONALITY**(기능 동작 확인)과 **ARCHITECT_REVIEW**(아키텍트 에이전트의 코드 리뷰) 단계 추가.

**적용 위치**: `internal/orchestrator/verify.go` 확장 + `.pylon/verify.yml` 스키마 확장

#### 4.5 PRD 기반 작업 추적

**OMC 출처**: `skills/ralph/SKILL.md`

**현재 Pylon**: PO Dialog에서 요구사항 확정 → PM이 작업 분해 → 완료 여부 확인.

**개선안**: 각 작업에 대해 수용 기준(acceptance criteria)을 구조화된 JSON으로 관리하고, 검증 단계에서 각 기준의 통과 여부를 개별 추적.

**적용 위치**: `internal/store/pipeline_state.go`에 수용 기준 테이블 추가 + `.pylon/tasks/` 형식 확장

#### 4.6 Notepad Wisdom 카테고리 체계

**OMC 출처**: `.omc/notepads/`

**현재 Pylon**: `.pylon/runtime/memory/`에 프로젝트 메모리를 저장하지만 카테고리 구분 없이 BM25 검색.

**개선안**: 메모리를 4개 카테고리(learnings, decisions, issues, problems)로 구조화하여 저장. 에이전트가 관련 카테고리의 메모리를 선택적으로 주입받도록 개선.

**적용 위치**: `internal/memory/manager.go`에 카테고리 필드 추가 + `internal/store/project_memory.go` 스키마 확장

#### 4.7 에이전트 벤치마크 시스템 구축

**OMC 출처**: `benchmarks/`

**현재 Pylon**: 에이전트 프롬프트 품질 측정 체계 없음.

**개선안**: 핵심 에이전트(PO, Architect, PM, Backend Dev)에 대한 벤치마크 시나리오 + ground-truth 작성. `pylon bench` 명령으로 에이전트 프롬프트 품질을 정량적으로 측정.

**적용 위치**: `benchmarks/` 디렉토리 + `internal/cli/bench.go` 명령 추가

### P2: 장기 검토 — 높은 가치, 높은 노력

#### 4.8 동적 모델 라우팅

**OMC 출처**: 위임 카테고리 시스템

**현재 Pylon**: `config.yml`에서 에이전트별 고정 모델 지정.

**개선안**: 작업 복잡도를 분석하여 모델을 동적으로 선택. 간단한 파일 수정 → Haiku, 아키텍처 분석 → Opus. 비용 최적화에 직결.

**적용 위치**: `internal/agent/runner.go`에 모델 선택 로직 + `internal/protocol/message.go`에 복잡도 필드

#### 4.9 Critic 에이전트 도입

**OMC 출처**: `skills/ralplan/SKILL.md`

**현재 Pylon**: Architect가 분석과 검증을 모두 담당.

**개선안**: Architect의 분석 결과를 독립적으로 검증하는 Critic 에이전트 추가. 특히 중요한 아키텍처 결정에서 "steelman antithesis"(가장 강력한 반론) 제시를 의무화.

**적용 위치**: `.pylon/agents/critic.md` 에이전트 정의 + 파이프라인에 검증 단계 삽입

#### 4.10 AI Slop Cleaner 에이전트

**OMC 출처**: `skills/ai-slop-cleaner/SKILL.md`

**현재 Pylon**: Cross-Validation은 빌드/테스트/린트 기반. 코드 품질 정리는 별도 없음.

**개선안**: 멀티 에이전트가 생성한 코드의 품질 불균일을 해결하는 전용 정리 에이전트. PR 생성 전에 실행하여 중복코드, 죽은코드, 불필요한 추상화를 제거.

**적용 위치**: `.pylon/agents/code-cleaner.md` + 파이프라인의 Validation 단계 후 Cleanup 단계 추가

#### 4.11 Claude Code 훅 주입

**OMC 출처**: 31개 훅 시스템

**현재 Pylon**: Claude Code CLI를 외부에서 실행하며, 내부 훅은 활용하지 않음.

**개선안**: Pylon이 에이전트별 `.claude/settings.json`을 동적으로 생성할 때, 훅을 포함시켜 도구 사용 제한, 상태 보고, 결과 검증 등을 Claude Code 레벨에서 강제.

**적용 위치**: `internal/agent/claudemd.go` 확장 (settings.json 생성 포함)

---

## 5. 참조할 만한 패턴/접근법 (코드 수준 아닌 개념 수준)

### 5.1 스킬 합성 패턴 (Composable Skills)

```
[실행 스킬] + [0-N 강화 스킬] + [선택적 보장 스킬] = 복합 행동
```

OMC의 스킬은 합성 가능하게 설계됨. 예: `ultrawork + ralph = 병렬 실행 + 완료 보장`. Pylon의 파이프라인 스테이지를 모듈화하고 조합 가능하게 만들 때 참조.

### 5.2 에이전트 정의 형식

OMC의 에이전트 마크다운 형식:

```markdown
---
name: architect
description: Strategic Architecture & Debugging Advisor (Opus, READ-ONLY)
model: claude-opus-4-6
disallowedTools: Write, Edit
---

<Role>...</Role>
<Why_This_Matters>...</Why_This_Matters>
<Success_Criteria>...</Success_Criteria>
<Constraints>...</Constraints>
<Investigation_Protocol>...</Investigation_Protocol>
<Tool_Usage>...</Tool_Usage>
<Execution_Policy>...</Execution_Policy>
<Output_Format>...</Output_Format>
```

Pylon의 에이전트 정의 형식(YAML frontmatter + Markdown body)과 유사하나, OMC는 XML-like 태그로 프롬프트를 구조화하여 각 섹션의 역할이 명확함. 특히 `<Why_This_Matters>`, `<Success_Criteria>`, `<Execution_Policy>` 섹션이 에이전트 행동을 더 예측 가능하게 만듦.

### 5.3 비용 추적 (HUD)

OMC의 HUD는 실시간 토큰/비용 모니터링 제공. Pylon의 Web Dashboard(`pylon dashboard`)에 비용 추적 패널을 추가할 때 참조. Claude Code CLI의 출력에서 토큰 사용량을 파싱하여 SQLite에 기록하는 방식으로 구현 가능.

### 5.4 브라운필드/그린필드 감지

기존 소스 코드, 패키지 파일, git 히스토리 존재 여부를 자동 감지하여 접근 방식 변경. Pylon의 `pylon add-project` 시 프로젝트 특성을 자동으로 파악하여 에이전트 구성을 최적화할 때 활용.

### 5.5 학습 추출 필터링

"5분 구글링으로 해결 가능한 것은 학습이 아니다" — OMC의 Learner 스킬이 적용하는 필터. Pylon의 Wiki 자동 업데이트에서 노이즈를 줄이는 데 참조. 모든 작업 결과를 기록하는 것이 아니라, 진정한 학습 가치가 있는 것만 선별.

---

## 6. 잠재적 충돌/주의사항

### 6.1 OMC와 동시 사용 시 충돌

만약 Pylon 사용자가 OMC도 설치한다면:

| 영역 | 충돌 |
|------|------|
| **CLAUDE.md** | OMC가 프로젝트 CLAUDE.md를 수정할 수 있음 → Pylon의 동적 CLAUDE.md 생성과 충돌 |
| **훅** | OMC 훅이 Pylon 에이전트의 동작에 개입할 수 있음 |
| **상태 관리** | `.omc/state/`와 `.pylon/runtime/`의 이중 상태 |
| **에이전트 도구** | OMC의 PreToolUse 훅이 Pylon 에이전트의 도구 사용을 차단할 수 있음 |

**권장**: Pylon은 OMC와 독립적으로 동작하도록 설계 유지. OMC의 패턴은 **개념적으로 흡수**하되, 직접 의존하지 않음.

### 6.2 Pylon 고유 강점 (OMC에 없는 것)

Pylon만의 가치로 보존해야 할 영역:

| Pylon 고유 기능 | OMC 대응 | 비고 |
|----------------|---------|------|
| **멀티 프로젝트 관리** | 단일 프로젝트만 | 모노레포/서브모듈 지원 |
| **PO 대화형 요구사항** | deep-interview (유사) | Pylon의 PO가 더 지속적 |
| **git worktree 격리** | Claude Code 내부 분리 | OS 레벨 격리가 더 안전 |
| **SQLite + SPOF 복구** | 파일 기반 | Pylon이 더 견고 |
| **Web Dashboard** | HUD (터미널) | Pylon이 더 풍부한 UI |
| **메시지 프로토콜** | Claude Code Team 도구 | Pylon이 더 유연 |
| **독립 바이너리** | npm 의존 | Pylon이 배포 용이 |
| **프로젝트별 에이전트** | 글로벌 에이전트만 | Pylon이 더 세밀 |
| **PR 자동 생성** | 없음 | Pylon 고유 |
| **메모리 BM25 검색** | 없음 | Pylon 고유 |

---

## 7. 종합 평가

### 7.1 핵심 결론

oh-my-claudecode는 Claude Code **내부**에서 동작하는 가장 성숙한 멀티 에이전트 오케스트레이션 플러그인이다. Pylon은 Claude Code **외부**에서 동작하는 메타-오케스트레이터로, 근본적으로 다른 레이어에 위치한다.

**Pylon의 상위 수준 가치**(멀티 프로젝트, OS 레벨 격리, SQLite 영속, PR 자동화)는 OMC에 없는 고유 영역이며, OMC의 **실행 수준 가치**(모호성 게이팅, circuit breaker, 7단계 검증, 모델 티어링, 학습 추출)는 Pylon의 에이전트 품질을 높이는 데 참조할 수 있다.

### 7.2 도입 우선순위 요약

| 우선순위 | 패턴 | 적용 위치 | 노력 |
|---------|------|-----------|------|
| **P0** | PO 모호성 점수 | `po.md` + `conversation.go` | 낮음 |
| **P0** | Circuit Breaker | `verify.go` | 낮음 |
| **P0** | `disallowedTools` 필드 | `agent.go` + `runner.go` | 낮음 |
| **P1** | 7단계 검증 프로토콜 | `verify.go` + `verify.yml` | 중간 |
| **P1** | PRD 기반 수용 기준 추적 | `pipeline_state.go` | 중간 |
| **P1** | 메모리 카테고리 체계 | `memory/manager.go` | 중간 |
| **P1** | 에이전트 벤치마크 | `benchmarks/` + `bench.go` | 중간 |
| **P2** | 동적 모델 라우팅 | `runner.go` + `message.go` | 높음 |
| **P2** | Critic 에이전트 | `critic.md` + 파이프라인 | 높음 |
| **P2** | AI Slop Cleaner | `code-cleaner.md` + 파이프라인 | 높음 |
| **P2** | Claude Code 훅 주입 | `claudemd.go` 확장 | 높음 |

### 7.3 최종 권장

OMC 플러그인 자체를 도입하는 것이 아니라, **핵심 패턴과 개념을 Pylon의 Go 코드와 에이전트 프롬프트에 네이티브로 구현**하는 것이 최적 전략이다. Pylon의 외부 오케스트레이터 아키텍처는 OMC의 플러그인 아키텍처보다 더 높은 제어력과 안정성을 제공하므로, OMC의 아이디어를 Pylon의 강점 위에 구현하면 양쪽의 장점을 모두 취할 수 있다.

---

## 참고 자료

- [oh-my-claudecode GitHub Repository](https://github.com/Yeachan-Heo/oh-my-claudecode)
- [oh-my-claudecode Official Website](https://yeachan-heo.github.io/oh-my-claudecode-website/)
- [REFERENCE.md](https://github.com/Yeachan-Heo/oh-my-claudecode/blob/main/docs/REFERENCE.md)
- [ARCHITECTURE.md](https://github.com/Yeachan-Heo/oh-my-claudecode/blob/main/docs/ARCHITECTURE.md)
- [Everything Claude Code vs Oh My ClaudeCode 비교](https://roboco.io/posts/everything-claude-code-vs-oh-my-claude-code/)
- [AI Coding Agent Ecosystem 분석](https://jeongil.dev/en/blog/trends/claude-code-agent-teams/)
