package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"go.solved.gg/climan/internal/auth"
)

func newLoginCmd(a *app) *cobra.Command {
	var noBrowser bool
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Sign in to chained.tools via PKCE",
		Long: `login starts an OAuth PKCE flow against api.chained.tools.
The API redirects the browser to accounts.chained.tools/oauth-consent
(Clerk). After you approve, climan stores credentials in
~/.config/climan/credentials.json and fetches CLI identity from the API.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			f := a.authFlow()
			f.NoBrowser = noBrowser
			f.Stdout = a.stdout
			f.Stderr = a.stderr
			sess, err := f.Login(cmd.Context())
			if err != nil {
				return err
			}
			who := sess.Email
			if who == "" {
				who = sess.UserID
			}
			if who == "" {
				a.println("signed in")
				return nil
			}
			a.printf("signed in as %s\n", who)
			return nil
		},
	}
	cmd.Flags().BoolVar(&noBrowser, "no-browser", false, "print the authorize URL and do not open a browser")
	return cmd
}

func newLogoutCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Sign out and drop stored credentials",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := a.authFlow().Logout(cmd.Context()); err != nil {
				return err
			}
			a.println("signed out")
			return nil
		},
	}
}

func newWhoAmICmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:     "whoami",
		Aliases: []string{"auth"},
		Short:   "Show the signed-in chained.tools identity",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			sess, err := a.authFlow().WhoAmI(cmd.Context())
			if err != nil {
				return err
			}
			if sess == nil {
				return fmt.Errorf("not signed in — run 'climan login'")
			}
			if sess.Email != "" {
				a.printf("%s\n", sess.Email)
			}
			if sess.UserID != "" {
				a.printf("user_id: %s\n", sess.UserID)
			}
			return nil
		},
	}
}

func (a *app) authFlow() *auth.Flow {
	api := strings.TrimSpace(a.opts.apiURL)
	if api == "" {
		api = strings.TrimSpace(os.Getenv("CLIMAN_API_URL"))
	}
	if api == "" {
		api = auth.DefaultAPIBase
	}
	return &auth.Flow{
		Client: &auth.Client{
			APIBase:   api,
			UserAgent: "climan/" + Version,
		},
		ClientID: strings.TrimSpace(os.Getenv("CLIMAN_CLIENT_ID")),
		Stdout:   a.stdout,
		Stderr:   a.stderr,
	}
}
