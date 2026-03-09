package manifest

import "testing"

func TestMCPServersEqual(t *testing.T) {
	base := MCPServer{
		Command: "npx",
		Args:    []string{"-y", "server"},
		Env:     map[string]string{"KEY": "val"},
	}
	same := MCPServer{
		Command: "npx",
		Args:    []string{"-y", "server"},
		Env:     map[string]string{"KEY": "val"},
	}
	different := MCPServer{
		Command: "npx",
		Args:    []string{"-y", "other-server"},
	}

	if !MCPServersEqual(base, same) {
		t.Error("expected equal servers to be equal")
	}
	if MCPServersEqual(base, different) {
		t.Error("expected different servers to not be equal")
	}
}

func TestMCPServersEqualFieldByField(t *testing.T) {
	base := MCPServer{
		Type:    "stdio",
		Command: "npx",
		Args:    []string{"-y", "server"},
		CWD:     "/work",
		Env:     map[string]string{"K": "V"},
		URL:     "https://example.com",
		Headers: map[string]string{"Auth": "token"},
		OAuth:   &MCPOAuth{ClientID: "cid", CallbackPort: 8080},
	}

	tests := []struct {
		name string
		mod  func(MCPServer) MCPServer
	}{
		{"Type", func(s MCPServer) MCPServer { s.Type = "sse"; return s }},
		{"Command", func(s MCPServer) MCPServer { s.Command = "node"; return s }},
		{"Args", func(s MCPServer) MCPServer { s.Args = []string{"other"}; return s }},
		{"CWD", func(s MCPServer) MCPServer { s.CWD = "/other"; return s }},
		{"Env", func(s MCPServer) MCPServer { s.Env = map[string]string{"X": "Y"}; return s }},
		{"URL", func(s MCPServer) MCPServer { s.URL = "https://other.com"; return s }},
		{"Headers", func(s MCPServer) MCPServer { s.Headers = map[string]string{"X": "Y"}; return s }},
		{"OAuth.ClientID", func(s MCPServer) MCPServer { s.OAuth = &MCPOAuth{ClientID: "other"}; return s }},
		{"OAuth nil vs non-nil", func(s MCPServer) MCPServer { s.OAuth = nil; return s }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			modified := tt.mod(base)
			if MCPServersEqual(base, modified) {
				t.Errorf("expected servers differing in %s to be unequal", tt.name)
			}
		})
	}
}

func TestMCPServersEqualNilVsEmpty(t *testing.T) {
	a := MCPServer{Command: "foo", Args: nil, Env: nil, Headers: nil}
	b := MCPServer{Command: "foo", Args: []string{}, Env: map[string]string{}, Headers: map[string]string{}}
	if !MCPServersEqual(a, b) {
		t.Error("nil and empty should be equal")
	}
}

func TestMCPServersEqualOAuth(t *testing.T) {
	withOAuth := MCPServer{OAuth: &MCPOAuth{ClientID: "abc", CallbackPort: 8080}}
	sameOAuth := MCPServer{OAuth: &MCPOAuth{ClientID: "abc", CallbackPort: 8080}}
	diffID := MCPServer{OAuth: &MCPOAuth{ClientID: "xyz", CallbackPort: 8080}}
	diffPort := MCPServer{OAuth: &MCPOAuth{ClientID: "abc", CallbackPort: 9090}}
	noOAuth := MCPServer{}

	if !MCPServersEqual(withOAuth, sameOAuth) {
		t.Error("same OAuth should be equal")
	}
	if MCPServersEqual(withOAuth, diffID) {
		t.Error("different ClientID should not be equal")
	}
	if MCPServersEqual(withOAuth, diffPort) {
		t.Error("different CallbackPort should not be equal")
	}
	if MCPServersEqual(withOAuth, noOAuth) {
		t.Error("OAuth present vs nil should not be equal")
	}
	if !MCPServersEqual(noOAuth, noOAuth) {
		t.Error("both nil OAuth should be equal")
	}
}

func TestPluginsEqual(t *testing.T) {
	base := Plugin{ID: "my-plugin", Scope: "user", Enabled: true}

	tests := []struct {
		name string
		a, b Plugin
		want bool
	}{
		{"identical", base, base, true},
		{"different ID", base, Plugin{ID: "other", Scope: "user", Enabled: true}, false},
		{"different scope", base, Plugin{ID: "my-plugin", Scope: "project", Enabled: true}, false},
		{"different enabled", base, Plugin{ID: "my-plugin", Scope: "user", Enabled: false}, false},
		{"zero values", Plugin{}, Plugin{}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := PluginsEqual(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("PluginsEqual() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMarketplacesEqual(t *testing.T) {
	tests := []struct {
		name string
		a, b Marketplace
		want bool
	}{
		{
			name: "identical github",
			a:    Marketplace{Source: "github", Repo: "owner/repo"},
			b:    Marketplace{Source: "github", Repo: "owner/repo"},
			want: true,
		},
		{
			name: "different repo",
			a:    Marketplace{Source: "github", Repo: "owner/repo"},
			b:    Marketplace{Source: "github", Repo: "owner/other"},
			want: false,
		},
		{
			name: "different source type",
			a:    Marketplace{Source: "github", Repo: "owner/repo"},
			b:    Marketplace{Source: "git", URL: "https://example.com"},
			want: false,
		},
		{
			name: "identical directory",
			a:    Marketplace{Source: "directory", Path: "/tmp/plugins"},
			b:    Marketplace{Source: "directory", Path: "/tmp/plugins"},
			want: true,
		},
		{
			name: "different path",
			a:    Marketplace{Source: "directory", Path: "/tmp/a"},
			b:    Marketplace{Source: "directory", Path: "/tmp/b"},
			want: false,
		},
		{
			name: "identical git",
			a:    Marketplace{Source: "git", URL: "https://example.com/repo.git"},
			b:    Marketplace{Source: "git", URL: "https://example.com/repo.git"},
			want: true,
		},
		{
			name: "different git URL",
			a:    Marketplace{Source: "git", URL: "https://example.com/a.git"},
			b:    Marketplace{Source: "git", URL: "https://example.com/b.git"},
			want: false,
		},
		{
			name: "stray cross-type field",
			a:    Marketplace{Source: "github", Repo: "owner/repo"},
			b:    Marketplace{Source: "github", Repo: "owner/repo", URL: "https://stray"},
			want: false,
		},
		{
			name: "zero values",
			a:    Marketplace{},
			b:    Marketplace{},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MarketplacesEqual(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("MarketplacesEqual() = %v, want %v", got, tt.want)
			}
		})
	}
}
