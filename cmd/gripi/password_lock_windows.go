//go:build windows

package main

import (
	"errors"
	"os"
	"time"

	"golang.org/x/sys/windows"
)

func lockPasswordFile(path string) (func(), error) {
	file, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	overlapped := new(windows.Overlapped)
	deadline := time.Now().Add(10 * time.Second)
	for {
		err = windows.LockFileEx(windows.Handle(file.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, overlapped)
		if err == nil {
			return func() {
				_ = windows.UnlockFileEx(windows.Handle(file.Fd()), 0, 1, 0, overlapped)
				_ = file.Close()
			}, nil
		}
		if !errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
			file.Close()
			return nil, err
		}
		if time.Now().After(deadline) {
			file.Close()
			return nil, errors.New("timed out waiting to create admin password")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
