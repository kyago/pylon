package protocol

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// ---------------------------------------------------------------------------
// ReadResult tests
// ---------------------------------------------------------------------------

func TestReadResult_FullEnvelope(t *testing.T) {
	dir := t.TempDir()

	msg := NewMessage(MsgResult, "agent-x", "orchestrator")
	msg.Subject = "task done"
	msg.Body = map[string]any{"task_id": "t-full", "status": "completed"}
	msg.Context = &MsgContext{TaskID: "t-full", ProjectID: "proj-1"}

	data, err := msg.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	path := filepath.Join(dir, "result.json")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got, err := ReadResult(path)
	if err != nil {
		t.Fatalf("ReadResult: %v", err)
	}
	if got.Type != MsgResult {
		t.Errorf("Type = %q, want %q", got.Type, MsgResult)
	}
	if got.ID != msg.ID {
		t.Errorf("ID = %q, want %q", got.ID, msg.ID)
	}
	if got.Context == nil || got.Context.ProjectID != "proj-1" {
		t.Errorf("Context.ProjectID not preserved: %+v", got.Context)
	}
}

func TestReadResult_FlatJSON_WithoutTaskID(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "no-task-id.result.json")

	flat := `{"status": "completed", "summary": "done"}`
	if err := os.WriteFile(path, []byte(flat), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got, err := ReadResult(path)
	if err != nil {
		t.Fatalf("ReadResult: %v", err)
	}
	if got.Type != MsgResult {
		t.Errorf("Type = %q, want %q", got.Type, MsgResult)
	}
	// task_id가 없으면 Context는 nil
	if got.Context != nil {
		t.Errorf("Context should be nil when no task_id, got %+v", got.Context)
	}
	body, ok := got.Body.(map[string]any)
	if !ok {
		t.Fatalf("Body type = %T, want map[string]any", got.Body)
	}
	if body["status"] != "completed" {
		t.Errorf("status = %v, want completed", body["status"])
	}
}

func TestReadResult_EmptyTypeEnvelope_FallsToFlat(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty-type.result.json")

	// envelope 구조이지만 type이 빈 문자열 → flat JSON으로 처리
	envelope := map[string]any{
		"id":        "some-id",
		"type":      "",
		"from":      "agent-a",
		"to":        "orchestrator",
		"body":      map[string]any{"task_id": "t-empty-type", "status": "completed"},
		"timestamp": "2025-01-01T00:00:00Z",
	}
	data, _ := json.Marshal(envelope)
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got, err := ReadResult(path)
	if err != nil {
		t.Fatalf("ReadResult: %v", err)
	}
	// Type이 빈 envelope → flat으로 재해석 → MsgResult
	if got.Type != MsgResult {
		t.Errorf("Type = %q, want %q", got.Type, MsgResult)
	}
}

