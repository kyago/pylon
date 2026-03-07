# claude-code-best-practice 리포지토리 조사 보고서

> 조사 대상: https://github.com/shanraisshan/claude-code-best-practice
> 조사 일자: 2026-03-06
> 목적: Pylon 프로젝트(AI 멀티 에이전트 개발팀 오케스트레이션 도구, Go)에 적용 가능한 패턴과 베스트 프랙티스 수집

---

## 1. 리포지토리 개요

**저자**: shanraisshan (Shayan)
**목적**: Claude Code 설정, 패턴, 워크플로우에 대한 종합 레퍼런스 구현 리포지토리
**갱신 상태**: 2026년 3월 6일 기준 활발하게 업데이트 중 (최신)

### 리포지토리 구조

```
claude-code-best-practice/
├── CLAUDE.md                          # 프로젝트 CLAUDE.md (200줄 이하 권장 준수)
├── .mcp.json                          # MCP 서버 설정 (5개 서버)
├── .claude/
│   ├── settings.json                  # 팀 공유 설정 (55+ 설정, 19개 훅 이벤트)
│   ├── agents/                        # 커스텀 서브에이전트
│   ├── commands/                      # 커스텀 슬래시 커맨드
│   ├── skills/                        # 커스텀 스킬
│   ├── hooks/                         # 라이프사이클 훅 (사운드 알림)
│   └── agent-memory/                  # 에이전트 지속 메모리
├── best-practice/                     # 7개 베스트 프랙티스 문서
│   ├── claude-memory.md               # CLAUDE.md 작성 + 모노레포 로딩 가이드
│   ├── claude-subagents.md            # 서브에이전트 완전 레퍼런스
│   ├── claude-commands.md             # 커맨드 완전 레퍼런스
│   ├── claude-skills.md               # 스킬 완전 레퍼런스
│   ├── claude-settings.md             # 설정 완전 레퍼런스 (55+ 설정, 110+ 환경변수)
│   ├── claude-mcp.md                  # MCP 서버 베스트 프랙티스
│   └── claude-cli-startup-flags.md    # CLI 시작 플래그 레퍼런스
├── orchestration-workflow/            # Command → Agent → Skill 패턴 시연
├── development-workflows/             # 개발 워크플로우 (Cross-Model, RPI)
├── reports/                           # 8개 심층 분석 보고서
├── tips/                              # Boris Cherny(Claude Code 창시자) 팁
└── implementation/                    # 구현 예시 문서
```

---

## 2. 핵심 개념 및 아키텍처 패턴

### 2.1 Command → Agent → Skill 오케스트레이션 패턴

이 리포의 가장 중요한 아키텍처 패턴. Pylon의 에이전트 오케스트레이션에 직접 참고할 수 있다.

```
User → Command (진입점, 사용자 인터랙션)
         → Agent (데이터 수집, preloaded skill 사용)
         → Skill (독립 실행, 결과물 생성)
```

**핵심 원칙**:
- **Command**: 워크플로우를 조율하고 사용자 인터랙션을 처리하는 오케스트레이터
- **Agent**: 사전 로드된 skill의 지식을 활용하여 데이터를 수집/처리
- **Skill**: 독립적으로 실행되어 최종 결과물을 생성

**두 가지 Skill 패턴**:

| 패턴 | 로딩 | 호출 | 용도 |
|------|------|------|------|
| **Agent Skill** (Preloaded) | 에이전트 시작 시 `skills:` 필드로 주입 | 자동 (context에 주입) | 도메인 지식, 절차 |
| **Skill** (Independent) | 온디맨드 | `/skill-name` 또는 `Skill(skill: "name")` | 독립 워크플로우 |

### 2.2 서브에이전트 정의 구조

서브에이전트의 YAML frontmatter 전체 필드:

```yaml
---
name: deploy-manager                    # 고유 식별자
description: Use PROACTIVELY for...     # 자동 호출 시 "PROACTIVELY" 사용
tools: Read, Write, Edit, Bash          # 도구 허용 목록 (생략 시 전체 상속)
disallowedTools: NotebookEdit           # 차단 도구
model: sonnet                           # haiku, sonnet, opus, inherit
permissionMode: acceptEdits             # default, acceptEdits, dontAsk, bypassPermissions, plan
maxTurns: 25                            # 최대 에이전틱 턴 수
skills:                                 # 사전 로드할 스킬
  - deploy-checklist
mcpServers:                             # 이 에이전트 전용 MCP 서버
  - slack
memory: project                         # user, project, local
background: false                       # 백그라운드 태스크 실행
isolation: worktree                     # git worktree 격리 실행
color: blue                             # CLI 출력 색상
hooks:                                  # 라이프사이클 훅
  PreToolUse: [...]
  PostToolUse: [...]
  Stop: [...]
---
```

**중요**: 서브에이전트는 bash 명령으로 다른 서브에이전트를 호출할 수 없다. 반드시 Task 도구를 사용해야 한다:
```
Task(subagent_type="agent-name", description="...", prompt="...", model="haiku")
```

### 2.3 에이전트 메모리 시스템

v2.1.33에서 도입된 persistent memory 시스템:

| 스코프 | 저장 위치 | 공유 | 버전 관리 | 용도 |
|--------|----------|------|----------|------|
| `user` | `~/.claude/agent-memory/<name>/` | No | No | 크로스 프로젝트 지식 (권장 기본) |
| `project` | `.claude/agent-memory/<name>/` | Yes | Yes | 팀 공유 프로젝트 지식 |
| `local` | `.claude/agent-memory-local/<name>/` | No | No | 개인 프로젝트 지식 |

**동작 방식**:
1. 시작 시 `MEMORY.md`의 첫 200줄이 시스템 프롬프트에 주입
2. `Read`, `Write`, `Edit` 도구가 자동 활성화
3. 실행 중 에이전트가 자유롭게 메모리 디렉토리를 읽고/쓸 수 있음
4. `MEMORY.md`가 200줄을 초과하면 주제별 파일로 분리

---

## 3. CLAUDE.md 작성 가이드라인

### 3.1 핵심 규칙

- **파일당 200줄 이하** 유지 (준수율 향상을 위해 필수)
- `MUST`를 대문자로 써도 100% 준수가 보장되지 않음 (알려진 한계)
- 모노레포에서는 **여러 CLAUDE.md를 계층적으로** 배치

### 3.2 로딩 메커니즘

| 메커니즘 | 방향 | 시점 |
|---------|------|------|
| **Ancestor Loading** | 현재 디렉토리에서 루트까지 위로 | 시작 시 즉시 |
| **Descendant Loading** | 하위 디렉토리 아래로 | 해당 디렉토리 파일 접근 시 (lazy) |
| **Sibling Loading** | 형제 디렉토리 | 로드되지 않음 |

### 3.3 모노레포 전략

```
/workspace/
├── CLAUDE.md          # 공유 컨벤션 (코딩 표준, 커밋 규칙)
├── frontend/
│   └── CLAUDE.md      # 프론트엔드 전용 (프레임워크 패턴)
├── backend/
│   └── CLAUDE.md      # 백엔드 전용 (API 설계 원칙)
└── api/
    └── CLAUDE.md      # API 전용 (엔드포인트 규칙)
```

### 3.4 포함해야 할 내용 (이 리포의 CLAUDE.md 분석)

1. **리포지토리 개요**: 프로젝트의 성격과 목적 (3-4줄)
2. **핵심 컴포넌트 설명**: 주요 시스템과 그 관계 (상세하지만 간결)
3. **스킬/에이전트 정의 구조**: frontmatter 필드 레퍼런스
4. **핵심 패턴과 제약**: 절대적 규칙 (예: Task 도구만 사용)
5. **설정 계층**: 파일 우선순위
6. **워크플로우 베스트 프랙티스**: 실전 경험 기반 팁
7. **디버깅 팁**: 문제 해결 지침
8. **문서/보고서 위치**: 관련 문서 경로

---

## 4. MCP 서버 설정 베스트 프랙티스

### 4.1 실용적 MCP 서버 선택

