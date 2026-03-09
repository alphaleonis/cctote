package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/alphaleonis/cctote/internal/claude"
	"github.com/alphaleonis/cctote/internal/cliutil"
	"github.com/alphaleonis/cctote/internal/manifest"
)

// --- Fixtures ---

var (
	pluginA = manifest.Plugin{ID: "plugin-a", Scope: "user", Enabled: true}
	pluginB = manifest.Plugin{ID: "plugin-b", Scope: "user", Enabled: false}
	pluginC = manifest.Plugin{ID: "plugin-c@my-marketplace", Scope: "user", Enabled: true}

	marketplaceCustom = manifest.Marketplace{Source: "github", Repo: "user/my-marketplace"}
)

// --- Fake runner ---

// fakeClaudeRunner dispatches by CLI args. Tests register handlers by joining
// args with spaces (e.g. "plugin list --json"). Unregistered commands return
// an error. The calls slice records all invocations for assertion.
type fakeClaudeRunner struct {
	calls    [][]string
	handlers map[string]func([]string) ([]byte, error)
}

func newFakeRunner() *fakeClaudeRunner {
	return &fakeClaudeRunner{
		handlers: make(map[string]func([]string) ([]byte, error)),
	}
}

func (f *fakeClaudeRunner) Run(_ context.Context, args ...string) ([]byte, error) {
	f.calls = append(f.calls, args)
	key := strings.Join(args, " ")
	// Try exact match first.
	if h, ok := f.handlers[key]; ok {
		return h(args)
	}
	// Try prefix match (longest-prefix-first for deterministic matching).
	prefixes := make([]string, 0, len(f.handlers))
	for k := range f.handlers {
		prefixes = append(prefixes, k)
	}
	sort.Slice(prefixes, func(i, j int) bool { return len(prefixes[i]) > len(prefixes[j]) })
	for _, prefix := range prefixes {
		if strings.HasPrefix(key, prefix) {
			return f.handlers[prefix](args)
		}
	}
	return nil, fmt.Errorf("unhandled command: %s", key)
}

// on registers a handler for the given key.
func (f *fakeClaudeRunner) on(key string, out []byte, err error) {
	f.handlers[key] = func([]string) ([]byte, error) {
		return out, err
	}
}

// onFunc registers a handler function for the given key.
func (f *fakeClaudeRunner) onFunc(key string, fn func([]string) ([]byte, error)) {
	f.handlers[key] = fn
}

// called returns true if any recorded call starts with the given prefix args.
func (f *fakeClaudeRunner) called(prefix ...string) bool {
	for _, call := range f.calls {
		if len(call) >= len(prefix) {
			match := true
			for i, p := range prefix {
				if call[i] != p {
					match = false
					break
				}
			}
			if match {
				return true
			}
		}
	}
	return false
}

// withPluginClient returns an App configurator that injects a fake Claude CLI client.
func withPluginClient(runner cliutil.Runner) func(*App) {
	return func(a *App) {
		a.newClaudeClient = func() *claude.Client { return claude.NewClient(runner) }
		a.ensureClaudeAvailable = func() error { return nil }
	}
}

// pluginsJSON encodes a slice of plugins to JSON bytes.
func pluginsJSON(plugins ...manifest.Plugin) []byte {
	// Claude CLI returns a slightly different format, but our fakeRunner
	// returns the same shape since internal/claude parses it the same way.
	type entry struct {
		ID      string `json:"id"`
		Scope   string `json:"scope"`
		Enabled bool   `json:"enabled"`
	}
	entries := make([]entry, len(plugins))
	for i, p := range plugins {
		entries[i] = entry{ID: p.ID, Scope: p.Scope, Enabled: p.Enabled}
	}
	b, _ := json.Marshal(entries)
	return b
}

// marketplacesJSON encodes a map of marketplaces to the CLI list format.
func marketplacesJSON(mps map[string]manifest.Marketplace) []byte {
	type entry struct {
		Name   string `json:"name"`
		Source string `json:"source"`
		Repo   string `json:"repo,omitempty"`
		URL    string `json:"url,omitempty"`
		Path   string `json:"path,omitempty"`
	}
	var entries []entry
	for name, mp := range mps {
		entries = append(entries, entry{
			Name:   name,
			Source: mp.Source,
			Repo:   mp.Repo,
			URL:    mp.URL,
			Path:   mp.Path,
		})
	}
	if entries == nil {
		entries = []entry{}
	}
	b, _ := json.Marshal(entries)
	return b
}

