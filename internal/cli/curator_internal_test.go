package cli

import "testing"

func TestInternalCuratorCommandsRegistered(t *testing.T) {
	internal := newInternalCmd()
	curatorCommand, _, err := internal.Find([]string{"curator"})
	if err != nil || curatorCommand == internal {
		t.Fatalf("curator command not registered: command=%v err=%v", curatorCommand, err)
	}
	for _, name := range []string{"propose", "review", "gate"} {
		command, _, err := curatorCommand.Find([]string{name})
		if err != nil || command == curatorCommand {
			t.Fatalf("curator %s command not registered: command=%v err=%v", name, command, err)
		}
	}
}
