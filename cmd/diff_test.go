package cmd

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alphaleonis/cctote/internal/manifest"
)

// newDiffManifest creates a manifest with MCP servers, plugins, and marketplaces.
func newDiffManifest(
	servers map[string]manifest.MCPServer,
	plugins []manifest.Plugin,
	marketplaces map[string]manifest.Marketplace,
	profiles map[string]manifest.Profile,
) *manifest.Manifest {
	m := &manifest.Manifest{
		Version:      manifest.CurrentVersion,
		MCPServers:   servers,
		Plugins:      plugins,
		Marketplaces: marketplaces,
		Profiles:     profiles,
	}
	if m.MCPServers == nil {
		m.MCPServers = map[string]manifest.MCPServer{}
	}
	if m.Plugins == nil {
		m.Plugins = []manifest.Plugin{}
	}
	if m.Marketplaces == nil {
		m.Marketplaces = map[string]manifest.Marketplace{}
	}
	return m
}

// parseDiffResult decodes the JSON diff output.
func parseDiffResult(t *testing.T, s string) diffResult {
	t.Helper()
	var r diffResult
	if err := json.Unmarshal([]byte(s), &r); err != nil {
		t.Fatalf("parseDiffResult: %v\ninput: %s", err, s)
	}
	return r
}

// assertExitCode checks that err is an *ExitError with the expected code.
func assertExitCode(t *testing.T, err error, wantCode int) {
	t.Helper()
	var exitErr *ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected *ExitError, got %T: %v", err, err)
	}
	if exitErr.Code != wantCode {
		t.Errorf("exit code = %d, want %d", exitErr.Code, wantCode)
	}
}

