package tui

import (
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/alphaleonis/cctote/internal/config"
)

// ConfigOverlayModel is the config editor overlay.
type ConfigOverlayModel struct {
	active     bool
	width      int
	height     int
	keys       KeyMap
	configPath string
	items      []configDisplayItem
	cursor     int
	editing    bool
	input      textinput.Model
	err        string // validation/save error
}

// configDisplayItem holds display data for a single config key.
type configDisplayItem struct {
	def   *config.KeyDef
	value string // current effective value
	isSet bool   // explicitly configured vs default
	dflt  string // default value
}

// NewConfigOverlay creates a new config overlay model.
func NewConfigOverlay(keys KeyMap) ConfigOverlayModel {
	ti := textinput.New()
	ti.CharLimit = 256
	return ConfigOverlayModel{keys: keys, input: ti}
}

// Activate opens the config overlay, loading current values from disk.
func (m *ConfigOverlayModel) Activate(configPath string) {
	m.active = true
	m.configPath = configPath
	m.cursor = 0
	m.editing = false
	m.err = ""
	m.input.Blur()
	m.reload()
}

// Deactivate closes the overlay.
func (m *ConfigOverlayModel) Deactivate() {
	m.active = false
	m.editing = false
	m.input.Blur()
}

// Active returns whether the overlay is showing.
func (m *ConfigOverlayModel) Active() bool {
	return m.active
}

// SetSize updates the available area.
func (m *ConfigOverlayModel) SetSize(width, height int) {
	m.width = width
	m.height = height
}

// Update handles key events for the config overlay.
func (m ConfigOverlayModel) Update(msg tea.Msg) (ConfigOverlayModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		if key.Matches(msg, m.keys.ForceQuit) {
			return m, tea.Quit
		}

		if m.editing {
			return m.updateEditing(msg)
		}
		return m.updateNavigating(msg)
	}

	// Forward non-key messages to text input when editing.
	if m.editing {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m ConfigOverlayModel) updateNavigating(msg tea.KeyPressMsg) (ConfigOverlayModel, tea.Cmd) {
	switch {
	case key.Matches(msg, key.NewBinding(key.WithKeys("esc"))):
		m.Deactivate()
		return m, nil

	case key.Matches(msg, m.keys.Up):
		if m.cursor > 0 {
			m.cursor--
			m.err = ""
		}

	case key.Matches(msg, m.keys.Down):
		if m.cursor < len(m.items)-1 {
			m.cursor++
			m.err = ""
		}

	case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
		if len(m.items) == 0 {
			return m, nil
		}
		item := &m.items[m.cursor]
		if item.def.Type == config.KeyTypeBool {
			return m.toggleBool()
		}
		if item.def.Type == config.KeyTypeEnum {
			return m.cycleEnum()
		}
		// String: enter edit mode.
		m.editing = true
		m.err = ""
		m.input.Reset()
		if item.isSet {
			m.input.SetValue(item.value)
		}
		m.input.Focus()

	case key.Matches(msg, m.keys.Delete):
		if len(m.items) == 0 {
			return m, nil
		}
		return m.resetKey()
	}

	return m, nil
}

func (m ConfigOverlayModel) updateEditing(msg tea.KeyPressMsg) (ConfigOverlayModel, tea.Cmd) {
	switch {
	case key.Matches(msg, key.NewBinding(key.WithKeys("esc"))):
		// Cancel edit.
		m.editing = false
		m.err = ""
		m.input.Blur()
		return m, nil

	case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
		// Confirm edit.
		return m.confirmEdit()
	}

	// Forward to text input.
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	m.err = "" // clear error on typing
	return m, cmd
}

// toggleBool toggles the current bool key and saves.
func (m ConfigOverlayModel) toggleBool() (ConfigOverlayModel, tea.Cmd) {
	item := &m.items[m.cursor]
	newVal := "true"
	if item.value == "true" {
		newVal = "false"
	}
	name := item.def.Name // capture before reload invalidates item

	if err := m.saveKey(item.def, newVal); err != nil {
		m.err = err.Error()
		return m, nil
	}

	m.reload()
	if m.err != "" {
		return m, nil
	}
	return m, func() tea.Msg {
		return ConfigSavedMsg{Key: name, Value: newVal}
	}
}

// cycleEnum cycles the current enum key to the next option and saves.
func (m ConfigOverlayModel) cycleEnum() (ConfigOverlayModel, tea.Cmd) {
	item := &m.items[m.cursor]
	opts := item.def.Options
	if len(opts) == 0 {
		return m, nil
	}

	// Find current index and advance to next.
	cur := 0
	for i, o := range opts {
		if o == item.value {
			cur = i
			break
		}
	}
	newVal := opts[(cur+1)%len(opts)]
	name := item.def.Name

	if err := m.saveKey(item.def, newVal); err != nil {
		m.err = err.Error()
		return m, nil
	}

	m.reload()
	if m.err != "" {
		return m, nil
	}
	return m, func() tea.Msg {
		return ConfigSavedMsg{Key: name, Value: newVal}
	}
}

