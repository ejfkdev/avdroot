package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ejfkdev/avdroot/internal/adb"
	"github.com/ejfkdev/avdroot/internal/avd"
	"github.com/ejfkdev/avdroot/internal/emulator"
	"github.com/ejfkdev/avdroot/internal/i18n"
	"github.com/ejfkdev/avdroot/internal/ui"
)

func newDoctorCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:     "doctor",
		Short:   i18n.T("doctor.short"),
		Example: i18n.T("doctor.example"),
		Long:    i18n.T("doctor.long"),
		Args:    cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireNoArgs(cmd, args); err != nil {
				return err
			}
			ui.Line(ui.Bold(i18n.T("doctor.host")))
			ui.Fieldf(i18n.T("doctor.os"), "%s/%s", runtime.GOOS, runtime.GOARCH)
			ui.Field(i18n.T("doctor.go_runtime"), runtime.Version())
			ui.Field(i18n.T("doctor.home"), homeOrUnknown())

			ui.Linef("\n%s", ui.Bold(i18n.T("list.sdk_heading")))
			ui.Field(i18n.T("doctor.selected"), a.sdk.Root)
			ui.Field(i18n.T("doctor.found_via"), a.sdk.SDKPathSource())
			ui.Field(i18n.T("doctor.markers"), describeSDKMarkers(a.sdk.Root))
			printCandidates(a.sdk.SDKPathCandidates())

			ui.Linef("\n%s", ui.Bold(i18n.T("doctor.avd_heading")))
			if a.sdk.AVDHome == "" {
				ui.Field(i18n.T("doctor.selected"), ui.Dim(i18n.T("doctor.none_found")))
			} else {
				ui.Field(i18n.T("doctor.selected"), a.sdk.AVDHome)
				ui.Field(i18n.T("doctor.found_via"), a.sdk.AVDHomePathSource())
			}
			printCandidates(a.sdk.AVDHomeCandidates())

			ui.Linef("\n%s", ui.Bold(i18n.T("doctor.tools")))
			printTool("adb", adbPath(a))
			printTool("emulator", emulatorPath(a))

			ui.Linef("\n%s", ui.Bold(i18n.T("doctor.env")))
			for _, name := range []string{
				"ANDROID_HOME", "ANDROID_SDK_ROOT", "ANDROID_USER_HOME",
				"ANDROID_AVD_HOME", "ANDROID_PREFS_ROOT", "ANDROID_SDK_HOME",
				"AVDROOT_MAGISK", "JAVA_HOME",
			} {
				if v := os.Getenv(name); v != "" {
					ui.Field(name, v)
				} else {
					ui.Field(name, ui.Dim(i18n.T("doctor.unset")))
				}
			}

			ui.Linef("\n%s", ui.Bold(i18n.T("doctor.magisk")))
			if p, err := a.findMagiskZIP(); err == nil {
				ui.Field(i18n.T("doctor.selected"), p)
			} else {
				ui.Field(i18n.T("doctor.selected"), ui.Dim(i18n.T("doctor.none_found")))
				ui.Info(i18n.T("doctor.fetch_hint"))
			}
			// Where an automatic download would land, so the cache can be
			// found and cleared without reading the documentation.
			ui.Field(i18n.T("doctor.cache"), cacheDir())

			// The AVD list is the practical outcome of all of the above.
			avds, err := a.sdk.ListAVDs()
			// A snapshot can shadow a patched ramdisk, so its presence is part
			// of the diagnosis.
			if avds, err := a.sdk.ListAVDs(); err == nil {
				for _, av := range avds {
					snaps := av.Snapshots()
					if len(snaps) == 0 {
						continue
					}
					ui.Field(i18n.T("field.snapshots"), strings.Join(snaps, ", "))
					if av.FastBoot() {
						ui.Field(i18n.T("field.fastboot"), ui.Bold(i18n.T("bool.yes"))+
							" (a snapshot may restore the pre-patch ramdisk)")
					}
				}
			}

			ui.Linef("\n%s", ui.Bold(i18n.T("doctor.result")))
			if err != nil {
				ui.Field("avds", ui.Dim(i18n.T("doctor.error", err)))
				return nil
			}
			if len(avds) == 0 {
				ui.Field("avds", ui.Dim(i18n.T("doctor.no_avds")))
				ui.Info(i18n.T("doctor.no_avds_hint"))
			}
			for _, av := range avds {
				ramdisk := av.RamdiskPath()
				if ramdisk == "" {
					ramdisk = ui.Dim(i18n.T("doctor.no_ramdisk"))
				}
				ui.Fieldf(av.Name, "%s  %s", flavor(&avd.Target{API: av.API(), Release: av.Release()}), ramdisk)
			}
			return nil
		},
	}
}

func homeOrUnknown() string {
	if h, err := os.UserHomeDir(); err == nil {
		return h
	}
	return "(unknown)"
}

// describeSDKMarkers lists which SDK marker directories are present.
func describeSDKMarkers(root string) string {
	var found []string
	for _, m := range []string{"platform-tools", "system-images", "cmdline-tools", "emulator", "platforms", "build-tools", "licenses"} {
		if fi, err := os.Stat(filepath.Join(root, m)); err == nil && fi.IsDir() {
			found = append(found, m)
		}
	}
	if len(found) == 0 {
		return ui.Dim(i18n.T("doctor.markers_none"))
	}
	return strings.Join(found, ", ")
}

// printCandidates renders the discovery trace.
func printCandidates(cands []avd.Candidate) {
	if len(cands) == 0 {
		ui.Linef("  %s", ui.Dim(i18n.T("doctor.no_candidates")))
		return
	}
	ui.Linef("  %s", i18n.T("doctor.candidates"))
	for _, c := range cands {
		state := i18n.T("doctor.state_missing")
		switch {
		case c.Exists && c.Valid:
			state = i18n.T("doctor.state_ok")
			if c.Note != "" {
				state = c.Note
			}
		case c.Exists:
			state = i18n.T("doctor.state_exists")
		}
		ui.Linef("    [%s] %s", state, c.Path)
		ui.Linef("          %s", ui.Dim(c.Source))
	}
}

func printTool(name, path string) {
	if path == "" {
		ui.Field(name, ui.Dim(i18n.T("doctor.not_found")))
		return
	}
	ui.Field(name, path)
	if out, err := exec.Command(path, "version").Output(); err == nil {
		line := strings.TrimSpace(strings.Split(strings.TrimSpace(string(out)), "\n")[0])
		ui.Fieldf(name, "%s", ui.Dim(line))
	}
}

func adbPath(a *app) string {
	if a.adbCli != nil {
		return a.adbCli.Path
	}
	if p, err := adb.Find(a.sdk.Root); err == nil {
		return p
	}
	return ""
}

func emulatorPath(a *app) string {
	if p, err := emulator.Binary(a.sdk.Root); err == nil {
		return p
	}
	return ""
}
