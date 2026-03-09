package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alphaleonis/cctote/internal/cliutil"
	"github.com/alphaleonis/cctote/internal/engine"
	"github.com/alphaleonis/cctote/internal/manifest"
)

// --- handleCopyResult ---

func TestHandleCopyResult(t *testing.T) {
	tests := []struct {
		name              string
		msg               CopyResultMsg
		wantFlash         string
		wantAlert         string // non-empty means error shows in alert overlay
		wantManifestDirty bool
	}{
		{
			name: "all succeed",
			msg: CopyResultMsg{
				Op: OpExportToManifest,
				Results: []ItemResult{
					{Name: "srv-a"},
					{Name: "srv-b"},
				},
			},
			wantFlash:         "Exported 2 item(s)",
			wantManifestDirty: true, // OpExportToManifest modifies manifest
		},
		{
			name: "all fail",
			msg: CopyResultMsg{
				Op: OpImportToClaude,
				Results: []ItemResult{
					{Name: "srv-a", Err: fmt.Errorf("network error")},
				},
			},
			wantAlert: "Import failed",
		},
		{
			name: "mixed results",
			msg: CopyResultMsg{
				Op: OpExportToManifest,
				Results: []ItemResult{
					{Name: "srv-a"},
					{Name: "srv-b", Err: fmt.Errorf("write error")},
				},
			},
			wantAlert:         "Exported 1, 1 failed",
			wantManifestDirty: true,
		},
		{
			name: "non-manifest op does not set dirty",
			msg: CopyResultMsg{
				Op: OpImportToClaude,
				Results: []ItemResult{
					{Name: "srv-a"},
				},
			},
			wantFlash:         "Imported 1 item(s)",
			wantManifestDirty: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newTUIModel(Options{})
			m.ready = true
			m.fullState = &FullState{
				Manifest:     &manifest.Manifest{Version: 1},
				MCPInstalled: map[string]manifest.MCPServer{},
			}
			// Populate compState so focusedPanel().ClearSelection() doesn't panic.
			m.compState = &engine.SyncState{
				MCPSync:  map[string]engine.ItemSync{},
				PlugSync: map[string]engine.ItemSync{},
				MktSync:  map[string]engine.ItemSync{},
			}

			result, _ := m.handleCopyResult(tt.msg)
			rm := result.(tuiModel)

			if tt.wantAlert != "" {
				if !rm.alert.Active() {
					t.Error("alert should be active")
				}
				if !strings.Contains(rm.alert.title, tt.wantAlert) {
					t.Errorf("alert.title = %q, want substring %q", rm.alert.title, tt.wantAlert)
				}
				if rm.status.flash != "" {
					t.Errorf("flash should be empty when error is shown in alert, got %q", rm.status.flash)
				}
			}
			if tt.wantFlash != "" && !strings.Contains(rm.status.flash, tt.wantFlash) {
				t.Errorf("flash = %q, want substring %q", rm.status.flash, tt.wantFlash)
			}
			if rm.manifestDirty != tt.wantManifestDirty {
				t.Errorf("manifestDirty = %v, want %v", rm.manifestDirty, tt.wantManifestDirty)
			}
		})
	}
}

// --- formatItemErrors ---

func TestFormatItemErrors(t *testing.T) {
	t.Run("single RunError shows stderr", func(t *testing.T) {
		results := []ItemResult{
			{Name: "ok-item"},
			{
				Name: "bad-plugin",
				Err: fmt.Errorf("installing plugin %q: %w", "bad-plugin", &cliutil.RunError{
					Command:  "claude",
					ExitCode: 1,
					Stderr:   "Plugin not found in marketplace",
					Err:      fmt.Errorf("exit status 1"),
				}),
			},
		}
		got := formatItemErrors(results)
		if got != "Plugin not found in marketplace" {
			t.Errorf("formatItemErrors() = %q, want stderr text", got)
		}
	})

	t.Run("multiple RunErrors show stderr with names", func(t *testing.T) {
		results := []ItemResult{
			{
				Name: "plug-a",
				Err: fmt.Errorf("installing plugin %q: %w", "plug-a", &cliutil.RunError{
					Command: "claude", ExitCode: 1, Stderr: "Error A",
					Err: fmt.Errorf("exit status 1"),
				}),
			},
			{
				Name: "plug-b",
				Err: fmt.Errorf("installing plugin %q: %w", "plug-b", &cliutil.RunError{
					Command: "claude", ExitCode: 1, Stderr: "Error B",
					Err: fmt.Errorf("exit status 1"),
				}),
			},
		}
		got := formatItemErrors(results)
		if !strings.Contains(got, "plug-a: Error A") {
			t.Errorf("formatItemErrors() = %q, want to contain 'plug-a: Error A'", got)
		}
		if !strings.Contains(got, "plug-b: Error B") {
			t.Errorf("formatItemErrors() = %q, want to contain 'plug-b: Error B'", got)
		}
	})

	t.Run("plain errors still work", func(t *testing.T) {
		results := []ItemResult{
			{Name: "item", Err: fmt.Errorf("plain error")},
		}
		got := formatItemErrors(results)
		if got != "plain error" {
			t.Errorf("formatItemErrors() = %q, want %q", got, "plain error")
		}
	})
}

// --- lookupStatus ---

