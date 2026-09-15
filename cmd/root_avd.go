package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/ejfkdev/avdroot/internal/adb"
	"github.com/ejfkdev/avdroot/internal/avd"
	"github.com/ejfkdev/avdroot/internal/emulator"
	"github.com/ejfkdev/avdroot/internal/i18n"
	"github.com/ejfkdev/avdroot/internal/ui"
)

func newRootAVDCmd(a *app) *cobra.Command {
	var (
		yes         bool
		noInstall   bool
		emulatorArg string
		timeout     time.Duration
		force       bool
	)
	cmd := &cobra.Command{
		Use:     "root [avd|image|ramdisk]",
		Short:   i18n.T("root.short"),
		Example: i18n.T("root.example"),
		Long:    i18n.T("root.long"),
		Args:    cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			t, err := a.requireTarget(args)
			if err != nil {
				return err
			}
			if a.adbCli == nil {
				return fmt.Errorf("adb is required for \"avdroot root\"; use \"avdroot patch\" instead")
			}

			avdName := a.avdNameFor(t)
			ui.Step(i18n.T("field.target"))
			ui.Infof("%s  %s", t.Name, describeTargetChoice(len(args) > 0, t))
			ui.Info(i18n.T("field.ramdisk_is", t.RamdiskPath))

			// The preinit device is read from the running device, so the
			// emulator has to be up before the ramdisk is written. Starting it
			// here is what makes this usable from a cold start.
			ui.Step(i18n.T("field.emulator"))
			if serial, err := a.adbCli.Online(); err == nil {
				ui.Info(i18n.T("root.already_running", serial))
				if !emulator.IsEmulator(a.adbCli) {
					return fmt.Errorf("the connected device is not the Android emulator; " +
						"run \"avdroot patch\" and reboot it yourself")
				}
			} else {
				if avdName == "" {
					return fmt.Errorf("no emulator is running and the AVD name is unknown; "+
						"start %s, or name the AVD explicitly", t.Name)
				}
				ui.Info(i18n.T("root.not_running"))
				ui.Detailf("%s", emulator.StartCommand(a.sdk.Root, avdName, emulator.Options{}))
				started, err := emulator.EnsureRunning(a.adbCli, a.sdk.Root, avdName, emulator.Options{}, timeout)
				if err != nil {
					return err
				}
				if started {
					ui.OK(i18n.T("root.started"))
				}
			}

			if !yes && !confirm("patch this ramdisk and restart the emulator?") {
				return fmt.Errorf("cancelled")
			}

			if err := runPatch(a, t, patchFlags{
				force:          force,
				keepVerity:     true,
				keepEncrypt:    true,
				assumeYes:      true, // confirmed above
				quietNextSteps: true,
			}); err != nil {
				return err
			}

			ui.Step(i18n.T("root.restarting"))
			// A saved snapshot carries the pre-patch memory image, so say so
			// when one is present rather than letting the cold boot look
			// arbitrary.
			if t.AVD != nil {
				if snaps := t.AVD.Snapshots(); len(snaps) > 0 {
					if t.AVD.FastBoot() {
						ui.Warn(i18n.T("root.snapshot_fastboot", strings.Join(snaps, ", ")))
					} else {
						ui.Info(i18n.T("root.snapshot_present", strings.Join(snaps, ", ")))
					}
					ui.Info(i18n.T("root.cold_boot"))
				}
			}
			if avdName == "" {
				return fmt.Errorf("cannot determine the AVD name to restart; " +
					"restart the emulator manually and then run \"avdroot verify\"")
			}
			// Reuse the options the running emulator was started with, so a
			// headless or writable-system launch is preserved.
			restartArgs := emulator.RunningQEMUArgs(avdName)
			if emulatorArg != "" {
				restartArgs = strings.Fields(emulatorArg)
			}
			// A cold boot is mandatory here: a snapshot would restore the
			// memory image, and with it the ramdisk that was loaded before the
			// patch, which would make the patch silently ineffective.
			command, err := emulator.Restart(a.adbCli, a.sdk.Root, avdName, emulator.Options{
				Args:          restartArgs,
				ForceColdBoot: true,
			})
			if err != nil {
				return err
			}
			ui.Detailf("%s", command)
			ui.Info(i18n.T("root.waiting_boot"))
			if err := emulator.WaitForBoot(a.adbCli, timeout); err != nil {
				return err
			}
			ui.OK(i18n.T("root.booted"))

			if !noInstall {
				if err := installMagiskApp(a); err != nil {
					ui.Warnf("%v", err)
				}
				if err := ensureMagiskEnvironment(a, timeout); err != nil {
					ui.Warnf("%v", err)
				}
			}
			if err := grantShellSu(a); err != nil {
				ui.Warnf("%v", err)
			}

			return reportRoot(a, t)
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, i18n.T("flag.yes"))
	cmd.Flags().BoolVar(&noInstall, "no-install", false, i18n.T("flag.no_install"))
	cmd.Flags().BoolVar(&force, "force", false, i18n.T("flag.force"))
	cmd.Flags().StringVar(&emulatorArg, "emulator-args", "", i18n.T("flag.emulator_args"))
	cmd.Flags().DurationVar(&timeout, "timeout", 6*time.Minute, i18n.T("flag.timeout"))
	return cmd
}

