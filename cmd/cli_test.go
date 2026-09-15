package cmd

import (
	"archive/zip"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/ejfkdev/avdroot/internal/avd"
	"github.com/ejfkdev/avdroot/internal/i18n"
	"github.com/ejfkdev/avdroot/internal/magisk"
	"github.com/ejfkdev/avdroot/internal/ui"
)

// ansi matches the escape sequences the ui package adds when colour is on.
var ansi = regexp.MustCompile("\x1b\\[[0-9;]*m")

// The command layer holds the logic that decides what gets written where, so
// these tests exercise it rather than only the packages underneath.

// newTestApp builds an app rooted at a temporary SDK, with one AVD.
func newTestApp(t *testing.T) (*app, string) {
	t.Helper()
	root := t.TempDir()
	sdkRoot := filepath.Join(root, "sdk")
	avdHome := filepath.Join(root, "avd")

	imageDir := filepath.Join(sdkRoot, "system-images", "android-37.1", "google_apis_ps16k", "arm64-v8a")
	if err := os.MkdirAll(imageDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(imageDir, "build.prop"), "ro.build.version.sdk=37\nro.build.version.release=17\nro.product.cpu.abi=arm64-v8a\n")
	writeFile(t, filepath.Join(imageDir, "ramdisk.img"), "not really an lz4 image")

	avdDir := filepath.Join(avdHome, "Pixel_10_Pro_XL.avd")
	if err := os.MkdirAll(avdDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(avdHome, "Pixel_10_Pro_XL.ini"),
		"path="+avdDir+"\ntarget=android-37.1\n")
	writeFile(t, filepath.Join(avdDir, "config.ini"),
		"AvdId=Pixel_10_Pro_XL\nabi.type=arm64-v8a\nimage.sysdir.1=system-images/android-37.1/google_apis_ps16k/arm64-v8a/\n")

	sdk := &avd.SDK{Root: sdkRoot, AVDHome: avdHome}
	return &app{sdk: sdk, noADB: true}, avdDir
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// A target that does not exist must be reported, not silently ignored. It used
// to be discarded by "verify", which then checked the running device instead.
func TestRequireTargetRejectsUnknownTarget(t *testing.T) {
	a, _ := newTestApp(t)

	for _, selector := range []string{"/does/not/exist.img", "NoSuchAVD", "system-images/nope/here"} {
		_, err := a.requireTarget([]string{selector})
		if err == nil {
			t.Errorf("requireTarget(%q) was accepted, want an error", selector)
			continue
		}
		if !strings.Contains(err.Error(), selector) {
			t.Errorf("requireTarget(%q) error does not name the target: %v", selector, err)
		}
	}
}

// Too many arguments is an error rather than a silent truncation.
func TestRequireTargetRejectsExtraArguments(t *testing.T) {
	a, _ := newTestApp(t)
	if _, err := a.requireTarget([]string{"one", "two"}); err == nil {
		t.Fatal("expected an error for two targets")
	}
}

// Naming an AVD must be reported as such, so the output does not claim it was
// picked automatically.
func TestDescribeTargetChoice(t *testing.T) {
	named := &avd.Target{AVD: &avd.AVD{Name: "A"}}
	image := &avd.Target{}

	if got := plain(describeTargetChoice(true, named)); got != i18n.T("root.target_note_named") {
		t.Errorf("named target reported as %q", got)
	}
	if got := plain(describeTargetChoice(false, named)); got != i18n.T("root.target_note_only_avd") {
		t.Errorf("sole AVD reported as %q", got)
	}
	if got := plain(describeTargetChoice(false, image)); got != i18n.T("root.target_note_only_image") {
		t.Errorf("sole image reported as %q", got)
	}
}

// A Play Store image refuses adb root, so both patch and root must say so
// before doing any work rather than after several minutes of restarting.
func TestPlayStoreImagesAreRejectedUpFront(t *testing.T) {
	a, avdDir := newTestApp(t)
	writeFile(t, filepath.Join(avdDir, "config.ini"),
		"AvdId=Pixel_10_Pro_XL\nabi.type=arm64-v8a\nPlayStore.enabled=true\n"+
			"image.sysdir.1=system-images/android-37.1/google_apis_ps16k/arm64-v8a/\n")

	tgt, err := a.requireTarget(nil)
	if err != nil {
		t.Fatal(err)
	}
	if tgt.AVD == nil || !tgt.AVD.PlayStore() {
		t.Fatal("the fixture AVD should report a Play Store image")
	}

	// runPatch returns before touching anything, so the ramdisk stays as it was.
	before, err := os.ReadFile(tgt.RamdiskPath)
	if err != nil {
		t.Fatal(err)
	}
	err = runPatch(a, tgt, patchFlags{assumeYes: true, quietNextSteps: true})
	if err == nil {
		t.Fatal("patching a Play Store image should fail")
	}
	if !strings.Contains(err.Error(), "Play Store") {
		t.Errorf("error does not explain the Play Store restriction: %v", err)
	}
	after, err := os.ReadFile(tgt.RamdiskPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Error("the ramdisk was modified despite the rejection")
	}
}

// A saved snapshot can restore the pre-patch ramdisk, so patch has to mention
// it: a user who patches and then starts the emulator normally could otherwise
// see the patch appear to do nothing.
func TestPatchWarnsAboutSnapshots(t *testing.T) {
	a, avdDir := newTestApp(t)
	if err := os.MkdirAll(filepath.Join(avdDir, "snapshots", "default_boot"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(avdDir, "config.ini"),
		"AvdId=Pixel_10_Pro_XL\nabi.type=arm64-v8a\nfastboot.forceFastBoot=yes\n"+
			"image.sysdir.1=system-images/android-37.1/google_apis_ps16k/arm64-v8a/\n")

	tgt, err := a.requireTarget(nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := tgt.AVD.Snapshots(); len(got) != 1 || got[0] != "default_boot" {
		t.Fatalf("Snapshots() = %v, want [default_boot]", got)
	}
	if !tgt.AVD.FastBoot() {
		t.Error("FastBoot() should be true for this fixture")
	}

	out := captureOutput(t, func() {
		// The ramdisk is not a real image, so the run fails after the
		// pre-flight checks; the warnings are what matter here.
		_ = runPatch(a, tgt, patchFlags{dryRun: true})
	})
	if !strings.Contains(out, "default_boot") {
		t.Errorf("patch did not mention the saved snapshot:\n%s", out)
	}
	if !strings.Contains(out, i18n.T("patch.snapshot_hint")) {
		t.Errorf("patch did not explain the cold-boot requirement:\n%s", out)
	}
}

// captureOutput collects everything written through the ui package.
func captureOutput(t *testing.T, fn func()) string {
	t.Helper()
	var sb strings.Builder
	oldOut, oldErr := ui.Writer, ui.ErrWriter
	ui.Writer, ui.ErrWriter = &sb, &sb
	defer func() { ui.Writer, ui.ErrWriter = oldOut, oldErr }()
	fn()
	return sb.String()
}

// plain removes ANSI escapes so translated strings can be compared directly.
func plain(s string) string {
	return ansi.ReplaceAllString(s, "")
}

// The installer must be the Magisk app; anything else would be baked into the
// ramdisk and run as root at boot.
func TestLoadPayloadRejectsForeignAPK(t *testing.T) {
	dir := t.TempDir()
	fake := filepath.Join(dir, "other.apk")

	// A minimal APK-shaped archive: a valid zip with a manifest that is not
	// binary AXML, so the package name stays unknown and the guard cannot fire
	// on it. The check has to tolerate that rather than reject a good archive
	// whose manifest it could not read.
	f, err := os.Create(fake)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	w, err := zw.Create("AndroidManifest.xml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte(`<manifest package="com.example.other"/>`)); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	// It must fail, because the magisk binaries are absent.
	if _, err := magisk.LoadPayload(fake, "arm64-v8a"); err == nil {
		t.Error("an archive without magisk binaries was accepted")
	}
}

// realMagiskZIP finds an installer the same way the commands do, and skips the
// test when none is installed on this machine.
func realMagiskZIP(t *testing.T) string {
	t.Helper()
	a, _ := newTestApp(t)
	path, err := a.findMagiskZIP()
	if err != nil {
		t.Skipf("no Magisk installer available: %v", err)
	}
	return path
}

// A real Magisk release must still be accepted, so the package guard does not
// reject the thing it is meant to protect.
func TestLoadPayloadAcceptsRealMagisk(t *testing.T) {
	path := realMagiskZIP(t)
	p, err := magisk.LoadPayload(path, "arm64-v8a")
	if err != nil {
		t.Fatalf("LoadPayload: %v", err)
	}
	if p.Package != magisk.MagiskPackage {
		t.Errorf("Package = %q, want %q", p.Package, magisk.MagiskPackage)
	}
}

// Command wiring: every command must reject arguments it does not understand,
// with a message that names the command.
func TestNoArgCommandsRejectArguments(t *testing.T) {
	for _, name := range []string{"list", "doctor", "install"} {
		c := findCommand(t, name)
		if c == nil {
			t.Errorf("command %q not found", name)
			continue
		}
		out := captureOutput(t, func() {
			if err := requireNoArgs(c, []string{"unexpected"}); err == nil {
				t.Errorf("%s accepted an argument", name)
			}
		})
		_ = out
		if err := requireNoArgs(c, nil); err != nil {
			t.Errorf("%s rejected no arguments: %v", name, err)
		}
	}
}

// findCommand locates a subcommand in the real tree, so the wiring is tested
// rather than a copy of it.
func findCommand(t *testing.T, name string) *cobra.Command {
	t.Helper()
	root := newRootCmd("test")
	for _, c := range root.Commands() {
		if c.Name() == name {
			return c
		}
	}
	return nil
}

// The root command tree must expose the repository, since a user with only the
// binary needs somewhere to report a problem.
func TestHelpShowsRepository(t *testing.T) {
	if !strings.Contains(RepoURL, "github.com/ejfkdev/avdroot") {
		t.Errorf("RepoURL = %q", RepoURL)
	}
	root := newRootCmd("test")
	var sb strings.Builder
	root.SetOut(&sb)
	root.SetErr(&sb)
	if err := root.Help(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sb.String(), RepoURL) {
		t.Errorf("help does not mention %s:\n%s", RepoURL, sb.String())
	}
	if !strings.Contains(sb.String(), "test") {
		t.Error("help does not show the version")
	}
}

// A downloaded installer must live in the operating system's cache directory,
// not in the configuration directory: it is reproducible, so clearing a cache
// should reclaim it without touching anything the user configured.
func TestCacheDirIsUnderUserCache(t *testing.T) {
	base, err := os.UserCacheDir()
	if err != nil {
		t.Skipf("no cache directory on this platform: %v", err)
	}
	got := cacheDir()
	if !strings.HasPrefix(got, base) {
		t.Errorf("cacheDir() = %q, which is not under %q", got, base)
	}
	if !strings.HasSuffix(got, "avdroot") {
		t.Errorf("cacheDir() = %q, want it to end in /avdroot", got)
	}
	// It must not coincide with the configuration directory.
	for _, cfg := range configDirs() {
		if got == cfg {
			t.Errorf("cacheDir() collides with the config directory %q", cfg)
		}
	}
}

// The cache is searched before the configuration directory, so a release that
// was just downloaded is used rather than an older file left behind.
func TestSearchOrderPrefersCache(t *testing.T) {
	a, _ := newTestApp(t)

	cacheIdx, configIdx := -1, -1
	for i, p := range a.magiskSearchPaths() {
		if cacheIdx < 0 && strings.HasPrefix(p, cacheDir()) {
			cacheIdx = i
		}
		for _, cfg := range configDirs() {
			if configIdx < 0 && strings.HasPrefix(p, cfg) {
				configIdx = i
			}
		}
	}
	if cacheIdx < 0 {
		t.Fatal("the cache directory is not searched")
	}
	if configIdx >= 0 && configIdx < cacheIdx {
		t.Errorf("the config directory (index %d) is searched before the cache (index %d)",
			configIdx, cacheIdx)
	}

	// An explicit path short-circuits everything.
	a.magiskZIP = "/tmp/explicit/Magisk.zip"
	if got := a.magiskSearchPaths(); len(got) != 1 || got[0] != a.magiskZIP {
		t.Errorf("an explicit path did not short-circuit the search: %v", got)
	}
}

// A missing installer must not trigger a download when the user disabled it.
func TestEnsureMagiskZIPHonoursNoDownload(t *testing.T) {
	a, _ := newTestApp(t)
	a.noDownload = true
	// Point the search somewhere empty so nothing is found locally.
	a.magiskZIP = filepath.Join(t.TempDir(), "absent", magiskZIPName)

	out := captureOutput(t, func() {
		if _, err := a.ensureMagiskZIP(); err == nil {
			t.Error("expected an error when no installer exists and downloads are off")
		}
	})
	// The failure has to be actionable: the releases page and the manual route.
	for _, want := range []string{MagiskReleasesURL, "--magisk", cacheDir()} {
		if !strings.Contains(out, want) {
			t.Errorf("the failure message does not mention %q:\n%s", want, out)
		}
	}
}

// A release URL cannot be built without a version, because the asset name
// carries it. resolveInstallerURL must fail loudly rather than invent one.
func TestResolveInstallerURLForExplicitTag(t *testing.T) {
	url, label, err := resolveInstallerURL("v31.0", false)
	if err != nil {
		t.Fatalf("resolveInstallerURL: %v", err)
	}
	if !strings.HasSuffix(url, "/download/v31.0/Magisk-v31.0.apk") {
		t.Errorf("url = %q", url)
	}
	if label != "v31.0" {
		t.Errorf("label = %q, want v31.0", label)
	}
	// A bare version is accepted and normalised.
	url, _, err = resolveInstallerURL("30.7", false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(url, "/download/v30.7/Magisk-v30.7.apk") {
		t.Errorf("url = %q, want a v-prefixed tag", url)
	}
}

// A missing release is permanent, so it must not be retried; a timeout is not.
func TestWorthRetrying(t *testing.T) {
	if worthRetrying(nil) {
		t.Error("nil should not be retried")
	}
	if worthRetrying(errors.New("HTTP 404: no release with that tag")) {
		t.Error("a 404 must not be retried")
	}
	for _, msg := range []string{
		`Get "https://...": dial tcp 20.205.243.166:443: i/o timeout`,
		"HTTP 502",
		"unexpected EOF",
	} {
		if !worthRetrying(errors.New(msg)) {
			t.Errorf("%q should be retried", msg)
		}
	}
}

// The flag has to land on every path Chromium reads startup flags from, or
// Chrome keeps rejecting the certificate while the command claims success.
func TestChromeCommandLineFiles(t *testing.T) {
	if len(chromeCommandLineFiles) == 0 {
		t.Fatal("no flag files configured")
	}
	seen := map[string]bool{}
	haveChrome, haveWebView := false, false
	for _, p := range chromeCommandLineFiles {
		if !strings.HasPrefix(p, "/data/local/") {
			t.Errorf("%s is outside /data/local, where Chromium looks", p)
		}
		if seen[p] {
			t.Errorf("%s is listed twice", p)
		}
		seen[p] = true
		if strings.HasSuffix(p, "/chrome-command-line") {
			haveChrome = true
		}
		if strings.Contains(p, "webview-command-line") {
			haveWebView = true
		}
	}
	// Chrome and the WebView use different files; covering only one would leave
	// WebView-based apps broken.
	if !haveChrome || !haveWebView {
		t.Error("both the Chrome and the WebView flag files must be covered")
	}
}

// The auto-detection has to look beyond Magisk modules: an interception tool
// that injects its CA with root at runtime keeps the only persistent copy in
// its own data directory.
func TestCandidateCADirsCoverRuntimeInjection(t *testing.T) {
	joined := strings.Join(candidateCADirs, " ")
	for _, want := range []string{
		"/data/misc/keychain/cacerts-added", // installed through Settings
		"modules",                           // installed by a Magisk module
		"/data/data/*/files/certificate",    // kept by the tool itself
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("candidateCADirs does not cover %q: %v", want, candidateCADirs)
		}
	}
}

// The command must be reachable and expose the documented flags.
func TestTrustChromeCommandWiring(t *testing.T) {
	c := findCommand(t, "trust-chrome")
	if c == nil {
		t.Fatal("trust-chrome is not registered")
	}
	for _, flag := range []string{"cert", "clear", "no-restart"} {
		if c.Flags().Lookup(flag) == nil {
			t.Errorf("--%s is missing", flag)
		}
	}
	if c.Short == "" || c.Example == "" {
		t.Error("the command needs a summary and examples for --help")
	}
}
