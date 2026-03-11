# Pylon 프로젝트 초기화용 패턴 큐레이션

> 소스: https://github.com/shanraisshan/claude-code-best-practice
> 큐레이션일: 2026-03-07
> 원본: `claudedocs/collected-skills-and-agents.md`
>
> ⚠️ **참고**: 이 문서에서 언급되는 tmux 관련 내용은 더 이상 유효하지 않습니다. tmux 세션 기반 프로세스 격리는 설계에서 제거되었으며, 현재는 직접 프로세스 실행으로 대체되었습니다.

---

## 1. Pylon 초기화에 적용할 핵심 패턴 7가지

### 패턴 1: Self-Evolution (자기 진화)

**출처**: `presentation-curator` 에이전트의 Step 5

**원본 구조**:
```markdown
### Step 5: Self-Evolution (after every execution)
After completing changes, you MUST update your own knowledge to stay in sync.
- 5a. Update the Framework Skill
- 5b. Update the Structure Skill
- 5c. Cross-Doc Consistency
- 5d. Update This Agent (yourself)

## Learnings
_Findings from previous executions are recorded here._
- Hook-event references drifted across files...
- Do not use shorthand agent names in examples...
```

**Pylon 적용**: Tech Writer 에이전트에 적용. 도메인 지식(위키) 갱신 후 자신의 스킬/규칙도 함께 업데이트하는 자기 진화 워크플로우.

**`pylon init` 시 생성할 템플릿**:
```markdown
# .pylon/agents/tech-writer.md (해당 섹션)

### 자기 진화 규칙
태스크 완료 후 반드시 다음을 수행:
1. 변경된 도메인 지식 문서와 관련 스킬 동기화
2. 교차 문서 일관성 검증 (컨벤션 ↔ 아키텍처 ↔ 용어집)
3. 학습 사항을 Learnings 섹션에 기록

## Learnings
_실행 결과에서 발견된 패턴을 여기에 기록합니다._
```

---

### 패턴 2: 완전한 에이전트 프론트매터 참조 구현

**출처**: `weather-agent.md`

**원본 구조** (모든 필드를 사용하는 완전한 예시):
```yaml
---
name: weather-agent
description: Use this agent PROACTIVELY when...
tools: WebFetch, Read, Write, Edit
model: sonnet
color: green
maxTurns: 5
permissionMode: acceptEdits
memory: project
skills:
  - weather-fetcher
hooks:
  PreToolUse:
    - matcher: ".*"
      hooks:
        - type: command
          command: python3 ${CLAUDE_PROJECT_DIR}/.claude/hooks/scripts/hooks.py
          timeout: 5000
          async: true
---
```

**Pylon 적용**: `pylon add-agent` 명령어가 생성하는 에이전트 파일 템플릿의 기본 구조.

**`pylon init` 시 생성할 에이전트 템플릿**:
```yaml
---
name: {agent-name}
role: {agent-role}
backend: claude-code
scope:
  - {project-name}
tools:
  - git
  - gh
maxTurns: 30
permissionMode: acceptEdits
isolation: worktree
model: sonnet
env:
  CLAUDE_CODE_EFFORT_LEVEL: high
skills:
  - {skill-name}
---

# {Agent Name}

## 역할
{역할 설명}

## 워크플로우
1. 태스크 수신 → inbox 확인
2. 컨텍스트 로드 → 프로젝트 context.md + 관련 스킬
3. 구현 → scope 내 파일만 수정
4. 검증 → verify.yml 실행
5. 결과 보고 → outbox에 result.json 생성
```

---

### 패턴 3: Command → Agent → Skill 오케스트레이션

**출처**: `weather-orchestrator.md` 커맨드

**원본 구조**:
```markdown
## Workflow

### Step 1: Ask User Preference
Use the AskUserQuestion tool...

### Step 2: Fetch Weather Data
Use the Task tool to invoke the weather agent:
- subagent_type: weather-agent
- prompt: Fetch the current temperature...
Wait for the agent to complete and capture the returned value.

### Step 3: Create SVG Weather Card
Use the Skill tool to invoke the weather-svg-creator skill.

## Critical Requirements
1. Use Task Tool for Agent: DO NOT use bash commands to invoke agents
2. Use Skill Tool for SVG Creator
3. Sequential Flow: Complete each step before moving to the next
```

**Pylon 적용**: 오케스트레이터가 태스크를 에이전트에 전달하는 흐름의 레퍼런스. Pylon에서는 Command = 오케스트레이터 태스크 할당, Agent = tmux 세션 에이전트, Skill = claude.md에 주입되는 도메인 지식.

**핵심 교훈**:
- 오케스트레이터는 에이전트를 직접 실행하지 않고 Task 인터페이스를 통해 호출
- Skill은 에이전트에 preload되어 컨텍스트로 제공
- 각 단계 완료를 확인한 후 다음 단계 진행 (Sequential Flow)

