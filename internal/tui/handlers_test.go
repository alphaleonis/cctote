package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/alphaleonis/cctote/internal/engine"
	"github.com/alphaleonis/cctote/internal/manifest"
)

// --- handleStateLoaded ---

func TestHandleStateLoaded(t *testing.T) {
	tests := []struct {
		name           string
		opts           Options
		msg            StateLoadedMsg
		wantReady      bool
		wantErr        bool
		wantFlash      string
		wantLeftSource PaneSource
	}{
		{
			name: "success",
			msg: StateLoadedMsg{
				State: &FullState{
					Manifest:     &manifest.Manifest{Version: 1},
					MCPInstalled: map[string]manifest.MCPServer{},
				},
			},
			wantReady:      true,
			wantLeftSource: PaneSource{Kind: SourceManifest},
		},
		{
			name:    "error causes quit",
			msg:     StateLoadedMsg{Err: fmt.Errorf("load failed")},
			wantErr: true,
		},
		{
			name: "profile not found falls back to manifest",
			opts: Options{Profile: "missing"},
			msg: StateLoadedMsg{
				State: &FullState{
					Manifest: &manifest.Manifest{
						Version:  1,
						Profiles: map[string]manifest.Profile{},
					},
					MCPInstalled: map[string]manifest.MCPServer{},
				},
			},
			wantReady:      true,
			wantFlash:      "not found",
			wantLeftSource: PaneSource{Kind: SourceManifest},
		},
		{
			name: "with warnings",
			msg: StateLoadedMsg{
				State: &FullState{
					Manifest:     &manifest.Manifest{Version: 1},
					MCPInstalled: map[string]manifest.MCPServer{},
				},
				Warnings: []string{"plugin list failed", "marketplace unavailable"},
			},
			wantReady: true,
			wantFlash: "plugin list failed",
		},
		{
			name: "valid profile preserved",
			opts: Options{Profile: "dev"},
			msg: StateLoadedMsg{
				State: &FullState{
					Manifest: &manifest.Manifest{
						Version: 1,
						Profiles: map[string]manifest.Profile{
							"dev": {MCPServers: []string{}, Plugins: []manifest.ProfilePlugin{}},
						},
					},
					MCPInstalled: map[string]manifest.MCPServer{},
				},
			},
			wantReady:      true,
			wantLeftSource: PaneSource{Kind: SourceProfile, ProfileName: "dev"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newTUIModel(tt.opts)
			result, _ := m.handleStateLoaded(tt.msg)
			rm := result.(tuiModel)

			if tt.wantErr {
				if rm.err == nil {
					t.Error("expected err to be set")
				}
				return
			}
			if rm.ready != tt.wantReady {
				t.Errorf("ready = %v, want %v", rm.ready, tt.wantReady)
			}
			if tt.wantFlash != "" && !strings.Contains(rm.status.flash, tt.wantFlash) {
				t.Errorf("flash = %q, want substring %q", rm.status.flash, tt.wantFlash)
			}
			if tt.wantLeftSource.Kind != 0 || tt.wantLeftSource.ProfileName != "" {
				if !rm.leftSource.Equal(tt.wantLeftSource) {
					t.Errorf("leftSource = %v, want %v", rm.leftSource, tt.wantLeftSource)
				}
			}
		})
	}
}

// --- handleStateRefreshed ---

func TestHandleStateRefreshed(t *testing.T) {
	tests := []struct {
		name            string
		leftSource      PaneSource
		rightSource     PaneSource
		msg             StateRefreshedMsg
		wantErr         bool
		wantFlash       string
		wantLeftSource  PaneSource
		wantRightSource PaneSource
		wantBusy        bool
	}{
		{
			name:        "success",
			leftSource:  PaneSource{Kind: SourceManifest},
			rightSource: PaneSource{Kind: SourceClaudeCode},
			msg: StateRefreshedMsg{
				State: &FullState{
					Manifest:     &manifest.Manifest{Version: 1},
					MCPInstalled: map[string]manifest.MCPServer{},
				},
			},
			wantLeftSource:  PaneSource{Kind: SourceManifest},
			wantRightSource: PaneSource{Kind: SourceClaudeCode},
		},
		{
			name:        "error sets err field",
			leftSource:  PaneSource{Kind: SourceManifest},
			rightSource: PaneSource{Kind: SourceClaudeCode},
			msg:         StateRefreshedMsg{Err: fmt.Errorf("refresh failed")},
			wantErr:     true,
		},
		{
			name:        "left profile dropped",
			leftSource:  PaneSource{Kind: SourceProfile, ProfileName: "gone"},
			rightSource: PaneSource{Kind: SourceClaudeCode},
			msg: StateRefreshedMsg{
				State: &FullState{
					Manifest: &manifest.Manifest{
						Version:  1,
						Profiles: map[string]manifest.Profile{},
					},
					MCPInstalled: map[string]manifest.MCPServer{},
				},
			},
			wantFlash:       "no longer exists",
			wantLeftSource:  PaneSource{Kind: SourceManifest},
			wantRightSource: PaneSource{Kind: SourceClaudeCode},
		},
		{
			name:        "right profile dropped",
			leftSource:  PaneSource{Kind: SourceManifest},
			rightSource: PaneSource{Kind: SourceProfile, ProfileName: "gone"},
			msg: StateRefreshedMsg{
				State: &FullState{
					Manifest: &manifest.Manifest{
						Version:  1,
						Profiles: map[string]manifest.Profile{},
					},
					MCPInstalled: map[string]manifest.MCPServer{},
				},
			},
			wantFlash:       "no longer exists",
			wantLeftSource:  PaneSource{Kind: SourceManifest},
			wantRightSource: PaneSource{Kind: SourceClaudeCode},
		},
		{
			name:        "with warnings",
			leftSource:  PaneSource{Kind: SourceManifest},
			rightSource: PaneSource{Kind: SourceClaudeCode},
			msg: StateRefreshedMsg{
				State: &FullState{
					Manifest:     &manifest.Manifest{Version: 1},
					MCPInstalled: map[string]manifest.MCPServer{},
				},
				Warnings: []string{"something off"},
			},
			wantFlash:       "something off",
			wantLeftSource:  PaneSource{Kind: SourceManifest},
			wantRightSource: PaneSource{Kind: SourceClaudeCode},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newTUIModel(Options{})
			m.ready = true
			m.busy = true
			m.leftSource = tt.leftSource
			m.rightSource = tt.rightSource

			result, _ := m.handleStateRefreshed(tt.msg)
			rm := result.(tuiModel)

			if tt.wantErr {
				if rm.err == nil {
					t.Error("expected err to be set")
				}
				return
			}
			if rm.busy {
				t.Error("busy should be cleared after refresh")
			}
			if tt.wantFlash != "" && !strings.Contains(rm.status.flash, tt.wantFlash) {
				t.Errorf("flash = %q, want substring %q", rm.status.flash, tt.wantFlash)
			}
			if !rm.leftSource.Equal(tt.wantLeftSource) {
				t.Errorf("leftSource = %v, want %v", rm.leftSource, tt.wantLeftSource)
			}
			if !rm.rightSource.Equal(tt.wantRightSource) {
				t.Errorf("rightSource = %v, want %v", rm.rightSource, tt.wantRightSource)
			}
		})
	}
}

