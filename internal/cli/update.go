package cli

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

func newUpdateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "update",
		Short: "Update pylon to the latest version",
		Long:  `Downloads and installs the latest version of pylon via 'go install', then syncs config defaults.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Printf("현재 버전: %s\n", version)
			fmt.Println("최신 버전을 설치합니다...")

			goCmd := exec.Command("go", "install", "github.com/kyago/pylon/cmd/pylon@latest")
			goCmd.Stdout = os.Stdout
			goCmd.Stderr = os.Stderr

			if err := goCmd.Run(); err != nil {
				return fmt.Errorf("업데이트 실패: %w", err)
			}

			fmt.Println("✅ pylon 업데이트 완료")

			// Sync config defaults for new fields added in the update
			syncConfigIfWorkspace()

			return nil
		},
	}
}
