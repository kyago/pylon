package cli

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
)

const installPkg = "github.com/kyago/pylon/cmd/pylon"

// semverRe matches semver: optional 'v' prefix + MAJOR.MINOR.PATCH with optional pre-release/build.
var semverRe = regexp.MustCompile(`^v?\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?$`)

func newUpdateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "update [version]",
		Short: "Update pylon to the latest or a specific version",
		Long: `Downloads and installs pylon via 'go install', then syncs config defaults.

Without arguments, installs the latest version.
With a version argument (e.g. v0.3.0), installs that specific version.

Examples:
  pylon update          # install latest
  pylon update v0.3.0   # install specific version
  pylon update 0.3.0    # 'v' prefix is added automatically`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if _, err := exec.LookPath("go"); err != nil {
				return fmt.Errorf("'go' 명령을 찾을 수 없습니다. Go 설치가 필요합니다: https://go.dev/dl/")
			}

			target, err := resolveInstallTarget(args)
			if err != nil {
				return err
			}

			fmt.Printf("현재 버전: %s\n", version)
			if target == "latest" {
				fmt.Println("최신 버전을 설치합니다...")
			} else {
				fmt.Printf("버전 %s을(를) 설치합니다...\n", target)
			}

			goCmd := exec.Command("go", "install", fmt.Sprintf("%s@%s", installPkg, target))
			goCmd.Stdout = os.Stdout
			goCmd.Stderr = os.Stderr

			if err := goCmd.Run(); err != nil {
				return fmt.Errorf("업데이트 실패: %w", err)
			}

			if target == "latest" {
				fmt.Println("✅ pylon 최신 버전 설치 완료")
			} else {
				fmt.Printf("✅ pylon %s 설치 완료\n", target)
			}

			// Run config sync using the NEW binary
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

// resolveInstallTarget determines the go install target version from CLI args.
func resolveInstallTarget(args []string) (string, error) {
	if len(args) == 0 {
		return "latest", nil
	}

	ver := strings.TrimSpace(args[0])
	if ver == "" {
		return "latest", nil
	}

	if strings.EqualFold(ver, "latest") {
		return "latest", nil
	}

	if !validateVersion(ver) {
		return "", fmt.Errorf("잘못된 버전 형식입니다: %q (예: v0.3.0, 1.2.3)", ver)
	}

	return normalizeVersion(ver), nil
}

// validateVersion checks whether s is a valid semver string or the keyword "latest".
func validateVersion(s string) bool {
	if s == "" {
		return false
	}
	if strings.EqualFold(s, "latest") {
		return true
	}
	return semverRe.MatchString(s)
}

// normalizeVersion ensures the version has a 'v' prefix.
// The keyword "latest" is returned as-is.
func normalizeVersion(s string) string {
	if strings.EqualFold(s, "latest") {
		return "latest"
	}
	if !strings.HasPrefix(s, "v") {
		return "v" + s
	}
	return s
}