// --- handleConfirmAccepted ---

func TestHandleConfirmAccepted(t *testing.T) {
	tests := []struct {
		name     string
		msg      ConfirmAcceptedMsg
		wantBusy bool
	}{
		{
			name: "profile delete dispatches delete",
			msg: ConfirmAcceptedMsg{
				Op:   OpDeleteProfile,
				From: PaneSource{Kind: SourceProfile, ProfileName: "dev"},
				To:   PaneSource{Kind: SourceProfile, ProfileName: "dev"},
			},
			wantBusy: true,
		},
		{
			name: "regular copy dispatches copy",
			msg: ConfirmAcceptedMsg{
				Items: []CopyItem{{Section: SectionMCP, Name: "srv"}},
				Op:    OpExportToManifest,
				From:  PaneSource{Kind: SourceClaudeCode},
				To:    PaneSource{Kind: SourceManifest},
			},
			wantBusy: true,
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

			result, cmd := m.handleConfirmAccepted(tt.msg)
			rm := result.(tuiModel)

			if rm.busy != tt.wantBusy {
				t.Errorf("busy = %v, want %v", rm.busy, tt.wantBusy)
			}
			if rm.confirm.Active() {
				t.Error("confirm should be deactivated")
			}
			if cmd == nil {
				t.Error("expected non-nil cmd")
			}
		})
	}
}

// --- handleSourceSelected ---

func TestHandleSourceSelected(t *testing.T) {
	tests := []struct {
		name            string
		msg             SourceSelectedMsg
		wantLeftSource  PaneSource
		wantRightSource PaneSource
	}{
		{
			name: "left side",
			msg: SourceSelectedMsg{
				Side:   SideLeft,
				Source: PaneSource{Kind: SourceProfile, ProfileName: "dev"},
			},
			wantLeftSource:  PaneSource{Kind: SourceProfile, ProfileName: "dev"},
			wantRightSource: PaneSource{Kind: SourceClaudeCode},
		},
		{
			name: "right side",
			msg: SourceSelectedMsg{
				Side:   SideRight,
				Source: PaneSource{Kind: SourceProject},
			},
			wantLeftSource:  PaneSource{Kind: SourceManifest},
			wantRightSource: PaneSource{Kind: SourceProject},
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

			result, _ := m.handleSourceSelected(tt.msg)
			rm := result.(tuiModel)

			if !rm.leftSource.Equal(tt.wantLeftSource) {
				t.Errorf("leftSource = %v, want %v", rm.leftSource, tt.wantLeftSource)
			}
			if !rm.rightSource.Equal(tt.wantRightSource) {
				t.Errorf("rightSource = %v, want %v", rm.rightSource, tt.wantRightSource)
			}
		})
	}
}

// --- handleBulkApplyResult ---

func TestHandleBulkApplyResult(t *testing.T) {
	tests := []struct {
		name              string
		msg               BulkApplyResultMsg
		wantFlash         string
		wantAlert         string // non-empty means error shows in alert overlay
		checkFlashStyle   bool
		wantFlashStyle    FlashStyle
		wantManifestDirty bool
	}{
		{
			name:            "success",
			msg:             BulkApplyResultMsg{},
			wantFlash:       "Applied",
			checkFlashStyle: true,
			wantFlashStyle:  FlashSuccess,
		},
		{
			name:      "error",
			msg:       BulkApplyResultMsg{Err: fmt.Errorf("apply failed")},
			wantAlert: "Apply failed",
		},
		{
			name:            "with warnings",
			msg:             BulkApplyResultMsg{Warnings: []string{"env var detected"}},
			wantFlash:       "warning",
			checkFlashStyle: true,
			wantFlashStyle:  FlashInfo,
		},
		{
			name:              "manifestDirty propagation",
			msg:               BulkApplyResultMsg{ManifestDirty: true},
			wantFlash:         "Applied",
			wantManifestDirty: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newTUIModel(Options{})
			m.busy = true

			result, _ := m.handleBulkApplyResult(tt.msg)
			rm := result.(tuiModel)

			if rm.busy {
				t.Error("busy should be cleared")
			}
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
			if tt.wantFlash != "" && !strings.Contains(strings.ToLower(rm.status.flash), strings.ToLower(tt.wantFlash)) {
				t.Errorf("flash = %q, want substring %q", rm.status.flash, tt.wantFlash)
			}
			if tt.checkFlashStyle && rm.status.flashStyle != tt.wantFlashStyle {
				t.Errorf("flashStyle = %v, want %v", rm.status.flashStyle, tt.wantFlashStyle)
			}
			if rm.manifestDirty != tt.wantManifestDirty {
				t.Errorf("manifestDirty = %v, want %v", rm.manifestDirty, tt.wantManifestDirty)
			}
		})
	}
}

// --- handleProfileCreateResult ---

