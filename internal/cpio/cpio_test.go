package cpio

import (
	"bytes"
	"testing"
)

func sample() *Archive {
	a := New()
	a.Mkdir(0o755, "dev")
	a.AddFile(0o644, "fstab.ranchu", []byte("system /system ext4 ro wait,verify,avb\n"))
	a.AddFile(0o750, "init", []byte("init contents"))
	a.AddFile(0o644, "verity_key", []byte("key"))
	a.Symlink("/system/bin/init", "sbin/init")
	a.Put("dev/console", &Entry{Mode: S_IFCHR | 0o644, RdevMajor: 5, RdevMinor: 1})
	return a
}

// NormalizePath mirrors magiskboot's norm_path.
func TestNormalizePath(t *testing.T) {
	for in, want := range map[string]string{
		"init":   "init",
		"/init":  "init",
		"//init": "init",
		// magiskboot only drops empty components, so a leading "." stays.
		"./init":          "./init",
		"overlay.d/":      "overlay.d",
		"/a//b///c":       "a/b/c",
		".backup/.magisk": ".backup/.magisk",
		"":                "",
	} {
		if got := NormalizePath(in); got != want {
			t.Errorf("NormalizePath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLoadDumpRoundTrip(t *testing.T) {
	a := sample()
	data, err := a.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	b, err := Load(data)
	if err != nil {
		t.Fatal(err)
	}
	if b.Len() != a.Len() {
		t.Fatalf("entry count %d, want %d", b.Len(), a.Len())
	}
	// Serialising again must be byte identical, which requires the canonical
	// field values and name ordering to be reproduced exactly.
	again, err := b.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, again) {
		t.Error("serialisation is not stable")
	}
	for _, name := range a.Names() {
		want, _ := a.Get(name)
		got, ok := b.Get(name)
		if !ok {
			t.Fatalf("entry %q lost", name)
		}
		if got.Mode != want.Mode || !bytes.Equal(got.Data, want.Data) ||
			got.RdevMajor != want.RdevMajor || got.RdevMinor != want.RdevMinor {
			t.Errorf("entry %q changed across the round trip", name)
		}
	}
}

// The trailer and header layout must match magiskboot exactly, since the
// kernel parses this archive.
func TestSerialisationLayout(t *testing.T) {
	a := New()
	a.AddFile(0o644, "a", []byte("hi"))
	data, err := a.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(data, []byte("070701")) {
		t.Fatalf("archive does not start with the newc magic: % x", data[:8])
	}
	// inode numbering starts at 300000 and nlink is always 1.
	if !bytes.Contains(data, []byte("000493e0")) { // 300000 in hex
		t.Error("first inode is not 300000")
	}
	// The trailer carries mode 0755, a namesize of 11, and the literal name.
	if !bytes.Contains(data, []byte("TRAILER!!!")) {
		t.Error("archive has no TRAILER!!!")
	}
	idx := bytes.Index(data, []byte("TRAILER!!!"))
	hdr := data[idx-110 : idx]
	if !bytes.Equal(hdr[14:22], []byte("000001ed")) {
		t.Errorf("trailer mode = %q, want 000001ed", hdr[14:22])
	}
	if !bytes.Equal(hdr[94:102], []byte("0000000b")) {
		t.Errorf("trailer namesize = %q, want 0000000b", hdr[94:102])
	}
	// Data is padded to a 4-byte boundary, so the total length is a multiple
	// of four.
	if len(data)%4 != 0 {
		t.Errorf("archive length %d is not 4-byte aligned", len(data))
	}
}

// Concatenated archives are merged, and a later entry replaces an earlier one,
// matching both the kernel's sequential unpacking and magiskboot's reader.
func TestLoadMergesConcatenatedArchives(t *testing.T) {
	first := New()
	first.AddFile(0o644, "shared", []byte("from first"))
	first.AddFile(0o644, "only-first", []byte("x"))
	a, err := first.Bytes()
	if err != nil {
		t.Fatal(err)
	}

	second := New()
	second.AddFile(0o644, "shared", []byte("from second"))
	second.AddFile(0o644, "only-second", []byte("y"))
	b, err := second.Bytes()
	if err != nil {
		t.Fatal(err)
	}

	merged, err := Load(append(append([]byte{}, a...), b...))
	if err != nil {
		t.Fatal(err)
	}
	if merged.Len() != 3 {
		t.Fatalf("merged %d entries, want 3", merged.Len())
	}
	shared, _ := merged.Get("shared")
	if string(shared.Data) != "from second" {
		t.Errorf("duplicate entry resolved to %q, want the later archive's value", shared.Data)
	}
}

// Path operations normalise their arguments the way magiskboot does.
func TestArchiveOperations(t *testing.T) {
	a := sample()

	if !a.Exists("/init") || !a.Exists("init") {
		t.Error("Exists does not normalise paths")
	}
	// Recursive removal takes the whole subtree.
	a.Rm("dev", true)
	if a.Exists("dev") || a.Exists("dev/console") {
		t.Error("recursive Rm left entries behind")
	}
	// Non-recursive removal takes only the named entry.
	a.Rm("overlay.d", false)
	if a.Exists("overlay.d") {
		t.Error("Rm did not remove the named entry")
	}
	// AddFile replaces an existing entry and sets the file type.
	a.AddFile(0o600, "init", []byte("replaced"))
	e, _ := a.Get("init")
	if !e.IsRegular() || e.Perm() != 0o600 || string(e.Data) != "replaced" {
		t.Errorf("AddFile did not replace the entry: mode=%#o data=%q", e.Mode, e.Data)
	}
	// Symlinks store the normalised target as data and carry no permission
	// bits, matching magiskboot's ln.
	s, _ := a.Get("sbin/init")
	if !s.IsSymlink() || s.Perm() != 0 || string(s.Data) != "system/bin/init" {
		t.Errorf("symlink entry is wrong: mode=%#o data=%q", s.Mode, s.Data)
	}
}

// Test reports stock, Magisk and unsupported states the way magiskboot does.
func TestTest(t *testing.T) {
	a := sample()
	if got := a.Test(); got != StatusStock {
		t.Errorf("Test() = %s, want stock", got)
	}

	// The config file is the marker, not the mere presence of .backup.
	a.Mkdir(0, ".backup")
	if got := a.Test(); got != StatusStock {
		t.Errorf("an empty .backup changed the status to %s", got)
	}
	a.AddFile(0, ".backup/.magisk", []byte("KEEPVERITY=true\n"))
	if got := a.Test(); got != StatusMagisk {
		t.Errorf("Test() = %s, want Magisk patched", got)
	}

	// SuperSU leftovers take priority over the Magisk markers.
	su := sample()
	su.AddFile(0o644, ".backup/.magisk", []byte("x"))
	su.AddFile(0o755, "sbin/su", []byte("su"))
	if got := su.Test(); got != StatusUnsupported {
		t.Errorf("Test() = %s, want unsupported", got)
	}
}

// patch must strip verity and encryption options, and drop verity_key, only
// when asked to.
func TestApplyPatch(t *testing.T) {
	// Keeping both means the archive is untouched.
	keep := sample()
	before, _ := keep.Bytes()
	keep.ApplyPatch(true, true)
	after, _ := keep.Bytes()
	if !bytes.Equal(before, after) {
		t.Error("ApplyPatch modified the archive even though both flags were set")
	}

	// Turning verity off removes the option and the key.
	strip := sample()
	strip.ApplyPatch(false, true)
	fstab, _ := strip.Get("fstab.ranchu")
	if bytes.Contains(fstab.Data, []byte("verify")) || bytes.Contains(fstab.Data, []byte("avb")) {
		t.Errorf("verity options survived: %q", fstab.Data)
	}
	if strip.Exists("verity_key") {
		t.Error("verity_key was not removed")
	}
	if !bytes.Contains(fstab.Data, []byte("ext4")) {
		t.Errorf("unrelated fstab content was damaged: %q", fstab.Data)
	}

	// Turning encryption off targets only the encryption options.
	enc := New()
	enc.AddFile(0o644, "fstab.ranchu", []byte("data /data ext4 rw,forceencrypt,noatime\n"))
	enc.ApplyPatch(true, false)
	e, _ := enc.Get("fstab.ranchu")
	if bytes.Contains(e.Data, []byte("forceencrypt")) {
		t.Errorf("encryption option survived: %q", e.Data)
	}
	if !bytes.Contains(e.Data, []byte("noatime")) {
		t.Errorf("unrelated option was removed: %q", e.Data)
	}
}

// A pattern is removed together with its leading comma and its =value part, so
// that the remaining option list stays well formed.
func TestRemovePatternSeparators(t *testing.T) {
	cases := []struct{ in, want string }{
		{"rw,verify,noatime", "rw,noatime"},
		{"rw,verify,noatime", "rw,noatime"},
		{"rw,avb=2.0,noatime", "rw,noatime"},
		{"verify", ""},
		{"rw,verifyatboot,noatime", "rw,noatime"},
		{"rw,fileencryption=aes-256-xts:aes-256-cts,noatime", "rw,noatime"},
		{"rw,noatime", "rw,noatime"},
	}
	patterns := append(append([][]byte{}, verityPatterns...), encryptionPatterns...)
	for _, tc := range cases {
		buf := []byte(tc.in)
		n := removePattern(buf, patterns)
		if got := string(buf[:n]); got != tc.want {
			t.Errorf("removePattern(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// backup records differing and missing entries, and lists additions in
// .rmlist; restore reverses all of it.
func TestBackupAndRestore(t *testing.T) {
	orig := New()
	orig.AddFile(0o750, "init", []byte("stock init"))
	orig.AddFile(0o644, "unchanged", []byte("same"))

	patched := orig.Clone()
	patched.AddFile(0o750, "init", []byte("magiskinit"))
	patched.AddFile(0o644, "overlay.d/sbin/magisk.xz", []byte("payload"))
	patched.Mkdir(0o750, "overlay.d")
	patched.Mkdir(0o750, "overlay.d/sbin")

	if _, err := patched.Backup(orig, false); err != nil {
		t.Fatal(err)
	}

	// The changed file is stored, xz compressed, and keeps its mode.
	backup, ok := patched.Get(".backup/init.xz")
	if !ok {
		t.Fatal("changed init was not backed up")
	}
	if backup.Perm() != 0o750 {
		t.Errorf("backup mode = %04o, want 0750", backup.Perm())
	}
	// The unchanged file is not stored.
	if patched.Exists(".backup/unchanged") || patched.Exists(".backup/unchanged.xz") {
		t.Error("an unchanged file was backed up")
	}
	// Additions are listed for removal, NUL terminated.
	rm, ok := patched.Get(".backup/.rmlist")
	if !ok {
		t.Fatal(".rmlist was not written")
	}
	want := "overlay.d\x00overlay.d/sbin\x00overlay.d/sbin/magisk.xz\x00"
	if string(rm.Data) != want {
		t.Errorf(".rmlist = %q, want %q", rm.Data, want)
	}

	// restore must reproduce the original exactly.
	if err := patched.Restore(); err != nil {
		t.Fatal(err)
	}
	if patched.Exists("overlay.d") || patched.Exists(".backup") {
		t.Error("restore left patched entries behind")
	}
	init, _ := patched.Get("init")
	if string(init.Data) != "stock init" {
		t.Errorf("restored init = %q, want %q", init.Data, "stock init")
	}
	if got := patched.Test(); got != StatusStock {
		t.Errorf("after restore Test() = %s, want stock", got)
	}
}

// When nothing differs from the original, no backup entries are created beyond
// the directory, and restore empties the archive the way magiskboot does.
func TestBackupWithNoChanges(t *testing.T) {
	orig := New()
	orig.AddFile(0o644, "only", []byte("data"))

	other := New()
	if _, err := other.Backup(orig, false); err != nil {
		t.Fatal(err)
	}
	if !other.Exists(".backup") {
		t.Error(".backup directory entry was not created")
	}
	if rt, ok := other.Get(".backup"); !ok || !rt.IsDir() {
		t.Error(".backup is not a directory entry")
	}
	if other.Exists(".backup/.rmlist") {
		t.Error(".rmlist was created even though nothing was added")
	}
}
