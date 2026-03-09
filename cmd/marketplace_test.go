package cmd

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alphaleonis/cctote/internal/manifest"
)

// --- Fixtures ---

var (
	mpGitHub = manifest.Marketplace{Source: "github", Repo: "user/my-marketplace"}
	mpGit    = manifest.Marketplace{Source: "git", URL: "https://git.example.com/plugins.git"}
	mpDir    = manifest.Marketplace{Source: "directory", Path: "/home/user/local-plugins"}

	mpGitHubV2 = manifest.Marketplace{Source: "github", Repo: "user/my-marketplace-v2"}
)

// newMarketplaceManifest creates a manifest with the given marketplaces and plugins.
func newMarketplaceManifest(mps map[string]manifest.Marketplace, plugins []manifest.Plugin) *manifest.Manifest {
	m := &manifest.Manifest{
		Version:      manifest.CurrentVersion,
		MCPServers:   map[string]manifest.MCPServer{},
		Plugins:      plugins,
		Marketplaces: mps,
	}
	if m.Plugins == nil {
		m.Plugins = []manifest.Plugin{}
	}
	if m.Marketplaces == nil {
		m.Marketplaces = map[string]manifest.Marketplace{}
	}
	return m
}

// --- TestMarketplaceExport ---

func TestMarketplaceExport(t *testing.T) {
	t.Run("export_all_fresh", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		runner := newFakeRunner()
		runner.on("plugin marketplace list --json", marketplacesJSON(map[string]manifest.Marketplace{
			"mp-github": mpGitHub,
			"mp-git":    mpGit,
		}), nil)

		res := execCmdWith(t, home, nil, withPluginClient(runner), "marketplace", "export", "--manifest", manPath, "--json")
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

		m := loadManifestFile(t, manPath)
		if len(m.Marketplaces) != 2 {
			t.Errorf("manifest has %d marketplaces, want 2", len(m.Marketplaces))
		}
	})

	t.Run("export_selective", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		runner := newFakeRunner()
		runner.on("plugin marketplace list --json", marketplacesJSON(map[string]manifest.Marketplace{
			"mp-github": mpGitHub,
			"mp-git":    mpGit,
		}), nil)

		res := execCmdWith(t, home, nil, withPluginClient(runner), "marketplace", "export", "--manifest", manPath, "--json", "mp-github")
		if res.err != nil {
			t.Fatalf("unexpected error: %v\nstderr: %s", res.err, res.stderr)
		}

		m := loadManifestFile(t, manPath)
		if len(m.Marketplaces) != 1 {
			t.Errorf("manifest has %d marketplaces, want 1", len(m.Marketplaces))
		}
		if _, ok := m.Marketplaces["mp-github"]; !ok {
			t.Error("manifest missing mp-github")
		}
	})

	t.Run("export_not_found", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		runner := newFakeRunner()
		runner.on("plugin marketplace list --json", marketplacesJSON(map[string]manifest.Marketplace{
			"mp-github": mpGitHub,
		}), nil)

		res := execCmdWith(t, home, nil, withPluginClient(runner), "marketplace", "export", "--manifest", manPath, "nonexistent")
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
		runner.on("plugin marketplace list --json", marketplacesJSON(nil), nil)

		res := execCmdWith(t, home, nil, withPluginClient(runner), "marketplace", "export", "--manifest", manPath)
		if res.err != nil {
			t.Fatalf("unexpected error: %v", res.err)
		}
		if !strings.Contains(res.stderr, "No marketplaces found") {
			t.Errorf("stderr missing 'No marketplaces found': %q", res.stderr)
		}
	})

	t.Run("export_empty_json", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		runner := newFakeRunner()
		runner.on("plugin marketplace list --json", marketplacesJSON(nil), nil)

		res := execCmdWith(t, home, nil, withPluginClient(runner), "marketplace", "export", "--manifest", manPath, "--json")
		if res.err != nil {
			t.Fatalf("unexpected error: %v", res.err)
		}

		got := parseJSON(t, res.stdout)
		if got["added"] != float64(0) {
			t.Errorf("added = %v, want 0", got["added"])
		}
	})

	t.Run("export_updates_existing", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		// Seed manifest with mp-github (old version).
		seedManifest(t, manPath, newMarketplaceManifest(
			map[string]manifest.Marketplace{"mp-github": mpGitHub}, nil,
		))

		// Claude has mp-github (new version).
		runner := newFakeRunner()
		runner.on("plugin marketplace list --json", marketplacesJSON(map[string]manifest.Marketplace{
			"mp-github": mpGitHubV2,
		}), nil)

		res := execCmdWith(t, home, nil, withPluginClient(runner), "marketplace", "export", "--manifest", manPath, "--json")
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
		if m.Marketplaces["mp-github"].Repo != "user/my-marketplace-v2" {
			t.Errorf("marketplace not updated: %+v", m.Marketplaces["mp-github"])
		}
	})

	t.Run("export_directory_source_warning", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		runner := newFakeRunner()
		runner.on("plugin marketplace list --json", marketplacesJSON(map[string]manifest.Marketplace{
			"mp-dir": mpDir,
		}), nil)

		res := execCmdWith(t, home, nil, withPluginClient(runner), "marketplace", "export", "--manifest", manPath)
		if res.err != nil {
			t.Fatalf("unexpected error: %v\nstderr: %s", res.err, res.stderr)
		}
		if !strings.Contains(res.stderr, "directory source") {
			t.Errorf("stderr missing directory source warning: %q", res.stderr)
		}
	})

	t.Run("export_text_output", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		runner := newFakeRunner()
		runner.on("plugin marketplace list --json", marketplacesJSON(map[string]manifest.Marketplace{
			"mp-github": mpGitHub,
		}), nil)

		res := execCmdWith(t, home, nil, withPluginClient(runner), "marketplace", "export", "--manifest", manPath)
		if res.err != nil {
			t.Fatalf("unexpected error: %v\nstderr: %s", res.err, res.stderr)
		}
		if !strings.Contains(res.stderr, "Exported 1 marketplace") {
			t.Errorf("stderr missing success message: %q", res.stderr)
		}
	})
}

