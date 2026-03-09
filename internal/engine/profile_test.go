package engine

import (
	"path/filepath"
	"sort"
	"testing"

	"github.com/alphaleonis/cctote/internal/manifest"
)

// --- ResolveProfile ---

func TestResolveProfile_ResolvesCorrectly(t *testing.T) {
	m := &manifest.Manifest{
		Version: 1,
		MCPServers: map[string]manifest.MCPServer{
			"srv-a": {Command: "a"},
			"srv-b": {Command: "b"},
		},
		Plugins: []manifest.Plugin{
			{ID: "plug-a", Scope: "global", Enabled: true},
			{ID: "plug-b", Scope: "project", Enabled: false},
		},
		Profiles: map[string]manifest.Profile{
			"test-profile": {
				MCPServers: []string{"srv-a"},
				Plugins:    []manifest.ProfilePlugin{{ID: "plug-b"}},
			},
		},
	}

	mcpServers, plugins, err := ResolveProfile(m, "test-profile")
	if err != nil {
		t.Fatal(err)
	}

	if len(mcpServers) != 1 {
		t.Errorf("mcpServers count = %d, want 1", len(mcpServers))
	}
	if _, ok := mcpServers["srv-a"]; !ok {
		t.Error("expected srv-a in resolved MCP servers")
	}

	if len(plugins) != 1 {
		t.Errorf("plugins count = %d, want 1", len(plugins))
	}
	if plugins[0].ID != "plug-b" {
		t.Errorf("plugin ID = %q, want %q", plugins[0].ID, "plug-b")
	}
	// Enabled should be inherited from manifest (plug-b: Enabled=false).
	if plugins[0].Enabled != false {
		t.Errorf("plugin Enabled = %v, want false (inherited from manifest)", plugins[0].Enabled)
	}
}

func TestResolveProfile_ErrorOnMissingProfile(t *testing.T) {
	m := &manifest.Manifest{Version: 1}

	_, _, err := ResolveProfile(m, "nonexistent")
	if err == nil {
		t.Fatal("expected error for missing profile")
	}
}

func TestResolveProfile_ErrorOnMissingMCPServer(t *testing.T) {
	m := &manifest.Manifest{
		Version:    1,
		MCPServers: map[string]manifest.MCPServer{},
		Profiles: map[string]manifest.Profile{
			"test": {MCPServers: []string{"missing-srv"}},
		},
	}

	_, _, err := ResolveProfile(m, "test")
	if err == nil {
		t.Fatal("expected error for missing MCP server reference")
	}
}

func TestResolveProfile_ErrorOnMissingPlugin(t *testing.T) {
	m := &manifest.Manifest{
		Version: 1,
		Profiles: map[string]manifest.Profile{
			"test": {Plugins: []manifest.ProfilePlugin{{ID: "missing-plug"}}},
		},
	}

	_, _, err := ResolveProfile(m, "test")
	if err == nil {
		t.Fatal("expected error for missing plugin reference")
	}
}

// --- ResolveProfileLenient ---

func TestResolveProfileLenient_SkipsMissing(t *testing.T) {
	m := &manifest.Manifest{
		Version: 1,
		MCPServers: map[string]manifest.MCPServer{
			"srv-a": {Command: "a"},
		},
		Plugins: []manifest.Plugin{
			{ID: "plug-a", Scope: "global", Enabled: true},
		},
		Profiles: map[string]manifest.Profile{
			"test": {
				MCPServers: []string{"srv-a", "missing-srv"},
				Plugins:    []manifest.ProfilePlugin{{ID: "plug-a"}, {ID: "missing-plug"}},
			},
		},
	}

	mcpServers, plugins, ok := ResolveProfileLenient(m, "test")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if len(mcpServers) != 1 {
		t.Errorf("mcpServers = %d, want 1 (missing-srv skipped)", len(mcpServers))
	}
	if len(plugins) != 1 {
		t.Errorf("plugins = %d, want 1 (missing-plug skipped)", len(plugins))
	}
}

