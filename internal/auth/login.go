package auth

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

// Flow is the interactive PKCE login against api.chained.tools.
type Flow struct {
	Client     *Client
	ClientID   string
	StorePath  string
	Stdout     io.Writer
	Stderr     io.Writer
	OpenURL    func(string) error
	Now        func() time.Time
	Wait       func(ctx context.Context, l *Listener) (Callback, error)
	NoBrowser  bool
}

func (f *Flow) stdout() io.Writer {
	if f != nil && f.Stdout != nil {
		return f.Stdout
	}
	return os.Stdout
}

func (f *Flow) stderr() io.Writer {
	if f != nil && f.Stderr != nil {
		return f.Stderr
	}
	return os.Stderr
}

func (f *Flow) now() time.Time {
	if f != nil && f.Now != nil {
		return f.Now()
	}
	return time.Now()
}

func (f *Flow) client() *Client {
	if f != nil && f.Client != nil {
		return f.Client
	}
	return &Client{}
}

func (f *Flow) storePath() string {
	if f != nil && f.StorePath != "" {
		return f.StorePath
	}
	return DefaultStorePath()
}

func (f *Flow) openURL(rawURL string) error {
	if f != nil && f.NoBrowser {
		return nil
	}
	if f != nil && f.OpenURL != nil {
		return f.OpenURL(rawURL)
	}
	return OpenBrowser(rawURL)
}

// Login runs PKCE, stores the session, and fetches CLI credentials from the API.
func (f *Flow) Login(ctx context.Context) (*Session, error) {
	c := f.client()
	cfg, err := c.FetchLoginConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("login-config: %w", err)
	}
	clientID := strings.TrimSpace(f.ClientID)
	if clientID == "" {
		clientID = strings.TrimSpace(cfg.ClientID)
	}
	if clientID == "" {
		clientID = strings.TrimSpace(os.Getenv("CLIMAN_CLIENT_ID"))
	}
	if clientID == "" {
		return nil, fmt.Errorf("missing OAuth client id (set CLIMAN_CLIENT_ID or CLERK_OAUTH_CLIENT_ID on the API)")
	}

	pair, err := Generate()
	if err != nil {
		return nil, err
	}

	listener, err := StartListener()
	if err != nil {
		return nil, err
	}
	defer listener.Close()
	redirect := listener.RedirectURI()

	authURL, err := AuthorizeURL(cfg.AuthorizationEndpoint, clientID, redirect, pair.Challenge, pair.State, cfg.Scopes)
	if err != nil {
		return nil, err
	}

	fmt.Fprintln(f.stdout(), "Opening browser for sign-in…")
	fmt.Fprintf(f.stdout(), "If nothing opens, visit:\n  %s\n", authURL)
	fmt.Fprintf(f.stdout(), "Consent page: %s\n", cfg.ConsentURL)
	if err := f.openURL(authURL); err != nil {
		fmt.Fprintln(f.stderr(), "climan: could not open a browser; open the URL above manually")
	}
	fmt.Fprintln(f.stdout(), "Waiting for authorization in the browser…")

	var cb Callback
	if f.Wait != nil {
		cb, err = f.Wait(ctx, listener)
	} else {
		cb, err = listener.Wait(ctx)
	}
	if err != nil {
		return nil, err
	}
	if cb.State != pair.State {
		return nil, fmt.Errorf("OAuth state mismatch (possible CSRF); try again")
	}

	tokens, err := c.ExchangeCode(ctx, cfg.TokenEndpoint, clientID, cb.Code, redirect, pair.Verifier)
	if err != nil {
		return nil, err
	}

	sess := &Session{
		AccessToken:  tokens.AccessToken,
		RefreshToken: tokens.RefreshToken,
		TokenType:    tokens.TokenType,
		Scope:        tokens.Scope,
		ExpiresAt:    f.now().Unix() + tokens.ExpiresIn,
	}

	if info, err := c.FetchUserInfo(ctx, cfg.UserinfoEndpoint, tokens.AccessToken); err == nil {
		sess.UserID = info.Sub
		sess.Email = info.Email
	}

	if creds, err := c.FetchCLICredentials(ctx, tokens.AccessToken); err == nil {
		if creds.UserID != "" {
			sess.UserID = creds.UserID
		}
		if creds.Email != "" {
			sess.Email = creds.Email
		}
	}

	if err := SaveSession(f.storePath(), sess); err != nil {
		return nil, err
	}
	return sess, nil
}

// Logout revokes the refresh token (best-effort) and deletes local credentials.
func (f *Flow) Logout(ctx context.Context) error {
	path := f.storePath()
	sess, err := LoadSession(path)
	if err != nil {
		return err
	}
	if sess != nil {
		c := f.client()
		cfg, cfgErr := c.FetchLoginConfig(ctx)
		if cfgErr == nil {
			token := sess.RefreshToken
			if token == "" {
				token = sess.AccessToken
			}
			clientID := strings.TrimSpace(f.ClientID)
			if clientID == "" {
				clientID = cfg.ClientID
			}
			_ = c.Revoke(ctx, cfg.RevocationEndpoint, clientID, token)
		}
	}
	return ClearSession(path)
}

// WhoAmI returns the stored session, refreshing the access token if needed.
func (f *Flow) WhoAmI(ctx context.Context) (*Session, error) {
	sess, err := LoadSession(f.storePath())
	if err != nil {
		return nil, err
	}
	if sess == nil {
		return nil, nil
	}
	if !sess.Expired(f.now(), time.Minute) {
		return sess, nil
	}
	if sess.RefreshToken == "" {
		_ = ClearSession(f.storePath())
		return nil, nil
	}
	c := f.client()
	cfg, err := c.FetchLoginConfig(ctx)
	if err != nil {
		return nil, err
	}
	clientID := strings.TrimSpace(f.ClientID)
	if clientID == "" {
		clientID = cfg.ClientID
	}
	tokens, err := c.Refresh(ctx, cfg.TokenEndpoint, clientID, sess.RefreshToken)
	if err != nil {
		_ = ClearSession(f.storePath())
		return nil, fmt.Errorf("refresh: %w", err)
	}
	sess.AccessToken = tokens.AccessToken
	if tokens.RefreshToken != "" {
		sess.RefreshToken = tokens.RefreshToken
	}
	if tokens.Scope != "" {
		sess.Scope = tokens.Scope
	}
	sess.ExpiresAt = f.now().Unix() + tokens.ExpiresIn
	if err := SaveSession(f.storePath(), sess); err != nil {
		return nil, err
	}
	return sess, nil
}
