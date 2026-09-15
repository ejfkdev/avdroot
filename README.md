<div align="center">

# avdroot

**Root an Android Studio emulator by patching its ramdisk with Magisk — in pure Go.**

[![CI](https://github.com/ejfkdev/avdroot/actions/workflows/ci.yml/badge.svg)](https://github.com/ejfkdev/avdroot/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/ejfkdev/avdroot?include_prereleases&sort=semver)](https://github.com/ejfkdev/avdroot/releases)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](https://github.com/ejfkdev/avdroot/blob/main/LICENSE)
[![Go version](https://img.shields.io/github/go-mod/go-version/ejfkdev/avdroot)](go.mod)
[![Platforms](https://img.shields.io/badge/platforms-macOS%20%7C%20Linux%20%7C%20Windows-informational)](#platform-support)
[![Built with ZCode](https://img.shields.io/badge/Built%20with%20ZCode-000000.svg?style=flat&logo=data:image/svg%2bxml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHdpZHRoPSIxMTgiIGhlaWdodD0iMTAwIiB2aWV3Qm94PSIwIDAgMjU2IDIxOCI+PHBhdGggZmlsbD0iI2ZmZmZmZiIgZD0iTTEzNC40IDAuMTMwMTUyTDExNi40OCAyNS42MDIyQzExMy42NjUgMjkuNTY5OSAxMDkuMDU0IDMyLjAwMTkgMTA0LjA2NCAzMi4wMDE5SDYuMzk5OVYwQzYuMzk5OSAwLjEzMDE0OSAxMzQuNCAwLjEzMDE1MiAxMzQuNCAwLjEzMDE1MloiLz48cGF0aCBmaWxsPSIjZmZmZmZmIiBkPSJNMjU2IDAuMTMwMTI3TDEwMi40MDEgMjE3LjczMkgwTDE1My41OTkgMC4xMzAxMjdIMjU2WiIvPjxwYXRoIGZpbGw9IiNmZmZmZmYiIGQ9Ik0xMjEuNjAxIDIxNy43MzJMMTM5LjY1IDE5Mi4xMzRDMTQyLjQ2NSAxODguMTY2IDE0Ny4wNzYgMTg1LjczNCAxNTIuMDY3IDE4NS43MzRIMjQ5LjYwNFYyMTcuNzM2SDEyMS42MDFWMjE3LjczMloiLz48L3N2Zz4=)](https://zcode.z.ai/)

[English](README.md) · [简体中文](README.zh-CN.md)

</div>

---

`avdroot` takes an AVD from a cold start to a working root shell with one
command. Everything happens on the host: the ramdisk is decompressed, rewritten
so Magisk's init replaces `/init`, and recompressed. Building the image needs no
shell script, no busybox, no `magiskboot`, and no code execution inside the
emulator.

```
$ avdroot root
==> Target
    Pixel_10_Pro_XL  (the only AVD)
==> Patching
    preinit device: vdd1 (read from the running emulator)
==> Restarting the emulator
    booting cold, because a snapshot would restore the pre-patch ramdisk
==> Installing the Magisk app
  ok installed com.topjohnwu.magisk (Magisk 31.0)
==> Preparing Magisk's environment
    the app has not unpacked Magisk yet, so the su on PATH is the emulator's own
    opening the Magisk app so it can finish installing itself
  ok the app asked to set itself up; pressed "OK"
    the app restarts the device to finish; waiting for it to come back
  ok the device restarted with Magisk's environment in place
==> Granting su to the adb shell
  ok uid 2000 allowed
==> Verifying root
  ok Magisk 31.0:MAGISK:R is running
  ok su grants root

root: uid=0(root) gid=0(root) groups=0(root) context=u:r:magisk:s0
```

## Table of contents

- [Why another one](#why-another-one)
- [Install](#install)
- [Quick start](#quick-start)
- [Commands](#commands)
- [How it works](#how-it-works)
- [Choose a Magisk release](#choose-a-magisk-release)
- [Traps this tool handles](#traps-this-tool-handles)
- [Capturing HTTPS from Chrome](#capturing-https-from-chrome)
- [Platform support](#platform-support)
- [Output language](#output-language)
- [Development](#development)
- [Credits and licence](#credits-and-licence)

## Why another one

[rootAVD](https://github.com/newbit1/rootAVD) pioneered this and is still the
reference for the technique, but it has not kept up with recent Android and
Magisk releases. Running it against an Android 17 AVD shows the problems
plainly, and each one is fixed here:

| rootAVD behaviour | avdroot |
|---|---|
| Requires `bash`, `busybox`, `unzip`, `xxd`, `strings`, `dd`, `cpio`, and GNU `stat`/`sed` (which differ on macOS) | One static Go binary. cpio, LZ4, gzip and xz are all implemented in-process |
| Copies the script plus binaries into the emulator and runs itself there | Builds the image entirely on the host |
| Backs up `ramdisk.img` on every run, so after the first patch the "pristine" backup is itself patched and `restore` cannot return to stock | Backs up once, never overwrites, and warns if an existing backup turns out to be patched |
| Patches for Magisk's old `magisk32`/`magisk64` layout | Implements Magisk 26+ (`magiskinit`, `magisk`, `init-ld`, `.backup/.rmlist`) |
| Needs the ramdisk path pasted as an argument | Discovers the SDK, AVDs and images, with `--sdk`/`--avd-home` to override |
| Unconditionally reboots and hopes | Forces a cold boot and verifies that `su` actually returns `uid=0` |

## Install

**Prebuilt binaries** — take the archive for your platform from
[Releases](https://github.com/ejfkdev/avdroot/releases):

```sh
tar -xzf avdroot_linux_amd64.tar.gz
chmod +x avdroot_linux_amd64
sudo mv avdroot_linux_amd64 /usr/local/bin/avdroot
```

Windows and macOS archives contain a single executable; on macOS you may need to
clear the quarantine attribute:

```sh
xattr -d com.apple.quarantine avdroot_darwin_arm64
```

Every release also publishes `SHA256SUMS.txt`:

```sh
sha256sum -c SHA256SUMS.txt --ignore-missing
```


**With Go** (1.24 or newer):

```sh
go install github.com/ejfkdev/avdroot@latest
```

Go 1.24 is the floor because earlier toolchains emit no `LC_UUID` load command,
and macOS 26's `dyld` refuses to launch such a binary. Go 1.21 and 1.22 stamp
`minos 26.0` as well, so they fail on both counts.

**From source:**

```sh
git clone https://github.com/ejfkdev/avdroot
cd avdroot
make build          # host binary
make all            # every release target into dist/
```

You also need an Android SDK with `adb` and the emulator, and a `google_apis` or
AOSP system image. Play Store images cannot be rooted this way: they refuse
`adb root`, so `patch` and `root` reject them immediately rather than failing
several minutes later.

## Quick start

```sh
avdroot list      # what exists, and what state it is in
avdroot root      # patch, restart, install the app, grant su, verify
```

That is the whole thing. `root` needs no AVD name when only one exists, asks for
confirmation once, and skips the question entirely when stdin is not a terminal:

```sh
avdroot root Pixel_10_Pro_XL --yes     # named AVD, unattended
```

If you would rather do it in steps:

```sh
avdroot patch                  # patch only; the emulator need not be running
avdroot patch --dry-run        # show what would change, write nothing
# restart the emulator yourself, then
avdroot install && avdroot verify
```

To undo:

```sh
avdroot restore                # restores ramdisk.img.backup
```

## Commands

| Command | Description |
|---|---|
| `avdroot root [target]` | Patch, restart, install, grant su, verify — unattended |
| `avdroot patch [target]` | Patch the ramdisk only; asks before writing |
| `avdroot restore [target]` | Restore from `ramdisk.img.backup` |
| `avdroot list` | AVDs, system images and their patch status |
| `avdroot status [target]` | Details for one target, including the preinit device |
| `avdroot verify [target]` | Check that Magisk runs and `su` returns `uid=0` |
| `avdroot install` | Install the Magisk app into the running emulator |
| `avdroot magisk info` | Which installer would be used, and what it supports |
| `avdroot magisk fetch` | Download a release (`--list`, `--version`, `--prerelease`) |
| `avdroot trust-chrome` | Make Chrome accept an interception CA (`--cert`, `--clear`) |
| `avdroot doctor` | Explain how every path was located |

A *target* is an AVD name, a system-image path, or a path to a `ramdisk.img`.
`avdroot <command> --help` shows examples for each.

### Global flags

These apply to every command:

| Flag | Effect |
|---|---|
| `--sdk PATH` | Android SDK root, when detection picks the wrong one |
| `--avd-home PATH` | Where AVD definitions live |
| `--serial SERIAL` | Which device to use when several are connected |
| `--magisk PATH` | Use a specific Magisk installer |
| `--abi ABI` | Override the detected ABI |
| `--compress FORMAT` | Override the ramdisk compression (`gzip`, `lz4_legacy`, `lz4`, `xz`) |
| `--preinit DEVICE` | Override the preinit block device |
| `--lang` | Output language (`en` or `zh`), overriding the system locale |
| `--no-adb` | Do not contact a running emulator |
| `--no-download` | Never fetch a Magisk installer automatically |

### Flags worth knowing

| Flag | Command | Effect |
|---|---|---|
| `-y`, `--yes` | `root`, `patch` | Skip the confirmation prompt |
| `--dry-run` | `patch` | Show the plan and write nothing |
| `--force` | `root`, `patch` | Re-patch an already patched ramdisk |
| `--no-install` | `root` | Skip installing the Magisk app |
| `--no-restart` | `root`, `trust-chrome` | Leave the emulator or browser alone |
| `--timeout 10m` | `root` | How long to wait for boot |
| `--keep-verity`, `--keep-forceencrypt` | `patch` | Both default to on; disable to strip the fstab options |
| `--recovery-mode` | `patch` | Patch for recovery (implied on API 28) |
| `--cert PATH` | `trust-chrome` | Which CA to trust; auto-detected if omitted |
| `--clear` | `trust-chrome` | Undo the browser trust |
| `--list` | `restore`, `magisk fetch` | List instead of acting |

## How it works

A modern system image's `ramdisk.img` is a **cpio `newc` archive**, compressed
with gzip, lz4 or xz. Android 11+ images often concatenate two archives — a
generic ramdisk and a vendor ramdisk — inside one compressed stream. Patching
means:

1. **Decompress** (`internal/imgfmt`). LZ4 legacy streams are decoded at block
   level, including the repeated magic that concatenated streams produce.
2. **Parse and merge** the cpio archives (`internal/cpio`). Later entries win on
   name collisions, matching how the kernel unpacks them sequentially.
3. **Apply the Magisk patch** (`internal/magisk`), reproducing `boot_patch.sh`:
   an existing patch is detected and reverted first; `/init` is replaced with
   `magiskinit` (mode `0750`); `overlay.d/sbin/{magisk,stub,init-ld}.xz` are
   added; fstab verity and encryption options are stripped if requested; the
   difference against the stock archive is recorded in `.backup`, with
   `.backup/.rmlist` listing what `restore` must delete.
4. **Recompress** in the original format and write the image atomically.

The serialisation is byte-compatible with Magisk v31's `magiskboot`: inodes
count from 300000, `nlink` is 1, `mtime` is 0, and entries are emitted in
byte-wise sorted order. `internal/magisk/parity_test.go` asserts that the entry
set, permissions, `.rmlist` contents and config this tool produces for a real
AVD match a ramdisk patched by Magisk's own installer.

## Choose a Magisk release

Magisk ships **one universal package**, not one per Android version. A release
carries native code for every ABI in a single archive and installs on any
Android at or above its declared `minSdkVersion`:

```
$ avdroot magisk info
  ok Magisk 31.0 (31000, arm64-v8a)
    package      com.topjohnwu.magisk
    supports     API 23+ (built against 37)
    abis         arm64-v8a, armeabi-v7a, x86, x86_64
  ok compatible with Pixel_10_Pro_XL
```

So only two things are decided per device:

- **ABI** — which `lib/<abi>/` directory to take from the archive. Detected from
  the target, overridable with `--abi`.
- **API range** — read from the archive's own manifest by parsing the binary
  `AndroidManifest.xml`, so no `aapt2` is needed. Below `minSdkVersion` patching
  stops with an error, because the app could not be installed. A release built
  against a noticeably older platform only warns; the root check settles it.

Releases come in two channels, and the newest work is often a pre-release —
`v31.0` is one, while `v30.7` is the newest stable:

```
$ avdroot magisk fetch --list
TAG    CHANNEL  PUBLISHED   SIZE
v30.7  stable   2026-02-23  11.1 MiB

$ avdroot magisk fetch --list --prerelease
v31.0  pre-release  2026-09-04  10.7 MiB
```

`fetch` resolves the newest release through the GitHub API, and uses a direct
URL when you name a tag, so it keeps working when the API is rate limited. If
the download fails it says so and points at the releases page:

```
warn download failed: HTTP 404: no release with that tag

Fetch it manually
    1. open https://github.com/topjohnwu/Magisk/releases
    2. download the Magisk-v<version>.apk asset you need
    3. pass it straight to a command, no need to move it into place:
         avdroot root --magisk /path/to/Magisk-v31.0.apk
       or put it at ~/Library/Caches/avdroot/Magisk.zip
```

### Where the installer is found, and where downloads go

An installer is **downloaded automatically** when none is present, so a first
run on a clean machine works. Nothing is fetched when a local one is found, and
`--no-download` disables fetching for offline or scripted use.

Downloads land in the operating system's cache directory — an installer is
reproducible, so it lives where clearing a cache reclaims it rather than in your
configuration:

| Platform | Location |
|---|---|
| macOS | `~/Library/Caches/avdroot/Magisk.zip` |
| Linux | `$XDG_CACHE_HOME/avdroot/Magisk.zip`, usually `~/.cache/avdroot/` |
| Windows | `%LocalAppData%\avdroot\Magisk.zip` |

`avdroot doctor` prints the exact path.

Lookup order, first hit wins:

1. `--magisk PATH`
2. `$AVDROOT_MAGISK`
3. the cache directory above
4. `$XDG_CONFIG_HOME/avdroot`, `~/.config/avdroot`, or the platform config dir
5. next to the `avdroot` binary, then one directory up
6. the current working directory
7. `~/Downloads`

The cache is searched before the configuration directory on purpose: a release
you just fetched should be the one that gets used, rather than an older file
left lying around.

A failed transfer is retried three times with a backoff, and `HTTPS_PROXY` is
honoured. If it still fails the command explains the failure and prints the
manual route, so a restricted network is never a dead end.

## Traps this tool handles

These are the behaviours that make emulator rooting fiddly. Each one is handled
automatically, and each was found by testing rather than guessed at.

**A guest reboot does not reload the ramdisk.** `adb reboot` reboots the guest
with the initrd the emulator loaded when its process started, so a newly patched
`ramdisk.img` is ignored. Only a *new emulator process* picks it up. `avdroot
root` therefore kills and relaunches the emulator, and `verify` reports a
version mismatch when the running Magisk does not match the ramdisk.

**The installer is not signature-checked.** A Magisk archive is verified to be
a usable release (it parses, it is the `com.topjohnwu.magisk` package, and it
carries the expected binaries for the target ABI), and it is fetched over HTTPS
from the official releases page. The APK's own signature is *not* verified, so
download from the official page or pass a file you trust with `--magisk`.

**A saved snapshot can restore the pre-patch ramdisk.** When an AVD is set to
fast boot the emulator may restore a saved VM state, and that state contains the
initrd it had loaded when the snapshot was taken — so a restart can bring back
the *old* ramdisk and make a patch look like it did nothing. `avdroot root`
always restarts with `-no-snapshot`, and `patch`, `root` and `doctor` all report
when a snapshot and fast boot are present. No snapshot is ever deleted.

**The backup must never be overwritten.** Backing up on every run is what leaves
users unable to restore: after the first patch their "pristine" copy is already
patched. `avdroot` backs up once and warns if the backup it finds is itself
patched.

**`PREINITDEVICE` is baked in at patch time.** Magisk derives it at runtime.
`avdroot` ports Magisk's `find_preinit_device` and evaluates it against the live
`/proc/self/mountinfo`, which yields `vdd1` for an AVD whose `/data` is on
device-mapper and whose `/metadata` is on a virtio disk. Pass `--preinit` when
the emulator is not running.

**A patched ramdisk is not a finished Magisk.** It installs Magisk's init and
its daemon, and nothing else: the rest of Magisk ships inside the manager app
and is written to `/data/adb/magisk` only when the app runs its first-start
setup. Until that has happened *and* the device has restarted, the `su` on
`PATH` is the emulator's own, which reads `-c` as a uid and fails with
`su: invalid uid/gid '-c'`. Nothing looks wrong — Magisk reports its version and
accepts su-policy writes the whole time — so the symptom is a tool that declares
Magisk healthy and then cannot use it. `avdroot root` opens the app, answers the
setup prompt, and waits for the restart the app performs, which is what makes
the first run succeed rather than the second.

**`su` normally needs a tap.** Magisk asks the app for a decision the first time
a uid requests root, and a script cannot answer that. `avdroot root` writes the
policy row with `magisk --sqlite` instead, using the values from Magisk's own
`SuPolicy` enum (`Allow = 2`, `until = 0` meaning never expires). The daemon
reads the row per request, so it applies immediately and survives reboots.

**`adb install` streams from the host** and insists on a `.apk` filename, so a
device path never works. The installer is staged as a temporary `.apk`.

## Capturing HTTPS from Chrome

Installing a CA certificate into the system store is enough for almost every
app, but not for Chrome. **Chrome on Android enforces Certificate Transparency
for anything chaining to a certificate in the system store** — its test for
"is this a public root CA" is simply "is it in the system store". A locally
generated interception CA has no transparency log entry, so Chrome refuses it
with `NET::ERR_CERT_AUTHORITY_INVALID` while every other app accepts it happily.

The failure is confusing precisely because everything else works:

| Client | Result with the same certificate |
|---|---|
| Android platform trust store (any normal app) | accepted |
| Chrome | `NET::ERR_CERT_AUTHORITY_INVALID` |

`avdroot trust-chrome` resolves it by telling Chromium to trust that one key
explicitly, through the `--ignore-certificate-errors-spki-list` startup flag:

```
$ avdroot trust-chrome
==> Preparing to trust a CA in Chrome
  ok certificate: CN=Reqable CA (Jul 20, 2026, 16EB1AA6), ...
    read from /data/data/com.reqable.android/files/certificate/reqable-root.crt
    SPKI fingerprint: nHhlCoiiwRRZzTU+d2MDZrdTs7bvRBrIWpFSTTyqWDU=
==> Writing the Chromium startup flags
  ok wrote 8 flag files
==> Restarting Chrome so it reads the flags
  ok Chrome stopped; open it again to pick up the flags
```

Every export format Reqable offers works except PKCS#12: PEM and DER are both
accepted whatever the extension, so `.pem`, `.crt` and `.0` are all fine. A
`.p12` is a container rather than a certificate, and decoding it needs the
password and its own encryption algorithms; export as PEM instead.

The certificate is auto-detected from the CA certificates installed on the
device that are outside the system image: ones added through Settings, ones
installed by a Magisk module, and ones a tool keeps in its own data directory
(where a tool that injects its CA with root at runtime leaves the only
persistent copy). Pass `--cert` to choose explicitly — a local path or a path on
the device. `--clear` undoes it.

Three things to know:

- **The fingerprint is tied to the key.** If the interception tool regenerates
  its CA, run the command again.
- **This disables Certificate Transparency for that one key**, which is a real
  reduction in protection. It is meant for emulators and test devices.
- **Chrome must be restarted** to read the flag, since it is only consulted at
  startup. The command does that for you.

## Platform support

| Platform | Path detection | Patching | Emulator restart |
|---|---|---|---|
| macOS | yes | verified | verified |
| Linux | yes | shared code | shared code |
| Windows | yes | shared code | implemented; launcher options are not auto-detected |

Patching is pure Go with no OS-specific code, so a ramdisk patched on one host
is byte-identical to one patched on another. macOS is the only platform
exercised end to end here, against an arm64 Android 17 AVD; Linux and Windows
are built and unit-tested on every change, including cross-compilation in CI.

On Windows the emulator is restarted with `-avd NAME` and nothing else, because
reading a process command line there is less reliable. Pass `--emulator-args` to
preserve something like `-no-window`. Colour is disabled on the legacy console,
which cannot render ANSI escapes.

## Output language

Output follows the system locale and falls back to English. Chinese is detected
from `LANG`/`LC_ALL`/`LC_MESSAGES` on Unix, and from `GetUserDefaultUILanguage`
on Windows where those variables are usually unset.

```sh
avdroot list --lang zh          # force Chinese
avdroot list --lang en          # force English
AVDROOT_LANG=zh avdroot list    # or via the environment
```

Help text is translated too:

```
$ LANG=zh_CN.UTF-8 avdroot patch --help
用法：
  avdroot patch [avd|image|ramdisk] [flags]

示例：
  avdroot patch --dry-run           # 只打印计划，不写任何东西
  avdroot patch Pixel_10_Pro_XL -y  # 指定 AVD，不询问

参数：
      --dry-run             只报告会改什么，不写入任何内容
```

Technical diagnostics raised inside the internal packages stay in English; the
command layer, its errors and all help text are translated. Adding a language
means adding one file — see `internal/i18n/zh.go` — and the parity tests will
point out anything you miss.

## Development

```sh
make build        # host binary
make all          # cross-compile every release target
make test
make vet
```

### Where the code lives

```
main.go                 entry point
cmd/                    the CLI: one file per command, plus installer resolution
internal/cpio/          the newc archive format, and Magisk's patch/backup/restore
internal/imgfmt/        gzip, lz4, lz4_legacy, xz, lzma and bzip2
internal/magisk/        installer extraction and the boot_patch.sh equivalent
internal/avd/           SDK, AVD and system-image discovery
internal/ramdisk/       load/save, atomic writes, backup and restore
internal/adb/           adb wrapper, su policy, preinit detection, app dialogs
internal/emulator/      emulator start, restart and boot waiting
internal/axml/          binary AndroidManifest.xml reader
internal/certutil/      certificate parsing and SPKI fingerprints
internal/i18n/          English and Chinese message catalogues
internal/ui/            terminal output
```

`internal/cpio` and `internal/imgfmt` are where correctness matters most: they
implement formats byte-for-byte, and the parity test checks the result against a
ramdisk patched by Magisk itself.

### Tests

The tests that need real data skip themselves when it is absent, so
`go test ./...` passes on a machine with no Android SDK — 105 tests run, 12 skip.
To run everything, point `AVDROOT_TEST_ASSETS` at a directory holding the assets
listed in [testdata/README.md](testdata/README.md):

```sh
export AVDROOT_TEST_ASSETS=~/avdroot-assets
go test ./...     # 116 tests run, 1 still skipped
```

The one that still skips is the certificate-export comparison, which needs the
proxy's exported CA in three formats; [testdata/README.md](testdata/README.md)
lists them.

Those assets turn the suite into a full regression test of the cpio, compression
and Magisk layers against real data, including a comparison against Magisk's own
patcher. They are never committed.

### Contributing

Patching is deliberately **pure Go with no OS-specific code**, so a ramdisk
patched on one host is byte-identical to one patched on another. Please keep it
that way: platform differences belong in path discovery, process handling and
output, not in the image path.

Anything user-visible needs a message in both `internal/i18n/en.go` and
`internal/i18n/zh.go`; the parity tests fail if only one is updated.

## Credits and licence

The technique, the file formats and the patch semantics come from
[Magisk](https://github.com/topjohnwu/Magisk) by John Wu, and from
[rootAVD](https://github.com/newbit1/rootAVD) by NewBit XDA, which showed that
emulator ramdisks could be patched at all.

This is an independent reimplementation: it shares no code with either project.
It does implement formats those projects define, which is why the credits above
are explicit.

Released under the [MIT licence](https://github.com/ejfkdev/avdroot/blob/main/LICENSE).
Magisk itself is not distributed with this project; download it from the
[official releases page](https://github.com/topjohnwu/Magisk/releases).
