package tui

// Type definitions and sync logic have moved to internal/engine.
// This file re-exports them for backward compatibility within the tui package.

import "github.com/alphaleonis/cctote/internal/engine"

// Type aliases — canonical definitions live in internal/engine.
type SyncStatus = engine.SyncStatus
type ItemSync = engine.ItemSync
type SectionKind = engine.SectionKind
type SyncState = engine.SyncState
type SourceData = engine.SourceData
type FullState = engine.FullState

// Re-export status constants.
const (
	Synced    = engine.Synced
	Different = engine.Different
	LeftOnly  = engine.LeftOnly
	RightOnly = engine.RightOnly
)

// Re-export section constants.
const (
	SectionMCP         = engine.SectionMCP
	SectionPlugin      = engine.SectionPlugin
	SectionMarketplace = engine.SectionMarketplace
)

// Re-export functions.
var (
	ComputeSyncState = engine.ComputeSyncState
	CompareSources   = engine.CompareSources
	SortedKeys       = engine.SortedKeys
)
