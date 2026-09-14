package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultManifest(t *testing.T) {
	cfg := DefaultManifest()
	if cfg.Version != ConfigVersion {
		t.Errorf("version = %d, want %d", cfg.Version, ConfigVersion)
	}
	if len(cfg.Tools) < 8 {
		t.Errorf("default manifest has %d tools, want >= 8", len(cfg.Tools))
	}
	for _, ref := range cfg.Tools {
		if ref.Version != "latest" {
			t.Errorf("default version for %s = %q, want latest", ref.Name, ref.Version)
		}
	}
}

func TestRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ManifestName)
	cfg := DefaultManifest()
	if err := cfg.Save(path); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != cfg.Version {
		t.Errorf("round-trip version mismatch")
	}
	if len(got.Tools) != len(cfg.Tools) {
		t.Errorf("round-trip tool count mismatch: %d != %d", len(got.Tools), len(cfg.Tools))
	}
	for i := range cfg.Tools {
		if got.Tools[i].Name != cfg.Tools[i].Name {
			t.Errorf("round-trip tool %d: %q != %q", i, got.Tools[i].Name, cfg.Tools[i].Name)
		}
	}
}

func TestLoadRejectsUnknownTool(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "climan.yaml")
	body := "version: 1\nbin_dir: ~/.local/bin\ntools:\n  - name: nope\n    version: latest\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(path)
	if err == nil || !strings.Contains(err.Error(), "unknown tool") {
		t.Errorf("expected unknown-tool error, got %v", err)
	}
}

func TestLoadMissingFileDefaults(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "none.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Tools) == 0 {
		t.Error("missing manifest should fall back to defaults")
	}
}

func TestAddRemove(t *testing.T) {
	cfg := DefaultManifest()
	// Re-add a tool (update version).
	if err := cfg.Add("mise", "v2026.1.0"); err != nil {
		t.Fatal(err)
	}
	if got := cfg.Get("mise"); got != "v2026.1.0" {
		t.Errorf("Get(mise) = %q, want v2026.1.0", got)
	}
	if err := cfg.Add("not-a-tool", "latest"); err == nil {
		t.Error("expected error adding unknown tool")
	}
	if !cfg.Remove("mise") {
		t.Error("Remove(mise) should return true")
	}
	if cfg.Has("mise") {
		t.Error("mise should be gone")
	}
	if cfg.Remove("mise") {
		t.Error("second Remove(mise) should return false")
	}
}

func TestBinDirDefaults(t *testing.T) {
	cfg := &Config{Version: ConfigVersion, Tools: []ToolRef{{Name: "mise"}}}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	if cfg.BinDir == "" {
		t.Error("bin_dir should default to something")
	}
}
