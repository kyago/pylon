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
		Long:  `Downloads and installs the latest version of pylon via 'go install', then syncs config defaults using the new binary.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Pre-check: go must be available
			if _, err := exec.LookPath("go"); err != nil {
				return fmt.Errorf("'go' 명령을 찾을 수 없습니다. Go 설치가 필요합니다: https://go.dev/dl/")
			}

			fmt.Printf("현재 버전: %s\n", version)
			fmt.Println("최신 버전을 설치합니다...")

			goCmd := exec.Command("go", "install", "github.com/kyago/pylon/cmd/pylon@latest")
			goCmd.Stdout = os.Stdout
			goCmd.Stderr = os.Stderr

			if err := goCmd.Run(); err != nil {
				return fmt.Errorf("업데이트 실패: %w", err)
			}

			fmt.Println("✅ pylon 업데이트 완료")

			// Run config sync using the NEW binary (not the current stale process)
			fmt.Println("\n설정 동기화 중...")
			pylonPath, lookErr := exec.LookPath("pylon")
			if lookErr != nil {
				fmt.Fprintf(os.Stderr, "⚠ 새 pylon 바이너리를 찾을 수 없습니다. 수동으로 'pylon doctor'를 실행하세요.\n")
			} else {
				doctorCmd := exec.Command(pylonPath, "doctor")
				doctorCmd.Stdout = os.Stdout
				doctorCmd.Stderr = os.Stderr
				if err := doctorCmd.Run(); err != nil {
					fmt.Fprintf(os.Stderr, "⚠ 설정 동기화 실패: %v (수동으로 'pylon doctor'를 실행하세요)\n", err)
				}
			}

			return nil
		},
	}
}
