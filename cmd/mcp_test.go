package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alphaleonis/cctote/internal/manifest"
	"github.com/alphaleonis/cctote/internal/mcp"
)

// --- Fixtures ---

var (
	srvContext7 = manifest.MCPServer{
		Command: "npx",
		Args:    []string{"-y", "@upstash/context7-mcp"},
	}
	srvContext7V2 = manifest.MCPServer{
		Command: "npx",
		Args:    []string{"-y", "@upstash/context7-mcp@2.0"},
	}
	srvPostgres = manifest.MCPServer{
		Command: "pg-mcp",
		Env:     map[string]string{"DSN": "postgres://localhost/dev"},
	}
	srvRemoteSSE = manifest.MCPServer{
		Type: "sse",
		URL:  "https://mcp.example.com/sse",
	}
)

// --- Helpers ---

type cmdResult struct {
	stdout string
	stderr string
	err    error
}

// execCmd runs a cctote command through the full cobra pipeline and captures
// all output. homeDir controls where mcp.DefaultPath() resolves (~/.claude.json).
// Pass a non-nil stdin to feed interactive prompts.
func execCmd(t *testing.T, homeDir string, stdin io.Reader, args ...string) cmdResult {
	t.Helper()
	return execCmdWith(t, homeDir, stdin, nil, args...)
}

// execCmdWith runs a cctote command with an optional App configurator applied
// before execution. Use withPluginClient, withProjectDir, etc. to build the
// configurator. Cobra does not reset local flag values between Execute() calls
// on the same command tree, so a fresh App is created for each invocation.
func execCmdWith(t *testing.T, homeDir string, stdin io.Reader, configure func(*App), args ...string) cmdResult {
	t.Helper()

	app := NewApp("")
	if configure != nil {
		configure(app)
	}

	// Redirect HOME/USERPROFILE so mcp.DefaultPath() resolves to our temp dir.
	t.Setenv("HOME", homeDir)
	t.Setenv("USERPROFILE", homeDir)

	// Capture stdout and stderr.
	var stdout, stderr bytes.Buffer
	app.root.SetOut(&stdout)
	app.root.SetErr(&stderr)

	if stdin != nil {
		app.root.SetIn(stdin)
	} else {
		app.root.SetIn(strings.NewReader(""))
	}

	// Prevent cobra from printing usage/error text to stderr.
	app.root.SilenceUsage = true
	app.root.SilenceErrors = true

	// Always inject --config pointing to a nonexistent path so we never
	// load the real user config. config.Load returns zero Config for
	// missing files.
	fullArgs := append([]string{
		"--config", filepath.Join(homeDir, "nonexistent-config", "cctote.toml"),
	}, args...)
	app.root.SetArgs(fullArgs)

	err := app.root.Execute()

	return cmdResult{
		stdout: stdout.String(),
		stderr: stderr.String(),
		err:    err,
	}
}

// withProjectDir returns an App configurator that overrides resolveProjectMcpPath
// to use a temp directory.
func withProjectDir(dir string) func(*App) {
	mcpPath := filepath.Join(dir, ".mcp.json")
	return func(a *App) { a.resolveProjectMcpPath = func() string { return mcpPath } }
}

// withChezmoiManaged returns an App configurator that stubs the chezmoiManaged seam.
func withChezmoiManaged(managed bool) func(*App) {
	return func(a *App) { a.chezmoiManaged = func(context.Context, string) bool { return managed } }
}

// appOpts composes multiple App configurators into one.
func appOpts(opts ...func(*App)) func(*App) {
	return func(a *App) {
		for _, o := range opts {
			o(a)
		}
	}
}

// seedProjectMcpConfig writes MCP servers into dir/.mcp.json.
func seedProjectMcpConfig(t *testing.T, dir string, servers map[string]manifest.MCPServer) {
	t.Helper()
	mcpPath := filepath.Join(dir, ".mcp.json")
	if err := mcp.UpdateProjectMcpServers(mcpPath, func(existing map[string]manifest.MCPServer) error {
		for k, v := range servers {
			existing[k] = v
		}
		return nil
	}); err != nil {
		t.Fatalf("seedProjectMcpConfig: %v", err)
	}
}

// readProjectServers reads MCP servers from dir/.mcp.json.
func readProjectServers(t *testing.T, dir string) map[string]manifest.MCPServer {
	t.Helper()
	servers, err := mcp.ReadProjectMcpServers(filepath.Join(dir, ".mcp.json"))
	if err != nil {
		t.Fatalf("readProjectServers: %v", err)
	}
	return servers
}

// seedManifest writes a manifest to path, creating parent directories.
func seedManifest(t *testing.T, path string, m *manifest.Manifest) {
	t.Helper()
	if err := manifest.Save(path, m); err != nil {
		t.Fatalf("seedManifest: %v", err)
	}
}

// seedClaudeConfig writes MCP servers into homeDir/.claude.json.
func seedClaudeConfig(t *testing.T, homeDir string, servers map[string]manifest.MCPServer) {
	t.Helper()
	p := filepath.Join(homeDir, ".claude.json")
	if err := mcp.WriteMcpServers(p, servers); err != nil {
		t.Fatalf("seedClaudeConfig: %v", err)
	}
}

// loadManifestFile reads and parses a manifest, failing the test on error.
func loadManifestFile(t *testing.T, path string) *manifest.Manifest {
	t.Helper()
	m, err := manifest.Load(path)
	if err != nil {
		t.Fatalf("loadManifestFile: %v", err)
	}
	return m
}

// readClaudeServers reads MCP servers from homeDir/.claude.json.
func readClaudeServers(t *testing.T, homeDir string) map[string]manifest.MCPServer {
	t.Helper()
	servers, err := mcp.ReadMcpServers(filepath.Join(homeDir, ".claude.json"))
	if err != nil {
		t.Fatalf("readClaudeServers: %v", err)
	}
	return servers
}

// parseJSON unmarshals a JSON string into a generic map.
func parseJSON(t *testing.T, s string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		t.Fatalf("parseJSON(%q): %v", s, err)
	}
	return m
}

// newManifest creates a manifest with CurrentVersion and initialized maps/slices.
func newManifest(servers map[string]manifest.MCPServer) *manifest.Manifest {
	m := &manifest.Manifest{
		Version:      manifest.CurrentVersion,
		MCPServers:   servers,
		Plugins:      []manifest.Plugin{},
		Marketplaces: map[string]manifest.Marketplace{},
	}
	if m.MCPServers == nil {
		m.MCPServers = map[string]manifest.MCPServer{}
	}
	return m
}

