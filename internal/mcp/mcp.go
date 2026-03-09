// Package mcp handles MCP server configuration in ~/.claude.json.
package mcp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/alphaleonis/cctote/internal/fileutil"
	"github.com/alphaleonis/cctote/internal/manifest"
	"github.com/iancoleman/orderedmap"
)

// DefaultPath returns the path to Claude Code's config file ($HOME/.claude.json).
// Unlike manifest/config paths, this does NOT respect XDG — Claude Code hardcodes
// the location.
func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("determining home directory: %w", err)
	}
	return filepath.Join(home, ".claude.json"), nil
}

// ReadMcpServers extracts the mcpServers map from a Claude Code config file.
// A missing file or missing/null mcpServers key returns an empty map and nil error.
func ReadMcpServers(path string) (map[string]manifest.MCPServer, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string]manifest.MCPServer), nil
		}
		return nil, fmt.Errorf("reading claude config: %w", err)
	}

	var top map[string]json.RawMessage
	if err := json.Unmarshal(data, &top); err != nil {
		return nil, fmt.Errorf("parsing claude config: %w", err)
	}

	raw, ok := top["mcpServers"]
	if !ok || string(raw) == "null" {
		return make(map[string]manifest.MCPServer), nil
	}

	var servers map[string]manifest.MCPServer
	if err := json.Unmarshal(raw, &servers); err != nil {
		return nil, fmt.Errorf("parsing mcpServers: %w", err)
	}

	return servers, nil
}

// WriteMcpServers merges the mcpServers map into a Claude Code config file,
// preserving all other keys and their ordering. Creates the file and parent
// directories if they don't exist. Acquires a proper-lockfile compatible
// directory lock to cooperate with Claude Code's own locking.
func WriteMcpServers(path string, servers map[string]manifest.MCPServer) error {
	return UpdateMcpServers(path, func(_ map[string]manifest.MCPServer) (map[string]manifest.MCPServer, error) {
		return servers, nil
	})
}

// UpdateMcpServers acquires a lock on the config file, reads the current
// mcpServers, applies the merge function, and writes the result atomically.
// The entire read-merge-write cycle runs under a single lock to prevent
// TOCTOU races with Claude Code or other cctote processes.
func UpdateMcpServers(path string, merge func(current map[string]manifest.MCPServer) (map[string]manifest.MCPServer, error)) error {
	// Ensure parent directory exists before acquiring the lock, since the
	// lock directory is created alongside the config file.
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	release, err := fileutil.AcquireLock(path)
	if err != nil {
		return err
	}
	defer release()

	// Read current servers under lock.
	current, err := ReadMcpServers(path)
	if err != nil {
		return err
	}

	// Apply the merge.
	result, err := merge(current)
	if err != nil {
		return err
	}

	return writeMcpServersLocked(path, result)
}

// writeMcpServersLocked performs the orderedmap-preserving write with atomic
// temp-file-plus-rename. Caller must hold the lock.
func writeMcpServersLocked(path string, servers map[string]manifest.MCPServer) error {
	om := orderedmap.New()
	om.SetEscapeHTML(false)

	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("reading claude config: %w", err)
		}
		// Missing file — start with empty ordered map.
	} else {
		if err := json.Unmarshal(data, om); err != nil {
			return fmt.Errorf("parsing claude config: %w", err)
		}
	}

	// Sort server keys for deterministic output — prevents unnecessary
	// VCS diffs when the content hasn't actually changed.
	sorted := orderedmap.New()
	sorted.SetEscapeHTML(false)
	names := make([]string, 0, len(servers))
	for name := range servers {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		sorted.Set(name, servers[name])
	}
	om.Set("mcpServers", sorted)

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(om); err != nil {
		return fmt.Errorf("marshaling claude config: %w", err)
	}

	// Preserve original file permissions if the file exists, else use 0644.
	perm := os.FileMode(0o644)
	if info, statErr := os.Stat(path); statErr == nil {
		perm = info.Mode()
	}

	if err := fileutil.AtomicWrite(path, buf.Bytes(), perm); err != nil {
		return fmt.Errorf("writing claude config: %w", err)
	}

	return nil
}
