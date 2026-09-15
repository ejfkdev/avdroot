//go:build windows

package emulator

import (
	"os/exec"
	"strings"
)

// findQEMUProcess returns the PID of the qemu process backing an AVD, or "".
//
// Windows has no pgrep, so tasklist is used. This is only needed for the
// optional launcher-flag detection: shutting the emulator down is detected
// through adb instead, which works identically on every platform.
func findQEMUProcess(avdName string) string {
	out, err := exec.Command("tasklist", "/FI", "IMAGENAME eq qemu-system-*", "/FO", "CSV", "/NH").Output()
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Split(strings.TrimSpace(line), ",")
		if len(fields) < 2 {
			continue
		}
		pid := strings.Trim(strings.TrimSpace(fields[1]), `"`)
		if pid == "" || pid == "PID" {
			continue
		}
		if avdName == "" || strings.Contains(processCommandLine(pid), avdName) {
			return pid
		}
	}
	return ""
}

// processCommandLine reads a process's full command line through PowerShell.
// Returns "" when the query is unavailable, which callers treat as "unknown".
func processCommandLine(pid string) string {
	const query = `(Get-CimInstance Win32_Process -Filter "ProcessId=` + "%s" + `").CommandLine`
	out, err := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command",
		strings.Replace(query, "%s", pid, 1)).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
