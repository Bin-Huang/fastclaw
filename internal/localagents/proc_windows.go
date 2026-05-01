//go:build windows

package localagents

import (
	"os"
	"os/exec"
)

func setSysProcAttr(cmd *exec.Cmd) {}

func signalPID(pid int, sig string) error {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	switch sig {
	case "TERM", "KILL":
		return proc.Kill()
	case "CHECK":
		return nil
	}
	return nil
}

func isProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	return signalPID(pid, "CHECK") == nil
}
