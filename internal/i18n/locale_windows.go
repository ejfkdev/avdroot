//go:build windows

package i18n

import "syscall"

// osLocale asks Windows for the user's UI language.
//
// Windows does not set LC_ALL or LANG, so without this a Chinese Windows
// installation would fall back to English. GetUserDefaultUILanguage returns a
// LANGID whose low ten bits are the primary language id: 0x04 is Chinese.
func osLocale() string {
	lib := syscall.NewLazyDLL("kernel32.dll")
	proc := lib.NewProc("GetUserDefaultUILanguage")
	if err := proc.Find(); err != nil {
		return ""
	}
	ret, _, _ := proc.Call()
	primary := ret & 0x3ff
	switch primary {
	case 0x04: // LANG_CHINESE
		return "zh"
	case 0x09: // LANG_ENGLISH
		return "en"
	default:
		return ""
	}
}
