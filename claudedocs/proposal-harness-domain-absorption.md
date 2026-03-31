# Pylon에 Harness 도메인 영역 흡수 전략 제안서 (v2)

> 작성일: 2026-03-29
> v2 개정: 단일 진입점 자동 라우팅 아키텍처

---

## v1 → v2 핵심 변경점

| 항목 | v1 (폐기) | v2 (채택) |
|------|-----------|-----------|
| 도메인 선택 | `pylon init --domain research` 또는 `pylon harness "리서치 팀"` | **자동 감지** — PO가 요구사항 분석하여 도메인 판별 |
| 에이전트 설치 | 선택한 프리셋의 에이전트만 설치 | **모든 도메인 에이전트 사전 설치** |
| 사용자 경험 | 도메인을 명시적으로 선택/요청 | **`pylon` 실행 → 요구사항 전달 → 끝** |
| 설계 영감 | Harness의 "하네스 구성해줘" 패턴 | **OMC Ralph** — 단일 진입점, 시스템이 알아서 판단 |
| `pylon harness` 커맨드 | Phase 3에서 추가 | **제거** — 별도 커맨드 불필요 |
| config.yml `domain.preset` | 사용자가 설정 | **제거** — 런타임 자동 감지 |

**핵심 철학 전환**: "사용자가 도메인을 선택한다" → **"PO가 요구사항에서 도메인을 읽는다"**

---

## 1. 설계 영감: OMC Ralph 패턴

OMC의 Ralph가 성공적인 이유는 **사용자에게 모드 선택을 요구하지 않기** 때문이다.

```
OMC Ralph:   사용자 → "이것 해줘" → Ralph가 알아서 PRD/실행/검증 루프
Pylon (v2):  사용자 → "이것 해줘" → PO가 알아서 도메인/워크플로우/에이전트 선택
```

Ralph에서 차용하는 패턴:
- **단일 진입점**: `pylon` 하나만 실행하면 됨 (Ralph가 `/ralph`만 치면 되듯이)
- **의도 자동 분석**: PO가 요구사항에서 도메인과 작업 유형을 추론
- **에이전트 자동 라우팅**: 분석 결과에 따라 적절한 에이전트와 워크플로우 자동 선택
- **사전 설치된 도구**: 모든 도메인 에이전트/스킬이 이미 설치되어 있어 즉시 사용 가능

---

## 2. 아키텍처: 단일 진입점 자동 라우팅

### 2.1 현재 구조 (소프트웨어 개발 전용)

```
사용자 → pylon → Claude Code TUI (PO)
                      │
                      ├── "소프트웨어 개발" 고정 가정
                      ├── 고정 파이프라인 (8단계)
                      └── 23종 소프트웨어 에이전트만 사용 가능
```

### 2.2 v2 구조 (도메인 자동 라우팅)

```
사용자 → pylon → Claude Code TUI (Universal PO)
                      │
                      ├── [1] 요구사항 수신
                      ├── [2] 도메인 자동 감지 ←── PO 시스템 프롬프트에 라우팅 로직 내장
                      │       ├── 소프트웨어 개발 → feature/bugfix/hotfix 워크플로우
                      │       ├── 리서치/조사    → research 워크플로우
                      │       ├── 콘텐츠 제작    → content 워크플로우
                      │       ├── 마케팅          → marketing 워크플로우
                      │       └── 범용/혼합      → PO가 판단하여 조합
                      │
                      ├── [3] 워크플로우 자동 선택
                      │       └── config.yml의 workflow.default_workflow 대신
                      │           PO가 요구사항에 맞는 워크플로우 직접 선택
                      │
                      └── [4] 도메인별 에이전트 오케스트레이션
                              ├── 소프트웨어: architect, backend-dev, frontend-dev, ...
                              ├── 리서치: lead-researcher, web-searcher, fact-checker, ...
                              ├── 콘텐츠: writer, editor, seo-specialist, ...
                              └── 마케팅: market-researcher, copywriter, data-analyst, ...
```

### 2.3 왜 이 구조가 더 나은가

