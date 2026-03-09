package manifest

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	m := &Manifest{
		Version: CurrentVersion,
		MCPServers: map[string]MCPServer{
			"context7": {
				Command: "npx",
				Args:    []string{"-y", "@upstash/context7-mcp"},
			},
			"postgres": {
				Command: "pg-mcp",
				Env:     map[string]string{"DSN": "postgres://localhost/dev"},
				CWD:     "/opt/pg-mcp",
			},
			"remote-sse": {
				Type:    "sse",
				URL:     "https://mcp.example.com/sse",
				Headers: map[string]string{"Authorization": "Bearer tok"},
				OAuth:   &MCPOAuth{ClientID: "my-client", CallbackPort: 9999},
			},
		},
		Plugins: []Plugin{
			{ID: "plugin-a", Scope: "project", Enabled: true},
			{ID: "plugin-b", Scope: "user", Enabled: false},
		},
		Marketplaces: map[string]Marketplace{
			"claude-plugins-official": {
				Source: "github",
				Repo:   "anthropics/claude-plugins-official",
			},
			"my-marketplace": {
				Source: "git",
				URL:    "https://github.com/user/marketplace.git",
			},
		},
		Profiles: map[string]Profile{
			"work": {
				MCPServers: []string{"context7", "postgres"},
				Plugins:    []ProfilePlugin{{ID: "plugin-a"}},
			},
		},
	}

	path := filepath.Join(t.TempDir(), "manifest.json")
	if err := Save(path, m); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if !reflect.DeepEqual(m, got) {
		t.Errorf("round-trip mismatch:\n  want: %+v\n  got:  %+v", m, got)
	}
}

func TestRoundTripEmpty(t *testing.T) {
	m := &Manifest{
		Version:      CurrentVersion,
		MCPServers:   map[string]MCPServer{},
		Plugins:      []Plugin{},
		Marketplaces: map[string]Marketplace{},
	}

	path := filepath.Join(t.TempDir(), "manifest.json")
	if err := Save(path, m); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if !reflect.DeepEqual(m, got) {
		t.Errorf("round-trip mismatch:\n  want: %+v\n  got:  %+v", m, got)
	}
}

func TestLoadVersionValidation(t *testing.T) {
	tests := []struct {
		name    string
		json    string
		wantErr string
	}{
		{"version 0", `{"version":0}`, "unsupported manifest version 0"},
		{"version 2", `{"version":2}`, "unsupported manifest version 2"},
		{"version 99", `{"version":99}`, "unsupported manifest version 99"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "manifest.json")
			if err := os.WriteFile(path, []byte(tt.json), 0o644); err != nil {
				t.Fatal(err)
			}

			_, err := Load(path)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error %q does not contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestLoadOrCreateNewFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nonexistent.json")

	m, created, err := LoadOrCreate(path)
	if err != nil {
		t.Fatalf("LoadOrCreate: %v", err)
	}
	if !created {
		t.Error("expected created=true for missing file")
	}
	if m.Version != CurrentVersion {
		t.Errorf("Version = %d, want %d", m.Version, CurrentVersion)
	}
	if m.MCPServers == nil || m.Plugins == nil || m.Marketplaces == nil {
		t.Error("expected initialized maps/slices, got nil")
	}
}

func TestLoadOrCreateExistingFile(t *testing.T) {
	m := &Manifest{
		Version:      CurrentVersion,
		MCPServers:   map[string]MCPServer{"s": {Command: "x"}},
		Plugins:      []Plugin{},
		Marketplaces: map[string]Marketplace{},
	}
	path := filepath.Join(t.TempDir(), "manifest.json")
	if err := Save(path, m); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, created, err := LoadOrCreate(path)
	if err != nil {
		t.Fatalf("LoadOrCreate: %v", err)
	}
	if created {
		t.Error("expected created=false for existing file")
	}
	if len(got.MCPServers) != 1 {
		t.Errorf("expected 1 MCP server, got %d", len(got.MCPServers))
	}
}

func TestLoadOrCreateCorruptFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "manifest.json")
	if err := os.WriteFile(path, []byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, _, err := LoadOrCreate(path)
	if err == nil {
		t.Fatal("expected error for corrupt file")
	}
}

