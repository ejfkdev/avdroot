//go:build !windows

package i18n

// osLocale has nothing to add on Unix, where the locale is in the environment.
func osLocale() string { return "" }
