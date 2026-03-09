package engine

import (
	"testing"

	"github.com/alphaleonis/cctote/internal/manifest"
)

func TestComputeSyncState_AllStatuses(t *testing.T) {
	m := &manifest.Manifest{
		MCPServers: map[string]manifest.MCPServer{
			"synced-srv":    {Command: "cmd1"},
			"diff-srv":      {Command: "cmd2"},
			"manifest-only": {Command: "cmd3"},
		},
		Plugins: []manifest.Plugin{
			{ID: "synced-plug", Scope: "global", Enabled: true},
			{ID: "diff-plug", Scope: "global", Enabled: true},
			{ID: "manifest-plug", Scope: "project", Enabled: false},
		},
		Marketplaces: map[string]manifest.Marketplace{
			"synced-mkt":   {Source: "github", Repo: "owner/repo"},
			"manifest-mkt": {Source: "git", URL: "https://example.com"},
		},
	}

	claudeMCP := map[string]manifest.MCPServer{
		"synced-srv":  {Command: "cmd1"},
		"diff-srv":    {Command: "cmd2-changed"},
		"claude-only": {Command: "cmd4"},
	}
	claudePlugins := []manifest.Plugin{
		{ID: "synced-plug", Scope: "global", Enabled: true},
		{ID: "diff-plug", Scope: "global", Enabled: false}, // enabled differs
		{ID: "claude-plug", Scope: "project", Enabled: true},
	}
	claudeMarketplaces := map[string]manifest.Marketplace{
		"synced-mkt": {Source: "github", Repo: "owner/repo"},
		"claude-mkt": {Source: "directory", Path: "/tmp/mp"},
	}

	s := ComputeSyncState(m, claudeMCP, claudePlugins, claudeMarketplaces)

	// MCP servers
	assertStatus(t, s.MCPSync, "synced-srv", Synced)
	assertStatus(t, s.MCPSync, "diff-srv", Different)
	assertStatus(t, s.MCPSync, "manifest-only", LeftOnly)
	assertStatus(t, s.MCPSync, "claude-only", RightOnly)

	// Plugins
	assertStatus(t, s.PlugSync, "synced-plug", Synced)
	assertStatus(t, s.PlugSync, "diff-plug", Different)
	assertStatus(t, s.PlugSync, "manifest-plug", LeftOnly)
	assertStatus(t, s.PlugSync, "claude-plug", RightOnly)

	// Marketplaces
	assertStatus(t, s.MktSync, "synced-mkt", Synced)
	assertStatus(t, s.MktSync, "manifest-mkt", LeftOnly)
	assertStatus(t, s.MktSync, "claude-mkt", RightOnly)

	// Counts
	synced, different, leftOnly, rightOnly := s.Counts()
	if synced != 3 {
		t.Errorf("synced = %d, want 3", synced)
	}
	if different != 2 {
		t.Errorf("different = %d, want 2", different)
	}
	if leftOnly != 3 {
		t.Errorf("leftOnly = %d, want 3", leftOnly)
	}
	if rightOnly != 3 {
		t.Errorf("rightOnly = %d, want 3", rightOnly)
	}
}

func TestComputeSyncState_EmptyInputs(t *testing.T) {
	m := &manifest.Manifest{}
	s := ComputeSyncState(m, nil, nil, nil)

	if len(s.MCPSync) != 0 {
		t.Errorf("MCPSync should be empty, got %d", len(s.MCPSync))
	}
	if len(s.PlugSync) != 0 {
		t.Errorf("PlugSync should be empty, got %d", len(s.PlugSync))
	}
	if len(s.MktSync) != 0 {
		t.Errorf("MktSync should be empty, got %d", len(s.MktSync))
	}

	synced, different, leftOnly, rightOnly := s.Counts()
	if synced+different+leftOnly+rightOnly != 0 {
		t.Error("all counts should be 0 for empty inputs")
	}
}

func TestComputeSyncState_NilMaps(t *testing.T) {
	// Manifest with nil maps should not panic.
	m := &manifest.Manifest{
		MCPServers:   nil,
		Plugins:      nil,
		Marketplaces: nil,
	}
	claudeMCP := map[string]manifest.MCPServer{"srv": {Command: "cmd"}}
	s := ComputeSyncState(m, claudeMCP, nil, nil)

	assertStatus(t, s.MCPSync, "srv", RightOnly)
}

func TestPluginMap(t *testing.T) {
	plugins := []manifest.Plugin{
		{ID: "a", Scope: "global", Enabled: true},
		{ID: "b", Scope: "project", Enabled: false},
	}
	m := PluginMap(plugins)
	if len(m) != 2 {
		t.Fatalf("len = %d, want 2", len(m))
	}
	if m["a"].Scope != "global" {
		t.Errorf("a.Scope = %q, want %q", m["a"].Scope, "global")
	}
	if m["b"].Enabled != false {
		t.Error("b.Enabled should be false")
	}
}

func TestMarketplaceFromPluginID(t *testing.T) {
	tests := []struct {
		id   string
		want string
	}{
		{"plug@mkt", "mkt"},
		{"plug@mkt@extra", "mkt@extra"},
		{"standalone", ""},
		{"", ""},
	}
	for _, tt := range tests {
		got := MarketplaceFromPluginID(tt.id)
		if got != tt.want {
			t.Errorf("MarketplaceFromPluginID(%q) = %q, want %q", tt.id, got, tt.want)
		}
	}
}

func assertStatus(t *testing.T, m map[string]ItemSync, name string, want SyncStatus) {
	t.Helper()
	got, ok := m[name]
	if !ok {
		t.Errorf("missing key %q", name)
		return
	}
	if got.Status != want {
		t.Errorf("%q: status = %d, want %d", name, got.Status, want)
	}
}
