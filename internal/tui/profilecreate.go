package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
)

// ProfileCreateModel is the profile creation overlay with a text input.
type ProfileCreateModel struct {
	active        bool
	input         textinput.Model
	summary       string   // e.g. "Will snapshot: 3 MCP, 2 plugins"
	existingNames []string // for duplicate validation
	err           string   // validation error
	focusIdx      int      // 0=input, 1=cancel, 2=create
	width, height int
	keys          KeyMap
}

// NewProfileCreate creates a new profile create model.
func NewProfileCreate(keys KeyMap) ProfileCreateModel {
	ti := textinput.New()
	ti.Prompt = ""
	ti.Placeholder = "profile-name"
	ti.CharLimit = 40
	ti.SetWidth(30)
	ti.SetStyles(profileInputStyles())
	return ProfileCreateModel{keys: keys, input: ti}
}

// Activate opens the create overlay.
func (m *ProfileCreateModel) Activate(summary string, existingNames []string) {
	m.active = true
	m.summary = summary
	m.existingNames = existingNames
	m.err = ""
	m.focusIdx = 0
	m.input.Reset()
	m.input.Focus()
}

// Deactivate hides the overlay.
func (m *ProfileCreateModel) Deactivate() {
	m.active = false
	m.input.Blur()
}

// Active returns whether the overlay is showing.
func (m *ProfileCreateModel) Active() bool {
	return m.active
}

// SetSize updates the available area.
func (m *ProfileCreateModel) SetSize(width, height int) {
	m.width = width
	m.height = height
}

// Update handles key events.
func (m ProfileCreateModel) Update(msg tea.Msg) (ProfileCreateModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, m.keys.ForceQuit):
			return m, tea.Quit

		// Esc always cancels (we can't use m.keys.Cancel which includes "n"
		// because "n" must reach the text input when focused).
		case key.Matches(msg, key.NewBinding(key.WithKeys("esc"))):
			m.active = false
			m.input.Blur()
			return m, nil

		case key.Matches(msg, m.keys.Tab):
			m.focusIdx = (m.focusIdx + 1) % 3
			if m.focusIdx == 0 {
				m.input.Focus()
			} else {
				m.input.Blur()
			}
			return m, nil

		case key.Matches(msg, key.NewBinding(key.WithKeys("shift+tab"))):
			m.focusIdx = (m.focusIdx + 2) % 3
			if m.focusIdx == 0 {
				m.input.Focus()
			} else {
				m.input.Blur()
			}
			return m, nil

		// Enter always handled (we can't use m.keys.Confirm which includes
		// "y" because "y" must reach the text input when focused).
		case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
			switch m.focusIdx {
			case 0:
				// Enter in input field — move to Create button.
				m.focusIdx = 2
				m.input.Blur()
				return m, nil
			case 1:
				m.active = false
				m.input.Blur()
				return m, nil
			case 2:
				return m.tryCreate()
			}

		// When on buttons, support y/n shortcuts for confirm/cancel.
		case m.focusIdx != 0 && key.Matches(msg, m.keys.Cancel):
			m.active = false
			m.input.Blur()
			return m, nil
		case m.focusIdx == 2 && key.Matches(msg, key.NewBinding(key.WithKeys("y"))):
			return m.tryCreate()
		}

		// If input is focused, forward all other keys to text input.
		if m.focusIdx == 0 {
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			m.err = "" // clear validation on typing
			return m, cmd
		}
	}
	return m, nil
}

func (m ProfileCreateModel) tryCreate() (ProfileCreateModel, tea.Cmd) {
	name := strings.TrimSpace(m.input.Value())
	if name == "" {
		m.err = "Name cannot be empty"
		return m, nil
	}
	for _, existing := range m.existingNames {
		if existing == name {
			m.err = fmt.Sprintf("Profile %q already exists", name)
			return m, nil
		}
	}
	m.active = false
	m.input.Blur()
	return m, func() tea.Msg { return ProfileCreateMsg{Name: name} }
}

// ViewOver renders the create overlay centered over a dimmed background.
func (m ProfileCreateModel) ViewOver(background string) string {
	if !m.active {
		return background
	}

	var lines []string
	lines = append(lines, StyleBold.Render("Create Profile"))
	lines = append(lines, "")
	lines = append(lines, StyleHint.Render(m.summary))
	lines = append(lines, "")

	// Input field.
	labelStyle := StyleItemNormal
	if m.focusIdx == 0 {
		labelStyle = StyleBold.Foreground(ColorBlue)
	}
	lines = append(lines, labelStyle.Render("Name: ")+m.input.View())

	// Validation error.
	if m.err != "" {
		lines = append(lines, StyleFlashError.Render("  "+m.err))
	}
	lines = append(lines, "")

	// Buttons.
	lines = append(lines, m.renderButtons())

	content := strings.Join(lines, "\n")

	boxWidth := 45
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

func (m ProfileCreateModel) renderButtons() string {
	var cancelBtn, createBtn string

	if m.focusIdx == 1 {
		cancelBtn = StyleButtonFocused.Render("Cancel")
	} else {
		cancelBtn = StyleButtonNormal.Render("Cancel")
	}

	if m.focusIdx == 2 {
		createBtn = StyleButtonFocused.Render("Create")
	} else {
		createBtn = StyleButtonNormal.Render("Create")
	}

	return "  " + cancelBtn + "  " + createBtn
}

// profileInputStyles returns textinput styles matching the TUI theme.
func profileInputStyles() textinput.Styles {
	inputBG := lipgloss.Color("#2a2d2e")
	base := textinput.StyleState{
		Text:        lipgloss.NewStyle().Foreground(ColorFG).Background(inputBG),
		Placeholder: lipgloss.NewStyle().Foreground(ColorDim).Background(inputBG),
		Prompt:      lipgloss.NewStyle(),
		Suggestion:  lipgloss.NewStyle().Foreground(ColorDim),
	}
	return textinput.Styles{
		Focused: base,
		Blurred: base,
	}
}
