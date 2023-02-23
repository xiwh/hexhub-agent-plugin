package executil

import (
	"os/exec"
	"syscall"
)

func initCmd(cmd *exec.Cmd) error {
	gid := syscall.Getgid()
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid:   true,
		Pgid:      gid,
		Pdeathsig: syscall.SIGHUP | syscall.SIGINT | syscall.SIGTERM | syscall.SIGQUIT,
	}
	return nil
}

func initPostCmd(cmd *exec.Cmd) error {
	return nil
}
