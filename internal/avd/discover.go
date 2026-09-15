package avd

import (
	"os"
	"os/exec"
	"path/filepath"

	"github.com/ejfkdev/avdroot/internal/i18n"
	"runtime"
	"strings"
)

// Locating the Android SDK and the AVD directory is deliberately conservative:
// an explicit flag always wins, then environment variables, then the location
// derived from tools that are already on PATH, then the conventional per-OS
// defaults, and only then a bounded search of likely install directories.
//
// Deriving the SDK root from `adb` or `emulator` on PATH is what makes the
// common cases work without any configuration: those binaries live inside the
// SDK, so walking up from the resolved symlink finds the root regardless of
// whether it came from Android Studio, the command line tools, Homebrew, snap
// or a distribution package.

// Origin explains how a location was found, for diagnostics.
type Origin string

const (
	// OriginFlag means the user named it explicitly.
	OriginFlag Origin = "flag"
	// OriginEnv means it came from an environment variable.
	OriginEnv Origin = "env"
	// OriginTool means it was derived from a tool found on PATH.
	OriginTool Origin = "tool"
	// OriginDefault means it is the conventional location for this OS.
	OriginDefault Origin = "default"
	// OriginSearch means it was found by scanning likely directories.
	OriginSearch Origin = "search"
)

// Candidate is one location considered during discovery.
type Candidate struct {
	Path string
	// Origin says how the path was suggested.
	Origin Origin
	// Source names the flag, variable or tool responsible.
	Source string
	// Exists reports whether the path is a directory on disk.
	Exists bool
	// Valid reports whether it looks like the thing being searched for.
	Valid bool
	// Score ranks candidates that are all valid: the number of AVD
	// definitions for an AVD directory, or 1 for an SDK holding system images.
	Score int
	// Note is a short annotation shown by "avdroot doctor".
	Note string
}

// discovery accumulates candidates and records which one was selected.
type discovery struct {
	candidates []Candidate
	chosen     int
	kind       string
}

func newDiscovery(kind string) *discovery {
	return &discovery{chosen: -1, kind: kind}
}

// add records a candidate, ignoring duplicates and empty paths.
func (d *discovery) add(path string, origin Origin, source string) {
	if strings.TrimSpace(path) == "" {
		return
	}
	expanded, err := expandPath(path)
	if err != nil {
		return
	}
	expanded = filepath.Clean(expanded)
	for _, c := range d.candidates {
		if samePath(c.Path, expanded) {
			return
		}
	}
	d.candidates = append(d.candidates, Candidate{
		Path:   expanded,
		Origin: origin,
		Source: source,
	})
}