// newPluginManifest creates a manifest with the given plugins.
func newPluginManifest(plugins []manifest.Plugin, marketplaces map[string]manifest.Marketplace) *manifest.Manifest {
	m := &manifest.Manifest{
		Version:      manifest.CurrentVersion,
		MCPServers:   map[string]manifest.MCPServer{},
		Plugins:      plugins,
		Marketplaces: marketplaces,
	}
	if m.Plugins == nil {
		m.Plugins = []manifest.Plugin{}
	}
	if m.Marketplaces == nil {
		m.Marketplaces = map[string]manifest.Marketplace{}
	}
	return m
}

// --- TestPluginExport ---

func TestPluginExport(t *testing.T) {
	t.Run("export_all_fresh", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		runner := newFakeRunner()
		runner.on("plugin list --json", pluginsJSON(pluginA, pluginB), nil)
		res := execCmdWith(t, home, nil, withPluginClient(runner), "plugin", "export", "--manifest", manPath)
		if res.err != nil {
			t.Fatalf("unexpected error: %v\nstderr: %s", res.err, res.stderr)
		}

		m := loadManifestFile(t, manPath)
		if len(m.Plugins) != 2 {
			t.Errorf("manifest has %d plugins, want 2", len(m.Plugins))
		}
		if idx := manifest.FindPlugin(m.Plugins, "plugin-a"); idx < 0 {
			t.Error("manifest missing plugin-a")
		}
		if idx := manifest.FindPlugin(m.Plugins, "plugin-b"); idx < 0 {
			t.Error("manifest missing plugin-b")
		}
	})

	t.Run("export_selective", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		runner := newFakeRunner()
		runner.on("plugin list --json", pluginsJSON(pluginA, pluginB), nil)
		res := execCmdWith(t, home, nil, withPluginClient(runner), "plugin", "export", "--manifest", manPath, "plugin-a")
		if res.err != nil {
			t.Fatalf("unexpected error: %v\nstderr: %s", res.err, res.stderr)
		}

		m := loadManifestFile(t, manPath)
		if len(m.Plugins) != 1 {
			t.Errorf("manifest has %d plugins, want 1", len(m.Plugins))
		}
		if m.Plugins[0].ID != "plugin-a" {
			t.Errorf("expected plugin-a, got %s", m.Plugins[0].ID)
		}
	})

	t.Run("export_not_found", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		runner := newFakeRunner()
		runner.on("plugin list --json", pluginsJSON(pluginA), nil)
		res := execCmdWith(t, home, nil, withPluginClient(runner), "plugin", "export", "--manifest", manPath, "nonexistent")
		if res.err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(res.err.Error(), `"nonexistent" not found`) {
			t.Errorf("error %q missing expected substring", res.err.Error())
		}
	})

	t.Run("export_empty", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		runner := newFakeRunner()
		runner.on("plugin list --json", pluginsJSON(), nil)

		res := execCmdWith(t, home, nil, withPluginClient(runner), "plugin", "export", "--manifest", manPath)
		if res.err != nil {
			t.Fatalf("unexpected error: %v", res.err)
		}
		if !strings.Contains(res.stderr, "No plugins found") {
			t.Errorf("stderr missing 'No plugins found': %q", res.stderr)
		}
	})

	t.Run("export_empty_json", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		runner := newFakeRunner()
		runner.on("plugin list --json", pluginsJSON(), nil)

		res := execCmdWith(t, home, nil, withPluginClient(runner), "plugin", "export", "--manifest", manPath, "--json")
		if res.err != nil {
			t.Fatalf("unexpected error: %v", res.err)
		}

		got := parseJSON(t, res.stdout)
		if got["added"] != float64(0) {
			t.Errorf("added = %v, want 0", got["added"])
		}
	})

	t.Run("export_json", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		runner := newFakeRunner()
		runner.on("plugin list --json", pluginsJSON(pluginA, pluginB), nil)

		res := execCmdWith(t, home, nil, withPluginClient(runner), "plugin", "export", "--manifest", manPath, "--json")
		if res.err != nil {
			t.Fatalf("unexpected error: %v\nstderr: %s", res.err, res.stderr)
		}

		got := parseJSON(t, res.stdout)
		if got["added"] != float64(2) {
			t.Errorf("added = %v, want 2", got["added"])
		}
		if got["updated"] != float64(0) {
			t.Errorf("updated = %v, want 0", got["updated"])
		}
		plugins, ok := got["plugins"].([]any)
		if !ok {
			t.Fatalf("plugins is %T, want []any", got["plugins"])
		}
		if len(plugins) != 2 {
			t.Errorf("plugins has %d entries, want 2", len(plugins))
		}
	})

	t.Run("export_updates_existing", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		// Seed manifest with plugin-a (disabled).
		seedManifest(t, manPath, newPluginManifest(
			[]manifest.Plugin{{ID: "plugin-a", Scope: "user", Enabled: false}},
			nil,
		))

		// Claude has plugin-a (enabled).
		runner := newFakeRunner()
		runner.on("plugin list --json", pluginsJSON(pluginA), nil)

		res := execCmdWith(t, home, nil, withPluginClient(runner), "plugin", "export", "--manifest", manPath, "--json")
		if res.err != nil {
			t.Fatalf("unexpected error: %v\nstderr: %s", res.err, res.stderr)
		}

		got := parseJSON(t, res.stdout)
		if got["added"] != float64(0) {
			t.Errorf("added = %v, want 0", got["added"])
		}
		if got["updated"] != float64(1) {
			t.Errorf("updated = %v, want 1", got["updated"])
		}

		m := loadManifestFile(t, manPath)
		if !m.Plugins[0].Enabled {
			t.Error("plugin-a should be enabled after update")
		}
	})

	t.Run("export_marketplace_auto_export_confirmed", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		runner := newFakeRunner()
		runner.on("plugin list --json", pluginsJSON(pluginC), nil)
		runner.on("plugin marketplace list --json", marketplacesJSON(map[string]manifest.Marketplace{
			"my-marketplace": marketplaceCustom,
		}), nil)
		// Answer "y" to auto-export prompt.
		res := execCmdWith(t, home, strings.NewReader("y\n"), withPluginClient(runner), "plugin", "export", "--manifest", manPath)
		if res.err != nil {
			t.Fatalf("unexpected error: %v\nstderr: %s", res.err, res.stderr)
		}

		m := loadManifestFile(t, manPath)
		if len(m.Plugins) != 1 {
			t.Fatalf("manifest has %d plugins, want 1", len(m.Plugins))
		}
		if _, ok := m.Marketplaces["my-marketplace"]; !ok {
			t.Error("marketplace 'my-marketplace' should be auto-exported")
		}
	})

	t.Run("export_marketplace_auto_export_declined_skips_plugin", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		runner := newFakeRunner()
		runner.on("plugin list --json", pluginsJSON(pluginA, pluginC), nil)
		runner.on("plugin marketplace list --json", marketplacesJSON(map[string]manifest.Marketplace{
			"my-marketplace": marketplaceCustom,
		}), nil)
		// Answer "n" to auto-export prompt.
		res := execCmdWith(t, home, strings.NewReader("n\n"), withPluginClient(runner), "plugin", "export", "--manifest", manPath)
		if res.err != nil {
			t.Fatalf("unexpected error: %v\nstderr: %s", res.err, res.stderr)
		}

		m := loadManifestFile(t, manPath)
		// Only plugin-a should be exported; plugin-c skipped.
		if len(m.Plugins) != 1 {
			t.Fatalf("manifest has %d plugins, want 1", len(m.Plugins))
		}
		if m.Plugins[0].ID != "plugin-a" {
			t.Errorf("expected plugin-a, got %s", m.Plugins[0].ID)
		}
	})

	t.Run("export_marketplace_already_in_manifest", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		// Marketplace already in manifest — no prompt needed.
		seedManifest(t, manPath, newPluginManifest(nil, map[string]manifest.Marketplace{
			"my-marketplace": marketplaceCustom,
		}))

		runner := newFakeRunner()
		runner.on("plugin list --json", pluginsJSON(pluginC), nil)

		res := execCmdWith(t, home, nil, withPluginClient(runner), "plugin", "export", "--manifest", manPath)
		if res.err != nil {
			t.Fatalf("unexpected error: %v\nstderr: %s", res.err, res.stderr)
		}

		m := loadManifestFile(t, manPath)
		if len(m.Plugins) != 1 {
			t.Fatalf("manifest has %d plugins, want 1", len(m.Plugins))
		}
	})

	t.Run("export_json_excludes_declined_plugins", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		runner := newFakeRunner()
		runner.on("plugin list --json", pluginsJSON(pluginA, pluginC), nil)
		runner.on("plugin marketplace list --json", marketplacesJSON(map[string]manifest.Marketplace{
			"my-marketplace": marketplaceCustom,
		}), nil)
		// Answer "n" to decline marketplace auto-export.
		res := execCmdWith(t, home, strings.NewReader("n\n"), withPluginClient(runner),
			"plugin", "export", "--manifest", manPath, "--json")
		if res.err != nil {
			t.Fatalf("unexpected error: %v\nstderr: %s", res.err, res.stderr)
		}

		got := parseJSON(t, res.stdout)
		plugins, ok := got["plugins"].([]any)
		if !ok {
			t.Fatalf("plugins is %T, want []any", got["plugins"])
		}
		// Only plugin-a should appear; plugin-c was declined.
		if len(plugins) != 1 {
			t.Errorf("plugins = %v, want [plugin-a] (plugin-c should be excluded)", plugins)
		}
		if len(plugins) > 0 && plugins[0] != "plugin-a" {
			t.Errorf("plugins[0] = %v, want plugin-a", plugins[0])
		}
	})

	t.Run("export_marketplace_not_in_claude", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		runner := newFakeRunner()
		runner.on("plugin list --json", pluginsJSON(pluginC), nil)
		runner.on("plugin marketplace list --json", marketplacesJSON(nil), nil)

		res := execCmdWith(t, home, nil, withPluginClient(runner), "plugin", "export", "--manifest", manPath)
		if res.err != nil {
			t.Fatalf("unexpected error: %v\nstderr: %s", res.err, res.stderr)
		}

		// Plugin should be skipped (marketplace not in Claude).
		m := loadManifestFile(t, manPath)
		if len(m.Plugins) != 0 {
			t.Errorf("manifest has %d plugins, want 0 (marketplace not available)", len(m.Plugins))
		}
	})
}

