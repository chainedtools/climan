package cli

import (
	"github.com/spf13/cobra"
	"go.solved.gg/climan/internal/config"
)

func newAddCmd(a *app) *cobra.Command {
	var version string
	var noInstall bool
	cmd := &cobra.Command{
		Use:   "add <tool>",
		Short: "Declare a tool in the manifest",
		Long: `add declares a tool in climan.yaml (creating the manifest if
needed) and, by default, installs it right away. Use --no-install to only
edit the manifest, or --version to pin a specific version.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if _, err := a.toolArg(name); err != nil {
				return err
			}
			if err := a.loadConfig(); err != nil {
				return err
			}
			if err := a.cfg.Add(name, version); err != nil {
				return err
			}
			path := a.opts.configPath
			if path == "" {
				path = config.FindPath()
			}
			if err := a.cfg.Save(path); err != nil {
				return err
			}
			a.printf("added %s (%s) to %s\n", name, a.cfg.Get(name), path)
			if noInstall {
				return nil
			}
			tool, _ := a.reg.Get(name)
			if err := a.installerFor(cmd).Install(cmd.Context(), tool, version); err != nil {
				return err
			}
			a.printf("✓ %s installed\n", name)
			return nil
		},
	}
	cmd.Flags().StringVar(&version, "version", "", "pin a version (default: latest)")
	cmd.Flags().BoolVar(&noInstall, "no-install", false, "only edit the manifest, do not install")
	return cmd
}