| 관점 | v1 (명시적 선택) | v2 (자동 라우팅) |
|------|-----------------|-----------------|
| **사용자 경험** | "먼저 도메인 선택 → 그 다음 작업" (2단계) | "작업만 말하면 됨" (1단계) |
| **혼합 작업** | 도메인 변경 시 재설정 필요 | PO가 작업별로 도메인 전환 |
| **학습 곡선** | 프리셋 목록 파악 필요 | 없음 — 그냥 말하면 됨 |
| **유연성** | 프리셋에 없는 도메인은 `custom` 선택 | PO가 기존 에이전트 조합으로 대응 |

---

## 3. 구체적 변경 지점

### 3.1 config.yml 스키마 (v1 대비 단순화)

```yaml
version: "0.2"

# v1의 domain.preset, domain.architecture_pattern 제거
# 도메인 선택은 PO가 런타임에 자동 수행

skills:
  enabled: true
  preload_to_agents: true
  progressive_disclosure: true
```

**변경 파일**: `internal/config/config.go`
- `SkillsConfig` 구조체만 추가 (v1의 `DomainConfig` 제거)
- `applyDefaults()`에 skills 기본값 추가

```go
// v1의 DomainConfig 제거 — 도메인은 런타임 자동 감지
// SkillsConfig만 추가
type SkillsConfig struct {
    Enabled               bool `yaml:"enabled"`
    PreloadToAgents       bool `yaml:"preload_to_agents"`
    ProgressiveDisclosure bool `yaml:"progressive_disclosure"`
}
```

### 3.2 에이전트 전체 사전 설치 (init 변경)

**변경 파일**: `internal/cli/init_cmd.go`

v1에서는 `pylon init --domain research`로 리서치 에이전트만 설치했지만,
v2에서는 **모든 도메인의 에이전트를 한번에 설치**한다.

```go
//go:embed agents/*.md
var embeddedAgents embed.FS  // 기존: 23종 소프트웨어 에이전트

// v2: 모든 도메인 에이전트 포함 (카테고리별 서브디렉토리)
//go:embed agents/software/*.md
var softwareAgents embed.FS

//go:embed agents/research/*.md
var researchAgents embed.FS

//go:embed agents/content/*.md
var contentAgents embed.FS

//go:embed agents/marketing/*.md
var marketingAgents embed.FS
```

`writeAgentTemplates()` 리팩토링:
```go
func writeAgentTemplates(pylonDir string) error {
    // 모든 도메인의 에이전트를 한번에 설치
    allDomains := []struct{
        fs   embed.FS
        dir  string
    }{
        {softwareAgents,  "agents/software"},
        {researchAgents,  "agents/research"},
        {contentAgents,   "agents/content"},
        {marketingAgents, "agents/marketing"},
    }
    for _, d := range allDomains {
        entries, _ := d.fs.ReadDir(d.dir)
        for _, entry := range entries {
            content, _ := d.fs.ReadFile(d.dir + "/" + entry.Name())
            path := filepath.Join(pylonDir, "agents", entry.Name())
            os.WriteFile(path, content, 0644)
        }
    }
    return nil
}
```

**결과 디렉토리**:
```
.pylon/agents/
├── # 소프트웨어 (기존 23종)
├── po.md, pm.md, architect.md, backend-dev.md, ...
├── # 리서치 (신규 5종)
├── lead-researcher.md, web-searcher.md, academic-analyst.md, ...
├── # 콘텐츠 (신규 5종)
├── content-strategist.md, writer.md, editor.md, ...
├── # 마케팅 (신규 5종)
└── market-researcher.md, copywriter.md, data-analyst.md, ...
```

> **참고**: Go 바이너리 내부에서는 `agents/software/`, `agents/research/` 등 서브디렉토리로 관리하되, `writeAgentTemplates()`가 설치 시 **평탄화(flatten)**하여 `.pylon/agents/`에 모든 `.md`를 동일 레벨로 복사한다. 서브디렉토리는 소스 코드 정리용이며, 설치 결과물은 항상 평탄 구조.

모든 에이전트가 같은 `.pylon/agents/` 평탄 디렉토리에 존재. PO가 요구사항에 따라 필요한 에이전트만 선택하여 호출. 에이전트 `.md` frontmatter에 `domain` 필드를 추가하여 PO의 선택을 보조:

```yaml
---
name: lead-researcher
role: Lead Researcher
domain: research          # PO가 도메인 라우팅 시 참조
---
```

