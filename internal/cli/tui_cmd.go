package cli

import (
	tea "charm.land/bubbletea/v2"
	"github.com/spf13/cobra"
	"go.solved.gg/climan/internal/tui"
)

func newTUICmd(a *app) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tui",
		Short: "Launch the graphical terminal manager",
		Long: `tui opens a full-screen, keyboard-driven interface for managing the
tools declared in climan.yaml: install, update, remove, filter and refresh
status, with live installer output.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := a.loadConfig(); err != nil {
				return err
			}
			inst := a.installerFor(cmd)
			inst.Out = nil // TUI streams output through its own log pane
			model := tui.New(inst, a.cfg)
			p := tea.NewProgram(model)
			if _, err := p.Run(); err != nil {
				return err
			}
			return nil
		},
	}
	return cmd
}
