package i18n

// english is the authoritative catalogue. Every key used anywhere must exist
// here; other languages fall back to it, and a test enforces the parity.
var english = map[string]string{
	// Application
	"app.name":    "avdroot",
	"app.tagline": "Root an Android Studio emulator by patching its ramdisk with Magisk",
	"app.long": `avdroot patches the ramdisk of an Android Studio AVD so that Magisk is
installed at boot, giving the emulator root access.

Everything is done on the host: the ramdisk is decompressed, patched in place
and recompressed by this binary. No shell, busybox or Magisk binary has to run
inside the emulator to build the image.

The patched image is written over ramdisk.img, and the original is preserved as
ramdisk.img.backup so "avdroot restore" can always undo the change.`,
	"app.example": `  # See which AVDs exist and whether they are already patched
  avdroot list

  # Root an emulator from a cold start, unattended
  avdroot root

  # Root a specific AVD, without the confirmation prompt
  avdroot root Pixel_10_Pro_XL --yes

  # Patch only; the emulator does not have to be running
  avdroot patch
  avdroot patch --dry-run          # show what would change

  # Undo it
  avdroot restore

  # Something looks wrong? Explain how every path was found
  avdroot doctor`,
	"help.repo":         "Repository: https://github.com/ejfkdev/avdroot\n",
	"help.usage":        "Usage:",
	"help.examples":     "Examples:\n",
	"help.commands":     "Available Commands:",
	"help.flags":        "Flags:\n",
	"help.global_flags": "Global Flags:\n",
	"help.more":         "Additional Commands:",
	"help.help_hint":    "Use \"{{.CommandPath}} [command] --help\" for more information about a command.",
	"help.version":      "Version ",

	"app.lang_hint": "output language follows $AVDROOT_LANG or the system locale; use --lang zh for Chinese",

	// Shared field labels
	"field.target":      "Target",
	"field.emulator":    "Emulator",
	"field.ramdisk":     "Ramdisk",
	"field.name":        "name",
	"field.abi":         "abi",
	"field.android":     "android",
	"field.status":      "status",
	"field.compression": "compression",
	"field.size":        "size on disk",
	"field.entries":     "cpio entries",
	"field.backup":      "backup",
	"field.serial":      "serial",
	"field.preinit":     "preinit device",
	"field.ramdisk_is":  "ramdisk: {0}",
	"field.problems":    "Problems",
	"field.next":        "Next steps",
	"field.snapshots":   "snapshots",
	"field.fastboot":    "fast boot",

	// list
	"list.short":      "List AVDs, installed system images and their patch status",
	"list.example":    "  avdroot list\n  avdroot list --lang zh",
	"list.sdk":        "Android SDK: {0}",
	"list.avd_dir":    "AVD directory: {0}",
	"list.avds":       "AVDs",
	"list.images":     "System images",
	"list.no_avds":    "no AVDs defined",
	"list.no_ramdisk": "no ramdisk",
	"list.unreadable": "unreadable",
	"list.next_hint":  "Next: \"avdroot patch\" to patch, \"avdroot root\" to patch and verify.",

	// status
	"status.short":   "Show detailed information about one target",
	"status.example": "  avdroot status\n  avdroot status Pixel_10_Pro_XL",

	// doctor
	"doctor.short": "Explain how the SDK, AVD directory, adb and emulator were located",
	"doctor.long": `doctor prints the environment avdroot resolved and every location it
considered. Use it when a path is detected wrongly, when no AVD is found, or
when something works on one machine but not another.

Every path can be overridden: --sdk, --avd-home, --serial, --magisk.`,
	"doctor.example":       "  avdroot doctor\n  avdroot doctor --sdk /custom/sdk",
	"doctor.host":          "Host",
	"doctor.package":       "package",
	"doctor.env":           "Environment variables",
	"list.sdk_heading":     "Android SDK",
	"doctor.avd_heading":   "AVD directory",
	"doctor.os":            "os",
	"doctor.go_runtime":    "go runtime",
	"doctor.home":          "home",
	"doctor.selected":      "selected",
	"doctor.found_via":     "found via",
	"doctor.markers":       "markers",
	"doctor.error":         "error: {0}",
	"doctor.no_avds":       "none",
	"doctor.magisk":        "Magisk installer",
	"doctor.result":        "Result",
	"doctor.none_found":    "(none found)",
	"doctor.unset":         "(unset)",
	"doctor.candidates":    "candidates:",
	"doctor.tools":         "Tools",
	"doctor.not_found":     "(not found)",
	"doctor.no_candidates": "no candidates were generated",
	"doctor.state_missing": "missing",
	"doctor.state_exists":  "exists, not recognised",
	"doctor.state_ok":      "ok",
	"doctor.markers_none":  "none (this may not be an SDK)",
	"doctor.fetch_hint":    "run \"avdroot magisk fetch\" or pass --magisk",
	"doctor.no_avds_hint":  "create one in Android Studio, or check the AVD directory above",
	"doctor.fastboot_warn": "yes (a snapshot may restore the pre-patch ramdisk)",

	// patch
	"patch.short": "Patch the ramdisk so Magisk is installed at boot",
	"patch.long": `patch decompresses the ramdisk, replaces /init with Magisk's init, embeds the
Magisk payload under overlay.d/sbin, and writes the result back in the original
compression format.

The pristine image is kept as ramdisk.img.backup the first time, and reused on
later runs, so "restore" always returns to the original.

dm-verity and forced encryption are left enabled by default, which is what an
emulator needs.

The ramdisk is modified in place, so a summary of what will change is shown and
confirmation is asked before writing. Pass --yes to skip the question (it is
skipped automatically when stdin is not a terminal), or --dry-run to stop
before anything is written.`,
	"patch.example": `  avdroot patch                     # the only AVD, after confirmation
  avdroot patch --dry-run           # show the plan, write nothing
  avdroot patch Pixel_10_Pro_XL -y  # named AVD, no prompt
  avdroot patch --preinit vdd1      # emulator not running: supply the device`,
	"patch.checking_target":        "Checking target",
	"patch.already_patched":        "this ramdisk is already Magisk patched; re-patching it",
	"patch.force_hint":             "pass --force to skip this warning",
	"patch.loading_magisk":         "Loading Magisk",
	"patch.supports":               "supports {0}, ships {1}",
	"patch.reading":                "Reading the ramdisk",
	"patch.compress_override":      "compression overridden: {0} -> {1}",
	"patch.uncompressed":           "{0}, {1} uncompressed",
	"patch.patching":               "Patching",
	"patch.prev_state":             "previous state: {0}",
	"patch.added":                  "added {0}",
	"patch.backed_up":              "backed up {0}",
	"patch.preinit_detected":       "preinit device: {0} (read from the running emulator)",
	"patch.preinit_inherited":      "preinit device: {0} (inherited from the previous patch)",
	"patch.preinit_override":       "preinit device: {0} (from --preinit)",
	"patch.preinit_missing":        "no preinit device recorded: Magisk will still boot, but module storage may fall back. Run while the emulator is up, or pass --preinit",
	"patch.preinit_none":           "no running emulator; cannot auto-detect the preinit device",
	"patch.preinit_query_failed":   "could not query the preinit device: {0}",
	"patch.preinit_none_on_device": "the emulator exposes no suitable preinit partition; Magisk will still boot but module storage may fall back to /data",
	"patch.summary":                "Summary",
	"patch.will_save":              "will save the original as {0}",
	"patch.backup_kept":            "existing backup kept: {0}",
	"patch.backup_kept_short":      "existing backup kept",
	"patch.backup_is_patched":      "the existing backup is itself Magisk patched; restoring from it will not return to a stock image",
	"patch.will_overwrite":         "will overwrite {0}",
	"patch.plan":                   "{0} cpio entries -> {1} entries, {2} -> {3} compressed",
	"patch.dry_run_done":           "dry run: nothing was written",
	"patch.preserving":             "Preserving the original ramdisk",
	"patch.saved_backup":           "saved {0}",
	"patch.writing":                "Writing the patched ramdisk",
	"patch.written":                "{0} written ({1} cpio entries)",
	"patch.next_restart":           "the emulator must be restarted (not merely rebooted) to use it:",
	"patch.next_root":              "  avdroot root      patches if needed, restarts, grants su and verifies",
	"patch.next_or_start":          "or start it yourself:",
	"patch.next_then":              "then run: avdroot install && avdroot verify",

	// restore
	"restore.short":          "Restore a ramdisk from its .backup copy",
	"restore.example":        "  avdroot restore\n  avdroot restore --list",
	"restore.restoring":      "Restoring {0}",
	"restore.from":           "from {0}",
	"restore.done":           "restored, ramdisk is now {0}",
	"restore.backup_kept":    "the backup file was kept; delete it manually to free space",
	"restore.backup_patched": "the backup at {0} is itself Magisk patched",
	"restore.keeps_patch":    "restoring will keep the patch in place",
	"restore.none":           "no ramdisk backups found",
	"restore.list_flag":      "list every ramdisk backup that was found",

	// root
	"root.short": "Root an emulator end to end, unattended",
	"root.long": `root performs the whole procedure without further input:

  1. start the emulator if it is not already running (needed to read the
     preinit block device from the live mount table)
  2. patch the ramdisk
  3. restart the emulator so the new ramdisk is used
  4. install the Magisk app
  5. grant the adb shell su access, so no tap in the Magisk app is needed
  6. verify that su returns uid=0

With a single AVD no target is needed. With several, name one.

Two details make the restart reliable. The emulator is restarted as a new
process rather than with "adb reboot", because a guest reboot reuses the ramdisk
the emulator loaded when it started. And the restart forces a cold boot with
-no-snapshot, because restoring a saved VM state would bring back the initrd
from before the patch.
Use "avdroot patch" to patch only, or "avdroot verify" to check a running one.
`,
	"root.example": `  avdroot root                  # the only AVD, with confirmation
  avdroot root --yes            # unattended
  avdroot root Pixel_10_Pro_XL  # a named AVD
  avdroot root --no-install     # skip the Magisk app
  avdroot root --emulator-args "-no-window -gpu swiftshader_indirect"`,
	"root.target_note_named":      "(named on the command line)",
	"root.target_note_only_avd":   "(the only AVD)",
	"root.target_note_only_image": "(the only system image)",
	"root.already_running":        "already running: {0}",
	"root.not_running":            "not running, starting it",
	"root.started":                "emulator started",
	"root.restarting":             "Restarting the emulator",
	"root.waiting_boot":           "waiting for the emulator to boot (this takes a minute or two)",
	"root.booted":                 "emulator booted",
	"root.snapshot_fastboot":      "this AVD is set to fast boot and has a saved state ({0})",
	"root.snapshot_present":       "saved emulator state present ({0})",
	"root.cold_boot":              "booting cold, because a snapshot would restore the pre-patch ramdisk",
	"root.granting_su":            "Granting su to the adb shell",
	"root.su_already":             "already granted for uid {0}",
	"root.su_allowed":             "uid {0} allowed",
	"root.adbd_still_root":        "adbd stayed root; the check below would pass trivially",
	"root.adbd_unrooted":          "adbd was returned to the normal shell, so the check below exercises Magisk's su; run \"adb root\" if you need it back",
	"root.verifying":              "Verifying root",
	"root.magisk_not_running":     "Magisk is not running: {0}",
	"root.maybe_stale":            "the emulator may not have restarted with the patched ramdisk",
	"root.magisk_running":         "Magisk {0} is running",
	"root.version_mismatch":       "the emulator runs Magisk {0} but the ramdisk contains {1}",
	"root.reboot_not_enough":      "the emulator has not restarted since patching; \"adb reboot\" is not enough",
	"root.shell_root":             "adb shell runs as root (adb root)",
	"root.su_not_run":             "su did not run: {0}",
	"root.su_hint":                "grant it with: avdroot root, or tap Allow in the Magisk app",
	"root.su_no_root":             "su did not grant root: {0}",
	"root.su_ok":                  "su grants root",
	"root.result":                 "root: {0}",

	// verify / install
	"verify.short":       "Check whether the running emulator grants root",
	"verify.example":     "  avdroot verify\n  adb shell su -c id",
	"install.short":      "Install the Magisk app into the running emulator",
	"install.example":    "  avdroot install",
	"install.installing": "Installing the Magisk app",
	"install.done":       "installed {0} (Magisk {1})",
	"install.done_nover": "installed {0}",
	"install.no_package": "the installer ran but no Magisk package is listed",

	// magisk
	"magisk.short":         "Manage the Magisk installer archive used for patching",
	"magisk.example":       "  avdroot magisk info\n  avdroot magisk fetch --list\n  avdroot magisk fetch --version v31.0",
	"magisk.info_short":    "Show which Magisk installer would be used, and what it supports",
	"magisk.info_example":  "  avdroot magisk info",
	"magisk.installer":     "installer: {0}",
	"magisk.file_info":     "size: {0}, modified {1}",
	"magisk.no_target_abi": "no target selected; reporting for {0}",
	"magisk.package":       "package      {0}",
	"magisk.supports":      "supports     {0}",
	"magisk.abis":          "abis         {0}",
	"magisk.magiskinit":    "magiskinit   {0}",
	"magisk.magisk":        "magisk       {0}",
	"magisk.initld":        "init-ld      {0}",
	"magisk.stub":          "stub apk     {0}",
	"magisk.compatible":    "compatible with {0}",
	"magisk.newer_hint":    "download a newer release: {0}",

	"magisk.fetch_short": "Download a Magisk installer archive",
	"magisk.fetch_long": `fetch downloads a Magisk release into the configuration directory, where
"patch" and "root" will find it automatically.

Any release can be requested by tag. Only download from the official releases
page: a tampered installer would be baked into the emulator's ramdisk.

If the download fails, fetch prints the releases page and the option to install
a file you obtained yourself:

  avdroot root --magisk /path/to/Magisk.apk`,
	"magisk.fetch_example": `  avdroot magisk fetch                    # newest stable release
  avdroot magisk fetch --list             # what is available
  avdroot magisk fetch --version v31.0    # a specific tag
  avdroot magisk fetch --prerelease       # include pre-releases`,
	"magisk.downloading":     "Downloading Magisk {0}",
	"magisk.asset":           "asset {0}",
	"magisk.api_failed":      "could not query the release list ({0})",
	"magisk.retrying":        "retrying the download (attempt {0} of {1}, waiting {2})",
	"magisk.saved":           "saved {0} ({1})",
	"magisk.verified":        "verified: {0} ({1})",
	"magisk.download_failed": "download failed: {0}",
	"magisk.manual_header":   "Fetch it manually",
	"magisk.manual_1":        "1. open {0}",
	"magisk.manual_2":        "2. download the Magisk-v<version>.apk asset you need",
	"magisk.manual_3":        "3. pass it straight to a command, no need to move it into place:",
	"magisk.manual_cmd":      "     avdroot root --magisk /path/to/Magisk-v31.0.apk",
	"magisk.manual_or":       "   or put it at {0}",
	"magisk.releases_failed": "could not list releases: {0}",
	"magisk.browse":          "browse them at {0}",
	"magisk.fetch_hint":      "Fetch one with: avdroot magisk fetch --version <tag>",
	"magisk.using":           "using {0} from {1}",

	"cmd.help_short":       "Help about any command",
	"cmd.help_long":        "Help provides help for any command in the application.\nSimply type {{.CommandPath}} help [path to command] for full details.",
	"cmd.completion_short": "Generate the autocompletion script for the specified shell",
	"cmd.completion_long":  "Generate the autocompletion script for avdroot for the specified shell.",

	// Flags
	"flag.sdk":             "Android SDK root (auto-detected from $ANDROID_HOME, adb on PATH, or the platform default)",
	"flag.avd_home":        "directory holding AVD definitions (defaults to $ANDROID_AVD_HOME or ~/.android/avd)",
	"flag.serial":          "adb device serial to use when several are connected",
	"flag.magisk":          "path to a Magisk installer zip",
	"flag.abi":             "override the detected ABI (e.g. arm64-v8a)",
	"flag.compress":        "override the ramdisk compression (gzip, lz4_legacy, lz4, xz)",
	"flag.preinit":         "override the preinit block device (normally auto-detected)",
	"flag.program_version": "print the version and exit",
	"flag.help":            "print help",
	"flag.no_adb":          "do not contact a running emulator",
	"flag.lang":            "output language: en or zh (default: the system locale)",
	"flag.yes":             "do not ask for confirmation",
	"flag.dry_run":         "report what would change without writing anything",
	"flag.force":           "re-patch even if the ramdisk is already patched",
	"flag.keep_verity":     "keep dm-verity/AVB options in fstab, on by default",
	"flag.keep_encrypt":    "keep forced-encryption options in fstab, on by default",
	"flag.recovery":        "patch for recovery mode (implied on API 28)",
	"flag.no_install":      "skip installing the Magisk app",
	"flag.emulator_args":   "launcher options to use instead of the detected ones",
	"flag.timeout":         "how long to wait for the emulator to boot (default: 6m)",
	"flag.version":         "Magisk version tag, e.g. v31.0 (default: the newest stable release)",
	"flag.dest":            "where to save the archive",
	"flag.prerelease":      "consider pre-releases when resolving the newest version",
	"flag.list_releases":   "list the available releases and exit",
	"flag.list_backups":    "list every ramdisk backup that was found",

	// Confirmation and prompts
	"prompt.patch": "patch this ramdisk and restart the emulator?",
	"prompt.write": "write the patched ramdisk?",

	// Errors surfaced by the CLI
	"err.no_sdk":               "no Android SDK found.\nTried:\n{0}\nSet ANDROID_HOME, or pass --sdk /path/to/sdk",
	"err.sdk_not_dir":          "{0}: {1} is not a directory.\nTried:\n{2}",
	"err.avdhome_not_dir":      "{0}: {1} is not a directory.\nTried:\n{2}",
	"err.sdk_suspect":          "{0} was used, but platform-tools, system-images, cmdline-tools, emulator, platforms and licenses are all absent",
	"err.read_ramdisk":         "reading ramdisk: {0}",
	"err.unsupported_patch":    "{0} was patched by an unsupported program; restore a stock image first (avdroot restore) or repatch the system image",
	"err.api_unsupported":      "{0}\nDownload a newer release from https://github.com/topjohnwu/Magisk/releases\nor pass --magisk /path/to/Magisk.apk",
	"err.cancelled_write":      "cancelled; nothing was written",
	"err.backup_failed":        "creating backup: {0}",
	"err.write_failed":         "writing {0}: {1}",
	"err.no_backup":            "no backup at {0}; nothing to restore",
	"err.target_not_found":     "{0} is not an AVD name, a system image or a ramdisk path, and no such file exists",
	"err.unexpected_args":      "{0} takes no arguments, got {1}",
	"err.too_many_targets":     "expected at most one target, got {0}",
	"err.no_installer":         "no {0} found; looked in:\n  {1}\nDownload one from https://github.com/topjohnwu/Magisk/releases and place it there,\nor pass --magisk /path/to/Magisk.apk, or run \"avdroot magisk fetch\"",
	"err.adb_required_root":    "adb is required for \"avdroot root\"; use \"avdroot patch\" instead",
	"err.not_emulator":         "the connected device is not the Android emulator; run \"avdroot patch\" and reboot it yourself",
	"err.no_emulator_named":    "no emulator is running and the AVD name is unknown; start {0}, or name the AVD explicitly",
	"err.cancelled":            "cancelled",
	"err.no_avd_name":          "cannot determine the AVD name to restart; restart the emulator manually and then run \"avdroot verify\"",
	"err.su_policy":            "cannot write the su policy: {0}",
	"err.adb_required_verify":  "adb is required to verify root",
	"err.no_emulator":          "no running emulator: {0}",
	"err.no_root":              "root is not available",
	"err.adb_required_install": "adb is required to install the app",
	"err.install_failed":       "installing the Magisk app: {1}: {0}",
	"err.no_release_asset":     "release {0} has no installer asset",
	"err.download_failed":      "could not download Magisk",
	"err.archive_unusable":     "the downloaded archive is not usable: {0}",
	"err.http_404":             "HTTP 404: no release with that tag",
	"err.http_other":           "HTTP {0}",
	"err.download_tiny":        "the download was only {0}, which is too small to be a release",
	"err.api_status":           "GitHub API returned {0}",
	"err.no_release":           "no downloadable release found",

	// Table headers and value fragments
	"table.name":             "NAME",
	"table.api":              "API",
	"table.abi":              "ABI",
	"table.status":           "STATUS",
	"table.backup":           "BACKUP",
	"table.image":            "IMAGE",
	"table.path":             "PATH",
	"table.ramdisk":          "RAMDISK",
	"table.tag":              "TAG",
	"table.channel":          "CHANNEL",
	"table.published":        "PUBLISHED",
	"table.size":             "SIZE",
	"flavor.api_and_release": "API {0} (Android {1})",
	"flavor.api":             "API {0}",
	"flavor.release":         "Android {0}",
	"flavor.unknown":         "unknown",

	"backup.present":            "present ({0})",
	"backup.none":               "none",
	"src.tool":                  "{0} on PATH",
	"src.default":               "default location for {0}",
	"src.search":                "found by scanning {0}",
	"note.has_images":           "has system-images",
	"note.no_images":            "no system-images",
	"note.avd_count":            "{0} AVD(s)",
	"note.no_avds":              "no AVD definitions",
	"kind.sdk":                  "an Android SDK",
	"kind.avd_home":             "an AVD directory",
	"doctor.state_unlike":       "exists, but does not look like {0}",
	"warn.playstore":            "this AVD uses a Play Store image: adb root is unavailable, so root can be patched in but the Magisk app cannot manage it over adb",
	"doctor.no_ramdisk":         "(no system image with a ramdisk)",
	"patch.next_root_cold":      "  avdroot root      does the whole sequence from a cold start",
	"patch.snapshot_fastboot":   "this AVD is set to fast boot and has a saved state ({0})",
	"patch.snapshot_present":    "saved emulator state present ({0})",
	"patch.snapshot_hint":       "a cold boot is required for the patch to take effect; \"avdroot root\" does this automatically. A stale snapshot can be deleted in Android Studio's device manager",
	"err.playstore_unsupported": "{0} uses a Play Store image, which refuses adb root and cannot be made writable; root cannot be installed this way",
	"magisk.auto_fetching":      "No Magisk installer found; downloading one",
	"magisk.none_found":         "no Magisk installer found, and downloading is disabled",
	"err.no_installer_disabled": "no Magisk installer available and --no-download was given; place one at {0} or pass --magisk",
	"flag.no_download":          "do not download a Magisk installer automatically",
	"doctor.cache":              "installer cache",
	"magisk.cache_hint":         "downloads are cached in {0}, which is safe to clear",
	// trust-chrome
	"trust.short": "Make Chrome trust a CA certificate on the emulator",
	"trust.long": `Chrome on Android enforces Certificate Transparency for anything that chains to a
certificate in the system store. A locally-generated interception CA has no
transparency log entry, so Chrome refuses it with
NET::ERR_CERT_AUTHORITY_INVALID, even though the rest of Android accepts it.

This command tells Chromium to trust that one key explicitly, by adding
--ignore-certificate-errors-spki-list to its startup flags. Chrome and the
WebView both read the flag from a file on the device, which is written here for
every path and variant they look at.

The certificate is taken from --cert, or detected from the CA certificates
installed on the device that are not part of the system image.

Export formats: PEM and DER are both accepted, regardless of the file
extension, so Reqable's .pem, .crt and .0 exports all work. A .p12 container
does not; export as PEM instead.`,
	"trust.example": `  avdroot trust-chrome                            # auto-detect the installed CA
  avdroot trust-chrome --cert ~/reqable-root.crt  # an explicit certificate
  avdroot trust-chrome --cert /data/local/tmp/ca.crt   # a path on the device
  avdroot trust-chrome --clear                    # undo it`,
	"trust.checking":            "Preparing to trust a CA in Chrome",
	"trust.cert_summary":        "certificate: {0}",
	"trust.cert_source":         "read from {0}",
	"trust.not_ca":              "this certificate is not a CA; interception will not work with it",
	"trust.spki":                "SPKI fingerprint: {0}",
	"trust.writing":             "Writing the Chromium startup flags",
	"trust.written":             "wrote {0} flag files",
	"trust.write_failed":        "could not write {0}: {1}",
	"trust.chmod_failed":        "could not set permissions on {0}",
	"trust.restarting":          "Restarting Chrome so it reads the flags",
	"trust.restarted":           "Chrome stopped; open it again to pick up the flags",
	"trust.restart_hint":        "Chrome must be restarted to read the flags",
	"trust.clearing":            "Removing the Chromium startup flags",
	"trust.cleared":             "flag files removed",
	"trust.caveat":              "Chrome will now accept certificates signed by this one key, without Certificate Transparency.",
	"trust.caveat_key":          "The fingerprint is tied to the key: if the interception tool regenerates its CA, run this again. Intended for emulators and test devices.",
	"err.adb_required_trust":    "trusting a certificate needs a running emulator; start one and try again",
	"err.root_required_trust":   "writing the Chromium flags needs root: {0}",
	"err.cert_unreadable":       "{0}: {1}",
	"err.no_candidate_ca":       "no CA certificate was found on the device outside the system image.\nLooked in:\n  {0}\nInstall the interception CA, or pass --cert /path/to/ca.crt",
	"err.multiple_candidate_ca": "several CA certificates are installed; choose one with --cert:\n  {0}",
	"err.trust_write_failed":    "none of the Chromium flag files could be written",
	"err.trust_clear_failed":    "could not remove the flag files: {0}",
	"flag.cert":                 "CA certificate to trust (PEM or DER, whatever the extension; a local or device path)",
	"flag.clear_trust":          "remove the flags instead of writing them",
	"flag.no_restart":           "do not restart Chrome afterwards",

	// Values
	"status.stock":       "stock",
	"status.magisk":      "Magisk patched",
	"status.unsupported": "unsupported patch",
	"bool.yes":           "yes",
	"bool.no":            "no",
}
