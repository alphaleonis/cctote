package tui

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/atotto/clipboard"

	"github.com/alphaleonis/cctote/internal/cliutil"
	"github.com/alphaleonis/cctote/internal/config"
	"github.com/alphaleonis/cctote/internal/engine"
	"github.com/alphaleonis/cctote/internal/manifest"
)

func (m tuiModel) handleStateLoaded(msg StateLoadedMsg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	m.status.SetLoading(false, "")
	if msg.Err != nil {
		m.err = msg.Err
		return m, tea.Quit
	}
	m.fullState = msg.State
	// Validate initial profile from opts.
	if m.leftSource.Kind == SourceProfile {
		if m.fullState.Manifest != nil {
			if _, ok := m.fullState.Manifest.Profiles[m.leftSource.ProfileName]; !ok {
				m.leftSource = PaneSource{Kind: SourceManifest}
				flashCmd := m.status.SetFlash(fmt.Sprintf("Profile %q not found", m.opts.Profile), FlashError)
				cmds = append(cmds, flashCmd)
			}
		}
	}
	m.recomputeComparison()
	m.ready = true
	m.populatePanels()
	m.updateDetail()
	// Show first warning as flash (multiple warnings merged).
	if len(msg.Warnings) > 0 {
		flashCmd := m.status.SetFlash(strings.Join(msg.Warnings, "; "), FlashError)
		cmds = append(cmds, flashCmd)
	}
	return m, tea.Batch(cmds...)
}

func (m tuiModel) handleStateRefreshed(msg StateRefreshedMsg) (tea.Model, tea.Cmd) {
	m.status.SetLoading(false, "")
	m.busy = false
	if msg.Err != nil {
		m.err = msg.Err
		return m, nil
	}
	m.fullState = msg.State
	// Validate profile sources still exist.
	profileDropped := false
	if m.leftSource.Kind == SourceProfile && m.fullState.Manifest != nil {
		if _, ok := m.fullState.Manifest.Profiles[m.leftSource.ProfileName]; !ok {
			m.leftSource = PaneSource{Kind: SourceManifest}
			profileDropped = true
		}
	}
	if m.rightSource.Kind == SourceProfile && m.fullState.Manifest != nil {
		if _, ok := m.fullState.Manifest.Profiles[m.rightSource.ProfileName]; !ok {
			m.rightSource = PaneSource{Kind: SourceClaudeCode}
			profileDropped = true
		}
	}
	m.recomputeComparison()
	m.populatePanels()
	m.updateDetail()
	if profileDropped {
		flashMsg := "Profile no longer exists"
		if len(msg.Warnings) > 0 {
			flashMsg += "; " + strings.Join(msg.Warnings, "; ")
		}
		return m, m.status.SetFlash(flashMsg, FlashError)
	}
	if len(msg.Warnings) > 0 {
		return m, m.status.SetFlash(strings.Join(msg.Warnings, "; "), FlashError)
	}
	return m, nil
}

func (m tuiModel) handleConfirmAccepted(msg ConfirmAcceptedMsg) (tea.Model, tea.Cmd) {
	m.confirm.Deactivate()
	// Profile-level ops are intercepted here before the item-based executeCopy path.
	if msg.Op == OpDeleteProfile {
		m.busy = true
		m.status.SetLoading(true, "Deleting profile…")
		return m, tea.Batch(
			executeProfileDelete(m.opts, msg.From.ProfileName),
			func() tea.Msg { return m.status.Tick() },
		)
	}
	// Use the progress overlay for multi-item CLI operations.
	if len(msg.Items) > 1 && msg.Op.InvokesCLI() {
		return m.launchCopyWithProgress(msg.Items, msg.Op, msg.From, msg.To)
	}
	m.busy = true
	m.status.SetLoading(true, "Applying…")
	return m, tea.Batch(
		executeCopy(m.opts, msg.Items, msg.Op, msg.From, msg.To, m.fullState),
		func() tea.Msg { return m.status.Tick() },
	)
}

func (m tuiModel) handleSourceSelected(msg SourceSelectedMsg) (tea.Model, tea.Cmd) {
	if msg.Side == SideLeft {
		m.leftSource = msg.Source
	} else {
		m.rightSource = msg.Source
	}
	m.recomputeComparison()
	m.populatePanels()
	m.updateDetail()
	return m, nil
}