// --- TestPluginImport ---

func TestPluginImport(t *testing.T) {
	t.Run("install_new", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		seedManifest(t, manPath, newPluginManifest(
			[]manifest.Plugin{pluginA, pluginB}, nil,
		))

		runner := newFakeRunner()
		runner.on("plugin list --json", pluginsJSON(), nil) // nothing installed
		runner.on("plugin install", nil, nil)
		runner.on("plugin enable", nil, nil)
		runner.on("plugin disable", nil, nil)

		res := execCmdWith(t, home, nil, withPluginClient(runner), "plugin", "import", "--manifest", manPath)
		if res.err != nil {
			t.Fatalf("unexpected error: %v\nstderr: %s", res.err, res.stderr)
		}
		if !strings.Contains(res.stderr, "2 installed") {
			t.Errorf("stderr missing install count: %q", res.stderr)
		}

		// Verify the correct CLI commands were issued.
		if !runner.called("plugin", "install", "plugin-a") {
			t.Error("expected 'plugin install plugin-a' call")
		}
		if !runner.called("plugin", "install", "plugin-b") {
			t.Error("expected 'plugin install plugin-b' call")
		}
		// plugin-a is Enabled: true — no SetPluginEnabled call (fresh installs default to enabled).
		if runner.called("plugin", "enable", "plugin-a") {
			t.Error("should not call 'plugin enable' for default-enabled plugin-a")
		}
		if !runner.called("plugin", "disable", "plugin-b") {
			t.Error("expected 'plugin disable plugin-b' call (plugin-b has Enabled: false)")
		}
	})

	t.Run("skip_identical", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		seedManifest(t, manPath, newPluginManifest(
			[]manifest.Plugin{pluginA}, nil,
		))

		runner := newFakeRunner()
		runner.on("plugin list --json", pluginsJSON(pluginA), nil)

		res := execCmdWith(t, home, nil, withPluginClient(runner), "plugin", "import", "--manifest", manPath, "--json")
		if res.err != nil {
			t.Fatalf("unexpected error: %v\nstderr: %s", res.err, res.stderr)
		}

		got := parseJSON(t, res.stdout)
		if got["installed"] != float64(0) {
			t.Errorf("installed = %v, want 0", got["installed"])
		}
		if got["skipped"] != float64(1) {
			t.Errorf("skipped = %v, want 1", got["skipped"])
		}
	})

	t.Run("reconcile_enabled_state", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		// Manifest wants enabled, Claude has disabled.
		seedManifest(t, manPath, newPluginManifest(
			[]manifest.Plugin{pluginA}, nil, // enabled: true
		))

		disabledA := pluginA
		disabledA.Enabled = false
		runner := newFakeRunner()
		runner.on("plugin list --json", pluginsJSON(disabledA), nil)
		runner.on("plugin enable", nil, nil)

		res := execCmdWith(t, home, nil, withPluginClient(runner), "plugin", "import", "--manifest", manPath, "--json")
		if res.err != nil {
			t.Fatalf("unexpected error: %v\nstderr: %s", res.err, res.stderr)
		}

		got := parseJSON(t, res.stdout)
		if got["reconciled"] != float64(1) {
			t.Errorf("reconciled = %v, want 1", got["reconciled"])
		}
	})

	t.Run("reconcile_disable", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		// Manifest wants disabled, Claude has enabled.
		disabledA := pluginA
		disabledA.Enabled = false
		seedManifest(t, manPath, newPluginManifest(
			[]manifest.Plugin{disabledA}, nil,
		))

		runner := newFakeRunner()
		runner.on("plugin list --json", pluginsJSON(pluginA), nil) // enabled in Claude
		runner.on("plugin disable", nil, nil)

		res := execCmdWith(t, home, nil, withPluginClient(runner), "plugin", "import", "--manifest", manPath, "--json")
		if res.err != nil {
			t.Fatalf("unexpected error: %v\nstderr: %s", res.err, res.stderr)
		}

		got := parseJSON(t, res.stdout)
		if got["reconciled"] != float64(1) {
			t.Errorf("reconciled = %v, want 1", got["reconciled"])
		}
		if !runner.called("plugin", "disable", "plugin-a") {
			t.Error("expected 'plugin disable plugin-a' call")
		}
	})

	t.Run("selective", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		seedManifest(t, manPath, newPluginManifest(
			[]manifest.Plugin{pluginA, pluginB}, nil,
		))

		runner := newFakeRunner()
		runner.on("plugin list --json", pluginsJSON(), nil)
		runner.on("plugin install", nil, nil)
		runner.on("plugin enable", nil, nil)

		res := execCmdWith(t, home, nil, withPluginClient(runner), "plugin", "import", "--manifest", manPath, "plugin-a")
		if res.err != nil {
			t.Fatalf("unexpected error: %v\nstderr: %s", res.err, res.stderr)
		}
		if !strings.Contains(res.stderr, "1 installed") {
			t.Errorf("stderr missing '1 installed': %q", res.stderr)
		}
	})

	t.Run("selective_not_in_manifest", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		seedManifest(t, manPath, newPluginManifest(
			[]manifest.Plugin{pluginA}, nil,
		))

		runner := newFakeRunner()
		runner.on("plugin list --json", pluginsJSON(), nil)

		res := execCmdWith(t, home, nil, withPluginClient(runner), "plugin", "import", "--manifest", manPath, "nonexistent")
		if res.err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(res.err.Error(), `"nonexistent" not found in manifest`) {
			t.Errorf("error %q missing expected substring", res.err.Error())
		}
	})

	t.Run("marketplace_prereq_missing", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		seedManifest(t, manPath, newPluginManifest(
			[]manifest.Plugin{pluginC}, nil,
		))

		runner := newFakeRunner()
		runner.on("plugin list --json", pluginsJSON(), nil)
		runner.on("plugin marketplace list --json", marketplacesJSON(nil), nil)

		res := execCmdWith(t, home, nil, withPluginClient(runner), "plugin", "import", "--manifest", manPath)
		if res.err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(res.err.Error(), "requires marketplace") {
			t.Errorf("error %q missing 'requires marketplace'", res.err.Error())
		}
	})

	t.Run("marketplace_prereq_satisfied", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		seedManifest(t, manPath, newPluginManifest(
			[]manifest.Plugin{pluginC},
			map[string]manifest.Marketplace{"my-marketplace": marketplaceCustom},
		))

		runner := newFakeRunner()
		runner.on("plugin list --json", pluginsJSON(), nil)
		runner.on("plugin marketplace list --json", marketplacesJSON(map[string]manifest.Marketplace{
			"my-marketplace": marketplaceCustom,
		}), nil)
		runner.on("plugin install", nil, nil)
		runner.on("plugin enable", nil, nil)

		res := execCmdWith(t, home, nil, withPluginClient(runner), "plugin", "import", "--manifest", manPath, "--json")
		if res.err != nil {
			t.Fatalf("unexpected error: %v\nstderr: %s", res.err, res.stderr)
		}

		got := parseJSON(t, res.stdout)
		if got["installed"] != float64(1) {
			t.Errorf("installed = %v, want 1", got["installed"])
		}
	})

	t.Run("strict_uninstall", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		seedManifest(t, manPath, newPluginManifest(
			[]manifest.Plugin{pluginA}, nil,
		))

		runner := newFakeRunner()
		runner.on("plugin list --json", pluginsJSON(pluginA, pluginB), nil)
		runner.on("plugin uninstall", nil, nil)

		// --force to skip confirmation.
		res := execCmdWith(t, home, nil, withPluginClient(runner), "plugin", "import", "--manifest", manPath, "--strict", "--force")
		if res.err != nil {
			t.Fatalf("unexpected error: %v\nstderr: %s", res.err, res.stderr)
		}

		if !strings.Contains(res.stderr, "1 uninstalled") {
			t.Errorf("stderr missing '1 uninstalled': %q", res.stderr)
		}

		// Verify plugin-b was uninstalled (not plugin-a).
		if !runner.called("plugin", "uninstall", "plugin-b") {
			t.Error("expected 'plugin uninstall plugin-b' call")
		}
		if runner.called("plugin", "uninstall", "plugin-a") {
			t.Error("plugin-a should NOT be uninstalled (it's in the manifest)")
		}
	})

	t.Run("strict_with_names_rejected", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		seedManifest(t, manPath, newPluginManifest(
			[]manifest.Plugin{pluginA, pluginB}, nil,
		))

		res := execCmd(t, home, nil, "plugin", "import", "--manifest", manPath, "--strict", "plugin-a")
		if res.err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(res.err.Error(), "--strict cannot be used with named plugins") {
			t.Errorf("error %q missing expected substring", res.err.Error())
		}
	})

	t.Run("dry_run", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		seedManifest(t, manPath, newPluginManifest(
			[]manifest.Plugin{pluginA, pluginB}, nil,
		))

		runner := newFakeRunner()
		runner.on("plugin list --json", pluginsJSON(), nil)

		res := execCmdWith(t, home, nil, withPluginClient(runner), "plugin", "import", "--manifest", manPath, "--dry-run", "--json")
		if res.err != nil {
			t.Fatalf("unexpected error: %v\nstderr: %s", res.err, res.stderr)
		}

		got := parseJSON(t, res.stdout)
		install, ok := got["install"].([]any)
		if !ok {
			t.Fatalf("install is %T, want []any", got["install"])
		}
		if len(install) != 2 {
			t.Errorf("install has %d entries, want 2", len(install))
		}
	})

	t.Run("dry_run_text", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		seedManifest(t, manPath, newPluginManifest(
			[]manifest.Plugin{pluginA}, nil,
		))

		runner := newFakeRunner()
		runner.on("plugin list --json", pluginsJSON(), nil)

		res := execCmdWith(t, home, nil, withPluginClient(runner), "plugin", "import", "--manifest", manPath, "--dry-run")
		if res.err != nil {
			t.Fatalf("unexpected error: %v\nstderr: %s", res.err, res.stderr)
		}
		if !strings.Contains(res.stderr, "plugin-a") {
			t.Errorf("stderr missing plugin name in dry-run: %q", res.stderr)
		}
	})

	t.Run("json_output", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		seedManifest(t, manPath, newPluginManifest(
			[]manifest.Plugin{pluginA, pluginB}, nil,
		))

		runner := newFakeRunner()
		runner.on("plugin list --json", pluginsJSON(pluginA), nil) // pluginA already installed
		runner.on("plugin install", nil, nil)
		runner.on("plugin disable", nil, nil)

		res := execCmdWith(t, home, nil, withPluginClient(runner), "plugin", "import", "--manifest", manPath, "--json")
		if res.err != nil {
			t.Fatalf("unexpected error: %v\nstderr: %s", res.err, res.stderr)
		}

		got := parseJSON(t, res.stdout)
		if got["installed"] != float64(1) {
			t.Errorf("installed = %v, want 1", got["installed"])
		}
		if got["skipped"] != float64(1) {
			t.Errorf("skipped = %v, want 1", got["skipped"])
		}
	})

	t.Run("partial_errors", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		seedManifest(t, manPath, newPluginManifest(
			[]manifest.Plugin{pluginA, pluginB}, nil,
		))

		runner := newFakeRunner()
		runner.on("plugin list --json", pluginsJSON(), nil)
		runner.onFunc("plugin install", func(args []string) ([]byte, error) {
			if len(args) >= 3 && args[2] == "plugin-b" {
				return nil, errors.New("network error")
			}
			return nil, nil
		})
		runner.on("plugin enable", nil, nil)
		runner.on("plugin disable", nil, nil)

		res := execCmdWith(t, home, nil, withPluginClient(runner), "plugin", "import", "--manifest", manPath, "--json")
		// In JSON mode, partial errors are embedded in the body and the command
		// returns nil (exit 0) so scripts can parse the structured output.
		if res.err != nil {
			t.Fatalf("in JSON mode errors should be in the body, not the return: %v", res.err)
		}
		got := parseJSON(t, res.stdout)
		if got["installed"] != float64(1) {
			t.Errorf("installed = %v, want 1", got["installed"])
		}
		errSlice, ok := got["errors"].([]any)
		if !ok || len(errSlice) == 0 {
			t.Error("expected errors in JSON output")
		}
	})
}

