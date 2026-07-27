# 메모리 주입 위생·저장 위생 구현 계획

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** proactive injection이 프로젝트 간 예산을 공정하게 나누고 중요도(confidence·recency) 순으로 절단되게 하며, 저장소가 근사 중복과 만료 항목을 스스로 정리하게 한다.

**Architecture:** (1) 주입 경로는 git 추적 `INDEX.md`(카테고리순, 사람용)를 그대로 두고, 주입 전용 렌더러 `InjectionMarkdown`(confidence desc → recency desc)을 신설해 `buildRootCLAUDEMD`가 프로젝트별 공정 분배(fair-share with carryover)로 호출한다. (2) 저장 경로는 `Insert`의 바이트-동일 dedup을 문자 bigram Dice 유사도 기반 근사 중복 감지로 확장하고, 카테고리별 보존 정책(`retention_days`)을 config에 추가해 Stop hook 경로(`sync-memory --from-session`)에서 만료 항목을 자동 정리한다.

**Tech Stack:** Go 1.24+ (toolchain go1.26), 표준 라이브러리 + `golang.org/x/text/unicode/norm` (이미 go.sum에 v0.23.0 indirect로 존재 — 새 모듈 다운로드 없음, direct로 승격만 됨).

## Global Constraints

- 새 외부 모듈 금지. `golang.org/x/text`는 이미 의존성 트리에 있으므로 import 후 `go mod tidy`로 direct 승격만 허용 (CLAUDE.md: "Dependencies are minimal by design").
- 사용자 대상 문자열은 한국어, 개발자용 에러/주석은 파일 관례에 따름 (CLAUDE.md 규약).
- 테스트는 CI와 동일하게 실행: `go test ./... -race -count=1` (`make test`).
- 린트: `make lint` (golangci-lint 기본 설정).
- `.pylon/` 경로는 반드시 `internal/layout` 헬퍼로만 구성 (기존 `Store` 메서드 사용으로 자동 충족).
- `INDEX.md`는 git 추적 사람용 파일 — 이 계획에서 포맷/정렬을 바꾸지 않는다. 주입 전용 출력만 신설한다.
- 커밋 메시지는 기존 로그 스타일(한국어, `feat:`/`fix:` prefix)을 따른다.

## 배경 (구현자가 알아야 할 현재 동작)

- `internal/memory/store.go:88-98` — `Insert`는 같은 project/category에서 **바이트 동일** content만 `ErrDuplicate`로 스킵한다(D4). 표현만 바뀐 재진술은 매 턴 새 파일로 쌓인다 (Stop hook이 매 턴 `sync-memory --from-session`을 호출하기 때문 — `internal/cli/hooks.json`).
- `internal/memory/index.go:51-67` — `IndexMarkdown`은 INDEX.md(카테고리 asc → 날짜 desc 정렬)를 바이트 경계에서 절단한다. 카테고리 알파벳순이라 절단 시 뒤 카테고리가 중요도와 무관하게 통째로 사라진다.
- `internal/cli/launch_claudemd.go:86-111` — 주입 예산(`ProactiveMaxTokens`×4 바이트, 기본 8000)이 **모든 프로젝트가 공유**하는 단일 풀이라 loop 앞쪽 프로젝트가 독식하면 뒤 프로젝트는 0바이트를 받는다.
- `IndexMarkdown`의 프로덕션 소비자는 `launch_claudemd.go:99` **한 곳뿐**이다 (Task 6에서 교체 후 삭제).
- 보존 정책은 존재하지 않는다. `mem prune`(`internal/cli/mem.go:143`)은 카테고리 일괄 삭제일 뿐 시간 기반이 아니다.

## File Structure

| 파일 | 작업 | 책임 |
|---|---|---|
| `internal/memory/similarity.go` | 생성 | 근사 중복 판정 (정규화·bigram·Dice) — 순수 함수만 |
| `internal/memory/similarity_test.go` | 생성 | 위 헬퍼 테스트 |
| `internal/memory/store.go` | 수정 | `Insert` 근사 중복 감지, `PruneExpired` 추가 |
| `internal/memory/store_test.go` | 수정 | 근사 중복·PruneExpired·InjectionMarkdown 테스트 |
| `internal/memory/index.go` | 수정 | `indexLine` 추출, `InjectionMarkdown` 신설, `IndexMarkdown` 삭제 |
| `internal/config/config.go` | 수정 | `MemoryConfig.RetentionDays` + 기본값 |
| `internal/config/config_test.go` | 수정 | retention_days 파싱/기본값 테스트 |
| `internal/cli/sync_memory.go` | 수정 | 저장 후 만료 자동 정리 |
| `internal/cli/sync_memory_test.go` | 수정 | 자동 정리 테스트 |
| `internal/cli/launch_claudemd.go` | 수정 | 공정 예산 분배 loop |
| `internal/cli/launch_claudemd_test.go` | 수정 | 굶주림 방지 테스트 |
| `README.md` | 수정 | `retention_days` 설정 문서 (416행 부근 memory 블록) |

---

### Task 1: 근사 중복 유사도 헬퍼 (`similarity.go`)

**Files:**
- Create: `internal/memory/similarity.go`
- Test: `internal/memory/similarity_test.go`

**Interfaces:**
- Consumes: 없음 (순수 함수)
- Produces: `isNearDuplicate(a, b string) bool` — Task 2의 `Insert`가 호출. 내부 헬퍼 `normalizeForCompare(s string) string`, `bigramSet(s string) map[string]struct{}`, `diceSimilarity(a, b string) float64`.

- [ ] **Step 1: 실패하는 테스트 작성**

`internal/memory/similarity_test.go`:

