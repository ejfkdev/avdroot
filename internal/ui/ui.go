// Package ui renders progress and results on the terminal. Colour is applied
// only when the output is a terminal and NO_COLOR is unset, so redirected
// output stays clean.
//
// Messages are composed by the caller, which translates them with i18n.T. The
// plain helpers (Step, Info, OK, Warn, Detail, Line) print a finished string;
// the f-suffixed ones format a literal layout string. Keeping the two apart
// means no computed string is ever used as a format, which "go vet" rejects,
// and it keeps translation at the call site where the context is known.
package ui

import (
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
)

var (
	enabled = detectColor()
	bold    = color("1")
	dim     = color("2")
	red     = color("31")
	green   = color("32")
	yellow  = color("33")
	cyan    = color("36")
)

func detectColor() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	if os.Getenv("TERM") == "dumb" {
		return false
	}
	if runtime.GOOS == "windows" && !windowsSupportsANSI() {
		return false
	}
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// windowsSupportsANSI reports whether the terminal understands ANSI escapes.
// The legacy console does not, and Go does not enable virtual terminal
// processing on its behalf, so colour is limited to terminals known to handle
// it natively.
func windowsSupportsANSI() bool {
	for _, v := range []string{"WT_SESSION", "ANSICON", "ConEmuANSI", "TERM_PROGRAM"} {
		if os.Getenv(v) != "" {
			return true
		}
	}
	if term := os.Getenv("TERM"); term != "" && term != "dumb" {
		return true
	}
	return false
}

func color(code string) func(string) string {
	return func(s string) string {
		if !enabled {
			return s
		}
		return "\x1b[" + code + "m" + s + "\x1b[0m"
	}
}

// Writer is where progress goes. Tests can redirect it.
var Writer io.Writer = os.Stdout

// ErrWriter is where failures go.
var ErrWriter io.Writer = os.Stderr

// Step announces the start of a phase.
func Step(msg string) { fmt.Fprintf(Writer, "%s %s\n", cyan("==>"), msg) }

// Info reports a neutral detail, indented under the current step.
func Info(msg string) { fmt.Fprintf(Writer, "    %s\n", msg) }

// Detail reports a secondary detail in dim text.
func Detail(msg string) { fmt.Fprintf(Writer, "    %s\n", dim(msg)) }

// OK reports success.
func OK(msg string) { fmt.Fprintf(Writer, "%s %s\n", green("  ok"), msg) }

// Warn reports something the user should notice but which is not fatal.
func Warn(msg string) { fmt.Fprintf(Writer, "%s %s\n", yellow("warn"), msg) }

// Line prints a message with no prefix.
func Line(msg string) { fmt.Fprintln(Writer, msg) }

func Infof(format string, a ...any)   { Info(fmt.Sprintf(format, a...)) }
func OKf(format string, a ...any)     { OK(fmt.Sprintf(format, a...)) }
func Warnf(format string, a ...any)   { Warn(fmt.Sprintf(format, a...)) }
func Detailf(format string, a ...any) { Detail(fmt.Sprintf(format, a...)) }
func Linef(format string, a ...any)   { Line(fmt.Sprintf(format, a...)) }

// Printf writes directly, for layout the helpers do not cover.
func Printf(format string, a ...any) { fmt.Fprintf(Writer, format, a...) }

// Fail reports an error to stderr.
func Fail(msg string) { fmt.Fprintf(ErrWriter, "%s %s\n", red("err "), msg) }

// Bold emphasises text.
func Bold(s string) string { return bold(s) }

// Dim de-emphasises text.
func Dim(s string) string { return dim(s) }

// Table renders a simple aligned table.
type Table struct {
	Headers []string
	Rows    [][]string
}

// Add appends a row.
func (t *Table) Add(cells ...string) { t.Rows = append(t.Rows, cells) }

// Render writes the table with columns padded to their widest cell.
func (t *Table) Render() {
	widths := make([]int, len(t.Headers))
	for i, h := range t.Headers {
		widths[i] = DisplayWidth(h)
	}
	for _, row := range t.Rows {
		for i, c := range row {
			if i < len(widths) && DisplayWidth(c) > widths[i] {
				widths[i] = DisplayWidth(c)
			}
		}
	}
	line := func(cells []string, style func(string) string) {
		var b strings.Builder
		for i, c := range cells {
			if i > 0 {
				b.WriteString("  ")
			}
			b.WriteString(style(c))
			if i < len(cells)-1 {
				b.WriteString(strings.Repeat(" ", widths[i]-DisplayWidth(c)))
			}
		}
		fmt.Fprintln(Writer, strings.TrimRight(b.String(), " "))
	}
	line(t.Headers, Bold)
	for _, row := range t.Rows {
		line(row, func(s string) string { return s })
	}
}

// DisplayWidth counts the columns a string occupies. CJK characters are
// double-width, so a translated table would otherwise be misaligned.
func DisplayWidth(s string) int {
	w := 0
	for _, r := range s {
		w += runeWidth(r)
	}
	return w
}

// runeWidth returns 2 for characters in the wide East Asian ranges and 1
// otherwise, which covers Chinese and the full-width punctuation used with it.
func runeWidth(r rune) int {
	switch {
	case r < 0x1100:
		return 1
	case r >= 0x1100 && r <= 0x115F, // Hangul Jamo
		r >= 0x2E80 && r <= 0x303E, // CJK radicals, Kangxi, CJK punctuation
		r >= 0x3041 && r <= 0x33FF, // Hiragana, Katakana, CJK compatibility
		r >= 0x3400 && r <= 0x4DBF, // CJK extension A
		r >= 0x4E00 && r <= 0x9FFF, // CJK unified ideographs
		r >= 0xA000 && r <= 0xA4CF, // Yi
		r >= 0xAC00 && r <= 0xD7A3, // Hangul syllables
		r >= 0xF900 && r <= 0xFAFF, // CJK compatibility ideographs
		r >= 0xFE30 && r <= 0xFE6F, // CJK compatibility forms
		r >= 0xFF00 && r <= 0xFF60, // Fullwidth forms
		r >= 0xFFE0 && r <= 0xFFE6:
		return 2
	default:
		return 1
	}
}

// Field prints an indented "label  value" line, padding the label to a fixed
// column measured in display width so that translated labels stay aligned.
func Field(label, value string) {
	const width = 14
	pad := width - DisplayWidth(label)
	if pad < 1 {
		pad = 1
	}
	Linef("  %s%s%s", label, strings.Repeat(" ", pad), value)
}

// Fieldf prints a field whose value is formatted.
func Fieldf(label, format string, a ...any) {
	Field(label, fmt.Sprintf(format, a...))
}
