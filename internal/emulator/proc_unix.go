//go:build !windows

package emulator

import (
	"os/exec"
	"strings"
)

// findQEMUProcess returns the PID of the qemu process backing an AVD, or "".
// Used only for the optional flag detection; shutting the emulator down is
// detected through adb so that it works on every platform.
func findQEMUProcess(avdName string) string {
	patterns := []string{"qemu-system.*-avd " + avdName, "qemu-system"}
	for _, pattern := range patterns {
		out, err := exec.Command("pgrep", "-f", pattern).Output()
		if err != nil {
			continue
		}
		if pids := strings.Fields(strings.TrimSpace(string(out))); len(pids) > 0 {
			return pids[0]
		}
	}
	return ""
}

// processCommandLine reads a process's full command line.
func processCommandLine(pid string) string {
	out, err := exec.Command("ps", "-o", "command=", "-p", pid).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
