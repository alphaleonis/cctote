package tui

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	"github.com/alphaleonis/cctote/internal/claude"
	"github.com/alphaleonis/cctote/internal/config"
)

// pluginScopeProject is the scope string emitted by `claude plugin list --json`
// for project-scoped plugins. Must match Claude Code's plugin scope names exactly.
const pluginScopeProject = "project"

// ClientFactory creates a Claude CLI client instance.
type ClientFactory func() *claude.Client

// Options configures the TUI.
type Options struct {
	ManifestPath  string
	ConfigPath    string
	Profile       string
	WorkDir       string        // captured at startup; used for project detection
	NewClient     ClientFactory // factory for Claude CLI client; defaults to claude.NewExecClient
	ClaudeMCPPath string        // override for ~/.claude.json path; empty = mcp.DefaultPath()
	ChezmoiManaged bool                                       // true if manifest is chezmoi-managed (computed once at launch)
	ChezmoiReAdd   func(ctx context.Context, path string) error
}

// tuiModel is the top-level bubbletea model.
type tuiModel struct {
	opts          Options
	keys          KeyMap
	left          PanelModel
	right         PanelModel
	detail        DetailModel
	status        StatusModel
	alert         AlertModel
	confirm       ConfirmModel
	picker        PickerModel
	bulkApply     BulkApplyModel
	progress      ProgressModel
	progressCh    <-chan tea.Msg
	profileCreate ProfileCreateModel
	configOverlay ConfigOverlayModel
	fullState     *FullState
	leftSource    PaneSource
	rightSource   PaneSource
	compState     *SyncState // comparison of leftSource vs rightSource
	layout        Layout
	showHelp      bool
	focusLeft     bool
	busy               bool   // mutation in-flight
	manifestDirty      bool   // true if any op modified the manifest this session
	chezmoiMode        string // effective mode: "never"/"ask"/"always"
	chezmoiPrompting   bool   // "ask" dialog is showing
	chezmoiRunning     bool // re-add in flight
	chezmoiFocusRun    bool // button focus in dialog (true=Run, false=Cancel)
	chezmoiQuitPending bool // quit after alert dismiss (on re-add error)
	filtering          bool // filter input is active
	filterInput   textinput.Model
	filterText    string // current applied filter (lowercased)
	err           error
	ready         bool
}

// Run starts the TUI program. It returns true if the manifest was modified
// during the session (useful for chezmoi integration).
func Run(opts Options) (manifestDirty bool, err error) {
	if opts.WorkDir == "" {
		opts.WorkDir, _ = os.Getwd()
	}
	if opts.NewClient == nil {
		opts.NewClient = claude.NewExecClient
	}
	m := newTUIModel(opts)
	p := tea.NewProgram(m)
	final, err := p.Run()
	if fm, ok := final.(tuiModel); ok {
		return fm.manifestDirty, err
	}
	return false, err
}

func newTUIModel(opts Options) tuiModel {
	keys := DefaultKeyMap()
	s := NewStatus()
	s.SetLoading(true, "Loading…")

	leftSource := PaneSource{Kind: SourceManifest}
	if opts.Profile != "" {
		leftSource = PaneSource{Kind: SourceProfile, ProfileName: opts.Profile}
	}
	rightSource := PaneSource{Kind: SourceClaudeCode}

	fi := textinput.New()
	fi.Prompt = "/"
	fi.Placeholder = "filter…"
	fi.CharLimit = 60
	fi.SetStyles(filterInputStyles())

	return tuiModel{
		opts:          opts,
		keys:          keys,
		left:          NewPanel(SideLeft, keys),
		right:         NewPanel(SideRight, keys),
		detail:        NewDetail(),
		status:        s,
		alert:         NewAlert(keys),
		confirm:       NewConfirm(keys),
		picker:        NewPicker(keys),
		bulkApply:     NewBulkApply(keys),
		progress:      NewProgress(keys),
		profileCreate: NewProfileCreate(keys),
		configOverlay: NewConfigOverlay(keys),
		filterInput:   fi,
		leftSource:    leftSource,
		rightSource:   rightSource,
		focusLeft:     true,
		chezmoiMode:   resolveChezmoiMode(opts),
	}
}