// --- TestMarketplaceImport ---

func TestMarketplaceImport(t *testing.T) {
	t.Run("add_new", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		seedManifest(t, manPath, newMarketplaceManifest(
			map[string]manifest.Marketplace{"mp-github": mpGitHub}, nil,
		))

		runner := newFakeRunner()
		runner.on("plugin marketplace list --json", marketplacesJSON(nil), nil)
		runner.on("plugin marketplace add", nil, nil)

		res := execCmdWith(t, home, nil, withPluginClient(runner), "marketplace", "import", "--manifest", manPath, "--json")
		if res.err != nil {
			t.Fatalf("unexpected error: %v\nstderr: %s", res.err, res.stderr)
		}

		got := parseJSON(t, res.stdout)
		if got["added"] != float64(1) {
			t.Errorf("added = %v, want 1", got["added"])
		}
		if !runner.called("plugin", "marketplace", "add", "user/my-marketplace") {
			t.Error("expected 'plugin marketplace add user/my-marketplace' call")
		}
	})

	t.Run("skip_identical", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		seedManifest(t, manPath, newMarketplaceManifest(
			map[string]manifest.Marketplace{"mp-github": mpGitHub}, nil,
		))

		runner := newFakeRunner()
		runner.on("plugin marketplace list --json", marketplacesJSON(map[string]manifest.Marketplace{
			"mp-github": mpGitHub,
		}), nil)

		res := execCmdWith(t, home, nil, withPluginClient(runner), "marketplace", "import", "--manifest", manPath, "--json")
		if res.err != nil {
			t.Fatalf("unexpected error: %v\nstderr: %s", res.err, res.stderr)
		}

		got := parseJSON(t, res.stdout)
		if got["added"] != float64(0) {
			t.Errorf("added = %v, want 0", got["added"])
		}
		if got["skipped"] != float64(1) {
			t.Errorf("skipped = %v, want 1", got["skipped"])
		}
	})

	t.Run("conflict_overwrite", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		seedManifest(t, manPath, newMarketplaceManifest(
			map[string]manifest.Marketplace{"mp-github": mpGitHubV2}, nil,
		))

		runner := newFakeRunner()
		runner.on("plugin marketplace list --json", marketplacesJSON(map[string]manifest.Marketplace{
			"mp-github": mpGitHub, // differs from manifest
		}), nil)
		runner.on("plugin marketplace remove", nil, nil)
		runner.on("plugin marketplace add", nil, nil)

		res := execCmdWith(t, home, nil, withPluginClient(runner), "marketplace", "import", "--manifest", manPath, "--overwrite", "--json")
		if res.err != nil {
			t.Fatalf("unexpected error: %v\nstderr: %s", res.err, res.stderr)
		}

		got := parseJSON(t, res.stdout)
		if got["overwritten"] != float64(1) {
			t.Errorf("overwritten = %v, want 1", got["overwritten"])
		}
		// Overwrite = remove then add.
		if !runner.called("plugin", "marketplace", "remove", "mp-github") {
			t.Error("expected remove call for overwrite")
		}
		if !runner.called("plugin", "marketplace", "add", "user/my-marketplace-v2") {
			t.Error("expected add call for overwrite")
		}
	})

	t.Run("conflict_no_overwrite", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		seedManifest(t, manPath, newMarketplaceManifest(
			map[string]manifest.Marketplace{"mp-github": mpGitHubV2}, nil,
		))

		runner := newFakeRunner()
		runner.on("plugin marketplace list --json", marketplacesJSON(map[string]manifest.Marketplace{
			"mp-github": mpGitHub,
		}), nil)

		res := execCmdWith(t, home, nil, withPluginClient(runner), "marketplace", "import", "--manifest", manPath, "--no-overwrite", "--json")
		if res.err != nil {
			t.Fatalf("unexpected error: %v\nstderr: %s", res.err, res.stderr)
		}

		got := parseJSON(t, res.stdout)
		if got["skipped"] != float64(1) {
			t.Errorf("skipped = %v, want 1", got["skipped"])
		}
		if got["overwritten"] != float64(0) {
			t.Errorf("overwritten = %v, want 0", got["overwritten"])
		}
	})

	t.Run("conflict_interactive_accept", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		seedManifest(t, manPath, newMarketplaceManifest(
			map[string]manifest.Marketplace{"mp-github": mpGitHubV2}, nil,
		))

		runner := newFakeRunner()
		runner.on("plugin marketplace list --json", marketplacesJSON(map[string]manifest.Marketplace{
			"mp-github": mpGitHub,
		}), nil)
		runner.on("plugin marketplace remove", nil, nil)
		runner.on("plugin marketplace add", nil, nil)

		// Answer "y" to overwrite prompt.
		res := execCmdWith(t, home, strings.NewReader("y\n"), withPluginClient(runner), "marketplace", "import", "--manifest", manPath, "--json")
		if res.err != nil {
			t.Fatalf("unexpected error: %v\nstderr: %s", res.err, res.stderr)
		}

		got := parseJSON(t, res.stdout)
		if got["overwritten"] != float64(1) {
			t.Errorf("overwritten = %v, want 1", got["overwritten"])
		}
	})

	t.Run("conflict_interactive_decline", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		seedManifest(t, manPath, newMarketplaceManifest(
			map[string]manifest.Marketplace{"mp-github": mpGitHubV2}, nil,
		))

		runner := newFakeRunner()
		runner.on("plugin marketplace list --json", marketplacesJSON(map[string]manifest.Marketplace{
			"mp-github": mpGitHub,
		}), nil)

		// Answer "n" to overwrite prompt.
		res := execCmdWith(t, home, strings.NewReader("n\n"), withPluginClient(runner), "marketplace", "import", "--manifest", manPath, "--json")
		if res.err != nil {
			t.Fatalf("unexpected error: %v\nstderr: %s", res.err, res.stderr)
		}

		got := parseJSON(t, res.stdout)
		if got["overwritten"] != float64(0) {
			t.Errorf("overwritten = %v, want 0", got["overwritten"])
		}
		if got["skipped"] != float64(1) {
			t.Errorf("skipped = %v, want 1", got["skipped"])
		}
	})

	t.Run("strict_removes", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		// Manifest has mp-github only.
		seedManifest(t, manPath, newMarketplaceManifest(
			map[string]manifest.Marketplace{"mp-github": mpGitHub}, nil,
		))

		// Claude has mp-github + mp-git (extra).
		runner := newFakeRunner()
		runner.on("plugin marketplace list --json", marketplacesJSON(map[string]manifest.Marketplace{
			"mp-github": mpGitHub,
			"mp-git":    mpGit,
		}), nil)
		runner.on("plugin marketplace remove", nil, nil)

		res := execCmdWith(t, home, nil, withPluginClient(runner), "marketplace", "import", "--manifest", manPath, "--strict", "--force", "--json")
		if res.err != nil {
			t.Fatalf("unexpected error: %v\nstderr: %s", res.err, res.stderr)
		}

		got := parseJSON(t, res.stdout)
		if got["removed"] != float64(1) {
			t.Errorf("removed = %v, want 1", got["removed"])
		}
		if !runner.called("plugin", "marketplace", "remove", "mp-git") {
			t.Error("expected 'plugin marketplace remove mp-git' call")
		}
		if runner.called("plugin", "marketplace", "remove", "mp-github") {
			t.Error("mp-github should NOT be removed (it's in the manifest)")
		}
	})

	t.Run("strict_removal_confirm_rejected", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		seedManifest(t, manPath, newMarketplaceManifest(
			map[string]manifest.Marketplace{"mp-github": mpGitHub}, nil,
		))

		runner := newFakeRunner()
		runner.on("plugin marketplace list --json", marketplacesJSON(map[string]manifest.Marketplace{
			"mp-github": mpGitHub,
			"mp-git":    mpGit,
		}), nil)

		// Answer "n" to strict removal confirmation.
		res := execCmdWith(t, home, strings.NewReader("n\n"), withPluginClient(runner), "marketplace", "import", "--manifest", manPath, "--strict")
		if res.err != nil {
			t.Fatalf("unexpected error: %v\nstderr: %s", res.err, res.stderr)
		}
		if !strings.Contains(res.stderr, "Aborted") {
			t.Errorf("stderr missing 'Aborted': %q", res.stderr)
		}
		if runner.called("plugin", "marketplace", "remove") {
			t.Error("no remove calls should be made after abort")
		}
	})

	t.Run("strict_with_names_rejected", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		seedManifest(t, manPath, newMarketplaceManifest(
			map[string]manifest.Marketplace{"mp-github": mpGitHub}, nil,
		))

		runner := newFakeRunner()

		res := execCmdWith(t, home, nil, withPluginClient(runner), "marketplace", "import", "--manifest", manPath, "--strict", "mp-github")
		if res.err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(res.err.Error(), "--strict cannot be used with named marketplaces") {
			t.Errorf("error %q missing expected substring", res.err.Error())
		}
	})

	t.Run("overwrite_no_overwrite_conflict", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		seedManifest(t, manPath, newMarketplaceManifest(
			map[string]manifest.Marketplace{"mp-github": mpGitHub}, nil,
		))

		runner := newFakeRunner()

		res := execCmdWith(t, home, nil, withPluginClient(runner), "marketplace", "import", "--manifest", manPath, "--overwrite", "--no-overwrite")
		if res.err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(res.err.Error(), "mutually exclusive") {
			t.Errorf("error %q missing 'mutually exclusive'", res.err.Error())
		}
	})

	t.Run("force_implies_overwrite", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		seedManifest(t, manPath, newMarketplaceManifest(
			map[string]manifest.Marketplace{"mp-github": mpGitHubV2}, nil,
		))

		runner := newFakeRunner()
		runner.on("plugin marketplace list --json", marketplacesJSON(map[string]manifest.Marketplace{
			"mp-github": mpGitHub, // differs
		}), nil)
		runner.on("plugin marketplace remove", nil, nil)
		runner.on("plugin marketplace add", nil, nil)

		res := execCmdWith(t, home, nil, withPluginClient(runner), "marketplace", "import", "--manifest", manPath, "--force", "--json")
		if res.err != nil {
			t.Fatalf("unexpected error: %v\nstderr: %s", res.err, res.stderr)
		}

		got := parseJSON(t, res.stdout)
		if got["overwritten"] != float64(1) {
			t.Errorf("overwritten = %v, want 1", got["overwritten"])
		}
	})

	t.Run("selective", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		seedManifest(t, manPath, newMarketplaceManifest(
			map[string]manifest.Marketplace{
				"mp-github": mpGitHub,
				"mp-git":    mpGit,
			}, nil,
		))

		runner := newFakeRunner()
		runner.on("plugin marketplace list --json", marketplacesJSON(nil), nil)
		runner.on("plugin marketplace add", nil, nil)

		res := execCmdWith(t, home, nil, withPluginClient(runner), "marketplace", "import", "--manifest", manPath, "--json", "mp-github")
		if res.err != nil {
			t.Fatalf("unexpected error: %v\nstderr: %s", res.err, res.stderr)
		}

		got := parseJSON(t, res.stdout)
		if got["added"] != float64(1) {
			t.Errorf("added = %v, want 1", got["added"])
		}
	})

	t.Run("selective_not_in_manifest", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		seedManifest(t, manPath, newMarketplaceManifest(nil, nil))

		runner := newFakeRunner()
		runner.on("plugin marketplace list --json", marketplacesJSON(nil), nil)

		res := execCmdWith(t, home, nil, withPluginClient(runner), "marketplace", "import", "--manifest", manPath, "nonexistent")
		if res.err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(res.err.Error(), `"nonexistent" not found in manifest`) {
			t.Errorf("error %q missing expected substring", res.err.Error())
		}
	})

	t.Run("dry_run_json", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		seedManifest(t, manPath, newMarketplaceManifest(
			map[string]manifest.Marketplace{"mp-github": mpGitHub}, nil,
		))

		runner := newFakeRunner()
		runner.on("plugin marketplace list --json", marketplacesJSON(nil), nil)

		res := execCmdWith(t, home, nil, withPluginClient(runner), "marketplace", "import", "--manifest", manPath, "--dry-run", "--json")
		if res.err != nil {
			t.Fatalf("unexpected error: %v\nstderr: %s", res.err, res.stderr)
		}

		got := parseJSON(t, res.stdout)
		add, ok := got["add"].([]any)
		if !ok {
			t.Fatalf("add is %T, want []any", got["add"])
		}
		if len(add) != 1 {
			t.Errorf("add has %d entries, want 1", len(add))
		}
		// No CLI calls should have been made.
		if runner.called("plugin", "marketplace", "add") {
			t.Error("dry-run should not call 'plugin marketplace add'")
		}
	})

	t.Run("dry_run_text", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		seedManifest(t, manPath, newMarketplaceManifest(
			map[string]manifest.Marketplace{"mp-github": mpGitHub}, nil,
		))

		runner := newFakeRunner()
		runner.on("plugin marketplace list --json", marketplacesJSON(nil), nil)

		res := execCmdWith(t, home, nil, withPluginClient(runner), "marketplace", "import", "--manifest", manPath, "--dry-run")
		if res.err != nil {
			t.Fatalf("unexpected error: %v\nstderr: %s", res.err, res.stderr)
		}
		if !strings.Contains(res.stderr, "mp-github") {
			t.Errorf("stderr missing marketplace name in dry-run: %q", res.stderr)
		}
	})

	t.Run("partial_errors", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		seedManifest(t, manPath, newMarketplaceManifest(
			map[string]manifest.Marketplace{
				"mp-github": mpGitHub,
				"mp-git":    mpGit,
			}, nil,
		))

		runner := newFakeRunner()
		runner.on("plugin marketplace list --json", marketplacesJSON(nil), nil)
		runner.onFunc("plugin marketplace add", func(args []string) ([]byte, error) {
			// Fail for mp-git's URL.
			if len(args) >= 4 && args[3] == "https://git.example.com/plugins.git" {
				return nil, errors.New("network error")
			}
			return nil, nil
		})

		res := execCmdWith(t, home, nil, withPluginClient(runner), "marketplace", "import", "--manifest", manPath, "--json")
		// JSON mode: partial errors are embedded in the body.
		if res.err != nil {
			t.Fatalf("in JSON mode errors should be in the body, not the return: %v", res.err)
		}

		got := parseJSON(t, res.stdout)
		if got["added"] != float64(1) {
			t.Errorf("added = %v, want 1", got["added"])
		}
		errSlice, ok := got["errors"].([]any)
		if !ok || len(errSlice) == 0 {
			t.Error("expected errors in JSON output")
		}
	})

	t.Run("partial_errors_text", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		seedManifest(t, manPath, newMarketplaceManifest(
			map[string]manifest.Marketplace{
				"mp-github": mpGitHub,
				"mp-git":    mpGit,
			}, nil,
		))

		runner := newFakeRunner()
		runner.on("plugin marketplace list --json", marketplacesJSON(nil), nil)
		runner.onFunc("plugin marketplace add", func(args []string) ([]byte, error) {
			if len(args) >= 4 && args[3] == "https://git.example.com/plugins.git" {
				return nil, errors.New("network error")
			}
			return nil, nil
		})

		res := execCmdWith(t, home, nil, withPluginClient(runner), "marketplace", "import", "--manifest", manPath)
		if res.err == nil {
			t.Fatal("expected error in text mode for partial failures")
		}
		if !strings.Contains(res.stderr, "1 added") {
			t.Errorf("stderr missing partial success: %q", res.stderr)
		}
	})

	t.Run("text_output", func(t *testing.T) {
		home := t.TempDir()
		manPath := filepath.Join(t.TempDir(), "manifest.json")

		seedManifest(t, manPath, newMarketplaceManifest(
			map[string]manifest.Marketplace{"mp-github": mpGitHub}, nil,
		))

		runner := newFakeRunner()
		runner.on("plugin marketplace list --json", marketplacesJSON(nil), nil)
		runner.on("plugin marketplace add", nil, nil)

		res := execCmdWith(t, home, nil, withPluginClient(runner), "marketplace", "import", "--manifest", manPath)
		if res.err != nil {
			t.Fatalf("unexpected error: %v\nstderr: %s", res.err, res.stderr)
		}
		if !strings.Contains(res.stderr, "1 added") {
			t.Errorf("stderr missing import summary: %q", res.stderr)
		}
	})
}