func (m tuiModel) handleBulkApplyMsg(msg BulkApplyMsg) (tea.Model, tea.Cmd) {
	m.bulkApply.Deactivate()
	m.busy = true

	// Compute the plan once so countBulkOps and the executor use identical data.
	sourceData := ExtractSourceData(msg.Source, m.fullState)
	targetData := ExtractSourceData(msg.Target, m.fullState)
	plan := bulkApplyPlan{
		DesiredMCP:     sourceData.MCP,
		DesiredPlugins: sourceData.Plugins,
		PlanMCP:        engine.ClassifyMCPImport(sourceData.MCP, targetData.MCP, msg.Strict),
		PlanPlug:       engine.ClassifyPluginImport(sourceData.Plugins, targetData.Plugins, msg.Strict),
	}
	total := countBulkOps(msg.Target, plan.PlanMCP, plan.PlanPlug, m.fullState)

	ctx, cancel := context.WithCancel(context.Background())
	// Buffer: 2 messages per operation (OnOpStart + OnOpDone) + 2 margin.
	// The extra capacity also ensures recoverProgressPanic can send a
	// ProgressFinishedMsg on panic without deadlocking.
	ch := make(chan tea.Msg, total*2+2)
	m.progressCh = ch

	title := fmt.Sprintf("Applying %s → %s…", msg.Source.Label(), msg.Target.Label())
	m.progress.Activate(title, total, cancel)

	return m, tea.Batch(
		executeBulkApplyWithProgress(ctx, m.opts, msg.Target, plan, m.fullState, ch, total),
		waitForProgress(ch),
		m.progress.tickCmd(),
	)
}

func (m tuiModel) handleBulkApplyResult(msg BulkApplyResultMsg) (tea.Model, tea.Cmd) {
	m.busy = false
	m.status.SetLoading(false, "")
	if msg.ManifestDirty {
		m.manifestDirty = true
	}
	if msg.Err != nil {
		m.showError("Apply failed", cliutil.UserMessage(msg.Err))
		return m, nil
	}
	flashStyle := FlashSuccess
	flashText := "Applied"
	if len(msg.Warnings) > 0 {
		flashStyle = FlashInfo
		flashText = fmt.Sprintf("Applied (%d warning(s): %s)",
			len(msg.Warnings), strings.Join(msg.Warnings, "; "))
	}
	flashCmd := m.status.SetFlash(flashText, flashStyle)
	m.status.SetLoading(true, "Refreshing…")
	return m, tea.Batch(
		flashCmd,
		refreshState(m.opts),
		func() tea.Msg { return m.status.Tick() },
	)
}

func (m tuiModel) handleProgressUpdate(msg ProgressUpdateMsg) (tea.Model, tea.Cmd) {
	m.progress.HandleUpdate(msg)
	return m, waitForProgress(m.progressCh)
}

func (m tuiModel) handleProgressFinished(msg ProgressFinishedMsg) (tea.Model, tea.Cmd) {
	m.busy = false
	if msg.ManifestDirty {
		m.manifestDirty = true
	}
	m.progress.HandleFinished(msg)
	// Don't refresh yet — wait for user to dismiss the overlay.
	return m, nil
}

// launchCopyWithProgress sets up the progress overlay and dispatches
// executeCopyWithProgress for multi-item CLI operations.
func (m tuiModel) launchCopyWithProgress(items []CopyItem, op ResolvedOp, from, to PaneSource) (tea.Model, tea.Cmd) {
	m.busy = true
	total := len(items)
	ctx, cancel := context.WithCancel(context.Background())
	// Buffer: 2 messages per operation (start + done) + 2 margin.
	// The extra capacity also ensures recoverProgressPanic can send a
	// ProgressFinishedMsg on panic without deadlocking.
	ch := make(chan tea.Msg, total*2+2)
	m.progressCh = ch

	title := fmt.Sprintf("%s %d items…", op.ProgressLabel(), total)
	m.progress.Activate(title, total, cancel)

	return m, tea.Batch(
		executeCopyWithProgress(ctx, m.opts, items, op, from, to, m.fullState, ch, total),
		waitForProgress(ch),
		m.progress.tickCmd(),
	)
}

func (m tuiModel) handleProgressDismissed() (tea.Model, tea.Cmd) {
	m.progress.Deactivate()
	m.progressCh = nil
	m.status.SetLoading(true, "Refreshing…")
	return m, tea.Batch(
		refreshState(m.opts),
		func() tea.Msg { return m.status.Tick() },
	)
}

func (m tuiModel) handleProfileCreateMsg(msg ProfileCreateMsg) (tea.Model, tea.Cmd) {
	m.profileCreate.Deactivate()
	m.busy = true
	m.status.SetLoading(true, "Creating profile…")
	return m, tea.Batch(
		executeProfileCreate(m.opts, msg.Name, m.fullState),
		func() tea.Msg { return m.status.Tick() },
	)
}

