// Package claude provides a typed client for the Claude Code CLI, translating
// Go method calls into CLI invocations and parsing their output.
//
// Mutation methods (Install/Uninstall/Enable/Disable/Add/Remove/Update) return
// errors without added context — callers are responsible for wrapping with
// operation context (e.g. fmt.Errorf("installing plugin %q: %w", id, err)).
// This keeps error messages composable across CLI and TUI surfaces that format
// them differently. Read methods (List*) still wrap for convenience.
package claude

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/alphaleonis/cctote/internal/cliutil"
	"github.com/alphaleonis/cctote/internal/manifest"
)

// Client wraps a cliutil.Runner to provide typed access to the Claude Code CLI.
// It is deliberately "dumb" — it maps Go methods to CLI args and parses output.
// No duplicate-checking or ordering logic lives here.
type Client struct {
	runner cliutil.Runner
}

// NewClient creates a Client using the provided Runner.
func NewClient(runner cliutil.Runner) *Client {
	return &Client{runner: runner}
}

// NewExecClient creates a Client that shells out to the "claude" binary.
func NewExecClient() *Client {
	return NewClient(&cliutil.ExecRunner{Command: "claude"})
}

// EnsureAvailable checks that the claude binary is on PATH.
func EnsureAvailable() error {
	return cliutil.LookPath("claude")
}

// ListPlugins returns the installed plugins by running `claude plugin list --json`.
func (c *Client) ListPlugins(ctx context.Context) ([]manifest.Plugin, error) {
	out, err := c.runner.Run(ctx, "plugin", "list", "--json")
	if err != nil {
		return nil, fmt.Errorf("listing plugins: %w", err)
	}

	var entries []pluginListEntry
	if err := json.Unmarshal(out, &entries); err != nil {
		return nil, fmt.Errorf("parsing plugin list output: %w", err)
	}

	plugins := make([]manifest.Plugin, len(entries))
	for i, e := range entries {
		plugins[i] = e.toPlugin()
	}
	return plugins, nil
}

// scopeArgs inserts -s <scope> after the subcommand verb (first two args)
// and before positional arguments, e.g. ["plugin", "install", "-s", "project", "id"].
func scopeArgs(scope string, base ...string) []string {
	if scope == "" {
		return base
	}
	if len(base) < 2 {
		// Fallback: no verb to insert after — append at end.
		return append(base, "-s", scope)
	}
	result := make([]string, 0, len(base)+2)
	result = append(result, base[:2]...)
	result = append(result, "-s", scope)
	result = append(result, base[2:]...)
	return result
}

// InstallPlugin runs `claude plugin install [-s <scope>] <id>`.
func (c *Client) InstallPlugin(ctx context.Context, id string, scope string) error {
	args := scopeArgs(scope, "plugin", "install", id)
	_, err := c.runner.Run(ctx, args...)
	return err
}

// SetPluginEnabled runs `claude plugin enable/disable [-s <scope>] <id>`.
func (c *Client) SetPluginEnabled(ctx context.Context, id string, enabled bool, scope string) error {
	verb := "disable"
	if enabled {
		verb = "enable"
	}
	args := scopeArgs(scope, "plugin", verb, id)
	_, err := c.runner.Run(ctx, args...)
	return err
}

// UninstallPlugin runs `claude plugin uninstall [-s <scope>] <id>`.
func (c *Client) UninstallPlugin(ctx context.Context, id string, scope string) error {
	args := scopeArgs(scope, "plugin", "uninstall", id)
	_, err := c.runner.Run(ctx, args...)
	return err
}

// AddMcpServer runs `claude mcp add [-s <scope>] <name> ...` with args based
// on transport type. Stdio servers use `-- <command> [args...]` with optional
// `-e KEY=VALUE` env flags. HTTP/SSE/WebSocket servers use `--transport <type> <name> <url>`.
func (c *Client) AddMcpServer(ctx context.Context, name string, server manifest.MCPServer, scope string) error {
	transport := server.Type
	if transport == "" {
		transport = "stdio"
	}

	var args []string
	switch transport {
	case "stdio":
		if server.Command == "" {
			return fmt.Errorf("stdio transport requires a command")
		}
		if server.CWD != "" {
			return fmt.Errorf("CWD not supported via CLI (use direct file write)")
		}
		args = scopeArgs(scope, "mcp", "add", name)
		envKeys := make([]string, 0, len(server.Env))
		for k := range server.Env {
			envKeys = append(envKeys, k)
		}
		sort.Strings(envKeys)
		for _, k := range envKeys {
			args = append(args, "-e", k+"="+server.Env[k])
		}
		args = append(args, "--")
		args = append(args, server.Command)
		args = append(args, server.Args...)
	case "http", "sse", "websocket":
		if len(server.Headers) > 0 {
			return fmt.Errorf("headers not supported via CLI (use direct file write)")
		}
		if server.OAuth != nil {
			return fmt.Errorf("oauth not supported via CLI (use direct file write)")
		}
		args = scopeArgs(scope, "mcp", "add", "--transport", transport, name, server.URL)
	default:
		return fmt.Errorf("unsupported transport %q", transport)
	}

	_, err := c.runner.Run(ctx, args...)
	return err
}

// RemoveMcpServer runs `claude mcp remove [-s <scope>] <name>`.
func (c *Client) RemoveMcpServer(ctx context.Context, name string, scope string) error {
	args := scopeArgs(scope, "mcp", "remove", name)
	_, err := c.runner.Run(ctx, args...)
	return err
}

// ListMarketplaces returns marketplace sources by running
// `claude plugin marketplace list --json`. Results are keyed by marketplace name.
func (c *Client) ListMarketplaces(ctx context.Context) (map[string]manifest.Marketplace, error) {
	out, err := c.runner.Run(ctx, "plugin", "marketplace", "list", "--json")
	if err != nil {
		return nil, fmt.Errorf("listing marketplaces: %w", err)
	}

	var entries []marketplaceListEntry
	if err := json.Unmarshal(out, &entries); err != nil {
		return nil, fmt.Errorf("parsing marketplace list output: %w", err)
	}

	marketplaces := make(map[string]manifest.Marketplace, len(entries))
	for _, e := range entries {
		marketplaces[e.Name] = e.toMarketplace()
	}
	return marketplaces, nil
}

// AddMarketplace runs `claude plugin marketplace add <source>`.
func (c *Client) AddMarketplace(ctx context.Context, source string) error {
	_, err := c.runner.Run(ctx, "plugin", "marketplace", "add", source)
	return err
}

// RemoveMarketplace runs `claude plugin marketplace remove <name>`.
func (c *Client) RemoveMarketplace(ctx context.Context, name string) error {
	_, err := c.runner.Run(ctx, "plugin", "marketplace", "remove", name)
	return err
}

// UpdateMarketplace runs `claude plugin marketplace update [name]`.
// If name is empty, all marketplaces are updated.
func (c *Client) UpdateMarketplace(ctx context.Context, name string) error {
	args := []string{"plugin", "marketplace", "update"}
	if name != "" {
		args = append(args, name)
	}
	_, err := c.runner.Run(ctx, args...)
	return err
}
