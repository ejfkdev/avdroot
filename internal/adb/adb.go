// Package adb wraps the Android Debug Bridge. It is only needed for the parts
// of the workflow that depend on a running emulator: determining the preinit
// block device, installing the Magisk app, and verifying that root works.
// Patching a ramdisk never requires adb.
package adb

import (
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// ErrNoDevice is returned when no emulator or device is available.
var ErrNoDevice = errors.New("adb: no device")

// Client runs adb commands against one device.
type Client struct {
	// Path is the adb executable.
	Path string
	// Serial selects a device; empty means the only connected one.
	Serial string
}

// Find locates the adb executable, preferring the SDK's platform-tools over
// whatever happens to be on PATH so the version matches the installed SDK.
func Find(sdkRoot string) (string, error) {
	var candidates []string
	if sdkRoot != "" {
		name := "adb"
		if runtime.GOOS == "windows" {
			name = "adb.exe"
		}
		candidates = append(candidates, filepath.Join(sdkRoot, "platform-tools", name))
	}
	if p, err := exec.LookPath("adb"); err == nil {
		candidates = append(candidates, p)
	}
	for _, c := range candidates {
		if fi, err := exec.LookPath(c); err == nil || isExecutable(c) {
			if err == nil {
				return fi, nil
			}
			return c, nil
		}
	}
	return "", errors.New("adb not found; install Android SDK platform-tools or put adb on PATH")
}

func isExecutable(p string) bool {
	cmd := exec.Command(p, "version")
	return cmd.Run() == nil
}

// New returns a client using the given adb path and device serial.
func New(adbPath, serial string) *Client { return &Client{Path: adbPath, Serial: serial} }

func (c *Client) args(sub ...string) []string {
	if c.Serial != "" {
		return append([]string{"-s", c.Serial}, sub...)
	}
	return sub
}

// Run executes an adb subcommand and returns its combined output.
func (c *Client) Run(sub ...string) (string, error) {
	cmd := exec.Command(c.Path, c.args(sub...)...)
	out, err := cmd.CombinedOutput()
	s := strings.TrimSpace(string(out))
	if err != nil {
		if s != "" {
			return s, fmt.Errorf("adb %s: %w: %s", strings.Join(sub, " "), err, s)
		}
		return s, fmt.Errorf("adb %s: %w", strings.Join(sub, " "), err)
	}
	return s, nil
}

// Shell runs a command inside the device and returns its output.
func (c *Client) Shell(command string) (string, error) {
	return c.Run("shell", command)
}

// Shellf runs a formatted command inside the device.
func (c *Client) Shellf(format string, a ...any) (string, error) {
	return c.Shell(fmt.Sprintf(format, a...))
}

// Device is one connected adb target.
type Device struct {
	Serial string
	State  string
	Detail string
}

// Devices lists the connected devices.
func (c *Client) Devices() ([]Device, error) {
	out, err := c.Run("devices", "-l")
	if err != nil {
		return nil, err
	}
	var devices []Device
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "List of devices") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		d := Device{Serial: fields[0], State: fields[1]}
		if len(fields) > 2 {
			d.Detail = strings.Join(fields[2:], " ")
		}
		devices = append(devices, d)
	}
	return devices, nil
}

// Online reports whether a usable device is connected, returning its serial.
func (c *Client) Online() (string, error) {
	devices, err := c.Devices()
	if err != nil {
		return "", err
	}
	for _, d := range devices {
		if d.State != "device" {
			continue
		}
		if c.Serial == "" || d.Serial == c.Serial {
			return d.Serial, nil
		}
	}
	return "", fmt.Errorf("%w: no device in the \"device\" state", ErrNoDevice)
}

// GetProp reads a system property.
func (c *Client) GetProp(name string) (string, error) {
	out, err := c.Shellf("getprop %s", name)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// Root restarts adbd as root, which Google APIs images allow and Play Store
// images do not.
//
// Restarting adbd drops the connection, and the command can fail with
// "unable to connect for root" if it arrives while adbd is busy — which happens
// readily straight after installing an APK. Those failures are transient, so
// they are retried rather than reported.
func (c *Client) Root() error {
	var lastErr error
	for attempt := 0; attempt < 5; attempt++ {
		if attempt > 0 {
			time.Sleep(3 * time.Second)
		}
		out, err := c.Run("root")
		if err == nil {
			if strings.Contains(out, "cannot run as root") {
				return errors.New("this image does not allow adb root (Play Store images cannot be rooted this way)")
			}
			if _, err := c.WaitOnline(30 * time.Second); err != nil {
				return err
			}
			return nil
		}
		lastErr = fmt.Errorf("adb root: %w: %s", err, out)
		if !retriable(out) {
			return lastErr
		}
	}
	return lastErr
}

// retriable reports whether an adb failure is the kind that goes away by itself
// while adbd restarts.
func retriable(output string) bool {
	for _, s := range []string{
		"unable to connect", "closed", "device offline", "device not found",
		"connection reset", "protocol fault",
	} {
		if strings.Contains(strings.ToLower(output), s) {
			return true
		}
	}
	return false
}

// WaitOnline waits for a device to appear and become usable, which is needed
// after an operation that restarts the device itself.
func (c *Client) WaitOnline(timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		if serial, err := c.Online(); err == nil {
			return serial, nil
		} else {
			lastErr = err
		}
		time.Sleep(time.Second)
	}
	return "", fmt.Errorf("adb: no device after %s: %w", timeout, lastErr)
}

// WaitReady waits for a connected device to become usable, without waiting for
// one to appear.
//
// It is for tolerating an adbd restart — after "adb root" or installing an APK,
// the device is briefly offline — as opposed to WaitOnline, which also waits
// for a device that is not there yet. Reporting "no emulator is running"
// immediately is much better than pausing for the full timeout, so callers that
// only need to ride out a restart should use this.
func (c *Client) WaitReady(timeout time.Duration) (string, error) {
	devices, err := c.Devices()
	if err != nil {
		return "", err
	}
	if len(devices) == 0 {
		return "", fmt.Errorf("%w: no device connected", ErrNoDevice)
	}
	return c.WaitOnline(timeout)
}

// Install installs an APK, replacing any existing copy.
func (c *Client) Install(apkPath string) (string, error) {
	return c.Run("install", "-r", "-d", apkPath)
}

// PackageInstalled reports whether a package is present by scanning the
// package list for a suffix match.
func (c *Client) PackageInstalled(name string) (string, bool) {
	out, err := c.Shell("pm list packages")
	if err != nil {
		return "", false
	}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		pkg := strings.TrimPrefix(line, "package:")
		if pkg == "" {
			continue
		}
		if pkg == name || strings.HasSuffix(pkg, "."+name) {
			return pkg, true
		}
	}
	return "", false
}
