package cmd

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alphaleonis/cctote/internal/config"
	"github.com/alphaleonis/cctote/internal/manifest"
)

// --- Helpers ---

// seedConfig writes a config file to the given path.
func seedConfig(t *testing.T, path string, c *config.Config) {
	t.Helper()
	if err := config.Save(path, c); err != nil {
		t.Fatalf("seedConfig: %v", err)
	}
}

// loadConfigFile loads a config from disk, failing the test on error.
func loadConfigFile(t *testing.T, path string) *config.Config {
	t.Helper()
	c, err := config.Load(path)
	if err != nil {
		t.Fatalf("loadConfigFile: %v", err)
	}
	return c
}

// --- TestConfigGet ---

func TestConfigGet(t *testing.T) {
	t.Run("string_set", func(t *testing.T) {
		home := t.TempDir()
		cfgDir := filepath.Join(home, ".config", "cctote")
		cfgFile := filepath.Join(cfgDir, "cctote.toml")
		seedConfig(t, cfgFile, &config.Config{ManifestPath: "/custom/manifest.json"})

		res := execCmd(t, home, nil, "--config", cfgFile, "config", "get", "manifest_path")
		if res.err != nil {
			t.Fatalf("unexpected error: %v\nstderr: %s", res.err, res.stderr)
		}
		got := strings.TrimSpace(res.stdout)
		if got != "/custom/manifest.json" {
			t.Errorf("stdout = %q, want %q", got, "/custom/manifest.json")
		}
	})

	t.Run("string_default", func(t *testing.T) {
		home := t.TempDir()
		// No config file — appConfig will be zero-value Config.
		res := execCmd(t, home, nil, "config", "get", "manifest_path")
		if res.err != nil {
			t.Fatalf("unexpected error: %v\nstderr: %s", res.err, res.stderr)
		}
		got := strings.TrimSpace(res.stdout)
		// Should show the effective default path from manifest.DefaultPath().
		wantDefault, err := manifest.DefaultPath()
		if err != nil {
			t.Fatalf("manifest.DefaultPath: %v", err)
		}
		if got != wantDefault {
			t.Errorf("stdout = %q, want default %q", got, wantDefault)
		}
	})

	t.Run("bool_true", func(t *testing.T) {
		home := t.TempDir()
		cfgFile := filepath.Join(home, "cctote.toml")
		seedConfig(t, cfgFile, &config.Config{Chezmoi: config.ChezmoiConfig{Enabled: config.BoolPtr(true)}})

		res := execCmd(t, home, nil, "--config", cfgFile, "config", "get", "chezmoi.enabled")
		if res.err != nil {
			t.Fatalf("unexpected error: %v", res.err)
		}
		got := strings.TrimSpace(res.stdout)
		if got != "true" {
			t.Errorf("stdout = %q, want %q", got, "true")
		}
	})

	t.Run("enum_default", func(t *testing.T) {
		home := t.TempDir()
		res := execCmd(t, home, nil, "config", "get", "chezmoi.auto_re_add")
		if res.err != nil {
			t.Fatalf("unexpected error: %v", res.err)
		}
		got := strings.TrimSpace(res.stdout)
		if got != "never" {
			t.Errorf("stdout = %q, want %q", got, "never")
		}
	})

	t.Run("unknown_key", func(t *testing.T) {
		home := t.TempDir()
		res := execCmd(t, home, nil, "config", "get", "nonexistent")
		if res.err == nil {
			t.Fatal("expected error for unknown key")
		}
		if !strings.Contains(res.err.Error(), "unknown config key") {
			t.Errorf("error = %q, want it to contain %q", res.err.Error(), "unknown config key")
		}
	})

	t.Run("json", func(t *testing.T) {
		home := t.TempDir()
		cfgFile := filepath.Join(home, "cctote.toml")
		seedConfig(t, cfgFile, &config.Config{ManifestPath: "/x"})

		res := execCmd(t, home, nil, "--config", cfgFile, "--json", "config", "get", "manifest_path")
		if res.err != nil {
			t.Fatalf("unexpected error: %v", res.err)
		}
		got := parseJSON(t, res.stdout)
		if got["key"] != "manifest_path" {
			t.Errorf("key = %v, want %q", got["key"], "manifest_path")
		}
		if got["value"] != "/x" {
			t.Errorf("value = %v, want %q", got["value"], "/x")
		}
	})
}

