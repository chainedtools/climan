package auth

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAuthorizeURLIncludesPKCE(t *testing.T) {
	got, err := AuthorizeURL(
		"https://api.chained.tools/v1/auth/oauth/authorize",
		"climan",
		"http://127.0.0.1:1234/callback",
		"chal",
		"st",
		"profile email",
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"https://api.chained.tools/v1/auth/oauth/authorize?",
		"response_type=code",
		"client_id=climan",
		"code_challenge=chal",
		"code_challenge_method=S256",
		"state=st",
		"redirect_uri=",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("url %q missing %q", got, want)
		}
	}
}

func TestFetchLoginConfigAndToken(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/auth/login-config", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(LoginConfig{
			Issuer:                "https://api.example/v1/auth",
			AuthorizationEndpoint: "https://api.example/v1/auth/oauth/authorize",
			TokenEndpoint:         "",
			ConsentURL:            "https://accounts.chained.tools/oauth-consent",
			ClientID:              "climan",
			Scopes:                "profile email",
		})
	})
	mux.HandleFunc("/v1/auth/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method %s", r.Method)
		}
		body, _ := io.ReadAll(r.Body)
		form := string(body)
		if !strings.Contains(form, "grant_type=authorization_code") || !strings.Contains(form, "code_verifier=ver") {
			t.Errorf("form = %q", form)
		}
		_ = json.NewEncoder(w).Encode(TokenSet{AccessToken: "at", RefreshToken: "rt", ExpiresIn: 120})
	})
	mux.HandleFunc("/v1/auth/oauth/userinfo", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer at" {
			t.Errorf("auth %q", r.Header.Get("Authorization"))
		}
		_ = json.NewEncoder(w).Encode(UserInfo{Sub: "user_1", Email: "a@b.co"})
	})
	mux.HandleFunc("/v1/cli/credentials", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(CLICredentials{UserID: "user_1", Email: "a@b.co"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	c := &Client{APIBase: srv.URL, HTTP: srv.Client()}
	ctx := context.Background()
	cfg, err := c.FetchLoginConfig(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ClientID != "climan" {
		t.Fatalf("client_id = %q", cfg.ClientID)
	}
	if cfg.TokenEndpoint != "https://api.example/v1/auth/oauth/token" {
		t.Fatalf("token filled from issuer: %q", cfg.TokenEndpoint)
	}
	if cfg.ConsentURL != DefaultConsentURL {
		t.Fatalf("consent = %q", cfg.ConsentURL)
	}

	ts, err := c.ExchangeCode(ctx, srv.URL+"/v1/auth/oauth/token", "climan", "code", "http://127.0.0.1/callback", "ver")
	if err != nil {
		t.Fatal(err)
	}
	if ts.AccessToken != "at" || ts.RefreshToken != "rt" || ts.ExpiresIn != 120 {
		t.Fatalf("token %+v", ts)
	}
	info, err := c.FetchUserInfo(ctx, srv.URL+"/v1/auth/oauth/userinfo", "at")
	if err != nil {
		t.Fatal(err)
	}
	if info.Sub != "user_1" || info.Email != "a@b.co" {
		t.Fatalf("userinfo %+v", info)
	}
	creds, err := c.FetchCLICredentials(ctx, "at")
	if err != nil {
		t.Fatal(err)
	}
	if creds.UserID != "user_1" {
		t.Fatalf("creds %+v", creds)
	}
}
