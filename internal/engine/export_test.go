package engine

import (
	"path/filepath"
	"testing"

	"github.com/alphaleonis/cctote/internal/manifest"
)

// --- ExportMCPServers ---

func TestExportMCPServers_AddAndUpdate(t *testing.T) {
	dir := t.TempDir()
	manPath := filepath.Join(dir, "manifest.json")

	// Seed with one existing server.
	m := &manifest.Manifest{
		Version:    1,
		MCPServers: map[string]manifest.MCPServer{"existing": {Command: "old"}},
	}
	if err := manifest.Save(manPath, m); err != nil {
		t.Fatal(err)
	}

	hooks := &mockHooks{cascadeOK: true}
	servers := map[string]manifest.MCPServer{
		"existing": {Command: "new"},   // update
		"fresh":    {Command: "fresh"}, // add
	}
	result, err := ExportMCPServers(manPath, servers, hooks)
	if err != nil {
		t.Fatal(err)
	}

	if result.Count(ActionAdded) != 1 {
		t.Errorf("added = %d, want 1", result.Count(ActionAdded))
	}
	if result.Count(ActionUpdated) != 1 {
		t.Errorf("updated = %d, want 1", result.Count(ActionUpdated))
	}

	loaded, err := manifest.Load(manPath)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.MCPServers["existing"].Command != "new" {
		t.Error("existing server should be updated")
	}
	if _, ok := loaded.MCPServers["fresh"]; !ok {
		t.Error("fresh server should be added")
	}
}

func TestExportMCPServers_CreatesManifest(t *testing.T) {
	dir := t.TempDir()
	manPath := filepath.Join(dir, "manifest.json")

	hooks := &mockHooks{cascadeOK: true}
	servers := map[string]manifest.MCPServer{"srv": {Command: "cmd"}}

	result, err := ExportMCPServers(manPath, servers, hooks)
	if err != nil {
		t.Fatal(err)
	}
	if result.Count(ActionAdded) != 1 {
		t.Errorf("added = %d, want 1", result.Count(ActionAdded))
	}

	// Should have reported manifest creation.
	if len(hooks.infoCalls) != 1 {
		t.Errorf("infoCalls = %d, want 1 (manifest creation)", len(hooks.infoCalls))
	}
}

// --- ExportPlugins ---

func TestExportPlugins_Simple(t *testing.T) {
	dir := t.TempDir()
	manPath := filepath.Join(dir, "manifest.json")

	m := &manifest.Manifest{Version: 1}
	if err := manifest.Save(manPath, m); err != nil {
		t.Fatal(err)
	}

	hooks := &mockHooks{cascadeOK: true}
	plugins := []manifest.Plugin{
		{ID: "standalone-plugin", Scope: "global", Enabled: true},
	}
	result, err := ExportPlugins(manPath, plugins, nil, hooks)
	if err != nil {
		t.Fatal(err)
	}

	if result.Count(ActionAdded) != 1 {
		t.Errorf("added = %d, want 1", result.Count(ActionAdded))
	}

	loaded, err := manifest.Load(manPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Plugins) != 1 || loaded.Plugins[0].ID != "standalone-plugin" {
		t.Errorf("plugins = %v, want [standalone-plugin]", loaded.Plugins)
	}
}

func TestExportPlugins_MarketplaceAutoExport(t *testing.T) {
	dir := t.TempDir()
	manPath := filepath.Join(dir, "manifest.json")

	m := &manifest.Manifest{Version: 1}
	if err := manifest.Save(manPath, m); err != nil {
		t.Fatal(err)
	}

	hooks := &mockHooks{cascadeOK: true}
	plugins := []manifest.Plugin{
		{ID: "plug-a@my-mkt", Scope: "global", Enabled: true},
		{ID: "plug-b@my-mkt", Scope: "global", Enabled: true},
		{ID: "standalone", Scope: "project", Enabled: false},
	}
	claudeMkts := map[string]manifest.Marketplace{
		"my-mkt": {Source: "github", Repo: "owner/repo"},
	}

	result, err := ExportPlugins(manPath, plugins, claudeMkts, hooks)
	if err != nil {
		t.Fatal(err)
	}

	// 3 plugins added + 1 marketplace cascaded.
	if result.Count(ActionAdded) != 3 {
		t.Errorf("added = %d, want 3", result.Count(ActionAdded))
	}
	if result.Count(ActionCascaded) != 1 {
		t.Errorf("cascaded = %d, want 1 (marketplace)", result.Count(ActionCascaded))
	}

	// OnCascade called once for the marketplace.
	if len(hooks.cascadeCalls) != 1 {
		t.Fatalf("cascadeCalls = %d, want 1", len(hooks.cascadeCalls))
	}
	if hooks.cascadeCalls[0].item != "my-mkt" {
		t.Errorf("cascade item = %q, want %q", hooks.cascadeCalls[0].item, "my-mkt")
	}
	if len(hooks.cascadeCalls[0].dependents) != 2 {
		t.Errorf("cascade dependents = %v, want 2 plugins", hooks.cascadeCalls[0].dependents)
	}

	loaded, err := manifest.Load(manPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Plugins) != 3 {
		t.Errorf("plugins count = %d, want 3", len(loaded.Plugins))
	}
	if _, ok := loaded.Marketplaces["my-mkt"]; !ok {
		t.Error("marketplace should be auto-exported")
	}
}