// windowSizePollMsg triggers a window size check on Windows,
// where SIGWINCH is unavailable and tea.WindowSizeMsg is only sent at startup.
type windowSizePollMsg struct{}

// pollWindowSize starts a periodic tick that requests the current terminal size.
// On non-Windows platforms this is a no-op (SIGWINCH handles resize natively).
func pollWindowSize() tea.Cmd {
	if runtime.GOOS != "windows" {
		return nil
	}
	return tea.Tick(250*time.Millisecond, func(time.Time) tea.Msg {
		return windowSizePollMsg{}
	})
}

// Init returns the initial command to load state.
func (m tuiModel) Init() tea.Cmd {
	return tea.Batch(
		loadState(m.opts),
		func() tea.Msg { return m.status.Tick() },
		pollWindowSize(),
	)
}

// Update handles all messages.
func (m tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case windowSizePollMsg:
		return m, tea.Batch(func() tea.Msg { return tea.RequestWindowSize() }, pollWindowSize())
	case tea.WindowSizeMsg:
		m.layout = ComputeLayout(msg.Width, msg.Height, m.currentHelpHeight())
		m.applyLayout()
		return m, nil
	case StateLoadedMsg:
		return m.handleStateLoaded(msg)
	case StateRefreshedMsg:
		return m.handleStateRefreshed(msg)
	case CopyResultMsg:
		return m.handleCopyResult(msg)
	case FlashExpiredMsg:
		m.status.ClearFlash(msg.ID)
		return m, nil
	case ConfirmAcceptedMsg:
		return m.handleConfirmAccepted(msg)
	case AlertDismissedMsg:
		m.alert.Deactivate()
		if m.chezmoiQuitPending {
			m.chezmoiQuitPending = false
			return m, tea.Quit
		}
		return m, nil
	case ChezmoiDoneMsg:
		return m.handleChezmoiDone(msg)
	case ConfirmCancelledMsg:
		m.confirm.Deactivate()
		return m, nil
	case SourceSelectedMsg:
		return m.handleSourceSelected(msg)
	case BulkApplyMsg:
		return m.handleBulkApplyMsg(msg)
	case BulkApplyResultMsg:
		return m.handleBulkApplyResult(msg)
	case ProgressUpdateMsg:
		return m.handleProgressUpdate(msg)
	case ProgressFinishedMsg:
		return m.handleProgressFinished(msg)
	case ProgressDismissedMsg:
		return m.handleProgressDismissed()
	case ConfigSavedMsg:
		if strings.HasPrefix(msg.Key, "chezmoi.") {
			m.refreshChezmoiMode()
		}
		return m, m.status.SetFlash(fmt.Sprintf("Set %s = %s", msg.Key, msg.Value), FlashSuccess)
	case ProfileCreateMsg:
		return m.handleProfileCreateMsg(msg)
	case ProfileCreateResultMsg:
		return m.handleProfileCreateResult(msg)
	case ProfileDeleteResultMsg:
		return m.handleProfileDeleteResult(msg)
	case ToggleResultMsg:
		return m.handleToggleResult(msg)
	case MktUpdateResultMsg:
		return m.handleMktUpdateResult(msg)
	case ClipboardResultMsg:
		if msg.Err != nil {
			return m, m.status.SetFlash(fmt.Sprintf("Clipboard error: %v", msg.Err), FlashError)
		}
		return m, nil
	case tea.KeyPressMsg:
		return m.handleKey(msg)
	case spinner.TickMsg:
		// Route spinner ticks to the progress overlay when active,
		// otherwise to the status line.
		if m.progress.Active() {
			var cmd tea.Cmd
			m.progress, cmd = m.progress.Update(msg)
			return m, cmd
		}
		var cmd tea.Cmd
		m.status, cmd = m.status.Update(msg)
		return m, cmd
	default:
		var cmd tea.Cmd
		m.status, cmd = m.status.Update(msg)
		return m, cmd
	}
}

func (m tuiModel) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	// Force quit always works.
	if key.Matches(msg, m.keys.ForceQuit) {
		return m, tea.Quit
	}
	if handled, model, cmd := m.routeOverlayKey(msg); handled {
		return model, cmd
	}
	if m.filtering {
		return m.handleFilterKey(msg)
	}
	if m.detail.Expanded() {
		return m.handleExpandedDetailKey(msg)
	}
	return m.handleNormalKey(msg)
}

