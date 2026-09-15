package adb

import (
	"path"
	"strconv"
	"strings"
)

// This file ports find_preinit_device from Magisk's native/src/core/mount.rs.
// The preinit device is the block device Magisk stores its preinit data on; the
// value has to be baked into the ramdisk at patch time, so it must be derived
// exactly the way magiskinit would derive it.

// encryptType mirrors Magisk's EncryptType.
type encryptType int

const (
	encNone encryptType = iota
	encBlock
	encFile
	encMetadata
)

// partID orders candidate partitions by preference, lowest first. Magisk logs
// note that metadata has limited space, persist is dangerous to write, and data
// is preferred when it is not block encrypted.
type partID int

const (
	partData partID = iota
	partCache
	partKlogdump
	partMetadata
	partPersist
)

// Dynamic device-mapper major range; sources in this range are rejected unless
// they are virtio disks or by-name links.
const (
	dynamicMajorMin = 240
	dynamicMajorMax = 254
)

type mountInfo struct {
	major   uint32
	root    string
	target  string
	options string
	fsType  string
	source  string
}

// unescapeMountField decodes the octal escapes mountinfo uses for whitespace.
func unescapeMountField(s string) string {
	if !strings.ContainsRune(s, '\\') {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+3 < len(s) {
			if n, err := strconv.ParseUint(s[i+1:i+4], 8, 16); err == nil {
				b.WriteByte(byte(n))
				i += 3
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// parseMountInfo reads the content of /proc/self/mountinfo.
//
// Format:
//
//	mountID parentID major:minor root mountpoint options [optional...] - fstype source superoptions
func parseMountInfo(text string) []mountInfo {
	var out []mountInfo
	for _, line := range strings.Split(text, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 7 {
			continue
		}
		// Locate the separator that introduces fstype/source.
		sep := -1
		for i := 6; i < len(fields); i++ {
			if fields[i] == "-" {
				sep = i
				break
			}
		}
		if sep < 0 || sep+2 >= len(fields) {
			continue
		}
		dev := fields[2]
		majorStr, _, ok := strings.Cut(dev, ":")
		if !ok {
			continue
		}
		major, err := strconv.ParseUint(majorStr, 10, 32)
		if err != nil {
			continue
		}
		out = append(out, mountInfo{
			major:   uint32(major),
			root:    unescapeMountField(fields[3]),
			target:  unescapeMountField(fields[4]),
			options: fields[5],
			fsType:  fields[sep+1],
			source:  unescapeMountField(fields[sep+2]),
		})
	}
	return out
}

// encryptTypeFromProps mirrors the property checks at the top of
// find_preinit_device.
func encryptTypeFromProps(cryptoState, cryptoType, metadataEnabled string) encryptType {
	switch {
	case cryptoState != "encrypted":
		return encNone
	case cryptoType == "block":
		return encBlock
	case metadataEnabled == "true":
		return encMetadata
	default:
		return encFile
	}
}

// preinitPartID maps a mount point onto a candidate partition.
func preinitPartID(m mountInfo, enc encryptType) (partID, bool) {
	switch m.target {
	case "/persist", "/mnt/vendor/persist":
		return partPersist, true
	case "/metadata":
		return partMetadata, true
	case "/klogdump":
		return partKlogdump, true
	case "/cache":
		return partCache, true
	case "/data":
		// Data is only usable if it is not encrypted, or encrypted per file
		// without metadata encryption.
		if enc == encNone || enc == encFile {
			return partData, true
		}
		return 0, false
	default:
		return 0, false
	}
}

// usableForPreinit applies the filter that decides whether a mount can host the
// preinit data at all.
func usableForPreinit(m mountInfo) bool {
	if m.root != "/" {
		return false
	}
	if !strings.HasPrefix(m.source, "/") || strings.Contains(m.source, "/dm-") {
		return false
	}
	if m.fsType != "ext4" && m.fsType != "f2fs" {
		return false
	}
	if !hasMountOption(m.options, "rw") {
		return false
	}
	parent := path.Dir(m.source)
	if !strings.HasSuffix(parent, "by-name") && !strings.HasSuffix(parent, "block") {
		return false
	}
	// Reject device-mapper devices by major number, but keep virtio disks and
	// by-name links, which legitimately live in the dynamic range.
	if m.major >= dynamicMajorMin && m.major <= dynamicMajorMax {
		if !strings.Contains(m.source, "/vd") && !strings.Contains(m.source, "/by-name/") {
			return false
		}
	}
	return true
}

func hasMountOption(options, want string) bool {
	for _, o := range strings.Split(options, ",") {
		if o == want {
			return true
		}
	}
	return false
}

// preferPreinit reports whether a should be chosen over b, mirroring Magisk's
// ordering: metadata is compared directly against ext4, ext4 beats f2fs (which
// has a kernel bug), otherwise the partition preference decides.
func preferPreinit(a, b mountInfo, ap, bp partID) bool {
	at, bt := a.fsType == "ext4", b.fsType == "ext4"
	if (ap == partMetadata && bt) || (bp == partMetadata && at) {
		return ap < bp
	}
	if at && !bt {
		return true
	}
	if !at && bt {
		return false
	}
	return ap < bp
}

// findPreinitDevice returns the basename of the block device to use, or "" when
// nothing suitable is mounted.
func findPreinitDevice(mounts []mountInfo, enc encryptType) string {
	var best *mountInfo
	var bestID partID

	for i := range mounts {
		m := mounts[i]
		if !usableForPreinit(m) {
			continue
		}
		id, ok := preinitPartID(m, enc)
		if !ok {
			continue
		}
		if best == nil || preferPreinit(m, *best, id, bestID) {
			best, bestID = &mounts[i], id
		}
	}
	if best == nil {
		return ""
	}
	return path.Base(best.source)
}

// PreinitDevice asks a running device for the preinit block device. It returns
// "" when the device is online but has no suitable partition mounted.
func (c *Client) PreinitDevice() (string, error) {
	cryptoState, _ := c.GetProp("ro.crypto.state")
	cryptoType, _ := c.GetProp("ro.crypto.type")
	metadataEnabled, _ := c.GetProp("ro.crypto.metadata.enabled")

	out, err := c.Shell("cat /proc/self/mountinfo")
	if err != nil {
		return "", err
	}
	enc := encryptTypeFromProps(cryptoState, cryptoType, metadataEnabled)
	return findPreinitDevice(parseMountInfo(out), enc), nil
}
