//go:build windows

package localagents

import (
	"os"
	"os/exec"

	"golang.org/x/sys/windows"
)

func setSysProcAttr(cmd *exec.Cmd) {}

func signalPID(pid int, sig string) error {
	switch sig {
	case "TERM", "KILL":
		proc, err := os.FindProcess(pid)
		if err != nil {
			return err
		}
		return proc.Kill()
	case "CHECK":
		if isProcessAlive(pid) {
			return nil
		}
		return os.ErrProcessDone
	}
	return nil
}

func isProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(h)
	var code uint32
	if err := windows.GetExitCodeProcess(h, &code); err != nil {
		return false
	}
	const stillActive = 259
	return code == stillActive
}