func TestExportPlugins_MarketplaceDeclined(t *testing.T) {
	dir := t.TempDir()
	manPath := filepath.Join(dir, "manifest.json")

	m := &manifest.Manifest{Version: 1}
	if err := manifest.Save(manPath, m); err != nil {
		t.Fatal(err)
	}

	hooks := &mockHooks{cascadeOK: false} // decline marketplace
	plugins := []manifest.Plugin{
		{ID: "plug-a@my-mkt", Scope: "global", Enabled: true},
		{ID: "standalone", Scope: "project", Enabled: false},
	}
	claudeMkts := map[string]manifest.Marketplace{
		"my-mkt": {Source: "github", Repo: "owner/repo"},
	}

	result, err := ExportPlugins(manPath, plugins, claudeMkts, hooks)
	if err != nil {
		t.Fatal(err)
	}

	// Only standalone should be added; plug-a skipped because marketplace declined.
	if result.Count(ActionAdded) != 1 {
		t.Errorf("added = %d, want 1", result.Count(ActionAdded))
	}

	loaded, err := manifest.Load(manPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Plugins) != 1 || loaded.Plugins[0].ID != "standalone" {
		t.Errorf("plugins = %v, want [standalone]", loaded.Plugins)
	}
	if _, ok := loaded.Marketplaces["my-mkt"]; ok {
		t.Error("marketplace should not be exported when declined")
	}
}

func TestExportPlugins_MarketplaceUnavailable(t *testing.T) {
	dir := t.TempDir()
	manPath := filepath.Join(dir, "manifest.json")

	m := &manifest.Manifest{Version: 1}
	if err := manifest.Save(manPath, m); err != nil {
		t.Fatal(err)
	}

	hooks := &mockHooks{cascadeOK: true}
	plugins := []manifest.Plugin{
		{ID: "plug-a@missing-mkt", Scope: "global", Enabled: true},
		{ID: "standalone", Scope: "project", Enabled: false},
	}
	// missing-mkt not in claudeMarketplaces
	claudeMkts := map[string]manifest.Marketplace{}

	result, err := ExportPlugins(manPath, plugins, claudeMkts, hooks)
	if err != nil {
		t.Fatal(err)
	}

	// Only standalone exported; plug-a skipped.
	if result.Count(ActionAdded) != 1 {
		t.Errorf("added = %d, want 1", result.Count(ActionAdded))
	}

	// OnInfo should have warned about the skipped plugin.
	if len(hooks.infoCalls) == 0 {
		t.Error("expected OnInfo call about skipped plugin")
	}
}

func TestExportPlugins_MarketplaceAlreadyInManifest(t *testing.T) {
	dir := t.TempDir()
	manPath := filepath.Join(dir, "manifest.json")

	m := &manifest.Manifest{
		Version: 1,
		Marketplaces: map[string]manifest.Marketplace{
			"my-mkt": {Source: "github", Repo: "owner/repo"},
		},
	}
	if err := manifest.Save(manPath, m); err != nil {
		t.Fatal(err)
	}

	hooks := &mockHooks{cascadeOK: true}
	plugins := []manifest.Plugin{
		{ID: "plug-a@my-mkt", Scope: "global", Enabled: true},
	}

	result, err := ExportPlugins(manPath, plugins, nil, hooks)
	if err != nil {
		t.Fatal(err)
	}

	// Plugin exported normally; no cascade needed (marketplace already in manifest).
	if result.Count(ActionAdded) != 1 {
		t.Errorf("added = %d, want 1", result.Count(ActionAdded))
	}
	if len(hooks.cascadeCalls) != 0 {
		t.Error("no cascade expected when marketplace already in manifest")
	}
}