```go
// internal/memory/similarity_test.go
package memory

import "testing"

func TestIsNearDuplicate(t *testing.T) {
	cases := []struct {
		name string
		a, b string
		want bool
	}{
		{"동일 문장", "이 프로젝트는 CGO_ENABLED=0으로 빌드해야 한다", "이 프로젝트는 CGO_ENABLED=0으로 빌드해야 한다", true},
		{"공백·대소문자 차이", "이 프로젝트는  CGO_ENABLED=0으로 빌드해야 한다", "이 프로젝트는 cgo_enabled=0으로 빌드해야 한다", true},
		{"어미만 다른 재진술", "테스트는 race 플래그를 켜고 실행해야 한다는 것", "테스트는 race 플래그를 켜고 실행해야 한다", true},
		{"다른 지식", "테스트는 race 플래그를 켜고 실행해야 한다", "릴리스는 goreleaser가 태그 푸시로 트리거한다", false},
		{"짧은 문자열은 정확 일치만", "빌드 성공", "빌드 성능", false},
		{"짧아도 정규화 후 동일하면 중복", "빌드 성공", "빌드  성공", true},
		// macOS 클립보드 등에서 NFD(자모 분해)로 들어온 한글은 NFC로 정규화해 비교한다
		{"NFD/NFC 정규화", "한글 인코딩 관련 메모", "한글 인코딩 관련 메모", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := isNearDuplicate(c.a, c.b); got != c.want {
				t.Errorf("isNearDuplicate(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
			}
		})
	}
}

func TestDiceSimilarityBounds(t *testing.T) {
	if got := diceSimilarity("", "아무 내용"); got != 0 {
		t.Errorf("빈 문자열 유사도 = %v, want 0", got)
	}
	if got := diceSimilarity("같은 내용 문자열", "같은 내용 문자열"); got != 1 {
		t.Errorf("동일 문자열 유사도 = %v, want 1", got)
	}
}
```

- [ ] **Step 2: 테스트 실패 확인**

Run: `go test ./internal/memory/ -run 'TestIsNearDuplicate|TestDiceSimilarity' -race -count=1`
Expected: FAIL — `undefined: isNearDuplicate`, `undefined: diceSimilarity` (컴파일 에러)

- [ ] **Step 3: 최소 구현 작성**

`internal/memory/similarity.go`:

```go
// internal/memory/similarity.go
package memory

import (
	"strings"

	"golang.org/x/text/unicode/norm"
)

// nearDuplicateThreshold: 이 값 이상의 Dice 유사도는 같은 지식의 재진술로
// 간주한다 (D4 확장 — Stop hook이 매 턴 표현만 바뀐 학습을 다시 보내는 문제).
const nearDuplicateThreshold = 0.90

// nearDuplicateMinRunes: 이보다 짧은 문자열은 bigram 집합이 작아 유사도
// 신호가 불안정하므로 정규화 후 정확 일치만 중복으로 본다.
const nearDuplicateMinRunes = 20

// normalizeForCompare canonicalizes content for duplicate comparison:
// NFC(한글 자모 결합) → 소문자 → 연속 공백 축약.
func normalizeForCompare(s string) string {
	s = norm.NFC.String(s)
	s = strings.ToLower(s)
	return strings.Join(strings.Fields(s), " ")
}

// bigramSet returns the set of adjacent-rune pairs in s.
func bigramSet(s string) map[string]struct{} {
	runes := []rune(s)
	set := make(map[string]struct{}, len(runes))
	for i := 0; i+1 < len(runes); i++ {
		set[string(runes[i:i+2])] = struct{}{}
	}
	return set
}

// diceSimilarity computes the Sørensen–Dice coefficient of two strings'
// bigram sets. 문서-문서처럼 크기가 비슷한 두 집합 비교에 적합하다.
func diceSimilarity(a, b string) float64 {
	sa, sb := bigramSet(a), bigramSet(b)
	if len(sa) == 0 || len(sb) == 0 {
		return 0
	}
	inter := 0
	for g := range sa {
		if _, ok := sb[g]; ok {
			inter++
		}
	}
	return 2 * float64(inter) / float64(len(sa)+len(sb))
}

// isNearDuplicate reports whether two contents carry the same knowledge.
func isNearDuplicate(a, b string) bool {
	na, nb := normalizeForCompare(a), normalizeForCompare(b)
	if na == nb {
		return true
	}
	if len([]rune(na)) < nearDuplicateMinRunes || len([]rune(nb)) < nearDuplicateMinRunes {
		return false
	}
	return diceSimilarity(na, nb) >= nearDuplicateThreshold
}
```

- [ ] **Step 4: 의존성 정리 후 테스트 통과 확인**

Run: `go mod tidy && go test ./internal/memory/ -run 'TestIsNearDuplicate|TestDiceSimilarity' -race -count=1`
Expected: PASS. `git diff go.mod`에서 `golang.org/x/text`가 direct require 블록으로 이동했는지 확인 (새 모듈 추가는 없어야 함).

- [ ] **Step 5: 커밋**

```bash
git add internal/memory/similarity.go internal/memory/similarity_test.go go.mod go.sum
git commit -m "feat: 메모리 근사 중복 판정 헬퍼 추가 (NFC + bigram Dice)"
```

---

### Task 2: `Insert` 근사 중복 감지

**Files:**
- Modify: `internal/memory/store.go:88-98` (dedup loop)
- Test: `internal/memory/store_test.go`

**Interfaces:**
- Consumes: `isNearDuplicate(a, b string) bool` (Task 1)
- Produces: `Insert`가 근사 중복에도 `ErrDuplicate`를 반환 (기존 시그니처 불변 — 호출부 수정 없음. `StoreLearnings`와 `mem store`는 이미 `errors.Is(err, ErrDuplicate)`로 스킵 처리함)

- [ ] **Step 1: 실패하는 테스트 작성**

`internal/memory/store_test.go` 끝에 추가:

```go
// Stop hook이 매 턴 표현만 바뀐 학습을 다시 보내므로, 바이트 동일이 아니어도
// 근사 중복이면 스킵되어야 한다 (D4 확장).
func TestInsertSkipsNearDuplicate(t *testing.T) {
	s := newTestStore(t)
	mustInsert(t, s, &Entry{ProjectID: "app", Category: "learning", Key: "race 플래그",
		Content: "테스트는 race 플래그를 켜고 실행해야 한다", Confidence: 0.8})

	e := &Entry{ProjectID: "app", Category: "learning", Key: "race 플래그 재진술",
		Content: "테스트는 race 플래그를 켜고 실행해야 한다는 것", Confidence: 0.8}
	err := s.Insert(e)
	if !errors.Is(err, ErrDuplicate) {
		t.Fatalf("근사 중복은 ErrDuplicate여야 한다: %v", err)
	}
	if e.Path == "" {
		t.Error("스킵 시 기존 항목의 Path가 채워져야 한다")
	}

	// 카테고리가 다르면 같은 내용도 저장된다 (기존 동작 유지)
	mustInsert(t, s, &Entry{ProjectID: "app", Category: "decision", Key: "race 플래그",
		Content: "테스트는 race 플래그를 켜고 실행해야 한다는 것", Confidence: 0.9})
}

// 짧은 항목은 bigram 신호가 불안정하므로 정확 일치만 중복 처리한다.
func TestInsertKeepsDistinctShortEntries(t *testing.T) {
	s := newTestStore(t)
	mustInsert(t, s, &Entry{ProjectID: "app", Category: "learning", Key: "짧은 항목 1",
		Content: "빌드 성공", Confidence: 0.8})
	mustInsert(t, s, &Entry{ProjectID: "app", Category: "learning", Key: "짧은 항목 2",
		Content: "빌드 성능", Confidence: 0.8})
}
```

- [ ] **Step 2: 테스트 실패 확인**

Run: `go test ./internal/memory/ -run 'TestInsertSkipsNearDuplicate|TestInsertKeepsDistinctShortEntries' -race -count=1`
Expected: `TestInsertSkipsNearDuplicate` FAIL — "근사 중복은 ErrDuplicate여야 한다: <nil>" (현재는 바이트 동일만 감지). `TestInsertKeepsDistinctShortEntries` PASS.

- [ ] **Step 3: 구현**

`internal/memory/store.go` 93-98행의 비교를 교체:

```go
	// 변경 전
	for _, prev := range existing {
		if prev.Content == e.Content {
			e.Path = prev.Path
			return fmt.Errorf("%w: %s", ErrDuplicate, prev.Path)
		}
	}
```

```go
	// 변경 후
	for _, prev := range existing {
		if isNearDuplicate(prev.Content, e.Content) {
			e.Path = prev.Path
			return fmt.Errorf("%w: %s", ErrDuplicate, prev.Path)
		}
	}
```

주변 주석(88행 "Stop hook이 매 턴 같은 학습을 보내므로 동일 내용은 저장하지 않는다 (D4).")을 다음으로 갱신:

```go
	// Stop hook이 매 턴 같은 학습을 보내므로 동일·근사 중복 내용은 저장하지
	// 않는다 (D4). 표현만 바뀐 재진술은 bigram Dice 유사도로 걸러낸다.
```

- [ ] **Step 4: 패키지 전체 테스트 통과 확인**

Run: `go test ./internal/memory/ -race -count=1`
Expected: PASS — 신규 2건 포함, 기존 테스트(`TestInsertResolvesFileNameCollision`의 "내용 하나/둘/셋"은 짧아서 정확 일치 규칙 적용, `TestSearchKoreanSubstring` 등) 모두 통과.

- [ ] **Step 5: 커밋**

```bash
git add internal/memory/store.go internal/memory/store_test.go
git commit -m "feat: Insert 근사 중복 감지 — 재진술 학습의 매 턴 누적 차단"
```

---

### Task 3: config `retention_days` 스키마·기본값

**Files:**
- Modify: `internal/config/config.go:91-94` (MemoryConfig), `config.go:347-352` (applyDefaults)
- Modify: `README.md` (416행 부근 `memory:` 블록)
- Test: `internal/config/config_test.go`

**Interfaces:**
- Consumes: 없음
- Produces: `MemoryConfig.RetentionDays map[string]int` (yaml `retention_days`) — Task 5가 `cfg.Memory.RetentionDays`로 소비. 의미: 카테고리 → 보존 일수. **미지정 카테고리 또는 0 이하 값 = 영구 보존.** 기본값 `{"learning": 30}`. 명시적 `retention_days: {}`는 전체 영구 보존 opt-out.

- [ ] **Step 1: 실패하는 테스트 작성**

`internal/config/config_test.go` 끝에 추가:

```go
func TestMemoryRetentionDaysDefault(t *testing.T) {
	cfg, err := ParseConfig([]byte("version: \"0.1\"\n"))
	if err != nil {
		t.Fatalf("ParseConfig 실패: %v", err)
	}
	if got := cfg.Memory.RetentionDays["learning"]; got != 30 {
		t.Errorf("retention_days.learning 기본값 = %d, want 30", got)
	}
}

func TestMemoryRetentionDaysOverride(t *testing.T) {
	yml := "version: \"0.1\"\nmemory:\n  retention_days:\n    learning: 7\n    note: 14\n"
	cfg, err := ParseConfig([]byte(yml))
	if err != nil {
		t.Fatalf("ParseConfig 실패: %v", err)
	}
	if cfg.Memory.RetentionDays["learning"] != 7 || cfg.Memory.RetentionDays["note"] != 14 {
		t.Errorf("명시값이 적용되어야 한다: %v", cfg.Memory.RetentionDays)
	}
}

// 빈 맵을 명시하면 "모든 카테고리 영구 보존" opt-out이다 (기본값 미적용).
func TestMemoryRetentionDaysExplicitEmpty(t *testing.T) {
	yml := "version: \"0.1\"\nmemory:\n  retention_days: {}\n"
	cfg, err := ParseConfig([]byte(yml))
	if err != nil {
		t.Fatalf("ParseConfig 실패: %v", err)
	}
	if len(cfg.Memory.RetentionDays) != 0 {
		t.Errorf("빈 맵 명시는 보존 정책 해제여야 한다: %v", cfg.Memory.RetentionDays)
	}
}
```