**변경 파일**: `internal/config/agent.go`
```go
type AgentConfig struct {
    // ... 기존 필드
    Domain string   `yaml:"domain"` // 소속 도메인 (software/research/content/marketing)
    Skills []string `yaml:"skills"` // 연결된 스킬 이름 목록
}
```

### 3.3 PO 시스템 프롬프트에 도메인 라우팅 로직 주입 (핵심)

**변경 파일**: `internal/cli/launch.go` — `buildRootCLAUDEMD()`

이것이 v2의 **가장 핵심적인 변경**이다. 현재 `buildRootCLAUDEMD()`는 PO를 "소프트웨어 개발 오케스트레이터"로 고정하지만, v2에서는 **"범용 오케스트레이터 + 도메인 라우터"**로 전환한다.

현재 (`launch.go:307-311`):
```go
b.WriteString("# Pylon — AI 개발팀 오케스트레이터\n\n")
b.WriteString("당신은 Pylon의 루트 에이전트(PO)입니다.\n")
b.WriteString("사용자의 요구사항을 분석하고, AI 에이전트 팀을 오케스트레이션하여\n")
b.WriteString("분석 → 설계 → 구현 → 검증 → PR 생성까지 자동 수행합니다.\n\n")
```

v2:
```go
b.WriteString("# Pylon — AI 멀티도메인 오케스트레이터\n\n")
b.WriteString("당신은 Pylon의 루트 에이전트(PO)입니다.\n")
b.WriteString("사용자의 요구사항을 분석하여 적절한 도메인과 워크플로우를 자동 선택하고,\n")
b.WriteString("해당 도메인의 전문 에이전트 팀을 오케스트레이션합니다.\n\n")
```

**도메인 라우팅 섹션 추가**:
```go
b.WriteString("## 도메인 자동 감지\n\n")
b.WriteString("사용자의 요구사항을 분석하여 다음 도메인 중 하나를 자동 선택합니다:\n\n")
b.WriteString("| 도메인 | 키워드/신호 | 워크플로우 | 핵심 에이전트 |\n")
b.WriteString("|--------|-----------|-----------|-------------|\n")
b.WriteString("| **소프트웨어 개발** | 구현, 코드, API, 버그, PR, 테스트 | feature/bugfix/hotfix | architect, backend-dev, frontend-dev, test-engineer |\n")
b.WriteString("| **리서치/조사** | 조사, 분석, 비교, 보고서, 논문 | research | lead-researcher, web-searcher, academic-analyst, fact-checker |\n")
b.WriteString("| **콘텐츠 제작** | 글, 블로그, 문서, 번역, 편집 | content | content-strategist, writer, editor, seo-specialist |\n")
b.WriteString("| **마케팅** | 캠페인, 광고, SEO, 타겟, 퍼널 | marketing | market-researcher, copywriter, data-analyst |\n\n")
b.WriteString("도메인이 모호하면 사용자에게 확인하지 말고 가장 적합한 도메인을 선택하세요.\n")
b.WriteString("혼합 작업(예: '리서치 후 구현')은 단계별로 도메인을 전환합니다.\n\n")
```

**도메인별 파이프라인 섹션 교체**:

현재 `launch.go:335-345`의 고정 8단계 파이프라인을 도메인별 분기로 교체:

```go
b.WriteString("## 도메인별 파이프라인\n\n")
b.WriteString("### 소프트웨어 개발\n")
b.WriteString("PO 대화 → Architect 분석 → PM 분해 → Agent 실행 → 검증 → PR\n\n")
b.WriteString("### 리서치/조사\n")
b.WriteString("PO 대화 → 병렬 조사 (web/academic/community) → 교차 검증 → 보고서 작성 → 팩트 체크\n\n")
b.WriteString("### 콘텐츠 제작\n")
b.WriteString("PO 대화 → 초안 작성 → 편집/리뷰 → (피드백 시 재작성 루프) → 최종본\n\n")
b.WriteString("### 마케팅\n")
b.WriteString("PO 대화 → 시장 조사 → 전략 수립 → 콘텐츠 생성 → 검증\n\n")
```

**에이전트 목록 섹션도 도메인별로 확장**:

현재 `launch.go:377-381`의 에이전트 섹션에 모든 도메인 에이전트를 나열:

```go
// 에이전트 목록을 동적으로 생성 (설치된 에이전트 파일 기반)
agents, _ := discoverAgentsByDomain(pylonDir)
for domain, agentList := range agents {
    b.WriteString(fmt.Sprintf("### %s 에이전트\n", domainLabel(domain)))
    for _, a := range agentList {
        b.WriteString(fmt.Sprintf("- `%s` — %s\n", a.Name, a.Role))
    }
    b.WriteString("\n")
}
```

**신규 헬퍼 함수**:
```go
// discoverAgentsByDomain reads .pylon/agents/ and groups agents by domain field.
func discoverAgentsByDomain(pylonDir string) (map[string][]config.AgentConfig, error) {
    result := make(map[string][]config.AgentConfig)
    entries, _ := os.ReadDir(filepath.Join(pylonDir, "agents"))
    for _, entry := range entries {
        if !strings.HasSuffix(entry.Name(), ".md") { continue }
        agent, err := config.ParseAgentFile(filepath.Join(pylonDir, "agents", entry.Name()))
        if err != nil { continue }
        domain := agent.Domain
        if domain == "" { domain = "software" }  // 기본값
        result[domain] = append(result[domain], *agent)
    }
    return result, nil
}
```

### 3.4 워크플로우 자동 선택 메커니즘

v1에서는 `config.yml`의 `workflow.default_workflow`로 고정 선택했지만,
v2에서는 **PO가 요구사항 분석 후 워크플로우를 직접 선택**한다.

#### 3.4.1 프롬프트 계층: PO 시스템 프롬프트에 라우팅 가이드

**변경 파일**: `internal/cli/launch.go` — `buildRootCLAUDEMD()` 내 워크플로우 가이드 추가

```go
b.WriteString("## 워크플로우 선택 가이드\n\n")
b.WriteString("요구사항 분석 후 적절한 워크플로우를 선택하세요:\n\n")
b.WriteString("| 상황 | 워크플로우 | 슬래시 커맨드 |\n")
b.WriteString("|------|-----------|-------------|\n")
b.WriteString("| 새 기능 구현 | feature | `/pl:pipeline --workflow=feature` |\n")
b.WriteString("| 버그 수정 | bugfix | `/pl:pipeline --workflow=bugfix` |\n")
b.WriteString("| 리서치/조사 | research | `/pl:pipeline --workflow=research` |\n")
b.WriteString("| 콘텐츠 제작 | content | `/pl:pipeline --workflow=content` |\n")
b.WriteString("| 마케팅 작업 | marketing | `/pl:pipeline --workflow=marketing` |\n\n")
b.WriteString("워크플로우를 지정하지 않으면 요구사항에서 자동 추론합니다.\n\n")
```

#### 3.4.2 코드 계층: init-pipeline.sh에서 워크플로우 YAML 로딩

현재 `init-pipeline.sh` (`internal/cli/scripts/bash/init-pipeline.sh`)는 파이프라인 초기화 시 고정된 산출물 구조를 생성한다. v2에서는 `--workflow` 인자를 받아 해당 YAML 템플릿을 로딩하는 분기를 추가한다.

**변경 파일**: `internal/cli/scripts/bash/init-pipeline.sh`
```bash
# v2: --workflow 인자 파싱
WORKFLOW="${WORKFLOW:-feature}"  # 기본값 feature (하위 호환)

# 워크플로우 YAML에서 스테이지 목록 추출
WORKFLOW_FILE=".pylon/workflows/${WORKFLOW}.yml"
if [ ! -f "$WORKFLOW_FILE" ]; then
    echo "워크플로우 '$WORKFLOW' 없음, feature 사용" >&2
    WORKFLOW_FILE=".pylon/workflows/feature.yml"
fi

# routing-decision.json 생성 (라우팅 투명성 확보)
cat > "${PIPELINE_DIR}/routing-decision.json" << EOF
{
  "detected_domain": "${DOMAIN}",
  "selected_workflow": "${WORKFLOW}",
  "timestamp": "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
}
EOF
```

**변경 파일**: `internal/orchestrator/pipeline.go`

오케스트레이터가 파이프라인 실행 시 `routing-decision.json`을 읽어 해당 워크플로우 템플릿의 스테이지 순서대로 진행:

```go
// loadWorkflowForPipeline reads routing-decision.json and loads the matching template.
func loadWorkflowForPipeline(pipelineDir string) (*workflow.WorkflowTemplate, error) {
    routingPath := filepath.Join(pipelineDir, "routing-decision.json")
    if _, err := os.Stat(routingPath); os.IsNotExist(err) {
        // 하위 호환: routing-decision.json 없으면 feature 워크플로우
        return workflow.LoadTemplate("feature")
    }
    data, _ := os.ReadFile(routingPath)
    var routing struct {
        SelectedWorkflow string `json:"selected_workflow"`
    }
    json.Unmarshal(data, &routing)
    return workflow.LoadTemplate(routing.SelectedWorkflow)
}
```

#### 3.4.3 투명성 계층: routing-decision.json (Architect 리뷰 반영)

PO의 라우팅 결정을 **관찰 가능한 산출물**로 기록한다. Pylon의 기존 "산출물 존재 = 스테이지 완료" 철학과 일관:

```json
{
  "detected_domain": "research",
  "confidence_signal": "키워드 '조사', '비교 분석', '보고서'에서 추론",
  "selected_workflow": "research",
  "available_agents": ["lead-researcher", "web-searcher", "fact-checker", "report-writer"],
  "timestamp": "2026-03-29T10:30:00Z"
}
```

이 파일은:
- 디버깅 시 "왜 이 워크플로우가 선택됐는지" 추적 가능
- `pylon status`에서 현재 워크플로우 표시 가능
- 잘못된 라우팅 시 사용자가 `--workflow` 플래그로 오버라이드 가능

**변경 파일**: `internal/cli/commands/pl-pipeline.md`

파이프라인 커맨드에 `--workflow` 옵션 추가 가이드:
```markdown
워크플로우를 지정하지 않으면 PO가 요구사항에서 자동 선택합니다.
명시적 오버라이드: `/pl:pipeline --workflow=research "AI 트렌드 조사"`
라우팅 결정은 routing-decision.json에 기록됩니다.
```

### 3.5 스킬 시스템 (v1과 동일)

**신규 파일**: `internal/config/skill.go`

```go
type SkillConfig struct {
    Name        string `yaml:"name"`
    Description string `yaml:"description"`
    Body        string `yaml:"-"`
    FilePath    string `yaml:"-"`
    References  []string `yaml:"-"`
}
```

스킬도 에이전트처럼 `.pylon/skills/`에 사전 설치:
```
.pylon/skills/
├── research-methodology.md      # 리서치 방법론
├── content-writing-guide.md     # 콘텐츠 작성 가이드
├── marketing-framework.md       # 마케팅 프레임워크
├── api-design-guide.md          # API 설계 가이드 (기존)
└── test-strategy.md             # 테스트 전략 (기존)
```

### 3.6 범용 스테이지 및 워크플로우 템플릿 (v1과 동일)

**변경 파일**: `internal/domain/stage.go` — 범용 스테이지 추가

```go
const (
    // 기존 소프트웨어 스테이지 유지...

    // 범용 추가
    StageFanOut          Stage = "fan_out"
    StageFanIn           Stage = "fan_in"
    StageExpertSelect    Stage = "expert_select"
    StageGenerate        Stage = "generate"
    StageValidate        Stage = "validate"
    StageSupervisorCheck Stage = "supervisor_check"
)
```

**신규 워크플로우 템플릿**: `internal/workflow/templates/`

```yaml
# research.yml
name: research
description: Multi-source research with cross-validation
stages:
  - init
  - po_conversation
  - fan_out
  - fan_in
  - generate
  - validate
  - completed
allow_dynamic_spawn: true

# content.yml
name: content
description: Content creation with review loops
stages:
  - init
  - po_conversation
  - generate
  - validate
  - completed
loops:
  - from: validate
    to: generate

# marketing.yml
name: marketing
description: Marketing campaign workflow
stages:
  - init
  - po_conversation
  - fan_out
  - fan_in
  - generate
  - validate
  - completed
```

### 3.7 ArtifactToStage 확장 (v1 Architect 리뷰 반영)

`StageFromArtifacts()`를 워크플로우별 산출물 매핑을 받도록 리팩토링:

```go
// internal/domain/stage.go
func StageFromArtifactsWithMap(existingFiles []string, artifactMap map[string]Stage) Stage {
    // artifactMap이 nil이면 기존 ArtifactToStage 사용 (하위 호환)
    m := artifactMap
    if m == nil {
        m = ArtifactToStage
    }
    // ... 기존 로직
}
```

