# Pylon

**여러 저장소를 하나의 워크스페이스로 묶어 Claude Code·Codex에 필요한 지침과 메모리를 준비하는 개인용 CLI입니다.**

- **잘 맞는 경우:** 한 디렉터리 아래에서 여러 저장소를 다루는 경우
- **맞지 않는 경우:** 저장소 하나만 쓰거나, 작업 실행까지 자동화하는 도구가 필요한 경우
- **현재 상태:** 제작자 개인 용도로 개발합니다. 호환성을 보장하지 않습니다.

Pylon이 하는 일은 파일을 준비하는 것까지입니다. 세션은 Claude Code, Codex, Paseo에서 직접 시작합니다. 자동으로 주입되는 지식은 없고, 세션이 `AGENTS.md`의 안내를 보고 필요할 때 찾습니다.

## 요구 사항

| 도구 | 용도 | 설치 |
|------|------|------|
| **git** | 프로젝트 clone과 제외 설정 | 기본 설치 |
| **gh** | `/pl:cleanup`의 PR 머지 확인 | `brew install gh` |
| **Go** 1.24+ | `go install`·소스 빌드 시에만 | https://go.dev/dl/ |

`pylon doctor`로 git과 gh 설치 상태를 확인할 수 있습니다.

## 설치

Pylon은 [CalVer](https://calver.org/)(`YYYY.M.SEQ`) 버전 체계를 사용하며, 플랫폼별 바이너리를 [GitHub Releases](https://github.com/kyago/pylon/releases)로 배포합니다. `main`에 머지되면 CI가 자동으로 릴리스합니다.

```bash
# 예시: macOS arm64 (x.x.x는 설치할 버전으로 교체)
curl -L -o pylon.tar.gz \
  https://github.com/kyago/pylon/releases/download/x.x.x/pylon_x.x.x_darwin_arm64.tar.gz
tar -xzf pylon.tar.gz
sudo mv pylon /usr/local/bin/    # 또는 PATH에 포함된 임의 디렉토리로 이동
pylon version
```

Go가 있으면 `go install github.com/kyago/pylon/cmd/pylon@latest`로도 설치할 수 있습니다. 이 경우 `main` 최신 커밋을 빌드하며 `pylon version`은 `dev`를 표시합니다.

## 업데이트

```bash
pylon update              # 최신 버전 설치
pylon update x.x.x        # 특정 버전 설치
```

바이너리를 교체한 뒤 실행한 위치의 워크스페이스에 `pylon doctor`를 실행합니다. 다른 워크스페이스는 각각 `pylon doctor`를 실행하세요. 릴리스 조회에는 `GITHUB_TOKEN`, 없으면 `gh auth token` 값을 사용합니다. Windows는 자동 교체를 지원하지 않으므로 Releases에서 직접 받으세요.

## 빠른 시작

```bash
mkdir my-workspace && cd my-workspace
pylon init
pylon add-project https://github.com/user/my-app.git
pylon add-project https://github.com/user/my-api.git
```

이제 이 디렉터리에서 Claude Code, Codex, Paseo 세션을 시작하면 됩니다. `add-project`와 `delete-project`는 `AGENTS.md`의 프로젝트 목록도 함께 갱신합니다.

작업을 마치면 Claude Code에서 `/pl:cleanup`을 실행합니다. 결과 확인, 메모리와 도메인 지식 갱신, 머지된 브랜치와 워크트리 정리를 맡습니다. 다른 도구에서는 `.pylon/builtin/commands/pl-cleanup.md`를 읽고 수행하게 합니다.

## 명령어

| 명령어 | 설명 |
|--------|------|
| `pylon init` | 워크스페이스 초기화 |
| `pylon add-project <url>` | 프로젝트를 clone하여 추가 (`--name`, `--force`로 재clone, `--skip-clone`으로 기존 디렉터리에 `.pylon/`만 생성) |
| `pylon delete-project <name>` | 프로젝트 등록 해제 (`--purge`로 디렉터리까지 삭제, `--force`로 확인 생략) |
| `pylon doctor` | 도구 확인, 내장 리소스와 생성 파일 갱신 (`--fix-excludes`, `--yes`로 비인터랙티브) |
| `pylon mem` | 프로젝트 메모리 (`store`, `search`, `list`, `delete`, `prune`) |
| `pylon update [version]` | 바이너리 업데이트 후 `doctor` 실행 |
| `pylon uninstall` | 워크스페이스 제거 (`--dry-run`, `--force`, `--remove-projects`, `--remove-binary`) |
| `pylon version` | 버전 정보 |

`mem`과 `delete-project`는 `--json`을 붙이면 JSON으로 출력합니다.

슬래시 커맨드는 `/pl:cleanup`과 `/pl:add-project` 두 개입니다.

## 메모리

메모리는 `.pylon/memory/<project>/<category>/<slug>.md`에 저장되는 마크다운 파일이고, 프로젝트별 `INDEX.md`가 목록입니다. 세션에 자동으로 주입되지 않으며, 필요할 때 찾습니다.

```bash
pylon mem search --project <name> --query "검색어"
pylon mem store --project <name> --category decision --key "슬러그" --content "한 줄 요약
근거와 맥락"                                                  # 같은 category와 key면 갱신
pylon mem list --project <name>
pylon mem delete --project <name> --key "슬러그"              # --category, --dry-run 지원
pylon mem prune --category <category>                       # --project 생략 시 전체, --dry-run 지원
```

## 워크스페이스 구조

```
workspace/
├── .pylon/
│   ├── config.yml             # 워크스페이스 표시
│   ├── builtin/               # 내장 리소스 (git 무시, doctor가 임베드와 동기화)
│   │   ├── agents/            # dev, code-reviewer, critic
│   │   ├── commands/          # pl-cleanup, pl-add-project
│   │   └── reference/         # 운영 매뉴얼 (pylon-usage.md)
│   ├── agents/                # 사용자 에이전트 (직접 추가)
│   ├── commands/              # 사용자 커맨드 (직접 추가)
│   ├── domain/                # 도메인 지식 (overview, practices, glossary)
│   └── memory/                # 프로젝트 메모리
│
├── .claude/                   # 생성 파일 (git 무시)
│   ├── agents/                # 에이전트 심링크
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

## 이전 버전에서 넘어올 때

- `pylon update`를 실행한 워크스페이스는 바로 정리됩니다. 다른 워크스페이스는 `pylon doctor`를 실행하거나, 옛 훅이 남아 있으면 다음 Claude Code 세션이 시작될 때 자동으로 정리됩니다.
- 없어지는 것: `/pl:pipeline`과 단계 커맨드, 파이프라인 스크립트, `pylon` 단독 실행, `SessionStart` 훅, 11개 에이전트, Codex `$pl-*` 스킬.
- 남는 것: `.pylon/runtime/`, `.pylon/history/`, `.pylon/skills/`, `*.pylon-bak` 파일, 프로젝트의 `.pylon/verify.yml`, `config.yml`의 옛 키. 더 이상 읽지 않으니 필요 없으면 직접 지우세요. `.pylon/runtime/`에는 읽기 전용 파일이 있어 `chmod -R u+w .pylon/runtime`을 먼저 실행해야 합니다.
- 진행 중인 파이프라인 run은 업데이트 전에 마감하세요. 끝나지 않은 run의 브랜치와 워크트리는 자동으로 정리되지 않습니다.

## 개발

개발 가이드는 [AGENTS.md](AGENTS.md)에 있습니다.

```bash
make build       # 빌드
make test        # 테스트
make lint        # 린트
make clean       # 정리
```

## 라이선스

MIT