// avdNameFor works out which AVD to start or restart.
func (a *app) avdNameFor(t *avd.Target) string {
	if name := emulator.RunningAVD(a.adbCli); name != "" {
		return name
	}
	if t.AVD != nil {
		return t.AVD.Name
	}
	// A system image was named rather than an AVD; if exactly one AVD uses it,
	// that is the one to operate on.
	if avds, err := a.sdk.ListAVDs(); err == nil {
		var matches []string
		for _, av := range avds {
			if av.RamdiskPath() == t.RamdiskPath {
				matches = append(matches, av.Name)
			}
		}
		if len(matches) == 1 {
			return matches[0]
		}
	}
	return ""
}

// describeTargetChoice explains why this target was chosen, so that acting on a
// single AVD without being asked is not surprising.
func describeTargetChoice(named bool, t *avd.Target) string {
	if named {
		return ui.Dim(i18n.T("root.target_note_named"))
	}
	if t.AVD != nil {
		return ui.Dim(i18n.T("root.target_note_only_avd"))
	}
	return ui.Dim(i18n.T("root.target_note_only_image"))
}

// setupDialogTimeout bounds how long the Magisk app is given to put its setup
// prompt on screen after it is opened. A device that is already set up never
// shows one, so this bounds a case that normally resolves in seconds.
const setupDialogTimeout = 45 * time.Second

// ensureMagiskEnvironment makes the Magisk app unpack the files Magisk needs.
//
// Patching a ramdisk installs Magisk's init and its daemon, but not the rest of
// Magisk: those ship inside the manager app and are written to /data/adb/magisk
// only when the app runs its first-start setup. Until that has happened and the
// device has restarted, "su" on PATH is the emulator's own su, which reads
// "-c" as a uid and fails with "invalid uid/gid '-c'". Magisk looks perfectly
// healthy the whole time — it reports its version and accepts policy writes —
// which is what makes this worth a step of its own rather than a longer wait.
func ensureMagiskEnvironment(a *app, timeout time.Duration) error {
	ui.Step(i18n.T("root.env_setup"))

	// The check below needs root, and gaining it here rather than in
	// grantShellSu saves a restart of adbd. It is handed back on the way out:
	// a root adbd would make every later su check pass on adbd's authority
	// instead of Magisk's, which is a verification of nothing.
	defer dropAdbdRoot(a)

	installed, err := a.adbCli.MagiskEnvInstalled()
	if err != nil {
		return err
	}
	if installed {
		ui.OK(i18n.T("root.env_ready"))
		return nil
	}
	ui.Info(i18n.T("root.env_missing"))

	// An unanswered permission prompt would be the only thing on screen when
	// the setup dialog is looked for, so it is settled before the app opens.
	if err := a.adbCli.AllowNotifications(adb.MagiskPackage); err != nil {
		return err
	}
	if err := a.adbCli.LaunchApp(adb.MagiskPackage); err != nil {
		return err
	}
	ui.Detail(i18n.T("root.env_opening"))

	// The boot id must be read before the dialog is answered, because answering
	// it is what triggers the restart.
	before, err := a.adbCli.BootID()
	if err != nil {
		return err
	}

	label, err := answerSetupDialog(a.adbCli, setupDialogTimeout)
	if err != nil {
		return err
	}
	if label == "" {
		ui.Warn(i18n.T("root.env_no_dialog"))
		ui.Info(i18n.T("root.env_no_dialog_hint"))
		return nil
	}
	ui.OK(i18n.T("root.env_answered", label))
	ui.Info(i18n.T("root.env_rebooting"))

	if err := waitForNewBoot(a.adbCli, before, timeout); err != nil {
		return err
	}
	ui.OK(i18n.T("root.env_rebooted"))
	return nil
}

