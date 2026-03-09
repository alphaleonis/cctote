package cmd

import (
	"fmt"

	"github.com/alphaleonis/cctote/internal/engine"
	"github.com/alphaleonis/cctote/internal/manifest"
	"github.com/alphaleonis/cctote/internal/mcp"
	"github.com/alphaleonis/cctote/internal/ui"
	"github.com/spf13/cobra"
)

// ExitError signals a non-failure exit with a specific code.
// Code 2 follows the diff(1) convention: 0 = identical, 1 = error, 2 = differences found.
type ExitError struct{ Code int }

func (e *ExitError) Error() string { return fmt.Sprintf("exit status %d", e.Code) }

// diffEntry represents a single item that differs between manifest and Claude Code.
type diffEntry struct {
	Kind     string `json:"kind"`               // "mcp", "plugin", "marketplace"
	Name     string `json:"name"`               // server name, plugin ID, or marketplace name
	Manifest any    `json:"manifest,omitempty"` // value from manifest (nil if only in Claude Code)
	Claude   any    `json:"claude,omitempty"`   // value from Claude Code (nil if only in manifest)
}

// diffResult holds the structured diff output.
type diffResult struct {
	OnlyInManifest   []diffEntry `json:"onlyInManifest"`
	OnlyInClaudeCode []diffEntry `json:"onlyInClaudeCode"`
	Different        []diffEntry `json:"different"`
}

func (a *App) addDiffCommands() {
	diffCmd := &cobra.Command{
		Use:   "diff",
		Short: "Show differences between manifest and Claude Code configuration",
		RunE:  a.runDiff,
	}
	diffCmd.Flags().String("profile", "", "compare a specific profile instead of the full repository")
	a.root.AddCommand(diffCmd)
}