// focusedPanel returns a pointer to the currently focused panel.
func (m *tuiModel) focusedPanel() *PanelModel {
	if m.focusLeft {
		return &m.left
	}
	return &m.right
}

// View renders the full TUI.
func (m tuiModel) View() tea.View {
	if m.err != nil {
		return tea.NewView(fmt.Sprintf("Error: %v\n", m.err))
	}

	if m.layout.Mode == LayoutTooNarrow {
		return tea.NewView("Terminal too narrow (min 60 columns)")
	}

	var sections []string

	if m.detail.Expanded() {
		// Expanded mode: detail takes over the full view (no panels).
		sections = append(sections, m.detail.View())
	} else {
		switch m.layout.Mode {
		case LayoutDual:
			leftView := m.left.View()
			rightView := m.right.View()
			panels := lipgloss.JoinHorizontal(lipgloss.Top, leftView, " ", rightView)
			sections = append(sections, panels)
		case LayoutSingle:
			if m.focusLeft {
				sections = append(sections, m.left.View())
			} else {
				sections = append(sections, m.right.View())
			}
		}

		if m.layout.DetailHeight > 0 {
			sections = append(sections, m.detail.View())
		}
	}

	// Build the help bar content (if active and no overlay).
	helpBar := ""
	if m.showHelp && !m.anyOverlayActive() {
		groups := m.keys.ContextualHelp(m.helpContext())
		helpBar = HelpBar(groups, m.layout.TotalWidth)
	}

	// Join panels + detail, then pad so help bar + status bar are pinned to the bottom.
	upper := strings.Join(sections, "\n")
	upperHeight := strings.Count(upper, "\n") + 1
	reservedBottom := 1 + m.layout.HelpHeight // status line + help bar
	targetUpperHeight := m.layout.TotalHeight - reservedBottom
	if upperHeight < targetUpperHeight {
		upper += strings.Repeat("\n", targetUpperHeight-upperHeight)
	}

	var bottomLine string
	if m.filtering {
		bottomLine = m.filterInput.View()
	} else if m.filterText != "" {
		// Show active filter with dismiss hint. Use filterText (single source of truth).
		bottomLine = StyleHint.Render("/") + m.filterText +
			"  " + StyleHint.Render("(esc to clear)")
	} else {
		bottomLine = m.status.View()
	}

	content := upper
	if helpBar != "" {
		content += "\n" + helpBar
	}
	content += "\n" + bottomLine

	// Overlays (rendered on top of dimmed background).
	if m.alert.Active() {
		content = m.alert.ViewOver(content)
	} else if m.picker.Active() {
		content = m.picker.ViewOver(content)
	} else if m.progress.Active() {
		content = m.progress.ViewOver(content)
	} else if m.bulkApply.Active() {
		content = m.bulkApply.ViewOver(content)
	} else if m.profileCreate.Active() {
		content = m.profileCreate.ViewOver(content)
	} else if m.configOverlay.Active() {
		content = m.configOverlay.ViewOver(content)
	} else if m.confirm.Active() {
		content = m.confirm.ViewOver(content)
	} else if m.chezmoiPrompting {
		content = m.viewChezmoiPrompt(content)
	}

	v := tea.NewView(content)
	v.AltScreen = true
	return v
}

func (m *tuiModel) applyLayout() {
	m.left.SetSize(m.layout.PanelWidth, m.layout.PanelHeight)
	m.right.SetSize(m.layout.PanelWidth, m.layout.PanelHeight)

	if m.detail.Expanded() {
		// Expanded detail takes the full view minus status line (1) and lipgloss
		// border overhead (2, since Width/Height include top+bottom borders).
		expandedHeight := m.layout.TotalHeight - 1 - 2
		if expandedHeight < 3 {
			expandedHeight = 3
		}
		m.detail.SetSize(m.layout.TotalWidth, expandedHeight)
	} else {
		m.detail.SetSize(m.layout.TotalWidth, m.layout.DetailHeight)
	}

	m.status.SetWidth(m.layout.TotalWidth)
	m.alert.SetSize(m.layout.TotalWidth, m.layout.TotalHeight)
	m.confirm.SetSize(m.layout.TotalWidth, m.layout.TotalHeight)
	m.picker.SetSize(m.layout.TotalWidth, m.layout.TotalHeight)
	m.bulkApply.SetSize(m.layout.TotalWidth, m.layout.TotalHeight)
	m.progress.SetSize(m.layout.TotalWidth, m.layout.TotalHeight)
	m.profileCreate.SetSize(m.layout.TotalWidth, m.layout.TotalHeight)
	m.configOverlay.SetSize(m.layout.TotalWidth, m.layout.TotalHeight)
}

