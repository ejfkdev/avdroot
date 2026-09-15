package cmd

import (
	"strings"
	"testing"
)

// Release assets are named Magisk-v<version>.apk. The other asset in every
// Magisk release, app-debug.apk, must not be chosen.
func TestReleaseAPKAsset(t *testing.T) {
	r := &release{Tag: "v31.0"}
	r.Assets = []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
		Size int64  `json:"size"`
	}{
		{Name: "app-debug.apk", URL: "https://example.invalid/app-debug.apk", Size: 25278536},
		{Name: "Magisk-v31.0.apk", URL: "https://example.invalid/Magisk-v31.0.apk", Size: 11613864},
		{Name: "notes.md", URL: "https://example.invalid/notes.md", Size: 582},
	}
	name, url, size, ok := r.apkAsset()
	if !ok {
		t.Fatal("no installer asset found")
	}
	if name != "Magisk-v31.0.apk" {
		t.Errorf("asset = %q, want Magisk-v31.0.apk", name)
	}
	if !strings.HasSuffix(url, "Magisk-v31.0.apk") {
		t.Errorf("url = %q", url)
	}
	if size != 11613864 {
		t.Errorf("size = %d", size)
	}

	// A release without an installer is reported as such rather than
	// returning a wrong asset.
	empty := &release{Tag: "v0"}
	if _, _, _, ok := empty.apkAsset(); ok {
		t.Error("expected no asset for an empty release")
	}
}

// An explicit tag needs no API call: the URL is derived from the tag itself,
// which keeps fetching working when the GitHub API is rate limited.
func TestDirectReleaseURL(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"v31.0", "https://github.com/topjohnwu/Magisk/releases/download/v31.0/Magisk-v31.0.apk"},
		{"31.0", "https://github.com/topjohnwu/Magisk/releases/download/v31.0/Magisk-v31.0.apk"},
		{"", "https://github.com/topjohnwu/Magisk/releases/download//Magisk-.apk"},
	} {
		if got := directReleaseURL(tc.in); got != tc.want {
			t.Errorf("directReleaseURL(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// The channel label is what tells a user whether they are picking up a stable
// release or a pre-release.
func TestReleaseKind(t *testing.T) {
	if got := (&release{Prerelease: true}).Kind(); got != "pre-release" {
		t.Errorf("Kind() = %q, want pre-release", got)
	}
	if got := (&release{}).Kind(); got != "stable" {
		t.Errorf("Kind() = %q, want stable", got)
	}
}

func TestPublishedDate(t *testing.T) {
	if got := (&release{Published: "2026-09-04T12:34:56Z"}).PublishedDate(); got != "2026-09-04" {
		t.Errorf("PublishedDate() = %q", got)
	}
	if got := (&release{Published: ""}).PublishedDate(); got != "" {
		t.Errorf("PublishedDate() on empty = %q", got)
	}
}
