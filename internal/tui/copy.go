package tui

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/alphaleonis/cctote/internal/cliutil"
	"github.com/alphaleonis/cctote/internal/engine"
	"github.com/alphaleonis/cctote/internal/manifest"
)

// handleDirectionalCopy collects items from the focused panel and dispatches
// a copy from fromSource to toSource.
func (m tuiModel) handleDirectionalCopy(from, to PaneSource) (tea.Model, tea.Cmd) {
	op := ResolveOp(from, to)
	if op == OpInvalid {
		cmd := m.status.SetFlash("Cannot copy between these sources", FlashInfo)
		return m, cmd
	}

	panel := m.focusedPanel()
	var items []CopyItem
	if panel.HasSelection() {
		for _, it := range panel.SelectedItems() {
			items = append(items, CopyItem{Section: it.Section, Name: it.Name})
		}
	} else {
		sel := panel.SelectedItem()
		if sel == nil {
			return m, nil
		}
		items = []CopyItem{{Section: sel.Section, Name: sel.Name}}
	}

	return m.dispatchCopy(items, op, from, to)
}

// handleDelete dispatches a delete operation on the focused panel's source.
func (m tuiModel) handleDelete() (tea.Model, tea.Cmd) {
	focusedSource := m.leftSource
	if !m.focusLeft {
		focusedSource = m.rightSource
	}

	op := ResolveDeleteOp(focusedSource)
	if op == OpInvalid {
		cmd := m.status.SetFlash("Cannot delete from this source", FlashInfo)
		return m, cmd
	}

	panel := m.focusedPanel()
	var items []CopyItem
	if panel.HasSelection() {
		for _, it := range panel.SelectedItems() {
			items = append(items, CopyItem{Section: it.Section, Name: it.Name})
		}
	} else {
		sel := panel.SelectedItem()
		if sel == nil {
			return m, nil
		}
		items = []CopyItem{{Section: sel.Section, Name: sel.Name}}
	}

	// Delete is a unary operation — from==to because dispatchCopy reuses the
	// copy path. executeCopy reads fromSource.ProfileName for OpRemoveFromProfile.
	return m.dispatchCopy(items, op, focusedSource, focusedSource)
}

// dispatchCopy examines sync status of items and either executes immediately,
// shows confirmation, or shows "already synced" flash.
func (m tuiModel) dispatchCopy(items []CopyItem, op ResolvedOp, from, to PaneSource) (tea.Model, tea.Cmd) {
	if m.compState == nil || len(items) == 0 {
		return m, nil
	}

	// Profiles don't include marketplaces — filter them out with a message.
	if op.TargetsProfile() {
		var filtered []CopyItem
		hadMarketplace := false
		for _, item := range items {
			if item.Section == SectionMarketplace {
				hadMarketplace = true
				continue
			}
			filtered = append(filtered, item)
		}
		if hadMarketplace && len(filtered) == 0 {
			cmd := m.status.SetFlash("Marketplaces are global, not per-profile", FlashInfo)
			return m, cmd
		}
		if hadMarketplace {
			items = filtered
		}
	}

	isDelete := op == OpDeleteFromManifest || op == OpRemoveFromProfile ||
		op == OpDeleteFromProject || op == OpRemoveFromClaude

	// Classify items by sync status.
	var actionable []CopyItem
	var needConfirm []CopyItem
	allSynced := true

	for _, item := range items {
		status := m.lookupStatus(item)
		switch {
		case status == Synced && !isDelete:
			continue
		case status == Different || isDelete:
			needConfirm = append(needConfirm, item)
			allSynced = false
		default:
			actionable = append(actionable, item)
			allSynced = false
		}
	}

	if allSynced && !isDelete {
		cmd := m.status.SetFlash("Already synced", FlashInfo)
		return m, cmd
	}

	// When importing MCP servers to a project that has no .mcp.json yet,
	// always confirm so the user is aware we're creating the file.
	// Only triggers when the batch contains MCP items (plugins go through
	// the Claude CLI and don't touch .mcp.json).
	mcpCreation := op == OpImportToProject && m.projectMcpMissing() && hasMCPItems(append(needConfirm, actionable...))

	// If there are items needing confirmation, show overlay for all of them.
	if len(needConfirm) > 0 {
		combined := append(needConfirm, actionable...)
		title := m.confirmTitle(op, combined)
		diffLines := m.buildDiffLines(needConfirm)
		if mcpCreation {
			diffLines = append([]string{"This will create .mcp.json in the project root.", ""}, diffLines...)
		}
		// Cascade details only apply to manifest deletes — profile removes only
		// drop the reference, and project deletes don't involve profiles.
		// Uses combined (needConfirm + actionable) because both sets are being
		// deleted and may have profile references that should be previewed.
		if op == OpDeleteFromManifest {
			if cascadeLines := m.buildCascadeLines(combined); cascadeLines != nil {
				if len(diffLines) > 0 {
					diffLines = append(diffLines, "")
				}
				diffLines = append(diffLines, cascadeLines...)
			}
		}
		m.confirm.Activate(title, combined, op, from, to, diffLines)
		return m, nil
	}

	if mcpCreation {
		title := "Create .mcp.json?"
		diffLines := []string{"This will create .mcp.json in the project root."}
		m.confirm.Activate(title, actionable, op, from, to, diffLines)
		return m, nil
	}

	// All items are one-side-only — execute immediately.
	panel := m.focusedPanel()
	panel.ClearSelection()
	if len(actionable) > 1 && op.InvokesCLI() {
		return m.launchCopyWithProgress(actionable, op, from, to)
	}
	m.busy = true
	m.status.SetLoading(true, "Applying…")
	return m, tea.Batch(
		executeCopy(m.opts, actionable, op, from, to, m.fullState),
		func() tea.Msg { return m.status.Tick() },
	)
}