---

## 4. 단계별 구현 로드맵

### Phase 1: PO 범용화 + 전체 에이전트 사전 설치 (~13 파일)

**목표**: 단일 진입점에서 PO가 모든 도메인을 처리할 수 있는 기반 구축

| 작업 | 파일 | 유형 |
|------|------|------|
| `SkillsConfig` 구조체 추가 | `internal/config/config.go` | 수정 |
| `AgentConfig`에 `domain`, `skills` 필드 추가 | `internal/config/agent.go` | 수정 |
| 스킬 파서 구현 | `internal/config/skill.go` | 신규 |
| 스킬 파서 테스트 | `internal/config/skill_test.go` | 신규 |
| 리서치 에이전트 5종 작성 | `internal/cli/agents/lead-researcher.md` 등 | 신규 ×5 |
| `writeAgentTemplates()` 전체 설치 리팩토링 | `internal/cli/init_cmd.go` | 수정 |
| `buildRootCLAUDEMD()` 범용 PO 프롬프트로 전환 | `internal/cli/launch.go` | 수정 |
| `discoverAgentsByDomain()` 헬퍼 추가 | `internal/cli/launch.go` | 수정 (포함) |
| `StageFromArtifactsWithMap()` 리팩토링 | `internal/domain/stage.go` | 수정 |
| `init-pipeline.sh`에 `--workflow` 분기 + `routing-decision.json` 생성 | `internal/cli/scripts/bash/init-pipeline.sh` | 수정 |
| `loadWorkflowForPipeline()` 워크플로우 로딩 | `internal/orchestrator/pipeline.go` | 수정 |

**예상 변경**: 신규 7개, 수정 7개 = **~14 파일**

### Phase 2: 비개발 워크플로우 + 스킬 시스템 (~14 파일)

**목표**: 리서치/콘텐츠/마케팅 워크플로우 추가, 스킬 주입 기능

| 작업 | 파일 | 유형 |
|------|------|------|
| 범용 스테이지 추가 | `internal/domain/stage.go` | 수정 |
| research 워크플로우 | `internal/workflow/templates/research.yml` | 신규 |
| content 워크플로우 | `internal/workflow/templates/content.yml` | 신규 |
| marketing 워크플로우 | `internal/workflow/templates/marketing.yml` | 신규 |
| 콘텐츠 에이전트 5종 | `internal/cli/agents/writer.md` 등 | 신규 ×5 |
| 마케팅 에이전트 5종 | `internal/cli/agents/market-researcher.md` 등 | 신규 ×5 |
| 스킬 파일 3종 | `internal/cli/skills/*.md` | 신규 ×3 |
| CLAUDE.md 빌드에 스킬 주입 | `internal/agent/claudemd.go` | 수정 |
| `pl-pipeline.md`에 `--workflow` 가이드 | `internal/cli/commands/pl-pipeline.md` | 수정 |

**예상 변경**: 신규 ~17개, 수정 3개 = **~20 파일** (대부분 에이전트/스킬 .md)

### Phase 3: PO 라우팅 고도화 + 혼합 작업 지원 (~8 파일)

**목표**: 혼합 도메인 작업 지원, PO 판단력 강화

| 작업 | 파일 | 유형 |
|------|------|------|
| PO 에이전트 정의 강화 (도메인 라우팅 역할 명시) | `internal/cli/agents/po.md` | 수정 |
| 혼합 워크플로우 (multi-domain) | `internal/workflow/templates/multi.yml` | 신규 |
| 워크플로우 전환 로직 (파이프라인 중간 변경) | `internal/orchestrator/pipeline.go` | 수정 |
| PO CLAUDE.md에 혼합 작업 가이드 추가 | `internal/cli/launch.go` | 수정 |
| `pylon sync-agents` 도메인 인식 | `internal/cli/sync_agents.go` | 수정 |
| sync 테스트 업데이트 | `internal/cli/sync_agents_test.go` | 수정 |
| 도메인별 도메인 지식 템플릿 | `internal/cli/domain_templates/*.md` | 신규 ×3 |

**예상 변경**: 신규 ~4개, 수정 ~5개 = **~9 파일**

