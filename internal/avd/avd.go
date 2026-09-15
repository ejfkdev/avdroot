// Package avd locates Android SDK installations, AVDs and system images, and
// resolves them to the ramdisk image that needs patching.
//
// Unlike rootAVD, which requires the user to paste a path to a ramdisk.img,
// this package discovers everything from the SDK layout and the AVD's own
// configuration files, falling back to the system image's build.prop for the
// authoritative API level and ABI.
package avd

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// ErrNotFound reports that a requested AVD or image does not exist.
var ErrNotFound = errors.New("avd: not found")

// SDK is an Android SDK installation.
type SDK struct {
	// Root is the SDK directory holding system-images and platform-tools.
	Root string
	// AVDHome is the directory holding the AVD definitions.
	AVDHome string

	// The *Source fields explain how the corresponding path was located, and
	// the candidate lists record everything that was considered, so that
	// "avdroot doctor" can explain a surprising result.
	RootSource    string
	AVDHomeSource string
	sdkCandidates []Candidate
	avdCandidates []Candidate
}

// Entry describes how the SDK and AVD directory were found.
func (s *SDK) SDKPathSource() string { return s.RootSource }

// AVDHomePathSource describes how the AVD directory was found.
func (s *SDK) AVDHomePathSource() string { return s.AVDHomeSource }

// SDKPathCandidates lists every SDK location that was considered.
func (s *SDK) SDKPathCandidates() []Candidate { return s.sdkCandidates }

// AVDHomeCandidates lists every AVD location that was considered.
func (s *SDK) AVDHomeCandidates() []Candidate { return s.avdCandidates }

// Options selects where to look for the SDK and the AVD directory. An empty
// field falls back to environment variables, tools on PATH, and the
// conventional locations for the operating system.
type Options struct {
	// SDK overrides the Android SDK root.
	SDK string
	// AVDHome overrides the directory holding AVD definitions.
	AVDHome string
}

// Discover locates the SDK and the AVD directory.
func Discover(opts Options) (*SDK, error) {
	sdkFound := findSDK(opts.SDK)
	switch {
	case sdkFound.path() == "":
		return nil, fmt.Errorf("no Android SDK found.\nTried:\n%s\n"+
			"Set ANDROID_HOME, or pass --sdk /path/to/sdk", sdkFound.describe())
	case !sdkFound.chosenExists():
		// An explicit path that does not exist is an error rather than a
		// reason to fall back: silently using a different SDK could patch the
		// wrong emulator.
		return nil, fmt.Errorf("%s: %s is not a directory.\nTried:\n%s",
			sdkFound.source(), sdkFound.path(), sdkFound.describe())
	case !sdkFound.chosenValid():
		// Honoured, but the user probably meant something else.
		sdkFound.warnInvalid("platform-tools, system-images, cmdline-tools, emulator, platforms and licenses are all absent")
	}

	avdFound := findAVDHome(opts.AVDHome)
	switch {
	case avdFound.path() == "":
		// No AVD directory is not fatal: the user may only want to patch a
		// system image directly.
	case !avdFound.chosenExists():
		return nil, fmt.Errorf("%s: %s is not a directory.\nTried:\n%s",
			avdFound.source(), avdFound.path(), avdFound.describe())
	}

	return &SDK{
		Root:          sdkFound.path(),
		RootSource:    sdkFound.source(),
		AVDHome:       avdFound.path(),
		AVDHomeSource: avdFound.source(),
		sdkCandidates: sdkFound.Candidates(),
		avdCandidates: avdFound.Candidates(),
	}, nil
}

// DiscoverSDK finds the Android SDK, honouring ANDROID_HOME and
// ANDROID_SDK_ROOT before falling back to tools on PATH and the platform's
// conventional locations. A non-empty override wins outright, so an explicitly
// requested SDK is always used even if it looks unusual.
func DiscoverSDK(override string) (*SDK, error) {
	return Discover(Options{SDK: override})
}

func isDir(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}

// expandPath resolves a leading ~ and makes the path absolute.
func expandPath(p string) (string, error) {
	if strings.HasPrefix(p, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		p = filepath.Join(home, strings.TrimPrefix(strings.TrimPrefix(p, "~"), string(os.PathSeparator)))
	}
	return filepath.Abs(p)
}

