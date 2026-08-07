# AGENTS.md AI-Authored Root Guide Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** pylon이 루트 시스템 프롬프트를 하드코딩으로 생성하는 대신, 실행된 claude 세션이 첫 턴에 `AGENTS.md`를 저작하도록 전환한다. pylon은 임베디드 매뉴얼(`.pylon/reference/pylon-usage.md`)만 공급하고 `CLAUDE.md`에는 `@AGENTS.md` import 마커만 남긴다.

**Architecture:** Go는 LLM을 호출하지 않는다. `init`/`launch`/`doctor`는 (1) 임베디드 매뉴얼을 `.pylon/reference/`로 배치·갱신하고, (2) `CLAUDE.md`에 결정적 마커 `@AGENTS.md`를 쓰고, (3) `AGENTS.md`가 없거나 매뉴얼 버전보다 낮으면 짧은 부트스트랩 파일을 써서 세션이 재저작하도록 유도한다. 최신 상태면 `AGENTS.md`를 건드리지 않는다.

**Tech Stack:** Go 1.24+ (toolchain go1.26), Cobra, `//go:embed`, 표준 `os`/`filepath`/`strings`/`regexp`. 테스트는 `go test ./... -race -count=1`.

## Global Constraints

- Go 1.24+ / toolchain go1.26. 의존성 추가 금지 (Cobra/huh/yaml.v3 외).
- 사용자 대면 문자열은 한국어. 개발자용 에러 wrap은 영어 허용 — 주변 파일 관례 준수.
- `.pylon/` 경로는 절대 손으로 조립하지 말고 `internal/layout` 헬퍼 사용.
- pylon-owned 리소스 소유권 계약 준수: 임베디드 이름과 같은 파일은 launch/doctor가 임베디드 버전으로 갱신(덮어쓰기), 그 외 파일은 절대 건드리지 않음.
- CLAUDE.md/AGENTS.md는 git-ignore 유지 (전자동 관리, 마커/영역 분리 없음).
- 버전 스탬프 트리거는 매뉴얼 전용 상수 `pylonUsageVersion` — pylon CalVer 릴리스와 독립.
- `make lint`는 gofmt 드리프트에도 실패 — 각 커밋 전 `gofmt`/`golangci-lint fmt` 정리.
- 빌드/테스트: `make build`, `make test` (= `go test ./... -race -count=1`).

---

## File Structure

- **Create** `internal/cli/reference/pylon-usage.md` — 임베디드 매뉴얼(“how to use pylon”). AI가 AGENTS.md 저작 시 읽는 소스.
- **Create** `internal/cli/launch_agentsmd.go` — 부트스트랩/포인터 빌더, stale 판정, `ensureRootAgentFiles`. (기존 `launch_claudemd.go`를 대체하는 신규 파일; 구파일은 Task 3에서 제거.)
- **Modify** `internal/cli/init_cmd.go` — `embeddedReference` embed 변수, `writeReferenceTemplates`, init 흐름에 reference+루트 파일 배치, `reference` 디렉토리, Created 출력.
- **Modify** `internal/cli/doctor.go` — `syncPylonResources`에 reference 편입, stale 체크·재부트스트랩·보고.
- **Modify** `internal/cli/launch.go` — `generateClaudeDir`의 무조건 CLAUDE.md 덮어쓰기 제거 → `ensureRootAgentFiles` 호출, `addClaudeDirToGitignore`에 `AGENTS.md` 추가.
- **Modify** `internal/layout/layout.go` — `ReferenceDir`, `RootClaudePath`, `RootAgentsPath` 헬퍼.
- **Delete (Task 3 내)** `internal/cli/launch_claudemd.go`의 `buildRootCLAUDEMD`/`renderVerificationCommands`/`writeVerifySteps` 및 `launch_claudemd_test.go`.

---

## Task 1: 임베디드 매뉴얼 + reference 동기화 배관

**Files:**
- Create: `internal/cli/reference/pylon-usage.md`
- Modify: `internal/cli/init_cmd.go` (embed 변수 추가, `writeReferenceTemplates`, dirs 목록, init 호출, Created 출력)
- Modify: `internal/cli/doctor.go:375-378` (sync 목록에 reference 추가)
- Modify: `internal/layout/layout.go` (`ReferenceDir`)
- Test: `internal/cli/init_reference_test.go`, `internal/cli/doctor_test.go` (기존 파일에 케이스 추가 가능)

**Interfaces:**
- Produces: `embeddedReference embed.FS`, `writeReferenceTemplates(pylonDir string) error`, `layout.ReferenceDir(root string) string`.
- Consumes: 기존 `syncEmbeddedDir`, `syncPylonResources`.

- [ ] **Step 1: 매뉴얼 파일 작성**

Create `internal/cli/reference/pylon-usage.md` with this exact content:

```markdown
<!-- pylon-usage-version: 1 -->
# pylon 워크스페이스 운영 매뉴얼

이 문서는 pylon 워크스페이스를 운영하는 루트 에이전트를 위한 참조 매뉴얼입니다.
당신(실행된 세션)은 이 매뉴얼과 실제 리포지토리·`.pylon/`을 읽고, 워크스페이스 루트의
`AGENTS.md`를 **이 워크스페이스에 맞춘 운영 가이드**로 작성/갱신합니다.

## 정체성과 책임

루트 에이전트는 요구사항을 분석하고, **직접 수행할지 위임할지 판단하며**, 최종 산출물의
정확성에 책임을 집니다. 위임 여부와 무관하게 품질 책임은 루트에 남습니다 — 서브 에이전트의
"완료했습니다"를 검증 없이 사용자에게 전달하지 않습니다.

## 워크스페이스 팩트를 어디서 얻나

AGENTS.md를 작성할 때 아래를 **직접 조회**해 최신 사실만 반영합니다(하드코딩 금지):

- 프로젝트 목록: 워크스페이스 루트의 하위 디렉토리(`.pylon/config.yml`의 등록 항목).
- 각 프로젝트 검증 명령: `<project>/.pylon/verify.yml`을 직접 읽어 build/test/lint 스텝을 확인.
  파일이 없거나 파싱 실패면 검증 미설정 — 변경 시 먼저 verify.yml을 작성해야 함.
- 프로젝트 메모리: `pylon mem list --project <name>` / `pylon mem search`.
- 도메인 지식: `.pylon/domain/overview.md`, `practices.md`, `glossary.md`,
  `<project>/.pylon/context.md`.

## 도메인 자동 감지

| 도메인 | 키워드/신호 | 워크플로우 | 핵심 에이전트 |
|--------|-----------|-----------|-------------|
| **소프트웨어 개발** | 구현, 코드, API, 버그, PR, 테스트 | feature/bugfix/hotfix | architect, backend-dev, frontend-dev, test-engineer |
| **리서치/조사** | 조사, 분석, 비교, 보고서, 논문, 트렌드 | research | lead-researcher, web-searcher, academic-analyst, fact-checker |
| **콘텐츠 제작** | 글, 블로그, 문서, 번역, 편집, 작성 | content | writer, editor, seo-specialist |
| **마케팅** | 캠페인, 광고, SEO, 타겟, 퍼널, 시장 | marketing | market-researcher, copywriter, data-analyst |

도메인이 모호하면 가장 적합한 도메인을 택하되 확신이 없으면 사용자에게 확인합니다.
혼합 작업(예: '리서치 후 구현')은 단계별로 도메인을 전환합니다.

## 도메인별 파이프라인

- **소프트웨어**: 요구사항 분석 → 아키텍처 → 태스크 분해 → 구현 → 검증 → PR → 위키 갱신
- **리서치**: 병렬 조사(web/academic/community) → 교차 검증 → 보고서 → 팩트 체크
- **콘텐츠**: 초안 → 편집/리뷰 → (피드백 루프) → 최종본
- **마케팅**: 시장 조사 → 전략 → 콘텐츠 생성 → 검증

파이프라인 전체를 돌릴지는 요구사항 규모로 판단합니다. 단일 파일 수정이나 질문 답변에
`/pl:pipeline`을 돌리지 않습니다 — 그냥 처리합니다.

## 상태 관리

- `.pylon/runtime/{pipeline-id}/`에 산출물이 파일로 저장됩니다.
- 산출물 존재 = 해당 스테이지 완료.
- `pylon status` CLI로 상태를 조회합니다.

## 프로젝트 메모리

프로젝트 지식은 `.pylon/memory/<project>/` 아래 마크다운 파일입니다. Grep/Read로 직접
탐색하거나 CLI를 사용합니다:

    pylon mem search --project <name> --query "검색어"   # 토큰 매칭 검색
    pylon mem store --project <name> --key "키" --content "내용"  # 저장
    pylon mem list --project <name>                       # 목록

## 슬래시 커맨드

- `/pl:pipeline` — 전체 파이프라인 실행 (요구사항 → PR). 모든 도메인의 범용 진입점.
- `/pl:architect` — 아키텍처 분석 단독 실행
- `/pl:breakdown` — PM 태스크 분해
- `/pl:execute` — 에이전트 병렬 실행
- `/pl:verify` — 교차 검증 실행 (빌드/테스트/린트)
- `/pl:pr` — PR 생성
- `/pl:status` — 파이프라인 상태 조회
- `/pl:cancel` — 파이프라인 취소

## 위임 판단

**기본값은 직접 수행입니다.** 아래 중 하나일 때만 위임합니다:

- 서로 파일이 겹치지 않는 독립 작업이 3개 이상이고 병렬 실행으로 실제 시간이 단축될 때
- 파일 수십 개를 훑어야 답이 나오고 최종적으로 필요한 건 결론뿐일 때 (탐색·조사)
- 자기 작업을 자기가 검증하면 안 될 때 (검증·비평·리뷰)

다음은 직접 처리합니다: 파일 3개 이하 수정·단일 함수 수정·원인을 아는 버그, 방금 나눈
대화 뉘앙스가 결과를 좌우하는 작업, 위임 프롬프트 비용이 직접 고치는 비용보다 큰 작업.
애매하면 직접 합니다.

### 위임 시 프롬프트에 반드시 넣을 것

서브 에이전트는 이 대화도, 읽은 파일도, 앞 단계 산출물도 보지 못합니다. 경로만 넘기지
말고 내용을 붙여넣습니다: (1) 배경·설계 근거 전문, (2) 완료 판정 기준, (3) 대상 파일
절대경로와 파악한 기존 패턴, (4) 완료 후 실행할 검증 명령(출력 전문 보고), (5) 범위 밖 항목.

### 서브 에이전트 선택 기준

전체 목록·정의는 `.pylon/agents/`. 아래에 해당하지 않으면 직접 처리합니다.

| 상황 | 에이전트 | 이유 |
|------|---------|------|
| 어디 있는지 모르는 코드를 파일 수십 개 훑어 찾아야 함 | `explorer` | 탐색 과정을 메인 컨텍스트에 남길 필요 없음 |
| 구현을 끝냈고 스스로 통과 판정하면 안 됨 | `verifier` | 자기 승인 방지 |
| 계획·설계 누락 점검 | `critic` | 별도 컨텍스트라야 전제를 의심 |
| PR diff 리뷰 | `code-reviewer` | 전용 체크리스트 보유 |
| 원인 모르는 버그 | `debugger`, `tracer` | 가설 검증에 긴 시행착오 |
| 겹치지 않는 구현 태스크 3개 이상 | `backend-dev`, `frontend-dev`, `test-engineer` | 병렬화 이득 |

목록에 있다는 이유만으로 에이전트를 쓰지 않습니다. 대부분 작업에 필요한 에이전트는 0개.

## 검증 규약

"완료"를 주장하기 전에 반드시 대상 프로젝트의 검증을 실행합니다. 검증 명령은 각
`<project>/.pylon/verify.yml`에서 읽습니다. 실행:

    .pylon/scripts/bash/run-verification.sh "$PIPELINE_DIR" --git-root <프로젝트 상대경로>

프로젝트 서브디렉토리가 없는 단일 저장소 워크스페이스에서는 `--git-root` 없이 루트에서
돕니다. 검증 결과가 `{"ok":false, ...}`이면 검증이 *수행되지 않은* 것 — 통과로 처리 금지.

## 행동 규칙

- 사용자와 한국어로 대화합니다.
- 요구사항이 모호하면 역질문으로 구체화합니다.
- 파이프라인 상태는 `.pylon/runtime/` 산출물로 자동 추적됩니다.
- 작업 완료 후 도메인 지식 갱신을 잊지 않습니다.
- 추측이 아닌 코드에서 확인된 사실만 기록합니다.

## 안티패턴 (하지 말 것)

- **경로만 던지는 위임** / **한 줄 태스크 위임**.
- **검증 없는 취합**: "완료했습니다"를 그대로 믿지 않고 변경 파일을 직접 읽고 검증을 돌린 뒤 완료 처리.
- **초록불 오독**: `{"ok":false}`를 통과로 처리하지 않음.
- **파일이 겹치는 병렬 실행**: 모순되는 설계 두 벌이 머지 충돌보다 나쁨.
- **에이전트 릴레이**: 출력을 읽지 않고 다음 에이전트에 넘기지 않음.
```

