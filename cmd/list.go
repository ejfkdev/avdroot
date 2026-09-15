package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/ejfkdev/avdroot/internal/avd"
	"github.com/ejfkdev/avdroot/internal/i18n"
	"github.com/ejfkdev/avdroot/internal/ui"
)

func newListCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Short:   i18n.T("list.short"),
		Example: i18n.T("list.example"),
		Args:    cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireNoArgs(cmd, args); err != nil {
				return err
			}
			ui.Info(i18n.T("list.sdk", a.sdk.Root))
			ui.Info(i18n.T("list.avd_dir", a.sdk.AVDHome))
			ui.Printf("\n")

			avds, err := a.sdk.ListAVDs()
			if err != nil {
				return err
			}
			if len(avds) == 0 {
				ui.Warn(i18n.T("list.no_avds"))
			} else {
				ui.Line(ui.Bold(i18n.T("list.avds")))
				t := &ui.Table{Headers: []string{
					i18n.T("table.name"), i18n.T("table.api"), i18n.T("table.abi"),
					i18n.T("table.status"), i18n.T("table.backup"), i18n.T("table.image"),
				}}
				for _, av := range avds {
					status, backup, image := i18n.T("list.no_ramdisk"), "-", "-"
					if av.Image != nil {
						image = av.Image.Tag
					}
					info := inspect(&avd.Target{
						RamdiskPath: av.RamdiskPath(),
						Name:        av.Name,
						ABI:         av.ABI(),
						API:         av.API(),
						AVD:         av,
					})
					if info.Target.RamdiskPath != "" {
						status = statusText(info.Status)
						if len(info.Errs) > 0 {
							status = i18n.T("list.unreadable")
						}
						if info.HasBackup {
							backup = statusText(info.BackupStatus)
						}
					}
					for _, e := range info.Errs {
						ui.Warnf("%s: %v", av.Name, e)
					}
					api := "-"
					if av.API() > 0 {
						api = itoa(av.API())
					}
					t.Add(av.Name, api, av.ABI(), status, backup, image)
				}
				t.Render()
			}

			images, err := a.sdk.ListSystemImages()
			if err != nil {
				return err
			}
			if len(images) > 0 {
				ui.Linef("\n%s", ui.Bold(i18n.T("list.images")))
				t := &ui.Table{Headers: []string{
					i18n.T("table.path"), i18n.T("table.abi"), i18n.T("table.status"), i18n.T("table.backup"),
				}}
				for _, img := range images {
					info := inspect(&avd.Target{RamdiskPath: img.Ramdisk, Name: img.RelPath})
					status, backup := "unreadable", "-"
					if len(info.Errs) == 0 {
						status = statusText(info.Status)
					}
					if info.HasBackup {
						backup = statusText(info.BackupStatus)
					}
					t.Add(img.RelPath, img.ABI, status, backup)
				}
				t.Render()
			}

			ui.Line(ui.Dim(i18n.T("list.next_hint")))
			return nil
		},
	}
}

func newStatusCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:     "status [avd|image|ramdisk]",
		Short:   i18n.T("status.short"),
		Example: i18n.T("status.example"),
		Args:    cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			t, err := a.requireTarget(args)
			if err != nil {
				return err
			}
			info := inspect(t)

			ui.Line(ui.Bold(i18n.T("field.target")))
			ui.Field(i18n.T("field.name"), t.Name)
			ui.Field(i18n.T("field.ramdisk"), t.RamdiskPath)
			ui.Field(i18n.T("field.abi"), t.ABI)
			ui.Field(i18n.T("field.android"), flavor(t))

			if len(info.Errs) > 0 {
				ui.Linef("\n%s", ui.Bold(i18n.T("field.problems")))
				for _, e := range info.Errs {
					ui.Printf("  %v\n", e)
				}
				return nil
			}

			ui.Linef("\n%s", ui.Bold(i18n.T("field.ramdisk")))
			ui.Field(i18n.T("field.status"), statusText(info.Status))
			ui.Field(i18n.T("field.compression"), info.Format.String())
			ui.Field(i18n.T("field.size"), bytes(info.Size))
			ui.Fieldf(i18n.T("field.entries"), "%d", info.Entries)
			ui.Field(i18n.T("field.backup"), backupText(info))

			// A re-patch needs the preinit device, so surface what the device
			// reports while it is available.
			if a.adbCli != nil {
				if serial, err := a.adbCli.Online(); err == nil {
					ui.Linef("\n%s", ui.Bold(i18n.T("field.emulator")))
					ui.Field(i18n.T("field.serial"), serial)
					for _, prop := range []string{"ro.build.version.sdk", "ro.product.cpu.abi", "ro.crypto.state", "ro.crypto.type"} {
						if v, err := a.adbCli.GetProp(prop); err == nil && v != "" {
							ui.Field(prop, v)
						}
					}
					if dev, err := a.adbCli.PreinitDevice(); err == nil {
						if dev == "" {
							dev = "(none found)"
						}
						ui.Field(i18n.T("field.preinit"), dev)
					}
				}
			}

			if t.AVD != nil && t.AVD.PlayStore() {
				ui.Printf("\n")
				ui.Warn(i18n.T("warn.playstore"))
			}
			return nil
		},
	}
}

func backupText(info *targetInfo) string {
	if !info.HasBackup {
		return i18n.T("backup.none")
	}
	return i18n.T("backup.present", statusText(info.BackupStatus))
}

// bytes renders a size in a compact human form.
func bytes(n int64) string {
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
