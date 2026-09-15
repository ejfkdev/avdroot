package magisk

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
)

// EnvFile is one file of Magisk's runtime environment, named as it must appear
// under /data/adb/magisk.
type EnvFile struct {
	Name string
	Data []byte
}

// envAssets are the archive entries the manager app copies verbatim, from
// MagiskInstaller.extractFiles.
var envAssets = []string{
	"assets/util_functions.sh",
	"assets/boot_patch.sh",
	"assets/addon.d.sh",
	"assets/stub.apk",
}

// envChromeOS are the ChromeOS flashing tools, which live in a subdirectory.
var envChromeOS = []string{"futility", "kernel_data_key.vbprivk", "kernel.keyblock"}

// EnvironmentFiles returns the files that make up Magisk's runtime environment
// under /data/adb/magisk.
//
// A patched ramdisk carries Magisk's init and its daemon and nothing else. The
// rest of Magisk lives in the manager app and is unpacked into /data/adb/magisk
// before it can be used — that is what the app calls fixing the environment,
// and it is why a freshly patched emulator can report Magisk running while su
// still resolves to the system's own su and rejects "-c" as a uid.
//
// The set is the one MagiskInstaller.extractFiles builds: every shared object
// under lib/<abi>/ loses its "lib" prefix and its ".so" suffix, and four assets
// plus the ChromeOS tools are copied as they are. Reproducing it here is what
// lets the whole step run from the host, with neither the app nor a tap.
func EnvironmentFiles(zipPath, abi string) ([]EnvFile, error) {
	libDir, err := NormalizeABI(abi)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(zipPath)
	if err != nil {
		return nil, err
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("%s is not a Magisk installer archive: %w", zipPath, err)
	}

	var files []EnvFile
	prefix := "lib/" + libDir + "/"
	for _, f := range zr.File {
		if f.FileInfo().IsDir() || !strings.HasPrefix(f.Name, prefix) || !strings.HasSuffix(f.Name, ".so") {
			continue
		}
		name := strings.TrimSuffix(strings.TrimPrefix(f.Name, prefix), ".so")
		if name == "" || strings.Contains(name, "/") {
			continue
		}
		// Magisk names these by dropping the three leading characters of "lib"
		// and the three trailing ones of ".so", so libbootctl.so becomes
		// bootctl. Both halves matter: keeping the "lib" leaves a file that
		// exists, satisfies a name check, and still cannot run.
		if len(name) <= 3 || !strings.HasPrefix(name, "lib") {
			continue
		}
		name = name[3:]
		b, err := readZipEntry(f)
		if err != nil {
			return nil, fmt.Errorf("reading %s from %s: %w", f.Name, zipPath, err)
		}
		files = append(files, EnvFile{Name: name, Data: b})
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("%s carries no %s binaries, so the environment cannot be built", zipPath, libDir)
	}

	for _, name := range envAssets {
		b, err := readZipFile(zr, name)
		if err != nil {
			return nil, fmt.Errorf("%s is missing %s, which the environment needs: %w", zipPath, name, err)
		}
		files = append(files, EnvFile{Name: strings.TrimPrefix(name, "assets/"), Data: b})
	}
	for _, name := range envChromeOS {
		b, err := readZipFile(zr, "assets/chromeos/"+name)
		if err != nil {
			return nil, fmt.Errorf("%s is missing the ChromeOS tool %s: %w", zipPath, name, err)
		}
		files = append(files, EnvFile{Name: "chromeos/" + name, Data: b})
	}

	// A stable order keeps the staging directory, and any error message about
	// it, reproducible.
	sort.Slice(files, func(i, j int) bool { return files[i].Name < files[j].Name })
	return files, nil
}

func readZipEntry(f *zip.File) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}
