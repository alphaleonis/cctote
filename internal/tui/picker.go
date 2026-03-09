package tui

import (
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
)

// PickerModel is the source picker overlay, rendered as a dropdown from the
// panel title bar.
type PickerModel struct {
	active        bool
	side          Side
	items         []PaneSource
	cursor        int
	anchorX       int // left edge X position for the dropdown
	width, height int
	keys          KeyMap
}

// NewPicker creates a new picker model.
func NewPicker(keys KeyMap) PickerModel {
	return PickerModel{keys: keys}
}

// Activate opens the picker with the given source options. anchorX is the
// X coordinate where the dropdown's left edge should be placed.
func (m *PickerModel) Activate(side Side, sources []PaneSource, current PaneSource, anchorX int) {
	m.side = side
	m.items = sources
	m.anchorX = anchorX
	m.cursor = 0
	for i, src := range m.items {
		if src.Equal(current) {
			m.cursor = i
			break
		}
	}
	m.active = true
}

// Deactivate hides the picker.
func (m *PickerModel) Deactivate() {
	m.active = false
	m.items = nil
}

// Active returns whether the picker is showing.
func (m *PickerModel) Active() bool {
	return m.active
}

// SetSize updates the available area.
func (m *PickerModel) SetSize(width, height int) {
	m.width = width
	m.height = height
}

// Update handles key events.
func (m PickerModel) Update(msg tea.Msg) (PickerModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, m.keys.Down):
			if m.cursor < len(m.items)-1 {
				m.cursor++
			}
		case key.Matches(msg, m.keys.Up):
			if m.cursor > 0 {
				m.cursor--
			}
		case key.Matches(msg, m.keys.Confirm):
			selected := m.items[m.cursor]
			m.active = false
			return m, func() tea.Msg {
				return SourceSelectedMsg{Side: m.side, Source: selected}
			}
		case key.Matches(msg, m.keys.Cancel):
			m.active = false
		case key.Matches(msg, m.keys.ForceQuit):
			return m, tea.Quit
		}
	}
	return m, nil
}

// ViewOver renders the picker as a dropdown anchored below the panel title,
// over a dimmed background.
func (m PickerModel) ViewOver(background string) string {
	if !m.active {
		return background
	}

	var rows []string
	for i, src := range m.items {
		label := src.Icon() + " " + src.Label()
		prefix := "  "
		if i == m.cursor {
			prefix = StyleBold.Foreground(ColorBlue).Render(IconSelected) + " "
		}
		style := StyleItemNormal
		if i == m.cursor {
			style = StyleItemSelected
		}
		rows = append(rows, prefix+style.Render(label))
	}

	content := strings.Join(rows, "\n")

	boxWidth := 30
	if boxWidth > m.width-4 {
		boxWidth = m.width - 4
	}
	if boxWidth < 10 {
		boxWidth = 10
	}

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorFocusBdr).
		Padding(0, 1).
		Width(boxWidth).
		Render(content)

	dimmed := dimContent(background)
	// Position: row 1 (just below the panel title border), anchored at m.anchorX.
	return placeDropdown(dimmed, box, m.anchorX, 1, m.width, m.height)
}

// placeDropdown composites fg at a fixed (x, y) position over bg.
func placeDropdown(bg, fg string, x, y, totalWidth, totalHeight int) string {
	bgLines := strings.Split(bg, "\n")
	fgLines := strings.Split(fg, "\n")

	// Pad bg to fill height if needed.
	for len(bgLines) < totalHeight {
		bgLines = append(bgLines, strings.Repeat(" ", totalWidth))
	}

	for i, fgLine := range fgLines {
		row := y + i
		if row >= len(bgLines) {
			break
		}
		bgPlain := stripAnsi(bgLines[row])

		var b strings.Builder
		leftRunes := []rune(bgPlain)
		if x > 0 {
			if x <= len(leftRunes) {
				dim := lipgloss.NewStyle().Foreground(ColorDim)
				b.WriteString(dim.Render(string(leftRunes[:x])))
			} else {
				b.WriteString(strings.Repeat(" ", x))
			}
		}
		b.WriteString(fgLine)
		bgLines[row] = b.String()
	}

	return strings.Join(bgLines, "\n")
}
