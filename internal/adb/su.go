package adb

import (
	"errors"
	"fmt"
	"strings"
)

// Su policy values, from Magisk's SuPolicy enum
// (native/src/core/lib.rs, #[repr(i32)]).
//
// Absence of a row is not the same as Deny: with no row the daemon asks the
// Magisk app, which is why an interactive tap is normally required.
const (
	SuPolicyQuery    = 0
	SuPolicyDeny     = 1
	SuPolicyAllow    = 2
	SuPolicyRestrict = 3
)

// AIDShell is the Android user id of the adb shell, and of the process that
// runs "su" when invoked over adb. Granting it is what allows an automated
// check to obtain root without tapping anything in the Magisk app.
const AIDShell = 2000

// magiskCandidates are the places Magisk puts its binary, most likely first.
// /debug_ramdisk is the Magisk tmpfs on two-stage-init devices, /sbin on older
// ones, and /data/adb holds the persisted copy.
var magiskCandidates = []string{
	"/debug_ramdisk/magisk",
	"/sbin/magisk",
	"/data/adb/magisk/magisk",
}

// MagiskBinary locates the magisk binary on the device.
func (c *Client) MagiskBinary() (string, error) {
	for _, path := range magiskCandidates {
		out, err := c.Shellf("[ -x %s ] && echo yes", path)
		if err == nil && strings.Contains(out, "yes") {
			return path, nil
		}
	}
	return "", errors.New("adb: the magisk binary was not found on the device; Magisk has not initialised")
}

// MagiskVersion returns the version string reported by the device, e.g.
// "31.0:MAGISK:R".
func (c *Client) MagiskVersion() (string, error) {
	bin, err := c.MagiskBinary()
	if err != nil {
		return "", err
	}
	out, err := c.Shellf("%s -v", bin)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// IsShellRoot reports whether adb commands already execute as uid 0, which is
// the case after "adb root".
func (c *Client) IsShellRoot() bool {
	out, err := c.Shell("id -u")
	if err != nil {
		return false
	}
	return strings.TrimSpace(out) == "0"
}

// EnsureRoot restarts adbd as root when it is not already, which Google APIs
// emulator images permit.
func (c *Client) EnsureRoot() error {
	if c.IsShellRoot() {
		return nil
	}
	if err := c.Root(); err != nil {
		return err
	}
	if !c.IsShellRoot() {
		return errors.New("adb: adbd did not switch to root")
	}
	return nil
}

// SetSuPolicy writes a su policy row for uid, so that Magisk grants or denies
// root without prompting. until=0 means the decision never expires.
//
// This is the documented policy store: the daemon reads the row for every
// request, so the change takes effect immediately and survives reboots.
func (c *Client) SetSuPolicy(uid, policy int) error {
	bin, err := c.MagiskBinary()
	if err != nil {
		return err
	}
	sql := fmt.Sprintf(
		"REPLACE INTO policies (uid, policy, until, logging, notification) VALUES (%d, %d, 0, 1, 1)",
		uid, policy)
	if out, err := c.Shellf(`%s --sqlite "%s"`, bin, sql); err != nil {
		return fmt.Errorf("adb: writing the su policy for uid %d: %w: %s", uid, err, out)
	}

	// Read the row back rather than trusting the write: a silent failure here
	// would only surface much later as a confusing permission denial.
	out, err := c.Shellf(`%s --sqlite "SELECT policy FROM policies WHERE uid=%d"`, bin, uid)
	if err != nil {
		return fmt.Errorf("adb: verifying the su policy for uid %d: %w", uid, err)
	}
	want := fmt.Sprintf("policy=%d", policy)
	if !strings.Contains(out, want) {
		return fmt.Errorf("adb: the su policy for uid %d was not stored (device reports %q, expected %s)",
			uid, strings.TrimSpace(out), want)
	}
	return nil
}

// SuGrantedToShell reports the current policy for the shell user, or false when
// no row exists (which means "ask the app").
func (c *Client) SuGrantedToShell() (bool, error) {
	bin, err := c.MagiskBinary()
	if err != nil {
		return false, err
	}
	out, err := c.Shellf(`%s --sqlite "SELECT policy FROM policies WHERE uid=%d"`, bin, AIDShell)
	if err != nil {
		return false, err
	}
	return strings.Contains(out, fmt.Sprintf("policy=%d", SuPolicyAllow)), nil
}

// SuID runs "su -c id" and returns its output. An error means su refused or
// could not run.
func (c *Client) SuID() (string, error) {
	out, err := c.Shell("su -c id")
	if err != nil {
		return out, err
	}
	return strings.TrimSpace(out), nil
}
