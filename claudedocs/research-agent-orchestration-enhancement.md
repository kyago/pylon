# 에이전트 오케스트레이션 고도화 리서치

> 외부 프로젝트 분석 + 실제 워크스페이스 검증 기반 Pylon 고도화 방향성
> 최초 분석일: 2026-03-12
> 갱신일: 2026-03-12 (semicolon 워크스페이스 실사 분석 융합)
> 갱신일: 2026-03-13 (미결사항 검토 — 내부 일관성, spec 커버리지, speckit 검증)
> 갱신일: 2026-03-13 (미결사항 12건 전체 해결 + 확정 결정사항 pylon-spec.md 반영)

## 분석 소스

| 소스 | URL / 출처 | 핵심 주제 |
|------|-----------|-----------|
| Hada News 댓글 | https://news.hada.io/topic?id=27414 | "잠자는 동안 실행되는 에이전트" HN 의견 요약 |
| Frequency.sh | https://www.frequency.sh/blog/introducing-frequency/ | 스크립트 기반 에이전트 오케스트레이션 플랫폼 |
| Unratified.org | https://blog.unratified.org/2026-03-06-receiving-side-agent-proposals/ | Proposal 기반 에이전트 간 자율 협업 |
| semicolon 워크스페이스 | Pylon을 사용하는 실제 프로젝트의 에이전트 분석 | 명세 vs 현실 갭 + speckit 실사 |

---

## 0. 명세 vs 현실 갭 (semicolon 워크스페이스 실사)

> 이 섹션은 Pylon을 사용하는 프로젝트의 에이전트가 발견한 내용입니다.
> 모든 고도화 제안은 이 현실을 전제로 읽어야 합니다.

### 0.1 발견된 갭

| 컴포넌트 | pylon-spec.md 명세 | 실제 상태 |
|----------|-------------------|-----------|
| pylon stage/mem CLI | 파이프라인 상태 관리, 메모리 검색 | 바이너리 미존재 |
| /pl:* 슬래시 커맨드 6개 | index, status, verify 등 사용 가능 | 구현체 없음 |
| verify.yml | 빌드/테스트/린트 교차 검증 | `echo 'no build configured'` |
| .pylon/runtime/results/ | 에이전트 결과 저장소 | 디렉토리 미존재 |
| 에이전트 outbox 통신 | inbox/outbox 파일 기반 통신 | 정의되지 않음 |
| 에이전트 프롬프트 | 역할별 상세 기술 | `developer.md` 2줄 수준 |
| 동시성 제어 | max_concurrent: 5 + PM 자율 판단 | 충돌 방지 없음 |

### 0.2 시사점

외부 레퍼런스 분석(섹션 1~3)은 "이미 동작하는 Pylon에 무엇을 더할까"를 전제했으나, 실제로는 **기반 통신 프로토콜과 파이프라인 엔진 자체가 미구현**입니다. 따라서:

- 고도화 제안의 우선순위를 "기반 구축 → 연동 → 확장"으로 재편
- speckit/.specify/ 시스템이 **이미 동작하는 결정론적 오케스트레이션**으로 발견됨
- "speckit을 base로 결정론적 오케스트레이션" 방향이 프로젝트에서 결정됨

### 0.3 speckit/.specify/ 시스템 (실전 레퍼런스)

semicolon 워크스페이스에서 실제 동작하는 결정론적 오케스트레이션 시스템:

```
.specify/
├── memory/constitution.md          # 프로젝트 "헌법" (개발 원칙 강제)
├── scripts/bash/
│   ├── create-new-feature.sh       # 번호 매긴 브랜치 + specs 디렉토리 자동 생성
│   ├── setup-plan.sh               # plan 템플릿 복사
│   ├── check-prerequisites.sh      # 완료도 검증 (JSON 출력)
│   ├── update-agent-context.sh     # plan.md → CLAUDE.md 에이전트 컨텍스트 갱신
│   └── common.sh                   # 공통 함수
├── specs/{###-feature}/
│   ├── spec.md                     # 사용자 스토리 + 수용 기준
│   ├── plan.md                     # 기술 접근 + 헌법 검증
│   ├── tasks.md                    # Phase별 작업 분해 ([P] = 병렬 가능)
│   ├── research.md                 # 기술 리서치
│   ├── data-model.md               # 데이터 모델
│   └── contracts/                  # API 계약 (YAML)
└── templates/                      # 모든 문서의 템플릿
```

**speckit이 이미 해결한 것들**:

| 우리가 제안한 것 | speckit의 기존 해법 |
|----------------|-------------------|
| 4.2 태스크 구체성 강화 (inbox body에 targets, api_contract) | `contracts/` YAML + `data-model.md` |
| 4.3 결정론적/에이전트 step 분리 (pipeline.yml) | `scripts/bash/` 셸 스크립트 |
| 컨텍스트 분리 검증 | `update-agent-context.sh`로 plan → CLAUDE.md 변환 |
| 수용 기준 정의 | `spec.md`의 사용자 스토리 + 수용 기준 |
| 병렬 가능 표시 | `tasks.md`의 `[P]` 마커 |

**speckit에 없고 Pylon이 제공해야 하는 것**:

| 영역 | speckit 상태 | Pylon이 담당 |
|------|-------------|-------------|
| 상태 머신 | 없음 (사람이 수동 진행) | 파이프라인 상태 전이 자동화 |
| 에이전트 프로세스 관리 | 없음 | 생명주기 관리, 격리, 동시성 |
| 멀티 프로젝트 | 없음 | 워크스페이스 레벨 오케스트레이션 |
| 교차 검증 | `check-prerequisites.sh` (단순) | verify.yml 기반 빌드/테스트/린트 |
| 프로젝트 메모리 | `constitution.md` (정적) | SQLite + BM25 동적 축적 |
| 에러 복구 | 없음 | 재시도 + 에스컬레이션 |

