package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"go.solved.gg/climan/internal/registry"
)

const (
	// DefaultAPIBase is the production Gleam API.
	DefaultAPIBase = "https://api.chained.tools"
	// DefaultIssuer is the PKCE issuer served by the API.
	DefaultIssuer = DefaultAPIBase + "/v1/auth"
	// DefaultConsentURL is Clerk's hosted OAuth consent page.
	DefaultConsentURL = "https://accounts.chained.tools/oauth-consent"
	// DefaultScopes matches the API login-config.
	DefaultScopes = "profile email offline_access openid"
)

// LoginConfig is GET /v1/auth/login-config.
type LoginConfig struct {
	Issuer                string   `json:"issuer"`
	AuthorizationEndpoint string   `json:"authorization_endpoint"`
	TokenEndpoint         string   `json:"token_endpoint"`
	UserinfoEndpoint      string   `json:"userinfo_endpoint"`
	RevocationEndpoint    string   `json:"revocation_endpoint"`
	JWKSURI               string   `json:"jwks_uri"`
	ConsentURL            string   `json:"consent_url"`
	ClientID              string   `json:"client_id"`
	Scopes                string   `json:"scopes"`
	CodeChallengeMethods  []string `json:"code_challenge_methods_supported"`
}

// TokenSet is a subset of the OAuth token response.
type TokenSet struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	Scope        string `json:"scope"`
	ExpiresIn    int64  `json:"expires_in"`
}

// UserInfo is GET /oauth/userinfo.
type UserInfo struct {
	Sub   string `json:"sub"`
	Email string `json:"email"`
}

// StatusError is an HTTP error from the API.
type StatusError struct {
	Status int
	URL    string
	Body   string
}

func (e *StatusError) Error() string {
	if e == nil {
		return "api error"
	}
	return fmt.Sprintf("GET %s: HTTP %d", e.URL, e.Status)
}

// IsUnauthorized reports whether err is an HTTP 401 from the API.
func IsUnauthorized(err error) bool {
	var s *StatusError
	return errors.As(err, &s) && s.Status == http.StatusUnauthorized
}

// Bootstrap is GET /v1/climan/bootstrap.
type Bootstrap struct {
	Service     string `json:"service"`
	MinVersion  string `json:"min_version"`
	UserID      string `json:"user_id"`
	Email       string `json:"email"`
	ToolsURL    string `json:"tools_url"`
	ReleasesURL string `json:"releases_url"`
	SdksURL     string `json:"sdks_url"`
	Login       string `json:"login"`
}

// ReleaseTicket is GET /v1/releases/{slug}/{version}/{file}.
type ReleaseTicket struct {
	URL           string            `json:"url"`
	Authorization string            `json:"authorization"`
	Date          string            `json:"date"`
	Nonce         string            `json:"nonce"`
	Expiry        string            `json:"expiry"`
	Azp           string            `json:"azp"`
	Sub           string            `json:"sub"`
	KeyID         string            `json:"key_id"`
	Headers       map[string]string `json:"headers"`
}

// SDKArtifact is GET /v1/sdks/{slug}/{version}.
type SDKArtifact struct {
	Slug      string `json:"slug"`
	Version   string `json:"version"`
	URL       string `json:"url"`
	SHA256URL string `json:"sha256_url"`
	Language  string `json:"language"`
}