// samePath compares paths, folding case on the platforms whose filesystems are
// case-insensitive by default.
func samePath(a, b string) bool {
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

// finish inspects the candidates and selects one.
//
// An explicit flag always wins, even when it does not validate or does not
// exist, because the caller must be able to override detection and needs a
// precise error naming the path they asked for rather than silently getting a
// different SDK.
//
// Otherwise the first valid candidate is used. When a scoring function is
// supplied, a candidate holding AVD definitions beats one that holds none, so
// that an empty ~/.android/avd does not mask a populated $ANDROID_AVD_HOME.
func (d *discovery) finish(validate func(string) bool, score func(string) (int, string)) {
	for i := range d.candidates {
		c := &d.candidates[i]
		c.Exists = isDir(c.Path)
		c.Valid = c.Exists && validate(c.Path)
		if score != nil {
			c.Score, c.Note = score(c.Path)
		}
	}

	// An explicit flag wins outright.
	for i := range d.candidates {
		if d.candidates[i].Origin == OriginFlag {
			d.chosen = i
			return
		}
	}

	if score != nil {
		best := -1
		for i := range d.candidates {
			if !d.candidates[i].Valid {
				continue
			}
			if best == -1 || d.candidates[i].Score > d.candidates[best].Score {
				best = i
			}
		}
		if best != -1 {
			d.chosen = best
			return
		}
	}
	for i := range d.candidates {
		if d.candidates[i].Valid {
			d.chosen = i
			return
		}
	}
}

func (d *discovery) path() string {
	if d.chosen < 0 {
		return ""
	}
	return d.candidates[d.chosen].Path
}

func (d *discovery) source() string {
	if d.chosen < 0 {
		return ""
	}
	return d.candidates[d.chosen].Source
}

// Candidates returns every location that was considered.
func (d *discovery) Candidates() []Candidate { return d.candidates }

// describe renders the candidate list for an error message.
func (d *discovery) describe() string {
	if len(d.candidates) == 0 {
		return "  (nothing to try)"
	}
	var b strings.Builder
	for _, c := range d.candidates {
		mark := " "
		switch {
		case c.Path == d.path():
			mark = ">"
		case c.Exists:
			mark = "+"
		}
		state := i18n.T("doctor.state_missing")
		if c.Exists {
			state = i18n.T("doctor.state_exists")
		}
		if c.Exists && !c.Valid {
			state = i18n.T("doctor.state_unlike", d.kind)
		}
		if c.Note != "" {
			state += ", " + c.Note
		}
		b.WriteString("  " + mark + " " + c.Path + "\n")
		b.WriteString("      " + c.Source + " (" + state + ")\n")
	}
	return b.String()
}

// sdkMarkers are the directories that identify an Android SDK root.
//
// platform-tools and licenses are deliberately excluded: Homebrew's
// android-platform-tools cask and similar packages install just those, and a
// directory holding only adb is not an SDK — treating it as one would hide the
// real installation.
var sdkMarkers = []string{
	"system-images", // the images this tool patches
	"cmdline-tools", // sdkmanager, avdmanager
	"emulator",      // the emulator itself
	"platforms",
	"build-tools",
	"sources",
	"ndk",
}

// sdkPreferred is the marker this tool actually needs, used to rank candidates
// when several SDKs are present.
const sdkPreferred = "system-images"

// LooksLikeSDK reports whether dir has the shape of an Android SDK.
func LooksLikeSDK(dir string) bool {
	if !isDir(dir) {
		return false
	}
	for _, m := range sdkMarkers {
		if isDir(filepath.Join(dir, m)) {
			return true
		}
	}
	return false
}

// HasSystemImages reports whether an SDK root contains system images, which is
// what avdroot patches.
func HasSystemImages(dir string) bool {
	return isDir(filepath.Join(dir, sdkPreferred))
}

// sdkTools are the executables whose location reveals the SDK root.
var sdkTools = []string{"adb", "emulator", "sdkmanager", "avdmanager"}

// sdkRootFromTool walks up from a tool on PATH until it finds the SDK root.
//
// Symlinks are resolved first so that a Homebrew or distribution package which
// links the tool into /usr/bin still resolves to the real SDK tree.
func sdkRootFromTool(tool string) string {
	exe := tool
	if runtime.GOOS == "windows" {
		exe += ".exe"
	}
	found, err := exec.LookPath(exe)
	if err != nil {
		// Fall back to the bare name, then the Windows spelling, so a tool
		// invoked without its extension is still found.
		if found, err = exec.LookPath(tool); err != nil {
			return ""
		}
	}
	if !filepath.IsAbs(found) {
		if abs, err := filepath.Abs(found); err == nil {
			found = abs
		}
	}

	// Walk up from both the path as found and its symlink-resolved form. The
	// raw spelling is preferred so the reported root matches what the user sees
	// in their environment (on macOS /var would otherwise become /private/var).
	if root := walkUpToSDK(found); root != "" {
		return root
	}
	if resolved, err := filepath.EvalSymlinks(found); err == nil && resolved != found {
		return walkUpToSDK(resolved)
	}
	return ""
}

// walkUpToSDK climbs from a file inside an SDK until it finds the root.
//
// adb lives in <sdk>/platform-tools, sdkmanager in
// <sdk>/cmdline-tools/<version>/bin, so a handful of levels covers every
// layout in use.
func walkUpToSDK(file string) string {
	dir := filepath.Dir(file)
	for i := 0; i < 6; i++ {
		if LooksLikeSDK(dir) {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}

// findSDK runs the full SDK discovery sequence.
func findSDK(override string) *discovery {
	d := newDiscovery(i18n.T("kind.sdk"))
	d.add(override, OriginFlag, "--sdk")

	for _, name := range []string{"ANDROID_HOME", "ANDROID_SDK_ROOT"} {
		if v := os.Getenv(name); v != "" {
			d.add(v, OriginEnv, "$"+name)
		}
	}
	for _, tool := range sdkTools {
		if root := sdkRootFromTool(tool); root != "" {
			d.add(root, OriginTool, i18n.T("src.tool", tool))
		}
	}
	for _, p := range platformSDKDefaults() {
		d.add(p, OriginDefault, i18n.T("src.default", runtime.GOOS))
	}
	for _, p := range searchSDK() {
		d.add(p, OriginSearch, i18n.T("src.search", filepath.Dir(p)))
	}
	d.finish(LooksLikeSDK, rankSDK)
	return d
}

// rankSDK prefers an SDK that actually contains system images, so that a
// platform-tools or command-line-tools installation elsewhere on PATH does not
// mask the full SDK installed by Android Studio.
func rankSDK(dir string) (int, string) {
	if HasSystemImages(dir) {
		return 1, i18n.T("note.has_images")
	}
	return 0, i18n.T("note.no_images")
}

// platformSDKDefaults lists the conventional SDK locations per operating system.
func platformSDKDefaults() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = ""
	}
	var out []string
	if home != "" {
		out = append(out,
			// Android Studio's own default on every platform.
			filepath.Join(home, "Library", "Android", "sdk"),
			filepath.Join(home, "Android", "Sdk"),
			filepath.Join(home, "Android", "sdk"),
			// Homebrew command line tools.
			filepath.Join(home, ".local", "share", "android-commandlinetools"),
		)
	}
	switch runtime.GOOS {
	case "darwin":
		out = append(out,
			"/opt/homebrew/share/android-commandlinetools", // Apple Silicon Homebrew
			"/usr/local/share/android-commandlinetools",    // Intel Homebrew
		)
	case "windows":
		if local := os.Getenv("LOCALAPPDATA"); local != "" {
			out = append(out, filepath.Join(local, "Android", "Sdk"))
		}
		if pf := os.Getenv("ProgramFiles"); pf != "" {
			out = append(out, filepath.Join(pf, "Android", "android-sdk"))
		}
		if pf := os.Getenv("ProgramFiles(x86)"); pf != "" {
			out = append(out,
				filepath.Join(pf, "Android", "android-sdk"),
				filepath.Join(pf, "Android", "android-sdk-windows"),
			)
		}
		out = append(out, `C:\Android\Sdk`, `C:\android-sdk`)
	default: // linux and the BSDs
		if home != "" {
			out = append(out,
				filepath.Join(home, "snap", "android-studio", "common", "Android", "Sdk"),
				filepath.Join(home, "snap", "android-studio", "current", "Android", "Sdk"),
				filepath.Join(home, ".var", "app", "com.google.AndroidStudio", "config", "Android", "Sdk"),
			)
		}
		out = append(out,
			"/opt/android-sdk",
			"/opt/android-sdk-linux",
			"/usr/lib/android-sdk",   // Debian/Ubuntu package
			"/usr/local/android-sdk", // manual install
			"/usr/local/share/android-commandlinetools",
			"/home/linuxbrew/.linuxbrew/share/android-commandlinetools",
		)
	}
	return out
}

// sdkDirNames are the directory names worth testing under a parent directory.
var sdkDirNames = []string{
	"Android/Sdk",
	"Android/sdk",
	"Library/Android/sdk",
	"android-sdk",
	"android-sdk-linux",
	"android-sdk-macosx",
	"android-sdk-windows",
	"Sdk",
	"sdk",
	"android-commandlinetools",
	"share/android-commandlinetools",
	"snap/android-studio/common/Android/Sdk",
	"snap/android-studio/current/Android/Sdk",
	".var/app/com.google.AndroidStudio/config/Android/Sdk",
	"Android/android-sdk",
}

// searchSDK probes likely parent directories for an SDK-shaped child. It only
// stats a few dozen paths, so it stays fast even on a large home directory.
func searchSDK() []string {
	var parents []string
	add := func(p string) {
		if p != "" && isDir(p) {
			parents = append(parents, p)
		}
	}
	if home, err := os.UserHomeDir(); err == nil {
		add(home)
		add(filepath.Join(home, "Android"))
		add(filepath.Join(home, "Library", "Android"))
	}
	switch runtime.GOOS {
	case "windows":
		add(os.Getenv("LOCALAPPDATA"))
		add(os.Getenv("ProgramFiles"))
		add(os.Getenv("ProgramFiles(x86)"))
	case "darwin":
		add("/opt/homebrew")
		add("/usr/local")
	default:
		add("/opt")
		add("/usr/local")
		add("/usr/lib")
		add("/usr/share")
		if home, err := os.UserHomeDir(); err == nil {
			add(filepath.Join(home, ".local", "share"))
			add(filepath.Join(home, "snap"))
		}
	}

	var out []string
	seen := map[string]bool{}
	for _, parent := range parents {
		for _, name := range sdkDirNames {
			p := filepath.Join(parent, filepath.FromSlash(name))
			if seen[p] {
				continue
			}
			seen[p] = true
			if LooksLikeSDK(p) {
				out = append(out, p)
			}
		}
	}
	return out
}

// findAVDHome runs the full AVD directory discovery sequence.
func findAVDHome(override string) *discovery {
	d := newDiscovery(i18n.T("kind.avd_home"))
	d.add(override, OriginFlag, "--avd-home")

	// ANDROID_AVD_HOME names the directory itself.
	if v := os.Getenv("ANDROID_AVD_HOME"); v != "" {
		d.add(v, OriginEnv, "$ANDROID_AVD_HOME")
	}
	// ANDROID_USER_HOME names the parent of the avd directory, and defaults to
	// ~/.android.
	if v := os.Getenv("ANDROID_USER_HOME"); v != "" {
		d.add(filepath.Join(v, "avd"), OriginEnv, "$ANDROID_USER_HOME/avd")
	}
	// ANDROID_PREFS_ROOT and the older ANDROID_SDK_HOME both name a directory
	// containing .android.
	for _, name := range []string{"ANDROID_PREFS_ROOT", "ANDROID_SDK_HOME"} {
		if v := os.Getenv(name); v != "" {
			d.add(filepath.Join(v, ".android", "avd"), OriginEnv, "$"+name+"/.android/avd")
		}
	}
	for _, p := range platformAVDDefaults() {
		d.add(p, OriginDefault, i18n.T("src.default", runtime.GOOS))
	}
	for _, p := range searchAVDHome() {
		d.add(p, OriginSearch, i18n.T("src.search", filepath.Dir(p)))
	}
	// Prefer a directory that actually holds AVD definitions.
	d.finish(isDir, rankAVDHome)
	return d
}

// rankAVDHome prefers a directory that holds AVD definitions, so that an empty
// ~/.android/avd does not hide a populated $ANDROID_AVD_HOME.
func rankAVDHome(dir string) (int, string) {
	n := countAVDs(dir)
	if n == 0 {
		return 0, i18n.T("note.no_avds")
	}
	return n, i18n.T("note.avd_count", n)
}

// platformAVDDefaults lists the conventional AVD locations per operating system.
func platformAVDDefaults() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = ""
	}
	var out []string
	if home != "" {
		// ~/.android/avd is the default on every platform, Windows included:
		// Android Studio does not use %LOCALAPPDATA% for AVD definitions.
		out = append(out, filepath.Join(home, ".android", "avd"))
	}
	if runtime.GOOS == "windows" {
		if local := os.Getenv("LOCALAPPDATA"); local != "" {
			out = append(out, filepath.Join(local, "Android", "avd"))
		}
	}
	if home != "" && runtime.GOOS != "windows" {
		out = append(out,
			filepath.Join(home, "snap", "android-studio", "common", ".android", "avd"),
			filepath.Join(home, "snap", "android-studio", "current", ".android", "avd"),
			filepath.Join(home, ".var", "app", "com.google.AndroidStudio", ".android", "avd"),
		)
	}
	return out
}

