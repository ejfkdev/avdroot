package cmd

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ejfkdev/avdroot/internal/avd"
	"github.com/ejfkdev/avdroot/internal/cpio"
	"github.com/ejfkdev/avdroot/internal/i18n"
	"github.com/ejfkdev/avdroot/internal/imgfmt"
	"github.com/ejfkdev/avdroot/internal/magisk"
	"github.com/ejfkdev/avdroot/internal/ramdisk"
	"github.com/ejfkdev/avdroot/internal/ui"
)

func newPatchCmd(a *app) *cobra.Command {
	var (
		force        bool
		keepVerity   bool
		keepEncrypt  bool
		recoveryMode bool
		yes          bool
		dryRun       bool
	)
	cmd := &cobra.Command{
		Use:     "patch [avd|image|ramdisk]",
		Short:   i18n.T("patch.short"),
		Example: i18n.T("patch.example"),
		Long:    i18n.T("patch.long"),
		Args:    cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			t, err := a.requireTarget(args)
			if err != nil {
				return err
			}
			return runPatch(a, t, patchFlags{
				force:        force,
				keepVerity:   keepVerity,
				keepEncrypt:  keepEncrypt,
				recoveryMode: recoveryMode,
				assumeYes:    yes,
				dryRun:       dryRun,
				targetNamed:  len(args) > 0,
			})
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, i18n.T("flag.force"))
	cmd.Flags().BoolVar(&keepVerity, "keep-verity", true, i18n.T("flag.keep_verity"))
	cmd.Flags().BoolVar(&keepEncrypt, "keep-forceencrypt", true, i18n.T("flag.keep_encrypt"))
	cmd.Flags().BoolVar(&recoveryMode, "recovery-mode", false, i18n.T("flag.recovery"))
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, i18n.T("flag.yes"))
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, i18n.T("flag.dry_run"))
	return cmd
}

type patchFlags struct {
	force        bool
	keepVerity   bool
	keepEncrypt  bool
	recoveryMode bool
	// assumeYes skips the confirmation, either because the user passed --yes
	// or because a caller such as "root" already asked.
	assumeYes bool
	// dryRun stops before the ramdisk is written.
	dryRun bool
	// quietNextSteps suppresses the closing advice, which is noise when a
	// caller such as "root" already describes what happens next.
	quietNextSteps bool
	// targetNamed records that the user named the target, so the output can
	// say so instead of always claiming it was picked automatically.
	targetNamed bool
}