> "15개 MCP를 설치했는데 실제 매일 쓰는 건 4개뿐" - r/mcp (682 upvotes)

**핵심 5개**:
| 서버 | 용도 |
|------|------|
| **Context7** | 최신 라이브러리 문서 검색 (API 할루시네이션 방지) |
| **Playwright** | 브라우저 자동화/테스트 |
| **Claude in Chrome** | 실제 Chrome 브라우저 연결 (콘솔, 네트워크, DOM 디버깅) |
| **DeepWiki** | GitHub 리포 구조적 문서 |
| **Excalidraw** | 아키텍처 다이어그램 생성 |

### 4.2 MCP 설정 구조

```json
{
  "mcpServers": {
    "context7": {
      "command": "npx",
      "args": ["-y", "@upstash/context7-mcp"]
    },
    "playwright": {
      "command": "npx",
      "args": ["-y", "@playwright/mcp"]
    },
    "remote-api": {
      "type": "http",
      "url": "https://mcp.example.com/mcp"
    }
  }
}
```

### 4.3 MCP 스코프 우선순위

| 스코프 | 위치 | 우선순위 |
|--------|------|---------|
| Subagent | 에이전트 frontmatter `mcpServers` 필드 | 최고 |
| Project | `.mcp.json` (리포 루트) | 중간 |
| User | `~/.claude.json` (`mcpServers` 키) | 최저 |

### 4.4 MCP 도구 권한 설정

```json
{
  "permissions": {
    "allow": ["mcp__*", "mcp__context7__*"],
    "deny": ["mcp__dangerous-server__*"]
  }
}
```

---

## 5. 설정(Settings) 심층 레퍼런스

### 5.1 설정 계층 (5단계 + 정책)

| 우선순위 | 위치 | 스코프 | 버전 관리 |
|---------|------|-------|----------|
| 1 | 커맨드라인 인자 | 세션 | N/A |
| 2 | `.claude/settings.local.json` | 프로젝트 | No (git-ignored) |
| 3 | `.claude/settings.json` | 프로젝트 | Yes (committed) |
| 4 | `~/.claude/settings.local.json` | 사용자 | N/A |
| 5 | `~/.claude/settings.json` | 사용자 | N/A |

**정책 계층**: `managed-settings.json`은 조직 강제이며 로컬 설정으로 오버라이드 불가

### 5.2 핵심 설정 항목

```json
{
  "model": "opus",
  "language": "korean",
  "alwaysThinkingEnabled": true,
  "outputStyle": "Explanatory",
  "plansDirectory": "./plans",
  "permissions": {
    "allow": ["Edit(*)", "Write(*)", "Bash(npm run *)", "mcp__*"],
    "deny": ["Read(.env)", "Read(./secrets/**)"],
    "defaultMode": "acceptEdits"
  },
  "sandbox": {
    "enabled": true,
    "excludedCommands": ["git", "docker"]
  },
  "env": {
    "CLAUDE_AUTOCOMPACT_PCT_OVERRIDE": "80",
    "CLAUDE_CODE_EFFORT_LEVEL": "high"
  }
}
```

### 5.3 주요 환경 변수

| 변수 | 설명 |
|------|------|
| `CLAUDE_AUTOCOMPACT_PCT_OVERRIDE` | 자동 컴팩트 임계값 (기본 ~95%, 낮출수록 빨리 컴팩트) |
| `CLAUDE_CODE_EFFORT_LEVEL` | 사고 깊이: `low`, `medium`, `high` |
| `MAX_THINKING_TOKENS` | 최대 사고 토큰 수 |
| `ENABLE_TOOL_SEARCH` | MCP 도구 검색 임계값 (e.g., `auto:5`) |
| `CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS` | 에이전트 팀 실험 기능 활성화 |
| `CLAUDE_CODE_MAX_TURNS` | 최대 에이전틱 턴 수 |

---

## 6. 개발 워크플로우

### 6.1 Cross-Model 워크플로우 (Claude Code + Codex)

