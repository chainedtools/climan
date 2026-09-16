package cli

import (
	"fmt"

	"github.com/spf13/cobra"
	"go.solved.gg/climan/internal/installer"
)

func newDoctorCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Check the environment and diagnose tool issues",
		Long: `doctor verifies prerequisites (bash, curl, tar, git, npm…),
reports the platform and manifest, and checks each tool: detected location,
version, and any setup notes. Exits non-zero when a prerequisite is missing.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := a.loadConfig(); err != nil {
				return err
			}
			inst := a.installerFor(cmd)
			a.printf("climan %s\n", Version)

			// Prerequisites.
			a.println("\nprerequisites:")
			missing := 0
			for _, req := range []string{"bash", "curl", "tar", "gzip", "git", "unzip"} {
				if p, err := lookPath(req); err == nil {
					a.printf("  ✓ %-8s %s\n", req, p)
				} else {
					a.printf("  ✗ %-8s not found\n", req)
					missing++
				}
			}
			if _, err := lookPath("npm"); err == nil {
				a.println("  ✓ npm (needed for npm-based tools)")
			} else {
				a.println("  · npm  not found (only needed for npm fallbacks)")
			}

			// Platform.
			osName, arch := osArch()
			a.printf("\nplatform: %s/%s\n", osName, arch)
			a.printf("manifest: %s (%d tools)\n", a.configPath(), len(a.cfg.Tools))
			a.printf("bin_dir:  %s\n", a.cfg.BinDir)
			if sess, err := a.authFlow().WhoAmI(cmd.Context()); err != nil {
				a.printf("auth:     error (%v)\n", err)
			} else if sess == nil {
				a.println("auth:     not signed in (climan login)")
			} else if sess.Email != "" {
				a.printf("auth:     %s\n", sess.Email)
			} else {
				a.printf("auth:     %s\n", sess.UserID)
			}

			// Per-tool checks.
			a.println("\ntools:")
			problems := 0
			for _, t := range a.reg.All() {
				st := inst.Status(t, a.cfg.Get(t.Name))
				status := "  ✓"
				switch st.State {
				case installer.StateMissing:
					status = "  ·"
				case installer.StateUnknown:
					status = "  ?"
					problems++
				}
				a.printf("%s %-10s %-14s %s\n", status, t.Name, t.Category.Label(), firstLine(st.Version))
				if st.State == installer.StateMissing {
					a.printf("    not installed (install via: climan install %s)\n", t.Name)
				}
				if st.Path != "" {
					a.printf("    %s\n", st.Path)
				}
				if t.Notes != "" {
					a.printf("    note: %s\n", t.Notes)
				}
				if st.Error != "" {
					a.printf("    error: %s\n", st.Error)
				}
			}

			if missing > 0 || problems > 0 {
				return fmt.Errorf("%d prerequisite(s) and %d tool check(s) failed", missing, problems)
			}
			a.println("\nall checks passed")
			return nil
		},
	}
	return cmd
}