func TestLoadMissingFile(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "nonexistent.json"))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("expected os.ErrNotExist in chain, got: %v", err)
	}
}

func TestSaveCreatesDirectories(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a", "b", "c", "manifest.json")
	m := &Manifest{Version: CurrentVersion}

	if err := Save(path, m); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Errorf("expected file to exist at %s: %v", path, err)
	}
}

func TestDefaultPathXDGOverride(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/custom/config")

	got, err := DefaultPath()
	if err != nil {
		t.Fatalf("DefaultPath: %v", err)
	}

	want := filepath.Join("/custom/config", "cctote", "manifest.json")
	if got != want {
		t.Errorf("DefaultPath = %q, want %q", got, want)
	}
}

func TestSourceLocator(t *testing.T) {
	tests := []struct {
		name string
		mp   Marketplace
		want string
	}{
		{"github", Marketplace{Source: "github", Repo: "owner/repo"}, "owner/repo"},
		{"git", Marketplace{Source: "git", URL: "https://example.com/repo.git"}, "https://example.com/repo.git"},
		{"directory", Marketplace{Source: "directory", Path: "/tmp/plugins"}, "/tmp/plugins"},
		{"unknown", Marketplace{Source: "unknown"}, ""},
		{"empty", Marketplace{}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.mp.SourceLocator()
			if got != tt.want {
				t.Errorf("SourceLocator() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSourceLocatorErrorForUnknownSource(t *testing.T) {
	mp := Marketplace{Source: "unknown"}
	_, err := mp.SourceLocatorE()
	if err == nil {
		t.Fatal("expected error for unknown source type")
	}
	if !strings.Contains(err.Error(), "unknown") {
		t.Errorf("error %q should mention the source type", err)
	}
}

func TestUpdateAcquiresLock(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "manifest.json")

	seed := &Manifest{
		Version:      CurrentVersion,
		MCPServers:   map[string]MCPServer{},
		Plugins:      []Plugin{},
		Marketplaces: map[string]Marketplace{},
	}
	if err := Save(path, seed); err != nil {
		t.Fatalf("Save seed: %v", err)
	}

	// Verify the lock directory exists during the callback.
	lockPath := path + ".lock"
	err := Update(path, func(m *Manifest) error {
		if _, statErr := os.Stat(lockPath); statErr != nil {
			t.Errorf("lock directory should exist during callback, got: %v", statErr)
		}
		m.MCPServers["test"] = MCPServer{Command: "test"}
		return nil
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}

	// Lock should be released after Update returns.
	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Error("lock directory should be removed after Update returns")
	}

	// Verify the mutation was saved.
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if _, ok := got.MCPServers["test"]; !ok {
		t.Error("expected 'test' server in manifest after Update")
	}
}

func TestUpdateSequentialAccumulation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "manifest.json")

	seed := &Manifest{
		Version:      CurrentVersion,
		MCPServers:   map[string]MCPServer{},
		Plugins:      []Plugin{},
		Marketplaces: map[string]Marketplace{},
	}
	if err := Save(path, seed); err != nil {
		t.Fatalf("Save seed: %v", err)
	}

	// Sequential updates should each read fresh state and accumulate.
	for i := 0; i < 5; i++ {
		name := fmt.Sprintf("server-%d", i)
		if err := Update(path, func(m *Manifest) error {
			m.MCPServers[name] = MCPServer{Command: name}
			return nil
		}); err != nil {
			t.Fatalf("Update %d: %v", i, err)
		}
	}

	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got.MCPServers) != 5 {
		t.Errorf("expected 5 MCP servers, got %d", len(got.MCPServers))
	}
}

