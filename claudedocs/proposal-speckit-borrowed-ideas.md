# Speckit 차용 아이디어 — Pylon 파이프라인 개선 후보

**작성일**: 2026-05-14
**상태**: 제안 (구현 미착수)
**배경**: `.specify/` 폴더(spec-kit 0.8.0 로컬 설치 결과물)를 분석하여 pylon에 적용 가능한 패턴을 추출. `.specify/` 폴더 자체는 pylon 런타임이 참조하지 않아 제거 예정이며, 본 문서로 아이디어만 보존.

---

## 1. 사전 분석: 통합 현황

### 1.1 코드 수준 통합 — 없음

```
grep -rni "speckit|spec-kit|\.specify" internal/
→ internal/config/agent.go:124  "don't specify the type field"  (영어 단어, 무관)
```

pylon 바이너리/스크립트 어디서도 `.specify/`를 읽지 않음. 런타임 의존도 0.

### 1.2 기능 평행 구현 — 이미 존재

| Speckit 단계 (`.specify/workflows/speckit/workflow.yml`) | Pylon 대응 |
|---|---|
| `specify` | `pl-pipeline` Step 2 (PO 요구사항 분석) |
| `plan` | `pl-architect` |
| `tasks` | `pl-breakdown` |
| `implement` | `pl-execute` |
| (검증 단계 없음) | `pl-verify` |
| (PR 단계 없음) | `pl-pr` |
| `memory/constitution.md` | `.pylon/domain/{overview,glossary,practices}.md` |
| `templates/*-template.md` | PO/Architect 에이전트 프롬프트에 내재화 |
| `type: gate` (`on_reject: abort`) | `pl-pipeline.md` 의 `handoffs` 메뉴 |
| `integration: claude\|copilot\|gemini` | `config.yml: runtime.backend: claude-code` (단일) |

→ 핵심 SDD 사이클은 평행 구현 완료. pylon이 검증/PR/도메인별 워크플로 4종(software/research/content/marketing)을 더 보유.

---

## 2. 차용 후보 아이디어

### 2.1 워크플로를 YAML 데이터로 분리

**현재 상태** (pylon):
- 4종 워크플로(software/research/content/marketing)가 `internal/cli/commands/pl-pipeline.md`의 산문 마크다운 표 안에 박혀 있음
- 도메인 추가 시 markdown 본문 수정 필요
- 워크플로 변경이 코드/문서 양쪽에 영향

**speckit 패턴** (`.specify/workflows/speckit/workflow.yml`):
```yaml
schema_version: "1.0"
workflow:
  id: "speckit"
  name: "Full SDD Cycle"
  description: "Runs specify → plan → tasks → implement with review gates"
inputs:
  spec: { type: string, required: true, prompt: "Describe what you want to build" }
  scope: { type: string, default: "full", enum: ["full", "backend-only", "frontend-only"] }
steps:
  - id: specify
    command: speckit.specify
    input: { args: "{{ inputs.spec }}" }
  - id: review-spec
    type: gate
    options: [approve, reject]
    on_reject: abort
  - id: plan
    ...
```

**Pylon에 적용 시 가치**:
- 새 도메인 워크플로 추가가 yml 파일 1개 추가로 완결 (markdown 수정 불필요)
- 워크플로 정의를 데이터로 분리하면 검증/시각화/문서 자동 생성 가능
- `pl-pipeline.md`가 워크플로 메타데이터를 yml에서 읽는 단일 진입점으로 단순화됨

**제안 위치**:
```
internal/cli/workflows/
  software.yml         (feature/bugfix/hotfix)
  research.yml
  content.yml
  marketing.yml
```
embed로 바이너리에 포함 → `pl-pipeline`이 `--workflow=<id>`로 선택 또는 PO가 자동 라우팅.

**스키마 초안**:
```yaml
schema_version: "1.0"
workflow:
  id: software
  domain: software
  description: "기능 개발 파이프라인"
inputs:
  requirement: { type: string, required: true }
steps:
  - { id: po-analysis,   command: pl-pipeline,  step: 2 }
  - { id: architect,     command: pl-architect, requires: [po-analysis] }
  - { id: review-arch,   type: gate, on_reject: abort }   # 2.2 참조
  - { id: breakdown,     command: pl-breakdown }
  - { id: execute,       command: pl-execute, parallel: true }
  - { id: verify,        command: pl-verify }
  - { id: pr,            command: pl-pr }
```

**예상 비용**: 중. `pl-pipeline.md` 본문 재구성 + yml 로더(Go) + `embed` 추가.
**예상 이득**: 중-고. 도메인 확장 빈도가 높을수록 ROI 상승.

---

### 2.2 명시적 게이트 (자동 중단 조건)