- [ ] **Step 2: embed 변수 추가**

Modify `internal/cli/init_cmd.go` — 기존 embed 블록(`//go:embed skills/*.md` 근처)에 추가:

```go
//go:embed reference/*.md
var embeddedReference embed.FS
```

- [ ] **Step 3: layout 헬퍼 추가**

Modify `internal/layout/layout.go` — `ScriptsDir` 아래에 추가:

```go
// ReferenceDir returns the .pylon/reference directory (embedded pylon usage manual).
func ReferenceDir(root string) string {
	return filepath.Join(PylonDir(root), "reference")
}
```

- [ ] **Step 4: syncPylonResources에 reference 편입 (실패하는 테스트 먼저)**

Add to `internal/cli/doctor_test.go`:

```go
func TestSyncPylonResourcesRefreshesReference(t *testing.T) {
	pylonDir := t.TempDir()
	// 오래된 내용을 미리 심어 둔다 — 동기화가 임베디드 버전으로 되돌려야 한다.
	refDir := filepath.Join(pylonDir, "reference")
	if err := os.MkdirAll(refDir, 0755); err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(refDir, "pylon-usage.md")
	if err := os.WriteFile(stale, []byte("STALE"), 0644); err != nil {
		t.Fatal(err)
	}
	syncPylonResources(pylonDir)
	got, err := os.ReadFile(stale)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "pylon-usage-version:") {
		t.Errorf("reference not refreshed from embed, got: %.40q", got)
	}
}
```

- [ ] **Step 5: 테스트 실패 확인**

Run: `go test ./internal/cli/ -run TestSyncPylonResourcesRefreshesReference -count=1`
Expected: FAIL (reference가 동기화되지 않아 여전히 "STALE").

- [ ] **Step 6: sync 목록에 reference 추가**

Modify `internal/cli/doctor.go` — `syncPylonResources` 내 sync 호출 블록(현재 375-378행)에 한 줄 추가:

```go
	sync(embeddedScripts, "scripts/bash", filepath.Join(pylonDir, "scripts", "bash"), ".sh", "scripts/bash")
	sync(embeddedReference, "reference", filepath.Join(pylonDir, "reference"), ".md", "reference")
```

- [ ] **Step 7: 테스트 통과 확인**

Run: `go test ./internal/cli/ -run TestSyncPylonResourcesRefreshesReference -count=1`
Expected: PASS

- [ ] **Step 8: writeReferenceTemplates + init 배치 (실패하는 테스트 먼저)**

Create `internal/cli/init_reference_test.go`:

```go
package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteReferenceTemplatesWritesManual(t *testing.T) {
	pylonDir := t.TempDir()
	if err := writeReferenceTemplates(pylonDir); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(pylonDir, "reference", "pylon-usage.md"))
	if err != nil {
		t.Fatalf("manual not written: %v", err)
	}
	if !strings.Contains(string(got), "pylon 워크스페이스 운영 매뉴얼") {
		t.Errorf("manual content missing, got: %.60q", got)
	}
}
```

- [ ] **Step 9: 테스트 실패 확인**

Run: `go test ./internal/cli/ -run TestWriteReferenceTemplatesWritesManual -count=1`
Expected: FAIL (`writeReferenceTemplates` 미정의로 컴파일 에러).

- [ ] **Step 10: writeReferenceTemplates 구현 + init 호출 + dirs + Created 출력**

Modify `internal/cli/init_cmd.go`:

`writeSkillTemplates` 함수 바로 아래에 추가 (같은 구조):

```go
func writeReferenceTemplates(pylonDir string) error {
	entries, err := embeddedReference.ReadDir("reference")
	if err != nil {
		return fmt.Errorf("failed to read embedded reference: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		content, err := embeddedReference.ReadFile("reference/" + entry.Name())
		if err != nil {
			return fmt.Errorf("failed to read reference %s: %w", entry.Name(), err)
		}
		path := filepath.Join(pylonDir, "reference", entry.Name())
		if err := os.WriteFile(path, content, 0644); err != nil {
			return fmt.Errorf("failed to create reference %s: %w", entry.Name(), err)
		}
	}
	return nil
}
```

dirs 목록(현재 `filepath.Join(pylonDir, "skills")` 포함 블록)에 추가:

```go
		filepath.Join(pylonDir, "reference"),
```

`writeSkillTemplates(pylonDir)` 호출 직후에 추가:

```go
	// Create embedded pylon usage manual (source for AI-authored AGENTS.md)
	if err := writeReferenceTemplates(pylonDir); err != nil {
		return err
	}
```

Created 출력의 skills 라인 아래에 추가:

```go
	fmt.Println("  .pylon/reference/          - embedded pylon usage manual")
```

- [ ] **Step 11: 테스트 통과 확인 + 전체 빌드**

Run: `go test ./internal/cli/ -run 'TestWriteReferenceTemplatesWritesManual|TestSyncPylonResourcesRefreshesReference' -count=1 && go build ./...`
Expected: PASS, 빌드 성공.

- [ ] **Step 12: 커밋**

```bash
gofmt -w internal/cli/init_cmd.go internal/cli/doctor.go internal/layout/layout.go
git add internal/cli/reference/pylon-usage.md internal/cli/init_cmd.go internal/cli/doctor.go internal/cli/init_reference_test.go internal/cli/doctor_test.go internal/layout/layout.go
git commit -m "feat: embed pylon usage manual and sync it to .pylon/reference"
```

---

## Task 2: 부트스트랩/포인터 빌더 + stale 판정 + ensureRootAgentFiles

기존 `buildRootCLAUDEMD`를 **아직 제거하지 않고** 신규 함수를 나란히 추가한다 (컴파일 유지). 제거는 Task 3.

**Files:**
- Create: `internal/cli/launch_agentsmd.go`
- Modify: `internal/layout/layout.go` (`RootClaudePath`, `RootAgentsPath`)
- Test: `internal/cli/launch_agentsmd_test.go`

**Interfaces:**
- Produces:
  - `const pylonUsageVersion = 1`
  - `func buildClaudeMDPointer() string` → `"@AGENTS.md\n"`
  - `func buildBootstrapAgentsMD(root string, projects []config.ProjectInfo) string`
  - `func agentsMDStale(root string) bool`
  - `func ensureRootAgentFiles(root string, projects []config.ProjectInfo) (bootstrapped bool, err error)`
  - `func layout.RootClaudePath(root string) string`, `func layout.RootAgentsPath(root string) string`
- Consumes: `config.ProjectInfo`, `layout.ReferenceDir`.

- [ ] **Step 1: layout 루트 파일 헬퍼 (테스트 먼저 아님 — 단순 경로)**

Modify `internal/layout/layout.go`:

```go
// RootClaudePath returns the workspace-root CLAUDE.md (a thin @AGENTS.md import marker).
func RootClaudePath(root string) string {
	return filepath.Join(root, "CLAUDE.md")
}

// RootAgentsPath returns the workspace-root AGENTS.md (AI-authored operating guide).
func RootAgentsPath(root string) string {
	return filepath.Join(root, "AGENTS.md")
}
```

- [ ] **Step 2: 실패하는 테스트 작성**

Create `internal/cli/launch_agentsmd_test.go`:

```go
package cli

import (
	"os"
	"strings"
	"testing"

	"github.com/kyago/pylon/internal/config"
	"github.com/kyago/pylon/internal/layout"
)

func TestBuildClaudeMDPointerIsImportMarker(t *testing.T) {
	if got := buildClaudeMDPointer(); strings.TrimSpace(got) != "@AGENTS.md" {
		t.Errorf("pointer should be the @AGENTS.md import marker, got %q", got)
	}
}

func TestBuildBootstrapAgentsMDHasStampAndInstruction(t *testing.T) {
	out := buildBootstrapAgentsMD(t.TempDir(), []config.ProjectInfo{{Name: "api"}})
	if !strings.Contains(out, "pylon-usage-version: 1") {
		t.Errorf("bootstrap missing version stamp: %s", out)
	}
	if !strings.Contains(out, ".pylon/reference/pylon-usage.md") {
		t.Errorf("bootstrap must point at the manual: %s", out)
	}
	if !strings.Contains(out, "AGENTS.md") {
		t.Errorf("bootstrap must instruct authoring AGENTS.md: %s", out)
	}
	if !strings.Contains(out, "api") {
		t.Errorf("bootstrap should list project facts: %s", out)
	}
}

func TestAgentsMDStale(t *testing.T) {
	root := t.TempDir()
	// (1) 파일 없음 → stale
	if !agentsMDStale(root) {
		t.Error("missing AGENTS.md should be stale")
	}
	// (2) 낮은 버전 스탬프 → stale
	if err := os.WriteFile(layout.RootAgentsPath(root), []byte("<!-- pylon-usage-version: 0 -->\n# guide"), 0644); err != nil {
		t.Fatal(err)
	}
	if !agentsMDStale(root) {
		t.Error("lower version stamp should be stale")
	}
	// (3) 스탬프 없음 → stale
	if err := os.WriteFile(layout.RootAgentsPath(root), []byte("# hand-written, no stamp"), 0644); err != nil {
		t.Fatal(err)
	}
	if !agentsMDStale(root) {
		t.Error("missing stamp should be stale")
	}
	// (4) 현재 버전 → not stale
	if err := os.WriteFile(layout.RootAgentsPath(root), []byte("<!-- pylon-usage-version: 1 -->\n# guide"), 0644); err != nil {
		t.Fatal(err)
	}
	if agentsMDStale(root) {
		t.Error("current version stamp should not be stale")
	}
}

func TestEnsureRootAgentFilesWritesPointerAndBootstrapWhenMissing(t *testing.T) {
	root := t.TempDir()
	bootstrapped, err := ensureRootAgentFiles(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !bootstrapped {
		t.Error("missing AGENTS.md should be bootstrapped")
	}
	ptr, _ := os.ReadFile(layout.RootClaudePath(root))
	if strings.TrimSpace(string(ptr)) != "@AGENTS.md" {
		t.Errorf("CLAUDE.md should be the import marker, got %q", ptr)
	}
	if _, err := os.Stat(layout.RootAgentsPath(root)); err != nil {
		t.Errorf("AGENTS.md bootstrap not written: %v", err)
	}
}

func TestEnsureRootAgentFilesPreservesFreshAgentsMD(t *testing.T) {
	root := t.TempDir()
	authored := "<!-- pylon-usage-version: 1 -->\n# 사용자 세션이 저작한 가이드"
	if err := os.WriteFile(layout.RootAgentsPath(root), []byte(authored), 0644); err != nil {
		t.Fatal(err)
	}
	bootstrapped, err := ensureRootAgentFiles(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if bootstrapped {
		t.Error("fresh AGENTS.md must not be bootstrapped over")
	}
	got, _ := os.ReadFile(layout.RootAgentsPath(root))
	if string(got) != authored {
		t.Errorf("fresh AGENTS.md was overwritten: %q", got)
	}
}
```

