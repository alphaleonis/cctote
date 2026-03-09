package tui

import (
	"fmt"
	"image/color"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
	"github.com/alphaleonis/cctote/internal/manifest"
)

// Side identifies which panel this is.
type Side int

const (
	SideLeft Side = iota
	SideRight
)

// PanelItem represents a single row in the panel.
type PanelItem struct {
	IsHeader   bool // section header (non-selectable)
	Section    SectionKind
	Name       string // server name / plugin ID / marketplace name
	Detail     string // transport type / scope / source type
	SyncStatus SyncStatus
	Enabled    *bool // for plugins only
}

// PanelModel is the panel sub-model.
type PanelModel struct {
	side      Side
	source    PaneSource // what this panel is showing
	title     string     // custom title (set via SetTitle)
	items     []PanelItem
	cursor    int
	highlight string // linked cursor name from other panel
	offset    int    // scroll offset
	width     int
	height    int
	focused   bool
	keys      KeyMap
	selected  map[string]bool // multi-select state keyed by item Name
	filter    string          // lowercased filter text; empty = no filter
}

// NewPanel creates a new panel model.
func NewPanel(side Side, keys KeyMap) PanelModel {
	return PanelModel{
		side:     side,
		keys:     keys,
		selected: make(map[string]bool),
	}
}

// SetSource sets the panel's data source.
func (m *PanelModel) SetSource(src PaneSource) {
	m.source = src
}

// SetFilter sets the filter text (lowercased). Empty string clears the filter.
func (m *PanelModel) SetFilter(text string) {
	m.filter = text
}

// SetTitle sets the panel's border title.
func (m *PanelModel) SetTitle(title string) {
	m.title = title
}

// SetItems builds the flat item list from the sync state for this panel's side.
func (m *PanelModel) SetItems(state *SyncState) {
	m.items = nil

	// MCP Servers
	mcpKeys := SortedKeys(state.MCPSync)
	hasMCP := false
	for _, name := range mcpKeys {
		if m.shouldShow(name, state.MCPSync[name]) {
			hasMCP = true
			break
		}
	}
	if hasMCP {
		m.items = append(m.items, PanelItem{IsHeader: true, Section: SectionMCP, Name: fmt.Sprintf("%s MCP Servers", IconMCP)})
		for _, name := range mcpKeys {
			sync := state.MCPSync[name]
			if m.shouldShow(name, sync) {
				m.items = append(m.items, m.mcpItem(name, sync))
			}
		}
	}

	// Plugins
	plugKeys := SortedKeys(state.PlugSync)
	hasPlug := false
	for _, pid := range plugKeys {
		if m.shouldShow(pid, state.PlugSync[pid]) {
			hasPlug = true
			break
		}
	}
	if hasPlug {
		m.items = append(m.items, PanelItem{IsHeader: true, Section: SectionPlugin, Name: fmt.Sprintf("%s Plugins", IconPlugin)})
		for _, pid := range plugKeys {
			sync := state.PlugSync[pid]
			if m.shouldShow(pid, sync) {
				m.items = append(m.items, m.pluginItem(pid, sync))
			}
		}
	}

	// Marketplaces
	mktKeys := SortedKeys(state.MktSync)
	hasMkt := false
	for _, name := range mktKeys {
		if m.shouldShow(name, state.MktSync[name]) {
			hasMkt = true
			break
		}
	}
	if hasMkt {
		m.items = append(m.items, PanelItem{IsHeader: true, Section: SectionMarketplace, Name: fmt.Sprintf("%s Marketplaces", IconMarketplace)})
		for _, name := range mktKeys {
			sync := state.MktSync[name]
			if m.shouldShow(name, sync) {
				m.items = append(m.items, m.mktItem(name, sync))
			}
		}
	}

	// Reset cursor if out of range.
	m.cursor = m.clampCursor(m.cursor)

	// Prune selections that are no longer in the item list.
	names := make(map[string]bool, len(m.items))
	for _, item := range m.items {
		if !item.IsHeader {
			names[item.Name] = true
		}
	}
	for name := range m.selected {
		if !names[name] {
			delete(m.selected, name)
		}
	}
}

