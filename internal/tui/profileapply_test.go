package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestBulkApplyModel_ShowStrict_FocusCycles3(t *testing.T) {
	keys := DefaultKeyMap()
	m := NewBulkApply(keys)
	m.Activate(BulkApplyPlan{
		Source:     PaneSource{Kind: SourceManifest},
		Target:     PaneSource{Kind: SourceClaudeCode},
		AddMCP:     []string{"srv"},
		RemoveMCP:  []string{"extra"},
		ShowStrict: true,
	})
	m.SetSize(80, 40)

	if m.itemCount() != 3 {
		t.Fatalf("itemCount = %d, want 3 (checkbox, cancel, apply)", m.itemCount())
	}

	// Default focus for showStrict is 2 (Apply).
	if m.focusIdx != 2 {
		t.Errorf("initial focusIdx = %d, want 2 (Apply)", m.focusIdx)
	}

	// Tab cycles: 2 -> 0 -> 1 -> 2.
	visited := make(map[int]bool)
	for i := 0; i < 3; i++ {
		visited[m.focusIdx] = true
		var updated BulkApplyModel
		updated, _ = m.Update(tabMsg())
		m = updated
	}
	if len(visited) != 3 {
		t.Errorf("visited %d indices, want 3 (should cycle through all)", len(visited))
	}
	// After 3 tabs we should be back to the start.
	if m.focusIdx != 2 {
		t.Errorf("after full cycle focusIdx = %d, want 2", m.focusIdx)
	}

	// Verify planned changes are rendered in the overlay.
	view := m.ViewOver("background content here")
	plain := stripAnsi(view)
	if !strings.Contains(plain, "srv") {
		t.Error("view should render the added MCP server name")
	}
	if !strings.Contains(plain, "Import:") {
		t.Error("view should contain 'Import:' section header for additions")
	}

	// Enable strict mode, then verify removals appear.
	m.focusIdx = 0 // checkbox
	m.strict = true
	view = m.ViewOver("background content here")
	plain = stripAnsi(view)
	if !strings.Contains(plain, "extra") {
		t.Error("view should render the removal item when strict is enabled")
	}
	if !strings.Contains(plain, "Remove (strict):") {
		t.Error("view should contain 'Remove (strict):' section header")
	}
}

func TestBulkApplyModel_NoStrict_FocusCycles2(t *testing.T) {
	keys := DefaultKeyMap()
	m := NewBulkApply(keys)
	m.Activate(BulkApplyPlan{
		Source: PaneSource{Kind: SourceClaudeCode},
		Target: PaneSource{Kind: SourceManifest},
		AddMCP: []string{"srv"},
	})
	m.SetSize(80, 40)

	if m.itemCount() != 2 {
		t.Fatalf("itemCount = %d, want 2 (cancel, apply)", m.itemCount())
	}

	// Default focus for !showStrict is 1 (Apply).
	if m.focusIdx != 1 {
		t.Errorf("initial focusIdx = %d, want 1 (Apply)", m.focusIdx)
	}

	// Tab cycles: 1 -> 0 -> 1.
	visited := make(map[int]bool)
	for i := 0; i < 2; i++ {
		visited[m.focusIdx] = true
		var updated BulkApplyModel
		updated, _ = m.Update(tabMsg())
		m = updated
	}
	if len(visited) != 2 {
		t.Errorf("visited %d indices, want 2 (should cycle through all)", len(visited))
	}
	if m.focusIdx != 1 {
		t.Errorf("after full cycle focusIdx = %d, want 1", m.focusIdx)
	}

	// Verify the view does NOT contain "Strict" checkbox text.
	view := m.ViewOver("background content here")
	plain := stripAnsi(view)
	if strings.Contains(plain, "Strict mode") {
		t.Error("view should not contain strict checkbox when showStrict=false")
	}
}

func TestBulkApplyModel_McpCreation_ViewContainsMcpJson(t *testing.T) {
	keys := DefaultKeyMap()
	m := NewBulkApply(keys)
	m.Activate(BulkApplyPlan{
		Source:      PaneSource{Kind: SourceProfile, ProfileName: "work"},
		Target:      PaneSource{Kind: SourceProject},
		AddMCP:      []string{"srv"},
		McpCreation: true,
	})
	m.SetSize(80, 40)

	view := m.ViewOver("background content here")
	plain := stripAnsi(view)

	if !strings.Contains(plain, ".mcp.json") {
		t.Errorf("view should mention .mcp.json when mcpCreation=true; got:\n%s", plain)
	}
}

func TestBulkApplyModel_McpCreation_False_NoMcpJson(t *testing.T) {
	keys := DefaultKeyMap()
	m := NewBulkApply(keys)
	m.Activate(BulkApplyPlan{
		Source: PaneSource{Kind: SourceManifest},
		Target: PaneSource{Kind: SourceClaudeCode},
		AddMCP: []string{"srv"},
	})
	m.SetSize(80, 40)

	view := m.ViewOver("background content here")
	plain := stripAnsi(view)

	if strings.Contains(plain, ".mcp.json") {
		t.Error("view should NOT mention .mcp.json when mcpCreation=false")
	}
}

func TestBulkApplyModel_TitleContainsSourceTarget(t *testing.T) {
	keys := DefaultKeyMap()
	m := NewBulkApply(keys)

	source := PaneSource{Kind: SourceManifest}
	target := PaneSource{Kind: SourceClaudeCode}

	m.Activate(BulkApplyPlan{Source: source, Target: target})
	m.SetSize(80, 40)

	view := m.ViewOver("background content here")
	plain := stripAnsi(view)

	if !strings.Contains(plain, source.Label()) {
		t.Errorf("view should contain source label %q; got:\n%s", source.Label(), plain)
	}
	if !strings.Contains(plain, target.Label()) {
		t.Errorf("view should contain target label %q; got:\n%s", target.Label(), plain)
	}
}

