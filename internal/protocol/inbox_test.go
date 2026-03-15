package protocol

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// ---------------------------------------------------------------------------
// WriteTask 테스트
// ---------------------------------------------------------------------------

func TestWriteTask_Normal(t *testing.T) {
	dir := t.TempDir()
	msg := NewMessage(MsgTaskAssign, "orchestrator", "backend-dev")
	msg.Subject = "정상 동작 테스트"
	msg.Body = map[string]any{"task_id": "task-001", "description": "test"}
	msg.Context = &MsgContext{TaskID: "task-001"}

	if err := WriteTask(dir, "backend-dev", msg); err != nil {
		t.Fatalf("WriteTask failed: %v", err)
	}

	path := filepath.Join(dir, "backend-dev", "task-001.task.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("파일 읽기 실패: %v", err)
	}

	// JSON 역직렬화 가능 확인
	parsed, err := Unmarshal(data)
	if err != nil {
		t.Fatalf("JSON 역직렬화 실패: %v", err)
	}
	if parsed.ID != msg.ID {
		t.Errorf("ID 불일치: got %q, want %q", parsed.ID, msg.ID)
	}
	if parsed.Subject != "정상 동작 테스트" {
		t.Errorf("Subject 불일치: got %q", parsed.Subject)
	}
}

func TestWriteTask_TaskIDFromContext(t *testing.T) {
	dir := t.TempDir()
	msg := NewMessage(MsgTaskAssign, "orchestrator", "agent-a")
	msg.Context = &MsgContext{TaskID: "ctx-task-42"}
	msg.Body = "simple string body"

	if err := WriteTask(dir, "agent-a", msg); err != nil {
		t.Fatalf("WriteTask failed: %v", err)
	}

	path := filepath.Join(dir, "agent-a", "ctx-task-42.task.json")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatalf("Context.TaskID가 파일명에 반영되지 않음: %s", path)
	}
}

func TestWriteTask_TaskIDFromBody(t *testing.T) {
	dir := t.TempDir()
	msg := NewMessage(MsgTaskAssign, "orchestrator", "agent-b")
	msg.Body = map[string]any{"task_id": "body-task-99", "description": "from body"}
	// Context nil — Body에서 추출해야 함

	if err := WriteTask(dir, "agent-b", msg); err != nil {
		t.Fatalf("WriteTask failed: %v", err)
	}

	path := filepath.Join(dir, "agent-b", "body-task-99.task.json")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatalf("Body의 task_id가 파일명에 반영되지 않음: %s", path)
	}
}

func TestWriteTask_FallbackToMsgID(t *testing.T) {
	dir := t.TempDir()
	msg := NewMessage(MsgTaskAssign, "orchestrator", "agent-c")
	msg.Body = "no task_id here"
	// Context nil, Body에 task_id 없음 → msg.ID 폴백

	if err := WriteTask(dir, "agent-c", msg); err != nil {
		t.Fatalf("WriteTask failed: %v", err)
	}

	path := filepath.Join(dir, "agent-c", msg.ID+".task.json")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatalf("msg.ID 폴백이 파일명에 반영되지 않음: %s", path)
	}
}

func TestWriteTask_CreatesAgentSubdir(t *testing.T) {
	dir := t.TempDir()
	agentDir := filepath.Join(dir, "new-agent")

	// 에이전트 디렉토리가 아직 없는 상태
	if _, err := os.Stat(agentDir); !os.IsNotExist(err) {
		t.Fatal("테스트 전제 조건 실패: 디렉토리가 이미 존재함")
	}

	msg := NewMessage(MsgTaskAssign, "orchestrator", "new-agent")
	msg.Context = &MsgContext{TaskID: "auto-dir-task"}

	if err := WriteTask(dir, "new-agent", msg); err != nil {
		t.Fatalf("WriteTask failed: %v", err)
	}

	info, err := os.Stat(agentDir)
	if err != nil {
		t.Fatalf("에이전트 디렉토리 자동 생성 실패: %v", err)
	}
	if !info.IsDir() {
		t.Error("생성된 경로가 디렉토리가 아님")
	}
}

