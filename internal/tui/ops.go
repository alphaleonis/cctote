package tui

import (
	"context"
	"fmt"
	"path/filepath"

	tea "charm.land/bubbletea/v2"
	"github.com/alphaleonis/cctote/internal/engine"
	"github.com/alphaleonis/cctote/internal/manifest"
	"github.com/alphaleonis/cctote/internal/mcp"
)

// resolveManPath returns the manifest path from Options, falling back to the
// XDG default. This replaces the 6-line boilerplate repeated across ops.go/tui.go.
func resolveManPath(opts Options) (string, error) {
	if opts.ManifestPath != "" {
		return opts.ManifestPath, nil
	}
	p, err := manifest.DefaultPath()
	if err != nil {
		return "", fmt.Errorf("resolving manifest path: %w", err)
	}
	return p, nil
}

// tuiHooks auto-approves cascades. The TUI shows cascade details in the
// confirm overlay (via buildCascadeLines) before executeCopy dispatches,
// so the engine's OnCascade prompt is a no-op here.
// When warnings is non-nil, OnInfo and OnWarn both collect messages into the
// same slice for later display. Severity distinction is intentionally lost —
// the TUI currently has no separate warn style (FlashWarn), so all messages
// are rendered as FlashInfo (cyan). Split into separate slices when a yellow
// flash style is added.
type tuiHooks struct {
	warnings *[]string
}

func (tuiHooks) OnCascade(_ string, _ []string) (bool, error) { return true, nil }
func (h tuiHooks) OnInfo(msg string) {
	if h.warnings != nil {
		*h.warnings = append(*h.warnings, msg)
	}
}
func (h tuiHooks) OnWarn(msg string) {
	if h.warnings != nil {
		*h.warnings = append(*h.warnings, msg)
	}
}

// dispatchOp executes a single copy operation for the given item.
func dispatchOp(opts Options, item CopyItem, op ResolvedOp,
	fromSource, toSource PaneSource,
	opState *SyncState, fullState *FullState) error {
	switch op {
	case OpExportToManifest, OpExportProjectToManifest:
		return execExport(opts, item, opState, "", fullState)
	case OpImportToClaude:
		return execImport(opts, item, opState, fullState)
	case OpAddToProfile:
		return execAddToProfile(opts, item, toSource.ProfileName)
	case OpExportAndAddProfile:
		return execExport(opts, item, opState, toSource.ProfileName, fullState)
	case OpCopyProfileRef:
		return execCopyProfileRef(opts, item, fromSource.ProfileName, toSource.ProfileName)
	case OpImportToProject:
		return execImportToProject(opts, item, opState, fullState)
	case OpDeleteFromManifest:
		return execDelete(opts, item, "")
	case OpRemoveFromProfile:
		return execDelete(opts, item, fromSource.ProfileName)
	case OpDeleteFromProject:
		return execDeleteFromProject(opts, item, fullState)
	case OpRemoveFromClaude:
		return execRemoveFromClaude(opts, item)
	default:
		return fmt.Errorf("unsupported operation: %d", op)
	}
}

// executeCopy returns a tea.Cmd that runs mutation(s) in a goroutine and
// sends a CopyResultMsg when complete.
func executeCopy(opts Options, items []CopyItem, op ResolvedOp,
	fromSource, toSource PaneSource,
	fullState *FullState) tea.Cmd {

	// Recompute the comparison oriented as from→to so exec functions
	// can consistently read from the Left (from) side.
	fromData := ExtractSourceData(fromSource, fullState)
	toData := ExtractSourceData(toSource, fullState)
	opState := CompareSources(fromData, toData)

	return func() tea.Msg {
		results := make([]ItemResult, len(items))
		for i, item := range items {
			results[i] = ItemResult{Name: item.Name, Err: dispatchOp(opts, item, op, fromSource, toSource, opState, fullState)}
		}
		return CopyResultMsg{Items: items, Op: op, Results: results}
	}
}

