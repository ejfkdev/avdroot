package cmd

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ejfkdev/avdroot/internal/i18n"
	"github.com/ejfkdev/avdroot/internal/ui"
)

// This file decides which Magisk installer to use: where to look for one, where
// to put a downloaded one, and what to say when neither is possible.

// magiskZIPName is the conventional installer file name.
const magiskZIPName = "Magisk.zip"

// downloadTimeout bounds a whole download. Releases are around 12 MB, so this
// only trips when the network is unusable.
const downloadTimeout = 5 * time.Minute

// cacheDir returns where a downloaded installer is kept.
//
// It is the operating system's cache directory — ~/Library/Caches on macOS,
// $XDG_CACHE_HOME or ~/.cache on Linux, %LocalAppData% on Windows — because an
// installer is reproducible: it can be deleted at any time and fetched again.
// Keeping it out of the configuration directory means "clear the cache" tools
// reclaim the 12 MB without touching anything the user configured.
func cacheDir() string {
	if base, err := os.UserCacheDir(); err == nil {
		return filepath.Join(base, "avdroot")
	}
	return filepath.Join(os.TempDir(), "avdroot")
}

// cacheDirs lists the cache locations to search, primary first.
func cacheDirs() []string {
	var dirs []string
	if xdg := os.Getenv("XDG_CACHE_HOME"); xdg != "" {
		dirs = append(dirs, filepath.Join(xdg, "avdroot"))
	}
	dirs = append(dirs, cacheDir())
	return dirs
}

// configDirs lists the configuration directories, primary first. They are kept
// in the search path so an installer a user placed there by hand still works.
func configDirs() []string {
	var dirs []string
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		dirs = append(dirs, filepath.Join(xdg, "avdroot"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(home, ".config", "avdroot"))
	}
	if cfg, err := os.UserConfigDir(); err == nil {
		dirs = append(dirs, filepath.Join(cfg, "avdroot"))
	}
	return dirs
}

// magiskSearchPaths lists where an installer is looked for, in order.
//
// The cache comes first: it holds what "magisk fetch" or an automatic download
// put there, so a freshly fetched installer is the one that gets used rather
// than an older file left in a configuration directory.
func (a *app) magiskSearchPaths() []string {
	if a.magiskZIP != "" {
		return []string{a.magiskZIP}
	}
	var out []string
	if v := os.Getenv("AVDROOT_MAGISK"); v != "" {
		out = append(out, v)
	}
	for _, dir := range cacheDirs() {
		out = append(out, filepath.Join(dir, magiskZIPName))
	}
	for _, dir := range configDirs() {
		out = append(out, filepath.Join(dir, magiskZIPName))
	}
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		out = append(out, filepath.Join(dir, magiskZIPName))
		// A binary built into bin/ usually sits next to the installer.
		out = append(out, filepath.Join(filepath.Dir(dir), magiskZIPName))
	}
	if wd, err := os.Getwd(); err == nil {
		out = append(out, filepath.Join(wd, magiskZIPName))
	}
	if home, err := os.UserHomeDir(); err == nil {
		out = append(out, filepath.Join(home, "Downloads", magiskZIPName))
	}
	return out
}

// findMagiskZIP looks for an installer without fetching anything. Commands that
// only report, such as "magisk info", use this so inspecting never downloads.
func (a *app) findMagiskZIP() (string, error) {
	paths := a.magiskSearchPaths()
	for _, p := range paths {
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			return p, nil
		}
	}
	return "", i18n.Errorf("err.no_installer", magiskZIPName, joinLines(paths))
}

// downloadAllowed reports whether a missing installer may be fetched.
func (a *app) downloadAllowed() bool { return !a.noDownload }

// ensureMagiskZIP returns a usable installer, downloading one when none is
// present.
//
// Every command that needs the payload goes through here, so a first run on a
// clean machine works without a separate step. A failure is not fatal to the
// explanation: it prints where to get the file by hand and where to put it.
func (a *app) ensureMagiskZIP() (string, error) {
	if p, err := a.findMagiskZIP(); err == nil {
		return p, nil
	}
	dest := filepath.Join(cacheDir(), magiskZIPName)
	if !a.downloadAllowed() {
		ui.Warn(i18n.T("magisk.none_found"))
		printManualInstall(dest)
		return "", i18n.Errorf("err.no_installer_disabled", dest)
	}

	ui.Step(i18n.T("magisk.auto_fetching"))
	url, label, err := resolveInstallerURL("", false)
	if err != nil {
		// No version could be discovered, so there is no URL to try. Say what
		// to do instead rather than guessing an asset name that will not exist.
		ui.Warn(i18n.T("magisk.api_failed", err))
		printManualInstall(dest)
		return "", i18n.Errorf("err.download_failed")
	}
	ui.Info(i18n.T("magisk.downloading", label))
	ui.Infof("%s", url)

	n, err := download(url, dest)
	if err != nil {
		ui.Warn(i18n.T("magisk.download_failed", err))
		printManualInstall(dest)
		return "", i18n.Errorf("err.download_failed")
	}
	ui.OK(i18n.T("magisk.saved", dest, bytesHuman(n)))

	// Validate before returning, so a truncated or wrong archive is caught
	// here rather than after the ramdisk has been read.
	abi := "arm64-v8a"
	if t, err := a.sdk.Resolve(""); err == nil && t.ABI != "" {
		abi = t.ABI
	}
	p, err := a.payloadFrom(dest, abi)
	if err != nil {
		return "", i18n.Errorf("err.archive_unusable", err)
	}
	ui.OK(i18n.T("magisk.verified", p.Description(), p.APIRange()))
	return dest, nil
}