---

## 1. Hada News / HN 댓글 핵심 논점

### 1.1 LLM 프레임워크 효율성 회의론
> "요즘 나오는 LLM 프레임워크들이 오히려 개발을 더 어렵고 비싸게 만드는 느낌"

- Pylon은 프레임워크가 아닌 오케스트레이터로, Claude Code CLI를 직접 실행하는 얇은 레이어
- "Go 오케스트레이터 + 직접 프로세스 실행" 접근이 이 비판을 회피함
- **교훈**: 추상화 레이어를 최소화하는 현재 방향 유지

### 1.2 에이전트 운영 수량
> "밤새 돌리기보다 한두 개만 운영하는 것이 효율적"

- `max_concurrent: 5` 상한선 + PM 자율 판단이 이 의견과 부합
- **교훈**: 에이전트 수를 늘리는 것보다 태스크 품질을 높이는 방향

### 1.3 "Test Theatre" (의미 없는 테스트 증가)
> Outside-in TDD + mutation testing 권장

- `verify.yml` 교차 검증이 "테스트 통과 여부"만 확인하지, "테스트 의미 여부"는 미검증
- **현실**: verify.yml 자체가 `echo 'no build configured'` 상태이므로, 기본 검증부터 구현 필요
- **고도화 아이템**: mutation testing 통합은 기본 검증 구현 이후 검토 (→ [제안 6.3](#63-verify-품질-게이트-강화) 참조)

### 1.4 컨텍스트 한계
> "200~500줄 범위에서 오류 급증"

- CLAUDE.md 주입 200줄 제한과 부합
- PM의 태스크 분해 역할의 중요성 재확인
- speckit의 `update-agent-context.sh`가 plan.md → CLAUDE.md 변환으로 이 문제를 실전에서 해결 중
- **교훈**: 태스크 크기를 작게 유지하는 것이 에이전트 성공률의 핵심

### 1.5 컨텍스트 분리 검증
> "생성 에이전트 ≠ 검증 에이전트"가 중요

- HN 댓글에서 강조된 "코드를 작성한 에이전트와 검증하는 에이전트를 분리"하는 원칙
- Pylon 명세의 교차 검증(오케스트레이터가 verify.yml 실행)이 이 원칙의 부분적 구현
- **교훈**: 검증 단계는 에이전트가 아닌 결정론적 스크립트로 수행하는 것이 신뢰성 높음

### 1.6 좁은 범위 + 명확한 목표
> "비즈니스 운영용 에이전트에서 명확한 목표와 좁은 범위가 효과적"

- PO의 수용 기준 정의 + PM의 태스크 분해가 이 원칙의 구현체
- speckit의 `spec.md` (수용 기준) + `tasks.md` (분해된 작업)이 실전 구현
- **교훈**: 현재 설계 방향 유지

---

## 2. Frequency.sh 상세 분석

### 2.1 아키텍처 개요

Frequency는 **"polling-and-advancement" 런타임**으로, 작업 단위(subject)를 정의된 상태 전이를 통해 전진시킴.

**Cashew Crate 사례**: 30개 프로덕션 앱을 자율적으로 배포. 아이디어 → 구현 → 리뷰 → 배포 → 마케팅까지 파이프라인이 연속 실행.

### 2.2 Pylon과 비교

| 특성 | Frequency | Pylon 명세 | speckit 실제 | 비고 |
|------|-----------|-----------|-------------|------|
| 상태 관리 | 레포 로컬 JSON | SQLite + state.json | 없음 (수동) | Pylon SQLite가 쿼리/히스토리에 유리 |
| 에이전트 결합도 | 에이전트 무관 | MVP: Claude Code | Claude Code | MVP는 전용, 추상화 인터페이스만 설계 |
| 워크플로 구성 | 자동 설정 생성 | 에이전트 .md 정의 | specs/ 디렉토리 | speckit이 워크플로 입력 제공 |
| 격리 | git worktree | git worktree | 없음 | Pylon이 격리 담당 |
| 동시성 제어 | 4계층 | max_concurrent 단일 | 없음 | **Pylon 개선 여지** |
| 에이전트/셸 비율 | 명시적 40/60 | 암시적 | **scripts/bash/로 실현** | speckit이 60% 셸 레퍼런스 |
| 결정론적 스크립트 | 워크플로 내장 | verify.yml만 | 5개 셸 스크립트 | speckit을 확장 가능 |

### 2.3 핵심 인사이트

#### (a) 40/60 비율 — 에이전트 vs 셸 커맨드

> "roughly 40% agent calls, 60% shell commands"

파이프라인의 대부분은 **결정론적 셸 커맨드**(빌드, 테스트, 린트, git 조작)이고, LLM이 필요한 부분은 40%에 불과함.

**speckit이 이미 증명한 60% 결정론적 부분**:
```
create-new-feature.sh    → 브랜치 생성, 디렉토리 구조 세팅
setup-plan.sh            → 템플릿 기반 문서 초기화
check-prerequisites.sh   → JSON 출력의 완료도 검증
update-agent-context.sh  → plan.md → CLAUDE.md 변환
```

이 스크립트들이 Pylon `pipeline.yml`의 `type: shell` step으로 흡수될 수 있음.

#### (b) 4계층 동시성 제어

```
1. Global worker limits        → Pylon의 max_concurrent (명세에만 존재)
2. Per-step worker caps        → ❌ 미구현
3. Per-state WIP limits        → ❌ 미구현
4. Named resource locks        → ❌ 미구현
```

**실전 교훈**: WIP 제한 없이 15개 subject 동시 실행 → 머지 충돌 폭발 → "일주일 분의 실행" 소요. 동시성 세분화가 안정성의 핵심.

**speckit 연관**: `tasks.md`의 `[P]` 마커가 병렬 가능 여부를 명시하므로, Pylon이 이를 파싱하여 동시성 제어에 활용 가능.

#### (c) Dead-letter 큐

에이전트의 비결정적 행동에 대비한 재시도 + dead-letter 시스템. Pylon의 `max_attempts + PM 에스컬레이션`과 유사하지만, **구조화된 큐**로 실패 이력을 관리.

#### (d) Cross-pipeline 의존성

마케팅이 배포 확인을 기다리고, 버그 수정이 빌드 단계로 루프백하는 등 **파이프라인 간 의존성** 지원. Pylon의 현재 설계는 단일 파이프라인 내 직렬/병렬만 다룸.

---

## 3. Receiving-side Agent Proposals 상세 분석

### 3.1 패러다임 전환

| | 전통적 오케스트레이션 (Pylon 현재) | Proposal 기반 |
|---|---|---|
| 방향 | Push: 중앙 컨트롤러 → 에이전트 | Pull: 에이전트가 제안 발행 → 수신 에이전트 평가 |
| 통신 | 명령형 태스크 할당 | 선언형 제안 + 수락/거부 |
| 발견 | 사전 정의된 에이전트 목록 | `.well-known/` 기반 자동 발견 |
| 트랜스포트 | 파일 기반 inbox/outbox | git PR 기반 |

### 3.2 핵심 설계 패턴

#### (a) 구체성이 곧 통제 (Specificity as Primary Control)

> "A specific proposal with API paths, example code, and exact page targets reduces implementation latency to near zero"

모호한 지시 → 해석 오류 발생. 구체적 제안(API 경로, 예시 코드, 정확한 타겟) → 즉시 실행 가능.

**speckit이 이미 실현한 구체성**:
```
specs/{###-feature}/
├── contracts/        → API 계약 (YAML) = API paths + 정확한 타겟
├── data-model.md     → 데이터 모델 = 구체적 스키마
└── spec.md           → 수용 기준 = 검증 방법
```

이 산출물이 Unratified의 "구체적 제안"과 정확히 동일한 역할을 수행.

#### (b) `.well-known/` 기반 에이전트 발견

```
/.well-known/agent-card.json   → 에이전트 능력 선언 (A2A v0.3.0 스키마)
/.well-known/agent-inbox.json  → 수신 가능한 제안 목록
```

RFC 5785 컨벤션으로 사전 관계 없이 에이전트 발견 가능.

#### (c) Git-PR 트랜스포트

메시지 큐 대신 **git PR**을 트랜스포트 레이어로 사용:
- `transport/sessions/` 디렉토리에 메시지 봉투 저장
- 감사 추적 + 버전 관리 자동 보장
- 브로커 인프라 불필요

#### (d) 인식론적 투명성 (Epistemic Transparency)

`interagent-epistemic/v1` 확장으로 각 클레임에 대한 신뢰도/신뢰 가정을 명시:
- `claims[]`: 에이전트가 주장하는 내용
- `epistemic_flags`: 신뢰도 수준
- `action_gate`: 실행 전 승인 요건

#### (e) Defense-in-Depth 인증

API 키 + git-PR 트랜스포트 + magic link 승인 게이트의 다층 보안. 사람 디렉터의 승인 링크가 1차 보안 경계.

---

## 4. 교차 분석: 외부 레퍼런스 × 실제 워크스페이스

### 4.1 두 분석의 관점 차이

| | 외부 레퍼런스 분석 (섹션 1~3) | semicolon 워크스페이스 실사 (섹션 0) |
|---|---|---|
| **전제** | pylon-spec.md가 현재 구현체 | 명세와 현실의 갭 발견 |
| **시각** | 완성된 시스템에 기능 추가 | 미완성 시스템의 재설계 방향 |
| **결론** | 8개 개선 아이템 제안 | "speckit base + 결정론적 오케스트레이션" 방향 |

### 4.2 수렴 지점 (양쪽이 같은 결론)

**40/60 결정론적/에이전트 분리가 핵심**

양쪽 모두 Frequency의 40/60 법칙을 가장 중요한 인사이트로 채택. speckit의 `scripts/bash/`가 **이미 동작하는 60% 결정론적 레퍼런스**.

**구체성 = 에이전트 성공률**

양쪽 모두 Unratified의 "구체성 원칙"을 높이 평가. speckit의 `contracts/` + `data-model.md`가 **이미 이 원칙을 구현**.

**동시성 제어 필요성**

양쪽 모두 `max_concurrent` 하나로는 부족하다고 판단. speckit의 `tasks.md` `[P]` 마커가 병렬 가능 여부를 제공.

### 4.3 실사에서만 발견된 핵심 요소

#### (a) Constitution (프로젝트 헌법)

speckit의 `memory/constitution.md`는 **프로젝트 개발 원칙을 강제하는 문서**. plan 작성 시 헌법 검증 체크리스트로 활용됨.

Pylon의 `domain/conventions.md`와 유사하지만, speckit은 이를 **에이전트가 plan을 작성할 때 반드시 참조하도록 강제**하는 점이 차이. 단순 참고 문서가 아닌 "검증 게이트"로 기능.

#### (b) 구조화된 산출물 파이프라인

```
spec.md → plan.md → tasks.md → contracts/ → 구현
```

이 워터폴이 Pylon의 `PO → Architect → PM → Developer` 흐름과 1:1 매핑:

| speckit 산출물 | Pylon 에이전트 | 역할 |
|---------------|--------------|------|
| spec.md (사용자 스토리 + 수용 기준) | PO | 요구사항 분석, 수용 기준 정의 |
| plan.md (기술 접근 + 헌법 검증) | Architect | 기술 방향성 결정 |
| tasks.md (Phase별 분해, [P] 마커) | PM | 태스크 분해, 병렬/직렬 결정 |
| contracts/ + data-model.md | PM → Developer | 구체적 구현 지시 |

#### (c) 템플릿 시스템

speckit의 `templates/` 디렉토리가 모든 문서의 구조를 표준화. Pylon에는 없는 개념이지만, 에이전트가 산출물을 작성할 때 일관된 품질을 보장하는 메커니즘으로 유효.

### 4.4 프로젝트에서 결정된 방향

> "Frequency처럼 결정론적 오케스트레이션을 위주로 가되,
> speckit을 base로 하는 도구로 잘 만들어진 도구를 기반으로 더 쉬운 사용 형태를 구성한다"

구체화하면:

```
speckit이 "무엇을 만들 것인가"를 정의 (spec/plan/tasks/contracts)
    ↓
Pylon이 "어떻게 실행할 것인가"를 담당 (상태 머신 + 프로세스 관리 + 검증)
```

- **Pylon = 상태 머신 기반 워크플로우 엔진**
- speckit의 산출물(spec/plan/tasks)을 **입력으로 받아 실행**
- **60% 결정론적 스크립트 + 40% 에이전트** (판단이 필요한 단계만)
- 멀티 프로젝트 오케스트레이션 지원

---

## 5. 미결 아키텍처 질문

| 질문 | 두 분석 종합 의견 | 근거 |
|------|-----------------|------|
| **Pylon-speckit 관계** | **speckit 연동 엔진**이 적절 | speckit의 산출물을 입력으로 받되 흡수하지 않음. speckit = "설계 도구", Pylon = "실행 엔진". 각자의 진화를 독립적으로 유지 |
| **CLI vs 선언적** | **하이브리드** | `pylon run`으로 실행하되, 워크플로 정의는 `pipeline.yml` 선언적. Frequency 모델과 부합 |
| **에이전트 비의존성** | **MVP는 Claude Code 전용**, 추상화 인터페이스만 설계 | Frequency가 에이전트 무관으로 설계했지만 실제로는 특정 에이전트에 최적화가 필요했던 교훈 |
| **상태 저장** | **SQLite 유지** | Frequency의 JSON은 단순하지만 쿼리/히스토리에 불리. pylon-spec.md의 SQLite 선택이 더 나은 결정 |
| **스코프** | **멀티 프로젝트 (워크스페이스)** | Pylon의 핵심 차별점 (pylon-spec.md Section 15 비교표). speckit은 단일 프로젝트 |

---

## 6. 고도화 제안 (로드맵 재편)

> Phase 0을 신설하여 명세-현실 갭을 해소하고, speckit 연동을 기반으로 재구성.

### Phase 0 — 기반 구축 + speckit 연동

> 명세에 존재하지만 실제로 미구현된 핵심 인프라 구축

#### 6.0.1 speckit 산출물 소비 — 에이전트 직접 읽기 (확정)

> **⚠️ 설계 변경 (2026-03-13)**: 별도 파싱/변환 레이어를 두지 않고, 에이전트가 직접 읽기 방식으로 확정됨.

오케스트레이터는 speckit 산출물을 파싱/변환하지 않고, **파일 경로만** inbox 메시지에 포함한다. 에이전트가 Read 도구로 마크다운/YAML을 네이티브로 읽고 이해한다.

```
오케스트레이터                         에이전트
─────────────                         ─────────────
inbox 메시지에 파일 경로 포함    →    Read 도구로 직접 읽기
{ "context": {                        spec.md, plan.md, tasks.md,
    "references": [                   contracts/*.yml 을 네이티브 이해
      ".specify/specs/001/spec.md",
      ".specify/specs/001/plan.md",
      ...
    ]
  }
}
```

**[P] 마커 처리**: PM 에이전트가 tasks.md를 읽고 스케줄링 계획(JSON)을 오케스트레이터에 보고. 오케스트레이터는 PM의 계획대로 에이전트 디스패치.

**기대 효과**: 파서 구현 비용 제거 + LLM의 네이티브 마크다운 이해 능력 활용 + speckit 템플릿 변경에 자동 적응.

#### 6.0.2 speckit 셸 스크립트 → pipeline.yml `type: shell` step 흡수

speckit의 결정론적 스크립트를 Pylon 파이프라인의 셸 step으로 통합:

```yaml
# .pylon/pipeline.yml
stages:
  # ─── 결정론적 60% ───────────────────────
  - name: create_feature
    type: shell
    command: ".specify/scripts/bash/create-new-feature.sh {{feature_number}}"

  - name: setup_plan
    type: shell
    command: ".specify/scripts/bash/setup-plan.sh {{feature_number}}"

  - name: check_prerequisites
    type: shell
    command: ".specify/scripts/bash/check-prerequisites.sh {{feature_number}}"
    output: json                    # 완료도 검증 결과를 파싱

  - name: update_agent_context
    type: shell
    command: ".specify/scripts/bash/update-agent-context.sh"

  # ─── 에이전트 40% ──────────────────────
  - name: spec_review
    type: agent
    agent: po
    input: "specs/{{feature_number}}/spec.md"

  - name: plan_creation
    type: agent
    agent: architect
    input: "specs/{{feature_number}}/plan.md"
    validate_against: "constitution.md"    # 헌법 검증

  - name: implementation
    type: agent
    agent: "{{assigned_agent}}"
    input: "specs/{{feature_number}}/tasks.md"

  # ─── 결정론적 마무리 ───────────────────
  - name: verify
    type: shell
    command: "pylon verify"

  - name: create_pr
    type: shell
    command: "gh pr create ..."
```

**기대 효과**:
- Frequency의 40/60 분리가 speckit 스크립트를 통해 구체화
- 비용 예측 가능 (에이전트 단계만 토큰 소비)
- 셸 단계는 즉시 실행 → 파이프라인 속도 향상

#### 6.0.3 Constitution (헌법) 검증 게이트 — 별도 reviewer 에이전트 (확정)

> **⚠️ 설계 변경 (2026-03-13)**: 오케스트레이터의 자동 검증이 아닌, 별도 reviewer 에이전트에 의한 독립적 검증으로 확정됨.

speckit의 `constitution.md` 준수 여부를 **별도 검증 에이전트 (reviewer)**가 독립적/객관적으로 검증한다.

**검증 흐름**:
```
에이전트가 산출물 작성 완료
    ↓
reviewer 에이전트: constitution.md 대비 검증
    ↓ (통과) → 다음 단계 진행
    ↓ (실패) → 위반 항목 + 사유를 원래 에이전트에 전달
    ↓
원래 에이전트: 수정 후 재제출
    ↓
reviewer 에이전트: 재검증 (1회)
    ↓ (통과) → 다음 단계 진행
    ↓ (재검증 실패) → 파이프라인 일시정지 + 사람 에스컬레이션
```

**기대 효과**: 산출물 작성자와 검증자 분리로 객관성 확보 + 실패 이력 추적 가능 + 사람 에스컬레이션으로 무한 루프 방지.

#### 6.0.4 inbox/outbox 통신 프로토콜 구현

pylon-spec.md에 정의된 통신 프로토콜의 실제 구현. 현재 runtime 디렉토리, 메시지 스키마, fsnotify 감지 등이 모두 미구현.

#### 6.0.5 에이전트 프롬프트 구체화

현재 `developer.md` 2줄 수준을 pylon-spec.md Section 5 수준으로 상세화. speckit의 `update-agent-context.sh`가 plan.md → CLAUDE.md 변환하는 패턴을 참고.

### Phase 1 — 운영 안정성

> Phase 0 기반 위에 안정적 운영을 위한 메커니즘

#### 6.1 동시성 제어 세분화

**출처**: Frequency (4계층 동시성) + speckit `[P]` 마커

```yaml
# config.yml 확장
runtime:
  max_concurrent: 5
  concurrency:
    per_project: 2          # 프로젝트당 동시 에이전트 제한
    per_stage: 3             # 파이프라인 단계당 제한
    locks:                   # Named resource lock
      - database-migration
      - shared-config
```

speckit `tasks.md`의 `[P]` 마커를 파싱하여 병렬 가능 태스크를 자동 식별:

```markdown
# tasks.md 예시
## Phase 1
- [P] API 엔드포인트 구현     ← 병렬 가능
- [P] 프론트엔드 컴포넌트     ← 병렬 가능
- [ ] 통합 테스트             ← 직렬 (위 두 작업 완료 후)
```

#### 6.2 Dead-letter 큐 구조화

**출처**: Frequency

```sql
CREATE TABLE dead_letter (
    task_id      TEXT PRIMARY KEY,
    pipeline_id  TEXT NOT NULL,
    agent_name   TEXT NOT NULL,
    attempts     TEXT NOT NULL,       -- JSON: [{attempt: 1, error: "...", log_path: "..."}]
    failure_type TEXT NOT NULL,       -- transient | permanent | unknown
    root_cause   TEXT,                -- PM의 분석 결과
    resolution   TEXT,                -- manual_fix | retry | abandoned
    created_at   DATETIME DEFAULT CURRENT_TIMESTAMP,
    resolved_at  DATETIME
);
```

#### 6.3 verify 품질 게이트 강화

**출처**: HN 댓글 (Test Theatre) + speckit `check-prerequisites.sh`

```yaml
# .pylon/verify.yml 확장
verification:
  # 기본 (Phase 0에서 실제 구현)
  build: "go build ./..."
  test: "go test ./..."
  lint: "golangci-lint run"

  # 고도화 (Phase 1)
  prerequisites:
    enabled: true
    command: ".specify/scripts/bash/check-prerequisites.sh"
    format: json
  coverage:
    enabled: true
    command: "go test -coverprofile=coverage.out ./..."
    threshold: 0.8

  # 추가 고도화 (Phase 2)
  mutation:
    enabled: false
    command: "go-mutesting ./..."
    threshold: 0.6
```

### Phase 2 — 에이전트 지능 강화

> 에이전트의 매칭 정확도와 실패 학습 능력 향상

#### 6.4 에이전트 카드 (Agent Card) 메타데이터

**출처**: Unratified.org (`.well-known/agent-card.json`)

```yaml
# .pylon/agents/backend-dev.md frontmatter 확장
name: backend-dev
role: Backend Developer
capabilities:
  languages: [go]
  frameworks: [echo, sqlc]
  domains: [authentication, database, api]
inputs:
  accepts: [api_contract, schema_definition, test_spec]
outputs:
  produces: [implementation, unit_tests, migration]
```

PM이 태스크와 에이전트를 프로그래매틱하게 매칭. speckit의 `contracts/`에 정의된 API 타입과 에이전트의 `accepts`를 자동 매칭.

#### 6.5 블랙보드 Confidence 기반 자동 메커니즘

**출처**: Unratified.org (Epistemic Transparency)

| confidence 범위 | 동작 |
|----------------|------|
| >= 0.8 | project_memory로 자동 승격 |
| 0.5 ~ 0.8 | 후속 에이전트에게 "참고" 플래그로 전달 |
| < 0.5 | 후속 에이전트에게 "검증 필요" 플래그로 전달 |
| 반론 발생 | 기존 항목의 confidence 하향 + 새 항목 생성 |

speckit의 `constitution.md`에 정의된 원칙을 Confidence 1.0으로 블랙보드에 사전 등록하여, 에이전트 결정이 헌법에 부합하는지 자동 검증.

#### 6.6 템플릿 시스템

**출처**: speckit `templates/`

에이전트 산출물(spec, plan, tasks, contracts)의 구조를 표준화하는 템플릿:

```
.pylon/templates/
├── spec.md.tmpl        # 사용자 스토리 + 수용 기준 구조
├── plan.md.tmpl        # 기술 접근 + 헌법 검증 체크리스트
├── tasks.md.tmpl       # Phase별 분해 + [P] 마커 가이드
└── contract.yml.tmpl   # API 계약 스키마
```

에이전트가 산출물을 작성할 때 템플릿을 참조하여 일관된 품질 보장.

### Phase 3 — 자율성 확장

> 기본 시스템이 안정된 후의 장기 비전

#### 6.7 Proposal 기반 에이전트 자율성

**출처**: Unratified.org

에이전트가 자발적으로 작업을 제안하는 모드:
```
Tech Writer가 코드 변경 감지
  → "문서 업데이트 필요" 제안 발행
  → 오케스트레이터가 제안 평가
  → 승인 시 자동 실행
```

#### 6.8 Cross-pipeline 선언적 의존성

**출처**: Frequency

```yaml
pipeline: deploy-web
depends_on:
  - pipeline: build-api
    stage: pr_merged
  - pipeline: e2e-test
    stage: verify_passed
```

#### 6.9 Mutation testing 통합

**출처**: HN 댓글 (Test Theatre)

`verify.yml`의 `mutation` 섹션 활성화. 기본 검증 + 커버리지가 안정된 후.

---

## 7. 종합 평가 매트릭스

| 소스 | 핵심 교훈 | Pylon 적용 가능성 | 우선순위 | 제안 섹션 |
|------|-----------|-----------------|---------|----------|
| HN 댓글 | 좁은 범위 + 명확한 목표 | 이미 설계에 반영 | — | — |
| HN 댓글 | Test Theatre 주의 | verify.yml 고도화 | Phase 1 | 6.3 |
| HN 댓글 | 200-500줄 컨텍스트 한계 | 200줄 제한과 부합 | — | — |
| HN 댓글 | 컨텍스트 분리 검증 | 결정론적 검증 step | Phase 0 | 6.0.2 |
| Frequency | **40/60 에이전트/셸 비율** | **speckit 스크립트로 실현** | **Phase 0** | 6.0.2 |
| Frequency | **4계층 동시성 제어** | per-project/named lock | **Phase 1** | 6.1 |
| Frequency | Dead-letter 큐 | 실패 분석 구조화 | Phase 1 | 6.2 |
| Frequency | Cross-pipeline 의존성 | 선언적 파이프라인 의존성 | Phase 3 | 6.8 |
| Frequency | Repo-local JSON state | SQLite가 더 나은 선택 | — | — |
| Unratified | **구체성 = 통제** | **speckit contracts/로 실현** | **Phase 0** | 6.0.1 |
| Unratified | Agent Card 발견 | capabilities 메타데이터 | Phase 2 | 6.4 |
| Unratified | Epistemic transparency | confidence 활용 강화 | Phase 2 | 6.5 |
| Unratified | Proposal 기반 자율성 | 에이전트 자발적 제안 | Phase 3 | 6.7 |
| Unratified | Git-PR 트랜스포트 | file-based inbox/outbox이 유사 | — | — |
| **semicolon 실사** | **명세 vs 현실 갭** | **Phase 0 신설의 근거** | **Phase 0** | 0.1 |
| **semicolon 실사** | **speckit = 실전 레퍼런스** | **모든 Phase의 입력 소스** | **전체** | 0.3 |
| **semicolon 실사** | **Constitution 검증 게이트** | **에이전트 품질 보장** | **Phase 0** | 6.0.3 |
| **semicolon 실사** | **템플릿 시스템** | **산출물 품질 표준화** | Phase 2 | 6.6 |

---

## 8. 구현 로드맵 요약

```
Phase 0: 기반 구축 + speckit 연동
├── 6.0.1 speckit 산출물 소비 (에이전트 직접 읽기, 파싱 레이어 없음)
├── 6.0.2 speckit 셸 스크립트 → pipeline.yml shell step
├── 6.0.3 Constitution 검증 게이트 (reviewer 에이전트 + /speckit.analyze)
├── 6.0.4 inbox/outbox 통신 프로토콜 실제 구현
├── 6.0.5 에이전트 프롬프트 구체화
├── PO 에이전트: /speckit.clarify 활용 (역질문 지원)
└── reviewer 에이전트: /speckit.analyze 활용 (constitution 검증)

Phase 1: 운영 안정성
├── 6.1 동시성 세분화 (per_project, locks, [P] 마커 연동)
├── 6.2 Dead-letter 큐
├── 6.3 verify 품질 게이트 강화
└── 6.4* Hooks 시스템 (speckit Extension/Hook 연계: before_implement, after_tasks 등)

Phase 2: 에이전트 지능 강화
├── 6.5 Agent Card 메타데이터 (capabilities accepts/produces)
├── 6.6 Confidence 기반 블랙보드
└── 6.7 템플릿 시스템

Phase 3: 자율성 확장
├── 6.8 Proposal 기반 에이전트 자율성
├── 6.9 Cross-pipeline 선언적 의존성
└── 6.10 Mutation testing 통합
```

---

## 9. 미결사항 목록 (2026-03-13 검토)

> 3개 독립 에이전트에 의한 병렬 검토 결과.
> 심각도 높음 항목은 Phase 0 시작 전 해결 필요.

### 9.1 에이전트 역할 재정의 (speckit 전용 모드) — ✅ 해결됨

**심각도**: 🔴 높음 (Phase 0 설계에 영향) → **해결 완료 (2026-03-13)**

**확정 결정**: speckit 전용 모드에서 에이전트 역할을 재정의함. pylon-spec.md Section 6 "speckit 전용 모드 에이전트 역할" 및 Section 8 "에이전트별 speckit 산출물 수정 권한 매트릭스"에 반영.

| 에이전트 | speckit 모드 역할 | 산출물 수정 권한 |
|---------|------------------|----------------|
| **PO** | 보완자 (Enricher) | spec.md: 제안 → 사람 승인 후 write |
| **Architect** | 검증 + 조정자 | plan.md, contracts/ (write 직접). constitution.md 대비 기술 적합성 검증 |
| **PM** | 할당 + 조율자 | tasks.md (read-only). [P] 마커 파싱 → 스케줄링 계획 → 에이전트 디스패치 |
| **Developer** | 구현자 | contracts/ 제한적 write (사소한 조정만). 구조적 변경은 Architect 에스컬레이션 |
| **Tech Writer** | 도메인 지식 전담 | `.pylon/domain/*` (write), context.md (write), constitution.md (read-only) |

**도메인 지식 2계층 분리**: constitution.md는 사람 소유(AI read-only), `.pylon/domain/*`는 AI 소유(자동 생성/갱신)

### 9.2 speckit 산출물 소비 방식 — ✅ 해결됨

**심각도**: 🔴 높음 (Phase 0의 6.0.1 구현 전제) → **해결 완료 (2026-03-13)**

**확정 결정**: 별도 파싱/변환 레이어를 두지 않고, **에이전트가 직접 읽기** 방식으로 결정.

- **오케스트레이터**: 파일 경로만 전달 (inbox 메시지의 `context.references`에 speckit 파일 경로 포함)
- **에이전트**: Read 도구로 마크다운/YAML을 네이티브로 읽고 이해
- **[P] 마커**: PM 에이전트가 tasks.md를 읽고 스케줄링 계획(JSON)을 오케스트레이터에 보고 → 오케스트레이터가 PM의 계획대로 에이전트 디스패치

**근거**: LLM 에이전트가 마크다운을 네이티브로 이해할 수 있으므로, 별도 파서 구현은 불필요한 복잡도 추가. pylon-spec.md Section 8 "speckit 산출물 소비 방식"에 반영.

### 9.3 Constitution 검증 실패 경로 — ✅ 해결됨

**심각도**: 🔴 높음 (Phase 0의 6.0.3 구현 전제) → **해결 완료 (2026-03-13)**

**확정 결정**:

- **검증 주체**: 별도 검증 에이전트 (reviewer) — 독립적/객관적 검증, 결과 추적 가능
- **실패 정책**: 재시도 1회 → 사람 에스컬레이션
  - reviewer가 산출물 반려 → 위반 항목 + 사유 전달 → 원래 에이전트 수정 → reviewer 재검증
  - 재검증 실패 → 파이프라인 일시정지 + 사람에게 알림

**근거**: 산출물을 작성한 에이전트와 검증 에이전트를 분리하여 객관성 확보 (HN 댓글 "컨텍스트 분리 검증" 원칙). pylon-spec.md Section 8 "Constitution 검증 흐름"에 반영.

### 9.4 speckit 위치 및 접근 방식 — ✅ 해결됨

**심각도**: 🔴 높음 (Phase 0 시작의 첫 번째 기술 결정) → **해결 완료 (2026-03-13)**

**확정 결정**: speckit = Pylon 내장. `pylon add-project` 시 `specify init` 자동 실행.

- **설치**: pylon 설치 시 speckit 동시 설치
- **프로젝트 초기화**: `pylon add-project` → 해당 프로젝트에 `specify init` 실행 → `.specify/` 생성
- **auto-detect/fallback 불필요**: 모든 프로젝트에 `.specify/` 존재가 보장됨

**근거**: speckit을 외부 의존성이 아닌 내장 도구로 취급하여 설정 복잡도를 제거. pylon-spec.md Section 7 `pylon add-project`에 반영.

### 9.5 "연동 엔진" vs "흡수"의 경계 미확정 — ✅ 해결됨

**심각도**: 🟠 높음 (Phase 0 설계 방향 결정) → **해결 완료 (2026-03-13)**

**확정 결정**: **speckit = Pylon 내장** (9.4 결정)으로 경계 문제 자체가 해소됨.

- speckit은 Pylon에 내장되어 `pylon add-project` 시 `specify init`이 자동 실행
- speckit 원본 파일(`.specify/`)은 그대로 유지, Pylon이 런타임에 파일 경로만 전달
- 에이전트가 speckit 산출물을 직접 읽기 (9.2 결정)하므로 파싱/변환 레이어 불필요
- "흡수"가 아닌 "내장 + 런타임 동적 바인딩" — speckit의 파일 구조와 독립적 진화를 보장하면서도 Pylon 설치 시 동시 제공

### 9.6 셸 step 에러 처리 — ✅ 해결됨

**심각도**: 🟠 높음 (Phase 0 안정성) → **해결 완료 (2026-03-13)**

**확정 결정**: 기본 동작 즉시 중단 (halt, `set -e` 스타일) + step별 `on_error` override 지원.

| on_error 값 | 동작 |
|-------------|------|
| `halt` (기본) | 즉시 중단 |
| `retry` | 재시도 (`max_retries` 필수, `timeout` 선택) |
| `continue` | 경고만 남기고 계속 |
| `escalate` | 사람에게 알림 + 일시정지 |

pylon-spec.md Section 8 "셸 step 에러 처리"에 반영.

### 9.7 pylon-spec.md 미결 사항 7/9개 미충당 — ✅ 해결됨

**심각도**: 🟡 중간 (고도화 문서의 범위 문제) → **해결 완료 (2026-03-13)**

**확정 결정**: Hooks 시스템만 Phase 1 로드맵에 포함. 나머지는 기존 Phase 배치를 유지.

| 미결 사항 | 결정 |
|----------|------|
| TUI 대화 인터페이스 UX | 고도화 문서 범위 밖 — pylon-spec.md 미결 유지 |
| Dashboard SSE 이벤트 포맷 | 고도화 문서 범위 밖 — pylon-spec.md 미결 유지 |
| Compaction 트리거 구현 | pylon-spec.md 미결 유지 (에이전트 주도 방식으로 이미 설계됨) |
| 벡터 임베딩 검색 | Phase 3 이후 — pylon-spec.md 미결 유지 |
| **Hooks 시스템 설계** | **Phase 1에 포함 확정** (speckit Extension/Hook 연계: `before_implement`, `after_tasks` 등) |
| 설정 계층화 (config.local.yml) | pylon-spec.md 미결 유지 |
| Skills 패턴 상세 설계 | pylon-spec.md 미결 유지 |

### 9.8 speckit 미반영 기능 3건 — ✅ 해결됨

**심각도**: 🟡 중간 (연동 설계 최적화) → **해결 완료 (2026-03-13)**

**확정 결정**: 3건 전부 로드맵에 반영.

| speckit 기능 | 반영 위치 | 설명 |
|-------------|----------|------|
| **Extension/Hook 시스템** | **Phase 1** | speckit hook → Pylon 트리거 연동. pylon-spec.md Section 17 Hooks 시스템에 반영 |
| **`/speckit.analyze`** | **Phase 0 (reviewer 에이전트)** | reviewer 에이전트가 `/speckit.analyze` 활용하여 산출물 간 교차 일관성 + constitution.md 준수 검증 |
| **`/speckit.clarify`** | **Phase 0 (PO 에이전트)** | PO 에이전트가 speckit 모드에서 `/speckit.clarify` 호출하여 모호한 요구사항을 질문으로 명확화 |

### 9.9 SQLite 스키마 확장 필요 — ✅ 해결됨

**심각도**: 🟡 중간 (Phase 0 구현 시) → **해결 완료 (2026-03-13)**

**확정 결정**: 구현 단계에서 정의. 아래 스키마 변경 방향은 확정하되, 정확한 DDL은 Phase 0 구현 시 결정.

| 테이블 | 추가 필요 | 용도 |
|--------|----------|------|
| pipeline_state | `spec_path TEXT` | speckit feature 디렉토리 추적 |
| project_memory | `type TEXT DEFAULT 'learned'` | `system` (constitution) vs `learned` (에이전트 학습) 구분 |
| blackboard | constitution 항목의 `author: 'system'` + 수정 불가 제약 | 헌법 원칙 보호 |
| message_queue | `source TEXT DEFAULT 'generated'` | `user_defined` (speckit) vs `generated` (에이전트) 구분 |

### 9.10 Phase 간 상호작용 미정의 — ✅ 결정됨 (구현 시 상세화 예정)

**심각도**: 🟡 중간 (Phase 1~2 설계 시) → **방향성 확정 (2026-03-13)**

**확정 결정**: 방향성 확정, 상세 구현은 해당 Phase 착수 시 pylon-spec.md에 반영 예정:

- **Phase 1 동시성 × Phase 2 Agent Card**: capabilities 매칭 → 동시성 제약 순서 (매칭 먼저, 제약 나중)
- **Phase 0 Constitution × Phase 2 Confidence**: Constitution 통과 시 Confidence 초기값 >= 0.8 (권장)
- **[P] 마커 파싱 위치**: Phase 0에서 PM이 tasks.md를 읽고 스케줄링 계획 JSON을 보고 → Phase 1에서 동시성 제어 레이어가 해당 JSON을 입력으로 사용

### 9.11 에이전트의 speckit 산출물 수정 정책 — ✅ 해결됨

**심각도**: 🟡 중간 (Phase 0~1) → **해결 완료 (2026-03-13)**

**확정 결정**: 9.1의 에이전트별 권한 체계로 해결됨. pylon-spec.md Section 8 "에이전트별 speckit 산출물 수정 권한 매트릭스"에 전체 권한 표가 정의됨.

| 산출물 | PO | Architect | PM | Developer | Tech Writer | Reviewer |
|-------|----|-----------|----|-----------|-------------|----------|
| spec.md | 제안→사람승인 | read | read | read | read | read |
| plan.md | read | **write** | read | read | read | read |
| tasks.md | read | read | **read-only** | read | read | read |
| contracts/ | read | **write** | read | **제한적 write** | read | read |
| constitution.md | read | read | read | read | **read-only** | read |

### 9.12 Phase 전환 기준 부재 — ✅ 해결됨

**심각도**: 🟢 낮음 → **해결 완료 (2026-03-13)**

**확정 결정**: 구현 단계에서 정의. 아래 방향성만 확정:

- Phase 0 → Phase 1: inbox/outbox 통신 동작 + 최소 1회 end-to-end 파이프라인 성공
- Phase 1 → Phase 2: 동시성 제어 하에서 3회 이상 안정적 파이프라인 실행
- Phase 2 → Phase 3: Agent Card 기반 자동 매칭 성공률 80% 이상
