package engine

import (
	"fmt"
	"sort"

	"github.com/alphaleonis/cctote/internal/manifest"
)

// DeleteMCP removes an MCP server from the manifest and cascade-cleans
// profile references. If profiles reference the server, hooks.OnCascade is
// called; returning false aborts the operation (nil result, nil error).
func DeleteMCP(manPath, name string, hooks Hooks) (*Result, error) {
	m, err := manifest.Load(manPath)
	if err != nil {
		return nil, fmt.Errorf("loading manifest: %w", err)
	}

	if _, ok := m.MCPServers[name]; !ok {
		return nil, fmt.Errorf("MCP server %q not found in manifest", name)
	}

	refs := FindMCPProfileRefs(m, name)

	if len(refs) > 0 {
		ok, err := hooks.OnCascade(name, refs)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, nil
		}
	}

	var cleanedProfiles []string
	if err := manifest.Update(manPath, func(m *manifest.Manifest) error {
		// Re-check under the lock to close the TOCTOU gap with the
		// pre-validation Load above.
		if _, ok := m.MCPServers[name]; !ok {
			return fmt.Errorf("MCP server %q not found in manifest", name)
		}
		delete(m.MCPServers, name)
		cleanedProfiles = cleanMCPProfileRefs(m, name)
		return nil
	}); err != nil {
		return nil, err
	}

	return &Result{
		Actions:         []ItemAction{{Section: SectionMCP, Name: name, Action: ActionRemoved}},
		CleanedProfiles: cleanedProfiles,
	}, nil
}

// DeletePlugin removes a plugin from the manifest and cascade-cleans
// profile references. If profiles reference the plugin, hooks.OnCascade is
// called; returning false aborts the operation (nil result, nil error).
func DeletePlugin(manPath, id string, hooks Hooks) (*Result, error) {
	m, err := manifest.Load(manPath)
	if err != nil {
		return nil, fmt.Errorf("loading manifest: %w", err)
	}

	if manifest.FindPlugin(m.Plugins, id) < 0 {
		return nil, fmt.Errorf("plugin %q not found in manifest", id)
	}

	refs := FindPluginProfileRefs(m, id)

	if len(refs) > 0 {
		ok, err := hooks.OnCascade(id, refs)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, nil
		}
	}

	var cleanedProfiles []string
	if err := manifest.Update(manPath, func(m *manifest.Manifest) error {
		// Re-check under the lock to close the TOCTOU gap with the
		// pre-validation Load above.
		if manifest.FindPlugin(m.Plugins, id) < 0 {
			return fmt.Errorf("plugin %q not found in manifest", id)
		}
		m.Plugins = removePlugin(m.Plugins, id)
		cleanedProfiles = cleanPluginProfileRefs(m, id)
		return nil
	}); err != nil {
		return nil, err
	}

	return &Result{
		Actions:         []ItemAction{{Section: SectionPlugin, Name: id, Action: ActionRemoved}},
		CleanedProfiles: cleanedProfiles,
	}, nil
}