func TestHandleProfileCreateResult(t *testing.T) {
	tests := []struct {
		name              string
		msg               ProfileCreateResultMsg
		wantFlash         string
		wantAlert         string
		wantManifestDirty bool
		wantLeftSource    PaneSource
	}{
		{
			name:              "success switches left source",
			msg:               ProfileCreateResultMsg{Name: "new-profile"},
			wantFlash:         "created",
			wantManifestDirty: true,
			wantLeftSource:    PaneSource{Kind: SourceProfile, ProfileName: "new-profile"},
		},
		{
			name:           "error",
			msg:            ProfileCreateResultMsg{Name: "bad", Err: fmt.Errorf("dup name")},
			wantAlert:      "Create profile failed",
			wantLeftSource: PaneSource{Kind: SourceManifest}, // unchanged
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newTUIModel(Options{})
			m.busy = true

			result, _ := m.handleProfileCreateResult(tt.msg)
			rm := result.(tuiModel)

			if rm.busy {
				t.Error("busy should be cleared")
			}
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
			if !rm.leftSource.Equal(tt.wantLeftSource) {
				t.Errorf("leftSource = %v, want %v", rm.leftSource, tt.wantLeftSource)
			}
		})
	}
}

// --- handleProfileDeleteResult ---

func TestHandleProfileDeleteResult(t *testing.T) {
	tests := []struct {
		name              string
		leftSource        PaneSource
		rightSource       PaneSource
		msg               ProfileDeleteResultMsg
		wantFlash         string
		wantAlert         string
		wantManifestDirty bool
		wantLeftSource    PaneSource
		wantRightSource   PaneSource
	}{
		{
			name:              "success",
			leftSource:        PaneSource{Kind: SourceManifest},
			rightSource:       PaneSource{Kind: SourceClaudeCode},
			msg:               ProfileDeleteResultMsg{Name: "dev"},
			wantFlash:         "deleted",
			wantManifestDirty: true,
			wantLeftSource:    PaneSource{Kind: SourceManifest},
			wantRightSource:   PaneSource{Kind: SourceClaudeCode},
		},
		{
			name:            "error",
			leftSource:      PaneSource{Kind: SourceManifest},
			rightSource:     PaneSource{Kind: SourceClaudeCode},
			msg:             ProfileDeleteResultMsg{Name: "dev", Err: fmt.Errorf("not found")},
			wantAlert:       "Delete profile failed",
			wantLeftSource:  PaneSource{Kind: SourceManifest},
			wantRightSource: PaneSource{Kind: SourceClaudeCode},
		},
		{
			name:              "left pane reset",
			leftSource:        PaneSource{Kind: SourceProfile, ProfileName: "dev"},
			rightSource:       PaneSource{Kind: SourceClaudeCode},
			msg:               ProfileDeleteResultMsg{Name: "dev"},
			wantFlash:         "deleted",
			wantManifestDirty: true,
			wantLeftSource:    PaneSource{Kind: SourceManifest},
			wantRightSource:   PaneSource{Kind: SourceClaudeCode},
		},
		{
			name:              "right pane reset",
			leftSource:        PaneSource{Kind: SourceManifest},
			rightSource:       PaneSource{Kind: SourceProfile, ProfileName: "dev"},
			msg:               ProfileDeleteResultMsg{Name: "dev"},
			wantFlash:         "deleted",
			wantManifestDirty: true,
			wantLeftSource:    PaneSource{Kind: SourceManifest},
			wantRightSource:   PaneSource{Kind: SourceClaudeCode},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newTUIModel(Options{})
			m.busy = true
			m.leftSource = tt.leftSource
			m.rightSource = tt.rightSource

			result, _ := m.handleProfileDeleteResult(tt.msg)
			rm := result.(tuiModel)

			if rm.busy {
				t.Error("busy should be cleared")
			}
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
			if !rm.leftSource.Equal(tt.wantLeftSource) {
				t.Errorf("leftSource = %v, want %v", rm.leftSource, tt.wantLeftSource)
			}
			if !rm.rightSource.Equal(tt.wantRightSource) {
				t.Errorf("rightSource = %v, want %v", rm.rightSource, tt.wantRightSource)
			}
		})
	}
}

// --- handleSourcePicker ---

func TestHandleSourcePicker(t *testing.T) {
	t.Run("nil fullState is no-op", func(t *testing.T) {
		m := newTUIModel(Options{})
		m.fullState = nil

		result, cmd := m.handleSourcePicker(SideLeft)
		rm := result.(tuiModel)

		if rm.picker.Active() {
			t.Error("picker should not activate with nil fullState")
		}
		if cmd != nil {
			t.Error("expected nil cmd")
		}
	})

	t.Run("left side excludes right source", func(t *testing.T) {
		m := newTUIModel(Options{})
		m.leftSource = PaneSource{Kind: SourceManifest}
		m.rightSource = PaneSource{Kind: SourceClaudeCode}
		m.fullState = &FullState{
			Manifest: &manifest.Manifest{
				Version:  1,
				Profiles: map[string]manifest.Profile{"dev": {}},
			},
		}

		result, _ := m.handleSourcePicker(SideLeft)
		rm := result.(tuiModel)

		if !rm.picker.Active() {
			t.Fatal("picker should be active")
		}
		// ClaudeCode is the right source, so it should be excluded.
		for _, src := range rm.picker.items {
			if src.Kind == SourceClaudeCode {
				t.Error("right source (ClaudeCode) should be excluded from picker")
			}
		}
	})

	t.Run("right side excludes left source", func(t *testing.T) {
		m := newTUIModel(Options{})
		m.leftSource = PaneSource{Kind: SourceManifest}
		m.rightSource = PaneSource{Kind: SourceClaudeCode}
		m.fullState = &FullState{
			Manifest: &manifest.Manifest{Version: 1},
		}

		result, _ := m.handleSourcePicker(SideRight)
		rm := result.(tuiModel)

		if !rm.picker.Active() {
			t.Fatal("picker should be active")
		}
		for _, src := range rm.picker.items {
			if src.Kind == SourceManifest {
				t.Error("left source (Manifest) should be excluded from picker")
			}
		}
	})

	t.Run("includes sorted profiles", func(t *testing.T) {
		m := newTUIModel(Options{})
		m.leftSource = PaneSource{Kind: SourceManifest}
		m.rightSource = PaneSource{Kind: SourceClaudeCode}
		m.fullState = &FullState{
			Manifest: &manifest.Manifest{
				Version: 1,
				Profiles: map[string]manifest.Profile{
					"zebra": {},
					"alpha": {},
					"mid":   {},
				},
			},
		}

		result, _ := m.handleSourcePicker(SideLeft)
		rm := result.(tuiModel)

		// Extract profile names from picker items.
		var profileNames []string
		for _, src := range rm.picker.items {
			if src.Kind == SourceProfile {
				profileNames = append(profileNames, src.ProfileName)
			}
		}
		if len(profileNames) != 3 {
			t.Fatalf("expected 3 profiles, got %d", len(profileNames))
		}
		// Verify sorted order.
		for i := 1; i < len(profileNames); i++ {
			if profileNames[i] < profileNames[i-1] {
				t.Errorf("profiles not sorted: %v", profileNames)
				break
			}
		}
	})
}