```
Step 1: PLAN (Claude Code Opus, Plan Mode)
  → Claude가 인터뷰 → 단계별 계획 생성 → plans/{feature}.md

Step 2: QA REVIEW (Codex CLI GPT-5.4)
  → 코드베이스 대조 → 누락 단계 삽입 → 계획 보강

Step 3: IMPLEMENT (Claude Code Opus)
  → 단계별 구현 + 테스트 게이트

Step 4: VERIFY (Codex CLI)
  → 계획 대비 구현 검증
```

### 6.2 RPI 워크플로우 (Research → Plan → Implement)

```
Step 1: Describe → 기능 설명 → REQUEST.md 생성
Step 2: Research → GO/NO-GO 분석 → RESEARCH.md
Step 3: Plan → PM/UX/ENG 문서 → PLAN.md
Step 4: Implement → 단계별 구현 + 테스트 → IMPLEMENT.md
```

**폴더 구조**:
```
rpi/{feature-slug}/
├── REQUEST.md          # 초기 기능 설명
├── research/
│   └── RESEARCH.md     # GO/NO-GO 분석
├── plan/
│   ├── PLAN.md         # 구현 로드맵
│   ├── pm.md           # 제품 요구사항
│   ├── ux.md           # UX 설계
│   └── eng.md          # 기술 명세
└── implement/
    └── IMPLEMENT.md    # 구현 기록
```

### 6.3 Ralph Wiggum Loop

자율 개발 루프: 장기 실행 태스크를 위해 완료될 때까지 반복하는 플러그인.

---

## 7. 실전 팁 (Shayan & Boris Cherny 종합)

### Planning (계획)

1. 항상 **plan mode**로 시작. Claude에게 인터뷰를 요청
2. **단계별 게이트 계획** 수립. 각 단계마다 테스트(유닛/자동화/통합)
3. Cross-model로 계획 검증

### Workflows (워크플로우)

4. CLAUDE.md는 **파일당 200줄 이하**
5. 모노레포는 **여러 CLAUDE.md** 사용 (ancestor + descendant 로딩)
6. **`.claude/rules/`** 로 큰 지시사항 분할
7. 워크플로우에는 standalone agent 대신 **commands** 사용
8. 범용 에이전트(qa, backend) 대신 **기능별 서브에이전트 + skills** (progressive disclosure)
9. **50% 컨텍스트**에서 수동 `/compact` 실행
10. 작은 태스크는 **vanilla Claude Code**가 어떤 워크플로우보다 낫다
11. 항상 **thinking mode true** + **Output Style Explanatory** 사용
12. 프롬프트에 **ultrathink** 키워드 사용 (고노력 추론)
13. 중요 세션은 `/rename` 후 나중에 `/resume`
14. Claude가 이탈하면 고치려 하지 말고 **`Esc Esc` 또는 `/rewind`**
15. **최소 1시간에 1회 커밋**

### Debugging (디버깅)

16. 문제 발생 시 **스크린샷을 찍어서 Claude에 공유**
17. MCP로 **Chrome 콘솔 로그를 Claude가 직접 확인**하게 하기
18. 긴 터미널 명령은 **백그라운드 태스크**로 실행
19. `/doctor`로 설치/인증/설정 진단

### Advanced (고급)

20. 아키텍처 이해에 **ASCII 다이어그램** 적극 활용
21. **tmux + git worktrees**로 병렬 개발
22. **Ralph Wiggum 플러그인**으로 장기 자율 태스크
23. `dangerously-skip-permissions` 대신 **와일드카드 권한** 사용
24. `/sandbox`로 파일/네트워크 격리하며 권한 프롬프트 줄이기

### Customization (커스터마이징 - Boris 12가지)

25. 터미널 설정 최적화 (iTerm2/Ghostty, `/terminal-setup`)
26. 노력 수준(Effort Level) 조정 (`/model` → High 권장)
27. 플러그인/MCP/스킬 설치 (`/plugin`)
28. 커스텀 에이전트 생성 (`/agents`)
29. 공통 권한 사전 승인 (`/permissions`)
30. 샌드박싱 활성화 (`/sandbox`)
31. 상태 라인 추가 (`/statusline`)
32. 키바인딩 커스터마이징 (`/keybindings`)
33. 훅 설정 (라이프사이클 자동화)
34. 스피너 동사 커스터마이징
35. 출력 스타일 설정 (Explanatory, Learning, Custom)
36. `settings.json`을 git에 체크인하여 팀 공유