// newManifestWithProfiles creates a manifest with servers and profiles.
func newManifestWithProfiles(servers map[string]manifest.MCPServer, profiles map[string]manifest.Profile) *manifest.Manifest {
	m := newManifest(servers)
	m.Profiles = profiles
	return m
}

// --- TestMcpExport ---

func TestMcpExport(t *testing.T) {
	t.Run("export_all_fresh", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		seedClaudeConfig(t, home, map[string]manifest.MCPServer{
			"context7": srvContext7,
			"postgres": srvPostgres,
		})

		res := execCmd(t, home, nil, "mcp", "export", "--manifest", manPath)
		if res.err != nil {
			t.Fatalf("unexpected error: %v", res.err)
		}

		if !strings.Contains(res.stderr, "Created manifest") {
			t.Errorf("stderr missing 'Created manifest': %q", res.stderr)
		}
		if !strings.Contains(res.stderr, "Exported 2") {
			t.Errorf("stderr missing 'Exported 2': %q", res.stderr)
		}

		m := loadManifestFile(t, manPath)
		if len(m.MCPServers) != 2 {
			t.Errorf("manifest has %d servers, want 2", len(m.MCPServers))
		}
		if !manifest.MCPServersEqual(m.MCPServers["context7"], srvContext7) {
			t.Error("context7 content mismatch")
		}
		if !manifest.MCPServersEqual(m.MCPServers["postgres"], srvPostgres) {
			t.Error("postgres content mismatch")
		}
	})

	t.Run("export_all_into_existing", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		seedClaudeConfig(t, home, map[string]manifest.MCPServer{
			"context7": srvContext7,
			"postgres": srvPostgres,
		})
		seedManifest(t, manPath, newManifest(map[string]manifest.MCPServer{
			"context7": srvContext7,
		}))

		res := execCmd(t, home, nil, "mcp", "export", "--manifest", manPath)
		if res.err != nil {
			t.Fatalf("unexpected error: %v", res.err)
		}

		m := loadManifestFile(t, manPath)
		if len(m.MCPServers) != 2 {
			t.Errorf("manifest has %d servers, want 2", len(m.MCPServers))
		}
	})

	t.Run("export_preserves_manifest_only", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		seedClaudeConfig(t, home, map[string]manifest.MCPServer{
			"context7": srvContext7,
		})
		seedManifest(t, manPath, newManifest(map[string]manifest.MCPServer{
			"postgres": srvPostgres,
		}))

		res := execCmd(t, home, nil, "mcp", "export", "--manifest", manPath)
		if res.err != nil {
			t.Fatalf("unexpected error: %v", res.err)
		}

		m := loadManifestFile(t, manPath)
		if _, ok := m.MCPServers["postgres"]; !ok {
			t.Error("manifest-only server 'postgres' should be preserved after export")
		}
		if _, ok := m.MCPServers["context7"]; !ok {
			t.Error("exported server 'context7' should be present")
		}
	})

	t.Run("export_selective", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		seedClaudeConfig(t, home, map[string]manifest.MCPServer{
			"context7": srvContext7,
			"postgres": srvPostgres,
			"sse":      srvRemoteSSE,
		})

		res := execCmd(t, home, nil, "mcp", "export", "--manifest", manPath, "context7", "sse")
		if res.err != nil {
			t.Fatalf("unexpected error: %v", res.err)
		}

		m := loadManifestFile(t, manPath)
		if len(m.MCPServers) != 2 {
			t.Errorf("manifest has %d servers, want 2", len(m.MCPServers))
		}
		if _, ok := m.MCPServers["postgres"]; ok {
			t.Error("manifest should not contain 'postgres' (not selected)")
		}
	})

	t.Run("export_selective_not_found", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		seedClaudeConfig(t, home, map[string]manifest.MCPServer{
			"context7": srvContext7,
		})

		res := execCmd(t, home, nil, "mcp", "export", "--manifest", manPath, "nonexistent")
		if res.err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(res.err.Error(), `"nonexistent" not found`) {
			t.Errorf("error %q missing expected substring", res.err.Error())
		}
	})

	t.Run("export_empty_claude", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		// No claude config seeded — mcp.ReadMcpServers returns empty map for missing file.

		res := execCmd(t, home, nil, "mcp", "export", "--manifest", manPath)
		if res.err != nil {
			t.Fatalf("unexpected error: %v", res.err)
		}
		if !strings.Contains(res.stderr, "No MCP servers found") {
			t.Errorf("stderr missing 'No MCP servers found': %q", res.stderr)
		}
	})

	t.Run("export_empty_json", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		res := execCmd(t, home, nil, "mcp", "export", "--manifest", manPath, "--json")
		if res.err != nil {
			t.Fatalf("unexpected error: %v", res.err)
		}

		got := parseJSON(t, res.stdout)
		if got["added"] != float64(0) {
			t.Errorf("added = %v, want 0", got["added"])
		}
		if got["updated"] != float64(0) {
			t.Errorf("updated = %v, want 0", got["updated"])
		}
		servers, ok := got["mcpServers"].([]any)
		if !ok {
			t.Fatalf("mcpServers is %T, want []any", got["mcpServers"])
		}
		if len(servers) != 0 {
			t.Errorf("mcpServers has %d entries, want 0", len(servers))
		}
	})

	t.Run("export_all_json", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		seedClaudeConfig(t, home, map[string]manifest.MCPServer{
			"context7": srvContext7,
			"postgres": srvPostgres,
		})

		res := execCmd(t, home, nil, "mcp", "export", "--manifest", manPath, "--json")
		if res.err != nil {
			t.Fatalf("unexpected error: %v", res.err)
		}

		got := parseJSON(t, res.stdout)
		if got["added"] != float64(2) {
			t.Errorf("added = %v, want 2", got["added"])
		}
		if got["updated"] != float64(0) {
			t.Errorf("updated = %v, want 0", got["updated"])
		}
		servers, ok := got["mcpServers"].([]any)
		if !ok {
			t.Fatalf("mcpServers is %T, want []any", got["mcpServers"])
		}
		if len(servers) != 2 {
			t.Errorf("mcpServers has %d entries, want 2", len(servers))
		}
		// Verify sorted order.
		if servers[0] != "context7" || servers[1] != "postgres" {
			t.Errorf("mcpServers = %v, want [context7 postgres]", servers)
		}
	})

	t.Run("export_mixed_json", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		seedClaudeConfig(t, home, map[string]manifest.MCPServer{
			"context7": srvContext7,
			"postgres": srvPostgres,
		})
		seedManifest(t, manPath, newManifest(map[string]manifest.MCPServer{
			"context7": srvContext7,
		}))

		res := execCmd(t, home, nil, "mcp", "export", "--manifest", manPath, "--json")
		if res.err != nil {
			t.Fatalf("unexpected error: %v", res.err)
		}

		got := parseJSON(t, res.stdout)
		if got["added"] != float64(1) {
			t.Errorf("added = %v, want 1", got["added"])
		}
		if got["updated"] != float64(1) {
			t.Errorf("updated = %v, want 1", got["updated"])
		}
	})

	t.Run("export_creates_dir", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "deep", "nested", "dir", "manifest.json")

		seedClaudeConfig(t, home, map[string]manifest.MCPServer{
			"context7": srvContext7,
		})

		res := execCmd(t, home, nil, "mcp", "export", "--manifest", manPath)
		if res.err != nil {
			t.Fatalf("unexpected error: %v", res.err)
		}

		m := loadManifestFile(t, manPath)
		if _, ok := m.MCPServers["context7"]; !ok {
			t.Error("manifest missing 'context7'")
		}
	})
}

