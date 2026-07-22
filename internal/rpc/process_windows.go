//go:build windows

package rpc

import (
	"os/exec"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

type processGroup interface {
	Signal(bool) error
	Close() error
}

type windowsProcessGroup struct {
	mu  sync.Mutex
	job windows.Handle
}

func configureProcessGroup(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
}

func attachProcessGroup(command *exec.Cmd) (processGroup, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, err
	}
	closeJob := true
	defer func() {
		if closeJob {
			_ = windows.CloseHandle(job)
		}
	}()
	information := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	information.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&information)), uint32(unsafe.Sizeof(information))); err != nil {
		return nil, err
	}
	process, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(command.Process.Pid))
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(process)
	if err := windows.AssignProcessToJobObject(job, process); err != nil {
		return nil, err
	}
	closeJob = false
	return &windowsProcessGroup{job: job}, nil
}

func (group *windowsProcessGroup) Signal(bool) error {
	group.mu.Lock()
	defer group.mu.Unlock()
	if group.job == 0 {
		return nil
	}
	return windows.TerminateJobObject(group.job, 1)
}

func (group *windowsProcessGroup) Close() error {
	group.mu.Lock()
	defer group.mu.Unlock()
	if group.job == 0 {
		return nil
	}
	err := windows.CloseHandle(group.job)
	group.job = 0
	return err
}