func TestLookupStatus(t *testing.T) {
	compState := &engine.SyncState{
		MCPSync: map[string]engine.ItemSync{
			"srv": {Status: engine.Different},
		},
		PlugSync: map[string]engine.ItemSync{
			"plug": {Status: engine.LeftOnly},
		},
		MktSync: map[string]engine.ItemSync{
			"mkt": {Status: engine.RightOnly},
		},
	}

	tests := []struct {
		name      string
		compState *engine.SyncState
		item      CopyItem
		want      SyncStatus
	}{
		{
			name:      "MCP found",
			compState: compState,
			item:      CopyItem{Section: SectionMCP, Name: "srv"},
			want:      Different,
		},
		{
			name:      "plugin found",
			compState: compState,
			item:      CopyItem{Section: SectionPlugin, Name: "plug"},
			want:      LeftOnly,
		},
		{
			name:      "marketplace found",
			compState: compState,
			item:      CopyItem{Section: SectionMarketplace, Name: "mkt"},
			want:      RightOnly,
		},
		{
			name:      "MCP not found falls back to Synced",
			compState: compState,
			item:      CopyItem{Section: SectionMCP, Name: "unknown"},
			want:      Synced,
		},
		{
			name:      "nil compState falls back to Synced",
			compState: nil,
			item:      CopyItem{Section: SectionMCP, Name: "srv"},
			want:      Synced,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := tuiModel{compState: tt.compState}
			got := m.lookupStatus(tt.item)
			if got != tt.want {
				t.Errorf("lookupStatus() = %v, want %v", got, tt.want)
			}
		})
	}
}

// --- confirmTitle ---

func TestConfirmTitle(t *testing.T) {
	tests := []struct {
		name  string
		op    ResolvedOp
		items []CopyItem
		want  string
	}{
		{
			name:  "single item",
			op:    OpExportToManifest,
			items: []CopyItem{{Section: SectionMCP, Name: "my-server"}},
			want:  "Export my-server?",
		},
		{
			name: "multiple items",
			op:   OpImportToClaude,
			items: []CopyItem{
				{Section: SectionMCP, Name: "srv-a"},
				{Section: SectionPlugin, Name: "plug-b"},
			},
			want: "Import 2 items?",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := tuiModel{}
			got := m.confirmTitle(tt.op, tt.items)
			if got != tt.want {
				t.Errorf("confirmTitle() = %q, want %q", got, tt.want)
			}
		})
	}
}

// --- projectMcpMissing ---

func TestProjectMcpMissing(t *testing.T) {
	t.Run("no project root", func(t *testing.T) {
		m := tuiModel{fullState: &FullState{ProjectRoot: ""}}
		if m.projectMcpMissing() {
			t.Error("expected false when no project root")
		}
	})

	t.Run("nil fullState", func(t *testing.T) {
		m := tuiModel{fullState: nil}
		if m.projectMcpMissing() {
			t.Error("expected false when fullState is nil")
		}
	})

	t.Run("file exists", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, ".mcp.json"), []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
		m := tuiModel{fullState: &FullState{ProjectRoot: dir}}
		if m.projectMcpMissing() {
			t.Error("expected false when .mcp.json exists")
		}
	})

	t.Run("file missing", func(t *testing.T) {
		dir := t.TempDir()
		m := tuiModel{fullState: &FullState{ProjectRoot: dir}}
		if !m.projectMcpMissing() {
			t.Error("expected true when .mcp.json is missing")
		}
	})
}

// --- dispatchCopy marketplace filtering ---

func TestDispatchCopy_MarketplaceFiltering(t *testing.T) {
	t.Run("profile target filters marketplace items", func(t *testing.T) {
		srv := manifest.MCPServer{Command: "cmd"}
		compState := engine.CompareSources(
			engine.SourceData{
				MCP:          map[string]manifest.MCPServer{"srv": srv},
				Marketplaces: map[string]manifest.Marketplace{"mkt": {Source: "github"}},
			},
			engine.SourceData{},
		)

		m := tuiModel{
			compState: compState,
			status:    NewStatus(),
			confirm:   NewConfirm(DefaultKeyMap()),
		}

		items := []CopyItem{
			{Section: SectionMCP, Name: "srv"},
			{Section: SectionMarketplace, Name: "mkt"},
		}

		result, _ := m.dispatchCopy(items, OpExportAndAddProfile,
			PaneSource{Kind: SourceClaudeCode},
			PaneSource{Kind: SourceProfile, ProfileName: "dev"})
		rm := result.(tuiModel)

		// Should NOT show "Marketplaces are global" flash because there's still
		// an MCP item left after filtering.
		if strings.Contains(rm.status.flash, "global") {
			t.Error("should not show global flash when non-marketplace items remain")
		}
	})

	t.Run("all-marketplace batch shows flash", func(t *testing.T) {
		compState := engine.CompareSources(
			engine.SourceData{
				Marketplaces: map[string]manifest.Marketplace{"mkt": {Source: "github"}},
			},
			engine.SourceData{},
		)

		m := tuiModel{
			compState: compState,
			status:    NewStatus(),
			confirm:   NewConfirm(DefaultKeyMap()),
		}

		items := []CopyItem{
			{Section: SectionMarketplace, Name: "mkt"},
		}

		result, _ := m.dispatchCopy(items, OpAddToProfile,
			PaneSource{Kind: SourceManifest},
			PaneSource{Kind: SourceProfile, ProfileName: "dev"})
		rm := result.(tuiModel)

		if !strings.Contains(rm.status.flash, "global") {
			t.Errorf("flash = %q, want substring 'global'", rm.status.flash)
		}
	})
}