### Phase 4 (미래): 사용자 에이전트/스킬 추가

- `pylon add-agent <name>` — 사용자 커스텀 에이전트 생성 가이드
- `pylon add-skill <name>` — 사용자 커스텀 스킬 생성 가이드

---

## 5. 핵심 설계 결정

### 5.1 왜 config.yml에 domain 설정이 없는가

v1에서는 `domain.preset: "research"`를 config.yml에 넣었지만, v2에서는 제거한다.

**이유**: Pylon 워크스페이스가 하나의 도메인에만 사용되리라는 가정이 틀리기 때문. 같은 워크스페이스에서 "코드 구현해줘"와 "시장 조사해줘"를 번갈아 요청할 수 있다. 도메인은 **워크스페이스의 속성이 아니라 요구사항의 속성**이다.

### 5.2 에이전트 평탄 디렉토리 구조

모든 도메인의 에이전트가 `.pylon/agents/`에 평탄하게 공존한다. 서브디렉토리 분리를 하지 않는 이유:
- Claude Code의 `agents/` 심링크가 평탄 구조를 기대
- PO가 `domain` frontmatter 필드로 라우팅 — 디렉토리 구조 불필요
- 사용자가 에이전트를 추가/수정할 때 단순

### 5.3 하위 호환성

- 기존 `version: "0.1"` 워크스페이스: 기존 23종 에이전트 + 기존 워크플로우 그대로 동작
- `pylon sync-agents --force`: 최신 에이전트(전체 도메인)로 갱신
- 새 에이전트의 `domain` 필드가 없으면 `software` 기본값

### 5.4 v1과의 차별점 요약

| 관점 | v1 | v2 |
|------|----|----|
| 도메인 결정 시점 | **설치 시** (init) | **실행 시** (PO 분석) |
| 결정 주체 | **사용자** | **PO 에이전트** |
| config 복잡도 | domain + skills 섹션 | skills 섹션만 |
| CLI 커맨드 | `pylon init --domain`, `pylon harness` | **변경 없음** — `pylon` 그대로 |
| 에이전트 설치 | 선택된 프리셋만 | 모든 도메인 전체 |
| 혼합 작업 | 워크스페이스 재설정 필요 | PO가 자연스럽게 전환 |

---

## 6. 리스크 및 완화

| 리스크 | 영향 | 완화 |
|--------|------|------|
| PO가 도메인을 잘못 판별 | 중간 | 라우팅 테이블을 CLAUDE.md에 명시적으로 제공; PO가 모호하면 사용자에게 확인 |
| 에이전트 수 증가 (23→38+)로 init 시간 증가 | 낮음 | embed.FS 기반이라 파일 복사 수준; 체감 차이 미미 |
| CLAUDE.md 컨텍스트 부담 증가 | 중간 | 도메인 라우팅 테이블은 ~20줄; PO가 선택 후 해당 에이전트만 호출하므로 실행 시 추가 부담 없음 |
| 비개발 도메인 에이전트 품질 | 중간 | 소프트웨어 에이전트 수준의 검증 후 릴리스; 커뮤니티 피드백 반영 |
| 기존 파이프라인 스크립트가 비개발 워크플로우에서 동작 불가 | 높음 | `run-verification.sh`는 빌드/테스트 전용 → 도메인별 검증 스크립트 분리 (Phase 2) |

---

## 7. 요약

```
Phase 1 (PO 범용화)    → 범용 PO + 전체 에이전트 사전 설치 + 라우팅 인프라  (~14 파일)
Phase 2 (워크플로우)    → 비개발 워크플로우 + 에이전트/스킬 추가            (~20 파일)
Phase 3 (고도화)       → 혼합 작업 지원 + PO 라우팅 강화                    (~9 파일)
Phase 4 (생태계)       → 사용자 에이전트/스킬 추가
```

**핵심 메시지**: 사용자에게 도메인을 선택하라고 요구하지 않는다. **PO가 요구사항에서 도메인을 읽고, 적절한 에이전트와 워크플로우를 자동으로 선택한다.** 마치 OMC Ralph가 "이것 해줘"만으로 PRD/실행/검증을 알아서 돌리듯이, Pylon은 "이것 해줘"만으로 소프트웨어 개발이든 리서치든 콘텐츠든 알아서 처리한다.