// --- TestConfigSet ---

func TestConfigSet(t *testing.T) {
	t.Run("string_value", func(t *testing.T) {
		home := t.TempDir()
		cfgFile := filepath.Join(home, "cctote.toml")
		// Start with empty config.
		seedConfig(t, cfgFile, &config.Config{})

		res := execCmd(t, home, nil, "--config", cfgFile, "config", "set", "manifest_path", "/new/path")
		if res.err != nil {
			t.Fatalf("unexpected error: %v\nstderr: %s", res.err, res.stderr)
		}

		// Verify persistence.
		c := loadConfigFile(t, cfgFile)
		if c.ManifestPath != "/new/path" {
			t.Errorf("ManifestPath = %q, want %q", c.ManifestPath, "/new/path")
		}
	})

	t.Run("bool_true", func(t *testing.T) {
		home := t.TempDir()
		cfgFile := filepath.Join(home, "cctote.toml")
		seedConfig(t, cfgFile, &config.Config{})

		res := execCmd(t, home, nil, "--config", cfgFile, "config", "set", "chezmoi.enabled", "true")
		if res.err != nil {
			t.Fatalf("unexpected error: %v", res.err)
		}

		c := loadConfigFile(t, cfgFile)
		if c.Chezmoi.Enabled == nil || !*c.Chezmoi.Enabled {
			t.Error("Chezmoi.Enabled should be *true after set")
		}
	})

	t.Run("bool_false", func(t *testing.T) {
		home := t.TempDir()
		cfgFile := filepath.Join(home, "cctote.toml")
		seedConfig(t, cfgFile, &config.Config{Chezmoi: config.ChezmoiConfig{Enabled: config.BoolPtr(true)}})

		res := execCmd(t, home, nil, "--config", cfgFile, "config", "set", "chezmoi.enabled", "false")
		if res.err != nil {
			t.Fatalf("unexpected error: %v", res.err)
		}

		c := loadConfigFile(t, cfgFile)
		if c.Chezmoi.Enabled == nil {
			t.Fatal("Chezmoi.Enabled = nil, want *false (explicitly set)")
		}
		if *c.Chezmoi.Enabled {
			t.Error("Chezmoi.Enabled = true, want false")
		}
	})

	t.Run("invalid_bool", func(t *testing.T) {
		home := t.TempDir()
		cfgFile := filepath.Join(home, "cctote.toml")
		seedConfig(t, cfgFile, &config.Config{})

		res := execCmd(t, home, nil, "--config", cfgFile, "config", "set", "chezmoi.enabled", "notabool")
		if res.err == nil {
			t.Fatal("expected error for invalid boolean")
		}
		if !strings.Contains(res.err.Error(), "invalid boolean") {
			t.Errorf("error = %q, want it to contain %q", res.err.Error(), "invalid boolean")
		}
	})

	t.Run("unknown_key", func(t *testing.T) {
		home := t.TempDir()
		res := execCmd(t, home, nil, "config", "set", "nonexistent", "value")
		if res.err == nil {
			t.Fatal("expected error for unknown key")
		}
		if !strings.Contains(res.err.Error(), "unknown config key") {
			t.Errorf("error = %q, want it to contain %q", res.err.Error(), "unknown config key")
		}
	})

	t.Run("creates_file", func(t *testing.T) {
		home := t.TempDir()
		cfgFile := filepath.Join(home, "new", "dir", "cctote.toml")

		res := execCmd(t, home, nil, "--config", cfgFile, "config", "set", "manifest_path", "/test")
		if res.err != nil {
			t.Fatalf("unexpected error: %v\nstderr: %s", res.err, res.stderr)
		}

		c := loadConfigFile(t, cfgFile)
		if c.ManifestPath != "/test" {
			t.Errorf("ManifestPath = %q, want %q", c.ManifestPath, "/test")
		}
	})

	t.Run("json", func(t *testing.T) {
		home := t.TempDir()
		cfgFile := filepath.Join(home, "cctote.toml")
		seedConfig(t, cfgFile, &config.Config{})

		res := execCmd(t, home, nil, "--config", cfgFile, "--json", "config", "set", "manifest_path", "/j")
		if res.err != nil {
			t.Fatalf("unexpected error: %v", res.err)
		}
		got := parseJSON(t, res.stdout)
		if got["key"] != "manifest_path" {
			t.Errorf("key = %v, want %q", got["key"], "manifest_path")
		}
		if got["value"] != "/j" {
			t.Errorf("value = %v, want %q", got["value"], "/j")
		}
	})
}

