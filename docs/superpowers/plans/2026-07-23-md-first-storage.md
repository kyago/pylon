# MD-First 저장소 전환 (SQLite·Fossil 제거) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 프로젝트 메모리와 작업 이력을 마크다운/JSON 파일 기반으로 전환하고, `modernc.org/sqlite` 의존성과 Fossil 외부 도구 의존을 완전히 제거한다.

**Architecture:** 메모리는 `.pylon/memory/<project>/<category>/<slug>.md`(YAML frontmatter + 본문)를 source of truth로 하고, 프로젝트별 `INDEX.md`를 생성해 CLAUDE.md에 주입한다(proactive_injection 실구현). 작업 이력은 `.pylon/history/pipelines/<pipeline-id>/<phase>/` 디렉토리 스냅샷으로 전환하며, 기존 큐레이션 로직(stageCheckpoint/digestTree/manifest)은 그대로 재사용한다. `projects` 테이블은 `config.DiscoverProjects()` 파생 데이터이므로 저장소 자체를 제거한다.

**Tech Stack:** Go 1.24+, Cobra, gopkg.in/yaml.v3 (기존 의존성만 사용, 신규 의존성 0개)

## Global Constraints

- 새 외부 Go 의존성 추가 금지. 완료 후 `go.mod`에서 `modernc.org/sqlite`, `google/uuid`가 사라져야 한다 (`go mod tidy`로 확인).
- 런타임 필수 외부 바이너리는 `git`, `gh`, `claude`, `diff`(POSIX)뿐이다. `fossil`과 `sqlite3` 의존은 완전히 사라진다 — 레거시 데이터는 이전 없이 폐기한다(의사결정 D2/D3 참조).
- 에러/출력 메시지는 기존 코드처럼 한국어, 주석 밀도·스타일은 주변 코드를 따른다.
- 각 태스크 종료 시 `make build && make test && make lint`가 통과해야 한다.
- CLI 표면 변화는 다음으로 한정한다: `mem prune --dedup` 제거, `pylon history init`/`pylon history sync` 서브커맨드 제거, `pylon sync-projects` 커맨드 제거, checkpoint JSON의 `checkin` 필드가 `ref`(`<pipeline-id>/<phase>`)로 대체. 그 외 커맨드·플래그·JSON 키는 유지.
- 커밋 메시지는 저장소 관례(한국어 conventional commit, 예: `refactor: ...`, `feat: ...`)를 따른다.
- 기존 테스트 파일을 수정할 때는 fossil `CommandRunner` mock을 파일시스템 직접 검증으로 대체한다. 테스트 헬퍼는 `t.TempDir()`를 사용한다.

## 확정된 의사결정 (2026-07-23, 사용자 확인 완료)

클린 컨텍스트에서 이어 작업할 때 이 결정들을 재질문하지 말 것. 변경하려면 사용자 승인이 필요하다.

| # | 결정 | 내용 | 계획 반영 위치 |
|---|------|------|---------------|
| D1 | 메모리 git 추적 | `.pylon/memory/`는 **git 추적 대상**. 멀티 repo 구조를 고려해 메모리는 **워크스페이스 레벨에 유지**한다 — 프로젝트 repo 안에 두면 에이전트의 feature 브랜치에 메모리 커밋이 섞이기 때문. `pylon init`이 `.pylon/memory/.gitkeep`을 생성하고, 워크스페이스 루트가 git repo가 아니면 "git init 권장" 힌트를 출력한다. `add-project`는 추가 작업 불필요(프로젝트별 메모리 디렉토리는 첫 저장 시 lazy 생성). | Task 9 Step 4 |
| D2 | Fossil 이력 폐기 | 기존 fossil 이력은 **이전하지 않고 폐기**한다. 백업 안내도 하지 않는다. 새 파일 기반 history는 빈 상태로 시작. | Task 9 Step 1 |
| D3 | 레거시 감지 없음 + 데이터 폐기 | `pylon.db`/`pylon-history.fossil` 감지·경고 코드를 추가하지 않는다. `/pl:migrate` 문서가 레거시 파일 삭제만 안내한다. **기존 project_memory(SQLite) 데이터도 이전 없이 폐기** — sqlite→md 마이그레이션 스크립트를 만들지 않는다(sqlite3 의존 완전 제거). | Task 9, Task 10 |
| D4 | Insert 중복 스킵 | 동일 project+category에 같은 content가 이미 있으면 `memory.ErrDuplicate`를 반환하고 저장하지 않는다. Stop hook이 매 턴 실행되어 생기는 파일 폭증·git 커밋 노이즈를 원천 완화. `StoreLearnings`는 중복을 조용히 건너뛰고, `mem store` CLI는 skip 메시지를 출력한다. | Task 3, Task 5 |

## 파일 구조 (최종 상태)

```
internal/
├── fsutil/                 # 신규: 공유 FS 프리미티브 (락, 원자적 쓰기, 디렉토리 복사)
│   ├── fsutil.go
│   └── fsutil_test.go
├── memory/                 # 재작성: md 파일 기반 메모리 저장소
│   ├── frontmatter.go      # Entry 타입 + 마샬/파싱
│   ├── store.go            # Insert/List/Delete/StoreLearnings
│   ├── search.go           # 토큰 매칭 검색
│   ├── index.go            # INDEX.md 생성/주입용 읽기
│   └── *_test.go
├── history/                # 재작성: 디렉토리 스냅샷 기반 이력
│   ├── manager.go          # Checkpoint/Log/Show/Diff/Export (fossil 제거)
│   └── manager_test.go
├── store/                  # 삭제 (전체)
└── cli/
    ├── sync_projects.go    # 삭제
    ├── mem.go              # memory.Store 사용으로 전환
    ├── sync_memory.go      # memory.Store 사용으로 전환
    ├── history.go          # 새 Manager 사용, init/sync 서브커맨드 제거
    ├── launch.go           # prepareHistory 제거, openWorkspace() 추가
    ├── init_cmd.go         # store/fossil 초기화 제거
    ├── add_project.go      # registerProjectInDB 제거
    ├── delete_project.go   # DiscoverProjects + 메모리 디렉토리 삭제로 전환
    ├── doctor.go           # fossil 체크 제거
    └── launch_claudemd.go  # 메모리 인덱스 주입 (proactive_injection 실구현)
```

워크스페이스 데이터 레이아웃 (신규):

```
.pylon/memory/<project>/INDEX.md                 # 생성 인덱스 (git 추적)
.pylon/memory/<project>/<category>/<slug>.md     # 항목 1건 = 파일 1개 (git 추적)
.pylon/history/pipelines/<id>/<phase>/manifest.json + 산출물  (git 무시 유지)
```

---

### Task 1: `internal/fsutil` — 공유 파일시스템 프리미티브

기존 `internal/history/manager.go`의 `acquireLock`/`writeJSONAtomic`을 공용 패키지로 승격하고 `CopyDir`를 추가한다. Task 3(memory)과 Task 4(history)가 모두 소비한다.

**Files:**
- Create: `internal/fsutil/fsutil.go`
- Test: `internal/fsutil/fsutil_test.go`

**Interfaces:**
- Consumes: 표준 라이브러리만.
- Produces: `fsutil.AcquireLock(path string, timeout time.Duration) (func(), error)`, `fsutil.WriteFileAtomic(path string, data []byte, mode os.FileMode) error`, `fsutil.WriteJSONAtomic(path string, value any) error`, `fsutil.CopyDir(src, dst string) error` — Task 3/4가 이 시그니처를 그대로 사용한다.

- [ ] **Step 1: 실패하는 테스트 작성**

```go
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
	os.WriteFile(filepath.Join(src, "root.txt"), []byte("r"), 0644)
	os.WriteFile(filepath.Join(src, "sub", "leaf.txt"), []byte("l"), 0644)

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
```

- [ ] **Step 2: 테스트 실패 확인**

Run: `go test ./internal/fsutil/ -v`
Expected: FAIL — `undefined: AcquireLock` 등 컴파일 에러

- [ ] **Step 3: 구현 작성**

```go
// internal/fsutil/fsutil.go
// Package fsutil provides shared filesystem primitives for pylon's
// file-based stores (memory, history).
package fsutil

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// AcquireLock creates a mkdir-based advisory lock and returns an unlock
// function. Waits up to timeout, polling every 25ms.
func AcquireLock(path string, timeout time.Duration) (func(), error) {
	deadline := time.Now().Add(timeout)
	for {
		if err := os.Mkdir(path, 0700); err == nil {
			return func() { _ = os.Remove(path) }, nil
		} else if !os.IsExist(err) {
			return nil, err
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("잠금 시간 초과: %s (실행 중인 pylon 프로세스가 없다면 이 디렉토리를 제거한 뒤 다시 시도하세요)", path)
		}
		time.Sleep(25 * time.Millisecond)
	}
}

// WriteFileAtomic writes data via a temp file + rename in the target directory.
func WriteFileAtomic(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".tmp-")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tempName, mode); err != nil {
		return err
	}
	return os.Rename(tempName, path)
}

// WriteJSONAtomic marshals value with indentation and writes it atomically.
func WriteJSONAtomic(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return WriteFileAtomic(path, data, 0644)
}

// CopyDir recursively copies src into dst. dst must not already exist.
func CopyDir(src, dst string) error {
	if _, err := os.Stat(dst); err == nil {
		return fmt.Errorf("복사 대상이 이미 존재합니다: %s", dst)
	} else if !os.IsNotExist(err) {
		return err
	}
	return filepath.WalkDir(src, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if entry.IsDir() {
			return os.MkdirAll(target, 0755)
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, in); err != nil {
			out.Close()
			return err
		}
		return out.Close()
	})
}
```

- [ ] **Step 4: 테스트 통과 확인**

Run: `go test ./internal/fsutil/ -v`
Expected: PASS (4개 테스트)

- [ ] **Step 5: 커밋**

```bash
git add internal/fsutil/
git commit -m "feat: fsutil 패키지 추가 — 파일 기반 저장소 공용 프리미티브"
```

---

### Task 2: `internal/layout` — 신규 경로 헬퍼 추가

`DBPath` 제거는 Task 7에서 한다(그때까지 소비자가 남아 있음). 이 태스크는 추가만 한다.

**Files:**
- Modify: `internal/layout/layout.go`
- Test: `internal/layout/layout_test.go`

**Interfaces:**
- Produces: `layout.MemoryDir(root) string` = `<root>/.pylon/memory`, `layout.ProjectMemoryDir(root, project) string` = `<root>/.pylon/memory/<project>`, `layout.HistoryDir(root) string` = `<root>/.pylon/history` — Task 3/4가 사용.

- [ ] **Step 1: 실패하는 테스트 추가** — `layout_test.go`의 기존 테스트 스타일을 확인하고 같은 스타일로 추가:

```go
func TestMemoryAndHistoryDirs(t *testing.T) {
	if got := MemoryDir("/ws"); got != filepath.Join("/ws", ".pylon", "memory") {
		t.Errorf("MemoryDir = %q", got)
	}
	if got := ProjectMemoryDir("/ws", "app"); got != filepath.Join("/ws", ".pylon", "memory", "app") {
		t.Errorf("ProjectMemoryDir = %q", got)
	}
	if got := HistoryDir("/ws"); got != filepath.Join("/ws", ".pylon", "history") {
		t.Errorf("HistoryDir = %q", got)
	}
}
```

