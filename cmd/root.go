// Package cmd implements the avdroot command line interface.
package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/ejfkdev/avdroot/internal/adb"
	"github.com/ejfkdev/avdroot/internal/avd"
	"github.com/ejfkdev/avdroot/internal/i18n"
	"github.com/ejfkdev/avdroot/internal/magisk"
	"github.com/ejfkdev/avdroot/internal/ui"
)

// RepoURL is the project's home on GitHub, shown in the help output so that a
// user who receives only the binary can find the source and report a problem.
const RepoURL = "https://github.com/ejfkdev/avdroot"

// app carries the resolved global state shared by every command.
type app struct {
	// flags
	sdkPath    string
	avdHome    string
	serial     string
	magiskZIP  string
	abi        string
	compress   string
	preinit    string
	lang       string
	noADB      bool
	noDownload bool

	sdk    *avd.SDK
	adbCli *adb.Client
}

// Execute runs the CLI. version is reported by --version and in the help
// header, and is set at build time from the git tag.
func Execute(version string) {
	// The help text is rendered before any command runs, so the language has to
	// be settled first, from the flag if it is present and otherwise from the
	// environment.
	i18n.Init(langFromArgs(os.Args))

	root := newRootCmd(version)
	if err := root.Execute(); err != nil {
		ui.Fail(i18n.T(err.Error()))
		os.Exit(1)
	}
}