func (a *App) runDiff(cmd *cobra.Command, _ []string) error {
	w := ui.NewWriter(cmd.ErrOrStderr(), a.jsonOutput)
	profileName, _ := cmd.Flags().GetString("profile")

	if err := a.ensureClaudeAvailable(); err != nil {
		return err
	}

	// Load manifest.
	manPath, err := a.resolveManifestPath()
	if err != nil {
		return err
	}
	m, _, err := manifest.LoadOrCreate(manPath)
	if err != nil {
		return fmt.Errorf("loading manifest: %w", err)
	}

	// Determine manifest-side scope.
	manMCP := m.MCPServers
	manPlugins := m.Plugins
	manMarketplaces := m.Marketplaces
	skipMarketplaces := false

	if profileName != "" {
		var err error
		manMCP, manPlugins, err = resolveProfile(m, profileName)
		if err != nil {
			return err
		}
		// Profiles don't reference marketplaces — skip comparison.
		skipMarketplaces = true
	}

	// Read Claude Code state.
	claudePath, err := mcp.DefaultPath()
	if err != nil {
		return err
	}
	claudeMCP, err := mcp.ReadMcpServers(claudePath)
	if err != nil {
		return fmt.Errorf("reading Claude Code MCP config: %w", err)
	}

	client := a.newClaudeClient()
	ctx := cmd.Context()

	claudePlugins, err := client.ListPlugins(ctx)
	if err != nil {
		return err
	}
	claudePluginMap := engine.PluginMap(claudePlugins)
	manPluginMap := engine.PluginMap(manPlugins)

	var claudeMarketplaces map[string]manifest.Marketplace
	if !skipMarketplaces {
		claudeMarketplaces, err = client.ListMarketplaces(ctx)
		if err != nil {
			return err
		}
	}

	// Build diff result.
	result := diffResult{
		OnlyInManifest:   []diffEntry{},
		OnlyInClaudeCode: []diffEntry{},
		Different:        []diffEntry{},
	}

	// --- MCP servers ---
	for _, name := range sortedKeys(manMCP) {
		manSrv := manMCP[name]
		claudeSrv, inClaude := claudeMCP[name]
		if !inClaude {
			result.OnlyInManifest = append(result.OnlyInManifest, diffEntry{Kind: "mcp", Name: name, Manifest: manSrv})
		} else if !manifest.MCPServersEqual(manSrv, claudeSrv) {
			result.Different = append(result.Different, diffEntry{Kind: "mcp", Name: name, Manifest: manSrv, Claude: claudeSrv})
		}
	}
	for _, name := range sortedKeys(claudeMCP) {
		if _, inManifest := manMCP[name]; !inManifest {
			result.OnlyInClaudeCode = append(result.OnlyInClaudeCode, diffEntry{Kind: "mcp", Name: name, Claude: claudeMCP[name]})
		}
	}

	// --- Plugins ---
	for _, pid := range sortedKeys(manPluginMap) {
		manP := manPluginMap[pid]
		claudeP, inClaude := claudePluginMap[pid]
		if !inClaude {
			result.OnlyInManifest = append(result.OnlyInManifest, diffEntry{Kind: "plugin", Name: pid, Manifest: manP})
		} else if !manifest.PluginsEqual(manP, claudeP) {
			result.Different = append(result.Different, diffEntry{Kind: "plugin", Name: pid, Manifest: manP, Claude: claudeP})
		}
	}
	for _, pid := range sortedKeys(claudePluginMap) {
		if _, inManifest := manPluginMap[pid]; !inManifest {
			result.OnlyInClaudeCode = append(result.OnlyInClaudeCode, diffEntry{Kind: "plugin", Name: pid, Claude: claudePluginMap[pid]})
		}
	}

	// --- Marketplaces ---
	if !skipMarketplaces {
		for _, name := range sortedKeys(manMarketplaces) {
			manMP := manMarketplaces[name]
			claudeMP, inClaude := claudeMarketplaces[name]
			if !inClaude {
				result.OnlyInManifest = append(result.OnlyInManifest, diffEntry{Kind: "marketplace", Name: name, Manifest: manMP})
			} else if !manifest.MarketplacesEqual(manMP, claudeMP) {
				result.Different = append(result.Different, diffEntry{Kind: "marketplace", Name: name, Manifest: manMP, Claude: claudeMP})
			}
		}
		for _, name := range sortedKeys(claudeMarketplaces) {
			if _, inManifest := manMarketplaces[name]; !inManifest {
				result.OnlyInClaudeCode = append(result.OnlyInClaudeCode, diffEntry{Kind: "marketplace", Name: name, Claude: claudeMarketplaces[name]})
			}
		}
	}

	// Output.
	hasDiffs := len(result.OnlyInManifest)+len(result.OnlyInClaudeCode)+len(result.Different) > 0

	if a.jsonOutput {
		if err := writeJSON(cmd, result); err != nil {
			return err
		}
		if hasDiffs {
			return &ExitError{Code: 2}
		}
		return nil
	}

	if !hasDiffs {
		w.NothingToDo()
		return nil
	}

	// Group entries by kind for human-readable output.
	printDiffSection(cmd, w, "MCP Servers", "mcp", result)
	printDiffSection(cmd, w, "Plugins", "plugin", result)
	if !skipMarketplaces {
		printDiffSection(cmd, w, "Marketplaces", "marketplace", result)
	}

	return &ExitError{Code: 2}
}

// printDiffSection prints a human-readable diff section for a specific kind.
func printDiffSection(cmd *cobra.Command, w *ui.Writer, header, kind string, result diffResult) {
	onlyManifest := filterByKind(result.OnlyInManifest, kind)
	onlyClaude := filterByKind(result.OnlyInClaudeCode, kind)
	different := filterByKind(result.Different, kind)

	if len(onlyManifest)+len(onlyClaude)+len(different) == 0 {
		return
	}

	// Labels reflect what "cctote import" would do: manifest-only items
	// would be added, Claude-only items would be removed (with --strict).
	w.Bold(cmd.ErrOrStderr(), "%s\n", header)
	w.DiffList(ui.DiffAdd, entryNames(onlyManifest))
	w.DiffList(ui.DiffRemove, entryNames(onlyClaude))
	w.DiffList(ui.DiffConflict, entryNames(different))
}

// filterByKind returns entries matching the given kind.
func filterByKind(entries []diffEntry, kind string) []diffEntry {
	var out []diffEntry
	for _, e := range entries {
		if e.Kind == kind {
			out = append(out, e)
		}
	}
	return out
}

// entryNames extracts the Name field from a slice of diffEntry.
func entryNames(entries []diffEntry) []string {
	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.Name
	}
	return names
}