- [ ] **Step 2: 실패 확인** — Run: `go test ./internal/layout/ -v` → FAIL (undefined)

- [ ] **Step 3: 구현** — `layout.go`의 `RuntimeDir` 아래에 추가:

```go
// MemoryDir returns the markdown memory store root (.pylon/memory).
func MemoryDir(root string) string {
	return filepath.Join(PylonDir(root), "memory")
}

// ProjectMemoryDir returns one project's memory directory (.pylon/memory/<project>).
func ProjectMemoryDir(root, project string) string {
	return filepath.Join(MemoryDir(root), project)
}

// HistoryDir returns the file-based work history root (.pylon/history).
func HistoryDir(root string) string {
	return filepath.Join(PylonDir(root), "history")
}
```

- [ ] **Step 4: 통과 확인** — Run: `go test ./internal/layout/ -v` → PASS

- [ ] **Step 5: 커밋**

```bash
git add internal/layout/
git commit -m "feat: layout에 memory/history 파일 저장소 경로 헬퍼 추가"
```

---

### Task 3: `internal/memory` — md 파일 기반 Store 구현

신규 파일로 추가한다. **기존 `manager.go`(SQLite 기반 Manager)는 이 태스크에서 건드리지 않는다** — CLI 전환(Task 5)에서 삭제해야 컴파일이 유지된다.

**Files:**
- Create: `internal/memory/frontmatter.go`
- Create: `internal/memory/store.go`
- Create: `internal/memory/search.go`
- Create: `internal/memory/index.go`
- Test: `internal/memory/store_test.go` (신규; 기존 `manager_test.go`는 유지)

**Interfaces:**
- Consumes: `fsutil.AcquireLock`/`WriteFileAtomic` (Task 1), `layout.MemoryDir`/`ProjectMemoryDir` (Task 2), `slug.Slugify` (기존).
- Produces (Task 5, 6, 8이 사용):
  - `memory.NewStore(root string) *Store`
  - `memory.ErrDuplicate` — 동일 project+category+content 존재 시 Insert가 반환하는 sentinel (`errors.Is`로 감지, D4)
  - `(*Store).Insert(e *Entry) error` — 중복이면 `ErrDuplicate` 반환(파일 미생성, `e.Path`에 기존 파일 경로 설정)
  - `(*Store).List(project string) ([]Entry, error)`
  - `(*Store).ListByCategory(project, category string) ([]Entry, error)`
  - `(*Store).Search(project, query string, limit int) ([]SearchResult, error)`
  - `(*Store).Delete(project, category, key string, dryRun bool) (int64, error)` — project가 빈 문자열이면 전체 프로젝트 대상
  - `(*Store).DeleteProject(project string) (int64, error)`
  - `(*Store).StoreLearnings(project, taskID, agent string, learnings []string) error`
  - `(*Store).IndexMarkdown(project string, maxBytes int) (string, error)`
  - `Entry{ProjectID, Category, Key, Content, Author string; Confidence float64; CreatedAt time.Time; Path string}`
  - `SearchResult{Entry; Rank float64}` — Rank는 0~1 (일치 토큰 비율)

- [ ] **Step 1: 실패하는 테스트 작성**

```go
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
	s.Insert(&Entry{ProjectID: "app", Category: "learning", Key: "메모리 관리",
		Content: "메모리를 파일로 관리한다", Confidence: 0.8})
	s.Insert(&Entry{ProjectID: "app", Category: "decision", Key: "무관한 항목",
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
	s.Insert(&Entry{ProjectID: "app", Category: "learning", Key: "a", Content: "빌드 캐시 설정", Confidence: 0.8})
	s.Insert(&Entry{ProjectID: "app", Category: "learning", Key: "b", Content: "빌드만 언급", Confidence: 0.8})

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
	s.Insert(&Entry{ProjectID: "app", Category: "learning", Key: "k1", Content: "c1", Confidence: 0.8})
	s.Insert(&Entry{ProjectID: "app", Category: "learning", Key: "k2", Content: "c2", Confidence: 0.8})
	s.Insert(&Entry{ProjectID: "app", Category: "decision", Key: "k3", Content: "c3", Confidence: 0.8})

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
	s.Insert(&Entry{ProjectID: "app1", Category: "change", Key: "k", Content: "c", Confidence: 0.8})
	s.Insert(&Entry{ProjectID: "app2", Category: "change", Key: "k", Content: "c", Confidence: 0.8})
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
		s.Insert(&Entry{ProjectID: "app", Category: "learning",
			Key: strings.Repeat("k", 10) + string(rune('a'+i)), Content: strings.Repeat("내용 ", 30), Confidence: 0.8})
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
```

- [ ] **Step 2: 실패 확인** — Run: `go test ./internal/memory/ -v` → FAIL (undefined: NewStore 등)

- [ ] **Step 3: `frontmatter.go` 구현**

```go
// internal/memory/frontmatter.go
package memory

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Entry is a single memory record backed by one markdown file.
type Entry struct {
	ProjectID  string    `yaml:"-"`
	Category   string    `yaml:"category"`
	Key        string    `yaml:"key"`
	Author     string    `yaml:"author,omitempty"`
	Confidence float64   `yaml:"confidence"`
	CreatedAt  time.Time `yaml:"created_at"`
	Content    string    `yaml:"-"`
	Path       string    `yaml:"-"` // 워크스페이스 루트 기준 상대 경로 (slash 구분)
}

const frontmatterDelim = "---\n"

// marshalEntry renders an Entry as markdown with YAML frontmatter.
func marshalEntry(e *Entry) ([]byte, error) {
	head, err := yaml.Marshal(e)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	buf.WriteString(frontmatterDelim)
	buf.Write(head)
	buf.WriteString(frontmatterDelim)
	buf.WriteString("\n")
	buf.WriteString(strings.TrimRight(e.Content, "\n"))
	buf.WriteString("\n")
	return buf.Bytes(), nil
}

// parseEntry parses a markdown memory file produced by marshalEntry.
func parseEntry(data []byte) (*Entry, error) {
	text := string(data)
	if !strings.HasPrefix(text, frontmatterDelim) {
		return nil, fmt.Errorf("frontmatter가 없습니다")
	}
	rest := text[len(frontmatterDelim):]
	const sep = "\n---\n"
	idx := strings.Index(rest, sep)
	if idx < 0 {
		return nil, fmt.Errorf("frontmatter 종료 구분자가 없습니다")
	}
	var e Entry
	if err := yaml.Unmarshal([]byte(rest[:idx+1]), &e); err != nil {
		return nil, fmt.Errorf("frontmatter 파싱 실패: %w", err)
	}
	e.Content = strings.TrimSpace(rest[idx+len(sep):])
	return &e, nil
}
```

- [ ] **Step 4: `store.go` 구현**

```go
// internal/memory/store.go
// Package memory implements the markdown-file-backed project memory store.
// 항목 1건 = .pylon/memory/<project>/<category>/<slug>.md 파일 1개.
package memory

import (
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/kyago/pylon/internal/fsutil"
	"github.com/kyago/pylon/internal/layout"
	"github.com/kyago/pylon/internal/slug"
)

const lockTimeout = 5 * time.Second

// ErrDuplicate is returned by Insert when identical content already exists in
// the same project/category. errors.Is로 감지한다. (의사결정 D4)
var ErrDuplicate = errors.New("동일한 내용이 이미 저장되어 있습니다")

// Store is a file-based project memory store rooted at a workspace.
type Store struct {
	Root string
	Now  func() time.Time
}

func NewStore(root string) *Store {
	return &Store{Root: root, Now: time.Now}
}

func (s *Store) baseDir() string                   { return layout.MemoryDir(s.Root) }
func (s *Store) projectDir(project string) string  { return layout.ProjectMemoryDir(s.Root, project) }
func (s *Store) lockPath() string                  { return filepath.Join(s.baseDir(), ".lock") }

func validateConfidence(confidence float64) error {
	if math.IsNaN(confidence) || math.IsInf(confidence, 0) || confidence < 0 || confidence > 1 {
		return fmt.Errorf("confidence는 0.0과 1.0 사이여야 합니다: %v", confidence)
	}
	return nil
}

// validateComponent rejects values unusable as a single path segment.
func validateComponent(name, value string) error {
	if value == "" || value == "." || value == ".." ||
		strings.ContainsAny(value, `/\`) || filepath.Base(value) != value {
		return fmt.Errorf("유효하지 않은 %s: %q", name, value)
	}
	return nil
}

// Insert writes a new entry as a markdown file and refreshes the project index.
func (s *Store) Insert(e *Entry) error {
	if err := validateComponent("project", e.ProjectID); err != nil {
		return err
	}
	if err := validateComponent("category", e.Category); err != nil {
		return err
	}
	if e.Key == "" || e.Content == "" {
		return fmt.Errorf("key와 content는 비어 있을 수 없습니다")
	}
	if err := validateConfidence(e.Confidence); err != nil {
		return err
	}
	if e.CreatedAt.IsZero() {
		e.CreatedAt = s.Now().UTC()
	}

	if err := os.MkdirAll(s.baseDir(), 0755); err != nil {
		return err
	}
	unlock, err := fsutil.AcquireLock(s.lockPath(), lockTimeout)
	if err != nil {
		return err
	}
	defer unlock()

	// Stop hook이 매 턴 같은 학습을 보내므로 동일 내용은 저장하지 않는다 (D4).
	existing, err := s.ListByCategory(e.ProjectID, e.Category)
	if err != nil {
		return err
	}
	for _, prev := range existing {
		if prev.Content == e.Content {
			e.Path = prev.Path
			return fmt.Errorf("%w: %s", ErrDuplicate, prev.Path)
		}
	}

	dir := filepath.Join(s.projectDir(e.ProjectID), e.Category)
	name, err := availableFileName(dir, e.Key)
	if err != nil {
		return err
	}
	data, err := marshalEntry(e)
	if err != nil {
		return err
	}
	path := filepath.Join(dir, name)
	if err := fsutil.WriteFileAtomic(path, data, 0644); err != nil {
		return err
	}
	rel, err := filepath.Rel(s.Root, path)
	if err != nil {
		return err
	}
	e.Path = filepath.ToSlash(rel)
	return s.rebuildIndexLocked(e.ProjectID)
}

// availableFileName picks a slug-based file name, suffixing -2, -3, … on collision.
func availableFileName(dir, key string) (string, error) {
	base := slug.Slugify(key)
	if runes := []rune(base); len(runes) > 80 {
		base = string(runes[:80])
	}
	for i := 1; i <= 999; i++ {
		name := base + ".md"
		if i > 1 {
			name = fmt.Sprintf("%s-%d.md", base, i)
		}
		_, err := os.Stat(filepath.Join(dir, name))
		if os.IsNotExist(err) {
			return name, nil
		}
		if err != nil {
			return "", err
		}
	}
	return "", fmt.Errorf("파일 이름 충돌이 너무 많습니다: %s", base)
}

// List returns all entries for a project, sorted by category then key.
func (s *Store) List(project string) ([]Entry, error) {
	if err := validateComponent("project", project); err != nil {
		return nil, err
	}
	dir := s.projectDir(project)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return nil, nil
	}
	var entries []Entry
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || !strings.HasSuffix(path, ".md") || d.Name() == indexFileName {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		e, err := parseEntry(data)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		e.ProjectID = project
		rel, err := filepath.Rel(s.Root, path)
		if err != nil {
			return err
		}
		e.Path = filepath.ToSlash(rel)
		entries = append(entries, *e)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Category != entries[j].Category {
			return entries[i].Category < entries[j].Category
		}
		return entries[i].Key < entries[j].Key
	})
	return entries, nil
}

// ListByCategory returns one category's entries, newest first.
func (s *Store) ListByCategory(project, category string) ([]Entry, error) {
	all, err := s.List(project)
	if err != nil {
		return nil, err
	}
	var entries []Entry
	for _, e := range all {
		if e.Category == category {
			entries = append(entries, e)
		}
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].CreatedAt.After(entries[j].CreatedAt)
	})
	return entries, nil
}

// Delete removes entries matching the filters. project가 빈 문자열이면 전체
// 프로젝트를 대상으로 한다. dryRun이면 건수만 반환한다.
func (s *Store) Delete(project, category, key string, dryRun bool) (int64, error) {
	if category == "" && key == "" {
		return 0, fmt.Errorf("category 또는 key 필터가 필요합니다")
	}
	projects := []string{project}
	if project == "" {
		dirs, err := os.ReadDir(s.baseDir())
		if os.IsNotExist(err) {
			return 0, nil
		}
		if err != nil {
			return 0, err
		}
		projects = projects[:0]
		for _, d := range dirs {
			if d.IsDir() {
				projects = append(projects, d.Name())
			}
		}
	}
	var total int64
	for _, p := range projects {
		n, err := s.deleteInProject(p, category, key, dryRun)
		if err != nil {
			return total, err
		}
		total += n
	}
	return total, nil
}

func (s *Store) deleteInProject(project, category, key string, dryRun bool) (int64, error) {
	entries, err := s.List(project)
	if err != nil {
		return 0, err
	}
	var targets []Entry
	for _, e := range entries {
		if category != "" && e.Category != category {
			continue
		}
		if key != "" && e.Key != key {
			continue
		}
		targets = append(targets, e)
	}
	if dryRun || len(targets) == 0 {
		return int64(len(targets)), nil
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

// DeleteProject removes the whole memory directory of a project.
func (s *Store) DeleteProject(project string) (int64, error) {
	entries, err := s.List(project)
	if err != nil {
		return 0, err
	}
	if err := os.RemoveAll(s.projectDir(project)); err != nil {
		return 0, err
	}
	return int64(len(entries)), nil
}

// StoreLearnings saves session learnings (Stop hook 경로).
func (s *Store) StoreLearnings(project, taskID, agent string, learnings []string) error {
	for _, learning := range learnings {
		// 바이트가 아닌 룬 단위로 잘라 멀티바이트 문자가 깨지지 않게 한다.
		keyRunes := []rune(learning)
		if len(keyRunes) > 50 {
			keyRunes = keyRunes[:50]
		}
		e := &Entry{
			ProjectID:  project,
			Category:   "learning",
			Key:        fmt.Sprintf("%s/%s", taskID, sanitizeKey(string(keyRunes))),
			Content:    learning,
			Author:     agent,
			Confidence: 0.8,
		}
		if err := s.Insert(e); err != nil {
			if errors.Is(err, ErrDuplicate) {
				continue // 매 턴 반복되는 동일 학습은 조용히 건너뛴다 (D4)
			}
			return fmt.Errorf("학습 내용 저장 실패: %w", err)
		}
	}
	return nil
}

func sanitizeKey(v string) string {
	return strings.NewReplacer(" ", "-", "/", "-", ":", "-").Replace(v)
}

// removeEmptyCategoryDirs prunes category directories emptied by deletion.
func removeEmptyCategoryDirs(projectDir string) {
	dirs, err := os.ReadDir(projectDir)
	if err != nil {
		return
	}
	for _, d := range dirs {
		if d.IsDir() {
			_ = os.Remove(filepath.Join(projectDir, d.Name())) // 비어 있을 때만 성공
		}
	}
}
```

