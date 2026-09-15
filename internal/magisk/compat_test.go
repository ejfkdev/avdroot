package magisk

import (
	"errors"
	"testing"
)

// A Magisk release declares the API range it can be installed on. Only the
// minSdkVersion is a hard limit, because it is a fact about the package; a
// newer Android may well work and is only worth a warning.
func TestCheckAPI(t *testing.T) {
	// Values taken from the real v31.0 release, which aapt2 also reports.
	p := &Payload{Version: "31.0", MinSDK: 23, TargetSDK: 37}

	// At or above minSdk with a contemporary platform: nothing to say.
	for _, api := range []int{23, 28, 37} {
		warning, err := p.CheckAPI(api)
		if err != nil {
			t.Errorf("API %d: unexpected error: %v", api, err)
		}
		if warning != "" {
			t.Errorf("API %d: unexpected warning: %s", api, warning)
		}
	}

	// Below minSdk the app cannot be installed, so it must be an error.
	for _, api := range []int{21, 22} {
		_, err := p.CheckAPI(api)
		if err == nil {
			t.Fatalf("API %d: expected an error", api)
		}
		var unsupported *ErrAPIUnsupported
		if !errors.As(err, &unsupported) {
			t.Fatalf("API %d: error is %T, want *ErrAPIUnsupported", api, err)
		}
		if unsupported.MinSDK != 23 || unsupported.DeviceAPI != api {
			t.Errorf("API %d: error carries %d/%d, want 23/%d",
				api, unsupported.MinSDK, unsupported.DeviceAPI, api)
		}
	}

	// An old release on a much newer platform is legal but suspicious.
	old := &Payload{Version: "26.4", MinSDK: 21, TargetSDK: 34}
	warning, err := old.CheckAPI(37)
	if err != nil {
		t.Fatalf("an old but installable release must not be fatal: %v", err)
	}
	if warning == "" {
		t.Error("expected a warning when the release predates the platform")
	}

	// Unknown values must not produce a false verdict.
	unknown := &Payload{Version: "x"}
	if warning, err := unknown.CheckAPI(37); err != nil || warning != "" {
		t.Errorf("an undeclared range produced %q/%v, want no verdict", warning, err)
	}
	if warning, err := p.CheckAPI(0); err != nil || warning != "" {
		t.Errorf("an unknown target API produced %q/%v, want no verdict", warning, err)
	}
}

func TestAPIRange(t *testing.T) {
	for _, tc := range []struct {
		p    Payload
		want string
	}{
		{Payload{MinSDK: 23, TargetSDK: 37}, "API 23+ (built against 37)"},
		{Payload{MinSDK: 23}, "API 23+"},
		{Payload{}, "undeclared"},
	} {
		if got := tc.p.APIRange(); got != tc.want {
			t.Errorf("APIRange() = %q, want %q", got, tc.want)
		}
	}
}

// ABI normalisation accepts the spellings that appear in AVD configs and in the
// archive's lib directory names.
func TestNormalizeABI(t *testing.T) {
	for in, want := range map[string]string{
		"arm64-v8a":   "arm64-v8a",
		"arm64":       "arm64-v8a",
		"aarch64":     "arm64-v8a",
		"x86_64":      "x86_64",
		"x64":         "x86_64",
		"x86":         "x86",
		"armeabi-v7a": "armeabi-v7a",
		"arm":         "armeabi-v7a",
		"  ARM64  ":   "arm64-v8a",
	} {
		got, err := NormalizeABI(in)
		if err != nil {
			t.Errorf("NormalizeABI(%q): %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("NormalizeABI(%q) = %q, want %q", in, got, want)
		}
	}
	if _, err := NormalizeABI("mips"); err == nil {
		t.Error("expected an error for an unsupported ABI")
	}
}

// The real release is universal: one archive carries every ABI and installs on
// any Android at or above its minSdkVersion. This test pins that understanding,
// since the whole "which APK for which Android" question depends on it.
func TestRealPayloadIsUniversal(t *testing.T) {
	p, err := LoadPayload(magiskZIPPath(t), "arm64-v8a")
	if err != nil {
		t.Skipf("installer unavailable: %v", err)
	}
	if p.Package != "com.topjohnwu.magisk" {
		t.Errorf("Package = %q", p.Package)
	}
	if p.MinSDK == 0 || p.TargetSDK == 0 {
		t.Fatalf("the API range should come from the manifest, got min=%d target=%d", p.MinSDK, p.TargetSDK)
	}
	if p.MinSDK > p.TargetSDK {
		t.Errorf("minSdk %d is above targetSdk %d", p.MinSDK, p.TargetSDK)
	}
	// Every supported ABI is present in the single archive.
	want := map[string]bool{"arm64-v8a": false, "armeabi-v7a": false, "x86": false, "x86_64": false}
	for _, abi := range p.ABIs {
		want[abi] = true
	}
	for abi, present := range want {
		if !present {
			t.Errorf("the archive does not ship %s", abi)
		}
	}
	// The same archive must serve every ABI, which is what makes per-ABI
	// selection a matter of choosing a directory rather than a download.
	for abi := range want {
		q, err := LoadPayload(magiskZIPPath(t), abi)
		if err != nil {
			t.Errorf("LoadPayload(%s): %v", abi, err)
			continue
		}
		if len(q.MagiskInit) == 0 || len(q.Magisk) == 0 {
			t.Errorf("%s: payload is incomplete", abi)
		}
	}
	t.Logf("%s: %s, %s", p.Description(), p.APIRange(), p.ABIs)
}
