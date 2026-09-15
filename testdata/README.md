# Test assets

A few tests run against real data: an actual system image ramdisk, an actual
Magisk installer. Shipping those in the repository would mean committing 12 MB
of someone else's release and an SDK image, so they are **not** included and the
tests that need them **skip themselves** when they are missing. `go test ./...`
therefore passes on a machine with no Android SDK at all.

To run the full suite, point `AVDROOT_TEST_ASSETS` at a directory containing the
files below, or drop them into a `testdata/` directory beside the package that
needs them.

| File | What it is | Used by |
|---|---|---|
| `Magisk.zip` | A Magisk installer from the [official releases](https://github.com/topjohnwu/Magisk/releases). A release archive is also a valid APK, so it doubles as the APK fixture. | `internal/axml`, `internal/magisk` |
| `ramdisk.stock.img` | A pristine `ramdisk.img` taken straight from a system image: `$ANDROID_HOME/system-images/<api>/<tag>/<abi>/ramdisk.img` | `internal/cpio`, `internal/magisk` |
| `ramdisk.patched.img` | The same image after patching it with this tool. Used to check restore and re-patch behaviour. | `internal/cpio` |
| `ramdisk.reference.img` | A ramdisk patched by **Magisk's own installer**, used to assert that this implementation produces the same entry set, permissions and `.rmlist`. Optional, but it is the strongest check in the suite. | `internal/magisk` |
| `reqable-ca.crt` | The root CA of a local interception proxy, in any format `Parse` accepts. The test pins the SPKI that the `openssl` command in Chromium's documentation produces for **this** certificate, so it has to be the same CA the pinned value came from. | `internal/certutil` |
| `export.pem`, `export.crt`, `export.0` | That same CA exported three ways from the proxy's export menu. Exporting the same CA as PEM, DER and Android's `.0` must yield one identical key. | `internal/certutil` |
| `export-nopass.p12`, `export-pass.p12` | The same CA as PKCS#12, with and without a password. Optional; when present they must be recognised as PKCS#12 rather than reported as garbage. | `internal/certutil` |

Example:

```sh
export AVDROOT_TEST_ASSETS=~/avdroot-assets
go test ./...
```

The assets are ignored by `.gitignore`; only this file is tracked. Everything
else the suite needs is either constructed in the test or generated on the spot,
so a checkout with none of the above still builds and tests cleanly.