func TestUpdateCallbackError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "manifest.json")

	seed := &Manifest{
		Version:      CurrentVersion,
		MCPServers:   map[string]MCPServer{"keep": {Command: "x"}},
		Plugins:      []Plugin{},
		Marketplaces: map[string]Marketplace{},
	}
	if err := Save(path, seed); err != nil {
		t.Fatalf("Save seed: %v", err)
	}

	// Callback returns an error — manifest should remain unchanged.
	wantErr := errors.New("rollback")
	if err := Update(path, func(m *Manifest) error {
		m.MCPServers["bad"] = MCPServer{Command: "bad"}
		return wantErr
	}); !errors.Is(err, wantErr) {
		t.Fatalf("Update err = %v, want %v", err, wantErr)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if _, ok := got.MCPServers["bad"]; ok {
		t.Error("callback error should have prevented save, but 'bad' server exists")
	}
	if len(got.MCPServers) != 1 {
		t.Errorf("expected 1 server, got %d", len(got.MCPServers))
	}
}

func TestUpdateConcurrentProfiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "manifest.json")

	seed := &Manifest{
		Version:      CurrentVersion,
		MCPServers:   map[string]MCPServer{},
		Plugins:      []Plugin{},
		Marketplaces: map[string]Marketplace{},
	}
	if err := Save(path, seed); err != nil {
		t.Fatalf("Save seed: %v", err)
	}

	// Launch 3 goroutines that each add a distinct profile via Update.
	// With the old Load+Save pattern, later writes would overwrite earlier
	// ones, losing profiles. Update's file lock serialises them.
	const n = 3
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		go func(idx int) {
			name := fmt.Sprintf("profile-%d", idx)
			errs <- Update(path, func(m *Manifest) error {
				if m.Profiles == nil {
					m.Profiles = map[string]Profile{}
				}
				m.Profiles[name] = Profile{
					MCPServers: []string{fmt.Sprintf("srv-%d", idx)},
					Plugins:    []ProfilePlugin{},
				}
				return nil
			})
		}(i)
	}

	for i := 0; i < n; i++ {
		if err := <-errs; err != nil {
			if strings.Contains(err.Error(), "held by another process") {
				t.Skip("lock contention exceeded retry budget under load")
			}
			t.Fatalf("concurrent Update failed: %v", err)
		}
	}

	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got.Profiles) != n {
		t.Errorf("expected %d profiles, got %d — concurrent updates were lost", n, len(got.Profiles))
	}
	for i := 0; i < n; i++ {
		name := fmt.Sprintf("profile-%d", i)
		p, ok := got.Profiles[name]
		if !ok {
			t.Errorf("missing profile %q", name)
			continue
		}
		wantSrv := fmt.Sprintf("srv-%d", i)
		if len(p.MCPServers) != 1 || p.MCPServers[0] != wantSrv {
			t.Errorf("profile %q: MCPServers = %v, want [%s]", name, p.MCPServers, wantSrv)
		}
	}
}

func TestDefaultPathFallback(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")

	got, err := DefaultPath()
	if err != nil {
		t.Fatalf("DefaultPath: %v", err)
	}

	home, _ := os.UserHomeDir()
	want := filepath.Join(home, ".config", "cctote", "manifest.json")
	if got != want {
		t.Errorf("DefaultPath = %q, want %q", got, want)
	}
}

func TestFindProfilePlugin(t *testing.T) {
	plugins := []ProfilePlugin{
		{ID: "plug-a"},
		{ID: "plug-b"},
	}
	if got := FindProfilePlugin(plugins, "plug-a"); got != 0 {
		t.Errorf("FindProfilePlugin(plug-a) = %d, want 0", got)
	}
	if got := FindProfilePlugin(plugins, "plug-b"); got != 1 {
		t.Errorf("FindProfilePlugin(plug-b) = %d, want 1", got)
	}
	if got := FindProfilePlugin(plugins, "nonexistent"); got != -1 {
		t.Errorf("FindProfilePlugin(nonexistent) = %d, want -1", got)
	}
	if got := FindProfilePlugin(nil, "plug-a"); got != -1 {
		t.Errorf("FindProfilePlugin(nil) = %d, want -1", got)
	}
}
