package cmd

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alphaleonis/cctote/internal/manifest"
)

// --- Helpers ---

// newFullManifest creates a manifest with MCP servers, plugins, and profiles.
func newFullManifest(
	servers map[string]manifest.MCPServer,
	plugins []manifest.Plugin,
	profiles map[string]manifest.Profile,
) *manifest.Manifest {
	m := &manifest.Manifest{
		Version:      manifest.CurrentVersion,
		MCPServers:   servers,
		Plugins:      plugins,
		Marketplaces: map[string]manifest.Marketplace{},
		Profiles:     profiles,
	}
	if m.MCPServers == nil {
		m.MCPServers = map[string]manifest.MCPServer{}
	}
	if m.Plugins == nil {
		m.Plugins = []manifest.Plugin{}
	}
	return m
}

// --- TestProfileList ---

func TestProfileList(t *testing.T) {
	t.Run("list_empty", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		seedManifest(t, manPath, newManifest(nil))

		res := execCmd(t, home, nil, "profile", "list", "--manifest", manPath)
		if res.err != nil {
			t.Fatalf("unexpected error: %v\nstderr: %s", res.err, res.stderr)
		}
		if !strings.Contains(res.stderr, "No profiles in manifest") {
			t.Errorf("stderr missing empty message: %q", res.stderr)
		}
	})

	t.Run("list_table", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		seedManifest(t, manPath, newFullManifest(
			map[string]manifest.MCPServer{"context7": srvContext7, "postgres": srvPostgres},
			[]manifest.Plugin{pluginA, pluginB},
			map[string]manifest.Profile{
				"work": {MCPServers: []string{"context7", "postgres"}, Plugins: []manifest.ProfilePlugin{{ID: "plugin-a"}}},
				"home": {MCPServers: []string{"context7"}, Plugins: []manifest.ProfilePlugin{{ID: "plugin-a"}, {ID: "plugin-b"}}},
			},
		))

		res := execCmd(t, home, nil, "profile", "list", "--manifest", manPath)
		if res.err != nil {
			t.Fatalf("unexpected error: %v\nstderr: %s", res.err, res.stderr)
		}
		if !strings.Contains(res.stdout, "work") {
			t.Errorf("stdout missing 'work': %q", res.stdout)
		}
		if !strings.Contains(res.stdout, "home") {
			t.Errorf("stdout missing 'home': %q", res.stdout)
		}
	})

	t.Run("list_json", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		seedManifest(t, manPath, newFullManifest(
			map[string]manifest.MCPServer{"context7": srvContext7},
			[]manifest.Plugin{pluginA},
			map[string]manifest.Profile{
				"work": {MCPServers: []string{"context7"}, Plugins: []manifest.ProfilePlugin{{ID: "plugin-a"}}},
				"home": {MCPServers: []string{}, Plugins: []manifest.ProfilePlugin{}},
			},
		))

		res := execCmd(t, home, nil, "profile", "list", "--manifest", manPath, "--json")
		if res.err != nil {
			t.Fatalf("unexpected error: %v\nstderr: %s", res.err, res.stderr)
		}

		var profiles map[string]manifest.Profile
		if err := json.Unmarshal([]byte(res.stdout), &profiles); err != nil {
			t.Fatalf("failed to parse JSON: %v\nstdout: %s", err, res.stdout)
		}
		if len(profiles) != 2 {
			t.Errorf("got %d profiles, want 2", len(profiles))
		}
		work := profiles["work"]
		if len(work.MCPServers) != 1 || work.MCPServers[0] != "context7" {
			t.Errorf("work.MCPServers = %v, want [context7]", work.MCPServers)
		}
	})
}

// --- TestProfileDelete ---

func TestProfileDelete(t *testing.T) {
	t.Run("delete_existing", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		seedManifest(t, manPath, newFullManifest(
			map[string]manifest.MCPServer{"context7": srvContext7},
			[]manifest.Plugin{pluginA},
			map[string]manifest.Profile{
				"work": {MCPServers: []string{"context7"}, Plugins: []manifest.ProfilePlugin{{ID: "plugin-a"}}},
			},
		))

		res := execCmd(t, home, nil, "profile", "delete", "--manifest", manPath, "work")
		if res.err != nil {
			t.Fatalf("unexpected error: %v\nstderr: %s", res.err, res.stderr)
		}

		m := loadManifestFile(t, manPath)
		if m.Profiles != nil {
			t.Errorf("profiles should be nil after deleting last profile, got %v", m.Profiles)
		}
		// Extensions should be preserved.
		if _, ok := m.MCPServers["context7"]; !ok {
			t.Error("MCP server 'context7' should still exist after profile delete")
		}
		if manifest.FindPlugin(m.Plugins, "plugin-a") < 0 {
			t.Error("plugin-a should still exist after profile delete")
		}
	})

	t.Run("delete_not_found", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		seedManifest(t, manPath, newManifest(nil))

		res := execCmd(t, home, nil, "profile", "delete", "--manifest", manPath, "nonexistent")
		if res.err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(res.err.Error(), `"nonexistent" not found`) {
			t.Errorf("error %q missing expected substring", res.err.Error())
		}
	})

	t.Run("delete_json", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		seedManifest(t, manPath, newFullManifest(
			nil, nil,
			map[string]manifest.Profile{
				"work": {MCPServers: []string{}, Plugins: []manifest.ProfilePlugin{}},
				"home": {MCPServers: []string{}, Plugins: []manifest.ProfilePlugin{}},
			},
		))

		res := execCmd(t, home, nil, "profile", "delete", "--manifest", manPath, "--json", "work")
		if res.err != nil {
			t.Fatalf("unexpected error: %v\nstderr: %s", res.err, res.stderr)
		}

		got := parseJSON(t, res.stdout)
		if got["deleted"] != "work" {
			t.Errorf("deleted = %v, want work", got["deleted"])
		}

		m := loadManifestFile(t, manPath)
		if _, ok := m.Profiles["work"]; ok {
			t.Error("work profile should be deleted")
		}
		if _, ok := m.Profiles["home"]; !ok {
			t.Error("home profile should be preserved")
		}
	})
}

