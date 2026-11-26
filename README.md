# Pylon

**여러 저장소를 하나의 워크스페이스로 묶어 Claude Code·Codex에 필요한 지침과 메모리를 준비하는 개인용 CLI입니다.**

- **잘 맞는 경우:** 한 디렉터리 아래에서 여러 저장소를 다루는 경우
- **맞지 않는 경우:** 저장소 하나만 쓰거나, 작업 실행까지 자동화하는 도구가 필요한 경우
- **현재 상태:** 제작자 개인 용도로 개발합니다. 호환성을 보장하지 않습니다.

Pylon이 하는 일은 파일을 준비하는 것까지입니다. 세션은 Claude Code, Codex, [Paseo](https://paseo.sh)에서 직접 시작합니다. 자동으로 주입되는 지식은 없고, 세션이 `AGENTS.md`의 안내를 보고 필요할 때 찾습니다. Paseo는 여러 코딩 에이전트를 동시에 실행하고 관리하는 도구입니다.

## 왜 필요한가

저장소 하나에서 시작한 세션은 그 저장소만 봅니다. 여러 저장소에 걸친 작업에서는 같은 일이 반복됩니다.

- 세션마다 저장소 구성과 관계를 다시 설명합니다.
- 저장소마다 복사해 둔 지침과 에이전트 설정이 서로 어긋납니다.
- 지난 세션에서 내린 결정과 겪은 함정이 다음 세션에 남지 않습니다.

Pylon은 저장소들을 담은 상위 디렉터리를 워크스페이스로 만듭니다. 세션을 거기서 시작하면 모든 저장소가 작업 디렉터리 안에 있고, 루트 `AGENTS.md`가 무엇을 어디서 찾을지 알려 줍니다. 프로젝트 두 개를 추가한 뒤의 `AGENTS.md`는 이렇습니다(발췌).

```markdown
<!-- pylon:begin -->
# pylon 워크스페이스 운영 매뉴얼
(중략)
## 필요할 때 찾기

- 프로젝트: 이 블록 아래의 프로젝트 목록. 각 프로젝트는 워크스페이스 바로 아래의 저장소입니다.
- 프로젝트 지식: `<project>/.pylon/context.md`, 그 저장소의 `README`, `AGENTS.md`, `docs/`.
- 도메인 지식: `.pylon/domain/`의 문서. 파일 구성은 워크스페이스마다 다릅니다.
- 메모리: `pylon mem search --project <name> --query "검색어"`. 목록은 `.pylon/memory/<project>/INDEX.md`.
(중략)
## 이 워크스페이스

- **프로젝트 2개**: my-api, my-app
<!-- pylon:end -->
```

매뉴얼에는 이 밖에 메모리에 무엇을 남길지, 언제 서브 에이전트에 위임할지, 완료를 주장하기 전에 무엇을 검증할지가 들어 있습니다. 프로젝트를 바꾸는 작업 전에 메모리를 검색하고, 작업을 마치면 `/pl:cleanup`으로 결정과 함정을 기록하도록 안내합니다.

같은 문제를 다루는 글로 [Home Repo 패턴](https://www.aihero.dev/the-home-repo-pattern)이 있습니다. Pylon은 저장소들을 워크스페이스 디렉터리 안에 두고, 프로젝트 목록과 매뉴얼 갱신을 CLI가 맡는다는 점이 다릅니다.

## 요구 사항

| 도구 | 용도 | 설치 |
|------|------|------|
| **git** | 프로젝트 clone과 제외 설정 | 기본 설치 |
| **gh** | `init`·`doctor`의 필수 검사, `/pl:cleanup`의 PR 머지 확인 | `brew install gh` |
| **Go** 1.24+ | `go install`·소스 빌드 시에만 | https://go.dev/dl/ |

git과 gh가 없으면 `pylon init`이 실패합니다. `pylon doctor`로 설치 상태를 확인할 수 있습니다.

## 설치

macOS와 Linux에서는 아래 명령으로 최신 버전을 설치합니다. `gh auth login`이 되어 있어야 합니다.

```bash
gh release download --repo kyago/pylon --output - \
  --pattern "*_$(uname -s | tr '[:upper:]' '[:lower:]')_$(uname -m | sed 's/x86_64/amd64/; s/aarch64/arm64/').tar.gz" \
  | tar -xzf - pylon
sudo mv pylon /usr/local/bin/    # 또는 PATH에 포함된 디렉터리
pylon version
```

Windows는 [GitHub Releases](https://github.com/kyago/pylon/releases)에서 zip을 직접 받습니다. Go가 있으면 `go install github.com/kyago/pylon/cmd/pylon@latest`로도 설치할 수 있습니다. 이 경우 `main` 최신 커밋을 빌드하며 `pylon version`은 `dev`를 표시합니다.

버전은 [CalVer](https://calver.org/)(`YYYY.M.SEQ`)이고, `main`에 머지되면 CI가 자동으로 릴리스합니다.

## 업데이트

```bash
pylon update              # 최신 버전 설치
pylon update x.x.x        # 특정 버전 설치
```

바이너리를 교체한 뒤 실행한 위치의 워크스페이스에 `pylon doctor`를 실행합니다. 다른 워크스페이스는 각각 `pylon doctor`를 실행하세요. 릴리스 조회에는 `GITHUB_TOKEN`, 없으면 `gh auth token` 값을 사용합니다. Windows는 자동 교체를 지원하지 않으므로 Releases에서 직접 받으세요.

2026.11.13보다 이전 버전에서 올라오는 경우 [이전 버전에서 넘어올 때](docs/UPGRADING.md)를 먼저 읽으세요.

## 빠른 시작

```bash
mkdir my-workspace && cd my-workspace
pylon init
pylon add-project https://github.com/user/my-app.git
pylon add-project https://github.com/user/my-api.git
```

이제 이 디렉터리에서 Claude Code, Codex, Paseo 세션을 시작하면 됩니다. `add-project`와 `delete-project`는 `AGENTS.md`의 프로젝트 목록도 함께 갱신합니다.

작업을 마치면 Claude Code에서 `/pl:cleanup`을 실행합니다. 결과 확인, 메모리와 도메인 지식 갱신, 머지된 브랜치와 워크트리 정리를 맡습니다. 다른 도구에서는 `.pylon/builtin/commands/pl-cleanup.md`를 읽고 수행하게 합니다.

Codex를 쓸 때는 다음을 지킵니다.

- 세션은 워크스페이스 루트에서 엽니다. 프로젝트 폴더에서 열면 pylon 매뉴얼이 로드되지 않고, 기본 샌드박스가 루트의 `.pylon/memory/` 쓰기를 막아 `pylon mem store`가 실패합니다.
- 기본 샌드박스는 네트워크를 막습니다. `pl-cleanup`의 `git fetch`, `gh`, 원격 브랜치 삭제는 승인하거나 네트워크를 허용해야 동작합니다.
- 루트가 git 저장소가 아니면 `codex exec`에 `--skip-git-repo-check`가 필요합니다.

## 명령어

| 명령어 | 설명 |
|--------|------|
| `pylon init` | 워크스페이스 초기화 |
| `pylon add-project <git-url>` | 프로젝트를 clone하여 추가 (`--name`, `--force`로 재clone, `--skip-clone`으로 기존 디렉터리에 `.pylon/`만 생성) |
| `pylon delete-project <name>` | 프로젝트 등록 해제 (`--purge`로 디렉터리까지 삭제, `--force`/`-f`로 확인 생략) |
| `pylon project-list` | 워크스페이스의 프로젝트 이름과 경로 출력 |
| `pylon doctor` | 도구 확인, 내장 리소스와 생성 파일 갱신 (`--fix-excludes`, `--yes`로 비인터랙티브, `--prune-legacy`로 이전 버전 잔여 파일 삭제) |
| `pylon mem` | 프로젝트 메모리 (`store`, `search`, `list`, `delete`, `prune`) |
| `pylon update [version]` | 바이너리 업데이트 후 `doctor` 실행 |
| `pylon uninstall` | 워크스페이스 제거 (`--dry-run`, `--force`/`-f`, `--remove-projects`, `--remove-binary`) |
| `pylon version` | 버전 정보 |

`mem`, `delete-project`, `project-list`는 `--json`을 붙이면 JSON으로 출력합니다.

슬래시 커맨드는 `/pl:cleanup`과 `/pl:add-project` 두 개입니다.

## 메모리

메모리는 `.pylon/memory/<project>/<category>/<slug>.md`에 저장되는 마크다운 파일이고, 프로젝트별 `INDEX.md`가 목록입니다. 세션에 자동으로 주입되지 않으며, 필요할 때 찾습니다.

```bash
pylon mem search --project <name> --query "검색어"            # --limit (기본 10)
pylon mem store --project <name> --category decision --key "슬러그" --content "한 줄 요약
근거와 맥락"                                                  # 같은 category와 key면 갱신
pylon mem list --project <name>                             # --category로 필터
pylon mem delete --project <name> --key "슬러그"              # --category, --dry-run, --yes 지원
pylon mem prune --category <category>                       # --project 생략 시 전체, --dry-run, --yes 지원
```

## 에이전트와 코드 리뷰

Pylon은 에이전트를 내장하지 않습니다. 매뉴얼은 세션이 아래 순서로 고르도록 안내합니다.

1. PC에 설치된 에이전트와 플러그인을 먼저 씁니다. 사용자가 추가한 에이전트, 설치한 플러그인, 워크스페이스의 `.pylon/agents/`가 해당합니다.
2. 맞는 것이 없으면 도구의 공식 기본 기능을 씁니다. Claude Code는 기본 에이전트와 `/code-review`, Codex는 `codex review`입니다.

리뷰 전용 에이전트를 더하려면 Claude Code 공식 플러그인(`pr-review-toolkit`, `feature-dev`)을 `/plugin`에서 설치합니다.

직접 만든 에이전트는 `.pylon/agents/`에 두면 `pylon doctor`가 `.claude/agents/`에 연결합니다.

직접 만든 스킬은 `.pylon/skills/<이름>/SKILL.md`로 두면 `pylon doctor`가 `.claude/skills/<이름>`에 연결합니다.

## 워크스페이스 구조

```
workspace/
├── .pylon/
│   ├── config.yml             # 워크스페이스 표시
│   ├── builtin/               # 내장 리소스 (doctor가 임베드와 동기화)
│   │   ├── commands/          # pl-cleanup, pl-add-project
│   │   └── reference/         # 운영 매뉴얼 (pylon-usage.md)
│   ├── agents/                # 사용자 에이전트 (직접 추가)
│   ├── skills/                # 사용자 스킬 (<이름>/SKILL.md, 직접 추가)
│   ├── commands/              # 사용자 커맨드 (직접 추가)
│   ├── domain/                # 도메인 지식 (overview, practices, glossary)
│   └── memory/                # 프로젝트 메모리
│
├── .claude/                   # 생성 파일
│   ├── agents/                # 사용자 에이전트 심링크
│   ├── skills/                # 사용자 스킬 심링크
│   └── commands/pl/           # 슬래시 커맨드
│
├── AGENTS.md                  # pylon 블록은 doctor가 재생성, 블록 밖은 사용자 소유
├── CLAUDE.md                  # @AGENTS.md import 마커
│
├── project-a/                 # 프로젝트 (독립 git 저장소)
│   └── .pylon/context.md      # 프로젝트 컨텍스트 (git exclude, 커밋되지 않음)
└── project-b/
```

`AGENTS.md`에서 pylon이 관리하는 부분은 `<!-- pylon:begin -->`과 `<!-- pylon:end -->` 사이의 블록뿐입니다. 워크스페이스 규칙은 블록 밖에 적습니다. 블록 안을 고치면 다음 `pylon doctor` 때 되돌아갑니다. Codex는 이 파일을 바로 읽습니다.

워크스페이스 루트가 git 저장소 안에 있으면 pylon이 `.claude/`, `.pylon/builtin/`, `CLAUDE.md`, `AGENTS.md`를 `.gitignore`에 추가합니다. 그렇지 않으면 `.gitignore`를 만들지 않습니다.

## 개발

개발 가이드는 [AGENTS.md](AGENTS.md)에 있습니다.

```bash
make build       # 빌드 (bin/pylon)
make install     # $(go env GOPATH)/bin에 설치
make test        # 테스트
make lint        # 린트
make clean       # 정리
```