func (m tuiModel) handleProfileCreateResult(msg ProfileCreateResultMsg) (tea.Model, tea.Cmd) {
	m.busy = false
	m.status.SetLoading(false, "")
	if msg.Err != nil {
		m.showError("Create profile failed", cliutil.UserMessage(msg.Err))
		return m, nil
	}
	m.manifestDirty = true
	// Switch left source to the newly created profile.
	m.leftSource = PaneSource{Kind: SourceProfile, ProfileName: msg.Name}
	flashCmd := m.status.SetFlash(fmt.Sprintf("Profile %q created", msg.Name), FlashSuccess)
	m.status.SetLoading(true, "Refreshing…")
	return m, tea.Batch(
		flashCmd,
		refreshState(m.opts),
		func() tea.Msg { return m.status.Tick() },
	)
}

func (m tuiModel) handleProfileDeleteResult(msg ProfileDeleteResultMsg) (tea.Model, tea.Cmd) {
	m.busy = false
	m.status.SetLoading(false, "")
	if msg.Err != nil {
		m.showError("Delete profile failed", cliutil.UserMessage(msg.Err))
		return m, nil
	}
	m.manifestDirty = true
	// Proactively reset pane sources that showed the deleted profile
	// so StateRefreshedMsg doesn't flash "Profile no longer exists".
	if m.leftSource.Kind == SourceProfile && m.leftSource.ProfileName == msg.Name {
		m.leftSource = PaneSource{Kind: SourceManifest}
	}
	if m.rightSource.Kind == SourceProfile && m.rightSource.ProfileName == msg.Name {
		m.rightSource = PaneSource{Kind: SourceClaudeCode}
	}
	flashCmd := m.status.SetFlash(fmt.Sprintf("Profile %q deleted", msg.Name), FlashSuccess)
	m.status.SetLoading(true, "Refreshing…")
	return m, tea.Batch(
		flashCmd,
		refreshState(m.opts),
		func() tea.Msg { return m.status.Tick() },
	)
}

// showError activates the alert overlay with an error title and message.
func (m *tuiModel) showError(title, msg string) {
	m.alert.Activate(title, msg)
}

// --- Action initiators ---

// handleSourcePicker opens the source picker overlay for the given side.
func (m tuiModel) handleSourcePicker(side Side) (tea.Model, tea.Cmd) {
	if m.fullState == nil {
		return m, nil
	}

	// The other pane's source is excluded so you can't select the same view
	// in both panes.
	other := m.rightSource
	current := m.leftSource
	if side == SideRight {
		other = m.leftSource
		current = m.rightSource
	}

	// Build source list: Manifest, Claude Code, Project, then sorted profiles —
	// excluding whatever the other pane is already showing.
	var sources []PaneSource
	for _, candidate := range []PaneSource{
		{Kind: SourceManifest},
		{Kind: SourceClaudeCode},
		{Kind: SourceProject},
	} {
		if !candidate.Equal(other) {
			sources = append(sources, candidate)
		}
	}
	if m.fullState.Manifest != nil {
		var profileNames []string
		for name := range m.fullState.Manifest.Profiles {
			profileNames = append(profileNames, name)
		}
		sort.Strings(profileNames)
		for _, name := range profileNames {
			candidate := PaneSource{Kind: SourceProfile, ProfileName: name}
			if !candidate.Equal(other) {
				sources = append(sources, candidate)
			}
		}
	}

	// Compute the X position for the dropdown anchor.
	anchorX := 0
	if side == SideRight && m.layout.Mode == LayoutDual {
		anchorX = m.layout.PanelWidth + 1 // after left panel + gap
	}
	m.picker.Activate(side, sources, current, anchorX)
	return m, nil
}

// activeProfileName returns a profile name if either pane is showing a profile.
// Prefers the focused pane. Returns "" if no profile is active.
func (m tuiModel) activeProfileName() string {
	focusedSource := m.leftSource
	otherSource := m.rightSource
	if !m.focusLeft {
		focusedSource = m.rightSource
		otherSource = m.leftSource
	}
	if focusedSource.Kind == SourceProfile {
		return focusedSource.ProfileName
	}
	if otherSource.Kind == SourceProfile {
		return otherSource.ProfileName
	}
	return ""
}

