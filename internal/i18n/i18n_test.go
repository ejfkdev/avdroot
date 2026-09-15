package i18n

import (
	"errors"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// Every language must define exactly the same keys, so a new message cannot be
// added to one language and forgotten in the other.
func TestCatalogueParity(t *testing.T) {
	en := map[string]bool{}
	for _, k := range Keys(EN) {
		en[k] = true
	}
	zh := map[string]bool{}
	for _, k := range Keys(ZH) {
		zh[k] = true
	}

	var missingZh, missingEn []string
	for k := range en {
		if !zh[k] {
			missingZh = append(missingZh, k)
		}
	}
	for k := range zh {
		if !en[k] {
			missingEn = append(missingEn, k)
		}
	}
	sort.Strings(missingZh)
	sort.Strings(missingEn)
	if len(missingZh) > 0 {
		t.Errorf("%d keys are missing from Chinese: %v", len(missingZh), missingZh)
	}
	if len(missingEn) > 0 {
		t.Errorf("%d keys are missing from English: %v", len(missingEn), missingEn)
	}
}

// An empty translation is worse than a missing one: it silently prints nothing.
func TestNoEmptyTranslations(t *testing.T) {
	for _, lang := range Available() {
		for k := range catalogs[lang] {
			if strings.TrimSpace(catalogs[lang][k]) == "" {
				t.Errorf("%s/%s is empty", lang, k)
			}
		}
	}
}

// Placeholders must be consistent between languages, or a translated message
// would silently drop a value or print an index that was never supplied.
func TestPlaceholdersMatch(t *testing.T) {
	re := regexp.MustCompile(`\{\d+\}`)
	for k, en := range catalogs[EN] {
		zh, ok := catalogs[ZH][k]
		if !ok {
			continue
		}
		want := re.FindAllString(en, -1)
		got := re.FindAllString(zh, -1)
		if len(want) != len(got) {
			t.Errorf("%s: %d placeholders in English but %d in Chinese\n  en: %s\n  zh: %s",
				k, len(want), len(got), en, zh)
			continue
		}
		// Comparing the highest index catches a translation that refers to an
		// argument the caller never passes.
		if maxIndex(want) != maxIndex(got) {
			t.Errorf("%s: highest placeholder index differs (%d vs %d)", k, maxIndex(want), maxIndex(got))
		}
	}
}

func maxIndex(ph []string) int {
	max := -1
	for _, p := range ph {
		n, err := strconv.Atoi(strings.Trim(p, "{}"))
		if err == nil && n > max {
			max = n
		}
	}
	return max
}

// Catalogue entries must not contain printf verbs. They use {0} placeholders,
// so a stray %s or %w would be printed to the user verbatim.
func TestNoPrintfVerbsInCatalogue(t *testing.T) {
	verb := regexp.MustCompile(`%[a-zA-Z]`)
	for _, lang := range Available() {
		for k, v := range catalogs[lang] {
			if verb.MatchString(v) {
				t.Errorf("%s/%s contains a printf verb, which will not be substituted: %q",
					lang, k, v)
			}
		}
	}
}

// Every placeholder must be supplied by the call, so no argument is left as a
// literal "{0}" in the output.
func TestSubstitute(t *testing.T) {
	cases := []struct {
		tmpl string
		args []any
		want string
	}{
		{"no placeholders", nil, "no placeholders"},
		{"{0}", []any{"a"}, "a"},
		{"{0} and {1}", []any{"a", "b"}, "a and b"},
		{"{1} then {0}", []any{"a", "b"}, "b then a"},
		{"{0}{0}", []any{"x"}, "xx"},
		{"{5}", []any{"a"}, "{5}"},             // index never supplied
		{"{unclosed", []any{"a"}, "{unclosed"}, // not a placeholder
		{"{}", []any{"a"}, "{}"},               // empty index
		{"a {0} b {1} c", []any{1, true}, "a 1 b true c"},
	}
	for _, tc := range cases {
		if got := substitute(tc.tmpl, tc.args); got != tc.want {
			t.Errorf("substitute(%q, %v) = %q, want %q", tc.tmpl, tc.args, got, tc.want)
		}
	}
	// An error argument is unwrapped rather than printed as a struct.
	if got := substitute("{0}", []any{errors.New("boom")}); got != "boom" {
		t.Errorf("error argument = %q, want boom", got)
	}
	if got := substitute("{0}", []any{nil}); got != "" {
		t.Errorf("nil argument = %q, want empty", got)
	}
}

// T passes anything that is not a catalogue key straight through, which lets
// values and layout fragments share the same helper.
func TestTPassThrough(t *testing.T) {
	for _, s := range []string{
		"", "%s", "      os               %s/%s\n", "Pixel_10_Pro_XL",
		"root: %s", "no dots here", "UPPER.case",
	} {
		if got := T(s); got != s {
			t.Errorf("T(%q) = %q, want it unchanged", s, got)
		}
	}
	// Arguments on a non-key string are ignored rather than formatted, since
	// only catalogue entries contain placeholders.
	if got := T("%s-%s", "a", "b"); got != "%s-%s" {
		t.Errorf("T with args on a literal = %q, want it unchanged", got)
	}
}

// A missing key must be visible rather than silently empty.
func TestTUnknownKey(t *testing.T) {
	if got := T("does.not.exist"); got != "does.not.exist" {
		t.Errorf("T(unknown) = %q, want the key back", got)
	}
}

func TestParse(t *testing.T) {
	cases := []struct {
		in   string
		want Lang
		ok   bool
	}{
		{"zh", ZH, true},
		{"zh_CN", ZH, true},
		{"zh_CN.UTF-8", ZH, true},
		{"zh-Hans", ZH, true},
		{"zh_TW", ZH, true},
		{"ZH_cn", ZH, true},
		{"en", EN, true},
		{"en_US.UTF-8", EN, true},
		{"C", EN, false},
		{"POSIX", EN, false},
		{"fr_FR", EN, false},
		{"", EN, false},
	}
	for _, tc := range cases {
		got, ok := Parse(tc.in)
		if ok != tc.ok || got != tc.want {
			t.Errorf("Parse(%q) = %v,%v want %v,%v", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}

// A key must never be mistaken for literal text, and vice versa.
func TestKeyPattern(t *testing.T) {
	for _, k := range []string{"a.b", "patch.checking_target", "root.su_ok", "flag.dry_run"} {
		if !keyPattern(k) {
			t.Errorf("%q should be treated as a key", k)
		}
	}
	for _, s := range []string{"", ".", "a.", ".b", "%s", "Android SDK: %s",
		"root: %s", "os               %s/%s", "UPPER.case", "a.b c", "a..b"} {
		if keyPattern(s) {
			t.Errorf("%q should not be treated as a key", s)
		}
	}
}

// Init follows the explicit argument, then AVDROOT_LANG, then the locale.
func TestInitPrecedence(t *testing.T) {
	t.Setenv("LC_ALL", "zh_CN.UTF-8")
	t.Setenv("LC_MESSAGES", "")
	t.Setenv("LANG", "")

	// System locale applies when nothing overrides it.
	if got := Init(""); got != ZH {
		t.Errorf("Init(\"\") with a Chinese locale = %v, want zh", got)
	}
	// The environment variable wins over the locale.
	t.Setenv(LangEnv, "en")
	if got := Init(""); got != EN {
		t.Errorf("Init with %s=en = %v, want en", LangEnv, got)
	}
	// The explicit argument wins over everything.
	if got := Init("zh"); got != ZH {
		t.Errorf("Init(zh) = %v, want zh", got)
	}
	// An unknown value is ignored rather than fatal, and detection carries on
	// to the next source, which is the Chinese locale set above.
	t.Setenv(LangEnv, "klingon")
	if got := Init(""); got != ZH {
		t.Errorf("Init with an unknown %s = %v, want the locale, zh", LangEnv, got)
	}
	if got := Init("klingon"); got != ZH {
		t.Errorf("Init(klingon) = %v, want the locale, zh", got)
	}
	// With no usable locale either, it lands on English.
	t.Setenv(LangEnv, "klingon")
	t.Setenv("LC_ALL", "C")
	t.Setenv("LC_MESSAGES", "C")
	t.Setenv("LANG", "C")
	if got := Init("klingon"); got != EN {
		t.Errorf("Init(klingon) with no locale = %v, want en", got)
	}

	// C and POSIX mean "no locale", which falls back to English.
	t.Setenv(LangEnv, "")
	t.Setenv("LC_ALL", "C")
	if got := Init(""); got != EN {
		t.Errorf("Init with LC_ALL=C = %v, want en", got)
	}
}

// Every key referenced from the command layer must exist, otherwise the literal
// key would be printed to the user.
func TestKeysUsedByCmdExist(t *testing.T) {
	out, err := exec.Command("go", "list", "-f", "{{.Dir}}", "../..").Output()
	if err != nil {
		t.Skipf("cannot locate the module root: %v", err)
	}
	root := strings.TrimSpace(string(out))

	files, err := os.ReadDir(root + "/cmd")
	if err != nil {
		t.Skipf("cannot read cmd: %v", err)
	}
	// ui.X("key") and i18n.T("key") are the two ways a message is referenced.
	call := regexp.MustCompile(`ui\.(?:Step|Info|OK|Warn|Detail)\("([^"]+)"\)`)
	trans := regexp.MustCompile(`(?:i18n\.T|ui\.T)\("([^"]+)"`)

	seen := map[string]bool{}
	for _, f := range files {
		if f.IsDir() || !strings.HasSuffix(f.Name(), ".go") || strings.HasSuffix(f.Name(), "_test.go") {
			continue
		}
		src, err := os.ReadFile(root + "/cmd/" + f.Name())
		if err != nil {
			t.Fatal(err)
		}
		for _, re := range []*regexp.Regexp{call, trans} {
			for _, m := range re.FindAllStringSubmatch(string(src), -1) {
				if keyPattern(m[1]) {
					seen[m[1]] = true
				}
			}
		}
	}
	if len(seen) == 0 {
		t.Skip("no catalogue keys found in the command layer")
	}
	var missing []string
	for k := range seen {
		if !Has(EN, k) {
			missing = append(missing, k)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf("%d keys used by cmd are not in the catalogue: %v", len(missing), missing)
	}
	t.Logf("verified %d keys referenced from cmd", len(seen))
}
