package engine

import (
	"sort"

	"github.com/alphaleonis/cctote/internal/manifest"
)

// SyncStatus indicates the sync state of an item.
type SyncStatus int

const (
	Synced    SyncStatus = iota // Identical on both sides
	Different                   // Present on both sides but differs
	LeftOnly                    // Present only in left source
	RightOnly                   // Present only in right source
)

// ItemSync holds an item's sync status and both-side values.
type ItemSync struct {
	Status SyncStatus
	Left   any // left-source value (nil if RightOnly)
	Right  any // right-source value (nil if LeftOnly)
}

// SectionKind identifies a config section.
type SectionKind int

const (
	SectionMCP SectionKind = iota
	SectionPlugin
	SectionMarketplace
)

// SourceData is the flattened item set from any single source (manifest,
// profile, or Claude Code). Used as input to CompareSources.
type SourceData struct {
	MCP          map[string]manifest.MCPServer
	Plugins      []manifest.Plugin
	Marketplaces map[string]manifest.Marketplace
}

// FullState holds raw loaded data, immutable between refreshes.
// It carries everything needed to build SourceData for any pane source.
type FullState struct {
	Manifest      *manifest.Manifest
	MCPInstalled  map[string]manifest.MCPServer
	PlugInstalled []manifest.Plugin
	MktInstalled  map[string]manifest.Marketplace

	// Project-level data (populated from working directory).
	// Claude Code uses cwd for project scoping, not git root.
	ProjectMCP     map[string]manifest.MCPServer // from .mcp.json
	ProjectPlugins []manifest.Plugin             // project-scoped plugins
	ProjectRoot    string                        // working directory, empty if unset
}

// SyncState holds the computed sync comparison between two sources.
type SyncState struct {
	MCPSync  map[string]ItemSync
	PlugSync map[string]ItemSync
	MktSync  map[string]ItemSync
}

// ComputeSyncState compares manifest data against Claude Code's installed state
// and returns a SyncState with per-item sync information. This is a convenience
// wrapper around CompareSources for callers that have raw manifest + Claude data.
func ComputeSyncState(
	m *manifest.Manifest,
	claudeMCP map[string]manifest.MCPServer,
	claudePlugins []manifest.Plugin,
	claudeMarketplaces map[string]manifest.Marketplace,
) *SyncState {
	left := SourceData{
		MCP:          m.MCPServers,
		Plugins:      m.Plugins,
		Marketplaces: m.Marketplaces,
	}
	right := SourceData{
		MCP:          claudeMCP,
		Plugins:      claudePlugins,
		Marketplaces: claudeMarketplaces,
	}
	return CompareSources(left, right)
}

// CompareSources compares two SourceData sets and returns a SyncState with
// per-item sync information. Either source can be any combination of manifest,
// profile, or Claude Code data.
func CompareSources(left, right SourceData) *SyncState {
	s := &SyncState{
		MCPSync:  make(map[string]ItemSync),
		PlugSync: make(map[string]ItemSync),
		MktSync:  make(map[string]ItemSync),
	}

	leftMCP := left.MCP
	if leftMCP == nil {
		leftMCP = map[string]manifest.MCPServer{}
	}
	rightMCP := right.MCP
	if rightMCP == nil {
		rightMCP = map[string]manifest.MCPServer{}
	}

	// MCP servers
	for name, lSrv := range leftMCP {
		rSrv, inRight := rightMCP[name]
		if !inRight {
			s.MCPSync[name] = ItemSync{Status: LeftOnly, Left: lSrv}
		} else if manifest.MCPServersEqual(lSrv, rSrv) {
			s.MCPSync[name] = ItemSync{Status: Synced, Left: lSrv, Right: rSrv}
		} else {
			s.MCPSync[name] = ItemSync{Status: Different, Left: lSrv, Right: rSrv}
		}
	}
	for name, rSrv := range rightMCP {
		if _, inLeft := leftMCP[name]; !inLeft {
			s.MCPSync[name] = ItemSync{Status: RightOnly, Right: rSrv}
		}
	}

	// Plugins (keyed by ID)
	leftPlugMap := PluginMap(left.Plugins)
	rightPlugMap := PluginMap(right.Plugins)

	for pid, lP := range leftPlugMap {
		rP, inRight := rightPlugMap[pid]
		if !inRight {
			s.PlugSync[pid] = ItemSync{Status: LeftOnly, Left: lP}
		} else if manifest.PluginsEqual(lP, rP) {
			s.PlugSync[pid] = ItemSync{Status: Synced, Left: lP, Right: rP}
		} else {
			s.PlugSync[pid] = ItemSync{Status: Different, Left: lP, Right: rP}
		}
	}
	for pid, rP := range rightPlugMap {
		if _, inLeft := leftPlugMap[pid]; !inLeft {
			s.PlugSync[pid] = ItemSync{Status: RightOnly, Right: rP}
		}
	}

	// Marketplaces
	leftMkt := left.Marketplaces
	if leftMkt == nil {
		leftMkt = map[string]manifest.Marketplace{}
	}
	rightMkt := right.Marketplaces
	if rightMkt == nil {
		rightMkt = map[string]manifest.Marketplace{}
	}

	for name, lMP := range leftMkt {
		rMP, inRight := rightMkt[name]
		if !inRight {
			s.MktSync[name] = ItemSync{Status: LeftOnly, Left: lMP}
		} else if manifest.MarketplacesEqual(lMP, rMP) {
			s.MktSync[name] = ItemSync{Status: Synced, Left: lMP, Right: rMP}
		} else {
			s.MktSync[name] = ItemSync{Status: Different, Left: lMP, Right: rMP}
		}
	}
	for name, rMP := range rightMkt {
		if _, inLeft := leftMkt[name]; !inLeft {
			s.MktSync[name] = ItemSync{Status: RightOnly, Right: rMP}
		}
	}

	return s
}

// Counts returns aggregate sync status counts across all sections.
func (s *SyncState) Counts() (synced, different, leftOnly, rightOnly int) {
	for _, m := range []map[string]ItemSync{s.MCPSync, s.PlugSync, s.MktSync} {
		for _, item := range m {
			switch item.Status {
			case Synced:
				synced++
			case Different:
				different++
			case LeftOnly:
				leftOnly++
			case RightOnly:
				rightOnly++
			}
		}
	}
	return
}

// SortedKeys returns the sorted keys from a sync map.
func SortedKeys(m map[string]ItemSync) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// PluginMap builds a map from plugin ID to Plugin for O(1) lookup.
func PluginMap(plugins []manifest.Plugin) map[string]manifest.Plugin {
	m := make(map[string]manifest.Plugin, len(plugins))
	for _, p := range plugins {
		m[p.ID] = p
	}
	return m
}