// langFromArgs extracts --lang from the raw arguments, so that even the help
// output is translated.
func langFromArgs(args []string) string {
	for i, a := range args {
		if v, ok := strings.CutPrefix(a, "--lang="); ok {
			return v
		}
		if a == "--lang" && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

func newRootCmd(version string) *cobra.Command {
	a := &app{}

	root := &cobra.Command{
		Use:     i18n.T("app.name"),
		Short:   i18n.T("app.tagline"),
		Long:    i18n.T("app.long"),
		Example: i18n.T("app.example"),
		Version: version,

		SilenceUsage:  true,
		SilenceErrors: true,

		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// --help and completion must work without an SDK present.
			if cmd.Name() == "help" || cmd.Name() == "completion" {
				return nil
			}
			if a.lang != "" {
				i18n.Init(a.lang)
			}
			// Discovery warnings are raised inside the avd package, so they
			// arrive as catalogue keys and are rendered here.
			avd.WarnFunc = func(key string, a ...any) { ui.Warn(i18n.T(key, a...)) }
			sdk, err := avd.Discover(avd.Options{SDK: a.sdkPath, AVDHome: a.avdHome})
			if err != nil {
				return err
			}
			a.sdk = sdk
			if !a.noADB {
				if p, err := adb.Find(sdk.Root); err == nil {
					a.adbCli = adb.New(p, a.serial)
				}
			}
			return nil
		},
	}

	root.SetVersionTemplate("{{.Name}} {{.Version}}\n")
	setHelpTemplate(root)

	pf := root.PersistentFlags()
	pf.StringVar(&a.sdkPath, "sdk", "", i18n.T("flag.sdk"))
	pf.StringVar(&a.avdHome, "avd-home", "", i18n.T("flag.avd_home"))
	pf.StringVar(&a.serial, "serial", "", i18n.T("flag.serial"))
	pf.StringVar(&a.magiskZIP, "magisk", "", i18n.T("flag.magisk"))
	pf.StringVar(&a.abi, "abi", "", i18n.T("flag.abi"))
	pf.StringVar(&a.compress, "compress", "", i18n.T("flag.compress"))
	pf.StringVar(&a.preinit, "preinit", "", i18n.T("flag.preinit"))
	pf.StringVar(&a.lang, "lang", "", i18n.T("flag.lang"))
	pf.BoolVar(&a.noADB, "no-adb", false, i18n.T("flag.no_adb"))
	pf.BoolVar(&a.noDownload, "no-download", false, i18n.T("flag.no_download"))

	root.AddCommand(
		newRootAVDCmd(a),
		newPatchCmd(a),
		newRestoreCmd(a),
		newListCmd(a),
		newStatusCmd(a),
		newVerifyCmd(a),
		newInstallCmd(a),
		newMagiskCmd(a),
		newTrustChromeCmd(a),
		newDoctorCmd(a),
	)
	// Cobra creates its help, completion and version flags lazily, and only
	// once the command tree exists, so everything generated is localised here
	// rather than at construction time.
	root.InitDefaultHelpCmd()
	root.InitDefaultCompletionCmd()
	root.InitDefaultVersionFlag()
	translateGeneratedCommands(root)
	for _, c := range append([]*cobra.Command{root}, root.Commands()...) {
		c.InitDefaultHelpFlag()
		if f := c.Flags().Lookup("help"); f != nil {
			f.Usage = i18n.T("flag.help")
		}
		// Keyed separately from the "magisk fetch --version" flag, which
		// names a Magisk release rather than the program version.
		for _, gen := range []struct{ flag, key string }{
			{"help", "flag.help"},
			{"completion", "flag.completion"},
			{"version", "flag.program_version"},
		} {
			for _, fs := range []*pflag.FlagSet{c.Flags(), c.PersistentFlags()} {
				if f := fs.Lookup(gen.flag); f != nil {
					f.Usage = i18n.T(gen.key)
				}
			}
		}
		// pflag appends "(default true)" verbatim, which cannot be translated;
		// the descriptions state the defaults instead.
		suppressDefaultAnnotations(c)
	}
	return root
}

// translateGeneratedCommands localises the commands cobra adds on its own.
func translateGeneratedCommands(root *cobra.Command) {
	for _, c := range root.Commands() {
		switch c.Name() {
		case "help":
			c.Short = i18n.T("cmd.help_short")
			c.Long = i18n.T("cmd.help_long")
		case "completion":
			c.Short = i18n.T("cmd.completion_short")
			c.Long = i18n.T("cmd.completion_long")
		}
	}
}

// suppressDefaultAnnotations removes pflag's "(default ...)" text, which cannot
// be translated. Every flag with a non-zero default states it in its
// description instead, so nothing is lost.
//
// pflag decides whether to show the annotation by comparing DefValue against
// the zero value for the flag's type, so each type gets its own zero spelling;
// an empty string only works for booleans and strings.
func suppressDefaultAnnotations(cmd *cobra.Command) {
	for _, fs := range []*pflag.FlagSet{cmd.Flags(), cmd.PersistentFlags()} {
		fs.VisitAll(func(f *pflag.Flag) {
			switch f.Value.Type() {
			case "duration":
				f.DefValue = "0s"
			case "int", "int8", "int16", "int32", "int64",
				"uint", "uint8", "uint16", "uint32", "uint64",
				"float32", "float64", "count":
				f.DefValue = "0"
			default: // bool, string, and everything else
				f.DefValue = ""
			}
		})
	}
}

// setHelpTemplate composes the help output: program name, version, what it
// does, usage, examples, then the commands and flags.
//
// Cobra's default omits the version and the examples, which are exactly what a
// user needs first.
func setHelpTemplate(root *cobra.Command) {
	root.SetHelpTemplate(`{{with (or .Long .Short)}}{{. | trimTrailingWhitespaces}}

{{end}}{{if .Version}}` + i18n.T("help.version") + `{{.Version}}
{{end}}` + i18n.T("help.repo") + i18n.T("help.usage") + `{{if .Runnable}}
  {{.UseLine}}{{end}}{{if .HasAvailableSubCommands}}
  {{.CommandPath}} [command]{{end}}{{if .HasExample}}

` + i18n.T("help.examples") + `{{.Example}}{{end}}{{if .HasAvailableSubCommands}}

` + i18n.T("help.commands") + `{{range .Commands}}{{if (or .IsAvailableCommand (eq .Name "help"))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableLocalFlags}}

` + i18n.T("help.flags") + `{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasAvailableInheritedFlags}}

` + i18n.T("help.global_flags") + `{{.InheritedFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasHelpSubCommands}}

` + i18n.T("help.more") + `{{range .Commands}}{{if .IsAdditionalHelpTopicCommand}}
  {{rpad .CommandPath .CommandPathPadding}} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableSubCommands}}

` + i18n.T("help.help_hint") + `{{end}}
`)
}

// requireTarget resolves the positional selector into a concrete ramdisk target.
func (a *app) requireTarget(args []string) (*avd.Target, error) {
	selector := ""
	switch len(args) {
	case 0:
	case 1:
		selector = args[0]
	default:
		return nil, i18n.Errorf("err.too_many_targets", len(args))
	}
	t, err := a.sdk.Resolve(selector)
	if err != nil {
		if errors.Is(err, avd.ErrNotFound) {
			return nil, i18n.Errorf("err.target_not_found", selector)
		}
		return nil, err
	}
	return t, nil
}

// requireNoArgs rejects arguments for commands that take none, with a
// translated message rather than cobra's own.
func requireNoArgs(cmd *cobra.Command, args []string) error {
	if len(args) > 0 {
		return i18n.Errorf("err.unexpected_args", cmd.CommandPath(), len(args))
	}
	return nil
}

// payload loads the Magisk installer for the target's ABI.
func (a *app) payload(abi string) (*magisk.Payload, error) {
	if a.abi != "" {
		abi = a.abi
	}
	zipPath, err := a.ensureMagiskZIP()
	if err != nil {
		return nil, err
	}
	return a.payloadFrom(zipPath, abi)
}

// payloadFrom loads a specific installer archive.
func (a *app) payloadFrom(zipPath, abi string) (*magisk.Payload, error) {
	p, err := magisk.LoadPayload(zipPath, abi)
	if err != nil {
		return nil, err
	}
	ui.Detail(i18n.T("magisk.using", p.Description(), zipPath))
	return p, nil
}

// detectPreinit determines the preinit device, preferring an explicit override,
// then a running device, and finally leaving it unset with a warning.
func (a *app) detectPreinit() string {
	if a.preinit != "" {
		ui.Detail(i18n.T("patch.preinit_override", a.preinit))
		return a.preinit
	}
	if a.adbCli == nil {
		return ""
	}
	if _, err := a.adbCli.Online(); err != nil {
		ui.Detail(i18n.T("patch.preinit_none"))
		return ""
	}
	dev, err := a.adbCli.PreinitDevice()
	if err != nil {
		ui.Warn(i18n.T("patch.preinit_query_failed", err))
		return ""
	}
	if dev == "" {
		ui.Warn(i18n.T("patch.preinit_none_on_device"))
		return ""
	}
	ui.Detail(i18n.T("patch.preinit_detected", dev))
	return dev
}

// platformHint explains how to start an emulator when none is running.
func platformHint(sdkRoot, avdName string) string {
	exe := filepath.Join(sdkRoot, "emulator", "emulator")
	if runtime.GOOS == "windows" {
		exe += ".exe"
	}
	return fmt.Sprintf("%s -avd %s", exe, avdName)
}
