package tui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
)

// StatusModel is the status line model.
type StatusModel struct {
	synced     int
	different  int
	leftOnly   int
	rightOnly  int
	leftLabel  string // left pane source label
	rightLabel string // right pane source label
	width      int
	message    string // transient override
	spinner    spinner.Model
	loading    bool

	flash      string
	flashStyle FlashStyle
	flashID    int
}

// NewStatus creates a new status model.
func NewStatus() StatusModel {
	s := spinner.New(spinner.WithSpinner(spinner.Dot))
	s.Style = StyleHint
	return StatusModel{spinner: s}
}

// SetCounts updates the sync summary counts.
func (m *StatusModel) SetCounts(synced, different, leftOnly, rightOnly int) {
	m.synced = synced
	m.different = different
	m.leftOnly = leftOnly
	m.rightOnly = rightOnly
}

// SetSources sets the source labels for display.
func (m *StatusModel) SetSources(left, right string) {
	m.leftLabel = left
	m.rightLabel = right
}

// SetWidth updates the status line width.
func (m *StatusModel) SetWidth(width int) {
	m.width = width
}

// SetLoading sets the loading state with a transient message.
func (m *StatusModel) SetLoading(loading bool, message string) {
	m.loading = loading
	m.message = message
}

// SetFlash sets a flash message and returns a tea.Cmd that will expire it.
func (m *StatusModel) SetFlash(text string, style FlashStyle) tea.Cmd {
	m.flashID++
	m.flash = text
	m.flashStyle = style
	id := m.flashID
	return tea.Tick(3*time.Second, func(_ time.Time) tea.Msg {
		return FlashExpiredMsg{ID: id}
	})
}

// ClearFlash clears the flash only if the ID matches the current one.
func (m *StatusModel) ClearFlash(id int) {
	if id == m.flashID {
		m.flash = ""
	}
}

// Update handles spinner ticks.
func (m StatusModel) Update(msg tea.Msg) (StatusModel, tea.Cmd) {
	if m.loading {
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}
	return m, nil
}

// Tick returns the spinner tick message (call from Init).
func (m StatusModel) Tick() tea.Msg {
	return m.spinner.Tick()
}

// View renders the status line.
func (m StatusModel) View() string {
	if m.width < 10 {
		return ""
	}

	var left string
	if m.loading && m.message != "" {
		left = fmt.Sprintf(" %s %s", m.spinner.View(), m.message)
	} else if m.flash != "" {
		var style lipgloss.Style
		switch m.flashStyle {
		case FlashSuccess:
			style = StyleFlashSuccess
		case FlashError:
			style = StyleFlashError
		default:
			style = StyleFlashInfo
		}
		left = " " + style.Render(m.flash)
	} else {
		parts := []string{
			StyleStatusSynced.Render(fmt.Sprintf("%d synced", m.synced)),
			StyleStatusDiff.Render(fmt.Sprintf("%d different", m.different)),
			StyleStatusLeft.Render(fmt.Sprintf("%d left-only", m.leftOnly)),
			StyleStatusRight.Render(fmt.Sprintf("%d right-only", m.rightOnly)),
		}
		counts := strings.Join(parts, StyleItemDim.Render(" · "))
		sourceLabel := StyleBold.Render(m.leftLabel) +
			StyleItemDim.Render(" ↔ ") +
			StyleBold.Render(m.rightLabel)
		left = " " + sourceLabel + StyleItemDim.Render(": ") + counts
	}

	hints := StyleHint.Render("?:help ▴▾  R:refresh  q:quit")

	gap := m.width - lipgloss.Width(left) - lipgloss.Width(hints)
	if gap < 1 {
		return left
	}

	return left + strings.Repeat(" ", gap) + hints
}