// runPatch performs the whole patch, from backup through to writing the image.
func runPatch(a *app, target *avd.Target, f patchFlags) error {
	ui.Step(i18n.T("patch.checking_target"))
	ui.Infof("%s  %s", target.Name, describeTargetChoice(f.targetNamed, target))
	ui.Info(i18n.T("field.ramdisk_is", target.RamdiskPath))
	if target.AVD != nil && target.AVD.PlayStore() {
		return i18n.Errorf("err.playstore_unsupported", target.AVD.Name)
	}
	// A snapshot holds the guest memory, including the ramdisk loaded when it
	// was taken, so a normal launch could boot the pre-patch image and make the
	// patch look ineffective. "root" restarts cold; say so when patching alone.
	if target.AVD != nil {
		if snaps := target.AVD.Snapshots(); len(snaps) > 0 {
			if target.AVD.FastBoot() {
				ui.Warn(i18n.T("patch.snapshot_fastboot", strings.Join(snaps, ", ")))
			} else {
				ui.Info(i18n.T("patch.snapshot_present", strings.Join(snaps, ", ")))
			}
			ui.Info(i18n.T("patch.snapshot_hint"))
		}
	}

	info := inspect(target)
	if len(info.Errs) > 0 {
		return fmt.Errorf("reading ramdisk: %w", info.Errs[0])
	}
	ui.Infof("%s, %s, %s", flavor(target), info.Format, statusText(info.Status))

	if info.Status == cpio.StatusUnsupported && !f.force {
		return fmt.Errorf("%s was patched by an unsupported program; "+
			"restore a stock image first (avdroot restore) or repatch the system image", target.RamdiskPath)
	}
	if info.Status == cpio.StatusMagisk && !f.force {
		ui.Warn(i18n.T("patch.already_patched"))
		ui.Detail(i18n.T("patch.force_hint"))
	}

	// Load the payload before touching the image, so a missing installer
	// cannot leave the ramdisk half processed.
	ui.Step(i18n.T("patch.loading_magisk"))
	p, err := a.payload(target.ABI)
	if err != nil {
		return err
	}
	ui.OKf("%s", p.Description())
	ui.Detail(i18n.T("patch.supports", p.APIRange(), strings.Join(p.ABIs, ", ")))

	// A release that cannot run on this Android version would produce a
	// ramdisk that boots without root, so refuse before touching anything.
	if warning, err := p.CheckAPI(target.API); err != nil {
		return fmt.Errorf("%w\nDownload a newer release from https://github.com/topjohnwu/Magisk/releases\n"+
			"or pass --magisk /path/to/Magisk.apk", err)
	} else if warning != "" {
		ui.Warnf("%s", warning)
	}

	ui.Step(i18n.T("patch.reading"))
	payload, format, err := ramdisk.Load(target.RamdiskPath)
	if err != nil {
		return err
	}
	if a.compress != "" {
		override, err := imgfmt.ParseFormat(a.compress)
		if err != nil {
			return err
		}
		ui.Info(i18n.T("patch.compress_override", format, override))
		format = override
	}
	fi, err := os.Stat(target.RamdiskPath)
	if err != nil {
		return err
	}
	ui.OK(i18n.T("patch.uncompressed", format, bytes(int64(len(payload)))))

	// Record the digest of the stock image so the Magisk app can identify it.
	sha := ""
	if sum := sha1.Sum(payload); len(sum) > 0 {
		sha = hex.EncodeToString(sum[:])
	}

	ui.Step(i18n.T("patch.patching"))
	detected := a.detectPreinit()
	opts := magisk.Options{
		KeepVerity:       f.keepVerity,
		KeepForceEncrypt: f.keepEncrypt,
		RecoveryMode:     f.recoveryMode || target.API == 28,
		PreinitDevice:    detected,
		SHA1:             sha,
	}
	res, err := magisk.Patch(payload, p, opts)
	if err != nil {
		return err
	}
	ui.Info(i18n.T("patch.prev_state", res.PreviousStatus))
	for _, name := range res.Added {
		ui.Detail(i18n.T("patch.added", name))
	}
	for _, name := range res.BackedUp {
		ui.Detail(i18n.T("patch.backed_up", name))
	}
	reportPreinit(detected, res.Options.PreinitDevice)

	patched, err := res.Archive.Bytes()
	if err != nil {
		return err
	}

	// Everything above is in memory; nothing on disk has changed yet.
	ui.Step(i18n.T("patch.summary"))
	backupPath := ramdisk.BackupPath(target.RamdiskPath)
	if _, err := os.Stat(backupPath); err != nil {
		ui.Info(i18n.T("patch.will_save", backupPath))
	} else {
		ui.Info(i18n.T("patch.backup_kept", backupPath))
		// An earlier rootAVD run can leave a backup that is itself patched, in
		// which case restore would not return to stock.
		if st, _, err := ramdisk.BackupStatus(target.RamdiskPath); err == nil && st == cpio.StatusMagisk {
			ui.Warn(i18n.T("patch.backup_is_patched"))
		}
	}
	ui.Info(i18n.T("patch.will_overwrite", target.RamdiskPath))
	ui.Info(i18n.T("patch.plan", info.Entries, res.Archive.Len(), info.Format, format))

	if f.dryRun {
		ui.OK(i18n.T("patch.dry_run_done"))
		return nil
	}
	if !f.assumeYes && !confirm("write the patched ramdisk?") {
		return fmt.Errorf("cancelled; nothing was written")
	}

	// The backup is taken immediately before the write, so an aborted or
	// dry run never leaves one behind.
	ui.Step(i18n.T("patch.preserving"))
	created, err := ramdisk.EnsureBackup(target.RamdiskPath)
	if err != nil {
		return fmt.Errorf("creating backup: %w", err)
	}
	if created {
		ui.OK(i18n.T("patch.saved_backup", backupPath))
	} else {
		ui.Info(i18n.T("patch.backup_kept_short"))
	}

	ui.Step(i18n.T("patch.writing"))
	if err := ramdisk.Save(target.RamdiskPath, patched, format, fi.Mode().Perm()); err != nil {
		return fmt.Errorf("writing %s: %w", target.RamdiskPath, err)
	}
	ui.OK(i18n.T("patch.written", target.RamdiskPath, res.Archive.Len()))

	if f.quietNextSteps {
		return nil
	}

	ui.Linef("\n%s", ui.Bold(i18n.T("field.next")))
	if a.adbCli != nil {
		if _, err := a.adbCli.Online(); err == nil {
			ui.Info(i18n.T("patch.next_restart"))
			ui.Info(i18n.T("patch.next_root"))
			return nil
		}
	}
	ui.Info(i18n.T("patch.next_restart"))
	if target.AVD != nil {
		ui.Info(i18n.T("patch.next_root_cold"))
		ui.Info(i18n.T("patch.next_or_start"))
		ui.Infof("  %s", platformHint(a.sdk.Root, target.AVD.Name))
		ui.Info(i18n.T("patch.next_then"))
	}
	return nil
}