// handleBulkApply opens the bulk apply overlay for the focused→other pane direction.
func (m tuiModel) handleBulkApply() (tea.Model, tea.Cmd) {
	if m.fullState == nil {
		return m, nil
	}

	// Determine source = focused pane, target = other pane.
	source := m.leftSource
	target := m.rightSource
	if !m.focusLeft {
		source = m.rightSource
		target = m.leftSource
	}

	// Reject invalid combos.
	if source.Equal(target) {
		cmd := m.status.SetFlash("Cannot apply to the same source", FlashInfo)
		return m, cmd
	}
	if source.Kind == SourceProfile && target.Kind == SourceManifest {
		cmd := m.status.SetFlash("Profile items are already in the manifest", FlashInfo)
		return m, cmd
	}
	if target.Kind == SourceProfile {
		cmd := m.status.SetFlash("Use per-item copy or Create Profile", FlashInfo)
		return m, cmd
	}

	// For profile sources, validate with strict resolution first so dangling
	// references surface as errors here rather than failing during execution.
	// ExtractSourceData uses ResolveProfileLenient (silently drops missing refs),
	// but doBulkApply uses ResolveProfile (hard error). This guard keeps them aligned.
	if source.Kind == SourceProfile && m.fullState.Manifest != nil {
		if _, _, err := engine.ResolveProfile(m.fullState.Manifest, source.ProfileName); err != nil {
			cmd := m.status.SetFlash(fmt.Sprintf("Profile error: %v", err), FlashError)
			return m, cmd
		}
	}

	// Extract desired (source) and current (target) data.
	sourceData := ExtractSourceData(source, m.fullState)
	targetData := ExtractSourceData(target, m.fullState)

	// Classify MCP and plugins (non-strict first).
	planMCP := engine.ClassifyMCPImport(sourceData.MCP, targetData.MCP, false)
	planPlug := engine.ClassifyPluginImport(sourceData.Plugins, targetData.Plugins, false)

	// Strict mode (remove extras from target) only makes sense when the source
	// represents a curated desired state (Profile, Manifest). ClaudeCode and
	// Project are live/working states that change frequently — strict removal
	// against them would be too destructive and unpredictable.
	showStrict := source.Kind == SourceProfile || source.Kind == SourceManifest

	var removeMCP, removePlugins []string
	if showStrict {
		planMCPStrict := engine.ClassifyMCPImport(sourceData.MCP, targetData.MCP, true)
		planPlugStrict := engine.ClassifyPluginImport(sourceData.Plugins, targetData.Plugins, true)
		removeMCP = planMCPStrict.Remove
		removePlugins = planPlugStrict.Remove
	}

	// Check if applying to a project will create .mcp.json.
	mcpCreation := target.Kind == SourceProject && m.projectMcpMissing() &&
		(len(planMCP.Add) > 0 || len(planMCP.Conflict) > 0)

	m.bulkApply.Activate(BulkApplyPlan{
		Source:        source,
		Target:        target,
		AddMCP:        planMCP.Add,
		AddPlugins:    planPlug.Add,
		ConflictMCP:   planMCP.Conflict,
		ConflictPlug:  planPlug.Conflict,
		RemoveMCP:     removeMCP,
		RemovePlugins: removePlugins,
		ShowStrict:    showStrict,
		McpCreation:   mcpCreation,
	})
	return m, nil
}

// handleProfileCreate opens the profile create overlay.
func (m tuiModel) handleProfileCreate() (tea.Model, tea.Cmd) {
	if m.fullState == nil {
		return m, nil
	}
	mcpCount := len(m.fullState.MCPInstalled)
	plugCount := len(m.fullState.PlugInstalled)
	summary := fmt.Sprintf("Will snapshot: %d MCP, %d plugins", mcpCount, plugCount)

	var existing []string
	if m.fullState.Manifest != nil {
		for name := range m.fullState.Manifest.Profiles {
			existing = append(existing, name)
		}
	}

	m.profileCreate.Activate(summary, existing)
	return m, nil
}

