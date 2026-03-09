// Package manifest handles the cctote manifest data model and IO.
package manifest

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/alphaleonis/cctote/internal/fileutil"
)

// CurrentVersion is the manifest schema version supported by this build.
const CurrentVersion = 1

// Manifest is the portable config file — a declarative desired state that
// travels between machines via chezmoi.
type Manifest struct {
	Version      int                    `json:"version"`
	MCPServers   map[string]MCPServer   `json:"mcpServers"`
	Plugins      []Plugin               `json:"plugins"`
	Marketplaces map[string]Marketplace `json:"marketplaces"`
	Profiles     map[string]Profile     `json:"profiles,omitempty"`
}

// MCPServer represents a single MCP server entry, matching the ~/.claude.json structure.
type MCPServer struct {
	// Type is the transport type: "stdio", "http", "sse", "websocket".
	// Empty defaults to "stdio" in Claude Code.
	Type string `json:"type,omitempty"`

	// Stdio fields
	Command string            `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	CWD     string            `json:"cwd,omitempty"`
	Env     map[string]string `json:"env,omitempty"`

	// HTTP/SSE fields
	URL     string            `json:"url,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
	OAuth   *MCPOAuth         `json:"oauth,omitempty"`
}

// MCPOAuth holds OAuth configuration for HTTP/SSE MCP servers.
type MCPOAuth struct {
	ClientID     string `json:"clientId"`
	CallbackPort int    `json:"callbackPort,omitempty"`
}

// Plugin represents a Claude Code plugin/extension.
type Plugin struct {
	ID      string `json:"id"`
	Scope   string `json:"scope"`
	Enabled bool   `json:"enabled"`
}

// Marketplace represents a plugin marketplace source.
// Keyed by name in Manifest.Marketplaces (name is the map key, not a struct field).
type Marketplace struct {
	// Source is the marketplace type: "github", "git", or "directory".
	Source string `json:"source"`

	// GitHub source locator (owner/repo format).
	Repo string `json:"repo,omitempty"`

	// Git source locator (clone URL).
	URL string `json:"url,omitempty"`

	// Directory source locator (local filesystem path).
	Path string `json:"path,omitempty"`
}

// SourceLocator returns the CLI source argument for `claude plugin marketplace add`.
// For github sources it returns the repo, for git sources the URL, and for
// directory sources the path. Returns "" for unknown source types.
func (m Marketplace) SourceLocator() string {
	s, _ := m.SourceLocatorE()
	return s
}

// SourceLocatorE is like SourceLocator but returns an error for unknown source types.
func (m Marketplace) SourceLocatorE() (string, error) {
	switch m.Source {
	case "github":
		return m.Repo, nil
	case "git":
		return m.URL, nil
	case "directory":
		return m.Path, nil
	default:
		return "", fmt.Errorf("unsupported marketplace source type %q", m.Source)
	}
}

// Profile is a named subset of the repository, referencing extensions by key/ID.
type Profile struct {
	MCPServers []string        `json:"mcpServers"`
	Plugins    []ProfilePlugin `json:"plugins"`
}

// ProfilePlugin is a profile's reference to a manifest plugin, with an optional
// enabled override. When Enabled is nil, the manifest default applies.
type ProfilePlugin struct {
	ID      string `json:"id"`
	Enabled *bool  `json:"enabled,omitempty"` // nil = inherit from manifest
}

// FindProfilePlugin returns the index of the profile plugin with the given ID, or -1.
func FindProfilePlugin(plugins []ProfilePlugin, id string) int {
	for i, p := range plugins {
		if p.ID == id {
			return i
		}
	}
	return -1
}

// DefaultPath returns the default manifest file path, respecting XDG_CONFIG_HOME.
func DefaultPath() (string, error) {
	if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
		return filepath.Join(dir, "cctote", "manifest.json"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("determining home directory: %w", err)
	}
	return filepath.Join(home, ".config", "cctote", "manifest.json"), nil
}

// LoadOrCreate reads and parses a manifest from the given path. If the file
// does not exist, it returns an empty manifest with initialized maps/slices.
// Use this for write commands (export, profile create) that bootstrap on first run.
func LoadOrCreate(path string) (*Manifest, bool, error) {
	m, err := Load(path)
	if err == nil {
		return m, false, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, false, err
	}
	return &Manifest{
		Version:      CurrentVersion,
		MCPServers:   map[string]MCPServer{},
		Plugins:      []Plugin{},
		Marketplaces: map[string]Marketplace{},
	}, true, nil
}

// Load reads and parses a manifest from the given path.
func Load(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading manifest: %w", err)
	}

	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parsing manifest: %w", err)
	}

	if m.Version != CurrentVersion {
		return nil, fmt.Errorf("unsupported manifest version %d (supported: %d)", m.Version, CurrentVersion)
	}

	return &m, nil
}

// Update performs a locked read-modify-write cycle on the manifest at path.
// The callback receives the current manifest (loaded fresh under the lock)
// and may mutate it in place. If the callback returns nil, the modified
// manifest is saved atomically. If it returns an error, the file is left
// unchanged. This prevents lost updates from concurrent cctote processes.
func Update(path string, fn func(m *Manifest) error) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating manifest directory: %w", err)
	}

	release, err := fileutil.AcquireLock(path)
	if err != nil {
		return err
	}
	defer release()

	m, err := Load(path)
	if err != nil {
		return err
	}

	if err := fn(m); err != nil {
		return err
	}

	return Save(path, m)
}

// Save writes a manifest to the given path as indented JSON, creating parent
// directories as needed. Uses atomic temp-file-plus-rename to prevent
// corruption from a crash mid-write.
func Save(path string, m *Manifest) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling manifest: %w", err)
	}
	data = append(data, '\n')

	if err := fileutil.AtomicWrite(path, data, 0o644); err != nil {
		return fmt.Errorf("writing manifest: %w", err)
	}

	return nil
}

// FindPlugin returns the index of the plugin with the given ID, or -1.
func FindPlugin(plugins []Plugin, id string) int {
	for i, p := range plugins {
		if p.ID == id {
			return i
		}
	}
	return -1
}
