package engine

import (
	"sort"

	"github.com/alphaleonis/cctote/internal/manifest"
	"github.com/alphaleonis/cctote/internal/mcp"
)

// ImportPlan classifies items into buckets for a bulk import operation.
// Slices are always non-nil (empty, not null) for clean JSON serialization.
type ImportPlan struct {
	Add      []string // in desired, not in current
	Skip     []string // in both, identical
	Conflict []string // in both, different
	Remove   []string // in current, not in desired (strict mode only)
}

func newImportPlan() *ImportPlan {
	return &ImportPlan{
		Add: []string{}, Skip: []string{}, Conflict: []string{}, Remove: []string{},
	}
}

func (p *ImportPlan) sort() {
	sort.Strings(p.Add)
	sort.Strings(p.Skip)
	sort.Strings(p.Conflict)
	sort.Strings(p.Remove)
}

// ClassifyMCPImport compares desired MCP servers against the current installed
// state and returns an import plan.
func ClassifyMCPImport(desired, current map[string]manifest.MCPServer, strict bool) *ImportPlan {
	plan := newImportPlan()
	for name := range desired {
		cur, exists := current[name]
		if !exists {
			plan.Add = append(plan.Add, name)
		} else if manifest.MCPServersEqual(cur, desired[name]) {
			plan.Skip = append(plan.Skip, name)
		} else {
			plan.Conflict = append(plan.Conflict, name)
		}
	}
	if strict {
		for name := range current {
			if _, inSet := desired[name]; !inSet {
				plan.Remove = append(plan.Remove, name)
			}
		}
	}
	plan.sort()
	return plan
}

// ClassifyPluginImport compares desired plugins against the current installed
// state and returns an import plan.
//
// Terminology mapping for CLI display:
//   - Add      → install (not yet in Claude Code)
//   - Conflict → reconcile (installed, but different enabled/scope)
//   - Remove   → uninstall (strict mode)
func ClassifyPluginImport(desired, current []manifest.Plugin, strict bool) *ImportPlan {
	plan := newImportPlan()
	currentMap := PluginMap(current)
	desiredMap := PluginMap(desired)
	for _, p := range desired {
		cur, exists := currentMap[p.ID]
		if !exists {
			plan.Add = append(plan.Add, p.ID)
		} else if !manifest.PluginsEqual(cur, p) {
			plan.Conflict = append(plan.Conflict, p.ID)
		} else {
			plan.Skip = append(plan.Skip, p.ID)
		}
	}
	if strict {
		for _, cur := range current {
			if _, inSet := desiredMap[cur.ID]; !inSet {
				plan.Remove = append(plan.Remove, cur.ID)
			}
		}
	}
	plan.sort()
	return plan
}

// ClassifyMarketplaceImport compares desired marketplaces against the current
// installed state and returns an import plan.
func ClassifyMarketplaceImport(desired, current map[string]manifest.Marketplace, strict bool) *ImportPlan {
	plan := newImportPlan()
	for name := range desired {
		cur, exists := current[name]
		if !exists {
			plan.Add = append(plan.Add, name)
		} else if manifest.MarketplacesEqual(cur, desired[name]) {
			plan.Skip = append(plan.Skip, name)
		} else {
			plan.Conflict = append(plan.Conflict, name)
		}
	}
	if strict {
		for name := range current {
			if _, inSet := desired[name]; !inSet {
				plan.Remove = append(plan.Remove, name)
			}
		}
	}
	plan.sort()
	return plan
}

// ApplyMCPImport atomically merges MCP server changes into the Claude Code
// config file (~/.claude.json). Items in add/overwrite are taken from desired;
// items in remove are deleted from the config. The merge re-reads the config
// under a file lock, so concurrent changes by Claude Code are preserved.
func ApplyMCPImport(claudePath string, desired map[string]manifest.MCPServer, add, overwrite, remove []string) error {
	return mcp.UpdateMcpServers(claudePath, func(current map[string]manifest.MCPServer) (map[string]manifest.MCPServer, error) {
		final := make(map[string]manifest.MCPServer, len(current))
		for name, srv := range current {
			final[name] = srv
		}
		for _, name := range add {
			final[name] = desired[name]
		}
		for _, name := range overwrite {
			final[name] = desired[name]
		}
		for _, name := range remove {
			delete(final, name)
		}
		return final, nil
	})
}

// ApplyMCPImportToProject atomically merges MCP server changes into a
// project-level .mcp.json file. Mirrors [ApplyMCPImport] but writes to
// the flat JSON format used by project configs.
func ApplyMCPImportToProject(mcpPath string, desired map[string]manifest.MCPServer, add, overwrite, remove []string) error {
	return mcp.UpdateProjectMcpServers(mcpPath, func(current map[string]manifest.MCPServer) error {
		for _, name := range add {
			current[name] = desired[name]
		}
		for _, name := range overwrite {
			current[name] = desired[name]
		}
		for _, name := range remove {
			delete(current, name)
		}
		return nil
	})
}
