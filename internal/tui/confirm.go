package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
)

// ConfirmModel is the confirmation overlay model.
type ConfirmModel struct {
	active       bool
	title        string
	items        []CopyItem
	op           ResolvedOp
	from         PaneSource // source we're copying from
	to           PaneSource // source we're copying to
	diffLines    []string   // pre-rendered diff for single item
	dangerAction bool       // render action button in danger style
	focusOK      bool       // true = action button focused, false = cancel
	width        int
	height       int
	keys         KeyMap
}

// NewConfirm creates a new confirmation model.
func NewConfirm(keys KeyMap) ConfirmModel {
	return ConfirmModel{keys: keys}
}

// Activate shows the confirmation overlay.
func (m *ConfirmModel) Activate(title string, items []CopyItem, op ResolvedOp, from, to PaneSource, diffLines []string) {
	m.active = true
	m.title = title
	m.items = items
	m.op = op
	m.from = from
	m.to = to
	m.diffLines = diffLines
	m.dangerAction = op.IsDangerous()
	m.focusOK = false // default to cancel for safety
}

// Deactivate hides the overlay.
func (m *ConfirmModel) Deactivate() {
	m.active = false
	m.items = nil
	m.diffLines = nil
}

// Active returns whether the overlay is showing.
func (m *ConfirmModel) Active() bool {
	return m.active
}

// SetSize updates the available area for the overlay.
func (m *ConfirmModel) SetSize(width, height int) {
	m.width = width
	m.height = height
}

// Update handles key events for the overlay.
func (m ConfirmModel) Update(msg tea.Msg) (ConfirmModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, m.keys.Confirm):
			if m.focusOK {
				return m, func() tea.Msg {
					return ConfirmAcceptedMsg{Items: m.items, Op: m.op, From: m.from, To: m.to}
				}
			}
			// Enter on Cancel = cancel
			return m, func() tea.Msg { return ConfirmCancelledMsg{} }
		case key.Matches(msg, m.keys.Cancel):
			return m, func() tea.Msg { return ConfirmCancelledMsg{} }
		case key.Matches(msg, m.keys.Tab),
			key.Matches(msg, m.keys.Left),
			key.Matches(msg, m.keys.Right):
			m.focusOK = !m.focusOK
		case key.Matches(msg, m.keys.ForceQuit):
			return m, tea.Quit
		}
	}
	return m, nil
}

// ViewOver renders the confirmation dialog centered over a dimmed background.
func (m ConfirmModel) ViewOver(background string) string {
	if !m.active {
		return background
	}

	boxWidth := 50
	if boxWidth > m.width-4 {
		boxWidth = m.width - 4
	}
	if boxWidth < 20 {
		boxWidth = 20
	}
	innerWidth := boxWidth - 4 // padding + border

	var lines []string

	// Title
	lines = append(lines, StyleBold.Render(m.title))
	lines = append(lines, "")

	// Body: diff lines for single item, or descriptive body when items is nil
	// (e.g., OpDeleteProfile passes profile contents as diffLines with no items).
	if len(m.items) <= 1 && len(m.diffLines) > 0 {
		lines = append(lines, m.diffLines...)
	} else if len(m.items) > 0 {
		for _, item := range m.items {
			icon := detailSectionIcon(item.Section)
			lines = append(lines, fmt.Sprintf("  %s %s", icon, item.Name))
		}
		// Append diff lines (e.g. cascade details) after the item list.
		if len(m.diffLines) > 0 {
			lines = append(lines, "")
			lines = append(lines, m.diffLines...)
		}
	}
	lines = append(lines, "")

	// Buttons
	lines = append(lines, m.renderButtons(innerWidth))

	content := strings.Join(lines, "\n")

	box := StyleConfirmBorder.
		Padding(1, 1).
		Width(boxWidth).
		Render(content)

	// Dim the background, then composite the dialog on top.
	dimmed := dimContent(background)
	return placeOverlay(dimmed, box, m.width, m.height)
}

