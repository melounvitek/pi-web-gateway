//go:build windows

package sessions

import "os"

func nativeFileIdentity(os.FileInfo) (uint64, uint64, bool) {
	return 0, 0, false
}
