package tui

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/alphaleonis/cctote/internal/claude"
	"github.com/alphaleonis/cctote/internal/engine"
	"github.com/alphaleonis/cctote/internal/manifest"
	"github.com/alphaleonis/cctote/internal/mcp"
)

// mockPluginRunner simulates the Claude CLI for plugin operations.
type mockPluginRunner struct {
	calls       [][]string
	failInstall map[string]bool // plugin IDs that fail on install
	failEnable  map[string]bool // plugin IDs that fail on enable/disable
	failRemove  map[string]bool // plugin IDs that fail on uninstall
}

func (r *mockPluginRunner) Run(_ context.Context, args ...string) ([]byte, error) {
	r.calls = append(r.calls, args)

	if len(args) >= 3 && args[0] == "plugin" {
		// Plugin ID is normally at args[2], but scopeArgs inserts
		// "-s <scope>" at position 2-3 when scope is non-empty,
		// pushing the ID to args[4].
		idIdx := 2
		if len(args) > 3 && args[2] == "-s" {
			idIdx = 4
		}
		if idIdx >= len(args) {
			return []byte("ok"), nil
		}
		id := args[idIdx]
		switch args[1] {
		case "install":
			if r.failInstall[id] {
				return nil, fmt.Errorf("simulated install failure for %s", id)
			}
		case "enable", "disable":
			if r.failEnable[id] {
				return nil, fmt.Errorf("simulated enable failure for %s", id)
			}
		case "uninstall":
			if r.failRemove[id] {
				return nil, fmt.Errorf("simulated uninstall failure for %s", id)
			}
		}
	}

	return []byte("ok"), nil
}

func (r *mockPluginRunner) installCallCount() int {
	n := 0
	for _, call := range r.calls {
		if len(call) >= 2 && call[0] == "plugin" && call[1] == "install" {
			n++
		}
	}
	return n
}

func (r *mockPluginRunner) uninstallCallCount() int {
	n := 0
	for _, call := range r.calls {
		if len(call) >= 2 && call[0] == "plugin" && call[1] == "uninstall" {
			n++
		}
	}
	return n
}

func (r *mockPluginRunner) marketplaceAddCalls() []string {
	var sources []string
	for _, call := range r.calls {
		if len(call) >= 4 && call[0] == "plugin" && call[1] == "marketplace" && call[2] == "add" {
			sources = append(sources, call[3])
		}
	}
	return sources
}

// --- engine.ApplyPluginImport (via engine layer) ---

func TestApplyPluginImport_AccumulatesInstallErrors(t *testing.T) {
	runner := &mockPluginRunner{
		failInstall: map[string]bool{"failing-plugin": true},
	}
	client := claude.NewClient(runner)

	plan := &engine.ImportPlan{
		Add:      []string{"good-a", "failing-plugin", "good-b"},
		Skip:     []string{},
		Conflict: []string{},
		Remove:   []string{},
	}
	desiredPlugMap := map[string]manifest.Plugin{
		"good-a":         {ID: "good-a", Enabled: true},
		"failing-plugin": {ID: "failing-plugin", Enabled: true},
		"good-b":         {ID: "good-b", Enabled: true},
	}

	result := engine.ApplyPluginImport(context.Background(), client, plan, desiredPlugMap, nil, tuiHooks{}, "")

	// Should return an error (the failing plugin).
	if result.Err() == nil {
		t.Fatal("expected error from failing plugin")
	}

	// All 3 installs should have been attempted — not just the 2 before failure.
	if runner.installCallCount() != 3 {
		t.Errorf("install attempts = %d, want 3 (all plugins attempted despite failure)", runner.installCallCount())
	}

	// Error should mention the failing plugin.
	if !strings.Contains(result.Err().Error(), "failing-plugin") {
		t.Errorf("error should mention failing-plugin: %v", result.Err())
	}
}

