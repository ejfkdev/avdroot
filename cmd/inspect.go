package cmd

import (
	"os"

	"github.com/ejfkdev/avdroot/internal/avd"
	"github.com/ejfkdev/avdroot/internal/cpio"
	"github.com/ejfkdev/avdroot/internal/i18n"
	"github.com/ejfkdev/avdroot/internal/imgfmt"
	"github.com/ejfkdev/avdroot/internal/ramdisk"
)

// targetInfo summarises a ramdisk without modifying it.
type targetInfo struct {
	Target    *avd.Target
	Status    cpio.Status
	Format    imgfmt.Format
	Entries   int
	Size      int64
	HasBackup bool
	// BackupStatus is only meaningful when HasBackup is true.
	BackupStatus cpio.Status
	// Errs collects non-fatal problems, so a listing can show every target
	// even when one of them is unreadable.
	Errs []error
}

// inspect gathers the state of a target's ramdisk.
func inspect(t *avd.Target) *targetInfo {
	info := &targetInfo{Target: t}
	if t.RamdiskPath == "" {
		info.Errs = append(info.Errs, os.ErrNotExist)
		return info
	}
	if fi, err := os.Stat(t.RamdiskPath); err == nil {
		info.Size = fi.Size()
	}
	payload, format, err := ramdisk.Load(t.RamdiskPath)
	if err != nil {
		info.Errs = append(info.Errs, err)
		return info
	}
	info.Format = format
	a, err := cpio.Load(payload)
	if err != nil {
		info.Errs = append(info.Errs, err)
		return info
	}
	info.Status = a.Test()
	info.Entries = a.Len()

	if _, err := os.Stat(ramdisk.BackupPath(t.RamdiskPath)); err == nil {
		info.HasBackup = true
		if st, _, err := ramdisk.BackupStatus(t.RamdiskPath); err == nil {
			info.BackupStatus = st
		} else {
			info.Errs = append(info.Errs, err)
		}
	}
	return info
}

// statusText renders the patch state compactly.
func statusText(s cpio.Status) string {
	switch s {
	case cpio.StatusStock:
		return i18n.T("status.stock")
	case cpio.StatusMagisk:
		return i18n.T("status.magisk")
	default:
		return i18n.T("status.unsupported")
	}
}

// flavor returns a short human description of an API level / release.
func flavor(t *avd.Target) string {
	switch {
	case t.API > 0 && t.Release != "":
		return i18n.T("flavor.api_and_release", t.API, t.Release)
	case t.API > 0:
		return i18n.T("flavor.api", t.API)
	case t.Release != "":
		return i18n.T("flavor.release", t.Release)
	default:
		return i18n.T("flavor.unknown")
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
