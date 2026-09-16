package auth

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSessionRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credentials.json")
	want := &Session{
		AccessToken:  "at",
		RefreshToken: "rt",
		TokenType:    "Bearer",
		Scope:        "profile email",
		ExpiresAt:    1_700_000_000,
		UserID:       "user_1",
		Email:        "a@b.co",
	}
	if err := SaveSession(path, want); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %o, want 0600", info.Mode().Perm())
	}
	got, err := LoadSession(path)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.AccessToken != "at" || got.Email != "a@b.co" || got.UserID != "user_1" {
		t.Fatalf("loaded %+v", got)
	}
	if err := ClearSession(path); err != nil {
		t.Fatal(err)
	}
	got, err = LoadSession(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Fatalf("expected nil after clear, got %+v", got)
	}
}

func TestLoadMissingIsNil(t *testing.T) {
	got, err := LoadSession(filepath.Join(t.TempDir(), "none.json"))
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Fatalf("missing file should be nil, got %+v", got)
	}
}

func TestExpired(t *testing.T) {
	s := &Session{ExpiresAt: time.Now().Add(time.Hour).Unix()}
	if s.Expired(time.Now(), time.Minute) {
		t.Fatal("fresh token should not be expired")
	}
	s.ExpiresAt = time.Now().Add(30 * time.Second).Unix()
	if !s.Expired(time.Now(), time.Minute) {
		t.Fatal("token within skew should be expired")
	}
}
