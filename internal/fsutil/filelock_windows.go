//go:build windows

package fsutil

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

// lockRegionLow는 잠글 바이트 범위의 길이(하위 32비트)다. 파일 내용과 무관한
// 순수 잠금 파일이므로 첫 1바이트만 잠가도 상호 배제에 충분하다.
const lockRegionLow = 1

// tryLockFile는 LockFileEx로 배타 잠금을 한 번 시도한다. flock(2)과 달리
// 경합 시 EWOULDBLOCK이 아니라 ERROR_LOCK_VIOLATION을 돌려주므로 그 값을
// errLockContended로 감싸 호출자가 재시도하게 한다. 잠금은 핸들에 걸리므로
// 프로세스가 비정상 종료해도 커널이 해제한다.
func tryLockFile(file *os.File) (func(), error) {
	handle := windows.Handle(file.Fd())
	overlapped := new(windows.Overlapped)
	flags := uint32(windows.LOCKFILE_EXCLUSIVE_LOCK | windows.LOCKFILE_FAIL_IMMEDIATELY)
	if err := windows.LockFileEx(handle, flags, 0, lockRegionLow, 0, overlapped); err != nil {
		if errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
			return nil, fmt.Errorf("%w: %v", errLockContended, err)
		}
		return nil, err
	}
	return func() {
		_ = windows.UnlockFileEx(handle, 0, lockRegionLow, 0, new(windows.Overlapped))
	}, nil
}
