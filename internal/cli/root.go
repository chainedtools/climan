// Package cli implements climan's scriptable command-line interface.
package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"strings"

	"github.com/spf13/cobra"
	"go.solved.gg/climan/internal/auth"
	"go.solved.gg/climan/internal/config"
	"go.solved.gg/climan/internal/installer"
	"go.solved.gg/climan/internal/registry"
)

// Version is the climan release version, overridable at build time via
// -ldflags "-X go.solved.gg/climan/internal/cli.Version=…".
var Version = "0.1.0-dev"

func init() {
	// When installed with `go install go.solved.gg/climan@vX.Y.Z` there are no
	// ldflags; the module version is embedded in the build info instead.
	if Version == "0.1.0-dev" {
		if bi, ok := debug.ReadBuildInfo(); ok {
			if v := bi.Main.Version; v != "" && v != "(devel)" {
				Version = strings.TrimPrefix(v, "v")
			}
		}
	}
}

// globalOptions are flags shared by every command.
type globalOptions struct {
	configPath string
	dryRun     bool
	verbose    bool
	yes        bool
	binDir     string
	apiURL     string
}

// app carries shared state through command execution.
type app struct {
	opts   *globalOptions
	cfg    *config.Config
	reg    *registry.Registry
	stdout io.Writer
	stderr io.Writer
	stdin  io.Reader
}

// NewRoot builds the climan command tree.
func NewRoot() *cobra.Command {
	opts := &globalOptions{}
	a := &app{opts: opts, reg: registry.New(), stdout: os.Stdout, stderr: os.Stderr, stdin: os.Stdin}

	root := &cobra.Command{
		Use:   "climan",
		Short: "Declarative manager for popular developer CLIs",
		Long: `climan is a unified, declarative way to install, update and manage
popular developer CLIs — version managers (mise, asdf, sdkman), cloud tools
(flyctl, doctl) and AI coding agents (grok, codex, claude).

Tools are declared in a climan.yaml manifest; climan reconciles your system
against it. Use the TUI (climan tui) for graphical management or the CLI for
scripts.`,
		Version:       Version,
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return fmt.Errorf("unknown command %q — see 'climan --help'", args[0])
			}
			return cmd.Help()
		},
	}

	pf := root.PersistentFlags()
	pf.StringVar(&opts.configPath, "config", "", "path to the climan.yaml manifest (default: ./climan.yaml, ~/.config/climan/climan.yaml)")
	pf.BoolVar(&opts.dryRun, "dry-run", false, "print commands without executing them")
	pf.BoolVarP(&opts.verbose, "verbose", "v", false, "stream installer output")
	pf.BoolVarP(&opts.yes, "yes", "y", false, "skip confirmation prompts")
	pf.StringVar(&opts.binDir, "bin-dir", "", "directory for standalone binaries (default ~/.local/bin)")
	pf.StringVar(&opts.apiURL, "api-url", "", "chained.tools API base (default https://api.chained.tools, or CLIMAN_API_URL)")

	root.PersistentPreRunE = func(cmd *cobra.Command, _ []string) error {
		if skipHydrate(cmd) {
			return nil
		}
		return a.hydrate(cmd.Context())
	}

	root.AddCommand(
		newInitCmd(a),
		newAddCmd(a),
		newInstallCmd(a),
		newUpdateCmd(a),
		newRemoveCmd(a),
		newListCmd(a),
		newDoctorCmd(a),
		newLoginCmd(a),
		newLogoutCmd(a),
		newWhoAmICmd(a),
		newTUICmd(a),
		newVersionCmd(),
	)
	return root
}

func skipHydrate(cmd *cobra.Command) bool {
	switch cmd.Name() {
	case "login", "logout", "version", "help", "completion", "climan", "whoami":
		return true
	default:
		return false
	}
}

func (a *app) hydrate(ctx context.Context) error {
	flow := a.authFlow()
	sess, err := flow.WhoAmI(ctx)
	if err != nil {
		return err
	}
	if sess == nil {
		return fmt.Errorf("not signed in — run 'climan login'")
	}
	reg, err := flow.Client.FetchTools(ctx, sess.AccessToken)
	if err != nil {
		if auth.IsUnauthorized(err) {
			return fmt.Errorf("not signed in — run 'climan login'")
		}
		fmt.Fprintf(a.stderr, "climan: API catalog unavailable (%v); using built-in tools\n", err)
		return nil
	}
	a.reg = reg
	return nil
}

