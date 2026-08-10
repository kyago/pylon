package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/kyago/pylon/internal/corpus"
	"github.com/kyago/pylon/internal/fsutil"
	"github.com/spf13/cobra"
)

func newInternalCorpusCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "corpus", Hidden: true}
	cmd.AddCommand(newInternalCorpusListCmd())
	cmd.AddCommand(newInternalCorpusRunCmd())
	return cmd
}

func newInternalCorpusListCmd() *cobra.Command {
	var fixtureDir string
	cmd := &cobra.Command{
		Use:          "list",
		Short:        "List provider-neutral acceptance corpus fixtures",
		Hidden:       true,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			fixtures, err := loadCorpusFixtures(fixtureDir)
			if err != nil {
				return err
			}
			return writeCommandJSON(cmd, fixtures)
		},
	}
	cmd.Flags().StringVar(&fixtureDir, "fixtures", "", "optional fixture directory; embedded corpus is the default")
	return cmd
}

func newInternalCorpusRunCmd() *cobra.Command {
	var fixtureDir, driver, outcomesDir, outputPath string
	var driverArgs []string
	var timeout time.Duration
	cmd := &cobra.Command{
		Use:          "run",
		Short:        "Run or validate the provider-neutral acceptance corpus",
		Hidden:       true,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if (driver == "") == (outcomesDir == "") {
				return fmt.Errorf("exactly one of --driver or --outcomes is required")
			}
			fixtures, err := loadCorpusFixtures(fixtureDir)
			if err != nil {
				return err
			}
			var executor corpus.Executor
			if driver != "" {
				executor = corpus.CommandExecutor{Executable: driver, Args: driverArgs}
			} else {
				executor = corpus.RecordedExecutor{Dir: outcomesDir}
			}
			report, err := corpus.Run(context.Background(), corpus.RunOptions{
				Fixtures: fixtures, Executor: executor, Timeout: timeout, Now: time.Now,
			})
			if err != nil {
				return err
			}
			if err := fsutil.WriteJSONAtomic(outputPath, report); err != nil {
				return err
			}
			if err := writeCommandJSON(cmd, report); err != nil {
				return err
			}
			if !report.Passed {
				return fmt.Errorf("acceptance corpus failed; report: %s", outputPath)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&fixtureDir, "fixtures", "", "optional fixture directory; embedded corpus is the default")
	cmd.Flags().StringVar(&driver, "driver", "", "provider/session driver executable; fixture JSON is passed on stdin")
	cmd.Flags().StringSliceVar(&driverArgs, "driver-arg", nil, "argument passed to the driver executable; repeatable")
	cmd.Flags().StringVar(&outcomesDir, "outcomes", "", "directory containing <fixture-id>.json recorded outcomes")
	cmd.Flags().StringVar(&outputPath, "output", "", "corpus report output path")
	cmd.Flags().DurationVar(&timeout, "timeout", 5*time.Minute, "timeout per fixture")
	_ = cmd.MarkFlagRequired("output")
	return cmd
}

func loadCorpusFixtures(fixtureDir string) ([]corpus.Fixture, error) {
	if fixtureDir == "" {
		return corpus.LoadEmbedded()
	}
	return corpus.LoadDir(fixtureDir)
}

func marshalCorpusOutcome(fixture corpus.Fixture, provider string) ([]byte, error) {
	return json.Marshal(corpus.Outcome{
		SchemaVersion:    corpus.SchemaVersion,
		FixtureID:        fixture.ID,
		Provider:         provider,
		Verdict:          fixture.Expected.Verdict,
		RunStatus:        fixture.Expected.RunStatus,
		TaskStatuses:     fixture.Expected.TaskStatuses,
		Artifacts:        fixture.Expected.RequiredArtifacts,
		Events:           fixture.Expected.RequiredEvents,
		CleanupStatus:    fixture.Expected.CleanupStatus,
		RuntimePreserved: fixture.Expected.RuntimePreserved,
	})
}
