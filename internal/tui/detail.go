package tui

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/alphaleonis/cctote/internal/manifest"
)

const maskPlaceholder = "••••••••"

// DetailModel is the detail pane model.
type DetailModel struct {
	item            *PanelItem
	sync            *ItemSync
	leftLabel       string // source label for left pane
	rightLabel      string // source label for right pane
	width           int
	height          int
	secretsRevealed bool
	expanded        bool
	scrollOffset    int
}

// NewDetail creates a new detail model.
func NewDetail() DetailModel {
	return DetailModel{}
}

// SetSize updates the detail pane dimensions.
func (m *DetailModel) SetSize(width, height int) {
	m.width = width
	m.height = height
}

// SetLabels sets the source labels used for "X only" badges.
func (m *DetailModel) SetLabels(left, right string) {
	m.leftLabel = left
	m.rightLabel = right
}

// SetItem sets the current item and its sync data to display.
// Resets secretsRevealed so secrets are masked when navigating between items.
func (m *DetailModel) SetItem(item *PanelItem, sync *ItemSync) {
	m.item = item
	m.sync = sync
	m.secretsRevealed = false
}

// ToggleSecrets toggles the visibility of secret values (env vars, headers).
func (m *DetailModel) ToggleSecrets() {
	m.secretsRevealed = !m.secretsRevealed
}

// Expand enters full-screen detail mode.
func (m *DetailModel) Expand() {
	m.expanded = true
	m.scrollOffset = 0
}

// Collapse returns to normal detail pane size.
func (m *DetailModel) Collapse() {
	m.expanded = false
	m.scrollOffset = 0
}

// Expanded returns whether the detail pane is in full-screen mode.
func (m *DetailModel) Expanded() bool {
	return m.expanded
}

// ScrollDown scrolls the expanded detail pane down by one line.
func (m *DetailModel) ScrollDown() {
	m.scrollOffset++
}

// ScrollUp scrolls the expanded detail pane up by one line.
func (m *DetailModel) ScrollUp() {
	if m.scrollOffset > 0 {
		m.scrollOffset--
	}
}

// HasSecrets returns true if the current item has maskable fields.
func (m *DetailModel) HasSecrets() bool {
	if m.item == nil || m.sync == nil || m.item.Section != SectionMCP {
		return false
	}
	check := func(srv manifest.MCPServer) bool {
		return len(srv.Env) > 0 || len(srv.Headers) > 0
	}
	if m.sync.Left != nil && check(m.sync.Left.(manifest.MCPServer)) {
		return true
	}
	if m.sync.Right != nil && check(m.sync.Right.(manifest.MCPServer)) {
		return true
	}
	return false
}

// View renders the detail pane.
func (m DetailModel) View() string {
	if m.height <= 0 || m.width < 4 {
		return ""
	}

	// lipgloss Width/Height include borders.
	contentWidth := m.width - 2
	contentHeight := m.height

	// Title as first content row, then detail content below.
	title := m.renderTitle()
	var body string
	if m.item == nil {
		body = StyleItemDim.Render("No item selected")
	} else if m.expanded {
		// Expanded: render all content, then apply scroll window.
		raw := m.renderContent(contentWidth-3, math.MaxInt) // no truncation in expanded mode
		allLines := strings.Split(raw, "\n")

		// Clamp scroll offset.
		visibleLines := contentHeight - 1 // -1 for title row
		maxOffset := len(allLines) - visibleLines
		if maxOffset < 0 {
			maxOffset = 0
		}
		offset := m.scrollOffset
		if offset > maxOffset {
			offset = maxOffset
		}

		end := offset + visibleLines
		if end > len(allLines) {
			end = len(allLines)
		}
		visible := allLines[offset:end]

		var indented []string
		for _, line := range visible {
			indented = append(indented, "   "+line)
		}
		body = strings.Join(indented, "\n")
	} else {
		raw := m.renderContent(contentWidth-3, contentHeight-1) // -1 for title row, -3 for indent
		var indented []string
		for _, line := range strings.Split(raw, "\n") {
			indented = append(indented, "   "+line)
		}
		body = strings.Join(indented, "\n")
	}

	content := title + "\n" + body

	borderStyle := UnfocusedBorder
	if m.expanded {
		borderStyle = FocusedBorder
	}

	return borderStyle.
		Width(m.width).
		Height(m.height).
		Render(content)
}

