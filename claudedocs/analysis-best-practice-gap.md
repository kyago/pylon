# Pylon 스펙 vs claude-code-best-practice 갭 분석

> 분석일: 2026-03-07
> 대상: pylon-spec.md v0.7 vs claude-code-best-practice 리포지토리 조사 결과
> 참조: research-claude-code-best-practice-repo.md, research-agent-communication-and-memory.md
>
> ⚠️ **참고**: 이 문서에서 언급되는 tmux 관련 내용은 더 이상 유효하지 않습니다. tmux 세션 기반 프로세스 격리는 설계에서 제거되었으며, 현재는 `syscall.Exec` / `exec.Command` 기반 직접 프로세스 실행으로 대체되었습니다. 이 문서는 과거 분석 기록으로서 보존됩니다.

---

## 1. 즉시 반영 항목 (스펙에 바로 추가)

### 1.1 에이전트 턴 수 제한 (maxTurns)

**현재 스펙 상태**: `task_timeout: 30m`과 `max_attempts: 2`만 존재. 에이전트의 LLM 턴(agentic turn) 수 제한이 없음.

**문제**: 타임아웃만으로는 에이전트가 무한 루프에 빠져 토큰을 과도하게 소비하는 것을 방지하지 못한다. 30분 동안 에이전트가 수백 턴을 돌며 같은 실수를 반복할 수 있다.

**변경 내용**:

1) `config.yml` 스키마에 `max_turns` 추가:
```yaml
runtime:
  backend: claude-code
  max_concurrent: 5
  task_timeout: 30m
  max_attempts: 2
  max_turns: 50          # 에이전트당 최대 LLM 턴 수 (기본 50)
```

2) 에이전트 frontmatter에 `maxTurns` 필드 추가:
```yaml
---
name: backend-dev
role: Backend Developer
backend: claude-code
maxTurns: 30              # 이 에이전트의 턴 제한 (config.yml 기본값 override)
---
```

3) 오케스트레이터 구현 시 Claude Code CLI 호출에 `--max-turns` 플래그 적용:
```bash
claude --max-turns 50 --print --output-format stream-json ...
```

**영향도**: 높음 -- 비용 폭주와 무한 루프 방지를 위한 핵심 안전장치

---

### 1.2 에이전트 권한 모드 (permissionMode)

**현재 스펙 상태**: "Claude Code CLI 네이티브 permission 시스템에 의존"이라고만 명시. 에이전트별 권한 모드 미정의.

**문제**: Claude Code CLI는 기본적으로 파일 수정, bash 실행 등에 대해 사용자 승인을 요구한다. 자동화된 에이전트가 매번 권한 프롬프트를 띄우면 무인 실행이 불가능하다.

**변경 내용**:

1) 에이전트 frontmatter에 `permissionMode` 필드 추가:
```yaml
---
name: backend-dev
role: Backend Developer
backend: claude-code
permissionMode: acceptEdits    # default | acceptEdits | bypassPermissions
---
```

| 모드 | 설명 | 권장 대상 |
|------|------|-----------|
| `default` | 모든 작업에 승인 필요 | PO (사용자 인터랙션) |
| `acceptEdits` | 파일 편집은 자동 승인, bash는 승인 필요 | 개발 에이전트 |
| `bypassPermissions` | 모든 작업 자동 승인 | 신뢰도 높은 자동화 에이전트 |

2) `config.yml`에 기본 permissionMode 추가:
```yaml
runtime:
  permission_mode: acceptEdits   # 전체 에이전트 기본값
```

3) 오케스트레이터가 Claude Code CLI 실행 시 `--permission-mode` 플래그 적용.

**영향도**: 높음 -- 이 설정 없이는 무인 에이전트 실행 자체가 불가능

---

### 1.3 Claude Code CLI 실행 명세

**현재 스펙 상태**: 오케스트레이터가 "tmux 세션에서 Claude Code CLI를 실행한다"고만 명시. 구체적인 CLI 플래그, 환경변수, 실행 방법이 미정의.

**문제**: 에이전트 실행의 핵심인 CLI 호출 방식이 정의되지 않으면 구현 단계에서 혼란이 발생한다.

**변경 내용**:

스펙 Section 8에 "에이전트 CLI 실행 명세" 추가:

```markdown
### Claude Code CLI 실행 명세

오케스트레이터가 각 에이전트를 실행할 때 다음 명세를 따른다.

#### 기본 실행 형식
\```bash
claude \
  --print \                                    # 비인터랙티브 출력
  --output-format stream-json \                # 구조화된 스트림 출력
  --max-turns {maxTurns} \                     # 턴 수 제한
  --permission-mode {permissionMode} \         # 권한 모드
  --model {model} \                            # 모델 지정 (선택)
  --append-system-prompt "$(cat claude.md)" \  # 통신 규칙 주입
  --prompt "{task_prompt}"                     # 태스크 프롬프트
\```

#### 환경변수 설정
\```bash
export CLAUDE_CODE_MAX_TURNS={maxTurns}
export CLAUDE_CODE_EFFORT_LEVEL=high           # 사고 깊이 (기본 high)
export CLAUDE_AUTOCOMPACT_PCT_OVERRIDE=80      # 자동 컴팩트 임계값
\```

#### 인터랙티브 에이전트 (PO)
PO 에이전트는 사용자와 대화해야 하므로 --print 없이 인터랙티브 모드로 실행:
\```bash
claude \
  --max-turns {maxTurns} \
  --permission-mode default \
  --append-system-prompt "$(cat claude.md)"
\```
```

**영향도**: 높음 -- 구현의 기초가 되는 명세

---

### 1.4 Git Worktree 격리 옵션

**현재 스펙 상태**: tmux 세션으로 프로세스 격리, 브랜치 전략으로 git 논리적 격리. 하지만 같은 프로젝트에서 여러 에이전트가 동시에 파일을 수정할 때의 물리적 격리가 없음.

**문제**: 에이전트 A가 `handler.go`를 수정 중일 때 에이전트 B가 같은 파일을 수정하면 git 충돌이 발생한다. 현재 스펙에서는 PM이 순서를 지정(직렬 실행)하거나 다른 프로젝트로 분배(병렬)하는 것으로만 처리하지만, 같은 프로젝트 내에서 병렬 실행이 필요한 경우가 있다.

**변경 내용**:

1) `config.yml`에 git worktree 설정 추가:
```yaml
git:
  branch_prefix: task
  default_base: main
  auto_push: true
  worktree:
    enabled: true              # 에이전트별 worktree 생성 (기본 true)
    base_dir: .git/worktrees   # worktree 위치
    auto_cleanup: true         # 태스크 완료 후 자동 정리
```

2) 실행 흐름 수정:
```
에이전트 태스크 할당 시:
1. git worktree add .git/worktrees/{agent}-{task} -b {branch} {project-dir}
2. tmux 세션의 작업 디렉토리를 worktree 경로로 설정
3. 에이전트 실행 (worktree 내에서 독립 작업)
4. 태스크 완료 후:
   a. 변경사항 커밋 + push
   b. git worktree remove (auto_cleanup=true 시)
```

3) 에이전트 frontmatter에 `isolation` 필드 추가 (선택적):
```yaml
---
name: backend-dev
isolation: worktree          # none | worktree (기본: worktree)
---
```

**영향도**: 높음 -- 같은 프로젝트의 병렬 에이전트 실행 시 git 충돌 방지

---

### 1.5 CLAUDE.md 주입 크기 제한

**현재 스펙 상태**: 에이전트 CLAUDE.md에 통신 규칙을 주입한다고 명시하지만, 크기 제한이 없음.

**문제**: Claude Code의 CLAUDE.md는 200줄 이하일 때 준수율이 높다. 너무 긴 시스템 프롬프트는 에이전트 성능을 저하시킨다.

**변경 내용**:

Section 8 "에이전트 CLAUDE.md 통신 규칙"에 크기 제한 명시:

```markdown
### CLAUDE.md 주입 규칙
- 오케스트레이터가 주입하는 CLAUDE.md는 **200줄 이하**를 유지
- 주입 내용의 우선순위:
  1. 통신 규칙 (inbox/outbox 절차) -- 필수
  2. 태스크 컨텍스트 (수용 기준, 제약 조건) -- 필수
  3. 프로젝트 메모리 요약 (선제적 주입) -- proactive_max_tokens 이내
  4. 도메인 지식 참조 경로 -- 선택
- 도메인 지식 본문은 CLAUDE.md에 직접 포함하지 않고, 파일 경로만 안내하여 에이전트가 필요 시 읽도록 유도
```

