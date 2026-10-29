// internal/memory/store_test.go
package memory

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s := NewStore(t.TempDir())
	s.Now = func() time.Time { return time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC) }
	return s
}

// mustInsert stores a setup entry, failing the test on any error.
func mustInsert(t *testing.T, s *Store, e *Entry) {
	t.Helper()
	if err := s.Insert(e); err != nil {
		t.Fatalf("Insert 실패: %v", err)
	}
}

func TestInsertWritesMarkdownWithFrontmatter(t *testing.T) {
	s := newTestStore(t)
	e := &Entry{ProjectID: "app", Category: "learning", Key: "빌드 시 CGO 비활성화",
		Content: "이 프로젝트는 CGO_ENABLED=0으로 빌드해야 한다", Author: "claude", Confidence: 0.9}
	if err := s.Insert(e); err != nil {
		t.Fatalf("Insert 실패: %v", err)
	}
	if e.Path == "" {
		t.Fatal("Insert 후 Path가 채워져야 한다")
	}
	data, err := os.ReadFile(filepath.Join(s.Root, filepath.FromSlash(e.Path)))
	if err != nil {
		t.Fatalf("파일 읽기 실패: %v", err)
	}
	text := string(data)
	for _, want := range []string{"---\n", "category: learning", "key: 빌드 시 CGO 비활성화", "confidence: 0.9", "CGO_ENABLED=0"} {
		if !strings.Contains(text, want) {
			t.Errorf("파일에 %q가 없습니다:\n%s", want, text)
		}
	}
	// INDEX.md 자동 생성
	index, err := os.ReadFile(filepath.Join(s.Root, ".pylon", "memory", "app", "INDEX.md"))
	if err != nil {
		t.Fatalf("INDEX.md가 생성되어야 한다: %v", err)
	}
	if !strings.Contains(string(index), "빌드 시 CGO 비활성화") {
		t.Errorf("인덱스에 키가 없습니다:\n%s", index)
	}
}

func TestInsertRejectsInvalidInput(t *testing.T) {
	s := newTestStore(t)
	cases := []Entry{
		{ProjectID: "../evil", Category: "learning", Key: "k", Content: "c", Confidence: 0.5},
		{ProjectID: "app", Category: "a/b", Key: "k", Content: "c", Confidence: 0.5},
		{ProjectID: "app", Category: "learning", Key: "", Content: "c", Confidence: 0.5},
		{ProjectID: "app", Category: "learning", Key: "k", Content: "c", Confidence: 1.5},
	}
	for i, e := range cases {
		if err := s.Insert(&e); err == nil {
			t.Errorf("case %d: 에러가 발생해야 한다", i)
		}
	}
}

// 같은 key로 다시 저장하면 기존 파일을 덮어쓴다(경로 유지, CreatedAt 갱신).
func TestInsertUpsertsSameKey(t *testing.T) {
	s := newTestStore(t)
	first := &Entry{ProjectID: "app", Category: "learning", Key: "같은 키", Content: "처음 저장한 내용", Confidence: 0.8}
	mustInsert(t, s, first)
	if first.Updated {
		t.Error("신규 저장은 Updated=false여야 한다")
	}

	later := time.Date(2026, 7, 24, 9, 0, 0, 0, time.UTC)
	s.Now = func() time.Time { return later }
	second := &Entry{ProjectID: "app", Category: "learning", Key: "같은 키", Content: "완전히 바뀐 새 내용", Confidence: 0.9}
	mustInsert(t, s, second)
	if !second.Updated {
		t.Error("같은 key 재저장은 Updated=true여야 한다")
	}
	if second.Path != first.Path {
		t.Errorf("경로가 유지되어야 한다: %q != %q", second.Path, first.Path)
	}
	entries, err := s.List("app")
	if err != nil || len(entries) != 1 {
		t.Fatalf("1건만 남아야 한다: %d, err=%v", len(entries), err)
	}
	if entries[0].Content != "완전히 바뀐 새 내용" || entries[0].Confidence != 0.9 {
		t.Errorf("내용이 갱신되어야 한다: %+v", entries[0])
	}
	if !entries[0].CreatedAt.Equal(later) {
		t.Errorf("CreatedAt은 갱신 시각이어야 한다: %v", entries[0].CreatedAt)
	}
}

