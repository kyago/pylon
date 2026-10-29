# Pylon

**사람은 요구사항만 전달하고, AI 에이전트 팀이 알아서 수행합니다.**

Pylon은 provider-neutral 멀티도메인 AI 오케스트레이터입니다. `pylon`을 실행하면 설정과 capability에 맞는 interactive provider adapter가 선택되고, 루트 에이전트(루트 세션)가 사용자의 요구사항을 분석하여 적절한 도메인과 전문 에이전트 팀을 오케스트레이션합니다. 기본 제공 adapter는 Claude Code와 Codex입니다. 파이프라인 커맨드는 Claude Code에서는 `/pl:*` 슬래시 커맨드로, Codex에서는 `.agents/skills/`에 생성되는 `$pl-*` 스킬로 노출됩니다. 설치된 provider가 하나뿐이면 그것이 primary로 자동 설정되고, 둘 이상이면 첫 실행(doctor/launch) 시 인터랙티브 선택 후 전용으로 고정됩니다.

## 요구 사항

| 도구 | 용도 | 설치 |
|------|------|------|
| **Go** 1.24+ | 빌드 | https://go.dev/dl/ |
| **git** | 버전 관리 | 기본 설치 |
| **gh** | GitHub PR 생성 | `brew install gh` |
| **Claude Code** | AI 에이전트 백엔드 | https://docs.anthropic.com/en/docs/claude-code |

`pylon doctor`로 설치 상태를 확인할 수 있습니다.

## 설치

