package tui

import (
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
)

// AlertDismissedMsg is sent when the user dismisses the alert overlay.
type AlertDismissedMsg struct{}

// AlertModel is a simple error dialog overlay with just an OK button.
type AlertModel struct {
	active bool
	title  string
	msg    string // original message, re-wrapped on each render
	width  int
	height int
	keys   KeyMap
}

// NewAlert creates a new alert model.
func NewAlert(keys KeyMap) AlertModel {
	return AlertModel{keys: keys}
}

// Activate shows the alert overlay with the given title and message.
// The message is word-wrapped to fit the dialog width.
func (m *AlertModel) Activate(title, msg string) {
	m.active = true
	m.title = title
	m.msg = msg
}

// Deactivate hides the alert overlay.
func (m *AlertModel) Deactivate() {
	m.active = false
	m.title = ""
	m.msg = ""
}

// Active returns whether the alert overlay is showing.
func (m *AlertModel) Active() bool {
	return m.active
}

// SetSize updates the available area for the overlay.
func (m *AlertModel) SetSize(width, height int) {
	m.width = width
	m.height = height
}

// Update handles key events for the alert overlay.
func (m AlertModel) Update(msg tea.Msg) (AlertModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, m.keys.Confirm), key.Matches(msg, m.keys.Cancel):
			return m, func() tea.Msg { return AlertDismissedMsg{} }
		case key.Matches(msg, m.keys.ForceQuit):
			return m, tea.Quit
		}
	}
	return m, nil
}

// ViewOver renders the alert dialog centered over a dimmed background.
func (m AlertModel) ViewOver(background string) string {
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

	// Re-wrap from original message to preserve paragraph breaks.
	wrapped := wrapText(m.msg, innerWidth)
	lines = append(lines, wrapped...)
	lines = append(lines, "")

	// OK button (always focused)
	okBtn := StyleButtonFocused.Render("OK")
	btnWidth := lipgloss.Width(okBtn)
	pad := (innerWidth - btnWidth) / 2
	if pad < 0 {
		pad = 0
	}
	lines = append(lines, strings.Repeat(" ", pad)+okBtn)

	content := strings.Join(lines, "\n")

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorRed).
		Padding(1, 1).
		Width(boxWidth).
		Render(content)

	dimmed := dimContent(background)
	return placeOverlay(dimmed, box, m.width, m.height)
}

// wrapText word-wraps text to the given width. It preserves existing
// newlines in the input so multi-line error messages render correctly.
func wrapText(text string, width int) []string {
	if width <= 0 {
		return []string{text}
	}
	var result []string
	for _, paragraph := range strings.Split(text, "\n") {
		if paragraph == "" {
			result = append(result, "")
			continue
		}
		words := strings.Fields(paragraph)
		if len(words) == 0 {
			result = append(result, "")
			continue
		}
		line := words[0]
		for _, w := range words[1:] {
			if len(line)+1+len(w) > width {
				result = append(result, line)
				line = w
			} else {
				line += " " + w
			}
		}
		result = append(result, line)
	}
	return result
}
