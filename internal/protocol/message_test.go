package protocol

import (
	"encoding/json"
	"os"
	"testing"
)

func TestNewMessage(t *testing.T) {
	msg := NewMessage(MsgTaskAssign, "orchestrator", "backend-dev")

	if msg.ID == "" {
		t.Error("expected auto-generated ID")
	}
	if msg.Type != MsgTaskAssign {
		t.Errorf("expected type %q, got %q", MsgTaskAssign, msg.Type)
	}
	if msg.From != "orchestrator" {
		t.Errorf("expected from 'orchestrator', got %q", msg.From)
	}
	if msg.To != "backend-dev" {
		t.Errorf("expected to 'backend-dev', got %q", msg.To)
	}
	if msg.Priority != PriorityNormal {
		t.Errorf("expected priority %d, got %d", PriorityNormal, msg.Priority)
	}
	if msg.Timestamp == "" {
		t.Error("expected non-empty timestamp")
	}
}

func TestMarshalUnmarshal(t *testing.T) {
	msg := NewMessage(MsgTaskAssign, "orchestrator", "backend-dev")
	msg.Subject = "Implement login API"
	msg.Body = TaskAssignBody{
		TaskID:      "20260305-user-login",
		Description: "JWT based login",
		AcceptanceCriteria: []string{
			"POST /auth/login works",
			"JWT token issued",
		},
	}
	msg.Context = &MsgContext{
		TaskID:    "20260305-user-login",
		ProjectID: "project-api",
		Summary:   "PO confirmed: email+kakao login",
		Decisions: []string{"JWT based auth"},
	}

	data, err := msg.Marshal()
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	parsed, err := Unmarshal(data)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if parsed.ID != msg.ID {
		t.Errorf("ID mismatch: %q vs %q", parsed.ID, msg.ID)
	}
	if parsed.Subject != "Implement login API" {
		t.Errorf("Subject mismatch: %q", parsed.Subject)
	}
	if parsed.Context == nil {
		t.Fatal("expected non-nil context")
	}
	if parsed.Context.TaskID != "20260305-user-login" {
		t.Errorf("TaskID mismatch: %q", parsed.Context.TaskID)
	}
}

func TestResultBody_JSON(t *testing.T) {
	body := ResultBody{
		TaskID:       "t1",
		Status:       "completed",
		FilesChanged: []string{"auth.go", "auth_test.go"},
		Commits:      []string{"abc123"},
		Summary:      "Login API implemented",
		Learnings:    []string{"sql.NullString needed for nullable fields"},
	}

	data, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var parsed ResultBody
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if parsed.Status != "completed" {
		t.Errorf("expected status 'completed', got %q", parsed.Status)
	}
	if len(parsed.Learnings) != 1 {
		t.Errorf("expected 1 learning, got %d", len(parsed.Learnings))
	}
}

func TestInboxOutbox_WriteRead(t *testing.T) {
	dir := t.TempDir()

	msg := NewMessage(MsgTaskAssign, "orchestrator", "backend-dev")
	msg.Subject = "Test task"
	msg.Body = map[string]any{"task_id": "t1", "description": "test"}
	msg.Context = &MsgContext{TaskID: "t1"}

	// Write to inbox
	if err := WriteTask(dir+"/inbox", "backend-dev", msg); err != nil {
		t.Fatalf("WriteTask failed: %v", err)
	}

	// Write result to outbox
	result := NewMessage(MsgResult, "backend-dev", "orchestrator")
	result.Body = map[string]any{"task_id": "t1", "status": "completed"}
	result.Context = &MsgContext{TaskID: "t1"}

	if err := WriteResult(dir+"/outbox", "backend-dev", result); err != nil {
		t.Fatalf("WriteResult failed: %v", err)
	}

	// Read result
	readMsg, err := ReadResult(dir + "/outbox/backend-dev/t1.result.json")
	if err != nil {
		t.Fatalf("ReadResult failed: %v", err)
	}
	if readMsg.Type != MsgResult {
		t.Errorf("expected type result, got %q", readMsg.Type)
	}
}

func TestScanOutbox(t *testing.T) {
	dir := t.TempDir()

	msg1 := NewMessage(MsgResult, "agent-a", "orchestrator")
	msg1.Body = map[string]any{"task_id": "t1", "status": "completed"}
	msg1.Context = &MsgContext{TaskID: "t1"}
	WriteResult(dir, "agent-a", msg1)

	msg2 := NewMessage(MsgResult, "agent-b", "orchestrator")
	msg2.Body = map[string]any{"task_id": "t2", "status": "completed"}
	msg2.Context = &MsgContext{TaskID: "t2"}
	WriteResult(dir, "agent-b", msg2)

	results, err := ScanOutbox(dir)
	if err != nil {
		t.Fatalf("ScanOutbox failed: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}
}

// TestReadResult_FlatJSON tests that ReadResult handles flat JSON (no envelope wrapper)
// which is what agents write based on the communication rules.
func TestReadResult_FlatJSON(t *testing.T) {
	dir := t.TempDir()
	agentDir := dir + "/backend-dev"
	os.MkdirAll(agentDir, 0755)

	// Write flat JSON (what an agent would actually produce)
	flatJSON := `{
		"task_id": "pipeline-001-backend-dev",
		"status": "completed",
		"summary": "로그인 API 구현 완료",
		"files_changed": ["internal/auth/login.go"],
		"learnings": ["JWT 토큰 만료 시간은 1시간이 적절"]
	}`
	os.WriteFile(agentDir+"/pipeline-001-backend-dev.result.json", []byte(flatJSON), 0644)

	env, err := ReadResult(agentDir + "/pipeline-001-backend-dev.result.json")
	if err != nil {
		t.Fatalf("ReadResult failed for flat JSON: %v", err)
	}
	if env.Type != MsgResult {
		t.Errorf("expected type 'result', got %q", env.Type)
	}

	bodyMap, ok := env.Body.(map[string]any)
	if !ok {
		t.Fatalf("expected body to be map[string]any, got %T", env.Body)
	}
	if bodyMap["status"] != "completed" {
		t.Errorf("expected status 'completed', got %v", bodyMap["status"])
	}
	if bodyMap["summary"] != "로그인 API 구현 완료" {
		t.Errorf("unexpected summary: %v", bodyMap["summary"])
	}

	// Context should be extracted from task_id
	if env.Context == nil || env.Context.TaskID != "pipeline-001-backend-dev" {
		t.Errorf("expected context.TaskID from flat JSON, got %+v", env.Context)
	}

	// Learnings should be accessible
	learnings, ok := bodyMap["learnings"].([]any)
	if !ok || len(learnings) != 1 {
		t.Errorf("expected 1 learning, got %v", bodyMap["learnings"])
	}
}

func TestScanOutbox_Empty(t *testing.T) {
	results, err := ScanOutbox("/nonexistent")
	if err != nil {
		t.Fatalf("ScanOutbox failed: %v", err)
	}
	if results != nil {
		t.Errorf("expected nil for nonexistent, got %v", results)
	}
}