// resolveInstallerURL works out what to download.
//
// An explicit tag needs no API call, because the asset name is derived from the
// tag. "Latest" does: release assets are named Magisk-v<version>.apk, so the
// version has to be discovered first.
func resolveInstallerURL(version string, prerelease bool) (url, label string, err error) {
	if version != "" {
		label = version
		if label[0] != 'v' {
			label = "v" + label
		}
		return directReleaseURL(version), label, nil
	}
	rel, err := latestRelease(prerelease)
	if err != nil {
		return "", "", err
	}
	name, assetURL, _, ok := rel.apkAsset()
	if !ok {
		return "", "", fmt.Errorf("release %s has no installer asset", rel.Tag)
	}
	_ = name
	return assetURL, fmt.Sprintf("%s (%s, %s)", rel.Tag, rel.Kind(), rel.PublishedDate()), nil
}

// printManualInstall explains how to supply an installer by hand. It is printed
// whenever automatic download is unavailable or fails.
func printManualInstall(dest string) {
	ui.Linef("\n%s", ui.Bold(i18n.T("magisk.manual_header")))
	ui.Info(i18n.T("magisk.manual_1", MagiskReleasesURL))
	ui.Info(i18n.T("magisk.manual_2"))
	ui.Info(i18n.T("magisk.manual_3"))
	ui.Info(i18n.T("magisk.manual_cmd"))
	ui.Info(i18n.T("magisk.manual_or", dest))
}

// downloadAttempts is how many times a transfer is tried. Reaching GitHub's
// release CDN is unreliable on some networks, and the failures are transient:
// the same URL that times out once often succeeds on the next attempt.
const downloadAttempts = 3

// download fetches url into dest, retrying failures that are likely to be
// temporary.
func download(url, dest string) (int64, error) {
	var lastErr error
	for attempt := 1; attempt <= downloadAttempts; attempt++ {
		if attempt > 1 {
			delay := time.Duration(attempt-1) * 4 * time.Second
			ui.Detail(i18n.T("magisk.retrying", attempt, downloadAttempts, delay))
			time.Sleep(delay)
		}
		n, err := downloadOnce(url, dest)
		if err == nil {
			return n, nil
		}
		lastErr = err
		if !worthRetrying(err) {
			break
		}
	}
	return 0, lastErr
}

// worthRetrying reports whether a failed transfer may succeed if repeated. A
// missing release never will, so it is not retried.
func worthRetrying(err error) bool {
	if err == nil {
		return false
	}
	// A missing release will never appear on a retry.
	return !strings.Contains(err.Error(), "HTTP 404")
}

// downloadOnce performs a single transfer, writing to a temporary file first so
// an interrupted attempt never leaves a usable-looking archive behind.
func downloadOnce(url, dest string) (int64, error) {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return 0, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), downloadTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, err
	}
	// GitHub rejects requests without a User-Agent.
	req.Header.Set("User-Agent", "avdroot")

	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return 0, fmt.Errorf("HTTP 404: no release with that tag")
	default:
		return 0, fmt.Errorf("HTTP %s", resp.Status)
	}

	tmp := dest + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		return 0, err
	}
	n, err := io.Copy(f, resp.Body)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(tmp)
		return 0, err
	}
	// A release is around 12 MB; anything tiny is an error page.
	if n < 1<<20 {
		os.Remove(tmp)
		return 0, fmt.Errorf("the download was only %s, which is too small to be a release", bytesHuman(n))
	}
	if err := os.Rename(tmp, dest); err != nil {
		os.Remove(tmp)
		return 0, err
	}
	return n, nil
}

// joinLines renders paths one per line, indented, for an error message.
func joinLines(items []string) string {
	out := ""
	for i, s := range items {
		if i > 0 {
			out += "\n"
		}
		out += "  " + s
	}
	return out
}
