package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/kyago/pylon/internal/config"
	"github.com/kyago/pylon/internal/layout"
)

func newAddProjectCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add-project <git-url>",
		Short: "프로젝트를 독립 git clone으로 워크스페이스에 추가",
		Long: `프로젝트를 독립 git clone으로 워크스페이스에 추가합니다.

기술 스택을 감지해 프로젝트 수준 .pylon/context.md를 만들고, 루트 AGENTS.md의
프로젝트 목록을 갱신합니다.

프로젝트 디렉터리가 이미 있으면 --force로 다시 clone하거나, --skip-clone으로
기존 디렉터리를 그대로 두고 .pylon/ 구성만 만듭니다.`,
		Args: cobra.ExactArgs(1),
		RunE: runAddProject,
	}

	cmd.Flags().String("name", "", "프로젝트 디렉터리 이름 (기본: URL에서 추론)")
	cmd.Flags().Bool("force", false, "기존 디렉터리를 지우고 다시 clone")
	cmd.Flags().Bool("skip-clone", false, "clone을 생략하고 기존 디렉터리에 .pylon/ 구성만 생성")

	return cmd
}

func runAddProject(cmd *cobra.Command, args []string) error {
	repoURL := args[0]

	// Find workspace root
	root, err := resolveRoot()
	if err != nil {
		return err
	}

	// Determine project name
	projectName, _ := cmd.Flags().GetString("name")
	if projectName == "" {
		projectName = inferProjectName(repoURL)
	}

	force, _ := cmd.Flags().GetBool("force")
	skipClone, _ := cmd.Flags().GetBool("skip-clone")

	if force && skipClone {
		return fmt.Errorf("--force와 --skip-clone은 함께 쓸 수 없습니다")
	}

	// Validate project name to prevent path traversal
	if err := validateProjectName(projectName); err != nil {
		return err
	}

	// Before prepareProjectDirectory: --force would delete the worktree.
	if config.IsWorkspaceWorktree(root, filepath.Join(root, projectName)) {
		return fmt.Errorf("%s은(는) 이 워크스페이스에 있는 체크아웃의 git 워크트리이며 프로젝트가 아닙니다", projectName)
	}

	projectDir, err := prepareProjectDirectory(root, projectName, force, skipClone)
	if err != nil {
		return err
	}
	if !skipClone {
		if err := cloneProject(root, repoURL, projectName); err != nil {
			return err
		}
	}
	contextPath, err := scaffoldProject(projectDir, projectName)
	if err != nil {
		return err
	}

	fmt.Println()
	fmt.Printf("프로젝트 %s을(를) 추가했습니다.\n", projectName)
	refreshProjectList(root)

	// Exclude .pylon/ from project git tracking via .git/info/exclude
	if err := excludePylonFromRepo(projectDir); err != nil {
		fmt.Printf("⚠ .pylon/을 git 추적에서 자동으로 제외하지 못했습니다: %v\n", err)
		fmt.Println("  프로젝트의 .git/info/exclude에 '.pylon/'을 직접 추가하세요")
	} else {
		fmt.Println("✓ .pylon/을 프로젝트 git 추적에서 제외했습니다")
	}

	fmt.Println()
	fmt.Print(projectNextSteps(contextPath))

	return nil
}

func prepareProjectDirectory(root, projectName string, force, skipClone bool) (string, error) {
	projectDir := filepath.Join(root, projectName)
	info, statErr := os.Stat(projectDir)
	switch {
	case statErr == nil && !info.IsDir():
		return "", fmt.Errorf("%s이(가) 있지만 디렉터리가 아닙니다", projectName)
	case statErr == nil && force:
		fmt.Printf("기존 디렉터리 제거: %s\n", projectName)
		if err := os.RemoveAll(projectDir); err != nil {
			return "", fmt.Errorf("기존 디렉터리 제거 실패: %w", err)
		}
	case statErr == nil && skipClone:
		fmt.Printf("기존 디렉터리 사용: %s\n", projectName)
	case statErr == nil:
		return "", fmt.Errorf("디렉터리 %s이(가) 이미 있습니다 (--force로 다시 clone하거나 --skip-clone으로 기존 디렉터리를 사용하세요)", projectName)
	case !os.IsNotExist(statErr):
		return "", fmt.Errorf("%s에 접근할 수 없습니다: %w", projectName, statErr)
	case skipClone:
		return "", fmt.Errorf("디렉터리 %s이(가) 없어 --skip-clone을 쓸 수 없습니다", projectName)
	}
	return projectDir, nil
}

func cloneProject(root, repoURL, projectName string) error {
	fmt.Printf("clone 중: %s\n", repoURL)
	gitCmd := exec.Command("git", "clone", repoURL, projectName)
	gitCmd.Dir = root
	if output, err := gitCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("clone 실패: %w\n%s", err, output)
	}
	fmt.Printf("✓ clone 완료: %s\n", projectName)
	return nil
}

func scaffoldProject(projectDir, projectName string) (string, error) {
	pylonDir := layout.PylonDir(projectDir)
	if err := os.MkdirAll(pylonDir, 0o755); err != nil {
		return "", fmt.Errorf("프로젝트 .pylon/ 생성 실패: %w", err)
	}
	contextPath := filepath.Join(pylonDir, "context.md")
	f, err := os.OpenFile(contextPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if os.IsExist(err) {
		fmt.Println("✓ 기존 context.md 유지")
		return contextPath, nil
	}
	if err != nil {
		return "", fmt.Errorf("context.md 생성 실패: %w", err)
	}
	fmt.Println("기술 스택 감지 중...")
	stack := detectTechStack(projectDir)
	_, writeErr := f.WriteString(generateContextMD(projectName, stack))
	closeErr := f.Close()
	if writeErr != nil {
		_ = os.Remove(contextPath) // 빈 파일이 남으면 다음 실행이 "기존 파일"로 보존한다
		return "", fmt.Errorf("context.md 쓰기 실패: %w", writeErr)
	}
	if closeErr != nil {
		return "", fmt.Errorf("context.md 닫기 실패: %w", closeErr)
	}
	fmt.Println("✓ context.md 생성")
	return contextPath, nil
}

func projectNextSteps(contextPath string) string {
	return fmt.Sprintf(`다음 단계:
  1. 확인: %s
  2. 작업 시작: 이 디렉터리에서 Claude Code, Codex, Paseo 세션을 엽니다
`, contextPath)
}

// validateProjectName ensures the project name is a safe single directory name
// without path separators or traversal components.
