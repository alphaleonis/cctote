package mcp

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/alphaleonis/cctote/internal/fileutil"
	"github.com/alphaleonis/cctote/internal/manifest"
)

// ReadProjectMcpServers reads a project-level .mcp.json file, which uses the
// same {"mcpServers": {...}} wrapper as ~/.claude.json.
// A missing file returns an empty map and nil error.
func ReadProjectMcpServers(path string) (map[string]manifest.MCPServer, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return make(map[string]manifest.MCPServer), nil
		}
		return nil, fmt.Errorf("reading project MCP config: %w", err)
	}

	var top map[string]json.RawMessage
	if err := json.Unmarshal(data, &top); err != nil {
		return nil, fmt.Errorf("parsing project MCP config: %w", err)
	}

	raw, ok := top["mcpServers"]
	if !ok || string(raw) == "null" {
		return make(map[string]manifest.MCPServer), nil
	}

	var servers map[string]manifest.MCPServer
	if err := json.Unmarshal(raw, &servers); err != nil {
		return nil, fmt.Errorf("parsing project mcpServers: %w", err)
	}

	if servers == nil {
		servers = make(map[string]manifest.MCPServer)
	}

	return servers, nil
}

// UpdateProjectMcpServers performs a locked read-modify-write on a project-level
// .mcp.json file. The callback receives the current servers map and may mutate
// it in place. If the callback returns nil, the modified map is saved atomically.
// A missing file is treated as an empty map.
func UpdateProjectMcpServers(path string, fn func(servers map[string]manifest.MCPServer) error) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating project config directory: %w", err)
	}

	release, err := fileutil.AcquireLock(path)
	if err != nil {
		return fmt.Errorf("acquiring project MCP lock: %w", err)
	}
	defer release()

	servers, err := ReadProjectMcpServers(path)
	if err != nil {
		return err
	}

	if err := fn(servers); err != nil {
		return err
	}

	wrapper := map[string]map[string]manifest.MCPServer{"mcpServers": servers}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(wrapper); err != nil {
		return fmt.Errorf("marshaling project MCP config: %w", err)
	}

	// 0600: MCP server configs may contain env vars with API keys.
	if err := fileutil.AtomicWrite(path, buf.Bytes(), 0o600); err != nil {
		return fmt.Errorf("writing project MCP config: %w", err)
	}

	return nil
}