---

### 패턴 4: Drift Analysis 워크플로우 (외부 → 내부 비교)

**출처**: `workflow-claude-settings.md`, `workflow-claude-subagents.md`, `workflow-concepts.md`

**원본 구조**:
```
Phase 0: 두 에이전트를 병렬 실행 (research agent + guide agent)
Phase 0.5: 검증 체크리스트
Phase 1: 이전 로그 확인
Phase 2: 분석 및 리포트 병합
Phase 2.5: 변경로그 추가
Phase 2.6: 배지 업데이트
Phase 2.7: 하이퍼링크 검증
Phase 3: 실행 제안 (사용자 승인 후)
```

**Pylon 적용**: QA 교차 검증 워크플로우 및 Tech Writer의 도메인 지식 최신성 검증에 적용.

**Pylon 도메인 지식 검증 흐름**:
```
1. 병렬 에이전트 실행:
   - Agent A: 코드베이스에서 실제 패턴 추출
   - Agent B: 도메인 지식 문서 읽기
2. 비교 분석: 코드 실상 vs 문서 기록
3. 드리프트 리포트 생성
4. Tech Writer에게 갱신 태스크 할당
```

---

### 패턴 5: Skill 아키텍처 (Progressive Disclosure)

**출처**: `agent-browser/SKILL.md`, `weather-fetcher/SKILL.md`

**두 가지 유형의 스킬**:

| 유형 | 예시 | `user-invocable` | 용도 |
|------|------|-------------------|------|
| 포괄적 도구 스킬 | agent-browser | (기본값=true) | CLI 도구의 전체 사용법 |
| 에이전트 전용 스킬 | weather-fetcher | `false` | 에이전트에만 preload |

**agent-browser 패턴** (포괄적 스킬):
```markdown
---
name: agent-browser
description: Browser automation CLI for AI agents...
allowed-tools: Bash(agent-browser:*)
---

## Core Workflow (필수 지식)
## Essential Commands (빠른 참조)
## Common Patterns (활용 예시)
## Deep-Dive Documentation (상세 참조 링크)
| Reference | When to Use |
|-----------|-------------|
| references/commands.md | Full command reference |
| references/snapshot-refs.md | Ref lifecycle, troubleshooting |
```

**weather-fetcher 패턴** (에이전트 전용 스킬):
```markdown
---
name: weather-fetcher
description: Instructions for fetching current weather...
user-invocable: false
---

## Task (단일 작업)
## Instructions (구체적 단계)
## Expected Output (출력 형식)
## Notes (주의사항)
```

**Pylon 적용**: `.pylon/skills/` 디렉토리 설계 시 두 가지 유형 지원.

**`pylon init` 시 생성할 스킬 템플릿**:
```markdown
# .pylon/skills/{skill-name}.md

---
name: {skill-name}
description: {스킬 설명}
# user-invocable: false  ← 에이전트 전용일 때만
---

## 목적
{이 스킬이 제공하는 도메인 지식}

## 핵심 규칙
{반드시 따라야 할 규칙들}

## 참조 경로
{상세 문서가 있는 경로}
```

---

### 패턴 6: Hooks 시스템 (19개 라이프사이클 이벤트)

**출처**: `HOOKS-README.md`, `hooks-config.json`, `hooks.py`

**Pylon에 적용 가능한 핵심 훅 이벤트**:

| 훅 | Pylon 매핑 | 용도 |
|----|-----------|------|
| `SubagentStart` | 에이전트 시작 | 에이전트 상태 추적 시작 |
| `SubagentStop` | 에이전트 종료 | 결과 수집, 다음 태스크 할당 |
| `Stop` | 응답 완료 | 태스크 완료 감지 |
| `PreToolUse` | 도구 사용 전 | 위험한 명령 차단 (rm -rf 등) |
| `PostToolUse` | 도구 사용 후 | 변경 사항 로깅 |
| `TeammateIdle` | 팀원 대기 | 다음 태스크 자동 할당 |
| `TaskCompleted` | 태스크 완료 | 파이프라인 진행 |
| `SessionStart/End` | 세션 관리 | 상태 저장/복원 |
| `WorktreeCreate/Remove` | Worktree 관리 | 병렬 작업 격리 |

**hooks-config.json 패턴** (개별 훅 on/off):
```json
{
  "disableSubagentStartHook": false,
  "disableSubagentStopHook": false,
  "disableStopHook": false,
  "disableTeammateIdleHook": false,
  "disableTaskCompletedHook": false,
  "disableLogging": true
}
```

**Pylon 적용**: 오케스트레이터의 에이전트 라이프사이클 관리에 훅 이벤트 개념 도입. 특히 `SubagentStop` → 결과 수집 → `TeammateIdle` → 다음 태스크 할당 흐름.