func TestReadResult_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.result.json")
	if err := os.WriteFile(path, []byte(`{invalid json`), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err := ReadResult(path)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestReadResult_NonexistentFile(t *testing.T) {
	_, err := ReadResult("/nonexistent/path/missing.result.json")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestReadResult_BinaryData(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "binary.result.json")
	// 바이너리 데이터 (유효하지 않은 JSON)
	data := []byte{0x00, 0x01, 0x02, 0xFF, 0xFE, 0xFD}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err := ReadResult(path)
	if err == nil {
		t.Fatal("expected error for binary data")
	}
}

func TestReadResult_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.result.json")
	if err := os.WriteFile(path, []byte{}, 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err := ReadResult(path)
	if err == nil {
		t.Fatal("expected error for empty file")
	}
}

func TestReadResult_FlatJSON_NumericTaskID(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "numeric.result.json")

	// task_id가 숫자인 경우 → string이 아니므로 Context에 반영 안 됨
	flat := `{"task_id": 12345, "status": "completed"}`
	if err := os.WriteFile(path, []byte(flat), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got, err := ReadResult(path)
	if err != nil {
		t.Fatalf("ReadResult: %v", err)
	}
	if got.Type != MsgResult {
		t.Errorf("Type = %q, want %q", got.Type, MsgResult)
	}
	// task_id가 string이 아니면 Context는 nil
	if got.Context != nil {
		t.Errorf("Context should be nil for numeric task_id, got %+v", got.Context)
	}
}

// ---------------------------------------------------------------------------
// WriteResult tests
// ---------------------------------------------------------------------------

func TestWriteResult_RoundTrip(t *testing.T) {
	dir := t.TempDir()

	msg := NewMessage(MsgResult, "agent-z", "orchestrator")
	msg.Body = map[string]any{"task_id": "round-trip", "status": "completed", "summary": "ok"}
	msg.Context = &MsgContext{TaskID: "round-trip"}

	if err := WriteResult(dir, "agent-z", msg); err != nil {
		t.Fatalf("WriteResult: %v", err)
	}

	resultPath := filepath.Join(dir, "agent-z", "round-trip.result.json")
	if _, err := os.Stat(resultPath); os.IsNotExist(err) {
		t.Fatalf("result file not created at %s", resultPath)
	}

	got, err := ReadResult(resultPath)
	if err != nil {
		t.Fatalf("ReadResult: %v", err)
	}
	if got.ID != msg.ID {
		t.Errorf("ID = %q, want %q", got.ID, msg.ID)
	}
	if got.From != "agent-z" {
		t.Errorf("From = %q, want agent-z", got.From)
	}
}

func TestWriteResult_TaskIDFromBody(t *testing.T) {
	dir := t.TempDir()

	msg := NewMessage(MsgResult, "agent-b", "orchestrator")
	msg.Body = map[string]any{"task_id": "body-task-id", "status": "completed"}
	// Context가 없으면 body에서 task_id 추출

	if err := WriteResult(dir, "agent-b", msg); err != nil {
		t.Fatalf("WriteResult: %v", err)
	}

	resultPath := filepath.Join(dir, "agent-b", "body-task-id.result.json")
	if _, err := os.Stat(resultPath); os.IsNotExist(err) {
		t.Fatalf("result file not created with body task_id: %s", resultPath)
	}
}

func TestWriteResult_FallbackToMsgID(t *testing.T) {
	dir := t.TempDir()

	msg := NewMessage(MsgResult, "agent-c", "orchestrator")
	msg.Body = map[string]any{"status": "completed"}
	// Context도 없고 body에도 task_id 없음 → msg.ID 사용

	if err := WriteResult(dir, "agent-c", msg); err != nil {
		t.Fatalf("WriteResult: %v", err)
	}

	resultPath := filepath.Join(dir, "agent-c", msg.ID+".result.json")
	if _, err := os.Stat(resultPath); os.IsNotExist(err) {
		t.Fatalf("result file not created with msg.ID fallback: %s", resultPath)
	}
}

func TestWriteResult_CreatesAgentDir(t *testing.T) {
	dir := t.TempDir()
	agentDir := filepath.Join(dir, "new-agent")

	// 아직 디렉토리가 없는 상태 확인
	if _, err := os.Stat(agentDir); !os.IsNotExist(err) {
		t.Fatalf("agent dir should not exist yet")
	}

	msg := NewMessage(MsgResult, "new-agent", "orchestrator")
	msg.Body = map[string]any{"task_id": "auto-dir", "status": "completed"}
	msg.Context = &MsgContext{TaskID: "auto-dir"}

	if err := WriteResult(dir, "new-agent", msg); err != nil {
		t.Fatalf("WriteResult: %v", err)
	}

	info, err := os.Stat(agentDir)
	if err != nil {
		t.Fatalf("agent dir not created: %v", err)
	}
	if !info.IsDir() {
		t.Errorf("expected directory, got file")
	}
}

func TestWriteResult_NestedAgentDir(t *testing.T) {
	dir := t.TempDir()
	// 여러 단계의 중첩 경로
	nestedOutbox := filepath.Join(dir, "deep", "nested", "outbox")

	msg := NewMessage(MsgResult, "agent-deep", "orchestrator")
	msg.Body = map[string]any{"task_id": "nested-task", "status": "completed"}
	msg.Context = &MsgContext{TaskID: "nested-task"}

	if err := WriteResult(nestedOutbox, "agent-deep", msg); err != nil {
		t.Fatalf("WriteResult: %v", err)
	}

	resultPath := filepath.Join(nestedOutbox, "agent-deep", "nested-task.result.json")
	if _, err := os.Stat(resultPath); os.IsNotExist(err) {
		t.Fatalf("result file not created in nested dir: %s", resultPath)
	}
}

// ---------------------------------------------------------------------------
// ScanOutbox tests
// ---------------------------------------------------------------------------

func TestScanOutbox_FiltersNonResultJSON(t *testing.T) {
	dir := t.TempDir()
	agentDir := filepath.Join(dir, "agent-filter")
	if err := os.MkdirAll(agentDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	// .result.json 파일 1개
	os.WriteFile(filepath.Join(agentDir, "t1.result.json"), []byte(`{"status":"ok"}`), 0644)
	// .result.json이 아닌 파일들
	os.WriteFile(filepath.Join(agentDir, "t2.task.json"), []byte(`{"status":"ok"}`), 0644)
	os.WriteFile(filepath.Join(agentDir, "notes.txt"), []byte("notes"), 0644)
	os.WriteFile(filepath.Join(agentDir, "result.json"), []byte(`{"status":"ok"}`), 0644) // .result.json suffix 아님
	os.WriteFile(filepath.Join(agentDir, "t3.result.json.bak"), []byte(`{}`), 0644)

	results, err := ScanOutbox(dir)
	if err != nil {
		t.Fatalf("ScanOutbox: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d: %v", len(results), results)
	}
}

func TestScanOutbox_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	// outbox 디렉토리는 존재하지만 비어 있음

	results, err := ScanOutbox(dir)
	if err != nil {
		t.Fatalf("ScanOutbox: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected empty slice, got %v", results)
	}
}

func TestScanOutbox_NonexistentDir(t *testing.T) {
	results, err := ScanOutbox(filepath.Join(t.TempDir(), "does-not-exist"))
	if err != nil {
		t.Fatalf("expected no error for nonexistent dir, got: %v", err)
	}
	if results != nil {
		t.Errorf("expected nil, got %v", results)
	}
}

func TestScanOutbox_SkipsFilesInRoot(t *testing.T) {
	dir := t.TempDir()

	// outbox 루트에 파일(디렉토리가 아닌) 배치 → 스킵되어야 함
	os.WriteFile(filepath.Join(dir, "stray-file.result.json"), []byte(`{}`), 0644)
	os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("hi"), 0644)

	// 정상 에이전트 디렉토리도 하나 추가
	agentDir := filepath.Join(dir, "agent-ok")
	os.MkdirAll(agentDir, 0755)
	os.WriteFile(filepath.Join(agentDir, "t1.result.json"), []byte(`{}`), 0644)

	results, err := ScanOutbox(dir)
	if err != nil {
		t.Fatalf("ScanOutbox: %v", err)
	}
	// 루트의 파일은 무시되고 에이전트 디렉토리 안의 파일만 수집
	if len(results) != 1 {
		t.Errorf("expected 1 result (only from agent dir), got %d: %v", len(results), results)
	}
}