// Execute runs the CLI and returns a process exit code.
func Execute() int {
	root := NewRoot()
	if err := root.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "climan: %s\n", err)
		return 1
	}
	return 0
}

// loadConfig loads the manifest (or defaults) honouring the --config flag.
func (a *app) loadConfig() error {
	cfg, err := config.LoadWith(a.opts.configPath, a.reg)
	if err != nil {
		return err
	}
	a.cfg = cfg
	if a.opts.binDir != "" {
		cfg.BinDir = a.opts.binDir
	}
	return nil
}

// printf renders a formatted line to stdout, ignoring write errors (CLI output).
func (a *app) printf(format string, args ...any) {
	_, _ = fmt.Fprintf(a.stdout, format, args...)
}

// println renders a space-joined line to stdout, ignoring write errors.
func (a *app) println(args ...any) {
	_, _ = fmt.Fprintln(a.stdout, args...)
}

// confirm asks yes/no on the terminal unless --yes is set.
func (a *app) confirm(format string, args ...any) (bool, error) {
	if a.opts.yes {
		return true, nil
	}
	a.printf(format+" [y/N] ", args...)
	var resp string
	if _, err := fmt.Fscanln(a.stdin, &resp); err != nil && err.Error() != "unexpected newline" {
		return false, nil
	}
	switch strings.ToLower(strings.TrimSpace(resp)) {
	case "y", "yes":
		return true, nil
	default:
		return false, nil
	}
}

// toolArg resolves one tool name against the registry with a helpful error.
func (a *app) toolArg(name string) (*registry.Tool, error) {
	if t, ok := a.reg.Get(name); ok {
		return t, nil
	}
	return nil, fmt.Errorf("unknown tool %q (known: %s)", name, a.reg.SuggestNames())
}

// resolveTools maps CLI args to tools; empty args mean "everything in the
// manifest".
func (a *app) resolveTools(args []string) ([]*registry.Tool, error) {
	if len(args) == 0 {
		if a.cfg == nil {
			if err := a.loadConfig(); err != nil {
				return nil, err
			}
		}
		tools := make([]*registry.Tool, 0, len(a.cfg.Tools))
		for _, ref := range a.cfg.Tools {
			t, _ := a.reg.Get(ref.Name)
			tools = append(tools, t)
		}
		return tools, nil
	}
	tools := make([]*registry.Tool, 0, len(args))
	for _, name := range args {
		t, err := a.toolArg(name)
		if err != nil {
			return nil, err
		}
		tools = append(tools, t)
	}
	return tools, nil
}

// desiredFor returns the manifest version for a tool.
func (a *app) desiredFor(t *registry.Tool) string {
	if a.cfg == nil {
		if err := a.loadConfig(); err != nil {
			return "latest"
		}
	}
	return a.cfg.Get(t.Name)
}

// installerFor builds an Installer honouring the global flags.
func (a *app) installerFor(cmd *cobra.Command) *installer.Installer {
	inst := installer.New()
	if a.cfg != nil && a.cfg.BinDir != "" {
		inst.BinDir = a.cfg.BinDir
	}
	inst.DryRun = a.opts.dryRun
	if a.opts.verbose {
		inst.Out = cmd.ErrOrStderr()
	}
	inst.UseRegistry(a.reg)
	flow := a.authFlow()
	inst.API = flow.Client
	if sess, err := flow.WhoAmI(cmd.Context()); err == nil && sess != nil {
		inst.AccessToken = sess.AccessToken
	}
	return inst
}

// printResults renders per-tool operation results and returns an aggregated
// error when any operation failed.
func printResults(a *app, results []installer.Result) error {
	failed := 0
	for _, r := range results {
		if r.Err != nil {
			failed++
			a.printf("✗ %-10s %s failed: %v\n", r.Tool.Name, r.Op, r.Err)
			continue
		}
		if r.Skipped {
			a.printf("· %-10s %s (already up to date)\n", r.Tool.Name, r.Op)
			continue
		}
		a.printf("✓ %-10s %s done\n", r.Tool.Name, r.Op)
	}
	if failed > 0 {
		return fmt.Errorf("%d operation(s) failed", failed)
	}
	return nil
}