// --- TestProfileRename ---

func TestProfileRename(t *testing.T) {
	t.Run("rename_success", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		seedManifest(t, manPath, newFullManifest(
			map[string]manifest.MCPServer{"context7": srvContext7},
			nil,
			map[string]manifest.Profile{
				"work": {MCPServers: []string{"context7"}, Plugins: []manifest.ProfilePlugin{{ID: "plugin-a"}}},
			},
		))

		res := execCmd(t, home, nil, "profile", "rename", "--manifest", manPath, "work", "office")
		if res.err != nil {
			t.Fatalf("unexpected error: %v\nstderr: %s", res.err, res.stderr)
		}

		m := loadManifestFile(t, manPath)
		if _, ok := m.Profiles["work"]; ok {
			t.Error("old profile 'work' should not exist")
		}
		office, ok := m.Profiles["office"]
		if !ok {
			t.Fatal("new profile 'office' should exist")
		}
		if len(office.MCPServers) != 1 || office.MCPServers[0] != "context7" {
			t.Errorf("office.MCPServers = %v, want [context7]", office.MCPServers)
		}
		if len(office.Plugins) != 1 || office.Plugins[0].ID != "plugin-a" {
			t.Errorf("office.Plugins = %v, want [plugin-a]", office.Plugins)
		}
	})

	t.Run("rename_old_not_found", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		seedManifest(t, manPath, newManifest(nil))

		res := execCmd(t, home, nil, "profile", "rename", "--manifest", manPath, "nope", "new")
		if res.err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(res.err.Error(), `"nope" not found`) {
			t.Errorf("error %q missing expected substring", res.err.Error())
		}
	})

	t.Run("rename_new_exists", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		seedManifest(t, manPath, newFullManifest(
			nil, nil,
			map[string]manifest.Profile{
				"work": {MCPServers: []string{}, Plugins: []manifest.ProfilePlugin{}},
				"home": {MCPServers: []string{}, Plugins: []manifest.ProfilePlugin{}},
			},
		))

		res := execCmd(t, home, nil, "profile", "rename", "--manifest", manPath, "work", "home")
		if res.err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(res.err.Error(), `"home" already exists`) {
			t.Errorf("error %q missing expected substring", res.err.Error())
		}
	})

	t.Run("rename_json", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		seedManifest(t, manPath, newFullManifest(
			nil, nil,
			map[string]manifest.Profile{
				"work": {MCPServers: []string{}, Plugins: []manifest.ProfilePlugin{}},
			},
		))

		res := execCmd(t, home, nil, "profile", "rename", "--manifest", manPath, "--json", "work", "office")
		if res.err != nil {
			t.Fatalf("unexpected error: %v\nstderr: %s", res.err, res.stderr)
		}

		got := parseJSON(t, res.stdout)
		if got["old"] != "work" {
			t.Errorf("old = %v, want work", got["old"])
		}
		if got["new"] != "office" {
			t.Errorf("new = %v, want office", got["new"])
		}
	})
}

// --- TestProfileCreate ---

