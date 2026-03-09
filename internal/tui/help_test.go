package tui

import (
	"strings"
	"testing"
)

func TestHelpBarHeight(t *testing.T) {
	tests := []struct {
		name   string
		groups []HelpGroup
		want   int
	}{
		{
			name:   "no groups",
			groups: nil,
			want:   0,
		},
		{
			name: "single group with 2 bindings",
			groups: []HelpGroup{{
				Name: "Filter",
				Bindings: []HelpBinding{
					{Binding: DefaultKeyMap().Confirm},
					{Binding: DefaultKeyMap().Cancel},
				},
			}},
			want: 4, // 1 rule + 1 header + 2 bindings
		},
		{
			name:   "normal mode 4 groups with up to 8 bindings",
			groups: DefaultKeyMap().ContextualHelp(HelpContext{Ready: true}),
			want:   10, // 1 rule + 1 header + 8 bindings (Actions group is tallest)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := HelpBarHeight(tt.groups)
			if got != tt.want {
				t.Errorf("HelpBarHeight() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestContextualHelp_NormalMode(t *testing.T) {
	keys := DefaultKeyMap()
	groups := keys.ContextualHelp(HelpContext{Ready: true, HasProfile: true})

	if len(groups) != 4 {
		t.Fatalf("expected 4 groups, got %d", len(groups))
	}

	names := []string{"Navigation", "Actions", "Sources", "General"}
	for i, want := range names {
		if groups[i].Name != want {
			t.Errorf("group[%d].Name = %q, want %q", i, groups[i].Name, want)
		}
	}

	// When ready and not busy, no bindings should be dimmed.
	for _, g := range groups {
		for _, b := range g.Bindings {
			if b.Dimmed {
				t.Errorf("binding %q in %q should not be dimmed when ready",
					b.Binding.Help().Key, g.Name)
			}
		}
	}
}

func TestContextualHelp_NormalBusy(t *testing.T) {
	keys := DefaultKeyMap()
	groups := keys.ContextualHelp(HelpContext{Ready: true, Busy: true})

	// Actions and Sources should be dimmed.
	for _, g := range groups {
		if g.Name == "Actions" || g.Name == "Sources" {
			for _, b := range g.Bindings {
				if !b.Dimmed {
					t.Errorf("binding %q in %q should be dimmed when busy",
						b.Binding.Help().Key, g.Name)
				}
			}
		}
	}

	// Navigation and General should NOT be dimmed.
	for _, g := range groups {
		if g.Name == "Navigation" || g.Name == "General" {
			for _, b := range g.Bindings {
				if b.Dimmed {
					t.Errorf("binding %q in %q should not be dimmed when busy",
						b.Binding.Help().Key, g.Name)
				}
			}
		}
	}
}

func TestContextualHelp_FilterActive(t *testing.T) {
	keys := DefaultKeyMap()
	groups := keys.ContextualHelp(HelpContext{FilterActive: true, Ready: true})

	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
	if groups[0].Name != "Filter" {
		t.Errorf("group name = %q, want %q", groups[0].Name, "Filter")
	}
	if len(groups[0].Bindings) != 2 {
		t.Errorf("expected 2 bindings, got %d", len(groups[0].Bindings))
	}
}

func TestContextualHelp_DetailExpanded(t *testing.T) {
	keys := DefaultKeyMap()
	groups := keys.ContextualHelp(HelpContext{DetailExpanded: true, Ready: true})

	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
	if groups[0].Name != "Detail" {
		t.Errorf("group name = %q, want %q", groups[0].Name, "Detail")
	}
	if len(groups[0].Bindings) != 8 {
		t.Errorf("expected 8 bindings, got %d", len(groups[0].Bindings))
	}
}

func TestContextualHelp_OverlayActive(t *testing.T) {
	keys := DefaultKeyMap()
	groups := keys.ContextualHelp(HelpContext{OverlayActive: true, Ready: true})

	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
	if groups[0].Name != "Overlay" {
		t.Errorf("group name = %q, want %q", groups[0].Name, "Overlay")
	}
	if len(groups[0].Bindings) != 2 {
		t.Errorf("expected 2 bindings, got %d", len(groups[0].Bindings))
	}
}

func TestContextualHelp_DeleteProfileDimmedWithoutProfile(t *testing.T) {
	keys := DefaultKeyMap()
	groups := keys.ContextualHelp(HelpContext{Ready: true, HasProfile: false})

	// Find DeleteProfile in Sources group.
	var found bool
	for _, g := range groups {
		if g.Name == "Sources" {
			for _, b := range g.Bindings {
				if b.Binding.Help().Key == keys.DeleteProfile.Help().Key {
					found = true
					if !b.Dimmed {
						t.Error("DeleteProfile should be dimmed without active profile")
					}
				}
			}
		}
	}
	if !found {
		t.Error("DeleteProfile binding not found in Sources group")
	}
}

func TestHelpBar_Render(t *testing.T) {
	keys := DefaultKeyMap()
	groups := keys.ContextualHelp(HelpContext{Ready: true})
	result := HelpBar(groups, 120)

	if result == "" {
		t.Fatal("expected non-empty help bar")
	}

	// Should contain the horizontal rule.
	if !strings.Contains(result, "─") {
		t.Error("help bar should contain horizontal rule")
	}

	// Should contain group names.
	for _, name := range []string{"Navigation", "Actions", "Sources", "General"} {
		if !strings.Contains(result, name) {
			t.Errorf("help bar should contain group name %q", name)
		}
	}

	// Should contain some key labels.
	for _, key := range []string{"↑/k", "↓/j", "tab"} {
		if !strings.Contains(result, key) {
			t.Errorf("help bar should contain key label %q", key)
		}
	}
}

func TestHelpBar_EmptyGroups(t *testing.T) {
	result := HelpBar(nil, 120)
	if result != "" {
		t.Errorf("expected empty string for nil groups, got %q", result)
	}
}

func TestHelpBar_NarrowWidth(t *testing.T) {
	result := HelpBar(nil, 5)
	if result != "" {
		t.Errorf("expected empty string for narrow width, got %q", result)
	}
}
