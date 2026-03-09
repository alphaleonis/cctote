package claude

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/alphaleonis/cctote/internal/manifest"
)

// fakeRunner records calls and returns pre-configured responses.
type fakeRunner struct {
	out  []byte
	err  error
	args []string // last call's args
}

func (f *fakeRunner) Run(_ context.Context, args ...string) ([]byte, error) {
	f.args = args
	return f.out, f.err
}

// --- ListPlugins ---

func TestListPluginsHappyPath(t *testing.T) {
	r := &fakeRunner{out: []byte(`[
		{"id":"plugin-a","scope":"user","enabled":true},
		{"id":"plugin-b","scope":"project","enabled":false}
	]`)}
	c := NewClient(r)

	got, err := c.ListPlugins(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []manifest.Plugin{
		{ID: "plugin-a", Scope: "user", Enabled: true},
		{ID: "plugin-b", Scope: "project", Enabled: false},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestListPluginsEmptyList(t *testing.T) {
	r := &fakeRunner{out: []byte(`[]`)}
	c := NewClient(r)

	got, err := c.ListPlugins(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty slice, got %d items", len(got))
	}
}

func TestListPluginsRunError(t *testing.T) {
	r := &fakeRunner{err: errors.New("connection refused")}
	c := NewClient(r)

	_, err := c.ListPlugins(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "listing plugins") {
		t.Errorf("error %q should wrap with context", err)
	}
}

func TestListPluginsMalformedJSON(t *testing.T) {
	r := &fakeRunner{out: []byte(`not json`)}
	c := NewClient(r)

	_, err := c.ListPlugins(context.Background())
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
	if !strings.Contains(err.Error(), "parsing plugin list") {
		t.Errorf("error %q should mention parsing", err)
	}
}

func TestListPluginsIgnoresExtraFields(t *testing.T) {
	r := &fakeRunner{out: []byte(`[{"id":"p","scope":"user","enabled":true,"installPath":"/tmp","extra":42}]`)}
	c := NewClient(r)

	got, err := c.ListPlugins(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got[0].ID != "p" {
		t.Errorf("got ID %q, want %q", got[0].ID, "p")
	}
}

func TestListPluginsVerifiesArgs(t *testing.T) {
	r := &fakeRunner{out: []byte(`[]`)}
	c := NewClient(r)

	_, _ = c.ListPlugins(context.Background())

	want := []string{"plugin", "list", "--json"}
	if !reflect.DeepEqual(r.args, want) {
		t.Errorf("args = %v, want %v", r.args, want)
	}
}

// --- InstallPlugin ---

func TestInstallPluginHappyPath(t *testing.T) {
	r := &fakeRunner{}
	c := NewClient(r)

	if err := c.InstallPlugin(context.Background(), "my-plugin", ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []string{"plugin", "install", "my-plugin"}
	if !reflect.DeepEqual(r.args, want) {
		t.Errorf("args = %v, want %v", r.args, want)
	}
}

func TestInstallPluginRunError(t *testing.T) {
	sentinel := errors.New("exit 1")
	r := &fakeRunner{err: sentinel}
	c := NewClient(r)

	err := c.InstallPlugin(context.Background(), "bad-plugin", "")
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("runner error not found in chain: %v", err)
	}
}

// --- SetPluginEnabled ---

func TestSetPluginEnabled(t *testing.T) {
	tests := []struct {
		name    string
		enabled bool
		verb    string
	}{
		{"enable", true, "enable"},
		{"disable", false, "disable"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &fakeRunner{}
			c := NewClient(r)

			if err := c.SetPluginEnabled(context.Background(), "my-plugin", tt.enabled, ""); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			want := []string{"plugin", tt.verb, "my-plugin"}
			if !reflect.DeepEqual(r.args, want) {
				t.Errorf("args = %v, want %v", r.args, want)
			}
		})
	}
}

func TestSetPluginEnabledRunError(t *testing.T) {
	sentinel := errors.New("exit 1")
	r := &fakeRunner{err: sentinel}
	c := NewClient(r)

	err := c.SetPluginEnabled(context.Background(), "p", true, "")
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("runner error not found in chain: %v", err)
	}
}

// --- UninstallPlugin ---

func TestUninstallPluginHappyPath(t *testing.T) {
	r := &fakeRunner{}
	c := NewClient(r)

	if err := c.UninstallPlugin(context.Background(), "my-plugin", ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []string{"plugin", "uninstall", "my-plugin"}
	if !reflect.DeepEqual(r.args, want) {
		t.Errorf("args = %v, want %v", r.args, want)
	}
}

func TestUninstallPluginRunError(t *testing.T) {
	sentinel := errors.New("exit 1")
	r := &fakeRunner{err: sentinel}
	c := NewClient(r)

	err := c.UninstallPlugin(context.Background(), "bad-plugin", "")
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("runner error not found in chain: %v", err)
	}
}

// --- ListMarketplaces ---

func TestListMarketplacesHappyPath(t *testing.T) {
	r := &fakeRunner{out: []byte(`[
		{"name":"claude-plugins-official","source":"github","repo":"anthropics/claude-plugins-official"},
		{"name":"local-dev","source":"directory","path":"/home/user/my-plugins"}
	]`)}
	c := NewClient(r)

	got, err := c.ListMarketplaces(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := map[string]manifest.Marketplace{
		"claude-plugins-official": {Source: "github", Repo: "anthropics/claude-plugins-official"},
		"local-dev":               {Source: "directory", Path: "/home/user/my-plugins"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}

	wantArgs := []string{"plugin", "marketplace", "list", "--json"}
	if !reflect.DeepEqual(r.args, wantArgs) {
		t.Errorf("args = %v, want %v", r.args, wantArgs)
	}
}

func TestListMarketplacesEmptyList(t *testing.T) {
	r := &fakeRunner{out: []byte(`[]`)}
	c := NewClient(r)

	got, err := c.ListMarketplaces(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty map, got %d items", len(got))
	}
}

func TestListMarketplacesRunError(t *testing.T) {
	r := &fakeRunner{err: errors.New("timeout")}
	c := NewClient(r)

	_, err := c.ListMarketplaces(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "listing marketplaces") {
		t.Errorf("error %q should wrap with context", err)
	}
}

// --- AddMarketplace ---

func TestAddMarketplaceHappyPath(t *testing.T) {
	r := &fakeRunner{}
	c := NewClient(r)

	if err := c.AddMarketplace(context.Background(), "https://example.com/mp"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []string{"plugin", "marketplace", "add", "https://example.com/mp"}
	if !reflect.DeepEqual(r.args, want) {
		t.Errorf("args = %v, want %v", r.args, want)
	}
}

func TestAddMarketplaceRunError(t *testing.T) {
	sentinel := errors.New("not found")
	r := &fakeRunner{err: sentinel}
	c := NewClient(r)

	err := c.AddMarketplace(context.Background(), "bad-source")
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("runner error not found in chain: %v", err)
	}
}

// --- RemoveMarketplace ---

func TestRemoveMarketplaceHappyPath(t *testing.T) {
	r := &fakeRunner{}
	c := NewClient(r)

	if err := c.RemoveMarketplace(context.Background(), "my-marketplace"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []string{"plugin", "marketplace", "remove", "my-marketplace"}
	if !reflect.DeepEqual(r.args, want) {
		t.Errorf("args = %v, want %v", r.args, want)
	}
}

func TestRemoveMarketplaceRunError(t *testing.T) {
	sentinel := errors.New("exit 1")
	r := &fakeRunner{err: sentinel}
	c := NewClient(r)

	err := c.RemoveMarketplace(context.Background(), "bad-mp")
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("runner error not found in chain: %v", err)
	}
}

// --- UpdateMarketplace ---

func TestUpdateMarketplaceHappyPath(t *testing.T) {
	r := &fakeRunner{}
	c := NewClient(r)

	if err := c.UpdateMarketplace(context.Background(), "my-marketplace"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []string{"plugin", "marketplace", "update", "my-marketplace"}
	if !reflect.DeepEqual(r.args, want) {
		t.Errorf("args = %v, want %v", r.args, want)
	}
}

func TestUpdateMarketplaceAllWhenEmpty(t *testing.T) {
	r := &fakeRunner{}
	c := NewClient(r)

	if err := c.UpdateMarketplace(context.Background(), ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []string{"plugin", "marketplace", "update"}
	if !reflect.DeepEqual(r.args, want) {
		t.Errorf("args = %v, want %v", r.args, want)
	}
}

func TestUpdateMarketplaceRunError(t *testing.T) {
	sentinel := errors.New("network error")
	r := &fakeRunner{err: sentinel}
	c := NewClient(r)

	err := c.UpdateMarketplace(context.Background(), "bad-mp")
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("runner error not found in chain: %v", err)
	}
}

// --- Scope parameter ---

func TestInstallPlugin_WithScope(t *testing.T) {
	r := &fakeRunner{}
	c := NewClient(r)

	if err := c.InstallPlugin(context.Background(), "my-plugin", "project"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []string{"plugin", "install", "-s", "project", "my-plugin"}
	if !reflect.DeepEqual(r.args, want) {
		t.Errorf("args = %v, want %v", r.args, want)
	}
}

func TestSetPluginEnabled_WithScope(t *testing.T) {
	r := &fakeRunner{}
	c := NewClient(r)

	if err := c.SetPluginEnabled(context.Background(), "my-plugin", true, "project"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []string{"plugin", "enable", "-s", "project", "my-plugin"}
	if !reflect.DeepEqual(r.args, want) {
		t.Errorf("args = %v, want %v", r.args, want)
	}
}

func TestUninstallPlugin_WithScope(t *testing.T) {
	r := &fakeRunner{}
	c := NewClient(r)

	if err := c.UninstallPlugin(context.Background(), "my-plugin", "project"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []string{"plugin", "uninstall", "-s", "project", "my-plugin"}
	if !reflect.DeepEqual(r.args, want) {
		t.Errorf("args = %v, want %v", r.args, want)
	}
}

// --- AddMcpServer ---

func TestAddMcpServer_Stdio(t *testing.T) {
	r := &fakeRunner{}
	c := NewClient(r)

	srv := manifest.MCPServer{
		Command: "npx",
		Args:    []string{"-y", "@upstash/context7-mcp"},
		Env:     map[string]string{"API_KEY": "secret"},
	}
	if err := c.AddMcpServer(context.Background(), "context7", srv, "project"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify key parts of the args: starts with [mcp add -s project name ...].
	args := r.args
	if len(args) < 6 {
		t.Fatalf("args too short: %v", args)
	}
	// Scope inserted after subcommand verb, before positional args.
	if args[0] != "mcp" || args[1] != "add" || args[2] != "-s" || args[3] != "project" || args[4] != "context7" {
		t.Errorf("expected [mcp add -s project context7 ...], got %v", args[:5])
	}
	// Should contain -e API_KEY=secret for env vars.
	foundEnv := false
	for i, a := range args {
		if a == "-e" && i+1 < len(args) && args[i+1] == "API_KEY=secret" {
			foundEnv = true
			break
		}
	}
	if !foundEnv {
		t.Errorf("args should contain -e API_KEY=secret: %v", args)
	}
	// Should end with -- npx -y @upstash/context7-mcp.
	dashIdx := -1
	for i, a := range args {
		if a == "--" {
			dashIdx = i
			break
		}
	}
	if dashIdx < 0 {
		t.Fatalf("args should contain --: %v", args)
	}
	cmdArgs := args[dashIdx+1:]
	wantCmd := []string{"npx", "-y", "@upstash/context7-mcp"}
	if !reflect.DeepEqual(cmdArgs, wantCmd) {
		t.Errorf("command args = %v, want %v", cmdArgs, wantCmd)
	}
}

func TestAddMcpServer_SSE(t *testing.T) {
	r := &fakeRunner{}
	c := NewClient(r)

	srv := manifest.MCPServer{
		Type: "sse",
		URL:  "https://mcp.example.com/sse",
	}
	if err := c.AddMcpServer(context.Background(), "remote", srv, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []string{"mcp", "add", "--transport", "sse", "remote", "https://mcp.example.com/sse"}
	if !reflect.DeepEqual(r.args, want) {
		t.Errorf("args = %v, want %v", r.args, want)
	}
}

func TestAddMcpServer_RunError(t *testing.T) {
	sentinel := errors.New("exit 1")
	r := &fakeRunner{err: sentinel}
	c := NewClient(r)

	err := c.AddMcpServer(context.Background(), "srv", manifest.MCPServer{Command: "cmd"}, "")
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("runner error not found in chain: %v", err)
	}
}

func TestAddMcpServer_UnknownTransport(t *testing.T) {
	r := &fakeRunner{}
	c := NewClient(r)

	srv := manifest.MCPServer{
		Type: "webscoket", // typo — should be rejected
		URL:  "https://mcp.example.com",
	}
	err := c.AddMcpServer(context.Background(), "bad-transport", srv, "")
	if err == nil {
		t.Fatal("expected error for unknown transport type")
	}
	if !strings.Contains(err.Error(), "unsupported transport") {
		t.Errorf("error should mention unsupported transport: %v", err)
	}
	// Runner should NOT have been called.
	if r.args != nil {
		t.Errorf("runner should not be called for unknown transport, got args: %v", r.args)
	}
}

// --- RemoveMcpServer ---

func TestRemoveMcpServer(t *testing.T) {
	r := &fakeRunner{}
	c := NewClient(r)

	if err := c.RemoveMcpServer(context.Background(), "my-server", "project"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []string{"mcp", "remove", "-s", "project", "my-server"}
	if !reflect.DeepEqual(r.args, want) {
		t.Errorf("args = %v, want %v", r.args, want)
	}
}

func TestRemoveMcpServer_RunError(t *testing.T) {
	sentinel := errors.New("exit 1")
	r := &fakeRunner{err: sentinel}
	c := NewClient(r)

	err := c.RemoveMcpServer(context.Background(), "bad-srv", "")
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("runner error not found in chain: %v", err)
	}
}