func TestProfileCreate(t *testing.T) {
	t.Run("create_fresh", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		// Claude has MCP servers + plugins.
		seedClaudeConfig(t, home, map[string]manifest.MCPServer{
			"context7": srvContext7,
			"postgres": srvPostgres,
		})

		runner := newFakeRunner()
		runner.on("plugin list --json", pluginsJSON(pluginA, pluginB), nil)

		res := execCmdWith(t, home, nil, withPluginClient(runner), "profile", "create", "--manifest", manPath, "work")
		if res.err != nil {
			t.Fatalf("unexpected error: %v\nstderr: %s", res.err, res.stderr)
		}

		m := loadManifestFile(t, manPath)

		// Extensions should be auto-exported.
		if len(m.MCPServers) != 2 {
			t.Errorf("manifest has %d MCP servers, want 2", len(m.MCPServers))
		}
		if _, ok := m.MCPServers["context7"]; !ok {
			t.Error("manifest missing 'context7' server")
		}
		if _, ok := m.MCPServers["postgres"]; !ok {
			t.Error("manifest missing 'postgres' server")
		}
		if len(m.Plugins) != 2 {
			t.Errorf("manifest has %d plugins, want 2", len(m.Plugins))
		}

		// Profile should reference them.
		profile, ok := m.Profiles["work"]
		if !ok {
			t.Fatal("profile 'work' not found")
		}
		if len(profile.MCPServers) != 2 {
			t.Errorf("profile has %d MCP servers, want 2", len(profile.MCPServers))
		}
		if len(profile.Plugins) != 2 {
			t.Errorf("profile has %d plugins, want 2", len(profile.Plugins))
		}
	})

	t.Run("create_already_exists", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		seedManifest(t, manPath, newFullManifest(
			nil, nil,
			map[string]manifest.Profile{
				"work": {MCPServers: []string{}, Plugins: []manifest.ProfilePlugin{}},
			},
		))
		seedClaudeConfig(t, home, nil)

		runner := newFakeRunner()
		runner.on("plugin list --json", pluginsJSON(), nil)

		res := execCmdWith(t, home, nil, withPluginClient(runner), "profile", "create", "--manifest", manPath, "work")
		if res.err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(res.err.Error(), `"work" already exists`) {
			t.Errorf("error %q missing expected substring", res.err.Error())
		}
	})

	t.Run("create_empty_claude", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		seedClaudeConfig(t, home, nil)

		runner := newFakeRunner()
		runner.on("plugin list --json", pluginsJSON(), nil)

		res := execCmdWith(t, home, nil, withPluginClient(runner), "profile", "create", "--manifest", manPath, "empty")
		if res.err != nil {
			t.Fatalf("unexpected error: %v\nstderr: %s", res.err, res.stderr)
		}

		m := loadManifestFile(t, manPath)
		profile, ok := m.Profiles["empty"]
		if !ok {
			t.Fatal("profile 'empty' not found")
		}
		if len(profile.MCPServers) != 0 {
			t.Errorf("profile has %d MCP servers, want 0", len(profile.MCPServers))
		}
		if len(profile.Plugins) != 0 {
			t.Errorf("profile has %d plugins, want 0", len(profile.Plugins))
		}
	})

	t.Run("create_json", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		seedClaudeConfig(t, home, map[string]manifest.MCPServer{
			"context7": srvContext7,
		})

		runner := newFakeRunner()
		runner.on("plugin list --json", pluginsJSON(pluginA), nil)

		res := execCmdWith(t, home, nil, withPluginClient(runner), "profile", "create", "--manifest", manPath, "--json", "work")
		if res.err != nil {
			t.Fatalf("unexpected error: %v\nstderr: %s", res.err, res.stderr)
		}

		got := parseJSON(t, res.stdout)
		if got["profile"] != "work" {
			t.Errorf("profile = %v, want work", got["profile"])
		}
		contents, ok := got["profileContents"].(map[string]any)
		if !ok {
			t.Fatalf("profileContents is %T, want map", got["profileContents"])
		}
		mcpServers, ok := contents["mcpServers"].([]any)
		if !ok {
			t.Fatalf("mcpServers is %T, want []any", contents["mcpServers"])
		}
		if len(mcpServers) != 1 {
			t.Errorf("mcpServers has %d entries, want 1", len(mcpServers))
		}
		plugins, ok := contents["plugins"].([]any)
		if !ok {
			t.Fatalf("plugins is %T, want []any", contents["plugins"])
		}
		if len(plugins) != 1 {
			t.Errorf("plugins has %d entries, want 1", len(plugins))
		}
	})
}

// --- TestProfileUpdate ---

