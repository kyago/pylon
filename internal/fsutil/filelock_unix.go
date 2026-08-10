//go:build unix

package fsutil

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

// tryLockFile는 flock(2)으로 배타 잠금을 한 번 시도한다. 이미 다른 보유자가
// 잡고 있으면 errLockContended를 반환해 호출자가 재시도하게 한다.
func tryLockFile(file *os.File) (func(), error) {
	fd := int(file.Fd())
	if err := syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
			return nil, fmt.Errorf("%w: %v", errLockContended, err)
		}
		return nil, err
	}
	return func() { _ = syscall.Flock(fd, syscall.LOCK_UN) }, nil
}
