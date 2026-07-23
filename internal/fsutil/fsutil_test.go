// internal/fsutil/fsutil_test.go
package fsutil

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAcquireLockBlocksSecondHolder(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), ".lock")

	unlock, err := AcquireLock(lockPath, time.Second)
	if err != nil {
		t.Fatalf("첫 잠금 실패: %v", err)
	}

	if _, err := AcquireLock(lockPath, 100*time.Millisecond); err == nil {
		t.Fatal("잠금 보유 중 두 번째 획득이 성공하면 안 된다")
	}

	unlock()
	unlock2, err := AcquireLock(lockPath, time.Second)
	if err != nil {
		t.Fatalf("해제 후 재획득 실패: %v", err)
	}
	unlock2()
}

func TestWriteFileAtomicCreatesParentDirs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a", "b", "file.md")
	if err := WriteFileAtomic(path, []byte("hello"), 0644); err != nil {
		t.Fatalf("쓰기 실패: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "hello" {
		t.Fatalf("내용 불일치: %q, err=%v", data, err)
	}
}

func TestWriteJSONAtomic(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.json")
	if err := WriteJSONAtomic(path, map[string]int{"n": 1}); err != nil {
		t.Fatalf("쓰기 실패: %v", err)
	}
	data, _ := os.ReadFile(path)
	if string(data) != "{\n  \"n\": 1\n}\n" {
		t.Fatalf("JSON 출력 불일치: %q", data)
	}
}

func TestCopyDir(t *testing.T) {
	src := t.TempDir()
	if err := os.MkdirAll(filepath.Join(src, "sub"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "root.txt"), []byte("r"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "sub", "leaf.txt"), []byte("l"), 0644); err != nil {
		t.Fatal(err)
	}

	dst := filepath.Join(t.TempDir(), "copy")
	if err := CopyDir(src, dst); err != nil {
		t.Fatalf("복사 실패: %v", err)
	}
	if data, _ := os.ReadFile(filepath.Join(dst, "sub", "leaf.txt")); string(data) != "l" {
		t.Fatalf("복사 내용 불일치: %q", data)
	}
	if err := CopyDir(src, dst); err == nil {
		t.Fatal("이미 존재하는 대상에 대한 복사는 실패해야 한다")
	}
}