// --- TestMarketplaceRemove ---

func TestMarketplaceRemove(t *testing.T) {
	t.Run("simple", func(t *testing.T) {
		manPath := filepath.Join(t.TempDir(), "manifest.json")
		home := t.TempDir()

		seedManifest(t, manPath, newMarketplaceManifest(
			map[string]manifest.Marketplace{
				"mp-github": mpGitHub,
				"mp-git":    mpGit,
			}, nil,
		))

		res := execCmd(t, home, nil, "marketplace", "remove", "--manifest", manPath, "mp-github")
		if res.err != nil {
			t.Fatalf("unexpected error: %v\nstderr: %s", res.err, res.stderr)
		}

		m := loadManifestFile(t, manPath)
		if len(m.Marketplaces) != 1 {
			t.Errorf("manifest has %d marketplaces, want 1", len(m.Marketplaces))
		}
		if _, ok := m.Marketplaces["mp-github"]; ok {
			t.Error("mp-github should have been removed")
		}
	})

	t.Run("not_found", func(t *testing.T) {
		manPath := filepath.Join(t.TempDir(), "manifest.json")
		home := t.TempDir()

		seedManifest(t, manPath, newMarketplaceManifest(
			map[string]manifest.Marketplace{"mp-github": mpGitHub}, nil,
		))

		res := execCmd(t, home, nil, "marketplace", "remove", "--manifest", manPath, "nonexistent")
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

		seedManifest(t, manPath, newMarketplaceManifest(
			map[string]manifest.Marketplace{"mp-github": mpGitHub}, nil,
		))

		res := execCmd(t, home, nil, "marketplace", "remove", "--manifest", manPath, "--json", "mp-github")
		if res.err != nil {
			t.Fatalf("unexpected error: %v\nstderr: %s", res.err, res.stderr)
		}

		got := parseJSON(t, res.stdout)
		if got["removed"] != "mp-github" {
			t.Errorf("removed = %v, want mp-github", got["removed"])
		}
	})

	t.Run("cascade_plugins_and_profiles", func(t *testing.T) {
		manPath := filepath.Join(t.TempDir(), "manifest.json")
		home := t.TempDir()

		m := newMarketplaceManifest(
			map[string]manifest.Marketplace{"my-marketplace": marketplaceCustom},
			[]manifest.Plugin{
				pluginA,
				pluginC, // plugin-c@my-marketplace
			},
		)
		m.Profiles = map[string]manifest.Profile{
			"work": {
				MCPServers: []string{},
				Plugins:    []manifest.ProfilePlugin{{ID: "plugin-a"}, {ID: "plugin-c@my-marketplace"}},
			},
			"home": {
				MCPServers: []string{},
				Plugins:    []manifest.ProfilePlugin{{ID: "plugin-c@my-marketplace"}},
			},
		}
		seedManifest(t, manPath, m)

		// Answer "y" to cascade confirmation (or use --force).
		res := execCmd(t, home, nil, "marketplace", "remove", "--manifest", manPath, "--force", "--json", "my-marketplace")
		if res.err != nil {
			t.Fatalf("unexpected error: %v\nstderr: %s", res.err, res.stderr)
		}

		got := parseJSON(t, res.stdout)
		if got["removed"] != "my-marketplace" {
			t.Errorf("removed = %v, want my-marketplace", got["removed"])
		}
		removedPlugins, ok := got["removedPlugins"].([]any)
		if !ok || len(removedPlugins) != 1 {
			t.Errorf("removedPlugins = %v, want [plugin-c@my-marketplace]", got["removedPlugins"])
		}

		loaded := loadManifestFile(t, manPath)
		// Marketplace should be gone.
		if _, ok := loaded.Marketplaces["my-marketplace"]; ok {
			t.Error("marketplace should have been removed")
		}
		// plugin-c should be gone, plugin-a should remain.
		if len(loaded.Plugins) != 1 || loaded.Plugins[0].ID != "plugin-a" {
			t.Errorf("plugins = %+v, want [plugin-a]", loaded.Plugins)
		}
		// Profile references should be cleaned.
		workProfile := loaded.Profiles["work"]
		if len(workProfile.Plugins) != 1 || workProfile.Plugins[0].ID != "plugin-a" {
			t.Errorf("work profile plugins = %v, want [plugin-a]", workProfile.Plugins)
		}
		homeProfile := loaded.Profiles["home"]
		if len(homeProfile.Plugins) != 0 {
			t.Errorf("home profile plugins = %v, want []", homeProfile.Plugins)
		}
	})

	t.Run("cascade_declined", func(t *testing.T) {
		manPath := filepath.Join(t.TempDir(), "manifest.json")
		home := t.TempDir()

		m := newMarketplaceManifest(
			map[string]manifest.Marketplace{"my-marketplace": marketplaceCustom},
			[]manifest.Plugin{pluginC},
		)
		seedManifest(t, manPath, m)

		// Answer "n" to cascade confirmation.
		res := execCmd(t, home, strings.NewReader("n\n"), "marketplace", "remove", "--manifest", manPath, "my-marketplace")
		if res.err != nil {
			t.Fatalf("unexpected error: %v\nstderr: %s", res.err, res.stderr)
		}
		if !strings.Contains(res.stderr, "Aborted") {
			t.Errorf("stderr missing 'Aborted': %q", res.stderr)
		}

		// Everything should still be present.
		loaded := loadManifestFile(t, manPath)
		if _, ok := loaded.Marketplaces["my-marketplace"]; !ok {
			t.Error("marketplace should still be present after decline")
		}
		if len(loaded.Plugins) != 1 {
			t.Errorf("plugins should still be present: %+v", loaded.Plugins)
		}
	})

	t.Run("no_associated_plugins", func(t *testing.T) {
		manPath := filepath.Join(t.TempDir(), "manifest.json")
		home := t.TempDir()

		// Marketplace has no associated plugins — no confirmation needed.
		m := newMarketplaceManifest(
			map[string]manifest.Marketplace{"my-marketplace": marketplaceCustom},
			[]manifest.Plugin{pluginA}, // no @my-marketplace plugin
		)
		seedManifest(t, manPath, m)

		res := execCmd(t, home, nil, "marketplace", "remove", "--manifest", manPath, "my-marketplace")
		if res.err != nil {
			t.Fatalf("unexpected error: %v\nstderr: %s", res.err, res.stderr)
		}

		loaded := loadManifestFile(t, manPath)
		if _, ok := loaded.Marketplaces["my-marketplace"]; ok {
			t.Error("marketplace should have been removed")
		}
		if len(loaded.Plugins) != 1 {
			t.Errorf("plugin-a should remain: %+v", loaded.Plugins)
		}
	})

	t.Run("text_output_with_cascade", func(t *testing.T) {
		manPath := filepath.Join(t.TempDir(), "manifest.json")
		home := t.TempDir()

		m := newMarketplaceManifest(
			map[string]manifest.Marketplace{"my-marketplace": marketplaceCustom},
			[]manifest.Plugin{pluginC},
		)
		seedManifest(t, manPath, m)

		res := execCmd(t, home, nil, "marketplace", "remove", "--manifest", manPath, "--force", "my-marketplace")
		if res.err != nil {
			t.Fatalf("unexpected error: %v\nstderr: %s", res.err, res.stderr)
		}
		if !strings.Contains(res.stderr, "Removed") {
			t.Errorf("stderr missing success message: %q", res.stderr)
		}
	})
}

