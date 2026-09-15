package adb

import (
	"errors"
	"fmt"
	"strings"
)

// MagiskEnvDir is where Magisk keeps what the manager app unpacks for it.
//
// A patched ramdisk carries Magisk's init and its daemon and nothing else, so
// this directory is the rest of Magisk. magiskd inspects it while starting and
// will not put su on PATH until it is complete, which is why a freshly patched
// emulator reports Magisk running while su still resolves to the system's own.
const MagiskEnvDir = "/data/adb/magisk"

// MagiskEnvStaging is where the files are pushed before being installed, so
// that a failed transfer cannot leave /data/adb/magisk half written.
const MagiskEnvStaging = "/data/local/tmp/avdroot-env"

// EnvState is how complete Magisk's environment is, in the terms Magisk itself
// uses. The values match the exit codes of its env_check shell function.
type EnvState int

const (
	// EnvOK means env_check would return 0.
	EnvOK EnvState = iota
	// EnvMissing means env_check would return 1: a file it insists on is absent.
	EnvMissing
	// EnvVersionMismatch means env_check would return 3: the environment belongs
	// to a different Magisk than the one asking.
	EnvVersionMismatch
	// EnvNoMagisk means there is no running Magisk to ask, so the question does
	// not arise yet. It is not a failure.
	EnvNoMagisk
)

// envCheckFiles are the files Magisk's env_check refuses to do without, plus
// magiskpolicy, which it requires from version 25000 on.
var envCheckFiles = []string{
	"busybox", "magiskboot", "magiskinit", "util_functions.sh", "boot_patch.sh", "magiskpolicy",
}

// MagiskEnvCheck reports how complete the environment is, mirroring Magisk's own
// env_check from scripts/app_functions.sh.
//
// One of its conditions is deliberately left out: it also insists on finding the
// preinit block device inside the running Magisk's tmpfs, which describes the
// patched ramdisk rather than this directory, and cannot be answered before the
// boot that would create it. That condition belongs to patching, where this tool
// already resolves the device.
func (c *Client) MagiskEnvCheck(version string, versionCode int) (EnvState, error) {
	if err := c.EnsureRoot(); err != nil {
		return EnvNoMagisk, err
	}

	// A single shell invocation, so the answer arrives in one round trip and
	// there is no window between the checks.
	var b strings.Builder
	b.WriteString("d=" + MagiskEnvDir + "; ")
	for _, f := range envCheckFiles {
		fmt.Fprintf(&b, "[ -f $d/%s ] || { echo missing; exit 0; }; ", f)
	}
	fmt.Fprintf(&b, "grep -xqF %s $d/util_functions.sh || { echo version; exit 0; }; ",
		shellQuote("MAGISK_VER='"+version+"'"))
	fmt.Fprintf(&b, "grep -xqF %s $d/util_functions.sh || { echo version; exit 0; }; ",
		shellQuote(fmt.Sprintf("MAGISK_VER_CODE=%d", versionCode)))
	b.WriteString("echo ok")

	out, err := c.Shell(b.String())
	if err != nil {
		return EnvNoMagisk, err
	}
	switch {
	case strings.Contains(out, "ok"):
		return EnvOK, nil
	case strings.Contains(out, "missing"):
		return EnvMissing, nil
	case strings.Contains(out, "version"):
		return EnvVersionMismatch, nil
	}
	return EnvNoMagisk, fmt.Errorf("unexpected answer from the environment check: %q", strings.TrimSpace(out))
}

// InstallMagiskEnv replaces the environment with the files staged in localDir.
//
// This is what the manager app does when it fixes the environment, and the
// steps are Magisk's own, from fix_env in scripts/app_functions.sh: clear the
// directory, copy the new files in, then hand them to root with 0755. The
// module directories it also creates are what Magisk expects to find beside the
// binaries at startup.
func (c *Client) InstallMagiskEnv(localDir string) error {
	if err := c.EnsureRoot(); err != nil {
		return err
	}
	// The transfer happens before anything is removed, so a device that cannot
	// take the files keeps the environment it already had.
	if _, err := c.Run("push", localDir+"/.", MagiskEnvStaging); err != nil {
		return fmt.Errorf("adb: pushing Magisk's files: %w", err)
	}

	script := strings.Join([]string{
		"set -e",
		"rm -rf " + MagiskEnvDir,
		"mkdir -p " + MagiskEnvDir,
		"chmod 700 /data/adb",
		"cp -af " + MagiskEnvStaging + "/. " + MagiskEnvDir + "/",
		"chmod -R 755 " + MagiskEnvDir,
		"chown -R 0:0 " + MagiskEnvDir,
		"mkdir -p /data/adb/modules /data/adb/post-fs-data.d /data/adb/service.d",
		"rm -rf " + MagiskEnvStaging,
		// The caller restarts the emulator next, and "adb emu kill" ends the
		// process without waiting for the guest to write anything back. Without
		// this the files are present right up until the moment they are needed
		// and then simply missing from the next boot, which is indistinguishable
		// from a patch that did not take.
		"sync",
	}, " && ")
	if out, err := c.Shell(script); err != nil {
		return fmt.Errorf("adb: installing Magisk's files: %w: %s", err, strings.TrimSpace(out))
	}

	// Read back what was written. A truncated push or a copy that silently did
	// nothing would otherwise only show up as su failing after a reboot.
	out, err := c.Shellf("ls %s | wc -l", MagiskEnvDir)
	if err != nil {
		return fmt.Errorf("adb: verifying Magisk's files: %w", err)
	}
	if n := strings.TrimSpace(out); n == "0" || n == "" {
		return errors.New("adb: Magisk's files were installed but the directory is empty")
	}
	return nil
}

// shellQuote wraps s in single quotes for the device shell, escaping any single
// quote inside it.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
