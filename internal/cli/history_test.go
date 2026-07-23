package cli

import "testing"

func TestHistoryCommandExposesApprovedSubcommands(t *testing.T) {
	cmd := newHistoryCmd()
	want := map[string]bool{
		"checkpoint": false, "log": false, "show": false,
		"diff": false, "export": false,
	}
	for _, child := range cmd.Commands() {
		if _, ok := want[child.Name()]; ok {
			want[child.Name()] = true
		}
	}
	for name, found := range want {
		if !found {
			t.Errorf("missing history subcommand %q", name)
		}
	}
}

func TestHistoryCheckpointRequiresPipelineAndPhase(t *testing.T) {
	cmd := newHistoryCheckpointCmd()
	cmd.SetArgs(nil)
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected required flag error")
	}
}
