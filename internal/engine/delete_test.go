package engine

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/alphaleonis/cctote/internal/manifest"
)

// mockHooks records calls and returns configured responses.
type mockHooks struct {
	cascadeCalls []cascadeCall
	infoCalls    []string
	warnCalls    []string
	cascadeOK    bool  // what OnCascade returns
	cascadeErr   error // error to return from OnCascade
}

type cascadeCall struct {
	item       string
	dependents []string
}

func (h *mockHooks) OnCascade(item string, dependents []string) (bool, error) {
	h.cascadeCalls = append(h.cascadeCalls, cascadeCall{item, dependents})
	if h.cascadeErr != nil {
		return false, h.cascadeErr
	}
	return h.cascadeOK, nil
}

func (h *mockHooks) OnInfo(msg string) {
	h.infoCalls = append(h.infoCalls, msg)
}

func (h *mockHooks) OnWarn(msg string) {
	h.warnCalls = append(h.warnCalls, msg)
}

// --- DeleteMCP ---

func TestDeleteMCP_Simple(t *testing.T) {
	dir := t.TempDir()
	manPath := filepath.Join(dir, "manifest.json")

	m := &manifest.Manifest{
		Version:    1,
		MCPServers: map[string]manifest.MCPServer{"foo": {Command: "foo-cmd"}, "bar": {Command: "bar-cmd"}},
	}
	if err := manifest.Save(manPath, m); err != nil {
		t.Fatal(err)
	}

	hooks := &mockHooks{cascadeOK: true}
	result, err := DeleteMCP(manPath, "foo", hooks)
	if err != nil {
		t.Fatal(err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	if result.Count(ActionRemoved) != 1 {
		t.Errorf("removed count = %d, want 1", result.Count(ActionRemoved))
	}
	if result.Names(ActionRemoved)[0] != "foo" {
		t.Errorf("removed name = %q, want %q", result.Names(ActionRemoved)[0], "foo")
	}
	if len(result.CleanedProfiles) != 0 {
		t.Errorf("cleanedProfiles = %v, want empty", result.CleanedProfiles)
	}
	if len(hooks.cascadeCalls) != 0 {
		t.Errorf("expected no cascade calls, got %d", len(hooks.cascadeCalls))
	}

	// Verify manifest on disk.
	loaded, err := manifest.Load(manPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := loaded.MCPServers["foo"]; ok {
		t.Error("foo should be removed from manifest")
	}
	if _, ok := loaded.MCPServers["bar"]; !ok {
		t.Error("bar should still be in manifest")
	}
}

func TestDeleteMCP_WithProfileCascade(t *testing.T) {
	dir := t.TempDir()
	manPath := filepath.Join(dir, "manifest.json")

	m := &manifest.Manifest{
		Version:    1,
		MCPServers: map[string]manifest.MCPServer{"foo": {Command: "cmd"}},
		Profiles: map[string]manifest.Profile{
			"dev":  {MCPServers: []string{"foo", "bar"}},
			"prod": {MCPServers: []string{"foo"}},
		},
	}
	if err := manifest.Save(manPath, m); err != nil {
		t.Fatal(err)
	}

	hooks := &mockHooks{cascadeOK: true}
	result, err := DeleteMCP(manPath, "foo", hooks)
	if err != nil {
		t.Fatal(err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	if len(hooks.cascadeCalls) != 1 {
		t.Fatalf("expected 1 cascade call, got %d", len(hooks.cascadeCalls))
	}
	if hooks.cascadeCalls[0].item != "foo" {
		t.Errorf("cascade item = %q, want %q", hooks.cascadeCalls[0].item, "foo")
	}

	// Both profiles reference foo, so both should be in dependents.
	if len(hooks.cascadeCalls[0].dependents) != 2 {
		t.Errorf("cascade dependents = %v, want 2 profiles", hooks.cascadeCalls[0].dependents)
	}

	wantProfiles := []string{"dev", "prod"}
	if len(result.CleanedProfiles) != len(wantProfiles) {
		t.Fatalf("cleanedProfiles = %v, want %v", result.CleanedProfiles, wantProfiles)
	}
	for i, name := range wantProfiles {
		if result.CleanedProfiles[i] != name {
			t.Errorf("cleanedProfiles[%d] = %q, want %q", i, result.CleanedProfiles[i], name)
		}
	}

	loaded, err := manifest.Load(manPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := loaded.MCPServers["foo"]; ok {
		t.Error("foo should be removed")
	}
	// dev profile should keep "bar" but lose "foo".
	if dev, ok := loaded.Profiles["dev"]; ok {
		if len(dev.MCPServers) != 1 || dev.MCPServers[0] != "bar" {
			t.Errorf("dev profile MCPServers = %v, want [bar]", dev.MCPServers)
		}
	}
	// prod profile should be empty.
	if prod, ok := loaded.Profiles["prod"]; ok {
		if len(prod.MCPServers) != 0 {
			t.Errorf("prod profile MCPServers = %v, want []", prod.MCPServers)
		}
	}
}

func TestDeleteMCP_CascadeDeclined(t *testing.T) {
	dir := t.TempDir()
	manPath := filepath.Join(dir, "manifest.json")

	m := &manifest.Manifest{
		Version:    1,
		MCPServers: map[string]manifest.MCPServer{"foo": {Command: "cmd"}},
		Profiles:   map[string]manifest.Profile{"dev": {MCPServers: []string{"foo"}}},
	}
	if err := manifest.Save(manPath, m); err != nil {
		t.Fatal(err)
	}

	hooks := &mockHooks{cascadeOK: false}
	result, err := DeleteMCP(manPath, "foo", hooks)
	if err != nil {
		t.Fatal(err)
	}
	if result != nil {
		t.Fatal("expected nil result when cascade is declined")
	}

	// Manifest should be unchanged.
	loaded, err := manifest.Load(manPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := loaded.MCPServers["foo"]; !ok {
		t.Error("foo should still be in manifest after declined cascade")
	}
}

func TestDeleteMCP_NotFound(t *testing.T) {
	dir := t.TempDir()
	manPath := filepath.Join(dir, "manifest.json")

	m := &manifest.Manifest{Version: 1, MCPServers: map[string]manifest.MCPServer{}}
	if err := manifest.Save(manPath, m); err != nil {
		t.Fatal(err)
	}

	hooks := &mockHooks{cascadeOK: true}
	_, err := DeleteMCP(manPath, "nonexistent", hooks)
	if err == nil {
		t.Fatal("expected error for nonexistent server")
	}
}

// --- DeletePlugin ---

func TestDeletePlugin_Simple(t *testing.T) {
	dir := t.TempDir()
	manPath := filepath.Join(dir, "manifest.json")

	m := &manifest.Manifest{
		Version: 1,
		Plugins: []manifest.Plugin{
			{ID: "plug-a", Scope: "global", Enabled: true},
			{ID: "plug-b", Scope: "project", Enabled: false},
		},
	}
	if err := manifest.Save(manPath, m); err != nil {
		t.Fatal(err)
	}

	hooks := &mockHooks{cascadeOK: true}
	result, err := DeletePlugin(manPath, "plug-a", hooks)
	if err != nil {
		t.Fatal(err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Count(ActionRemoved) != 1 {
		t.Errorf("removed count = %d, want 1", result.Count(ActionRemoved))
	}

	loaded, err := manifest.Load(manPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Plugins) != 1 || loaded.Plugins[0].ID != "plug-b" {
		t.Errorf("plugins = %v, want [plug-b]", loaded.Plugins)
	}
}

func TestDeletePlugin_WithProfileCascade(t *testing.T) {
	dir := t.TempDir()
	manPath := filepath.Join(dir, "manifest.json")

	m := &manifest.Manifest{
		Version: 1,
		Plugins: []manifest.Plugin{{ID: "plug-a", Scope: "global", Enabled: true}},
		Profiles: map[string]manifest.Profile{
			"dev": {Plugins: []manifest.ProfilePlugin{{ID: "plug-a"}, {ID: "plug-b"}}},
		},
	}
	if err := manifest.Save(manPath, m); err != nil {
		t.Fatal(err)
	}

	hooks := &mockHooks{cascadeOK: true}
	result, err := DeletePlugin(manPath, "plug-a", hooks)
	if err != nil {
		t.Fatal(err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(result.CleanedProfiles) != 1 || result.CleanedProfiles[0] != "dev" {
		t.Errorf("cleanedProfiles = %v, want [dev]", result.CleanedProfiles)
	}

	loaded, err := manifest.Load(manPath)
	if err != nil {
		t.Fatal(err)
	}
	if dev := loaded.Profiles["dev"]; len(dev.Plugins) != 1 || dev.Plugins[0].ID != "plug-b" {
		t.Errorf("dev profile plugins = %v, want [plug-b]", dev.Plugins)
	}
}

// --- DeleteMarketplace ---

func TestDeleteMarketplace_Simple(t *testing.T) {
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
	result, err := DeleteMarketplace(manPath, "my-mkt", hooks)
	if err != nil {
		t.Fatal(err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Count(ActionRemoved) != 1 {
		t.Errorf("removed count = %d, want 1", result.Count(ActionRemoved))
	}
	if len(hooks.cascadeCalls) != 0 {
		t.Errorf("expected no cascade calls (no affected plugins)")
	}

	loaded, err := manifest.Load(manPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := loaded.Marketplaces["my-mkt"]; ok {
		t.Error("marketplace should be removed")
	}
}

func TestDeleteMarketplace_CascadePlugins(t *testing.T) {
	dir := t.TempDir()
	manPath := filepath.Join(dir, "manifest.json")

	m := &manifest.Manifest{
		Version: 1,
		Plugins: []manifest.Plugin{
			{ID: "plug-a@my-mkt", Scope: "global", Enabled: true},
			{ID: "plug-b@my-mkt", Scope: "global", Enabled: true},
			{ID: "plug-c@other-mkt", Scope: "global", Enabled: true},
			{ID: "standalone", Scope: "project", Enabled: false},
		},
		Marketplaces: map[string]manifest.Marketplace{
			"my-mkt":    {Source: "github", Repo: "owner/repo"},
			"other-mkt": {Source: "github", Repo: "other/repo"},
		},
		Profiles: map[string]manifest.Profile{
			"dev": {Plugins: []manifest.ProfilePlugin{{ID: "plug-a@my-mkt"}, {ID: "plug-c@other-mkt"}, {ID: "standalone"}}},
		},
	}
	if err := manifest.Save(manPath, m); err != nil {
		t.Fatal(err)
	}

	hooks := &mockHooks{cascadeOK: true}
	result, err := DeleteMarketplace(manPath, "my-mkt", hooks)
	if err != nil {
		t.Fatal(err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	// Should have 1 removed (marketplace) + 2 cascaded (plugins).
	if result.Count(ActionRemoved) != 1 {
		t.Errorf("removed count = %d, want 1", result.Count(ActionRemoved))
	}
	if result.Count(ActionCascaded) != 2 {
		t.Errorf("cascaded count = %d, want 2", result.Count(ActionCascaded))
	}

	// OnCascade should have been called with affected plugin IDs.
	if len(hooks.cascadeCalls) != 1 {
		t.Fatalf("expected 1 cascade call, got %d", len(hooks.cascadeCalls))
	}
	if len(hooks.cascadeCalls[0].dependents) != 2 {
		t.Errorf("cascade dependents = %v, want 2 plugins", hooks.cascadeCalls[0].dependents)
	}

	// Profile cleanup: dev should lose plug-a@my-mkt but keep the others.
	if len(result.CleanedProfiles) != 1 || result.CleanedProfiles[0] != "dev" {
		t.Errorf("cleanedProfiles = %v, want [dev]", result.CleanedProfiles)
	}

	loaded, err := manifest.Load(manPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := loaded.Marketplaces["my-mkt"]; ok {
		t.Error("my-mkt should be removed")
	}
	if _, ok := loaded.Marketplaces["other-mkt"]; !ok {
		t.Error("other-mkt should still exist")
	}
	if len(loaded.Plugins) != 2 {
		t.Fatalf("plugins count = %d, want 2", len(loaded.Plugins))
	}
	wantPlugins := []string{"plug-c@other-mkt", "standalone"}
	for i, wantID := range wantPlugins {
		if loaded.Plugins[i].ID != wantID {
			t.Errorf("plugins[%d].ID = %q, want %q", i, loaded.Plugins[i].ID, wantID)
		}
	}
	dev := loaded.Profiles["dev"]
	if len(dev.Plugins) != 2 {
		t.Errorf("dev profile plugins = %v, want [plug-c@other-mkt standalone]", dev.Plugins)
	}
}

func TestDeleteMarketplace_CascadeDeclined(t *testing.T) {
	dir := t.TempDir()
	manPath := filepath.Join(dir, "manifest.json")

	m := &manifest.Manifest{
		Version: 1,
		Plugins: []manifest.Plugin{
			{ID: "plug-a@my-mkt", Scope: "global", Enabled: true},
		},
		Marketplaces: map[string]manifest.Marketplace{
			"my-mkt": {Source: "github", Repo: "owner/repo"},
		},
	}
	if err := manifest.Save(manPath, m); err != nil {
		t.Fatal(err)
	}

	hooks := &mockHooks{cascadeOK: false}
	result, err := DeleteMarketplace(manPath, "my-mkt", hooks)
	if err != nil {
		t.Fatal(err)
	}
	if result != nil {
		t.Fatal("expected nil result when cascade is declined")
	}

	// Manifest unchanged.
	loaded, err := manifest.Load(manPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := loaded.Marketplaces["my-mkt"]; !ok {
		t.Error("marketplace should still exist after declined cascade")
	}
	if len(loaded.Plugins) != 1 {
		t.Error("plugins should be unchanged")
	}
}

// --- NotFound ---

func TestDeletePlugin_NotFound(t *testing.T) {
	dir := t.TempDir()
	manPath := filepath.Join(dir, "manifest.json")

	m := &manifest.Manifest{Version: 1, Plugins: []manifest.Plugin{}}
	if err := manifest.Save(manPath, m); err != nil {
		t.Fatal(err)
	}

	hooks := &mockHooks{cascadeOK: true}
	_, err := DeletePlugin(manPath, "nonexistent", hooks)
	if err == nil {
		t.Fatal("expected error for nonexistent plugin")
	}
}

// --- OnCascade error propagation ---

func TestDeleteMCP_CascadeError(t *testing.T) {
	dir := t.TempDir()
	manPath := filepath.Join(dir, "manifest.json")

	m := &manifest.Manifest{
		Version:    1,
		MCPServers: map[string]manifest.MCPServer{"foo": {Command: "cmd"}},
		Profiles:   map[string]manifest.Profile{"dev": {MCPServers: []string{"foo"}}},
	}
	if err := manifest.Save(manPath, m); err != nil {
		t.Fatal(err)
	}

	hooks := &mockHooks{cascadeErr: fmt.Errorf("user cancelled")}
	_, err := DeleteMCP(manPath, "foo", hooks)
	if err == nil {
		t.Fatal("expected cascade error to propagate")
	}
	if err.Error() != "user cancelled" {
		t.Errorf("error = %q, want %q", err.Error(), "user cancelled")
	}
}

func TestDeletePlugin_CascadeError(t *testing.T) {
	dir := t.TempDir()
	manPath := filepath.Join(dir, "manifest.json")

	m := &manifest.Manifest{
		Version:  1,
		Plugins:  []manifest.Plugin{{ID: "plug-a", Scope: "global", Enabled: true}},
		Profiles: map[string]manifest.Profile{"dev": {Plugins: []manifest.ProfilePlugin{{ID: "plug-a"}}}},
	}
	if err := manifest.Save(manPath, m); err != nil {
		t.Fatal(err)
	}

	hooks := &mockHooks{cascadeErr: fmt.Errorf("user cancelled")}
	_, err := DeletePlugin(manPath, "plug-a", hooks)
	if err == nil {
		t.Fatal("expected cascade error to propagate")
	}
}

func TestDeleteMarketplace_CascadeError(t *testing.T) {
	dir := t.TempDir()
	manPath := filepath.Join(dir, "manifest.json")

	m := &manifest.Manifest{
		Version:      1,
		Plugins:      []manifest.Plugin{{ID: "plug@my-mkt", Scope: "global", Enabled: true}},
		Marketplaces: map[string]manifest.Marketplace{"my-mkt": {Source: "github", Repo: "o/r"}},
	}
	if err := manifest.Save(manPath, m); err != nil {
		t.Fatal(err)
	}

	hooks := &mockHooks{cascadeErr: fmt.Errorf("user cancelled")}
	_, err := DeleteMarketplace(manPath, "my-mkt", hooks)
	if err == nil {
		t.Fatal("expected cascade error to propagate")
	}
}

// --- Exported helpers ---

func TestFindMCPProfileRefs(t *testing.T) {
	m := &manifest.Manifest{
		Version:    1,
		MCPServers: map[string]manifest.MCPServer{"foo": {Command: "cmd"}},
		Profiles: map[string]manifest.Profile{
			"dev":  {MCPServers: []string{"foo", "bar"}},
			"prod": {MCPServers: []string{"foo"}},
			"test": {MCPServers: []string{"baz"}},
		},
	}
	refs := FindMCPProfileRefs(m, "foo")
	if len(refs) != 2 {
		t.Fatalf("got %d refs, want 2", len(refs))
	}
	if refs[0] != "dev" || refs[1] != "prod" {
		t.Errorf("refs = %v, want [dev prod]", refs)
	}

	// No refs.
	refs = FindMCPProfileRefs(m, "nonexistent")
	if len(refs) != 0 {
		t.Errorf("got %d refs for nonexistent, want 0", len(refs))
	}
}

func TestFindPluginProfileRefs(t *testing.T) {
	m := &manifest.Manifest{
		Version: 1,
		Plugins: []manifest.Plugin{{ID: "plug-a"}, {ID: "plug-b"}},
		Profiles: map[string]manifest.Profile{
			"dev":  {Plugins: []manifest.ProfilePlugin{{ID: "plug-a"}, {ID: "plug-b"}}},
			"prod": {Plugins: []manifest.ProfilePlugin{{ID: "plug-b"}}},
		},
	}
	refs := FindPluginProfileRefs(m, "plug-a")
	if len(refs) != 1 || refs[0] != "dev" {
		t.Errorf("refs = %v, want [dev]", refs)
	}

	refs = FindPluginProfileRefs(m, "plug-b")
	if len(refs) != 2 {
		t.Fatalf("got %d refs, want 2", len(refs))
	}
	if refs[0] != "dev" || refs[1] != "prod" {
		t.Errorf("refs = %v, want [dev prod]", refs)
	}
}