// shouldShow returns whether an item should appear in this panel.
func (m *PanelModel) shouldShow(name string, sync ItemSync) bool {
	switch sync.Status {
	case Synced, Different:
		// ok
	case LeftOnly:
		if m.side != SideLeft {
			return false
		}
	case RightOnly:
		if m.side != SideRight {
			return false
		}
	default:
		return false
	}
	if m.filter != "" && !strings.Contains(strings.ToLower(name), m.filter) {
		return false
	}
	return true
}

func (m *PanelModel) mcpItem(name string, sync ItemSync) PanelItem {
	var srv manifest.MCPServer
	if m.side == SideLeft && sync.Left != nil {
		srv = sync.Left.(manifest.MCPServer)
	} else if m.side == SideRight && sync.Right != nil {
		srv = sync.Right.(manifest.MCPServer)
	}
	transport := srv.Type
	if transport == "" {
		transport = "stdio"
	}
	return PanelItem{
		Section:    SectionMCP,
		Name:       name,
		Detail:     transport,
		SyncStatus: sync.Status,
	}
}

func (m *PanelModel) pluginItem(pid string, sync ItemSync) PanelItem {
	var plug manifest.Plugin
	if m.side == SideLeft && sync.Left != nil {
		plug = sync.Left.(manifest.Plugin)
	} else if m.side == SideRight && sync.Right != nil {
		plug = sync.Right.(manifest.Plugin)
	}
	enabled := plug.Enabled
	return PanelItem{
		Section:    SectionPlugin,
		Name:       pid,
		Detail:     plug.Scope,
		SyncStatus: sync.Status,
		Enabled:    &enabled,
	}
}

func (m *PanelModel) mktItem(name string, sync ItemSync) PanelItem {
	var mkt manifest.Marketplace
	if m.side == SideLeft && sync.Left != nil {
		mkt = sync.Left.(manifest.Marketplace)
	} else if m.side == SideRight && sync.Right != nil {
		mkt = sync.Right.(manifest.Marketplace)
	}
	return PanelItem{
		Section:    SectionMarketplace,
		Name:       name,
		Detail:     mkt.Source,
		SyncStatus: sync.Status,
	}
}

// SetSize updates the panel dimensions.
func (m *PanelModel) SetSize(width, height int) {
	m.width = width
	m.height = height
}

// SetFocused sets whether this panel is the active one.
func (m *PanelModel) SetFocused(focused bool) {
	m.focused = focused
}

// SetHighlight sets the linked cursor highlight name and scrolls to make the
// highlighted item visible.
func (m *PanelModel) SetHighlight(name string) {
	m.highlight = name
	if idx := m.indexOfItem(name); idx >= 0 {
		m.ensureIndexVisible(idx)
	}
}

// MoveCursorToHighlight moves the cursor to the item matching m.highlight.
func (m *PanelModel) MoveCursorToHighlight() {
	if idx := m.indexOfItem(m.highlight); idx >= 0 {
		m.cursor = idx
		m.ensureVisible()
	}
}

// indexOfItem returns the index of the first non-header item with the given
// name, or -1 if not found.
func (m *PanelModel) indexOfItem(name string) int {
	if name == "" {
		return -1
	}
	for i, item := range m.items {
		if !item.IsHeader && item.Name == name {
			return i
		}
	}
	return -1
}

// SelectedItem returns the currently selected item, or nil if none.
func (m *PanelModel) SelectedItem() *PanelItem {
	if m.cursor < 0 || m.cursor >= len(m.items) {
		return nil
	}
	item := m.items[m.cursor]
	if item.IsHeader {
		return nil
	}
	return &item
}

// ToggleSelect toggles the multi-select state of the cursor item and moves down.
func (m *PanelModel) ToggleSelect() {
	item := m.SelectedItem()
	if item == nil {
		return
	}
	if m.selected[item.Name] {
		delete(m.selected, item.Name)
	} else {
		m.selected[item.Name] = true
	}
	m.moveDown()
}

