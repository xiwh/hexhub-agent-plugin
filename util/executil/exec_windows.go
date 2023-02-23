package executil

import (
	"golang.org/x/sys/windows"
	"os"
	"os/exec"
	"syscall"
	"unsafe"
)

func initCmd(cmd *exec.Cmd) error {
	currentProcess, err := syscall.GetCurrentProcess()
	if err != nil {
		return err
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		ParentProcess: currentProcess,
	}
	//windows.StartupInfo{}

	return nil
}

func initPostCmd(cmd *exec.Cmd) error {
	g, err := newProcessExitGroup()
	if err != nil {
		return err
	}

	if err = g.addProcess(cmd.Process); err != nil {
		_ = g.dispose()
		return err
	}

	go func() {
		_ = cmd.Wait()
		g.dispose()
	}()

	return nil
}

type process struct {
	Pid    int
	Handle uintptr
}

type processExitGroup windows.Handle

func newProcessExitGroup() (processExitGroup, error) {
	handle, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return 0, err
	}

	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{
		BasicLimitInformation: windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{
			LimitFlags: windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE,
		},
	}
	if _, err := windows.SetInformationJobObject(
		handle,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info))); err != nil {
		return 0, err
	}

	return processExitGroup(handle), nil
}

func (g processExitGroup) dispose() error {
	return windows.CloseHandle(windows.Handle(g))
}

func (g processExitGroup) addProcess(p *os.Process) error {
	return windows.AssignProcessToJobObject(
		windows.Handle(g),
		windows.Handle((*process)(unsafe.Pointer(p)).Handle))
}