// placeOverlay composites fg centered over bg, replacing background lines
// in the overlay region.
func placeOverlay(bg, fg string, width, height int) string {
	bgLines := strings.Split(bg, "\n")
	fgLines := strings.Split(fg, "\n")

	fgW := lipgloss.Width(fg)
	fgH := len(fgLines)

	// Center position.
	startY := (height - fgH) / 2
	startX := (width - fgW) / 2
	if startY < 0 {
		startY = 0
	}
	if startX < 0 {
		startX = 0
	}

	// Pad bg to fill height if needed.
	for len(bgLines) < height {
		bgLines = append(bgLines, strings.Repeat(" ", width))
	}

	// Composite: replace bg lines in the overlay region.
	for i, fgLine := range fgLines {
		y := startY + i
		if y >= len(bgLines) {
			break
		}
		bgPlain := stripAnsi(bgLines[y])
		// Build: left padding from bg + fg line + right remainder from bg.
		var b strings.Builder
		// Left portion (dimmed background chars before the overlay).
		leftRunes := []rune(bgPlain)
		if startX > 0 {
			if startX <= len(leftRunes) {
				dim := lipgloss.NewStyle().Foreground(ColorDim)
				b.WriteString(dim.Render(string(leftRunes[:startX])))
			} else {
				b.WriteString(strings.Repeat(" ", startX))
			}
		}
		b.WriteString(fgLine)
		// Right portion (dimmed background chars after the overlay).
		rightStart := startX + fgW
		if rightStart < len(leftRunes) {
			dim := lipgloss.NewStyle().Foreground(ColorDim)
			b.WriteString(dim.Render(string(leftRunes[rightStart:])))
		}
		bgLines[y] = b.String()
	}

	return strings.Join(bgLines, "\n")
}

// dimContent applies the dim style to every line of content.
func dimContent(s string) string {
	dim := lipgloss.NewStyle().Foreground(ColorDim)
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		// Strip existing ANSI and re-render dimmed.
		plain := stripAnsi(line)
		lines[i] = dim.Render(plain)
	}
	return strings.Join(lines, "\n")
}

// stripAnsi removes ANSI escape sequences from a string.
// It operates on runes to avoid corrupting multi-byte UTF-8 characters.
func stripAnsi(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	runes := []rune(s)
	i := 0
	for i < len(runes) {
		if runes[i] == '\x1b' && i+1 < len(runes) && runes[i+1] == '[' {
			// CSI sequence: skip ESC [ ... <terminator>
			// Parameters/intermediates are in 0x20–0x3F, terminator is 0x40–0x7E.
			j := i + 2
			for j < len(runes) && runes[j] >= 0x20 && runes[j] <= 0x3F {
				j++
			}
			if j < len(runes) && runes[j] >= 0x40 && runes[j] <= 0x7E {
				j++ // skip the terminator
			}
			i = j
		} else {
			b.WriteRune(runes[i])
			i++
		}
	}
	return b.String()
}

func (m ConfirmModel) renderButtons(width int) string {
	actionLabel := m.op.Label()

	var cancelBtn, actionBtn string
	if m.focusOK {
		cancelBtn = StyleButtonNormal.Render("Cancel")
		if m.dangerAction {
			actionBtn = StyleButtonDanger.Render(actionLabel)
		} else {
			actionBtn = StyleButtonFocused.Render(actionLabel)
		}
	} else {
		cancelBtn = StyleButtonFocused.Render("Cancel")
		if m.dangerAction {
			actionBtn = StyleButtonNormal.Foreground(ColorRed).Render(actionLabel)
		} else {
			actionBtn = StyleButtonNormal.Render(actionLabel)
		}
	}

	buttons := cancelBtn + "  " + actionBtn
	btnWidth := lipgloss.Width(buttons)
	pad := (width - btnWidth) / 2
	if pad < 0 {
		pad = 0
	}

	return strings.Repeat(" ", pad) + buttons
}