func TestWriteTask_ErrorInvalidParentDir(t *testing.T) {
	// 존재하지 않는 상위 디렉토리에 쓰기 시도 (read-only 파일로 차단)
	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocker")

	// 일반 파일을 생성하여 같은 이름의 디렉토리를 만들 수 없게 함
	if err := os.WriteFile(blocker, []byte("x"), 0644); err != nil {
		t.Fatalf("blocker 파일 생성 실패: %v", err)
	}

	msg := NewMessage(MsgTaskAssign, "orchestrator", "agent")
	msg.Context = &MsgContext{TaskID: "fail-task"}

	// blocker는 파일이므로 blocker/agent 디렉토리를 생성할 수 없음
	err := WriteTask(blocker, "agent", msg)
	if err == nil {
		t.Fatal("상위 경로가 파일인 경우 에러가 발생해야 함")
	}
}

// ---------------------------------------------------------------------------
// extractTaskID 테스트
// ---------------------------------------------------------------------------

func TestExtractTaskID_ContextPriority(t *testing.T) {
	msg := &MessageEnvelope{
		Context: &MsgContext{TaskID: "from-context"},
		Body:    map[string]any{"task_id": "from-body"},
	}
	got := extractTaskID(msg)
	if got != "from-context" {
		t.Errorf("Context가 우선되어야 함: got %q, want %q", got, "from-context")
	}
}

func TestExtractTaskID_FallbackToBody(t *testing.T) {
	msg := &MessageEnvelope{
		Context: nil,
		Body:    map[string]any{"task_id": "from-body"},
	}
	got := extractTaskID(msg)
	if got != "from-body" {
		t.Errorf("Context nil일 때 Body 폴백: got %q, want %q", got, "from-body")
	}
}

func TestExtractTaskID_ContextEmptyFallbackToBody(t *testing.T) {
	msg := &MessageEnvelope{
		Context: &MsgContext{TaskID: ""},
		Body:    map[string]any{"task_id": "from-body"},
	}
	got := extractTaskID(msg)
	if got != "from-body" {
		t.Errorf("Context.TaskID 빈 문자열일 때 Body 폴백: got %q, want %q", got, "from-body")
	}
}

func TestExtractTaskID_BodyNotMap(t *testing.T) {
	msg := &MessageEnvelope{
		Context: nil,
		Body:    "just a string",
	}
	got := extractTaskID(msg)
	if got != "" {
		t.Errorf("Body가 map이 아닐 때 빈 문자열 반환: got %q", got)
	}
}

func TestExtractTaskID_NeitherContextNorBody(t *testing.T) {
	msg := &MessageEnvelope{
		Context: nil,
		Body:    map[string]any{"description": "no task_id key"},
	}
	got := extractTaskID(msg)
	if got != "" {
		t.Errorf("task_id 없을 때 빈 문자열 반환: got %q", got)
	}
}

// ---------------------------------------------------------------------------
// writeAtomically 테스트
// ---------------------------------------------------------------------------

func TestWriteAtomically_Normal(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.json")
	content := []byte(`{"key":"value"}`)

	if err := writeAtomically(path, content); err != nil {
		t.Fatalf("writeAtomically failed: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("파일 읽기 실패: %v", err)
	}

	// 내용 검증
	var parsed map[string]string
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("JSON 파싱 실패: %v", err)
	}
	if parsed["key"] != "value" {
		t.Errorf("내용 불일치: got %q, want %q", parsed["key"], "value")
	}
}

func TestWriteAtomically_Overwrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "overwrite.json")

	// 기존 파일 작성
	original := []byte(`{"version":1}`)
	if err := writeAtomically(path, original); err != nil {
		t.Fatalf("첫 번째 쓰기 실패: %v", err)
	}

	// 덮어쓰기
	updated := []byte(`{"version":2}`)
	if err := writeAtomically(path, updated); err != nil {
		t.Fatalf("덮어쓰기 실패: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("파일 읽기 실패: %v", err)
	}

	var parsed map[string]int
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("JSON 파싱 실패: %v", err)
	}
	if parsed["version"] != 2 {
		t.Errorf("덮어쓰기 반영 안됨: got %d, want 2", parsed["version"])
	}
}

func TestWriteAtomically_NoTmpFileRemains(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "clean.json")

	if err := writeAtomically(path, []byte(`{"ok":true}`)); err != nil {
		t.Fatalf("writeAtomically failed: %v", err)
	}

	tmpPath := path + ".tmp"
	if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
		t.Errorf("성공 후 .tmp 파일이 남아있음: %s", tmpPath)
	}
}
