// Package config loads, validates and persists climan's declarative
// manifest (climan.yaml). The manifest is the source of truth: climan
// reconciles the system against it.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"go.solved.gg/climan/internal/registry"
	"go.solved.gg/climan/internal/system"
	"gopkg.in/yaml.v3"
)

const (
	// ManifestName is the default manifest file name.
	ManifestName = "climan.yaml"
	// ConfigVersion is the manifest schema version.
	ConfigVersion = 1
)

// ToolRef is one entry in the manifest.
type ToolRef struct {
	Name    string `yaml:"name"`
	Version string `yaml:"version,omitempty"`
}

// Config is the top-level declarative manifest.
type Config struct {
	Version int       `yaml:"version"`
	BinDir  string    `yaml:"bin_dir,omitempty"`
	Tools   []ToolRef `yaml:"tools"`
}

// DefaultBinDir is where climan places standalone binaries.
func DefaultBinDir() string {
	return filepath.Join(system.Home(), ".local", "bin")
}

// DefaultManifest returns a manifest declaring every known tool at "latest".
func DefaultManifest() *Config {
	return DefaultManifestWith(registry.New())
}

// DefaultManifestWith declares tools from reg that are marked for init.
func DefaultManifestWith(reg *registry.Registry) *Config {
	cfg := &Config{Version: ConfigVersion, BinDir: DefaultBinDir()}
	for _, t := range reg.All() {
		if !t.Init {
			continue
		}
		cfg.Tools = append(cfg.Tools, ToolRef{Name: t.Name, Version: "latest"})
	}
	return cfg
}

// DefaultPaths returns candidate manifest locations in priority order.
func DefaultPaths() []string {
	return []string{
		ManifestName,
		filepath.Join(system.Home(), ".config", "climan", ManifestName),
		filepath.Join(system.Home(), ".climan", ManifestName),
	}
}

// FindPath locates an existing manifest, or returns the first default path
// when none exists (used for writes).
func FindPath() string {
	for _, p := range DefaultPaths() {
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			return p
		}
	}
	return DefaultPaths()[0]
}

// Load reads and validates a manifest. Missing files yield DefaultManifest.
// Unknown tool names are rejected.
func Load(path string) (*Config, error) {
	if path == "" {
		path = FindPath()
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultManifest(), nil
		}
		return nil, fmt.Errorf("read manifest %s: %w", path, err)
	}
	cfg := &Config{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse manifest %s: %w", path, err)
	}
	if cfg.Version == 0 {
		cfg.Version = ConfigVersion
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid manifest %s: %w", path, err)
	}
	return cfg, nil
}

// LoadWith is Load, validating tool names against the given registry.
func LoadWith(path string, reg *registry.Registry) (*Config, error) {
	if path == "" {
		path = FindPath()
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultManifestWith(reg), nil
		}
		return nil, fmt.Errorf("read manifest %s: %w", path, err)
	}
	cfg := &Config{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse manifest %s: %w", path, err)
	}
	if cfg.Version == 0 {
		cfg.Version = ConfigVersion
	}
	if err := cfg.ValidateWith(reg); err != nil {
		return nil, fmt.Errorf("invalid manifest %s: %w", path, err)
	}
	return cfg, nil
}

// Save writes the manifest atomically.
func (c *Config) Save(path string) error {
	if path == "" {
		path = FindPath()
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Validate checks the manifest against the built-in registry.
func (c *Config) Validate() error {
	return c.ValidateWith(registry.New())
}

// ValidateWith checks the manifest against the given registry.
func (c *Config) ValidateWith(reg *registry.Registry) error {
	if c.Version != ConfigVersion {
		return fmt.Errorf("unsupported manifest version %d (want %d)", c.Version, ConfigVersion)
	}
	if c.BinDir == "" {
		c.BinDir = DefaultBinDir()
	}
	seen := map[string]bool{}
	for i, t := range c.Tools {
		if t.Name == "" {
			return fmt.Errorf("tool #%d has no name", i+1)
		}
		if _, ok := reg.Get(t.Name); !ok {
			return fmt.Errorf("unknown tool %q (known: %s)", t.Name, reg.SuggestNames())
		}
		if seen[t.Name] {
			return fmt.Errorf("duplicate tool %q", t.Name)
		}
		seen[t.Name] = true
		if t.Version == "" {
			c.Tools[i].Version = "latest"
		}
	}
	return nil
}

// Get returns the desired version for a tool (default "latest").
func (c *Config) Get(name string) string {
	for _, t := range c.Tools {
		if t.Name == name {
			return t.Version
		}
	}
	return "latest"
}

// Has reports whether a tool is declared.
func (c *Config) Has(name string) bool {
	for _, t := range c.Tools {
		if t.Name == name {
			return true
		}
	}
	return false
}

// Add inserts or updates a tool, keeping manifest order deterministic.
func (c *Config) Add(name, version string) error {
	return c.AddWith(registry.New(), name, version)
}

// AddWith inserts or updates a tool using the given registry for name checks.
func (c *Config) AddWith(reg *registry.Registry, name, version string) error {
	if _, ok := reg.Get(name); !ok {
		return fmt.Errorf("unknown tool %q (known: %s)", name, reg.SuggestNames())
	}
	if version == "" {
		version = "latest"
	}
	for i := range c.Tools {
		if c.Tools[i].Name == name {
			c.Tools[i].Version = version
			return nil
		}
	}
	c.Tools = append(c.Tools, ToolRef{Name: name, Version: version})
	sort.Slice(c.Tools, func(i, j int) bool { return c.Tools[i].Name < c.Tools[j].Name })
	return nil
}

// Remove drops a tool from the manifest. Returns false when it wasn't there.
func (c *Config) Remove(name string) bool {
	out := c.Tools[:0]
	found := false
	for _, t := range c.Tools {
		if t.Name == name {
			found = true
			continue
		}
		out = append(out, t)
	}
	c.Tools = out
	return found
}

// Summary renders the manifest as a readable YAML-like listing.
func (c *Config) Summary() string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "manifest version: %d\n", c.Version)
	fmt.Fprintf(&sb, "bin_dir: %s\n", c.BinDir)
	fmt.Fprintf(&sb, "tools (%d):\n", len(c.Tools))
	for _, t := range c.Tools {
		fmt.Fprintf(&sb, "  - %-10s %s\n", t.Name, t.Version)
	}
	return sb.String()
}
