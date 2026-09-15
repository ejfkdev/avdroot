// Command avdroot installs Magisk into an Android Studio emulator by patching
// the AVD's ramdisk image, all from the host and without a shell or busybox.
package main

import (
	"runtime/debug"

	"github.com/ejfkdev/avdroot/cmd"
)

// version is set at build time with -X main.version, which the release
// workflow does. A binary installed with "go install pkg@version" carries no
// ldflags, so its version comes from the build info instead.
var version = "dev"

// devVersion marks a build with no version stamped in, either by ldflags or by
// the toolchain. It is deliberately the same string the variable defaults to.
const devVersion = "dev"

func main() {
	cmd.Execute(resolveVersion())
}

func resolveVersion() string {
	if version != devVersion {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		// The toolchain records the module version it resolved. "(devel)" is
		// what a plain "go build" from a checkout reports, which is no more
		// informative than the default.
		if v := info.Main.Version; v != "" && v != "(devel)" {
			return v
		}
	}
	return devVersion
}