**영향도**: 중간 -- 에이전트 성능과 규칙 준수율에 직접 영향

---

### 1.6 에이전트 frontmatter 확장 종합

현재 에이전트 frontmatter에 없는 필드들을 일괄 추가:

**현재**:
```yaml
---
name: backend-dev
role: Backend Developer
backend: claude-code
scope:
  - project-api
tools:
  - git
  - gh
  - docker
---
```

**확장**:
```yaml
---
name: backend-dev
role: Backend Developer
backend: claude-code
scope:
  - project-api
tools:
  - git
  - gh
  - docker
maxTurns: 30                   # 최대 LLM 턴 수 (기본: config.yml의 max_turns)
permissionMode: acceptEdits    # 권한 모드 (기본: config.yml의 permission_mode)
isolation: worktree            # 격리 방식 (기본: worktree)
model: sonnet                  # 모델 지정 (기본: config.yml의 backend 모델, 선택)
---
```

| 필드 | 타입 | 기본값 | 설명 |
|------|------|--------|------|
| `maxTurns` | int | config.yml `max_turns` | 에이전트별 LLM 턴 수 제한 |
| `permissionMode` | string | config.yml `permission_mode` | default, acceptEdits, bypassPermissions |
| `isolation` | string | `worktree` | none, worktree |
| `model` | string | (backend 기본) | sonnet, opus, haiku 등 |

**영향도**: 높음 -- 에이전트 설정의 표현력과 안전성 동시 향상

---

## 2. 중기 반영 항목 (구현 시 고려)

### 2.1 Hooks 시스템 (에이전트 내부 라이프사이클)

**현재 스펙 상태**: 오케스트레이터 레벨의 이벤트 처리(토픽 구독, 파이프라인 상태)는 설계되어 있으나, 에이전트 "내부"의 도구 실행 전/후 제어가 없음.

**Claude Code의 Hooks**: `.claude/settings.json`에 PreToolUse, PostToolUse, Stop 등 19개 이벤트에 대한 훅을 정의할 수 있음.

**Pylon에 필요한 훅**:

| 훅 이벤트 | 용도 | 우선순위 |
|-----------|------|---------|
| `PreToolUse` | 위험 bash 명령 차단 (rm -rf, git push --force 등) | 높음 |
| `PostToolUse` | 도구 실행 결과 로깅, 파일 변경 추적 | 중간 |
| `Stop` | 에이전트 종료 시 정리 작업, outbox 결과 파일 무결성 검증 | 높음 |

**구현 방식**: 오케스트레이터가 에이전트용 `.claude/settings.json`을 동적으로 생성하여 tmux 세션의 작업 디렉토리에 배치.

**난이도**: 중간 -- Claude Code의 기존 hooks 메커니즘을 활용하므로 별도 구현 불필요, 설정 파일 생성만 필요

---

### 2.2 설정 계층화

**현재 스펙 상태**: 단일 `config.yml`

**제안**: 2단계 계층으로 확장 (managed는 MVP 범위 밖)

```
.pylon/
├── config.yml              # 팀 공유 (git 커밋)
└── config.local.yml        # 개인 설정 (.gitignore)
```

**config.local.yml 용도**:
- 개인 API 키 관련 설정 (향후 다른 백엔드 지원 시)
- 개인 선호 동시 실행 수 (config.yml의 max_concurrent override)
- 대시보드 포트 변경
- PR reviewer 개인 설정

**병합 규칙**: `config.local.yml` > `config.yml` (같은 키는 local이 우선)

**난이도**: 낮음 -- YAML 로딩 시 두 파일을 머지하면 됨

---

### 2.3 도메인 지식 분리 전략 (Skills 패턴)

**현재 스펙 상태**: `.pylon/domain/`에 팀 전체 도메인 지식을 모으고, 에이전트 .md 파일의 Markdown body에 역할별 컨텍스트를 기술.

**Best Practice의 Skills 패턴**:
- **Agent Skill (Preloaded)**: 에이전트 시작 시 자동으로 컨텍스트에 주입되는 전문 지식
- **Skill (Independent)**: 온디맨드로 호출되는 독립 워크플로우

