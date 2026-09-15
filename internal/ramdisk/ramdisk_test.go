package ramdisk

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/ejfkdev/avdroot/internal/cpio"
	"github.com/ejfkdev/avdroot/internal/imgfmt"
)

// stockRamdisk builds a small but realistic ramdisk image: a cpio archive
// holding /init, compressed with the given format.
func stockRamdisk(t *testing.T, format imgfmt.Format) []byte {
	t.Helper()
	a := cpio.New()
	a.Mkdir(0o755, "dev")
	a.AddFile(0o750, "init", bytes.Repeat([]byte("init-binary"), 100))
	a.Mkdir(0o755, "overlay.d")
	payload, err := a.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := imgfmt.Compress(format, payload)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func writeRamdisk(t *testing.T, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ramdisk.img")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// Load must report the compression format so the image can be written back the
// way it was found.
func TestLoadReportsFormat(t *testing.T) {
	for _, format := range []imgfmt.Format{imgfmt.Gzip, imgfmt.LZ4Legacy, imgfmt.XZ} {
		path := writeRamdisk(t, stockRamdisk(t, format))
		_, got, err := Load(path)
		if err != nil {
			t.Fatalf("%s: %v", format, err)
		}
		if got != format {
			t.Errorf("Load reported %s, want %s", got, format)
		}
	}
}

// Save must be atomic and preserve the requested mode.
func TestSaveWritesAndPreservesMode(t *testing.T) {
	path := writeRamdisk(t, stockRamdisk(t, imgfmt.LZ4Legacy))
	payload, format, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := Save(path, payload, format, 0o600); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v, want 0600", fi.Mode().Perm())
	}
	// The rewritten image must still be readable and detect as the same format.
	if _, got, err := Load(path); err != nil || got != format {
		t.Errorf("after Save: format=%s err=%v, want %s", got, err, format)
	}
	// No temporary files may be left behind.
	entries, _ := os.ReadDir(filepath.Dir(path))
	for _, e := range entries {
		if e.Name() != filepath.Base(path) {
			t.Errorf("stray file left behind: %s", e.Name())
		}
	}
}

// A format that cannot be encoded must be refused rather than written.
func TestSaveRefusesUncompressedFormat(t *testing.T) {
	path := writeRamdisk(t, stockRamdisk(t, imgfmt.Gzip))
	if err := Save(path, []byte("data"), imgfmt.Unknown, 0o644); err == nil {
		t.Error("expected Save to refuse an unknown format")
	}
	if err := Save(path, []byte("data"), imgfmt.Lzop, 0o644); err == nil {
		t.Error("expected Save to refuse lzop")
	}
}

func TestStatus(t *testing.T) {
	path := writeRamdisk(t, stockRamdisk(t, imgfmt.LZ4Legacy))
	st, err := Status(path)
	if err != nil {
		t.Fatal(err)
	}
	if st != cpio.StatusStock {
		t.Errorf("Status = %s, want stock", st)
	}
}

// EnsureBackup must create the backup once and then leave it alone. The second
// run is the important case: overwriting it would replace the pristine image
// with an already patched one, which is exactly the bug that made rootAVD
// unable to restore.
func TestEnsureBackupNeverOverwrites(t *testing.T) {
	path := writeRamdisk(t, stockRamdisk(t, imgfmt.LZ4Legacy))
	pristine, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	created, err := EnsureBackup(path)
	if err != nil {
		t.Fatal(err)
	}
	if !created {
		t.Fatal("first call did not create a backup")
	}

	// Simulate a patch by overwriting the ramdisk with different content.
	patched := stockRamdisk(t, imgfmt.Gzip)
	if err := os.WriteFile(path, patched, 0o644); err != nil {
		t.Fatal(err)
	}

	created, err = EnsureBackup(path)
	if err != nil {
		t.Fatal(err)
	}
	if created {
		t.Error("second call reported creating a backup")
	}
	onDisk, err := os.ReadFile(BackupPath(path))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(onDisk, pristine) {
		t.Error("the backup was overwritten with the patched image")
	}
}

// Restore must put the pristine image back and keep the backup file.
func TestRestore(t *testing.T) {
	path := writeRamdisk(t, stockRamdisk(t, imgfmt.LZ4Legacy))
	pristine, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := EnsureBackup(path); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, stockRamdisk(t, imgfmt.Gzip), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := Restore(path); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, pristine) {
		t.Error("restore did not reproduce the pristine image")
	}
	if _, err := os.Stat(BackupPath(path)); err != nil {
		t.Errorf("restore removed the backup file: %v", err)
	}
}

// Restoring without a backup must fail with a clear error.
func TestRestoreWithoutBackup(t *testing.T) {
	path := writeRamdisk(t, stockRamdisk(t, imgfmt.Gzip))
	if err := Restore(path); err == nil {
		t.Error("expected an error when no backup exists")
	}
}

// BackupStatus reports the state of the pristine copy, so callers can warn when
// an earlier tool left a patched image in its place.
func TestBackupStatus(t *testing.T) {
	path := writeRamdisk(t, stockRamdisk(t, imgfmt.LZ4Legacy))
	if _, _, err := BackupStatus(path); err == nil {
		t.Error("expected an error when no backup exists")
	}
	if _, err := EnsureBackup(path); err != nil {
		t.Fatal(err)
	}
	st, dst, err := BackupStatus(path)
	if err != nil {
		t.Fatal(err)
	}
	if st != cpio.StatusStock {
		t.Errorf("backup status = %s, want stock", st)
	}
	if dst != BackupPath(path) {
		t.Errorf("backup path = %s, want %s", dst, BackupPath(path))
	}
}