func TestApplyPluginImport_AccumulatesUninstallErrors(t *testing.T) {
	runner := &mockPluginRunner{
		failRemove: map[string]bool{"failing-remove": true},
	}
	client := claude.NewClient(runner)

	plan := &engine.ImportPlan{
		Add:      []string{},
		Skip:     []string{},
		Conflict: []string{},
		Remove:   []string{"good-remove", "failing-remove", "another-remove"},
	}

	result := engine.ApplyPluginImport(context.Background(), client, plan, nil, nil, tuiHooks{}, "")

	if result.Err() == nil {
		t.Fatal("expected error from failing uninstall")
	}

	// All 3 uninstalls should have been attempted.
	if runner.uninstallCallCount() != 3 {
		t.Errorf("uninstall attempts = %d, want 3", runner.uninstallCallCount())
	}
}

func TestApplyPluginImport_SkipsEnableAfterFailedInstall(t *testing.T) {
	runner := &mockPluginRunner{
		failInstall: map[string]bool{"failing": true},
	}
	client := claude.NewClient(runner)

	plan := &engine.ImportPlan{
		Add:      []string{"failing"},
		Skip:     []string{},
		Conflict: []string{},
		Remove:   []string{},
	}
	desiredPlugMap := map[string]manifest.Plugin{
		"failing": {ID: "failing", Enabled: false},
	}

	_ = engine.ApplyPluginImport(context.Background(), client, plan, desiredPlugMap, nil, tuiHooks{}, "")

	// Should NOT attempt enable/disable after failed install.
	for _, call := range runner.calls {
		if len(call) >= 2 && (call[1] == "enable" || call[1] == "disable") {
			t.Error("should not attempt enable/disable after failed install")
		}
	}
}

// --- execExport with swapped panes (Finding #1) ---

func TestExecExport_SwappedPanes(t *testing.T) {
	// Scenario: user swaps panes so Claude Code is on the left and
	// Manifest is on the right. compState is computed as
	// CompareSources(claudeData, manifestData), so Claude Code items
	// are on the Left side and Manifest items are on the Right.
	//
	// execExport should still read the correct source data (Claude Code)
	// regardless of pane orientation.

	dir := t.TempDir()
	manPath := dir + "/manifest.json"
	m := &manifest.Manifest{
		Version:    1,
		MCPServers: map[string]manifest.MCPServer{},
		Plugins:    nil,
	}
	if err := manifest.Save(manPath, m); err != nil {
		t.Fatal(err)
	}

	claudeMCP := map[string]manifest.MCPServer{
		"my-server": {Command: "my-cmd"},
	}

	// Swapped panes: left=ClaudeCode, right=Manifest.
	leftData := engine.SourceData{MCP: claudeMCP}
	rightData := engine.SourceData{MCP: map[string]manifest.MCPServer{}}
	compState := engine.CompareSources(leftData, rightData)

	// "my-server" is LeftOnly in this comparison (only in Claude Code / left pane).
	sync, ok := compState.MCPSync["my-server"]
	if !ok || sync.Status != engine.LeftOnly {
		t.Fatalf("expected LeftOnly, got %v (ok=%v)", sync.Status, ok)
	}

	fullState := &engine.FullState{
		MCPInstalled: claudeMCP,
	}

	from := PaneSource{Kind: SourceClaudeCode}
	to := PaneSource{Kind: SourceManifest}

	// executeCopy should handle the swapped orientation correctly.
	opts := Options{ManifestPath: manPath}
	item := CopyItem{Section: SectionMCP, Name: "my-server"}
	cmd := executeCopy(opts, []CopyItem{item}, OpExportToManifest, from, to, fullState)

	// Run the command synchronously.
	msg := cmd()
	result, ok := msg.(CopyResultMsg)
	if !ok {
		t.Fatalf("expected CopyResultMsg, got %T", msg)
	}
	if len(result.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result.Results))
	}
	if result.Results[0].Err != nil {
		t.Fatalf("export failed: %v", result.Results[0].Err)
	}

	// Verify the server was actually written to the manifest.
	loaded, err := manifest.Load(manPath)
	if err != nil {
		t.Fatal(err)
	}
	srv, ok := loaded.MCPServers["my-server"]
	if !ok {
		t.Fatal("server not found in manifest after export")
	}
	if srv.Command != "my-cmd" {
		t.Errorf("exported command = %q, want %q", srv.Command, "my-cmd")
	}
}