---

### 패턴 7: 에이전트 전용 훅 (Frontmatter Hooks)

**출처**: `weather-agent.md`의 hooks 필드, `HOOKS-README.md`

**원본 구조** (에이전트 프론트매터에 직접 훅 정의):
```yaml
hooks:
  PreToolUse:
    - matcher: ".*"
      hooks:
        - type: command
          command: python3 ${CLAUDE_PROJECT_DIR}/.claude/hooks/scripts/hooks.py --agent=voice-hook-agent
          timeout: 5000
          async: true
  PostToolUse:
    - matcher: ".*"
      hooks:
        - type: command
          command: ...
```

**에이전트 프론트매터에서 지원하는 훅 (6개)**:
- `PreToolUse`, `PostToolUse`, `PermissionRequest`, `PostToolUseFailure`, `Stop`, `SubagentStop`

**Pylon 적용**: 에이전트별 커스텀 훅을 frontmatter에 정의하여 에이전트 레벨 라이프사이클 제어 가능. 예: QA 에이전트의 `PostToolUse`에서 테스트 결과 자동 수집.

---

## 2. 적용 불필요 항목 (도메인 특화)

다음은 수집했으나 Pylon 초기화에 불필요한 항목들:

| 항목 | 불필요 사유 |
|------|-----------|
| `presentation-curator` 에이전트 전체 | 프레젠테이션 도메인 특화 |
| `presentation-structure` 스킬 | HTML 슬라이드 구조 지식 |
| `presentation-styling` 스킬 | CSS 클래스 패턴 지식 |
| `vibe-to-agentic-framework` 스킬 | 교육 프레임워크 서사 |
| `weather-*` 구현체 전체 | 데모용 날씨 도메인 |
| `workflow-*` 구현체 전체 | 문서 드리프트 분석 도메인 |
| `hooks.py` 사운드 재생 | 사운드 알림은 Pylon 범위 밖 |

→ **패턴만 추출하고 도메인 구현은 버림**

---

## 3. `pylon init` 시 생성할 초기 파일 목록

수집된 패턴을 기반으로, `pylon init`이 생성해야 할 파일:

```
.pylon/
├── config.yml                    ← 도구 설정 (패턴 6: hooks 설정 포함)
├── domain/
│   ├── conventions.md            ← 빈 템플릿
│   ├── architecture.md           ← 빈 템플릿
│   └── glossary.md               ← 빈 템플릿
├── agents/
│   ├── po.md                     ← 패턴 2 기반 프론트매터 + 패턴 7 에이전트 훅
│   ├── pm.md                     ← 패턴 3 오케스트레이션 규칙 포함
│   ├── architect.md              ← 패턴 5 스킬 preload 구조
│   └── tech-writer.md            ← 패턴 1 자기진화 + 패턴 4 드리프트 분석
├── skills/                       ← 패턴 5 구조
│   └── .gitkeep
├── runtime/                      ← .gitignore 대상
│   ├── inbox/
│   ├── outbox/
│   └── memory/
└── conversations/                ← .gitignore 대상
```

---

## 4. 스펙 반영 시 추가/수정해야 할 섹션

| 스펙 섹션 | 반영할 패턴 | 변경 유형 |
|-----------|------------|----------|
| Section 5 (에이전트 설정 포맷) | 패턴 7: `hooks` 프론트매터 필드 추가 | 수정 |
| Section 8 (에이전트 실행) | 패턴 3: 오케스트레이터 → Agent → Skill 흐름 명세화 | 추가 |
| Section 8 (CLAUDE.md 주입) | 패턴 5: skills preload 메커니즘 상세화 | 추가 |
| Section 10 (Tech Writer) | 패턴 1: 자기진화 규칙 + 패턴 4: 드리프트 분석 | 추가 |
| Section 16 (config.yml) | 패턴 6: hooks 설정 스키마 | 추가 |
| Section 17 (미결사항) | Hooks 시스템 설계 → 해결 | 업데이트 |

---

## 5. 요약

| # | 패턴 | 핵심 가치 | Pylon 적용 대상 |
|---|------|----------|----------------|
| 1 | Self-Evolution | 지식 드리프트 방지 | Tech Writer |
| 2 | 완전한 프론트매터 | 에이전트 정의 표준 | 모든 에이전트 |
| 3 | Command → Agent → Skill | 오케스트레이션 흐름 | 오케스트레이터 |
| 4 | Drift Analysis | 외부↔내부 비교 | QA, Tech Writer |
| 5 | Skill 아키텍처 | Progressive Disclosure | skills/ 디렉토리 |
| 6 | Hooks 시스템 (19개) | 라이프사이클 관리 | 오케스트레이터 |
| 7 | Agent Frontmatter Hooks | 에이전트별 훅 | 에이전트 설정 |