- [ ] **Step 3: 테스트 실패 확인**

Run: `go test ./internal/cli/ -run 'ClaudeMDPointer|BootstrapAgentsMD|AgentsMDStale|EnsureRootAgentFiles' -count=1`
Expected: FAIL (신규 함수 미정의 컴파일 에러).

- [ ] **Step 4: launch_agentsmd.go 구현**

Create `internal/cli/launch_agentsmd.go`:

```go
package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/kyago/pylon/internal/config"
	"github.com/kyago/pylon/internal/layout"
)

// pylonUsageVersion is the version of the embedded pylon usage manual
// (internal/cli/reference/pylon-usage.md). Bump it ONLY when that manual changes —
// it is the sole trigger that marks an existing AGENTS.md stale and forces the next
// session to re-author it. Independent of pylon's CalVer release version.
const pylonUsageVersion = 1

var usageVersionRe = regexp.MustCompile(`pylon-usage-version:\s*(\d+)`)

// buildClaudeMDPointer returns the workspace-root CLAUDE.md content: a single
// Claude Code import marker that pulls AGENTS.md into the session prompt. Kept
// deterministic so launch may rewrite it every time without churn.
func buildClaudeMDPointer() string {
	return "@AGENTS.md\n"
}

// buildBootstrapAgentsMD returns the short placeholder AGENTS.md written when the
// file is missing or stale. Its job is to make the launched session author the real
// guide on its first turn; it also stands on its own as a minimal fallback if
// authoring never happens.
func buildBootstrapAgentsMD(root string, projects []config.ProjectInfo) string {
	var b strings.Builder
	fmt.Fprintf(&b, "<!-- pylon-usage-version: %d -->\n", pylonUsageVersion)
	b.WriteString("# pylon 워크스페이스 — 운영 가이드 (자동 생성 부트스트랩)\n\n")
	b.WriteString("이 파일은 아직 이 워크스페이스에 맞게 저작되지 않았습니다.\n")
	b.WriteString("**다른 작업을 하기 전에**, `.pylon/reference/pylon-usage.md`와 실제 리포지토리·`.pylon/`을 읽고\n")
	b.WriteString("이 파일(AGENTS.md)을 이 워크스페이스의 운영 가이드로 다시 작성하세요.\n")
	fmt.Fprintf(&b, "완료하면 최상단 스탬프를 `<!-- pylon-usage-version: %d -->`로 남기세요.\n\n", pylonUsageVersion)
	fmt.Fprintf(&b, "- **워크스페이스 루트**: `%s`\n", root)
	b.WriteString("- **매뉴얼**: `.pylon/reference/pylon-usage.md`\n")
	b.WriteString("- **설정**: `.pylon/config.yml` / **도메인 지식**: `.pylon/domain/` / **에이전트**: `.pylon/agents/`\n")
	if len(projects) > 0 {
		fmt.Fprintf(&b, "- **프로젝트 %d개**: ", len(projects))
		names := make([]string, len(projects))
		for i, p := range projects {
			names[i] = p.Name
		}
		b.WriteString(strings.Join(names, ", "))
		b.WriteString("\n")
	} else {
		b.WriteString("- **프로젝트**: 없음 — `pylon add-project <git-url>`로 추가\n")
	}
	return b.String()
}

// agentsMDStale reports whether the workspace-root AGENTS.md needs (re)authoring:
// missing, no parseable version stamp, or a stamp older than pylonUsageVersion.
func agentsMDStale(root string) bool {
	data, err := os.ReadFile(layout.RootAgentsPath(root))
	if err != nil {
		return true
	}
	m := usageVersionRe.FindSubmatch(data)
	if m == nil {
		return true
	}
	v, err := strconv.Atoi(string(m[1]))
	if err != nil {
		return true
	}
	return v < pylonUsageVersion
}

// ensureRootAgentFiles keeps the two workspace-root files in the desired state:
// CLAUDE.md is always (re)written to the deterministic @AGENTS.md import marker, and
// AGENTS.md is (re)written to the bootstrap ONLY when stale — a session-authored,
// current AGENTS.md is left untouched. Returns whether AGENTS.md was bootstrapped.
func ensureRootAgentFiles(root string, projects []config.ProjectInfo) (bool, error) {
	if err := os.WriteFile(layout.RootClaudePath(root), []byte(buildClaudeMDPointer()), 0644); err != nil {
		return false, fmt.Errorf("CLAUDE.md 생성 실패: %w", err)
	}
	if !agentsMDStale(root) {
		return false, nil
	}
	if err := os.WriteFile(layout.RootAgentsPath(root), []byte(buildBootstrapAgentsMD(root, projects)), 0644); err != nil {
		return false, fmt.Errorf("AGENTS.md 부트스트랩 실패: %w", err)
	}
	return true, nil
}

var _ = filepath.Join // (경로 조립은 layout 헬퍼 사용; 임포트 유지용 — 실제 사용 시 제거)
```

주의: 위 마지막 `var _ = filepath.Join` 줄은 `filepath`를 실제로 안 쓰면 넣고, 쓰면 지웁니다. 현재 구현은 `filepath`를 직접 쓰지 않으므로 **`filepath` 임포트와 그 줄을 함께 삭제**하세요. (gofmt/lint가 미사용 임포트를 잡습니다.)

