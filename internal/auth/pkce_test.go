package auth

import (
	"crypto/sha256"
	"encoding/base64"
	"testing"
)

func TestGenerateShapesAndChallenge(t *testing.T) {
	p, err := Generate()
	if err != nil {
		t.Fatal(err)
	}
	if p.Verifier == "" || p.Challenge == "" || p.State == "" {
		t.Fatal("empty pkce fields")
	}
	sum := sha256.Sum256([]byte(p.Verifier))
	want := base64.RawURLEncoding.EncodeToString(sum[:])
	if p.Challenge != want {
		t.Fatalf("challenge = %q, want S256(verifier) %q", p.Challenge, want)
	}
	p2, err := Generate()
	if err != nil {
		t.Fatal(err)
	}
	if p.Verifier == p2.Verifier || p.State == p2.State {
		t.Fatal("successive draws should differ")
	}
}
