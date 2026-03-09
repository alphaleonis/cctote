package engine

import (
	"fmt"

	"github.com/alphaleonis/cctote/internal/manifest"
)

// ExportMCPServers upserts MCP servers from Claude Code into the manifest.
// The manifest is created if it does not yet exist.
func ExportMCPServers(manPath string, servers map[string]manifest.MCPServer, hooks Hooks) (*Result, error) {
	if err := ensureManifest(manPath, hooks); err != nil {
		return nil, err
	}

	result := &Result{}
	if err := manifest.Update(manPath, func(m *manifest.Manifest) error {
		result.Actions = ApplyMCPUpserts(m, servers)
		return nil
	}); err != nil {
		return nil, err
	}

	return result, nil
}

// ApplyMCPUpserts merges servers into m in-place and returns item actions.
// This is the pure mutation step — no file I/O — suitable for composing
// inside a single manifest.Update call.
func ApplyMCPUpserts(m *manifest.Manifest, servers map[string]manifest.MCPServer) []ItemAction {
	if m.MCPServers == nil {
		m.MCPServers = make(map[string]manifest.MCPServer)
	}
	var actions []ItemAction
	for name, srv := range servers {
		action := ActionAdded
		if _, exists := m.MCPServers[name]; exists {
			action = ActionUpdated
		}
		m.MCPServers[name] = srv
		actions = append(actions, ItemAction{
			Section: SectionMCP,
			Name:    name,
			Action:  action,
		})
	}
	return actions
}

// ExportPlugins upserts plugins from Claude Code into the manifest. For
// plugins with a marketplace dependency (pluginId@marketplace), the
// marketplace is auto-exported if it exists in claudeMarketplaces but not
// in the manifest. hooks.OnCascade is called once per marketplace that would
// be auto-exported, with the marketplace name as item and the dependent
// plugin IDs as dependents. Plugins whose marketplace is unavailable or
// declined are skipped.
//
// claudeMarketplaces semantics:
//   - nil: no marketplace data was fetched; marketplace-dependent plugins are
//     silently skipped with an OnInfo warning ("not available").
//   - empty map: data was fetched but no marketplaces exist in Claude Code;
//     marketplace-dependent plugins are skipped ("not available in Claude Code").
//   - populated: marketplace lookups proceed normally.
func ExportPlugins(manPath string, plugins []manifest.Plugin, claudeMarketplaces map[string]manifest.Marketplace, hooks Hooks) (*Result, error) {
	if err := ensureManifest(manPath, hooks); err != nil {
		return nil, err
	}

	exportable, approvedMkts, err := ResolvePluginExports(manPath, plugins, claudeMarketplaces, hooks)
	if err != nil {
		return nil, err
	}

	result := &Result{}
	if err := manifest.Update(manPath, func(m *manifest.Manifest) error {
		result.Actions = ApplyPluginUpserts(m, exportable, approvedMkts)
		return nil
	}); err != nil {
		return nil, err
	}

	return result, nil
}