func (m tuiModel) lookupStatus(item CopyItem) SyncStatus {
	if m.compState == nil {
		return Synced
	}
	switch item.Section {
	case SectionMCP:
		if s, ok := m.compState.MCPSync[item.Name]; ok {
			return s.Status
		}
	case SectionPlugin:
		if s, ok := m.compState.PlugSync[item.Name]; ok {
			return s.Status
		}
	case SectionMarketplace:
		if s, ok := m.compState.MktSync[item.Name]; ok {
			return s.Status
		}
	}
	return Synced // fallback
}

// projectMcpMissing returns true when the project root is set but .mcp.json
// does not yet exist on disk.
func (m tuiModel) projectMcpMissing() bool {
	if m.fullState == nil || m.fullState.ProjectRoot == "" {
		return false
	}
	_, err := os.Stat(filepath.Join(m.fullState.ProjectRoot, ".mcp.json"))
	return errors.Is(err, fs.ErrNotExist)
}

// hasMCPItems returns true if any item in the batch is an MCP server.
func hasMCPItems(items []CopyItem) bool {
	for _, item := range items {
		if item.Section == SectionMCP {
			return true
		}
	}
	return false
}

func (m tuiModel) confirmTitle(op ResolvedOp, items []CopyItem) string {
	action := op.Label()
	if len(items) == 1 {
		return fmt.Sprintf("%s %s?", action, items[0].Name)
	}
	return fmt.Sprintf("%s %d items?", action, len(items))
}

func (m tuiModel) buildDiffLines(items []CopyItem) []string {
	if len(items) != 1 || m.compState == nil {
		return nil
	}
	item := items[0]
	switch item.Section {
	case SectionMCP:
		if s, ok := m.compState.MCPSync[item.Name]; ok && s.Left != nil && s.Right != nil {
			return diffMCPLines(s.Left.(manifest.MCPServer), s.Right.(manifest.MCPServer))
		}
	case SectionPlugin:
		if s, ok := m.compState.PlugSync[item.Name]; ok && s.Left != nil && s.Right != nil {
			return diffPluginLines(s.Left.(manifest.Plugin), s.Right.(manifest.Plugin))
		}
	case SectionMarketplace:
		if s, ok := m.compState.MktSync[item.Name]; ok && s.Left != nil && s.Right != nil {
			return diffMarketplaceLines(s.Left.(manifest.Marketplace), s.Right.(manifest.Marketplace))
		}
	}
	return nil
}

