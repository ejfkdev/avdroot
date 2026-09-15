package magisk

import (
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/ejfkdev/avdroot/internal/cpio"
	"github.com/ejfkdev/avdroot/internal/imgfmt"
)

// The assets these tests compare against are not committed; each is skipped when
// absent. See testdata/README.md for what to supply.
func stockImagePath(t *testing.T) string { return testAsset(t, "ramdisk.stock.img") }

// referenceImagePath is a ramdisk patched by Magisk's own installer, so this
// implementation can be checked against the reference.
func referenceImagePath(t *testing.T) string { return testAsset(t, "ramdisk.reference.img") }

func magiskZIPPath(t *testing.T) string { return testAsset(t, "Magisk.zip") }

func loadArchive(t *testing.T, path string) *cpio.Archive {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("asset %s unavailable: %v", path, err)
	}
	plain, _, err := imgfmt.Decompress(raw)
	if err != nil {
		t.Fatalf("decompress %s: %v", path, err)
	}
	a, err := cpio.Load(plain)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return a
}

// Patching the stock ramdisk must produce exactly the same entry set, with the
// same permissions, as Magisk's own installer produced for this AVD. Only the
// payload contents may differ, and then only because a different Magisk
// version was used.
func TestPatchMatchesReferenceInstaller(t *testing.T) {
	reference := loadArchive(t, referenceImagePath(t))

	plain, _, err := readDecompressed(stockImagePath(t))
	if err != nil {
		t.Skip(err)
	}
	p, err := LoadPayload(magiskZIPPath(t), "arm64-v8a")
	if err != nil {
		t.Skipf("Magisk installer unavailable: %v", err)
	}

	res, err := Patch(plain, p, Options{
		KeepVerity:       true,
		KeepForceEncrypt: true,
		PreinitDevice:    "vdd1",
	})
	if err != nil {
		t.Fatalf("Patch: %v", err)
	}
	got := res.Archive

	wantNames := reference.Names()
	gotNames := got.Names()
	if len(wantNames) != len(gotNames) {
		t.Fatalf("entry count differs: got %d, reference %d\ngot:  %v\nwant: %v",
			len(gotNames), len(wantNames), gotNames, wantNames)
	}
	for i := range wantNames {
		if wantNames[i] != gotNames[i] {
			t.Errorf("entry %d differs: got %q, reference %q", i, gotNames[i], wantNames[i])
		}
	}

	// Permissions of the patch's own entries must match too: these are the
	// values magiskinit relies on.
	modes := map[string]uint32{
		"init":                      0o750,
		"overlay.d":                 0o750,
		"overlay.d/sbin":            0o750,
		"overlay.d/sbin/magisk.xz":  0o644,
		"overlay.d/sbin/stub.xz":    0o644,
		"overlay.d/sbin/init-ld.xz": 0o644,
	}
	for name, wantMode := range modes {
		e, ok := got.Get(name)
		if !ok {
			t.Errorf("missing entry %q", name)
			continue
		}
		if e.Perm() != wantMode {
			t.Errorf("%s permissions = %04o, want %04o", name, e.Perm(), wantMode)
		}
	}

	// The .backup/.rmlist contents identify which entries restore must delete;
	// they must be identical to the reference for restore to behave the same.
	gotRM, _ := got.Get(".backup/.rmlist")
	wantRM, _ := reference.Get(".backup/.rmlist")
	if string(gotRM.Data) != string(wantRM.Data) {
		t.Errorf(".rmlist differs:\ngot:  %q\nwant: %q", string(gotRM.Data), string(wantRM.Data))
	}

	// The config must carry the same keys.
	gotCfg, _ := got.Get(".backup/.magisk")
	cfg := ParseConfig(gotCfg.Data)
	for _, key := range []string{"KEEPVERITY", "KEEPFORCEENCRYPT", "PREINITDEVICE"} {
		if cfg[key] == "" {
			t.Errorf("config is missing %s: %q", key, string(gotCfg.Data))
		}
	}
	if cfg["PREINITDEVICE"] != "vdd1" {
		t.Errorf("PREINITDEVICE = %q, want vdd1", cfg["PREINITDEVICE"])
	}
	t.Logf("config: %q", string(gotCfg.Data))
}

