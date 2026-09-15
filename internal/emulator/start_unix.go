//go:build !windows

package emulator

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
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
	// A new session detaches the emulator from our terminal and process
	// group, so Ctrl-C on avdroot does not kill it.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	// The child is intentionally not waited for.
	return cmd.Process.Release()
}