// SelectSection selects all items in the cursor item's section.
func (m *PanelModel) SelectSection() {
	item := m.SelectedItem()
	if item == nil {
		return
	}
	sect := item.Section
	for _, it := range m.items {
		if !it.IsHeader && it.Section == sect {
			m.selected[it.Name] = true
		}
	}
}

// ClearSelection removes all multi-select state.
func (m *PanelModel) ClearSelection() {
	m.selected = make(map[string]bool)
}

// HasSelection returns whether any items are multi-selected.
func (m *PanelModel) HasSelection() bool {
	return len(m.selected) > 0
}

// SelectedItems returns all multi-selected items.
func (m *PanelModel) SelectedItems() []PanelItem {
	var result []PanelItem
	for _, item := range m.items {
		if !item.IsHeader && m.selected[item.Name] {
			result = append(result, item)
		}
	}
	return result
}

// SelectionCount returns the number of multi-selected items.
func (m *PanelModel) SelectionCount() int {
	return len(m.selected)
}

// SectionItems returns all non-header items in the cursor item's section.
func (m *PanelModel) SectionItems() []PanelItem {
	item := m.SelectedItem()
	if item == nil {
		return nil
	}
	sect := item.Section
	var result []PanelItem
	for _, it := range m.items {
		if !it.IsHeader && it.Section == sect {
			result = append(result, it)
		}
	}
	return result
}

// Update handles key messages for panel navigation.
func (m PanelModel) Update(msg tea.Msg) (PanelModel, tea.Cmd) {
	if !m.focused {
		return m, nil
	}

	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, m.keys.Down):
			m.moveDown()
		case key.Matches(msg, m.keys.Up):
			m.moveUp()
		case key.Matches(msg, m.keys.Top):
			m.cursor = m.firstSelectable()
			m.ensureVisible()
		case key.Matches(msg, m.keys.Bottom):
			m.cursor = m.lastSelectable()
			m.ensureVisible()
		case key.Matches(msg, m.keys.SectMCP):
			m.jumpToSection(SectionMCP)
		case key.Matches(msg, m.keys.SectPlug):
			m.jumpToSection(SectionPlugin)
		case key.Matches(msg, m.keys.SectMkt):
			m.jumpToSection(SectionMarketplace)
		case key.Matches(msg, m.keys.Select):
			m.ToggleSelect()
		case key.Matches(msg, m.keys.SelectSect):
			m.SelectSection()
		}
	}

	return m, nil
}

// View renders the panel as a string.
func (m PanelModel) View() string {
	if m.width < 4 || m.height < 4 {
		return ""
	}

	// lipgloss Width/Height include borders, so content area is width-2 / height-2.
	contentWidth := m.width - 2
	contentHeight := m.height - 2

	// Panel title for the border.
	title := m.title
	if title == "" {
		if m.side == SideLeft {
			title = fmt.Sprintf(" %s Manifest ", IconMCP)
		} else {
			title = fmt.Sprintf(" %s Claude Code ", IconMCP)
		}
	}

	// Empty line at top for breathing room, then item rows.
	var rows []string
	rows = append(rows, "") // blank line

	maxLines := contentHeight - 1 // reserve 1 row for blank line
	usedLines := 0

	if len(m.items) == 0 {
		// Show placeholder when panel has no items.
		hint := m.emptyHint()
		rows = append(rows, "  "+StyleItemDim.Render(hint))
		usedLines++
	}

	for i := m.offset; i < len(m.items) && usedLines < maxLines; i++ {
		rendered := m.renderItem(i, contentWidth)
		lines := strings.Count(rendered, "\n") + 1
		if usedLines+lines > maxLines {
			break
		}
		rows = append(rows, rendered)
		usedLines += lines
	}

	// Pad remaining lines.
	for usedLines < maxLines {
		rows = append(rows, strings.Repeat(" ", contentWidth))
		usedLines++
	}

	content := strings.Join(rows, "\n")

	// Render body with left/right/bottom border only (no top).
	borderColor := SourceBorderColor(m.source.Kind, m.focused)
	body := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		BorderTop(false).
		Width(m.width).
		Height(m.height - 1). // -1 because we provide the top border ourselves
		Render(content)

	// Build top border with embedded title — must match body's rendered width.
	topBorder := borderTopWithTitle(title, m.width, borderColor, m.focused)

	return topBorder + "\n" + body
}