// 자기 자신의 소폭 수정은 근사 중복으로 거부되지 않는다.
func TestInsertUpsertAllowsSmallSelfEdit(t *testing.T) {
	s := newTestStore(t)
	mustInsert(t, s, &Entry{ProjectID: "app", Category: "learning", Key: "race 플래그",
		Content: "테스트는 race 플래그를 켜고 실행해야 한다", Confidence: 0.8})
	e := &Entry{ProjectID: "app", Category: "learning", Key: "race 플래그",
		Content: "테스트는 race 플래그를 켜고 실행해야 한다는 것", Confidence: 0.8}
	if err := s.Insert(e); err != nil {
		t.Fatalf("자기 수정은 거부되면 안 된다: %v", err)
	}
	entries, _ := s.List("app")
	if len(entries) != 1 || entries[0].Content != e.Content {
		t.Fatalf("1건이 수정된 내용으로 남아야 한다: %+v", entries)
	}
}

// key는 다르지만 슬러그가 같으면 -2, -3 접미로 따로 저장되고, 접미 파일의
// key로 다시 저장하면 그 파일이 갱신된다.
func TestInsertResolvesFileNameCollision(t *testing.T) {
	s := newTestStore(t)
	keys := []string{"Same Key", "same key", "same-key"}
	for i, content := range []string{"내용 하나", "내용 둘", "내용 셋"} {
		mustInsert(t, s, &Entry{ProjectID: "app", Category: "learning", Key: keys[i], Content: content, Confidence: 0.8})
	}
	entries, err := s.List("app")
	if err != nil || len(entries) != 3 {
		t.Fatalf("3건이 저장되어야 한다: %d, err=%v", len(entries), err)
	}
	e := &Entry{ProjectID: "app", Category: "learning", Key: "same key", Content: "내용 둘 수정", Confidence: 0.8}
	mustInsert(t, s, e)
	if !strings.HasSuffix(e.Path, "/same-key-2.md") || !e.Updated {
		t.Errorf("same-key-2.md가 갱신되어야 한다: %q updated=%v", e.Path, e.Updated)
	}
	if entries, _ := s.List("app"); len(entries) != 3 {
		t.Fatalf("갱신 후에도 3건이어야 한다: %d", len(entries))
	}
}

func TestInsertSkipsDuplicateContent(t *testing.T) {
	s := newTestStore(t)
	e1 := &Entry{ProjectID: "app", Category: "learning", Key: "k1", Content: "같은 내용", Confidence: 0.8}
	if err := s.Insert(e1); err != nil {
		t.Fatalf("첫 저장 실패: %v", err)
	}
	// 키가 달라도 같은 project+category+content면 스킵 (D4)
	e2 := &Entry{ProjectID: "app", Category: "learning", Key: "k2", Content: "같은 내용", Confidence: 0.8}
	if err := s.Insert(e2); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("동일 내용은 ErrDuplicate여야 한다: %v", err)
	}
	if e2.Path != e1.Path {
		t.Errorf("중복 시 기존 파일 경로가 설정되어야 한다: %q != %q", e2.Path, e1.Path)
	}
	// 다른 카테고리면 저장된다
	e3 := &Entry{ProjectID: "app", Category: "decision", Key: "k3", Content: "같은 내용", Confidence: 0.8}
	if err := s.Insert(e3); err != nil {
		t.Fatalf("다른 카테고리는 저장되어야 한다: %v", err)
	}
	if entries, _ := s.List("app"); len(entries) != 2 {
		t.Fatalf("2건이어야 한다: %d", len(entries))
	}
}

// 후행 개행이 붙은 content도 중복으로 인식되어야 한다 (D4).
// marshal(TrimRight) / parse(TrimSpace) / 비교(원본) 정규화가 어긋나면
// 같은 내용이 자기 자신과도 매칭되지 않아 새 파일이 쌓인다.
func TestInsertSkipsDuplicateWithTrailingWhitespace(t *testing.T) {
	s := newTestStore(t)
	for i, key := range []string{"k1", "k2", "k3"} {
		e := &Entry{ProjectID: "app", Category: "learning", Key: key, Content: "반복 내용\n", Confidence: 0.8}
		err := s.Insert(e)
		if i == 0 && err != nil {
			t.Fatalf("첫 저장 실패: %v", err)
		}
		if i > 0 && !errors.Is(err, ErrDuplicate) {
			t.Fatalf("%d번째: 후행 개행 content는 중복이어야 한다: %v", i, err)
		}
	}
	entries, _ := s.ListByCategory("app", "learning")
	if len(entries) != 1 {
		t.Fatalf("1건만 저장되어야 한다: %d건", len(entries))
	}
}

