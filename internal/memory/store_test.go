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

func TestInsertResolvesFileNameCollision(t *testing.T) {
	s := newTestStore(t)
	// 같은 키, 다른 내용 → 파일명 -2, -3 suffix로 3건 모두 저장되어야 한다
	for _, content := range []string{"내용 하나", "내용 둘", "내용 셋"} {
		e := &Entry{ProjectID: "app", Category: "learning", Key: "같은 키", Content: content, Confidence: 0.8}
		if err := s.Insert(e); err != nil {
			t.Fatalf("Insert(%q) 실패: %v", content, err)
		}
	}
	entries, err := s.List("app")
	if err != nil || len(entries) != 3 {
		t.Fatalf("3건이 저장되어야 한다: %d, err=%v", len(entries), err)
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
// 같은 내용이 자기 자신과도 매칭되지 않아 매 턴 새 파일이 쌓인다.
func TestInsertSkipsDuplicateWithTrailingWhitespace(t *testing.T) {
	s := newTestStore(t)
	for i := 0; i < 3; i++ {
		e := &Entry{ProjectID: "app", Category: "learning", Key: "k", Content: "반복 내용\n", Confidence: 0.8}
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

func TestStoreLearningsSilentlySkipsDuplicates(t *testing.T) {
	s := newTestStore(t)
	// Stop hook이 매 턴 같은 학습 내용을 보내는 상황 재현 — 에러 없이 1건만 남아야 한다
	for i := 0; i < 3; i++ {
		if err := s.StoreLearnings("app", "task-1", "claude", []string{"반복되는 학습 내용"}); err != nil {
			t.Fatalf("StoreLearnings %d 실패: %v", i, err)
		}
	}
	entries, _ := s.ListByCategory("app", "learning")
	if len(entries) != 1 {
		t.Fatalf("중복 학습은 1건만 저장되어야 한다: %d", len(entries))
	}
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

func TestStoreLearningsTruncatesKeyByRunes(t *testing.T) {
	s := newTestStore(t)
	long := strings.Repeat("가", 60)
	if err := s.StoreLearnings("app", "task-1", "architect", []string{long}); err != nil {
		t.Fatalf("StoreLearnings 실패: %v", err)
	}
	entries, _ := s.ListByCategory("app", "learning")
	if len(entries) != 1 {
		t.Fatalf("1건 저장: %d", len(entries))
	}
	if entries[0].Content != long {
		t.Error("Content는 잘리면 안 된다")
	}
	if !strings.HasPrefix(entries[0].Key, "task-1/") {
		t.Errorf("Key는 taskID 접두사를 가져야 한다: %q", entries[0].Key)
	}
}

func TestIndexMarkdownTruncation(t *testing.T) {
	s := newTestStore(t)
	for i := 0; i < 20; i++ {
		// content는 항목마다 달라야 한다 — 동일 내용은 D4로 중복 스킵된다.
		mustInsert(t, s, &Entry{ProjectID: "app", Category: "learning",
			Key: strings.Repeat("k", 10) + string(rune('a'+i)), Content: strings.Repeat("내용 ", 30) + string(rune('a'+i)), Confidence: 0.8})
	}
	full, err := s.IndexMarkdown("app", 0)
	if err != nil || full == "" {
		t.Fatalf("전체 인덱스: err=%v", err)
	}
	capped, err := s.IndexMarkdown("app", 300)
	if err != nil {
		t.Fatalf("잘린 인덱스: %v", err)
	}
	if len(capped) > 300+len("\n…(생략)\n") {
		t.Errorf("maxBytes를 초과했습니다: %d바이트", len(capped))
	}
	// 존재하지 않는 프로젝트는 빈 문자열
	if empty, err := s.IndexMarkdown("ghost", 100); err != nil || empty != "" {
		t.Errorf("없는 프로젝트: %q, err=%v", empty, err)
	}
}
