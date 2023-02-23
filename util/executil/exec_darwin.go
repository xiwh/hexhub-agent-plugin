package executil

import (
	"os/exec"
)

func initCmd(cmd *exec.Cmd) error {
	//gid := syscall.Getgid()

	//cmd.SysProcAttr = &syscall.SysProcAttr{
	//	Setpgid: true,
	//	Pgid:    gid,
	//}
	return nil
}

func initPostCmd(cmd *exec.Cmd) error {
	return nil
}
