package cli

import (
	"fmt"

	"github.com/spf13/cobra"
	"go.solved.gg/climan/internal/config"
	"go.solved.gg/climan/internal/installer"
)

func newRemoveCmd(a *app) *cobra.Command {
	var keepManifest bool
	cmd := &cobra.Command{
		Use:   "remove <tool>",
		Short: "Uninstall a tool and drop it from the manifest",
		Long: `remove uninstalls a tool from your system (npm uninstall, removal
of install directories, or deletion of the standalone binary) and removes its
entry from climan.yaml. Use --keep-manifest to uninstall without touching the
manifest.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			tool, err := a.toolArg(name)
			if err != nil {
				return err
			}
			if err := a.loadConfig(); err != nil {
				return err
			}
			inst := a.installerFor(cmd)
			st := inst.Status(tool, "")
			if st.State == installer.StateMissing {
				return fmt.Errorf("%s is not installed", name)
			}
			if !a.opts.dryRun {
				ok, err := a.confirm("remove %s from the system?", name)
				if err != nil {
					return err
				}
				if !ok {
					a.println("aborted")
					return nil
				}
			}
			if err := inst.Remove(cmd.Context(), tool); err != nil {
				return err
			}
			a.printf("✓ %s removed\n", name)
			if keepManifest {
				return nil
			}
			if a.cfg.Remove(name) {
				path := a.configPath()
				if err := a.cfg.Save(path); err != nil {
					return err
				}
				a.printf("  dropped %s from %s\n", name, path)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&keepManifest, "keep-manifest", false, "uninstall but keep the manifest entry")
	return cmd
}

// configPath returns the effective manifest path.
func (a *app) configPath() string {
	if a.opts.configPath != "" {
		return a.opts.configPath
	}
	return config.FindPath()
}