// executeCopyWithProgress wraps executeCopy's logic with progress reporting
// via a channel, suitable for operations that invoke the CLI per item.
func executeCopyWithProgress(
	ctx context.Context,
	opts Options,
	items []CopyItem,
	op ResolvedOp,
	fromSource, toSource PaneSource,
	fullState *FullState,
	ch chan<- tea.Msg,
	total int,
) tea.Cmd {
	fromData := ExtractSourceData(fromSource, fullState)
	toData := ExtractSourceData(toSource, fullState)
	opState := CompareSources(fromData, toData)

	return func() tea.Msg {
		defer close(ch)
		defer recoverProgressPanic(ch)

		actionKind := op.ActionKind()

		var results []ItemResult
		var firstErr error
		current := 0
		for _, item := range items {
			if ctx.Err() != nil {
				break
			}
			current++
			ch <- ProgressUpdateMsg{
				Section: item.Section,
				Name:    item.Name,
				Action:  actionKind,
				Current: current,
				Total:   total,
			}
			err := dispatchOp(opts, item, op, fromSource, toSource, opState, fullState)
			results = append(results, ItemResult{Name: item.Name, Err: err})
			ch <- ProgressUpdateMsg{
				Section: item.Section,
				Name:    item.Name,
				Action:  actionKind,
				Done:    true,
				Err:     err,
				Current: current,
				Total:   total,
			}
			if err != nil && firstErr == nil {
				firstErr = err
			}
		}

		manifestDirty := false
		if op.ModifiesManifest() {
			for _, r := range results {
				if r.Err == nil {
					manifestDirty = true
					break
				}
			}
		}
		ch <- ProgressFinishedMsg{
			Err:           firstErr,
			ManifestDirty: manifestDirty,
		}
		return nil
	}
}

// execExport copies an item from the source (Left in the from→to comparison)
// into the manifest. When profileName is set, also adds the item to the
// profile's reference list.
func execExport(opts Options, item CopyItem, state *SyncState, profileName string, fullState *FullState) error {
	manPath, err := resolveManPath(opts)
	if err != nil {
		return err
	}

	hooks := tuiHooks{}

	switch item.Section {
	case SectionMCP:
		sync, ok := state.MCPSync[item.Name]
		if !ok || sync.Left == nil {
			return fmt.Errorf("MCP server %q not found in source", item.Name)
		}
		srv := sync.Left.(manifest.MCPServer)
		if profileName == "" {
			_, err := engine.ExportMCPServers(manPath, map[string]manifest.MCPServer{item.Name: srv}, hooks)
			return err
		}
		// Profile mode: single atomic update for export + profile reference.
		return manifest.Update(manPath, func(m *manifest.Manifest) error {
			engine.ApplyMCPUpserts(m, map[string]manifest.MCPServer{item.Name: srv})
			return addProfileRef(m, profileName, item)
		})

	case SectionPlugin:
		sync, ok := state.PlugSync[item.Name]
		if !ok || sync.Left == nil {
			return fmt.Errorf("plugin %q not found in source", item.Name)
		}
		plug := sync.Left.(manifest.Plugin)

		// Build Claude marketplace data for auto-export of dependencies.
		var claudeMkts map[string]manifest.Marketplace
		if fullState != nil {
			mpName := engine.MarketplaceFromPluginID(plug.ID)
			if mpName != "" {
				if mkt, ok := fullState.MktInstalled[mpName]; ok {
					claudeMkts = map[string]manifest.Marketplace{mpName: mkt}
				}
			}
		}

		if profileName == "" {
			_, err := engine.ExportPlugins(manPath, []manifest.Plugin{plug}, claudeMkts, hooks)
			return err
		}
		// Profile mode: resolve marketplace deps outside the lock, then
		// single atomic update for export + profile reference.
		exportable, approvedMkts, err := engine.ResolvePluginExports(manPath, []manifest.Plugin{plug}, claudeMkts, hooks)
		if err != nil {
			return err
		}
		return manifest.Update(manPath, func(m *manifest.Manifest) error {
			engine.ApplyPluginUpserts(m, exportable, approvedMkts)
			return addProfileRef(m, profileName, item)
		})

	case SectionMarketplace:
		sync, ok := state.MktSync[item.Name]
		if !ok || sync.Left == nil {
			return fmt.Errorf("marketplace %q not found in source", item.Name)
		}
		mkt := sync.Left.(manifest.Marketplace)
		_, err := engine.ExportMarketplaces(manPath, map[string]manifest.Marketplace{item.Name: mkt}, hooks)
		return err
	}

	return fmt.Errorf("unknown section: %d", item.Section)
}