func newRestoreCmd(a *app) *cobra.Command {
	var listBackups bool
	cmd := &cobra.Command{
		Use:     "restore [avd|image|ramdisk]",
		Short:   i18n.T("restore.short"),
		Example: i18n.T("restore.example"),
		Args:    cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if listBackups {
				return listBackupFiles(a)
			}
			t, err := a.requireTarget(args)
			if err != nil {
				return err
			}
			backup := ramdisk.BackupPath(t.RamdiskPath)
			if _, err := os.Stat(backup); err != nil {
				return fmt.Errorf("no backup at %s; nothing to restore", backup)
			}
			if st, _, err := ramdisk.BackupStatus(t.RamdiskPath); err == nil && st == cpio.StatusMagisk {
				ui.Warn(i18n.T("restore.backup_patched", backup))
				ui.Warn(i18n.T("restore.keeps_patch"))
			}
			ui.Step(i18n.T("restore.restoring", t.RamdiskPath))
			ui.Info(i18n.T("restore.from", backup))
			if err := ramdisk.Restore(t.RamdiskPath); err != nil {
				return err
			}
			st, err := ramdisk.Status(t.RamdiskPath)
			if err != nil {
				return err
			}
			ui.OK(i18n.T("restore.done", statusText(st)))
			ui.Info(i18n.T("restore.backup_kept"))
			return nil
		},
	}
	cmd.Flags().BoolVar(&listBackups, "list", false, i18n.T("restore.list_flag"))
	return cmd
}

func listBackupFiles(a *app) error {
	type found struct{ target, backup string }
	var all []found
	add := func(p string) {
		if p == "" {
			return
		}
		if _, err := os.Stat(ramdisk.BackupPath(p)); err == nil {
			all = append(all, found{p, ramdisk.BackupPath(p)})
		}
	}
	if avds, err := a.sdk.ListAVDs(); err == nil {
		for _, av := range avds {
			add(av.RamdiskPath())
		}
	}
	if imgs, err := a.sdk.ListSystemImages(); err == nil {
		for _, img := range imgs {
			add(img.Ramdisk)
		}
	}
	if len(all) == 0 {
		ui.Info(i18n.T("restore.none"))
		return nil
	}
	t := &ui.Table{Headers: []string{i18n.T("table.ramdisk"), i18n.T("table.backup")}}
	for _, f := range all {
		t.Add(f.target, f.backup)
	}
	t.Render()
	return nil
}

// reportPreinit explains where the preinit device value came from, since a
// silently inherited value looks the same as a missing one in the output.
func reportPreinit(detected, effective string) {
	switch {
	case detected != "":
		ui.Detail(i18n.T("patch.preinit_detected", detected))
	case effective != "":
		ui.Detail(i18n.T("patch.preinit_inherited", effective))
	default:
		ui.Warn(i18n.T("patch.preinit_missing"))
	}
}
