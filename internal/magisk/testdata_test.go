package magisk

import (
	"os"
	"path/filepath"
	"testing"
)

// testAsset locates a file the tests need.
//
// The search order is $AVDROOT_TEST_ASSETS, then a testdata directory beside
// the test. Tests skip themselves when nothing is found, because a contribution
// should not have to ship a 12 MB Magisk release or an SDK system image; see
// testdata/README.md for the expected file names.
func testAsset(t *testing.T, name string) string {
	t.Helper()
	var candidates []string
	if dir := os.Getenv("AVDROOT_TEST_ASSETS"); dir != "" {
		candidates = append(candidates, filepath.Join(dir, name))
	}
	if wd, err := os.Getwd(); err == nil {
		candidates = append(candidates, filepath.Join(wd, "testdata", name))
	}
	for _, c := range candidates {
		if fi, err := os.Stat(c); err == nil && !fi.IsDir() {
			return c
		}
	}
	t.Skipf("skipping: %s not found. Set AVDROOT_TEST_ASSETS to a directory holding the test assets, "+
		"or drop the file in testdata/ beside this package (see testdata/README.md)", name)
	return ""
}
