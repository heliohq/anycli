//go:build windows

package binresolve

import "errors"

// acquireLock is not implemented on windows: lazy install fails fast there
// (levels ① and ② still resolve normally). The sha256 pins for win32 stay in
// the definitions so a future windows installer needs no schema change.
func acquireLock(path string) (release func(), err error) {
	return nil, errors.New("lazy binary install is not supported on windows")
}