// iniFile is a simple key=value reader for AVD configuration files, which use
// the same syntax as build.prop.
func iniFile(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	out := map[string]string{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		out[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}
	return out, sc.Err()
}

// BuildProp reads a build.prop style file. Missing files yield no error and an
// empty map, since every property has a fallback.
func BuildProp(path string) map[string]string {
	props, err := iniFile(path)
	if err != nil {
		return map[string]string{}
	}
	return props
}

// AVD is an Android Virtual Device definition.
type AVD struct {
	Name    string
	INIPath string
	Dir     string
	Config  map[string]string

	// Resolved from the system image referenced by the AVD.
	Image *SystemImage
}

// Sysdir returns the system image path recorded in the AVD configuration.
func (a *AVD) Sysdir() string {
	// image.sysdir.1 is the modern key; image.sysdir is the legacy one.
	for _, key := range []string{"image.sysdir.1", "image.sysdir.2", "image.sysdir"} {
		if v := a.Config[key]; v != "" {
			return strings.TrimSuffix(strings.TrimSpace(v), "/")
		}
	}
	return ""
}

// PlayStore reports whether the AVD uses a Play Store image, which forbids
// "adb root" and therefore cannot be patched the usual way.
func (a *AVD) PlayStore() bool {
	return strings.EqualFold(a.Config["PlayStore.enabled"], "true")
}

// ABI returns the ABI configured for the AVD, preferring the value recorded in
// the system image over the AVD's own copy.
func (a *AVD) ABI() string {
	if a.Image != nil && a.Image.ABI != "" {
		return a.Image.ABI
	}
	if v := a.Config["abi.type"]; v != "" {
		return v
	}
	return a.Config["hw.cpu.arch"]
}

// API returns the device API level.
func (a *AVD) API() int {
	if a.Image != nil {
		return a.Image.API
	}
	// "target=android-37.1" or "android-37"
	if t := a.Config["target"]; t != "" {
		return parseAPITarget(t)
	}
	return 0
}

// Release returns the Android version string, e.g. "17".
func (a *AVD) Release() string {
	if a.Image != nil {
		return a.Image.Release
	}
	return ""
}

// SnapshotsDir is where the emulator keeps saved VM states for this AVD.
func (a *AVD) SnapshotsDir() string {
	if a.Dir == "" {
		return ""
	}
	return filepath.Join(a.Dir, "snapshots")
}

// Snapshots lists the saved VM states, if any.
//
// A snapshot captures the emulator's memory, including the initrd it loaded at
// the time, so one taken before a ramdisk was patched can restore the old
// ramdisk and hide the patch. When fast boot is enabled the emulator may load
// one automatically on the next start.
func (a *AVD) Snapshots() []string {
	dir := a.SnapshotsDir()
	if dir == "" {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out
}

// FastBoot reports whether the AVD is configured to boot from a snapshot.
func (a *AVD) FastBoot() bool {
	if strings.EqualFold(a.Config["fastboot.forceColdBoot"], "yes") {
		return false
	}
	return strings.EqualFold(a.Config["fastboot.forceFastBoot"], "yes")
}

// RamdiskPath returns the ramdisk image belonging to the AVD, or "" when it
// cannot be determined.
func (a *AVD) RamdiskPath() string {
	if a.Image != nil {
		return a.Image.Ramdisk
	}
	return ""
}

// ListAVDs enumerates the AVD definitions, resolving each to its system image.
func (s *SDK) ListAVDs() ([]*AVD, error) {
	if s.AVDHome == "" {
		return nil, fmt.Errorf("no AVD directory found")
	}
	entries, err := os.ReadDir(s.AVDHome)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var out []*AVD
	for _, e := range entries {
		// Only the .ini files are definitions; .avd directories are data.
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".ini") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".ini")
		iniPath := filepath.Join(s.AVDHome, e.Name())
		ini, err := iniFile(iniPath)
		if err != nil {
			continue
		}
		dir := ini["path"]
		if dir == "" {
			dir = filepath.Join(s.AVDHome, name+".avd")
		}
		a := &AVD{Name: name, INIPath: iniPath, Dir: dir, Config: map[string]string{}}
		if cfg, err := iniFile(filepath.Join(dir, "config.ini")); err == nil {
			a.Config = cfg
		}
		// target= may only live in the .ini file.
		if a.Config["target"] == "" && ini["target"] != "" {
			a.Config["target"] = ini["target"]
		}
		if sysdir := a.Sysdir(); sysdir != "" {
			if img, err := s.LoadSystemImage(sysdir); err == nil {
				a.Image = img
			}
		}
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// FindAVD returns the AVD with the given name.
func (s *SDK) FindAVD(name string) (*AVD, error) {
	avds, err := s.ListAVDs()
	if err != nil {
		return nil, err
	}
	for _, a := range avds {
		if a.Name == name {
			return a, nil
		}
	}
	return nil, fmt.Errorf("%w: no AVD named %q", ErrNotFound, name)
}

// SystemImage is one installed system image, identified by its path relative to
// the SDK's system-images directory.
type SystemImage struct {
	Dir     string
	RelPath string
	ABI     string
	API     int
	Release string
	Tag     string
	Ramdisk string
	// Props holds the image's build.prop, the authoritative source for the
	// API level and ABI the emulator will report.
	Props map[string]string
}

// LoadSystemImage resolves a system image directory, which may be given as an
// absolute path or relative to the SDK root.
func (s *SDK) LoadSystemImage(dir string) (*SystemImage, error) {
	if dir == "" {
		return nil, fmt.Errorf("%w: empty system image path", ErrNotFound)
	}
	abs := dir
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(s.Root, filepath.FromSlash(dir))
	}
	abs = filepath.Clean(abs)
	if !isDir(abs) {
		return nil, fmt.Errorf("%w: system image %s", ErrNotFound, abs)
	}

	img := &SystemImage{Dir: abs}
	if rel, err := filepath.Rel(s.Root, abs); err == nil {
		img.RelPath = filepath.ToSlash(rel)
	}
	img.Props = BuildProp(filepath.Join(abs, "build.prop"))
	img.ABI = img.Props["ro.product.cpu.abi"]
	if v, err := strconv.Atoi(img.Props["ro.build.version.sdk"]); err == nil {
		img.API = v
	}
	img.Release = img.Props["ro.build.version.release"]

	// source.properties fills in whatever build.prop did not provide.
	if sp, err := iniFile(filepath.Join(abs, "source.properties")); err == nil {
		if img.ABI == "" {
			img.ABI = sp["SystemImage.Abi"]
		}
		if img.API == 0 {
			img.API = parseAPITarget(sp["AndroidVersion.ApiLevel"])
		}
		if img.Tag == "" {
			img.Tag = sp["SystemImage.TagId"]
		}
	}
	// The tag is encoded in the relative path:
	// system-images/android-37.1/google_apis_ps16k/arm64-v8a
	parts := strings.Split(filepath.ToSlash(img.RelPath), "/")
	if len(parts) >= 3 {
		if img.Tag == "" {
			img.Tag = parts[1]
		}
		if img.ABI == "" {
			img.ABI = parts[2]
		}
	}
	img.Ramdisk = findRamdisk(abs)
	return img, nil
}

// ListSystemImages enumerates the installed system images, skipping any that
// lack a ramdisk.
func (s *SDK) ListSystemImages() ([]*SystemImage, error) {
	root := filepath.Join(s.Root, "system-images")
	if !isDir(root) {
		return nil, nil
	}
	var out []*SystemImage
	// Layout: system-images/<api>/<tag>/<abi>/
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return nil
		}
		rel, rerr := filepath.Rel(root, path)
		if rerr != nil {
			return nil
		}
		// Depth 3 is exactly <api>/<tag>/<abi>.
		if len(strings.Split(filepath.ToSlash(rel), "/")) != 3 {
			return nil
		}
		if findRamdisk(path) == "" {
			return filepath.SkipDir
		}
		if img, lerr := s.LoadSystemImage(path); lerr == nil {
			out = append(out, img)
		}
		return filepath.SkipDir
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool { return out[i].RelPath < out[j].RelPath })
	return out, nil
}

