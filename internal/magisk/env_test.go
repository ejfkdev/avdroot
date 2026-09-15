package magisk

import (
	"sort"
	"strconv"
	"strings"
	"testing"
)

// wantEnvNames is the set a real emulator ends up with under /data/adb/magisk
// after the manager app has fixed the environment. It was read off a device
// running Magisk 31.0, and it is what MagiskInstaller.extractFiles produces:
// seven shared objects with their "lib" prefix and ".so" suffix stripped, four
// assets, and the ChromeOS directory.
var wantEnvNames = []string{
	"addon.d.sh",
	"boot_patch.sh",
	"bootctl",
	"busybox",
	"chromeos/futility",
	"chromeos/kernel.keyblock",
	"chromeos/kernel_data_key.vbprivk",
	"init-ld",
	"magisk",
	"magiskboot",
	"magiskinit",
	"magiskpolicy",
	"stub.apk",
	"util_functions.sh",
}

func TestEnvironmentFilesMatchTheManagerApp(t *testing.T) {
	zipPath := testAsset(t, "Magisk.zip")

	files, err := EnvironmentFiles(zipPath, "arm64-v8a")
	if err != nil {
		t.Fatal(err)
	}

	var got []string
	sizes := map[string]int{}
	for _, f := range files {
		got = append(got, f.Name)
		sizes[f.Name] = len(f.Data)
	}
	sort.Strings(got)

	if strings.Join(got, "\n") != strings.Join(wantEnvNames, "\n") {
		t.Errorf("environment files:\n got: %v\nwant: %v", got, wantEnvNames)
	}

	// An empty or truncated entry would still satisfy a name comparison, and
	// the failure would only show up as su not working on the device.
	for _, name := range got {
		if sizes[name] == 0 {
			t.Errorf("%s is empty", name)
		}
	}

	// Everything lands under /data/adb/magisk, one level deep at most.
	for _, name := range got {
		if strings.HasPrefix(name, "/") || strings.Count(name, "/") > 1 {
			t.Errorf("%s is not a plausible path under /data/adb/magisk", name)
		}
	}
}

// The version the environment declares has to be the version the ramdisk was
// patched with, because Magisk's own env_check greps for both and refuses to
// use an environment that disagrees. Both come from this one archive, and this
// asserts they still do.
func TestEnvironmentVersionMatchesTheInstaller(t *testing.T) {
	zipPath := testAsset(t, "Magisk.zip")

	p, err := LoadPayload(zipPath, "arm64-v8a")
	if err != nil {
		t.Fatal(err)
	}
	if p.Version == "" || p.VersionCode == 0 {
		t.Fatalf("the installer reported no version (%q, %d)", p.Version, p.VersionCode)
	}

	files, err := EnvironmentFiles(zipPath, "arm64-v8a")
	if err != nil {
		t.Fatal(err)
	}
	var uf []byte
	for _, f := range files {
		if f.Name == "util_functions.sh" {
			uf = f.Data
		}
	}
	if uf == nil {
		t.Fatal("util_functions.sh is not part of the environment")
	}
	for _, want := range []string{
		"MAGISK_VER='" + p.Version + "'",
		"MAGISK_VER_CODE=" + strconv.Itoa(p.VersionCode),
	} {
		if !containsLine(string(uf), want) {
			t.Errorf("util_functions.sh has no line %q, so env_check would reject the environment", want)
		}
	}
}

// containsLine mirrors grep -x: the whole line has to match.
func containsLine(text, want string) bool {
	for _, line := range strings.Split(text, "\n") {
		if line == want {
			return true
		}
	}
	return false
}