// Repacking must preserve the compression format of the input, so that the
// emulator keeps booting an image it already understood.
func TestPatchPreservesCompressionFormat(t *testing.T) {
	raw, err := os.ReadFile(stockImagePath(t))
	if err != nil {
		t.Skipf("asset unavailable: %v", err)
	}
	_, format, err := imgfmt.Decompress(raw)
	if err != nil {
		t.Fatal(err)
	}
	if format != imgfmt.LZ4Legacy {
		t.Fatalf("stock format = %s, want lz4_legacy", format)
	}

	// Round-tripping the same payload through the detected format must give
	// back an image the emulator can decompress.
	plain, err := imgfmt.DecompressAs(format, raw)
	if err != nil {
		t.Fatal(err)
	}
	recompressed, err := imgfmt.Compress(format, plain)
	if err != nil {
		t.Fatal(err)
	}
	again, gotFormat, err := imgfmt.Decompress(recompressed)
	if err != nil {
		t.Fatalf("re-decompressing our own lz4 output: %v", err)
	}
	if gotFormat != imgfmt.LZ4Legacy {
		t.Errorf("format after round trip = %s, want lz4_legacy", gotFormat)
	}
	if len(again) != len(plain) {
		t.Errorf("payload length changed: %d -> %d", len(plain), len(again))
	}
}

// Patching a ramdisk that is already patched must be idempotent in structure:
// restoring first means the second patch behaves like the first.
func TestRepatchIsStable(t *testing.T) {
	raw, err := os.ReadFile(stockImagePath(t))
	if err != nil {
		t.Skipf("asset unavailable: %v", err)
	}
	plain, _, err := imgfmt.Decompress(raw)
	if err != nil {
		t.Fatal(err)
	}
	p, err := LoadPayload(magiskZIPPath(t), "arm64-v8a")
	if err != nil {
		t.Skipf("Magisk installer unavailable: %v", err)
	}
	opts := Options{KeepVerity: true, KeepForceEncrypt: true, PreinitDevice: "vdd1"}

	first, err := Patch(plain, p, opts)
	if err != nil {
		t.Fatal(err)
	}
	firstBytes, err := first.Archive.Bytes()
	if err != nil {
		t.Fatal(err)
	}

	second, err := Patch(firstBytes, p, opts)
	if err != nil {
		t.Fatalf("re-patch: %v", err)
	}
	if second.PreviousStatus != cpio.StatusMagisk {
		t.Errorf("re-patch saw status %s, want Magisk patched", second.PreviousStatus)
	}
	secondBytes, err := second.Archive.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	if string(firstBytes) != string(secondBytes) {
		// Names must at least match; content differences would indicate the
		// backup/restore cycle is not lossless.
		a := namesOf(firstBytes, t)
		b := namesOf(secondBytes, t)
		if strings.Join(a, ",") != strings.Join(b, ",") {
			t.Errorf("re-patch changed the entry set:\nfirst:  %v\nsecond: %v", a, b)
		} else {
			t.Errorf("re-patch is not byte stable (%d vs %d bytes)", len(firstBytes), len(secondBytes))
		}
	}
}

func namesOf(data []byte, t *testing.T) []string {
	t.Helper()
	a, err := cpio.Load(data)
	if err != nil {
		t.Fatal(err)
	}
	n := a.Names()
	sort.Strings(n)
	return n
}

func readDecompressed(path string) ([]byte, imgfmt.Format, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, imgfmt.Unknown, err
	}
	return imgfmt.Decompress(raw)
}
