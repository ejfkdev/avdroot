package cpio

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ejfkdev/avdroot/internal/imgfmt"
)

// These tests run against real ramdisks from an Android Studio system image. The
// assets are not committed, so they are skipped when absent and the suite still
// passes on a machine with no SDK; see testdata/README.md for what to provide.
//
// assetPath resolves an asset from $AVDROOT_TEST_ASSETS, or from a testdata
// directory beside this package.
func assetPath(t *testing.T, name string) string {
	t.Helper()
	var candidates []string
	if dir := os.Getenv("AVDROOT_TEST_ASSETS"); dir != "" {
		candidates = append(candidates, filepath.Join(dir, name))
	}
	if wd, err := os.Getwd(); err == nil {
		candidates = append(candidates, filepath.Join(wd, "testdata", name))
	}
	for _, c := range candidates {
		if fi, err := os.Stat(c); err == nil && !fi.IsDir() {
			return c
		}
	}
	t.Skipf("skipping: %s not found. Set AVDROOT_TEST_ASSETS to a directory holding the test assets, "+
		"or drop the file in testdata/ beside this package (see testdata/README.md)", name)
	return ""
}

func stockAssetPath(t *testing.T) string   { return assetPath(t, "ramdisk.stock.img") }
func patchedAssetPath(t *testing.T) string { return assetPath(t, "ramdisk.patched.img") }

func loadReal(t *testing.T, path string) (*Archive, imgfmt.Format) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Skipf("asset %s not available", path)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	plain, format, err := imgfmt.Decompress(raw)
	if err != nil {
		t.Fatalf("decompress %s: %v", path, err)
	}
	a, err := Load(plain)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return a, format
}

// The stock ramdisk from the SDK is an lz4 legacy stream holding two
// concatenated cpio archives; both must be merged and the result must test as
// stock.
func TestLoadStockRamdisk(t *testing.T) {
	a, format := loadReal(t, stockAssetPath(t))
	if format != imgfmt.LZ4Legacy {
		t.Errorf("format = %s, want lz4_legacy", format)
	}
	if got := a.Test(); got != StatusStock {
		t.Errorf("Test() = %s, want stock", got)
	}
	for _, name := range []string{"init", "lib/modules/modules.load", "first_stage_ramdisk/fstab.ranchu"} {
		if !a.Exists(name) {
			t.Errorf("stock archive missing %q", name)
		}
	}
	t.Logf("stock: %d entries, format %s", a.Len(), format)
}

// The ramdisk on disk was patched by the reference installer; it must be
// recognised as Magisk patched and carry the expected payload.
func TestLoadPatchedRamdisk(t *testing.T) {
	a, _ := loadReal(t, patchedAssetPath(t))
	if got := a.Test(); got != StatusMagisk {
		t.Fatalf("Test() = %s, want Magisk patched", got)
	}
	for _, name := range []string{
		"init",
		"overlay.d/sbin/magisk.xz",
		"overlay.d/sbin/stub.xz",
		"overlay.d/sbin/init-ld.xz",
		".backup/.magisk",
		".backup/init.xz",
		".backup/.rmlist",
	} {
		if !a.Exists(name) {
			t.Errorf("patched archive missing %q", name)
		}
	}
	cfg, _ := a.Get(".backup/.magisk")
	t.Logf("config: %q", string(cfg.Data))
	rm, _ := a.Get(".backup/.rmlist")
	t.Logf("rmlist: %q", string(rm.Data))
}

// Serialising and re-parsing must be lossless, and the serialisation must be
// stable so that repeated runs produce identical ramdisks.
func TestDumpRoundTrip(t *testing.T) {
	a, _ := loadReal(t, patchedAssetPath(t))

	first, err := a.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	reloaded, err := Load(first)
	if err != nil {
		t.Fatal(err)
	}
	second, err := reloaded.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatal("serialisation is not stable across load/dump")
	}
	if reloaded.Len() != a.Len() {
		t.Fatalf("entry count changed: %d -> %d", a.Len(), reloaded.Len())
	}
	for _, name := range a.Names() {
		want, _ := a.Get(name)
		got, ok := reloaded.Get(name)
		if !ok {
			t.Fatalf("entry %q lost", name)
		}
		if want.Mode != got.Mode || string(want.Data) != string(got.Data) {
			t.Fatalf("entry %q changed", name)
		}
	}
	t.Logf("serialised %d entries, %d bytes", a.Len(), len(first))
}

// Restoring the reference patch must bring back the stock init binary and drop
// every file the patch added.
func TestRestoreReferencePatch(t *testing.T) {
	a, _ := loadReal(t, patchedAssetPath(t))
	originalInit, _ := a.Get(".backup/init.xz")

	packed, _ := a.Get(".backup/init.xz")
	_ = packed
	plainInit, err := imgfmt.DecompressAs(imgfmt.XZ, originalInit.Data)
	if err != nil {
		t.Fatalf("decompress .backup/init.xz: %v", err)
	}

	if err := a.Restore(); err != nil {
		t.Fatal(err)
	}
	if got := a.Test(); got != StatusStock {
		t.Errorf("after restore Test() = %s, want stock", got)
	}
	init, ok := a.Get("init")
	if !ok {
		t.Fatal("restore did not bring back init")
	}
	if string(init.Data) != string(plainInit) {
		t.Error("restored init differs from the backed up original")
	}
	for _, name := range []string{"overlay.d", "overlay.d/sbin", "overlay.d/sbin/magisk.xz", ".backup"} {
		if a.Exists(name) {
			t.Errorf("restore left %q behind", name)
		}
	}
}
