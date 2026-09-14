package cli

import (
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"go.solved.gg/climan/internal/installer"
)

func newListCmd(a *app) *cobra.Command {
	var all bool
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls", "status"},
		Short:   "Show the status of every tool",
		Long: `list shows the observed state of every tool in the catalog
(default) or every tool declared in the manifest (--manifest).`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := a.loadConfig(); err != nil {
				return err
			}
			inst := a.installerFor(cmd)
			tools := a.reg.All()
			if !all {
				tools = nil
				for _, ref := range a.cfg.Tools {
					if t, ok := a.reg.Get(ref.Name); ok {
						tools = append(tools, t)
					}
				}
				if len(tools) == 0 {
					a.println("no tools declared — run 'climan add <tool>' or 'climan init'")
					return nil
				}
			}
			rows := make([]installer.Status, 0, len(tools))
			for _, t := range tools {
				rows = append(rows, inst.Status(t, a.cfg.Get(t.Name)))
			}
			printStatusTable(a, rows)
			a.printf("\nmanifest: %s\n", a.configPath())
			return nil
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "show every tool in the catalog, not just the manifest")
	return cmd
}

func printStatusTable(a *app, rows []installer.Status) {
	w := tabwriter.NewWriter(a.stdout, 2, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "TOOL\tCATEGORY\tSTATE\tVERSION\tDESIRED\tLOCATION")
	for _, r := range rows {
		state := r.State.Label()
		if r.State == installer.StateMissing {
			state += "  (" + r.InstallBy + ")"
		}
		loc := r.Path
		if loc == "" {
			loc = "—"
		}
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			r.Tool.Name,
			r.Tool.Category.Label(),
			state,
			firstLine(r.Version),
			orDash(r.Desired),
			loc,
		)
	}
	_ = w.Flush()
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return orDash(strings.TrimSpace(s))
}

func orDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "—"
	}
	return strings.TrimSpace(s)
}