- [ ] **Step 2: 테스트 실패 확인**

Run: `go test ./internal/config/ -run TestMemoryRetentionDays -race -count=1`
Expected: FAIL — `cfg.Memory.RetentionDays undefined` (컴파일 에러)

- [ ] **Step 3: 구현**

`internal/config/config.go` 91-94행:

```go
// MemoryConfig defines agent memory management settings.
// Spec Reference: Section 16 "memory"
type MemoryConfig struct {
	ProactiveInjection bool           `yaml:"proactive_injection"`
	ProactiveMaxTokens int            `yaml:"proactive_max_tokens"`
	// RetentionDays: 카테고리별 보존 일수. 미지정 카테고리·0 이하 = 영구 보존.
	// nil(설정 파일에 키 없음)이면 기본값 {"learning": 30}이 적용되고,
	// 명시적 빈 맵({})은 전체 영구 보존 opt-out이다.
	RetentionDays map[string]int `yaml:"retention_days"`
}
```

`applyDefaults` 347-352행의 Memory 블록:

```go
	// Memory defaults
	if cfg.Memory.ProactiveMaxTokens == 0 {
		cfg.Memory.ProactiveMaxTokens = 2000
	}
	if cfg.Memory.RetentionDays == nil {
		cfg.Memory.RetentionDays = map[string]int{"learning": 30}
	}
	// ProactiveInjection defaults to true,
	// handled by pre-initialization in ParseConfig.
```

- [ ] **Step 4: 테스트 통과 확인**

Run: `go test ./internal/config/ -race -count=1`
Expected: PASS (신규 3건 + 기존 전부)

- [ ] **Step 5: README 문서화**

`README.md` 416-417행의

```yaml
  proactive_injection: true
  proactive_max_tokens: 2000
```

바로 아래에 추가:

```yaml
  retention_days:        # 카테고리별 보존 일수 — 미지정/0 이하 = 영구 보존, {} = 전체 영구 보존
    learning: 30
```

- [ ] **Step 6: 커밋**

```bash
git add internal/config/config.go internal/config/config_test.go README.md
git commit -m "feat: 메모리 카테고리별 보존 정책 설정(retention_days) 추가"
```

---

### Task 4: `Store.PruneExpired`

**Files:**
- Modify: `internal/memory/store.go` (`StoreLearnings` 아래에 추가)
- Test: `internal/memory/store_test.go`

**Interfaces:**
- Consumes: 기존 `List`, `rebuildIndexLocked`, `removeEmptyCategoryDirs`, `fsutil.AcquireLock`, `s.Now`
- Produces: `func (s *Store) PruneExpired(project string, retentionDays map[string]int) (int64, error)` — Task 5가 소비. 삭제 건수 반환. `retentionDays`가 nil/빈 맵이면 no-op `(0, nil)`.

- [ ] **Step 1: 실패하는 테스트 작성**

`internal/memory/store_test.go` 끝에 추가 (파일 상단 import에 `time`은 이미 있음):

```go
// 카테고리별 보존 일수를 넘긴 항목은 삭제되고, 정책 없는 카테고리는 영구 보존된다.
func TestPruneExpiredDeletesOldEntries(t *testing.T) {
	s := newTestStore(t) // Now = 2026-07-23 12:00 UTC 고정
	mustInsert(t, s, &Entry{ProjectID: "app", Category: "learning", Key: "오래된 학습",
		Content: "32일 전에 저장된 학습 내용이라 만료 대상이다", Confidence: 0.8,
		CreatedAt: time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC)})
	mustInsert(t, s, &Entry{ProjectID: "app", Category: "learning", Key: "최근 학습",
		Content: "어제 저장된 학습이라 보존 기간 안에 있다", Confidence: 0.8,
		CreatedAt: time.Date(2026, 7, 22, 12, 0, 0, 0, time.UTC)})
	mustInsert(t, s, &Entry{ProjectID: "app", Category: "decision", Key: "오래된 결정",
		Content: "decision 카테고리는 보존 정책이 없으므로 영구 보존되어야 한다", Confidence: 0.9,
		CreatedAt: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)})

	n, err := s.PruneExpired("app", map[string]int{"learning": 30})
	if err != nil {
		t.Fatalf("PruneExpired 실패: %v", err)
	}
	if n != 1 {
		t.Fatalf("만료 1건만 삭제되어야 한다: %d", n)
	}
	entries, err := s.List("app")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Key == "오래된 학습" {
			t.Error("만료 항목이 남아 있다")
		}
	}
	if len(entries) != 2 {
		t.Errorf("최근 학습과 decision은 남아야 한다: %d건", len(entries))
	}
	// INDEX.md도 갱신되어야 한다
	index, err := os.ReadFile(filepath.Join(s.Root, ".pylon", "memory", "app", "INDEX.md"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(index), "오래된 학습") {
		t.Error("INDEX.md에서 만료 항목이 제거되어야 한다")
	}
}

// 0 이하의 보존 일수는 영구 보존을 뜻한다.
func TestPruneExpiredZeroMeansPermanent(t *testing.T) {
	s := newTestStore(t)
	mustInsert(t, s, &Entry{ProjectID: "app", Category: "learning", Key: "아주 오래된 학습",
		Content: "보존 일수가 0이면 아무리 오래돼도 지우지 않는다", Confidence: 0.8,
		CreatedAt: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)})
	n, err := s.PruneExpired("app", map[string]int{"learning": 0})
	if err != nil || n != 0 {
		t.Fatalf("0일 정책은 no-op여야 한다: n=%d, err=%v", n, err)
	}
}

// nil/빈 정책은 no-op이다.
func TestPruneExpiredNilPolicy(t *testing.T) {
	s := newTestStore(t)
	if n, err := s.PruneExpired("app", nil); err != nil || n != 0 {
		t.Fatalf("nil 정책은 (0, nil)이어야 한다: n=%d, err=%v", n, err)
	}
}
```

- [ ] **Step 2: 테스트 실패 확인**