// --- TestPluginRemove ---

func TestPluginRemove(t *testing.T) {
	t.Run("simple", func(t *testing.T) {
		manPath := filepath.Join(t.TempDir(), "manifest.json")
		home := t.TempDir()

		seedManifest(t, manPath, newPluginManifest(
			[]manifest.Plugin{pluginA, pluginB}, nil,
		))

		res := execCmd(t, home, nil, "plugin", "remove", "--manifest", manPath, "plugin-a")
		if res.err != nil {
			t.Fatalf("unexpected error: %v\nstderr: %s", res.err, res.stderr)
		}

		m := loadManifestFile(t, manPath)
		if len(m.Plugins) != 1 {
			t.Errorf("manifest has %d plugins, want 1", len(m.Plugins))
		}
		if m.Plugins[0].ID != "plugin-b" {
			t.Errorf("remaining plugin = %s, want plugin-b", m.Plugins[0].ID)
		}
	})

	t.Run("not_found", func(t *testing.T) {
		manPath := filepath.Join(t.TempDir(), "manifest.json")
		home := t.TempDir()

		seedManifest(t, manPath, newPluginManifest(
			[]manifest.Plugin{pluginA}, nil,
		))

		res := execCmd(t, home, nil, "plugin", "remove", "--manifest", manPath, "nonexistent")
		if res.err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(res.err.Error(), `"nonexistent" not found`) {
			t.Errorf("error %q missing expected substring", res.err.Error())
		}
	})

	t.Run("json", func(t *testing.T) {
		manPath := filepath.Join(t.TempDir(), "manifest.json")
		home := t.TempDir()

		seedManifest(t, manPath, newPluginManifest(
			[]manifest.Plugin{pluginA}, nil,
		))

		res := execCmd(t, home, nil, "plugin", "remove", "--manifest", manPath, "--json", "plugin-a")
		if res.err != nil {
			t.Fatalf("unexpected error: %v\nstderr: %s", res.err, res.stderr)
		}

		got := parseJSON(t, res.stdout)
		if got["removed"] != "plugin-a" {
			t.Errorf("removed = %v, want plugin-a", got["removed"])
		}
	})

	t.Run("cascade_with_force", func(t *testing.T) {
		manPath := filepath.Join(t.TempDir(), "manifest.json")
		home := t.TempDir()

		m := newPluginManifest(
			[]manifest.Plugin{pluginA, pluginB}, nil,
		)
		m.Profiles = map[string]manifest.Profile{
			"work": {Plugins: []manifest.ProfilePlugin{{ID: "plugin-a"}, {ID: "plugin-b"}}},
			"home": {Plugins: []manifest.ProfilePlugin{{ID: "plugin-a"}}},
		}
		seedManifest(t, manPath, m)

		res := execCmd(t, home, nil, "plugin", "remove", "--manifest", manPath, "--force", "--json", "plugin-a")
		if res.err != nil {
			t.Fatalf("unexpected error: %v\nstderr: %s", res.err, res.stderr)
		}

		got := parseJSON(t, res.stdout)
		if got["removed"] != "plugin-a" {
			t.Errorf("removed = %v, want plugin-a", got["removed"])
		}
		profiles, ok := got["cleanedProfiles"].([]any)
		if !ok || len(profiles) != 2 {
			t.Errorf("cleanedProfiles = %v, want [home work]", got["cleanedProfiles"])
		}

		loaded := loadManifestFile(t, manPath)
		workProfile := loaded.Profiles["work"]
		if len(workProfile.Plugins) != 1 || workProfile.Plugins[0].ID != "plugin-b" {
			t.Errorf("work profile plugins = %v, want [plugin-b]", workProfile.Plugins)
		}
		homeProfile := loaded.Profiles["home"]
		if len(homeProfile.Plugins) != 0 {
			t.Errorf("home profile plugins = %v, want []", homeProfile.Plugins)
		}
	})

	t.Run("cascade_declined", func(t *testing.T) {
		manPath := filepath.Join(t.TempDir(), "manifest.json")
		home := t.TempDir()

		m := newPluginManifest(
			[]manifest.Plugin{pluginA}, nil,
		)
		m.Profiles = map[string]manifest.Profile{
			"work": {Plugins: []manifest.ProfilePlugin{{ID: "plugin-a"}}},
		}
		seedManifest(t, manPath, m)

		res := execCmd(t, home, strings.NewReader("n\n"), "plugin", "remove", "--manifest", manPath, "plugin-a")
		if res.err != nil {
			t.Fatalf("unexpected error: %v\nstderr: %s", res.err, res.stderr)
		}

		// Plugin should still be in manifest.
		loaded := loadManifestFile(t, manPath)
		if len(loaded.Plugins) != 1 {
			t.Errorf("manifest has %d plugins, want 1 (removal was declined)", len(loaded.Plugins))
		}
	})
}