// --- activeProfileName ---

func TestActiveProfileName(t *testing.T) {
	tests := []struct {
		name        string
		leftSource  PaneSource
		rightSource PaneSource
		focusLeft   bool
		want        string
	}{
		{
			name:        "focused pane is profile",
			leftSource:  PaneSource{Kind: SourceProfile, ProfileName: "dev"},
			rightSource: PaneSource{Kind: SourceClaudeCode},
			focusLeft:   true,
			want:        "dev",
		},
		{
			name:        "other pane is profile",
			leftSource:  PaneSource{Kind: SourceManifest},
			rightSource: PaneSource{Kind: SourceProfile, ProfileName: "staging"},
			focusLeft:   true,
			want:        "staging",
		},
		{
			name:        "neither is profile",
			leftSource:  PaneSource{Kind: SourceManifest},
			rightSource: PaneSource{Kind: SourceClaudeCode},
			focusLeft:   true,
			want:        "",
		},
		{
			name:        "focus right prefers right profile",
			leftSource:  PaneSource{Kind: SourceProfile, ProfileName: "left"},
			rightSource: PaneSource{Kind: SourceProfile, ProfileName: "right"},
			focusLeft:   false,
			want:        "right",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := tuiModel{
				leftSource:  tt.leftSource,
				rightSource: tt.rightSource,
				focusLeft:   tt.focusLeft,
			}
			got := m.activeProfileName()
			if got != tt.want {
				t.Errorf("activeProfileName() = %q, want %q", got, tt.want)
			}
		})
	}
}

// --- handleProfileCreate ---

func TestHandleProfileCreate(t *testing.T) {
	t.Run("nil fullState is no-op", func(t *testing.T) {
		m := newTUIModel(Options{})
		m.fullState = nil

		result, cmd := m.handleProfileCreate()
		rm := result.(tuiModel)

		if rm.profileCreate.Active() {
			t.Error("profileCreate should not activate with nil fullState")
		}
		if cmd != nil {
			t.Error("expected nil cmd")
		}
	})

	t.Run("opens overlay with summary", func(t *testing.T) {
		m := newTUIModel(Options{})
		m.fullState = &FullState{
			Manifest:     &manifest.Manifest{Version: 1},
			MCPInstalled: map[string]manifest.MCPServer{"srv-a": {}, "srv-b": {}},
			PlugInstalled: []manifest.Plugin{
				{ID: "plug-1"},
			},
		}

		result, _ := m.handleProfileCreate()
		rm := result.(tuiModel)

		if !rm.profileCreate.Active() {
			t.Fatal("profileCreate should be active")
		}
		if !strings.Contains(rm.profileCreate.summary, "2 MCP") {
			t.Errorf("summary = %q, want to contain '2 MCP'", rm.profileCreate.summary)
		}
		if !strings.Contains(rm.profileCreate.summary, "1 plugins") {
			t.Errorf("summary = %q, want to contain '1 plugins'", rm.profileCreate.summary)
		}
	})

	t.Run("passes existing profile names", func(t *testing.T) {
		m := newTUIModel(Options{})
		m.fullState = &FullState{
			Manifest: &manifest.Manifest{
				Version: 1,
				Profiles: map[string]manifest.Profile{
					"dev":  {},
					"prod": {},
				},
			},
			MCPInstalled: map[string]manifest.MCPServer{},
		}

		result, _ := m.handleProfileCreate()
		rm := result.(tuiModel)

		if len(rm.profileCreate.existingNames) != 2 {
			t.Errorf("existingNames count = %d, want 2", len(rm.profileCreate.existingNames))
		}
	})
}

// --- handleProfileDelete ---

func TestHandleProfileDelete(t *testing.T) {
	t.Run("no profile active shows flash", func(t *testing.T) {
		m := newTUIModel(Options{})
		m.leftSource = PaneSource{Kind: SourceManifest}
		m.rightSource = PaneSource{Kind: SourceClaudeCode}
		m.focusLeft = true
		m.fullState = &FullState{
			Manifest: &manifest.Manifest{Version: 1},
		}

		result, _ := m.handleProfileDelete()
		rm := result.(tuiModel)

		if rm.confirm.Active() {
			t.Error("confirm should not activate with no profile active")
		}
		if !strings.Contains(rm.status.flash, "No profile active") {
			t.Errorf("flash = %q, want 'No profile active'", rm.status.flash)
		}
	})

	t.Run("profile not found shows flash", func(t *testing.T) {
		m := newTUIModel(Options{})
		m.leftSource = PaneSource{Kind: SourceProfile, ProfileName: "ghost"}
		m.rightSource = PaneSource{Kind: SourceClaudeCode}
		m.focusLeft = true
		m.fullState = &FullState{
			Manifest: &manifest.Manifest{
				Version:  1,
				Profiles: map[string]manifest.Profile{},
			},
		}

		result, _ := m.handleProfileDelete()
		rm := result.(tuiModel)

		if rm.confirm.Active() {
			t.Error("confirm should not activate for missing profile")
		}
		if !strings.Contains(rm.status.flash, "not found") {
			t.Errorf("flash = %q, want substring 'not found'", rm.status.flash)
		}
	})

	t.Run("opens confirm overlay", func(t *testing.T) {
		m := newTUIModel(Options{})
		m.leftSource = PaneSource{Kind: SourceProfile, ProfileName: "dev"}
		m.rightSource = PaneSource{Kind: SourceClaudeCode}
		m.focusLeft = true
		m.fullState = &FullState{
			Manifest: &manifest.Manifest{
				Version: 1,
				MCPServers: map[string]manifest.MCPServer{
					"srv": {Command: "cmd"},
				},
				Profiles: map[string]manifest.Profile{
					"dev": {MCPServers: []string{"srv"}, Plugins: []manifest.ProfilePlugin{}},
				},
			},
		}

		result, _ := m.handleProfileDelete()
		rm := result.(tuiModel)

		if !rm.confirm.Active() {
			t.Fatal("confirm should be active for valid profile delete")
		}
		if rm.confirm.op != OpDeleteProfile {
			t.Errorf("confirm.op = %d, want OpDeleteProfile (%d)", rm.confirm.op, OpDeleteProfile)
		}
	})
}

