// Package config handles the cctote app configuration (per-machine settings).
package config

import (
	"fmt"
	"os"
	"path/filepath"

	toml "github.com/pelletier/go-toml/v2"
)

// Config holds per-machine cctote settings. All fields are optional —
// a missing config file produces a zero-value Config with nil error.
type Config struct {
	ManifestPath string        `toml:"manifest_path,omitempty"`
	Chezmoi      ChezmoiConfig `toml:"chezmoi,omitempty"`
}

// ChezmoiConfig controls chezmoi integration behavior.
// Pointer fields distinguish "explicitly set" from "never configured".
type ChezmoiConfig struct {
	Enabled   *bool   `toml:"enabled,omitempty"`
	AutoReAdd *string `toml:"auto_re_add,omitempty"`
}

// BoolPtr returns a pointer to b. Convenience for building ChezmoiConfig literals.
func BoolPtr(b bool) *bool { return &b }

// BoolVal returns the value of a *bool, defaulting to false if nil.
func BoolVal(b *bool) bool {
	if b == nil {
		return false
	}
	return *b
}

// StrPtr returns a pointer to s. Convenience for building config literals.
func StrPtr(s string) *string { return &s }

// Auto-re-add mode constants.
const (
	AutoReAddNever  = "never"
	AutoReAddAsk    = "ask"
	AutoReAddAlways = "always"
)

// AutoReAddOptions is the canonical list of valid auto_re_add values.
var AutoReAddOptions = []string{AutoReAddNever, AutoReAddAsk, AutoReAddAlways}

// AutoReAddMode returns the effective auto_re_add mode from a *string,
// defaulting to "never" if nil or unrecognized.
func AutoReAddMode(s *string) string {
	if s == nil {
		return AutoReAddNever
	}
	switch *s {
	case AutoReAddNever, AutoReAddAsk, AutoReAddAlways:
		return *s
	default:
		fmt.Fprintf(os.Stderr, "⚠ unknown chezmoi.auto_re_add value %q, defaulting to %q\n", *s, AutoReAddNever)
		return AutoReAddNever
	}
}

// DefaultPath returns the default config file path, respecting XDG_CONFIG_HOME.
func DefaultPath() (string, error) {
	if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
		return filepath.Join(dir, "cctote", "cctote.toml"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("determining home directory: %w", err)
	}
	return filepath.Join(home, ".config", "cctote", "cctote.toml"), nil
}

// Load reads and parses a config from the given path. A missing file returns
// a zero-value Config and nil error (config is optional). A malformed file
// returns an error.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{}, nil
		}
		return nil, fmt.Errorf("reading config: %w", err)
	}

	var c Config
	if err := toml.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	return &c, nil
}

// Save writes a config to the given path as TOML, creating parent directories
// as needed.
func Save(path string, c *Config) error {
	data, err := toml.Marshal(c)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}

	return nil
}