// execImport copies an item from the source (Left in the from→to comparison)
// into Claude Code.
func execImport(opts Options, item CopyItem, state *SyncState, fullState *FullState) error {
	switch item.Section {
	case SectionMCP:
		sync, ok := state.MCPSync[item.Name]
		if !ok || sync.Left == nil {
			return fmt.Errorf("MCP server %q not found in source", item.Name)
		}
		srv := sync.Left.(manifest.MCPServer)

		claudePath, err := mcp.DefaultPath()
		if err != nil {
			return err
		}
		return engine.ApplyMCPImport(claudePath,
			map[string]manifest.MCPServer{item.Name: srv},
			[]string{item.Name}, nil, nil,
		)

	case SectionPlugin:
		sync, ok := state.PlugSync[item.Name]
		if !ok || sync.Left == nil {
			return fmt.Errorf("plugin %q not found in source", item.Name)
		}
		plug := sync.Left.(manifest.Plugin)

		ctx := context.Background()
		client := opts.NewClient()

		// Auto-import the plugin's marketplace dependency if not in Claude Code.
		// Stale marketplace indexes are handled by ApplyPluginImport's retry logic.
		if fullState != nil {
			mpName := engine.MarketplaceFromPluginID(plug.ID)
			if mpName != "" {
				if _, installed := fullState.MktInstalled[mpName]; !installed {
					if fullState.Manifest != nil {
						if mkt, ok := fullState.Manifest.Marketplaces[mpName]; ok {
							source, err := mkt.SourceLocatorE()
							if err != nil {
								return fmt.Errorf("marketplace %q: %w", mpName, err)
							}
							if err := client.AddMarketplace(ctx, source); err != nil {
								return fmt.Errorf("auto-importing marketplace %q: %w", mpName, err)
							}
						}
					}
				}
			}
		}

		// Classify against installed state so conflicts (e.g. enabled-state
		// drift) are properly reconciled rather than re-added.
		var currentPlugins []manifest.Plugin
		if fullState != nil {
			currentPlugins = fullState.PlugInstalled
		}
		plan := engine.ClassifyPluginImport([]manifest.Plugin{plug}, currentPlugins, false)
		desired := map[string]manifest.Plugin{plug.ID: plug}
		currentMap := engine.PluginMap(currentPlugins)
		result := engine.ApplyPluginImport(ctx, client, plan, desired, currentMap, tuiHooks{}, "")
		return result.Err()

	case SectionMarketplace:
		sync, ok := state.MktSync[item.Name]
		if !ok || sync.Left == nil {
			return fmt.Errorf("marketplace %q not found in source", item.Name)
		}
		mkt := sync.Left.(manifest.Marketplace)

		ctx := context.Background()
		client := opts.NewClient()
		plan := &engine.ImportPlan{
			Add:      []string{item.Name},
			Skip:     []string{},
			Conflict: []string{},
			Remove:   []string{},
		}
		desired := map[string]manifest.Marketplace{item.Name: mkt}
		result := engine.ApplyMarketplaceImport(ctx, client, plan, desired, nil, tuiHooks{})
		return result.Err()
	}

	return fmt.Errorf("unknown section: %d", item.Section)
}

// execAddToProfile adds an item's reference to a profile. The item must
// already exist in the manifest.
func execAddToProfile(opts Options, item CopyItem, profileName string) error {
	manPath, err := resolveManPath(opts)
	if err != nil {
		return err
	}
	return manifest.Update(manPath, func(m *manifest.Manifest) error {
		// Verify item exists in manifest.
		switch item.Section {
		case SectionMCP:
			if _, ok := m.MCPServers[item.Name]; !ok {
				return fmt.Errorf("MCP server %q not in manifest", item.Name)
			}
		case SectionPlugin:
			if manifest.FindPlugin(m.Plugins, item.Name) < 0 {
				return fmt.Errorf("plugin %q not in manifest", item.Name)
			}
		}
		return addProfileRef(m, profileName, item)
	})
}

// execCopyProfileRef copies a profile reference from one profile to another.
// The item must exist in the manifest.
func execCopyProfileRef(opts Options, item CopyItem, fromProfile, toProfile string) error {
	manPath, err := resolveManPath(opts)
	if err != nil {
		return err
	}
	return manifest.Update(manPath, func(m *manifest.Manifest) error {
		// Verify item exists in manifest.
		switch item.Section {
		case SectionMCP:
			if _, ok := m.MCPServers[item.Name]; !ok {
				return fmt.Errorf("MCP server %q not in manifest", item.Name)
			}
		case SectionPlugin:
			if manifest.FindPlugin(m.Plugins, item.Name) < 0 {
				return fmt.Errorf("plugin %q not in manifest", item.Name)
			}
		}
		// Verify fromProfile contains the item reference.
		fromP, ok := m.Profiles[fromProfile]
		if !ok {
			return fmt.Errorf("profile %q not found", fromProfile)
		}
		if !profileContains(fromP, item) {
			return fmt.Errorf("%q not found in profile %q", item.Name, fromProfile)
		}
		return addProfileRef(m, toProfile, item)
	})
}

// profileContains checks whether a profile references the given item.
func profileContains(p manifest.Profile, item CopyItem) bool {
	switch item.Section {
	case SectionMCP:
		for _, name := range p.MCPServers {
			if name == item.Name {
				return true
			}
		}
	case SectionPlugin:
		return manifest.FindProfilePlugin(p.Plugins, item.Name) >= 0
		// SectionMarketplace: marketplaces are global, not per-profile.
	}
	return false
}

