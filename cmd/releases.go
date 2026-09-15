package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/ejfkdev/avdroot/internal/i18n"
	"github.com/ejfkdev/avdroot/internal/ui"
)

// MagiskReleasesURL is the official download page, shown whenever an installer
// is missing or unusable so the user can fetch one by hand.
const MagiskReleasesURL = "https://github.com/topjohnwu/Magisk/releases"

// magiskAPIBase is the GitHub API endpoint for the Magisk repository.
const magiskAPIBase = "https://api.github.com/repos/topjohnwu/Magisk/releases"

// apiTimeout bounds a metadata request. It is short because the result is only
// used to pick a download URL, and a failure falls back to manual instructions.
const apiTimeout = 30 * time.Second

// release is one published Magisk release.
type release struct {
	Tag        string `json:"tag_name"`
	Name       string `json:"name"`
	Prerelease bool   `json:"prerelease"`
	Draft      bool   `json:"draft"`
	Published  string `json:"published_at"`
	Assets     []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
		Size int64  `json:"size"`
	} `json:"assets"`
}

// PublishedDate renders the publication date, or "" when unset.
func (r *release) PublishedDate() string {
	if len(r.Published) >= 10 {
		return r.Published[:10]
	}
	return r.Published
}

// Kind describes the release channel.
func (r *release) Kind() string {
	if r.Prerelease {
		return "pre-release"
	}
	return "stable"
}

// apkAsset returns the downloadable installer.
//
// Release assets are named Magisk-v<version>.apk — there is no plain
// "Magisk.apk" — so the name is matched by prefix rather than hard coded.
func (r *release) apkAsset() (name, url string, size int64, ok bool) {
	for _, a := range r.Assets {
		if len(a.Name) > 4 && a.Name[:6] == "Magisk" && a.Name[len(a.Name)-4:] == ".apk" {
			return a.Name, a.URL, a.Size, true
		}
	}
	return "", "", 0, false
}

// fetchReleases retrieves the recent releases, newest first.
func fetchReleases(limit int) ([]release, error) {
	if limit <= 0 || limit > 100 {
		limit = 10
	}
	ctx, cancel := context.WithTimeout(context.Background(), apiTimeout)
	defer cancel()

	url := fmt.Sprintf("%s?per_page=%d", magiskAPIBase, limit)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	// The GitHub API rejects requests without a User-Agent.
	req.Header.Set("User-Agent", "avdroot")
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned %s", resp.Status)
	}
	var out []release
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}

// latestRelease picks the newest usable release. By default pre-releases are
// skipped, because the stable channel is what most users want; pass
// includePrerelease to consider them, which matters when the newest work is
// only published as a pre-release.
func latestRelease(includePrerelease bool) (*release, error) {
	releases, err := fetchReleases(15)
	if err != nil {
		return nil, err
	}
	for i := range releases {
		r := &releases[i]
		if r.Draft {
			continue
		}
		if r.Prerelease && !includePrerelease {
			continue
		}
		if _, _, _, ok := r.apkAsset(); !ok {
			continue
		}
		return r, nil
	}
	return nil, fmt.Errorf("no downloadable release found")
}

// directReleaseURL builds the asset URL for an explicit tag. No API call is
// needed, so this works even when the API is rate limited.
func directReleaseURL(tag string) string {
	if tag != "" && tag[0] != 'v' {
		tag = "v" + tag
	}
	return fmt.Sprintf("%s/download/%s/Magisk-%s.apk", MagiskReleasesURL, tag, tag)
}

// printReleases lists releases so the user can choose a version to fetch.
func printReleases(includePrerelease bool) error {
	releases, err := fetchReleases(10)
	if err != nil {
		ui.Warn(i18n.T("magisk.releases_failed", err))
		ui.Info(i18n.T("magisk.browse", MagiskReleasesURL))
		return err
	}
	t := &ui.Table{Headers: []string{i18n.T("table.tag"), i18n.T("table.channel"), i18n.T("table.published"), i18n.T("table.size")}}
	for i := range releases {
		r := &releases[i]
		if r.Draft {
			continue
		}
		if r.Prerelease && !includePrerelease {
			continue
		}
		_, _, size, ok := r.apkAsset()
		if !ok {
			continue
		}
		t.Add(r.Tag, r.Kind(), r.PublishedDate(), bytesHuman(size))
	}
	t.Render()
	ui.Linef("\n%s", ui.Dim(i18n.T("magisk.fetch_hint")))
	return nil
}
