package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"

	"github.com/alphaleonis/cctote/internal/engine"
)

// progressState tracks the lifecycle of the progress overlay.
type progressState int

const (
	progressRunning   progressState = iota
	progressCompleted               // finished successfully
	progressErrored                 // finished with error
	progressCancelled               // cancelled by user
)

// progressEntry is a single operation in the progress log.
type progressEntry struct {
	section engine.SectionKind
	name    string
	action  engine.ActionKind
	done    bool
	err     error
}

// ProgressModel is the progress overlay for bulk apply operations.
type ProgressModel struct {
	active   bool
	state    progressState
	title    string
	total    int
	cancelFn context.CancelFunc
	entries  []progressEntry
	spinner  spinner.Model
	err      error // final error (for errored state)
	warnings []string

	// Scrolling
	scrollOffset int  // first visible entry index
	autoScroll   bool // auto-scroll to bottom
	entrySlots   int  // fixed number of visible entry lines (set at activation)

	width, height int
	keys          KeyMap
}

// NewProgress creates a new progress model.
func NewProgress(keys KeyMap) ProgressModel {
	s := spinner.New(spinner.WithSpinner(spinner.Dot))
	s.Style = StyleHint
	return ProgressModel{keys: keys, spinner: s, autoScroll: true}
}

// Active returns whether the overlay is showing.
func (m *ProgressModel) Active() bool {
	return m.active
}

// progressOverhead is the *minimum* number of lines consumed by non-entry
// chrome in ViewOver. ViewOver may render additional lines for errors,
// warnings, or scroll indicators, so entries may get fewer lines than
// (height - progressOverhead). This constant sizes the entry viewport for
// the common (no-error) case.
// Breakdown: border(2) + padding(2) + title(1) + blank(1) + counter(1) + blank(1) + button(1) + blank(1) = 10
const progressOverhead = 10

// Activate shows the progress overlay.
func (m *ProgressModel) Activate(title string, total int, cancelFn context.CancelFunc) {
	m.active = true
	m.state = progressRunning
	m.title = title
	m.total = total
	m.cancelFn = cancelFn
	m.entries = nil
	m.err = nil
	m.warnings = nil
	m.scrollOffset = 0
	m.autoScroll = true

	// Fix entry area height at activation: sized for total operations,
	// capped so the dialog fits on screen. Minimum 3 lines.
	slots := total
	maxSlots := m.height - progressOverhead
	if maxSlots < 3 {
		maxSlots = 3
	}
	if slots > maxSlots {
		slots = maxSlots
	}
	if slots < 3 {
		slots = 3
	}
	m.entrySlots = slots
}

// Deactivate hides the overlay.
func (m *ProgressModel) Deactivate() {
	m.active = false
	m.cancelFn = nil
	m.entries = nil
}

// SetSize updates the available area.
func (m *ProgressModel) SetSize(width, height int) {
	m.width = width
	m.height = height
}

// HandleUpdate processes a ProgressUpdateMsg.
func (m *ProgressModel) HandleUpdate(msg ProgressUpdateMsg) {
	if msg.Done {
		// Find the matching in-progress entry and mark it done.
		for i := len(m.entries) - 1; i >= 0; i-- {
			e := &m.entries[i]
			if e.section == msg.Section && e.name == msg.Name && e.action == msg.Action && !e.done {
				e.done = true
				e.err = msg.Err
				return
			}
		}
		// No matching start found — add as a completed entry.
		m.entries = append(m.entries, progressEntry{
			section: msg.Section, name: msg.Name, action: msg.Action,
			done: true, err: msg.Err,
		})
	} else {
		m.entries = append(m.entries, progressEntry{
			section: msg.Section, name: msg.Name, action: msg.Action,
		})
	}
	if m.autoScroll {
		m.scrollToBottom()
	}
}

// HandleFinished transitions the overlay to a completed/errored/cancelled state.
func (m *ProgressModel) HandleFinished(msg ProgressFinishedMsg) {
	m.warnings = msg.Warnings
	if msg.Err != nil {
		if errors.Is(msg.Err, context.Canceled) {
			m.state = progressCancelled
		} else {
			m.state = progressErrored
			m.err = msg.Err
		}
	} else {
		m.state = progressCompleted
	}
}

// tickCmd returns the spinner tick command.
func (m *ProgressModel) tickCmd() tea.Cmd {
	return m.spinner.Tick
}

// Update handles key events and spinner ticks.
func (m ProgressModel) Update(msg tea.Msg) (ProgressModel, tea.Cmd) {
	switch msg := msg.(type) {
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, m.keys.ForceQuit):
			return m, tea.Quit
		case key.Matches(msg, m.keys.Up):
			if m.scrollOffset > 0 {
				m.scrollOffset--
				m.autoScroll = false
			}
			return m, nil
		case key.Matches(msg, m.keys.Down):
			m.scrollOffset++
			maxOff := m.maxScrollOffset()
			if m.scrollOffset >= maxOff {
				m.scrollOffset = maxOff
				m.autoScroll = true
			}
			return m, nil
		case key.Matches(msg, m.keys.Confirm), key.Matches(msg, m.keys.Cancel):
			if m.state == progressRunning {
				// Cancel the operation.
				if m.cancelFn != nil {
					m.cancelFn()
				}
			} else {
				// Dismiss the overlay.
				return m, func() tea.Msg { return ProgressDismissedMsg{} }
			}
			return m, nil
		}
	}
	return m, nil
}

// visibleEntries returns the number of entries visible in the scroll area.
// Uses the fixed entrySlots set at activation time.
func (m *ProgressModel) visibleEntries() int {
	return m.entrySlots
}