func TestResolveProfile_AppliesEnabledOverride(t *testing.T) {
	enabled := true
	disabled := false
	m := &manifest.Manifest{
		Version: 1,
		Plugins: []manifest.Plugin{
			{ID: "plug-a", Scope: "global", Enabled: true},
			{ID: "plug-b", Scope: "global", Enabled: false},
			{ID: "plug-c", Scope: "global", Enabled: true},
		},
		Profiles: map[string]manifest.Profile{
			"test": {
				Plugins: []manifest.ProfilePlugin{
					{ID: "plug-a", Enabled: &disabled}, // override: true → false
					{ID: "plug-b", Enabled: &enabled},  // override: false → true
					{ID: "plug-c"},                     // inherit: true
				},
			},
		},
	}

	_, plugins, err := ResolveProfile(m, "test")
	if err != nil {
		t.Fatal(err)
	}
	if len(plugins) != 3 {
		t.Fatalf("plugins count = %d, want 3", len(plugins))
	}
	if plugins[0].ID != "plug-a" || plugins[0].Enabled != false {
		t.Errorf("plug-a: enabled = %v, want false (overridden)", plugins[0].Enabled)
	}
	if plugins[1].ID != "plug-b" || plugins[1].Enabled != true {
		t.Errorf("plug-b: enabled = %v, want true (overridden)", plugins[1].Enabled)
	}
	if plugins[2].ID != "plug-c" || plugins[2].Enabled != true {
		t.Errorf("plug-c: enabled = %v, want true (inherited)", plugins[2].Enabled)
	}

	// Verify original manifest plugins are not mutated by the override.
	if m.Plugins[0].Enabled != true {
		t.Errorf("manifest plug-a mutated: Enabled = %v, want true (original)", m.Plugins[0].Enabled)
	}
	if m.Plugins[1].Enabled != false {
		t.Errorf("manifest plug-b mutated: Enabled = %v, want false (original)", m.Plugins[1].Enabled)
	}
}

func TestResolveProfileLenient_AppliesEnabledOverride(t *testing.T) {
	disabled := false
	m := &manifest.Manifest{
		Version: 1,
		Plugins: []manifest.Plugin{
			{ID: "plug-a", Scope: "global", Enabled: true},
		},
		Profiles: map[string]manifest.Profile{
			"test": {
				Plugins: []manifest.ProfilePlugin{
					{ID: "plug-a", Enabled: &disabled},
				},
			},
		},
	}

	_, plugins, ok := ResolveProfileLenient(m, "test")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if len(plugins) != 1 {
		t.Fatalf("plugins = %d, want 1", len(plugins))
	}
	if plugins[0].Enabled != false {
		t.Errorf("plug-a: enabled = %v, want false (overridden)", plugins[0].Enabled)
	}
}

func TestResolveProfileLenient_MissingProfile(t *testing.T) {
	m := &manifest.Manifest{Version: 1}
	_, _, ok := ResolveProfileLenient(m, "nonexistent")
	if ok {
		t.Fatal("expected ok=false for missing profile")
	}
}

// --- CheckPluginMarketplacePrereqs ---

func TestCheckPluginMarketplacePrereqs_MissingMarketplace(t *testing.T) {
	pluginsToAdd := []string{"plug-a@my-mkt", "standalone"}
	installed := map[string]manifest.Marketplace{}

	err := CheckPluginMarketplacePrereqs(pluginsToAdd, installed)
	if err == nil {
		t.Fatal("expected error for missing marketplace prerequisite")
	}
}

