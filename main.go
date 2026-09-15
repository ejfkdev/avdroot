// Command avdroot installs Magisk into an Android Studio emulator by patching
// the AVD's ramdisk image, all from the host and without a shell or busybox.
package main

import "github.com/ejfkdev/avdroot/cmd"

// version is set at build time with -X main.version.
var version = "dev"

func main() {
	cmd.Execute(version)
}