func TestBulkApplyModel_TitleWithProfileLabels(t *testing.T) {
	keys := DefaultKeyMap()
	m := NewBulkApply(keys)

	source := PaneSource{Kind: SourceProfile, ProfileName: "dev"}
	target := PaneSource{Kind: SourceProject}

	m.Activate(BulkApplyPlan{Source: source, Target: target, AddMCP: []string{"srv"}, McpCreation: true})
	m.SetSize(80, 40)

	view := m.ViewOver("background content here")
	plain := stripAnsi(view)

	if !strings.Contains(plain, "Profile: dev") {
		t.Errorf("view should contain 'Profile: dev'; got:\n%s", plain)
	}
	if !strings.Contains(plain, "Project") {
		t.Errorf("view should contain 'Project'; got:\n%s", plain)
	}
}

func TestBulkApplyModel_Inactive_ReturnsBackground(t *testing.T) {
	keys := DefaultKeyMap()
	m := NewBulkApply(keys)
	// Not activated.
	bg := "some background"
	got := m.ViewOver(bg)
	if got != bg {
		t.Errorf("inactive ViewOver should return background unchanged; got %q", got)
	}
}

func TestBulkApplyModel_LeftRight_ToggleButtons(t *testing.T) {
	keys := DefaultKeyMap()
	m := NewBulkApply(keys)
	m.Activate(BulkApplyPlan{
		Source: PaneSource{Kind: SourceManifest},
		Target: PaneSource{Kind: SourceClaudeCode},
		AddMCP: []string{"srv"},
	})
	m.SetSize(80, 40)

	// Default focus is Apply (index 1).
	if m.focusIdx != 1 {
		t.Fatalf("initial focusIdx = %d, want 1 (Apply)", m.focusIdx)
	}

	// Left arrow should move to Cancel (index 0).
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	if m.focusIdx != 0 {
		t.Errorf("after Left, focusIdx = %d, want 0 (Cancel)", m.focusIdx)
	}

	// Right arrow should move back to Apply (index 1).
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	if m.focusIdx != 1 {
		t.Errorf("after Right, focusIdx = %d, want 1 (Apply)", m.focusIdx)
	}

	// Left from Cancel should go to Apply.
	m.focusIdx = 0
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	if m.focusIdx != 1 {
		t.Errorf("Left from Cancel, focusIdx = %d, want 1 (Apply)", m.focusIdx)
	}
}

func TestBulkApplyModel_LeftRight_WithStrict_SkipsCheckbox(t *testing.T) {
	keys := DefaultKeyMap()
	m := NewBulkApply(keys)
	m.Activate(BulkApplyPlan{
		Source:     PaneSource{Kind: SourceManifest},
		Target:     PaneSource{Kind: SourceClaudeCode},
		AddMCP:     []string{"srv"},
		RemoveMCP:  []string{"extra"},
		ShowStrict: true,
	})
	m.SetSize(80, 40)

	// Default focus is Apply (index 2).
	if m.focusIdx != 2 {
		t.Fatalf("initial focusIdx = %d, want 2 (Apply)", m.focusIdx)
	}

	// Left arrow should toggle to Cancel (index 1), skipping checkbox (0).
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	if m.focusIdx != 1 {
		t.Errorf("after Left from Apply, focusIdx = %d, want 1 (Cancel)", m.focusIdx)
	}

	// Right arrow from Cancel should go back to Apply (index 2).
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	if m.focusIdx != 2 {
		t.Errorf("after Right from Cancel, focusIdx = %d, want 2 (Apply)", m.focusIdx)
	}

	// From checkbox (0), Left/Right is a no-op (checkbox is not a button).
	m.focusIdx = 0
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	if m.focusIdx != 0 {
		t.Errorf("Right from checkbox, focusIdx = %d, want 0 (stay on checkbox)", m.focusIdx)
	}
}

func TestBulkApplyModel_UpDown_SpatialNavigation(t *testing.T) {
	keys := DefaultKeyMap()
	m := NewBulkApply(keys)
	m.Activate(BulkApplyPlan{
		Source:     PaneSource{Kind: SourceManifest},
		Target:     PaneSource{Kind: SourceClaudeCode},
		AddMCP:     []string{"srv"},
		RemoveMCP:  []string{"extra"},
		ShowStrict: true,
	})
	m.SetSize(80, 40)

	// Start at Apply (2). Up should go to checkbox (0), not Cancel.
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	if m.focusIdx != 0 {
		t.Errorf("Up from Apply, focusIdx = %d, want 0 (checkbox)", m.focusIdx)
	}

	// Down from checkbox should go to Apply (2), not Cancel.
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if m.focusIdx != 2 {
		t.Errorf("Down from checkbox, focusIdx = %d, want 2 (Apply)", m.focusIdx)
	}

	// Up from Cancel (1) should also go to checkbox (0).
	m.focusIdx = 1
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	if m.focusIdx != 0 {
		t.Errorf("Up from Cancel, focusIdx = %d, want 0 (checkbox)", m.focusIdx)
	}
}

// tabMsg creates a tea.KeyPressMsg for the tab key.
func tabMsg() tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: tea.KeyTab}
}