// --- TestMcpImportFlagValidation ---

func TestMcpImportFlagValidation(t *testing.T) {
	t.Run("strict_with_names", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")
		seedManifest(t, manPath, newManifest(map[string]manifest.MCPServer{
			"srv": srvContext7,
		}))

		res := execCmd(t, home, nil, "mcp", "import", "--manifest", manPath, "--strict", "srv")
		if res.err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(res.err.Error(), "--strict cannot be used with named") {
			t.Errorf("error %q missing expected substring", res.err.Error())
		}
	})

	t.Run("overwrite_and_no_overwrite", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")
		seedManifest(t, manPath, newManifest(nil))

		res := execCmd(t, home, nil, "mcp", "import", "--manifest", manPath, "--overwrite", "--no-overwrite")
		if res.err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(res.err.Error(), "mutually exclusive") {
			t.Errorf("error %q missing expected substring", res.err.Error())
		}
	})

	t.Run("force_and_no_overwrite", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")
		seedManifest(t, manPath, newManifest(nil))

		res := execCmd(t, home, nil, "--force", "mcp", "import", "--manifest", manPath, "--no-overwrite")
		if res.err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(res.err.Error(), "mutually exclusive") {
			t.Errorf("error %q missing expected substring", res.err.Error())
		}
	})
}

// --- TestMcpImportBasic ---

func TestMcpImportBasic(t *testing.T) {
	t.Run("add_new", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		seedManifest(t, manPath, newManifest(map[string]manifest.MCPServer{
			"context7": srvContext7,
			"postgres": srvPostgres,
		}))
		// Empty claude config (no file).

		res := execCmd(t, home, nil, "mcp", "import", "--manifest", manPath)
		if res.err != nil {
			t.Fatalf("unexpected error: %v", res.err)
		}

		servers := readClaudeServers(t, home)
		if len(servers) != 2 {
			t.Errorf("claude config has %d servers, want 2", len(servers))
		}
		if !manifest.MCPServersEqual(servers["context7"], srvContext7) {
			t.Error("context7 content mismatch after import")
		}
		if !manifest.MCPServersEqual(servers["postgres"], srvPostgres) {
			t.Error("postgres content mismatch after import")
		}
	})

	t.Run("skip_identical", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		seedManifest(t, manPath, newManifest(map[string]manifest.MCPServer{
			"context7": srvContext7,
		}))
		seedClaudeConfig(t, home, map[string]manifest.MCPServer{
			"context7": srvContext7,
		})

		res := execCmd(t, home, nil, "mcp", "import", "--manifest", manPath, "--json")
		if res.err != nil {
			t.Fatalf("unexpected error: %v", res.err)
		}

		got := parseJSON(t, res.stdout)
		if got["added"] != float64(0) {
			t.Errorf("added = %v, want 0", got["added"])
		}
		if got["skipped"] != float64(1) {
			t.Errorf("skipped = %v, want 1", got["skipped"])
		}
	})

	t.Run("all_json", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		seedManifest(t, manPath, newManifest(map[string]manifest.MCPServer{
			"context7": srvContext7,
			"postgres": srvPostgres,
		}))

		res := execCmd(t, home, nil, "mcp", "import", "--manifest", manPath, "--json")
		if res.err != nil {
			t.Fatalf("unexpected error: %v", res.err)
		}

		got := parseJSON(t, res.stdout)
		if got["added"] != float64(2) {
			t.Errorf("added = %v, want 2", got["added"])
		}
		if got["overwritten"] != float64(0) {
			t.Errorf("overwritten = %v, want 0", got["overwritten"])
		}
		if got["skipped"] != float64(0) {
			t.Errorf("skipped = %v, want 0", got["skipped"])
		}
		if got["removed"] != float64(0) {
			t.Errorf("removed = %v, want 0", got["removed"])
		}
	})

	t.Run("selective", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		seedManifest(t, manPath, newManifest(map[string]manifest.MCPServer{
			"context7": srvContext7,
			"postgres": srvPostgres,
		}))

		res := execCmd(t, home, nil, "mcp", "import", "--manifest", manPath, "context7")
		if res.err != nil {
			t.Fatalf("unexpected error: %v", res.err)
		}

		servers := readClaudeServers(t, home)
		if _, ok := servers["context7"]; !ok {
			t.Error("claude config missing 'context7'")
		}
		if _, ok := servers["postgres"]; ok {
			t.Error("claude config should not contain 'postgres' (not selected)")
		}
	})

	t.Run("selective_not_found", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		seedManifest(t, manPath, newManifest(map[string]manifest.MCPServer{
			"context7": srvContext7,
		}))

		res := execCmd(t, home, nil, "mcp", "import", "--manifest", manPath, "nonexistent")
		if res.err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(res.err.Error(), `"nonexistent" not found in manifest`) {
			t.Errorf("error %q missing expected substring", res.err.Error())
		}
	})

	t.Run("preserves_existing", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		seedManifest(t, manPath, newManifest(map[string]manifest.MCPServer{
			"context7": srvContext7,
		}))
		seedClaudeConfig(t, home, map[string]manifest.MCPServer{
			"postgres": srvPostgres,
		})

		res := execCmd(t, home, nil, "mcp", "import", "--manifest", manPath)
		if res.err != nil {
			t.Fatalf("unexpected error: %v", res.err)
		}

		servers := readClaudeServers(t, home)
		if _, ok := servers["context7"]; !ok {
			t.Error("claude config missing 'context7' (newly imported)")
		}
		if _, ok := servers["postgres"]; !ok {
			t.Error("claude config missing 'postgres' (should be preserved)")
		}
	})
}

// --- TestMcpImportConflicts ---

