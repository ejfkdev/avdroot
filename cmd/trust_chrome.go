package cmd

import (
	"crypto/x509"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ejfkdev/avdroot/internal/certutil"
	"github.com/ejfkdev/avdroot/internal/i18n"
	"github.com/ejfkdev/avdroot/internal/ui"
)

// chromeCommandLineFiles are the paths Chromium reads startup flags from.
//
// Chrome and the WebView read different files, and the location differs between
// production and userdebug builds, so all of them are written. Chromium only
// reads a file whose permissions prevent modification while running.
var chromeCommandLineFiles = []string{
	"/data/local/chrome-command-line",
	"/data/local/android-webview-command-line",
	"/data/local/webview-command-line",
	"/data/local/content-shell-command-line",
	"/data/local/tmp/chrome-command-line",
	"/data/local/tmp/android-webview-command-line",
	"/data/local/tmp/webview-command-line",
	"/data/local/tmp/content-shell-command-line",
}

// candidateCADirs are where a locally-installed CA certificate lives, as opposed
// to the stock certificates that ship with the system image.
var candidateCADirs = []string{
	"/data/misc/keychain/cacerts-added",               // installed through Settings
	"/data/adb/modules/*/system/etc/security/cacerts", // installed by a Magisk module
	"/data/data/*/files/certificate",                  // kept by the tool itself
}

func newTrustChromeCmd(a *app) *cobra.Command {
	var (
		certPath  string
		clear     bool
		noRestart bool
	)
	cmd := &cobra.Command{
		Use:     "trust-chrome",
		Short:   i18n.T("trust.short"),
		Long:    i18n.T("trust.long"),
		Example: i18n.T("trust.example"),
		Args:    cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireNoArgs(cmd, args); err != nil {
				return err
			}
			if a.adbCli == nil {
				return i18n.Errorf("err.adb_required_trust")
			}
			if _, err := a.adbCli.Online(); err != nil {
				return i18n.Errorf("err.no_emulator", err)
			}

			ui.Step(i18n.T("trust.checking"))
			// Writing under /data/local and removing files needs root.
			if err := a.adbCli.EnsureRoot(); err != nil {
				return i18n.Errorf("err.root_required_trust", err)
			}

			if clear {
				return clearChromeTrust(a, noRestart)
			}

			cert, source, err := a.resolveCA(certPath)
			if err != nil {
				return err
			}
			ui.OK(i18n.T("trust.cert_summary", certutil.Subject(cert)))
			ui.Detail(i18n.T("trust.cert_source", source))
			if !certutil.IsCA(cert) {
				ui.Warn(i18n.T("trust.not_ca"))
			}

			spki := certutil.SPKI(cert)
			ui.Info(i18n.T("trust.spki", spki))

			ui.Step(i18n.T("trust.writing"))
			line := fmt.Sprintf("chrome --ignore-certificate-errors-spki-list=%s\n", spki)
			wrote, err := writeChromeCommandLine(a, line)
			if err != nil {
				return err
			}
			ui.OK(i18n.T("trust.written", wrote))

			if noRestart {
				ui.Info(i18n.T("trust.restart_hint"))
			} else {
				restartChrome(a)
			}

			ui.Printf("\n")
			ui.Info(i18n.T("trust.caveat"))
			ui.Detail(i18n.T("trust.caveat_key"))
			return nil
		},
	}
	cmd.Flags().StringVar(&certPath, "cert", "", i18n.T("flag.cert"))
	cmd.Flags().BoolVar(&clear, "clear", false, i18n.T("flag.clear_trust"))
	cmd.Flags().BoolVar(&noRestart, "no-restart", false, i18n.T("flag.no_restart"))
	return cmd
}

