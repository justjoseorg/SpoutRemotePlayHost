package update

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

// apply runs Setup through a short-lived helper (spout-host -run-setup). Setup stops every
// spout-host.exe and its child processes, so it must not be a child of this host.
func apply(path string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(exe, "-run-setup", path)
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.DETACHED_PROCESS | windows.CREATE_NEW_PROCESS_GROUP}
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("starting Setup: %w", err)
	}
	return nil
}

// RunSetup starts Setup silently, asking for administrator rights if the host doesn't have
// them, and returns as soon as it started. Setup restarts the host when it's done.
func RunSetup(path string) error {
	args := "/VERYSILENT /SUPPRESSMSGBOXES /NORESTART /SP- /RELAUNCH=1"
	return windows.ShellExecute(0, windows.StringToUTF16Ptr("runas"), windows.StringToUTF16Ptr(path),
		windows.StringToUTF16Ptr(args), nil, windows.SW_HIDE)
}