func (m DetailModel) renderTitle() string {
	if m.item == nil {
		return " Detail "
	}

	icon := detailSectionIcon(m.item.Section)
	typeName := detailSectionTypeName(m.item.Section)
	badge := m.statusBadge(m.item.SyncStatus)

	title := fmt.Sprintf(" %s %s %s %s %s",
		icon, m.item.Name,
		StyleItemDim.Render("·"),
		StyleItemDim.Render(typeName),
		badge,
	)

	if m.HasSecrets() {
		if m.secretsRevealed {
			title += "  " + StyleHint.Render("(s to hide)")
		} else {
			title += "  " + StyleHint.Render("(s to reveal)")
		}
	}

	if m.expanded {
		title += "  " + StyleHint.Render("j/k:scroll  esc:collapse")
	}

	return title + " "
}

func (m DetailModel) renderContent(width, height int) string {
	if m.sync == nil {
		return ""
	}

	compact := height <= 2

	switch m.item.Section {
	case SectionMCP:
		return m.renderMCP(width, compact)
	case SectionPlugin:
		return m.renderPlugin(compact)
	case SectionMarketplace:
		return m.renderMarketplace(compact)
	}
	return ""
}

// maskValue returns the value as-is if secrets are revealed, or a mask placeholder.
func (m DetailModel) maskValue(val string) string {
	if m.secretsRevealed {
		return val
	}
	return maskPlaceholder
}

func (m DetailModel) renderMCP(_ int, compact bool) string {
	if compact {
		return m.compactLine()
	}

	var lines []string

	if m.sync.Status == Different {
		var leftSrv, rightSrv manifest.MCPServer
		if m.sync.Left != nil {
			leftSrv = m.sync.Left.(manifest.MCPServer)
		}
		if m.sync.Right != nil {
			rightSrv = m.sync.Right.(manifest.MCPServer)
		}
		lines = append(lines, m.diffMCPFieldsFull(leftSrv, rightSrv)...)
	} else {
		srv := m.getSingleMCP()
		transport := srv.Type
		if transport == "" {
			transport = "stdio"
		}
		lines = append(lines, fmt.Sprintf("Type: %s", transport))
		if srv.Command != "" {
			cmd := srv.Command
			if len(srv.Args) > 0 {
				cmd += " " + strings.Join(srv.Args, " ")
			}
			lines = append(lines, fmt.Sprintf("Command: %s", cmd))
		}
		if srv.URL != "" {
			lines = append(lines, fmt.Sprintf("URL: %s", srv.URL))
		}
		if srv.CWD != "" {
			lines = append(lines, fmt.Sprintf("CWD: %s", srv.CWD))
		}
		lines = append(lines, m.renderEnvMap("Env", srv.Env)...)
		lines = append(lines, m.renderEnvMap("Headers", srv.Headers)...)
	}

	return strings.Join(lines, "\n")
}

// renderEnvMap renders a map of key-value pairs with masked values.
func (m DetailModel) renderEnvMap(label string, kvs map[string]string) []string {
	if len(kvs) == 0 {
		return nil
	}
	keys := sortedMapKeys(kvs)
	var lines []string
	for i, k := range keys {
		val := m.maskValue(kvs[k])
		if i == 0 {
			lines = append(lines, fmt.Sprintf("%s: %s=%s", label, k, val))
		} else {
			lines = append(lines, fmt.Sprintf("%s  %s=%s", strings.Repeat(" ", len(label)+1), k, val))
		}
	}
	return lines
}

// diffMCPFieldsFull is like detailDiffMCPFields but includes Env and Headers
// with secret masking.
func (m DetailModel) diffMCPFieldsFull(left, right manifest.MCPServer) []string {
	// Start with the existing structural diff (Type, Command, URL).
	lines := detailDiffMCPFields(left, right)

	// Env diff.
	lines = append(lines, m.diffEnvMap("Env", left.Env, right.Env)...)

	// Headers diff.
	lines = append(lines, m.diffEnvMap("Headers", left.Headers, right.Headers)...)

	return lines
}