// DeleteMarketplace removes a marketplace from the manifest, cascade-removes
// plugins belonging to that marketplace, and cascade-cleans profile references.
// If there are affected plugins, hooks.OnCascade is called with the plugin IDs;
// returning false aborts the operation (nil result, nil error).
func DeleteMarketplace(manPath, name string, hooks Hooks) (*Result, error) {
	m, err := manifest.Load(manPath)
	if err != nil {
		return nil, fmt.Errorf("loading manifest: %w", err)
	}

	if _, ok := m.Marketplaces[name]; !ok {
		return nil, fmt.Errorf("marketplace %q not found in manifest", name)
	}

	affectedPlugins := FindMarketplacePlugins(m, name)

	if len(affectedPlugins) > 0 {
		ok, err := hooks.OnCascade(name, affectedPlugins)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, nil
		}
	}

	var cleanedProfiles []string
	if err := manifest.Update(manPath, func(m *manifest.Manifest) error {
		// Build set of affected plugin IDs (re-check under lock).
		affected := make(map[string]bool)
		for _, p := range m.Plugins {
			if MarketplaceFromPluginID(p.ID) == name {
				affected[p.ID] = true
			}
		}

		// Remove affected plugins.
		if len(affected) > 0 {
			filtered := make([]manifest.Plugin, 0, len(m.Plugins))
			for _, p := range m.Plugins {
				if !affected[p.ID] {
					filtered = append(filtered, p)
				}
			}
			m.Plugins = filtered

			// Clean profile references for removed plugins.
			for pName, profile := range m.Profiles {
				clean := make([]manifest.ProfilePlugin, 0, len(profile.Plugins))
				for _, pp := range profile.Plugins {
					if !affected[pp.ID] {
						clean = append(clean, pp)
					}
				}
				if len(clean) != len(profile.Plugins) {
					profile.Plugins = clean
					m.Profiles[pName] = profile
					cleanedProfiles = append(cleanedProfiles, pName)
				}
			}
		}

		delete(m.Marketplaces, name)
		return nil
	}); err != nil {
		return nil, err
	}

	sort.Strings(cleanedProfiles)

	actions := []ItemAction{{Section: SectionMarketplace, Name: name, Action: ActionRemoved}}
	for _, pid := range affectedPlugins {
		actions = append(actions, ItemAction{Section: SectionPlugin, Name: pid, Action: ActionCascaded})
	}

	return &Result{
		Actions:         actions,
		CleanedProfiles: cleanedProfiles,
	}, nil
}

// --- helpers ---

func removePlugin(plugins []manifest.Plugin, id string) []manifest.Plugin {
	filtered := make([]manifest.Plugin, 0, len(plugins))
	for _, p := range plugins {
		if p.ID != id {
			filtered = append(filtered, p)
		}
	}
	return filtered
}

// FindMarketplacePlugins returns sorted plugin IDs belonging to the given marketplace.
func FindMarketplacePlugins(m *manifest.Manifest, name string) []string {
	var ids []string
	for _, p := range m.Plugins {
		if MarketplaceFromPluginID(p.ID) == name {
			ids = append(ids, p.ID)
		}
	}
	sort.Strings(ids)
	return ids
}

// FindMCPProfileRefs returns sorted profile names that reference the given MCP server.
func FindMCPProfileRefs(m *manifest.Manifest, name string) []string {
	var refs []string
	for pName, profile := range m.Profiles {
		for _, s := range profile.MCPServers {
			if s == name {
				refs = append(refs, pName)
				break
			}
		}
	}
	sort.Strings(refs)
	return refs
}

// FindPluginProfileRefs returns sorted profile names that reference the given plugin.
func FindPluginProfileRefs(m *manifest.Manifest, id string) []string {
	var refs []string
	for pName, profile := range m.Profiles {
		if manifest.FindProfilePlugin(profile.Plugins, id) >= 0 {
			refs = append(refs, pName)
		}
	}
	sort.Strings(refs)
	return refs
}

func cleanMCPProfileRefs(m *manifest.Manifest, name string) []string {
	var cleaned []string
	for pName, profile := range m.Profiles {
		filtered := make([]string, 0, len(profile.MCPServers))
		for _, s := range profile.MCPServers {
			if s != name {
				filtered = append(filtered, s)
			}
		}
		if len(filtered) != len(profile.MCPServers) {
			profile.MCPServers = filtered
			m.Profiles[pName] = profile
			cleaned = append(cleaned, pName)
		}
	}
	sort.Strings(cleaned)
	return cleaned
}

func cleanPluginProfileRefs(m *manifest.Manifest, id string) []string {
	var cleaned []string
	for pName, profile := range m.Profiles {
		filtered := make([]manifest.ProfilePlugin, 0, len(profile.Plugins))
		for _, pp := range profile.Plugins {
			if pp.ID != id {
				filtered = append(filtered, pp)
			}
		}
		if len(filtered) != len(profile.Plugins) {
			profile.Plugins = filtered
			m.Profiles[pName] = profile
			cleaned = append(cleaned, pName)
		}
	}
	sort.Strings(cleaned)
	return cleaned
}