// --- handleBulkApply rejection cases ---

func TestHandleBulkApply_InvalidCombos(t *testing.T) {
	tests := []struct {
		name        string
		leftSource  PaneSource
		rightSource PaneSource
		focusLeft   bool
		wantFlash   string
	}{
		{
			name:        "same source",
			leftSource:  PaneSource{Kind: SourceManifest},
			rightSource: PaneSource{Kind: SourceManifest},
			focusLeft:   true,
			wantFlash:   "same source",
		},
		{
			name:        "profile to manifest",
			leftSource:  PaneSource{Kind: SourceProfile, ProfileName: "dev"},
			rightSource: PaneSource{Kind: SourceManifest},
			focusLeft:   true,
			wantFlash:   "already in the manifest",
		},
		{
			name:        "target is profile",
			leftSource:  PaneSource{Kind: SourceManifest},
			rightSource: PaneSource{Kind: SourceProfile, ProfileName: "dev"},
			focusLeft:   true,
			wantFlash:   "per-item copy",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newTUIModel(Options{})
			m.ready = true
			m.leftSource = tt.leftSource
			m.rightSource = tt.rightSource
			m.focusLeft = tt.focusLeft
			m.fullState = &FullState{
				Manifest: &manifest.Manifest{
					Version: 1,
					Profiles: map[string]manifest.Profile{
						"dev": {MCPServers: []string{}, Plugins: []manifest.ProfilePlugin{}},
					},
				},
				MCPInstalled: map[string]manifest.MCPServer{},
			}

			result, _ := m.handleBulkApply()
			rm := result.(tuiModel)

			if rm.bulkApply.Active() {
				t.Error("bulkApply should not activate for invalid combo")
			}
			if !strings.Contains(strings.ToLower(rm.status.flash), strings.ToLower(tt.wantFlash)) {
				t.Errorf("flash = %q, want substring %q", rm.status.flash, tt.wantFlash)
			}
		})
	}
}

// --- handleBulkApplyMsg ---

func TestHandleBulkApplyMsg(t *testing.T) {
	m := newTUIModel(Options{})
	m.ready = true
	m.fullState = &engine.FullState{
		Manifest:     &manifest.Manifest{Version: 1},
		MCPInstalled: map[string]manifest.MCPServer{},
	}

	msg := BulkApplyMsg{
		Source: PaneSource{Kind: SourceManifest},
		Target: PaneSource{Kind: SourceClaudeCode},
		Strict: false,
	}

	result, cmd := m.handleBulkApplyMsg(msg)
	rm := result.(tuiModel)

	if !rm.busy {
		t.Error("busy should be set")
	}
	if rm.bulkApply.active {
		t.Error("bulkApply should be deactivated after dispatch")
	}
	if cmd == nil {
		t.Error("expected non-nil cmd")
	}
}

// --- handleCopyJSON ---

