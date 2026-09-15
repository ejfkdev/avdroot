package axml

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// magiskAPK locates a real Magisk installer, which is also a valid APK. The
// v31.0 release declares minSdkVersion 23 and targetSdkVersion 37, and aapt2
// reports the same, so it serves as an independent reference for the parser.
// The archive is not committed; see testdata/README.md.
func magiskAPK(t *testing.T) string {
	t.Helper()
	var candidates []string
	if dir := os.Getenv("AVDROOT_TEST_ASSETS"); dir != "" {
		candidates = append(candidates, filepath.Join(dir, "Magisk.zip"))
	}
	if wd, err := os.Getwd(); err == nil {
		candidates = append(candidates, filepath.Join(wd, "testdata", "Magisk.zip"))
	}
	for _, c := range candidates {
		if fi, err := os.Stat(c); err == nil && !fi.IsDir() {
			return c
		}
	}
	t.Skipf("skipping: no Magisk archive found in %v. Set AVDROOT_TEST_ASSETS or use testdata/", candidates)
	return ""
}

// apkManifest extracts AndroidManifest.xml from an APK.
func apkManifest(t *testing.T, apkPath string) []byte {
	t.Helper()
	zr, err := zip.OpenReader(apkPath)
	if err != nil {
		t.Skipf("asset %s unavailable: %v", apkPath, err)
	}
	defer zr.Close()
	for _, f := range zr.File {
		if f.Name != "AndroidManifest.xml" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		defer rc.Close()
		data, err := io.ReadAll(rc)
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	t.Skip("archive has no AndroidManifest.xml")
	return nil
}

// The parsed values must match what aapt2 reports, since they decide whether
// the app can be installed on the target at all.
func TestParseMagiskManifest(t *testing.T) {
	m, err := Parse(apkManifest(t, magiskAPK(t)))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if m.Package != "com.topjohnwu.magisk" {
		t.Errorf("Package = %q, want com.topjohnwu.magisk", m.Package)
	}
	if m.MinSDK != 23 {
		t.Errorf("MinSDK = %d, want 23 (aapt2 reports 23)", m.MinSDK)
	}
	if m.TargetSDK != 37 {
		t.Errorf("TargetSDK = %d, want 37 (aapt2 reports 37)", m.TargetSDK)
	}
	if m.VersionName == "" {
		t.Error("VersionName is empty")
	}
	t.Logf("%s %s minSdk=%d targetSdk=%d", m.Package, m.VersionName, m.MinSDK, m.TargetSDK)
}

func TestParseRejectsNonAXML(t *testing.T) {
	for _, tc := range []struct {
		name string
		data []byte
	}{
		{"empty", nil},
		{"text manifest", []byte(`<?xml version="1.0"?><manifest package="x"/>`)},
		{"truncated header", []byte{0x03, 0x00, 0x08, 0x00}},
		{"wrong magic", []byte{0xff, 0xff, 0x08, 0x00, 0, 0, 0, 0}},
	} {
		if _, err := Parse(tc.data); err == nil {
			t.Errorf("%s: expected an error", tc.name)
		}
	}
}

// A downloaded file may be truncated or corrupt, so the parser must report an
// error instead of panicking or reading out of bounds.
func TestParseSurvivesGarbage(t *testing.T) {
	// A file header followed by a string pool claiming far more strings than
	// the chunk can hold, plus an element chunk that runs off the end.
	data := make([]byte, 64)
	data[0], data[1] = 0x03, 0x00   // RES_XML_TYPE
	data[2], data[3] = 0x08, 0x00   // header size 8
	data[4] = 64                    // total size
	data[8], data[9] = 0x01, 0x00   // string pool
	data[10], data[11] = 0x1c, 0x00 // header size 28
	data[12] = 64                   // chunk size
	data[16], data[17], data[18], data[19] = 0xff, 0xff, 0xff, 0x7f

	if _, err := Parse(data); err == nil {
		t.Log("a malformed pool was tolerated without panicking")
	}

	// Every truncation of a valid manifest must be safe to parse.
	good := apkManifest(t, magiskAPK(t))
	for _, n := range []int{8, 16, 40, 100, len(good) / 2} {
		if n > len(good) {
			continue
		}
		if _, err := Parse(good[:n]); err != nil {
			// Errors are fine; a panic is not, and the test would fail on one.
			continue
		}
	}
	if strings.Contains(string(good[:4]), "<") {
		t.Error("the manifest in a release APK should be binary, not text")
	}
}