func TestCheckPluginMarketplacePrereqs_AllAvailable(t *testing.T) {
	pluginsToAdd := []string{"plug-a@my-mkt", "standalone"}
	installed := map[string]manifest.Marketplace{
		"my-mkt": {Source: "github", Repo: "owner/repo"},
	}

	err := CheckPluginMarketplacePrereqs(pluginsToAdd, installed)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCheckPluginMarketplacePrereqs_NilMarketplaces(t *testing.T) {
	pluginsToAdd := []string{"plug-a@my-mkt"}

	err := CheckPluginMarketplacePrereqs(pluginsToAdd, nil)
	if err == nil {
		t.Fatal("expected error when installedMarketplaces is nil")
	}
}

func TestCheckPluginMarketplacePrereqs_NoMarketplacePlugins(t *testing.T) {
	pluginsToAdd := []string{"standalone-a", "standalone-b"}

	err := CheckPluginMarketplacePrereqs(pluginsToAdd, nil)
	if err != nil {
		t.Fatalf("unexpected error for plugins without marketplace deps: %v", err)
	}
}

// --- SnapshotProfile ---

func TestSnapshotProfile_Basic(t *testing.T) {
	dir := t.TempDir()
	manPath := filepath.Join(dir, "manifest.json")

	m := &manifest.Manifest{Version: 1}
	if err := manifest.Save(manPath, m); err != nil {
		t.Fatal(err)
	}

	hooks := &mockHooks{cascadeOK: true}
	installed := map[string]manifest.MCPServer{
		"srv-a": {Command: "a"},
	}
	plugins := []manifest.Plugin{
		{ID: "plug-a", Scope: "global", Enabled: true},
	}

	result, err := SnapshotProfile(manPath, "test", false, installed, plugins, nil, hooks)
	if err != nil {
		t.Fatal(err)
	}

	if result.Count(ActionAdded) != 2 { // 1 MCP + 1 plugin
		t.Errorf("added = %d, want 2", result.Count(ActionAdded))
	}

	loaded, err := manifest.Load(manPath)
	if err != nil {
		t.Fatal(err)
	}

	profile, ok := loaded.Profiles["test"]
	if !ok {
		t.Fatal("profile should be created")
	}
	if len(profile.MCPServers) != 1 || profile.MCPServers[0] != "srv-a" {
		t.Errorf("MCPServers = %v, want [srv-a]", profile.MCPServers)
	}
	if len(profile.Plugins) != 1 || profile.Plugins[0].ID != "plug-a" {
		t.Errorf("Plugins = %v, want [plug-a]", profile.Plugins)
	}
}

func TestSnapshotProfile_FiltersUnavailableMarketplace(t *testing.T) {
	dir := t.TempDir()
	manPath := filepath.Join(dir, "manifest.json")

	m := &manifest.Manifest{Version: 1}
	if err := manifest.Save(manPath, m); err != nil {
		t.Fatal(err)
	}

	hooks := &mockHooks{cascadeOK: true}
	installed := map[string]manifest.MCPServer{"srv": {Command: "cmd"}}
	plugins := []manifest.Plugin{
		{ID: "plug-a@missing-mkt", Scope: "global", Enabled: true},
		{ID: "standalone", Scope: "project", Enabled: false},
	}
	// missing-mkt is NOT in claudeMarketplaces
	claudeMkts := map[string]manifest.Marketplace{}

	_, err := SnapshotProfile(manPath, "test", false, installed, plugins, claudeMkts, hooks)
	if err != nil {
		t.Fatal(err)
	}

	loaded, err := manifest.Load(manPath)
	if err != nil {
		t.Fatal(err)
	}

	profile := loaded.Profiles["test"]
	// Only standalone should be in profile (plug-a skipped due to missing marketplace).
	if len(profile.Plugins) != 1 || profile.Plugins[0].ID != "standalone" {
		t.Errorf("Plugins = %v, want [standalone]", profile.Plugins)
	}
}

func TestSnapshotProfile_AutoExportsMarketplace(t *testing.T) {
	dir := t.TempDir()
	manPath := filepath.Join(dir, "manifest.json")

	m := &manifest.Manifest{Version: 1}
	if err := manifest.Save(manPath, m); err != nil {
		t.Fatal(err)
	}

	hooks := &mockHooks{cascadeOK: true}
	installed := map[string]manifest.MCPServer{"srv": {Command: "cmd"}}
	plugins := []manifest.Plugin{
		{ID: "plug-a@my-mkt", Scope: "global", Enabled: true},
	}
	claudeMkts := map[string]manifest.Marketplace{
		"my-mkt": {Source: "github", Repo: "owner/repo"},
	}

	_, err := SnapshotProfile(manPath, "test", false, installed, plugins, claudeMkts, hooks)
	if err != nil {
		t.Fatal(err)
	}

	loaded, err := manifest.Load(manPath)
	if err != nil {
		t.Fatal(err)
	}

	if _, ok := loaded.Marketplaces["my-mkt"]; !ok {
		t.Error("marketplace should be auto-exported")
	}

	profile := loaded.Profiles["test"]
	if len(profile.Plugins) != 1 || profile.Plugins[0].ID != "plug-a@my-mkt" {
		t.Errorf("Plugins = %v, want [plug-a@my-mkt]", profile.Plugins)
	}
}

func TestSnapshotProfile_SortedOutput(t *testing.T) {
	dir := t.TempDir()
	manPath := filepath.Join(dir, "manifest.json")

	m := &manifest.Manifest{Version: 1}
	if err := manifest.Save(manPath, m); err != nil {
		t.Fatal(err)
	}

	hooks := &mockHooks{cascadeOK: true}
	installed := map[string]manifest.MCPServer{
		"z-server": {Command: "z"},
		"a-server": {Command: "a"},
	}
	plugins := []manifest.Plugin{
		{ID: "z-plugin", Scope: "global", Enabled: true},
		{ID: "a-plugin", Scope: "project", Enabled: false},
	}

	_, err := SnapshotProfile(manPath, "test", false, installed, plugins, nil, hooks)
	if err != nil {
		t.Fatal(err)
	}

	loaded, err := manifest.Load(manPath)
	if err != nil {
		t.Fatal(err)
	}

	profile := loaded.Profiles["test"]
	wantMCP := []string{"a-server", "z-server"}
	if len(profile.MCPServers) != 2 || !sort.StringsAreSorted(profile.MCPServers) {
		t.Errorf("MCPServers = %v, want sorted %v", profile.MCPServers, wantMCP)
	}
	wantPlugins := []string{"a-plugin", "z-plugin"}
	pluginIDs := make([]string, len(profile.Plugins))
	for i, p := range profile.Plugins {
		pluginIDs[i] = p.ID
	}
	if len(profile.Plugins) != 2 || !sort.StringsAreSorted(pluginIDs) {
		t.Errorf("Plugins = %v, want sorted %v", pluginIDs, wantPlugins)
	}
}

func TestSnapshotProfile_ErrorIfExists(t *testing.T) {
	dir := t.TempDir()
	manPath := filepath.Join(dir, "manifest.json")

	m := &manifest.Manifest{
		Version:  1,
		Profiles: map[string]manifest.Profile{"existing": {}},
	}
	if err := manifest.Save(manPath, m); err != nil {
		t.Fatal(err)
	}

	hooks := &mockHooks{cascadeOK: true}
	_, err := SnapshotProfile(manPath, "existing", false, nil, nil, nil, hooks)
	if err == nil {
		t.Fatal("expected error when profile already exists")
	}
}

func TestSnapshotProfile_UpdatePreservesEnabledOverrides(t *testing.T) {
	dir := t.TempDir()
	manPath := filepath.Join(dir, "manifest.json")

	disabled := false
	m := &manifest.Manifest{
		Version: 1,
		Plugins: []manifest.Plugin{
			{ID: "plug-a", Scope: "global", Enabled: true},
			{ID: "plug-b", Scope: "global", Enabled: true},
		},
		Profiles: map[string]manifest.Profile{
			"test": {
				Plugins: []manifest.ProfilePlugin{
					{ID: "plug-a", Enabled: &disabled}, // explicit override
					{ID: "plug-b"},                     // no override (nil)
				},
			},
		},
	}
	if err := manifest.Save(manPath, m); err != nil {
		t.Fatal(err)
	}

	hooks := &mockHooks{cascadeOK: true}
	// Update the profile — plug-a and plug-b are still in the installed set.
	plugins := []manifest.Plugin{
		{ID: "plug-a", Scope: "global", Enabled: true},
		{ID: "plug-b", Scope: "global", Enabled: true},
	}

	_, err := SnapshotProfile(manPath, "test", true, nil, plugins, nil, hooks)
	if err != nil {
		t.Fatal(err)
	}

	loaded, err := manifest.Load(manPath)
	if err != nil {
		t.Fatal(err)
	}

	profile := loaded.Profiles["test"]

	// Find plug-a in the updated profile.
	var foundA *manifest.ProfilePlugin
	for i := range profile.Plugins {
		if profile.Plugins[i].ID == "plug-a" {
			foundA = &profile.Plugins[i]
			break
		}
	}
	if foundA == nil {
		t.Fatal("plug-a should be in profile")
	}
	if foundA.Enabled == nil || *foundA.Enabled != false {
		t.Errorf("plug-a Enabled = %v, want ptr to false (override should be preserved)", foundA.Enabled)
	}

	// plug-b should still have no override.
	var foundB *manifest.ProfilePlugin
	for i := range profile.Plugins {
		if profile.Plugins[i].ID == "plug-b" {
			foundB = &profile.Plugins[i]
			break
		}
	}
	if foundB == nil {
		t.Fatal("plug-b should be in profile")
	}
	if foundB.Enabled != nil {
		t.Errorf("plug-b Enabled = %v, want nil (no override)", foundB.Enabled)
	}
}

func TestSnapshotProfile_UpdateRequiresExisting(t *testing.T) {
	dir := t.TempDir()
	manPath := filepath.Join(dir, "manifest.json")

	m := &manifest.Manifest{Version: 1}
	if err := manifest.Save(manPath, m); err != nil {
		t.Fatal(err)
	}

	hooks := &mockHooks{cascadeOK: true}
	_, err := SnapshotProfile(manPath, "missing", true, nil, nil, nil, hooks)
	if err == nil {
		t.Fatal("expected error when profile doesn't exist and mustExist=true")
	}
}