// --- TestMarketplaceList ---

func TestMarketplaceList(t *testing.T) {
	t.Run("manifest_table", func(t *testing.T) {
		manPath := filepath.Join(t.TempDir(), "manifest.json")
		home := t.TempDir()

		seedManifest(t, manPath, newMarketplaceManifest(
			map[string]manifest.Marketplace{
				"mp-github": mpGitHub,
				"mp-git":    mpGit,
			}, nil,
		))

		res := execCmd(t, home, nil, "marketplace", "list", "--manifest", manPath)
		if res.err != nil {
			t.Fatalf("unexpected error: %v\nstderr: %s", res.err, res.stderr)
		}
		if !strings.Contains(res.stdout, "mp-github") {
			t.Errorf("stdout missing mp-github: %q", res.stdout)
		}
		if !strings.Contains(res.stdout, "mp-git") {
			t.Errorf("stdout missing mp-git: %q", res.stdout)
		}
	})

	t.Run("manifest_json", func(t *testing.T) {
		manPath := filepath.Join(t.TempDir(), "manifest.json")
		home := t.TempDir()

		seedManifest(t, manPath, newMarketplaceManifest(
			map[string]manifest.Marketplace{"mp-github": mpGitHub}, nil,
		))

		res := execCmd(t, home, nil, "marketplace", "list", "--manifest", manPath, "--json")
		if res.err != nil {
			t.Fatalf("unexpected error: %v\nstderr: %s", res.err, res.stderr)
		}

		var mps map[string]manifest.Marketplace
		if err := json.Unmarshal([]byte(res.stdout), &mps); err != nil {
			t.Fatalf("failed to parse JSON: %v\nstdout: %s", err, res.stdout)
		}
		if len(mps) != 1 {
			t.Errorf("got %d marketplaces, want 1", len(mps))
		}
		if _, ok := mps["mp-github"]; !ok {
			t.Error("missing mp-github in JSON output")
		}
	})

	t.Run("manifest_empty", func(t *testing.T) {
		manPath := filepath.Join(t.TempDir(), "manifest.json")
		home := t.TempDir()

		seedManifest(t, manPath, newMarketplaceManifest(nil, nil))

		res := execCmd(t, home, nil, "marketplace", "list", "--manifest", manPath)
		if res.err != nil {
			t.Fatalf("unexpected error: %v\nstderr: %s", res.err, res.stderr)
		}
		if !strings.Contains(res.stderr, "No marketplaces in manifest") {
			t.Errorf("stderr missing empty message: %q", res.stderr)
		}
	})

	t.Run("installed_table", func(t *testing.T) {
		home := t.TempDir()

		runner := newFakeRunner()
		runner.on("plugin marketplace list --json", marketplacesJSON(map[string]manifest.Marketplace{
			"mp-github": mpGitHub,
		}), nil)

		res := execCmdWith(t, home, nil, withPluginClient(runner), "marketplace", "list", "--installed")
		if res.err != nil {
			t.Fatalf("unexpected error: %v\nstderr: %s", res.err, res.stderr)
		}
		if !strings.Contains(res.stdout, "mp-github") {
			t.Errorf("stdout missing mp-github: %q", res.stdout)
		}
	})

	t.Run("installed_json", func(t *testing.T) {
		home := t.TempDir()

		runner := newFakeRunner()
		runner.on("plugin marketplace list --json", marketplacesJSON(map[string]manifest.Marketplace{
			"mp-github": mpGitHub,
		}), nil)

		res := execCmdWith(t, home, nil, withPluginClient(runner), "marketplace", "list", "--installed", "--json")
		if res.err != nil {
			t.Fatalf("unexpected error: %v\nstderr: %s", res.err, res.stderr)
		}

		var mps map[string]manifest.Marketplace
		if err := json.Unmarshal([]byte(res.stdout), &mps); err != nil {
			t.Fatalf("failed to parse JSON: %v\nstdout: %s", err, res.stdout)
		}
		if len(mps) != 1 || mps["mp-github"].Repo != "user/my-marketplace" {
			t.Errorf("unexpected JSON output: %+v", mps)
		}
	})

	t.Run("installed_empty", func(t *testing.T) {
		home := t.TempDir()

		runner := newFakeRunner()
		runner.on("plugin marketplace list --json", marketplacesJSON(nil), nil)

		res := execCmdWith(t, home, nil, withPluginClient(runner), "marketplace", "list", "--installed")
		if res.err != nil {
			t.Fatalf("unexpected error: %v\nstderr: %s", res.err, res.stderr)
		}
		if !strings.Contains(res.stderr, "No marketplaces in Claude Code") {
			t.Errorf("stderr missing empty message: %q", res.stderr)
		}
	})
}
