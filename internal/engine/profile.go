package engine

import (
	"fmt"
	"sort"

	"github.com/alphaleonis/cctote/internal/manifest"
)

// ResolveProfile resolves a named profile to its referenced MCP servers and
// plugins from the manifest. Returns an error if the profile or any of its
// referenced entries doesn't exist in the manifest.
//
// This is the canonical profile resolution function shared by CLI and TUI.
func ResolveProfile(m *manifest.Manifest, name string) (map[string]manifest.MCPServer, []manifest.Plugin, error) {
	profile, ok := m.Profiles[name]
	if !ok {
		return nil, nil, fmt.Errorf("profile %q not found", name)
	}

	mcpServers := make(map[string]manifest.MCPServer, len(profile.MCPServers))
	for _, srvName := range profile.MCPServers {
		srv, exists := m.MCPServers[srvName]
		if !exists {
			return nil, nil, fmt.Errorf("profile %q references MCP server %q, which is not in the manifest", name, srvName)
		}
		mcpServers[srvName] = srv
	}

	plugins := make([]manifest.Plugin, 0, len(profile.Plugins))
	for _, pp := range profile.Plugins {
		idx := manifest.FindPlugin(m.Plugins, pp.ID)
		if idx < 0 {
			return nil, nil, fmt.Errorf("profile %q references plugin %q, which is not in the manifest", name, pp.ID)
		}
		p := m.Plugins[idx]
		if pp.Enabled != nil {
			p.Enabled = *pp.Enabled
		}
		plugins = append(plugins, p)
	}

	return mcpServers, plugins, nil
}

// ResolveProfileLenient resolves a named profile to its referenced MCP servers
// and plugins from the manifest, silently skipping missing references.
// Use this for display/filtering contexts where missing refs are non-fatal.
// Returns ok=false if the profile itself doesn't exist.
func ResolveProfileLenient(m *manifest.Manifest, name string) (mcpServers map[string]manifest.MCPServer, plugins []manifest.Plugin, ok bool) {
	profile, exists := m.Profiles[name]
	if !exists {
		return nil, nil, false
	}

	mcpServers = make(map[string]manifest.MCPServer, len(profile.MCPServers))
	for _, srvName := range profile.MCPServers {
		if srv, found := m.MCPServers[srvName]; found {
			mcpServers[srvName] = srv
		}
	}

	pmap := PluginMap(m.Plugins)
	plugins = make([]manifest.Plugin, 0, len(profile.Plugins))
	for _, pp := range profile.Plugins {
		if p, found := pmap[pp.ID]; found {
			if pp.Enabled != nil {
				p.Enabled = *pp.Enabled
			}
			plugins = append(plugins, p)
		}
	}

	return mcpServers, plugins, true
}

// CheckPluginMarketplacePrereqs checks that plugins being added have their
// marketplace dependencies available in Claude Code. Returns an error for the
// first plugin whose marketplace is missing, or nil if all prerequisites are
// satisfied. Plugins without marketplace dependencies are skipped.
func CheckPluginMarketplacePrereqs(pluginsToAdd []string, installedMarketplaces map[string]manifest.Marketplace) error {
	for _, pid := range pluginsToAdd {
		mpName := MarketplaceFromPluginID(pid)
		if mpName == "" {
			continue
		}
		if _, ok := installedMarketplaces[mpName]; !ok {
			return fmt.Errorf("plugin %q requires marketplace %q, which is not available in Claude Code — run 'cctote marketplace import %s' first", pid, mpName, mpName)
		}
	}
	return nil
}

// SnapshotProfile creates (or updates) a profile by exporting the given
// installed state into the manifest. It handles marketplace dependency
// resolution via [ResolvePluginExports], auto-exports approved marketplaces,
// and produces deterministic (sorted) profile reference lists.
//
// This is the engine-level function shared by CLI (runProfileSnapshot) and
// TUI (doProfileCreate). Presentation-layer decisions are delegated via hooks.
//
// When mustExist is false, the profile must not already exist (create mode).
// When mustExist is true, the profile must already exist (update mode).
func SnapshotProfile(manPath, name string, mustExist bool, installed map[string]manifest.MCPServer, plugins []manifest.Plugin, claudeMarketplaces map[string]manifest.Marketplace, hooks Hooks) (*Result, error) {
	if err := ensureManifest(manPath, hooks); err != nil {
		return nil, err
	}

	// Pre-validate profile existence (fast fail before marketplace prompts).
	// This is an optimization only — the Update callback re-checks under
	// the lock and is the authoritative guard.
	m, err := manifest.Load(manPath)
	if err != nil {
		return nil, fmt.Errorf("loading manifest: %w", err)
	}
	if mustExist {
		if _, ok := m.Profiles[name]; !ok {
			return nil, fmt.Errorf("profile %q not found", name)
		}
	} else {
		if _, ok := m.Profiles[name]; ok {
			return nil, fmt.Errorf("profile %q already exists (use 'profile update' to overwrite)", name)
		}
	}

	// Resolve plugin marketplace dependencies (prompts happen here, outside any lock).
	exportable, approvedMkts, err := ResolvePluginExports(manPath, plugins, claudeMarketplaces, hooks)
	if err != nil {
		return nil, err
	}

	// Build sorted MCP reference list for deterministic output.
	mcpNames := sortedMapKeys(installed)

	// Single atomic write: MCP servers + plugins + marketplaces + profile entry.
	// Re-check profile existence under the lock to close the TOCTOU gap
	// between the pre-validation Load and this Update. Enabled overrides are
	// also read under the lock to avoid a stale-read race.
	result := &Result{}
	if err := manifest.Update(manPath, func(m *manifest.Manifest) error {
		_, exists := m.Profiles[name]
		if mustExist && !exists {
			return fmt.Errorf("profile %q not found", name)
		}
		if !mustExist && exists {
			return fmt.Errorf("profile %q already exists (use 'profile update' to overwrite)", name)
		}
		mcpActions := ApplyMCPUpserts(m, installed)
		pluginActions := ApplyPluginUpserts(m, exportable, approvedMkts)
		result.Actions = append(mcpActions, pluginActions...)

		// Build pluginRefs inside the lock so we read Enabled overrides
		// from the fresh, lock-protected manifest (not the pre-lock snapshot).
		var pluginRefs []manifest.ProfilePlugin
		for _, p := range exportable {
			pp := manifest.ProfilePlugin{ID: p.ID}
			if mustExist {
				if existing, ok := m.Profiles[name]; ok {
					if idx := manifest.FindProfilePlugin(existing.Plugins, p.ID); idx >= 0 {
						pp.Enabled = existing.Plugins[idx].Enabled
					}
				}
			}
			pluginRefs = append(pluginRefs, pp)
		}
		sort.Slice(pluginRefs, func(i, j int) bool {
			return pluginRefs[i].ID < pluginRefs[j].ID
		})

		if m.Profiles == nil {
			m.Profiles = map[string]manifest.Profile{}
		}
		m.Profiles[name] = manifest.Profile{MCPServers: mcpNames, Plugins: pluginRefs}
		return nil
	}); err != nil {
		return nil, err
	}

	return result, nil
}

// sortedMapKeys returns the sorted keys from any map[string]V.
func sortedMapKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