- [ ] **Step 5: 테스트 통과 확인**

Run: `go test ./internal/cli/ -run 'ClaudeMDPointer|BootstrapAgentsMD|AgentsMDStale|EnsureRootAgentFiles' -count=1`
Expected: PASS

- [ ] **Step 6: 전체 빌드(구 buildRootCLAUDEMD 공존 확인)**

Run: `go build ./... && go vet ./internal/cli/`
Expected: 성공 (신·구 함수 공존, 아직 launch는 구함수 사용).

- [ ] **Step 7: 커밋**

```bash
gofmt -w internal/cli/launch_agentsmd.go internal/layout/layout.go
git add internal/cli/launch_agentsmd.go internal/cli/launch_agentsmd_test.go internal/layout/layout.go
git commit -m "feat: add AGENTS.md bootstrap/pointer builders and stale detection"
```

---

## Task 3: launch을 ensureRootAgentFiles로 전환 + 구 생성기 제거

**Files:**
- Modify: `internal/cli/launch.go` (`generateClaudeDir:153-156`, `addClaudeDirToGitignore:199`)
- Delete: `internal/cli/launch_claudemd.go` (전체)
- Delete: `internal/cli/launch_claudemd_test.go` (전체)
- Test: `internal/cli/launch_test.go` (통합 케이스 추가; 기존 파일 없으면 생성)

**Interfaces:**
- Consumes: `ensureRootAgentFiles` (Task 2).

- [ ] **Step 1: 실패하는 통합 테스트 작성**

Add to `internal/cli/launch_test.go` (파일 없으면 `package cli` 헤더로 생성):

```go
func TestGenerateClaudeDirWritesPointerAndBootstrap(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(layout.PylonDir(root), 0755); err != nil {
		t.Fatal(err)
	}
	if err := generateClaudeDir(root, &config.Config{}, nil); err != nil {
		t.Fatal(err)
	}
	ptr, _ := os.ReadFile(layout.RootClaudePath(root))
	if strings.TrimSpace(string(ptr)) != "@AGENTS.md" {
		t.Errorf("CLAUDE.md should be @AGENTS.md marker, got %q", ptr)
	}
	if _, err := os.Stat(layout.RootAgentsPath(root)); err != nil {
		t.Errorf("AGENTS.md bootstrap missing: %v", err)
	}
}

func TestGenerateClaudeDirDoesNotClobberAuthoredAgentsMD(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(layout.PylonDir(root), 0755); err != nil {
		t.Fatal(err)
	}
	authored := "<!-- pylon-usage-version: 1 -->\n# 저작된 가이드 (보존되어야 함)"
	if err := os.WriteFile(layout.RootAgentsPath(root), []byte(authored), 0644); err != nil {
		t.Fatal(err)
	}
	if err := generateClaudeDir(root, &config.Config{}, nil); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(layout.RootAgentsPath(root))
	if string(got) != authored {
		t.Errorf("authored AGENTS.md was overwritten: %q", got)
	}
}
```

> 참고: `generateClaudeDir`는 `.claude/` 하위와 `syncPylonResources`도 실행합니다. 테스트는
> 루트 파일 동작만 검증하며, 임베디드 리소스 동기화는 tmp에서 정상 동작합니다.

- [ ] **Step 2: 테스트 실패 확인**

Run: `go test ./internal/cli/ -run 'GenerateClaudeDirWritesPointerAndBootstrap|GenerateClaudeDirDoesNotClobber' -count=1`
Expected: FAIL (현재는 구 buildRootCLAUDEMD가 CLAUDE.md에 긴 프로즈를 쓰므로 마커 불일치, AGENTS.md 없음).

- [ ] **Step 3: generateClaudeDir 교체**

Modify `internal/cli/launch.go` — 다음 블록(153-156행):

```go
	// Generate CLAUDE.md at workspace root
	claudeMD := buildRootCLAUDEMD(cfg, projects, root)
	if err := os.WriteFile(filepath.Join(root, "CLAUDE.md"), []byte(claudeMD), 0644); err != nil {
		return fmt.Errorf("CLAUDE.md 생성 실패: %w", err)
	}
```

을 아래로 교체:

```go
	// Root agent files: CLAUDE.md is a deterministic @AGENTS.md import marker; AGENTS.md
	// is (re)bootstrapped only when missing/stale so a session-authored guide survives
	// launch. The launched claude session authors AGENTS.md on its first turn.
	if bootstrapped, err := ensureRootAgentFiles(root, projects); err != nil {
		return err
	} else if bootstrapped {
		fmt.Fprintln(os.Stderr, "ℹ AGENTS.md를 부트스트랩했습니다 — 세션이 첫 턴에 이 워크스페이스에 맞게 재작성합니다.")
	}
```

> 참고: 이 교체 후 `cfg` 매개변수가 `generateClaudeDir` 안에서 여전히
> `generateClaudeAgentsWithSkills(root, cfg)`에 쓰이므로 미사용이 되지 않습니다.

- [ ] **Step 4: gitignore에 AGENTS.md 추가**

Modify `internal/cli/launch.go` — `addClaudeDirToGitignore`의 엔트리 목록(199행):

```go
	for _, entry := range []string{".claude/", "CLAUDE.md", "AGENTS.md", ".pylon/logs/"} {
```

- [ ] **Step 5: 구 생성기·구 테스트 삭제**

```bash
git rm internal/cli/launch_claudemd.go internal/cli/launch_claudemd_test.go
```

- [ ] **Step 6: 미사용 임포트 정리 + 빌드**

`launch.go`가 `filepath`를 다른 곳에서도 쓰는지 확인하고, 안 쓰면 임포트 제거.