**Pylon 적용 제안**:

```
.pylon/
├── domain/                    # 팀 공통 지식 (모든 에이전트의 CLAUDE.md에 경로 안내)
│   ├── conventions.md
│   ├── architecture.md
│   └── glossary.md
└── skills/                    # 에이전트별 전문 지식 (preload 대상)
    ├── api-design-guide.md    # backend-dev에 preload
    ├── test-strategy.md       # qa에 preload
    └── code-review-rules.md   # 코드 리뷰 시 사용
```

에이전트 frontmatter에 skills 필드 추가:
```yaml
---
name: backend-dev
skills:
  - api-design-guide
  - test-strategy
---
```

**이점**: Progressive disclosure -- 에이전트가 필요한 지식만 컨텍스트에 로딩. domain/의 모든 파일을 읽는 것보다 효율적.

**난이도**: 낮음 -- 디렉토리 구조 추가 + 에이전트 실행 시 skills 파일 내용을 시스템 프롬프트에 주입

---

### 2.4 에이전트별 모델 지정

**현재 스펙 상태**: `runtime.backend: claude-code`만 명시. 에이전트별 모델 선택 불가.

**제안**: 에이전트 frontmatter의 `model` 필드로 비용/성능 최적화

```yaml
# 루트 에이전트: 고성능 모델 사용
---
name: architect
model: opus        # 복잡한 아키텍처 결정에는 최고 모델
---

# 프로젝트 에이전트: 표준 모델
---
name: backend-dev
model: sonnet      # 일반 구현에는 표준 모델 (비용 효율)
---
```

**Claude Code CLI에서**: `claude --model sonnet ...`으로 에이전트별 모델 지정.

**난이도**: 낮음 -- CLI 플래그 하나 추가

---

### 2.5 에이전트별 도구 차단 (disallowedTools)

**현재 스펙 상태**: `tools` 필드로 허용 도구 목록만 정의 (허용 목록 방식)

**제안**: `disallowedTools` 필드 추가 (차단 목록 방식)

```yaml
---
name: qa
tools:
  - git
  - gh
disallowedTools:
  - Write         # QA는 소스코드 직접 수정 금지
  - Edit          # 테스트 코드만 수정 가능하도록 별도 규칙
---
```

**용도**: 역할에 부적절한 도구 사용 방지. 예를 들어 QA 에이전트가 소스코드를 직접 수정하는 것을 차단.

**난이도**: 낮음 -- Claude Code의 settings.json permissions.deny에 매핑

---

### 2.6 컨텍스트 관리 환경변수 명세

**현재 스펙 상태**: `config.yml`의 `memory.compaction_threshold`만 존재. 실제 Claude Code CLI에 전달할 환경변수가 미정의.

**제안**: config.yml에 에이전트 환경변수 섹션 추가:

```yaml
runtime:
  env:
    CLAUDE_AUTOCOMPACT_PCT_OVERRIDE: "80"   # 자동 컴팩트 임계값
    CLAUDE_CODE_EFFORT_LEVEL: "high"        # 사고 깊이
```

에이전트별 override:
```yaml
---
name: architect
env:
  CLAUDE_CODE_EFFORT_LEVEL: "high"          # 아키텍트는 깊은 사고 필요
---

---
name: backend-dev
env:
  CLAUDE_CODE_EFFORT_LEVEL: "medium"        # 구현은 표준 깊이
---
```

**난이도**: 낮음 -- tmux 세션 생성 시 환경변수 설정

---

### 2.7 에이전트별 MCP 서버 지정

**현재 스펙 상태**: 에이전트별 MCP 서버 설정이 없음.

**제안**: 특정 에이전트에만 필요한 MCP 서버를 지정:

```yaml
---
name: frontend-dev
mcpServers:
  - playwright     # 프론트엔드 에이전트만 브라우저 테스팅 필요
  - context7       # 프레임워크 문서 검색
---
```

**용도**: 불필요한 MCP 서버 로딩을 방지하여 에이전트 컨텍스트 절약.

**난이도**: 중간 -- 에이전트별 `.mcp.json` 동적 생성 필요

---

## 3. 이미 반영된 항목