Pylon은 [CalVer](https://calver.org/)(`YYYY.M.SEQ`, 예: `2026.6.3`) 버전 체계를 사용하며,
플랫폼별 사전 빌드 바이너리를 [GitHub Releases](https://github.com/kyago/pylon/releases)로 배포합니다.

### 1) 사전 빌드 바이너리 (권장)

[Releases](https://github.com/kyago/pylon/releases/latest)에서 플랫폼에 맞는 아카이브를 받습니다.
파일명 형식: `pylon_<버전>_<OS>_<ARCH>.tar.gz` (Windows는 `.zip`).

```bash
# 예시: macOS arm64, 최신 버전이 2026.6.3인 경우
curl -L -o pylon.tar.gz \
  https://github.com/kyago/pylon/releases/latest/download/pylon_2026.6.3_darwin_arm64.tar.gz
tar -xzf pylon.tar.gz
sudo mv pylon /usr/local/bin/    # 또는 PATH에 포함된 임의 디렉토리로 이동
pylon version
```

이후 업데이트는 `pylon update` 한 줄로 끝납니다 (아래 [업데이트](#업데이트) 참고).

### 2) go install

```bash
go install github.com/kyago/pylon/cmd/pylon@latest
```

> **참고:** 릴리스 태그(`2026.6.3`)는 Go 모듈 버전 형식(`vX.Y.Z`)이 아니므로 `go install`은
> 항상 `main` 최신 커밋을 빌드하며 `@<버전>` 지정은 동작하지 않습니다. 이 경우 `pylon version`은
> 릴리스 번호 대신 커밋 기반 의사 버전을 표시합니다. 특정 릴리스가 필요하면 사전 빌드 바이너리를 사용하세요.
>
> `go install`은 바이너리를 `$GOPATH/bin`(기본값: `~/go/bin`)에 설치합니다. 해당 디렉토리가
> `PATH`에 포함되어 있는지 확인하세요: `export PATH="$PATH:$(go env GOPATH)/bin"`

### 3) 소스 빌드

```bash
git clone https://github.com/kyago/pylon.git
cd pylon
make build    # bin/pylon 생성
make install  # $(go env GOPATH)/bin/pylon 설치
```

## 업데이트

### pylon update (권장)

설치된 바이너리는 내장 명령으로 GitHub Releases의 최신/특정 릴리스를 받아 스스로 교체합니다:

```bash
pylon update              # 최신 버전 설치
pylon update 2026.6.3     # 특정 버전 설치 (CalVer)
```

`pylon update`는 플랫폼에 맞는 바이너리를 내려받아 현재 실행 파일을 교체한 뒤, 자동으로
`pylon doctor`를 실행하여 워크스페이스의 내장 리소스(에이전트/스킬/커맨드/스크립트)와 설정
기본값을 최신 상태로 동기화합니다.

> Windows는 실행 중인 바이너리 자동 교체를 지원하지 않으므로 Releases에서 수동으로 받으세요.

### 소스 빌드로 설치한 경우

```bash
cd pylon
git pull origin main
make build
make install
```

### 업데이트 후 확인

```bash
pylon version    # 버전 확인
pylon doctor     # 의존성 + 워크스페이스 리소스 동기화 상태 확인
```

> **기존 워크스페이스 마이그레이션:** 레거시 데이터(SQLite 메모리, Fossil 이력)는 새 버전으로
> **이전되지 않고 삭제**됩니다. 슬래시 커맨드 `/pl:migrate`가 레거시 파일 폐기 절차를 안내합니다.
> 새 메모리(`.pylon/memory/`)와 이력(`.pylon/history/pipelines/`)은 빈 상태로 시작합니다.

## 빠른 시작

### 1. 워크스페이스 초기화

```bash
mkdir my-workspace && cd my-workspace
pylon init
```

### 2. 프로젝트 추가

```bash
pylon add-project https://github.com/user/my-app.git
```

### 3. 실행

```bash
pylon
```

Permission Mode를 선택하면 Claude Code TUI 세션이 시작됩니다.

```
Permission Mode 선택
Claude Code 실행 권한을 설정합니다

> default — 매번 권한 확인
  acceptEdits — 파일 편집 자동 허용
  bypassPermissions — 모든 권한 자동 허용
```

### 4. 파이프라인 실행

TUI 세션에서 슬래시 커맨드 또는 자연어로 요구사항을 전달합니다:

```
> /pl:pipeline 로그인 기능 구현해줘
```

또는 직접 대화:

```
> 로그인 기능 구현해줘

루트 세션: 몇 가지 확인이 필요합니다.
  1. 인증 방식은 JWT와 세션 중 어떤 것을 선호하시나요?
  2. 소셜 로그인도 필요한가요?
  ...
```

Claude Code TUI가 LLM-as-Orchestrator로 동작하며, 슬래시 커맨드와 셸 스크립트를 조합하여 전체 파이프라인(분석 → 설계 → 구현 → PR)을 자동 수행합니다.

## 동작 방식

### 아키텍처

```
사용자 ←→ Claude Code TUI (LLM-as-Orchestrator)
              │
              ├── 슬래시 커맨드 (.pylon/builtin/commands/)
              │   ├── /pl:pipeline ─ 전체 파이프라인
              │   ├── /pl:architect ─ 아키텍처 분석
              │   ├── /pl:breakdown ─ 태스크 분해
              │   ├── /pl:execute ─── 에이전트 병렬 실행
              │   ├── /pl:verify ─── 빌드/테스트/린트 검증
              │   ├── /pl:pr ─────── PR 생성
              │   ├── /pl:status ─── 파이프라인 상태 조회
              │   ├── /pl:cancel ─── 취소 및 정리
              │   ├── /pl:index ─── 코드베이스 인덱싱
              │   ├── /pl:project-list ─ 프로젝트 목록 조회
              │   ├── /pl:add-project ─ 프로젝트 추가
              │   └── /pl:migrate ─── 레거시 마이그레이션 안내
              │
              └── 셸 스크립트 (.pylon/builtin/scripts/bash/)
                   ├── init-pipeline.sh
                   ├── run-verification.sh
                   ├── create-pr.sh
                   └── ... (원자적 작업)
```

Pylon은 Go CLI가 워크스페이스를 준비한 뒤 provider router로 interactive adapter를 선택하고, adapter가 만든 실행 계획을 `syscall.Exec`으로 시작합니다. 현재 Claude Code adapter는 TUI를 LLM-as-Orchestrator로 실행하며, 슬래시 커맨드와 셸 스크립트를 조합해 파이프라인을 수행합니다.

### 핵심 개념

| 개념 | 설명 |
|------|------|
| **`.pylon/`** | 워크스페이스 소스 오브 트루스 (설정, 에이전트 정의, 도메인 지식) |
| **`.claude/`** | `pylon` 실행 시 `.pylon/`에서 동적 생성되는 Claude Code 설정 (agents 심링크, commands, hooks) |
| **AGENTS.md** | 루트 에이전트 운영 가이드. pylon 블록(매뉴얼 + 워크스페이스 정보)은 실행마다 재생성되고, 블록 밖은 사용자 규칙 영역 (`CLAUDE.md`는 `@AGENTS.md` import 마커) |
| **슬래시 커맨드** | AI가 사용하는 내부 스킬 (`.pylon/builtin/commands/`, 사용자 추가분은 `.pylon/commands/`) |
| **파이프라인** | 파일 기반 워크플로우 (산출물 존재 = 단계 완료) |
| **task runtime** | append-only event와 lease/fencing으로 재시작 가능한 태스크 제어 상태 |
| **프로젝트 메모리** | `.pylon/memory/` 마크다운 파일 기반 지식 저장소 (git 추적) |

### 파이프라인 흐름

```
/pl:pipeline "로그인 기능 구현"
    │
    ├─ [1] root pipeline 초기화 ── requirement.md (branch 없음)
    ├─ [2] 루트 세션 분석 ──────── requirement-analysis.md
    ├─ [3] Architect 분석 ──────── architecture.md + repos.json
    ├─ [4] repo sub-pipeline ───── repo별 branch + base revision
    ├─ [5] 루트 세션 태스크 분해 ─ tasks.json (repo 소유권 포함)
    ├─ [6] Agent 병렬 실행 ─────── execution-log.json
    ├─ [7] repo별 검증/PR ──────── verification.json + pr.json
    ├─ [8] terminal checkpoint
    ├─ [9] checkpoint 확인 cleanup
    └─ 완료 보고
```

### Durable task runtime

`.pylon/runtime/<run-id>/events.jsonl`은 태스크 상태 전이의 source of truth입니다. 각 이벤트가
먼저 append·fsync된 뒤 `tasks/<task-id>/state.json`과 attempt별 `lease.json`, `provider.json`,
`result.json`이 materialize됩니다. 프로세스가 이벤트 기록 직후 종료되어도 replay가 파생 상태를
복구하며, 커널 file lock은 비정상 종료 시 자동 해제됩니다.

태스크는 `pending → ready → claimed → running → verifying → succeeded` 순서로 진행합니다.
worker 성공 보고만으로는 완료되지 않으며 deterministic verification과 evaluator가 모두 통과해야
`succeeded`가 됩니다. lease가 만료된 실행은 `interrupted`로 전환되고, provider 세션을 이어가는
`resume`은 attempt를 유지하는 반면 새 작업을 시작하는 `retry`는 attempt를 증가시킵니다. 두 경우
모두 fencing token을 교체하여 이전 worker의 늦은 쓰기를 거부합니다.

repo worker가 시작되기 전 `.pylon/verify.yml`과 정규화된 acceptance criteria는 repo runtime의
`criteria.json`으로 snapshot되고 digest가 repo `status.json`에 결합됩니다. deterministic verifier는
live 설정이 아니라 snapshot 명령만 실행합니다. snapshot/manifest가 달라지면 명령 실행 전 실패하고,
live `verify.yml`만 변경되면 frozen 명령을 실행해 증거를 남긴 뒤 전체 결과를 fail-closed합니다.
`verify.yml`의 `held_out` 명령도 snapshot에 포함되어 일반 build/test/lint 뒤에 실행됩니다.

deterministic gate가 통과하면 Pylon은 requirement, criteria snapshot, verification output, diff, task report만
별도 read-only evaluator bundle로 복사합니다. 결정 evaluator는 `Read/Grep/Glob`과 structured output만
사용하며 Bash/Edit/Write, 구현 agent memory, 전체 대화 이력을 받지 않습니다. evaluator 응답은 Pylon이
request digest와 모든 criterion coverage를 검증한 뒤 `evaluator-result.json`으로 기록합니다. `explorer`,
`analyst`, `researcher`는 조사 역할이며 최종 PASS/FAIL을 소유하지 않습니다.

각 task attempt는 provider 자연어 응답 대신 digest가 포함된 `task-report.json`을 남깁니다. 보고서에는
provider/capability, 변경 파일, 검증 증거, 배제된 가설, 남은 불확실성이 포함됩니다. terminal failure는
`failure-record.json`을 먼저 원자적으로 기록하고 status manifest에 결합한 뒤 `failed` history checkpoint를
생성합니다. failed checkpoint가 실패하면 cleanup이 `preserved`로 고정되어 runtime, worktree, 로그가
삭제되지 않습니다.

## 슬래시 커맨드

TUI 세션 내에서 AI가 사용하는 내부 스킬입니다:

| 커맨드 | 설명 |
|--------|------|
| `/pl:pipeline` | 전체 파이프라인 실행 (요구사항 → 분석 → 설계 → 구현 → PR) |
| `/pl:architect` | 아키텍처 분석 단독 실행 |
| `/pl:breakdown` | 태스크 분해 |
| `/pl:execute` | 에이전트 병렬 실행 |
| `/pl:verify` | 빌드/테스트/린트 교차 검증 |
| `/pl:pr` | PR 생성 |
| `/pl:status` | 파이프라인 상태 조회 (파일 기반) |
| `/pl:cancel` | 파이프라인 취소 및 정리 |
| `/pl:index` | 프로젝트 코드베이스 인덱싱 |
| `/pl:project-list` | 프로젝트 목록 및 인덱싱 상태 조회 |
| `/pl:add-project` | 프로젝트를 워크스페이스에 clone하여 추가 |
| `/pl:migrate` | 레거시 워크스페이스(SQLite/Fossil) 폐기·이전 절차 안내 |

## 내장 에이전트 (20종)

`.pylon/builtin/agents/`에 설치되며 실행(`pylon`)과 `pylon doctor` 때마다 바이너리 버전에 맞춰 자동 갱신됩니다. 직접 만든 에이전트는 `.pylon/agents/`에 둡니다(`pylon add-agent`). 루트 세션이 요구사항의 도메인을 판별해 적절한 에이전트를 고릅니다.

### 소프트웨어 개발 (14종)

| 에이전트 | 역할 |
|----------|------|
| architect | 아키텍트 (설계, `architecture.md`·`repos.json` 작성) |
| dev | 개발자 (백엔드·프론트엔드·테스트 구현) |
| code-reviewer | 코드 리뷰어 (보안 점검 포함) |
| critic | 계획·설계 비평 |
| verifier | 격리 evaluator 검증 |
| debugger | 디버거 (근본 원인 추적, 진단 전용 모드) |
| explorer | 코드베이스 탐색 |
| researcher | 리서치 (웹·학술·공식 문서) |
| analyst | 요구사항 분석 |
| refactorer | 리팩토링·코드 단순화 |
| perf-engineer | 성능 엔지니어 |
| devops | DevOps (인프라, CI, 배포 설정) |
| git-master | Git 워크플로 |
| designer | UI/UX 디자이너 |

### 리서치·콘텐츠·마케팅 (6종)

| 에이전트 | 역할 |
|----------|------|
| writer | 작가 (콘텐츠, 보고서, 카피) |
| editor | 편집자 (콘텐츠 QA) |
| strategist | 전략가 (콘텐츠, 브랜드, 캠페인) |
| market-researcher | 시장 조사 분석가 |
| fact-checker | 팩트 체커 (교차 검증) |
| seo-specialist | SEO 전문가 |

## CLI 명령어

Go 바이너리가 제공하는 유틸리티 명령입니다. 루트 에이전트가 파이프라인 제어와 메모리 접근에 사용합니다.

### 기본 명령

| 명령어 | 설명 |
|--------|------|
| `pylon` | 선택된 interactive provider 세션 실행 (기본 동작) |
| `pylon init` | 워크스페이스 초기화 |
| `pylon doctor` | 필수 도구(git/gh/claude) 확인 + 워크스페이스 리소스·설정 동기화 (`--fix-excludes`) |
| `pylon version` | 버전 정보 |
| `pylon update [version]` | 최신/특정 릴리스 바이너리로 자체 업데이트 |
| `pylon add-project <url>` | 프로젝트를 워크스페이스에 clone하여 추가 |
| `pylon delete-project <name>` | 프로젝트 등록 해제 (`--purge`로 디렉터리까지 삭제, `--force`로 확인 생략) |
| `pylon add-agent <name>` | 커스텀 에이전트 추가 (`--domain`, `--role`) |
| `pylon add-skill <name>` | 커스텀 스킬 추가 |
| `pylon status` | 파이프라인 및 에이전트 상태 조회 |
| `pylon cancel [pipeline-id]` | 진행 중인 파이프라인 취소 |
| `pylon uninstall` | 워크스페이스 완전 제거 |

### 프로젝트 메모리

에이전트가 프로젝트 지식을 저장/검색할 때 사용합니다:

```bash
pylon mem list --project <name>                        # 메모리 목록
pylon mem search --project <name> --query "검색어"      # 토큰 매칭 검색
pylon mem store --project <name> --key "키" --content "내용"  # 저장, 같은 key면 갱신 (--category 지정 가능)
pylon mem delete --project <name> --key "키"            # 삭제 (--category, --dry-run 지원)
```

### 동기화

| 명령어 | 설명 |
|--------|------|
| `pylon sync-agents` | 내장 리소스(`.pylon/builtin/`)와 `.claude/agents/` 심링크를 즉시 동기화 |
| `pylon mem inject` | 세션 시작 시 호출(Claude Code 훅; Codex는 수동 `pylon mem inject`) — 만료 메모리(`retention_days`)를 정리한 뒤 프로젝트 메모리를 세션 컨텍스트에 주입 |

### 작업 이력

Pylon은 `.pylon/history/pipelines/<pipeline-id>/<phase>/` 디렉토리 스냅샷에 선별된 작업 이력을
기록합니다. 파이프라인마다 planned, executed, completed/cancelled/failed 체크포인트를 생성할 수 있습니다.

terminal snapshot은 criteria, deterministic verification, evaluator verdict, task reports, attempt state,
provider handle/capability evidence, failure records와 rejected hypotheses를 함께 보존합니다. 대용량 원본 로그는
복사하지 않아도 되지만 task/failure report의 안정적인 상대 경로와 digest로 참조합니다.

### Acceptance corpus

Pylon에는 multi-repo, resume/retry, stale fencing, partial-write replay, criteria tampering, live verify 변경,
held-out failure, evaluator write boundary, cleanup gate, rejected hypotheses, curator approval을 다루는 13개 provider-neutral
fixture가 내장되어 있습니다. driver는 fixture JSON을 stdin으로 받고 `PYLON_CORPUS_FIXTURE_ID`와
`PYLON_CORPUS_WORKDIR` 환경 변수를 사용해 격리 실행한 뒤 기계적 outcome JSON 하나를 stdout으로 반환합니다.
provider 이름과 자연어 `narrative`는 판정에 사용되지 않습니다.

```bash
# fixture 목록
pylon internal corpus list

# provider/session driver 실행
pylon internal corpus run --driver ./my-corpus-driver --output corpus-report.json

# 이전에 기록한 <fixture-id>.json 결과 재검증
pylon internal corpus run --outcomes ./recorded-outcomes --output corpus-report.json
```

각 fixture는 verdict, run/task 상태, 필수 artifact/event, 금지 mutation, cleanup 및 runtime 보존 여부만
비교합니다. 따라서 두 provider의 설명 문구가 달라도 동일한 상태·증거를 만들면 같은 판정을 받습니다.

### Native curator

curator는 active run이나 evaluator가 미확정인 run을 읽지 않습니다. completed/failed checkpoint의 artifact와
combined digest를 다시 검증하고, criteria·verification·evaluator·task report·failure collection이 모두 확정된
경우에만 `.pylon/learning/candidates/<candidate-id>/` 아래에 후보를 만듭니다.

```bash
cat > proposal.json <<'JSON'
{
  "type": "memory",
  "title": "Preserve failed verification evidence",
  "summary": "실패 cleanup 전에 evidence reference를 보존한다.",
  "rationale": "finalized run에서 재현된 운영 위험이다.",
  "target_files": [".pylon/memory/app/learning/cleanup.md"],
  "evidence_refs": ["failure-records-summary.json"],
  "regression_fixtures": ["cleanup-terminal-checkpoint-gate"]
}
JSON

pylon internal curator propose --checkpoint <pipeline-id>/failed --input proposal.json
pylon internal curator review --candidate <candidate-id> --decision approve --reason "evidence reviewed"
pylon internal curator gate --candidate <candidate-id> --report corpus-report.json
```

후보의 proposal/evidence/source/target 파일 digest는 `status.json`에 고정되어 review/gate 전에 재검증됩니다.
승인과 regression gate는 candidate 상태만 변경하며 active memory, skill, agent prompt, pipeline rule을 자동으로
수정하지 않습니다. 실제 적용은 사용자가 candidate diff를 검토한 뒤 별도 commit 또는 PR로 수행합니다.

```bash
pylon history log --pipeline <pipeline-id>
pylon history show <pipeline-id>/<phase>
pylon history diff <from-ref> <to-ref>
pylon history export <pipeline-id>/<phase> --output <new-directory>
```

모든 명령에 `--json` 플래그를 추가하면 JSON 형식으로 출력됩니다.

## 워크스페이스 구조

```
workspace/
├── .pylon/                    # 소스 오브 트루스 (git 추적)
│   ├── config.yml             # 워크스페이스 설정
│   ├── builtin/               # 내장 리소스 (git 무시, 버전이 바뀌면 통째로 재생성)
│   │   ├── agents/            # 에이전트 정의 (20종)
│   │   ├── skills/            # 에이전트 스킬
│   │   ├── commands/          # 파이프라인 슬래시 커맨드 (pl-pipeline.md, ...)
│   │   ├── scripts/bash/      # 파이프라인 셸 스크립트 (common.sh, init-pipeline.sh, ...)
│   │   └── reference/         # pylon 사용 매뉴얼 (pylon-usage.md)
│   ├── agents/                # 사용자 추가 에이전트 (pylon add-agent)
│   ├── skills/                # 사용자 추가 스킬 (pylon add-skill)
│   ├── commands/              # 사용자 추가 파이프라인 커맨드
│   ├── domain/                # 팀 도메인 지식 (위키)
│   ├── tasks/                 # 확정된 태스크 스펙
│   ├── memory/                # 프로젝트 메모리 (git 추적)
│   │   └── {project}/          # 프로젝트별 <category>/<slug>.md + INDEX.md
│   ├── runtime/               # (git 무시)
│   │   ├── {pipeline-id}/     # 논리 root pipeline 산출물
│   │   │   ├── events.jsonl   # durable task 상태 전이 source of truth
│   │   │   ├── tasks/         # task spec/state + attempt별 lease/provider/result
│   │   │   └── repos/         # repo별 branch/base/criteria/verification/PR 상태
│   │   └── sessions/          # 세션 상태
│   ├── conversations/         # 대화 이력 (git 무시)
│   └── history/               # 파일 기반 작업 이력 (git 무시)
│       └── pipelines/         # {pipeline-id}/{phase}/ 디렉토리 스냅샷
│
├── .claude/                   # 동적 생성 (git 무시)
│   ├── agents/                # .pylon/builtin/agents/, .pylon/agents/ 심링크
│   ├── commands/              # 슬래시 커맨드 심링크
│   └── settings.json          # Claude Code hooks 설정
│
├── AGENTS.md                  # 운영 가이드 (pylon 블록은 재생성, 블록 밖은 사용자 소유)
├── CLAUDE.md                  # @AGENTS.md import 마커 (동적 생성)
│
├── project-a/                 # 프로젝트 디렉토리
│   └── .pylon/context.md      # 프로젝트 컨텍스트
└── project-b/                 # 프로젝트 디렉토리
```

## 설정

`.pylon/config.yml`:

```yaml
version: "0.2"

runtime:
  provider: auto               # provider 이름 또는 auto
  execution_mode: session-native
  permission_mode: acceptEdits # default | acceptEdits | bypassPermissions
  # effort: high               # 선택, 기본값 없음 — provider별 허용값 (claude: low|medium|high|xhigh|max, codex: model_reasoning_effort 값)
  # model: opus                # 선택, 기본값 없음 — provider별 모델 이름

providers:
  claude-code:
    enabled: auto
    command: claude
  codex:
    enabled: auto
    command: codex

routing:
  fallback: serial
  require_resume_for_background: true

git:
  branch_prefix: task          # 작업 브랜치 접두사
  default_base: main           # 기본 베이스 브랜치
  auto_push: true              # 자동 푸시
  worktree:
    enabled: true              # git worktree 격리
    auto_cleanup: true         # 완료 후 자동 정리
  pr:
    draft: false
    reviewers: []

wiki:
  auto_update: true            # 위키 자동 갱신
  update_on:
    - task_complete
    - pr_merged

memory:
  proactive_injection: true    # 세션 시작 시 프로젝트 메모리 주입 on/off (Claude Code 훅; Codex는 수동 `pylon mem inject`)
  proactive_max_tokens: 2000   # 주입 예산(토큰, 1토큰≈4바이트로 환산)
  retention_days:        # 카테고리별 보존 일수 — 미지정/0 이하 = 영구 보존, {} = 전체 영구 보존
    learning: 30

skills:
  enabled: true                # 에이전트 스킬 주입 활성화
  preload_to_agents: true      # .claude/agents/ 생성 시 스킬 주입
  progressive_disclosure: true # 메타데이터만 주입(true) vs 전체 본문 주입(false)
```

기존 `runtime.backend`와 agent frontmatter의 `backend`는 마이그레이션 기간 동안 deprecated alias로
읽지만 자동으로 파일을 다시 쓰지는 않습니다. `provider`가 함께 있으면 `provider`가 우선합니다.

`runtime`의 `effort`/`model`/`env`(비어 있으면 provider 기본값)를 제외한 모든 설정에는 기본값이 있으므로 `version` 필드만 필수입니다. 위 예시는 전체 필드와 기본값을 보여주며,
명시하지 않은 필드는 로드 시 코드 기본값으로 자동 채워집니다. `pylon init`이 처음 생성하는 `config.yml`은
최소 필드(`runtime`의 `provider`/`execution_mode`/`permission_mode`)만 포함하고, 이후
`pylon doctor`가 누락된 필드를 감지해 기본값으로 동기화합니다.

## 개발

```bash
make build       # 빌드
make test        # 테스트
make lint        # 린트
make clean       # 정리
```

## 기술 스택

| 영역 | 기술 |
|------|------|
| 언어 | Go |
| CLI 프레임워크 | Cobra |
| TUI 컴포넌트 | charmbracelet/huh |
| 프로세스 실행 | syscall.Exec (최소 런처) + Claude Code Agent 도구 (서브에이전트) |
| 파이프라인 작업 | Shell Scripts (원자적 작업) |
| AI 백엔드 | Claude Code CLI |
| 저장소 | 마크다운/JSON 파일 (.pylon/memory, .pylon/history) |
| 메모리 검색 | 토큰 매칭 + INDEX.md 주입 |
| 작업 이력 | 디렉토리 스냅샷 (.pylon/history/pipelines) |

## 라이선스

MIT
