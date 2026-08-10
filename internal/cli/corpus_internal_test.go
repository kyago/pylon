package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/kyago/pylon/internal/corpus"
)

func TestInternalCorpusCommandsRegistered(t *testing.T) {
	internal := newInternalCmd()
	corpusCommand, _, err := internal.Find([]string{"corpus"})
	if err != nil || corpusCommand == internal {
		t.Fatalf("corpus command not registered: command=%v err=%v", corpusCommand, err)
	}
	for _, name := range []string{"list", "run"} {
		command, _, err := corpusCommand.Find([]string{name})
		if err != nil || command == corpusCommand {
			t.Fatalf("corpus %s command not registered: command=%v err=%v", name, command, err)
		}
	}
}

func TestRecordedCorpusExecutorProducesProviderNeutralReport(t *testing.T) {
	fixtures, err := corpus.LoadEmbedded()
	if err != nil {
		t.Fatal(err)
	}
	outcomesDir := t.TempDir()
	for _, fixture := range fixtures {
		data, err := marshalCorpusOutcome(fixture, "recorded-provider")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(outcomesDir, fixture.ID+".json"), data, 0644); err != nil {
			t.Fatal(err)
		}
	}
	report, err := corpus.Run(t.Context(), corpus.RunOptions{Fixtures: fixtures, Executor: corpus.RecordedExecutor{Dir: outcomesDir}})
	if err != nil {
		t.Fatal(err)
	}
	if !report.Passed || len(report.Cases) != len(fixtures) {
		data, _ := json.Marshal(report)
		t.Fatalf("recorded corpus failed: %s", data)
	}
}
