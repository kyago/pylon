# /pl:cleanup 워크플로우 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 파이프라인 없이 진행한 세션을 마무리할 때 세션 결정·학습을 프로젝트 메모리에 저장하고 검증·정리하는 `/pl:cleanup` 슬래시 명령을 추가한다.

**Architecture:** 임베드 슬래시 명령 파일 하나(`internal/cli/commands/pl-cleanup.md`)만 추가한다. `//go:embed commands/*.md`가 자동 임베드하고 `buildDesiredClaudeCommands`가 `pl-cleanup.md → pl/cleanup`으로 자동 매핑하므로 Go 코드 변경은 없다. 명령 본문은 Claude Code TUI가 따르는 지시서이며, 기존 CLI(`pylon mem store`, `pylon internal verify`, `git status`)만 호출한다.

**Tech Stack:** Go 1.24+ (embed.FS), Cobra, 마크다운 슬래시 명령.

## Global Constraints

- 사용자 대면 문자열은 **한국어**. 코드 식별자/주석은 주변 파일에 맞춘다.
- **Go 코드 변경 금지** — 명령 파일 추가만으로 완성되어야 한다(임베드·등록·doctor 동기화는 기존 메커니즘이 처리).
- `.pylon/history/`·세션 요약 파일·`verify.json`을 **생성하지 않는다**. 기록은 `.pylon/memory/`에만.
- 저장 카테고리는 `decision` / `learning` / `pattern`만 사용(history가 큐레이션하는 카테고리와 일치).
- `make test`는 `go test ./... -race -count=1`.
- `pylon mem store` 필수 플래그: `--project`, `--key`, `--content` (+ `--category`, 기본 general).

---

### Task 1: pl-cleanup.md 명령 파일 추가 + 임베드·생성 테스트

**Files:**
- Create: `internal/cli/commands/pl-cleanup.md`
- Test: `internal/cli/launch_commands_test.go` (테스트 함수 1개 추가)

**Interfaces:**
- Consumes: `embeddedCommands embed.FS` (`launch_commands.go:16`), `generateClaudeDir(root, cfg, nil)` (`launch.go`), `config.Config` (`internal/config`).
- Produces: 임베드 리소스 `commands/pl-cleanup.md`, 생성 산출물 `.claude/commands/pl/cleanup.md`. 새 Go 심볼 없음.

- [ ] **Step 1: 실패하는 테스트 작성**

`internal/cli/launch_commands_test.go` 끝에 아래 함수를 추가한다. 파일 상단 import는 이미 `os`, `path/filepath`, `testing`, `config`를 포함하므로 수정 불필요.

```go
// pl-cleanup 명령이 임베드되고 같은 실행에서 .claude/commands/pl/cleanup.md로 생성되어야 한다.
func TestGenerateClaudeDir_InstallsCleanupCommand(t *testing.T) {
	root := t.TempDir()

	if err := generateClaudeDir(root, &config.Config{}, nil); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(filepath.Join(root, ".claude", "commands", "pl", "cleanup.md"))
	if err != nil {
		t.Fatalf("pl:cleanup 명령이 생성되지 않았다: %v", err)
	}
	want, err := embeddedCommands.ReadFile("commands/pl-cleanup.md")
	if err != nil {
		t.Fatalf("pl-cleanup.md가 임베드되지 않았다: %v", err)
	}
	if string(got) != string(want) {
		t.Fatal("생성된 cleanup 명령이 임베드 원본과 일치하지 않는다")
	}
}
```

- [ ] **Step 2: 테스트 실패 확인**

Run: `go test ./internal/cli/ -run TestGenerateClaudeDir_InstallsCleanupCommand -count=1`
Expected: FAIL — `pl-cleanup.md가 임베드되지 않았다` (파일이 아직 없어 `embeddedCommands.ReadFile`이 에러).

- [ ] **Step 3: pl-cleanup.md 작성**