// handleProfileDelete opens the confirm overlay for deleting the active profile.
func (m tuiModel) handleProfileDelete() (tea.Model, tea.Cmd) {
	profileName := m.activeProfileName()
	if profileName == "" {
		cmd := m.status.SetFlash("No profile active", FlashInfo)
		return m, cmd
	}
	if m.fullState == nil || m.fullState.Manifest == nil {
		return m, nil
	}

	profile, ok := m.fullState.Manifest.Profiles[profileName]
	if !ok {
		cmd := m.status.SetFlash("Profile not found", FlashError)
		return m, cmd
	}

	// Build descriptive body lines showing profile contents.
	var bodyLines []string
	if len(profile.MCPServers) > 0 {
		bodyLines = append(bodyLines, fmt.Sprintf("  %d MCP server(s):", len(profile.MCPServers)))
		for _, name := range profile.MCPServers {
			bodyLines = append(bodyLines, fmt.Sprintf("    %s %s", IconMCP, name))
		}
	}
	if len(profile.Plugins) > 0 {
		bodyLines = append(bodyLines, fmt.Sprintf("  %d plugin(s):", len(profile.Plugins)))
		for _, pp := range profile.Plugins {
			bodyLines = append(bodyLines, fmt.Sprintf("    %s %s", IconPlugin, pp.ID))
		}
	}
	if len(bodyLines) == 0 {
		bodyLines = append(bodyLines, "  (empty profile)")
	}

	title := fmt.Sprintf("Delete profile %q?", profileName)
	from := PaneSource{Kind: SourceProfile, ProfileName: profileName}
	m.confirm.Activate(title, nil, OpDeleteProfile, from, from, bodyLines)
	return m, nil
}

// handleEnable enables a disabled plugin in the focused pane's source.
func (m tuiModel) handleEnable() (tea.Model, tea.Cmd) {
	return m.handleSetEnabled(true)
}

// handleDisable disables an enabled plugin in the focused pane's source.
func (m tuiModel) handleDisable() (tea.Model, tea.Cmd) {
	return m.handleSetEnabled(false)
}

// handleSetEnabled sets a plugin's enabled state. If the plugin is already in
// the desired state, it shows a flash instead of dispatching.
func (m tuiModel) handleSetEnabled(wantEnabled bool) (tea.Model, tea.Cmd) {
	panel := m.focusedPanel()
	sel := panel.SelectedItem()
	if sel == nil {
		return m, m.status.SetFlash("No item selected", FlashInfo)
	}
	action := "Enable"
	if !wantEnabled {
		action = "Disable"
	}
	if sel.Section != SectionPlugin {
		return m, m.status.SetFlash(action+" is only available for plugins", FlashInfo)
	}
	if m.compState == nil {
		return m, nil
	}

	sync, ok := m.compState.PlugSync[sel.Name]
	if !ok {
		return m, nil
	}

	// Get the plugin from the focused pane's side.
	var plug manifest.Plugin
	var isPlug bool
	if m.focusLeft && sync.Left != nil {
		plug, isPlug = sync.Left.(manifest.Plugin)
	} else if !m.focusLeft && sync.Right != nil {
		plug, isPlug = sync.Right.(manifest.Plugin)
	}
	if !isPlug {
		return m, m.status.SetFlash("Plugin not available in this pane", FlashInfo)
	}

	// Already in the desired state?
	if plug.Enabled == wantEnabled {
		state := "enabled"
		if !wantEnabled {
			state = "disabled"
		}
		return m, m.status.SetFlash(fmt.Sprintf("%s is already %s", sel.Name, state), FlashInfo)
	}

	// Determine which source to toggle on.
	source := m.leftSource
	if !m.focusLeft {
		source = m.rightSource
	}

	m.busy = true
	label := "Enabling"
	if !wantEnabled {
		label = "Disabling"
	}
	m.status.SetLoading(true, fmt.Sprintf("%s %s…", label, sel.Name))
	return m, tea.Batch(
		executeToggle(m.opts, sel.Name, plug.Enabled, source),
		func() tea.Msg { return m.status.Tick() },
	)
}

// handleToggleResult processes the result of a plugin toggle operation.
func (m tuiModel) handleToggleResult(msg ToggleResultMsg) (tea.Model, tea.Cmd) {
	m.busy = false
	m.status.SetLoading(false, "")
	if msg.ManifestDirty {
		m.manifestDirty = true
	}
	if msg.Err != nil {
		m.showError("Toggle failed", cliutil.UserMessage(msg.Err))
		return m, nil
	}
	flashCmd := m.status.SetFlash(fmt.Sprintf("Toggled %s", msg.Name), FlashSuccess)
	m.status.SetLoading(true, "Refreshing…")
	return m, tea.Batch(
		flashCmd,
		refreshState(m.opts),
		func() tea.Msg { return m.status.Tick() },
	)
}