// diffEnvMap renders a diff of two key-value maps with secret masking.
func (m DetailModel) diffEnvMap(label string, leftMap, rightMap map[string]string) []string {
	if len(leftMap) == 0 && len(rightMap) == 0 {
		return nil
	}
	if leftMap == nil {
		leftMap = map[string]string{}
	}
	if rightMap == nil {
		rightMap = map[string]string{}
	}

	// Collect all keys.
	allKeys := make(map[string]bool)
	for k := range leftMap {
		allKeys[k] = true
	}
	for k := range rightMap {
		allKeys[k] = true
	}
	keys := make([]string, 0, len(allKeys))
	for k := range allKeys {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var lines []string
	for _, k := range keys {
		lv, inLeft := leftMap[k]
		rv, inRight := rightMap[k]
		switch {
		case inLeft && inRight && lv == rv:
			lines = append(lines, fmt.Sprintf("%s: %s=%s", label, k, m.maskValue(lv)))
		case inLeft && inRight:
			lines = append(lines,
				StyleDiffRemove.Render(fmt.Sprintf("- %s: %s=%s", label, k, m.maskValue(lv))),
				StyleDiffAdd.Render(fmt.Sprintf("+ %s: %s=%s", label, k, m.maskValue(rv))),
			)
		case inLeft:
			lines = append(lines, StyleDiffRemove.Render(fmt.Sprintf("- %s: %s=%s", label, k, m.maskValue(lv))))
		case inRight:
			lines = append(lines, StyleDiffAdd.Render(fmt.Sprintf("+ %s: %s=%s", label, k, m.maskValue(rv))))
		}
	}
	return lines
}

func (m DetailModel) renderPlugin(compact bool) string {
	if compact {
		return m.compactLine()
	}

	var lines []string

	if m.sync.Status == Different {
		var leftP, rightP manifest.Plugin
		if m.sync.Left != nil {
			leftP = m.sync.Left.(manifest.Plugin)
		}
		if m.sync.Right != nil {
			rightP = m.sync.Right.(manifest.Plugin)
		}
		if leftP.Scope != rightP.Scope {
			lines = append(lines,
				StyleDiffRemove.Render(fmt.Sprintf("- Scope: %s", leftP.Scope)),
				StyleDiffAdd.Render(fmt.Sprintf("+ Scope: %s", rightP.Scope)),
			)
		} else {
			lines = append(lines, fmt.Sprintf("Scope: %s", leftP.Scope))
		}
		if leftP.Enabled != rightP.Enabled {
			lines = append(lines,
				StyleDiffRemove.Render(fmt.Sprintf("- Enabled: %v", leftP.Enabled)),
				StyleDiffAdd.Render(fmt.Sprintf("+ Enabled: %v", rightP.Enabled)),
			)
		} else {
			lines = append(lines, fmt.Sprintf("Enabled: %v", leftP.Enabled))
		}
	} else {
		plug := m.getSinglePlugin()
		lines = append(lines, fmt.Sprintf("Scope: %s", plug.Scope))
		lines = append(lines, fmt.Sprintf("Enabled: %v", plug.Enabled))
	}

	return strings.Join(lines, "\n")
}

func (m DetailModel) renderMarketplace(compact bool) string {
	if compact {
		return m.compactLine()
	}

	var lines []string

	if m.sync.Status == Different {
		var leftMkt, rightMkt manifest.Marketplace
		if m.sync.Left != nil {
			leftMkt = m.sync.Left.(manifest.Marketplace)
		}
		if m.sync.Right != nil {
			rightMkt = m.sync.Right.(manifest.Marketplace)
		}
		if leftMkt.Source != rightMkt.Source {
			lines = append(lines,
				StyleDiffRemove.Render(fmt.Sprintf("- Source: %s", leftMkt.Source)),
				StyleDiffAdd.Render(fmt.Sprintf("+ Source: %s", rightMkt.Source)),
			)
		} else {
			lines = append(lines, fmt.Sprintf("Source: %s", leftMkt.Source))
		}
		leftLoc := leftMkt.SourceLocator()
		rightLoc := rightMkt.SourceLocator()
		if leftLoc != rightLoc {
			lines = append(lines,
				StyleDiffRemove.Render(fmt.Sprintf("- Location: %s", leftLoc)),
				StyleDiffAdd.Render(fmt.Sprintf("+ Location: %s", rightLoc)),
			)
		}
	} else {
		mkt := m.getSingleMarketplace()
		lines = append(lines, fmt.Sprintf("Source: %s", mkt.Source))
		loc := mkt.SourceLocator()
		if loc != "" {
			lines = append(lines, fmt.Sprintf("Location: %s", loc))
		}
	}

	return strings.Join(lines, "\n")
}

func (m DetailModel) compactLine() string {
	typeName := detailSectionTypeName(m.item.Section)
	badge := m.statusBadge(m.item.SyncStatus)
	return fmt.Sprintf("%s · %s %s", m.item.Name, typeName, badge)
}

func (m DetailModel) getSingleMCP() manifest.MCPServer {
	if m.sync.Left != nil {
		return m.sync.Left.(manifest.MCPServer)
	}
	if m.sync.Right != nil {
		return m.sync.Right.(manifest.MCPServer)
	}
	return manifest.MCPServer{}
}

func (m DetailModel) getSinglePlugin() manifest.Plugin {
	if m.sync.Left != nil {
		return m.sync.Left.(manifest.Plugin)
	}
	if m.sync.Right != nil {
		return m.sync.Right.(manifest.Plugin)
	}
	return manifest.Plugin{}
}

func (m DetailModel) getSingleMarketplace() manifest.Marketplace {
	if m.sync.Left != nil {
		return m.sync.Left.(manifest.Marketplace)
	}
	if m.sync.Right != nil {
		return m.sync.Right.(manifest.Marketplace)
	}
	return manifest.Marketplace{}
}

// diffMCPLines returns diff lines for two MCP server configs.
// Exported within the package for use by confirm.go.
func diffMCPLines(left, right manifest.MCPServer) []string {
	return detailDiffMCPFields(left, right)
}

// diffPluginLines returns diff lines for two plugin configs.
func diffPluginLines(left, right manifest.Plugin) []string {
	var lines []string
	if left.Scope != right.Scope {
		lines = append(lines,
			StyleDiffRemove.Render(fmt.Sprintf("- Scope: %s", left.Scope)),
			StyleDiffAdd.Render(fmt.Sprintf("+ Scope: %s", right.Scope)),
		)
	} else {
		lines = append(lines, fmt.Sprintf("Scope: %s", left.Scope))
	}
	if left.Enabled != right.Enabled {
		lines = append(lines,
			StyleDiffRemove.Render(fmt.Sprintf("- Enabled: %v", left.Enabled)),
			StyleDiffAdd.Render(fmt.Sprintf("+ Enabled: %v", right.Enabled)),
		)
	} else {
		lines = append(lines, fmt.Sprintf("Enabled: %v", left.Enabled))
	}
	return lines
}

// diffMarketplaceLines returns diff lines for two marketplace configs.
func diffMarketplaceLines(left, right manifest.Marketplace) []string {
	var lines []string
	if left.Source != right.Source {
		lines = append(lines,
			StyleDiffRemove.Render(fmt.Sprintf("- Source: %s", left.Source)),
			StyleDiffAdd.Render(fmt.Sprintf("+ Source: %s", right.Source)),
		)
	} else {
		lines = append(lines, fmt.Sprintf("Source: %s", left.Source))
	}
	leftLoc := left.SourceLocator()
	rightLoc := right.SourceLocator()
	if leftLoc != rightLoc {
		lines = append(lines,
			StyleDiffRemove.Render(fmt.Sprintf("- Location: %s", leftLoc)),
			StyleDiffAdd.Render(fmt.Sprintf("+ Location: %s", rightLoc)),
		)
	}
	return lines
}

func detailDiffMCPFields(left, right manifest.MCPServer) []string {
	var lines []string

	leftType := left.Type
	if leftType == "" {
		leftType = "stdio"
	}
	rightType := right.Type
	if rightType == "" {
		rightType = "stdio"
	}

	if leftType != rightType {
		lines = append(lines,
			StyleDiffRemove.Render(fmt.Sprintf("- Type: %s", leftType)),
			StyleDiffAdd.Render(fmt.Sprintf("+ Type: %s", rightType)),
		)
	} else {
		lines = append(lines, fmt.Sprintf("Type: %s", leftType))
	}

	if left.Command != right.Command {
		lines = append(lines,
			StyleDiffRemove.Render(fmt.Sprintf("- Command: %s", left.Command)),
			StyleDiffAdd.Render(fmt.Sprintf("+ Command: %s", right.Command)),
		)
	} else if left.Command != "" {
		lines = append(lines, fmt.Sprintf("Command: %s", left.Command))
	}

	if left.URL != right.URL {
		lines = append(lines,
			StyleDiffRemove.Render(fmt.Sprintf("- URL: %s", left.URL)),
			StyleDiffAdd.Render(fmt.Sprintf("+ URL: %s", right.URL)),
		)
	} else if left.URL != "" {
		lines = append(lines, fmt.Sprintf("URL: %s", left.URL))
	}

	return lines
}

func detailSectionIcon(s SectionKind) string {
	switch s {
	case SectionMCP:
		return IconMCP
	case SectionPlugin:
		return IconPlugin
	case SectionMarketplace:
		return IconMarketplace
	}
	return ""
}

func detailSectionTypeName(s SectionKind) string {
	switch s {
	case SectionMCP:
		return "MCP Server"
	case SectionPlugin:
		return "Plugin"
	case SectionMarketplace:
		return "Marketplace"
	}
	return ""
}

func sortedMapKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func (m DetailModel) statusBadge(s SyncStatus) string {
	leftLabel := m.leftLabel
	if leftLabel == "" {
		leftLabel = "left"
	}
	rightLabel := m.rightLabel
	if rightLabel == "" {
		rightLabel = "right"
	}

	switch s {
	case Synced:
		return StyleStatusSynced.Render(IconSynced + " synced")
	case Different:
		return StyleStatusDiff.Render(IconDifferent + " different")
	case LeftOnly:
		return StyleStatusLeft.Render(leftLabel + " only")
	case RightOnly:
		return StyleStatusRight.Render(rightLabel + " only")
	}
	return ""
}