// resetKey resets the current key to its default and saves.
func (m ConfigOverlayModel) resetKey() (ConfigOverlayModel, tea.Cmd) {
	item := &m.items[m.cursor]
	if !item.isSet {
		// Already at default — nothing to do.
		return m, nil
	}

	c, err := config.Load(m.configPath)
	if err != nil {
		m.err = err.Error()
		return m, nil
	}

	item.def.Reset(c)

	if err := config.Save(m.configPath, c); err != nil {
		m.err = err.Error()
		return m, nil
	}

	name := item.def.Name
	m.reload()
	if m.err != "" {
		return m, nil
	}
	return m, func() tea.Msg {
		return ConfigSavedMsg{Key: name, Value: "(default)"}
	}
}

// confirmEdit validates and saves the edited string value.
func (m ConfigOverlayModel) confirmEdit() (ConfigOverlayModel, tea.Cmd) {
	item := &m.items[m.cursor]
	value := strings.TrimSpace(m.input.Value())

	if err := m.saveKey(item.def, value); err != nil {
		m.err = err.Error()
		return m, nil
	}

	m.editing = false
	m.input.Blur()
	name := item.def.Name
	m.reload()
	if m.err != "" {
		return m, nil
	}
	return m, func() tea.Msg {
		return ConfigSavedMsg{Key: name, Value: value}
	}
}

// saveKey and reload use synchronous I/O rather than bubbletea's Cmd pattern.
// Config files are small TOML (~100 bytes), so latency is negligible. Synchronous
// access ensures reload() always sees the just-saved state without race conditions.

// saveKey loads config from disk, applies the set, and saves.
func (m *ConfigOverlayModel) saveKey(kd *config.KeyDef, value string) error {
	c, err := config.Load(m.configPath)
	if err != nil {
		return err
	}
	if err := kd.Set(c, value); err != nil {
		return err
	}
	return config.Save(m.configPath, c)
}

// reload re-reads config from disk and refreshes the display items.
func (m *ConfigOverlayModel) reload() {
	c, err := config.Load(m.configPath)
	if err != nil {
		m.items = nil
		m.err = err.Error()
		return
	}

	allKeys := config.Keys()
	m.items = make([]configDisplayItem, len(allKeys))
	for i := range allKeys {
		k := &allKeys[i]
		m.items[i] = configDisplayItem{
			def:   k,
			value: k.Get(c),
			isSet: k.IsSet(c),
			dflt:  k.Default(),
		}
	}
}

// ViewOver renders the config overlay centered over a dimmed background.
func (m ConfigOverlayModel) ViewOver(background string) string {
	if !m.active {
		return background
	}

	var lines []string
	lines = append(lines, StyleBold.Render(IconConfig+" Config"))
	lines = append(lines, "")

	// Render each config item as a row.
	for i, item := range m.items {
		lines = append(lines, m.renderItem(i, item))
	}

	// Validation/error message.
	if m.err != "" {
		lines = append(lines, "")
		lines = append(lines, StyleFlashError.Render("  "+m.err))
	}

	lines = append(lines, "")
	lines = append(lines, StyleHint.Render("  j/k: navigate  Enter: edit  x: reset  Esc: close"))

	content := strings.Join(lines, "\n")

	boxWidth := 60
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

func (m ConfigOverlayModel) renderItem(idx int, item configDisplayItem) string {
	isCursor := idx == m.cursor

	// When editing the current row, show text input inline (no selection style).
	if m.editing && isCursor {
		name := "  " + item.def.Name
		namePad := strings.Repeat(" ", max(0, 22-len(name)))
		return name + namePad + "  " + m.input.View()
	}

	// Build plain-text columns for correct alignment, then style the full row.
	name := "  " + item.def.Name
	namePad := strings.Repeat(" ", max(0, 22-len(name)))

	// Value: show effective value or dim placeholder.
	var value, source string
	valueDim := false
	if !item.isSet {
		value = "(default)"
		valueDim = true
	} else {
		value = item.value
	}
	valuePad := strings.Repeat(" ", max(0, 20-len(value)))

	if item.isSet {
		source = "set"
	} else {
		source = "default"
	}

	plainRow := name + namePad + "  " + value + valuePad + "  " + source

	// Apply styles to the entire row to avoid nested ANSI clipping.
	if isCursor {
		return StyleItemSelected.Render(plainRow)
	}

	// Style individual columns for non-cursor rows.
	styledValue := value
	if valueDim {
		styledValue = StyleItemDim.Render(value)
	}
	var styledSource string
	if item.isSet {
		styledSource = StyleStatusDiff.Render(source)
	} else {
		styledSource = StyleItemDim.Render(source)
	}

	return name + namePad + "  " + styledValue + valuePad + "  " + styledSource
}