// handleUpdateMarketplace refreshes the marketplace under the cursor from its source.
func (m tuiModel) handleUpdateMarketplace() (tea.Model, tea.Cmd) {
	panel := m.focusedPanel()
	sel := panel.SelectedItem()
	if sel == nil {
		return m, m.status.SetFlash("No item selected", FlashInfo)
	}
	if sel.Section != SectionMarketplace {
		return m, m.status.SetFlash("Update is only available for marketplaces", FlashInfo)
	}
	focusedSource := m.leftSource
	if !m.focusLeft {
		focusedSource = m.rightSource
	}
	if focusedSource.Kind != SourceClaudeCode {
		return m, m.status.SetFlash("Update applies to Claude Code marketplaces only", FlashInfo)
	}

	m.busy = true
	m.status.SetLoading(true, fmt.Sprintf("Updating %s…", sel.Name))
	return m, tea.Batch(
		executeUpdateMarketplace(m.opts, sel.Name),
		func() tea.Msg { return m.status.Tick() },
	)
}

// handleMktUpdateResult processes the result of a marketplace update operation.
func (m tuiModel) handleMktUpdateResult(msg MktUpdateResultMsg) (tea.Model, tea.Cmd) {
	m.busy = false
	m.status.SetLoading(false, "")
	if msg.Err != nil {
		m.showError("Update failed", cliutil.UserMessage(msg.Err))
		return m, nil
	}
	flashCmd := m.status.SetFlash(fmt.Sprintf("Updated %s", msg.Name), FlashSuccess)
	m.status.SetLoading(true, "Refreshing…")
	return m, tea.Batch(
		flashCmd,
		refreshState(m.opts),
		func() tea.Msg { return m.status.Tick() },
	)
}

// --- Chezmoi quit intercept ---

// handleQuit intercepts quit when the manifest was modified and chezmoi mode
// requires confirmation or auto-run.
func (m tuiModel) handleQuit() (tea.Model, tea.Cmd) {
	if !m.manifestDirty {
		return m, tea.Quit
	}
	switch m.chezmoiMode {
	case config.AutoReAddAsk:
		m.chezmoiPrompting = true
		return m, nil
	case config.AutoReAddAlways:
		return m.startChezmoiReAdd()
	default:
		return m, tea.Quit
	}
}

// startChezmoiReAdd begins the chezmoi re-add operation in the background.
func (m tuiModel) startChezmoiReAdd() (tea.Model, tea.Cmd) {
	m.chezmoiPrompting = false
	m.chezmoiRunning = true
	m.status.SetLoading(true, "Running chezmoi re-add…")
	return m, tea.Batch(
		runChezmoiCmd(m.opts),
		func() tea.Msg { return m.status.Tick() },
	)
}

// runChezmoiCmd returns a tea.Cmd that runs chezmoi re-add.
// Uses a 30-second timeout to prevent the TUI from hanging if the subprocess stalls.
func runChezmoiCmd(opts Options) tea.Cmd {
	return func() tea.Msg {
		if opts.ChezmoiReAdd == nil {
			return ChezmoiDoneMsg{Err: fmt.Errorf("chezmoi re-add function not configured")}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		err := opts.ChezmoiReAdd(ctx, opts.ManifestPath)
		return ChezmoiDoneMsg{Err: err}
	}
}

// handleChezmoiDone processes the result of a chezmoi re-add operation.
func (m tuiModel) handleChezmoiDone(msg ChezmoiDoneMsg) (tea.Model, tea.Cmd) {
	m.chezmoiRunning = false
	m.status.SetLoading(false, "")
	if msg.Err != nil {
		m.chezmoiQuitPending = true
		m.showError("chezmoi re-add failed", msg.Err.Error())
		return m, nil
	}
	return m, tea.Quit
}

// handleChezmoiPromptKey handles keys when the chezmoi prompt dialog is showing.
func (m tuiModel) handleChezmoiPromptKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Confirm):
		if m.chezmoiFocusRun {
			return m.startChezmoiReAdd()
		}
		// Enter on Cancel = quit without chezmoi
		m.chezmoiPrompting = false
		return m, tea.Quit
	case key.Matches(msg, m.keys.Cancel):
		m.chezmoiPrompting = false
		return m, tea.Quit
	case key.Matches(msg, m.keys.Tab),
		key.Matches(msg, m.keys.Left),
		key.Matches(msg, m.keys.Right):
		m.chezmoiFocusRun = !m.chezmoiFocusRun
		return m, nil
	default:
		return m, nil
	}
}

