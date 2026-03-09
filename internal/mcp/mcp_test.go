package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/alphaleonis/cctote/internal/manifest"
)

func TestRoundTrip(t *testing.T) {
	servers := map[string]manifest.MCPServer{
		"context7": {
			Command: "npx",
			Args:    []string{"-y", "@upstash/context7-mcp"},
		},
		"remote-sse": {
			Type:    "sse",
			URL:     "https://mcp.example.com/sse?foo=1&bar=2",
			Headers: map[string]string{"Authorization": "Bearer tok"},
			OAuth:   &manifest.MCPOAuth{ClientID: "my-client", CallbackPort: 9999},
		},
	}

	path := filepath.Join(t.TempDir(), ".claude.json")
	if err := WriteMcpServers(path, servers); err != nil {
		t.Fatalf("WriteMcpServers: %v", err)
	}

	got, err := ReadMcpServers(path)
	if err != nil {
		t.Fatalf("ReadMcpServers: %v", err)
	}

	if !reflect.DeepEqual(servers, got) {
		t.Errorf("round-trip mismatch:\n  want: %+v\n  got:  %+v", servers, got)
	}
}

func TestReadMcpServersMissingFile(t *testing.T) {
	got, err := ReadMcpServers(filepath.Join(t.TempDir(), "nonexistent.json"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty map, got %v", got)
	}
}

func TestReadMcpServersMissingKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".claude.json")
	if err := os.WriteFile(path, []byte(`{"numStartups": 42}`), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := ReadMcpServers(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty map, got %v", got)
	}
}

func TestReadMcpServersNullKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".claude.json")
	if err := os.WriteFile(path, []byte(`{"mcpServers": null}`), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := ReadMcpServers(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty map, got %v", got)
	}
}

func TestReadMcpServersMalformedJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".claude.json")
	if err := os.WriteFile(path, []byte(`{not json`), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := ReadMcpServers(path)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "parsing claude config") {
		t.Errorf("error %q does not contain %q", err.Error(), "parsing claude config")
	}
}

func TestWriteMcpServersPreservesOtherKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".claude.json")
	original := `{
  "numStartups": 42,
  "autoUpdaterStatus": "enabled",
  "mcpServers": {},
  "preferredNotifChannel": "toast"
}
`
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	servers := map[string]manifest.MCPServer{
		"test": {Command: "echo"},
	}
	if err := WriteMcpServers(path, servers); err != nil {
		t.Fatalf("WriteMcpServers: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	var parsed map[string]json.RawMessage
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}

	// Verify preserved keys exist with correct values.
	wantKeys := map[string]string{
		"numStartups":           "42",
		"autoUpdaterStatus":     `"enabled"`,
		"preferredNotifChannel": `"toast"`,
	}
	for key, wantVal := range wantKeys {
		raw, ok := parsed[key]
		if !ok {
			t.Errorf("output missing preserved key %q", key)
			continue
		}
		if string(raw) != wantVal {
			t.Errorf("key %q = %s, want %s", key, raw, wantVal)
		}
	}
}

func TestWriteMcpServersPreservesKeyOrder(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".claude.json")
	original := `{
  "zebra": 1,
  "alpha": 2,
  "mcpServers": {},
  "middle": 3,
  "omega": 4
}
`
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	servers := map[string]manifest.MCPServer{
		"test": {Command: "echo"},
	}
	if err := WriteMcpServers(path, servers); err != nil {
		t.Fatalf("WriteMcpServers: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)

	// Verify ordering: zebra < alpha < mcpServers < middle < omega
	keys := []string{"zebra", "alpha", "mcpServers", "middle", "omega"}
	lastIdx := -1
	for _, key := range keys {
		idx := strings.Index(content, `"`+key+`"`)
		if idx == -1 {
			t.Fatalf("key %q not found in output", key)
		}
		if idx <= lastIdx {
			t.Errorf("key %q at index %d is not after previous key at index %d", key, idx, lastIdx)
		}
		lastIdx = idx
	}
}

func TestWriteMcpServersMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".claude.json")

	servers := map[string]manifest.MCPServer{
		"test": {Command: "echo"},
	}
	if err := WriteMcpServers(path, servers); err != nil {
		t.Fatalf("WriteMcpServers: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	var top map[string]json.RawMessage
	if err := json.Unmarshal(data, &top); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	raw, ok := top["mcpServers"]
	if !ok {
		t.Fatal("output missing mcpServers key")
	}

	var got map[string]manifest.MCPServer
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("parsing mcpServers: %v", err)
	}
	if srv, ok := got["test"]; !ok {
		t.Error("mcpServers missing 'test' entry")
	} else if srv.Command != "echo" {
		t.Errorf("test server command = %q, want %q", srv.Command, "echo")
	}
}

func TestWriteMcpServersCreatesDirectories(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a", "b", "c", ".claude.json")

	servers := map[string]manifest.MCPServer{
		"test": {Command: "echo"},
	}
	if err := WriteMcpServers(path, servers); err != nil {
		t.Fatalf("WriteMcpServers: %v", err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Errorf("expected file to exist at %s: %v", path, err)
	}
}

func TestWriteMcpServersAtomicNoTmpLeftover(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".claude.json")

	// Write initial content.
	original := `{"numStartups": 42, "mcpServers": {}}`
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	// Perform a write.
	servers := map[string]manifest.MCPServer{
		"test": {Command: "echo"},
	}
	if err := WriteMcpServers(path, servers); err != nil {
		t.Fatalf("WriteMcpServers: %v", err)
	}

	// No temp files should linger in the directory.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), ".tmp") {
			t.Errorf("temporary file %s should not exist after successful write", e.Name())
		}
	}

	// Original file should have the new content.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"test"`) {
		t.Error("output missing new server entry")
	}
}

func TestDefaultPath(t *testing.T) {
	got, err := DefaultPath()
	if err != nil {
		t.Fatalf("DefaultPath: %v", err)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("os.UserHomeDir: %v", err)
	}
	want := filepath.Join(home, ".claude.json")
	if got != want {
		t.Errorf("DefaultPath = %q, want %q", got, want)
	}
}

func TestDefaultPathIgnoresXDG(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/custom/config")

	got, err := DefaultPath()
	if err != nil {
		t.Fatalf("DefaultPath: %v", err)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("os.UserHomeDir: %v", err)
	}
	want := filepath.Join(home, ".claude.json")
	if got != want {
		t.Errorf("DefaultPath = %q, want %q (should ignore XDG)", got, want)
	}
}

func TestTrailingNewline(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".claude.json")

	servers := map[string]manifest.MCPServer{
		"test": {Command: "echo"},
	}
	if err := WriteMcpServers(path, servers); err != nil {
		t.Fatalf("WriteMcpServers: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(string(data), "\n") {
		t.Error("output does not end with trailing newline")
	}
}

func TestNoHTMLEscaping(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".claude.json")

	servers := map[string]manifest.MCPServer{
		"test": {
			Type: "sse",
			URL:  "https://example.com/api?foo=1&bar=2",
		},
	}
	if err := WriteMcpServers(path, servers); err != nil {
		t.Fatalf("WriteMcpServers: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)

	if strings.Contains(content, `\u0026`) {
		t.Error("output contains escaped ampersand (\\u0026), expected literal &")
	}
	if !strings.Contains(content, `foo=1&bar=2`) {
		t.Error("output does not contain literal ampersand in URL")
	}
}