Run: `go test ./internal/memory/ -run TestPruneExpired -race -count=1`
Expected: FAIL — `s.PruneExpired undefined` (컴파일 에러)

- [ ] **Step 3: 구현**

`internal/memory/store.go`의 `StoreLearnings` 함수 아래(`sanitizeKey` 위)에 추가:

```go
// PruneExpired deletes entries older than their category's retention window.
// retentionDays: 카테고리 → 보존 일수. 미지정 카테고리·0 이하 값은 영구 보존.
// 삭제 건수를 반환하며, 삭제가 있었으면 INDEX.md를 갱신한다.
func (s *Store) PruneExpired(project string, retentionDays map[string]int) (int64, error) {
	if len(retentionDays) == 0 {
		return 0, nil
	}
	entries, err := s.List(project)
	if err != nil {
		return 0, err
	}
	now := s.Now().UTC()
	var targets []Entry
	for _, e := range entries {
		days, ok := retentionDays[e.Category]
		if !ok || days <= 0 {
			continue
		}
		if now.Sub(e.CreatedAt) > time.Duration(days)*24*time.Hour {
			targets = append(targets, e)
		}
	}
	if len(targets) == 0 {
		return 0, nil
	}
	unlock, err := fsutil.AcquireLock(s.lockPath(), lockTimeout)
	if err != nil {
		return 0, err
	}
	defer unlock()
	for _, e := range targets {
		if err := os.Remove(filepath.Join(s.Root, filepath.FromSlash(e.Path))); err != nil && !os.IsNotExist(err) {
			return 0, err
		}
	}
	removeEmptyCategoryDirs(s.projectDir(project))
	return int64(len(targets)), s.rebuildIndexLocked(project)
}
```

- [ ] **Step 4: 테스트 통과 확인**

Run: `go test ./internal/memory/ -race -count=1`
Expected: PASS

- [ ] **Step 5: 커밋**

```bash
git add internal/memory/store.go internal/memory/store_test.go
git commit -m "feat: PruneExpired — 카테고리 보존 일수 기반 만료 메모리 삭제"
```

---

### Task 5: `sync-memory` 저장 후 자동 정리

**Files:**
- Modify: `internal/cli/sync_memory.go:66-115` (`runSyncFromSession`)
- Test: `internal/cli/sync_memory_test.go`

**Interfaces:**
- Consumes: `PruneExpired(project string, retentionDays map[string]int) (int64, error)` (Task 4), `cfg.Memory.RetentionDays` (Task 3), 기존 `openWorkspace() (string, *config.Config, error)`
- Produces: Stop hook 경로에서 저장 후 만료 항목 자동 정리. 정리 실패는 경고만 출력하고 저장 성공을 막지 않는다. JSON 출력에 `pruned` 필드 추가.

- [ ] **Step 1: 실패하는 테스트 작성**

`internal/cli/sync_memory_test.go` 끝에 추가 (import에 `time` 추가 필요):

```go
// Stop hook 경로는 학습 저장 후 만료 항목을 자동 정리한다 (기본 learning 30일).
func TestSyncFromSessionPrunesExpired(t *testing.T) {
	root := setupTestWorkspace(t)

	prev := flagWorkspace
	flagWorkspace = root
	defer func() { flagWorkspace = prev }()

	s := memory.NewStore(root)
	if err := s.Insert(&memory.Entry{ProjectID: "app", Category: "learning",
		Key: "오래된 학습", Content: "31일이 지나 만료 정리 대상인 오래된 학습 내용",
		Confidence: 0.8, CreatedAt: time.Now().UTC().Add(-31 * 24 * time.Hour)}); err != nil {
		t.Fatalf("사전 저장 실패: %v", err)
	}

	if err := runSyncFromSession("app", "claude", "- 새로 들어온 학습 내용"); err != nil {
		t.Fatalf("sync 실패: %v", err)
	}

	entries, err := s.ListByCategory("app", "learning")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Key == "오래된 학습" {
			t.Error("30일 지난 learning 항목은 자동 정리되어야 한다")
		}
	}
}
```

- [ ] **Step 2: 테스트 실패 확인**

Run: `go test ./internal/cli/ -run TestSyncFromSessionPrunesExpired -race -count=1`
Expected: FAIL — "30일 지난 learning 항목은 자동 정리되어야 한다"

- [ ] **Step 3: 구현**

`internal/cli/sync_memory.go`의 `runSyncFromSession`에서:

67행 `root, _, err := openWorkspace()` → `root, cfg, err := openWorkspace()` 로 변경.

96-98행 `StoreLearnings` 호출 직후에 추가:

```go
	if err := memStore.StoreLearnings(project, taskID, agent, learnings); err != nil {
		return fmt.Errorf("학습 내용 저장 실패: %w", err)
	}

	// 저장 위생: 보존 기간이 지난 항목을 함께 정리한다. 정리 실패는 저장
	// 성공을 막지 않는다 (Stop hook이 실패하면 세션 흐름을 방해하므로).
	pruned, pruneErr := memStore.PruneExpired(project, cfg.Memory.RetentionDays)
	if pruneErr != nil {
		fmt.Fprintf(os.Stderr, "경고: 만료 메모리 정리 실패: %v\n", pruneErr)
	}
```

100-112행의 출력 블록에 `pruned` 반영:

```go
	if flagJSON {
		data, _ := json.Marshal(map[string]any{
			"status":   "ok",
			"project":  project,
			"agent":    agent,
			"task_id":  taskID,
			"count":    len(learnings),
			"category": "learning",
			"pruned":   pruned,
		})
		fmt.Println(string(data))
	} else {
		fmt.Printf("✓ %d개 학습 내용을 %s 프로젝트에 저장했습니다 (task: %s)\n", len(learnings), project, taskID)
		if pruned > 0 {
			fmt.Printf("✓ 보존 기간이 지난 메모리 %d건을 정리했습니다\n", pruned)
		}
	}
```

- [ ] **Step 4: 패키지 테스트 통과 확인**

