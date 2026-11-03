# Pylon

**여러 저장소를 Claude Code·Codex로 다룰 때 필요한 프로젝트 지식, 메모리, 검증, 세션 마무리를 맡는 개인용 도구입니다.**

- **잘 맞는 경우:** 한 워크스페이스에서 여러 저장소를 다루는 개인이나 소규모 팀
- **맞지 않는 경우:** 요구사항만 주면 PR까지 자동으로 나오길 기대하는 경우, 안정적인 제품 지원이 필요한 경우
- **현재 상태:** 제작자 개인 용도로 개발합니다. 호환성을 보장하지 않습니다.

Pylon은 Go CLI입니다. 워크스페이스에 `AGENTS.md`, 에이전트, 커맨드, 메모리 훅을 준비해 두면 Claude Code나 Codex 세션이 이를 읽습니다. 단계별 실행은 [Paseo 투두 보드](https://github.com/kyago/paseo-todo-board)에 맡기는 방향으로 옮기고 있고, `/pl:pipeline`은 폐기 예정으로 동결했습니다. 기본 제공 provider adapter는 Claude Code와 Codex입니다. 파이프라인 커맨드는 Claude Code에서는 `/pl:*` 슬래시 커맨드로, Codex에서는 `.agents/skills/`에 생성되는 `$pl-*` 스킬로 노출됩니다. 설치된 provider가 하나뿐이면 그것이 primary로 자동 설정되고, 둘 이상이면 첫 실행(doctor/launch) 시 인터랙티브 선택 후 전용으로 고정됩니다.

## 요구 사항

| 도구 | 용도 | 설치 |
|------|------|------|
| **git** | 버전 관리 | 기본 설치 |
| **gh** | GitHub PR 생성 | `brew install gh` |
| **jq** | 파이프라인 스크립트의 JSON 처리 | `brew install jq` |
| **yq** 또는 **python3 + PyYAML** | 파이프라인 스크립트의 `config.yml` 읽기 | `brew install yq` 또는 `pip3 install pyyaml` |
| **Claude Code** 또는 **Codex** | AI 에이전트 provider (하나 이상) | https://docs.anthropic.com/en/docs/claude-code · https://github.com/openai/codex |
| **Go** 1.24+ | `go install`·소스 빌드 시에만 | https://go.dev/dl/ |

`pylon doctor`로 git·gh·provider 설치 상태를 확인할 수 있습니다 (jq와 yq/python3은 파이프라인 스크립트가 실행 시 확인). yq와 python3이 모두 없으면 파이프라인이 중단되고, python3만 있고 PyYAML이 없으면 `git.branch_prefix`, `git.pr.auto_pr` 설정을 읽지 못해 기본값으로 동작합니다.

## 설치

Pylon은 [CalVer](https://calver.org/)(`YYYY.M.SEQ`) 버전 체계를 사용하며,
플랫폼별 사전 빌드 바이너리를 [GitHub Releases](https://github.com/kyago/pylon/releases)로 배포합니다.

### 1) 사전 빌드 바이너리 (권장)

[Releases](https://github.com/kyago/pylon/releases/latest)에서 플랫폼에 맞는 아카이브를 받습니다.
파일명 형식: `pylon_<버전>_<OS>_<ARCH>.tar.gz` (Windows는 `.zip`).

```bash
# 예시: macOS arm64 (x.x.x는 설치할 버전으로 교체)
curl -L -o pylon.tar.gz \
  https://github.com/kyago/pylon/releases/download/x.x.x/pylon_x.x.x_darwin_arm64.tar.gz
tar -xzf pylon.tar.gz
sudo mv pylon /usr/local/bin/    # 또는 PATH에 포함된 임의 디렉토리로 이동
pylon version
```

이후 업데이트는 `pylon update` 한 줄로 끝납니다 (아래 [업데이트](#업데이트) 참고).

### 2) go install

```bash
go install github.com/kyago/pylon/cmd/pylon@latest
```

> **참고:** 릴리스 태그(`YYYY.M.SEQ`)는 Go 모듈 버전 형식(`vX.Y.Z`)이 아니므로 `go install`은
> 항상 `main` 최신 커밋을 빌드하며 `@<버전>` 지정은 동작하지 않습니다. 이 경우 `pylon version`은
> 릴리스 번호 대신 `dev`를 표시합니다. 특정 릴리스가 필요하면 사전 빌드 바이너리를 사용하세요.
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
pylon update x.x.x        # 특정 버전 설치 (CalVer)
```

`pylon update`는 플랫폼에 맞는 바이너리를 내려받아 현재 실행 파일을 교체한 뒤, 자동으로
`pylon doctor`를 실행하여 워크스페이스의 내장 리소스(에이전트/커맨드/스크립트/매뉴얼)와 설정
기본값을 최신 상태로 동기화합니다. 동기화 대상은 실행한 위치의 워크스페이스 하나이므로, 다른 워크스페이스에서는
각각 `pylon doctor`를 실행하세요. 없어진 내장 리소스는 함께 지워지지만, 더 이상 쓰지 않는 설정 키와 빈 디렉터리,
`*.pylon-bak` 백업은 남습니다(동작에는 영향 없음).

릴리스 조회에는 `GITHUB_TOKEN`, 없으면 `gh auth token` 값을 사용합니다. 둘 다 없으면 토큰 없이
조회하며 GitHub API rate limit(공인 IP 기준)에 걸릴 수 있습니다.

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

Permission Mode를 선택하면 Claude Code TUI 세션이 시작됩니다 (Codex는 `workspace-write`/`read-only`/`danger-full-access` 중 Sandbox를 고릅니다).

```
Permission Mode 선택
Claude Code 실행 권한을 설정합니다

> default — 매번 권한 확인
  acceptEdits — 파일 편집 자동 허용
  bypassPermissions — 모든 권한 자동 허용
```

Paseo를 쓰면 `pylon`을 거치지 않고 워크스페이스에서 세션을 바로 시작해도 됩니다. 이때 생성 파일 갱신은 `pylon doctor`나 `pylon update`가 맡습니다.

### 4. 요구사항 전달

세션에서 자연어로 요구사항을 전달합니다. 모호한 부분은 루트 세션이 되묻습니다:

```
> 로그인 기능 구현해줘

루트 세션: 몇 가지 확인이 필요합니다.
  1. 인증 방식은 JWT와 세션 중 어떤 것을 선호하시나요?
  2. 소셜 로그인도 필요한가요?
  ...
```

큰 작업은 [Paseo 투두 보드](https://github.com/kyago/paseo-todo-board)에 카드로 올려 계획 → 구현 → 검토 → 품질 검증을 단계별로 진행할 수 있습니다. 보드는 별도 플러그인이며, 카드는 git 저장소인 프로젝트 디렉터리에서 추가합니다.

`/pl:pipeline`은 폐기 예정입니다. 기존 방식이 필요할 때만 명시적으로 실행합니다 (Codex는 `$pl-pipeline`).

### 5. PR·머지·마무리

검증이 끝나면 다음 단계는 사용자가 진행합니다 (Codex는 `$pl-cleanup`):

1. **PR 생성**: 대화로 요청합니다. 보드는 품질 검증을 통과하면 초안 PR을 만듭니다. 파이프라인 run은 `/pl:pr`을 쓰고, `git.pr.auto_pr: true`면 파이프라인이 검증 후 직접 만듭니다.
2. **리뷰·머지**: 머지 여부는 사용자가 결정합니다.
3. **마무리**: 머지 후 `/pl:cleanup`. 메모리·도메인 지식을 갱신하고 run을 마감한 뒤, 머지된 브랜치·워크트리를 정리합니다.

대화나 보드로 작업한 경우에도 `/pl:cleanup`으로 끝냅니다.

## 동작 방식

### 아키텍처

```
사용자 ←→ Claude Code TUI (LLM-as-Orchestrator)
              │
              ├── 슬래시 커맨드 (.pylon/builtin/commands/ + Go 내장 index·project-list)
              │   ├── /pl:pipeline ─ 전체 파이프라인 (폐기 예정)
              │   ├── /pl:cleanup ── 세션 마무리 (run 마감·메모리·브랜치 정리)
              │   ├── /pl:architect ─ 아키텍처 분석
              │   ├── /pl:breakdown ─ 태스크 분해
              │   ├── /pl:execute ─── 에이전트 병렬 실행
              │   ├── /pl:verify ─── 빌드/테스트/린트 검증
              │   ├── /pl:pr ─────── PR 생성
              │   ├── /pl:status ─── 파이프라인 상태 조회
              │   ├── /pl:index ─── 코드베이스 인덱싱
              │   ├── /pl:project-list ─ 프로젝트 목록 조회
              │   └── /pl:add-project ─ 프로젝트 추가
              │
              └── 셸 스크립트 (.pylon/builtin/scripts/bash/)
                   ├── init-pipeline.sh
                   ├── run-verification.sh
                   ├── create-pr.sh
                   └── ... (원자적 작업)
```

Pylon은 Go CLI가 워크스페이스를 준비한 뒤 provider router로 interactive adapter를 선택하고, adapter가 만든 실행 계획을 `syscall.Exec`으로 시작합니다. 현재 Claude Code adapter는 TUI를 LLM-as-Orchestrator로 실행하며, 슬래시 커맨드와 셸 스크립트를 조합해 파이프라인을 수행합니다. Codex adapter는 같은 절차를 `$pl-*` 스킬로 수행합니다.

### 핵심 개념

| 개념 | 설명 |
|------|------|
| **`.pylon/`** | 워크스페이스 소스 오브 트루스 (설정, 에이전트 정의, 도메인 지식) |
| **`.claude/`** | `pylon` 실행 시 `.pylon/`에서 동적 생성되는 Claude Code 설정 (agents 심링크, commands, hooks) |
| **AGENTS.md** | 루트 에이전트 운영 가이드. pylon 블록(매뉴얼 + 워크스페이스 정보)은 실행마다 재생성되고, 블록 밖은 사용자 규칙 영역 (`CLAUDE.md`는 `@AGENTS.md` import 마커) |
| **슬래시 커맨드** | 사용자가 입력하거나 루트 세션이 실행하는 워크플로우 (`.pylon/builtin/commands/`, 사용자 추가분은 `.pylon/commands/`, Codex는 `$pl-*` 스킬) |
| **파이프라인** | 파일 기반 워크플로우 (산출물 존재 = 단계 완료) |
| **검증 게이트** | 고정된 criteria snapshot으로 돌린 deterministic 검증 + 격리 evaluator 판정이 모두 통과해야 완료 |
| **프로젝트 메모리** | `.pylon/memory/` 마크다운 파일 기반 지식 저장소 (git 추적) |

### 파이프라인 흐름

> 폐기 예정으로 동결된 `/pl:pipeline`의 흐름입니다. 새로운 큰 작업은 투두 보드를 권장합니다.

```
/pl:pipeline "로그인 기능 구현"
    │
    ├─ [1] root pipeline 초기화 ── requirement.md (branch 없음)
    ├─ [2] 루트 세션 분석 ──────── requirement-analysis.md
    ├─ [3] Architect 분석 ──────── architecture.md + repos.json
    ├─ [4] repo sub-pipeline ───── repo별 branch + base revision
    ├─ [5] 루트 세션 태스크 분해 ─ tasks.json (repo 소유권 포함)
    ├─ [6] Agent 병렬 실행 ─────── execution-log.json
    ├─ [6B] repo별 커밋 ─────────── task 브랜치에 이번 run의 변경 커밋
    ├─ [7] repo별 검증 ─────────── verification.json + task-report.json
    ├─ [7B] 격리 evaluator ─────── evaluator-result.json (두 gate 통과 = verified)
    ├─ [8] PR (git.pr.auto_pr=true일 때만, 아니면 /pl:pr)
    ├─ [9] 완료 보고 + 인계 ──── status: awaiting_cleanup
    └─ 머지 후 /pl:cleanup ──── 메모리·도메인 지식 → terminal checkpoint → runtime·worktree·브랜치 정리
```

bugfix·hotfix는 결함 재현을 첫 수용 기준으로 두고, 재현 테스트(수정 전 실패)를 첫 태스크로 실행한 뒤 수정
태스크가 그것에 의존합니다.

### 검증 게이트

파이프라인 상태는 `.pylon/runtime/<pipeline-id>/`의 산출물 파일이 전부입니다. root run은 요구사항과 repo
목록을 묶는 논리 run이고(branch 없음), branch·base revision·worktree·검증·PR은 repo별 sub-pipeline
(`repos/<repo-id>/`)이 소유합니다. 재실행하면 이미 있는 산출물은 건너뛰고 `status.json`의 마지막 단계부터
재개합니다. worker의 "완료" 보고만으로는 성공 처리하지 않으며, 아래 두 gate가 모두 통과해야 태스크가
`execution-log.json`에 `verified: true`로 기록됩니다.

repo worker가 시작되기 전 `.pylon/verify.yml`과 정규화된 acceptance criteria는 repo runtime의
`criteria.json`으로 snapshot되고 digest가 repo `status.json`에 결합됩니다. deterministic verifier는
live 설정이 아니라 snapshot 명령만 실행합니다. snapshot/manifest가 달라지면 명령 실행 전 실패하고,
live `verify.yml`만 변경되면 frozen 명령을 실행해 증거를 남긴 뒤 전체 결과를 fail-closed합니다.
`verify.yml`의 `held_out` 명령은 별도 `evaluator-only/held-out.json`으로 snapshot되어 일반 build/test/lint 뒤에
실행되며, 그 경로·내용은 구현 agent 프롬프트나 evaluator bundle에 전달하지 않습니다.

검증은 커밋된 코드에 대해서만 근거를 남깁니다. 파이프라인은 검증 전에 task 브랜치에 커밋하고(`init-pipeline.sh`는
추적 안 된 파일까지 없는 깨끗한 트리에서만 시작), `verification.json`의 `tree`에는 작업 트리가 깨끗할 때 검증한
HEAD의 tree가 기록됩니다. 미커밋 변경·추적 안 된 파일·assume-unchanged/skip-worktree 표시가 붙은 파일·stage된 `.pylon/`
변경이 있거나, 검증 단계가 추적 파일을 고치거나 커밋하면 `tree` 대신 `tree_reason`이 남습니다. 검증 명령이 새로
만든 추적 안 된 파일(coverage, `__pycache__` 등)은 HEAD와 동결된 명령으로 다시 만들어지므로 검증이 직접 run 폴더의
`verify-artifacts/`(같은 경로는 최신본으로 덮어씀)로 옮기고, 커밋 뒤 남은 run 잔여물은 지정한 파일만 `leftovers/<시각>/`으로 옮기고 run을 정리할 때
`.pylon/runtime/leftovers/<run>/`에 보존합니다 — repo나 ignore 규칙에 쌓이지 않습니다. `/pl:pr`은 이 tree를 브랜치 tree와 대조해 다르면 재검증한 뒤 PR을 만들고,
`/pl:cleanup`은 tree가 머지된 PR head와 같을 때만 그 검증 결과를 근거로 씁니다. 이 규칙의 정본은 AGENTS.md의
"Verification evidence contract"입니다 — 검증하지 않은 코드에 tree를 남기지 않고 사용자 파일을 잃지 않으며, 그 밖의
드문 경우는 tree 없이 실패합니다.

deterministic gate가 통과하면 Pylon은 requirement, criteria snapshot, verification output, diff, task report만
별도 read-only evaluator bundle로 복사합니다. 결정 evaluator는 `Read/Grep/Glob`과 structured output만
사용하며 Bash/Edit/Write, 구현 agent memory, 전체 대화 이력을 받지 않습니다. evaluator 응답은 Pylon이
request digest와 모든 criterion coverage를 검증한 뒤 `evaluator-result.json`으로 기록합니다. `explorer`,
`analyst`, `researcher`는 조사 역할이며 최종 PASS/FAIL을 소유하지 않습니다.

deterministic gate를 통과한 repo는 worker 자연어 보고 대신 digest가 포함된 `task-report.json`을 남깁니다
(`pylon internal trajectory task-report`). 보고서에는 provider/capability, 변경 파일, 검증 증거, 배제된 가설,
남은 불확실성이 포함됩니다. terminal failure는
`failure-record.json`을 먼저 원자적으로 기록하고 status manifest에 결합한 뒤 `failed` history checkpoint를
생성합니다. failed checkpoint가 실패하면 cleanup이 `preserved`로 고정되어 runtime, worktree, 로그가
삭제되지 않습니다.

## 슬래시 커맨드

세션에서 사용자가 직접 입력하거나 루트 세션이 판단해 실행합니다. Codex에서는 같은 커맨드가 `$pl-<이름>` 스킬로 생성됩니다
(`/pl:index`·`/pl:project-list`는 Claude Code 전용).

### 주로 직접 실행하는 커맨드

| 커맨드 | 설명 |
|--------|------|
| `/pl:pipeline` | **폐기 예정.** 전체 파이프라인 실행 (요구사항 → 분석 → 설계 → 구현 → 검증). 끝나면 run을 `/pl:cleanup`에 넘김 |
| `/pl:cleanup` | 세션 마무리 (파이프라인 사용 여부와 무관): 검증 근거, 메모리·도메인 지식 갱신, 머지된 파이프라인 run 마감, 머지된 브랜치·워크트리 정리 |
| `/pl:pr` | **폐기 예정.** 파이프라인 run의 PR 생성 (repo sub-pipeline 단위) |
| `/pl:status` | 파이프라인 상태 조회 (파일 기반) |
| `/pl:index` | 프로젝트 코드베이스 인덱싱 |
| `/pl:project-list` | 프로젝트 목록 및 인덱싱 상태 조회 |
| `/pl:add-project` | 프로젝트를 워크스페이스에 clone하여 추가 |

### 파이프라인 단계 단독 실행

`/pl:pipeline`이 수행하는 단계를 따로 돌릴 때 씁니다. 파이프라인과 함께 폐기 예정입니다.

| 커맨드 | 설명 |
|--------|------|
| `/pl:architect` | 아키텍처 분석 단독 실행 |
| `/pl:breakdown` | 태스크 분해 |
| `/pl:execute` | 에이전트 병렬 실행 |
| `/pl:verify` | 빌드/테스트/린트 교차 검증 |

## 내장 에이전트 (14종)

`.pylon/builtin/agents/`에 설치되며 실행(`pylon`)과 `pylon doctor` 때마다 바이너리 버전에 맞춰 자동 갱신됩니다. 직접 만든 에이전트는 `.pylon/agents/`에 둡니다(`pylon add-agent`). 루트 세션이 작업에 맞는 에이전트를 고릅니다.

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

## CLI 명령어

Go 바이너리가 제공하는 명령입니다. 기본 명령은 주로 사용자가 실행하고, 메모리·이력·내부 명령은 주로 루트 에이전트와 파이프라인이 사용합니다.

### 기본 명령

| 명령어 | 설명 |
|--------|------|
| `pylon` | 선택된 interactive provider 세션 실행 (기본 동작) |
| `pylon init` | 워크스페이스 초기화 |
| `pylon doctor` | 필수 도구(git/gh/provider) 확인 + 워크스페이스 리소스·훅·설정 동기화 (`--fix-excludes`, `--yes`로 비인터랙티브) |
| `pylon version` | 버전 정보 |
| `pylon update [version]` | 최신/특정 릴리스 바이너리로 자체 업데이트 후 `doctor` 실행 |
| `pylon add-project <url>` | 프로젝트를 워크스페이스에 clone하여 추가 (`--name`, `--force`로 재clone, `--skip-clone`으로 기존 디렉터리에 `.pylon/`만 생성) |
| `pylon delete-project <name>` | 프로젝트 등록 해제 (`--purge`로 디렉터리까지 삭제, `--force`로 확인 생략) |
| `pylon add-agent <name>` | 커스텀 에이전트 추가 (`--domain`, `--role`) |
| `pylon add-skill <name>` | 커스텀 스킬 추가 |
| `pylon status` | 파이프라인 상태 조회 |
| `pylon uninstall` | 워크스페이스 완전 제거 (`--dry-run`, `--force`, `--remove-projects`, `--remove-binary`) |

### 프로젝트 메모리

에이전트가 프로젝트 지식을 저장/검색할 때 사용합니다:

```bash
pylon mem list --project <name>                        # 메모리 목록
pylon mem search --project <name> --query "검색어"      # 토큰 매칭 검색
pylon mem store --project <name> --key "키" --content "내용"  # 저장, 같은 key면 갱신 (--category 지정 가능)
pylon mem delete --project <name> --key "키"            # 삭제 (--category, --dry-run 지원)
pylon mem prune --category <category>                 # 오염 항목 일괄 삭제 (--project 생략 시 전체, --dry-run 지원)
```

### 동기화

| 명령어 | 설명 |
|--------|------|
| `pylon sync-agents` | 내장 리소스(`.pylon/builtin/`)와 `.claude/agents/` 심링크를 즉시 동기화 |
| `pylon mem inject` | 세션 시작 시 호출(Claude Code 훅; Codex는 수동 `pylon mem inject`) — 구버전 Stop 훅이 저장한 훅 JSON 항목과 만료 메모리(`retention_days`)를 정리한 뒤 프로젝트 메모리를 세션 컨텍스트에 주입 |

### 작업 이력

Pylon은 `.pylon/history/pipelines/<pipeline-id>/<phase>/` 디렉토리 스냅샷에 선별된 작업 이력을
기록합니다. 파이프라인마다 planned, executed, completed/cancelled/failed 체크포인트를 생성할 수 있습니다.

terminal snapshot은 criteria, deterministic verification, evaluator verdict, task reports, failure records와
rejected hypotheses의 요약을 함께 보존합니다. 대용량 원본 로그는 복사하지 않고 task/failure report의 안정적인
상대 경로와 digest로 참조합니다.

```bash
pylon history log --pipeline <pipeline-id>
pylon history show <pipeline-id>/<phase>
pylon history diff <from-ref> <to-ref>
pylon history export <pipeline-id>/<phase> --output <new-directory>
pylon history validate <pipeline-id>/<phase>       # 체크포인트 artifact 무결성 확인
pylon history checkpoint --pipeline <id> --phase <planned|executed|completed|cancelled|failed>
```

### 파이프라인 내부 명령

`/pl:*` 커맨드가 검증 게이트를 기록할 때 호출하는 명령입니다 (`pylon internal --help`):

| 명령어 | 설명 |
|--------|------|
| `pylon internal criteria snapshot` | `verify.yml` + acceptance criteria를 `criteria.json`으로 동결하고 digest를 manifest에 결합 |
| `pylon internal verify` | snapshot 명령으로 deterministic 검증 실행 → `verification.json` (깨끗한 HEAD면 `tree`, 산출물은 `verify-artifacts/`로 이동). `--workdir`와 `--config`만 주면 파이프라인 없이 실행하고 결과를 표준 출력으로 냄 (`ok`, 실패 시 종료 코드 1) |
| `pylon internal move-untracked` | 지정한 run 잔여물(추적 안 된 파일)을 경로째 run 폴더(`.pylon/runtime/` 아래)로 이동 |
| `pylon internal evaluator prepare\|record` | 격리 evaluator bundle 생성 / 판정 검증·기록 |
| `pylon internal trajectory task-report\|failure` | task report / failure record 기록 (failure는 `failed` 체크포인트까지) |

`mem`(`inject` 제외), `history`, `delete-project`는 `--json` 플래그를 추가하면 JSON 형식으로 출력합니다. 다른 명령은 이 플래그를 받아도 출력이 바뀌지 않습니다.

## 워크스페이스 구조

```
workspace/
├── .pylon/                    # 소스 오브 트루스 (git 추적)
│   ├── config.yml             # 워크스페이스 설정
│   ├── builtin/               # 내장 리소스 (git 무시, 실행·doctor마다 파일 단위로 임베드와 동기화)
│   │   ├── agents/            # 에이전트 정의 (14종)
│   │   ├── commands/          # 파이프라인 슬래시 커맨드 (pl-pipeline.md, ...)
│   │   ├── scripts/bash/      # 파이프라인 셸 스크립트 (common.sh, init-pipeline.sh, ...)
│   │   └── reference/         # pylon 사용 매뉴얼 (pylon-usage.md)
│   ├── agents/                # 사용자 추가 에이전트 (pylon add-agent)
│   ├── skills/                # 사용자 추가 스킬 (pylon add-skill)
│   ├── commands/              # 사용자 추가 파이프라인 커맨드
│   ├── domain/                # 팀 도메인 지식 (위키)
│   ├── memory/                # 프로젝트 메모리 (git 추적)
│   │   └── {project}/          # 프로젝트별 <category>/<slug>.md + INDEX.md
│   ├── runtime/               # (git 무시)
│   │   └── {pipeline-id}/     # 논리 root pipeline 산출물 (requirement.md, tasks.json, execution-log.json, status.json ...)
│   │       └── repos/{repo-id}/  # repo sub-pipeline: criteria.json, verification.json, task-report.json,
│   │                             #   evaluator-input/, evaluator-result.json, pr.json, status.json,
│   │                             #   verify-artifacts/(검증 산출물), leftovers/(커밋 안 한 run 잔여물)
│   └── history/               # 파일 기반 작업 이력 (git 무시)
│       └── pipelines/         # {pipeline-id}/{phase}/ 디렉토리 스냅샷
│
├── .claude/                   # 동적 생성 (git 무시)
│   ├── agents/                # .pylon/builtin/agents/, .pylon/agents/ 심링크
│   ├── commands/              # 슬래시 커맨드 (내장 + .pylon/commands/에서 생성)
│   └── settings.json          # Claude Code hooks 설정
│
├── .agents/skills/pl-*/       # Codex용 $pl-* 스킬 (동적 생성, git 무시)
│
├── AGENTS.md                  # 운영 가이드 (pylon 블록은 재생성, 블록 밖은 사용자 소유)
├── CLAUDE.md                  # @AGENTS.md import 마커 (동적 생성)
│
├── project-a/                 # 프로젝트 디렉토리
│   └── .pylon/                # 프로젝트 repo의 git exclude 대상 (커밋되지 않음)
│       ├── context.md         # 프로젝트 컨텍스트
│       └── verify.yml         # 프로젝트 검증 명령 (add-project·doctor가 생성)
└── project-b/                 # 프로젝트 디렉토리
```

## 설정

`.pylon/config.yml`:

```yaml
version: "0.2"

runtime:
  provider: auto               # provider 이름 또는 auto
  permission_mode: acceptEdits # default | acceptEdits | bypassPermissions
  # effort: high               # 선택, 기본값 없음 — provider별 허용값 (claude: low|medium|high|xhigh|max, codex: model_reasoning_effort 값)
  # model: opus                # 선택, 기본값 없음 — provider별 모델 이름
  # env: { KEY: value }        # 선택, 기본값 없음 — provider 프로세스에 적용할 환경 변수

providers:
  claude-code:
    enabled: auto
    command: claude
  codex:
    enabled: auto
    command: codex

git:
  branch_prefix: task          # 작업 브랜치 접두사 — 사용자 명명 규칙이 없을 때만 씀 (feature/처럼 /로 끝나면 구분자 없이 이어짐)
  default_base: main           # /pl:cleanup이 삭제하지 않는 기본 브랜치 (파이프라인 base는 초기화 시점의 현재 브랜치)
  pr:
    auto_pr: false             # true면 /pl:pipeline이 검증 후 PR까지 생성 (기본: /pl:pr로 수동)

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

작업 브랜치 이름은 `/pl:pipeline`이 repo의 `CLAUDE.md`·`AGENTS.md`·`CONTRIBUTING.md`, 그다음 워크스페이스 `AGENTS.md`와
사용자 지침에서 찾은 명명 규칙을 먼저 따릅니다. 규칙이 없으면 `git.branch_prefix`로 `<접두사>-<pipeline-id>-<repo-id>`를
만듭니다(`feature/`처럼 `/`로 끝나면 `<접두사><pipeline-id>-<repo-id>`, 기본은 `task-<pipeline-id>-<repo-id>`).
접두사를 읽으려면 `yq` 또는 PyYAML이 필요합니다 ([요구 사항](#요구-사항) 참고).

`runtime`의 `effort`/`model`/`env`(비어 있으면 provider 기본값)를 제외한 모든 설정에는 기본값이 있으므로 `version` 필드만 필수입니다. 위 예시는 주요 필드와 기본값을 보여주며,
명시하지 않은 필드는 로드 시 코드 기본값으로 자동 채워집니다. `pylon init`이 처음 생성하는 `config.yml`은
최소 필드(`runtime`의 `provider`/`permission_mode`)만 포함하고, 이후
`pylon doctor`가 누락된 필드를 감지해 기본값으로 동기화합니다.

## 개발

아키텍처·규약 등 개발 가이드는 [AGENTS.md](AGENTS.md)에 있습니다 (`CLAUDE.md`는 이를 import).

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
| AI 백엔드 | Claude Code CLI, Codex CLI (provider adapter) |
| 저장소 | 마크다운/JSON 파일 (.pylon/memory, .pylon/history) |
| 메모리 검색 | 토큰 매칭 + INDEX.md 주입 |
| 작업 이력 | 디렉토리 스냅샷 (.pylon/history/pipelines) |

## 라이선스

MIT