### 3.1 3계층 메모리 아키텍처

**Best Practice**: Claude Code의 agent-memory (user/project/local 3스코프, MEMORY.md 파일 기반)

**Pylon 스펙 현황**: Working Context / Session Memory / Project Memory 3계층 모델이 이미 설계됨 (Section 8). SQLite + BM25 풀텍스트 검색, 카테고리별 분류(architecture, pattern, decision, learning, codebase), 선제적+반응적 접근 방식까지 정의.

**평가**: Pylon의 설계가 Claude Code의 파일 기반 단순 메모리보다 훨씬 정교하고 체계적이다. 추가 변경 불필요.

---

### 3.2 RPI 워크플로우

**Best Practice**: Research -> Plan -> Implement (REQUEST.md -> RESEARCH.md -> PLAN.md -> IMPLEMENT.md)

**Pylon 스펙 현황**: 대화(PO 인터뷰) -> 작업지시서(PM 태스크 분해) -> 에이전트 구현 파이프라인이 구조적으로 동일. conversations/ -> tasks/ -> 구현 흐름으로 이미 반영.

**차이점**: RPI는 단일 에이전트 워크플로우이지만, Pylon은 이를 멀티에이전트 오케스트레이션으로 확장한 상위 개념.

---

### 3.3 Agent Teams

**Best Practice**: 여러 Claude Code 세션이 협업하는 실험적 기능 (tmux/iTerm2 split panes)

**Pylon 스펙 현황**: tmux 기반 멀티에이전트 아키텍처가 핵심 설계. 오케스트레이터가 각 에이전트의 생명주기를 관리하고, inbox/outbox 파일 기반 통신을 중재.

**평가**: Pylon이 Agent Teams의 완성된 프로덕션 버전을 설계하고 있다. Claude Code의 실험적 기능보다 훨씬 체계적.

---

### 3.4 블랙보드 패턴

**Best Practice**: 에이전트 간 비동기 지식 공유를 위한 중앙 저장소

**Pylon 스펙 현황**: SQLite `blackboard` 테이블이 이미 설계됨 (hypothesis, evidence, decision, constraint, result 카테고리). 에이전트의 블랙보드 접근은 오케스트레이터를 통한 간접 접근으로 설계.

---

### 3.5 Narrative Casting (핸드오프 프로토콜)

**Best Practice**: Google ADK의 에이전트 핸드오프 시 서사적 컨텍스트 재구성

**Pylon 스펙 현황**: Section 8 "핸드오프 프로토콜 (Narrative Casting)"에 이미 상세 설계됨. 오케스트레이터가 핵심만 추출하여 압축된 서사적 맥락으로 변환하는 방식.

---

### 3.6 토픽 기반 구독 (Pub/Sub)

**Best Practice**: AutoGen의 토픽 구독 모델 -- 에이전트가 관심 이벤트만 수신

**Pylon 스펙 현황**: `topic_subscriptions` 테이블과 토픽 계층 (task.*, decision.*, bug.* 등)이 이미 설계됨. 에이전트 역할별 기본 구독도 정의 완료.

---

### 3.7 컨텍스트 압축 전략

**Best Practice**: 50% 시점에서 /compact, CLAUDE_AUTOCOMPACT_PCT_OVERRIDE 환경변수

**Pylon 스펙 현황**: 에이전트 주도 방식의 Compaction 전략이 이미 설계됨. CLAUDE.md에 compaction 규칙 주입, config.yml에 compaction_threshold 설정 존재. 오케스트레이터가 compaction 자체에 개입하지 않고, 태스크를 적절한 크기로 분해하여 세션이 과도하게 길어지지 않도록 하는 전략.

---

### 3.8 프로젝트 메모리 + BM25 검색

**Best Practice**: QMD 기반 세션 메모리 시스템 (BM25 + 시맨틱 벡터 검색 하이브리드)

**Pylon 스펙 현황**: `project_memory` 테이블 + `project_memory_fts` (FTS5 BM25 인덱스)가 이미 설계됨. 자동 추출(learnings 필드), 블랙보드 승격, 선제적+반응적 메모리 접근까지 정의.

---

### 3.9 태스크 의존성 관리

**Best Practice**: Claude Code의 Task 시스템 (addBlockedBy/addBlocks 의존성 그래프)