func TestHandleCopyJSON(t *testing.T) {
	srv := manifest.MCPServer{
		Command: "npx",
		Args:    []string{"-y", "@modelcontextprotocol/server-filesystem"},
		Env:     map[string]string{"HOME": "/home/user"},
	}

	t.Run("no item selected shows flash", func(t *testing.T) {
		m := newTUIModel(Options{})
		m.ready = true
		m.compState = &engine.SyncState{
			MCPSync:  map[string]engine.ItemSync{},
			PlugSync: map[string]engine.ItemSync{},
			MktSync:  map[string]engine.ItemSync{},
		}
		// No items in panel → SelectedItem() returns nil.

		result, _ := m.handleCopyJSON()
		rm := result.(tuiModel)

		if !strings.Contains(rm.status.flash, "No item selected") {
			t.Errorf("flash = %q, want 'No item selected'", rm.status.flash)
		}
	})

	t.Run("non-MCP item shows flash", func(t *testing.T) {
		m := newTUIModel(Options{})
		m.ready = true
		m.focusLeft = true
		m.left.items = []PanelItem{
			{Section: SectionPlugin, Name: "my-plugin"},
		}
		m.left.cursor = 0
		m.compState = &engine.SyncState{
			PlugSync: map[string]engine.ItemSync{
				"my-plugin": {Status: engine.Synced, Left: manifest.Plugin{ID: "my-plugin"}},
			},
			MCPSync: map[string]engine.ItemSync{},
			MktSync: map[string]engine.ItemSync{},
		}

		result, _ := m.handleCopyJSON()
		rm := result.(tuiModel)

		if !strings.Contains(rm.status.flash, "only available for MCP") {
			t.Errorf("flash = %q, want 'only available for MCP'", rm.status.flash)
		}
	})

	t.Run("copies MCP server as JSON", func(t *testing.T) {
		m := newTUIModel(Options{})
		m.ready = true
		m.focusLeft = true
		m.left.items = []PanelItem{
			{Section: SectionMCP, Name: "fs-server"},
		}
		m.left.cursor = 0
		m.compState = &engine.SyncState{
			MCPSync: map[string]engine.ItemSync{
				"fs-server": {Status: engine.LeftOnly, Left: srv},
			},
			PlugSync: map[string]engine.ItemSync{},
			MktSync:  map[string]engine.ItemSync{},
		}

		result, cmd := m.handleCopyJSON()
		rm := result.(tuiModel)

		if !strings.Contains(rm.status.flash, "Copied") {
			t.Errorf("flash = %q, want substring 'Copied'", rm.status.flash)
		}
		if cmd == nil {
			t.Fatal("expected non-nil cmd (SetClipboard + flash)")
		}
	})

	t.Run("uses focused pane version for Different status", func(t *testing.T) {
		leftSrv := manifest.MCPServer{Command: "left-cmd"}
		rightSrv := manifest.MCPServer{Command: "right-cmd"}

		m := newTUIModel(Options{})
		m.ready = true
		m.focusLeft = false // focus right
		m.right.items = []PanelItem{
			{Section: SectionMCP, Name: "diff-server", SyncStatus: Different},
		}
		m.right.cursor = 0
		m.compState = &engine.SyncState{
			MCPSync: map[string]engine.ItemSync{
				"diff-server": {Status: engine.Different, Left: leftSrv, Right: rightSrv},
			},
			PlugSync: map[string]engine.ItemSync{},
			MktSync:  map[string]engine.ItemSync{},
		}

		result, cmd := m.handleCopyJSON()
		rm := result.(tuiModel)

		if !strings.Contains(rm.status.flash, "Copied") {
			t.Errorf("flash = %q, want substring 'Copied'", rm.status.flash)
		}
		if cmd == nil {
			t.Fatal("expected non-nil cmd")
		}
	})

	t.Run("JSON output format", func(t *testing.T) {
		// Verify the JSON serialization directly.
		wrapped := map[string]manifest.MCPServer{"test-srv": srv}
		data, err := json.MarshalIndent(wrapped, "", "  ")
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		jsonStr := string(data)

		// Should contain the server name as a key.
		if !strings.Contains(jsonStr, `"test-srv"`) {
			t.Errorf("JSON should contain server name, got: %s", jsonStr)
		}
		// Should contain the command.
		if !strings.Contains(jsonStr, `"npx"`) {
			t.Errorf("JSON should contain command, got: %s", jsonStr)
		}
		// Should contain the args.
		if !strings.Contains(jsonStr, `@modelcontextprotocol/server-filesystem`) {
			t.Errorf("JSON should contain args, got: %s", jsonStr)
		}
		// Should have env.
		if !strings.Contains(jsonStr, `"HOME"`) {
			t.Errorf("JSON should contain env keys, got: %s", jsonStr)
		}
	})
}

// --- handleEnable / handleDisable ---

func TestHandleEnable(t *testing.T) {
	t.Run("no item selected shows flash", func(t *testing.T) {
		m := newTUIModel(Options{})
		m.ready = true
		m.compState = &engine.SyncState{
			MCPSync:  map[string]engine.ItemSync{},
			PlugSync: map[string]engine.ItemSync{},
			MktSync:  map[string]engine.ItemSync{},
		}

		result, _ := m.handleEnable()
		rm := result.(tuiModel)

		if !strings.Contains(rm.status.flash, "No item selected") {
			t.Errorf("flash = %q, want 'No item selected'", rm.status.flash)
		}
	})

	t.Run("non-plugin item shows flash", func(t *testing.T) {
		m := newTUIModel(Options{})
		m.ready = true
		m.focusLeft = true
		m.left.items = []PanelItem{
			{Section: SectionMCP, Name: "my-server"},
		}
		m.left.cursor = 0
		m.compState = &engine.SyncState{
			MCPSync: map[string]engine.ItemSync{
				"my-server": {Status: engine.Synced, Left: manifest.MCPServer{Command: "cmd"}},
			},
			PlugSync: map[string]engine.ItemSync{},
			MktSync:  map[string]engine.ItemSync{},
		}

		result, _ := m.handleEnable()
		rm := result.(tuiModel)

		if !strings.Contains(rm.status.flash, "only available for plugins") {
			t.Errorf("flash = %q, want 'only available for plugins'", rm.status.flash)
		}
	})

	t.Run("enable disabled plugin dispatches command", func(t *testing.T) {
		m := newTUIModel(Options{})
		m.ready = true
		m.focusLeft = true
		m.leftSource = PaneSource{Kind: SourceManifest}
		m.left.items = []PanelItem{
			{Section: SectionPlugin, Name: "my-plugin"},
		}
		m.left.cursor = 0
		m.compState = &engine.SyncState{
			MCPSync: map[string]engine.ItemSync{},
			PlugSync: map[string]engine.ItemSync{
				"my-plugin": {Status: engine.Synced, Left: manifest.Plugin{ID: "my-plugin", Enabled: false}},
			},
			MktSync: map[string]engine.ItemSync{},
		}

		result, cmd := m.handleEnable()
		rm := result.(tuiModel)

		if !rm.busy {
			t.Error("busy should be set")
		}
		if cmd == nil {
			t.Error("expected non-nil cmd")
		}
	})

	t.Run("enable already-enabled plugin shows flash", func(t *testing.T) {
		m := newTUIModel(Options{})
		m.ready = true
		m.focusLeft = true
		m.leftSource = PaneSource{Kind: SourceManifest}
		m.left.items = []PanelItem{
			{Section: SectionPlugin, Name: "my-plugin"},
		}
		m.left.cursor = 0
		m.compState = &engine.SyncState{
			MCPSync: map[string]engine.ItemSync{},
			PlugSync: map[string]engine.ItemSync{
				"my-plugin": {Status: engine.Synced, Left: manifest.Plugin{ID: "my-plugin", Enabled: true}},
			},
			MktSync: map[string]engine.ItemSync{},
		}

		result, _ := m.handleEnable()
		rm := result.(tuiModel)

		if rm.busy {
			t.Error("busy should NOT be set for already-enabled plugin")
		}
		if !strings.Contains(rm.status.flash, "already enabled") {
			t.Errorf("flash = %q, want substring 'already enabled'", rm.status.flash)
		}
	})

	t.Run("focused pane nil plugin returns flash", func(t *testing.T) {
		m := newTUIModel(Options{})
		m.ready = true
		m.focusLeft = true
		m.leftSource = PaneSource{Kind: SourceManifest}
		m.rightSource = PaneSource{Kind: SourceClaudeCode}
		m.left.items = []PanelItem{
			{Section: SectionPlugin, Name: "my-plugin"},
		}
		m.left.cursor = 0
		m.compState = &engine.SyncState{
			MCPSync: map[string]engine.ItemSync{},
			PlugSync: map[string]engine.ItemSync{
				"my-plugin": {Status: engine.RightOnly, Left: nil, Right: manifest.Plugin{ID: "my-plugin", Enabled: true}},
			},
			MktSync: map[string]engine.ItemSync{},
		}

		result, _ := m.handleEnable()
		rm := result.(tuiModel)

		if rm.busy {
			t.Error("busy should NOT be set when focused pane has no plugin")
		}
		if !strings.Contains(rm.status.flash, "not available") {
			t.Errorf("flash = %q, want flash containing 'not available'", rm.status.flash)
		}
	})
}

