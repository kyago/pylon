package cli

import (
	"os"
	"strings"
	"testing"

	"github.com/kyago/pylon/internal/layout"
)

// 구버전 Stop 훅(`pylon sync-memory --from-session --agent claude`)이 남은 settings.json에서도 훅이
// 실패하지 않아야 한다: 숨긴 no-op이 stdin을 소비하고 아무것도 저장하지 않고 exit 0으로 끝난다.
func TestSyncMemoryIsHiddenNoOp(t *testing.T) {
	root := setupTestWorkspace(t)
	oldWorkspace := flagWorkspace
	flagWorkspace = root
	t.Cleanup(func() { flagWorkspace = oldWorkspace })

	cmd, _, err := rootCmd.Find([]string{"sync-memory"})
	if err != nil || cmd.Name() != "sync-memory" || !cmd.Hidden {
		t.Fatalf("hidden sync-memory not registered: cmd=%v err=%v", cmd, err)
	}

	stdin := strings.NewReader(`{"session_id":"s1","transcript_path":"/tmp/t.jsonl","hook_event_name":"Stop"}`)
	rootCmd.SetIn(stdin)
	rootCmd.SetArgs([]string{"sync-memory", "--from-session", "--agent", "claude"})
	t.Cleanup(func() {
		rootCmd.SetIn(nil)
		rootCmd.SetArgs(nil)
	})

	out := captureStdout(t, func() { err = rootCmd.Execute() })
	if err != nil {
		t.Fatalf("legacy hook invocation must exit 0: %v", err)
	}
	if out != "" {
		t.Errorf("no-op must print nothing, got %q", out)
	}
	if stdin.Len() != 0 {
		t.Errorf("hook stdin must be drained, %d bytes left", stdin.Len())
	}
	if _, err := os.Stat(layout.MemoryDir(root)); !os.IsNotExist(err) {
		t.Errorf("no-op must not create memory files: %v", err)
	}
}
