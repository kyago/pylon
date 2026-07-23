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
		{"DBPath", DBPath(root), filepath.Join(root, ".pylon", "pylon.db")},
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