// --- TestConfigReset ---

func TestConfigReset(t *testing.T) {
	t.Run("string", func(t *testing.T) {
		home := t.TempDir()
		cfgFile := filepath.Join(home, "cctote.toml")
		seedConfig(t, cfgFile, &config.Config{ManifestPath: "/custom"})

		res := execCmd(t, home, nil, "--config", cfgFile, "config", "reset", "manifest_path")
		if res.err != nil {
			t.Fatalf("unexpected error: %v\nstderr: %s", res.err, res.stderr)
		}

		c := loadConfigFile(t, cfgFile)
		if c.ManifestPath != "" {
			t.Errorf("ManifestPath = %q, want empty", c.ManifestPath)
		}
	})

	t.Run("bool", func(t *testing.T) {
		home := t.TempDir()
		cfgFile := filepath.Join(home, "cctote.toml")
		seedConfig(t, cfgFile, &config.Config{Chezmoi: config.ChezmoiConfig{Enabled: config.BoolPtr(true)}})

		res := execCmd(t, home, nil, "--config", cfgFile, "config", "reset", "chezmoi.enabled")
		if res.err != nil {
			t.Fatalf("unexpected error: %v", res.err)
		}

		c := loadConfigFile(t, cfgFile)
		if c.Chezmoi.Enabled != nil {
			t.Errorf("Chezmoi.Enabled = %v, want nil after reset", c.Chezmoi.Enabled)
		}
	})

	t.Run("unknown_key", func(t *testing.T) {
		home := t.TempDir()
		res := execCmd(t, home, nil, "config", "reset", "nonexistent")
		if res.err == nil {
			t.Fatal("expected error for unknown key")
		}
		if !strings.Contains(res.err.Error(), "unknown config key") {
			t.Errorf("error = %q, want it to contain %q", res.err.Error(), "unknown config key")
		}
	})

	t.Run("already_default", func(t *testing.T) {
		home := t.TempDir()
		cfgFile := filepath.Join(home, "cctote.toml")
		seedConfig(t, cfgFile, &config.Config{})

		res := execCmd(t, home, nil, "--config", cfgFile, "config", "reset", "manifest_path")
		if res.err != nil {
			t.Fatalf("unexpected error: %v", res.err)
		}

		// File should still be valid.
		c := loadConfigFile(t, cfgFile)
		if c.ManifestPath != "" {
			t.Errorf("ManifestPath = %q, want empty", c.ManifestPath)
		}
	})

	t.Run("json", func(t *testing.T) {
		home := t.TempDir()
		cfgFile := filepath.Join(home, "cctote.toml")
		seedConfig(t, cfgFile, &config.Config{ManifestPath: "/old"})

		res := execCmd(t, home, nil, "--config", cfgFile, "--json", "config", "reset", "manifest_path")
		if res.err != nil {
			t.Fatalf("unexpected error: %v", res.err)
		}
		got := parseJSON(t, res.stdout)
		if got["key"] != "manifest_path" {
			t.Errorf("key = %v, want %q", got["key"], "manifest_path")
		}
		wantDefault, err := manifest.DefaultPath()
		if err != nil {
			t.Fatalf("manifest.DefaultPath: %v", err)
		}
		if got["default"] != wantDefault {
			t.Errorf("default = %v, want %q", got["default"], wantDefault)
		}
	})
}