func TestExportPlugins_UpsertExisting(t *testing.T) {
	dir := t.TempDir()
	manPath := filepath.Join(dir, "manifest.json")

	m := &manifest.Manifest{
		Version: 1,
		Plugins: []manifest.Plugin{
			{ID: "plug-a", Scope: "global", Enabled: true},
		},
	}
	if err := manifest.Save(manPath, m); err != nil {
		t.Fatal(err)
	}

	hooks := &mockHooks{cascadeOK: true}
	plugins := []manifest.Plugin{
		{ID: "plug-a", Scope: "global", Enabled: false}, // update enabled
	}

	result, err := ExportPlugins(manPath, plugins, nil, hooks)
	if err != nil {
		t.Fatal(err)
	}

	if result.Count(ActionUpdated) != 1 {
		t.Errorf("updated = %d, want 1", result.Count(ActionUpdated))
	}

	loaded, err := manifest.Load(manPath)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Plugins[0].Enabled {
		t.Error("plugin should be updated to Enabled=false")
	}
}

// --- ExportMarketplaces ---

// --- ApplyBulkExportToManifest ---

func TestApplyBulkExportToManifest_UpsertsAll(t *testing.T) {
	dir := t.TempDir()
	manPath := filepath.Join(dir, "manifest.json")

	// Seed with one existing server and one existing plugin.
	m := &manifest.Manifest{
		Version:    1,
		MCPServers: map[string]manifest.MCPServer{"existing-srv": {Command: "old"}},
		Plugins:    []manifest.Plugin{{ID: "existing-plug", Scope: "global", Enabled: false}},
	}
	if err := manifest.Save(manPath, m); err != nil {
		t.Fatal(err)
	}

	hooks := &mockHooks{cascadeOK: true}
	mcpDesired := map[string]manifest.MCPServer{
		"existing-srv": {Command: "new"},   // update
		"fresh-srv":    {Command: "fresh"}, // add
		"ignored-srv":  {Command: "nope"},  // not in add/overwrite lists
	}
	plugDesired := []manifest.Plugin{
		{ID: "existing-plug", Scope: "global", Enabled: true}, // update
		{ID: "fresh-plug", Scope: "project", Enabled: true},   // add
		{ID: "ignored-plug", Scope: "global", Enabled: true},  // not in add/overwrite lists
	}

	err := ApplyBulkExportToManifest(manPath, BulkExportInput{
		MCPDesired:    mcpDesired,
		MCPAdd:        []string{"fresh-srv"},
		MCPOverwrite:  []string{"existing-srv"},
		PlugDesired:   plugDesired,
		PlugAdd:       []string{"fresh-plug"},
		PlugOverwrite: []string{"existing-plug"},
	}, nil, hooks)
	if err != nil {
		t.Fatal(err)
	}

	loaded, err := manifest.Load(manPath)
	if err != nil {
		t.Fatal(err)
	}

	// MCP: existing-srv updated, fresh-srv added, ignored-srv absent.
	if loaded.MCPServers["existing-srv"].Command != "new" {
		t.Errorf("existing-srv.Command = %q, want %q", loaded.MCPServers["existing-srv"].Command, "new")
	}
	if _, ok := loaded.MCPServers["fresh-srv"]; !ok {
		t.Error("fresh-srv should be added")
	}
	if _, ok := loaded.MCPServers["ignored-srv"]; ok {
		t.Error("ignored-srv should NOT be exported (not in add/overwrite lists)")
	}

	// Plugins: existing-plug updated, fresh-plug added, ignored-plug absent.
	idx := manifest.FindPlugin(loaded.Plugins, "existing-plug")
	if idx < 0 {
		t.Fatal("existing-plug should still be in manifest")
	}
	if !loaded.Plugins[idx].Enabled {
		t.Error("existing-plug should be updated to Enabled=true")
	}
	if manifest.FindPlugin(loaded.Plugins, "fresh-plug") < 0 {
		t.Error("fresh-plug should be added")
	}
	if manifest.FindPlugin(loaded.Plugins, "ignored-plug") >= 0 {
		t.Error("ignored-plug should NOT be exported (not in add/overwrite lists)")
	}
}

