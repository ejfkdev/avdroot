package magisk

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/ejfkdev/avdroot/internal/cpio"
	"github.com/ejfkdev/avdroot/internal/imgfmt"
)

// Options controls how a ramdisk is patched. The defaults match what a Magisk
// installation on an emulator uses: dm-verity and forced encryption are kept,
// because the AVD's system and data partitions rely on them.
type Options struct {
	KeepVerity       bool
	KeepForceEncrypt bool
	RecoveryMode     bool
	VendorBoot       bool
	// PreinitDevice names the block device Magisk stores its preinit data on.
	// On an emulator this is normally the partition mounted at /metadata.
	PreinitDevice string
	// SHA1 records the image the patch was derived from. Purely informational
	// for the Magisk app, and optional.
	SHA1 string
}

// Config renders the .backup/.magisk file exactly as boot_patch.sh does.
func (o Options) Config() []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "KEEPVERITY=%t\n", o.KeepVerity)
	fmt.Fprintf(&b, "KEEPFORCEENCRYPT=%t\n", o.KeepForceEncrypt)
	fmt.Fprintf(&b, "RECOVERYMODE=%t\n", o.RecoveryMode)
	fmt.Fprintf(&b, "VENDORBOOT=%t\n", o.VendorBoot)
	if o.PreinitDevice != "" {
		fmt.Fprintf(&b, "PREINITDEVICE=%s\n", o.PreinitDevice)
	}
	if o.SHA1 != "" {
		fmt.Fprintf(&b, "SHA1=%s\n", o.SHA1)
	}
	return []byte(b.String())
}

// ParseConfig reads a .backup/.magisk style key=value file. Lines without '='
// and comments are ignored, matching Magisk's for_each_prop.
func ParseConfig(data []byte) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		// Drop a trailing NUL that old Magisk versions counted in the size.
		out[strings.TrimSpace(k)] = strings.TrimSpace(strings.TrimRight(v, "\x00"))
	}
	return out
}

// Result reports what Patch did.
type Result struct {
	// Archive is the patched ramdisk payload, ready to be serialised.
	Archive *cpio.Archive
	// PreviousStatus is what the ramdisk looked like before patching.
	PreviousStatus cpio.Status
	// Options is the effective option set, after inheriting values from an
	// earlier patch when the caller left them empty.
	Options Options
	// Added lists the entries the patch introduced.
	Added []string
	// BackedUp lists the stock entries whose original contents were preserved,
	// so restore can put them back.
	BackedUp []string
}

// ErrUnsupported is returned when the ramdisk carries a foreign patch.
var ErrUnsupported = errors.New("ramdisk was patched by an unsupported program")

// Patch applies the Magisk ramdisk patch to stock, which must be the raw
// (already decompressed) cpio payload of a ramdisk image.
//
// The sequence mirrors Magisk's boot_patch.sh: an existing patch is restored
// first, the payload is added under /init and overlay.d/sbin, fstab options are
// stripped if requested, and finally the difference against the stock archive
// is recorded so it can be reverted.
func Patch(stock []byte, p *Payload, opts Options) (*Result, error) {
	a, err := cpio.Load(stock)
	if err != nil {
		return nil, err
	}

	res := &Result{Archive: a, PreviousStatus: a.Test(), Options: opts}
	if res.PreviousStatus == cpio.StatusUnsupported {
		return nil, ErrUnsupported
	}

	// Inherit settings recorded by a previous patch that the caller did not
	// override, so re-patching keeps a working PREINITDEVICE.
	if res.PreviousStatus == cpio.StatusMagisk {
		if cfg, ok := a.Get(".backup/.magisk"); ok {
			prev := ParseConfig(cfg.Data)
			if res.Options.PreinitDevice == "" {
				res.Options.PreinitDevice = prev["PREINITDEVICE"]
			}
			if res.Options.SHA1 == "" {
				res.Options.SHA1 = prev["SHA1"]
			}
		}
		// Revert to stock so the backup step compares against a clean base.
		if err := a.Restore(); err != nil {
			return nil, fmt.Errorf("restoring previous patch: %w", err)
		}
	}
	orig := a.Clone()

	// The payload is stored compressed inside the ramdisk to keep it small;
	// magiskinit decompresses it at boot.
	magiskXZ, err := imgfmt.Compress(imgfmt.XZ, p.Magisk)
	if err != nil {
		return nil, fmt.Errorf("compressing magisk: %w", err)
	}
	stubXZ, err := imgfmt.Compress(imgfmt.XZ, p.Stub)
	if err != nil {
		return nil, fmt.Errorf("compressing stub apk: %w", err)
	}
	initLDXZ, err := imgfmt.Compress(imgfmt.XZ, p.InitLD)
	if err != nil {
		return nil, fmt.Errorf("compressing init-ld: %w", err)
	}

	a.AddFile(0o750, "init", p.MagiskInit)
	a.Mkdir(0o750, "overlay.d")
	a.Mkdir(0o750, "overlay.d/sbin")
	a.AddFile(0o644, "overlay.d/sbin/magisk.xz", magiskXZ)
	a.AddFile(0o644, "overlay.d/sbin/stub.xz", stubXZ)
	a.AddFile(0o644, "overlay.d/sbin/init-ld.xz", initLDXZ)

	a.ApplyPatch(res.Options.KeepVerity, res.Options.KeepForceEncrypt)

	backedUp, err := a.Backup(orig, false)
	if err != nil {
		return nil, fmt.Errorf("recording backup: %w", err)
	}
	a.Mkdir(0, ".backup")
	a.AddFile(0, ".backup/.magisk", res.Options.Config())

	// Report what changed, for the user's benefit.
	before := map[string]bool{}
	for _, n := range orig.Names() {
		before[n] = true
	}
	for _, n := range a.Names() {
		if !before[n] {
			res.Added = append(res.Added, n)
		}
	}
	res.BackedUp = backedUp
	sort.Strings(res.Added)

	return res, nil
}