func TestHandleDisable(t *testing.T) {
	t.Run("disable enabled plugin dispatches command", func(t *testing.T) {
		m := newTUIModel(Options{})
		m.ready = true
		m.focusLeft = true
		m.leftSource = PaneSource{Kind: SourceManifest}
		m.left.items = []PanelItem{
			{Section: SectionPlugin, Name: "my-plugin"},
		}
		m.left.cursor = 0
		m.compState = &engine.SyncState{
			MCPSync: map[string]engine.ItemSync{},
			PlugSync: map[string]engine.ItemSync{
				"my-plugin": {Status: engine.Synced, Left: manifest.Plugin{ID: "my-plugin", Enabled: true}},
			},
			MktSync: map[string]engine.ItemSync{},
		}

		result, cmd := m.handleDisable()
		rm := result.(tuiModel)

		if !rm.busy {
			t.Error("busy should be set")
		}
		if cmd == nil {
			t.Error("expected non-nil cmd")
		}
	})

	t.Run("disable already-disabled plugin shows flash", func(t *testing.T) {
		m := newTUIModel(Options{})
		m.ready = true
		m.focusLeft = true
		m.leftSource = PaneSource{Kind: SourceManifest}
		m.left.items = []PanelItem{
			{Section: SectionPlugin, Name: "my-plugin"},
		}
		m.left.cursor = 0
		m.compState = &engine.SyncState{
			MCPSync: map[string]engine.ItemSync{},
			PlugSync: map[string]engine.ItemSync{
				"my-plugin": {Status: engine.Synced, Left: manifest.Plugin{ID: "my-plugin", Enabled: false}},
			},
			MktSync: map[string]engine.ItemSync{},
		}

		result, _ := m.handleDisable()
		rm := result.(tuiModel)

		if rm.busy {
			t.Error("busy should NOT be set for already-disabled plugin")
		}
		if !strings.Contains(rm.status.flash, "already disabled") {
			t.Errorf("flash = %q, want substring 'already disabled'", rm.status.flash)
		}
	})

	t.Run("right pane disable dispatches command", func(t *testing.T) {
		m := newTUIModel(Options{})
		m.ready = true
		m.focusLeft = false
		m.rightSource = PaneSource{Kind: SourceClaudeCode}
		m.right.items = []PanelItem{
			{Section: SectionPlugin, Name: "my-plugin"},
		}
		m.right.cursor = 0
		m.compState = &engine.SyncState{
			MCPSync: map[string]engine.ItemSync{},
			PlugSync: map[string]engine.ItemSync{
				"my-plugin": {Status: engine.Synced, Right: manifest.Plugin{ID: "my-plugin", Enabled: true}},
			},
			MktSync: map[string]engine.ItemSync{},
		}

		result, cmd := m.handleDisable()
		rm := result.(tuiModel)

		if !rm.busy {
			t.Error("busy should be set")
		}
		if cmd == nil {
			t.Error("expected non-nil cmd")
		}
	})
}

// --- handleQuit (chezmoi integration) ---

func TestHandleQuit(t *testing.T) {
	t.Run("not dirty quits immediately", func(t *testing.T) {
		m := newTUIModel(Options{})
		m.chezmoiMode = "ask"
		m.manifestDirty = false

		_, cmd := m.handleQuit()
		if cmd == nil {
			t.Fatal("expected non-nil cmd (tea.Quit)")
		}
	})

	t.Run("never mode quits even when dirty", func(t *testing.T) {
		m := newTUIModel(Options{})
		m.chezmoiMode = "never"
		m.manifestDirty = true

		_, cmd := m.handleQuit()
		if cmd == nil {
			t.Fatal("expected non-nil cmd (tea.Quit)")
		}
	})

	t.Run("empty mode (default) quits even when dirty", func(t *testing.T) {
		m := newTUIModel(Options{})
		m.manifestDirty = true

		_, cmd := m.handleQuit()
		if cmd == nil {
			t.Fatal("expected non-nil cmd (tea.Quit)")
		}
	})

	t.Run("ask mode shows prompt when dirty", func(t *testing.T) {
		m := newTUIModel(Options{})
		m.chezmoiMode = "ask"
		m.manifestDirty = true

		result, cmd := m.handleQuit()
		rm := result.(tuiModel)

		if !rm.chezmoiPrompting {
			t.Error("expected chezmoiPrompting to be true")
		}
		if cmd != nil {
			t.Error("expected nil cmd (no quit)")
		}
	})

	t.Run("always mode starts re-add when dirty", func(t *testing.T) {
		m := newTUIModel(Options{
			ChezmoiReAdd: func(_ context.Context, _ string) error {
				return nil
			},
		})
		m.chezmoiMode = "always"
		m.manifestDirty = true

		result, cmd := m.handleQuit()
		rm := result.(tuiModel)

		if !rm.chezmoiRunning {
			t.Error("expected chezmoiRunning to be true")
		}
		if rm.chezmoiPrompting {
			t.Error("expected chezmoiPrompting to be false")
		}
		if cmd == nil {
			t.Error("expected non-nil cmd (chezmoi re-add)")
		}
	})
}