func TestApplyBulkExportToManifest_CreatesManifest(t *testing.T) {
	dir := t.TempDir()
	manPath := filepath.Join(dir, "manifest.json")

	hooks := &mockHooks{cascadeOK: true}
	mcpDesired := map[string]manifest.MCPServer{"srv": {Command: "cmd"}}
	plugDesired := []manifest.Plugin{{ID: "plug", Scope: "global", Enabled: true}}

	err := ApplyBulkExportToManifest(manPath, BulkExportInput{
		MCPDesired:  mcpDesired,
		MCPAdd:      []string{"srv"},
		PlugDesired: plugDesired,
		PlugAdd:     []string{"plug"},
	}, nil, hooks)
	if err != nil {
		t.Fatal(err)
	}

	loaded, err := manifest.Load(manPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := loaded.MCPServers["srv"]; !ok {
		t.Error("srv should be added")
	}
	if manifest.FindPlugin(loaded.Plugins, "plug") < 0 {
		t.Error("plug should be added")
	}

	// Should have reported manifest creation.
	if len(hooks.infoCalls) != 1 {
		t.Errorf("infoCalls = %d, want 1 (manifest creation)", len(hooks.infoCalls))
	}
}

func TestApplyBulkExportToManifest_EmptyLists(t *testing.T) {
	dir := t.TempDir()
	manPath := filepath.Join(dir, "manifest.json")

	m := &manifest.Manifest{
		Version:    1,
		MCPServers: map[string]manifest.MCPServer{"keep": {Command: "keep-cmd"}},
		Plugins:    []manifest.Plugin{{ID: "keep-plug", Scope: "global", Enabled: true}},
	}
	if err := manifest.Save(manPath, m); err != nil {
		t.Fatal(err)
	}

	hooks := &mockHooks{cascadeOK: true}
	// Desired has items but add/overwrite lists are empty — nothing should change.
	err := ApplyBulkExportToManifest(manPath, BulkExportInput{
		MCPDesired:  map[string]manifest.MCPServer{"new": {Command: "new"}},
		PlugDesired: []manifest.Plugin{{ID: "new-plug", Scope: "global", Enabled: true}},
	}, nil, hooks)
	if err != nil {
		t.Fatal(err)
	}

	loaded, err := manifest.Load(manPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := loaded.MCPServers["new"]; ok {
		t.Error("'new' should not be exported when add/overwrite lists are empty")
	}
	if manifest.FindPlugin(loaded.Plugins, "new-plug") >= 0 {
		t.Error("'new-plug' should not be exported when add/overwrite lists are empty")
	}
}

func TestApplyBulkExportToManifest_AutoExportsMarketplace(t *testing.T) {
	dir := t.TempDir()
	manPath := filepath.Join(dir, "manifest.json")

	m := &manifest.Manifest{
		Version:    1,
		MCPServers: map[string]manifest.MCPServer{},
	}
	if err := manifest.Save(manPath, m); err != nil {
		t.Fatal(err)
	}

	hooks := &mockHooks{cascadeOK: true}
	// Plugin with marketplace dependency: "plug-a@my-mkt".
	plugDesired := []manifest.Plugin{
		{ID: "plug-a@my-mkt", Scope: "global", Enabled: true},
		{ID: "plain-plug", Scope: "global", Enabled: true},
	}
	claudeMkts := map[string]manifest.Marketplace{
		"my-mkt": {Source: "github", Repo: "org/mkt"},
	}

	err := ApplyBulkExportToManifest(manPath, BulkExportInput{
		PlugDesired: plugDesired,
		PlugAdd:     []string{"plug-a@my-mkt", "plain-plug"},
	}, claudeMkts, hooks)
	if err != nil {
		t.Fatal(err)
	}

	loaded, err := manifest.Load(manPath)
	if err != nil {
		t.Fatal(err)
	}

	// Marketplace should have been auto-exported.
	if _, ok := loaded.Marketplaces["my-mkt"]; !ok {
		t.Error("marketplace 'my-mkt' should be auto-exported with its dependent plugin")
	}
	// Both plugins should be present.
	if manifest.FindPlugin(loaded.Plugins, "plug-a@my-mkt") < 0 {
		t.Error("plug-a@my-mkt should be added")
	}
	if manifest.FindPlugin(loaded.Plugins, "plain-plug") < 0 {
		t.Error("plain-plug should be added")
	}
}

func TestExportMarketplaces_AddAndUpdate(t *testing.T) {
	dir := t.TempDir()
	manPath := filepath.Join(dir, "manifest.json")

	m := &manifest.Manifest{
		Version: 1,
		Marketplaces: map[string]manifest.Marketplace{
			"existing": {Source: "github", Repo: "old/repo"},
		},
	}
	if err := manifest.Save(manPath, m); err != nil {
		t.Fatal(err)
	}

	hooks := &mockHooks{cascadeOK: true}
	mkts := map[string]manifest.Marketplace{
		"existing": {Source: "github", Repo: "new/repo"},
		"fresh":    {Source: "git", URL: "https://example.com"},
	}

	result, err := ExportMarketplaces(manPath, mkts, hooks)
	if err != nil {
		t.Fatal(err)
	}

	if result.Count(ActionAdded) != 1 {
		t.Errorf("added = %d, want 1", result.Count(ActionAdded))
	}
	if result.Count(ActionUpdated) != 1 {
		t.Errorf("updated = %d, want 1", result.Count(ActionUpdated))
	}

	loaded, err := manifest.Load(manPath)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Marketplaces["existing"].Repo != "new/repo" {
		t.Error("existing marketplace should be updated")
	}
	if _, ok := loaded.Marketplaces["fresh"]; !ok {
		t.Error("fresh marketplace should be added")
	}
}
