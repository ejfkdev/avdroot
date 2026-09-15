package avd

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// makeSDK creates a directory that looks like an Android SDK.
func makeSDK(t *testing.T, root string, markers ...string) string {
	t.Helper()
	if len(markers) == 0 {
		markers = []string{"platform-tools", "system-images"}
	}
	for _, m := range markers {
		if err := os.MkdirAll(filepath.Join(root, m), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// toolName is the file name a tool has on this platform.
func toolName(base string) string {
	if runtime.GOOS == "windows" {
		return base + ".exe"
	}
	return base
}

func TestLooksLikeSDK(t *testing.T) {
	dir := t.TempDir()

	if LooksLikeSDK(dir) {
		t.Error("an empty directory was recognised as an SDK")
	}
	if LooksLikeSDK(filepath.Join(dir, "nope")) {
		t.Error("a missing directory was recognised as an SDK")
	}
	// Any single marker is enough.
	for _, marker := range sdkMarkers {
		d := makeSDK(t, filepath.Join(dir, marker), marker)
		if !LooksLikeSDK(d) {
			t.Errorf("a directory with %s/ was not recognised as an SDK", marker)
		}
	}

	// A platform-tools-only tree is what Homebrew's cask installs. It must not
	// be mistaken for an SDK, or the real installation would be hidden.
	for _, marker := range []string{"platform-tools", "licenses"} {
		d := makeSDK(t, filepath.Join(dir, "pt-"+marker), marker)
		if LooksLikeSDK(d) {
			t.Errorf("a directory holding only %s/ was recognised as an SDK", marker)
		}
	}
}

// When several SDKs are present the one holding system images must win, even
// if a higher-priority candidate was found first. This is the real situation
// when Homebrew's command-line-tools cask is on PATH next to an Android Studio
// SDK. The discovery is built by hand so the check does not depend on whatever
// SDK happens to be installed on the machine running the tests.
func TestSDKRankingPrefersSystemImages(t *testing.T) {
	homebrew := makeSDK(t, filepath.Join(t.TempDir(), "homebrew"), "cmdline-tools")
	full := makeSDK(t, filepath.Join(t.TempDir(), "sdk"))

	if HasSystemImages(homebrew) {
		t.Fatal("the fake command-line-tools tree should not have system-images")
	}
	if !HasSystemImages(full) {
		t.Fatal("the fake SDK should have system-images")
	}
	if got, _ := rankSDK(full); got != 1 {
		t.Errorf("rankSDK(SDK with images) = %d, want 1", got)
	}
	if got, note := rankSDK(homebrew); got != 0 || note == "" {
		t.Errorf("rankSDK(no images) = %d/%q, want 0 with a note", got, note)
	}

	// The higher-priority candidate is added first, as PATH and ANDROID_HOME
	// would be, and must still lose to the complete SDK.
	d := newDiscovery("an Android SDK")
	d.add(homebrew, OriginTool, "sdkmanager on PATH")
	d.add(full, OriginDefault, "default location")
	d.finish(LooksLikeSDK, rankSDK)

	if d.path() != full {
		t.Fatalf("selected %q, want the SDK with system-images %q", d.path(), full)
	}
	if got := d.candidates[d.chosen].Note; got != "has system-images" {
		t.Errorf("note = %q, want %q", got, "has system-images")
	}

	// With no candidate holding images, the first valid one is used.
	other := makeSDK(t, filepath.Join(t.TempDir(), "other"), "platforms")
	d2 := newDiscovery("an Android SDK")
	d2.add(homebrew, OriginTool, "sdkmanager on PATH")
	d2.add(other, OriginDefault, "default location")
	d2.finish(LooksLikeSDK, rankSDK)
	if d2.path() != homebrew {
		t.Errorf("without any system-images the first valid candidate should win, got %q", d2.path())
	}
}

func TestSDKRootFromToolAbsent(t *testing.T) {
	t.Setenv("PATH", t.TempDir()) // empty directory, nothing on it
	if got := sdkRootFromTool("definitely-not-installed"); got != "" {
		t.Errorf("sdkRootFromTool returned %q for a missing tool", got)
	}
}

// An explicit --sdk wins over everything, including a valid SDK on PATH.
func TestFindSDKPrefersExplicitFlag(t *testing.T) {
	override := makeSDK(t, filepath.Join(t.TempDir(), "chosen"))
	onPath := makeSDK(t, filepath.Join(t.TempDir(), "onpath"))
	exe := filepath.Join(onPath, "platform-tools", toolName("adb"))
	if err := os.WriteFile(exe, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", filepath.Join(onPath, "platform-tools"))
	t.Setenv("ANDROID_HOME", onPath)

	d := findSDK(override)
	if d.path() != override {
		t.Errorf("selected %q, want the explicit override %q", d.path(), override)
	}
	if d.source() != "--sdk" {
		t.Errorf("source = %q, want --sdk", d.source())
	}
	// The override is offered first, and the alternatives are still recorded.
	if len(d.candidates) < 2 {
		t.Fatalf("expected several candidates, got %d", len(d.candidates))
	}
	if d.candidates[0].Origin != OriginFlag {
		t.Errorf("first candidate origin = %q, want flag", d.candidates[0].Origin)
	}
}

// Environment variables are consulted before tools on PATH and defaults.
func TestFindSDKPrefersEnvironment(t *testing.T) {
	env := makeSDK(t, filepath.Join(t.TempDir(), "from-env"))
	onPath := makeSDK(t, filepath.Join(t.TempDir(), "onpath"))
	exe := filepath.Join(onPath, "platform-tools", toolName("adb"))
	if err := os.WriteFile(exe, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", filepath.Join(onPath, "platform-tools"))
	t.Setenv("ANDROID_HOME", env)
	t.Setenv("ANDROID_SDK_ROOT", "")

	d := findSDK("")
	if d.path() != env {
		t.Errorf("selected %q, want $ANDROID_HOME %q", d.path(), env)
	}
	if d.source() != "$ANDROID_HOME" {
		t.Errorf("source = %q, want $ANDROID_HOME", d.source())
	}
}

// ANDROID_SDK_ROOT is the older spelling and must still be honoured.
func TestFindSDKUsesSDKRoot(t *testing.T) {
	root := makeSDK(t, filepath.Join(t.TempDir(), "sdkroot"))
	t.Setenv("PATH", t.TempDir())
	t.Setenv("ANDROID_HOME", "")
	t.Setenv("ANDROID_SDK_ROOT", root)

	d := findSDK("")
	if d.path() != root {
		t.Errorf("selected %q, want %q", d.path(), root)
	}
}

// A path that does not look like an SDK must not be chosen over a valid one,
// but is still accepted when it is the explicit flag.
func TestSDKValidation(t *testing.T) {
	notAnSDK := t.TempDir() // exists, but has no SDK markers
	real := makeSDK(t, filepath.Join(t.TempDir(), "real"))
	t.Setenv("PATH", t.TempDir())
	t.Setenv("ANDROID_HOME", notAnSDK)
	t.Setenv("ANDROID_SDK_ROOT", "")

	// The search phase must not treat an arbitrary directory as an SDK.
	if LooksLikeSDK(notAnSDK) {
		t.Error("a plain directory was recognised as an SDK")
	}
	if gotsdk := findSDK(""); gotsdk.path() != "" && !LooksLikeSDK(gotsdk.path()) {
		t.Errorf("findSDK selected a non-SDK path %q", gotsdk.path())
	}

	// With an explicit flag, a non-conforming path is still used so that
	// unusual layouts remain patchable.
	d := findSDK(notAnSDK)
	if d.path() != notAnSDK {
		t.Errorf("explicit --sdk=%q was ignored (got %q)", notAnSDK, d.path())
	}
	_ = real
}

func TestFindAVDHomePrefersExplicitAndPopulated(t *testing.T) {
	// An empty default location must not mask a populated override.
	empty := t.TempDir()
	populated := t.TempDir()
	if err := os.WriteFile(filepath.Join(populated, "Pixel.ini"), []byte("path=x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ANDROID_AVD_HOME", populated)
	t.Setenv("ANDROID_USER_HOME", empty)

	d := findAVDHome("")
	if d.path() != populated {
		t.Errorf("selected %q, want %q", d.path(), populated)
	}
	if d.source() != "$ANDROID_AVD_HOME" {
		t.Errorf("source = %q, want $ANDROID_AVD_HOME", d.source())
	}
}

// ANDROID_USER_HOME names the parent of the avd directory.
func TestFindAVDHomeUserHome(t *testing.T) {
	userHome := t.TempDir()
	avdDir := filepath.Join(userHome, "avd")
	if err := os.MkdirAll(avdDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(avdDir, "A.ini"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ANDROID_AVD_HOME", "")
	t.Setenv("ANDROID_USER_HOME", userHome)

	if got := findAVDHome("").path(); got != avdDir {
		t.Errorf("selected %q, want %q", got, avdDir)
	}
}

// The legacy ANDROID_PREFS_ROOT / ANDROID_SDK_HOME variables name a directory
// that contains .android.
func TestFindAVDHomeLegacyPrefsRoot(t *testing.T) {
	prefs := t.TempDir()
	avdDir := filepath.Join(prefs, ".android", "avd")
	if err := os.MkdirAll(avdDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(avdDir, "Legacy.ini"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ANDROID_AVD_HOME", "")
	t.Setenv("ANDROID_USER_HOME", "")
	t.Setenv("ANDROID_PREFS_ROOT", prefs)
	t.Setenv("ANDROID_SDK_HOME", "")

	if got := findAVDHome("").path(); got != avdDir {
		t.Errorf("selected %q, want %q", got, avdDir)
	}
}

func TestCountAVDs(t *testing.T) {
	dir := t.TempDir()
	if got := countAVDs(dir); got != 0 {
		t.Errorf("countAVDs on an empty dir = %d, want 0", got)
	}
	for _, n := range []string{"One.ini", "Two.ini"} {
		if err := os.WriteFile(filepath.Join(dir, n), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(dir, "One.avd"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Only .ini files count; .avd directories are data, not definitions.
	if got := countAVDs(dir); got != 2 {
		t.Errorf("countAVDs = %d, want 2", got)
	}
	if got := countAVDs(filepath.Join(dir, "missing")); got != 0 {
		t.Errorf("countAVDs on a missing dir = %d, want 0", got)
	}
}

// Discovery must never return the same path twice, since candidates are shown
// to the user.
func TestCandidateDeduplication(t *testing.T) {
	dir := makeSDK(t, filepath.Join(t.TempDir(), "sdk"))
	d := newDiscovery("an Android SDK")
	d.add(dir, OriginDefault, "first")
	d.add(dir, OriginSearch, "second")
	d.add(filepath.Join(dir, "."), OriginEnv, "same path, different spelling")
	if len(d.candidates) != 1 {
		t.Errorf("recorded %d candidates for one path: %+v", len(d.candidates), d.candidates)
	}
}

// The candidate trace must describe every location, for "avdroot doctor".
func TestCandidateDescribe(t *testing.T) {
	present := makeSDK(t, filepath.Join(t.TempDir(), "sdk"))
	missing := filepath.Join(t.TempDir(), "absent")

	d := newDiscovery("an Android SDK")
	d.add(present, OriginFlag, "--sdk")
	d.add(missing, OriginEnv, "$ANDROID_HOME")
	d.finish(LooksLikeSDK, nil)

	out := d.describe()
	for _, want := range []string{present, missing, "--sdk", "$ANDROID_HOME", "missing"} {
		if !containsStr(out, want) {
			t.Errorf("describe() does not mention %q:\n%s", want, out)
		}
	}
	if d.path() != present {
		t.Errorf("selected %q, want %q", d.path(), present)
	}
}

func containsStr(haystack, needle string) bool {
	return len(haystack) >= len(needle) && indexOfStr(haystack, needle) >= 0
}

func indexOfStr(h, n string) int {
	for i := 0; i+len(n) <= len(h); i++ {
		if h[i:i+len(n)] == n {
			return i
		}
	}
	return -1
}

// Platform defaults must be plausible for the host, and must not be empty:
// they are the last resort before searching.
func TestPlatformDefaultsNonEmpty(t *testing.T) {
	if len(platformSDKDefaults()) == 0 {
		t.Error("platformSDKDefaults returned nothing")
	}
	if len(platformAVDDefaults()) == 0 {
		t.Error("platformAVDDefaults returned nothing")
	}
	// Every platform keeps AVDs in <home>/.android/avd, Windows included.
	home, err := os.UserHomeDir()
	if err == nil {
		want := filepath.Join(home, ".android", "avd")
		found := false
		for _, p := range platformAVDDefaults() {
			if p == want {
				found = true
			}
		}
		if !found {
			t.Errorf("platformAVDDefaults does not include %s: %v", want, platformAVDDefaults())
		}
	}
}