func (m *tuiModel) recomputeComparison() {
	if m.fullState == nil {
		m.compState = nil
		return
	}
	leftData := ExtractSourceData(m.leftSource, m.fullState)
	rightData := ExtractSourceData(m.rightSource, m.fullState)
	m.compState = CompareSources(leftData, rightData)
}

func (m *tuiModel) populatePanels() {
	if m.compState == nil {
		return
	}
	m.left.SetItems(m.compState)
	m.right.SetItems(m.compState)
	m.left.SetFocused(m.focusLeft)
	m.right.SetFocused(!m.focusLeft)

	// Set source on each panel.
	m.left.SetSource(m.leftSource)
	m.right.SetSource(m.rightSource)

	// Panel titles with F-key hint.
	m.left.SetTitle(fmt.Sprintf(" [F1] %s %s ▾ ", m.leftSource.Icon(), m.leftSource.Label()))
	m.right.SetTitle(fmt.Sprintf(" [F2] %s %s ▾ ", m.rightSource.Icon(), m.rightSource.Label()))

	synced, different, leftOnly, rightOnly := m.compState.Counts()
	m.status.SetCounts(synced, different, leftOnly, rightOnly)
	m.status.SetSources(m.leftSource.Label(), m.rightSource.Label())

	// Update detail labels.
	m.detail.SetLabels(m.leftSource.Label(), m.rightSource.Label())
}

func (m *tuiModel) toggleFocus() {
	m.focusLeft = !m.focusLeft
	m.left.SetFocused(m.focusLeft)
	m.right.SetFocused(!m.focusLeft)
	m.focusedPanel().MoveCursorToHighlight()
	m.updateLinkedCursor()
}

func (m *tuiModel) updateLinkedCursor() {
	var focusedPanel, unfocusedPanel *PanelModel
	if m.focusLeft {
		focusedPanel = &m.left
		unfocusedPanel = &m.right
	} else {
		focusedPanel = &m.right
		unfocusedPanel = &m.left
	}

	sel := focusedPanel.SelectedItem()
	if sel != nil {
		unfocusedPanel.SetHighlight(sel.Name)
	} else {
		unfocusedPanel.SetHighlight("")
	}
}

func (m *tuiModel) updateDetail() {
	var focused *PanelModel
	if m.focusLeft {
		focused = &m.left
	} else {
		focused = &m.right
	}

	sel := focused.SelectedItem()
	if sel == nil || m.compState == nil {
		m.detail.SetItem(nil, nil)
		return
	}

	var sync *ItemSync
	switch sel.Section {
	case SectionMCP:
		if s, ok := m.compState.MCPSync[sel.Name]; ok {
			sync = &s
		}
	case SectionPlugin:
		if s, ok := m.compState.PlugSync[sel.Name]; ok {
			sync = &s
		}
	case SectionMarketplace:
		if s, ok := m.compState.MktSync[sel.Name]; ok {
			sync = &s
		}
	}

	m.detail.SetItem(sel, sync)
}

// helpContext builds a HelpContext from the current model state.
func (m *tuiModel) helpContext() HelpContext {
	return HelpContext{
		FilterActive:   m.filtering,
		DetailExpanded: m.detail.Expanded(),
		OverlayActive:  m.anyOverlayActive(),
		Ready:          m.ready,
		Busy:           m.busy,
		HasProfile:     m.activeProfileName() != "",
	}
}

// anyOverlayActive returns true if any modal overlay is currently shown.
func (m *tuiModel) anyOverlayActive() bool {
	return m.alert.Active() || m.picker.Active() || m.bulkApply.Active() || m.progress.Active() ||
		m.profileCreate.Active() || m.configOverlay.Active() || m.confirm.Active() ||
		m.chezmoiPrompting || m.chezmoiRunning
}