// findRamdisk prefers ramdisk.img and ignores the copies users leave behind,
// which are common when a system image has been edited by hand or by a tool that
// keeps its own backup beside the original.
func findRamdisk(dir string) string {
	preferred := filepath.Join(dir, "ramdisk.img")
	if fi, err := os.Stat(preferred); err == nil && !fi.IsDir() {
		return preferred
	}
	return ""
}

// parseAPITarget extracts the leading integer from strings like "android-37.1",
// "37" or "android-28".
func parseAPITarget(s string) int {
	s = strings.TrimSpace(s)
	if i := strings.LastIndex(s, "-"); i >= 0 {
		s = s[i+1:]
	}
	// Trim any minor version: "37.1" -> "37".
	if i := strings.IndexByte(s, '.'); i >= 0 {
		s = s[:i]
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return n
}

// Target is a fully resolved patch target: somewhere a ramdisk.img exists,
// together with the properties needed to patch it.
type Target struct {
	// RamdiskPath is the absolute path to the ramdisk image to patch.
	RamdiskPath string
	// Name describes the target for messages.
	Name string
	ABI  string
	API  int
	// Release is the Android version string, when known.
	Release string
	// AVD and Image are set when the target came from an AVD definition or a
	// system image directory respectively.
	AVD   *AVD
	Image *SystemImage
}

// Resolve turns a user supplied selector into a Target. The selector may be an
// AVD name, a system image path (absolute or relative to the SDK), a directory
// containing ramdisk.img, or a path to a ramdisk file itself. An empty selector
// picks the only AVD, or the only system image when no AVD exists.
func (s *SDK) Resolve(selector string) (*Target, error) {
	if selector == "" {
		return s.resolveDefault()
	}

	// 1. A direct filesystem path always wins, so users can point at a copy.
	if looksLikePath(selector) {
		if t, err := s.resolvePath(selector); err == nil {
			return t, nil
		} else if !errors.Is(err, ErrNotFound) {
			return nil, err
		}
	}

	// 2. An AVD by name.
	if a, err := s.FindAVD(selector); err == nil {
		t := &Target{Name: a.Name, AVD: a, ABI: a.ABI(), API: a.API(), Release: a.Release()}
		t.RamdiskPath = a.RamdiskPath()
		if t.RamdiskPath == "" {
			return nil, fmt.Errorf("AVD %s has no system image with a ramdisk.img; check image.sysdir.1", a.Name)
		}
		if t.ABI == "" {
			t.ABI = a.Image.ABI
		}
		return t, nil
	}

	// 3. A system image, given relative to the SDK root.
	if img, err := s.LoadSystemImage(selector); err == nil {
		return s.targetFromImage(img)
	}

	return nil, fmt.Errorf("%w: %q is not an AVD name, system image or ramdisk path", ErrNotFound, selector)
}

// looksLikePath reports whether s is better treated as a path than a name.
func looksLikePath(s string) bool {
	return strings.ContainsRune(s, os.PathSeparator) ||
		strings.Contains(s, "/") ||
		strings.HasSuffix(s, ".img") ||
		strings.HasPrefix(s, "~")
}

func (s *SDK) resolvePath(p string) (*Target, error) {
	abs, err := expandPath(p)
	if err != nil {
		return nil, err
	}
	fi, err := os.Stat(abs)
	if err != nil {
		// Not an absolute/relative hit: retry relative to the SDK root, which
		// is how system images are usually quoted.
		if !filepath.IsAbs(p) {
			if alt := filepath.Join(s.Root, filepath.FromSlash(p)); isDir(alt) {
				abs = alt
				fi, err = os.Stat(abs)
			}
		}
		if err != nil {
			return nil, fmt.Errorf("%w: %s", ErrNotFound, abs)
		}
	}
	if fi.IsDir() {
		if img, err := s.LoadSystemImage(abs); err == nil && img.Ramdisk != "" {
			return s.targetFromImage(img)
		}
		rd := findRamdisk(abs)
		if rd == "" {
			return nil, fmt.Errorf("%w: no ramdisk.img in %s", ErrNotFound, abs)
		}
		return &Target{RamdiskPath: rd, Name: abs}, nil
	}
	// A file: only accept a ramdisk, since patching anything else is wrong.
	base := filepath.Base(abs)
	if !strings.HasPrefix(base, "ramdisk") || !strings.HasSuffix(base, ".img") {
		return nil, fmt.Errorf("%w: %s is not a ramdisk image", ErrNotFound, abs)
	}
	t := &Target{RamdiskPath: abs, Name: abs}
	// If the file sits in a system image, pick up its properties.
	if img, err := s.LoadSystemImage(filepath.Dir(abs)); err == nil {
		t.ABI, t.API, t.Release, t.Image = img.ABI, img.API, img.Release, img
	}
	return t, nil
}

func (s *SDK) targetFromImage(img *SystemImage) (*Target, error) {
	if img.Ramdisk == "" {
		return nil, fmt.Errorf("%w: no ramdisk.img in %s", ErrNotFound, img.Dir)
	}
	return &Target{
		RamdiskPath: img.Ramdisk,
		Name:        img.RelPath,
		ABI:         img.ABI,
		API:         img.API,
		Release:     img.Release,
		Image:       img,
	}, nil
}

func (s *SDK) resolveDefault() (*Target, error) {
	avds, err := s.ListAVDs()
	if err == nil && len(avds) == 1 {
		return s.Resolve(avds[0].Name)
	}
	if err == nil && len(avds) > 1 {
		names := make([]string, len(avds))
		for i, a := range avds {
			names[i] = a.Name
		}
		return nil, fmt.Errorf("several AVDs are defined (%s); name one explicitly", strings.Join(names, ", "))
	}
	imgs, err := s.ListSystemImages()
	if err != nil {
		return nil, err
	}
	switch len(imgs) {
	case 0:
		return nil, errors.New("no AVDs and no system images found")
	case 1:
		return s.targetFromImage(imgs[0])
	default:
		rel := make([]string, len(imgs))
		for i, im := range imgs {
			rel[i] = im.RelPath
		}
		return nil, fmt.Errorf("several system images are installed (%s); name one explicitly", strings.Join(rel, ", "))
	}
}