// addProfileRef adds an item's reference to a profile (no file I/O).
// Suitable for use inside a manifest.Update callback.
func addProfileRef(m *manifest.Manifest, profileName string, item CopyItem) error {
	profile, ok := m.Profiles[profileName]
	if !ok {
		return fmt.Errorf("profile %q not found", profileName)
	}

	switch item.Section {
	case SectionMCP:
		for _, name := range profile.MCPServers {
			if name == item.Name {
				return nil // already in profile
			}
		}
		profile.MCPServers = append(profile.MCPServers, item.Name)

	case SectionPlugin:
		if manifest.FindProfilePlugin(profile.Plugins, item.Name) >= 0 {
			return nil // already in profile
		}
		profile.Plugins = append(profile.Plugins, manifest.ProfilePlugin{ID: item.Name})

	case SectionMarketplace:
		return fmt.Errorf("marketplaces cannot be added to profiles")
	}

	m.Profiles[profileName] = profile
	return nil
}

// removeFromProfile removes an item's reference from a profile's MCP or plugin list.
func removeFromProfile(manPath, profileName string, item CopyItem) error {
	return manifest.Update(manPath, func(m *manifest.Manifest) error {
		profile, ok := m.Profiles[profileName]
		if !ok {
			return fmt.Errorf("profile %q not found", profileName)
		}

		switch item.Section {
		case SectionMCP:
			filtered := profile.MCPServers[:0]
			for _, name := range profile.MCPServers {
				if name != item.Name {
					filtered = append(filtered, name)
				}
			}
			profile.MCPServers = filtered

		case SectionPlugin:
			filtered := profile.Plugins[:0]
			for _, pp := range profile.Plugins {
				if pp.ID != item.Name {
					filtered = append(filtered, pp)
				}
			}
			profile.Plugins = filtered

		case SectionMarketplace:
			return fmt.Errorf("marketplaces cannot be removed from profiles")
		}

		m.Profiles[profileName] = profile
		return nil
	})
}

// execDelete removes an item. When profileName is set, only removes from the
// profile's reference list. When empty, removes from the manifest entirely.
func execDelete(opts Options, item CopyItem, profileName string) error {
	manPath, err := resolveManPath(opts)
	if err != nil {
		return err
	}

	// Profile-scoped delete: only remove from the profile reference list.
	if profileName != "" {
		return removeFromProfile(manPath, profileName, item)
	}

	// Global delete: remove from manifest (with cascade cleanup).
	hooks := tuiHooks{}

	switch item.Section {
	case SectionMCP:
		_, err := engine.DeleteMCP(manPath, item.Name, hooks)
		return err
	case SectionPlugin:
		_, err := engine.DeletePlugin(manPath, item.Name, hooks)
		return err
	case SectionMarketplace:
		_, err := engine.DeleteMarketplace(manPath, item.Name, hooks)
		return err
	}

	return fmt.Errorf("unknown section: %d", item.Section)
}

// projectMcpPath returns the path to the project's .mcp.json, or an error
// if no project root is available.
func projectMcpPath(fullState *FullState) (string, error) {
	if fullState == nil || fullState.ProjectRoot == "" {
		return "", fmt.Errorf("no project root available")
	}
	return filepath.Join(fullState.ProjectRoot, ".mcp.json"), nil
}

// execImportToProject copies an item from the source into the project config.
func execImportToProject(opts Options, item CopyItem, state *SyncState, fullState *FullState) error {
	switch item.Section {
	case SectionMCP:
		// Write directly to .mcp.json instead of using `claude mcp add -s project`
		// because the Claude CLI's mcp add is not idempotent — it creates _1
		// duplicates (see CLAUDE.md). Direct write gives us clean upsert semantics
		// and preserves all server fields (CWD, Headers, OAuth) that the CLI
		// cannot express. This matches the global MCP import path
		// (engine.ApplyMCPImport → mcp.UpdateMcpServers).
		sync, ok := state.MCPSync[item.Name]
		if !ok || sync.Left == nil {
			return fmt.Errorf("MCP server %q not found in source", item.Name)
		}
		srv, ok := sync.Left.(manifest.MCPServer)
		if !ok {
			return fmt.Errorf("MCP server %q: unexpected type %T", item.Name, sync.Left)
		}
		mcpPath, err := projectMcpPath(fullState)
		if err != nil {
			return err
		}
		return mcp.UpdateProjectMcpServers(mcpPath, func(servers map[string]manifest.MCPServer) error {
			servers[item.Name] = srv
			return nil
		})

	case SectionPlugin:
		// Route through the engine classification layer for idempotency
		// checks and error accumulation, matching execImport's plugin path.
		sync, ok := state.PlugSync[item.Name]
		if !ok || sync.Left == nil {
			return fmt.Errorf("plugin %q not found in source", item.Name)
		}
		plug, ok := sync.Left.(manifest.Plugin)
		if !ok {
			return fmt.Errorf("plugin %q: unexpected type %T", item.Name, sync.Left)
		}

		ctx := context.Background()
		client := opts.NewClient()

		var currentPlugins []manifest.Plugin
		if fullState != nil {
			currentPlugins = fullState.ProjectPlugins
		}
		plan := engine.ClassifyPluginImport([]manifest.Plugin{plug}, currentPlugins, false)
		desired := map[string]manifest.Plugin{plug.ID: plug}
		currentMap := engine.PluginMap(currentPlugins)
		result := engine.ApplyPluginImport(ctx, client, plan, desired, currentMap, tuiHooks{}, "project")
		return result.Err()

	case SectionMarketplace:
		return fmt.Errorf("marketplaces are not project-scoped")
	}

	return fmt.Errorf("unknown section: %d", item.Section)
}