// --- execCopyProfileRef missing fromProfile check (Finding #4) ---

func TestExecCopyProfileRef_MissingFromProfile(t *testing.T) {
	// Scenario: item exists in manifest and in toProfile, but fromProfile
	// does NOT contain a reference to the item. The function should error
	// instead of silently adding to toProfile.

	dir := t.TempDir()
	manPath := dir + "/manifest.json"
	m := &manifest.Manifest{
		Version: 1,
		MCPServers: map[string]manifest.MCPServer{
			"my-server": {Command: "cmd"},
		},
		Profiles: map[string]manifest.Profile{
			"from-profile": {MCPServers: []string{}, Plugins: []manifest.ProfilePlugin{}}, // does NOT have "my-server"
			"to-profile":   {MCPServers: []string{}, Plugins: []manifest.ProfilePlugin{}},
		},
	}
	if err := manifest.Save(manPath, m); err != nil {
		t.Fatal(err)
	}

	opts := Options{ManifestPath: manPath}
	item := CopyItem{Section: SectionMCP, Name: "my-server"}

	err := execCopyProfileRef(opts, item, "from-profile", "to-profile")
	if err == nil {
		t.Fatal("expected error when item not in fromProfile, got nil")
	}
	if !strings.Contains(err.Error(), "from-profile") {
		t.Errorf("error should mention from-profile: %v", err)
	}
}

// --- doProfileDelete ---