func TestMcpImportConflicts(t *testing.T) {
	// All subtests: manifest has v2, claude has v1 → conflict.
	setupConflict := func(t *testing.T) (home, manPath string) {
		t.Helper()
		home = t.TempDir()
		manPath = filepath.Join(t.TempDir(), "manifest.json")
		seedManifest(t, manPath, newManifest(map[string]manifest.MCPServer{
			"context7": srvContext7V2,
		}))
		seedClaudeConfig(t, home, map[string]manifest.MCPServer{
			"context7": srvContext7,
		})
		return home, manPath
	}

	t.Run("overwrite", func(t *testing.T) {
		home, manPath := setupConflict(t)

		res := execCmd(t, home, nil, "mcp", "import", "--manifest", manPath, "--overwrite", "--json")
		if res.err != nil {
			t.Fatalf("unexpected error: %v", res.err)
		}

		got := parseJSON(t, res.stdout)
		if got["overwritten"] != float64(1) {
			t.Errorf("overwritten = %v, want 1", got["overwritten"])
		}

		servers := readClaudeServers(t, home)
		if !manifest.MCPServersEqual(servers["context7"], srvContext7V2) {
			t.Error("claude config should have v2 after overwrite")
		}
	})

	t.Run("no_overwrite", func(t *testing.T) {
		home, manPath := setupConflict(t)

		res := execCmd(t, home, nil, "mcp", "import", "--manifest", manPath, "--no-overwrite", "--json")
		if res.err != nil {
			t.Fatalf("unexpected error: %v", res.err)
		}

		got := parseJSON(t, res.stdout)
		if got["skipped"] != float64(1) {
			t.Errorf("skipped = %v, want 1", got["skipped"])
		}

		servers := readClaudeServers(t, home)
		if !manifest.MCPServersEqual(servers["context7"], srvContext7) {
			t.Error("claude config should still have v1 after no-overwrite")
		}
	})

	t.Run("force", func(t *testing.T) {
		home, manPath := setupConflict(t)

		res := execCmd(t, home, nil, "--force", "mcp", "import", "--manifest", manPath, "--json")
		if res.err != nil {
			t.Fatalf("unexpected error: %v", res.err)
		}

		got := parseJSON(t, res.stdout)
		if got["overwritten"] != float64(1) {
			t.Errorf("overwritten = %v, want 1", got["overwritten"])
		}

		servers := readClaudeServers(t, home)
		if !manifest.MCPServersEqual(servers["context7"], srvContext7V2) {
			t.Error("claude config should have v2 after --force")
		}
	})

	t.Run("interactive_yes", func(t *testing.T) {
		home, manPath := setupConflict(t)

		res := execCmd(t, home, strings.NewReader("y\n"), "mcp", "import", "--manifest", manPath)
		if res.err != nil {
			t.Fatalf("unexpected error: %v", res.err)
		}

		servers := readClaudeServers(t, home)
		if !manifest.MCPServersEqual(servers["context7"], srvContext7V2) {
			t.Error("claude config should have v2 after confirming overwrite")
		}
	})

	t.Run("interactive_no", func(t *testing.T) {
		home, manPath := setupConflict(t)

		res := execCmd(t, home, strings.NewReader("n\n"), "mcp", "import", "--manifest", manPath)
		if res.err != nil {
			t.Fatalf("unexpected error: %v", res.err)
		}

		servers := readClaudeServers(t, home)
		if !manifest.MCPServersEqual(servers["context7"], srvContext7) {
			t.Error("claude config should still have v1 after declining overwrite")
		}
	})

	t.Run("multiple_interactive", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		// Two conflicts: context7 (say yes) and postgres (say no).
		// Production code sorts toConflict alphabetically (mcp.go:191),
		// so prompts arrive in order: context7, postgres.
		srvPostgresV2 := manifest.MCPServer{
			Command: "pg-mcp-v2",
			Env:     map[string]string{"DSN": "postgres://localhost/prod"},
		}

		seedManifest(t, manPath, newManifest(map[string]manifest.MCPServer{
			"context7": srvContext7V2,
			"postgres": srvPostgresV2,
		}))
		seedClaudeConfig(t, home, map[string]manifest.MCPServer{
			"context7": srvContext7,
			"postgres": srvPostgres,
		})

		// Conflicts are sorted alphabetically: context7 first, then postgres.
		// Answer "y" for context7, "n" for postgres.
		res := execCmd(t, home, strings.NewReader("y\nn\n"), "mcp", "import", "--manifest", manPath)
		if res.err != nil {
			t.Fatalf("unexpected error: %v", res.err)
		}

		servers := readClaudeServers(t, home)
		if !manifest.MCPServersEqual(servers["context7"], srvContext7V2) {
			t.Error("context7 should be overwritten (answered yes)")
		}
		if !manifest.MCPServersEqual(servers["postgres"], srvPostgres) {
			t.Error("postgres should be kept (answered no)")
		}
	})
}

// --- TestMcpImportStrict ---