// execDeleteFromProject removes an item from the project config.
func execDeleteFromProject(opts Options, item CopyItem, fullState *FullState) error {
	switch item.Section {
	case SectionMCP:
		// Direct write for the same reasons as execImportToProject — see comment there.
		mcpPath, err := projectMcpPath(fullState)
		if err != nil {
			return err
		}
		return mcp.UpdateProjectMcpServers(mcpPath, func(servers map[string]manifest.MCPServer) error {
			delete(servers, item.Name)
			return nil
		})

	case SectionPlugin:
		// item.Name is the plugin ID (PlugSync is keyed by manifest.Plugin.ID).
		// Direct CLI call — uninstall is unconditional (no conflict reconciliation needed).
		ctx := context.Background()
		client := opts.NewClient()
		if err := client.UninstallPlugin(ctx, item.Name, "project"); err != nil {
			return fmt.Errorf("uninstalling plugin %q: %w", item.Name, err)
		}
		return nil

	case SectionMarketplace:
		return fmt.Errorf("marketplaces are not project-scoped")
	}

	return fmt.Errorf("unknown section: %d", item.Section)
}

// execRemoveFromClaude removes an item from Claude Code's live config.
func execRemoveFromClaude(opts Options, item CopyItem) error {
	switch item.Section {
	case SectionMCP:
		// Use ApplyMCPImport (direct file write) rather than client.RemoveMcpServer
		// for the same reasons as execImport: file-locked atomic update preserves all
		// per-machine keys in ~/.claude.json and avoids CLI idempotency issues.
		claudePath := opts.ClaudeMCPPath
		if claudePath == "" {
			var err error
			claudePath, err = mcp.DefaultPath()
			if err != nil {
				return err
			}
		}
		return engine.ApplyMCPImport(claudePath, nil, nil, nil, []string{item.Name})

	case SectionPlugin:
		ctx := context.Background()
		client := opts.NewClient()
		if err := client.UninstallPlugin(ctx, item.Name, "" /* user scope */); err != nil {
			return fmt.Errorf("uninstalling plugin %q: %w", item.Name, err)
		}
		return nil

	case SectionMarketplace:
		ctx := context.Background()
		client := opts.NewClient()
		if err := client.RemoveMarketplace(ctx, item.Name); err != nil {
			return fmt.Errorf("removing marketplace %q: %w", item.Name, err)
		}
		return nil
	}

	return fmt.Errorf("unknown section: %d", item.Section)
}

// executeUpdateMarketplace returns a tea.Cmd that refreshes a marketplace's index.
func executeUpdateMarketplace(opts Options, name string) tea.Cmd {
	return func() tea.Msg {
		client := opts.NewClient()
		err := client.UpdateMarketplace(context.Background(), name)
		if err != nil {
			err = fmt.Errorf("updating marketplace %q: %w", name, err)
		}
		return MktUpdateResultMsg{Name: name, Err: err}
	}
}

// waitForProgress reads the next message from the progress channel.
func waitForProgress(ch <-chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-ch
		if !ok {
			return nil // channel closed
		}
		return msg
	}
}

// tuiProgressHooks implements engine.ProgressHooks, sending updates through a channel.
type tuiProgressHooks struct {
	tuiHooks
	ch      chan<- tea.Msg
	current int
	total   int
}

func (h *tuiProgressHooks) OnOpStart(section engine.SectionKind, name string, action engine.ActionKind) {
	h.current++
	h.ch <- ProgressUpdateMsg{Section: section, Name: name, Action: action, Done: false, Current: h.current, Total: h.total}
}

func (h *tuiProgressHooks) OnOpDone(section engine.SectionKind, name string, action engine.ActionKind, err error) {
	h.ch <- ProgressUpdateMsg{Section: section, Name: name, Action: action, Done: true, Err: err, Current: h.current, Total: h.total}
}