// answerSetupDialog waits for the Magisk app to ask about setting itself up and
// presses the button that accepts. It returns the label it pressed, or an empty
// string when no prompt ever appeared.
//
// Only a dialog the Magisk app owns is answered, so a system prompt cannot be
// dismissed by mistake, and the button is chosen by position rather than by
// label because Magisk ships ninety translations.
func answerSetupDialog(cli *adb.Client, timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if pkg, err := cli.ForegroundPackage(); err == nil && pkg == adb.MagiskPackage {
			if nodes, err := cli.UIDump(); err == nil {
				if btn, ok := adb.FindAffirmativeButton(nodes, adb.MagiskPackage); ok {
					label := adb.ButtonLabel(nodes, btn)
					x, y := btn.Bounds.Center()
					if err := cli.Tap(x, y); err != nil {
						return "", err
					}
					return label, nil
				}
			}
		}
		time.Sleep(2 * time.Second)
	}
	return "", nil
}

// waitForNewBoot blocks until the device has restarted and finished booting.
// Waiting for a boot is not the same thing: the device is up already, so it is
// the boot id that tells a fresh boot apart from the current one.
func waitForNewBoot(cli *adb.Client, previous string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		time.Sleep(2 * time.Second)
		id, err := cli.BootID()
		if err != nil {
			continue // the device is down, which is what a restart looks like
		}
		if id != previous {
			return emulator.WaitForBoot(cli, time.Until(deadline))
		}
	}
	return fmt.Errorf("the emulator did not restart within %s", timeout)
}

// dropAdbdRoot returns adbd to the normal shell if it is running as root.
//
// Root held by adbd is what makes an su check meaningless: every command
// succeeds regardless of what Magisk does. Anywhere root is taken for a
// purpose, it is given back when that purpose is served.
func dropAdbdRoot(a *app) {
	if !a.adbCli.IsShellRoot() {
		return
	}
	if _, err := a.adbCli.Run("unroot"); err != nil {
		return
	}
	// adbd restarts, so the device needs a moment before it answers again.
	time.Sleep(2 * time.Second)
	_, _ = a.adbCli.Run("wait-for-device")
	_, _ = a.adbCli.WaitReady(30 * time.Second)
}

// grantShellSu allows the adb shell to run su without a tap in the Magisk app.
//
// Magisk asks for a decision the first time a uid requests root, and that
// question cannot be answered from a script. Writing the policy row is the
// supported way to pre-approve it, and the daemon reads the row for every
// request, so no restart is needed.
func grantShellSu(a *app) error {
	ui.Step(i18n.T("root.granting_su"))

	if granted, err := a.adbCli.SuGrantedToShell(); err == nil && granted {
		ui.OK(i18n.T("root.su_already", adb.AIDShell))
		return nil
	}

	// Writing the policy database needs root, which the emulator grants
	// through "adb root".
	if err := a.adbCli.EnsureRoot(); err != nil {
		return fmt.Errorf("cannot write the su policy: %w", err)
	}
	if _, err := a.adbCli.WaitReady(30 * time.Second); err != nil {
		return err
	}
	if err := a.adbCli.SetSuPolicy(adb.AIDShell, adb.SuPolicyAllow); err != nil {
		return err
	}
	ui.OK(i18n.T("root.su_allowed", adb.AIDShell))

	// adbd is root now. Drop back to the normal shell so the check that
	// follows exercises Magisk's su rather than the emulator's own root shell.
	// This changes the state the user's own tooling sees, so it is announced.
	dropAdbdRoot(a)
	if a.adbCli.IsShellRoot() {
		ui.Detail(i18n.T("root.adbd_still_root"))
	} else {
		ui.Detail(i18n.T("root.adbd_unrooted"))
	}
	return nil
}