func TestProfileUpdate(t *testing.T) {
	t.Run("update_existing", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		// Existing manifest with old profile.
		seedManifest(t, manPath, newFullManifest(
			map[string]manifest.MCPServer{"context7": srvContext7},
			[]manifest.Plugin{pluginA},
			map[string]manifest.Profile{
				"work": {MCPServers: []string{"context7"}, Plugins: []manifest.ProfilePlugin{{ID: "plugin-a"}}},
			},
		))

		// Claude now has different config.
		seedClaudeConfig(t, home, map[string]manifest.MCPServer{
			"postgres": srvPostgres,
		})

		runner := newFakeRunner()
		runner.on("plugin list --json", pluginsJSON(pluginB), nil)

		res := execCmdWith(t, home, nil, withPluginClient(runner), "profile", "update", "--manifest", manPath, "work")
		if res.err != nil {
			t.Fatalf("unexpected error: %v\nstderr: %s", res.err, res.stderr)
		}

		m := loadManifestFile(t, manPath)
		profile := m.Profiles["work"]
		if len(profile.MCPServers) != 1 || profile.MCPServers[0] != "postgres" {
			t.Errorf("profile.MCPServers = %v, want [postgres]", profile.MCPServers)
		}
		if len(profile.Plugins) != 1 || profile.Plugins[0].ID != "plugin-b" {
			t.Errorf("profile.Plugins = %v, want [plugin-b]", profile.Plugins)
		}
		// Old extensions should still exist (auto-export merges, doesn't replace).
		if _, ok := m.MCPServers["context7"]; !ok {
			t.Error("old MCP server 'context7' should be preserved in manifest")
		}
	})

	t.Run("update_not_found", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		seedManifest(t, manPath, newManifest(nil))
		seedClaudeConfig(t, home, nil)

		runner := newFakeRunner()
		runner.on("plugin list --json", pluginsJSON(), nil)

		res := execCmdWith(t, home, nil, withPluginClient(runner), "profile", "update", "--manifest", manPath, "nonexistent")
		if res.err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(res.err.Error(), `"nonexistent" not found`) {
			t.Errorf("error %q missing expected substring", res.err.Error())
		}
	})
}

// --- TestProfileApply ---

