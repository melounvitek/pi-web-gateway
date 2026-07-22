//go:build linux || darwin

package update

import (
	"errors"
	"os"
	"os/exec"

	"golang.org/x/sys/unix"
)

func configureCommand(command *exec.Cmd) {
	command.SysProcAttr = &unix.SysProcAttr{Setpgid: true}
	command.Cancel = func() error {
		if command.Process == nil {
			return os.ErrProcessDone
		}
		if err := unix.Kill(-command.Process.Pid, unix.SIGKILL); err != nil {
			if errors.Is(err, unix.ESRCH) {
				return os.ErrProcessDone
			}
			return err
		}
		return nil
	}
}