// resolveCA returns the certificate to trust and where it came from.
//
// An explicit path is accepted either as a local file or as a path on the
// device, because both are natural ways to refer to a certificate.
func (a *app) resolveCA(certPath string) (*x509.Certificate, string, error) {
	if certPath != "" {
		if data, err := os.ReadFile(certPath); err == nil {
			cert, err := certutil.Parse(data)
			if err != nil {
				return nil, "", i18n.Errorf("err.cert_unreadable", certPath, err)
			}
			return cert, certPath, nil
		}
		// Not a local file: try it as a path on the device.
		out, err := a.adbCli.Shellf("cat %q 2>/dev/null", certPath)
		if err != nil || strings.TrimSpace(out) == "" {
			return nil, "", i18n.Errorf("err.cert_unreadable", certPath,
				fmt.Errorf("not found locally or on the device"))
		}
		cert, err := certutil.Parse([]byte(out))
		if err != nil {
			return nil, "", i18n.Errorf("err.cert_unreadable", certPath, err)
		}
		return cert, certPath, nil
	}

	candidates, err := a.candidateCAs()
	if err != nil {
		return nil, "", err
	}
	switch len(candidates) {
	case 0:
		return nil, "", i18n.Errorf("err.no_candidate_ca", strings.Join(candidateCADirs, "\n  "))
	case 1:
		out, err := a.adbCli.Shellf("cat %q", candidates[0])
		if err != nil {
			return nil, "", i18n.Errorf("err.cert_unreadable", candidates[0], err)
		}
		cert, err := certutil.Parse([]byte(out))
		if err != nil {
			return nil, "", i18n.Errorf("err.cert_unreadable", candidates[0], err)
		}
		return cert, candidates[0], nil
	default:
		// Several installed CAs: picking one silently could whitelist the wrong
		// key, so the choice is left to the user.
		return nil, "", i18n.Errorf("err.multiple_candidate_ca", strings.Join(candidates, "\n  "))
	}
}

// candidateCAs lists the certificates installed on the device that are not part
// of the system image.
func (a *app) candidateCAs() ([]string, error) {
	seen := map[string]bool{}
	var paths []string
	for _, dir := range candidateCADirs {
		out, err := a.adbCli.Shellf("ls -1 %s/* 2>/dev/null", dir)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(out, "\n") {
			p := strings.TrimSpace(line)
			if p == "" || strings.HasSuffix(p, ":") || seen[p] {
				continue
			}
			seen[p] = true
			// A directory may hold unrelated files beside the certificate, so
			// each candidate is read and parsed: only something that parses as
			// a certificate is offered.
			data, err := a.adbCli.Shellf("cat %q 2>/dev/null", p)
			if err != nil || strings.TrimSpace(data) == "" {
				continue
			}
			if _, err := certutil.Parse([]byte(data)); err != nil {
				continue
			}
			paths = append(paths, p)
		}
	}
	return paths, nil
}

// writeChromeCommandLine writes the flag to every path Chromium reads it from
// and returns how many were written.
//
// Chromium ignores the file unless it is readable but not writable, hence the
// 0555 mode.
func writeChromeCommandLine(a *app, line string) (int, error) {
	tmp, err := os.CreateTemp("", "avdroot-chrome-command-line*")
	if err != nil {
		return 0, err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.WriteString(line); err != nil {
		tmp.Close()
		return 0, err
	}
	if err := tmp.Close(); err != nil {
		return 0, err
	}
	if err := os.Chmod(tmp.Name(), 0o444); err != nil {
		return 0, err
	}

	written := 0
	for _, remote := range chromeCommandLineFiles {
		if out, err := a.adbCli.Run("push", tmp.Name(), remote); err != nil {
			ui.Detail(i18n.T("trust.write_failed", filepath.Base(remote), firstLine(out)))
			continue
		}
		if _, err := a.adbCli.Shellf("chmod 555 %q", remote); err != nil {
			ui.Detail(i18n.T("trust.chmod_failed", remote))
			continue
		}
		written++
	}
	if written == 0 {
		return 0, i18n.Errorf("err.trust_write_failed")
	}
	return written, nil
}

// clearChromeTrust removes the flag files, which undoes what this command did.
func clearChromeTrust(a *app, noRestart bool) error {
	ui.Step(i18n.T("trust.clearing"))
	quoted := make([]string, len(chromeCommandLineFiles))
	for i, p := range chromeCommandLineFiles {
		quoted[i] = fmt.Sprintf("%q", p)
	}
	if _, err := a.adbCli.Shellf("rm -f %s", strings.Join(quoted, " ")); err != nil {
		return i18n.Errorf("err.trust_clear_failed", err)
	}
	ui.OK(i18n.T("trust.cleared"))
	if !noRestart {
		restartChrome(a)
	}
	return nil
}

// restartChrome restarts the browser so it reads the flag file, which is only
// consulted at startup.
func restartChrome(a *app) {
	ui.Step(i18n.T("trust.restarting"))
	_, _ = a.adbCli.Shell("am force-stop com.android.chrome")
	ui.OK(i18n.T("trust.restarted"))
}
