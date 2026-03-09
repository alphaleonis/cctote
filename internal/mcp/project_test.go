package mcp

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/alphaleonis/cctote/internal/manifest"
)

func TestReadProjectMcpServersMissingFile(t *testing.T) {
	got, err := ReadProjectMcpServers(filepath.Join(t.TempDir(), ".mcp.json"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty map, got %v", got)
	}
}

func TestReadProjectMcpServersValidFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".mcp.json")
	content := `{
  "mcpServers": {
    "context7": {
      "command": "npx",
      "args": ["-y", "@upstash/context7-mcp"]
    },
    "remote": {
      "type": "sse",
      "url": "https://mcp.example.com/sse"
    }
  }
}`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := ReadProjectMcpServers(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := map[string]manifest.MCPServer{
		"context7": {
			Command: "npx",
			Args:    []string{"-y", "@upstash/context7-mcp"},
		},
		"remote": {
			Type: "sse",
			URL:  "https://mcp.example.com/sse",
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("mismatch:\n  got:  %+v\n  want: %+v", got, want)
	}
}

func TestReadProjectMcpServersEnvTemplates(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".mcp.json")
	content := `{
  "mcpServers": {
    "my-server": {
      "command": "node",
      "args": ["server.js"],
      "env": {
        "API_KEY": "${API_KEY}",
        "DB_URL": "${DB_URL:-postgres://localhost/dev}"
      }
    }
  }
}`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := ReadProjectMcpServers(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	srv, ok := got["my-server"]
	if !ok {
		t.Fatal("expected 'my-server' entry")
	}
	if srv.Command != "node" {
		t.Errorf("Command = %q, want %q", srv.Command, "node")
	}
	if !reflect.DeepEqual(srv.Args, []string{"server.js"}) {
		t.Errorf("Args = %v, want [server.js]", srv.Args)
	}
	// Env template strings must be preserved as-is (not expanded).
	if srv.Env["API_KEY"] != "${API_KEY}" {
		t.Errorf("API_KEY = %q, want ${API_KEY}", srv.Env["API_KEY"])
	}
	if srv.Env["DB_URL"] != "${DB_URL:-postgres://localhost/dev}" {
		t.Errorf("DB_URL = %q, want ${DB_URL:-postgres://localhost/dev}", srv.Env["DB_URL"])
	}
}

func TestReadProjectMcpServersMalformedJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".mcp.json")
	if err := os.WriteFile(path, []byte(`{not json`), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := ReadProjectMcpServers(path)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "parsing project MCP config") {
		t.Errorf("error %q does not contain expected message", err.Error())
	}
}

func TestReadProjectMcpServersJSONNull(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".mcp.json")
	if err := os.WriteFile(path, []byte(`{"mcpServers": null}`), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := ReadProjectMcpServers(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty map, got %v", got)
	}
}

func TestReadProjectMcpServersEmptyObject(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".mcp.json")
	if err := os.WriteFile(path, []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := ReadProjectMcpServers(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty map, got %v", got)
	}
}

func TestReadProjectMcpServersEmptyServers(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".mcp.json")
	if err := os.WriteFile(path, []byte(`{"mcpServers": {}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := ReadProjectMcpServers(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty map, got %v", got)
	}
}

// --- UpdateProjectMcpServers ---

func TestUpdateProjectMcpServers_Merge(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".mcp.json")

	// Pre-populate with one server.
	initial := `{
  "mcpServers": {
    "existing": {
      "command": "node",
      "args": ["server.js"]
    }
  }
}`
	if err := os.WriteFile(path, []byte(initial), 0o644); err != nil {
		t.Fatal(err)
	}

	// Add a new server.
	err := UpdateProjectMcpServers(path, func(servers map[string]manifest.MCPServer) error {
		servers["new-server"] = manifest.MCPServer{Command: "python", Args: []string{"main.py"}}
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, err := ReadProjectMcpServers(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 servers, got %d", len(got))
	}
	if got["existing"].Command != "node" {
		t.Errorf("existing server command = %q, want %q", got["existing"].Command, "node")
	}
	if got["new-server"].Command != "python" {
		t.Errorf("new server command = %q, want %q", got["new-server"].Command, "python")
	}
}

func TestUpdateProjectMcpServers_Delete(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".mcp.json")

	initial := `{
  "mcpServers": {
    "to-keep": {"command": "keep"},
    "to-delete": {"command": "delete"}
  }
}`
	if err := os.WriteFile(path, []byte(initial), 0o644); err != nil {
		t.Fatal(err)
	}

	err := UpdateProjectMcpServers(path, func(servers map[string]manifest.MCPServer) error {
		delete(servers, "to-delete")
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, err := ReadProjectMcpServers(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 server, got %d", len(got))
	}
	if _, ok := got["to-delete"]; ok {
		t.Error("to-delete should have been removed")
	}
}

func TestUpdateProjectMcpServers_CreateNew(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "subdir", ".mcp.json")

	err := UpdateProjectMcpServers(path, func(servers map[string]manifest.MCPServer) error {
		servers["first"] = manifest.MCPServer{Command: "first-cmd"}
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, err := ReadProjectMcpServers(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 server, got %d", len(got))
	}
	if got["first"].Command != "first-cmd" {
		t.Errorf("command = %q, want %q", got["first"].Command, "first-cmd")
	}
}

func TestUpdateProjectMcpServers_EscapeHTML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".mcp.json")

	err := UpdateProjectMcpServers(path, func(servers map[string]manifest.MCPServer) error {
		servers["api"] = manifest.MCPServer{
			Type: "sse",
			URL:  "https://example.com/api?foo=1&bar=2",
		}
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Read raw file to verify & is not escaped and mcpServers wrapper is present.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	raw := string(data)
	if !strings.Contains(raw, "foo=1&bar=2") {
		t.Errorf("expected unescaped &, got:\n%s", raw)
	}
	if !strings.Contains(raw, `"mcpServers"`) {
		t.Errorf("expected mcpServers wrapper, got:\n%s", raw)
	}
}

func TestUpdateProjectMcpServers_CallbackError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".mcp.json")

	if err := os.WriteFile(path, []byte(`{"mcpServers": {"keep": {"command": "cmd"}}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	err := UpdateProjectMcpServers(path, func(servers map[string]manifest.MCPServer) error {
		return fmt.Errorf("test error")
	})
	if err == nil {
		t.Fatal("expected error")
	}

	// File should be unchanged.
	got, err := ReadProjectMcpServers(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got["keep"].Command != "cmd" {
		t.Errorf("file should be unchanged after callback error, got %v", got)
	}
}
