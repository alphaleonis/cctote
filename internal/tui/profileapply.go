package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

// BulkApplyPlan holds the pre-computed plan data for a bulk apply overlay.
// Named fields prevent parameter transposition (6 []string slices are easy to mix up).
type BulkApplyPlan struct {
	Source, Target            PaneSource
	AddMCP, AddPlugins        []string
	ConflictMCP, ConflictPlug []string
	RemoveMCP, RemovePlugins  []string
	ShowStrict                bool // whether to show the strict checkbox
	McpCreation               bool // whether applying will create .mcp.json
}

// BulkApplyModel is the bulk apply confirmation overlay.
type BulkApplyModel struct {
	active        bool
	plan          BulkApplyPlan
	sourceLabel   string
	targetLabel   string
	strict        bool // strict mode checkbox
	focusIdx      int  // 0=checkbox (or cancel when !showStrict), 1=cancel (or apply when !showStrict), 2=apply
	width, height int
	keys          KeyMap
}

// NewBulkApply creates a new bulk apply model.
func NewBulkApply(keys KeyMap) BulkApplyModel {
	return BulkApplyModel{keys: keys}
}

// Activate opens the apply overlay with pre-computed plan data.
func (m *BulkApplyModel) Activate(plan BulkApplyPlan) {
	m.active = true
	m.plan = plan
	m.sourceLabel = plan.Source.Label()
	m.targetLabel = plan.Target.Label()
	m.strict = false
	if plan.ShowStrict {
		m.focusIdx = 2 // default to Apply
	} else {
		m.focusIdx = 1 // default to Apply (cancel=0, apply=1)
	}
}

// Deactivate hides the overlay.
func (m *BulkApplyModel) Deactivate() {
	m.active = false
}

// Active returns whether the overlay is showing.
func (m *BulkApplyModel) Active() bool {
	return m.active
}

// SetSize updates the available area.
func (m *BulkApplyModel) SetSize(width, height int) {
	m.width = width
	m.height = height
}

// itemCount returns the number of focusable items (checkbox+buttons or just buttons).
func (m *BulkApplyModel) itemCount() int {
	if m.plan.ShowStrict {
		return 3 // checkbox, cancel, apply
	}
	return 2 // cancel, apply
}

// Update handles key events.
func (m BulkApplyModel) Update(msg tea.Msg) (BulkApplyModel, tea.Cmd) {
	count := m.itemCount()
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, m.keys.Cancel):
			m.active = false
		case key.Matches(msg, m.keys.ForceQuit):
			return m, tea.Quit
		case key.Matches(msg, m.keys.Tab):
			m.focusIdx = (m.focusIdx + 1) % count
		case key.Matches(msg, m.keys.Down):
			if m.plan.ShowStrict && m.focusIdx == 0 {
				// Checkbox → Apply (skip Cancel, Apply is the default action).
				_, applyIdx := m.buttonIndices()
				m.focusIdx = applyIdx
			} else {
				m.focusIdx = (m.focusIdx + 1) % count
			}
		case key.Matches(msg, m.keys.Up):
			if m.plan.ShowStrict && m.focusIdx != 0 {
				// Either button → checkbox (one visual row up).
				m.focusIdx = 0
			} else {
				m.focusIdx = (m.focusIdx + count - 1) % count
			}
		case key.Matches(msg, m.keys.Left), key.Matches(msg, m.keys.Right):
			// Left/Right toggle between the two buttons (skip checkbox).
			cancelIdx, applyIdx := m.buttonIndices()
			switch m.focusIdx {
			case cancelIdx:
				m.focusIdx = applyIdx
			case applyIdx:
				m.focusIdx = cancelIdx
			}
		case key.Matches(msg, key.NewBinding(key.WithKeys("space"))):
			if m.plan.ShowStrict && m.focusIdx == 0 {
				m.strict = !m.strict
			}
		case key.Matches(msg, m.keys.Confirm):
			cancelIdx, applyIdx := m.buttonIndices()
			switch m.focusIdx {
			case 0:
				if m.plan.ShowStrict {
					// Checkbox
					m.strict = !m.strict
					m.focusIdx = applyIdx // advance to Apply so next Enter confirms
				} else {
					// Cancel button
					m.active = false
				}
			case cancelIdx:
				m.active = false
			case applyIdx:
				strict := m.strict
				src := m.plan.Source
				tgt := m.plan.Target
				m.active = false
				return m, func() tea.Msg {
					return BulkApplyMsg{Source: src, Target: tgt, Strict: strict}
				}
			}
		}
	}
	return m, nil
}

