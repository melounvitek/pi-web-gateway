//go:build !linux && !darwin

package update

import "os/exec"

func configureCommand(*exec.Cmd) {}