**Pylon 스펙 현황**: PM 에이전트가 태스크 복잡도와 의존성을 분석하여 직렬/병렬 결정. 파이프라인 상태 관리(state.json, pipeline_state 테이블)로 단계별 진행.

**차이점**: Claude Code의 Task는 에이전트 내부 도구이고, Pylon의 태스크는 오케스트레이터가 관리하는 외부 시스템. 설계 수준이 다르므로 직접 비교 부적절.

---

### 3.10 백그라운드 실행

**Best Practice**: 서브에이전트의 `background: true` 필드

**Pylon 스펙 현황**: 모든 에이전트가 tmux 세션에서 실행되므로 본질적으로 백그라운드 실행. 별도 설정 불필요.

---

### 3.11 SPOF 복구

**Best Practice**: (해당하는 구체적 best practice 없음, Pylon 고유 설계)

**Pylon 스펙 현황**: state.json + outbox 스캔 + tmux 세션 생존을 활용한 SPOF 복구 알고리즘이 이미 설계됨.

---

### 3.12 세션 자동 아카이빙

**Best Practice**: QMD의 세션 종료 시 자동 export + embed + index

**Pylon 스펙 현황**: `session_archive` 테이블, 아카이브 트리거(태스크 완료/에이전트 종료/Compaction 발생), 핵심 정보 추출 후 project_memory로 승격하는 파이프라인이 이미 설계됨.

---

## 4. 불필요 항목 (Pylon에 맞지 않거나 과잉)

### 4.1 PTC (Programmatic Tool Calling)

**Best Practice**: Python 스크립트로 여러 도구를 한번에 호출하여 토큰 37% 절감

**불필요 이유**: PTC는 API/Foundry 전용 기능이며 Claude Code CLI에서는 지원되지 않는다. Pylon의 에이전트는 CLI 기반이므로 적용 불가.

---

### 4.2 에이전트 color 필드

**Best Practice**: CLI 출력 색상으로 에이전트 시각 구분

**불필요 이유**: Pylon은 TUI(Bubble Tea)와 Web Dashboard에서 에이전트 상태를 표시한다. 에이전트 시각 구분은 UI 레이어에서 자체적으로 처리하면 되며, 에이전트 설정에 색상 필드를 두는 것은 관심사 혼재(concerns mixing).

---

### 4.3 description의 "PROACTIVELY" 키워드

**Best Practice**: 에이전트 description에 "PROACTIVELY"를 포함하면 Claude Code가 자동으로 해당 에이전트를 호출

**불필요 이유**: Pylon에서는 오케스트레이터가 파이프라인 단계에 따라 에이전트를 직접 호출한다. 에이전트가 다른 에이전트를 자율적으로 호출하는 패턴이 아니므로 PROACTIVELY 키워드가 불필요하다. 에이전트 호출 판단은 PM 에이전트 + 오케스트레이터가 담당한다.

---

### 4.4 Ralph Wiggum Loop (자율 개발 루프)

**Best Practice**: 완료될 때까지 반복하는 자율 루프 플러그인

**불필요 이유**: Pylon의 오케스트레이터가 이미 교차 검증 -> 수정 요청 -> 재검증 루프를 관리한다 (max_attempts로 제한). 에이전트 자체의 자율 루프는 오케스트레이터의 제어권을 우회하므로 Pylon의 아키텍처와 충돌한다.

---

### 4.5 Tool Search

**Best Practice**: MCP 도구가 많을 때 온디맨드로 도구를 검색하여 컨텍스트 절약

**불필요 이유**: Pylon의 에이전트는 제한된 도구 세트(git, gh, docker 등)만 사용하며, MCP 서버 수도 제한적이다. 도구 수가 컨텍스트를 차지할 정도로 많아지는 상황이 발생하지 않는다. 향후 MCP 서버가 10개 이상으로 늘어나면 재검토.

---

### 4.6 CLAUDE.md 모노레포 전략 (Ancestor/Descendant 로딩)

**Best Practice**: 모노레포에서 여러 CLAUDE.md를 계층적으로 배치하여 ancestor/descendant 로딩 활용