func TestMcpImportStrict(t *testing.T) {
	t.Run("removes_extra_force", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		seedManifest(t, manPath, newManifest(map[string]manifest.MCPServer{
			"context7": srvContext7,
		}))
		seedClaudeConfig(t, home, map[string]manifest.MCPServer{
			"context7": srvContext7,
			"postgres": srvPostgres,
		})

		res := execCmd(t, home, nil, "--force", "mcp", "import", "--manifest", manPath, "--strict", "--json")
		if res.err != nil {
			t.Fatalf("unexpected error: %v", res.err)
		}

		got := parseJSON(t, res.stdout)
		if got["removed"] != float64(1) {
			t.Errorf("removed = %v, want 1", got["removed"])
		}

		servers := readClaudeServers(t, home)
		if _, ok := servers["postgres"]; ok {
			t.Error("postgres should be removed by --strict")
		}
		if _, ok := servers["context7"]; !ok {
			t.Error("context7 should still exist")
		}
	})

	t.Run("removes_extra_confirmed", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		seedManifest(t, manPath, newManifest(map[string]manifest.MCPServer{
			"context7": srvContext7,
		}))
		seedClaudeConfig(t, home, map[string]manifest.MCPServer{
			"context7": srvContext7,
			"postgres": srvPostgres,
		})

		res := execCmd(t, home, strings.NewReader("y\n"), "mcp", "import", "--manifest", manPath, "--strict")
		if res.err != nil {
			t.Fatalf("unexpected error: %v", res.err)
		}

		servers := readClaudeServers(t, home)
		if _, ok := servers["postgres"]; ok {
			t.Error("postgres should be removed after confirming --strict removal")
		}
	})

	t.Run("removes_extra_declined", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		seedManifest(t, manPath, newManifest(map[string]manifest.MCPServer{
			"context7": srvContext7,
		}))
		seedClaudeConfig(t, home, map[string]manifest.MCPServer{
			"context7": srvContext7,
			"postgres": srvPostgres,
		})

		res := execCmd(t, home, strings.NewReader("n\n"), "mcp", "import", "--manifest", manPath, "--strict")
		if res.err != nil {
			t.Fatalf("unexpected error: %v", res.err)
		}

		if !strings.Contains(res.stderr, "Aborted") {
			t.Errorf("stderr missing 'Aborted': %q", res.stderr)
		}

		servers := readClaudeServers(t, home)
		if _, ok := servers["postgres"]; !ok {
			t.Error("postgres should still exist after declining --strict removal")
		}
	})

	t.Run("nothing_to_remove", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		seedManifest(t, manPath, newManifest(map[string]manifest.MCPServer{
			"context7": srvContext7,
		}))
		seedClaudeConfig(t, home, map[string]manifest.MCPServer{
			"context7": srvContext7,
		})

		res := execCmd(t, home, nil, "mcp", "import", "--manifest", manPath, "--strict", "--json")
		if res.err != nil {
			t.Fatalf("unexpected error: %v", res.err)
		}

		got := parseJSON(t, res.stdout)
		if got["removed"] != float64(0) {
			t.Errorf("removed = %v, want 0", got["removed"])
		}
	})

	t.Run("add_and_remove", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		seedManifest(t, manPath, newManifest(map[string]manifest.MCPServer{
			"context7": srvContext7,
		}))
		seedClaudeConfig(t, home, map[string]manifest.MCPServer{
			"postgres": srvPostgres,
		})

		res := execCmd(t, home, nil, "--force", "mcp", "import", "--manifest", manPath, "--strict", "--json")
		if res.err != nil {
			t.Fatalf("unexpected error: %v", res.err)
		}

		got := parseJSON(t, res.stdout)
		if got["added"] != float64(1) {
			t.Errorf("added = %v, want 1", got["added"])
		}
		if got["removed"] != float64(1) {
			t.Errorf("removed = %v, want 1", got["removed"])
		}

		servers := readClaudeServers(t, home)
		if _, ok := servers["context7"]; !ok {
			t.Error("context7 should be added")
		}
		if _, ok := servers["postgres"]; ok {
			t.Error("postgres should be removed by --strict")
		}
	})
}

// --- TestMcpImportDryRun ---

func TestMcpImportDryRun(t *testing.T) {
	t.Run("add", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		seedManifest(t, manPath, newManifest(map[string]manifest.MCPServer{
			"context7": srvContext7,
		}))

		res := execCmd(t, home, nil, "mcp", "import", "--manifest", manPath, "--dry-run", "--json")
		if res.err != nil {
			t.Fatalf("unexpected error: %v", res.err)
		}

		got := parseJSON(t, res.stdout)
		add, ok := got["add"].([]any)
		if !ok {
			t.Fatalf("add is %T, want []any", got["add"])
		}
		if len(add) != 1 || add[0] != "context7" {
			t.Errorf("add = %v, want [context7]", add)
		}

		// Verify no changes to claude config.
		servers := readClaudeServers(t, home)
		if len(servers) != 0 {
			t.Errorf("claude config should be unchanged after dry-run, has %d servers", len(servers))
		}
	})

	t.Run("conflict", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		seedManifest(t, manPath, newManifest(map[string]manifest.MCPServer{
			"context7": srvContext7V2,
		}))
		seedClaudeConfig(t, home, map[string]manifest.MCPServer{
			"context7": srvContext7,
		})

		res := execCmd(t, home, nil, "mcp", "import", "--manifest", manPath, "--dry-run", "--json")
		if res.err != nil {
			t.Fatalf("unexpected error: %v", res.err)
		}

		got := parseJSON(t, res.stdout)
		conflict, ok := got["conflict"].([]any)
		if !ok {
			t.Fatalf("conflict is %T, want []any", got["conflict"])
		}
		if len(conflict) != 1 || conflict[0] != "context7" {
			t.Errorf("conflict = %v, want [context7]", conflict)
		}

		// Verify no changes.
		servers := readClaudeServers(t, home)
		if !manifest.MCPServersEqual(servers["context7"], srvContext7) {
			t.Error("claude config should be unchanged after dry-run")
		}
	})

	t.Run("strict", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		seedManifest(t, manPath, newManifest(map[string]manifest.MCPServer{
			"context7": srvContext7,
		}))
		seedClaudeConfig(t, home, map[string]manifest.MCPServer{
			"context7": srvContext7,
			"postgres": srvPostgres,
		})

		res := execCmd(t, home, nil, "mcp", "import", "--manifest", manPath, "--dry-run", "--strict", "--json")
		if res.err != nil {
			t.Fatalf("unexpected error: %v", res.err)
		}

		got := parseJSON(t, res.stdout)
		skip, ok := got["skip"].([]any)
		if !ok {
			t.Fatalf("skip is %T, want []any", got["skip"])
		}
		if len(skip) != 1 || skip[0] != "context7" {
			t.Errorf("skip = %v, want [context7]", skip)
		}
		remove, ok := got["remove"].([]any)
		if !ok {
			t.Fatalf("remove is %T, want []any", got["remove"])
		}
		if len(remove) != 1 || remove[0] != "postgres" {
			t.Errorf("remove = %v, want [postgres]", remove)
		}

		// Verify no changes.
		servers := readClaudeServers(t, home)
		if len(servers) != 2 {
			t.Errorf("claude config should be unchanged after dry-run, has %d servers", len(servers))
		}
	})

	t.Run("nothing", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		seedManifest(t, manPath, newManifest(nil))

		res := execCmd(t, home, nil, "mcp", "import", "--manifest", manPath, "--dry-run", "--json")
		if res.err != nil {
			t.Fatalf("unexpected error: %v", res.err)
		}

		got := parseJSON(t, res.stdout)
		for _, key := range []string{"add", "skip", "conflict", "remove"} {
			arr, ok := got[key].([]any)
			if !ok {
				t.Errorf("%s is %T, want []any", key, got[key])
				continue
			}
			if len(arr) != 0 {
				t.Errorf("%s = %v, want empty", key, arr)
			}
		}
	})
}

// --- TestMcpRemove ---