func TestProfileApply(t *testing.T) {
	t.Run("apply_all_new", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		seedManifest(t, manPath, newFullManifest(
			map[string]manifest.MCPServer{"context7": srvContext7, "postgres": srvPostgres},
			[]manifest.Plugin{pluginA, pluginB},
			map[string]manifest.Profile{
				"work": {MCPServers: []string{"context7", "postgres"}, Plugins: []manifest.ProfilePlugin{{ID: "plugin-a"}, {ID: "plugin-b"}}},
			},
		))
		// Empty Claude config.
		seedClaudeConfig(t, home, nil)

		runner := newFakeRunner()
		runner.on("plugin list --json", pluginsJSON(), nil)
		runner.on("plugin install", nil, nil)
		runner.on("plugin disable", nil, nil)

		res := execCmdWith(t, home, nil, withPluginClient(runner), "profile", "apply", "--manifest", manPath, "work")
		if res.err != nil {
			t.Fatalf("unexpected error: %v\nstderr: %s", res.err, res.stderr)
		}

		// MCP servers should be written.
		servers := readClaudeServers(t, home)
		if len(servers) != 2 {
			t.Errorf("claude has %d MCP servers, want 2", len(servers))
		}
		// Plugins should be installed.
		if !runner.called("plugin", "install", "plugin-a") {
			t.Error("expected 'plugin install plugin-a' call")
		}
		if !runner.called("plugin", "install", "plugin-b") {
			t.Error("expected 'plugin install plugin-b' call")
		}
		// plugin-b has Enabled:false — should be disabled after install.
		if !runner.called("plugin", "disable", "plugin-b") {
			t.Error("expected 'plugin disable plugin-b' call for Enabled:false plugin")
		}
	})

	t.Run("apply_all_existing", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		seedManifest(t, manPath, newFullManifest(
			map[string]manifest.MCPServer{"context7": srvContext7},
			[]manifest.Plugin{pluginA},
			map[string]manifest.Profile{
				"work": {MCPServers: []string{"context7"}, Plugins: []manifest.ProfilePlugin{{ID: "plugin-a"}}},
			},
		))
		seedClaudeConfig(t, home, map[string]manifest.MCPServer{"context7": srvContext7})

		runner := newFakeRunner()
		runner.on("plugin list --json", pluginsJSON(pluginA), nil)

		res := execCmdWith(t, home, nil, withPluginClient(runner), "profile", "apply", "--manifest", manPath, "--json", "work")
		if res.err != nil {
			t.Fatalf("unexpected error: %v\nstderr: %s", res.err, res.stderr)
		}

		got := parseJSON(t, res.stdout)
		mcpResult, ok := got["mcp"].(map[string]any)
		if !ok {
			t.Fatalf("mcp is %T, want map[string]any; stdout: %s", got["mcp"], res.stdout)
		}
		if mcpResult["added"] != float64(0) {
			t.Errorf("mcp.added = %v, want 0", mcpResult["added"])
		}
		if mcpResult["skipped"] != float64(1) {
			t.Errorf("mcp.skipped = %v, want 1", mcpResult["skipped"])
		}
		pluginResult, ok := got["plugins"].(map[string]any)
		if !ok {
			t.Fatalf("plugins is %T, want map[string]any; stdout: %s", got["plugins"], res.stdout)
		}
		if pluginResult["installed"] != float64(0) {
			t.Errorf("plugins.installed = %v, want 0", pluginResult["installed"])
		}
		if pluginResult["skipped"] != float64(1) {
			t.Errorf("plugins.skipped = %v, want 1", pluginResult["skipped"])
		}
	})

	t.Run("apply_mcp_conflict_overwrite", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		seedManifest(t, manPath, newFullManifest(
			map[string]manifest.MCPServer{"context7": srvContext7V2},
			nil,
			map[string]manifest.Profile{
				"work": {MCPServers: []string{"context7"}, Plugins: []manifest.ProfilePlugin{}},
			},
		))
		seedClaudeConfig(t, home, map[string]manifest.MCPServer{"context7": srvContext7})

		runner := newFakeRunner()
		runner.on("plugin list --json", pluginsJSON(), nil)

		res := execCmdWith(t, home, nil, withPluginClient(runner), "profile", "apply", "--manifest", manPath, "--overwrite", "--json", "work")
		if res.err != nil {
			t.Fatalf("unexpected error: %v\nstderr: %s", res.err, res.stderr)
		}

		got := parseJSON(t, res.stdout)
		mcpResult, ok := got["mcp"].(map[string]any)
		if !ok {
			t.Fatalf("mcp is %T, want map[string]any; stdout: %s", got["mcp"], res.stdout)
		}
		if mcpResult["overwritten"] != float64(1) {
			t.Errorf("mcp.overwritten = %v, want 1", mcpResult["overwritten"])
		}

		servers := readClaudeServers(t, home)
		if !manifest.MCPServersEqual(servers["context7"], srvContext7V2) {
			t.Error("context7 should be overwritten with V2")
		}
	})

	t.Run("apply_mcp_conflict_no_overwrite", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		seedManifest(t, manPath, newFullManifest(
			map[string]manifest.MCPServer{"context7": srvContext7V2},
			nil,
			map[string]manifest.Profile{
				"work": {MCPServers: []string{"context7"}, Plugins: []manifest.ProfilePlugin{}},
			},
		))
		seedClaudeConfig(t, home, map[string]manifest.MCPServer{"context7": srvContext7})

		runner := newFakeRunner()
		runner.on("plugin list --json", pluginsJSON(), nil)

		res := execCmdWith(t, home, nil, withPluginClient(runner), "profile", "apply", "--manifest", manPath, "--no-overwrite", "--json", "work")
		if res.err != nil {
			t.Fatalf("unexpected error: %v\nstderr: %s", res.err, res.stderr)
		}

		got := parseJSON(t, res.stdout)
		mcpResult, ok := got["mcp"].(map[string]any)
		if !ok {
			t.Fatalf("got[\"mcp\"] type = %T, want map[string]any", got["mcp"])
		}
		if mcpResult["skipped"] != float64(1) {
			t.Errorf("mcp.skipped = %v, want 1", mcpResult["skipped"])
		}

		// Original should be preserved.
		servers := readClaudeServers(t, home)
		if !manifest.MCPServersEqual(servers["context7"], srvContext7) {
			t.Error("context7 should NOT be overwritten when --no-overwrite")
		}
	})

	t.Run("apply_strict_removes", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		seedManifest(t, manPath, newFullManifest(
			map[string]manifest.MCPServer{"context7": srvContext7},
			[]manifest.Plugin{pluginA},
			map[string]manifest.Profile{
				"work": {MCPServers: []string{"context7"}, Plugins: []manifest.ProfilePlugin{{ID: "plugin-a"}}},
			},
		))
		// Claude has extras.
		seedClaudeConfig(t, home, map[string]manifest.MCPServer{
			"context7": srvContext7,
			"postgres": srvPostgres,
		})

		runner := newFakeRunner()
		runner.on("plugin list --json", pluginsJSON(pluginA, pluginB), nil)
		runner.on("plugin uninstall", nil, nil)

		res := execCmdWith(t, home, nil, withPluginClient(runner), "profile", "apply", "--manifest", manPath, "--strict", "--force", "--json", "work")
		if res.err != nil {
			t.Fatalf("unexpected error: %v\nstderr: %s", res.err, res.stderr)
		}

		got := parseJSON(t, res.stdout)
		mcpResult, ok := got["mcp"].(map[string]any)
		if !ok {
			t.Fatalf("mcp is %T, want map[string]any; stdout: %s", got["mcp"], res.stdout)
		}
		if mcpResult["removed"] != float64(1) {
			t.Errorf("mcp.removed = %v, want 1", mcpResult["removed"])
		}
		pluginResult, ok := got["plugins"].(map[string]any)
		if !ok {
			t.Fatalf("plugins is %T, want map[string]any; stdout: %s", got["plugins"], res.stdout)
		}
		if pluginResult["uninstalled"] != float64(1) {
			t.Errorf("plugins.uninstalled = %v, want 1", pluginResult["uninstalled"])
		}

		// postgres should be removed.
		servers := readClaudeServers(t, home)
		if _, ok := servers["postgres"]; ok {
			t.Error("postgres should be removed with --strict")
		}
		// plugin-b should be uninstalled.
		if !runner.called("plugin", "uninstall", "plugin-b") {
			t.Error("expected 'plugin uninstall plugin-b' call")
		}
	})

	t.Run("strict_user_scope_ignores_project_plugins", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		// Profile contains only plugin-a (user scope).
		seedManifest(t, manPath, newFullManifest(
			map[string]manifest.MCPServer{"context7": srvContext7},
			[]manifest.Plugin{pluginA},
			map[string]manifest.Profile{
				"work": {MCPServers: []string{"context7"}, Plugins: []manifest.ProfilePlugin{{ID: "plugin-a"}}},
			},
		))
		seedClaudeConfig(t, home, map[string]manifest.MCPServer{"context7": srvContext7})

		// Claude reports plugin-a (user) + a project-scoped plugin.
		projectPlugin := manifest.Plugin{ID: "proj-only", Scope: "project", Enabled: true}
		runner := newFakeRunner()
		runner.on("plugin list --json", pluginsJSON(pluginA, projectPlugin), nil)

		// Default scope is user — --strict should NOT try to uninstall the project plugin.
		res := execCmdWith(t, home, nil, withPluginClient(runner), "profile", "apply", "--manifest", manPath, "--strict", "--force", "--json", "work")
		if res.err != nil {
			t.Fatalf("unexpected error: %v\nstderr: %s", res.err, res.stderr)
		}

		got := parseJSON(t, res.stdout)
		pluginResult, ok := got["plugins"].(map[string]any)
		if !ok {
			t.Fatalf("plugins is %T, want map[string]any; stdout: %s", got["plugins"], res.stdout)
		}
		if pluginResult["uninstalled"] != float64(0) {
			t.Errorf("plugins.uninstalled = %v, want 0 (project plugin should be ignored)", pluginResult["uninstalled"])
		}
		if runner.called("plugin", "uninstall") {
			t.Error("should not call 'plugin uninstall' for project-scoped plugin at user scope")
		}
	})

	t.Run("apply_dry_run", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		seedManifest(t, manPath, newFullManifest(
			map[string]manifest.MCPServer{"context7": srvContext7},
			[]manifest.Plugin{pluginA},
			map[string]manifest.Profile{
				"work": {MCPServers: []string{"context7"}, Plugins: []manifest.ProfilePlugin{{ID: "plugin-a"}}},
			},
		))
		seedClaudeConfig(t, home, nil)

		runner := newFakeRunner()
		runner.on("plugin list --json", pluginsJSON(), nil)

		res := execCmdWith(t, home, nil, withPluginClient(runner), "profile", "apply", "--manifest", manPath, "--dry-run", "work")
		if res.err != nil {
			t.Fatalf("unexpected error: %v\nstderr: %s", res.err, res.stderr)
		}

		// Should mention the server/plugin name in output.
		if !strings.Contains(res.stderr, "context7") {
			t.Errorf("stderr missing 'context7': %q", res.stderr)
		}
		if !strings.Contains(res.stderr, "plugin-a") {
			t.Errorf("stderr missing 'plugin-a': %q", res.stderr)
		}

		// No changes should have been made.
		servers := readClaudeServers(t, home)
		if len(servers) != 0 {
			t.Errorf("dry-run should not write MCP servers, got %d", len(servers))
		}
		if runner.called("plugin", "install") {
			t.Error("dry-run should not call 'plugin install'")
		}
	})

	t.Run("apply_dry_run_json", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		seedManifest(t, manPath, newFullManifest(
			map[string]manifest.MCPServer{"context7": srvContext7},
			[]manifest.Plugin{pluginA},
			map[string]manifest.Profile{
				"work": {MCPServers: []string{"context7"}, Plugins: []manifest.ProfilePlugin{{ID: "plugin-a"}}},
			},
		))
		seedClaudeConfig(t, home, nil)

		runner := newFakeRunner()
		runner.on("plugin list --json", pluginsJSON(), nil)

		res := execCmdWith(t, home, nil, withPluginClient(runner), "profile", "apply", "--manifest", manPath, "--dry-run", "--json", "work")
		if res.err != nil {
			t.Fatalf("unexpected error: %v\nstderr: %s", res.err, res.stderr)
		}

		got := parseJSON(t, res.stdout)
		mcpPlan, ok := got["mcp"].(map[string]any)
		if !ok {
			t.Fatalf("mcp is %T, want map", got["mcp"])
		}
		mcpAddList, ok := mcpPlan["add"].([]any)
		if !ok {
			t.Fatalf("mcp.add is %T, want []any", mcpPlan["add"])
		}
		if len(mcpAddList) != 1 {
			t.Errorf("mcp.add has %d entries, want 1", len(mcpAddList))
		}

		pluginPlan, ok := got["plugins"].(map[string]any)
		if !ok {
			t.Fatalf("plugins is %T, want map", got["plugins"])
		}
		pluginInstallList, ok := pluginPlan["install"].([]any)
		if !ok {
			t.Fatalf("plugins.install is %T, want []any", pluginPlan["install"])
		}
		if len(pluginInstallList) != 1 {
			t.Errorf("plugins.install has %d entries, want 1", len(pluginInstallList))
		}
	})

	t.Run("apply_not_found", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		seedManifest(t, manPath, newManifest(nil))

		runner := newFakeRunner()
		runner.on("plugin list --json", pluginsJSON(), nil)

		res := execCmdWith(t, home, nil, withPluginClient(runner), "profile", "apply", "--manifest", manPath, "nonexistent")
		if res.err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(res.err.Error(), `"nonexistent" not found`) {
			t.Errorf("error %q missing expected substring", res.err.Error())
		}
	})

	t.Run("apply_dangling_reference", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		// Profile references an MCP server not in the manifest.
		seedManifest(t, manPath, newFullManifest(
			map[string]manifest.MCPServer{}, // no servers
			nil,
			map[string]manifest.Profile{
				"work": {MCPServers: []string{"missing-server"}, Plugins: []manifest.ProfilePlugin{}},
			},
		))
		seedClaudeConfig(t, home, nil)

		runner := newFakeRunner()
		runner.on("plugin list --json", pluginsJSON(), nil)

		res := execCmdWith(t, home, nil, withPluginClient(runner), "profile", "apply", "--manifest", manPath, "work")
		if res.err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(res.err.Error(), "missing-server") {
			t.Errorf("error %q missing server name", res.err.Error())
		}
		if !strings.Contains(res.err.Error(), "not in the manifest") {
			t.Errorf("error %q missing 'not in the manifest'", res.err.Error())
		}
	})

	t.Run("apply_dangling_plugin_reference", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		// Profile references a plugin not in the manifest.
		seedManifest(t, manPath, newFullManifest(
			nil, nil,
			map[string]manifest.Profile{
				"work": {MCPServers: []string{}, Plugins: []manifest.ProfilePlugin{{ID: "missing-plugin"}}},
			},
		))
		seedClaudeConfig(t, home, nil)

		runner := newFakeRunner()
		runner.on("plugin list --json", pluginsJSON(), nil)

		res := execCmdWith(t, home, nil, withPluginClient(runner), "profile", "apply", "--manifest", manPath, "work")
		if res.err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(res.err.Error(), "missing-plugin") {
			t.Errorf("error %q missing plugin name", res.err.Error())
		}
	})

	t.Run("apply_force_implies_overwrite", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		seedManifest(t, manPath, newFullManifest(
			map[string]manifest.MCPServer{"context7": srvContext7V2},
			nil,
			map[string]manifest.Profile{
				"work": {MCPServers: []string{"context7"}, Plugins: []manifest.ProfilePlugin{}},
			},
		))
		seedClaudeConfig(t, home, map[string]manifest.MCPServer{"context7": srvContext7})

		runner := newFakeRunner()
		runner.on("plugin list --json", pluginsJSON(), nil)

		res := execCmdWith(t, home, nil, withPluginClient(runner), "profile", "apply", "--manifest", manPath, "--force", "--json", "work")
		if res.err != nil {
			t.Fatalf("unexpected error: %v\nstderr: %s", res.err, res.stderr)
		}

		got := parseJSON(t, res.stdout)
		mcpResult, ok := got["mcp"].(map[string]any)
		if !ok {
			t.Fatalf("mcp is %T, want map[string]any; stdout: %s", got["mcp"], res.stdout)
		}
		if mcpResult["overwritten"] != float64(1) {
			t.Errorf("mcp.overwritten = %v, want 1", mcpResult["overwritten"])
		}

		servers := readClaudeServers(t, home)
		if !manifest.MCPServersEqual(servers["context7"], srvContext7V2) {
			t.Error("context7 should be overwritten with --force")
		}
	})

	t.Run("apply_overwrite_no_overwrite_conflict", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		seedManifest(t, manPath, newFullManifest(
			map[string]manifest.MCPServer{"context7": srvContext7},
			nil,
			map[string]manifest.Profile{
				"work": {MCPServers: []string{"context7"}, Plugins: []manifest.ProfilePlugin{}},
			},
		))

		runner := newFakeRunner()
		runner.on("plugin list --json", pluginsJSON(), nil)

		res := execCmdWith(t, home, nil, withPluginClient(runner), "profile", "apply", "--manifest", manPath, "--overwrite", "--no-overwrite", "work")
		if res.err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(res.err.Error(), "mutually exclusive") {
			t.Errorf("error %q missing 'mutually exclusive'", res.err.Error())
		}
	})

	t.Run("apply_scope_project", func(t *testing.T) {
		home := t.TempDir()
		projDir := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		// Empty project — no .mcp.json yet.

		seedManifest(t, manPath, newFullManifest(
			map[string]manifest.MCPServer{"context7": srvContext7, "postgres": srvPostgres},
			[]manifest.Plugin{pluginA, pluginB},
			map[string]manifest.Profile{
				"work": {
					MCPServers: []string{"context7", "postgres"},
					Plugins:    []manifest.ProfilePlugin{{ID: "plugin-a"}, {ID: "plugin-b"}},
				},
			},
		))

		runner := newFakeRunner()
		runner.on("plugin list --json", pluginsJSON(), nil)
		runner.on("plugin install", nil, nil)
		runner.on("plugin disable", nil, nil)

		res := execCmdWith(t, home, nil, appOpts(withProjectDir(projDir), withPluginClient(runner)), "profile", "apply", "--manifest", manPath, "--scope", "project", "--json", "work")
		if res.err != nil {
			t.Fatalf("unexpected error: %v\nstderr: %s", res.err, res.stderr)
		}

		// MCP servers should be written to .mcp.json.
		servers := readProjectServers(t, projDir)
		if len(servers) != 2 {
			t.Errorf("project has %d MCP servers, want 2", len(servers))
		}
		if _, ok := servers["context7"]; !ok {
			t.Error("project missing 'context7' server")
		}
		if _, ok := servers["postgres"]; !ok {
			t.Error("project missing 'postgres' server")
		}

		// Plugins should be installed with project scope (-s project).
		if !runner.called("plugin", "install") {
			t.Error("expected plugin install calls")
		}
		// plugin-b has Enabled:false — should be disabled after install.
		// With --scope project, the CLI sends "plugin disable -s project plugin-b".
		if !runner.called("plugin", "disable") {
			t.Error("expected 'plugin disable' call for Enabled:false plugin")
		}

		// JSON output should report additions.
		got := parseJSON(t, res.stdout)
		mcpResult, ok := got["mcp"].(map[string]any)
		if !ok {
			t.Fatalf("mcp is %T, want map[string]any; stdout: %s", got["mcp"], res.stdout)
		}
		if mcpResult["added"] != float64(2) {
			t.Errorf("mcp.added = %v, want 2", mcpResult["added"])
		}
	})

	t.Run("apply_scope_project_strict", func(t *testing.T) {
		home := t.TempDir()
		projDir := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		// Project has extras that should be removed with --strict.
		seedProjectMcpConfig(t, projDir, map[string]manifest.MCPServer{
			"context7": srvContext7,
			"postgres": srvPostgres,
		})

		// Profile only has context7 — postgres should be removed.
		seedManifest(t, manPath, newFullManifest(
			map[string]manifest.MCPServer{"context7": srvContext7},
			[]manifest.Plugin{pluginA},
			map[string]manifest.Profile{
				"work": {
					MCPServers: []string{"context7"},
					Plugins:    []manifest.ProfilePlugin{{ID: "plugin-a"}},
				},
			},
		))

		// Claude has plugin-a (matching) and extra plugin-b (project scope).
		// plugin-a matches the manifest exactly (skip). plugin-b is extra (uninstall).
		// With --scope project, currentPlugins is filtered to scope=="project",
		// so we use project scope for both installed plugins.
		projPluginA := manifest.Plugin{ID: "plugin-a", Scope: "project", Enabled: true}
		projPluginB := manifest.Plugin{ID: "plugin-b", Scope: "project", Enabled: false}
		runner := newFakeRunner()
		runner.on("plugin list --json", pluginsJSON(projPluginA, projPluginB), nil)
		runner.on("plugin install", nil, nil)
		runner.on("plugin enable", nil, nil)
		runner.on("plugin uninstall", nil, nil)

		res := execCmdWith(t, home, nil, appOpts(withProjectDir(projDir), withPluginClient(runner)), "profile", "apply", "--manifest", manPath, "--scope", "project", "--strict", "--force", "--json", "work")
		if res.err != nil {
			t.Fatalf("unexpected error: %v\nstderr: %s", res.err, res.stderr)
		}

		// postgres should be removed from project.
		servers := readProjectServers(t, projDir)
		if _, ok := servers["postgres"]; ok {
			t.Error("postgres should be removed with --strict")
		}
		if _, ok := servers["context7"]; !ok {
			t.Error("context7 should still exist")
		}

		// plugin-b should be uninstalled.
		got := parseJSON(t, res.stdout)
		mcpResult, ok := got["mcp"].(map[string]any)
		if !ok {
			t.Fatalf("mcp is %T, want map[string]any; stdout: %s", got["mcp"], res.stdout)
		}
		if mcpResult["removed"] != float64(1) {
			t.Errorf("mcp.removed = %v, want 1", mcpResult["removed"])
		}
		pluginResult, ok := got["plugins"].(map[string]any)
		if !ok {
			t.Fatalf("plugins is %T, want map[string]any; stdout: %s", got["plugins"], res.stdout)
		}
		if pluginResult["uninstalled"] != float64(1) {
			t.Errorf("plugins.uninstalled = %v, want 1", pluginResult["uninstalled"])
		}
		if !runner.called("plugin", "uninstall") {
			t.Error("expected 'plugin uninstall' call for plugin-b")
		}
	})
}