// ResolvePluginExports determines which plugins can be exported and which
// marketplaces need auto-exporting. All OnCascade prompts happen here,
// outside any file lock.
//
// Returns the exportable plugins and approved marketplaces. Plugins whose
// marketplace is unavailable or declined by hooks are excluded.
func ResolvePluginExports(manPath string, plugins []manifest.Plugin, claudeMarketplaces map[string]manifest.Marketplace, hooks Hooks) (exportable []manifest.Plugin, approvedMkts map[string]manifest.Marketplace, err error) {
	// Pre-check: read manifest (unlocked) to determine which marketplaces
	// need auto-exporting. This avoids holding a lock during OnCascade.
	m, err := manifest.Load(manPath)
	if err != nil {
		return nil, nil, fmt.Errorf("loading manifest: %w", err)
	}

	// Group plugins by marketplace dependency.
	type mktGroup struct {
		mpName  string
		mkt     manifest.Marketplace
		plugins []manifest.Plugin
	}
	var (
		noMktPlugins []manifest.Plugin        // plugins without marketplace deps
		mktGroups    = map[string]*mktGroup{} // grouped by marketplace name
		mktOrder     []string                 // insertion order
		skippedMkts  = map[string]bool{}      // marketplaces we can't resolve
	)

	for _, p := range plugins {
		mpName := MarketplaceFromPluginID(p.ID)
		if mpName == "" {
			noMktPlugins = append(noMktPlugins, p)
			continue
		}

		// Already in manifest?
		if _, ok := m.Marketplaces[mpName]; ok {
			noMktPlugins = append(noMktPlugins, p)
			continue
		}

		// Already grouped or skipped?
		if _, ok := mktGroups[mpName]; ok {
			mktGroups[mpName].plugins = append(mktGroups[mpName].plugins, p)
			continue
		}
		if skippedMkts[mpName] {
			hooks.OnInfo(fmt.Sprintf("skipping plugin %q — marketplace %q not available", p.ID, mpName))
			continue
		}

		// Look up marketplace in Claude Code data.
		if claudeMarketplaces == nil {
			hooks.OnInfo(fmt.Sprintf("skipping plugin %q — marketplace %q not available", p.ID, mpName))
			skippedMkts[mpName] = true
			continue
		}
		mkt, available := claudeMarketplaces[mpName]
		if !available {
			hooks.OnInfo(fmt.Sprintf("skipping plugin %q — marketplace %q not available in Claude Code", p.ID, mpName))
			skippedMkts[mpName] = true
			continue
		}

		mktGroups[mpName] = &mktGroup{mpName: mpName, mkt: mkt, plugins: []manifest.Plugin{p}}
		mktOrder = append(mktOrder, mpName)
	}

	// Resolve marketplace dependencies via OnCascade.
	approvedMkts = map[string]manifest.Marketplace{}
	exportable = append(exportable, noMktPlugins...)

	for _, mpName := range mktOrder {
		grp := mktGroups[mpName]
		depNames := make([]string, len(grp.plugins))
		for i, p := range grp.plugins {
			depNames[i] = p.ID
		}

		ok, err := hooks.OnCascade(mpName, depNames)
		if err != nil {
			return nil, nil, err
		}
		if !ok {
			for _, p := range grp.plugins {
				hooks.OnInfo(fmt.Sprintf("skipping plugin %q — marketplace %q declined", p.ID, mpName))
			}
			continue
		}

		approvedMkts[mpName] = grp.mkt
		exportable = append(exportable, grp.plugins...)
	}

	return exportable, approvedMkts, nil
}

// ApplyPluginUpserts merges approved marketplaces and exportable plugins into
// m in-place and returns item actions. This is the pure mutation step — no
// file I/O — suitable for composing inside a single manifest.Update call.
func ApplyPluginUpserts(m *manifest.Manifest, exportable []manifest.Plugin, approvedMkts map[string]manifest.Marketplace) []ItemAction {
	var actions []ItemAction

	// Auto-export approved marketplaces.
	if m.Marketplaces == nil && len(approvedMkts) > 0 {
		m.Marketplaces = make(map[string]manifest.Marketplace)
	}
	for mpName, mkt := range approvedMkts {
		m.Marketplaces[mpName] = mkt
		actions = append(actions, ItemAction{
			Section: SectionMarketplace,
			Name:    mpName,
			Action:  ActionCascaded,
		})
	}

	// Upsert plugins.
	for _, p := range exportable {
		action := ActionAdded
		idx := manifest.FindPlugin(m.Plugins, p.ID)
		if idx >= 0 {
			m.Plugins[idx] = p
			action = ActionUpdated
		} else {
			m.Plugins = append(m.Plugins, p)
		}
		actions = append(actions, ItemAction{
			Section: SectionPlugin,
			Name:    p.ID,
			Action:  action,
		})
	}

	return actions
}

// BulkExportInput groups the slices/maps for ApplyBulkExportToManifest,
// preventing accidental parameter transposition at call sites.
type BulkExportInput struct {
	MCPDesired    map[string]manifest.MCPServer
	MCPAdd        []string
	MCPOverwrite  []string
	PlugDesired   []manifest.Plugin
	PlugAdd       []string
	PlugOverwrite []string
}