// --- TestConfigList ---

func TestConfigList(t *testing.T) {
	t.Run("with_values", func(t *testing.T) {
		home := t.TempDir()
		cfgFile := filepath.Join(home, "cctote.toml")
		seedConfig(t, cfgFile, &config.Config{
			ManifestPath: "/custom/manifest.json",
			Chezmoi: config.ChezmoiConfig{
				Enabled:   config.BoolPtr(true),
				AutoReAdd: config.StrPtr("always"),
			},
		})

		res := execCmd(t, home, nil, "--config", cfgFile, "config", "list")
		if res.err != nil {
			t.Fatalf("unexpected error: %v\nstderr: %s", res.err, res.stderr)
		}
		// Table output should contain all 3 keys with their values.
		for _, want := range []string{
			"manifest_path", "/custom/manifest.json",
			"chezmoi.enabled", "true",
			"chezmoi.auto_re_add", "always",
		} {
			if !strings.Contains(res.stdout, want) {
				t.Errorf("stdout missing %q:\n%s", want, res.stdout)
			}
		}
	})

	t.Run("defaults", func(t *testing.T) {
		home := t.TempDir()
		// No config file — all defaults.
		res := execCmd(t, home, nil, "config", "list")
		if res.err != nil {
			t.Fatalf("unexpected error: %v\nstderr: %s", res.err, res.stderr)
		}
		// Should still show all 3 keys with default values.
		for _, key := range []string{"manifest_path", "chezmoi.enabled", "chezmoi.auto_re_add"} {
			if !strings.Contains(res.stdout, key) {
				t.Errorf("stdout missing key %q:\n%s", key, res.stdout)
			}
		}
		// chezmoi.enabled default should show "false", auto_re_add default should show "never".
		if !strings.Contains(res.stdout, "false") {
			t.Errorf("stdout should contain 'false' for default bool:\n%s", res.stdout)
		}
		if !strings.Contains(res.stdout, "never") {
			t.Errorf("stdout should contain 'never' for default auto_re_add:\n%s", res.stdout)
		}
	})

	t.Run("json", func(t *testing.T) {
		home := t.TempDir()
		cfgFile := filepath.Join(home, "cctote.toml")
		seedConfig(t, cfgFile, &config.Config{
			ManifestPath: "/custom",
			Chezmoi:      config.ChezmoiConfig{Enabled: config.BoolPtr(true)},
		})

		res := execCmd(t, home, nil, "--config", cfgFile, "--json", "config", "list")
		if res.err != nil {
			t.Fatalf("unexpected error: %v", res.err)
		}

		var items []map[string]any
		if err := json.Unmarshal([]byte(res.stdout), &items); err != nil {
			t.Fatalf("parseJSON: %v\nstdout: %s", err, res.stdout)
		}

		if len(items) < 3 {
			t.Fatalf("got %d items, want at least 3", len(items))
		}

		findItem := func(key string) map[string]any {
			for _, item := range items {
				if item["key"] == key {
					return item
				}
			}
			t.Fatalf("item with key %q not found in list output", key)
			return nil
		}

		mp := findItem("manifest_path")
		if mp["value"] != "/custom" {
			t.Errorf("manifest_path value = %v, want %q", mp["value"], "/custom")
		}
		if mp["isSet"] != true {
			t.Errorf("manifest_path isSet = %v, want true", mp["isSet"])
		}

		ara := findItem("chezmoi.auto_re_add")
		if ara["isSet"] != false {
			t.Errorf("chezmoi.auto_re_add isSet = %v, want false", ara["isSet"])
		}
	})
}