// --- TestPluginList ---

func TestPluginList(t *testing.T) {
	t.Run("manifest_table", func(t *testing.T) {
		manPath := filepath.Join(t.TempDir(), "manifest.json")
		home := t.TempDir()

		seedManifest(t, manPath, newPluginManifest(
			[]manifest.Plugin{pluginA, pluginB}, nil,
		))

		res := execCmd(t, home, nil, "plugin", "list", "--manifest", manPath)
		if res.err != nil {
			t.Fatalf("unexpected error: %v\nstderr: %s", res.err, res.stderr)
		}
		if !strings.Contains(res.stdout, "plugin-a") {
			t.Errorf("stdout missing plugin-a: %q", res.stdout)
		}
		if !strings.Contains(res.stdout, "plugin-b") {
			t.Errorf("stdout missing plugin-b: %q", res.stdout)
		}
	})

	t.Run("manifest_json", func(t *testing.T) {
		manPath := filepath.Join(t.TempDir(), "manifest.json")
		home := t.TempDir()

		seedManifest(t, manPath, newPluginManifest(
			[]manifest.Plugin{pluginA}, nil,
		))

		res := execCmd(t, home, nil, "plugin", "list", "--manifest", manPath, "--json")
		if res.err != nil {
			t.Fatalf("unexpected error: %v\nstderr: %s", res.err, res.stderr)
		}

		var plugins []manifest.Plugin
		if err := json.Unmarshal([]byte(res.stdout), &plugins); err != nil {
			t.Fatalf("failed to parse JSON: %v\nstdout: %s", err, res.stdout)
		}
		if len(plugins) != 1 || plugins[0].ID != "plugin-a" {
			t.Errorf("got %+v, want [{plugin-a user true}]", plugins)
		}
	})

	t.Run("manifest_empty", func(t *testing.T) {
		manPath := filepath.Join(t.TempDir(), "manifest.json")
		home := t.TempDir()

		seedManifest(t, manPath, newPluginManifest(nil, nil))

		res := execCmd(t, home, nil, "plugin", "list", "--manifest", manPath)
		if res.err != nil {
			t.Fatalf("unexpected error: %v\nstderr: %s", res.err, res.stderr)
		}
		if !strings.Contains(res.stderr, "No plugins in manifest") {
			t.Errorf("stderr missing empty message: %q", res.stderr)
		}
	})

	t.Run("installed_table", func(t *testing.T) {
		home := t.TempDir()

		runner := newFakeRunner()
		runner.on("plugin list --json", pluginsJSON(pluginA, pluginB), nil)

		res := execCmdWith(t, home, nil, withPluginClient(runner), "plugin", "list", "--installed")
		if res.err != nil {
			t.Fatalf("unexpected error: %v\nstderr: %s", res.err, res.stderr)
		}
		if !strings.Contains(res.stdout, "plugin-a") {
			t.Errorf("stdout missing plugin-a: %q", res.stdout)
		}
	})

	t.Run("installed_json", func(t *testing.T) {
		home := t.TempDir()

		runner := newFakeRunner()
		runner.on("plugin list --json", pluginsJSON(pluginA), nil)

		res := execCmdWith(t, home, nil, withPluginClient(runner), "plugin", "list", "--installed", "--json")
		if res.err != nil {
			t.Fatalf("unexpected error: %v\nstderr: %s", res.err, res.stderr)
		}

		var plugins []manifest.Plugin
		if err := json.Unmarshal([]byte(res.stdout), &plugins); err != nil {
			t.Fatalf("failed to parse JSON: %v\nstdout: %s", err, res.stdout)
		}
		if len(plugins) != 1 || plugins[0].ID != "plugin-a" {
			t.Errorf("got %+v, want [{plugin-a user true}]", plugins)
		}
	})
}
