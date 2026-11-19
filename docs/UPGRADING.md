# 이전 버전에서 넘어올 때

2026.11.13에서 파이프라인, 훅, 다수의 에이전트가 없어졌습니다. 그보다 이전 버전을 쓰던 워크스페이스에 해당하는 내용입니다.

- `pylon update`를 실행한 워크스페이스는 바로 정리됩니다. 다른 워크스페이스는 `pylon doctor`를 실행하거나, 옛 훅이 남아 있으면 다음 Claude Code 세션이 시작될 때 자동으로 정리됩니다.
- 이 정리에서 구버전 Stop 훅이 메모리에 저장한 훅 JSON 항목도 지웁니다. 되돌릴 수 없으니 워크스페이스가 git 저장소가 아니면 필요할 때 `.pylon/memory`를 먼저 복사해 두세요.
- 없어지는 것: `/pl:pipeline`과 단계 커맨드, 파이프라인 스크립트, `pylon` 단독 실행, `SessionStart` 훅, 11개 에이전트, Codex `$pl-*` 스킬.
- 남는 것: `.pylon/runtime/`, `.pylon/history/`, `.pylon/skills/`, `*.pylon-bak` 파일, 프로젝트의 `.pylon/verify.yml`. 더 이상 읽지 않습니다. `pylon doctor`가 목록을 보여 주고, `pylon doctor --prune-legacy`가 삭제합니다. `config.yml`의 옛 키는 그대로 두어도 됩니다.
- 진행 중인 파이프라인 run은 업데이트 전에 마감하세요. 끝나지 않은 run의 브랜치와 워크트리는 자동으로 정리되지 않습니다.

## 내장 에이전트 제거

2026.11.17보다 뒤의 버전에는 내장 에이전트(`dev`, `code-reviewer`, `critic`)가 없습니다. 업데이트 뒤 `pylon doctor`가 `.pylon/builtin/agents/`와 그 `.claude/agents/` 링크를 지웁니다.

- PC에 설치된 에이전트와 플러그인이 있으면 세션이 그것을 먼저 씁니다.
- 없으면 도구의 공식 기본 기능을 씁니다. Claude Code는 기본 에이전트(`general-purpose`, `Explore`, `Plan`)와 `/code-review`, Codex는 `codex review`입니다.
- 이전 에이전트를 계속 쓰려면 업데이트 전에 `.pylon/builtin/agents/`의 파일을 `.pylon/agents/`로 복사해 두세요. 사용자 에이전트는 그대로 연결됩니다.