func TestDiff(t *testing.T) {
	t.Run("no_differences", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		srv := manifest.MCPServer{Command: "npx", Args: []string{"-y", "@upstash/context7-mcp"}}
		plug := manifest.Plugin{ID: "plugin-a", Scope: "user", Enabled: true}
		mp := manifest.Marketplace{Source: "github", Repo: "user/repo"}

		seedManifest(t, manPath, newDiffManifest(
			map[string]manifest.MCPServer{"context7": srv},
			[]manifest.Plugin{plug},
			map[string]manifest.Marketplace{"my-mp": mp},
			nil,
		))
		seedClaudeConfig(t, home, map[string]manifest.MCPServer{"context7": srv})

		runner := newFakeRunner()
		runner.on("plugin list --json", pluginsJSON(plug), nil)
		runner.on("plugin marketplace list --json", marketplacesJSON(map[string]manifest.Marketplace{"my-mp": mp}), nil)
		res := execCmdWith(t, home, nil, withPluginClient(runner), "diff", "--manifest", manPath, "--json")
		if res.err != nil {
			t.Fatalf("unexpected error: %v\nstderr: %s", res.err, res.stderr)
		}

		r := parseDiffResult(t, res.stdout)
		if len(r.OnlyInManifest) != 0 {
			t.Errorf("onlyInManifest = %d, want 0", len(r.OnlyInManifest))
		}
		if len(r.OnlyInClaudeCode) != 0 {
			t.Errorf("onlyInClaudeCode = %d, want 0", len(r.OnlyInClaudeCode))
		}
		if len(r.Different) != 0 {
			t.Errorf("different = %d, want 0", len(r.Different))
		}
	})

	t.Run("only_in_manifest", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		srv := manifest.MCPServer{Command: "npx", Args: []string{"-y", "my-mcp"}}
		plug := manifest.Plugin{ID: "plugin-a", Scope: "user", Enabled: true}

		seedManifest(t, manPath, newDiffManifest(
			map[string]manifest.MCPServer{"my-server": srv},
			[]manifest.Plugin{plug},
			nil, nil,
		))
		seedClaudeConfig(t, home, map[string]manifest.MCPServer{})

		runner := newFakeRunner()
		runner.on("plugin list --json", pluginsJSON(), nil)
		runner.on("plugin marketplace list --json", marketplacesJSON(nil), nil)
		res := execCmdWith(t, home, nil, withPluginClient(runner), "diff", "--manifest", manPath, "--json")
		assertExitCode(t, res.err, 2)

		r := parseDiffResult(t, res.stdout)
		if len(r.OnlyInManifest) != 2 {
			t.Fatalf("onlyInManifest = %d, want 2", len(r.OnlyInManifest))
		}
		// Entries are sorted: mcp first, then plugin.
		if r.OnlyInManifest[0].Kind != "mcp" || r.OnlyInManifest[0].Name != "my-server" {
			t.Errorf("entry[0] = %+v, want mcp/my-server", r.OnlyInManifest[0])
		}
		if r.OnlyInManifest[1].Kind != "plugin" || r.OnlyInManifest[1].Name != "plugin-a" {
			t.Errorf("entry[1] = %+v, want plugin/plugin-a", r.OnlyInManifest[1])
		}
		if len(r.OnlyInClaudeCode) != 0 {
			t.Errorf("onlyInClaudeCode = %d, want 0", len(r.OnlyInClaudeCode))
		}
	})

	t.Run("only_in_claude_code", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		srv := manifest.MCPServer{Command: "pg-mcp"}
		plug := manifest.Plugin{ID: "plugin-x", Scope: "project", Enabled: false}

		seedManifest(t, manPath, newDiffManifest(nil, nil, nil, nil))
		seedClaudeConfig(t, home, map[string]manifest.MCPServer{"postgres": srv})

		runner := newFakeRunner()
		runner.on("plugin list --json", pluginsJSON(plug), nil)
		runner.on("plugin marketplace list --json", marketplacesJSON(nil), nil)
		res := execCmdWith(t, home, nil, withPluginClient(runner), "diff", "--manifest", manPath, "--json")
		assertExitCode(t, res.err, 2)

		r := parseDiffResult(t, res.stdout)
		if len(r.OnlyInClaudeCode) != 2 {
			t.Fatalf("onlyInClaudeCode = %d, want 2", len(r.OnlyInClaudeCode))
		}
		if r.OnlyInClaudeCode[0].Kind != "mcp" || r.OnlyInClaudeCode[0].Name != "postgres" {
			t.Errorf("entry[0] = %+v, want mcp/postgres", r.OnlyInClaudeCode[0])
		}
		if r.OnlyInClaudeCode[1].Kind != "plugin" || r.OnlyInClaudeCode[1].Name != "plugin-x" {
			t.Errorf("entry[1] = %+v, want plugin/plugin-x", r.OnlyInClaudeCode[1])
		}
	})

	t.Run("different_config", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		manSrv := manifest.MCPServer{Command: "npx", Args: []string{"-y", "mcp@1.0"}}
		claudeSrv := manifest.MCPServer{Command: "npx", Args: []string{"-y", "mcp@2.0"}}
		manPlug := manifest.Plugin{ID: "plugin-a", Scope: "user", Enabled: true}
		claudePlug := manifest.Plugin{ID: "plugin-a", Scope: "user", Enabled: false}
		manMP := manifest.Marketplace{Source: "github", Repo: "user/repo-v1"}
		claudeMP := manifest.Marketplace{Source: "github", Repo: "user/repo-v2"}

		seedManifest(t, manPath, newDiffManifest(
			map[string]manifest.MCPServer{"my-mcp": manSrv},
			[]manifest.Plugin{manPlug},
			map[string]manifest.Marketplace{"my-mp": manMP},
			nil,
		))
		seedClaudeConfig(t, home, map[string]manifest.MCPServer{"my-mcp": claudeSrv})

		runner := newFakeRunner()
		runner.on("plugin list --json", pluginsJSON(claudePlug), nil)
		runner.on("plugin marketplace list --json", marketplacesJSON(map[string]manifest.Marketplace{"my-mp": claudeMP}), nil)
		res := execCmdWith(t, home, nil, withPluginClient(runner), "diff", "--manifest", manPath, "--json")
		assertExitCode(t, res.err, 2)

		r := parseDiffResult(t, res.stdout)
		if len(r.Different) != 3 {
			t.Fatalf("different = %d, want 3", len(r.Different))
		}
		// Entries appear in processing order: mcp, plugin, marketplace.
		if r.Different[0].Kind != "mcp" || r.Different[0].Name != "my-mcp" {
			t.Errorf("different[0] = %+v, want mcp/my-mcp", r.Different[0])
		}
		if r.Different[1].Kind != "plugin" || r.Different[1].Name != "plugin-a" {
			t.Errorf("different[1] = %+v, want plugin/plugin-a", r.Different[1])
		}
		if r.Different[2].Kind != "marketplace" || r.Different[2].Name != "my-mp" {
			t.Errorf("different[2] = %+v, want marketplace/my-mp", r.Different[2])
		}
	})

	t.Run("mixed", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		srvShared := manifest.MCPServer{Command: "shared-mcp"}
		srvManOnly := manifest.MCPServer{Command: "man-only"}
		srvClaudeOnly := manifest.MCPServer{Command: "claude-only"}
		srvManDiff := manifest.MCPServer{Command: "npx", Args: []string{"v1"}}
		srvClaudeDiff := manifest.MCPServer{Command: "npx", Args: []string{"v2"}}

		seedManifest(t, manPath, newDiffManifest(
			map[string]manifest.MCPServer{
				"shared":  srvShared,
				"man-srv": srvManOnly,
				"diff":    srvManDiff,
			},
			nil, nil, nil,
		))
		seedClaudeConfig(t, home, map[string]manifest.MCPServer{
			"shared":     srvShared,
			"claude-srv": srvClaudeOnly,
			"diff":       srvClaudeDiff,
		})

		runner := newFakeRunner()
		runner.on("plugin list --json", pluginsJSON(), nil)
		runner.on("plugin marketplace list --json", marketplacesJSON(nil), nil)
		res := execCmdWith(t, home, nil, withPluginClient(runner), "diff", "--manifest", manPath, "--json")
		assertExitCode(t, res.err, 2)

		r := parseDiffResult(t, res.stdout)
		if len(r.OnlyInManifest) != 1 || r.OnlyInManifest[0].Name != "man-srv" {
			t.Errorf("onlyInManifest = %+v, want [man-srv]", r.OnlyInManifest)
		}
		if len(r.OnlyInClaudeCode) != 1 || r.OnlyInClaudeCode[0].Name != "claude-srv" {
			t.Errorf("onlyInClaudeCode = %+v, want [claude-srv]", r.OnlyInClaudeCode)
		}
		if len(r.Different) != 1 || r.Different[0].Name != "diff" {
			t.Errorf("different = %+v, want [diff]", r.Different)
		}
	})

	t.Run("profile_scoping", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		srvA := manifest.MCPServer{Command: "srv-a"}
		srvB := manifest.MCPServer{Command: "srv-b"}
		plugA := manifest.Plugin{ID: "plug-a", Scope: "user", Enabled: true}
		plugB := manifest.Plugin{ID: "plug-b", Scope: "user", Enabled: true}

		seedManifest(t, manPath, newDiffManifest(
			map[string]manifest.MCPServer{"srv-a": srvA, "srv-b": srvB},
			[]manifest.Plugin{plugA, plugB},
			map[string]manifest.Marketplace{"my-mp": {Source: "github", Repo: "user/repo"}},
			map[string]manifest.Profile{
				"work": {MCPServers: []string{"srv-a"}, Plugins: []manifest.ProfilePlugin{{ID: "plug-a"}}},
			},
		))
		// Claude Code has srv-a (matching) but not srv-b. Also has extra srv-c.
		seedClaudeConfig(t, home, map[string]manifest.MCPServer{
			"srv-a": srvA,
			"srv-c": {Command: "extra"},
		})

		runner := newFakeRunner()
		runner.on("plugin list --json", pluginsJSON(
			manifest.Plugin{ID: "plug-a", Scope: "user", Enabled: true},
			manifest.Plugin{ID: "plug-extra", Scope: "user", Enabled: true},
		), nil)
		res := execCmdWith(t, home, nil, withPluginClient(runner), "diff", "--manifest", manPath, "--profile", "work", "--json")
		// Profile scoping: only compares srv-a and plug-a.
		// srv-a matches, plug-a matches → but Claude has extra srv-c and plug-extra.
		assertExitCode(t, res.err, 2)

		r := parseDiffResult(t, res.stdout)
		// srv-b and plug-b are NOT in the profile, so they don't appear.
		// Marketplaces are skipped for profiles.
		if len(r.OnlyInManifest) != 0 {
			t.Errorf("onlyInManifest = %+v, want empty", r.OnlyInManifest)
		}
		// srv-c and plug-extra are only in Claude Code.
		if len(r.OnlyInClaudeCode) != 2 {
			t.Fatalf("onlyInClaudeCode = %d, want 2", len(r.OnlyInClaudeCode))
		}
		if r.OnlyInClaudeCode[0].Kind != "mcp" || r.OnlyInClaudeCode[0].Name != "srv-c" {
			t.Errorf("entry[0] = %+v, want mcp/srv-c", r.OnlyInClaudeCode[0])
		}
		if r.OnlyInClaudeCode[1].Kind != "plugin" || r.OnlyInClaudeCode[1].Name != "plug-extra" {
			t.Errorf("entry[1] = %+v, want plugin/plug-extra", r.OnlyInClaudeCode[1])
		}
	})

	t.Run("profile_not_found", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		seedManifest(t, manPath, newDiffManifest(nil, nil, nil, nil))

		runner := newFakeRunner()
		res := execCmdWith(t, home, nil, withPluginClient(runner), "diff", "--manifest", manPath, "--profile", "nonexistent")
		if res.err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(res.err.Error(), `"nonexistent" not found`) {
			t.Errorf("error %q missing expected substring", res.err.Error())
		}
	})

	t.Run("dangling_mcp_reference", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		seedManifest(t, manPath, newDiffManifest(
			nil, nil, nil,
			map[string]manifest.Profile{
				"broken": {MCPServers: []string{"nonexistent-srv"}, Plugins: []manifest.ProfilePlugin{}},
			},
		))

		runner := newFakeRunner()
		res := execCmdWith(t, home, nil, withPluginClient(runner), "diff", "--manifest", manPath, "--profile", "broken")
		if res.err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(res.err.Error(), "nonexistent-srv") {
			t.Errorf("error %q missing expected server name", res.err.Error())
		}
	})

	t.Run("dangling_plugin_reference", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		seedManifest(t, manPath, newDiffManifest(
			nil, nil, nil,
			map[string]manifest.Profile{
				"broken": {MCPServers: []string{}, Plugins: []manifest.ProfilePlugin{{ID: "nonexistent-plugin"}}},
			},
		))

		runner := newFakeRunner()
		res := execCmdWith(t, home, nil, withPluginClient(runner), "diff", "--manifest", manPath, "--profile", "broken")
		if res.err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(res.err.Error(), "nonexistent-plugin") {
			t.Errorf("error %q missing expected plugin name", res.err.Error())
		}
	})

	t.Run("human_readable_output", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		seedManifest(t, manPath, newDiffManifest(
			map[string]manifest.MCPServer{"man-only": {Command: "man-cmd"}},
			[]manifest.Plugin{{ID: "man-plug", Scope: "user", Enabled: true}},
			nil, nil,
		))
		seedClaudeConfig(t, home, map[string]manifest.MCPServer{
			"claude-only": {Command: "claude-cmd"},
		})

		runner := newFakeRunner()
		runner.on("plugin list --json", pluginsJSON(
			manifest.Plugin{ID: "claude-plug", Scope: "user", Enabled: true},
		), nil)
		runner.on("plugin marketplace list --json", marketplacesJSON(nil), nil)
		res := execCmdWith(t, home, nil, withPluginClient(runner), "diff", "--manifest", manPath)
		assertExitCode(t, res.err, 2)

		// Verify section headers and entry names are present in stderr.
		if !strings.Contains(res.stderr, "MCP Servers") {
			t.Errorf("stderr missing 'MCP Servers' header:\n%s", res.stderr)
		}
		if !strings.Contains(res.stderr, "Plugins") {
			t.Errorf("stderr missing 'Plugins' header:\n%s", res.stderr)
		}
		if !strings.Contains(res.stderr, "man-only") {
			t.Errorf("stderr missing 'man-only' entry:\n%s", res.stderr)
		}
		if !strings.Contains(res.stderr, "claude-only") {
			t.Errorf("stderr missing 'claude-only' entry:\n%s", res.stderr)
		}
	})

	t.Run("human_readable_no_diff", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		seedManifest(t, manPath, newDiffManifest(nil, nil, nil, nil))
		seedClaudeConfig(t, home, map[string]manifest.MCPServer{})

		runner := newFakeRunner()
		runner.on("plugin list --json", pluginsJSON(), nil)
		runner.on("plugin marketplace list --json", marketplacesJSON(nil), nil)
		res := execCmdWith(t, home, nil, withPluginClient(runner), "diff", "--manifest", manPath)
		if res.err != nil {
			t.Fatalf("unexpected error: %v", res.err)
		}
		if !strings.Contains(res.stderr, "Nothing to do") {
			t.Errorf("stderr missing 'Nothing to do': %q", res.stderr)
		}
	})

	t.Run("profile_skips_marketplaces", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		srv := manifest.MCPServer{Command: "srv"}

		seedManifest(t, manPath, newDiffManifest(
			map[string]manifest.MCPServer{"srv": srv},
			nil,
			map[string]manifest.Marketplace{"mp": {Source: "github", Repo: "user/repo"}},
			map[string]manifest.Profile{
				"work": {MCPServers: []string{"srv"}, Plugins: []manifest.ProfilePlugin{}},
			},
		))
		seedClaudeConfig(t, home, map[string]manifest.MCPServer{"srv": srv})

		runner := newFakeRunner()
		runner.on("plugin list --json", pluginsJSON(), nil)
		// No marketplace list handler — if it were called, it would error.
		res := execCmdWith(t, home, nil, withPluginClient(runner), "diff", "--manifest", manPath, "--profile", "work", "--json")
		if res.err != nil {
			t.Fatalf("unexpected error: %v\nstderr: %s", res.err, res.stderr)
		}

		r := parseDiffResult(t, res.stdout)
		// No marketplace entries should appear in any category.
		for _, e := range r.OnlyInManifest {
			if e.Kind == "marketplace" {
				t.Error("profile diff should not include marketplace entries in onlyInManifest")
			}
		}
		for _, e := range r.OnlyInClaudeCode {
			if e.Kind == "marketplace" {
				t.Error("profile diff should not include marketplace entries in onlyInClaudeCode")
			}
		}
		for _, e := range r.Different {
			if e.Kind == "marketplace" {
				t.Error("profile diff should not include marketplace entries in different")
			}
		}
	})
}