// borderTopWithTitle renders a rounded top border with an inline title.
// e.g. "╭─ 󰒍 Manifest ───────────────╮"
// totalWidth must match the lipgloss Width() used for the body (which
// includes the left+right border characters).
func borderTopWithTitle(title string, totalWidth int, borderColor color.Color, focused bool) string {
	bc := lipgloss.NewStyle().Foreground(borderColor)
	titleStyle := StyleBold
	if focused {
		titleStyle = titleStyle.Foreground(ColorFG)
	}

	topLeft := bc.Render("╭")
	topRight := bc.Render("╮")
	horiz := bc.Render("─")

	rendered := titleStyle.Render(title)
	titleWidth := lipgloss.Width(rendered)

	// totalWidth = ╭ + (inner) + ╮, so inner fill = totalWidth - 2.
	// Of that inner fill: 1 dash before title + titleWidth + remaining dashes.
	remaining := totalWidth - 2 - 1 - titleWidth
	if remaining < 0 {
		remaining = 0
	}

	fill := bc.Render(strings.Repeat("─", remaining))
	return topLeft + horiz + rendered + fill + topRight
}

func (m PanelModel) renderItem(idx, width int) string {
	item := m.items[idx]

	if item.IsHeader {
		text := StyleSectionHeader.Render(item.Name)
		row := panelPadRight(text, width)
		if idx > 0 {
			row = strings.Repeat(" ", width) + "\n" + row
		}
		return row
	}

	isCursor := idx == m.cursor && m.focused
	isHighlighted := item.Name == m.highlight && !m.focused
	isMultiSel := m.selected[item.Name]

	// Row style — every segment inherits this so the background is
	// continuous and right-side icons are never truncated by ANSI-inflated
	// rune counts.
	var rowStyle lipgloss.Style
	switch {
	case isCursor:
		rowStyle = StyleItemSelected
	case isMultiSel:
		rowStyle = StyleItemMultiSel
	case isHighlighted:
		rowStyle = StyleItemHighlight
	default:
		if item.SyncStatus == Synced && m.focused {
			rowStyle = StyleItemSynced
		} else {
			rowStyle = StyleItemNormal
		}
	}
	r := rowStyle.Render
	rfg := func(fg color.Color, s string) string {
		return rowStyle.Foreground(fg).Render(s)
	}

	// --- Fixed-width segments ---

	// Sync column (left-side, 2 chars: icon + space) so the presence
	// indicator is immediately visible next to the item name.
	const syncWidth = 2

	enabledText := ""
	enabledWidth := 0
	if item.Enabled != nil {
		if *item.Enabled {
			enabledText = "on"
			enabledWidth = 4 // "  on"
		} else {
			enabledText = "off"
			enabledWidth = 5 // "  off"
		}
	}

	// Available width for name+detail (prefix=2, sync=2, suffix=2, rest is fixed).
	availWidth := width - 2 - syncWidth - enabledWidth - 2
	if availWidth < 0 {
		availWidth = 0
	}

	// --- Name + detail: truncate plain text, then style ---
	nameDetail := item.Name
	if item.Detail != "" {
		nameDetail += "  " + item.Detail
	}
	nameDetail = panelTruncate(nameDetail, availWidth)
	nameRunes := []rune(item.Name)
	truncRunes := []rune(nameDetail)
	ndLen := len(truncRunes)

	var styledND string
	if ndLen <= len(nameRunes) {
		styledND = r(nameDetail)
	} else {
		styledND = r(item.Name) + rfg(ColorDim, string(truncRunes[len(nameRunes):]))
	}

	pad := availWidth - ndLen
	if pad < 0 {
		pad = 0
	}

	// --- Assemble line ---
	var b strings.Builder

	// Prefix (2 chars).
	switch {
	case isMultiSel:
		b.WriteString(rfg(ColorBlue, IconSelected))
		b.WriteString(r(" "))
	default:
		b.WriteString(r("  "))
	}

	// Sync icon (left of name so it's immediately visible).
	switch item.SyncStatus {
	case Synced:
		b.WriteString(rfg(ColorGreen, IconSynced))
		b.WriteString(r(" "))
	case Different:
		b.WriteString(rfg(ColorYellow, IconDifferent))
		b.WriteString(r(" "))
	default:
		b.WriteString(r("  "))
	}

	// Name + detail + padding.
	b.WriteString(styledND)
	if pad > 0 {
		b.WriteString(r(strings.Repeat(" ", pad)))
	}

	// Enabled indicator.
	if enabledText != "" {
		b.WriteString(r("  "))
		if *item.Enabled {
			b.WriteString(rfg(ColorGreen, enabledText))
		} else {
			b.WriteString(rfg(ColorDim, enabledText))
		}
	}

	// Suffix (2 chars).
	b.WriteString(r("  "))

	return b.String()
}

