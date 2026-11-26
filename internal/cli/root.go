// Package cli implements the Cobra-based CLI for pylon.
// Spec Reference: Section 7 "Command Interface"
package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	// Global flags
	flagWorkspace string
	flagVerbose   bool
	flagJSON      bool

	// version is injected at build time via ldflags.
	version = "dev"
)

// SetVersion sets the version string (called from main).
func SetVersion(v string) {
	version = v
	rootCmd.Version = v // --version 플래그도 켠다
}

// rootCmd is the base command for pylon.
var rootCmd = &cobra.Command{
	Use:   "pylon",
	Short: "여러 저장소를 묶은 워크스페이스를 Claude Code·Codex용으로 구성하는 도구",
	Long: `Pylon - 여러 저장소를 묶은 워크스페이스를 Claude Code·Codex용으로 구성하는 도구.

워크스페이스 루트에 AGENTS.md, 프로젝트 목록, 슬래시 커맨드, 간단한 프로젝트 메모리를
준비합니다. 세션은 Claude Code, Codex, Paseo에서 직접 시작합니다.`,
	SilenceUsage:  true,
	SilenceErrors: true,
}

// versionCmd prints build version information.
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "버전 정보 출력",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("pylon version %s\n", version)
	},
}

func init() {
	// Global flags (Spec Section 7: --workspace, --verbose, --json)
	rootCmd.PersistentFlags().StringVar(&flagWorkspace, "workspace", "", "워크스페이스 경로 지정")
	rootCmd.PersistentFlags().BoolVarP(&flagVerbose, "verbose", "v", false, "자세한 출력")
	rootCmd.PersistentFlags().BoolVar(&flagJSON, "json", false, "JSON으로 출력")

	// Register subcommands
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(newDoctorCmd())
	rootCmd.AddCommand(newInitCmd())
	rootCmd.AddCommand(newUninstallCmd())
	rootCmd.AddCommand(newAddProjectCmd())
	rootCmd.AddCommand(newDeleteProjectCmd())
	rootCmd.AddCommand(newProjectListCmd())
	rootCmd.AddCommand(newMemCmd())
	rootCmd.AddCommand(newUpdateCmd())

	localizeCobra()
}

// usageTemplate is cobra's default usage template with Korean labels. Command groups
// and help topics are left out because pylon uses neither.
const usageTemplate = `사용법:{{if .Runnable}}
  {{.UseLine}}{{end}}{{if .HasAvailableSubCommands}}
  {{.CommandPath}} [command]{{end}}{{if gt (len .Aliases) 0}}

별칭:
  {{.NameAndAliases}}{{end}}{{if .HasExample}}

예시:
{{.Example}}{{end}}{{if .HasAvailableSubCommands}}

명령:{{range .Commands}}{{if (or .IsAvailableCommand (eq .Name "help"))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableLocalFlags}}

플래그:
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasAvailableInheritedFlags}}

전역 플래그:
{{.InheritedFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasAvailableSubCommands}}

"{{.CommandPath}} [command] --help"로 각 명령의 도움말을 볼 수 있습니다.{{end}}
`

// localizeCobra replaces the English text cobra generates on its own: the usage labels,
// the help flag, and the summaries of the built-in help and completion commands.
func localizeCobra() {
	rootCmd.SetUsageTemplate(usageTemplate)
	rootCmd.PersistentFlags().BoolP("help", "h", false, "도움말 표시")

	rootCmd.InitDefaultHelpCmd()
	rootCmd.InitDefaultCompletionCmd()
	for _, c := range rootCmd.Commands() {
		switch c.Name() {
		case "help":
			c.Short = "명령 도움말 표시"
			c.Long = "명령의 도움말을 표시합니다. pylon help [command] 형식으로 씁니다."
		case "completion":
			c.Short = "셸 자동완성 스크립트 생성"
		}
	}
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}
