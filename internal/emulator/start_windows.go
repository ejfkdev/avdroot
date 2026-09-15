//go:build windows

package emulator

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

// Windows process creation flags used to detach the emulator from avdroot.
const (
	createNewProcessGroup = 0x00000200
	detachedProcess       = 0x00000008
)

// start launches the emulator detached from this process, so it keeps running
// after avdroot exits, and redirects its output to a log file.
func start(binary string, args []string, logPath string) error {
	if logPath == "" {
		logPath = filepath.Join(os.TempDir(), "avdroot-emulator.log")
	}
	logFile, err := os.Create(logPath)
	if err != nil {
		return err
	}
	defer logFile.Close()

	cmd := exec.Command(binary, args...)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: createNewProcessGroup | detachedProcess,
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}