func TestMcpRemove(t *testing.T) {
	t.Run("simple", func(t *testing.T) {
		manPath := filepath.Join(t.TempDir(), "manifest.json")
		home := t.TempDir()

		seedManifest(t, manPath, newManifest(map[string]manifest.MCPServer{
			"context7": srvContext7,
			"postgres": srvPostgres,
		}))

		res := execCmd(t, home, nil, "mcp", "remove", "--manifest", manPath, "context7")
		if res.err != nil {
			t.Fatalf("unexpected error: %v", res.err)
		}

		m := loadManifestFile(t, manPath)
		if _, ok := m.MCPServers["context7"]; ok {
			t.Error("context7 should be removed")
		}
		if _, ok := m.MCPServers["postgres"]; !ok {
			t.Error("postgres should still exist")
		}
	})

	t.Run("not_found", func(t *testing.T) {
		manPath := filepath.Join(t.TempDir(), "manifest.json")
		home := t.TempDir()

		seedManifest(t, manPath, newManifest(map[string]manifest.MCPServer{
			"context7": srvContext7,
		}))

		res := execCmd(t, home, nil, "mcp", "remove", "--manifest", manPath, "nonexistent")
		if res.err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(res.err.Error(), `"nonexistent" not found in manifest`) {
			t.Errorf("error %q missing expected substring", res.err.Error())
		}
	})

	t.Run("json", func(t *testing.T) {
		manPath := filepath.Join(t.TempDir(), "manifest.json")
		home := t.TempDir()

		seedManifest(t, manPath, newManifest(map[string]manifest.MCPServer{
			"context7": srvContext7,
		}))

		res := execCmd(t, home, nil, "mcp", "remove", "--manifest", manPath, "--json", "context7")
		if res.err != nil {
			t.Fatalf("unexpected error: %v", res.err)
		}

		got := parseJSON(t, res.stdout)
		if got["removed"] != "context7" {
			t.Errorf("removed = %v, want 'context7'", got["removed"])
		}
	})

	t.Run("no_args", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")
		seedManifest(t, manPath, newManifest(nil))

		res := execCmd(t, home, nil, "mcp", "remove", "--manifest", manPath)
		if res.err == nil {
			t.Fatal("expected error for missing arg")
		}
		if !strings.Contains(res.err.Error(), "accepts 1 arg") {
			t.Errorf("error %q missing 'accepts 1 arg'", res.err.Error())
		}
	})

	t.Run("two_args", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")
		seedManifest(t, manPath, newManifest(nil))

		res := execCmd(t, home, nil, "mcp", "remove", "--manifest", manPath, "a", "b")
		if res.err == nil {
			t.Fatal("expected error for too many args")
		}
		if !strings.Contains(res.err.Error(), "accepts 1 arg") {
			t.Errorf("error %q missing 'accepts 1 arg'", res.err.Error())
		}
	})

	t.Run("cascade_force", func(t *testing.T) {
		manPath := filepath.Join(t.TempDir(), "manifest.json")
		home := t.TempDir()

		seedManifest(t, manPath, newManifestWithProfiles(
			map[string]manifest.MCPServer{
				"context7": srvContext7,
				"postgres": srvPostgres,
			},
			map[string]manifest.Profile{
				"work": {MCPServers: []string{"context7"}, Plugins: []manifest.ProfilePlugin{}},
			},
		))

		res := execCmd(t, home, nil, "--force", "mcp", "remove", "--manifest", manPath, "--json", "context7")
		if res.err != nil {
			t.Fatalf("unexpected error: %v", res.err)
		}

		got := parseJSON(t, res.stdout)
		if got["removed"] != "context7" {
			t.Errorf("removed = %v, want 'context7'", got["removed"])
		}

		cleaned, ok := got["cleanedProfiles"].([]any)
		if !ok || len(cleaned) != 1 || cleaned[0] != "work" {
			t.Errorf("cleanedProfiles = %v, want [work]", got["cleanedProfiles"])
		}

		m := loadManifestFile(t, manPath)
		if _, ok := m.MCPServers["context7"]; ok {
			t.Error("context7 should be removed from servers")
		}
		if profile, ok := m.Profiles["work"]; ok {
			for _, s := range profile.MCPServers {
				if s == "context7" {
					t.Error("context7 should be removed from 'work' profile")
				}
			}
		}
	})

	t.Run("cascade_confirmed", func(t *testing.T) {
		manPath := filepath.Join(t.TempDir(), "manifest.json")
		home := t.TempDir()

		seedManifest(t, manPath, newManifestWithProfiles(
			map[string]manifest.MCPServer{
				"context7": srvContext7,
			},
			map[string]manifest.Profile{
				"work": {MCPServers: []string{"context7"}, Plugins: []manifest.ProfilePlugin{}},
			},
		))

		res := execCmd(t, home, strings.NewReader("y\n"), "mcp", "remove", "--manifest", manPath, "context7")
		if res.err != nil {
			t.Fatalf("unexpected error: %v", res.err)
		}

		m := loadManifestFile(t, manPath)
		if _, ok := m.MCPServers["context7"]; ok {
			t.Error("context7 should be removed after confirming cascade")
		}
	})

	t.Run("cascade_declined", func(t *testing.T) {
		manPath := filepath.Join(t.TempDir(), "manifest.json")
		home := t.TempDir()

		seedManifest(t, manPath, newManifestWithProfiles(
			map[string]manifest.MCPServer{
				"context7": srvContext7,
			},
			map[string]manifest.Profile{
				"work": {MCPServers: []string{"context7"}, Plugins: []manifest.ProfilePlugin{}},
			},
		))

		res := execCmd(t, home, strings.NewReader("n\n"), "mcp", "remove", "--manifest", manPath, "context7")
		if res.err != nil {
			t.Fatalf("unexpected error: %v", res.err)
		}

		if !strings.Contains(res.stderr, "Aborted") {
			t.Errorf("stderr missing 'Aborted': %q", res.stderr)
		}

		m := loadManifestFile(t, manPath)
		if _, ok := m.MCPServers["context7"]; !ok {
			t.Error("context7 should still exist after declining cascade")
		}
	})

	t.Run("multiple_profiles", func(t *testing.T) {
		manPath := filepath.Join(t.TempDir(), "manifest.json")
		home := t.TempDir()

		seedManifest(t, manPath, newManifestWithProfiles(
			map[string]manifest.MCPServer{
				"context7": srvContext7,
			},
			map[string]manifest.Profile{
				"work": {MCPServers: []string{"context7"}, Plugins: []manifest.ProfilePlugin{}},
				"home": {MCPServers: []string{"context7"}, Plugins: []manifest.ProfilePlugin{}},
			},
		))

		res := execCmd(t, home, nil, "--force", "mcp", "remove", "--manifest", manPath, "--json", "context7")
		if res.err != nil {
			t.Fatalf("unexpected error: %v", res.err)
		}

		got := parseJSON(t, res.stdout)
		cleaned, ok := got["cleanedProfiles"].([]any)
		if !ok {
			t.Fatalf("cleanedProfiles is %T, want []any", got["cleanedProfiles"])
		}
		if len(cleaned) != 2 {
			t.Errorf("cleanedProfiles has %d entries, want 2", len(cleaned))
		}
		// Verify sorted: home before work.
		if len(cleaned) == 2 && (cleaned[0] != "home" || cleaned[1] != "work") {
			t.Errorf("cleanedProfiles = %v, want [home work] (sorted)", cleaned)
		}

		m := loadManifestFile(t, manPath)
		if _, ok := m.MCPServers["context7"]; ok {
			t.Error("context7 should be removed")
		}
		for pName, profile := range m.Profiles {
			for _, s := range profile.MCPServers {
				if s == "context7" {
					t.Errorf("context7 should be removed from profile %q", pName)
				}
			}
		}
	})
}

