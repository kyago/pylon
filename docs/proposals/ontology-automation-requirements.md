# Pylon 온톨로지 자동화 요구사항 분석서

> **작성일**: 2026-03-20
> **상태**: 요구사항 분석 (pre-design)
> **목적**: Pylon의 원래 비전인 "조직의 개발 지식을 자동으로 구조화하는 시스템"을 재정립하고, 온톨로지 자동화의 구체적 요구사항을 정의

---

## 목차

1. [프로젝트 비전 재정립](#1-프로젝트-비전-재정립)
2. [현재 상태와 비전의 괴리](#2-현재-상태와-비전의-괴리)
3. [외부 생태계와의 비교](#3-외부-생태계와의-비교)
4. [온톨로지 자동화 요구사항](#4-온톨로지-자동화-요구사항)
5. [추출 접근법 분석](#5-추출-접근법-분석)
6. [아키텍처 유연성 요구사항](#6-아키텍처-유연성-요구사항)
7. [미결 설계 결정](#7-미결-설계-결정)

---

## 1. 프로젝트 비전 재정립

### 1.1 최초 비전

Pylon의 최초 기획 목적은 단순한 에이전트 파이프라인이 아니다:

- **유저는 PO 또는 PM하고만 소통**하고, 나머지 분석/계획/작업/리뷰/문서화는 에이전트가 자율 수행
- 완료된 코드에 대한 **비즈니스 지식을 담은 온톨로지를 자동으로 생성·축적**
- 에이전트가 일을 할수록 **조직의 도메인 지식이 구조화되어 쌓이는** 시스템

### 1.2 핵심 차별 가치

멀티에이전트 오케스트레이션 자체는 이미 여러 프로젝트(ClawTeam, AutoGen, CrewAI 등)가 풀고 있는 문제다. Pylon의 차별적 가치는:

> **"코드를 만드는 도구"가 아니라 "조직의 개발 지식을 자동으로 구조화하는 시스템"**

이것은 현재 ClawTeam을 포함한 어떤 오픈소스 멀티에이전트 프레임워크에도 없는 기능이다.

### 1.3 비전 구성 요소

| 구성 요소 | 설명 |
|-----------|------|
| **에이전트 오케스트레이션** | PO/PM/Architect/Dev/Reviewer/Tech Writer 역할 기반 파이프라인 |
| **온톨로지 자동화** | 코드에서 비즈니스 용어, 아키텍처 결정, 컨벤션을 자동 추출·구조화 |
| **지식 영속성** | 프로젝트 메모리로 에이전트가 과거 작업을 기억하고 학습 |
| **팀 지식 공유** | 팀 단위로 온톨로지를 공유하고 발전시키는 메커니즘 |

---

## 2. 현재 상태와 비전의 괴리

### 2.1 구현 완성도

코드베이스 분석 결과, 인프라(파이프라인 엔진)는 ~70% 완성이나, 차별적 가치(온톨로지 자동화 + 역할 기반 지능)는 ~10-20% 수준이다.

| 영역 | 구현 상태 | 비전 대비 |
|------|-----------|-----------|
| 파이프라인 오케스트레이션 (11단계) | 동작함 | ~70% |
| 프로세스 격리 (Git worktree) | 동작함 | ~90% |
| 메시지 프로토콜 (inbox/outbox) | 동작함 | ~95% |
| 태스크 의존성 그래프 | 동작함 | ~80% |
| 웹 대시보드 (Templ + HTMX + SSE) | 동작함 | ~80% |
| 메모리 시스템 (BM25 FTS5) | 동작함 | ~40% |
| **에이전트 역할 분화** | **이름만 존재, 코드 레벨 차이 없음** | **~20%** |
| **온톨로지 자동화** | **템플릿 파일만 존재, 자동화 없음** | **~5%** |
| **Capabilities 기반 매칭** | **구조체 필드만 존재, 사용 안 됨** | **~0%** |
| **Reviewer/Constitution 검증** | **미구현** | **~0%** |

### 2.2 구체적 괴리 지점

**온톨로지 관련:**
- `.pylon/domain/` 디렉토리는 `pylon init` 시 빈 템플릿(`conventions.md`, `glossary.md`, `architecture.md`)만 생성
- 자동 갱신 메커니즘 없음
- `StoreLearnings()`가 자유 텍스트를 confidence 0.8로 저장하는 수준

**에이전트 역할 관련:**
- PO 검증: `auto-approved` 하드코딩
- Tech Writer: fire-and-forget (실패해도 무시)
- Reviewer: 완전 미구현
- 모든 에이전트가 generic — `type` 필드가 "dev"이냐 아니냐만 구분

**아키텍처 유연성 관련:**
- `"claude"` 명령어가 6곳 이상 하드코딩
- 파이프라인 스테이지가 switch 문에 고정
- 에이전트 이름(`"architect"`, `"pm"`, `"po"`)이 오케스트레이션 루프에 리터럴로 존재
- 훅/플러그인 시스템 없음

---

## 3. 외부 생태계와의 비교

### 3.1 ClawTeam과의 비교

ClawTeam(HKUDS, 2026-03-17)은 Pylon의 최초 비전과 구조적으로 유사한 에이전트 스웜 프레임워크다.

| 영역 | Pylon | ClawTeam | 비고 |
|------|-------|----------|------|
| 리더/워커 구조 | PO/PM → 에이전트 | Leader → Worker | 유사 |
| 프로세스 격리 | Git worktree | Git worktree | 동일 |
| 메시지 통신 | inbox/outbox + SQLite | inbox + ZeroMQ P2P | 유사 |
| 태스크 의존성 | TaskGraph DAG | blocks/blocked_by DAG | 유사 |
| 오케스트레이션 주체 | **Go 프로세스 (결정론적)** | **에이전트 자체 (LLM 자율)** | **철학적 차이** |
| 동적 팀 구성 | 정적 (YAML) | `request-join`으로 런타임 합류 | ClawTeam 우위 |
| **온톨로지/지식 관리** | **비전에는 있으나 미구현** | **없음** | **Pylon 차별 지점** |
| **지식 영속성** | **SQLite + FTS5** | **이벤트 로그만 (쿼리 불가)** | **Pylon 우위** |

### 3.2 핵심 발견

- 파이프라인/오케스트레이션 측면에서 Pylon과 ClawTeam은 비슷한 문제를 비슷한 방식으로 풀고 있음
- ClawTeam은 에이전트 자율성(동적 팀, 에이전트 불가지론)에서 앞서 있음
- **온톨로지 자동화는 ClawTeam에 전혀 없는, Pylon만의 진짜 차별적 가치**

### 3.3 외부 도구 통합 현실

개발자들은 Claude Code 생태계에서 `oh-my-claude`, `GSD`, `speckit` 등 외부 도구를 조합하여 사용한다. 현재 Pylon은 이러한 도구를 손쉽게 적용하거나 전환할 수 있는 구조가 아니다:

- Backend 바이너리가 하드코딩 (`"claude"` 문자열 리터럴)
- 파이프라인 스테이지가 설정이 아닌 코드에 고정
- 훅/미들웨어/플러그인 포인트 없음
- 유일한 확장 지점은 CLAUDE.md 빌더의 `BuildInput` 5단계 우선순위 주입

---

## 4. 온톨로지 자동화 요구사항

### 4.1 온톨로지의 범위

Pylon 온톨로지가 관리하는 지식의 종류:

| 종류 (kind) | 설명 | 예시 |
|-------------|------|------|
| **term** | 비즈니스/도메인 용어 | `PaymentIntent`: 결제 요청의 생명주기를 관리하는 도메인 객체 |
| **decision** | 아키텍처 결정 | SQLite 선택 이유, 워크트리 격리 방식 채택 |
| **convention** | 코딩 컨벤션 | 에러 핸들링 패턴, 네이밍 규칙 |

### 4.2 핵심 원칙: Code-Grounded Ontology

> **코드에 없는 지식은 온톨로지에 넣지 않는다.**

모든 온톨로지 항목은 코드에 근거(evidence)를 가져야 한다. 이 제약이 "자동 쓰레기 생성"과 "자동 지식 구조화" 사이의 경계선이다.

자동 추출이 쓰레기가 되는 3가지 패턴과 방지 전략:

| 쓰레기 패턴 | 예시 | 방지 전략 |
|-------------|------|-----------|
| 근거 없는 추상화 | "마이크로서비스 아키텍처" (검증 불가) | evidence 필수, 코드에서 심볼 존재 검증 |
| 중복·모순 | `User`와 `사용자`가 별개 항목 | Dedup + 충돌 감지 |
| 부패한 지식 | "JWT 인증" (이미 세션으로 변경됨) | Liveness Check (주기적 evidence 유효성 확인) |

### 4.3 추출 → 검증 → 등록 파이프라인

```
에이전트 실행 완료
       │
       ▼
┌──────────────┐
│  1. Extract   │  코드 diff + 에이전트 결과물에서 후보 추출
│  (자동)       │  AST 파싱으로 구조적 심볼 추출
└──────┬───────┘
       ▼
┌──────────────┐
│  2. Ground    │  후보의 evidence가 실제 코드에 존재하는지 검증
│  (자동)       │  grep/AST로 symbol이 현재 코드에 있는지 확인
└──────┬───────┘
       ▼
┌──────────────┐
│  3. Dedup     │  기존 온톨로지와 비교
│  (자동)       │  동일 심볼 → 병합, 모순 → 충돌 플래그
└──────┬───────┘
       ▼
┌──────────────┐
│  4. Classify  │  confidence 판정
│  (자동)       │  high: 자동 등록 / low: 리뷰 큐
└──────┬───────┘
       ▼
  ┌────┴────┐
  │         │
  ▼         ▼
자동등록   리뷰큐 → PO/개발자가 승인/거부
```

### 4.4 검증 단계 상세 기준

**Grounding (코드 근거 검증):**

```
통과 조건 (하나 이상 충족):
  ✓ evidence.symbol이 코드에서 grep으로 발견됨
  ✓ evidence.file이 실제로 존재함
  ✓ type이 "decision"이고, 관련 파일/디렉토리 구조가 존재함

거부 조건:
  ✗ evidence가 비어있음
  ✗ evidence.file이 존재하지 않음
  ✗ evidence.symbol이 코드 어디에도 없음
```

**Dedup (중복/모순 감지):**

```
동일 symbol을 가리키는 기존 항목이 있으면:
  - definition이 호환 → 병합 (evidence 합산)
  - definition이 모순 → 충돌 플래그, 리뷰큐로

동일 term인데 다른 symbol:
  - 동음이의어 가능 → 리뷰큐로
```

**Classify (confidence 판정):**

```
HIGH (자동 등록):
  - evidence가 2개 이상
  - symbol이 public (exported)
  - 기존 항목과 충돌 없음

LOW (리뷰큐):
  - evidence가 1개
  - symbol이 private/internal
  - 기존 항목과 잠재적 충돌
  - definition이 너무 추상적
```

### 4.5 부패 방지: Liveness Check

등록된 지식이 코드 변경으로 인해 무효화되지 않도록:

```
파이프라인 실행 시 (또는 주기적으로):
  1. 온톨로지의 모든 항목에 대해
  2. evidence.file 존재 여부 확인
  3. evidence.symbol이 해당 파일에 존재하는지 확인
  4. 실패 → stale 마킹 (즉시 삭제 아님)
  5. 2회 연속 stale → 아카이브 (활성 온톨로지에서 제거)
```

### 4.6 데이터 모델

단일 온톨로지 항목의 구조:

```yaml
kind: term                          # term | decision | convention
name: PaymentIntent
definition: |
  결제 요청의 생명주기를 관리하는 도메인 객체.
  pending → authorized → captured → refunded 상태 전이를 가짐.
evidence:
  - file: internal/payment/intent.go
    symbol: "type PaymentIntent struct"
    line: 15
  - file: internal/payment/intent.go
    symbol: "func (p *PaymentIntent) Capture"
    line: 42
relations:
  - type: creates
    target: ChargeRecord
  - type: belongs_to
    target: Order
tags: [payment, domain-model]
confidence: high                    # high | low
created_by: pipeline-20260320-001
created_at: 2026-03-20
last_verified: 2026-03-20
stale_count: 0
```

### 4.7 팀 단위 온톨로지

**저장소: Git 자체를 활용**

```
.pylon/
└── ontology/
    ├── _index.json              # 전체 항목 인덱스 (빠른 조회용)
    ├── terms/
    │   ├── payment-intent.yaml
    │   └── charge-record.yaml
    ├── decisions/
    │   ├── 001-use-sqlite.yaml
    │   └── 002-worktree-isolation.yaml
    ├── conventions/
    │   ├── error-handling.yaml
    │   └── naming.yaml
    ├── _stale/                  # liveness check 실패한 항목
    └── _review/                 # 리뷰 대기 항목
```

**Git 기반의 이유:**
- 팀원이 `git pull`하면 자동으로 최신 온톨로지를 받음
- 충돌 시 git merge로 해결 (이미 팀이 익숙한 워크플로우)
- 히스토리가 자동으로 남음
- 별도 인프라 불필요

**팀 워크플로우:**

```
개발자 A (feature-auth 브랜치):
  → 파이프라인 완료 → AuthToken, SessionStore 항목 자동 추출
  → PR에 ontology 변경사항 포함

개발자 B (feature-payment 브랜치):
  → 파이프라인 완료 → PaymentIntent, ChargeRecord 항목 자동 추출
  → PR에 ontology 변경사항 포함

main 머지 시:
  → 두 온톨로지 변경이 자연스럽게 합쳐짐
  → 같은 term을 다르게 정의한 경우 → git conflict → 리뷰어 판단
```

**멀티 프로젝트 구조:**

```
.pylon/
└── ontology/
    ├── shared/                  # 프로젝트 공통 (도메인 용어, 조직 컨벤션)
    ├── projects/
    │   ├── api/                 # api 프로젝트 고유
    │   └── web/                 # web 프로젝트 고유
    └── _index.json
```

에이전트가 온톨로지를 조회할 때는 `shared/ + projects/{current}/`를 합쳐서 참조.

---

## 5. 추출 접근법 분석

온톨로지 자동 추출의 에이전트 플로우를 결정하기 위해 세 가지 접근법을 병렬 분석하였다.

### 5.1 결정론적 스크립트 기반

AST 파싱 + regex로 코드에서 구조적 심볼을 추출. LLM 미사용.

| 항목 | 평가 |
|------|------|
| 추출 범위 | 코드 심볼만 (struct, func, type) |
| 쓰레기 위험 | 낮음 — 존재하지 않는 것을 만들 수 없음 |
| 품질 한계 | 용어 ~30% recall, 아키텍처 결정 ~5% recall |
| 비용 | 0 |
| 일관성 | 100% 결정론적 |
| 핵심 한계 | "무엇이 있는가"만 알지, "이것이 무엇을 의미하는가"를 모름 |

### 5.2 LLM 자율 추출

LLM에게 코드 diff + 에이전트 결과물을 주고 자유롭게 온톨로지 항목을 추출하게 함.

| 항목 | 평가 |
|------|------|
| 추출 범위 | 의미적 개념까지 (아키텍처 결정, 컨벤션, 관계) |
| 쓰레기 위험 | 높음 — 환각, 의미 오류, 근거 없는 관계 생성 가능 |
| 품질 한계 | 정밀도 ~85-90%, 재현율 측정 불가 |
| 비용 | $0.01-0.40/파이프라인 |
| 일관성 | 60-70% — 같은 입력에 다른 출력 |
| 핵심 한계 | 자기 검증 불가, confidence 점수가 무의미, dedup 어려움 |

### 5.3 하이브리드 및 대안 접근법

4가지 대안을 분석하였다:

**Approach A — 하이브리드 (결정론적 추출 + LLM 보강):**
AST로 후보 추출 → LLM은 정의/관계만 작성. 근거는 자동 확보.

**Approach B — LLM 추출 + 결정론적 검증:**
LLM이 자유 추출 → grep/AST로 근거 검증, 실패 시 거부.

**Approach C — 어노테이션 기반:**
`// @pylon:term PaymentIntent - 결제 요청의 생명주기` 같은 마커를 개발자가 삽입. 파싱만으로 추출.

**Approach D — 에이전트 통합 (실행 중 추출):**
구현 에이전트가 작업 중 `ResultBody.OntologyItems`로 지식 항목을 함께 출력. 별도 추출 파이프라인 불필요.

### 5.4 비교 매트릭스

| | 결정론적 | LLM 자율 | A: 하이브리드 | B: LLM+검증 | C: 어노테이션 | D: 에이전트통합 |
|---|---|---|---|---|---|---|
| 쓰레기 방지 | ★★★★★ | ★★☆☆☆ | ★★★★☆ | ★★★☆☆ | ★★★★★ | ★★☆☆☆ |
| 의미 품질 | ★☆☆☆☆ | ★★★★☆ | ★★★★☆ | ★★★★☆ | ★★★★★ | ★★★☆☆ |
| 비용 | ★★★★★ | ★★☆☆☆ | ★★★★☆ | ★★★☆☆ | ★★★★★ | ★★★★★ |
| 일관성 | ★★★★★ | ★★☆☆☆ | ★★★★☆ | ★★★☆☆ | ★★★★★ | ★★★☆☆ |
| 개발자 부담 | ★★★★★ | ★★★★★ | ★★★★★ | ★★★★★ | ★★☆☆☆ | ★★★★★ |
| 팀 확장성 | ★★★★☆ | ★★★★☆ | ★★★★☆ | ★★★★☆ | ★★☆☆☆ | ★★★★☆ |

### 5.5 관통하는 통찰

세 분석을 관통하는 공통 발견:

```
코드에서 "무엇이 있는가"   → 결정론적으로 풀 수 있음 (AST/grep)
코드에서 "이것이 뭘 의미하는가" → LLM이 필요함
코드에서 "이것이 정말 있는가"  → 결정론적으로 풀 수 있음 (grep)
```

**추출(discover)과 검증(verify)은 결정론적**이고, **해석(interpret)만 LLM**이면 된다.

### 5.6 권장 접근: A+D 조합

분석 결과, Approach A (하이브리드)와 Approach D (에이전트 통합)의 조합이 가장 실용적이다:

```
┌─────────────────────────────────────────────────────────┐
│ 에이전트 실행 중                                          │
│  → 기존 ResultBody.Learnings에 도메인 용어를              │
│    자연스럽게 언급 (추가 비용 0)                           │
└──────────────┬──────────────────────────────────────────┘
               ▼
┌─────────────────────────────────────────────────────────┐
│ Phase 1: AST 추출 (결정론적)                              │
│  → FilesChanged의 모든 파일에서 type, func, struct 추출   │
│  → 근거(evidence)가 자동으로 확보됨                       │
│  → confidence: 0.6으로 Blackboard에 등록                  │
└──────────────┬──────────────────────────────────────────┘
               ▼
┌─────────────────────────────────────────────────────────┐
│ Phase 2: Learnings 매칭 (결정론적)                        │
│  → 에이전트가 남긴 learnings에서 Phase 1 후보와           │
│    매칭되는 용어 탐색                                     │
│  → 매칭 시 learning 텍스트를 definition으로 사용          │
│  → confidence: 0.6 → 0.9 승격                            │
└──────────────┬──────────────────────────────────────────┘
               ▼
┌─────────────────────────────────────────────────────────┐
│ Phase 3: LLM 보강 (선택적, 배치)                          │
│  → confidence 0.6인 채로 남은 후보만 모아서               │
│    한 번의 LLM 호출로 정의 작성                           │
│  → 비용: ~$0.002-0.01 (소수 항목만 대상)                  │
└─────────────────────────────────────────────────────────┘
```

**이 조합의 근거:**
- 심볼은 AST에서 왔으므로 100% 실재 (환각 불가)
- LLM은 "해석"에만 사용 (가장 잘하는 것만 시킴)
- 에이전트의 기존 learnings를 재활용 (추가 프롬프트 비용 0)
- Phase 3은 선택적이므로 비용이 극도로 낮음
- Pylon의 기존 인프라(`ResultBody.Learnings`, `BlackboardEntry`, `PollOnce()`)와 자연스럽게 맞물림

**보완 — 어노테이션은 opt-in 오버라이드:**
`// @pylon:term` 어노테이션이 발견되면 `confidence: 1.0`으로 등록하여 자동 생성 정의를 덮어씀. 강제가 아닌 선택적 개입 경로.

---

## 6. 아키텍처 유연성 요구사항

온톨로지 자동화 구현에 앞서, 현재 코드베이스의 유연성 부족이 선결 과제로 확인되었다.

### 6.1 필요한 아키텍처 변경

| 우선순위 | 항목 | 현재 상태 | 필요 상태 |
|---------|------|-----------|-----------|
| P0 | Backend Abstraction | `"claude"` 하드코딩 6곳+ | `AgentBackend` 인터페이스, `backend:` 필드 기반 디스패치 |
| P0 | 파이프라인 스테이지 | switch 문에 11단계 고정 | `[]StageHandler` + 설정 파일 기반 |
| P1 | Pre/Post Stage Hooks | 없음 | 각 스테이지 전후에 외부 스크립트 실행 가능 |
| P1 | 에이전트 이름 하드코딩 | `"architect"`, `"pm"` 리터럴 | 설정 기반 역할 매핑 |

### 6.2 온톨로지 추출의 통합 지점

현재 Pylon 인프라에서 온톨로지 추출이 연결되어야 하는 기존 코드:

| 컴포넌트 | 파일 | 용도 |
|----------|------|------|
| `ResultBody.Learnings` | `internal/protocol/message.go:79` | 에이전트 학습 출력 (Phase 2 입력) |
| `OutboxWatcher.PollOnce()` | `internal/orchestrator/watcher.go:44` | 결과 수집 지점 (추출 트리거) |
| `StoreLearnings()` | `internal/memory/manager.go:84` | 기존 학습 저장 (온톨로지 저장으로 확장) |
| `BlackboardEntry` | `internal/store/blackboard.go:12` | UPSERT + confidence + supersession (온톨로지 저장소 후보) |
| `project_memory_fts` | `internal/store/migrations/001_initial.sql` | BM25 FTS5 인덱스 (온톨로지 검색) |
| `ClaudeMDBuilder.BuildInput` | `internal/agent/claudemd.go:28` | 에이전트 프롬프트 주입 (온톨로지 컨텍스트 주입) |

---

## 7. 미결 설계 결정

다음 단계(설계)에서 결정해야 할 사항:

| # | 결정 사항 | 선택지 | 판단 기준 |
|---|-----------|--------|-----------|
| 1 | AST 파서 범위 | Go `go/parser`만 vs tree-sitter 다국어 | 대상 프로젝트의 언어 다양성 |
| 2 | Learnings 매칭 알고리즘 | 단순 문자열 매칭 vs BM25 (기존 FTS5 활용) | 매칭 정확도 요구 수준 |
| 3 | Phase 3 LLM 보강 트리거 | 모든 미보강 항목 vs N개 누적 시 배치 | 비용 vs 즉시성 |
| 4 | 저장소 | 기존 Blackboard 테이블 재사용 vs 전용 ontology 테이블 | 스키마 복잡도 vs 관심사 분리 |
| 5 | Liveness Check 주기 | 매 파이프라인 실행 시 vs 별도 주기 | 성능 영향 vs 최신성 |
| 6 | 리뷰큐 UI | 대시보드 통합 vs CLI 전용 | 팀 접근성 |
| 7 | 온톨로지 → 에이전트 주입 방식 | CLAUDE.md Priority 4에 포함 vs 별도 파일 참조 | 200라인 예산 제약 |

---

## 부록: 참고 자료

- [ClawTeam (HKUDS)](https://github.com/HKUDS/ClawTeam) — Agent Swarm Intelligence Framework
- `pylon-spec.md` — Pylon 정식 스펙 문서 (73KB)
- `IMPLEMENTATION_PLAN.md` — 구현 로드맵 (39KB)
- `docs/proposals/adaptive-workflow-with-swarm-pattern.md` — 적응형 워크플로우 제안서
- `docs/proposals/orchestration-and-dashboard-improvements.md` — 오케스트레이션 개선 제안서
