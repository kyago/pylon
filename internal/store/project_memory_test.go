package store

import (
	"testing"
	"time"
)

func insertChange(t *testing.T, s *Store, project, key, content string, createdAt time.Time) {
	t.Helper()
	entry := &MemoryEntry{
		ProjectID: project,
		Category:  "change",
		Key:       key,
		Content:   content,
	}
	if err := s.InsertMemory(entry); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`UPDATE project_memory SET created_at = ? WHERE id = ?`, createdAt, entry.ID); err != nil {
		t.Fatal(err)
	}
}

func TestDeleteMemory_ByKey(t *testing.T) {
	s := setupTestStore(t)

	s.InsertMemory(&MemoryEntry{ProjectID: "p1", Category: "general", Key: "keep", Content: "keep me"})
	s.InsertMemory(&MemoryEntry{ProjectID: "p1", Category: "general", Key: "gone", Content: "delete me"})

	n, err := s.DeleteMemory("p1", "", "gone", false)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("deleted = %d, want 1", n)
	}

	entries, _ := s.ListProjectMemory("p1")
	if len(entries) != 1 || entries[0].Key != "keep" {
		t.Fatalf("unexpected remaining entries: %+v", entries)
	}
}

func TestDeleteMemory_ByCategory_DryRun(t *testing.T) {
	s := setupTestStore(t)

	s.InsertMemory(&MemoryEntry{ProjectID: "p1", Category: "change", Key: "a/1", Content: "diff1"})
	s.InsertMemory(&MemoryEntry{ProjectID: "p1", Category: "change", Key: "b/1", Content: "diff2"})
	s.InsertMemory(&MemoryEntry{ProjectID: "p1", Category: "general", Key: "g", Content: "knowledge"})

	n, err := s.DeleteMemory("p1", "change", "", true)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("dry-run count = %d, want 2", n)
	}
	entries, _ := s.ListProjectMemory("p1")
	if len(entries) != 3 {
		t.Fatalf("dry-run must not delete, got %d entries", len(entries))
	}

	n, err = s.DeleteMemory("p1", "change", "", false)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("deleted = %d, want 2", n)
	}
	entries, _ = s.ListProjectMemory("p1")
	if len(entries) != 1 || entries[0].Category != "general" {
		t.Fatalf("unexpected remaining entries: %+v", entries)
	}
}

func TestDeleteMemory_RequiresFilter(t *testing.T) {
	s := setupTestStore(t)
	if _, err := s.DeleteMemory("", "", "", false); err == nil {
		t.Fatal("expected error when no filter is given")
	}
}

func TestDeleteMemory_SyncsFTS(t *testing.T) {
	s := setupTestStore(t)

	s.InsertMemory(&MemoryEntry{ProjectID: "p1", Category: "change", Key: "k", Content: "uniqueword"})
	if _, err := s.DeleteMemory("p1", "change", "", false); err != nil {
		t.Fatal(err)
	}
	results, err := s.SearchMemory("p1", "uniqueword", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Fatalf("FTS index should be synced after delete, got %d results", len(results))
	}
}

func TestDedupMemoryChanges(t *testing.T) {
	s := setupTestStore(t)
	base := time.Now().Add(-time.Hour)

	// Same file edited three times, plus a distinct file and another project.
	insertChange(t, s, "p1", "src-main-go/1", "old edit 1", base)
	insertChange(t, s, "p1", "src-main-go/2", "old edit 2", base.Add(time.Minute))
	insertChange(t, s, "p1", "src-main-go/3", "latest edit", base.Add(2*time.Minute))
	insertChange(t, s, "p1", "other-file/1", "other", base)
	insertChange(t, s, "p2", "src-main-go/1", "p2 edit", base)

	n, err := s.DedupMemoryChanges("p1", true)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("dry-run dedup count = %d, want 2", n)
	}

	n, err = s.DedupMemoryChanges("p1", false)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("dedup deleted = %d, want 2", n)
	}

	entries, _ := s.GetMemoryByCategory("p1", "change")
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries after dedup, got %d", len(entries))
	}
	for _, e := range entries {
		if e.Key == "src-main-go/1" || e.Key == "src-main-go/2" {
			t.Fatalf("older duplicate %q should have been removed", e.Key)
		}
	}

	// Other project must be untouched.
	p2, _ := s.GetMemoryByCategory("p2", "change")
	if len(p2) != 1 {
		t.Fatalf("p2 entries should be untouched, got %d", len(p2))
	}
}

func TestDedupMemoryChanges_AllProjects(t *testing.T) {
	s := setupTestStore(t)
	base := time.Now().Add(-time.Hour)

	insertChange(t, s, "p1", "f/1", "old", base)
	insertChange(t, s, "p1", "f/2", "new", base.Add(time.Minute))
	insertChange(t, s, "p2", "f/1", "old", base)
	insertChange(t, s, "p2", "f/2", "new", base.Add(time.Minute))

	n, err := s.DedupMemoryChanges("", false)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("deleted = %d, want 2", n)
	}
}

func TestVacuum(t *testing.T) {
	s := setupTestStore(t)
	if err := s.Vacuum(); err != nil {
		t.Fatalf("Vacuum() error = %v", err)
	}
}