// 정규화가 선행 들여쓰기를 훼손하면 안 된다 — 들여쓴 코드 조각이 저장 대상이다.
func TestInsertPreservesLeadingIndentation(t *testing.T) {
	s := newTestStore(t)
	const padded = "    indented code\n    second line"
	mustInsert(t, s, &Entry{ProjectID: "app", Category: "learning", Key: "k", Content: padded, Confidence: 0.8})

	entries, err := s.List("app")
	if err != nil || len(entries) != 1 {
		t.Fatalf("1건 조회: %d, err=%v", len(entries), err)
	}
	if entries[0].Content != padded {
		t.Errorf("들여쓰기가 보존되어야 한다: wrote %q, read %q", padded, entries[0].Content)
	}
	// 들여쓴 동일 내용도 중복으로 인식되어야 한다 (D4).
	if err := s.Insert(&Entry{ProjectID: "app", Category: "learning", Key: "k2", Content: padded, Confidence: 0.8}); !errors.Is(err, ErrDuplicate) {
		t.Errorf("동일 들여쓰기 content는 중복이어야 한다: %v", err)
	}
}

// 손상된 .md 파일 1개가 프로젝트 메모리 전체를 마비시키면 안 된다.
// .pylon/memory/는 git 추적 대상이므로 병합 충돌 마커는 예상 가능한 상태다 (D1).
func TestListSkipsCorruptFiles(t *testing.T) {
	s := newTestStore(t)
	mustInsert(t, s, &Entry{ProjectID: "app", Category: "learning", Key: "정상", Content: "정상 내용", Confidence: 0.8})

	corrupt := filepath.Join(s.Root, ".pylon", "memory", "app", "learning", "conflict.md")
	if err := os.WriteFile(corrupt, []byte("<<<<<<< HEAD\n앞\n=======\n뒤\n>>>>>>> branch\n"), 0644); err != nil {
		t.Fatal(err)
	}

	entries, err := s.List("app")
	if err != nil {
		t.Fatalf("손상 파일이 있어도 List는 성공해야 한다: %v", err)
	}
	if len(entries) != 1 || entries[0].Key != "정상" {
		t.Fatalf("정상 항목은 계속 조회되어야 한다: %+v", entries)
	}
	// 손상 파일이 있어도 저장은 계속 가능해야 한다(자가 복구 가능).
	mustInsert(t, s, &Entry{ProjectID: "app", Category: "learning", Key: "추가", Content: "추가 내용", Confidence: 0.8})
}

func TestSearchKoreanSubstring(t *testing.T) {
	s := newTestStore(t)
	mustInsert(t, s, &Entry{ProjectID: "app", Category: "learning", Key: "메모리 관리",
		Content: "메모리를 파일로 관리한다", Confidence: 0.8})
	mustInsert(t, s, &Entry{ProjectID: "app", Category: "decision", Key: "무관한 항목",
		Content: "전혀 다른 내용", Confidence: 0.9})

	// 조사(助詞)가 붙은 형태("메모리를")도 부분 문자열로 매치되어야 한다
	results, err := s.Search("app", "메모리", 10)
	if err != nil {
		t.Fatalf("Search 실패: %v", err)
	}
	if len(results) != 1 || results[0].Key != "메모리 관리" {
		t.Fatalf("한국어 부분 일치 검색 실패: %+v", results)
	}
	if results[0].Rank != 1.0 {
		t.Errorf("모든 토큰 일치 시 Rank는 1.0이어야 한다: %f", results[0].Rank)
	}
}

func TestSearchRanksByMatchedTokens(t *testing.T) {
	s := newTestStore(t)
	mustInsert(t, s, &Entry{ProjectID: "app", Category: "learning", Key: "a", Content: "빌드 캐시 설정", Confidence: 0.8})
	mustInsert(t, s, &Entry{ProjectID: "app", Category: "learning", Key: "b", Content: "빌드만 언급", Confidence: 0.8})

	results, err := s.Search("app", "빌드 캐시", 10)
	if err != nil || len(results) != 2 {
		t.Fatalf("2건이 나와야 한다: %v, err=%v", results, err)
	}
	if results[0].Key != "a" {
		t.Errorf("더 많은 토큰이 일치한 항목이 먼저 와야 한다: %+v", results)
	}
}

