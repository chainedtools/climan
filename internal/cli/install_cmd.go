package cli

import (
	"github.com/spf13/cobra"
)

func newInstallCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "install [tool...]",
		Short: "Install tools from the manifest",
		Long: `install reconciles your system with the manifest: every declared
tool that is missing is installed at its declared version. Pass tool names to
install a subset (they do not need to be in the manifest).`,
		Example: `  climan install            # install everything declared in climan.yaml
  climan install grok claude # install just those two`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := a.loadConfig(); err != nil {
				return err
			}
			tools, err := a.resolveTools(args)
			if err != nil {
				return err
			}
			inst := a.installerFor(cmd)
			results := inst.InstallAll(cmd.Context(), tools, a.desiredFor)
			return printResults(a, results)
		},
	}
	return cmd
}

func newUpdateCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update [tool...]",
		Short: "Update installed tools to their latest version",
		Long: `update refreshes installed tools, using each tool's native update
mechanism (mise self-update, re-running official installers, npm upgrade,
git pull…). Pass tool names to update a subset.`,
		Example: `  climan update        # update everything that is installed
  climan update codex  # update just codex`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := a.loadConfig(); err != nil {
				return err
			}
			tools, err := a.resolveTools(args)
			if err != nil {
				return err
			}
			inst := a.installerFor(cmd)
			results := inst.UpdateAll(cmd.Context(), tools)
			return printResults(a, results)
		},
	}
	return cmd
}
