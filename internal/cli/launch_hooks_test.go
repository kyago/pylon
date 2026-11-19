package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func setupTestWorkspace(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()
	pylonDir := filepath.Join(tmpDir, ".pylon")
	if err := os.MkdirAll(pylonDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pylonDir, "config.yml"), []byte("version: \"0.1\"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	return tmpDir
}

func writeSettings(t *testing.T, dir, body string) string {
	t.Helper()
	path := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// pylon 훅만 지우고, 같은 그룹의 사용자 훅과 다른 설정은 남긴다.
func TestRemovePylonHooks_KeepsUserHooksAndSettings(t *testing.T) {
	dir := t.TempDir()
	path := writeSettings(t, dir, `{"theme":"dark","hooks":{
  "SessionStart":[
    {"matcher":"startup","hooks":[{"type":"command","command":"pylon mem inject"}]},
    {"matcher":"resume","hooks":[{"type":"command","command":"pylon mem inject"},{"type":"command","command":"my-start"}]}
  ],
  "PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"my-guard"}]}]
}}`)
	removed, err := removePylonHooks(dir)
	if err != nil || !removed {
		t.Fatalf("removed=%v err=%v", removed, err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Theme string `json:"theme"`
		Hooks map[string][]struct {
			Matcher string `json:"matcher"`
			Hooks   []struct {
				Command string `json:"command"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.Theme != "dark" {
		t.Errorf("other settings lost: %s", data)
	}
	start := got.Hooks["SessionStart"]
	if len(start) != 1 || start[0].Matcher != "resume" || len(start[0].Hooks) != 1 || start[0].Hooks[0].Command != "my-start" {
		t.Errorf("SessionStart = %+v", start)
	}
	if pre := got.Hooks["PreToolUse"]; len(pre) != 1 || pre[0].Hooks[0].Command != "my-guard" {
		t.Errorf("PreToolUse = %+v", pre)
	}
}

// 훅이 pylon 것뿐이면 hooks 키까지 지운다.
func TestRemovePylonHooks_RemovesEmptyHooksKey(t *testing.T) {
	dir := t.TempDir()
	path := writeSettings(t, dir, `{"hooks":{"SessionStart":[{"matcher":"","hooks":[{"type":"command","command":"pylon mem inject"}]}]}}`)
	if removed, err := removePylonHooks(dir); err != nil || !removed {
		t.Fatalf("removed=%v err=%v", removed, err)
	}
	data, _ := os.ReadFile(path)
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if _, ok := got["hooks"]; ok {
		t.Errorf("empty hooks key left: %s", data)
	}
}

// pylon 훅이 없으면 파일을 다시 쓰지 않는다.
func TestRemovePylonHooks_NoChangeWithoutPylonHook(t *testing.T) {
	dir := t.TempDir()
	body := `{ "hooks":   {"Stop":[{"matcher":"","hooks":[{"type":"command","command":"my-stop"}]}]} }`
	path := writeSettings(t, dir, body)
	if removed, err := removePylonHooks(dir); err != nil || removed {
		t.Fatalf("removed=%v err=%v", removed, err)
	}
	if data, _ := os.ReadFile(path); string(data) != body {
		t.Errorf("file rewritten: %s", data)
	}
}

// 파일이 없으면 만들지 않고, 깨진 파일은 고치지 않고 알린다.
func TestRemovePylonHooks_MissingAndInvalidFiles(t *testing.T) {
	dir := t.TempDir()
	if removed, err := removePylonHooks(dir); err != nil || removed {
		t.Fatalf("missing file: removed=%v err=%v", removed, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "settings.json")); !os.IsNotExist(err) {
		t.Error("settings.json must not be created")
	}
	path := writeSettings(t, dir, "{not json")
	if removed, err := removePylonHooks(dir); err == nil || removed {
		t.Fatalf("invalid file: removed=%v err=%v", removed, err)
	}
	if data, _ := os.ReadFile(path); string(data) != "{not json" {
		t.Errorf("invalid file rewritten: %s", data)
	}
}
