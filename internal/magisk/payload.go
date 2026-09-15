// Package magisk reads the Magisk installer archive and performs the ramdisk
// patch that Magisk's own boot_patch.sh would apply, without needing a shell,
// busybox or any Magisk binary to run on the host.
package magisk

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/ejfkdev/avdroot/internal/axml"
)

// Payload holds the binaries extracted from a Magisk installer archive for one
// target ABI.
//
// A Magisk release is a single universal package: the same archive carries
// native code for every supported ABI and installs on any Android version at or
// above its minSdkVersion. There is no per-Android-version Magisk build, so the
// only per-device decisions are which ABI directory to use and whether the
// declared API range covers the target.
type Payload struct {
	Version     string // human readable, e.g. "31.0"
	VersionCode int    // numeric, e.g. 31000
	ABI         string // the archive's lib directory that was used

	// Package, MinSDK and TargetSDK come from the archive's own manifest.
	Package   string
	MinSDK    int
	TargetSDK int
	// ABIs lists every ABI the archive ships native code for.
	ABIs []string

	MagiskInit []byte // replaces /init inside the ramdisk
	Magisk     []byte // the magisk binary, stored compressed in overlay.d/sbin
	InitLD     []byte // LD_PRELOAD shim for /init
	Stub       []byte // stub APK embedded in the ramdisk

	// APK is the complete installer archive. A Magisk release zip is also a
	// valid APK, which is how the Magisk app gets installed into the AVD.
	APK []byte
}

// abiAliases maps the many ways an ABI can be spelled onto the lib directory
// names used inside the Magisk archive.
var abiAliases = map[string]string{
	"arm64-v8a":   "arm64-v8a",
	"arm64":       "arm64-v8a",
	"aarch64":     "arm64-v8a",
	"armeabi-v7a": "armeabi-v7a",
	"armeabi":     "armeabi-v7a",
	"armv7":       "armeabi-v7a",
	"arm":         "armeabi-v7a",
	"x86_64":      "x86_64",
	"x64":         "x86_64",
	"x86":         "x86",
	"i686":        "x86",
}

// NormalizeABI maps an ABI spelling onto a Magisk lib directory name.
func NormalizeABI(abi string) (string, error) {
	if v, ok := abiAliases[strings.ToLower(strings.TrimSpace(abi))]; ok {
		return v, nil
	}
	return "", fmt.Errorf("unsupported ABI %q", abi)
}

var (
	reMagiskVer     = regexp.MustCompile(`(?m)^MAGISK_VER='?([^'\r\n]+)'?`)
	reMagiskVerCode = regexp.MustCompile(`(?m)^MAGISK_VER_CODE=(\d+)`)
)

// LoadPayload opens a Magisk installer archive and extracts the files needed
// to patch a ramdisk for the given ABI.
func LoadPayload(zipPath, abi string) (*Payload, error) {
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

	p := &Payload{ABI: libDir, APK: data}

	// The archive is also a valid APK, so its manifest states the API range it
	// can be installed on. Reading it is more reliable than any external table.
	if manifest, err := readZipFile(zr, "AndroidManifest.xml"); err == nil {
		if m, err := axml.Parse(manifest); err == nil {
			p.Package = m.Package
			p.MinSDK = m.MinSDK
			p.TargetSDK = m.TargetSDK
			if p.Version == "" {
				p.Version = m.VersionName
			}
			if p.VersionCode == 0 {
				p.VersionCode = int(m.VersionCode)
			}
		}
	}
	p.ABIs = availableABIs(zr)

	// Read the version from the installer's utility script.
	if uf, err := readZipFile(zr, "assets/util_functions.sh"); err == nil {
		if m := reMagiskVer.FindSubmatch(uf); m != nil {
			p.Version = string(m[1])
		}
		if m := reMagiskVerCode.FindSubmatch(uf); m != nil {
			fmt.Sscanf(string(m[1]), "%d", &p.VersionCode)
		}
	}

	// A Magisk release archive ships exactly one magisk binary per ABI. The
	// older magisk32/magisk64 split is intentionally not supported: it was
	// removed in Magisk 26, and mixing it with a modern archive would produce
	// a ramdisk the init cannot use.
	assets := []struct {
		dst  *[]byte
		path string
		name string
	}{
		{&p.MagiskInit, fmt.Sprintf("lib/%s/libmagiskinit.so", libDir), "libmagiskinit.so"},
		{&p.Magisk, fmt.Sprintf("lib/%s/libmagisk.so", libDir), "libmagisk.so"},
		{&p.InitLD, fmt.Sprintf("lib/%s/libinit-ld.so", libDir), "libinit-ld.so"},
		{&p.Stub, "assets/stub.apk", "assets/stub.apk"},
	}
	for _, a := range assets {
		b, err := readZipFile(zr, a.path)
		if err != nil {
			// A missing magisk binary means an archive from before Magisk 26,
			// which shipped separate magisk32 and magisk64 files; say so
			// rather than reporting a bare open failure.
			if strings.Contains(a.name, "magisk") && !hasEntry(zr, a.path) {
				return nil, fmt.Errorf("the archive has no %s for ABI %s (available: %s); "+
					"this tool requires Magisk 26 or newer, which ships one magisk binary per ABI",
					a.name, libDir, strings.Join(availableABIs(zr), ", "))
			}
			return nil, fmt.Errorf("reading %s from archive: %w", a.path, err)
		}
		*a.dst = b
	}

	if p.Version == "" {
		p.Version = "unknown"
	}
	// Guard against a wrong file being passed: anything that is not the Magisk
	// app would otherwise be baked into the ramdisk and run as root at boot.
	// A full APK signature check is not attempted; see the README.
	if p.Package != "" && p.Package != MagiskPackage {
		return nil, fmt.Errorf("%s is a %s package, not the Magisk installer (%s)",
			zipPath, p.Package, MagiskPackage)
	}
	return p, nil
}

