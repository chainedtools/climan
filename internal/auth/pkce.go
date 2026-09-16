package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
)

// Pair is a PKCE verifier/challenge plus CSRF state (RFC 7636 S256).
type Pair struct {
	Verifier  string
	Challenge string
	State     string
}

// Generate returns a fresh PKCE pair and CSRF state.
func Generate() (Pair, error) {
	verifier, err := randomB64(32)
	if err != nil {
		return Pair{}, err
	}
	sum := sha256.Sum256([]byte(verifier))
	state, err := randomB64(16)
	if err != nil {
		return Pair{}, err
	}
	return Pair{
		Verifier:  verifier,
		Challenge: base64.RawURLEncoding.EncodeToString(sum[:]),
		State:     state,
	}, nil
}

func randomB64(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("pkce: entropy: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
