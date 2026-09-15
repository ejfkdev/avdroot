package avd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// buildFakeSDK creates a minimal SDK tree: one AVD definition pointing at one
// system image, laid out the way the SDK manager writes them.
func buildFakeSDK(t *testing.T) (sdkRoot, avdHome string) {
	t.Helper()
	root := t.TempDir()
	sdkRoot = filepath.Join(root, "sdk")
	avdHome = filepath.Join(root, "avd")

	imageDir := filepath.Join(sdkRoot, "system-images", "android-37.1", "google_apis_ps16k", "arm64-v8a")
	if err := os.MkdirAll(imageDir, 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(imageDir, "build.prop"), `
# comment line
ro.build.version.sdk=37
ro.build.version.release=17
ro.product.cpu.abi=arm64-v8a
ro.product.first_api_level=37
`)
	write(t, filepath.Join(imageDir, "source.properties"), `
Pkg.Desc=System Image Page Size 16k arm64-v8a with Google APIs.
AndroidVersion.ApiLevel=37.1
SystemImage.Abi=arm64-v8a
SystemImage.TagId=google_apis,page_size_16kb
`)
	write(t, filepath.Join(imageDir, "ramdisk.img"), "lz4 bytes")

	avdDir := filepath.Join(avdHome, "Pixel_10_Pro_XL.avd")
	if err := os.MkdirAll(avdDir, 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(avdHome, "Pixel_10_Pro_XL.ini"), `
avd.ini.encoding=UTF-8
path=`+avdDir+`
path.rel=avd/Pixel_10_Pro_XL.avd
target=android-37.1
`)
	write(t, filepath.Join(avdDir, "config.ini"), `
AvdId=Pixel_10_Pro_XL
PlayStore.enabled=false
abi.type=arm64-v8a
hw.cpu.arch=arm64
image.sysdir.1=system-images/android-37.1/google_apis_ps16k/arm64-v8a/
tag.id=google_apis
`)
	return sdkRoot, avdHome
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDiscoverSDKHonoursOverride(t *testing.T) {
	sdkRoot, _ := buildFakeSDK(t)
	s, err := DiscoverSDK(sdkRoot)
	if err != nil {
		t.Fatal(err)
	}
	if s.Root != sdkRoot {
		t.Errorf("Root = %s, want %s", s.Root, sdkRoot)
	}

	// A non-existent override must fail rather than silently using a default.
	if _, err := DiscoverSDK(filepath.Join(sdkRoot, "nope")); err == nil {
		t.Error("expected an error for a missing SDK directory")
	}
}

func TestListAVDsAndResolve(t *testing.T) {
	sdkRoot, avdHome := buildFakeSDK(t)
	s := &SDK{Root: sdkRoot, AVDHome: avdHome}

	avds, err := s.ListAVDs()
	if err != nil {
		t.Fatal(err)
	}
	if len(avds) != 1 {
		t.Fatalf("found %d AVDs, want 1", len(avds))
	}
	a := avds[0]
	if a.Name != "Pixel_10_Pro_XL" {
		t.Errorf("Name = %q", a.Name)
	}
	if got := a.ABI(); got != "arm64-v8a" {
		t.Errorf("ABI = %q, want arm64-v8a", got)
	}
	if got := a.API(); got != 37 {
		t.Errorf("API = %d, want 37", got)
	}
	if got := a.Release(); got != "17" {
		t.Errorf("Release = %q, want 17", got)
	}
	if a.PlayStore() {
		t.Error("PlayStore() = true, want false")
	}
	if a.RamdiskPath() == "" {
		t.Fatal("RamdiskPath() is empty")
	}
	if a.Image == nil || a.Image.Tag != "google_apis,page_size_16kb" {
		t.Errorf("image tag not resolved: %+v", a.Image)
	}
}

// With a single AVD, an empty selector must pick it, which is what makes
// "avdroot patch" work with no arguments.
func TestResolveDefaultsToOnlyAVD(t *testing.T) {
	sdkRoot, avdHome := buildFakeSDK(t)
	s := &SDK{Root: sdkRoot, AVDHome: avdHome}

	tgt, err := s.Resolve("")
	if err != nil {
		t.Fatal(err)
	}
	if tgt.AVD == nil || tgt.AVD.Name != "Pixel_10_Pro_XL" {
		t.Fatalf("resolved to %+v, want the only AVD", tgt)
	}
	if tgt.ABI != "arm64-v8a" || tgt.API != 37 {
		t.Errorf("ABI/API = %s/%d, want arm64-v8a/37", tgt.ABI, tgt.API)
	}
}

func TestResolveByAVDNameAndImagePath(t *testing.T) {
	sdkRoot, avdHome := buildFakeSDK(t)
	s := &SDK{Root: sdkRoot, AVDHome: avdHome}

	byName, err := s.Resolve("Pixel_10_Pro_XL")
	if err != nil {
		t.Fatal(err)
	}
	byImage, err := s.Resolve("system-images/android-37.1/google_apis_ps16k/arm64-v8a")
	if err != nil {
		t.Fatal(err)
	}
	if byName.RamdiskPath != byImage.RamdiskPath {
		t.Errorf("AVD and image resolved to different ramdisks:\n%s\n%s",
			byName.RamdiskPath, byImage.RamdiskPath)
	}
}

// A direct path to a ramdisk must be accepted, since users keep copies.
func TestResolveByRamdiskPath(t *testing.T) {
	sdkRoot, avdHome := buildFakeSDK(t)
	s := &SDK{Root: sdkRoot, AVDHome: avdHome}

	path := filepath.Join(sdkRoot, "system-images", "android-37.1", "google_apis_ps16k", "arm64-v8a", "ramdisk.img")
	tgt, err := s.Resolve(path)
	if err != nil {
		t.Fatal(err)
	}
	if tgt.RamdiskPath != path {
		t.Errorf("RamdiskPath = %s, want %s", tgt.RamdiskPath, path)
	}
	// Properties are picked up from the surrounding system image.
	if tgt.API != 37 {
		t.Errorf("API = %d, want 37", tgt.API)
	}
}

// Patching a non-ramdisk file must be refused rather than corrupting it.
func TestResolveRejectsNonRamdisk(t *testing.T) {
	sdkRoot, avdHome := buildFakeSDK(t)
	s := &SDK{Root: sdkRoot, AVDHome: avdHome}

	other := filepath.Join(sdkRoot, "system-images", "android-37.1", "google_apis_ps16k", "arm64-v8a", "system.img")
	write(t, other, "not a ramdisk")
	if _, err := s.Resolve(other); err == nil {
		t.Error("expected an error when resolving a non-ramdisk file")
	}
}

func TestListSystemImages(t *testing.T) {
	sdkRoot, avdHome := buildFakeSDK(t)
	s := &SDK{Root: sdkRoot, AVDHome: avdHome}

	images, err := s.ListSystemImages()
	if err != nil {
		t.Fatal(err)
	}
	if len(images) != 1 {
		t.Fatalf("found %d images, want 1", len(images))
	}
	img := images[0]
	if img.ABI != "arm64-v8a" || img.API != 37 {
		t.Errorf("ABI/API = %s/%d", img.ABI, img.API)
	}
	if img.Ramdisk == "" {
		t.Error("Ramdisk is empty")
	}
}

// findRamdisk must ignore the copies users leave beside the original; a copy can
// be named anything, so only the canonical name is accepted.
func TestFindRamdiskIgnoresCopies(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "ramdisk.img.orig"), "copy")
	if got := findRamdisk(dir); got != "" {
		t.Errorf("findRamdisk picked up a stray copy: %s", got)
	}
	want := filepath.Join(dir, "ramdisk.img")
	write(t, want, "real")
	if got := findRamdisk(dir); got != want {
		t.Errorf("findRamdisk = %s, want %s", got, want)
	}
}

func TestParseAPITarget(t *testing.T) {
	for in, want := range map[string]int{
		"android-37.1": 37,
		"android-28":   28,
		"37":           37,
		"37.1":         37,
		"":             0,
		"android":      0,
	} {
		if got := parseAPITarget(in); got != want {
			t.Errorf("parseAPITarget(%q) = %d, want %d", in, got, want)
		}
	}
}

// Several AVDs make an empty selector ambiguous, and the error must name them.
func TestResolveAmbiguousTargets(t *testing.T) {
	sdkRoot, avdHome := buildFakeSDK(t)
	write(t, filepath.Join(avdHome, "Second.ini"), "path="+filepath.Join(avdHome, "Second.avd")+"\ntarget=android-37.1\n")

	s := &SDK{Root: sdkRoot, AVDHome: avdHome}
	_, err := s.Resolve("")
	if err == nil {
		t.Fatal("expected an error when several AVDs exist")
	}
	if !strings.Contains(err.Error(), "Pixel_10_Pro_XL") || !strings.Contains(err.Error(), "Second") {
		t.Errorf("error does not list the AVDs: %v", err)
	}
}
