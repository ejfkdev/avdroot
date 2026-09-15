package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/ejfkdev/avdroot/internal/i18n"
	"github.com/ejfkdev/avdroot/internal/magisk"
	"github.com/ejfkdev/avdroot/internal/ui"
)

func newMagiskCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "magisk",
		Short:   i18n.T("magisk.short"),
		Example: i18n.T("magisk.example"),
	}
	cmd.AddCommand(newMagiskInfoCmd(a), newMagiskFetchCmd(a))
	return cmd
}

func newMagiskInfoCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:     "info",
		Short:   i18n.T("magisk.info_short"),
		Example: i18n.T("magisk.info_example"),
		Args:    cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireNoArgs(cmd, args); err != nil {
				return err
			}
			zipPath, err := a.findMagiskZIP()
			if err != nil {
				return err
			}
			ui.Info(i18n.T("magisk.installer", zipPath))
			if fi, err := os.Stat(zipPath); err == nil {
				ui.Info(i18n.T("magisk.file_info", bytesHuman(fi.Size()), fi.ModTime().Format(time.RFC3339)))
			}

			// Report compatibility against the selected target when there is
			// one, since that is the question the user is really asking.
			abi := a.abi
			var targetAPI int
			var targetName string
			if t, err := a.sdk.Resolve(""); err == nil {
				if abi == "" {
					abi = t.ABI
				}
				targetAPI, targetName = t.API, t.Name
			}
			if abi == "" {
				abi = "arm64-v8a"
				ui.Detail(i18n.T("magisk.no_target_abi", abi))
			}

			p, err := magisk.LoadPayload(zipPath, abi)
			if err != nil {
				return err
			}
			ui.Detail(i18n.T("magisk.cache_hint", cacheDir()))
			ui.Printf("\n")
			ui.OKf("%s", p.Description())
			ui.Info(i18n.T("magisk.package", p.Package))
			ui.Info(i18n.T("magisk.supports", p.APIRange()))
			ui.Info(i18n.T("magisk.abis", strings.Join(p.ABIs, ", ")))
			ui.Info(i18n.T("magisk.magiskinit", bytesHuman(int64(len(p.MagiskInit)))))
			ui.Info(i18n.T("magisk.magisk", bytesHuman(int64(len(p.Magisk)))))
			ui.Info(i18n.T("magisk.initld", bytesHuman(int64(len(p.InitLD)))))
			ui.Info(i18n.T("magisk.stub", bytesHuman(int64(len(p.Stub)))))

			if targetAPI > 0 {
				if warning, err := p.CheckAPI(targetAPI); err != nil {
					ui.Printf("\n")
					ui.Warnf("%v", err)
					ui.Info(i18n.T("magisk.newer_hint", MagiskReleasesURL))
				} else if warning != "" {
					ui.Printf("\n")
					ui.Warnf("%s", warning)
				} else {
					ui.Printf("\n")
					ui.OK(i18n.T("magisk.compatible", targetName))
				}
			}
			return nil
		},
	}
}

func newMagiskFetchCmd(a *app) *cobra.Command {
	var (
		version    string
		dest       string
		prerelease bool
		list       bool
	)
	cmd := &cobra.Command{
		Use:     "fetch",
		Short:   i18n.T("magisk.fetch_short"),
		Example: i18n.T("magisk.fetch_example"),
		Long:    i18n.T("magisk.fetch_long"),
		Args:    cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireNoArgs(cmd, args); err != nil {
				return err
			}
			if list {
				return printReleases(prerelease)
			}
			if dest == "" {
				// The cache, not the configuration directory: an installer is
				// reproducible, so it belongs where clearing a cache reclaims it.
				dest = filepath.Join(cacheDir(), magiskZIPName)
			}

			url, label, err := resolveInstallerURL(version, prerelease)
			if err != nil {
				// Without a resolvable version there is no URL to try; the
				// asset name carries the version, so it cannot be guessed.
				ui.Warn(i18n.T("magisk.api_failed", err))
				printManualInstall(dest)
				return i18n.Errorf("err.download_failed")
			}
			ui.Step(i18n.T("magisk.downloading", label))
			ui.Infof("%s", url)

			n, err := download(url, dest)
			if err != nil {
				ui.Warn(i18n.T("magisk.download_failed", err))
				printManualInstall(dest)
				return i18n.Errorf("err.download_failed")
			}
			ui.OK(i18n.T("magisk.saved", dest, bytesHuman(n)))

			// Validate immediately, so a bad or truncated download is caught
			// now rather than during a patch.
			abi := "arm64-v8a"
			var targetAPI int
			if t, err := a.sdk.Resolve(""); err == nil {
				if t.ABI != "" {
					abi = t.ABI
				}
				targetAPI = t.API
			}
			p, err := magisk.LoadPayload(dest, abi)
			if err != nil {
				return fmt.Errorf("the downloaded archive is not usable: %w", err)
			}
			ui.OK(i18n.T("magisk.verified", p.Description(), p.APIRange()))
			if warning, err := p.CheckAPI(targetAPI); err != nil {
				ui.Warnf("%v", err)
			} else if warning != "" {
				ui.Warnf("%s", warning)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&version, "version", "", i18n.T("flag.version"))
	cmd.Flags().StringVar(&dest, "dest", "", i18n.T("flag.dest"))
	cmd.Flags().BoolVar(&prerelease, "prerelease", false, i18n.T("flag.prerelease"))
	cmd.Flags().BoolVar(&list, "list", false, i18n.T("flag.list_releases"))
	return cmd
}

// download fetches url into dest, writing to a temporary file first so an
// interrupted transfer never leaves a usable-looking archive behind.

// bytesHuman renders a size compactly.
func bytesHuman(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for v := n / unit; v >= unit && exp < 3; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGT"[exp])
}
