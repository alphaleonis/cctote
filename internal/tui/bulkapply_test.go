package tui

import (
	"testing"

	"github.com/alphaleonis/cctote/internal/manifest"
)

func TestHandleBulkApply_DanglingProfileRef_ShowsError(t *testing.T) {
	// Profile references "missing-srv" which is not in the manifest.
	// handleBulkApply should show an error flash instead of opening the overlay.
	m := newTUIModel(Options{})
	m.ready = true
	m.leftSource = PaneSource{Kind: SourceProfile, ProfileName: "work"}
	m.rightSource = PaneSource{Kind: SourceClaudeCode}
	m.focusLeft = true
	m.fullState = &FullState{
		Manifest: &manifest.Manifest{
			Version: 1,
			MCPServers: map[string]manifest.MCPServer{
				"real-srv": {Command: "cmd"},
			},
			Plugins: []manifest.Plugin{
				{ID: "real-plug", Scope: "global", Enabled: true},
			},
			Profiles: map[string]manifest.Profile{
				"work": {
					MCPServers: []string{"real-srv", "missing-srv"},
					Plugins:    []manifest.ProfilePlugin{{ID: "real-plug"}},
				},
			},
		},
		MCPInstalled: map[string]manifest.MCPServer{},
	}

	result, _ := m.handleBulkApply()
	resultModel := result.(tuiModel)

	// The overlay should NOT have been opened.
	if resultModel.bulkApply.Active() {
		t.Error("bulkApply overlay should NOT be active when profile has dangling references")
	}
}

func TestHandleBulkApply_ValidProfile_OpensOverlay(t *testing.T) {
	// Profile with all valid references should open the overlay normally.
	m := newTUIModel(Options{})
	m.ready = true
	m.leftSource = PaneSource{Kind: SourceProfile, ProfileName: "work"}
	m.rightSource = PaneSource{Kind: SourceClaudeCode}
	m.focusLeft = true
	m.fullState = &FullState{
		Manifest: &manifest.Manifest{
			Version: 1,
			MCPServers: map[string]manifest.MCPServer{
				"srv-a": {Command: "cmd"},
			},
			Plugins: []manifest.Plugin{
				{ID: "plug-a", Scope: "global", Enabled: true},
			},
			Profiles: map[string]manifest.Profile{
				"work": {
					MCPServers: []string{"srv-a"},
					Plugins:    []manifest.ProfilePlugin{{ID: "plug-a"}},
				},
			},
		},
		MCPInstalled: map[string]manifest.MCPServer{},
	}

	result, _ := m.handleBulkApply()
	resultModel := result.(tuiModel)

	if !resultModel.bulkApply.Active() {
		t.Error("bulkApply overlay should be active for a valid profile")
	}
}