Run: `gofmt -w internal/cli/launch.go && go build ./... && go vet ./internal/cli/`
Expected: 성공. (`buildRootCLAUDEMD` 참조가 모두 사라졌는지 컴파일러가 보증.)

- [ ] **Step 7: 테스트 통과 확인**

Run: `go test ./internal/cli/ -run 'GenerateClaudeDir' -count=1`
Expected: PASS

- [ ] **Step 8: 커밋**

```bash
git add internal/cli/launch.go internal/cli/launch_test.go
git commit -m "feat: launch writes @AGENTS.md pointer and bootstraps AGENTS.md instead of hardcoded CLAUDE.md"
```

---

## Task 4: init이 루트 파일을 배치하도록 연결

`init`은 현재 CLAUDE.md/AGENTS.md를 만들지 않는다. 워크스페이스 초기화 직후에도 루트 파일이 준비되도록 연결한다.

**주의(테스트 전략):** `runInit`은 `initDoctorChecks()`(외부 도구 필요)와 `bufio.NewReader(os.Stdin)`(대화형 입력)을 실행하므로 순수 단위 테스트 대상이 아니다. 배치되는 핵심 함수 `ensureRootAgentFiles`는 Task 2/3에서 이미 단위 테스트로 커버됐다. 따라서 이 태스크의 검증은 **빌드 + 실제 `pylon init` 스모크 테스트**로 한다.

**Files:**
- Modify: `internal/cli/init_cmd.go` (프로젝트 탐색 후 `ensureRootAgentFiles` 호출, gitignore에 루트 파일 추가, Created 출력)

**Interfaces:**
- Consumes: `ensureRootAgentFiles` (Task 2), `config.DiscoverProjects`.

- [ ] **Step 1: init에 루트 파일 배치 추가**

Modify `internal/cli/init_cmd.go` — `projects, err := config.DiscoverProjects(workDir)` 이후 프로젝트별 exclude 루프 **직후**(“Pylon workspace initialized” 출력 전)에 추가:

```go
	// Root agent files so the workspace is launch-ready: CLAUDE.md import marker +
	// bootstrap AGENTS.md the first session will author against the embedded manual.
	if _, err := ensureRootAgentFiles(workDir, projects); err != nil {
		return err
	}
```

Created 출력 블록에 추가:

```go
	fmt.Println("  CLAUDE.md                  - @AGENTS.md import marker")
	fmt.Println("  AGENTS.md                  - operating guide (authored by first session)")
```

- [ ] **Step 2: gitignore에 루트 파일 추가**

Modify `internal/cli/init_cmd.go` — init의 `gitignoreEntries` 목록에 추가(런처의 `addClaudeDirToGitignore`와 별개로 init 시점에도 무시되도록):

```go
		"",
		"# Pylon root agent files (regenerated; AI-authored)",
		"CLAUDE.md",
		"AGENTS.md",
		"",
```

- [ ] **Step 3: 빌드**

Run: `gofmt -w internal/cli/init_cmd.go && go build ./...`
Expected: 성공.

- [ ] **Step 4: 실제 init 스모크 테스트**

임시 디렉토리에서 방금 빌드한 바이너리로 init을 실행하고 루트 파일을 확인한다(대화형 reviewer 입력은 빈 stdin으로 스킵):

```bash
make build
TMP="$(mktemp -d)"
printf '\n' | ./bin/pylon init --workspace "$TMP"
test "$(tr -d '[:space:]' < "$TMP/CLAUDE.md")" = "@AGENTS.md" && echo "OK: CLAUDE.md marker"
grep -q "pylon-usage-version:" "$TMP/AGENTS.md" && echo "OK: AGENTS.md bootstrap"
grep -q "운영 매뉴얼" "$TMP/.pylon/reference/pylon-usage.md" && echo "OK: reference manual"
grep -qx "AGENTS.md" "$TMP/.gitignore" && echo "OK: gitignore"
rm -rf "$TMP"
```

Expected: 네 개의 `OK:` 라인 모두 출력. (`initDoctorChecks`가 요구하는 git 등 외부 도구는 개발 환경에 설치돼 있어야 함.)

- [ ] **Step 5: init 관련 기존 테스트 회귀 확인**

Run: `go test ./internal/cli/ -run TestInit -count=1`
Expected: PASS.

- [ ] **Step 6: 커밋**

```bash
git add internal/cli/init_cmd.go
git commit -m "feat: init places @AGENTS.md marker, bootstrap AGENTS.md, and gitignores them"
```

---

## Task 5: doctor가 stale AGENTS.md를 재부트스트랩·보고

**Files:**
- Modify: `internal/cli/doctor.go` (`syncResourcesIfWorkspace` 또는 `runDoctor` 흐름에 stale 체크 추가)
- Test: `internal/cli/doctor_test.go`

**Interfaces:**
- Consumes: `agentsMDStale`, `ensureRootAgentFiles`, `resolveRoot`/워크스페이스 로케이터.

- [ ] **Step 1: 실패하는 테스트 작성**

Add to `internal/cli/doctor_test.go`:

```go
func TestReconcileRootAgentFilesRebootstrapsStale(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(layout.PylonDir(root), 0755); err != nil {
		t.Fatal(err)
	}
	// 저버전 스탬프 = stale
	if err := os.WriteFile(layout.RootAgentsPath(root), []byte("<!-- pylon-usage-version: 0 -->\n# old"), 0644); err != nil {
		t.Fatal(err)
	}
	bootstrapped, err := reconcileRootAgentFiles(root)
	if err != nil {
		t.Fatal(err)
	}
	if !bootstrapped {
		t.Error("stale AGENTS.md should be re-bootstrapped")
	}
	got, _ := os.ReadFile(layout.RootAgentsPath(root))
	if !strings.Contains(string(got), "pylon-usage-version: 1") {
		t.Errorf("AGENTS.md not refreshed to current stamp: %.60q", got)
	}
}

func TestReconcileRootAgentFilesLeavesCurrent(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(layout.PylonDir(root), 0755); err != nil {
		t.Fatal(err)
	}
	authored := "<!-- pylon-usage-version: 1 -->\n# 저작됨"
	if err := os.WriteFile(layout.RootAgentsPath(root), []byte(authored), 0644); err != nil {
		t.Fatal(err)
	}
	bootstrapped, err := reconcileRootAgentFiles(root)
	if err != nil {
		t.Fatal(err)
	}
	if bootstrapped {
		t.Error("current AGENTS.md must be left alone")
	}
	got, _ := os.ReadFile(layout.RootAgentsPath(root))
	if string(got) != authored {
		t.Errorf("current AGENTS.md was overwritten: %q", got)
	}
}
```