**현재 상태** (pylon):
- `pl-pipeline.md` 의 `handoffs:` 메뉴는 **사용자가 어느 단계로 분기할지 선택**하는 UX 도구
- 검증 실패 시 자동으로 PR 생성을 막거나 워크플로를 중단하는 명시적 메커니즘 없음 (개별 에이전트가 알아서 처리)

**speckit 패턴**:
```yaml
- id: review-spec
  type: gate
  message: "Review the generated spec before planning."
  options: [approve, reject]
  on_reject: abort
```

**Pylon에 적용 시 가치**:
- `pl-verify` 실패 → `pl-pr` 자동 차단 같은 안전망을 워크플로 정의에서 선언적으로 표현
- 사용자 승인 게이트와 자동 검증 게이트를 통일된 모델로 처리
- 부분 실패 시 어디서 멈췄는지 파이프라인 상태(`runtime/`)에 명확히 기록됨

**제안 게이트 타입**:
| 타입 | 조건 | 실패 시 |
|---|---|---|
| `user-approval` | 사용자 명시적 승인 | `abort` 또는 `loop-back` |
| `verify` | 직전 단계 `pl-verify` 결과 통과 | `abort` 또는 `retry` |
| `quality` | 정량 임계값 (테스트 커버리지, 린트 점수) | `abort` 또는 `warn` |

**예상 비용**: 중. 2.1과 결합 시 한 번에 처리 가능.
**예상 이득**: 중. 검증 실패 후 PR 생성 사고를 구조적으로 방지.

---

### 2.3 백엔드 비종속 추상화 (어댑터 다중화)

**현재 상태** (pylon):
- `.pylon/config.yml: runtime.backend: claude-code` (단일 enum)
- 에이전트 정의(`agents/*.md`), 스킬(`skills/*.md`), 명령(`commands/*.md`)이 Claude Code 형식에 종속
- `.claude/agents/` 심볼릭 링크로 Claude CLI 네이티브 디스커버리 활용

**speckit 패턴**:
```yaml
requires:
  integrations:
    any: ["copilot", "claude", "gemini"]
steps:
  - id: specify
    command: speckit.specify
    integration: "{{ inputs.integration }}"
```
설치 시점에 `integrations/claude.manifest.json` 같은 정합 파일을 생성하여 어댑터별 산출물 추적.

**Pylon에 적용 시 가치**:
- Copilot CLI, Gemini CLI, Codex 등 다른 에이전트 백엔드 지원 가능성 확보
- 에이전트 정의를 백엔드 비종속 형식(중립 YAML/JSON)으로 두고, 백엔드별 변환기 분리

**선결 과제** (간단하지 않음):
- 현재 에이전트 정의가 Claude의 frontmatter/markdown 관습에 강하게 결합 (`description`, `tools` 필드 등)
- 스킬도 Claude의 `Skill` 도구 형식에 종속
- 백엔드별 도구 카탈로그가 다름 (예: Claude의 `Bash` ≠ Copilot의 `shell`)
- 정말 다중 백엔드가 필요한지 사용자 요구 검증 필요

**예상 비용**: 고. 에이전트/스킬/명령 형식 전반 재설계 필요.
**예상 이득**: 변동성 큼. 단일 백엔드(Claude Code)로 충분하다면 ROI 음수.

**판단**: 당장 착수 비추천. 다중 백엔드 요구가 실제로 들어오면 재검토.

---

## 3. 우선순위 및 다음 단계

| 아이디어 | 비용 | 이득 | 우선순위 |
|---|---|---|---|
| 2.1 워크플로 YAML 분리 | 중 | 중-고 | **상** — 도메인 추가가 잦으면 우선 |
| 2.2 명시적 게이트 | 중 (2.1과 결합 시 저) | 중 | **중** — 2.1과 묶어서 진행 권장 |
| 2.3 백엔드 비종속 | 고 | 변동 | **하** — 트리거(사용자 요구)가 오기 전까지 보류 |

### 권장 진행 순서

1. **단기**: `.specify/` 폴더 제거 + `.gitignore` 추가 (이미 합의됨)
2. **중기 검토**: 2.1 + 2.2를 묶어 `specs/00X-workflow-yaml-extraction/spec.md` 로 정식 spec 작성 → speckit-spec 또는 자체 SDD 사이클로 구현
3. **장기 보류**: 2.3은 다중 백엔드 요구가 발생하면 재논의

---

## 4. 참조

- Speckit 워크플로 정의 예시: `.specify/workflows/speckit/workflow.yml` (제거 전 스냅샷)
- Speckit GitHub: https://github.com/github/spec-kit
- Pylon 현재 파이프라인: `internal/cli/commands/pl-pipeline.md`
- Pylon 자체 도메인 워크플로: `pl-pipeline.md` 안 산문 표 (4종)
- 관련 spec: `specs/001-multi-repo-pipeline/spec.md` (멀티 리포 파이프라인)
