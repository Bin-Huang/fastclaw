//go:build !windows

package localagents

import (
	"os"
	"os/exec"
	"syscall"
)

func setSysProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}

func signalPID(pid int, sig string) error {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	switch sig {
	case "TERM":
		return proc.Signal(syscall.SIGTERM)
	case "KILL":
		return proc.Signal(syscall.SIGKILL)
	case "CHECK":
		return proc.Signal(syscall.Signal(0))
	}
	return nil
}

func isProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	return signalPID(pid, "CHECK") == nil
}