// buildCascadeLines returns styled lines describing cascade effects for
// items about to be deleted from the manifest. Returns nil when no cascades exist.
//
// Note: reads m.fullState.Manifest, a snapshot from the last state refresh.
// The engine's manifest.Update always performs a fresh locked read before
// executing, so the actual deletion is correct regardless of preview accuracy.
func (m tuiModel) buildCascadeLines(items []CopyItem) []string {
	if m.fullState == nil {
		return nil
	}
	man := m.fullState.Manifest
	if man == nil {
		return nil
	}

	var pluginRemoveLines []string
	var profileRefLines []string

	for _, item := range items {
		switch item.Section {
		case SectionMCP:
			for _, pName := range engine.FindMCPProfileRefs(man, item.Name) {
				profileRefLines = append(profileRefLines, StyleDiffRemove.Render(fmt.Sprintf("  %s %s → %s %s", IconProfile, pName, IconMCP, item.Name)))
			}

		case SectionPlugin:
			for _, pName := range engine.FindPluginProfileRefs(man, item.Name) {
				profileRefLines = append(profileRefLines, StyleDiffRemove.Render(fmt.Sprintf("  %s %s → %s %s", IconProfile, pName, IconPlugin, item.Name)))
			}

		case SectionMarketplace:
			affectedPlugins := engine.FindMarketplacePlugins(man, item.Name)
			for _, pid := range affectedPlugins {
				pluginRemoveLines = append(pluginRemoveLines, StyleDiffRemove.Render(fmt.Sprintf("  %s %s", IconPlugin, pid)))
			}
			for _, pid := range affectedPlugins {
				for _, pName := range engine.FindPluginProfileRefs(man, pid) {
					profileRefLines = append(profileRefLines, StyleDiffRemove.Render(fmt.Sprintf("  %s %s → %s %s", IconProfile, pName, IconPlugin, pid)))
				}
			}
		}
	}

	var lines []string
	if len(pluginRemoveLines) > 0 {
		lines = append(lines, StyleHint.Render("Will also remove plugins:"))
		lines = append(lines, pluginRemoveLines...)
	}
	if len(profileRefLines) > 0 {
		lines = append(lines, StyleHint.Render("Will clean profile references:"))
		lines = append(lines, profileRefLines...)
	}

	if len(lines) == 0 {
		return nil
	}
	return lines
}

// formatItemErrors builds the error message body for the alert dialog.
// For a single error, shows just the user-facing message (the item name is
// already in the error chain). For multiple errors, prefixes each with the
// item name. Uses cliutil.UserMessage to extract clean stderr from RunErrors.
func formatItemErrors(results []ItemResult) string {
	var errCount int
	for _, r := range results {
		if r.Err != nil {
			errCount++
		}
	}
	var errLines []string
	for _, r := range results {
		if r.Err != nil {
			msg := cliutil.UserMessage(r.Err)
			if errCount > 1 {
				msg = fmt.Sprintf("%s: %s", r.Name, msg)
			}
			errLines = append(errLines, msg)
		}
	}
	return strings.Join(errLines, "\n")
}

func (m tuiModel) handleCopyResult(msg CopyResultMsg) (tea.Model, tea.Cmd) {
	// Summarize results.
	var succeeded, failed int
	for _, r := range msg.Results {
		if r.Err != nil {
			failed++
		} else {
			succeeded++
		}
	}

	if succeeded > 0 && msg.Op.ModifiesManifest() {
		m.manifestDirty = true
	}

	// Clear selection.
	m.focusedPanel().ClearSelection()

	action := msg.Op.PastTense()

	// Show errors in a persistent dialog; success as a transient flash.
	var flashCmd tea.Cmd
	switch {
	case failed > 0 && succeeded > 0:
		title := fmt.Sprintf("%s %d, %d failed", action, succeeded, failed)
		m.showError(title, formatItemErrors(msg.Results))
	case failed > 0:
		title := fmt.Sprintf("%s failed", msg.Op.Label())
		m.showError(title, formatItemErrors(msg.Results))
	default:
		text := fmt.Sprintf("%s %d item(s)", action, succeeded)
		flashCmd = m.status.SetFlash(text, FlashSuccess)
	}

	// Refresh state after mutations.
	m.status.SetLoading(true, "Refreshing…")
	return m, tea.Batch(
		flashCmd,
		refreshState(m.opts),
		func() tea.Msg { return m.status.Tick() },
	)
}
