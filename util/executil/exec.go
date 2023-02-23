package executil

import (
	"os/exec"
)

func ExecChildProcess(path string, args ...string) (*exec.Cmd, error) {
	cmd := exec.Command(
		path,
		args...,
	)
	err := initCmd(cmd)
	if err != nil {
		return nil, err
	}
	err = cmd.Start()
	if err != nil {
		return nil, err
	}
	err = initPostCmd(cmd)
	if err != nil {
		_ = cmd.Process.Kill()
		return nil, err
	}
	return cmd, nil
}