// currentHelpHeight returns the help bar height for the current state.
func (m *tuiModel) currentHelpHeight() int {
	if !m.showHelp {
		return 0
	}
	groups := m.keys.ContextualHelp(m.helpContext())
	return HelpBarHeight(groups)
}

// recomputeLayout recalculates layout dimensions accounting for the current help bar height.
func (m *tuiModel) recomputeLayout() {
	m.layout = ComputeLayout(m.layout.TotalWidth, m.layout.TotalHeight, m.currentHelpHeight())
	m.applyLayout()
}

// applyFilter sets the filter on both panels and repopulates them.
func (m *tuiModel) applyFilter(text string) {
	m.filterText = strings.ToLower(text)
	m.left.SetFilter(m.filterText)
	m.right.SetFilter(m.filterText)
	m.populatePanels()
	m.updateDetail()
}

// clearFilter removes the filter and repopulates panels.
func (m *tuiModel) clearFilter() {
	m.filtering = false
	m.filterText = ""
	m.filterInput.Reset()
	m.filterInput.Blur()
	m.left.SetFilter("")
	m.right.SetFilter("")
	m.populatePanels()
	m.updateDetail()
}

// resolveChezmoiMode computes the effective chezmoi mode from config and the
// managed flag. Returns AutoReAddNever if chezmoi is disabled or the manifest
// is not managed.
func resolveChezmoiMode(opts Options) string {
	if !opts.ChezmoiManaged {
		return config.AutoReAddNever
	}
	c, err := config.Load(opts.ConfigPath)
	if err != nil || c == nil {
		return config.AutoReAddNever
	}
	if !config.BoolVal(c.Chezmoi.Enabled) {
		return config.AutoReAddNever
	}
	return config.AutoReAddMode(c.Chezmoi.AutoReAdd)
}

// refreshChezmoiMode recomputes the effective chezmoi mode from the current
// config on disk. Called when a chezmoi-related config key is changed in the
// config overlay.
func (m *tuiModel) refreshChezmoiMode() {
	m.chezmoiMode = resolveChezmoiMode(m.opts)
}

// viewChezmoiPrompt renders the "Run chezmoi re-add?" dialog over dimmed content.
func (m tuiModel) viewChezmoiPrompt(background string) string {
	boxWidth := 50
	if boxWidth > m.layout.TotalWidth-4 {
		boxWidth = m.layout.TotalWidth - 4
	}
	if boxWidth < 20 {
		boxWidth = 20
	}
	innerWidth := boxWidth - 4

	var lines []string
	lines = append(lines, StyleBold.Render("Run chezmoi re-add?"))
	lines = append(lines, "")
	lines = append(lines, "Manifest was modified this session.")
	lines = append(lines, "")

	// Buttons: [Cancel] [Run]
	var cancelBtn, runBtn string
	if m.chezmoiFocusRun {
		cancelBtn = StyleButtonNormal.Render("Cancel")
		runBtn = StyleButtonFocused.Render("Run")
	} else {
		cancelBtn = StyleButtonFocused.Render("Cancel")
		runBtn = StyleButtonNormal.Render("Run")
	}
	buttons := cancelBtn + "  " + runBtn
	btnWidth := lipgloss.Width(buttons)
	pad := (innerWidth - btnWidth) / 2
	if pad < 0 {
		pad = 0
	}
	lines = append(lines, strings.Repeat(" ", pad)+buttons)

	content := strings.Join(lines, "\n")
	box := StyleConfirmBorder.
		Padding(1, 1).
		Width(boxWidth).
		Render(content)

	dimmed := dimContent(background)
	return placeOverlay(dimmed, box, m.layout.TotalWidth, m.layout.TotalHeight)
}

// filterInputStyles returns textinput styles matching the TUI theme.
func filterInputStyles() textinput.Styles {
	base := textinput.StyleState{
		Text:        lipgloss.NewStyle().Foreground(ColorFG),
		Placeholder: lipgloss.NewStyle().Foreground(ColorDim),
		Prompt:      StyleHint,
	}
	return textinput.Styles{
		Focused: base,
		Blurred: base,
	}
}