func TestDoProfileDelete_Success(t *testing.T) {
	dir := t.TempDir()
	manPath := dir + "/manifest.json"
	m := &manifest.Manifest{
		Version:    1,
		MCPServers: map[string]manifest.MCPServer{},
		Profiles: map[string]manifest.Profile{
			"dev":     {MCPServers: []string{"srv"}, Plugins: []manifest.ProfilePlugin{}},
			"staging": {MCPServers: []string{}, Plugins: []manifest.ProfilePlugin{}},
		},
	}
	if err := manifest.Save(manPath, m); err != nil {
		t.Fatal(err)
	}

	opts := Options{ManifestPath: manPath}
	if err := doProfileDelete(opts, "dev"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	loaded, err := manifest.Load(manPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := loaded.Profiles["dev"]; ok {
		t.Error("profile 'dev' should have been deleted")
	}
	if _, ok := loaded.Profiles["staging"]; !ok {
		t.Error("profile 'staging' should still exist")
	}
}

func TestDoProfileDelete_NotFound(t *testing.T) {
	dir := t.TempDir()
	manPath := dir + "/manifest.json"
	m := &manifest.Manifest{
		Version:    1,
		MCPServers: map[string]manifest.MCPServer{},
		Profiles: map[string]manifest.Profile{
			"dev": {MCPServers: []string{}, Plugins: []manifest.ProfilePlugin{}},
		},
	}
	if err := manifest.Save(manPath, m); err != nil {
		t.Fatal(err)
	}

	opts := Options{ManifestPath: manPath}
	err := doProfileDelete(opts, "nonexistent")
	if err == nil {
		t.Fatal("expected error for missing profile")
	}
	if !strings.Contains(err.Error(), "nonexistent") {
		t.Errorf("error should mention profile name: %v", err)
	}
}

func TestDoProfileDelete_LastProfile_NilsMap(t *testing.T) {
	dir := t.TempDir()
	manPath := dir + "/manifest.json"
	m := &manifest.Manifest{
		Version:    1,
		MCPServers: map[string]manifest.MCPServer{},
		Profiles: map[string]manifest.Profile{
			"only": {MCPServers: []string{}, Plugins: []manifest.ProfilePlugin{}},
		},
	}
	if err := manifest.Save(manPath, m); err != nil {
		t.Fatal(err)
	}

	opts := Options{ManifestPath: manPath}
	if err := doProfileDelete(opts, "only"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	loaded, err := manifest.Load(manPath)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Profiles != nil {
		t.Errorf("expected nil Profiles map, got %v", loaded.Profiles)
	}
}

func TestApplyPluginImport_ScopedFailInstall(t *testing.T) {
	// Verifies that mockPluginRunner correctly matches plugin IDs
	// even when scope args shift the ID to a later position.
	runner := &mockPluginRunner{
		failInstall: map[string]bool{"scoped-plugin": true},
	}
	client := claude.NewClient(runner)

	plan := &engine.ImportPlan{
		Add:      []string{"scoped-plugin"},
		Skip:     []string{},
		Conflict: []string{},
		Remove:   []string{},
	}
	desiredPlugMap := map[string]manifest.Plugin{
		"scoped-plugin": {ID: "scoped-plugin", Enabled: true},
	}

	result := engine.ApplyPluginImport(context.Background(), client, plan, desiredPlugMap, nil, tuiHooks{}, "project")

	// With scope="project", args become ["plugin", "install", "-s", "project", "scoped-plugin"].
	// The mock must find "scoped-plugin" (not "-s") to trigger the failure.
	if result.Err() == nil {
		t.Fatal("expected error from scoped failing plugin, but got nil — mock did not match the plugin ID")
	}
	if !strings.Contains(result.Err().Error(), "scoped-plugin") {
		t.Errorf("error should mention scoped-plugin: %v", result.Err())
	}
}

func TestApplyPluginImport_NoErrorWhenAllSucceed(t *testing.T) {
	runner := &mockPluginRunner{}
	client := claude.NewClient(runner)

	plan := &engine.ImportPlan{
		Add:      []string{"plugin-a", "plugin-b"},
		Skip:     []string{},
		Conflict: []string{"plugin-c"},
		Remove:   []string{"plugin-d"},
	}
	desiredPlugMap := map[string]manifest.Plugin{
		"plugin-a": {ID: "plugin-a", Enabled: true},
		"plugin-b": {ID: "plugin-b", Enabled: false},
		"plugin-c": {ID: "plugin-c", Enabled: true},
	}

	result := engine.ApplyPluginImport(context.Background(), client, plan, desiredPlugMap, nil, tuiHooks{}, "")
	if result.Err() != nil {
		t.Fatalf("unexpected error: %v", result.Err())
	}
	if result.Installed != 2 {
		t.Errorf("Installed = %d, want 2", result.Installed)
	}
	if result.Reconciled != 1 {
		t.Errorf("Reconciled = %d, want 1", result.Reconciled)
	}
	if result.Uninstalled != 1 {
		t.Errorf("Uninstalled = %d, want 1", result.Uninstalled)
	}
}

// --- execToggleManifestPlugin ---

// --- execRemoveFromClaude ---

func TestExecRemoveFromClaude_MCP(t *testing.T) {
	dir := t.TempDir()
	claudePath := filepath.Join(dir, "claude.json")

	// Write a claude.json with an MCP server.
	initial := map[string]manifest.MCPServer{
		"my-server": {Command: "npx", Args: []string{"run"}},
		"keep-me":   {Command: "keep"},
	}
	if err := mcp.WriteMcpServers(claudePath, initial); err != nil {
		t.Fatal(err)
	}

	opts := Options{ClaudeMCPPath: claudePath}
	err := execRemoveFromClaude(opts, CopyItem{Section: SectionMCP, Name: "my-server"})
	if err != nil {
		t.Fatal(err)
	}

	remaining, err := mcp.ReadMcpServers(claudePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := remaining["my-server"]; ok {
		t.Error("my-server should have been removed")
	}
	if _, ok := remaining["keep-me"]; !ok {
		t.Error("keep-me should still be present")
	}
}

func TestExecRemoveFromClaude_Plugin(t *testing.T) {
	runner := &mockPluginRunner{}
	client := claude.NewClient(runner)
	opts := Options{
		NewClient: func() *claude.Client { return client },
	}

	err := execRemoveFromClaude(opts, CopyItem{Section: SectionPlugin, Name: "my-plugin"})
	if err != nil {
		t.Fatal(err)
	}

	// Should have called `plugin uninstall my-plugin`.
	if len(runner.calls) != 1 {
		t.Fatalf("calls = %d, want 1", len(runner.calls))
	}
	call := runner.calls[0]
	expected := []string{"plugin", "uninstall", "my-plugin"}
	if len(call) != len(expected) {
		t.Fatalf("call args = %v, want %v", call, expected)
	}
	for i, arg := range expected {
		if call[i] != arg {
			t.Errorf("call[%d] = %q, want %q", i, call[i], arg)
		}
	}
}

func TestExecRemoveFromClaude_Marketplace(t *testing.T) {
	runner := &mockPluginRunner{}
	client := claude.NewClient(runner)
	opts := Options{
		NewClient: func() *claude.Client { return client },
	}

	err := execRemoveFromClaude(opts, CopyItem{Section: SectionMarketplace, Name: "my-mkt"})
	if err != nil {
		t.Fatal(err)
	}

	// Should have called `plugin marketplace remove my-mkt`.
	if len(runner.calls) != 1 {
		t.Fatalf("calls = %d, want 1", len(runner.calls))
	}
	call := runner.calls[0]
	expected := []string{"plugin", "marketplace", "remove", "my-mkt"}
	if len(call) != len(expected) {
		t.Fatalf("call args = %v, want %v", call, expected)
	}
	for i, arg := range expected {
		if call[i] != arg {
			t.Errorf("call[%d] = %q, want %q", i, call[i], arg)
		}
	}
}

// --- execToggleManifestPlugin ---

func TestExecToggleManifestPlugin_Success(t *testing.T) {
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

	opts := Options{ManifestPath: manPath}
	if err := execToggleManifestPlugin(opts, "plug-a", false); err != nil {
		t.Fatal(err)
	}

	loaded, err := manifest.Load(manPath)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Plugins[0].Enabled != false {
		t.Errorf("Enabled = %v, want false", loaded.Plugins[0].Enabled)
	}
}

func TestExecToggleManifestPlugin_NotFound(t *testing.T) {
	dir := t.TempDir()
	manPath := filepath.Join(dir, "manifest.json")

	m := &manifest.Manifest{Version: 1}
	if err := manifest.Save(manPath, m); err != nil {
		t.Fatal(err)
	}

	opts := Options{ManifestPath: manPath}
	err := execToggleManifestPlugin(opts, "nonexistent", true)
	if err == nil {
		t.Fatal("expected error for missing plugin")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, want 'not found' substring", err)
	}
}

// --- execToggleProfilePlugin ---

func TestExecToggleProfilePlugin_SetsOverride(t *testing.T) {
	dir := t.TempDir()
	manPath := filepath.Join(dir, "manifest.json")

	m := &manifest.Manifest{
		Version: 1,
		Plugins: []manifest.Plugin{
			{ID: "plug-a", Scope: "global", Enabled: true},
		},
		Profiles: map[string]manifest.Profile{
			"test": {
				Plugins: []manifest.ProfilePlugin{{ID: "plug-a"}},
			},
		},
	}
	if err := manifest.Save(manPath, m); err != nil {
		t.Fatal(err)
	}

	opts := Options{ManifestPath: manPath}
	// Toggle to false — differs from manifest default (true), so override should be stored.
	if err := execToggleProfilePlugin(opts, "plug-a", false, "test"); err != nil {
		t.Fatal(err)
	}

	loaded, err := manifest.Load(manPath)
	if err != nil {
		t.Fatal(err)
	}
	pp := loaded.Profiles["test"].Plugins[0]
	if pp.Enabled == nil || *pp.Enabled != false {
		t.Errorf("Enabled = %v, want ptr to false", pp.Enabled)
	}
}

func TestExecToggleProfilePlugin_ClearsOverrideWhenMatchingDefault(t *testing.T) {
	dir := t.TempDir()
	manPath := filepath.Join(dir, "manifest.json")

	disabled := false
	m := &manifest.Manifest{
		Version: 1,
		Plugins: []manifest.Plugin{
			{ID: "plug-a", Scope: "global", Enabled: true},
		},
		Profiles: map[string]manifest.Profile{
			"test": {
				Plugins: []manifest.ProfilePlugin{{ID: "plug-a", Enabled: &disabled}},
			},
		},
	}
	if err := manifest.Save(manPath, m); err != nil {
		t.Fatal(err)
	}

	opts := Options{ManifestPath: manPath}
	// Toggle to true — matches manifest default, so override should be cleared to nil.
	if err := execToggleProfilePlugin(opts, "plug-a", true, "test"); err != nil {
		t.Fatal(err)
	}

	loaded, err := manifest.Load(manPath)
	if err != nil {
		t.Fatal(err)
	}
	pp := loaded.Profiles["test"].Plugins[0]
	if pp.Enabled != nil {
		t.Errorf("Enabled = %v, want nil (cleared to inherit)", pp.Enabled)
	}
}

func TestExecToggleProfilePlugin_NotFoundProfile(t *testing.T) {
	dir := t.TempDir()
	manPath := filepath.Join(dir, "manifest.json")

	m := &manifest.Manifest{Version: 1}
	if err := manifest.Save(manPath, m); err != nil {
		t.Fatal(err)
	}

	opts := Options{ManifestPath: manPath}
	err := execToggleProfilePlugin(opts, "plug-a", true, "nonexistent")
	if err == nil {
		t.Fatal("expected error for missing profile")
	}
}

func TestExecToggleProfilePlugin_NotFoundPlugin(t *testing.T) {
	dir := t.TempDir()
	manPath := filepath.Join(dir, "manifest.json")

	m := &manifest.Manifest{
		Version: 1,
		Profiles: map[string]manifest.Profile{
			"test": {Plugins: []manifest.ProfilePlugin{}},
		},
	}
	if err := manifest.Save(manPath, m); err != nil {
		t.Fatal(err)
	}

	opts := Options{ManifestPath: manPath}
	err := execToggleProfilePlugin(opts, "nonexistent", true, "test")
	if err == nil {
		t.Fatal("expected error for missing plugin in profile")
	}
}

// --- autoImportMarketplaces ---

func newTestProgressHooks(ch chan<- tea.Msg, total int) *tuiProgressHooks {
	return &tuiProgressHooks{ch: ch, total: total}
}

func TestAutoImportMarketplaces_InstallsMissing(t *testing.T) {
	runner := &mockPluginRunner{}
	client := claude.NewClient(runner)
	ch := make(chan tea.Msg, 20)
	hooks := newTestProgressHooks(ch, 5)

	full := &FullState{
		Manifest: &manifest.Manifest{
			Version: 1,
			Marketplaces: map[string]manifest.Marketplace{
				"mp1": {Source: "github", Repo: "owner1/repo1"},
				"mp2": {Source: "github", Repo: "owner2/repo2"},
			},
		},
		MktInstalled: map[string]manifest.Marketplace{},
	}

	err := autoImportMarketplaces(context.Background(), client, []string{"p1@mp1", "p2@mp1", "p3@mp2"}, full, hooks)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should install each marketplace exactly once (mp1 deduplicated).
	calls := runner.marketplaceAddCalls()
	if len(calls) != 2 {
		t.Fatalf("marketplace add calls = %d, want 2; calls: %v", len(calls), calls)
	}

	// The shared MktInstalled map must NOT be mutated (race safety).
	if _, ok := full.MktInstalled["mp1"]; ok {
		t.Error("shared MktInstalled should not be mutated from background goroutine")
	}
}

func TestAutoImportMarketplaces_SkipsAlreadyInstalled(t *testing.T) {
	runner := &mockPluginRunner{}
	client := claude.NewClient(runner)
	ch := make(chan tea.Msg, 20)
	hooks := newTestProgressHooks(ch, 5)

	full := &FullState{
		Manifest: &manifest.Manifest{
			Version: 1,
			Marketplaces: map[string]manifest.Marketplace{
				"mp1": {Source: "github", Repo: "o/r"},
			},
		},
		MktInstalled: map[string]manifest.Marketplace{
			"mp1": {Source: "github", Repo: "o/r"},
		},
	}

	err := autoImportMarketplaces(context.Background(), client, []string{"p1@mp1"}, full, hooks)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	calls := runner.marketplaceAddCalls()
	if len(calls) != 0 {
		t.Errorf("should not call marketplace add for already-installed, got %v", calls)
	}
}

func TestAutoImportMarketplaces_ErrorsWhenNotInManifest(t *testing.T) {
	runner := &mockPluginRunner{}
	client := claude.NewClient(runner)
	ch := make(chan tea.Msg, 20)
	hooks := newTestProgressHooks(ch, 5)

	full := &FullState{
		Manifest: &manifest.Manifest{
			Version:      1,
			Marketplaces: map[string]manifest.Marketplace{},
		},
		MktInstalled: map[string]manifest.Marketplace{},
	}

	err := autoImportMarketplaces(context.Background(), client, []string{"p1@unknown-mp"}, full, hooks)
	if err == nil {
		t.Fatal("expected error when marketplace not in manifest")
	}
	if !strings.Contains(err.Error(), "unknown-mp") {
		t.Errorf("error should mention the marketplace name, got: %v", err)
	}
}

func TestAutoImportMarketplaces_SkipsPluginsWithoutMarketplace(t *testing.T) {
	runner := &mockPluginRunner{}
	client := claude.NewClient(runner)
	ch := make(chan tea.Msg, 20)
	hooks := newTestProgressHooks(ch, 5)

	full := &FullState{
		Manifest:     &manifest.Manifest{Version: 1},
		MktInstalled: map[string]manifest.Marketplace{},
	}

	err := autoImportMarketplaces(context.Background(), client, []string{"plain-plugin"}, full, hooks)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	calls := runner.marketplaceAddCalls()
	if len(calls) != 0 {
		t.Errorf("should not call marketplace add for plugin without marketplace ref, got %v", calls)
	}
}

func TestAutoImportMarketplaces_NilFullState(t *testing.T) {
	runner := &mockPluginRunner{}
	client := claude.NewClient(runner)
	ch := make(chan tea.Msg, 20)
	hooks := newTestProgressHooks(ch, 5)

	// When full is nil and plugins have marketplace deps, should return
	// an error (missing prereqs) without panicking on nil dereference.
	err := autoImportMarketplaces(context.Background(), client, []string{"p1@mp1"}, nil, hooks)
	if err == nil {
		t.Fatal("expected error when full state is nil and plugin has marketplace dep")
	}
}

func TestRecoverProgressPanic(t *testing.T) {
	// Verify that recoverProgressPanic sends a ProgressFinishedMsg
	// when a panic occurs, preventing the progress overlay from getting stuck.
	ch := make(chan tea.Msg, 5)

	func() {
		defer recoverProgressPanic(ch)
		panic("simulated crash")
	}()

	select {
	case msg := <-ch:
		fin, ok := msg.(ProgressFinishedMsg)
		if !ok {
			t.Fatalf("expected ProgressFinishedMsg, got %T", msg)
		}
		if fin.Err == nil {
			t.Fatal("expected non-nil error in ProgressFinishedMsg")
		}
		if !strings.Contains(fin.Err.Error(), "simulated crash") {
			t.Errorf("error should contain panic value, got: %v", fin.Err)
		}
	default:
		t.Fatal("no message sent to channel after panic recovery")
	}
}

func TestRecoverProgressPanic_NoPanic(t *testing.T) {
	// Verify that recoverProgressPanic is a no-op when no panic occurs.
	ch := make(chan tea.Msg, 5)

	func() {
		defer recoverProgressPanic(ch)
		// No panic — normal exit.
	}()

	select {
	case msg := <-ch:
		t.Fatalf("unexpected message sent when no panic: %T", msg)
	default:
		// Expected: no message.
	}
}

func TestAutoImportMarketplaces_NilFullStateNoMarketplaceDeps(t *testing.T) {
	runner := &mockPluginRunner{}
	client := claude.NewClient(runner)
	ch := make(chan tea.Msg, 20)
	hooks := newTestProgressHooks(ch, 5)

	// When full is nil but no plugins need marketplaces, should succeed
	// without panicking on nil dereference.
	err := autoImportMarketplaces(context.Background(), client, []string{"standalone"}, nil, hooks)
	if err != nil {
		t.Fatalf("unexpected error for standalone plugins with nil full state: %v", err)
	}
}