- [ ] **Step 5: `search.go` 구현**

```go
// internal/memory/search.go
package memory

import (
	"sort"
	"strings"
)

// SearchResult is a search hit with a normalized match score.
type SearchResult struct {
	Entry
	Rank float64 // 일치한 토큰 수 / 전체 토큰 수 (1.0 = 모든 토큰 일치)
}

// Search scores entries by case-insensitive substring containment per query
// token. 부분 문자열 매칭이므로 한국어 조사 변형("메모리를")도 "메모리"로 검색된다.
func (s *Store) Search(project, query string, limit int) ([]SearchResult, error) {
	if limit <= 0 {
		limit = 10
	}
	tokens := strings.Fields(strings.ToLower(query))
	if len(tokens) == 0 {
		return nil, nil
	}
	entries, err := s.List(project)
	if err != nil {
		return nil, err
	}
	var results []SearchResult
	for _, e := range entries {
		text := strings.ToLower(e.Key + "\n" + e.Content)
		matched := 0
		for _, tok := range tokens {
			if strings.Contains(text, tok) {
				matched++
			}
		}
		if matched == 0 {
			continue
		}
		results = append(results, SearchResult{Entry: e, Rank: float64(matched) / float64(len(tokens))})
	}
	sort.Slice(results, func(i, j int) bool {
		if results[i].Rank != results[j].Rank {
			return results[i].Rank > results[j].Rank
		}
		if results[i].Confidence != results[j].Confidence {
			return results[i].Confidence > results[j].Confidence
		}
		return results[i].CreatedAt.After(results[j].CreatedAt)
	})
	if len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}
```

- [ ] **Step 6: `index.go` 구현**

```go
// internal/memory/index.go
package memory

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/kyago/pylon/internal/fsutil"
)

const indexFileName = "INDEX.md"

// rebuildIndexLocked regenerates INDEX.md. Caller must hold the store lock.
func (s *Store) rebuildIndexLocked(project string) error {
	entries, err := s.List(project)
	if err != nil {
		return err
	}
	indexPath := filepath.Join(s.projectDir(project), indexFileName)
	if len(entries) == 0 {
		if err := os.Remove(indexPath); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Category != entries[j].Category {
			return entries[i].Category < entries[j].Category
		}
		return entries[i].CreatedAt.After(entries[j].CreatedAt)
	})
	var b strings.Builder
	fmt.Fprintf(&b, "# %s 프로젝트 메모리 인덱스\n\n", project)
	for _, e := range entries {
		summary := strings.Join(strings.Fields(e.Content), " ")
		if runes := []rune(summary); len(runes) > 100 {
			summary = string(runes[:100]) + "…"
		}
		fmt.Fprintf(&b, "- [%s] `%s` — %s (%.1f, %s)\n",
			e.Category, e.Key, summary, e.Confidence, e.CreatedAt.Format("2006-01-02"))
	}
	return fsutil.WriteFileAtomic(indexPath, []byte(b.String()), 0644)
}

// IndexMarkdown returns the project index, truncated at a line boundary when
// maxBytes > 0. 존재하지 않는 프로젝트는 빈 문자열을 반환한다.
func (s *Store) IndexMarkdown(project string, maxBytes int) (string, error) {
	data, err := os.ReadFile(filepath.Join(s.projectDir(project), indexFileName))
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if maxBytes <= 0 || len(data) <= maxBytes {
		return string(data), nil
	}
	cut := bytes.LastIndexByte(data[:maxBytes], '\n')
	if cut <= 0 {
		return "…(생략)\n", nil
	}
	return string(data[:cut]) + "\n…(생략)\n", nil
}
```

- [ ] **Step 7: 통과 확인** — Run: `go test ./internal/memory/ -v` → 신규 테스트 전부 PASS (기존 `manager_test.go`도 계속 PASS — 아직 공존)

- [ ] **Step 8: 커밋**

```bash
git add internal/memory/frontmatter.go internal/memory/store.go internal/memory/search.go internal/memory/index.go internal/memory/store_test.go
git commit -m "feat: md 파일 기반 memory.Store 구현 — frontmatter/검색/인덱스"
```

---

### Task 4: `internal/history` 재작성 + fossil 호출부 일괄 제거

fossil을 제거하고 디렉토리 스냅샷으로 전환한다. history 패키지의 공개 API가 바뀌므로 **모든 소비자(cli/history.go, cancel.go, launch.go, init_cmd.go, doctor.go)를 같은 태스크에서 수정**해야 컴파일이 유지된다. 큐레이션 로직은 기존 코드를 그대로 옮긴다.

**Files:**
- Rewrite: `internal/history/manager.go`
- Rewrite: `internal/history/manager_test.go`
- Modify: `internal/cli/history.go` (전면 재작성)
- Modify: `internal/cli/launch.go:33` (prepareHistory 호출 제거), `launch.go:85-92` (함수 삭제), `openWorkspace()` 추가
- Modify: `internal/cli/cancel.go:60-72`
- Modify: `internal/cli/init_cmd.go:199-206` (history Initialize 블록), `init_cmd.go:242` (안내 문구)
- Modify: `internal/cli/doctor.go:33-38` (fossil 체크), `doctor.go:16` (import)
- Test: `internal/cli/history_test.go`, `internal/cli/cancel_test.go`, `internal/cli/launch_history_test.go`, `internal/cli/doctor_test.go`, `internal/cli/init_cmd_test.go` 갱신

**Interfaces:**
- Consumes: `fsutil.*` (Task 1), `layout.HistoryDir`/`ProjectMemoryDir` (Task 2).
- Produces (cancel.go, cli/history.go가 사용):
  - `history.NewManager(root string) *Manager` — config/store 매개변수 제거
  - `(*Manager).Checkpoint(pipelineID string, phase Phase) (CheckpointResult, error)`
  - `CheckpointResult{PipelineID string; Phase Phase; Ref string; Digest string; Duplicate bool}` — `Ref` = `"<pipeline-id>/<phase>"`
  - `(*Manager).Log(pipelineID string, limit int) ([]LogEntry, error)`, `LogEntry{Ref string; PipelineID string; Phase Phase; RecordedAt time.Time; Status string}`
  - `(*Manager).Show(ref string) (*Manifest, []string, error)`
  - `(*Manager).Diff(from, to string) (string, error)` — POSIX `diff -ru` 사용, 종료 코드 1(차이 존재)은 정상
  - `(*Manager).Export(ref, output string) error`
  - 제거: `Initialize`, `IsInitialized`, `Sync`, `VerifyFossil`, `CommandRunner`, `Manager.Store`, `Manager.Config`
  - `launch.go`에 신규: `openWorkspace() (string, *config.Config, error)` — Task 5/6이 사용

- [ ] **Step 1: history 패키지 테스트 재작성** — `internal/history/manager_test.go`를 새로 작성한다. fossil mock(CommandRunner) 기반 테스트를 전부 버리고 파일시스템 검증으로 바꾼다. 핵심 테스트:

```go
// internal/history/manager_test.go
package history

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// newTestManager는 runtime 디렉토리에 최소 파이프라인 산출물을 깔아 둔 Manager를 만든다.
func newTestManager(t *testing.T) (*Manager, string) {
	t.Helper()
	root := t.TempDir()
	pipelineDir := filepath.Join(root, ".pylon", "runtime", "pipe-1")
	if err := os.MkdirAll(pipelineDir, 0755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(pipelineDir, "requirement.md"), []byte("# 요구사항"), 0644)
	os.WriteFile(filepath.Join(pipelineDir, "tasks.json"),
		[]byte(`{"tasks":[{"id":"T1","repo":"app","secret_field":"drop-me"}]}`), 0644)
	os.WriteFile(filepath.Join(pipelineDir, "status.json"),
		[]byte(`{"status":"completed","stage":"done","noise":"drop-me"}`), 0644)
	m := NewManager(root)
	m.Now = func() time.Time { return time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC) }
	return m, root
}

func TestCheckpointCreatesSnapshotDirectory(t *testing.T) {
	m, root := newTestManager(t)
	result, err := m.Checkpoint("pipe-1", PhasePlanned)
	if err != nil {
		t.Fatalf("Checkpoint 실패: %v", err)
	}
	if result.Ref != "pipe-1/planned" || result.Duplicate {
		t.Fatalf("결과 불일치: %+v", result)
	}
	snapDir := filepath.Join(root, ".pylon", "history", "pipelines", "pipe-1", "planned")
	data, err := os.ReadFile(filepath.Join(snapDir, "manifest.json"))
	if err != nil {
		t.Fatalf("manifest.json이 있어야 한다: %v", err)
	}
	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Digest != result.Digest || manifest.PipelineID != "pipe-1" {
		t.Fatalf("manifest 불일치: %+v vs %+v", manifest, result)
	}
	if _, err := os.Stat(filepath.Join(snapDir, "requirement.md")); err != nil {
		t.Error("requirement.md가 스냅샷에 있어야 한다")
	}
	if manifest.AffectedProjects[0] != "app" {
		t.Errorf("affected_projects: %v", manifest.AffectedProjects)
	}
}

func TestCheckpointDeduplicatesByDigest(t *testing.T) {
	m, _ := newTestManager(t)
	first, err := m.Checkpoint("pipe-1", PhasePlanned)
	if err != nil {
		t.Fatal(err)
	}
	second, err := m.Checkpoint("pipe-1", PhasePlanned)
	if err != nil {
		t.Fatal(err)
	}
	if !second.Duplicate || second.Digest != first.Digest {
		t.Fatalf("동일 내용 재체크포인트는 Duplicate여야 한다: %+v", second)
	}
}

func TestTerminalPhaseCopiesMemorySnapshot(t *testing.T) {
	m, root := newTestManager(t)
	memFile := filepath.Join(root, ".pylon", "memory", "app", "learning", "k.md")
	os.MkdirAll(filepath.Dir(memFile), 0755)
	os.WriteFile(memFile, []byte("---\nkey: k\ncategory: learning\nconfidence: 0.8\ncreated_at: 2026-07-23T00:00:00Z\n---\n\n내용\n"), 0644)

	if _, err := m.Checkpoint("pipe-1", PhaseCompleted); err != nil {
		t.Fatalf("Checkpoint 실패: %v", err)
	}
	copied := filepath.Join(root, ".pylon", "history", "pipelines", "pipe-1", "completed", "memory", "app", "learning", "k.md")
	if _, err := os.Stat(copied); err != nil {
		t.Errorf("종료 체크포인트에 메모리 스냅샷이 복사되어야 한다: %v", err)
	}
}

func TestCheckpointCuratesJSONKeys(t *testing.T) {
	m, root := newTestManager(t)
	if _, err := m.Checkpoint("pipe-1", PhaseCompleted); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, ".pylon", "history", "pipelines", "pipe-1", "completed", "status-summary.json"))
	if err != nil {
		t.Fatalf("status-summary.json이 있어야 한다: %v", err)
	}
	if strings.Contains(string(data), "drop-me") {
		t.Error("allowlist에 없는 키는 제거되어야 한다")
	}
}

func TestLogSortsByRecordedAtDesc(t *testing.T) {
	m, root := newTestManager(t)
	base := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	m.Now = func() time.Time { return base }
	m.Checkpoint("pipe-1", PhasePlanned)

	// 두 번째 파이프라인 — 더 늦은 시각, 다른 내용
	pipelineDir := filepath.Join(root, ".pylon", "runtime", "pipe-2")
	os.MkdirAll(pipelineDir, 0755)
	os.WriteFile(filepath.Join(pipelineDir, "requirement.md"), []byte("# 다른 요구사항"), 0644)
	m.Now = func() time.Time { return base.Add(time.Hour) }
	m.Checkpoint("pipe-2", PhasePlanned)

	entries, err := m.Log("", 20)
	if err != nil || len(entries) != 2 {
		t.Fatalf("2건: %v, err=%v", entries, err)
	}
	if entries[0].PipelineID != "pipe-2" {
		t.Errorf("최신이 먼저: %+v", entries)
	}
	filtered, err := m.Log("pipe-1", 20)
	if err != nil || len(filtered) != 1 || filtered[0].PipelineID != "pipe-1" {
		t.Errorf("파이프라인 필터: %+v, err=%v", filtered, err)
	}
}

func TestShowAndExport(t *testing.T) {
	m, _ := newTestManager(t)
	m.Checkpoint("pipe-1", PhasePlanned)

	manifest, files, err := m.Show("pipe-1/planned")
	if err != nil || manifest == nil || len(files) == 0 {
		t.Fatalf("Show 실패: %+v, %v, err=%v", manifest, files, err)
	}
	if _, _, err := m.Show("ghost/planned"); err == nil {
		t.Error("없는 ref는 에러여야 한다")
	}

	out := filepath.Join(t.TempDir(), "export")
	if err := m.Export("pipe-1/planned", out); err != nil {
		t.Fatalf("Export 실패: %v", err)
	}
	if _, err := os.Stat(filepath.Join(out, "requirement.md")); err != nil {
		t.Errorf("export 결과 누락: %v", err)
	}
}

func TestDiffBetweenPhases(t *testing.T) {
	m, root := newTestManager(t)
	m.Checkpoint("pipe-1", PhasePlanned)
	os.WriteFile(filepath.Join(root, ".pylon", "runtime", "pipe-1", "requirement.md"), []byte("# 수정된 요구사항"), 0644)
	m.Checkpoint("pipe-1", PhaseExecuted)

	out, err := m.Diff("pipe-1/planned", "pipe-1/executed")
	if err != nil {
		t.Fatalf("Diff 실패: %v", err)
	}
	if !strings.Contains(out, "수정된 요구사항") {
		t.Errorf("diff 출력에 변경 내용이 있어야 한다:\n%s", out)
	}
}
```

- [ ] **Step 2: 실패 확인** — Run: `go test ./internal/history/ -v` → FAIL (NewManager 시그니처 불일치 등)

- [ ] **Step 3: `manager.go` 재작성** — 아래 원칙으로 전면 교체한다.

**유지(기존 코드 그대로 복사):** `Phase` 상수/`validPhases`/`isTerminalPhase`, `Manifest`(필드 추가), `stageCheckpoint`, `summarizeResultFiles`, `executionKeys`/`verificationKeys`/`prKeys`/`statusKeys`, `summarizeJSONFile`, `curateJSON`, `inspectPipeline`, `collectStringField`, `digestTree`, `validatePipelineID`, `phaseLabel`, `copyIfExists`.

**삭제:** `CommandRunner`/`fossilRunner`, `VerifyFossil`, `Initialize`/`IsInitialized`/`checkoutInitialized`, `Sync`, `parseCheckin`, `extractTarball`, `historyState`/`checkpointState`/`loadState`/`saveState`, `acquireLock`/`writeJSONAtomic`(fsutil로 이동), `repositoryPath`/`checkoutDir`. import에서 `archive/tar`, `compress/gzip`, `bufio`, `os/exec`(Diff에서 재사용), `config`, `store` 정리.

**신규/변경 핵심 코드:**

```go
// Package history manages the file-based, curated workspace history.
// 체크포인트 1건 = .pylon/history/pipelines/<pipeline-id>/<phase>/ 디렉토리.
package history

type Manager struct {
	Root string
	Now  func() time.Time
}

func NewManager(root string) *Manager {
	return &Manager{Root: root, Now: time.Now}
}

func (m *Manager) historyDir() string   { return layout.HistoryDir(m.Root) }
func (m *Manager) pipelinesDir() string { return filepath.Join(m.historyDir(), "pipelines") }

func (m *Manager) phaseDir(pipelineID string, phase Phase) string {
	return filepath.Join(m.pipelinesDir(), pipelineID, string(phase))
}

type Manifest struct {
	SchemaVersion    int               `json:"schema_version"` // 파일 기반 포맷은 2
	PipelineID       string            `json:"pipeline_id"`
	Phase            Phase             `json:"phase"`
	RecordedAt       time.Time         `json:"recorded_at"`
	Status           string            `json:"status"`
	Digest           string            `json:"digest"`
	AffectedProjects []string          `json:"affected_projects"`
	Artifacts        map[string]string `json:"artifacts"`
}

type CheckpointResult struct {
	PipelineID string `json:"pipeline_id"`
	Phase      Phase  `json:"phase"`
	Ref        string `json:"ref"`
	Digest     string `json:"digest"`
	Duplicate  bool   `json:"duplicate"`
}

func (m *Manager) Checkpoint(pipelineID string, phase Phase) (CheckpointResult, error) {
	result := CheckpointResult{PipelineID: pipelineID, Phase: phase}
	if err := validatePipelineID(pipelineID); err != nil {
		return result, err
	}
	if !validPhases[phase] {
		return result, fmt.Errorf("지원하지 않는 history phase: %s", phase)
	}
	if err := os.MkdirAll(m.pipelinesDir(), 0755); err != nil {
		return result, err
	}

	unlock, err := fsutil.AcquireLock(filepath.Join(m.historyDir(), ".checkpoint.lock"), 10*time.Second)
	if err != nil {
		return result, err
	}
	defer unlock()

	sourceDir := filepath.Join(m.Root, ".pylon", "runtime", pipelineID)
	if info, err := os.Stat(sourceDir); err != nil || !info.IsDir() {
		return result, fmt.Errorf("파이프라인 디렉토리를 찾을 수 없습니다: %s", sourceDir)
	}

	tempDir, err := os.MkdirTemp(m.pipelinesDir(), "."+pipelineID+"-")
	if err != nil {
		return result, err
	}
	defer os.RemoveAll(tempDir)

	projects, status, err := m.stageCheckpoint(sourceDir, tempDir, phase)
	if err != nil {
		return result, err
	}
	digest, artifacts, err := digestTree(tempDir)
	if err != nil {
		return result, err
	}
	result.Ref = pipelineID + "/" + string(phase)
	result.Digest = digest

	target := m.phaseDir(pipelineID, phase)
	prev, err := readManifest(target)
	if err != nil {
		return result, err
	}
	if prev != nil && prev.Digest == digest {
		result.Duplicate = true
		return result, nil
	}

	manifest := Manifest{
		SchemaVersion: 2, PipelineID: pipelineID, Phase: phase,
		RecordedAt: m.Now().UTC(), Status: status, Digest: digest,
		AffectedProjects: projects, Artifacts: artifacts,
	}
	if err := fsutil.WriteJSONAtomic(filepath.Join(tempDir, "manifest.json"), manifest); err != nil {
		return result, err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return result, err
	}
	if err := os.RemoveAll(target); err != nil {
		return result, err
	}
	if err := os.Rename(tempDir, target); err != nil {
		return result, err
	}
	return result, nil
}

func readManifest(dir string) (*Manifest, error) {
	data, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("manifest 파싱 실패 (%s): %w", dir, err)
	}
	return &m, nil
}
```