// --- TestMcpList ---

func TestMcpList(t *testing.T) {
	t.Run("manifest_table", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		seedManifest(t, manPath, newManifest(map[string]manifest.MCPServer{
			"context7": srvContext7,
			"sse":      srvRemoteSSE,
		}))

		res := execCmd(t, home, nil, "mcp", "list", "--manifest", manPath)
		if res.err != nil {
			t.Fatalf("unexpected error: %v", res.err)
		}

		if !strings.Contains(res.stdout, "context7") {
			t.Errorf("stdout missing 'context7': %q", res.stdout)
		}
		if !strings.Contains(res.stdout, "sse") {
			t.Errorf("stdout missing 'sse': %q", res.stdout)
		}
	})

	t.Run("manifest_json", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		seedManifest(t, manPath, newManifest(map[string]manifest.MCPServer{
			"context7": srvContext7,
			"sse":      srvRemoteSSE,
		}))

		res := execCmd(t, home, nil, "mcp", "list", "--manifest", manPath, "--json")
		if res.err != nil {
			t.Fatalf("unexpected error: %v", res.err)
		}

		got := parseJSON(t, res.stdout)
		if _, ok := got["context7"]; !ok {
			t.Error("JSON missing 'context7'")
		}
		if _, ok := got["sse"]; !ok {
			t.Error("JSON missing 'sse'")
		}
	})

	t.Run("manifest_empty", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")
		seedManifest(t, manPath, newManifest(nil))

		res := execCmd(t, home, nil, "mcp", "list", "--manifest", manPath)
		if res.err != nil {
			t.Fatalf("unexpected error: %v", res.err)
		}

		if !strings.Contains(res.stderr, "No MCP servers") {
			t.Errorf("stderr missing 'No MCP servers': %q", res.stderr)
		}
	})

	t.Run("manifest_empty_json", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")
		seedManifest(t, manPath, newManifest(nil))

		res := execCmd(t, home, nil, "mcp", "list", "--manifest", manPath, "--json")
		if res.err != nil {
			t.Fatalf("unexpected error: %v", res.err)
		}

		got := parseJSON(t, res.stdout)
		if len(got) != 0 {
			t.Errorf("expected empty JSON object, got %v", got)
		}
	})

	t.Run("installed_table", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		seedClaudeConfig(t, home, map[string]manifest.MCPServer{
			"context7": srvContext7,
			"sse":      srvRemoteSSE,
		})

		res := execCmd(t, home, nil, "mcp", "list", "--manifest", manPath, "--installed")
		if res.err != nil {
			t.Fatalf("unexpected error: %v", res.err)
		}

		if !strings.Contains(res.stdout, "context7") {
			t.Errorf("stdout missing 'context7': %q", res.stdout)
		}
		if !strings.Contains(res.stdout, "sse") {
			t.Errorf("stdout missing 'sse': %q", res.stdout)
		}
	})

	t.Run("installed_json", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		seedClaudeConfig(t, home, map[string]manifest.MCPServer{
			"context7": srvContext7,
			"sse":      srvRemoteSSE,
		})

		res := execCmd(t, home, nil, "mcp", "list", "--manifest", manPath, "--installed", "--json")
		if res.err != nil {
			t.Fatalf("unexpected error: %v", res.err)
		}

		got := parseJSON(t, res.stdout)
		if _, ok := got["context7"]; !ok {
			t.Error("JSON missing 'context7'")
		}
		if _, ok := got["sse"]; !ok {
			t.Error("JSON missing 'sse'")
		}
	})

	t.Run("installed_empty", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		// No claude config file seeded.

		res := execCmd(t, home, nil, "mcp", "list", "--manifest", manPath, "--installed")
		if res.err != nil {
			t.Fatalf("unexpected error: %v", res.err)
		}

		if !strings.Contains(res.stderr, "No MCP servers") {
			t.Errorf("stderr missing 'No MCP servers': %q", res.stderr)
		}
	})
}

// --- TestMcpListTransportAndDetail ---

func TestMcpListTransportAndDetail(t *testing.T) {
	tests := []struct {
		name          string
		server        manifest.MCPServer
		wantTransport string // Expected type field in JSON (empty means omitted).
		wantDetail    string // Substring to find in the JSON output.
	}{
		{
			name:          "stdio_with_args",
			server:        manifest.MCPServer{Command: "npx", Args: []string{"-y", "pkg"}},
			wantTransport: "",
			wantDetail:    "npx",
		},
		{
			name:          "stdio_no_args",
			server:        manifest.MCPServer{Command: "echo"},
			wantTransport: "",
			wantDetail:    "echo",
		},
		{
			name:          "sse_with_url",
			server:        manifest.MCPServer{Type: "sse", URL: "https://example.com/sse"},
			wantTransport: "sse",
			wantDetail:    "https://example.com/sse",
		},
		{
			name:          "http_with_url",
			server:        manifest.MCPServer{Type: "http", URL: "https://example.com/api"},
			wantTransport: "http",
			wantDetail:    "https://example.com/api",
		},
		{
			name:          "empty_type_defaults_stdio",
			server:        manifest.MCPServer{Command: "foo"},
			wantTransport: "",
			wantDetail:    "foo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			manPath := filepath.Join(t.TempDir(), "manifest.json")

			seedManifest(t, manPath, newManifest(map[string]manifest.MCPServer{
				"test-server": tt.server,
			}))

			res := execCmd(t, home, nil, "mcp", "list", "--manifest", manPath, "--json")
			if res.err != nil {
				t.Fatalf("unexpected error: %v", res.err)
			}

			got := parseJSON(t, res.stdout)
			srvRaw, ok := got["test-server"]
			if !ok {
				t.Fatal("JSON missing 'test-server'")
			}

			srv, ok := srvRaw.(map[string]any)
			if !ok {
				t.Fatalf("test-server is %T, want map[string]any", srvRaw)
			}

			// Check transport type.
			gotType, _ := srv["type"].(string)
			if gotType != tt.wantTransport {
				t.Errorf("type = %q, want %q", gotType, tt.wantTransport)
			}

			// Check that the detail data is present in the serialized output.
			if !strings.Contains(res.stdout, tt.wantDetail) {
				t.Errorf("stdout missing %q", tt.wantDetail)
			}
		})
	}
}