아래를 시작점으로 `internal/cli/commands/pl-cleanup.md`에 작성한다. (참고: 구현 중
코드리뷰 반영으로 Step 1의 프로젝트-이름 정합, Step 3의 verify Go 폴백/`skipped` 해석,
Step 4의 prune 문구가 갱신되었다 — **실제 파일이 source of truth**다.)

````markdown
---
description: "비파이프라인 세션 마무리 — 세션 결정·학습을 메모리에 저장하고 검증·정리"
---

# Pylon Cleanup — 비파이프라인 세션 마무리

파이프라인을 쓰지 않고 탐색·대화로 진행한 작업을 마무리합니다.
세션에서 도출된 결정·학습을 프로젝트 메모리에 저장하고, 검증을 실행한 뒤,
남은 정리를 안내합니다. `.pylon/history/`에는 아무것도 남기지 않습니다 —
기록은 다음 세션에 자동 참조되는 `.pylon/memory/`에만 남깁니다.

## Step 1: 대상 프로젝트 결정

메모리 저장에는 프로젝트 이름이 필요합니다. 다음 순서로 결정합니다:

1. 워크스페이스에 프로젝트가 1개면 그것으로 확정합니다.
2. 여러 프로젝트(멀티레포)면 `git status`로 uncommitted 변경이 있는 repo를 대상으로 추론합니다.
3. 변경이 여러 repo에 걸쳐 모호하거나 하나도 없으면 사용자에게 대상 프로젝트를 확인합니다.
4. 판단이 어려우면 워크스페이스 이름을 기본값으로 씁니다.

## Step 2: 세션 핵심을 메모리에 저장

이번 세션에서 도출된, 재사용 가치가 있는 항목만 간결하게 추려 카테고리별로 저장합니다:

- 방향을 정한 **결정**과 그 근거 → `decision`
- **학습/교훈**(다음에 알았으면 좋았을 것) → `learning`
- 재사용 가능한 **패턴** → `pattern`

각 항목을 CLI로 저장합니다:

```bash
pylon mem store --project <프로젝트> --category decision --key <슬러그> --content "결정과 근거"
pylon mem store --project <프로젝트> --category learning --key <슬러그> --content "학습 내용"
```

저장 위생: 변경 파일 목록 같은 raw 이력은 검색을 오염시키므로 저장하지 않습니다.
저장할 만한 결정·학습이 없으면 이 단계를 건너뜁니다.

## Step 3: 검증

코드 변경이 있었다면 검증을 실행하고 결과를 인라인으로 보고합니다(파일로 저장하지 않습니다):

```bash
pylon internal verify --workdir <git-root> --config <git-root>/.pylon/verify.yml
```

`verify.yml`이 없거나 검증이 설정되지 않았으면 "검증 미설정"으로 보고하고 넘어갑니다
— cleanup 전체를 실패시키지 않습니다.

## Step 4: 정리

- `git status`에 커밋되지 않은 변경이 있으면 커밋을 안내합니다(자동 커밋하지 않습니다).
- 만료 메모리 정리(prune)는 이 명령의 책임이 아닙니다 — 별도로 호출하지 않습니다.

## Step 5: 완료 보고

다음을 요약합니다:
- 저장한 메모리 항목 (카테고리 · key)
- 검증 결과 (또는 "검증 미설정")
- 커밋 안내 여부
````

- [ ] **Step 4: 테스트 통과 확인**

Run: `go test ./internal/cli/ -run TestGenerateClaudeDir_InstallsCleanupCommand -count=1`
Expected: PASS.

- [ ] **Step 5: 전체 테스트 회귀 확인**

Run: `make test`
Expected: 전부 PASS (임베드 파일 추가가 다른 command 테스트를 깨지 않는지 확인).

- [ ] **Step 6: 커밋**

```bash
git add internal/cli/commands/pl-cleanup.md internal/cli/launch_commands_test.go
git commit -m "feat: /pl:cleanup 비파이프라인 세션 마무리 명령 추가

세션 결정·학습을 프로젝트 메모리(decision/learning/pattern)에 저장하고
검증을 인라인 보고한 뒤 정리를 안내한다. history/요약 파일은 남기지 않는다.
임베드 슬래시 명령 파일 하나로 구현 — Go 코드 변경 없음."
```