func TestScanOutbox_MultipleAgentsMultipleResults(t *testing.T) {
	dir := t.TempDir()

	agents := []struct {
		name  string
		tasks []string
	}{
		{"agent-a", []string{"t1", "t2", "t3"}},
		{"agent-b", []string{"t4"}},
		{"agent-c", []string{"t5", "t6"}},
	}

	for _, a := range agents {
		agentDir := filepath.Join(dir, a.name)
		os.MkdirAll(agentDir, 0755)
		for _, task := range a.tasks {
			os.WriteFile(filepath.Join(agentDir, task+".result.json"), []byte(`{}`), 0644)
		}
	}

	results, err := ScanOutbox(dir)
	if err != nil {
		t.Fatalf("ScanOutbox: %v", err)
	}
	if len(results) != 6 {
		t.Errorf("expected 6 results across 3 agents, got %d", len(results))
	}
}

func TestScanOutbox_AgentDirWithNoResults(t *testing.T) {
	dir := t.TempDir()
	agentDir := filepath.Join(dir, "empty-agent")
	os.MkdirAll(agentDir, 0755)
	// 에이전트 디렉토리 존재하지만 .result.json 파일 없음
	os.WriteFile(filepath.Join(agentDir, "notes.txt"), []byte("notes"), 0644)

	results, err := ScanOutbox(dir)
	if err != nil {
		t.Fatalf("ScanOutbox: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}
