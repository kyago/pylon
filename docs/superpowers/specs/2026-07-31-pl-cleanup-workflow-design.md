# /pl:cleanup 워크플로우 설계

## 배경 · 문제

`/pl:pipeline`은 요구사항이 **명확할 때** 처음부터 전체 워크플로우를 태우는 startup-workflow다.
파이프라인은 종료 시 `pylon history checkpoint`로 산출물과 큐레이션 메모리를
`.pylon/history/pipelines/`에 남긴다.

그러나 실제 사용에서 **요구사항이 모호한 경우**에는 파이프라인을 쓰지 않고, AI가 먼저 탐색한 뒤
대화로 방향을 도출하고 작업을 착수하는 비파이프라인 경로를 탄다. 이 경로에는 종료 시
기록을 남기고 정리하는 대칭 단계가 없다. 파이프라인 런타임 디렉토리가 없으므로
`history checkpoint`는 애초에 남길 수 없고(`RuntimeDir/<id>` 부재), Stop 훅의
`sync-memory --from-session`은 학습 내용이 stdin으로 주어질 때만 저장한다.

→ **비파이프라인 세션을 마무리할 때 호출하는 cleanup 워크플로우**를 추가한다.

## 설계 원칙 — 왜 메모리에만 남기고 history엔 안 남기는가

`.pylon/history/`는 **자동으로 참조되지 않는다**. 오직 사람이 `pylon history log/show/diff/export`로
수동 열람하는 감사 기록일 뿐, 다음 세션에 주입되는 경로가 없다(코드 확인:
`internal/cli/commands`·`launch*.go`에서 history를 읽어 프롬프트에 넣는 곳 없음).

반대로 다음 작업에 되먹임되는 것은 **메모리(`.pylon/memory/`)**다:
- 런치마다 `launch_claudemd.go`가 메모리 인덱스를 root CLAUDE.md에 주입
- 파이프라인 시작 시 `pylon mem search`로 검색

따라서 cleanup은 **살아있는 메모리에만** 남기고 `.pylon/history/`·세션 요약 파일·`verify.json`은
일절 생성하지 않는다. 남기는 모든 것이 실제로 다음 세션에 참조된다.

## 명령 사양

```
/pl:cleanup
```

구현은 임베드 슬래시 명령 파일 **하나** — `internal/cli/commands/pl-cleanup.md`.
`//go:embed commands/*.md`가 자동 임베드하고 `buildDesiredClaudeCommands`가
`pl-cleanup.md → pl/cleanup`으로 자동 매핑하므로 **Go 코드 변경은 없다**. 등록할 소비자 목록도 없다.

명령 본문은 Claude Code TUI가 따르는 지시서다(레포의 "오케스트레이션은 Claude가" 철학과 동일).
인자는 받지 않는다(현재 대화 세션 자체가 입력).

## 동작 흐름 (pl-cleanup.md가 지시하는 단계)

### Step 1: 대상 프로젝트 결정
`pylon mem store`는 `--project`가 필요하다. 프로젝트 해석 규칙:
1. 워크스페이스에 프로젝트가 1개면 그것으로 자동 확정(`resolveProject`의 단일-프로젝트 동작과 일치).
2. 멀티레포면 `git status`로 **uncommitted 변경이 있는 repo**를 대상으로 추론한다.
3. 변경이 여러 repo에 걸쳐 모호하거나 하나도 없으면 **사용자에게 확인**한다.
4. 최후 폴백은 워크스페이스 basename(단일-프로젝트로 취급).

### Step 2: 세션 핵심을 메모리로 저장
Claude가 이번 세션에서 도출된 **결정·학습·재사용 패턴**을 합성해 항목별로 저장한다.
카테고리는 `pylon mem store --category`로 지정한다(기본 general이 아니라 명시):

- 결정(왜 이 방향으로 갔는가) → `--category decision`
- 학습/교훈 → `--category learning`
- 재사용 패턴 → `--category pattern`

```bash
pylon mem store --project <p> --category decision --key <slug> --content "..."
pylon mem store --project <p> --category learning --key <slug> --content "..."
```

이 세 카테고리는 history가 큐레이션하는 `curatedMemoryCategories`(learning/decision/architecture/pattern)와
일치하며, `mem search`·CLAUDE.md 주입에 잡히는 검색 대상이다.

**저장 위생:** 항목은 재사용 가치가 있는 것만 간결하게. 변경 파일 목록 같은 raw 이력은
검색 오염(issue #76)을 유발하므로 저장하지 않는다.

### Step 3: 검증
```bash
pylon internal verify --workdir <git-root> --config <git-root>/.pylon/verify.yml
```
결과는 완료 보고에 **인라인 출력**하고 파일로 저장하지 않는다.
`verify.yml`이 없거나 검증이 미설정이면 우아하게 스킵하고 "검증 미설정"으로 보고한다
(cleanup 전체를 실패시키지 않는다).

### Step 4: 정리(tidy)
- 만료 메모리 prune은 **별도로 호출하지 않는다.** Stop 훅(`sync-memory --from-session`)이
  cleanup 완료 턴 직후를 포함해 매 턴 `PruneExpired`를 수행하므로 위임한다.
  (cleanup은 Step 2에서 `mem store`로 저장만 하고, prune은 훅에 맡긴다.)
- git에 uncommitted 변경이 있으면 커밋을 안내한다(자동 커밋하지 않음).

### Step 5: 완료 보고
- 저장한 메모리 항목 목록(카테고리·key)
- 검증 결과 요약(또는 "검증 미설정")
- 커밋 안내 여부

## 남기지 않는 것 (명시적 non-goals)
- `.pylon/history/`에 어떤 스냅샷도 남기지 않는다(파이프라인 전용 유지).
- 세션 요약 마크다운 파일을 별도로 만들지 않는다(요약은 메모리 항목으로 흡수).
- `verify.json`을 저장하지 않는다(검증 결과는 휘발성 보고).
- startup 대칭 명령은 이번 범위에서 제외(추후 별도 스펙).

## 구현 범위
- **추가 파일 1개**: `internal/cli/commands/pl-cleanup.md`
- **Go 변경 없음.** 임베드·등록·doctor 동기화는 기존 메커니즘이 자동 처리.
- 기존 CLI만 사용: `pylon mem store`, `pylon sync-memory`, `pylon internal verify`, `git status`.

## 테스트 / 검증
- `pl-cleanup.md`가 임베드되고 `pylon doctor`/launch 후 `.claude/commands/pl/cleanup.md`로
  생성되는지 확인(기존 command 동기화 테스트 패턴 재사용 가능).
- 명령이 참조하는 CLI 플래그(`mem store --category/--key/--content`,
  `internal verify --config`)가 실제로 존재하는지 확인(본 스펙 작성 시 확인 완료).
- 실제 검증: 단일-프로젝트 워크스페이스에서 `/pl:cleanup`을 돌려 메모리 항목이
  `.pylon/memory/<project>/<category>/`에 생성되고 다음 `mem search`에 잡히는지 확인.