// CLICredentials is GET /v1/cli/credentials.
type CLICredentials struct {
	UserID    string `json:"user_id"`
	Email     string `json:"email"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Issuer    string `json:"issuer"`
}

// Client talks to api.chained.tools (token/userinfo/credentials) over HTTP.
type Client struct {
	APIBase   string
	HTTP      *http.Client
	UserAgent string
}

func (c *Client) http() *http.Client {
	if c != nil && c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 30 * time.Second}
}

func (c *Client) apiBase() string {
	if c != nil && c.APIBase != "" {
		return strings.TrimRight(c.APIBase, "/")
	}
	return DefaultAPIBase
}

func (c *Client) userAgent() string {
	if c != nil && c.UserAgent != "" {
		return c.UserAgent
	}
	return "climan"
}

// FetchLoginConfig loads PKCE endpoints from the API.
func (c *Client) FetchLoginConfig(ctx context.Context) (*LoginConfig, error) {
	var cfg LoginConfig
	if err := c.getJSON(ctx, c.apiBase()+"/v1/auth/login-config", "", &cfg); err != nil {
		return nil, err
	}
	if cfg.Issuer == "" {
		cfg.Issuer = c.apiBase() + "/v1/auth"
	}
	if cfg.AuthorizationEndpoint == "" {
		cfg.AuthorizationEndpoint = cfg.Issuer + "/oauth/authorize"
	}
	if cfg.TokenEndpoint == "" {
		cfg.TokenEndpoint = cfg.Issuer + "/oauth/token"
	}
	if cfg.UserinfoEndpoint == "" {
		cfg.UserinfoEndpoint = cfg.Issuer + "/oauth/userinfo"
	}
	if cfg.RevocationEndpoint == "" {
		cfg.RevocationEndpoint = cfg.Issuer + "/oauth/revoke"
	}
	if cfg.ConsentURL == "" {
		cfg.ConsentURL = DefaultConsentURL
	}
	if cfg.Scopes == "" {
		cfg.Scopes = DefaultScopes
	}
	return &cfg, nil
}

// AuthorizeURL builds the PKCE authorize URL against the API (which 302s to Clerk consent).
func AuthorizeURL(authorizationEndpoint, clientID, redirectURI, challenge, state, scopes string) (string, error) {
	u, err := url.Parse(authorizationEndpoint)
	if err != nil {
		return "", fmt.Errorf("authorize url: %w", err)
	}
	q := u.Query()
	q.Set("response_type", "code")
	q.Set("client_id", clientID)
	q.Set("redirect_uri", redirectURI)
	q.Set("code_challenge", challenge)
	q.Set("code_challenge_method", "S256")
	q.Set("state", state)
	if scopes != "" {
		q.Set("scope", scopes)
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// ExchangeCode POSTs authorization_code + PKCE verifier to the token endpoint.
func (c *Client) ExchangeCode(ctx context.Context, tokenURL, clientID, code, redirectURI, verifier string) (*TokenSet, error) {
	return c.postToken(ctx, tokenURL, url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"client_id":     {clientID},
		"code_verifier": {verifier},
	})
}

// Refresh exchanges a refresh token for a new access token.
func (c *Client) Refresh(ctx context.Context, tokenURL, clientID, refreshToken string) (*TokenSet, error) {
	return c.postToken(ctx, tokenURL, url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
		"client_id":     {clientID},
	})
}

// Revoke best-effort revokes a token. Errors are returned but callers may ignore them.
func (c *Client) Revoke(ctx context.Context, revokeURL, clientID, token string) error {
	if revokeURL == "" || token == "" {
		return nil
	}
	form := url.Values{
		"token":     {token},
		"client_id": {clientID},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, revokeURL, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.userAgent())
	resp, err := c.http().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode >= 400 {
		return fmt.Errorf("revoke: HTTP %d", resp.StatusCode)
	}
	return nil
}

// FetchUserInfo loads the OIDC userinfo document.
func (c *Client) FetchUserInfo(ctx context.Context, userinfoURL, accessToken string) (*UserInfo, error) {
	var info UserInfo
	if err := c.getJSON(ctx, userinfoURL, accessToken, &info); err != nil {
		return nil, err
	}
	if info.Sub == "" {
		return nil, fmt.Errorf("userinfo: missing sub")
	}
	return &info, nil
}

// FetchCLICredentials loads identity from the API with the access token.
func (c *Client) FetchCLICredentials(ctx context.Context, accessToken string) (*CLICredentials, error) {
	var creds CLICredentials
	if err := c.getJSON(ctx, c.apiBase()+"/v1/cli/credentials", accessToken, &creds); err != nil {
		return nil, err
	}
	return &creds, nil
}

// FetchBootstrap loads climan session metadata from the API.
func (c *Client) FetchBootstrap(ctx context.Context, accessToken string) (*Bootstrap, error) {
	var boot Bootstrap
	if err := c.getJSON(ctx, c.apiBase()+"/v1/climan/bootstrap", accessToken, &boot); err != nil {
		return nil, err
	}
	return &boot, nil
}

// FetchTools loads the live tool catalog from the API.
func (c *Client) FetchTools(ctx context.Context, accessToken string) (*registry.Registry, error) {
	body, err := c.getBytes(ctx, c.apiBase()+"/v1/climan/tools", accessToken, 4<<20)
	if err != nil {
		return nil, err
	}
	return registry.ParseCatalog(body)
}

// FetchReleaseTicket mints a path-bound pull ticket for a first-party artifact.
func (c *Client) FetchReleaseTicket(ctx context.Context, accessToken, slug, version, file string) (*ReleaseTicket, error) {
	var ticket ReleaseTicket
	path := c.apiBase() + "/v1/releases/" + url.PathEscape(slug) + "/" + url.PathEscape(version) + "/" + url.PathEscape(file)
	if err := c.getJSON(ctx, path, accessToken, &ticket); err != nil {
		return nil, err
	}
	if ticket.URL == "" || ticket.Authorization == "" {
		return nil, fmt.Errorf("releases ticket missing url or authorization")
	}
	return &ticket, nil
}

// FetchReleaseVersions lists published versions for a first-party product.
func (c *Client) FetchReleaseVersions(ctx context.Context, accessToken, slug string) ([]string, error) {
	var info struct {
		Versions []string `json:"versions"`
	}
	if err := c.getJSON(ctx, c.apiBase()+"/v1/releases/"+url.PathEscape(slug), accessToken, &info); err != nil {
		return nil, err
	}
	return info.Versions, nil
}

// FetchSDKArtifact returns the public download URL for an SDK tarball.
func (c *Client) FetchSDKArtifact(ctx context.Context, accessToken, slug, version string) (*SDKArtifact, error) {
	var art SDKArtifact
	path := c.apiBase() + "/v1/sdks/" + url.PathEscape(slug) + "/" + url.PathEscape(version)
	if err := c.getJSON(ctx, path, accessToken, &art); err != nil {
		return nil, err
	}
	if art.URL == "" {
		return nil, fmt.Errorf("sdk artifact missing url")
	}
	return &art, nil
}

func (c *Client) postToken(ctx context.Context, tokenURL string, form url.Values) (*TokenSet, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.userAgent())
	resp, err := c.http().Do(req)
	if err != nil {
		return nil, fmt.Errorf("token: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("token: read: %w", err)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("token: HTTP %d", resp.StatusCode)
	}
	var ts TokenSet
	if err := json.Unmarshal(body, &ts); err != nil {
		return nil, fmt.Errorf("token: parse: %w", err)
	}
	if ts.AccessToken == "" {
		return nil, fmt.Errorf("token: missing access_token")
	}
	if ts.ExpiresIn <= 0 {
		ts.ExpiresIn = 3600
	}
	return &ts, nil
}

func (c *Client) getJSON(ctx context.Context, rawURL, bearer string, dest any) error {
	body, err := c.getBytes(ctx, rawURL, bearer, 1<<20)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(body, dest); err != nil {
		return fmt.Errorf("GET %s: parse: %w", rawURL, err)
	}
	return nil
}

func (c *Client) getBytes(ctx context.Context, rawURL, bearer string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.userAgent())
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := c.http().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit))
	if err != nil {
		return nil, fmt.Errorf("GET %s: read: %w", rawURL, err)
	}
	if resp.StatusCode >= 400 {
		return nil, &StatusError{Status: resp.StatusCode, URL: rawURL, Body: string(body)}
	}
	return body, nil
}
