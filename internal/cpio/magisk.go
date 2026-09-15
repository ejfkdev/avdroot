package cpio

import (
	"bytes"
	"sort"
	"strings"

	"github.com/ejfkdev/avdroot/internal/imgfmt"
)

// Status is the result of probing a ramdisk for an existing patch, mirroring
// "magiskboot cpio <archive> test" from Magisk v31.
type Status int

const (
	// StatusStock means the ramdisk carries no known patch.
	StatusStock Status = 0
	// StatusMagisk means a Magisk patch was detected.
	StatusMagisk Status = 1
	// StatusUnsupported means SuperSU, Xposed or similar leftovers were found.
	StatusUnsupported Status = 2
)

func (s Status) String() string {
	switch s {
	case StatusStock:
		return "stock"
	case StatusMagisk:
		return "Magisk patched"
	case StatusUnsupported:
		return "patched by unsupported program"
	default:
		return "unknown"
	}
}

// unsupportedMarkers are checked first: their presence means the ramdisk was
// modified by something else and must not be patched blindly.
var unsupportedMarkers = []string{
	"sbin/launch_daemonsu.sh",
	"sbin/su",
	"init.xposed.rc",
	"boot/sbin/launch_daemonsu.sh",
}

// magiskMarkers identify a Magisk patched ramdisk. Note that the config file
// is the authoritative marker, not merely the presence of a .backup directory.
var magiskMarkers = []string{
	".backup/.magisk",
	"init.magisk.rc",
	"overlay/init.magisk.rc",
}

// Test reports whether the archive is stock, Magisk patched or unsupported.
func (a *Archive) Test() Status {
	for _, name := range unsupportedMarkers {
		if a.Exists(name) {
			return StatusUnsupported
		}
	}
	for _, name := range magiskMarkers {
		if a.Exists(name) {
			return StatusMagisk
		}
	}
	return StatusStock
}

// ApplyPatch strips dm-verity and forced-encryption options from fstab files
// when the corresponding "keep" flag is false, and removes verity_key. This is
// magiskboot's "cpio patch" command; the /init replacement is performed
// separately by the installer, see magisk.Patch.
func (a *Archive) ApplyPatch(keepVerity, keepForceEncrypt bool) {
	for name, e := range a.entries {
		fstab := (!keepVerity || !keepForceEncrypt) &&
			e.IsRegular() &&
			!strings.HasPrefix(name, ".backup") &&
			!strings.HasPrefix(name, "twrp") &&
			!strings.HasPrefix(name, "recovery") &&
			strings.HasPrefix(name, "fstab")

		if !keepVerity {
			if fstab {
				e.Data = e.Data[:PatchVerity(e.Data)]
			} else if name == "verity_key" {
				delete(a.entries, name)
				continue
			}
		}
		if !keepForceEncrypt && fstab {
			e.Data = e.Data[:PatchEncryption(e.Data)]
		}
	}
}

// Backup records the difference between this archive and origin into
// ./.backup, so the patch can be reverted later:
//
//   - entries present in origin but missing here, or whose contents changed,
//     are stored as .backup/<name> (xz compressed when possible)
//   - entries that exist only here are listed in .backup/.rmlist so that
//     restore knows to delete them
//
// It is a faithful port of magiskboot's "cpio backup ORIG". The returned slice
// names the entries that were backed up, for reporting.
func (a *Archive) Backup(origin *Archive, skipCompress bool) ([]string, error) {
	backups := map[string]*Entry{
		// The directory entry is always emitted, even when nothing is
		// backed up, and carries no permission bits.
		".backup": {Mode: S_IFDIR},
	}

	orig := origin.Clone()
	orig.Rm(".backup", true)
	a.Rm(".backup", true)

	left := orig.Names()
	right := a.Names()
	var rmList []byte
	var backedUp []string

	li, ri := 0, 0
	for li < len(left) || ri < len(right) {
		switch {
		case li < len(left) && ri < len(right):
			ln, rn := left[li], right[ri]
			switch strings.Compare(ln, rn) {
			case -1:
				backedUp = append(backedUp, a.stash(backups, ln, orig.entries[ln], skipCompress))
				li++
			case 1:
				rmList = append(append(rmList, rn...), 0)
				ri++
			default:
				if !bytes.Equal(a.entries[rn].Data, orig.entries[ln].Data) {
					backedUp = append(backedUp, a.stash(backups, ln, orig.entries[ln], skipCompress))
				}
				li++
				ri++
			}
		case li < len(left):
			backedUp = append(backedUp, a.stash(backups, left[li], orig.entries[left[li]], skipCompress))
			li++
		default:
			rmList = append(append(rmList, right[ri]...), 0)
			ri++
		}
	}

	if len(rmList) > 0 {
		backups[".backup/.rmlist"] = &Entry{Mode: S_IFREG, Data: rmList}
	}
	for name, e := range backups {
		a.entries[name] = e
	}
	sort.Strings(backedUp)
	return backedUp, nil
}

// stash stores one original entry, compressing regular files with xz unless
// skipCompress is set. The entry keeps its original metadata; only the data is
// replaced, exactly like magiskboot. It returns the name stored.
func (a *Archive) stash(backups map[string]*Entry, name string, e *Entry, skipCompress bool) string {
	b := e.clone()
	target := ".backup/" + name
	if !skipCompress && b.IsRegular() {
		if compressed, err := imgfmt.Compress(imgfmt.XZ, b.Data); err == nil {
			b.Data = compressed
			target += ".xz"
		}
	}
	backups[target] = b
	return name
}

// Restore reverts a Magisk patch using the data stored in .backup, mirroring
// magiskboot's "cpio restore". Backed up files are put back (decompressing as
// needed), entries listed in .rmlist are deleted, and the .backup tree itself
// is removed. If there is nothing to restore the archive is emptied, which is
// how an A-only ramdisk regains its "no ramdisk" state.
func (a *Archive) Restore() error {
	backups := map[string]*Entry{}
	var rmList []byte

	for name, e := range a.entries {
		if !strings.HasPrefix(name, ".backup/") {
			continue
		}
		switch name {
		case ".backup/.rmlist":
			rmList = append(rmList, e.Data...)
		case ".backup/.magisk":
			// Config is intentionally dropped, never restored.
		default:
			newName := name[len(".backup/"):]
			data := e.Data
			if strings.HasSuffix(newName, ".xz") && e.IsRegular() {
				if plain, err := imgfmt.DecompressAs(imgfmt.XZ, data); err == nil {
					data = plain
					newName = strings.TrimSuffix(newName, ".xz")
				}
			}
			restored := e.clone()
			restored.Data = data
			backups[newName] = restored
		}
		delete(a.entries, name)
	}

	a.Rm(".backup", false)

	if len(rmList) == 0 && len(backups) == 0 {
		a.entries = map[string]*Entry{}
		return nil
	}
	for _, rm := range bytes.Split(rmList, []byte{0}) {
		if len(rm) > 0 {
			a.Rm(string(rm), false)
		}
	}
	for name, e := range backups {
		a.entries[name] = e
	}
	return nil
}