func TestHandleChezmoiDone(t *testing.T) {
	t.Run("success quits", func(t *testing.T) {
		m := newTUIModel(Options{})
		m.chezmoiRunning = true

		result, cmd := m.handleChezmoiDone(ChezmoiDoneMsg{Err: nil})
		rm := result.(tuiModel)

		if rm.chezmoiRunning {
			t.Error("chezmoiRunning should be cleared")
		}
		if cmd == nil {
			t.Fatal("expected non-nil cmd (tea.Quit)")
		}
	})

	t.Run("error shows alert and sets quit pending", func(t *testing.T) {
		m := newTUIModel(Options{})
		m.chezmoiRunning = true

		result, cmd := m.handleChezmoiDone(ChezmoiDoneMsg{Err: fmt.Errorf("chezmoi not found")})
		rm := result.(tuiModel)

		if rm.chezmoiRunning {
			t.Error("chezmoiRunning should be cleared")
		}
		if !rm.chezmoiQuitPending {
			t.Error("chezmoiQuitPending should be true")
		}
		if !rm.alert.Active() {
			t.Error("alert should be active")
		}
		if !strings.Contains(rm.alert.title, "chezmoi") {
			t.Errorf("alert.title = %q, want substring 'chezmoi'", rm.alert.title)
		}
		if cmd != nil {
			t.Error("expected nil cmd (waiting for alert dismiss)")
		}
	})
}

func TestHandleChezmoiPromptKey(t *testing.T) {
	t.Run("tab toggles focus", func(t *testing.T) {
		m := newTUIModel(Options{})
		m.chezmoiPrompting = true
		m.chezmoiFocusRun = false

		result, _ := m.handleChezmoiPromptKey(tea.KeyPressMsg{Code: tea.KeyTab})
		rm := result.(tuiModel)

		if !rm.chezmoiFocusRun {
			t.Error("expected focus to toggle to Run")
		}
	})

	t.Run("esc cancels and quits", func(t *testing.T) {
		m := newTUIModel(Options{})
		m.chezmoiPrompting = true

		result, cmd := m.handleChezmoiPromptKey(tea.KeyPressMsg{Code: tea.KeyEscape})
		rm := result.(tuiModel)

		if rm.chezmoiPrompting {
			t.Error("chezmoiPrompting should be cleared")
		}
		if cmd == nil {
			t.Fatal("expected non-nil cmd (tea.Quit)")
		}
	})

	t.Run("enter on Cancel quits without chezmoi", func(t *testing.T) {
		m := newTUIModel(Options{})
		m.chezmoiPrompting = true
		m.chezmoiFocusRun = false

		result, cmd := m.handleChezmoiPromptKey(tea.KeyPressMsg{Code: tea.KeyEnter})
		rm := result.(tuiModel)

		if rm.chezmoiPrompting {
			t.Error("chezmoiPrompting should be cleared")
		}
		if cmd == nil {
			t.Fatal("expected non-nil cmd (tea.Quit)")
		}
	})

	t.Run("enter on Run starts chezmoi re-add", func(t *testing.T) {
		m := newTUIModel(Options{
			ChezmoiReAdd: func(_ context.Context, _ string) error {
				return nil
			},
		})
		m.chezmoiPrompting = true
		m.chezmoiFocusRun = true

		result, cmd := m.handleChezmoiPromptKey(tea.KeyPressMsg{Code: tea.KeyEnter})
		rm := result.(tuiModel)

		if rm.chezmoiPrompting {
			t.Error("chezmoiPrompting should be cleared")
		}
		if !rm.chezmoiRunning {
			t.Error("chezmoiRunning should be true")
		}
		if cmd == nil {
			t.Error("expected non-nil cmd (chezmoi re-add)")
		}
	})
}

// --- anyOverlayActive with chezmoi states ---

func TestAnyOverlayActive_Chezmoi(t *testing.T) {
	t.Run("chezmoiPrompting is overlay", func(t *testing.T) {
		m := newTUIModel(Options{})
		m.chezmoiPrompting = true
		if !m.anyOverlayActive() {
			t.Error("chezmoiPrompting should count as active overlay")
		}
	})

	t.Run("chezmoiRunning is overlay", func(t *testing.T) {
		m := newTUIModel(Options{})
		m.chezmoiRunning = true
		if !m.anyOverlayActive() {
			t.Error("chezmoiRunning should count as active overlay")
		}
	})
}

// --- handleToggleResult ---

func TestHandleToggleResult(t *testing.T) {
	tests := []struct {
		name              string
		msg               ToggleResultMsg
		wantFlash         string
		wantAlert         string
		checkFlashStyle   bool
		wantFlashStyle    FlashStyle
		wantManifestDirty bool
	}{
		{
			name:              "success",
			msg:               ToggleResultMsg{Name: "my-plugin", ManifestDirty: true},
			wantFlash:         "Toggled my-plugin",
			checkFlashStyle:   true,
			wantFlashStyle:    FlashSuccess,
			wantManifestDirty: true,
		},
		{
			name:      "error",
			msg:       ToggleResultMsg{Name: "my-plugin", Err: fmt.Errorf("not found")},
			wantAlert: "Toggle failed",
		},
		{
			name:      "claude code toggle (not manifest dirty)",
			msg:       ToggleResultMsg{Name: "my-plugin"},
			wantFlash: "Toggled my-plugin",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newTUIModel(Options{})
			m.busy = true

			result, _ := m.handleToggleResult(tt.msg)
			rm := result.(tuiModel)

			if rm.busy {
				t.Error("busy should be cleared")
			}
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
			if tt.checkFlashStyle && rm.status.flashStyle != tt.wantFlashStyle {
				t.Errorf("flashStyle = %v, want %v", rm.status.flashStyle, tt.wantFlashStyle)
			}
			if rm.manifestDirty != tt.wantManifestDirty {
				t.Errorf("manifestDirty = %v, want %v", rm.manifestDirty, tt.wantManifestDirty)
			}
		})
	}
}