// ApplyBulkExportToManifest upserts MCP servers and plugins from the
// add+overwrite lists into the manifest in a single atomic update. This is the
// engine-level function backing the TUI's bulk Apply when the target is Manifest.
// No strict removal is needed (strict is never enabled when target=Manifest).
//
// claudeMarketplaces provides the set of marketplaces available in Claude Code.
// Plugins with marketplace dependencies (id@marketplace) trigger auto-export of
// the marketplace into the manifest, matching the per-item export path
// (ResolvePluginExports). Pass nil if marketplace data is unavailable — plugins
// with marketplace deps will still be exported but their marketplace won't be
// auto-added.
func ApplyBulkExportToManifest(
	manPath string,
	input BulkExportInput,
	claudeMarketplaces map[string]manifest.Marketplace,
	hooks Hooks,
) error {
	if err := ensureManifest(manPath, hooks); err != nil {
		return err
	}

	// Filter MCP servers to only those in add+overwrite.
	filtered := make(map[string]manifest.MCPServer, len(input.MCPAdd)+len(input.MCPOverwrite))
	for _, name := range input.MCPAdd {
		if srv, ok := input.MCPDesired[name]; ok {
			filtered[name] = srv
		}
	}
	for _, name := range input.MCPOverwrite {
		if srv, ok := input.MCPDesired[name]; ok {
			filtered[name] = srv
		}
	}

	// Filter plugins to only those in add+overwrite.
	plugSet := make(map[string]bool, len(input.PlugAdd)+len(input.PlugOverwrite))
	for _, id := range input.PlugAdd {
		plugSet[id] = true
	}
	for _, id := range input.PlugOverwrite {
		plugSet[id] = true
	}
	var filteredPlugins []manifest.Plugin
	for _, p := range input.PlugDesired {
		if plugSet[p.ID] {
			filteredPlugins = append(filteredPlugins, p)
		}
	}

	// Resolve marketplace dependencies outside the lock (OnCascade may prompt).
	// This mirrors the per-item export path: ResolvePluginExports runs before
	// manifest.Update, and approvedMkts is passed into ApplyPluginUpserts.
	exportable, approvedMkts, err := ResolvePluginExports(manPath, filteredPlugins, claudeMarketplaces, hooks)
	if err != nil {
		return fmt.Errorf("resolving plugin marketplace deps: %w", err)
	}

	// Actions from ApplyMCPUpserts/ApplyPluginUpserts are intentionally
	// discarded — the TUI triggers a full state refresh after bulk apply,
	// making per-item action tracking unnecessary.
	return manifest.Update(manPath, func(m *manifest.Manifest) error {
		ApplyMCPUpserts(m, filtered)
		ApplyPluginUpserts(m, exportable, approvedMkts)
		return nil
	})
}

// ExportMarketplaces upserts marketplaces from Claude Code into the manifest.
// The manifest is created if it does not yet exist.
func ExportMarketplaces(manPath string, marketplaces map[string]manifest.Marketplace, hooks Hooks) (*Result, error) {
	if err := ensureManifest(manPath, hooks); err != nil {
		return nil, err
	}

	result := &Result{}
	if err := manifest.Update(manPath, func(m *manifest.Manifest) error {
		if m.Marketplaces == nil {
			m.Marketplaces = make(map[string]manifest.Marketplace)
		}
		for name, mkt := range marketplaces {
			action := ActionAdded
			if _, exists := m.Marketplaces[name]; exists {
				action = ActionUpdated
			}
			m.Marketplaces[name] = mkt
			result.Actions = append(result.Actions, ItemAction{
				Section: SectionMarketplace,
				Name:    name,
				Action:  action,
			})
		}
		return nil
	}); err != nil {
		return nil, err
	}

	return result, nil
}

// ensureManifest creates an empty manifest on disk if none exists.
// manifest.Update calls Load under a file lock, which would fail with
// ErrNotExist on a brand-new machine. Writing here first ensures
// Update always finds a parseable file.
func ensureManifest(manPath string, hooks Hooks) error {
	m, created, err := manifest.LoadOrCreate(manPath)
	if err != nil {
		return err
	}
	if created {
		if err := manifest.Save(manPath, m); err != nil {
			return err
		}
		hooks.OnInfo(fmt.Sprintf("Created manifest at %s", manPath))
	}
	return nil
}