// bulkApplyPlan holds the pre-computed plan for a bulk apply operation.
// Computing the plan once and passing it to both countBulkOps and the executor
// prevents total drift between the progress counter and actual execution.
type bulkApplyPlan struct {
	DesiredMCP     map[string]manifest.MCPServer
	DesiredPlugins []manifest.Plugin
	PlanMCP        *engine.ImportPlan
	PlanPlug       *engine.ImportPlan
}

// countBulkOps returns the total number of individual operations in a bulk apply.
// full may be nil, in which case marketplace auto-imports are not counted.
func countBulkOps(target PaneSource, planMCP, planPlug *engine.ImportPlan, full *FullState) int {
	if target.Kind == SourceManifest {
		return 1 // single atomic write
	}
	n := 0
	// Count marketplace auto-imports.
	if full != nil && full.Manifest != nil {
		seen := make(map[string]bool)
		for _, pid := range planPlug.Add {
			mpName := engine.MarketplaceFromPluginID(pid)
			if mpName == "" {
				continue
			}
			if _, installed := full.MktInstalled[mpName]; installed {
				continue
			}
			if _, inManifest := full.Manifest.Marketplaces[mpName]; !inManifest {
				continue
			}
			if !seen[mpName] {
				seen[mpName] = true
				n++
			}
		}
	}
	if len(planMCP.Add)+len(planMCP.Conflict)+len(planMCP.Remove) > 0 {
		n++ // single atomic MCP write
	}
	n += len(planPlug.Remove) + len(planPlug.Add) + len(planPlug.Conflict)
	return n
}

// recoverProgressPanic recovers from a panic and sends a ProgressFinishedMsg
// with the panic value as an error. This prevents the progress overlay from
// getting stuck in "running" state when the operation panics.
func recoverProgressPanic(ch chan<- tea.Msg) {
	if r := recover(); r != nil {
		ch <- ProgressFinishedMsg{Err: fmt.Errorf("panic: %v", r)}
	}
}

// executeBulkApplyWithProgress returns a tea.Cmd that runs the bulk apply with progress reporting.
func executeBulkApplyWithProgress(
	ctx context.Context,
	opts Options,
	target PaneSource,
	plan bulkApplyPlan,
	fullState *FullState,
	ch chan<- tea.Msg,
	total int,
) tea.Cmd {
	return func() tea.Msg {
		defer close(ch)
		defer recoverProgressPanic(ch)
		warnings, manifestDirty, err := doBulkApplyWithProgress(ctx, opts, target, plan, fullState, ch, total)
		ch <- ProgressFinishedMsg{Warnings: warnings, Err: err, ManifestDirty: manifestDirty}
		return nil // results go through the channel
	}
}

// autoImportMarketplaces installs marketplace dependencies required by the
// plugins being added, if the marketplaces are defined in the manifest but
// not yet installed in Claude Code. Each auto-import is reported via hooks.
func autoImportMarketplaces(
	ctx context.Context,
	client engine.ImportClient,
	pluginsToAdd []string,
	full *FullState,
	hooks *tuiProgressHooks,
) error {
	if full == nil {
		return engine.CheckPluginMarketplacePrereqs(pluginsToAdd, nil)
	}
	if full.Manifest == nil {
		return engine.CheckPluginMarketplacePrereqs(pluginsToAdd, full.MktInstalled)
	}

	// Snapshot installed marketplaces to avoid mutating the shared FullState
	// map from this background goroutine while the TUI may read it.
	installed := make(map[string]manifest.Marketplace, len(full.MktInstalled))
	for k, v := range full.MktInstalled {
		installed[k] = v
	}

	// Collect unique missing marketplaces that are available in the manifest.
	seen := make(map[string]bool)
	var missing []string
	for _, pid := range pluginsToAdd {
		mpName := engine.MarketplaceFromPluginID(pid)
		if mpName == "" {
			continue
		}
		if _, ok := installed[mpName]; ok {
			continue
		}
		if _, inManifest := full.Manifest.Marketplaces[mpName]; !inManifest {
			return fmt.Errorf("plugin %q requires marketplace %q, which is not available in Claude Code or the manifest", pid, mpName)
		}
		if !seen[mpName] {
			seen[mpName] = true
			missing = append(missing, mpName)
		}
	}

	for _, mpName := range missing {
		mkt := full.Manifest.Marketplaces[mpName]
		source, err := mkt.SourceLocatorE()
		if err != nil {
			return fmt.Errorf("marketplace %q: %w", mpName, err)
		}
		hooks.OnOpStart(engine.SectionMarketplace, mpName, engine.ActionAdded)
		err = client.AddMarketplace(ctx, source)
		hooks.OnOpDone(engine.SectionMarketplace, mpName, engine.ActionAdded, err)
		if err != nil {
			return fmt.Errorf("auto-importing marketplace %q: %w", mpName, err)
		}
		// Track as installed in local copy so subsequent checks see it.
		installed[mpName] = mkt
	}
	return nil
}