Run: `go test ./internal/cli/ -race -count=1`
Expected: PASS — 신규 1건 + 기존 sync/워크스페이스 테스트 전부 (`setupTestWorkspace`의 `version: "0.1"` config는 `LoadConfig`→`ParseConfig` 기본값 경로로 `RetentionDays{"learning":30}`을 받는다)

- [ ] **Step 5: 커밋**

```bash
git add internal/cli/sync_memory.go internal/cli/sync_memory_test.go
git commit -m "feat: sync-memory 저장 후 만료 메모리 자동 정리"
```

---

### Task 6: 주입 전용 렌더러 `InjectionMarkdown` (+`IndexMarkdown` 삭제)

**Files:**
- Modify: `internal/memory/index.go` (`indexLine` 추출, `InjectionMarkdown` 신설, `IndexMarkdown` 삭제, `bytes` import 제거)
- Test: `internal/memory/store_test.go` (`TestIndexMarkdownTruncation`(278-298행) 삭제 후 신규 테스트로 대체)

**Interfaces:**
- Consumes: 기존 `List`, `Entry`
- Produces:
  - `func (s *Store) InjectionMarkdown(project string, maxBytes int) (string, error)` — Task 7이 소비. confidence desc → CreatedAt desc 정렬, `#### <project>` 헤더 + 항목 라인, 줄 단위 절단(`…(생략)` 포함 예산 내). 항목이 없거나 예산이 한 줄도 못 담으면 `""`.
  - `func indexLine(e Entry) string` — `rebuildIndexLocked`와 `InjectionMarkdown`이 공유하는 라인 렌더러 (포맷: `- [category] \`key\` — summary (conf, date)\n`, 요약 100룬 절단 — 기존 `rebuildIndexLocked` 포맷과 동일).
  - **삭제:** `IndexMarkdown` (유일한 프로덕션 소비자가 `launch_claudemd.go:99`였고 Task 7에서 교체됨. Task 6→7 사이에 빌드가 깨지지 않도록 삭제는 Task 7에서 수행한다 — 이 태스크에서는 신설만.)

- [ ] **Step 1: 실패하는 테스트 작성**

`internal/memory/store_test.go` 끝에 추가:

```go
// 주입 출력은 confidence 우선, 동률이면 최신 우선으로 정렬된다 —
// 예산 절단 시 카테고리 알파벳순이 아니라 중요도 낮은 항목부터 떨어진다.
func TestInjectionMarkdownRanksByConfidenceThenRecency(t *testing.T) {
	s := newTestStore(t)
	mustInsert(t, s, &Entry{ProjectID: "app", Category: "learning", Key: "낮은 확신 항목",
		Content: "확신도가 낮아 주입 순위에서 뒤로 밀려야 하는 항목", Confidence: 0.5,
		CreatedAt: time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC)})
	mustInsert(t, s, &Entry{ProjectID: "app", Category: "z-category", Key: "높은 확신 항목",
		Content: "카테고리 알파벳순으로는 마지막이지만 확신도가 높아 먼저 나와야 한다", Confidence: 0.9,
		CreatedAt: time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)})

	out, err := s.InjectionMarkdown("app", 0)
	if err != nil {
		t.Fatalf("InjectionMarkdown 실패: %v", err)
	}
	hi := strings.Index(out, "높은 확신 항목")
	lo := strings.Index(out, "낮은 확신 항목")
	if hi < 0 || lo < 0 {
		t.Fatalf("두 항목 모두 포함되어야 한다:\n%s", out)
	}
	if hi > lo {
		t.Errorf("높은 확신 항목이 먼저 나와야 한다:\n%s", out)
	}
	if !strings.Contains(out, "#### app") {
		t.Errorf("프로젝트 헤더가 있어야 한다:\n%s", out)
	}
}

// 예산 절단은 줄 단위이며, 절단 시 "…(생략)"을 표기한다.
func TestInjectionMarkdownTruncatesWholeLines(t *testing.T) {
	s := newTestStore(t)
	mustInsert(t, s, &Entry{ProjectID: "app", Category: "learning", Key: "우선 항목",
		Content: "확신도가 높아 절단 후에도 살아남아야 하는 항목", Confidence: 0.9})
	mustInsert(t, s, &Entry{ProjectID: "app", Category: "learning", Key: "후순위 항목",
		Content: "확신도가 낮아 좁은 예산에서는 잘려 나가야 하는 항목", Confidence: 0.3})

	full, err := s.InjectionMarkdown("app", 0)
	if err != nil {
		t.Fatalf("전체 출력 실패: %v", err)
	}
	// 한 줄만 담길 만큼의 예산: 헤더 + 첫 줄 + 생략 표기
	capped, err := s.InjectionMarkdown("app", len(full)-10)
	if err != nil {
		t.Fatalf("절단 출력 실패: %v", err)
	}
	if !strings.Contains(capped, "우선 항목") {
		t.Errorf("우선 항목은 살아남아야 한다:\n%s", capped)
	}
	if strings.Contains(capped, "후순위 항목") {
		t.Errorf("후순위 항목은 잘려야 한다:\n%s", capped)
	}
	if !strings.Contains(capped, "…(생략)") {
		t.Errorf("절단 표기가 있어야 한다:\n%s", capped)
	}
}

// 예산이 한 줄도 못 담으면 빈 문자열을 반환해 다음 프로젝트에 예산을 양보한다.
func TestInjectionMarkdownTinyBudgetYields(t *testing.T) {
	s := newTestStore(t)
	mustInsert(t, s, &Entry{ProjectID: "app", Category: "learning", Key: "항목",
		Content: "예산이 너무 작으면 아예 출력하지 않는 편이 낫다", Confidence: 0.8})
	out, err := s.InjectionMarkdown("app", 20)
	if err != nil || out != "" {
		t.Fatalf("초소형 예산은 빈 출력이어야 한다: %q, err=%v", out, err)
	}
}

// 존재하지 않는 프로젝트는 빈 문자열을 반환한다.
func TestInjectionMarkdownGhostProject(t *testing.T) {
	s := newTestStore(t)
	if out, err := s.InjectionMarkdown("ghost", 100); err != nil || out != "" {
		t.Fatalf("빈 프로젝트: %q, err=%v", out, err)
	}
}
```

