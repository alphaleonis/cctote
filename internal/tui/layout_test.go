package tui

import "testing"

func TestComputeLayout(t *testing.T) {
	tests := []struct {
		name         string
		width        int
		height       int
		helpHeight   int
		wantMode     LayoutMode
		wantDetail   int
		wantMinPanel int // minimum expected panel height
	}{
		{
			name:         "dual pane with full detail",
			width:        120,
			height:       30,
			wantMode:     LayoutDual,
			wantDetail:   5,
			wantMinPanel: 3,
		},
		{
			name:         "dual pane with compact detail",
			width:        100,
			height:       20,
			wantMode:     LayoutDual,
			wantDetail:   2,
			wantMinPanel: 3,
		},
		{
			name:         "dual pane no detail",
			width:        100,
			height:       14,
			wantMode:     LayoutDual,
			wantDetail:   0,
			wantMinPanel: 3,
		},
		{
			name:         "single pane",
			width:        80,
			height:       30,
			wantMode:     LayoutSingle,
			wantDetail:   5,
			wantMinPanel: 3,
		},
		{
			name:       "too narrow",
			width:      50,
			height:     30,
			wantMode:   LayoutTooNarrow,
			wantDetail: 0,
		},
		{
			name:         "boundary dual pane",
			width:        100,
			height:       24,
			wantMode:     LayoutDual,
			wantDetail:   5,
			wantMinPanel: 3,
		},
		{
			name:         "boundary single pane",
			width:        60,
			height:       24,
			wantMode:     LayoutSingle,
			wantDetail:   5,
			wantMinPanel: 3,
		},
		{
			name:         "help bar reduces panel height",
			width:        120,
			height:       30,
			helpHeight:   7,
			wantMode:     LayoutDual,
			wantDetail:   5,
			wantMinPanel: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := ComputeLayout(tt.width, tt.height, tt.helpHeight)

			if l.Mode != tt.wantMode {
				t.Errorf("Mode = %d, want %d", l.Mode, tt.wantMode)
			}
			if l.DetailHeight != tt.wantDetail {
				t.Errorf("DetailHeight = %d, want %d", l.DetailHeight, tt.wantDetail)
			}
			if tt.wantMode != LayoutTooNarrow && l.PanelHeight < tt.wantMinPanel {
				t.Errorf("PanelHeight = %d, want >= %d", l.PanelHeight, tt.wantMinPanel)
			}
			if l.TotalWidth != tt.width {
				t.Errorf("TotalWidth = %d, want %d", l.TotalWidth, tt.width)
			}
			if l.TotalHeight != tt.height {
				t.Errorf("TotalHeight = %d, want %d", l.TotalHeight, tt.height)
			}
			if l.HelpHeight != tt.helpHeight {
				t.Errorf("HelpHeight = %d, want %d", l.HelpHeight, tt.helpHeight)
			}

			// In dual mode, panel width should be roughly half.
			if l.Mode == LayoutDual {
				expected := (tt.width - 1) / 2
				if l.PanelWidth != expected {
					t.Errorf("PanelWidth = %d, want %d", l.PanelWidth, expected)
				}
			}

			// When help is active, panel height should be smaller.
			if tt.helpHeight > 0 && tt.wantMode != LayoutTooNarrow {
				noHelp := ComputeLayout(tt.width, tt.height, 0)
				if l.PanelHeight >= noHelp.PanelHeight {
					t.Errorf("PanelHeight with help (%d) should be less than without (%d)",
						l.PanelHeight, noHelp.PanelHeight)
				}
			}
		})
	}
}
