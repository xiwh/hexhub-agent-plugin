package executil

import (
	"os/exec"
	"syscall"
)

func initCmd(cmd *exec.Cmd) error {
	cmd.SysProcAttr = &syscall.SysProcAttr{}
	return nil
}

func initPostCmd(cmd *exec.Cmd) error {
	return nil
}