func doBulkApplyWithProgress(
	ctx context.Context,
	opts Options,
	target PaneSource,
	plan bulkApplyPlan,
	full *FullState,
	ch chan<- tea.Msg,
	total int,
) (warnings []string, manifestDirty bool, err error) {
	if full == nil {
		return nil, false, fmt.Errorf("no state loaded")
	}

	desiredMCP := plan.DesiredMCP
	desiredPlugins := plan.DesiredPlugins
	planMCP := plan.PlanMCP
	planPlug := plan.PlanPlug

	hooks := &tuiProgressHooks{
		tuiHooks: tuiHooks{warnings: &warnings},
		ch:       ch,
		total:    total,
	}

	switch target.Kind {
	case SourceClaudeCode:
		client := opts.NewClient()
		if err := autoImportMarketplaces(ctx, client, planPlug.Add, full, hooks); err != nil {
			return nil, false, err
		}
		claudePath, err := mcp.DefaultPath()
		if err != nil {
			return nil, false, err
		}
		engine.WarnSecretEnvVars(desiredMCP, append(planMCP.Add, planMCP.Conflict...), hooks)
		// MCP write is a single atomic op — report as one operation.
		if len(planMCP.Add)+len(planMCP.Conflict)+len(planMCP.Remove) > 0 {
			hooks.OnOpStart(SectionMCP, "MCP servers", engine.ActionUpdated)
			mcpErr := engine.ApplyMCPImport(claudePath, desiredMCP, planMCP.Add, planMCP.Conflict, planMCP.Remove)
			hooks.OnOpDone(SectionMCP, "MCP servers", engine.ActionUpdated, mcpErr)
			if mcpErr != nil {
				return nil, false, fmt.Errorf("applying MCP changes: %w", mcpErr)
			}
		}
		currentPluginMap := engine.PluginMap(full.PlugInstalled)
		plugResult := engine.ApplyPluginImport(ctx, client, planPlug, engine.PluginMap(desiredPlugins), currentPluginMap, hooks, "")
		return warnings, false, plugResult.Err()

	case SourceProject:
		client := opts.NewClient()
		if err := autoImportMarketplaces(ctx, client, planPlug.Add, full, hooks); err != nil {
			return nil, false, err
		}
		mcpPath, err := projectMcpPath(full)
		if err != nil {
			return nil, false, err
		}
		engine.WarnProjectEnvVars(desiredMCP, append(planMCP.Add, planMCP.Conflict...), hooks)
		engine.WarnSecretEnvVars(desiredMCP, append(planMCP.Add, planMCP.Conflict...), hooks)
		if len(planMCP.Add)+len(planMCP.Conflict)+len(planMCP.Remove) > 0 {
			hooks.OnOpStart(SectionMCP, "MCP servers", engine.ActionUpdated)
			mcpErr := engine.ApplyMCPImportToProject(mcpPath, desiredMCP, planMCP.Add, planMCP.Conflict, planMCP.Remove)
			hooks.OnOpDone(SectionMCP, "MCP servers", engine.ActionUpdated, mcpErr)
			if mcpErr != nil {
				return nil, false, fmt.Errorf("applying MCP changes to project: %w", mcpErr)
			}
		}
		currentPluginMap := engine.PluginMap(full.ProjectPlugins)
		plugResult := engine.ApplyPluginImport(ctx, client, planPlug, engine.PluginMap(desiredPlugins), currentPluginMap, hooks, pluginScopeProject)
		return warnings, false, plugResult.Err()

	case SourceManifest:
		// Export to manifest — single atomic op, report as one operation.
		manPath, merr := resolveManPath(opts)
		if merr != nil {
			return nil, false, merr
		}
		hooks.OnOpStart(SectionMCP, "manifest", engine.ActionUpdated)
		exportErr := engine.ApplyBulkExportToManifest(manPath, engine.BulkExportInput{
			MCPDesired:    desiredMCP,
			MCPAdd:        planMCP.Add,
			MCPOverwrite:  planMCP.Conflict,
			PlugDesired:   desiredPlugins,
			PlugAdd:       planPlug.Add,
			PlugOverwrite: planPlug.Conflict,
		}, full.MktInstalled, hooks)
		hooks.OnOpDone(SectionMCP, "manifest", engine.ActionUpdated, exportErr)
		if exportErr != nil {
			return nil, false, exportErr
		}
		return warnings, true, nil

	default:
		return nil, false, fmt.Errorf("unsupported apply target: %s", target.Label())
	}
}

// executeProfileDelete returns a tea.Cmd that deletes a profile from the manifest.
func executeProfileDelete(opts Options, name string) tea.Cmd {
	return func() tea.Msg {
		err := doProfileDelete(opts, name)
		return ProfileDeleteResultMsg{Name: name, Err: err}
	}
}