- [ ] **Step 2: 테스트 실패 확인**

Run: `go test ./internal/memory/ -run TestInjectionMarkdown -race -count=1`
Expected: FAIL — `s.InjectionMarkdown undefined` (컴파일 에러)

- [ ] **Step 3: 구현**

`internal/memory/index.go`에서:

(a) `rebuildIndexLocked`의 38-45행(라인 렌더링 루프)을 교체하고 `indexLine`으로 추출:

```go
	// 변경 전 (rebuildIndexLocked 내부)
	for _, e := range entries {
		summary := strings.Join(strings.Fields(e.Content), " ")
		if runes := []rune(summary); len(runes) > 100 {
			summary = string(runes[:100]) + "…"
		}
		fmt.Fprintf(&b, "- [%s] `%s` — %s (%.1f, %s)\n",
			e.Category, e.Key, summary, e.Confidence, e.CreatedAt.Format("2006-01-02"))
	}
```

```go
	// 변경 후 (rebuildIndexLocked 내부)
	for _, e := range entries {
		b.WriteString(indexLine(e))
	}
```

(b) 파일 끝에 추가:

```go
// indexLine renders one entry as an index/injection line.
// INDEX.md와 주입 출력이 같은 포맷을 공유한다.
func indexLine(e Entry) string {
	summary := strings.Join(strings.Fields(e.Content), " ")
	if runes := []rune(summary); len(runes) > 100 {
		summary = string(runes[:100]) + "…"
	}
	return fmt.Sprintf("- [%s] `%s` — %s (%.1f, %s)\n",
		e.Category, e.Key, summary, e.Confidence, e.CreatedAt.Format("2006-01-02"))
}

// InjectionMarkdown renders a project's entries for proactive injection,
// ranked by confidence then recency. INDEX.md(카테고리순, 사람용)와 달리
// 예산 절단이 중요도 낮은 항목부터 떨어뜨리도록 정렬한다. 예산이 한 줄도
// 담지 못하면 빈 문자열을 반환해 호출자가 예산을 다음 프로젝트로 넘기게 한다.
func (s *Store) InjectionMarkdown(project string, maxBytes int) (string, error) {
	entries, err := s.List(project)
	if err != nil || len(entries) == 0 {
		return "", err
	}
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].Confidence != entries[j].Confidence {
			return entries[i].Confidence > entries[j].Confidence
		}
		return entries[i].CreatedAt.After(entries[j].CreatedAt)
	})
	const ellipsis = "…(생략)\n"
	var b strings.Builder
	fmt.Fprintf(&b, "#### %s\n\n", project)
	wrote := 0
	truncated := false
	for _, e := range entries {
		line := indexLine(e)
		if maxBytes > 0 && b.Len()+len(line)+len(ellipsis) > maxBytes {
			truncated = true
			break
		}
		b.WriteString(line)
		wrote++
	}
	if wrote == 0 {
		return "", nil
	}
	if truncated {
		b.WriteString(ellipsis)
	}
	return b.String(), nil
}
```

주의: `List`는 category→key 정렬을 반환하므로 `sort.SliceStable`로 동률 시 결정적 순서가 유지된다. `IndexMarkdown`은 이 태스크에서 아직 삭제하지 않는다 (launch_claudemd.go:99가 여전히 호출 — Task 7에서 함께 제거).

- [ ] **Step 4: 테스트 통과 확인**

Run: `go test ./internal/memory/ -race -count=1`
Expected: PASS — 신규 4건 + 기존 `TestIndexMarkdownTruncation`(아직 존재) 포함 전부

- [ ] **Step 5: 커밋**

```bash
git add internal/memory/index.go internal/memory/store_test.go
git commit -m "feat: 주입 전용 InjectionMarkdown — confidence·recency 순 절단"
```

---

### Task 7: 주입 공정 예산 분배 + `IndexMarkdown` 제거 + 최종 검증

**Files:**
- Modify: `internal/cli/launch_claudemd.go:86-111` (주입 loop)
- Modify: `internal/memory/index.go` (`IndexMarkdown` 삭제, `bytes`·`os` import 정리)
- Modify: `internal/memory/store_test.go` (`TestIndexMarkdownTruncation` 278-298행 삭제)
- Test: `internal/cli/launch_claudemd_test.go`

**Interfaces:**
- Consumes: `InjectionMarkdown(project string, maxBytes int) (string, error)` (Task 6)
- Produces: `buildRootCLAUDEMD`가 잔여 예산을 남은 프로젝트 수로 나눈 공정 분배(fair-share with carryover)로 주입. 미사용 예산은 뒤 프로젝트로 이월.

- [ ] **Step 1: 실패하는 테스트 작성**

`internal/cli/launch_claudemd_test.go`에 추가 (import에 `fmt` 추가 필요):