// maxScrollOffset returns the maximum valid scroll offset.
func (m *ProgressModel) maxScrollOffset() int {
	max := len(m.entries) - m.visibleEntries()
	if max < 0 {
		return 0
	}
	return max
}

// scrollToBottom moves scroll to show the latest entries.
func (m *ProgressModel) scrollToBottom() {
	m.scrollOffset = m.maxScrollOffset()
}

// ViewOver renders the progress dialog centered over a dimmed background.
func (m ProgressModel) ViewOver(background string) string {
	if !m.active {
		return background
	}

	boxWidth := m.width * 3 / 5 // 60% of terminal width
	if boxWidth < 45 {
		boxWidth = 45
	}
	if boxWidth > m.width-4 {
		boxWidth = m.width - 4
	}
	innerWidth := boxWidth - 4 // padding + border

	var lines []string

	// Title
	switch m.state {
	case progressRunning:
		lines = append(lines, StyleBold.Render(m.title))
	case progressCompleted:
		lines = append(lines, StyleBold.Foreground(ColorGreen).Render("Applied successfully"))
	case progressErrored:
		lines = append(lines, StyleBold.Foreground(ColorRed).Render("Apply failed"))
	case progressCancelled:
		lines = append(lines, StyleBold.Foreground(ColorYellow).Render("Apply cancelled (partial)"))
	}
	lines = append(lines, "")

	// Counter
	doneCount := 0
	errorCount := 0
	for _, e := range m.entries {
		if e.done {
			if e.err != nil {
				errorCount++
			} else {
				doneCount++
			}
		}
	}
	counterText := fmt.Sprintf("(%d/%d operations", doneCount, m.total)
	if errorCount > 0 {
		counterText += fmt.Sprintf(", %d error(s)", errorCount)
	}
	counterText += ")"
	lines = append(lines, StyleHint.Render(counterText))

	// Entry log (scrollable)
	visible := m.visibleEntries()
	start := m.scrollOffset
	if start > len(m.entries) {
		start = len(m.entries)
	}
	end := start + visible
	if end > len(m.entries) {
		end = len(m.entries)
	}

	rendered := 0
	for i := start; i < end; i++ {
		e := m.entries[i]
		icon := sectionIcon(e.section)
		var status string
		if e.done {
			if e.err != nil {
				status = lipgloss.NewStyle().Foreground(ColorRed).Render("✗")
			} else {
				status = lipgloss.NewStyle().Foreground(ColorGreen).Render("✓")
			}
		} else {
			status = m.spinner.View()
		}
		actionLabel := actionVerb(e.action)
		// Truncate the name (plain text) before assembling with styled
		// components. Using len([]rune) on the assembled line would count
		// ANSI escape codes and truncate far too early.
		prefix := fmt.Sprintf("  %s %s %s ", status, actionLabel, icon)
		prefixWidth := lipgloss.Width(prefix)
		nameWidth := innerWidth - prefixWidth
		name := e.name
		if nameWidth > 0 && len([]rune(name)) > nameWidth {
			name = string([]rune(name)[:nameWidth-1]) + "…"
		}
		line := prefix + name
		lines = append(lines, line)
		rendered++
	}
	// Pad with empty lines to maintain fixed height.
	for rendered < visible {
		lines = append(lines, "")
		rendered++
	}

	// Scroll indicator
	if len(m.entries) > visible {
		if start > 0 {
			lines[len(lines)-visible] = StyleHint.Render("  ↑ more") // replace first visible
		}
	}

	lines = append(lines, "")

	// Error details (errored state only)
	if m.state == progressErrored && m.err != nil {
		wrapped := wrapText(m.err.Error(), innerWidth)
		for _, w := range wrapped {
			lines = append(lines, lipgloss.NewStyle().Foreground(ColorRed).Render(w))
		}
		lines = append(lines, "")
	}

	// Warnings
	if len(m.warnings) > 0 && m.state != progressRunning {
		for _, w := range m.warnings {
			lines = append(lines, StyleHint.Render("⚠ "+w))
		}
		lines = append(lines, "")
	}

	// Button
	var btn string
	if m.state == progressRunning {
		btn = StyleButtonDanger.Render("Cancel")
	} else {
		btn = StyleButtonFocused.Render("Close")
	}
	btnWidth := lipgloss.Width(btn)
	pad := (innerWidth - btnWidth) / 2
	if pad < 0 {
		pad = 0
	}
	lines = append(lines, strings.Repeat(" ", pad)+btn)

	content := strings.Join(lines, "\n")

	borderColor := ColorFocusBdr
	switch m.state {
	case progressErrored:
		borderColor = ColorRed
	case progressCancelled:
		borderColor = ColorYellow
	}

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Padding(1, 1).
		Width(boxWidth).
		Render(content)

	dimmed := dimContent(background)
	return placeOverlay(dimmed, box, m.width, m.height)
}

// sectionIcon returns the icon for a section kind.
func sectionIcon(s engine.SectionKind) string {
	switch s {
	case engine.SectionMCP:
		return IconMCP
	case engine.SectionPlugin:
		return IconPlugin
	case engine.SectionMarketplace:
		return IconMarketplace
	default:
		return "?"
	}
}

// actionVerb returns a human-readable verb for an action kind.
// Verbs are padded to 7 chars for column alignment in the progress log.
func actionVerb(a engine.ActionKind) string {
	switch a {
	case engine.ActionAdded:
		return "install"
	case engine.ActionRemoved:
		return "remove "
	case engine.ActionUpdated:
		return "update "
	default:
		return string(a)
	}
}