// handleCopyJSON copies the focused MCP server definition as JSON to the clipboard.
func (m tuiModel) handleCopyJSON() (tea.Model, tea.Cmd) {
	panel := m.focusedPanel()
	sel := panel.SelectedItem()
	if sel == nil || m.compState == nil {
		return m, m.status.SetFlash("No item selected", FlashInfo)
	}
	if sel.Section != SectionMCP {
		return m, m.status.SetFlash("Copy JSON is only available for MCP servers", FlashInfo)
	}

	sync, ok := m.compState.MCPSync[sel.Name]
	if !ok {
		return m, nil
	}

	// Use the focused pane's version of the server.
	var srv manifest.MCPServer
	if m.focusLeft && sync.Left != nil {
		srv = sync.Left.(manifest.MCPServer)
	} else if !m.focusLeft && sync.Right != nil {
		srv = sync.Right.(manifest.MCPServer)
	} else if sync.Left != nil {
		srv = sync.Left.(manifest.MCPServer)
	} else if sync.Right != nil {
		srv = sync.Right.(manifest.MCPServer)
	}

	wrapped := map[string]manifest.MCPServer{sel.Name: srv}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(wrapped); err != nil {
		return m, m.status.SetFlash(fmt.Sprintf("JSON encode error: %v", err), FlashError)
	}

	jsonStr := strings.TrimSuffix(buf.String(), "\n")
	return m, tea.Batch(
		copyToClipboard(sel.Name, jsonStr),
		m.status.SetFlash(fmt.Sprintf("Copied %q JSON to clipboard", sel.Name), FlashSuccess),
	)
}

// ClipboardResultMsg is returned after a clipboard write attempt.
type ClipboardResultMsg struct {
	Name string
	Err  error
}

// copyToClipboard returns a tea.Cmd that writes text to the system clipboard.
func copyToClipboard(name, text string) tea.Cmd {
	return func() tea.Msg {
		return ClipboardResultMsg{Name: name, Err: clipboard.WriteAll(text)}
	}
}

// --- Key routing helpers (extracted from handleKey) ---

// routeOverlayKey dispatches the key to the active overlay, if any.
// Returns (true, model, cmd) if an overlay handled the key.
func (m tuiModel) routeOverlayKey(msg tea.KeyPressMsg) (bool, tea.Model, tea.Cmd) {
	if m.alert.Active() {
		var cmd tea.Cmd
		m.alert, cmd = m.alert.Update(msg)
		return true, m, cmd
	}
	if m.picker.Active() {
		var cmd tea.Cmd
		m.picker, cmd = m.picker.Update(msg)
		return true, m, cmd
	}
	if m.progress.Active() {
		var cmd tea.Cmd
		m.progress, cmd = m.progress.Update(msg)
		return true, m, cmd
	}
	if m.bulkApply.Active() {
		var cmd tea.Cmd
		m.bulkApply, cmd = m.bulkApply.Update(msg)
		return true, m, cmd
	}
	if m.profileCreate.Active() {
		var cmd tea.Cmd
		m.profileCreate, cmd = m.profileCreate.Update(msg)
		return true, m, cmd
	}
	if m.configOverlay.Active() {
		var cmd tea.Cmd
		m.configOverlay, cmd = m.configOverlay.Update(msg)
		return true, m, cmd
	}
	if m.confirm.Active() {
		var cmd tea.Cmd
		m.confirm, cmd = m.confirm.Update(msg)
		return true, m, cmd
	}
	if m.chezmoiPrompting {
		model, cmd := m.handleChezmoiPromptKey(msg)
		return true, model, cmd
	}
	if m.chezmoiRunning {
		// Ignore all keys while chezmoi is running (ForceQuit handled earlier).
		return true, m, nil
	}
	return false, m, nil
}

// handleFilterKey handles keys when the filter input is active.
func (m tuiModel) handleFilterKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.clearFilter()
		return m, nil
	case "enter":
		m.filtering = false
		m.filterInput.Blur()
		return m, nil
	default:
		var cmd tea.Cmd
		m.filterInput, cmd = m.filterInput.Update(msg)
		m.applyFilter(m.filterInput.Value())
		return m, cmd
	}
}

// handleExpandedDetailKey handles keys when the detail pane is expanded.
func (m tuiModel) handleExpandedDetailKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Quit):
		m.detail.Collapse()
		m.recomputeLayout()
		return m, nil
	case key.Matches(msg, m.keys.Down):
		m.detail.ScrollDown()
		return m, nil
	case key.Matches(msg, m.keys.Up):
		m.detail.ScrollUp()
		return m, nil
	case key.Matches(msg, m.keys.ToggleSecrets):
		m.detail.ToggleSecrets()
		return m, nil
	case key.Matches(msg, m.keys.CopyJSON):
		return m.handleCopyJSON()
	case key.Matches(msg, m.keys.Enable):
		return m.handleEnable()
	case key.Matches(msg, m.keys.Disable):
		return m.handleDisable()
	case key.Matches(msg, m.keys.Help):
		m.showHelp = !m.showHelp
		m.recomputeLayout()
		return m, nil
	default:
		return m, nil
	}
}

