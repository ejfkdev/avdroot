// Package emulator starts and restarts Android emulators.
//
// A full process restart is required after patching a ramdisk: "adb reboot"
// only reboots the guest, which reuses the initrd the emulator loaded when it
// started, so a new ramdisk.img is ignored. Only a new emulator process picks
// up the patched image.
package emulator

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/ejfkdev/avdroot/internal/adb"
)

// ErrNotFound reports that the emulator executable is missing.
var ErrNotFound = errors.New("emulator: executable not found")

// Binary is the emulator launcher for a given SDK.
func Binary(sdkRoot string) (string, error) {
	name := "emulator"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	candidates := []string{
		filepath.Join(sdkRoot, "emulator", name),
	}
	if p, err := exec.LookPath(name); err == nil {
		candidates = append(candidates, p)
	}
	for _, c := range candidates {
		if fi, err := os.Stat(c); err == nil && !fi.IsDir() {
			return c, nil
		}
	}
	return "", fmt.Errorf("%w: expected %s", ErrNotFound, candidates[0])
}

// RunningAVD returns the name of the AVD the connected emulator runs, or "" when
// the device is not an emulator.
func RunningAVD(cli *adb.Client) string {
	if cli == nil {
		return ""
	}
	for _, prop := range []string{"ro.boot.qemu.avd_name", "ro.kernel.qemu.avd_name"} {
		if v, err := cli.GetProp(prop); err == nil && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// IsEmulator reports whether the connected device is the Android emulator, as
// opposed to a physical device where an emulator restart is meaningless.
func IsEmulator(cli *adb.Client) bool {
	if cli == nil {
		return false
	}
	if v, err := cli.GetProp("ro.boot.hardware"); err == nil && strings.TrimSpace(v) == "ranchu" {
		return true
	}
	if v, err := cli.GetProp("ro.kernel.qemu"); err == nil && strings.TrimSpace(v) == "1" {
		return true
	}
	return RunningAVD(cli) != ""
}

// passthroughFlags are emulator options worth carrying over to a restart. They
// are recognised in the running qemu command line; anything not listed is left
// out because the launcher regenerates it.
var passthroughFlags = map[string]bool{
	"-no-window":       false,
	"-no-audio":        false,
	"-no-boot-anim":    false,
	"-no-snapstorage":  false,
	"-no-snapshot":     false,
	"-read-only":       false,
	"-wipe-data":       false,
	"-writable-system": false,
	// Options taking a value.
	"-gpu":            true,
	"-memory":         true,
	"-cores":          true,
	"-accel":          true,
	"-engine":         true,
	"-selinux":        true,
	"-feature":        true,
	"-prop":           true,
	"-dns-server":     true,
	"-timezone":       true,
	"-netdelay":       true,
	"-netspeed":       true,
	"-http-proxy":     true,
	"-partition-size": true,
}

// RunningQEMUArgs inspects the running emulator's command line and returns the
// subset of launcher options that should be reused on restart.
//
// This is best effort: the launcher expands -avd into many qemu arguments, and
// only the ones a user would have typed are worth keeping.
func RunningQEMUArgs(avdName string) []string {
	pid := findQEMUProcess(avdName)
	if pid == "" {
		return nil
	}
	cmdline := processCommandLine(pid)
	if cmdline == "" {
		return nil
	}
	fields := strings.Fields(cmdline)
	var out []string
	for i := 0; i < len(fields); i++ {
		f := fields[i]
		takesValue, ok := passthroughFlags[f]
		if !ok {
			continue
		}
		if takesValue {
			if i+1 < len(fields) {
				out = append(out, f, fields[i+1])
				i++
			}
			continue
		}
		out = append(out, f)
	}
	return out
}

// Options configures a restart.
type Options struct {
	// Args are launcher options appended after -avd NAME.
	Args []string
	// LogPath receives the emulator's output; defaults to a file in the
	// system temp directory.
	LogPath string
	// ForceColdBoot adds -no-snapshot so the emulator cannot restore a saved
	// VM state. A snapshot captures memory, including the initrd loaded when
	// it was taken, so loading one after a ramdisk patch would resurrect the
	// previous image and hide the patch. Always set this when the restart is
	// meant to pick up a new ramdisk.
	ForceColdBoot bool
	// WaitTimeout bounds how long to wait for the device to come back.
	WaitTimeout time.Duration
}

// Restart kills the running emulator and starts a new process for avdName.
//
// Shutdown is detected by watching the device disappear from adb rather than by
// polling for the process, because adb works identically on every platform and
// is the signal that actually matters: the port has to be free before the
// replacement can bind it.
//
// It returns the command that was launched so callers can show it to the user.
func Restart(cli *adb.Client, sdkRoot, avdName string, opts Options) (string, error) {
	if avdName == "" {
		return "", errors.New("cannot restart the emulator: the AVD name is unknown")
	}
	bin, err := Binary(sdkRoot)
	if err != nil {
		return "", err
	}

	// Remember which device to watch, in case the caller did not pin a serial.
	serial := cli.Serial
	if serial == "" {
		serial, _ = cli.Online()
	}

	// A clean shutdown through the emulator console avoids leaving a stale
	// process holding the AVD's lock files.
	_, _ = cli.Run("emu", "kill")
	waitTimeout := opts.WaitTimeout
	if waitTimeout <= 0 {
		waitTimeout = 60 * time.Second
	}
	if err := waitForDeviceGone(cli, serial, waitTimeout); err != nil {
		return "", err
	}

	args := launchArgs(avdName, opts)
	command := bin + " " + strings.Join(args, " ")
	if err := start(bin, args, opts.LogPath); err != nil {
		return command, err
	}
	return command, nil
}

// launchArgs builds the launcher command line, replacing any snapshot flags the
// caller supplied when a cold boot is required.
func launchArgs(avdName string, opts Options) []string {
	args := []string{"-avd", avdName}
	for _, a := range opts.Args {
		if opts.ForceColdBoot && isSnapshotFlag(a) {
			continue
		}
		args = append(args, a)
	}
	if opts.ForceColdBoot {
		args = append(args, "-no-snapshot")
	}
	return args
}

// isSnapshotFlag reports whether an option governs snapshot load or save.
func isSnapshotFlag(arg string) bool {
	switch arg {
	case "-no-snapshot", "-no-snapshot-load", "-no-snapshot-save",
		"-no-snapstorage", "-snapshot", "-snapstorage", "-snapshot-list":
		return true
	}
	return strings.HasPrefix(arg, "-snapshot")
}

// waitForDeviceGone blocks until the adb device is no longer listed. When the
// serial is unknown, any still-listed emulator is treated as the one to wait
// for.
func waitForDeviceGone(cli *adb.Client, serial string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		devices, err := cli.Devices()
		if err != nil {
			// adb itself may be restarting; keep waiting rather than failing.
			time.Sleep(time.Second)
			continue
		}
		present := false
		for _, d := range devices {
			if serial != "" {
				if d.Serial == serial {
					present = true
					break
				}
				continue
			}
			if strings.HasPrefix(d.Serial, "emulator-") {
				present = true
				break
			}
		}
		if !present {
			return nil
		}
		time.Sleep(time.Second)
	}
	if serial != "" {
		return fmt.Errorf("device %s was still listed after %s; the emulator may not have shut down", serial, timeout)
	}
	return fmt.Errorf("the emulator was still listed after %s; it may not have shut down", timeout)
}

// WaitForBoot blocks until the restarted device finishes booting.
func WaitForBoot(cli *adb.Client, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	// The device disappears and reappears, so allow for both phases.
	for time.Now().Before(deadline) {
		if _, err := cli.Online(); err == nil {
			if v, err := cli.GetProp("sys.boot_completed"); err == nil && strings.TrimSpace(v) == "1" {
				return nil
			}
		}
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("the emulator did not finish booting within %s", timeout)
}

// EnsureRunning makes sure the emulator for avdName is up, starting it when
// nothing is connected, and returns whether it had to start one.
//
// Some steps need a running device before the image can be patched at all:
// the preinit block device is discovered from the live mount table, so the
// emulator has to be up before the ramdisk is written. Starting it here is what
// lets "avdroot root" run unattended from a cold start.
func EnsureRunning(cli *adb.Client, sdkRoot, avdName string, opts Options, timeout time.Duration) (bool, error) {
	if _, err := cli.Online(); err == nil {
		return false, nil
	}
	if avdName == "" {
		return false, errors.New("cannot start the emulator: the AVD name is unknown")
	}
	bin, err := Binary(sdkRoot)
	if err != nil {
		return false, err
	}
	args := launchArgs(avdName, opts)
	if err := start(bin, args, opts.LogPath); err != nil {
		return false, fmt.Errorf("starting the emulator: %w", err)
	}
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	if err := WaitForBoot(cli, timeout); err != nil {
		return true, err
	}
	return true, nil
}

// StartCommand renders the command that would launch an AVD, for messages.
func StartCommand(sdkRoot, avdName string, opts Options) string {
	bin, err := Binary(sdkRoot)
	if err != nil {
		bin = "emulator"
	}
	return bin + " " + strings.Join(launchArgs(avdName, opts), " ")
}