// reportRoot performs the final check and prints the uid line.
func reportRoot(a *app, t *avd.Target) error {
	ui.Step(i18n.T("root.verifying"))

	if a.adbCli == nil {
		return fmt.Errorf("adb is required to verify root")
	}
	// Installing an APK or restarting adbd can leave the device briefly
	// unavailable, so tolerate that; but if nothing is connected at all, say so
	// at once rather than pausing for the whole timeout.
	if _, err := a.adbCli.WaitReady(30 * time.Second); err != nil {
		return i18n.Errorf("err.no_emulator", err)
	}

	version, err := a.adbCli.MagiskVersion()
	if err != nil {
		ui.Warn(i18n.T("root.magisk_not_running", err))
		ui.Info(i18n.T("root.maybe_stale"))
		return err
	}
	ui.OK(i18n.T("root.magisk_running", version))

	if t != nil && t.ABI != "" {
		if p, err := a.payload(t.ABI); err == nil && p.Version != "" && !strings.HasPrefix(version, p.Version) {
			ui.Warn(i18n.T("root.version_mismatch", version, p.Version))
			ui.Info(i18n.T("root.reboot_not_enough"))
		}
	}

	// Root held by adbd would make the check below pass whatever Magisk does,
	// so it is given up first: the question is whether Magisk grants root, not
	// whether the emulator does.
	if a.adbCli.IsShellRoot() {
		ui.Info(i18n.T("root.dropping_adbd_root"))
		dropAdbdRoot(a)
	}
	if a.adbCli.IsShellRoot() {
		// Nothing can be concluded here, because every command already runs as
		// root. Say so rather than reporting a success that was not tested.
		ui.Warn(i18n.T("root.shell_root"))
		ui.Info(i18n.T("root.shell_root_hint"))
		return nil
	}

	out, err := a.adbCli.SuID()
	switch {
	case err != nil:
		ui.Warn(i18n.T("root.su_not_run", err))
		ui.Info(i18n.T("root.su_hint"))
		return fmt.Errorf("root is not available")
	case !strings.Contains(out, "uid=0"):
		ui.Warn(i18n.T("root.su_no_root", out))
		return fmt.Errorf("root is not available")
	}

	ui.OK(i18n.T("root.su_ok"))
	ui.Printf("\n%s\n", ui.Bold("root: "+out))
	return nil
}

func newVerifyCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:     "verify [avd|image|ramdisk]",
		Short:   i18n.T("verify.short"),
		Example: i18n.T("verify.example"),
		Args:    cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			t, err := a.requireTarget(args)
			if err != nil {
				return err
			}
			return reportRoot(a, t)
		},
	}
}

func newInstallCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:     "install",
		Short:   i18n.T("install.short"),
		Example: i18n.T("install.example"),
		Args:    cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireNoArgs(cmd, args); err != nil {
				return err
			}
			if a.adbCli == nil {
				return fmt.Errorf("adb is required to install the app")
			}
			if _, err := a.adbCli.Online(); err != nil {
				return fmt.Errorf("no running emulator: %w", err)
			}
			return installMagiskApp(a)
		},
	}
}

// installMagiskApp installs the Magisk manager app.
//
// A Magisk release archive is a valid APK, so the same file serves both
// purposes. adb streams the package from the host and insists on a .apk
// filename, so the installer is copied to a temporary .apk when needed; a
// device-side path would not work with "adb install".
func installMagiskApp(a *app) error {
	ui.Step(i18n.T("install.installing"))
	installer, err := a.ensureMagiskZIP()
	if err != nil {
		return err
	}
	abi := a.abi
	if abi == "" {
		if t, err := a.sdk.Resolve(""); err == nil {
			abi = t.ABI
		}
	}
	var version string
	if p, err := a.payload(abi); err == nil {
		version = p.Version
	}

	apk := installer
	if !strings.EqualFold(filepath.Ext(installer), ".apk") {
		staged, err := stageAsAPK(installer)
		if err != nil {
			return err
		}
		defer os.Remove(staged)
		apk = staged
	}

	out, err := a.adbCli.Install(apk)
	if err != nil {
		return fmt.Errorf("installing the Magisk app: %w: %s", err, firstLine(out))
	}
	if pkg, ok := a.adbCli.PackageInstalled("magisk"); ok {
		if version != "" {
			ui.OK(i18n.T("install.done", pkg, version))
		} else {
			ui.OK(i18n.T("install.done_nover", pkg))
		}
		return nil
	}
	ui.Warn(i18n.T("install.no_package"))
	return nil
}

// stageAsAPK copies the installer to a temporary file with a .apk extension so
// that "adb install" accepts it.
func stageAsAPK(src string) (string, error) {
	in, err := os.Open(src)
	if err != nil {
		return "", err
	}
	defer in.Close()

	tmp, err := os.CreateTemp("", "Magisk-*.apk")
	if err != nil {
		return "", err
	}
	defer tmp.Close()
	if _, err := io.Copy(tmp, in); err != nil {
		os.Remove(tmp.Name())
		return "", err
	}
	return tmp.Name(), nil
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// confirm asks a yes/no question, and answers yes automatically when there is
// no terminal to ask, so pipelines and CI keep working.
func confirm(question string) bool {
	fi, err := os.Stdin.Stat()
	if err != nil || fi.Mode()&os.ModeCharDevice == 0 {
		return true
	}
	ui.Printf("%s [Y/n] ", question)
	var answer string
	if _, err := fmt.Scanln(&answer); err != nil {
		// An empty line, or EOF, means accept.
		return true
	}
	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "", "y", "yes":
		return true
	default:
		return false
	}
}
