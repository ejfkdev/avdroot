// Package i18n holds the message catalogue used for output and help text.
//
// Messages are identified by short keys such as "patch.checking_target", and
// arguments are substituted into positional placeholders:
//
//	i18n.T("patch.will_overwrite", path)   // "will overwrite /path/to/ramdisk.img"
//
// Placeholders are written {0}, {1} and so on rather than %s, for two reasons:
// a translation can reorder its arguments, and no caller ever passes a computed
// string as a format, which "go vet" rightly rejects.
//
// A string that does not look like a key is returned unchanged, so values and
// layout fragments flow through the same helper without being translated.
//
// The English catalogue is authoritative. A key missing from another language
// falls back to English, and a key present in no catalogue is returned as the
// key itself, which makes an untranslated string obvious rather than silent.
package i18n

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Lang is a supported output language.
type Lang string

const (
	// EN is English, the fallback for everything.
	EN Lang = "en"
	// ZH is Simplified Chinese.
	ZH Lang = "zh"
)

// LangEnv overrides detection, e.g. AVDROOT_LANG=zh.
const LangEnv = "AVDROOT_LANG"

var (
	current  Lang = EN
	catalogs      = map[Lang]map[string]string{
		EN: english,
		ZH: chinese,
	}
)

// Available lists the supported languages, English first.
func Available() []Lang { return []Lang{EN, ZH} }

// Init picks the language. The order is: the explicit argument, AVDROOT_LANG,
// then the operating system's locale, then English.
func Init(explicit string) Lang {
	if explicit != "" {
		if l, ok := Parse(explicit); ok {
			current = l
			return current
		}
	}
	if v := os.Getenv(LangEnv); v != "" {
		if l, ok := Parse(v); ok {
			current = l
			return current
		}
	}
	if l, ok := Parse(systemLocale()); ok {
		current = l
		return current
	}
	current = EN
	return current
}

// Parse maps a locale string such as "zh_CN.UTF-8", "zh-Hans" or "en_US" onto a
// supported language.
func Parse(s string) (Lang, bool) {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return EN, false
	}
	// Drop the encoding and modifier: "zh_cn.utf-8@euro" -> "zh_cn".
	if i := strings.IndexAny(s, ".@"); i >= 0 {
		s = s[:i]
	}
	// Normalise separators: "zh-cn" -> "zh_cn".
	s = strings.ReplaceAll(s, "-", "_")
	base, _, _ := strings.Cut(s, "_")
	switch base {
	case "en":
		return EN, true
	case "zh":
		// Simplified and traditional both map here: the catalogue is
		// Simplified, which is far more useful to those users than English.
		return ZH, true
	default:
		return EN, false
	}
}

// systemLocale returns the locale reported by the environment. On Windows,
// where LC_ALL and LANG are usually unset, a platform-specific lookup is used
// instead.
func systemLocale() string {
	for _, name := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if v := os.Getenv(name); v != "" && v != "C" && v != "POSIX" {
			return v
		}
	}
	return osLocale()
}

// keyPattern reports whether a string is a catalogue key rather than literal
// text. Keys are dot-separated lowercase identifiers, e.g. "root.granting_su",
// and every segment must be non-empty so that a stray "a..b" or a sentence
// containing a full stop is not mistaken for one.
func keyPattern(s string) bool {
	segments := strings.Split(s, ".")
	if len(segments) < 2 {
		return false
	}
	for _, seg := range segments {
		if seg == "" {
			return false
		}
		for _, r := range seg {
			switch {
			case r >= 'a' && r <= 'z':
			case r >= '0' && r <= '9':
			case r == '_':
			default:
				return false
			}
		}
	}
	return true
}

// T translates a key, substituting the arguments into {0}, {1}, ... A string
// that is not a key is returned unchanged.
func T(key string, a ...any) string {
	return lookup(current, key, a...)
}

// lookup resolves key in lang, falling back to English and then to the key.
func lookup(lang Lang, key string, a ...any) string {
	if !keyPattern(key) {
		return key
	}
	tmpl, ok := catalogs[lang][key]
	if !ok {
		tmpl, ok = catalogs[EN][key]
	}
	if !ok {
		return key
	}
	if len(a) == 0 {
		return tmpl
	}
	return substitute(tmpl, a)
}

// substitute replaces {0}, {1}, ... with the corresponding argument. Braces
// without a valid index are left alone, so a catalogue entry can contain them
// literally.
func substitute(tmpl string, a []any) string {
	var b strings.Builder
	b.Grow(len(tmpl) + 16*len(a))
	for i := 0; i < len(tmpl); {
		c := tmpl[i]
		if c != '{' {
			b.WriteByte(c)
			i++
			continue
		}
		end := strings.IndexByte(tmpl[i:], '}')
		if end < 0 {
			b.WriteString(tmpl[i:])
			break
		}
		idx := tmpl[i+1 : i+end]
		n, err := strconv.Atoi(idx)
		if err != nil || n < 0 || n >= len(a) {
			// Not a placeholder we know: keep it verbatim.
			b.WriteString(tmpl[i : i+end+1])
			i += end + 1
			continue
		}
		b.WriteString(argString(a[n]))
		i += end + 1
	}
	return b.String()
}

// argString renders one argument. Errors are unwrapped so that a translated
// message reads as prose rather than as a Go error chain.
func argString(v any) string {
	if v == nil {
		return ""
	}
	if err, ok := v.(error); ok {
		return err.Error()
	}
	return fmt.Sprint(v)
}

// Has reports whether key exists in the given language.
func Has(lang Lang, key string) bool {
	_, ok := catalogs[lang][key]
	return ok
}

// Keys returns every key in a language, for tests.
func Keys(lang Lang) []string {
	out := make([]string, 0, len(catalogs[lang]))
	for k := range catalogs[lang] {
		out = append(out, k)
	}
	return out
}

// Errorf builds an error whose message is translated and formatted with the
// positional placeholders used by the catalogue.
//
// It exists because fmt.Errorf cannot be used here: catalogue entries use {0}
// rather than %s, so passing a translated message to fmt.Errorf would print the
// placeholder verbatim.
func Errorf(key string, a ...any) error {
	return errors.New(T(key, a...))
}