// Navigation helpers

func (m *PanelModel) moveDown() {
	for i := m.cursor + 1; i < len(m.items); i++ {
		if !m.items[i].IsHeader {
			m.cursor = i
			m.ensureVisible()
			return
		}
	}
}

func (m *PanelModel) moveUp() {
	for i := m.cursor - 1; i >= 0; i-- {
		if !m.items[i].IsHeader {
			m.cursor = i
			m.ensureVisible()
			return
		}
	}
}

func (m *PanelModel) jumpToSection(section SectionKind) {
	for i, item := range m.items {
		if item.IsHeader && item.Section == section {
			for j := i + 1; j < len(m.items); j++ {
				if !m.items[j].IsHeader {
					m.cursor = j
					m.ensureVisible()
					return
				}
			}
			return
		}
	}
}

func (m *PanelModel) firstSelectable() int {
	for i, item := range m.items {
		if !item.IsHeader {
			return i
		}
	}
	return 0
}

func (m *PanelModel) lastSelectable() int {
	for i := len(m.items) - 1; i >= 0; i-- {
		if !m.items[i].IsHeader {
			return i
		}
	}
	return 0
}

func (m *PanelModel) clampCursor(pos int) int {
	if len(m.items) == 0 {
		return 0
	}
	if pos >= len(m.items) {
		pos = len(m.items) - 1
	}
	if pos < 0 {
		pos = 0
	}
	if m.items[pos].IsHeader {
		for i := pos; i < len(m.items); i++ {
			if !m.items[i].IsHeader {
				return i
			}
		}
		for i := pos; i >= 0; i-- {
			if !m.items[i].IsHeader {
				return i
			}
		}
	}
	return pos
}

func (m *PanelModel) ensureVisible() {
	m.ensureIndexVisible(m.cursor)
}

func (m *PanelModel) ensureIndexVisible(idx int) {
	// height - 2 (border top+bottom) - 1 (blank line at top)
	maxLines := m.height - 3
	if maxLines < 1 {
		maxLines = 1
	}
	if idx < m.offset {
		m.offset = idx
		return
	}
	// Count visual lines from offset to idx (inclusive).
	lines := 0
	for i := m.offset; i <= idx && i < len(m.items); i++ {
		h := 1
		if m.items[i].IsHeader && i > 0 {
			h = 2 // blank line above non-first headers
		}
		lines += h
	}
	for lines > maxLines {
		h := 1
		if m.items[m.offset].IsHeader && m.offset > 0 {
			h = 2
		}
		lines -= h
		m.offset++
	}
}

func panelTruncate(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	if max <= 1 {
		return string(runes[:max])
	}
	return string(runes[:max-1]) + "…"
}

// emptyHint returns a context-appropriate placeholder for an empty panel.
func (m *PanelModel) emptyHint() string {
	if m.filter != "" {
		return "(no matches)"
	}
	switch m.source.Kind {
	case SourceManifest:
		return "(no items in manifest — export from Claude Code with < or >)"
	case SourceProfile:
		return "(empty profile)"
	case SourceClaudeCode:
		return "(no items found in Claude Code)"
	case SourceProject:
		return "(no project configuration — no .mcp.json or .claude/ found)"
	}
	return "(no items)"
}

func panelPadRight(s string, width int) string {
	runes := []rune(s)
	if len(runes) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(runes))
}