**불필요 이유**: Pylon은 워크스페이스 + git submodule 구조이지 모노레포가 아니다. 각 프로젝트는 독립된 git repo이므로 CLAUDE.md 계층적 로딩 전략이 적용되지 않는다. 대신 오케스트레이터가 에이전트 실행 시 필요한 컨텍스트를 명시적으로 주입하는 방식을 사용한다.

---

### 4.7 Cross-Model 워크플로우

**Best Practice**: Plan(Opus) -> QA(GPT-5.4) -> Implement(Opus) -> Verify(Codex)

**불필요 이유**: MVP 범위에서는 단일 백엔드(Claude Code)로 충분하다. 다중 모델 조합은 비용 최적화에 유용하지만, 구현 복잡도를 크게 증가시킨다. 장기적으로 에이전트별 model 필드(2.4에서 중기 반영으로 제안)를 통해 부분적으로 달성 가능하므로, Cross-Model 자체를 스펙에 반영할 필요는 없다.

---

### 4.8 settings.json 팀 공유 (git 체크인)

**Best Practice**: `.claude/settings.json`을 git에 커밋하여 팀 공유

**불필요 이유**: Pylon에서는 `.pylon/config.yml`이 이 역할을 대신한다. Claude Code의 settings.json은 에이전트별로 오케스트레이터가 동적으로 생성하므로 git 커밋 대상이 아니다.

---

## 5. 종합 요약

### 우선순위 매트릭스

| # | 항목 | 카테고리 | 난이도 | 영향도 |
|---|------|---------|--------|--------|
| 1.1 | maxTurns 안전장치 | 즉시 반영 | 낮음 | 높음 |
| 1.2 | permissionMode | 즉시 반영 | 낮음 | 높음 |
| 1.3 | CLI 실행 명세 | 즉시 반영 | 낮음 | 높음 |
| 1.4 | Git Worktree 격리 | 즉시 반영 | 중간 | 높음 |
| 1.5 | CLAUDE.md 크기 제한 | 즉시 반영 | 낮음 | 중간 |
| 1.6 | frontmatter 확장 종합 | 즉시 반영 | 낮음 | 높음 |
| 2.1 | Hooks 시스템 | 중기 | 중간 | 중간 |
| 2.2 | 설정 계층화 | 중기 | 낮음 | 낮음 |
| 2.3 | Skills 패턴 | 중기 | 낮음 | 중간 |
| 2.4 | 에이전트별 모델 | 중기 | 낮음 | 중간 |
| 2.5 | disallowedTools | 중기 | 낮음 | 중간 |
| 2.6 | 환경변수 명세 | 중기 | 낮음 | 중간 |
| 2.7 | MCP 서버 지정 | 중기 | 중간 | 낮음 |

### 핵심 인사이트

1. **Pylon 스펙이 이미 상당히 완성도 높다**: 3계층 메모리, 블랙보드, Pub/Sub, Narrative Casting, SPOF 복구 등 핵심 설계가 이미 반영됨. claude-code-best-practice에서 참고할 수 있는 고급 패턴의 대부분이 이미 스펙에 포함되어 있다.

2. **누락된 것은 주로 "실행 레벨"의 구체적 명세**: 에이전트를 실제로 어떻게 실행하는가(CLI 플래그, 환경변수, 권한 모드, 턴 제한)에 대한 구체적 명세가 빠져 있었다. 이것은 아키텍처 설계가 아닌 구현 상세이므로, 스펙에 추가하면 즉시 구현에 활용 가능하다.

3. **Git Worktree가 가장 중요한 신규 항목**: 멀티에이전트가 같은 프로젝트에서 병렬로 코드를 수정하는 시나리오에서 git 충돌을 물리적으로 방지하는 유일한 방법이다. 현재 스펙에서는 PM이 순서를 정하거나 다른 프로젝트로 분배하는 것으로만 처리하지만, worktree 격리를 추가하면 같은 프로젝트 내 병렬 작업도 안전하게 처리할 수 있다.

4. **claude-code-best-practice의 독자적 가치는 "실전 운영 팁"에 있다**: 200줄 CLAUDE.md 제한, 50% 시점 compact, 에이전트 턴 제한 등은 이론이 아닌 실전 경험에서 나온 것이다. Pylon의 설계에 이러한 운영 수치를 반영하면 실제 사용 시 안정성이 크게 향상된다.