- [ ] **Step 2: 테스트 실패 확인**

Run: `go test ./internal/cli/ -run 'ReconcileRootAgentFiles' -count=1`
Expected: FAIL (`reconcileRootAgentFiles` 미정의).

- [ ] **Step 3: reconcileRootAgentFiles 구현 + doctor 흐름 연결**

Modify `internal/cli/doctor.go` — 다음 헬퍼 추가 (projects는 doctor 컨텍스트에서 탐색):

```go
// reconcileRootAgentFiles refreshes the CLAUDE.md marker and re-bootstraps AGENTS.md
// when it is missing or older than the embedded manual, so `pylon doctor` recovers a
// workspace whose root guide drifted. It never invokes an LLM — the next launched
// session authors the refreshed bootstrap. Returns whether AGENTS.md was bootstrapped.
func reconcileRootAgentFiles(root string) (bool, error) {
	projects, err := config.DiscoverProjects(root)
	if err != nil {
		projects = nil // 탐색 실패 시 팩트 없는 부트스트랩이라도 최신화한다
	}
	return ensureRootAgentFiles(root, projects)
}
```

`syncResourcesIfWorkspace`(doctor.go:316)는 시작에서 이미 `root, err := resolveRoot()`로
`root`를 확보한다. 그 함수 내 `syncPylonResources(pylonDir)` 호출 직후에 연결:

```go
	if bootstrapped, err := reconcileRootAgentFiles(root); err != nil {
		fmt.Printf("⚠ 루트 에이전트 파일 갱신 실패: %v\n", err)
	} else if bootstrapped {
		fmt.Println("✓ AGENTS.md를 부트스트랩했습니다 — 다음 실행 시 세션이 이 워크스페이스에 맞게 재작성합니다.")
	}
```

> `config` 임포트는 doctor.go에 이미 존재(`config.LoadConfig` 사용 중)하므로 추가 불필요.

- [ ] **Step 4: 테스트 통과 확인**

Run: `go test ./internal/cli/ -run 'ReconcileRootAgentFiles' -count=1`
Expected: PASS

- [ ] **Step 5: 전체 테스트 + 린트**

Run: `make test && make lint`
Expected: 모두 통과. (린트 실패 시 `golangci-lint fmt`로 정리 후 재실행.)

- [ ] **Step 6: 커밋**

```bash
gofmt -w internal/cli/doctor.go
git add internal/cli/doctor.go internal/cli/doctor_test.go
git commit -m "feat: doctor re-bootstraps stale AGENTS.md and reports it"
```

---

## Self-Review

**Spec coverage:**
- 임베디드 매뉴얼 공급 → Task 1. ✓
- CLAUDE.md = `@AGENTS.md` 마커 → Task 2(빌더)+Task 3(launch)+Task 4(init). ✓
- AGENTS.md 세션 저작 + 부트스트랩 폴백 → Task 2(부트스트랩)+Task 3/4/5(배치). ✓
- 버전 스탬프 상수·stale 판정 → Task 2. ✓
- launch 무조건 덮어쓰기 제거 → Task 3. ✓
- init 오프라인·AI 미호출 → 전체(Go가 LLM 미호출). ✓
- doctor stale 재부트스트랩·보고 → Task 5. ✓
- `renderVerificationCommands`/메모리 주입 제거 → Task 3(파일 삭제). ✓
- reference를 소유권 계약에 편입 → Task 1(syncPylonResources). ✓
- git-ignore 유지 → Task 3(launcher)+Task 4(init). ✓
- `@AGENTS.md` import 동작 검증(미해결 항목) → 아래 "구현 후 수동 검증"에서 다룸.

**Placeholder scan:** 코드 스텝은 실제 코드 포함. 참조 함수(`runInit`, `syncResourcesIfWorkspace`, `config.DiscoverProjects`, `resolveRoot`)와 연결 지점(doctor.go:316의 `root`)은 실제 코드베이스에서 확인됨. init은 단위 테스트 불가(doctor 체크+stdin)라 스모크 테스트로 검증. 플레이스홀더 없음.

**Type consistency:** `ensureRootAgentFiles(root string, projects []config.ProjectInfo) (bool, error)`가 Task 2 정의와 Task 3/4/5 호출에서 일치. `agentsMDStale(root string) bool`, `buildClaudeMDPointer() string`, `buildBootstrapAgentsMD(root string, projects []config.ProjectInfo) string`, `layout.RootClaudePath/RootAgentsPath/ReferenceDir` 시그니처 전 태스크 일관. `pylonUsageVersion=1`과 매뉴얼/부트스트랩/테스트 스탬프 값 일치.

## 구현 후 수동 검증 (플랜 완료 시 1회)

- `make build && make test && make lint` 통과.
- 임시 워크스페이스에서 `pylon init` → `.pylon/reference/pylon-usage.md`, `CLAUDE.md`(=`@AGENTS.md`), 부트스트랩 `AGENTS.md` 생성 확인.
- 실제 `pylon` 실행(claude 세션)에서 `@AGENTS.md` import가 세션 프롬프트에 AGENTS.md 내용을 끌어오는지, 세션이 첫 턴에 AGENTS.md를 재저작하는지 확인. (미해결 항목의 상대경로 import 동작 확인 포함.)
```
