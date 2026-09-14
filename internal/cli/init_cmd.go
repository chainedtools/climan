package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"go.solved.gg/climan/internal/config"
)

func newInitCmd(a *app) *cobra.Command {
	var force bool
	var path string
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Scaffold a climan.yaml manifest",
		Long: `init writes a declarative climan.yaml manifest declaring every
supported tool. Edit it to pin versions, then run 'climan install' to
reconcile your system.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if path == "" {
				path = a.opts.configPath
			}
			if path == "" {
				path = config.FindPath()
			}
			if _, err := os.Stat(path); err == nil && !force {
				return fmt.Errorf("%s already exists (use --force to overwrite)", path)
			}
			if err := config.DefaultManifest().Save(path); err != nil {
				return err
			}
			a.printf("wrote %s\n", path)
			a.println(config.DefaultManifest().Summary())
			a.println("next: climan install")
			return nil
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "overwrite an existing manifest")
	cmd.Flags().StringVar(&path, "path", "", "manifest path (default: climan.yaml in CWD)")
	return cmd
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print climan version",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "climan %s\n", Version)
			return nil
		},
	}
}