---

## 8. Advanced Tool Use 패턴

### 8.1 Programmatic Tool Calling (PTC)

기존: 각 도구 호출마다 모델 라운드트립 → 토큰 소비
PTC: Claude가 Python 스크립트를 작성하여 여러 도구를 한번에 호출 → stdout만 컨텍스트에 진입

**토큰 절감**: ~37% 감소 (10개 도구 = 기존 대비 1/10 토큰)

**적용 대상**: API/Foundry (CLI에서는 미지원, Agent SDK 사용자에게 유용)

### 8.2 Tool Search Tool

MCP 도구 정의가 많아 컨텍스트를 차지할 때, `defer_loading: true`로 표시하여 온디맨드 검색.

**토큰 절감**: ~85% 감소 (77K → 8.7K)

**Claude Code에서**: `MCPSearch` 자동 모드 (v2.1.7 이후 기본 활성)

### 8.3 Tool Use Examples

도구 정의에 `input_examples` 추가 → 정확도 72% → 90% 향상

### 8.4 Dynamic Filtering (웹 검색/추출)

Claude가 Python 코드를 작성하여 웹 결과를 필터링한 후 컨텍스트에 주입.

**토큰 절감**: ~24% 감소

---

## 9. Agent SDK vs CLI 시스템 프롬프트

### 핵심 차이점

| 측면 | Claude CLI | Agent SDK (기본) | Agent SDK (Preset) |
|------|-----------|-----------------|-------------------|
| 시스템 프롬프트 | 모듈식 (~269+ 기본 토큰) | 최소 | 모듈식 (CLI 매칭) |
| 도구 포함 | 18+ 내장 | 제공한 것만 | 18+ 내장 |
| CLAUDE.md 자동 로드 | Yes | No | No (설정 필요) |
| 결정론적 출력 | No | No | No |

**중요**: 동일 입력 + `temperature=0`이라도 동일 출력을 보장하지 않는다 (seed 파라미터 부재, MoE 라우팅 변동).

---

## 10. Task 시스템 (TodoWrite 대체)

v2.1.16에서 도입. `~/.claude/tasks/`에 파일시스템 기반 저장.

| 특성 | Old Todos | New Tasks |
|------|-----------|-----------|
| 스코프 | 단일 세션 | 크로스 세션, 크로스 에이전트 |
| 의존성 | 없음 | 전체 의존성 그래프 |
| 저장 | 인메모리 | 파일시스템 |
| 지속성 | 세션 종료 시 유실 | 재시작/크래시에도 생존 |
| 멀티 세션 | 불가 | `CLAUDE_CODE_TASK_LIST_ID`로 가능 |

---

## 11. Agent Teams (실험 기능)

2026년 2월 도입. 여러 Claude Code 세션이 협업.

```json
// ~/.claude/settings.json
{
  "env": {
    "CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS": "1"
  }
}
```

**모드**:
| 모드 | 설명 | 요구사항 |
|------|------|---------|
| In-process (기본) | 모든 팀원이 하나의 터미널에서 실행 | 없음 |
| Split panes | 각 팀원이 별도 패널 | tmux 또는 iTerm2 |

---

## 12. Pylon 프로젝트에 직접 적용 가능한 항목

### 12.1 에이전트 정의 포맷 → Pylon 에이전트 설정

Pylon의 `.pylon/agents/*.md` 포맷이 Claude Code의 `.claude/agents/*.md` 포맷과 유사한 구조를 채택하고 있다. 다음 필드를 참고하여 확장:

| Claude Code 필드 | Pylon 적용 | 상세 |
|-----------------|----------|------|
| `name` | 이미 사용 중 | 고유 식별자 |
| `description` | 확장 가능 | `"PROACTIVELY"` 키워드로 자동 호출 트리거 |
| `tools` | 적용 가능 | 도구 허용 목록으로 에이전트 능력 제한 |
| `maxTurns` | **권장 적용** | 에이전트 턴 수 제한으로 무한 루프 방지 |
| `skills` | **권장 적용** | 도메인 지식을 스킬로 분리하여 progressive disclosure |
| `memory` | **핵심 적용** | 에이전트별 지속 메모리 (user/project/local) |
| `isolation: worktree` | **핵심 적용** | git worktree 격리 (Pylon의 tmux 격리와 유사) |
| `color` | 적용 가능 | TUI/대시보드에서 에이전트 시각 구분 |
| `hooks` | **권장 적용** | 에이전트 라이프사이클 이벤트 처리 |

### 12.2 Command → Agent → Skill 패턴 → Pylon 오케스트레이션

Pylon의 오케스트레이터가 Task 분배하는 패턴과 직접 대응:

```
[Pylon]                              [Claude Code Best Practice]
사용자 요청                          → Command (진입점)
오케스트레이터 → 루트 에이전트       → Agent (데이터 수집/처리)
                → 프로젝트 에이전트  → Skill (독립 실행)
```

**적용 포인트**:
- 오케스트레이터가 Task 도구 패턴으로 에이전트를 호출하는 방식
- Skill을 에이전트에 preload하는 패턴 (도메인 지식 주입)
- Command가 사용자 인터랙션을 담당하는 분리 원칙

### 12.3 도메인 지식 → CLAUDE.md + Skills 조합

Pylon의 `.pylon/domain/` 디렉토리의 도메인 지식을:
- **CLAUDE.md**: 프로젝트 전반에 항상 필요한 지식 (컨벤션, 아키텍처 원칙)
- **Skills**: 특정 에이전트에만 필요한 전문 지식 (API 설계 가이드, 테스트 규칙)

로 분리하는 전략이 효과적.

### 12.4 에이전트 메모리 시스템 → Pylon의 memory/ 디렉토리

Pylon의 `.pylon/runtime/memory/` 구현에 참고:
- `MEMORY.md` 첫 200줄이 시스템 프롬프트에 자동 주입되는 패턴
- 200줄 초과 시 주제별 파일로 자동 분리하는 큐레이션 메커니즘
- 에이전트 자신이 메모리를 읽고/쓰는 자율적 학습 패턴

### 12.5 설정 계층 → Pylon의 config.yml 확장

현재 단일 `config.yml` 구조를 다음과 같이 계층화 검토:

```
.pylon/
├── config.yml                  # 팀 공유 (git 커밋)
├── config.local.yml            # 개인 설정 (.gitignore)
└── config.managed.yml          # 조직 정책 (오버라이드 불가)
```

### 12.6 Hooks 시스템 → Pylon 이벤트 처리

Claude Code의 19개 훅 이벤트 중 Pylon에 적용 가능한 것:

| Claude Code 훅 | Pylon 대응 | 용도 |
|----------------|----------|------|
| `PreToolUse` | 도구 실행 전 검증 | 위험 명령 차단/로깅 |
| `PostToolUse` | 도구 실행 후 처리 | 결과 로깅, 알림 |
| `SubagentStart/Stop` | 에이전트 시작/종료 | 라이프사이클 관리 |
| `TaskCompleted` | 태스크 완료 | 다음 단계 트리거 |
| `TeammateIdle` | 팀원 대기 | 작업 재분배 |
| `Stop` | 에이전트 정지 | 정리 작업, 결과 보고 |

### 12.7 컨텍스트 관리 전략

- **50% 컨텍스트에서 `/compact`**: Pylon의 에이전트 세션 관리에서도 컨텍스트 사용량 모니터링 + 자동 컴팩트 구현
- **CLAUDE_AUTOCOMPACT_PCT_OVERRIDE=80**: 에이전트별 컴팩트 임계값 설정 지원
- **`/clear`로 태스크 전환**: 태스크 간 컨텍스트 오염 방지

### 12.8 RPI 워크플로우 → Pylon 파이프라인

Pylon의 `대화 → 작업지시서 → 구현` 파이프라인과 RPI 워크플로우가 유사:

```
[Pylon]                    [RPI]
대화 (PO 인터뷰)           → Research (GO/NO-GO)
작업지시서 생성             → Plan (PM/UX/ENG)
에이전트 구현               → Implement (단계별 + 테스트)
```

### 12.9 Git Worktree 격리 → Pylon의 tmux 세션 격리

Claude Code의 `isolation: worktree`는 Pylon이 이미 tmux 세션으로 에이전트를 격리하는 것과 동일한 문제를 해결한다. 추가로:
- 각 에이전트가 독립된 git worktree에서 작업하여 충돌 방지
- 작업 완료 후 자동 정리 (변경사항이 없으면 worktree 삭제)

### 12.10 Task 시스템 → Pylon 태스크 관리

Claude Code의 새 Task 시스템 (파일시스템 기반, 의존성 그래프)은 Pylon의 `tasks/` 디렉토리와 유사. 참고할 패턴:
- `addBlockedBy`/`addBlocks`로 의존성 그래프 관리
- `CLAUDE_CODE_TASK_LIST_ID`로 멀티 세션 공유
- 파일시스템 기반 저장으로 크래시 복구 보장

---

## 13. 참고 리소스

### 공식 문서
- [Claude Code Docs](https://code.claude.com/docs/en/)
- [Claude Code CHANGELOG](https://github.com/anthropics/claude-code/blob/main/CHANGELOG.md)
- [Anthropic Prompt Engineering Tutorial](https://github.com/anthropics/prompt-eng-interactive-tutorial)

### 커뮤니티
- [r/ClaudeAI](https://www.reddit.com/r/ClaudeAI/)
- [r/ClaudeCode](https://www.reddit.com/r/ClaudeCode/)

### 핵심 인물 (X/Twitter)
- [Boris Cherny](https://x.com/bcherny) - Claude Code 창시자
- [Thariq](https://x.com/trq212) - Claude Code 팀
- [Cat Wu](https://x.com/_catwu) - Claude Code 팀
- [Lydia Hallie](https://x.com/lydiahallie) - Claude Code 팀

### 관련 워크플로우 리포
- [obra/superpowers](https://github.com/obra/superpowers) (72k stars)
- [Github Speckit](https://github.com/github/spec-kit) (74k stars)
- [OpenSpec OPSX](https://github.com/Fission-AI/OpenSpec/blob/main/docs/opsx.md) (28k stars)
- [get-shit-done (GSD)](https://github.com/gsd-build/get-shit-done) (25k stars)
- [HumanLayer RPI](https://github.com/humanlayer/advanced-context-engineering-for-coding-agents) (1.5k stars)

---

## 14. 결론 및 권장 사항

### 즉시 적용 (High Priority)

1. **에이전트 frontmatter 확장**: `maxTurns`, `skills`, `memory`, `hooks` 필드를 Pylon 에이전트 설정에 반영
2. **도메인 지식 분리 전략**: CLAUDE.md(공통) + Skills(전문) 이원화
3. **에이전트 메모리 시스템**: `MEMORY.md` 자동 주입 + 자율 학습 패턴 구현
4. **컨텍스트 관리**: 50% 임계값 자동 컴팩트, 태스크 전환 시 컨텍스트 초기화

### 중기 적용 (Medium Priority)

5. **설정 계층화**: config.yml → config.local.yml → config.managed.yml
6. **훅 시스템**: 에이전트 라이프사이클 이벤트 처리 프레임워크
7. **Task 의존성 그래프**: `addBlockedBy`/`addBlocks` 패턴으로 태스크 간 의존성 관리
8. **RPI 워크플로우 통합**: Research → Plan → Implement 파이프라인

### 장기 참고 (Low Priority)

9. **Agent Teams**: 멀티 에이전트 tmux 협업 패턴
10. **PTC (Programmatic Tool Calling)**: 토큰 효율 최적화
11. **Tool Search**: MCP 도구 수가 많아질 때 온디맨드 검색
12. **Cross-Model 워크플로우**: 계획(Opus) + 검증(다른 모델) 분리