`exportMemory`는 SQLite 대신 md 디렉토리 복사로 교체 (stageCheckpoint의 terminal 분기에서 기존과 동일하게 호출):

```go
// exportMemory copies the affected projects' markdown memory into the snapshot.
func (m *Manager) exportMemory(destDir string, projects []string) error {
	for _, project := range projects {
		projectID := filepath.Base(project)
		if project == "." {
			projectID = filepath.Base(m.Root)
		}
		src := layout.ProjectMemoryDir(m.Root, projectID)
		if info, err := os.Stat(src); err != nil || !info.IsDir() {
			continue
		}
		if err := fsutil.CopyDir(src, filepath.Join(destDir, "memory", projectID)); err != nil {
			return err
		}
	}
	return nil
}
```

Log/Show/Diff/Export:

```go
type LogEntry struct {
	Ref        string    `json:"ref"`
	PipelineID string    `json:"pipeline_id"`
	Phase      Phase     `json:"phase"`
	RecordedAt time.Time `json:"recorded_at"`
	Status     string    `json:"status"`
}

// Log returns checkpoints sorted by recording time, newest first.
func (m *Manager) Log(pipelineID string, limit int) ([]LogEntry, error) {
	if limit <= 0 {
		limit = 20
	}
	pattern := filepath.Join(m.pipelinesDir(), "*", "*", "manifest.json")
	if pipelineID != "" {
		if err := validatePipelineID(pipelineID); err != nil {
			return nil, err
		}
		pattern = filepath.Join(m.pipelinesDir(), pipelineID, "*", "manifest.json")
	}
	paths, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}
	var entries []LogEntry
	for _, path := range paths {
		manifest, err := readManifest(filepath.Dir(path))
		if err != nil {
			return nil, err
		}
		if manifest == nil {
			continue
		}
		entries = append(entries, LogEntry{
			Ref:        manifest.PipelineID + "/" + string(manifest.Phase),
			PipelineID: manifest.PipelineID,
			Phase:      manifest.Phase,
			RecordedAt: manifest.RecordedAt,
			Status:     manifest.Status,
		})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].RecordedAt.After(entries[j].RecordedAt) })
	if len(entries) > limit {
		entries = entries[:limit]
	}
	return entries, nil
}

// resolveRef parses "<pipeline-id>/<phase>" into an existing snapshot directory.
func (m *Manager) resolveRef(ref string) (string, error) {
	id, phase, ok := strings.Cut(ref, "/")
	if !ok || id == "" || phase == "" {
		return "", fmt.Errorf("ref 형식은 <pipeline-id>/<phase> 입니다: %q", ref)
	}
	if err := validatePipelineID(id); err != nil {
		return "", err
	}
	if !validPhases[Phase(phase)] {
		return "", fmt.Errorf("지원하지 않는 phase: %s", phase)
	}
	dir := m.phaseDir(id, Phase(phase))
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return "", fmt.Errorf("체크포인트를 찾을 수 없습니다: %s", ref)
	}
	return dir, nil
}

// Show returns the manifest and sorted artifact list for a checkpoint ref.
func (m *Manager) Show(ref string) (*Manifest, []string, error) {
	dir, err := m.resolveRef(ref)
	if err != nil {
		return nil, nil, err
	}
	manifest, err := readManifest(dir)
	if err != nil {
		return nil, nil, err
	}
	if manifest == nil {
		return nil, nil, fmt.Errorf("manifest.json이 없습니다: %s", ref)
	}
	files := make([]string, 0, len(manifest.Artifacts))
	for name := range manifest.Artifacts {
		files = append(files, name)
	}
	sort.Strings(files)
	return manifest, files, nil
}

// Diff runs POSIX diff -ru between two checkpoint snapshots.
func (m *Manager) Diff(from, to string) (string, error) {
	fromDir, err := m.resolveRef(from)
	if err != nil {
		return "", err
	}
	toDir, err := m.resolveRef(to)
	if err != nil {
		return "", err
	}
	cmd := exec.Command("diff", "-ru", fromDir, toDir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			return string(out), nil // 종료 코드 1 = 차이 존재 (정상)
		}
		return string(out), fmt.Errorf("diff 실행 실패: %w", err)
	}
	return string(out), nil
}

// Export copies a checkpoint snapshot to a new directory.
func (m *Manager) Export(ref, output string) error {
	if output == "" {
		return fmt.Errorf("output 경로가 필요합니다")
	}
	dir, err := m.resolveRef(ref)
	if err != nil {
		return err
	}
	absOutput, err := filepath.Abs(output)
	if err != nil {
		return err
	}
	return fsutil.CopyDir(dir, absOutput)
}
```

- [ ] **Step 4: history 패키지 테스트 통과 확인** — Run: `go test ./internal/history/ -v` → PASS

- [ ] **Step 5: `internal/cli/history.go` 재작성** — `init`/`sync` 서브커맨드 제거, 새 Manager 사용:

```go
// internal/cli/history.go
package cli

import (
	"encoding/json"
	"fmt"

	"github.com/kyago/pylon/internal/history"
	"github.com/spf13/cobra"
)

func newHistoryCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "history",
		Short: "Manage file-based workspace history",
	}
	cmd.AddCommand(
		newHistoryCheckpointCmd(),
		newHistoryLogCmd(),
		newHistoryShowCmd(),
		newHistoryDiffCmd(),
		newHistoryExportCmd(),
	)
	return cmd
}

func withHistoryManager(run func(*history.Manager) error) error {
	root, _, err := openWorkspace()
	if err != nil {
		return err
	}
	return run(history.NewManager(root))
}

func newHistoryCheckpointCmd() *cobra.Command {
	var pipelineID, phase string
	cmd := &cobra.Command{
		Use:   "checkpoint",
		Short: "Create a curated pipeline history checkpoint",
		RunE: func(cmd *cobra.Command, args []string) error {
			if pipelineID == "" || phase == "" {
				return fmt.Errorf("--pipeline과 --phase가 필요합니다")
			}
			return withHistoryManager(func(manager *history.Manager) error {
				result, err := manager.Checkpoint(pipelineID, history.Phase(phase))
				if err != nil {
					return err
				}
				if flagJSON {
					return printJSON(result)
				}
				label := "체크포인트 기록 완료"
				if result.Duplicate {
					label = "동일 내용 — 기존 체크포인트 유지"
				}
				fmt.Printf("✓ %s (%s)\n", label, result.Ref)
				return nil
			})
		},
	}
	cmd.Flags().StringVar(&pipelineID, "pipeline", "", "pipeline ID")
	cmd.Flags().StringVar(&phase, "phase", "", "planned, executed, completed, cancelled, or failed")
	return cmd
}

func newHistoryLogCmd() *cobra.Command {
	var pipelineID string
	var limit int
	cmd := &cobra.Command{
		Use:   "log",
		Short: "Show checkpoint history",
		RunE: func(cmd *cobra.Command, args []string) error {
			return withHistoryManager(func(manager *history.Manager) error {
				entries, err := manager.Log(pipelineID, limit)
				if err != nil {
					return err
				}
				if flagJSON {
					return printJSON(entries)
				}
				if len(entries) == 0 {
					fmt.Println("기록된 체크포인트가 없습니다")
					return nil
				}
				for _, e := range entries {
					fmt.Printf("%s  %-30s %-10s %s\n",
						e.RecordedAt.Format("2006-01-02T15:04:05Z"), e.PipelineID, e.Phase, e.Status)
				}
				return nil
			})
		},
	}
	cmd.Flags().StringVar(&pipelineID, "pipeline", "", "filter by pipeline ID")
	cmd.Flags().IntVar(&limit, "limit", 20, "maximum checkpoints")
	return cmd
}

func newHistoryShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <pipeline-id>/<phase>",
		Short: "Show a checkpoint snapshot",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withHistoryManager(func(manager *history.Manager) error {
				manifest, files, err := manager.Show(args[0])
				if err != nil {
					return err
				}
				if flagJSON {
					return printJSON(map[string]any{"manifest": manifest, "files": files})
				}
				fmt.Printf("ref:      %s/%s\n", manifest.PipelineID, manifest.Phase)
				fmt.Printf("recorded: %s\n", manifest.RecordedAt.Format("2006-01-02T15:04:05Z"))
				fmt.Printf("status:   %s\n", manifest.Status)
				fmt.Printf("projects: %v\n", manifest.AffectedProjects)
				fmt.Println("files:")
				for _, f := range files {
					fmt.Printf("  %s\n", f)
				}
				return nil
			})
		},
	}
}

func newHistoryDiffCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "diff <from-ref> <to-ref>",
		Short: "Diff two checkpoint snapshots",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return withHistoryManager(func(manager *history.Manager) error {
				output, err := manager.Diff(args[0], args[1])
				if err != nil {
					return err
				}
				if flagJSON {
					return printJSON(map[string]string{"from": args[0], "to": args[1], "output": output})
				}
				fmt.Println(output)
				return nil
			})
		},
	}
}

func newHistoryExportCmd() *cobra.Command {
	var output string
	cmd := &cobra.Command{
		Use:   "export <pipeline-id>/<phase>",
		Short: "Export a checkpoint snapshot to a new directory",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if output == "" {
				return fmt.Errorf("--output이 필요합니다")
			}
			return withHistoryManager(func(manager *history.Manager) error {
				if err := manager.Export(args[0], output); err != nil {
					return err
				}
				if flagJSON {
					return printJSON(map[string]string{"status": "ok", "output": output})
				}
				fmt.Printf("✓ history snapshot export 완료: %s\n", output)
				return nil
			})
		},
	}
	cmd.Flags().StringVar(&output, "output", "", "new destination directory")
	return cmd
}

func printJSON(value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}
```

- [ ] **Step 6: `launch.go` 수정** — 33행의 `if err := prepareHistory(root, cfg.History); err != nil { ... }` 블록과 85-92행의 `prepareHistory` 함수를 삭제하고, `openWorkspaceStore` 위에 신규 헬퍼를 추가한다 (`openWorkspaceStore`는 Task 5-6에서 소비자 이관 후 Task 7에서 삭제):

```go
// openWorkspace finds the workspace root and loads config.
func openWorkspace() (string, *config.Config, error) {
	root, err := resolveRoot()
	if err != nil {
		return "", nil, err
	}
	cfg, err := config.LoadConfig(layout.ConfigPath(root))
	if err != nil {
		return "", nil, fmt.Errorf("failed to load config: %w", err)
	}
	return root, cfg, nil
}
```

- [ ] **Step 7: `cancel.go` 수정** — config/store 로딩 없이 체크포인트만 시도하도록 교체. 기존:

```go
		checkpointed := false
		if cfg, cfgErr := config.LoadConfig(layout.ConfigPath(root)); cfgErr == nil {
			if s, storeErr := store.NewStore(layout.DBPath(root)); storeErr == nil {
				if s.Migrate() == nil {
					mgr := history.NewManager(root, cfg.History, s, nil)
					if _, cpErr := mgr.Checkpoint(pipelineID, history.PhaseCancelled); cpErr == nil {
						checkpointed = true
					}
				}
				s.Close()
			}
		}
```

교체 후:

```go
		checkpointed := false
		if _, cpErr := history.NewManager(root).Checkpoint(pipelineID, history.PhaseCancelled); cpErr == nil {
			checkpointed = true
		}
```

사용하지 않게 된 `config`/`store` import를 정리한다 (cancel.go 내 다른 사용처가 있는지 `goimports`로 확인).

- [ ] **Step 8: `init_cmd.go` 수정** — 199-206행(config 로드 + `history.NewManager(...).Initialize()`) 블록을 삭제한다. 242행 출력을 다음으로 교체:

```go
	fmt.Println("  .pylon/history/            - pipeline work history (file-based)")
```

`history` import가 다른 곳에서 안 쓰이면 제거. (store 관련 코드는 Task 6에서 제거하므로 여기서는 건드리지 않는다.)

- [ ] **Step 9: `doctor.go` 수정** — `checks` 슬라이스에서 fossil 항목(33-38행)을 삭제하고, 16행의 `history` import를 제거한다. 63행 Long 설명의 `(fossil, git, gh, claude)`를 `(git, gh, claude)`로 수정.

- [ ] **Step 10: CLI 테스트 갱신**
  - `internal/cli/history_test.go`: fossil runner mock 기반 테스트 제거. 새 테스트는 `t.TempDir()` 워크스페이스(`.pylon/config.yml`에 `version: "0.1"` 작성)에 runtime 디렉토리를 깔고 checkpoint→log를 검증한다.
  - `internal/cli/launch_history_test.go`: `prepareHistory` 테스트 삭제 (파일 자체가 prepareHistory만 다루면 파일 삭제).
  - `internal/cli/cancel_test.go`: fossil mock 의존 부분을 파일 존재 검증으로 교체 — 취소 후 `.pylon/history/pipelines/<id>/cancelled/manifest.json` 존재 확인.
  - `internal/cli/doctor_test.go`: fossil 체크 기대값 제거.
  - `internal/cli/init_cmd_test.go`: fossil 초기화 검증과 `installFakeFossil` 헬퍼 및 그 호출부를 전부 제거.

- [ ] **Step 11: 전체 테스트** — Run: `make build && make test` → PASS

- [ ] **Step 12: 커밋**

```bash
git add internal/history/ internal/cli/
git commit -m "refactor: Fossil 제거 — 디렉토리 스냅샷 기반 history로 전환"
```

---

### Task 5: 메모리 CLI를 파일 저장소로 전환

`mem.go`/`sync_memory.go`를 `memory.Store`로 전환하고, SQLite 기반 `memory/manager.go`를 삭제한다.

**Files:**
- Modify: `internal/cli/mem.go`
- Modify: `internal/cli/sync_memory.go`
- Delete: `internal/memory/manager.go`, `internal/memory/manager_test.go`
- Test: `internal/cli/sync_memory_test.go` 갱신

**Interfaces:**
- Consumes: `memory.NewStore(root)`, `(*Store).Insert/Search/List/ListByCategory/Delete/StoreLearnings` (Task 3), `openWorkspace()` (Task 4).
- Produces: CLI 동작 — `mem search/store/list/delete/prune`(--dedup 제거). JSON 출력 키는 기존과 동일(`key`, `category`, `content`, `confidence`, `rank` 등).

- [ ] **Step 1: `mem.go` 전환**
  - 모든 `openWorkspaceStore()` 호출을 `openWorkspace()`로 바꾸고 `memory.NewStore(root)`를 생성한다. import에서 `store` 제거, `memory`와 `errors` 추가(ErrDuplicate 감지용).
  - `runMemSearch`: `s.SearchMemory(project, query, limit)` → `memStore.Search(project, query, limit)`. 출력 코드는 필드명이 동일하므로(Key/Category/Content/Confidence/Rank) 그대로 유지.
  - `runMemStore`: `store.MemoryEntry{...}` + `s.InsertMemory(entry)` → 아래로 교체:

```go
	entry := &memory.Entry{
		ProjectID:  project,
		Category:   category,
		Key:        key,
		Content:    content,
		Author:     author,
		Confidence: confidence,
	}
	if err := memStore.Insert(entry); err != nil {
		if errors.Is(err, memory.ErrDuplicate) {
			if flagJSON {
				data, _ := json.Marshal(map[string]string{
					"status": "skip",
					"reason": "duplicate content",
					"path":   entry.Path,
				})
				fmt.Println(string(data))
			} else {
				fmt.Printf("동일한 내용이 이미 저장되어 있어 건너뜁니다: %s\n", entry.Path)
			}
			return nil
		}
		return fmt.Errorf("failed to store memory: %w", err)
	}
	if flagJSON {
		data, _ := json.Marshal(map[string]string{
			"path":    entry.Path,
			"project": project,
			"key":     key,
			"status":  "ok",
		})
		fmt.Println(string(data))
	} else {
		fmt.Printf("✓ 메모리 저장: %s\n", entry.Path)
	}
```

  - `runMemList`: `GetMemoryByCategory`/`ListProjectMemory` → `ListByCategory`/`List`. 출력 구조는 유지.
  - `runMemDelete`: `s.DeleteMemory(project, category, key, dryRun)` → `memStore.Delete(project, category, key, dryRun)` (시그니처 동일 형태).
  - `runMemPrune`: `--dedup` 플래그, `DedupMemoryChanges` 분기, `s.Vacuum()` 호출 제거. `countFn`은 `memStore.Delete(project, category, "", dry)`만 남긴다. `newMemPruneCmd`의 Long 도움말에서 `--dedup` 예시 2줄 삭제, 플래그 정의에서 `dedup` 제거, `if !dedup && category == ""` → `if category == ""`.

- [ ] **Step 2: `sync_memory.go` 전환**
  - `runSyncFromSession`: `openWorkspaceStore()` → `openWorkspace()`, `memory.NewManager(s, cfg.Memory)` → `memory.NewStore(root)`, `mgr.StoreLearnings(...)` → `memStore.StoreLearnings(...)`. `defer s.Close()` 제거.
  - `runSyncIncremental`의 메시지에서 Fossil 표현 수정: `"파일 변경 이력은 history 체크포인트가 담당합니다 — 저장을 건너뜁니다"`, JSON reason은 `"change tracking moved to history checkpoints"`.
  - `--incremental`/`--file` deprecated 문구도 동일하게 `Fossil` → `history 체크포인트`로 수정.
  - Long 도움말의 `project_memory(SQLite + BM25 FTS)` → `프로젝트 메모리(.pylon/memory/ 마크다운 파일)`.

- [ ] **Step 3: 구 SQLite Manager 삭제**

```bash
git rm internal/memory/manager.go internal/memory/manager_test.go
```

- [ ] **Step 4: `sync_memory_test.go` 갱신** — SQLite 검증 대신 파일 검증으로 교체. 핵심 테스트 형태:

```go
func TestSyncFromSessionWritesMarkdown(t *testing.T) {
	// 테스트 워크스페이스: .pylon/config.yml 생성 후 작업 디렉토리 변경
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, ".pylon"), 0755)
	os.WriteFile(filepath.Join(root, ".pylon", "config.yml"), []byte("version: \"0.1\"\n"), 0644)
	t.Chdir(root)

	if err := runSyncFromSession("app", "architect", "- 학습 내용 하나\n- 학습 내용 둘"); err != nil {
		t.Fatalf("sync 실패: %v", err)
	}
	entries, err := memory.NewStore(root).ListByCategory("app", "learning")
	if err != nil || len(entries) != 2 {
		t.Fatalf("2건 저장되어야 한다: %d, err=%v", len(entries), err)
	}
}
```

(기존 파일의 `parseLearnings`/`tryParseJSONLearnings` 단위 테스트는 저장소와 무관하므로 그대로 유지한다.)

- [ ] **Step 5: 전체 테스트** — Run: `make build && make test` → PASS

- [ ] **Step 6: 커밋**

```bash
git add -A internal/cli/mem.go internal/cli/sync_memory.go internal/cli/sync_memory_test.go internal/memory/
git commit -m "refactor: mem/sync-memory CLI를 md 파일 메모리 저장소로 전환"
```

---

### Task 6: projects 레지스트리 제거

`projects` 테이블은 `config.yml` + 파일시스템 스캔(`config.DiscoverProjects`)의 파생 데이터다. 저장소를 없애고 소비자가 직접 스캔하게 한다.

**Files:**
- Delete: `internal/cli/sync_projects.go` (+ 존재 시 `sync_projects_test.go`)
- Modify: `internal/cli/root.go:73` (`newSyncProjectsCmd()` 등록 제거)
- Modify: `internal/cli/init_cmd.go` (store open/Migrate/UpsertProject 제거)
- Modify: `internal/cli/add_project.go:161-163` 호출부와 `add_project.go:576-595` `registerProjectInDB` 삭제
- Modify: `internal/cli/delete_project.go`
- Test: `internal/cli/init_cmd_test.go`, `internal/cli/add_project_test.go`(존재 시), `internal/cli/delete_project_test.go`(존재 시) 갱신

**Interfaces:**
- Consumes: `config.DiscoverProjects(root) ([]config.ProjectInfo, error)` (기존), `memory.NewStore(root).DeleteProject(name)` (Task 3).
- Produces: 없음 (제거 태스크).

- [ ] **Step 1: `init_cmd.go` 정리** — "Step 6: Initialize DB and sync discovered projects" 블록에서 `store.NewStore`/`s.Migrate()`/`s.UpsertProject(...)` 루프와 `"✓ %d project(s) registered in DB"` 출력을 제거한다. `DiscoverProjects` + `excludePylonFromRepo` 루프는 유지한다. `store` import 제거.

- [ ] **Step 2: `add_project.go` 정리** — 161-163행의 `registerProjectInDB(...)` 호출(에러 처리 포함)과 576-595행의 함수 본문을 삭제. `store`/`layout` import 사용 여부 확인 후 정리.

- [ ] **Step 3: `delete_project.go` 재작성** — 레지스트리 대신 스캔으로 프로젝트를 찾고, 메모리는 md 디렉토리를 삭제한다:

```go
func runDeleteProject(name string, purge, force bool) error {
	if err := validateProjectName(name); err != nil {
		return err
	}
	root, _, err := openWorkspace()
	if err != nil {
		return err
	}

	// 레지스트리 대신 워크스페이스 스캔으로 프로젝트를 찾는다.
	discovered, err := config.DiscoverProjects(root)
	if err != nil {
		return fmt.Errorf("프로젝트 탐색 실패: %w", err)
	}
	var projPath string
	for _, p := range discovered {
		if p.Name == name {
			projPath = p.Path
			break
		}
	}
	if projPath == "" {
		return fmt.Errorf("project %q is not registered", name)
	}

	projDir, dirSafe := resolveProjectDir(root, projPath)
	removeTarget := ""
	if dirSafe {
		if purge {
			removeTarget = projDir
		} else {
			marker := layout.PylonDir(projDir)
			if dirExists(marker) {
				removeTarget = marker
			}
		}
	}

	if !force && !flagJSON {
		fmt.Printf("Delete project %q:\n", name)
		fmt.Println("  - memory records (.pylon/memory/" + name + "/)")
		if purge {
			if removeTarget != "" {
				fmt.Printf("  - directory: %s\n", removeTarget)
			} else {
				fmt.Printf("  ⚠ --purge requested but directory %q is outside the workspace or missing; it will be kept\n", projPath)
			}
		} else if removeTarget != "" {
			fmt.Printf("  - .pylon/ marker: %s\n", removeTarget)
		}
		fmt.Printf("계속하시겠습니까? [y/N]: ")
		var answer string
		fmt.Scanln(&answer)
		if a := strings.ToLower(strings.TrimSpace(answer)); a != "y" && a != "yes" {
			fmt.Println("취소되었습니다")
			return nil
		}
	}

	memDeleted, err := memory.NewStore(root).DeleteProject(name)
	if err != nil {
		return fmt.Errorf("failed to delete project memory: %w", err)
	}
	// ... 이하 removeTarget 삭제 및 결과 출력은 기존 코드 유지.
	// 기존 JSON 출력의 res.Memory 자리는 memDeleted로, res.Projects 자리는 1로 대체한다.
```

- [ ] **Step 4: `sync_projects.go` 삭제 및 등록 해제**

```bash
git rm internal/cli/sync_projects.go
```

`root.go`에서 `rootCmd.AddCommand(newSyncProjectsCmd())` 라인 삭제.

- [ ] **Step 5: 테스트 갱신** — init/add/delete 테스트에서 DB 검증(projects 테이블 조회) 부분을 제거하거나 파일 검증으로 교체. delete_project 테스트는 `.pylon/memory/<name>/` 디렉토리가 삭제되는지 확인하도록 수정.

- [ ] **Step 6: 전체 테스트** — Run: `make build && make test` → PASS

- [ ] **Step 7: 커밋**

```bash
git add -A internal/cli/
git commit -m "refactor: projects SQLite 레지스트리 제거 — DiscoverProjects 직접 사용"
```

---

### Task 7: `internal/store` 삭제 및 의존성/설정 정리

이 시점에는 store 소비자가 없어야 한다. 패키지·의존성·설정 필드·경로 헬퍼를 일괄 제거한다.

**Files:**
- Delete: `internal/store/` (전체)
- Modify: `internal/cli/launch.go` (`openWorkspaceStore` 삭제)
- Modify: `internal/config/config.go` (`HistoryConfig` 타입·필드·기본값, `MemoryConfig.CompactionThreshold` 제거)
- Modify: `internal/config/config_test.go` (해당 필드 테스트 제거)
- Modify: `internal/layout/layout.go` (`DBPath` 제거) + `layout_test.go`
- Modify: `go.mod`/`go.sum` (`go mod tidy`)

**Interfaces:**
- Consumes: 없음.
- Produces: 최종 `config.Config`에서 `History` 필드 부재 — 이후 태스크는 `cfg.History`를 참조하면 안 된다.

- [ ] **Step 1: 소비자 부재 확인**

Run: `grep -rn "internal/store\|openWorkspaceStore\|layout.DBPath\|cfg.History\|config.HistoryConfig" internal/ --include="*.go"`
Expected: `internal/store/` 자신과 launch.go의 `openWorkspaceStore` 정의, config.go의 타입 정의만 검출

- [ ] **Step 2: 삭제 실행**

```bash
git rm -r internal/store
```

`launch.go`에서 `openWorkspaceStore` 함수와 `store` import 삭제. `config.go`에서 `HistoryConfig` 타입, `Config.History` 필드, 기본값 `History: HistoryConfig{SyncOnCheckpoint: true}`, `MemoryConfig.CompactionThreshold` 필드를 삭제. `SyncConfigDefaults`와 init의 config.yml 템플릿에 `history:`/`remote:`/`sync_on_checkpoint:`/`compaction_threshold:` 문자열이 있으면 함께 삭제한다:

Run: `grep -rn "sync_on_checkpoint\|compaction_threshold\|history:" internal/ --include="*.go"`

`layout.go`의 `DBPath` 함수와 그 테스트를 삭제.

- [ ] **Step 3: 의존성 정리**

Run: `go mod tidy && grep -E "modernc|uuid" go.mod`
Expected: 검출 없음 (`modernc.org/sqlite`, `modernc.org/libc` 등 indirect 포함 전부 제거, `google/uuid`도 제거 — 다른 사용처가 있어 남으면 해당 사용처를 확인하고 유지)

- [ ] **Step 4: 전체 테스트** — Run: `make build && make test && make lint` → PASS

- [ ] **Step 5: 커밋**

```bash
git add -A
git commit -m "refactor: internal/store 및 SQLite 의존성 제거"
```

---

### Task 8: CLAUDE.md 생성 갱신 — 메모리 인덱스 주입 (proactive_injection 실구현)

**Files:**
- Modify: `internal/cli/launch_claudemd.go` (77-83행 "## 프로젝트 메모리" 섹션)
- Test: `internal/cli/launch_claudemd_test.go` (존재 시 갱신, 없으면 신규)

**Interfaces:**
- Consumes: `memory.NewStore(root).IndexMarkdown(project, maxBytes)` (Task 3), `buildRootCLAUDEMD(cfg *config.Config, projects []config.ProjectInfo, root string) string` (기존 시그니처 그대로).
- Produces: 생성된 CLAUDE.md에 프로젝트별 메모리 인덱스 섹션.

- [ ] **Step 1: 실패하는 테스트 작성**

```go
func TestBuildRootCLAUDEMDInjectsMemoryIndex(t *testing.T) {
	root := t.TempDir()
	memStore := memory.NewStore(root)
	memStore.Insert(&memory.Entry{ProjectID: "app", Category: "decision",
		Key: "저장소는 md 파일", Content: "SQLite 대신 md 파일을 쓴다", Confidence: 0.9})

	cfg := &config.Config{}
	cfg.Memory.ProactiveInjection = true
	projects := []config.ProjectInfo{{Name: "app", Path: filepath.Join(root, "app")}}

	out := buildRootCLAUDEMD(cfg, projects, root)
	if !strings.Contains(out, "저장소는 md 파일") {
		t.Error("메모리 인덱스가 주입되어야 한다")
	}

	cfg.Memory.ProactiveInjection = false
	out = buildRootCLAUDEMD(cfg, projects, root)
	if strings.Contains(out, "저장소는 md 파일") {
		t.Error("비활성화 시 주입되면 안 된다")
	}
}
```

- [ ] **Step 2: 실패 확인** — Run: `go test ./internal/cli/ -run TestBuildRootCLAUDEMDInjectsMemoryIndex -v` → FAIL

- [ ] **Step 3: 구현** — `launch_claudemd.go`의 기존 "Memory access" 섹션(77-83행)을 다음으로 교체:

```go
	// Memory access
	b.WriteString("## 프로젝트 메모리\n\n")
	b.WriteString("프로젝트 지식은 `.pylon/memory/<project>/` 아래 마크다운 파일입니다.\n")
	b.WriteString("Grep/Read로 직접 탐색하거나 `pylon mem` CLI를 사용합니다:\n")
	b.WriteString("```bash\n")
	b.WriteString("pylon mem search --project <name> --query \"검색어\"   # 토큰 매칭 검색\n")
	b.WriteString("pylon mem store --project <name> --key \"키\" --content \"내용\"  # 저장\n")
	b.WriteString("pylon mem list --project <name>                       # 목록\n")
	b.WriteString("```\n\n")

	// Proactive memory index injection
	if cfg.Memory.ProactiveInjection {
		maxTokens := cfg.Memory.ProactiveMaxTokens
		if maxTokens <= 0 {
			maxTokens = 2000
		}
		remaining := maxTokens * 4 // 대략적인 토큰→바이트 환산
		memStore := memory.NewStore(root)
		wroteHeader := false
		for _, p := range projects {
			if remaining <= 0 {
				break
			}
			index, err := memStore.IndexMarkdown(p.Name, remaining)
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

import에 `github.com/kyago/pylon/internal/memory` 추가.

- [ ] **Step 4: 통과 확인** — Run: `go test ./internal/cli/ -run TestBuildRootCLAUDEMD -v` → PASS

- [ ] **Step 5: 커밋**

```bash
git add internal/cli/launch_claudemd*.go
git commit -m "feat: CLAUDE.md 생성 시 프로젝트 메모리 인덱스 주입 (proactive_injection 실구현)"
```

---

### Task 9: 레거시 폐기 안내 + 문서/셋업 갱신

레거시 데이터는 이전하지 않는다(의사결정 D2/D3 — 마이그레이션 스크립트 없음, sqlite3 의존 없음). `/pl:migrate`는 폐기 절차만 안내하고, `pylon init`은 메모리 git 추적 셋업(D1)을 갖춘다.

**Files:**
- Modify: `internal/cli/commands/pl-migrate.md`
- Modify: `internal/cli/commands/pl-pipeline.md` (125행, 182행 Fossil 표현)
- Modify: `internal/cli/commands/pl-cancel.md` (Fossil 표현)
- Modify: `README.md`
- Modify: `internal/cli/init_cmd.go` (gitignore 주석, `.gitkeep`, git 힌트)
- Test: `internal/cli/init_cmd_test.go`

**Interfaces:**
- Consumes: `layout.MemoryDir` (Task 2).
- Produces: `/pl:migrate` = 레거시 파일 폐기 절차, `pylon init` = 메모리 git 추적 셋업 (D1).

- [ ] **Step 1: `pl-migrate.md` 재작성** — 기존 checkpoint 재기록/`mem prune --dedup` 절차를 전부 제거하고 폐기 절차로 교체:
  1. 구버전 판별: `.pylon/pylon.db` 또는 `.pylon/history/pylon-history.fossil` 존재 여부 확인
  2. 사용자 고지: "레거시 데이터(SQLite 메모리, Fossil 이력)는 새 버전으로 **이전되지 않고 삭제**됩니다" — 명시적 확인을 받은 뒤에만 진행
  3. 확인 후 삭제 — 기존 문서 관례대로 판별-삭제를 `&&` 체이닝으로 강제:

```bash
# 확인 후 실행 (되돌릴 수 없음)
test -f .pylon/pylon.db && rm .pylon/pylon.db
rm -rf .pylon/history/pylon-history.fossil .pylon/history/checkout .pylon/history/state.json
```

  4. 삭제 후 `pylon doctor`로 커맨드/설정 동기화 안내 (구버전 커맨드 md가 새 버전으로 교체됨)

- [ ] **Step 2: `pl-pipeline.md` / `pl-cancel.md` 문구 수정**
  - `pl-pipeline.md:125` "계획 산출물이 확정되면 Fossil 이력 체크포인트를 생성합니다:" → "계획 산출물이 확정되면 이력 체크포인트를 생성합니다:"
  - `pl-pipeline.md:182` "(이력은 Fossil에 보존됨)" → "(이력은 .pylon/history/에 보존됨)"
  - `pl-pipeline.md:319-323`의 mem search 안내에서 "BM25" 표현이 있으면 "토큰 매칭"으로 수정
  - `pl-cancel.md`의 checkpoint 관련 Fossil 언급 동일 수정

- [ ] **Step 3: README.md 갱신**
  - 12행 필수 도구 표에서 Fossil 행 삭제
  - 95-97행 마이그레이션 안내를 `/pl:migrate` 폐기 절차로 교체 (데이터 이전 없음 명시)
  - 297행 doctor 설명 `(fossil/git/gh/claude)` → `(git/gh/claude)`
  - 328행 "작업 이력" 섹션: "Fossil shadow checkout" → ".pylon/history/pipelines/ 디렉토리 스냅샷", 명령 목록에서 `history init`/`history sync` 제거, `history show <checkin>` → `history show <pipeline-id>/<phase>`
  - 워크스페이스 구조도(362-367행): `runtime/memory/` 삭제, `.pylon/memory/` 추가(git 추적 명시), `history/` 하위를 `pipelines/`로 교체, `pylon.db` 행 삭제
  - 설정 예시(415-423행): `memory.compaction_threshold`, `memory.session_archive`, `history:` 블록 삭제
  - 기술 스택 표(457행 부근): "저장소 SQLite (CGO-free, WAL 모드)" → "저장소 | 마크다운/JSON 파일 (.pylon/memory, .pylon/history)", "메모리 검색 FTS5 BM25" → "메모리 검색 | 토큰 매칭 + INDEX.md 주입", "작업 이력 Fossil shadow repository" → "작업 이력 | 디렉토리 스냅샷 (.pylon/history/pipelines)"
  - `sync-projects` 행 삭제

- [ ] **Step 4: init의 메모리 git 추적 셋업 (D1)** — `init_cmd.go`를 세 가지로 수정한다.

(a) gitignore 항목에 의도 주석 추가 — `.pylon/history/`는 유지(아카이브는 무시), `.pylon/memory/`는 **추가하지 않는다**(메모리는 git 추적이 원칙 — source of truth):

```go
	gitignoreEntries := []string{
		"# Pylon runtime (agent communication, state)",
		".pylon/runtime/",
		".pylon/conversations/",
		".pylon/history/",
		// .pylon/memory/ 는 의도적으로 제외 — 메모리는 git으로 추적/리뷰한다 (D1).
		"",
		"# Claude CLI agent symlinks (managed by pylon)",
		".claude/agents/",
		"",
	}
```

(b) gitignore 갱신 직후 `.pylon/memory/.gitkeep`을 생성해 빈 워크스페이스에서도 메모리 디렉토리가 커밋되게 한다:

```go
	// 메모리 저장소 루트를 미리 만들어 git 추적을 시작한다 (D1).
	if err := os.MkdirAll(layout.MemoryDir(workDir), 0755); err != nil {
		return fmt.Errorf("memory 디렉토리 생성 실패: %w", err)
	}
	if err := os.WriteFile(filepath.Join(layout.MemoryDir(workDir), ".gitkeep"), nil, 0644); err != nil {
		return fmt.Errorf(".gitkeep 생성 실패: %w", err)
	}
```

(c) 워크스페이스 루트가 git repo가 아니면 안내를 출력한다 (강제하지 않음 — 워크스페이스는 git repo 의무가 아님):

```go
	if _, err := os.Stat(filepath.Join(workDir, ".git")); os.IsNotExist(err) {
		fmt.Println("ℹ 워크스페이스가 git 저장소가 아닙니다 — 프로젝트 메모리(.pylon/memory/)의 버전 관리를 위해 'git init'을 권장합니다")
	}
```

`add-project`/`delete-project`는 추가 작업이 없다: 프로젝트별 메모리 디렉토리는 첫 `Insert`에서 lazy 생성되고(Task 3), 삭제는 `DeleteProject`가 처리한다(Task 6). 이 사실을 init_cmd.go 수정 PR 설명에 명시한다.

- [ ] **Step 5: init 테스트 추가** — `init_cmd_test.go`의 기존 패턴(`flagWorkspace` 교체 + `newInitCmd().Execute()`)을 따라 추가한다 (`strings` import 필요):

```go
func TestInitSetsUpTrackedMemoryDir(t *testing.T) {
	requireGit(t)
	tmp := t.TempDir()
	oldWorkspace := flagWorkspace
	flagWorkspace = tmp
	defer func() { flagWorkspace = oldWorkspace }()

	cmd := newInitCmd()
	if err := cmd.Execute(); err != nil {
		t.Fatalf("init failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmp, ".pylon", "memory", ".gitkeep")); err != nil {
		t.Errorf(".pylon/memory/.gitkeep이 생성되어야 한다: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(tmp, ".gitignore"))
	if err != nil {
		t.Fatalf(".gitignore 읽기 실패: %v", err)
	}
	if strings.Contains(string(data), ".pylon/memory") {
		t.Error(".pylon/memory는 git 추적 대상이어야 한다 — gitignore에 없어야 함 (D1)")
	}
	if !strings.Contains(string(data), ".pylon/history/") {
		t.Error(".pylon/history/는 계속 무시되어야 한다")
	}
}
```

- [ ] **Step 6: 전체 테스트** — Run: `make build && make test && make lint` → PASS

- [ ] **Step 7: 커밋**

```bash
git add internal/cli/commands/ README.md internal/cli/init_cmd.go internal/cli/init_cmd_test.go
git commit -m "docs: 레거시 폐기 절차 문서화 및 init 메모리 git 추적 셋업"
```

---

### Task 10: 종합 검증

**Files:** 없음 (검증 전용)

- [ ] **Step 1: 잔재 스위프**

Run: `grep -rni "fossil" internal/ README.md Makefile go.mod | grep -v pl-migrate.md`
Expected: 검출 없음 (pl-migrate.md의 레거시 파일 삭제 안내만 예외)

Run: `grep -rn "sqlite\|pylon\.db\|modernc\|database/sql" internal/ go.mod | grep -v pl-migrate.md`
Expected: 검출 없음

- [ ] **Step 2: 빌드/테스트/린트**

Run: `make build && make test && make lint`
Expected: 전부 PASS

- [ ] **Step 3: 스모크 테스트** — 임시 워크스페이스에서 실제 바이너리로 E2E 확인:

```bash
WORK=$(mktemp -d)
cd "$WORK"
pylon init  # 프롬프트가 있으면 문서에 따라 응답
# 1) 메모리 왕복
pylon mem store --project demo --key "테스트 키" --content "메모리를 파일로 관리한다" --category decision
pylon mem search --project demo --query "메모리"        # → 1건 검색 (조사 변형 매칭 확인)
pylon mem list --project demo                            # → 1건 표시
cat .pylon/memory/demo/INDEX.md                          # → 인덱스 존재
# 2) 이력 왕복
mkdir -p .pylon/runtime/pipe-smoke
echo '# req' > .pylon/runtime/pipe-smoke/requirement.md
echo '{"status":"completed"}' > .pylon/runtime/pipe-smoke/status.json
pylon history checkpoint --pipeline pipe-smoke --phase completed --json   # → {"ref":"pipe-smoke/completed",...}
pylon history log                                        # → 1건
pylon history show pipe-smoke/completed                  # → manifest 출력
pylon history export pipe-smoke/completed --output ./exported && ls exported/
# 3) doctor에 fossil 없음
pylon doctor                                             # → git/gh/claude만 체크
```

Expected: 모든 명령 성공, fossil/sqlite 관련 언급 없음

- [ ] **Step 4: 레거시 파일 공존/폐기 검증 (D3)** — 새 바이너리가 레거시 파일 존재에 영향받지 않고, `/pl:migrate`의 삭제 명령이 깨끗이 정리하는지 확인:

```bash
# 스모크 워크스페이스에 레거시 파일을 흉내낸다
touch .pylon/pylon.db
mkdir -p .pylon/history/checkout
touch .pylon/history/pylon-history.fossil .pylon/history/state.json

pylon mem list --project demo        # 레거시 파일 존재와 무관하게 정상 동작
pylon history log                    # 파일 기반 이력만 표시, fossil 파일 무시
pylon doctor                         # 에러/경고 없음 (감지 코드 없음 확인)

# pl-migrate.md의 폐기 절차 실행
test -f .pylon/pylon.db && rm .pylon/pylon.db
rm -rf .pylon/history/pylon-history.fossil .pylon/history/checkout .pylon/history/state.json
ls .pylon/history/                   # → pipelines/ 만 남음
```

- [ ] **Step 5: 최종 커밋** (검증 중 수정 사항이 있었다면)

```bash
git add -A
git commit -m "test: md-first 전환 종합 검증 반영"
```

---

## Self-Review 체크 결과

- **Spec 커버리지**: SQLite 제거(메모리 Task 3/5, projects Task 6, 패키지 Task 7), Fossil 제거(Task 4), 경량화(Task 7 의존성 정리 — modernc.org/sqlite는 바이너리 크기 수 MB 감소), proactive_injection 실구현(Task 8), 레거시 폐기 D2/D3 + 메모리 git 셋업 D1(Task 9), 중복 스킵 D4(Task 3/5), 문서(Task 9). ✅
- **타입 일관성**: `memory.Entry`/`Store` 시그니처는 Task 3 정의를 Task 5/6/8이 동일하게 사용. `history.CheckpointResult.Ref`/`NewManager(root)`는 Task 4 정의를 cancel.go/history.go가 동일하게 사용. `openWorkspace()`는 Task 4에서 도입, Task 5/6이 사용, Task 7에서 구 헬퍼 삭제. ✅
- **컴파일 연속성**: Task 3은 신규 파일만 추가(구 Manager 공존), Task 4는 history API 변경과 소비자 수정을 한 태스크로 묶음, Task 5에서 구 Manager 삭제, Task 7에서 store 삭제 — 각 태스크 종료 시점에 `go build ./...`가 항상 성공. ✅

## 의도된 동작 변화 (리뷰어 참고)

1. `mem search`의 rank 의미 변화: BM25 음수 점수 → 0~1 토큰 일치 비율. 한국어 조사 변형이 검색되게 되어 리콜은 오히려 상승 (기존 FTS5 unicode61은 "메모리를"을 "메모리"로 못 찾았음).
2. `history` ref 형식 변화: fossil 해시 → `<pipeline-id>/<phase>`. 사람이 읽고 예측 가능.
3. `history init`/`history sync`/`sync-projects`/`mem prune --dedup` 제거. 원격 백업이 필요하면 `.pylon/history/`가 일반 파일이므로 사용자가 원하는 방식(rsync, git 등)으로 처리.
4. 메모리의 `metadata`/`access_count`/`expires_at` 필드 제거 — 현재 소비자 없음(조사로 확인됨).
5. `.pylon/memory/`는 git 추적 대상 — 메모리 큐레이션이 코드 리뷰 흐름에 들어온다 (D1: init이 `.gitkeep` 생성 + 비-git 워크스페이스에 힌트 출력).
6. `mem store`/Stop hook의 동일 내용 재저장은 `ErrDuplicate`로 스킵된다 (D4) — 구 SQLite는 무조건 새 행을 삽입했다.
7. **레거시 데이터는 이전되지 않는다** (D2/D3): 기존 pylon.db의 project_memory와 fossil 이력은 `/pl:migrate` 절차로 삭제된다. 감지/경고 코드도 추가하지 않는다.