// MagiskPackage is the application id of the official Magisk app.
const MagiskPackage = "com.topjohnwu.magisk"

func readZipFile(zr *zip.Reader, name string) ([]byte, error) {
	for _, f := range zr.File {
		if f.Name != name {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		defer rc.Close()
		return io.ReadAll(rc)
	}
	return nil, fmt.Errorf("%s not found in archive", name)
}

func hasEntry(zr *zip.Reader, name string) bool {
	for _, f := range zr.File {
		if f.Name == name {
			return true
		}
	}
	return false
}

// availableABIs lists the lib directories present in the archive, for error
// messages when the requested ABI is missing.
func availableABIs(zr *zip.Reader) []string {
	seen := map[string]bool{}
	for _, f := range zr.File {
		parts := strings.Split(f.Name, "/")
		if len(parts) == 3 && parts[0] == "lib" {
			seen[parts[1]] = true
		}
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Description renders a short summary for logging.
func (p *Payload) Description() string {
	return fmt.Sprintf("Magisk %s (%d, %s)", p.Version, p.VersionCode, p.ABI)
}

// APIRange renders the supported API levels, when the archive declares them.
func (p *Payload) APIRange() string {
	switch {
	case p.MinSDK > 0 && p.TargetSDK > 0:
		return fmt.Sprintf("API %d+ (built against %d)", p.MinSDK, p.TargetSDK)
	case p.MinSDK > 0:
		return fmt.Sprintf("API %d+", p.MinSDK)
	default:
		return "undeclared"
	}
}

// ErrAPIUnsupported reports a Magisk release that cannot run on the target.
type ErrAPIUnsupported struct {
	DeviceAPI int
	MinSDK    int
	Version   string
}

func (e *ErrAPIUnsupported) Error() string {
	return fmt.Sprintf("Magisk %s requires Android API %d or newer, but the target reports API %d",
		e.Version, e.MinSDK, e.DeviceAPI)
}

// CheckAPI reports whether this release can run on a device reporting deviceAPI.
//
// Only the manifest's own minSdkVersion is treated as a hard limit, because it
// is a fact about the package. Whether a *newer* Android is supported cannot be
// decided from the manifest: a release built against an older platform often
// works on a newer one. That case produces a warning instead, and the final
// root check is what settles it.
func (p *Payload) CheckAPI(deviceAPI int) (warning string, err error) {
	if deviceAPI <= 0 || p.MinSDK == 0 {
		return "", nil
	}
	if deviceAPI < p.MinSDK {
		return "", &ErrAPIUnsupported{DeviceAPI: deviceAPI, MinSDK: p.MinSDK, Version: p.Version}
	}
	// A large gap between the platform the release was built for and the
	// device's platform is worth flagging, but is not fatal.
	if p.TargetSDK > 0 && deviceAPI > p.TargetSDK+2 {
		return fmt.Sprintf(
			"Magisk %s was built for Android API %d but the target is API %d; "+
				"if root does not work, try a newer Magisk release", p.Version, p.TargetSDK, deviceAPI), nil
	}
	return "", nil
}