// avdDirNames are the relative names worth testing when searching for an AVD
// directory under a candidate parent.
var avdDirNames = []string{
	".android/avd",
	"avd",
	"snap/android-studio/common/.android/avd",
	".var/app/com.google.AndroidStudio/.android/avd",
}

// searchAVDHome probes likely parents for an AVD-shaped child.
func searchAVDHome() []string {
	var parents []string
	add := func(p string) {
		if p != "" && isDir(p) {
			parents = append(parents, p)
		}
	}
	if home, err := os.UserHomeDir(); err == nil {
		add(home)
		add(filepath.Join(home, ".android"))
	}
	if runtime.GOOS == "windows" {
		add(os.Getenv("LOCALAPPDATA"))
		add(os.Getenv("USERPROFILE"))
	}
	var out []string
	seen := map[string]bool{}
	for _, parent := range parents {
		for _, name := range avdDirNames {
			p := filepath.Join(parent, filepath.FromSlash(name))
			if seen[p] || !isDir(p) {
				continue
			}
			seen[p] = true
			out = append(out, p)
		}
	}
	return out
}

// countAVDs counts the .ini files in a directory, which is how many AVDs it
// defines.
func countAVDs(dir string) int {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".ini") {
			n++
		}
	}
	return n
}

// chosenExists reports whether the selected candidate is a directory on disk.
func (d *discovery) chosenExists() bool {
	return d.chosen >= 0 && d.candidates[d.chosen].Exists
}

// chosenValid reports whether the selected candidate has the expected shape.
func (d *discovery) chosenValid() bool {
	return d.chosen >= 0 && d.candidates[d.chosen].Valid
}

// WarnFunc receives non-fatal discovery warnings as a catalogue key plus its
// arguments. The CLI sets it, so this package does not depend on the output or
// translation layers.
var WarnFunc = func(key string, a ...any) {}

// warnInvalid reports a selected candidate that does not have the expected
// shape.
func (d *discovery) warnInvalid(key string) {
	WarnFunc(key, d.path())
}