```go
// 여러 프로젝트가 예산을 공유할 때, 앞 프로젝트의 큰 인덱스가 예산을
// 독식해 뒤 프로젝트가 0바이트를 받는 굶주림이 없어야 한다.
func TestBuildRootCLAUDEMDFairShareAcrossProjects(t *testing.T) {
	root := t.TempDir()
	s := memory.NewStore(root)
	// aaa: 기본 예산(2000토큰×4=8000바이트)을 혼자 초과할 만큼의 필러 항목.
	// 근사 중복 감지에 걸리지 않도록 항목마다 뚜렷이 다른 토큰을 반복한다.
	// (동률 confidence는 recency desc로 정렬되므로 필러의 삽입 순서에 의존한
	// 단정은 하지 않는다 — 상위 노출 검증은 아래 고확신 항목이 담당한다.)
	for i := 0; i < 60; i++ {
		e := &memory.Entry{ProjectID: "aaa", Category: "learning",
			Key:        fmt.Sprintf("항목-%02d", i),
			Content:    strings.Repeat(fmt.Sprintf("학습토큰%02d ", i), 12),
			Confidence: 0.5}
		if err := s.Insert(e); err != nil {
			t.Fatalf("Insert(%d) 실패: %v", i, err)
		}
	}
	// aaa의 고확신 항목: 절단 후에도 반드시 살아남아야 한다 (confidence 우선 정렬)
	if err := s.Insert(&memory.Entry{ProjectID: "aaa", Category: "learning",
		Key: "최상위 항목", Content: "확신도가 가장 높아 절단 후에도 살아남아야 하는 핵심 지식",
		Confidence: 0.95}); err != nil {
		t.Fatalf("최상위 항목 Insert 실패: %v", err)
	}
	if err := s.Insert(&memory.Entry{ProjectID: "bbb", Category: "decision",
		Key: "굶주림 방지 확인용", Content: "뒤 프로젝트도 예산을 배정받아 주입되어야 한다",
		Confidence: 0.9}); err != nil {
		t.Fatalf("bbb Insert 실패: %v", err)
	}

	cfg := &config.Config{}
	cfg.Memory.ProactiveInjection = true
	projects := []config.ProjectInfo{
		{Name: "aaa", Path: filepath.Join(root, "aaa")},
		{Name: "bbb", Path: filepath.Join(root, "bbb")},
	}

	out := buildRootCLAUDEMD(cfg, projects, root)
	if !strings.Contains(out, "굶주림 방지 확인용") {
		t.Error("뒤 프로젝트가 예산에서 굶으면 안 된다 (공정 분배)")
	}
	if !strings.Contains(out, "최상위 항목") {
		t.Error("앞 프로젝트의 고확신 항목은 절단 후에도 주입되어야 한다")
	}
}
```

- [ ] **Step 2: 테스트 실패 확인**

Run: `go test ./internal/cli/ -run TestBuildRootCLAUDEMDFairShare -race -count=1`
Expected: FAIL — "뒤 프로젝트가 예산에서 굶으면 안 된다 (공정 분배)" (현재 공유 풀 + aaa 인덱스 8000바이트 초과 → bbb는 0)

- [ ] **Step 3: 구현**

`internal/cli/launch_claudemd.go` 86-111행 전체 교체:

```go
	// Proactive memory index injection — 잔여 예산을 남은 프로젝트 수로 나눠
	// 공정 분배한다(앞 프로젝트의 독식 방지). 미사용분은 뒤 프로젝트로 이월.
	if cfg.Memory.ProactiveInjection {
		maxTokens := cfg.Memory.ProactiveMaxTokens
		if maxTokens <= 0 {
			maxTokens = 2000
		}
		remaining := maxTokens * 4 // 대략적인 토큰→바이트 환산
		memStore := memory.NewStore(root)
		wroteHeader := false
		for i, p := range projects {
			if remaining <= 0 {
				break
			}
			share := remaining / (len(projects) - i)
			index, err := memStore.InjectionMarkdown(p.Name, share)
			if err != nil || index == "" {
				continue
			}
			if !wroteHeader {
				b.WriteString("### 메모리 인덱스\n\n")
				wroteHeader = true
			}
			b.WriteString(index)
			b.WriteString("\n")
			remaining -= len(index)
		}
	}
```

- [ ] **Step 4: `IndexMarkdown` 제거**

이제 프로덕션 소비자가 없다. `internal/memory/index.go`에서 `IndexMarkdown` 함수(49-67행)를 삭제하고, 그 함수만 쓰던 `bytes` import를 제거한다 (`os`는 `rebuildIndexLocked`의 `os.Remove`가 계속 사용하므로 유지). `internal/memory/store_test.go`의 `TestIndexMarkdownTruncation`(278-298행)을 삭제한다 (역할은 Task 6의 `TestInjectionMarkdownTruncatesWholeLines`가 대체).

Run: `go build ./...`
Expected: 컴파일 성공, `IndexMarkdown` 참조 없음 (`grep -rn "IndexMarkdown" --include="*.go" .` 결과 0건)

- [ ] **Step 5: 전체 테스트·린트 통과 확인**

Run: `make test && make lint`
Expected: 전체 PASS, 린트 클린. 기존 `TestBuildRootCLAUDEMDInjectsMemoryIndex`(주입 on/off)도 그대로 통과해야 한다.

- [ ] **Step 6: 커밋**

```bash
git add internal/cli/launch_claudemd.go internal/cli/launch_claudemd_test.go internal/memory/index.go internal/memory/store_test.go
git commit -m "feat: 메모리 주입 공정 예산 분배 — 프로젝트 굶주림 제거, IndexMarkdown 대체"
```

---

## 검증 시나리오 (전체 완료 후)

1. `make build` 후 임시 워크스페이스에서 `pylon init` → learning 항목 여러 개 저장(`pylon mem store ... --category learning`) → 생성된 `CLAUDE.md`의 "### 메모리 인덱스"가 confidence 순으로 정렬돼 있는지 확인.
2. 같은 내용을 표현만 바꿔 `pylon mem store` 2회 → 두 번째가 "동일한 내용이 이미 저장되어 있어 건너뜁니다"로 스킵되는지 확인.
3. `config.yml`에 `memory.retention_days: {learning: 0}` 설정 → `pylon sync-memory --from-session`이 아무것도 정리하지 않는지 확인.

## 명시적 비범위 (이 계획에서 하지 않는 것)

- 검색(`mem search`)의 한국어 토크나이저 개선 — 별도 계획 (NFC + unigram/bigram + 기존 질의 커버리지 랭커 유지).
- `INDEX.md` 포맷/정렬 변경 — git 추적 사람용 파일이므로 유지.
- frontmatter 관계 키(`related`/`supersedes`) — 수요 확인 후 별도 결정.
- 요약 압축(오래된 항목 LLM 요약) — 보존 정책·근사 중복만으로 부족하다고 관측될 때 재검토.
