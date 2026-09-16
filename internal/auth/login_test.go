package auth

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoginStoresSession(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/auth/login-config", func(w http.ResponseWriter, r *http.Request) {
		base := "http://" + r.Host + "/v1/auth"
		_ = json.NewEncoder(w).Encode(LoginConfig{
			Issuer:                base,
			AuthorizationEndpoint: base + "/oauth/authorize",
			TokenEndpoint:         base + "/oauth/token",
			UserinfoEndpoint:      base + "/oauth/userinfo",
			RevocationEndpoint:    base + "/oauth/revoke",
			ConsentURL:            "https://accounts.chained.tools/oauth-consent",
			ClientID:              "climan",
			Scopes:                "profile email",
		})
	})
	mux.HandleFunc("/v1/auth/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), "grant_type=authorization_code") {
			http.Error(w, "bad grant", 400)
			return
		}
		_ = json.NewEncoder(w).Encode(TokenSet{
			AccessToken:  "access",
			RefreshToken: "refresh",
			ExpiresIn:    3600,
			TokenType:    "Bearer",
			Scope:        "profile email",
		})
	})
	mux.HandleFunc("/v1/auth/oauth/userinfo", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(UserInfo{Sub: "user_1", Email: "a@b.co"})
	})
	mux.HandleFunc("/v1/cli/credentials", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer access" {
			http.Error(w, "nope", 401)
			return
		}
		_ = json.NewEncoder(w).Encode(CLICredentials{UserID: "user_1", Email: "a@b.co"})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	store := filepath.Join(t.TempDir(), "credentials.json")
	var stdout strings.Builder
	f := &Flow{
		Client:    &Client{APIBase: srv.URL, HTTP: srv.Client()},
		StorePath: store,
		Stdout:    &stdout,
		Stderr:    io.Discard,
		Now:       func() time.Time { return time.Unix(1_700_000_000, 0) },
		NoBrowser: true,
	}

	// Generate() is random, so Wait echoes the state from the printed authorize URL.
	f.Wait = func(ctx context.Context, l *Listener) (Callback, error) {
		out := stdout.String()
		idx := strings.Index(out, "state=")
		if idx < 0 {
			return Callback{}, errStatus(1)
		}
		rest := out[idx+len("state="):]
		if i := strings.IndexAny(rest, "&\n "); i >= 0 {
			rest = rest[:i]
		}
		return Callback{Code: "the-code", State: rest}, nil
	}

	sess, err := f.Login(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if sess.AccessToken != "access" || sess.Email != "a@b.co" || sess.UserID != "user_1" {
		t.Fatalf("session %+v", sess)
	}
	loaded, err := LoadSession(store)
	if err != nil || loaded == nil || loaded.RefreshToken != "refresh" {
		t.Fatalf("stored %+v err=%v", loaded, err)
	}
	if !strings.Contains(stdout.String(), "accounts.chained.tools/oauth-consent") {
		t.Fatalf("expected consent url in output: %s", stdout.String())
	}
}

func TestLogoutClearsStore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credentials.json")
	if err := SaveSession(path, &Session{AccessToken: "at", RefreshToken: "rt"}); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/auth/login-config", func(w http.ResponseWriter, r *http.Request) {
		base := "http://" + r.Host + "/v1/auth"
		_ = json.NewEncoder(w).Encode(LoginConfig{
			Issuer:             base,
			RevocationEndpoint: base + "/oauth/revoke",
			ClientID:           "climan",
		})
	})
	revoked := false
	mux.HandleFunc("/v1/auth/oauth/revoke", func(w http.ResponseWriter, r *http.Request) {
		revoked = true
		w.WriteHeader(200)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	f := &Flow{
		Client:    &Client{APIBase: srv.URL, HTTP: srv.Client()},
		StorePath: path,
	}
	if err := f.Logout(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !revoked {
		t.Fatal("expected revoke")
	}
	got, err := LoadSession(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Fatalf("expected cleared store, got %+v", got)
	}
}