func TestDeleteByKeyAndCategory(t *testing.T) {
	s := newTestStore(t)
	mustInsert(t, s, &Entry{ProjectID: "app", Category: "learning", Key: "k1", Content: "c1", Confidence: 0.8})
	mustInsert(t, s, &Entry{ProjectID: "app", Category: "learning", Key: "k2", Content: "c2", Confidence: 0.8})
	mustInsert(t, s, &Entry{ProjectID: "app", Category: "decision", Key: "k3", Content: "c3", Confidence: 0.8})

	// dry-run은 삭제하지 않는다
	n, err := s.Delete("app", "learning", "", true)
	if err != nil || n != 2 {
		t.Fatalf("dry-run 카운트: %d, err=%v", n, err)
	}
	if entries, _ := s.List("app"); len(entries) != 3 {
		t.Fatal("dry-run이 실제로 삭제하면 안 된다")
	}

	n, err = s.Delete("app", "", "k3", false)
	if err != nil || n != 1 {
		t.Fatalf("키 삭제: %d, err=%v", n, err)
	}
	n, err = s.Delete("app", "learning", "", false)
	if err != nil || n != 2 {
		t.Fatalf("카테고리 삭제: %d, err=%v", n, err)
	}
	if entries, _ := s.List("app"); len(entries) != 0 {
		t.Fatalf("모두 삭제되어야 한다: %d건 남음", len(entries))
	}
}

func TestDeleteAcrossAllProjects(t *testing.T) {
	s := newTestStore(t)
	mustInsert(t, s, &Entry{ProjectID: "app1", Category: "change", Key: "k", Content: "c", Confidence: 0.8})
	mustInsert(t, s, &Entry{ProjectID: "app2", Category: "change", Key: "k", Content: "c", Confidence: 0.8})
	n, err := s.Delete("", "change", "", false)
	if err != nil || n != 2 {
		t.Fatalf("전체 프로젝트 삭제: %d, err=%v", n, err)
	}
}

// 같은 교훈의 재진술을 거르기 위해, 바이트 동일이 아니어도 근사 중복이면
// 스킵되어야 한다 (D4 확장).
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

