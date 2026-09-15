package main

import "testing"

// A released binary must report the version stamped in by ldflags, never the
// fallback: the release workflow passes -X main.version=<tag> and that is the
// only place the exact released version is recorded.
func TestResolveVersionPrefersLdflags(t *testing.T) {
	original := version
	t.Cleanup(func() { version = original })

	version = "v9.9.9"
	if got := resolveVersion(); got != "v9.9.9" {
		t.Errorf("resolveVersion() = %q, want v9.9.9", got)
	}
}

// With no ldflags the version comes from the build info. Whatever the
// toolchain recorded, the raw "(devel)" placeholder must never reach the user.
func TestResolveVersionNeverReturnsDevelPlaceholder(t *testing.T) {
	original := version
	t.Cleanup(func() { version = original })

	version = devVersion
	got := resolveVersion()
	if got == "" {
		t.Fatal("resolveVersion() returned an empty version")
	}
	if got == "(devel)" {
		t.Errorf("resolveVersion() = %q, want a usable version or %q", got, devVersion)
	}
}
