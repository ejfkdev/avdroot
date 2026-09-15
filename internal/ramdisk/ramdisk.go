// Package ramdisk reads and writes ramdisk.img files: decompressing on load,
// recompressing with the same format on save, and preserving a pristine copy
// so a patch can always be undone.
package ramdisk

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/ejfkdev/avdroot/internal/cpio"
	"github.com/ejfkdev/avdroot/internal/imgfmt"
)

// BackupSuffix is appended to a ramdisk path to form the pristine copy.
const BackupSuffix = ".backup"

// BackupPath returns the path of the pristine copy of a ramdisk.
func BackupPath(ramdisk string) string { return ramdisk + BackupSuffix }

// Load reads a ramdisk image and returns its uncompressed cpio payload along
// with the compression format that was used, so the image can be written back
// in the same format.
func Load(path string) ([]byte, imgfmt.Format, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, imgfmt.Unknown, err
	}
	payload, format, err := imgfmt.Decompress(raw)
	if err != nil {
		return nil, format, fmt.Errorf("%s: %w", path, err)
	}
	return payload, format, nil
}

// Save compresses payload with format and writes it to path atomically, so an
// interrupted run cannot leave a truncated ramdisk behind.
func Save(path string, payload []byte, format imgfmt.Format, mode os.FileMode) error {
	if !format.IsCompressed() {
		return fmt.Errorf("refusing to write ramdisk as %s: %w", format, imgfmt.ErrUnsupportedFormat)
	}
	encoded, err := imgfmt.Compress(format, payload)
	if err != nil {
		return err
	}

	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		tmp.Close()
		os.Remove(tmpName) // no-op once the rename succeeded
	}()

	if _, err := tmp.Write(encoded); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if mode == 0 {
		mode = 0o644
	}
	if err := os.Chmod(tmpName, mode); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

// Status reports whether the ramdisk at path is stock, Magisk patched or
// carrying a foreign patch.
func Status(path string) (cpio.Status, error) {
	payload, _, err := Load(path)
	if err != nil {
		return cpio.StatusStock, err
	}
	a, err := cpio.Load(payload)
	if err != nil {
		return cpio.StatusStock, err
	}
	return a.Test(), nil
}

// EnsureBackup copies the ramdisk to <path>.backup unless a backup already
// exists, and reports whether it created one.
//
// Never overwriting an existing backup matters: rootAVD's unconditional
// backup turns the pristine copy into a patched image on the second run, so
// the original can no longer be recovered.
func EnsureBackup(path string) (bool, error) {
	dst := path + BackupSuffix
	if _, err := os.Stat(dst); err == nil {
		return false, nil
	} else if !os.IsNotExist(err) {
		return false, err
	}
	if err := copyFile(path, dst); err != nil {
		return false, err
	}
	return true, nil
}

// BackupStatus inspects the pristine copy and returns its status, or an error
// when there is no backup.
func BackupStatus(path string) (cpio.Status, string, error) {
	dst := path + BackupSuffix
	fi, err := os.Stat(dst)
	if err != nil {
		return cpio.StatusStock, dst, err
	}
	if fi.Size() == 0 {
		return cpio.StatusStock, dst, fmt.Errorf("backup %s is empty", dst)
	}
	st, err := Status(dst)
	return st, dst, err
}

// Restore replaces the ramdisk with its pristine copy.
func Restore(path string) error {
	dst := path + BackupSuffix
	if _, err := os.Stat(dst); err != nil {
		return fmt.Errorf("no backup at %s: %w", dst, err)
	}
	// Read the mode of the current file so restore keeps it.
	var mode os.FileMode = 0o644
	if fi, err := os.Stat(path); err == nil {
		mode = fi.Mode().Perm()
	}
	return copyFileMode(dst, path, mode)
}

func copyFile(src, dst string) error {
	fi, err := os.Stat(src)
	if err != nil {
		return err
	}
	return copyFileMode(src, dst, fi.Mode().Perm())
}

func copyFileMode(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	tmp, err := os.CreateTemp(filepath.Dir(dst), filepath.Base(dst)+".tmp*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		tmp.Close()
		os.Remove(tmpName)
	}()

	if _, err := io.Copy(tmp, in); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, mode); err != nil {
		return err
	}
	return os.Rename(tmpName, dst)
}
