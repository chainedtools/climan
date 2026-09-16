package auth

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"go.solved.gg/climan/internal/system"
)

const (
	storeDirName  = "climan"
	storeFileName = "credentials.json"
)

// Session is the on-disk OAuth token set plus userinfo.
type Session struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token,omitempty"`
	TokenType    string `json:"token_type,omitempty"`
	Scope        string `json:"scope,omitempty"`
	ExpiresAt    int64  `json:"expires_at"`
	UserID       string `json:"user_id,omitempty"`
	Email        string `json:"email,omitempty"`
}

// DefaultStorePath is ~/.config/climan/credentials.json.
func DefaultStorePath() string {
	return filepath.Join(system.Home(), ".config", storeDirName, storeFileName)
}

// LoadSession reads the credential file. Missing file yields (nil, nil).
func LoadSession(path string) (*Session, error) {
	if path == "" {
		path = DefaultStorePath()
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read credentials: %w", err)
	}
	var s Session
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parse credentials: %w", err)
	}
	if s.AccessToken == "" {
		return nil, nil
	}
	return &s, nil
}

// SaveSession writes credentials with mode 0600.
func SaveSession(path string, s *Session) error {
	if path == "" {
		path = DefaultStorePath()
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("credentials dir: %w", err)
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write credentials: %w", err)
	}
	if err := os.Chmod(tmp, 0o600); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("chmod credentials: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("replace credentials: %w", err)
	}
	return nil
}

// ClearSession deletes the credential file.
func ClearSession(path string) error {
	if path == "" {
		path = DefaultStorePath()
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("clear credentials: %w", err)
	}
	return nil
}

// Expired reports whether the access token is within skew of expiry.
func (s *Session) Expired(now time.Time, skew time.Duration) bool {
	if s == nil || s.ExpiresAt == 0 {
		return false
	}
	return now.Add(skew).Unix() >= s.ExpiresAt
}