// handleNormalKey handles keys in the default (non-overlay, non-filter, non-expanded) state.
func (m tuiModel) handleNormalKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	// Esc priority chain: clear filter > clear selection > quit.
	if key.Matches(msg, m.keys.Quit) {
		if m.filterText != "" {
			m.clearFilter()
			return m, nil
		}
		panel := m.focusedPanel()
		if panel.HasSelection() {
			panel.ClearSelection()
			return m, nil
		}
		return m.handleQuit()
	}

	switch {
	case key.Matches(msg, m.keys.SourceLeft):
		if m.ready && !m.busy {
			return m.handleSourcePicker(SideLeft)
		}
		return m, nil
	case key.Matches(msg, m.keys.SourceRight):
		if m.ready && !m.busy {
			return m.handleSourcePicker(SideRight)
		}
		return m, nil
	case key.Matches(msg, m.keys.BulkApply):
		if m.ready && !m.busy {
			return m.handleBulkApply()
		}
		return m, nil
	case key.Matches(msg, m.keys.CreateProfile):
		if m.ready && !m.busy {
			return m.handleProfileCreate()
		}
		return m, nil
	case key.Matches(msg, m.keys.DeleteProfile):
		if m.ready && !m.busy {
			return m.handleProfileDelete()
		}
		return m, nil

	case key.Matches(msg, m.keys.Config):
		if !m.busy {
			m.configOverlay.Activate(m.opts.ConfigPath)
		}
		return m, nil

	case key.Matches(msg, m.keys.ToggleSecrets):
		m.detail.ToggleSecrets()
		return m, nil

	case key.Matches(msg, m.keys.CopyJSON):
		if m.ready {
			return m.handleCopyJSON()
		}
		return m, nil

	case key.Matches(msg, m.keys.ExpandDetail):
		if m.ready && m.detail.item != nil {
			m.detail.Expand()
			m.applyLayout()
		}
		return m, nil

	case key.Matches(msg, m.keys.Filter):
		if m.ready {
			m.filtering = true
			m.filterInput.Reset()
			return m, m.filterInput.Focus()
		}
		return m, nil

	case key.Matches(msg, m.keys.Help):
		m.showHelp = !m.showHelp
		m.recomputeLayout()
		return m, nil

	case key.Matches(msg, m.keys.Tab):
		m.toggleFocus()
		m.updateDetail()
		return m, nil

	case key.Matches(msg, m.keys.Left):
		if !m.focusLeft {
			m.toggleFocus()
			m.updateDetail()
		}
		return m, nil
	case key.Matches(msg, m.keys.Right):
		if m.focusLeft {
			m.toggleFocus()
			m.updateDetail()
		}
		return m, nil

	// Directional copy actions.
	case key.Matches(msg, m.keys.CopyRight):
		if m.ready && !m.busy {
			return m.handleDirectionalCopy(m.leftSource, m.rightSource)
		}
		return m, nil
	case key.Matches(msg, m.keys.CopyLeft):
		if m.ready && !m.busy {
			return m.handleDirectionalCopy(m.rightSource, m.leftSource)
		}
		return m, nil
	case key.Matches(msg, m.keys.Delete):
		if m.ready && !m.busy {
			return m.handleDelete()
		}
		return m, nil
	case key.Matches(msg, m.keys.Enable):
		if m.ready && !m.busy {
			return m.handleEnable()
		}
		return m, nil
	case key.Matches(msg, m.keys.Disable):
		if m.ready && !m.busy {
			return m.handleDisable()
		}
		return m, nil
	case key.Matches(msg, m.keys.UpdateMkt):
		if m.ready && !m.busy {
			return m.handleUpdateMarketplace()
		}
		return m, nil

	case key.Matches(msg, m.keys.Refresh):
		m.status.SetLoading(true, "Refreshing…")
		return m, tea.Batch(
			refreshState(m.opts),
			func() tea.Msg { return m.status.Tick() },
		)

	default:
		// Delegate to focused panel for navigation + select.
		if m.focusLeft {
			var cmd tea.Cmd
			m.left, cmd = m.left.Update(msg)
			m.updateLinkedCursor()
			m.updateDetail()
			return m, cmd
		}
		var cmd tea.Cmd
		m.right, cmd = m.right.Update(msg)
		m.updateLinkedCursor()
		m.updateDetail()
		return m, cmd
	}
}