func TestPruneExpiredSkipsZeroCreatedAt(t *testing.T) {
	s := newTestStore(t)
	// created_at 없는 항목을 직접 파일로 쓴다 (parseEntry는 이를 손상으로 보지 않고
	// CreatedAt=zero로 둔다). 정책이 있어도 나이를 몰라 삭제되면 안 된다.
	dir := filepath.Join(s.projectDir("app"), "learning")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	body := "---\ncategory: learning\nkey: 날짜없는 항목\nconfidence: 0.8\n---\n\ncreated_at 없이 손으로 작성한 메모리 항목\n"
	if err := os.WriteFile(filepath.Join(dir, "no-date.md"), []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	n, err := s.PruneExpired("app", map[string]int{"learning": 30})
	if err != nil {
		t.Fatalf("PruneExpired 실패: %v", err)
	}
	if n != 0 {
		t.Fatalf("created_at 결측 항목은 삭제되면 안 된다: %d건 삭제됨", n)
	}
}

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

// 인덱스 요약은 본문의 첫 비어 있지 않은 줄이다.
func TestIndexLineUsesFirstNonEmptyLine(t *testing.T) {
	e := Entry{Category: "decision", Key: "k", Confidence: 0.8, Path: ".pylon/memory/app/decision/k-2.md",
		Content: "\n  첫 줄 요약  \n\n둘째 줄 상세 설명", CreatedAt: time.Date(2026, 7, 23, 0, 0, 0, 0, time.UTC)}
	got := indexLine(e)
	if got != "- [decision] `k` — 첫 줄 요약 (0.8, 2026-07-23) → app/decision/k-2.md\n" {
		t.Errorf("첫 줄이 요약이어야 한다: %q", got)
	}
	if strings.Contains(got, "둘째 줄") {
		t.Errorf("둘째 줄 이후는 요약에 들어가면 안 된다: %q", got)
	}
}

// 첫 줄이 200룬을 넘으면 200룬에서 자르고 …를 붙인다.
func TestIndexLineTruncatesAt200Runes(t *testing.T) {
	long := strings.Repeat("가", 250)
	got := indexLine(Entry{Category: "learning", Key: "k", Content: long})
	if !strings.Contains(got, strings.Repeat("가", 200)+"…") || strings.Contains(got, strings.Repeat("가", 201)) {
		t.Errorf("200룬 절단 + … 이어야 한다: %q", got)
	}
	short := strings.Repeat("나", 200)
	if got := indexLine(Entry{Category: "learning", Key: "k", Content: short}); strings.Contains(got, "…") {
		t.Errorf("200룬 이하는 자르지 않는다: %q", got)
	}
}

// 첫 줄이 마크다운 제목이면 앞의 #과 공백을 떼어 요약으로 쓴다.
func TestIndexLineStripsHeadingMarks(t *testing.T) {
	got := indexLine(Entry{Category: "learning", Key: "k", Content: "\n## 빌드 규칙 요약\n본문"})
	if !strings.Contains(got, "— 빌드 규칙 요약 (") {
		t.Errorf("제목 표시 #이 제거되어야 한다: %q", got)
	}
	// #만 있는 줄은 비어 있는 줄로 보고 다음 줄을 쓴다
	if got := indexLine(Entry{Category: "learning", Key: "k", Content: "###\n다음 줄"}); !strings.Contains(got, "— 다음 줄 (") {
		t.Errorf("#만 있는 줄은 건너뛰어야 한다: %q", got)
	}
}

// upsert 이전 시대에 같은 key로 쌓인 사본(k.md, k-2.md)이 있어도 소폭 수정이
// 거부되지 않고, 최신 사본 하나만 갱신되어 남는다.
// upsert 이전 릴리스는 같은 key·다른 내용을 k.md, k-2.md로 따로 저장했다. 각각 별개의 지식이므로
// upsert는 최신 파일만 덮어쓰고 나머지 형제는 지우지 않는다.
func TestInsertUpsertKeepsLegacySameKeySiblings(t *testing.T) {
	s := newTestStore(t)
	dir := filepath.Join(s.Root, ".pylon", "memory", "app", "learning")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	for name, e := range map[string]*Entry{
		"k.md":   {Category: "learning", Key: "k", Content: "테스트는 race 플래그를 켜고 실행해야 한다", Confidence: 0.8, CreatedAt: time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)},
		"k-2.md": {Category: "learning", Key: "k", Content: "전혀 다른 최신 사본의 본문 내용입니다", Confidence: 0.8, CreatedAt: time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC)},
	} {
		data, err := marshalEntry(e)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), data, 0644); err != nil {
			t.Fatal(err)
		}
	}

	// 오래된 사본(k.md)의 소폭 수정 — 같은 key의 사본은 근사 중복 비교에서 빠져야 한다
	e := &Entry{ProjectID: "app", Category: "learning", Key: "k",
		Content: "테스트는 race 플래그를 켜고 실행해야 한다는 것", Confidence: 0.8}
	if err := s.Insert(e); err != nil {
		t.Fatalf("같은 key 사본 때문에 거부되면 안 된다: %v", err)
	}
	if !e.Updated || !strings.HasSuffix(e.Path, "/k-2.md") {
		t.Errorf("최신 파일(k-2.md)을 갱신해야 한다: updated=%v path=%s", e.Updated, e.Path)
	}
	entries, err := s.List("app")
	if err != nil || len(entries) != 2 {
		t.Fatalf("형제 파일은 남아야 한다: %d, err=%v", len(entries), err)
	}
	byPath := map[string]string{}
	for _, entry := range entries {
		byPath[filepath.Base(entry.Path)] = entry.Content
	}
	if byPath["k-2.md"] != e.Content {
		t.Errorf("k-2.md = %q, want updated content", byPath["k-2.md"])
	}
	if byPath["k.md"] != "테스트는 race 플래그를 켜고 실행해야 한다" {
		t.Errorf("k.md must be left as is: %q", byPath["k.md"])
	}
}

