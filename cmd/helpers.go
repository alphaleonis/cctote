package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/alphaleonis/cctote/internal/claude"
	"github.com/alphaleonis/cctote/internal/engine"
	"github.com/alphaleonis/cctote/internal/manifest"
	"github.com/alphaleonis/cctote/internal/ui"
	"github.com/spf13/cobra"
)

const (
	scopeUser    = "user"
	scopeProject = "project"
)

// addScopeFlag adds a --scope / -s flag to the command.
func addScopeFlag(cmd *cobra.Command) {
	cmd.Flags().StringP("scope", "s", scopeUser, "configuration scope: user (default) or project")
}

// getScope reads and validates the --scope flag value.
func getScope(cmd *cobra.Command) (string, error) {
	scope, _ := cmd.Flags().GetString("scope")
	switch scope {
	case scopeUser, scopeProject:
		return scope, nil
	default:
		return "", fmt.Errorf("invalid --scope %q: must be %q or %q", scope, scopeUser, scopeProject)
	}
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func writeJSON(cmd *cobra.Command, v any) error {
	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}

// resolveProfile resolves a named profile to its referenced MCP servers and
// plugins from the manifest. Delegates to [engine.ResolveProfile].
func resolveProfile(m *manifest.Manifest, name string) (map[string]manifest.MCPServer, []manifest.Plugin, error) {
	return engine.ResolveProfile(m, name)
}

// lazyLoadMarketplaces loads marketplace data from Claude Code only when at
// least one plugin has a @marketplace suffix whose marketplace is NOT already
// known. This avoids an unnecessary CLI call when all referenced marketplaces
// are already present. A single ListMarketplaces call returns all marketplaces,
// so we short-circuit on the first missing one. Pass existing marketplace names
// from a pre-loaded manifest (nil is safe — treats all marketplaces as missing).
func lazyLoadMarketplaces(ctx context.Context, client *claude.Client, existing map[string]manifest.Marketplace, plugins []manifest.Plugin) (map[string]manifest.Marketplace, error) {
	for _, p := range plugins {
		mpName := engine.MarketplaceFromPluginID(p.ID)
		if mpName != "" {
			if _, ok := existing[mpName]; !ok {
				return client.ListMarketplaces(ctx)
			}
		}
	}
	return nil, nil
}

// lazyCheckPluginPrereqs verifies that marketplace dependencies for new
// plugins are available in Claude Code. Only loads marketplace data from
// the CLI when at least one plugin has a @marketplace suffix.
//
// pluginIDs should contain only plugins that will be newly installed
// (e.g., plan.Add), not the full import set — already-installed plugins
// do not need marketplace verification.
func lazyCheckPluginPrereqs(ctx context.Context, client *claude.Client, pluginIDs []string) error {
	for _, pid := range pluginIDs {
		if engine.MarketplaceFromPluginID(pid) != "" {
			// At least one plugin needs a marketplace — load and check all.
			installed, err := client.ListMarketplaces(ctx)
			if err != nil {
				return err
			}
			return engine.CheckPluginMarketplacePrereqs(pluginIDs, installed)
		}
	}
	return nil
}

// cliHooks implements engine.Hooks for CLI commands.
type cliHooks struct {
	cmd        *cobra.Command
	w          *ui.Writer
	section    string // e.g., "MCP server", "plugin", "marketplace"
	force      bool
	cascadeMsg string // format string with one %q arg for item name; overrides default "<section> %q will cascade to:"
}

func (h *cliHooks) OnCascade(item string, dependents []string) (bool, error) {
	if h.cascadeMsg != "" {
		h.w.Warn(h.cascadeMsg, item)
	} else {
		h.w.Warn("%s %q will cascade to:", h.section, item)
	}
	h.w.List(dependents)
	return ui.Confirm(h.cmd.InOrStdin(), h.cmd.ErrOrStderr(), "Proceed?", h.force)
}

func (h *cliHooks) OnInfo(msg string) {
	h.w.Info("%s", msg)
}

func (h *cliHooks) OnWarn(msg string) {
	h.w.Warn("%s", msg)
}
