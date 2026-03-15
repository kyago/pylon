package git

import "os/exec"

// CommandRunner는 외부 명령 실행을 추상화합니다.
type CommandRunner interface {
	Run(dir string, name string, args ...string) ([]byte, error)
}

// ExecRunner는 실제 os/exec를 사용하는 구현체입니다.
type ExecRunner struct{}

func (r *ExecRunner) Run(dir string, name string, args ...string) ([]byte, error) {
	cmd := exec.Command(name, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	return cmd.CombinedOutput()
}

// defaultRunner는 패키지 레벨 기본 CommandRunner입니다.
// 테스트에서 교체하여 외부 명령 실행 없이 테스트할 수 있습니다.
var defaultRunner CommandRunner = &ExecRunner{}