// .lock 같은 점 디렉토리는 프로젝트가 아니다.
func TestListProjectsSkipsDotDirs(t *testing.T) {
	s := newTestStore(t)
	mustInsert(t, s, &Entry{ProjectID: "beta", Category: "learning", Key: "k", Content: "c", Confidence: 0.8})
	mustInsert(t, s, &Entry{ProjectID: "alpha", Category: "learning", Key: "k", Content: "c", Confidence: 0.8})
	if err := os.Mkdir(filepath.Join(s.Root, ".pylon", "memory", ".lock"), 0700); err != nil {
		t.Fatal(err)
	}
	got, err := s.ListProjects()
	if err != nil || strings.Join(got, ",") != "alpha,beta" {
		t.Fatalf("ListProjects = %v, err=%v; want [alpha beta]", got, err)
	}
}

// 2026.9.2 이하의 Stop 훅(`pylon sync-memory --from-session --agent claude`)은 훅 JSON 원문을
// learning으로 저장했다. 그 항목만 정확히 지우고, 비슷해 보이는 사용자 항목은 남긴다.
func TestPruneLegacyHookEntries(t *testing.T) {
	s := newTestStore(t)
	payload := `{"session_id":"3f2a9c1e-7b44-4d21-9a0e-5c8d1f6e2b90","transcript_path":"/x/3f2a9c1e.jsonl","cwd":"/tmp/ws","permission_mode":"acceptEdits","hook_event_name":"Stop","stop_hook_active":false,"last_assistant_message":"Done.\nAll tests pass."}`
	write := func(category, name string, e *Entry) string {
		t.Helper()
		dir := filepath.Join(s.Root, ".pylon", "memory", "app", category)
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		e.Category = category
		data, err := marshalEntry(e)
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, data, 0644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	sessionKey := `session-20261029-172146/{"session_id"-"3f2a9c1e-7b44-4d21-9a0e-5c8d1f6e2b9`
	junk := write("learning", "junk.md", &Entry{Key: sessionKey, Content: payload, Author: "claude", Confidence: 0.8})
	kept := []string{
		write("learning", "prose.md", &Entry{Key: "session-20261029-172146/빌드 규칙", Content: "빌드는 make build로 한다", Author: "claude", Confidence: 0.8}),
		write("learning", "other-author.md", &Entry{Key: sessionKey, Content: payload, Author: "architect", Confidence: 0.8}),
		write("learning", "user.md", &Entry{Key: "race 플래그", Content: "테스트는 race로 돌린다", Author: "agent", Confidence: 0.8}),
		write("decision", "decision.md", &Entry{Key: sessionKey, Content: payload, Author: "claude", Confidence: 0.8}),
		write("learning", "plain-json.md", &Entry{Key: "session-20261029-172146/config", Content: `{"name":"pylon"}`, Author: "claude", Confidence: 0.8}),
	}

	n, err := s.PruneLegacyHookEntries("app")
	if err != nil || n != 1 {
		t.Fatalf("PruneLegacyHookEntries = %d, %v; want 1", n, err)
	}
	if _, err := os.Stat(junk); !os.IsNotExist(err) {
		t.Error("legacy hook entry must be deleted")
	}
	for _, path := range kept {
		if _, err := os.Stat(path); err != nil {
			t.Errorf("%s must be kept: %v", filepath.Base(path), err)
		}
	}
	if n, err := s.PruneLegacyHookEntries("app"); err != nil || n != 0 {
		t.Errorf("second run = %d, %v; want 0", n, err)
	}
}

// 정리할 훅 항목이 없으면 잠금을 잡지 않는다 — 세션 시작마다 호출되므로, 남은 .lock이 있어도
// 매번 lockTimeout만큼 기다리지 않게 한다.
func TestPruneLegacyHookEntries_SkipsLockWhenNothingToDelete(t *testing.T) {
	s := newTestStore(t)
	mustInsert(t, s, &Entry{ProjectID: "app", Category: "learning", Key: "race 플래그", Content: "테스트는 race로 돌린다", Author: "claude", Confidence: 0.8})
	if err := os.Mkdir(s.lockPath(), 0700); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	n, err := s.PruneLegacyHookEntries("app")
	if err != nil || n != 0 {
		t.Fatalf("PruneLegacyHookEntries = %d, %v; want 0, nil", n, err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("waited %v on the lock with nothing to delete", elapsed)
	}
}