// buttonIndices returns the focusIdx values for cancel and apply buttons.
func (m *BulkApplyModel) buttonIndices() (cancelIdx, applyIdx int) {
	if m.plan.ShowStrict {
		return 1, 2
	}
	return 0, 1
}

// ViewOver renders the apply overlay centered over a dimmed background.
func (m BulkApplyModel) ViewOver(background string) string {
	if !m.active {
		return background
	}

	var lines []string
	lines = append(lines, StyleBold.Render(fmt.Sprintf("Apply %s → %s", m.sourceLabel, m.targetLabel)))
	lines = append(lines, "")

	if m.plan.McpCreation {
		lines = append(lines, StyleHint.Render("This will create .mcp.json in the project root."))
		lines = append(lines, "")
	}

	hasChanges := false

	// Additions.
	if len(m.plan.AddMCP) > 0 || len(m.plan.AddPlugins) > 0 {
		hasChanges = true
		lines = append(lines, StyleSectionHeader.Render("Import:"))
		for _, name := range m.plan.AddMCP {
			lines = append(lines, StyleDiffAdd.Render(fmt.Sprintf("  + %s %s", IconMCP, name)))
		}
		for _, name := range m.plan.AddPlugins {
			lines = append(lines, StyleDiffAdd.Render(fmt.Sprintf("  + %s %s", IconPlugin, name)))
		}
	}

	// Conflicts (overwrite).
	if len(m.plan.ConflictMCP) > 0 || len(m.plan.ConflictPlug) > 0 {
		hasChanges = true
		lines = append(lines, StyleSectionHeader.Render("Overwrite:"))
		for _, name := range m.plan.ConflictMCP {
			lines = append(lines, StyleStatusDiff.Render(fmt.Sprintf("  ~ %s %s", IconMCP, name)))
		}
		for _, name := range m.plan.ConflictPlug {
			lines = append(lines, StyleStatusDiff.Render(fmt.Sprintf("  ~ %s %s", IconPlugin, name)))
		}
	}

	// Strict removals.
	if m.strict && (len(m.plan.RemoveMCP) > 0 || len(m.plan.RemovePlugins) > 0) {
		lines = append(lines, StyleSectionHeader.Render("Remove (strict):"))
		for _, name := range m.plan.RemoveMCP {
			lines = append(lines, StyleDiffRemove.Render(fmt.Sprintf("  - %s %s", IconMCP, name)))
		}
		for _, name := range m.plan.RemovePlugins {
			lines = append(lines, StyleDiffRemove.Render(fmt.Sprintf("  - %s %s", IconPlugin, name)))
		}
	}

	if !hasChanges && (!m.strict || (len(m.plan.RemoveMCP) == 0 && len(m.plan.RemovePlugins) == 0)) {
		lines = append(lines, StyleHint.Render("  No changes needed"))
	}

	lines = append(lines, "")

	// Strict checkbox (only when showStrict is true).
	if m.plan.ShowStrict {
		checkIcon := "[ ]"
		if m.strict {
			checkIcon = "[x]"
		}
		checkStyle := StyleItemNormal
		if m.focusIdx == 0 {
			checkStyle = StyleItemSelected
		}
		lines = append(lines, checkStyle.Render(fmt.Sprintf(" %s Strict mode (remove extras)", checkIcon)))
		lines = append(lines, "")
	}

	// Buttons.
	lines = append(lines, m.renderButtons())

	content := strings.Join(lines, "\n")

	boxWidth := 55
	if boxWidth > m.width-4 {
		boxWidth = m.width - 4
	}

	box := StyleConfirmBorder.
		Padding(1, 1).
		Width(boxWidth).
		Render(content)

	dimmed := dimContent(background)
	return placeOverlay(dimmed, box, m.width, m.height)
}

func (m BulkApplyModel) renderButtons() string {
	cancelIdx, applyIdx := m.buttonIndices()
	var cancelBtn, applyBtn string

	if m.focusIdx == cancelIdx {
		cancelBtn = StyleButtonFocused.Render("Cancel")
	} else {
		cancelBtn = StyleButtonNormal.Render("Cancel")
	}

	if m.focusIdx == applyIdx {
		applyBtn = StyleButtonFocused.Render("Apply")
	} else {
		applyBtn = StyleButtonNormal.Render("Apply")
	}

	return "  " + cancelBtn + "  " + applyBtn
}
