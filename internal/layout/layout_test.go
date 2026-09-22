package layout

import (
	"path/filepath"
	"testing"
)

func TestLayoutPaths(t *testing.T) {
	root := filepath.Join("some", "root")
	cases := []struct {
		name string
		got  string
		want string
	}{
		{"PylonDir", PylonDir(root), filepath.Join(root, ".pylon")},
		{"ConfigPath", ConfigPath(root), filepath.Join(root, ".pylon", "config.yml")},
		{"RuntimeDir", RuntimeDir(root), filepath.Join(root, ".pylon", "runtime")},
		{"CommandsDir", CommandsDir(root), filepath.Join(root, ".pylon", "commands")},
		{"ScriptsDir", ScriptsDir(root), filepath.Join(root, ".pylon", "scripts", "bash")},
		{"ClaudeDir", ClaudeDir(root), filepath.Join(root, ".claude")},
		{"ClaudeAgentsDir", ClaudeAgentsDir(root), filepath.Join(root, ".claude", "agents")},
		{"ClaudeCommandsDir", ClaudeCommandsDir(root), filepath.Join(root, ".claude", "commands")},
		{"AgentLinkTarget", AgentLinkTarget("a.md"), filepath.Join("..", "..", ".pylon", "agents", "a.md")},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s = %q, want %q", c.name, c.got, c.want)
		}
	}
}

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
