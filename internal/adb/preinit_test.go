package adb

import "testing"

// mountinfo captured from a running Android 17 (API 37) arm64 AVD, which is
// the configuration the preinit detection has to get right.
const realMountInfo = `
22 28 0:21 / /sys rw,nosuid,nodev,noexec,relatime shared:7 - sysfs sysfs rw,seclabel
28 29 253:49 / /metadata rw,nosuid,nodev,noatime shared:8 - ext4 /dev/block/vdd1 rw,seclabel
161 29 254:5 / /data rw,nosuid,nodev,noatime shared:36 - ext4 /dev/block/dm-5 rw,seclabel,resgid=1065,errors=panic
171 161 254:5 /data /data/user/0 rw,nosuid,nodev,noatime shared:36 - ext4 /dev/block/dm-5 rw,seclabel,resgid=1065,errors=panic
30 29 8:1 / /mnt/foo\040bar rw,relatime shared:9 - ext4 /dev/block/vda rw
`

func TestPreinitDeviceDetection(t *testing.T) {
	mounts := parseMountInfo(realMountInfo)
	if len(mounts) != 5 {
		t.Fatalf("parsed %d mounts, want 5", len(mounts))
	}

	// The AVD reports encrypted + file + metadata enabled.
	enc := encryptTypeFromProps("encrypted", "file", "true")
	if enc != encMetadata {
		t.Errorf("encryptType = %d, want metadata", enc)
	}
	if got := findPreinitDevice(mounts, enc); got != "vdd1" {
		t.Errorf("findPreinitDevice = %q, want %q", got, "vdd1")
	}
}

func TestPreinitRejectsDeviceMapper(t *testing.T) {
	// Only the /data entry, which lives on device-mapper, is present. It must
	// be rejected, leaving no candidate.
	mounts := parseMountInfo(`161 29 254:5 / /data rw,noatime shared:36 - ext4 /dev/block/dm-5 rw`)
	enc := encryptTypeFromProps("encrypted", "file", "false")
	if got := findPreinitDevice(mounts, enc); got != "" {
		t.Errorf("findPreinitDevice = %q, want no candidate", got)
	}
}

// With no metadata partition, an unencrypted /data should be chosen, but a
// block-encrypted one must not be.
func TestPreinitDataOnlyWhenUnencrypted(t *testing.T) {
	const mi = `161 29 8:1 / /data rw,noatime shared:36 - ext4 /dev/block/vdb rw`
	mounts := parseMountInfo(mi)

	if got := findPreinitDevice(mounts, encNone); got != "vdb" {
		t.Errorf("unencrypted: got %q, want vdb", got)
	}
	if got := findPreinitDevice(mounts, encBlock); got != "" {
		t.Errorf("block encrypted: got %q, want no candidate", got)
	}
}

// PartId ordering puts data first and metadata last (metadata space is
// limited), so when both are eligible data wins.
func TestPreinitPrefersDataOverMetadata(t *testing.T) {
	const mi = `
161 29 8:1 / /data rw,noatime shared:36 - ext4 /dev/block/vdb rw
28 29 8:2 / /metadata rw,noatime shared:8 - ext4 /dev/block/vdc rw
`
	got := findPreinitDevice(parseMountInfo(mi), encNone)
	if got != "vdb" {
		t.Errorf("got %q, want vdb (data outranks metadata)", got)
	}
}

// metadata still wins when the other candidate is f2fs, because f2fs has a
// kernel bug that metadata is not affected by.
func TestPreinitMetadataPreferredOverF2fsData(t *testing.T) {
	const mi = `
161 29 8:1 / /data rw,noatime shared:36 - f2fs /dev/block/vdb rw
28 29 8:2 / /metadata rw,noatime shared:8 - ext4 /dev/block/vdc rw
`
	got := findPreinitDevice(parseMountInfo(mi), encNone)
	if got != "vdc" {
		t.Errorf("got %q, want vdc (metadata)", got)
	}
}

// ext4 must win over f2fs because of the kernel bug Magisk works around.
func TestPreinitPrefersExt4OverF2fs(t *testing.T) {
	const mi = `
28 29 253:49 / /metadata rw,noatime shared:8 - f2fs /dev/block/vdd1 rw
161 29 8:1 / /data rw,noatime shared:36 - ext4 /dev/block/vdb rw
`
	got := findPreinitDevice(parseMountInfo(mi), encNone)
	if got != "vdb" {
		t.Errorf("got %q, want vdb (ext4 beats f2fs)", got)
	}
}

func TestPreinitRejectsNonRwAndBadFSType(t *testing.T) {
	const mi = `
28 29 8:2 / /metadata ro,noatime shared:8 - ext4 /dev/block/vdc ro
161 29 8:1 / /data rw,noatime shared:36 - vfat /dev/block/vdb rw
`
	if got := findPreinitDevice(parseMountInfo(mi), encNone); got != "" {
		t.Errorf("got %q, want no candidate", got)
	}
}

func TestParseMountInfoUnescapes(t *testing.T) {
	mounts := parseMountInfo(`30 29 8:1 / /mnt/foo\040bar rw,relatime shared:9 - ext4 /dev/block/vda rw`)
	if len(mounts) != 1 {
		t.Fatalf("parsed %d mounts, want 1", len(mounts))
	}
	if mounts[0].target != "/mnt/foo bar" {
		t.Errorf("target = %q, want %q", mounts[0].target, "/mnt/foo bar")
	}
	if mounts[0].major != 8 {
		t.Errorf("major = %d, want 8", mounts[0].major)
	}
}