// --- TestMcpExportScopeProject ---

func TestMcpExportScopeProject(t *testing.T) {
	t.Run("exports_from_project_mcp_json", func(t *testing.T) {
		home := t.TempDir()
		projectDir := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		seedProjectMcpConfig(t, projectDir, map[string]manifest.MCPServer{
			"context7": srvContext7,
		})

		res := execCmdWith(t, home, nil, withProjectDir(projectDir), "mcp", "export", "--scope", "project", "--manifest", manPath)
		if res.err != nil {
			t.Fatalf("unexpected error: %v\nstderr: %s", res.err, res.stderr)
		}

		m := loadManifestFile(t, manPath)
		if len(m.MCPServers) != 1 {
			t.Errorf("manifest has %d servers, want 1", len(m.MCPServers))
		}
		if !manifest.MCPServersEqual(m.MCPServers["context7"], srvContext7) {
			t.Error("context7 content mismatch")
		}
	})

	t.Run("empty_project_mcp_json", func(t *testing.T) {
		home := t.TempDir()
		projectDir := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		// No .mcp.json seeded — ReadProjectMcpServers returns empty map for missing file.

		res := execCmdWith(t, home, nil, withProjectDir(projectDir), "mcp", "export", "--scope", "project", "--manifest", manPath)
		if res.err != nil {
			t.Fatalf("unexpected error: %v", res.err)
		}
		if !strings.Contains(res.stderr, "No MCP servers") {
			t.Errorf("stderr missing empty message: %q", res.stderr)
		}
	})
}

// --- TestMcpImportScopeProject ---

func TestMcpImportScopeProject(t *testing.T) {
	t.Run("imports_to_project_mcp_json", func(t *testing.T) {
		home := t.TempDir()
		projectDir := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		seedManifest(t, manPath, newManifest(map[string]manifest.MCPServer{
			"context7": srvContext7,
			"postgres": srvPostgres,
		}))

		res := execCmdWith(t, home, nil, withProjectDir(projectDir), "mcp", "import", "--scope", "project", "--manifest", manPath)
		if res.err != nil {
			t.Fatalf("unexpected error: %v\nstderr: %s", res.err, res.stderr)
		}

		servers := readProjectServers(t, projectDir)
		if len(servers) != 2 {
			t.Fatalf("project has %d servers, want 2", len(servers))
		}
		if !manifest.MCPServersEqual(servers["context7"], srvContext7) {
			t.Error("context7 content mismatch")
		}
		if !manifest.MCPServersEqual(servers["postgres"], srvPostgres) {
			t.Error("postgres content mismatch")
		}
	})

	t.Run("overwrite_existing_project_server", func(t *testing.T) {
		home := t.TempDir()
		projectDir := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		seedProjectMcpConfig(t, projectDir, map[string]manifest.MCPServer{
			"context7": srvContext7,
		})
		seedManifest(t, manPath, newManifest(map[string]manifest.MCPServer{
			"context7": srvContext7V2,
		}))

		res := execCmdWith(t, home, nil, withProjectDir(projectDir), "mcp", "import", "--scope", "project", "--manifest", manPath, "--overwrite")
		if res.err != nil {
			t.Fatalf("unexpected error: %v\nstderr: %s", res.err, res.stderr)
		}

		servers := readProjectServers(t, projectDir)
		if len(servers) != 1 {
			t.Fatalf("project has %d servers, want 1", len(servers))
		}
		if !manifest.MCPServersEqual(servers["context7"], srvContext7V2) {
			t.Error("context7 should be overwritten with v2")
		}
	})

	t.Run("strict_removes_from_project", func(t *testing.T) {
		home := t.TempDir()
		projectDir := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		seedProjectMcpConfig(t, projectDir, map[string]manifest.MCPServer{
			"context7": srvContext7,
			"postgres": srvPostgres,
		})
		seedManifest(t, manPath, newManifest(map[string]manifest.MCPServer{
			"context7": srvContext7,
		}))

		res := execCmdWith(t, home, nil, withProjectDir(projectDir), "mcp", "import", "--scope", "project", "--manifest", manPath, "--strict", "-f")
		if res.err != nil {
			t.Fatalf("unexpected error: %v\nstderr: %s", res.err, res.stderr)
		}

		servers := readProjectServers(t, projectDir)
		if _, ok := servers["postgres"]; ok {
			t.Error("postgres should be removed by --strict")
		}
		if _, ok := servers["context7"]; !ok {
			t.Error("context7 should still exist")
		}
	})
}

// --- TestMcpListScopeProject ---

func TestMcpListScopeProject(t *testing.T) {
	t.Run("lists_from_project_mcp_json", func(t *testing.T) {
		home := t.TempDir()
		projectDir := t.TempDir()

		seedProjectMcpConfig(t, projectDir, map[string]manifest.MCPServer{
			"context7": srvContext7,
		})

		res := execCmdWith(t, home, nil, withProjectDir(projectDir), "mcp", "list", "--installed", "--scope", "project", "--json")
		if res.err != nil {
			t.Fatalf("unexpected error: %v", res.err)
		}

		out := parseJSON(t, res.stdout)
		if _, ok := out["context7"]; !ok {
			t.Error("expected context7 in JSON output")
		}
	})

	t.Run("empty_project", func(t *testing.T) {
		home := t.TempDir()
		projectDir := t.TempDir()

		res := execCmdWith(t, home, nil, withProjectDir(projectDir), "mcp", "list", "--installed", "--scope", "project")
		if res.err != nil {
			t.Fatalf("unexpected error: %v", res.err)
		}
		if !strings.Contains(res.stderr, "No MCP servers in .mcp.json") {
			t.Errorf("stderr missing empty message: %q", res.stderr)
		}
	})
}

// --- TestScopeValidation ---

func TestScopeValidation(t *testing.T) {
	t.Run("invalid_scope_rejected", func(t *testing.T) {
		home := t.TempDir()
		res := execCmd(t, home, nil, "mcp", "export", "--scope", "invalid")
		if res.err == nil {
			t.Fatal("expected error for invalid scope")
		}
		if !strings.Contains(res.err.Error(), "invalid --scope") {
			t.Errorf("error missing scope message: %v", res.err)
		}
	})
}