---

### Task 2: 실제 워크스페이스에서 동작 검증

Task 1은 명령이 **설치**됨을 보장하지만, 명령 **본문의 동작**(메모리에 항목이 실제로 쌓이는지)은 Go 단위 테스트로 확인할 수 없다. 아래는 빌드한 바이너리로 하는 통합 검증이다.

**Files:** 없음(수동 검증). 산출물이 코드가 아니므로 커밋 없음.

- [ ] **Step 1: 바이너리 빌드**

Run: `make build`
Expected: `bin/pylon` 생성.

- [ ] **Step 2: 스크래치 워크스페이스에서 명령이 참조하는 CLI가 실제로 동작하는지 확인**

`/pl:cleanup`은 Claude Code TUI에서 실행되지만, 본문이 호출하는 CLI 자체는 바이너리로 직접 검증할 수 있다. 임시 워크스페이스에서:

```bash
TMP=$(mktemp -d)
cd "$TMP" && git init -q && bin/pylon >/dev/null 2>&1 || true   # .pylon 부트스트랩
# Step 2가 호출하는 저장 경로 검증
bin/pylon mem store --project "$(basename "$TMP")" --category decision --key test-decision --content "cleanup 스모크 테스트"
bin/pylon mem search --project "$(basename "$TMP")" --query "cleanup 스모크"
```

Expected: `mem store`가 성공하고, `mem search`가 방금 저장한 `test-decision` 항목을 반환한다. `.pylon/memory/<project>/decision/` 아래에 마크다운 파일이 생성되어 있어야 한다.

- [ ] **Step 3: verify 미설정 스킵 확인**

`verify.yml`이 없는 워크스페이스에서 검증 스텝이 cleanup을 실패시키지 않고 우아하게 스킵하는지 확인한다:

```bash
bin/pylon internal verify --workdir "$TMP" --config "$TMP/.pylon/verify.yml"; echo "exit=$?"
```

Expected: 검증 미설정을 알리고 cleanup을 막지 않는 동작(비치명적 종료 또는 skip 메시지). 치명적 에러로 죽지 않아야 한다. 실제 종료 코드/메시지를 확인해 pl-cleanup.md Step 3의 문구("검증 미설정")가 실제 동작과 일치하는지 대조하고, 어긋나면 명령 본문을 실제 동작에 맞춰 수정 후 Task 1 재커밋.

- [ ] **Step 4: (선택) TUI 실사용 검증**

실제 pylon 워크스페이스에서 Claude Code로 `/pl:cleanup`을 호출해, 5단계(프로젝트 결정 → 메모리 저장 → 검증 → 정리 안내 → 완료 보고)가 의도대로 흐르는지 확인한다. 이건 사람이 세션에서 직접 돌려 확인한다.

---

## Self-Review

**1. Spec coverage:**
- 대상 프로젝트 결정(멀티레포 추론/확인) → Task 1 Step 3 본문 Step 1 ✓
- 결정/학습/패턴 카테고리 메모리 저장 → 본문 Step 2 ✓
- 검증 인라인 + verify.yml 미설정 스킵 → 본문 Step 3, Task 2 Step 3 ✓
- prune은 이 명령 책임 아님 + 커밋 안내 → 본문 Step 4 ✓
- history/요약파일/verify.json 미생성(non-goals) → Global Constraints + 본문 도입부 ✓
- Go 변경 없음, 파일 하나 → Architecture + Task 1 ✓

**2. Placeholder scan:** `<프로젝트>`/`<슬러그>`/`<git-root>`는 명령 본문이 런타임에 채우는 자리표시자(플레이스홀더 아님, 지시서의 정상 표기). TBD/TODO 없음.

**3. Type consistency:** 새 Go 심볼 없음. 테스트는 기존 `generateClaudeDir`·`embeddedCommands`만 사용. CLI 플래그(`mem store --project/--key/--content/--category`, `internal verify --workdir/--config`)는 스펙 작성 시 소스로 확인 완료.