func doProfileDelete(opts Options, name string) error {
	manPath, err := resolveManPath(opts)
	if err != nil {
		return err
	}

	return manifest.Update(manPath, func(m *manifest.Manifest) error {
		if _, ok := m.Profiles[name]; !ok {
			return fmt.Errorf("profile %q not found", name)
		}
		delete(m.Profiles, name)
		if len(m.Profiles) == 0 {
			m.Profiles = nil
		}
		return nil
	})
}

// executeProfileCreate returns a tea.Cmd that creates a new profile from the
// current Claude Code state.
func executeProfileCreate(opts Options, name string, full *FullState) tea.Cmd {
	return func() tea.Msg {
		err := doProfileCreate(opts, name, full)
		return ProfileCreateResultMsg{Name: name, Err: err}
	}
}

// executeToggle returns a tea.Cmd that toggles a plugin's enabled state.
func executeToggle(opts Options, name string, currentEnabled bool, source PaneSource) tea.Cmd {
	newEnabled := !currentEnabled
	return func() tea.Msg {
		var err error
		var manifestDirty bool
		switch source.Kind {
		case SourceManifest:
			err = execToggleManifestPlugin(opts, name, newEnabled)
			manifestDirty = err == nil
		case SourceProfile:
			err = execToggleProfilePlugin(opts, name, newEnabled, source.ProfileName)
			manifestDirty = err == nil
		case SourceClaudeCode:
			err = execToggleClaudePlugin(opts, name, newEnabled, "")
		case SourceProject:
			err = execToggleClaudePlugin(opts, name, newEnabled, pluginScopeProject)
		default:
			err = fmt.Errorf("unsupported source for toggle: %s", source.Label())
		}
		return ToggleResultMsg{Name: name, Err: err, ManifestDirty: manifestDirty}
	}
}

// execToggleManifestPlugin flips a plugin's Enabled field in the manifest.
func execToggleManifestPlugin(opts Options, pluginID string, newEnabled bool) error {
	manPath, err := resolveManPath(opts)
	if err != nil {
		return err
	}
	return manifest.Update(manPath, func(m *manifest.Manifest) error {
		idx := manifest.FindPlugin(m.Plugins, pluginID)
		if idx < 0 {
			return fmt.Errorf("plugin %q not found in manifest", pluginID)
		}
		m.Plugins[idx].Enabled = newEnabled
		return nil
	})
}

// execToggleClaudePlugin calls `claude plugin enable/disable` via the CLI client.
func execToggleClaudePlugin(opts Options, pluginID string, newEnabled bool, scope string) error {
	client := opts.NewClient()
	if err := client.SetPluginEnabled(context.Background(), pluginID, newEnabled, scope); err != nil {
		return fmt.Errorf("setting plugin %q enabled=%v: %w", pluginID, newEnabled, err)
	}
	return nil
}

// execToggleProfilePlugin sets or clears the Enabled override on a profile plugin entry.
func execToggleProfilePlugin(opts Options, pluginID string, newEnabled bool, profileName string) error {
	manPath, err := resolveManPath(opts)
	if err != nil {
		return err
	}
	return manifest.Update(manPath, func(m *manifest.Manifest) error {
		profile, ok := m.Profiles[profileName]
		if !ok {
			return fmt.Errorf("profile %q not found", profileName)
		}
		idx := manifest.FindProfilePlugin(profile.Plugins, pluginID)
		if idx < 0 {
			return fmt.Errorf("plugin %q not found in profile %q", pluginID, profileName)
		}
		// If the new enabled state matches the manifest-level default, clear the
		// override (nil = inherit from manifest) to keep profile overrides
		// minimal — only divergences from the manifest default are stored.
		// When the plugin isn't in the manifest (manIdx < 0), always store
		// the override since there's no default to inherit.
		manIdx := manifest.FindPlugin(m.Plugins, pluginID)
		if manIdx >= 0 && m.Plugins[manIdx].Enabled == newEnabled {
			profile.Plugins[idx].Enabled = nil
		} else {
			enabled := newEnabled
			profile.Plugins[idx].Enabled = &enabled
		}
		m.Profiles[profileName] = profile
		return nil
	})
}

// doProfileCreate delegates to engine.SnapshotProfile, which handles
// marketplace dependency resolution, auto-export of approved marketplaces,
// and profile creation. This ensures the profile's references are valid.
func doProfileCreate(opts Options, name string, full *FullState) error {
	if full == nil {
		return fmt.Errorf("no state loaded")
	}

	manPath, err := resolveManPath(opts)
	if err != nil {
		return err
	}

	hooks := tuiHooks{}
	_, err = engine.SnapshotProfile(manPath, name, false, full.MCPInstalled, full.PlugInstalled, full.MktInstalled, hooks)
	return err
}
