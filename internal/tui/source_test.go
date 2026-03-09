package tui

import (
	"strings"
	"testing"

	"github.com/alphaleonis/cctote/internal/engine"
	"github.com/alphaleonis/cctote/internal/manifest"
)

// --- ResolveOp ---

func TestResolveOp_AllCombinations(t *testing.T) {
	man := PaneSource{Kind: SourceManifest}
	cc := PaneSource{Kind: SourceClaudeCode}
	prof := PaneSource{Kind: SourceProfile, ProfileName: "dev"}
	profB := PaneSource{Kind: SourceProfile, ProfileName: "staging"}
	proj := PaneSource{Kind: SourceProject}

	tests := []struct {
		name string
		from PaneSource
		to   PaneSource
		want ResolvedOp
	}{
		{"ClaudeCode→Manifest", cc, man, OpExportToManifest},
		{"Manifest→ClaudeCode", man, cc, OpImportToClaude},
		{"ClaudeCode→Profile", cc, prof, OpExportAndAddProfile},
		{"Manifest→Profile", man, prof, OpAddToProfile},
		{"Profile→ClaudeCode", prof, cc, OpImportToClaude},
		{"Profile→Manifest", prof, man, OpInvalid},
		{"Profile→Profile", prof, profB, OpCopyProfileRef},
		{"Manifest→Manifest", man, man, OpInvalid},
		{"ClaudeCode→ClaudeCode", cc, cc, OpInvalid},
		{"Project→Manifest", proj, man, OpExportProjectToManifest},
		{"Project→ClaudeCode", proj, cc, OpImportToClaude},
		{"Project→Profile", proj, prof, OpExportAndAddProfile},
		{"Manifest→Project", man, proj, OpImportToProject},
		{"ClaudeCode→Project", cc, proj, OpImportToProject},
		{"Profile→Project", prof, proj, OpImportToProject},
		{"Project→Project", proj, proj, OpInvalid},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveOp(tt.from, tt.to)
			if got != tt.want {
				t.Errorf("ResolveOp(%s, %s) = %d, want %d", tt.from.Label(), tt.to.Label(), got, tt.want)
			}
		})
	}
}

// --- ResolveDeleteOp ---

func TestResolveDeleteOp_AllKinds(t *testing.T) {
	tests := []struct {
		name    string
		focused PaneSource
		want    ResolvedOp
	}{
		{"Manifest", PaneSource{Kind: SourceManifest}, OpDeleteFromManifest},
		{"Profile", PaneSource{Kind: SourceProfile, ProfileName: "dev"}, OpRemoveFromProfile},
		{"ClaudeCode", PaneSource{Kind: SourceClaudeCode}, OpRemoveFromClaude},
		{"Project", PaneSource{Kind: SourceProject}, OpDeleteFromProject},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveDeleteOp(tt.focused)
			if got != tt.want {
				t.Errorf("ResolveDeleteOp(%s) = %d, want %d", tt.focused.Label(), got, tt.want)
			}
		})
	}
}

// --- ExtractSourceData ---

func TestExtractSourceData_Manifest(t *testing.T) {
	m := &manifest.Manifest{
		Version: 1,
		MCPServers: map[string]manifest.MCPServer{
			"srv": {Command: "cmd"},
		},
		Plugins:      []manifest.Plugin{{ID: "plug-a", Enabled: true}},
		Marketplaces: map[string]manifest.Marketplace{"mkt": {Source: "github", Repo: "org/repo"}},
	}
	full := &engine.FullState{Manifest: m}

	sd := ExtractSourceData(PaneSource{Kind: SourceManifest}, full)
	if len(sd.MCP) != 1 {
		t.Errorf("MCP count = %d, want 1", len(sd.MCP))
	}
	if len(sd.Plugins) != 1 {
		t.Errorf("Plugins count = %d, want 1", len(sd.Plugins))
	}
	if len(sd.Marketplaces) != 1 {
		t.Errorf("Marketplaces count = %d, want 1", len(sd.Marketplaces))
	}
}

func TestExtractSourceData_ManifestNil(t *testing.T) {
	full := &engine.FullState{Manifest: nil}
	sd := ExtractSourceData(PaneSource{Kind: SourceManifest}, full)
	if sd.MCP != nil || sd.Plugins != nil || sd.Marketplaces != nil {
		t.Error("expected empty SourceData for nil manifest")
	}
}

func TestExtractSourceData_ClaudeCode(t *testing.T) {
	full := &engine.FullState{
		MCPInstalled:  map[string]manifest.MCPServer{"srv": {Command: "cmd"}},
		PlugInstalled: []manifest.Plugin{{ID: "p1"}},
		MktInstalled:  map[string]manifest.Marketplace{"mkt": {Source: "github"}},
	}

	sd := ExtractSourceData(PaneSource{Kind: SourceClaudeCode}, full)
	if len(sd.MCP) != 1 {
		t.Errorf("MCP count = %d, want 1", len(sd.MCP))
	}
	if len(sd.Plugins) != 1 {
		t.Errorf("Plugins count = %d, want 1", len(sd.Plugins))
	}
	if len(sd.Marketplaces) != 1 {
		t.Errorf("Marketplaces count = %d, want 1", len(sd.Marketplaces))
	}
}

func TestExtractSourceData_Profile(t *testing.T) {
	m := &manifest.Manifest{
		Version: 1,
		MCPServers: map[string]manifest.MCPServer{
			"srv-a": {Command: "a"},
			"srv-b": {Command: "b"},
		},
		Plugins: []manifest.Plugin{
			{ID: "plug-1", Enabled: true},
			{ID: "plug-2", Enabled: false},
		},
		Profiles: map[string]manifest.Profile{
			"dev": {
				MCPServers: []string{"srv-a"},
				Plugins:    []manifest.ProfilePlugin{{ID: "plug-1"}},
			},
		},
	}
	full := &engine.FullState{Manifest: m}

	sd := ExtractSourceData(PaneSource{Kind: SourceProfile, ProfileName: "dev"}, full)
	if len(sd.MCP) != 1 {
		t.Errorf("MCP count = %d, want 1 (only referenced servers)", len(sd.MCP))
	}
	if _, ok := sd.MCP["srv-a"]; !ok {
		t.Error("expected srv-a in profile source data")
	}
	if len(sd.Plugins) != 1 {
		t.Errorf("Plugins count = %d, want 1", len(sd.Plugins))
	}
	if sd.Marketplaces != nil {
		t.Error("profiles should not include marketplaces")
	}
}

func TestExtractSourceData_ProfileNotFound(t *testing.T) {
	m := &manifest.Manifest{Version: 1, Profiles: map[string]manifest.Profile{}}
	full := &engine.FullState{Manifest: m}

	sd := ExtractSourceData(PaneSource{Kind: SourceProfile, ProfileName: "missing"}, full)
	if sd.MCP != nil || sd.Plugins != nil {
		t.Error("expected empty SourceData for missing profile")
	}
}

// --- SourceProject ---

func TestExtractSourceData_Project(t *testing.T) {
	full := &engine.FullState{
		ProjectMCP: map[string]manifest.MCPServer{
			"proj-srv": {Command: "node", Args: []string{"server.js"}},
		},
		ProjectPlugins: []manifest.Plugin{
			{ID: "proj-plug", Scope: "project", Enabled: true},
		},
		ProjectRoot: "/tmp/myrepo",
	}

	sd := ExtractSourceData(PaneSource{Kind: SourceProject}, full)
	if len(sd.MCP) != 1 {
		t.Errorf("MCP count = %d, want 1", len(sd.MCP))
	}
	if _, ok := sd.MCP["proj-srv"]; !ok {
		t.Error("expected proj-srv in project source data")
	}
	if len(sd.Plugins) != 1 {
		t.Errorf("Plugins count = %d, want 1", len(sd.Plugins))
	}
	if sd.Marketplaces != nil {
		t.Error("projects should not include marketplaces")
	}
}

func TestExtractSourceData_ProjectEmpty(t *testing.T) {
	full := &engine.FullState{
		ProjectMCP:     map[string]manifest.MCPServer{},
		ProjectPlugins: nil,
		ProjectRoot:    "/tmp/myrepo",
	}

	sd := ExtractSourceData(PaneSource{Kind: SourceProject}, full)
	if len(sd.MCP) != 0 {
		t.Errorf("MCP count = %d, want 0", len(sd.MCP))
	}
	if sd.Plugins != nil {
		t.Error("expected nil plugins for empty project")
	}
}

func TestExtractSourceData_ProjectNilState(t *testing.T) {
	sd := ExtractSourceData(PaneSource{Kind: SourceProject}, nil)
	if sd.MCP != nil || sd.Plugins != nil || sd.Marketplaces != nil {
		t.Error("expected empty SourceData for nil state")
	}
}

// --- New op method tests ---

func TestOpMethods_ExportProjectToManifest(t *testing.T) {
	op := OpExportProjectToManifest
	if op.Label() != "Export" {
		t.Errorf("Label = %q, want %q", op.Label(), "Export")
	}
	if op.PastTense() != "Exported" {
		t.Errorf("PastTense = %q, want %q", op.PastTense(), "Exported")
	}
	if op.IsDangerous() {
		t.Error("IsDangerous should be false")
	}
	if !op.ModifiesManifest() {
		t.Error("ModifiesManifest should be true")
	}
	if op.TargetsProfile() {
		t.Error("TargetsProfile should be false")
	}
}

func TestOpMethods_ImportToProject(t *testing.T) {
	op := OpImportToProject
	if op.Label() != "Copy to project" {
		t.Errorf("Label = %q, want %q", op.Label(), "Copy to project")
	}
	if op.PastTense() != "Copied to project" {
		t.Errorf("PastTense = %q, want %q", op.PastTense(), "Copied to project")
	}
	if op.IsDangerous() {
		t.Error("IsDangerous should be false")
	}
	if op.ModifiesManifest() {
		t.Error("ModifiesManifest should be false")
	}
	if op.TargetsProfile() {
		t.Error("TargetsProfile should be false")
	}
}

func TestOpMethods_DeleteFromProject(t *testing.T) {
	op := OpDeleteFromProject
	if op.Label() != "Remove" {
		t.Errorf("Label = %q, want %q", op.Label(), "Remove")
	}
	if op.PastTense() != "Removed" {
		t.Errorf("PastTense = %q, want %q", op.PastTense(), "Removed")
	}
	if !op.IsDangerous() {
		t.Error("IsDangerous should be true")
	}
	if op.ModifiesManifest() {
		t.Error("ModifiesManifest should be false")
	}
	if op.TargetsProfile() {
		t.Error("TargetsProfile should be false")
	}
}

func TestInvokesCLI(t *testing.T) {
	cliOps := []ResolvedOp{OpImportToClaude, OpRemoveFromClaude, OpImportToProject, OpDeleteFromProject}
	for _, op := range cliOps {
		if !op.InvokesCLI() {
			t.Errorf("%v.InvokesCLI() = false, want true", op)
		}
	}
	nonCLI := []ResolvedOp{OpExportToManifest, OpAddToProfile, OpDeleteFromManifest, OpRemoveFromProfile, OpCopyProfileRef, OpExportAndAddProfile}
	for _, op := range nonCLI {
		if op.InvokesCLI() {
			t.Errorf("%v.InvokesCLI() = true, want false", op)
		}
	}
}

func TestProgressLabel(t *testing.T) {
	tests := []struct {
		op   ResolvedOp
		want string
	}{
		{OpImportToClaude, "Importing"},
		{OpRemoveFromClaude, "Uninstalling"},
		{OpImportToProject, "Copying to project:"},
		{OpDeleteFromProject, "Removing from project:"},
	}
	for _, tt := range tests {
		if got := tt.op.ProgressLabel(); got != tt.want {
			t.Errorf("%v.ProgressLabel() = %q, want %q", tt.op, got, tt.want)
		}
	}
}

// --- hasMCPItems ---

func TestHasMCPItems(t *testing.T) {
	tests := []struct {
		name  string
		items []CopyItem
		want  bool
	}{
		{"nil", nil, false},
		{"empty", []CopyItem{}, false},
		{"mcp only", []CopyItem{{Section: SectionMCP, Name: "srv"}}, true},
		{"plugin only", []CopyItem{{Section: SectionPlugin, Name: "plug"}}, false},
		{"marketplace only", []CopyItem{{Section: SectionMarketplace, Name: "mkt"}}, false},
		{"mixed with mcp", []CopyItem{
			{Section: SectionPlugin, Name: "plug"},
			{Section: SectionMCP, Name: "srv"},
		}, true},
		{"mixed without mcp", []CopyItem{
			{Section: SectionPlugin, Name: "plug"},
			{Section: SectionMarketplace, Name: "mkt"},
		}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hasMCPItems(tt.items)
			if got != tt.want {
				t.Errorf("hasMCPItems() = %v, want %v", got, tt.want)
			}
		})
	}
}

// --- PaneSource.Label / Icon ---

func TestPaneSourceLabel_Project(t *testing.T) {
	s := PaneSource{Kind: SourceProject}
	if got := s.Label(); got != "Project" {
		t.Errorf("Label = %q, want %q", got, "Project")
	}
	if got := s.Icon(); got != IconProject {
		t.Errorf("Icon = %q, want %q", got, IconProject)
	}
}

// TestDispatchCopy_DeleteOps_SyncedNotSkipped verifies that delete operations
// do not skip items with Synced status. Delete operations compare from==to
// (same source), so items always appear "synced" — the isDelete guard in
// dispatchCopy must prevent them from being skipped.
func TestDispatchCopy_DeleteOps_SyncedNotSkipped(t *testing.T) {
	tests := []struct {
		name   string
		op     ResolvedOp
		source PaneSource
	}{
		{"DeleteFromProject", OpDeleteFromProject, PaneSource{Kind: SourceProject}},
		{"RemoveFromClaude", OpRemoveFromClaude, PaneSource{Kind: SourceClaudeCode}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := manifest.MCPServer{Command: "some-cmd"}

			// Build a compState where the server is Synced (same on both sides).
			compState := engine.CompareSources(
				engine.SourceData{MCP: map[string]manifest.MCPServer{"my-server": srv}},
				engine.SourceData{MCP: map[string]manifest.MCPServer{"my-server": srv}},
			)
			if compState.MCPSync["my-server"].Status != engine.Synced {
				t.Fatal("precondition: expected Synced status")
			}

			m := tuiModel{
				compState: compState,
				status:    NewStatus(),
				confirm:   NewConfirm(DefaultKeyMap()),
			}

			item := CopyItem{Section: SectionMCP, Name: "my-server"}
			// Delete passes from==to because dispatchCopy receives the op from
			// ResolveDeleteOp, not from re-resolving the source pair.
			m2, _ := m.dispatchCopy([]CopyItem{item}, tt.op, tt.source, tt.source)
			updated := m2.(tuiModel)

			if updated.status.flash == "Already synced" {
				t.Errorf("%s must not skip Synced items — got 'Already synced' flash", tt.name)
			}
			if !updated.confirm.Active() {
				t.Errorf("%s: expected confirmation overlay to be active", tt.name)
			}
		})
	}
}

// --- buildCascadeLines ---

func TestBuildCascadeLines_MCP_WithProfileRefs(t *testing.T) {
	m := tuiModel{
		fullState: &FullState{
			Manifest: &manifest.Manifest{
				Version:    1,
				MCPServers: map[string]manifest.MCPServer{"srv": {Command: "cmd"}},
				Profiles: map[string]manifest.Profile{
					"dev":  {MCPServers: []string{"srv"}},
					"prod": {MCPServers: []string{"srv", "other"}},
				},
			},
		},
	}
	items := []CopyItem{{Section: SectionMCP, Name: "srv"}}
	lines := m.buildCascadeLines(items)
	if lines == nil {
		t.Fatal("expected non-nil cascade lines")
	}
	// Should mention both profiles.
	joined := joinPlain(lines)
	if missing := firstMissing(joined, "dev", "prod", "srv"); missing != "" {
		t.Errorf("cascade lines missing %q; full text:\n%s", missing, joined)
	}
}

func TestBuildCascadeLines_MCP_NoRefs(t *testing.T) {
	m := tuiModel{
		fullState: &FullState{
			Manifest: &manifest.Manifest{
				Version:    1,
				MCPServers: map[string]manifest.MCPServer{"srv": {Command: "cmd"}},
				Profiles:   map[string]manifest.Profile{},
			},
		},
	}
	items := []CopyItem{{Section: SectionMCP, Name: "srv"}}
	lines := m.buildCascadeLines(items)
	if lines != nil {
		t.Errorf("expected nil cascade lines, got %v", lines)
	}
}

func TestBuildCascadeLines_Plugin_WithProfileRefs(t *testing.T) {
	m := tuiModel{
		fullState: &FullState{
			Manifest: &manifest.Manifest{
				Version: 1,
				Plugins: []manifest.Plugin{{ID: "plug-a", Enabled: true}},
				Profiles: map[string]manifest.Profile{
					"dev": {Plugins: []manifest.ProfilePlugin{{ID: "plug-a"}}},
				},
			},
		},
	}
	items := []CopyItem{{Section: SectionPlugin, Name: "plug-a"}}
	lines := m.buildCascadeLines(items)
	if lines == nil {
		t.Fatal("expected non-nil cascade lines")
	}
	joined := joinPlain(lines)
	if missing := firstMissing(joined, "dev", "plug-a"); missing != "" {
		t.Errorf("cascade lines missing %q; full text:\n%s", missing, joined)
	}
}

func TestBuildCascadeLines_Marketplace_CascadesPlugins(t *testing.T) {
	m := tuiModel{
		fullState: &FullState{
			Manifest: &manifest.Manifest{
				Version: 1,
				Plugins: []manifest.Plugin{
					{ID: "plug-a@my-mkt", Enabled: true},
					{ID: "plug-b@my-mkt", Enabled: true},
					{ID: "plug-c@other", Enabled: true},
				},
				Marketplaces: map[string]manifest.Marketplace{
					"my-mkt": {Source: "github", Repo: "org/repo"},
				},
				Profiles: map[string]manifest.Profile{
					"dev": {Plugins: []manifest.ProfilePlugin{{ID: "plug-a@my-mkt"}, {ID: "plug-c@other"}}},
				},
			},
		},
	}
	items := []CopyItem{{Section: SectionMarketplace, Name: "my-mkt"}}
	lines := m.buildCascadeLines(items)
	if lines == nil {
		t.Fatal("expected non-nil cascade lines")
	}
	joined := joinPlain(lines)
	// Should mention both affected plugins and the profile ref.
	if missing := firstMissing(joined, "plug-a@my-mkt", "plug-b@my-mkt", "dev"); missing != "" {
		t.Errorf("cascade lines missing %q; full text:\n%s", missing, joined)
	}
	// Should NOT mention plug-c@other (belongs to different marketplace).
	if strings.Contains(joined, "plug-c@other") {
		t.Error("cascade lines should not include plugins from other marketplaces")
	}
}

func TestBuildCascadeLines_Marketplace_BothPluginsInSameProfile(t *testing.T) {
	m := tuiModel{
		fullState: &FullState{
			Manifest: &manifest.Manifest{
				Version: 1,
				Plugins: []manifest.Plugin{
					{ID: "plug-a@my-mkt", Enabled: true},
					{ID: "plug-b@my-mkt", Enabled: true},
				},
				Marketplaces: map[string]manifest.Marketplace{
					"my-mkt": {Source: "github", Repo: "org/repo"},
				},
				Profiles: map[string]manifest.Profile{
					"dev": {Plugins: []manifest.ProfilePlugin{{ID: "plug-a@my-mkt"}, {ID: "plug-b@my-mkt"}}},
				},
			},
		},
	}
	items := []CopyItem{{Section: SectionMarketplace, Name: "my-mkt"}}
	lines := m.buildCascadeLines(items)
	if lines == nil {
		t.Fatal("expected cascade lines")
	}
	joined := joinPlain(lines)
	if missing := firstMissing(joined, "plug-a@my-mkt", "plug-b@my-mkt", "dev"); missing != "" {
		t.Errorf("missing %q; full text:\n%s", missing, joined)
	}
	// dev should appear twice — once per affected plugin in profile refs.
	if count := strings.Count(joined, "dev"); count < 2 {
		t.Errorf("expected dev at least 2 times in profile refs, got %d; output:\n%s", count, joined)
	}
}

func TestBuildCascadeLines_MultipleItems(t *testing.T) {
	m := tuiModel{
		fullState: &FullState{
			Manifest: &manifest.Manifest{
				Version: 1,
				MCPServers: map[string]manifest.MCPServer{
					"srv-a": {Command: "a"},
					"srv-b": {Command: "b"},
				},
				Profiles: map[string]manifest.Profile{
					"dev": {MCPServers: []string{"srv-a", "srv-b"}},
				},
			},
		},
	}
	items := []CopyItem{
		{Section: SectionMCP, Name: "srv-a"},
		{Section: SectionMCP, Name: "srv-b"},
	}
	lines := m.buildCascadeLines(items)
	if lines == nil {
		t.Fatal("expected non-nil cascade lines")
	}
	joined := joinPlain(lines)
	if missing := firstMissing(joined, "dev", "srv-a", "srv-b"); missing != "" {
		t.Errorf("cascade lines missing %q; full text:\n%s", missing, joined)
	}
	// The header should appear exactly once (not once per item).
	if count := strings.Count(joined, "Will clean profile references:"); count != 1 {
		t.Errorf("expected 1 header, got %d; full text:\n%s", count, joined)
	}
}

func TestBuildCascadeLines_NilFullState(t *testing.T) {
	m := tuiModel{fullState: nil}
	lines := m.buildCascadeLines([]CopyItem{{Section: SectionMCP, Name: "srv"}})
	if lines != nil {
		t.Errorf("expected nil for nil fullState, got %v", lines)
	}
}

func TestBuildCascadeLines_NilManifest(t *testing.T) {
	m := tuiModel{
		fullState: &FullState{Manifest: nil},
	}
	items := []CopyItem{{Section: SectionMCP, Name: "srv"}}
	lines := m.buildCascadeLines(items)
	if lines != nil {
		t.Errorf("expected nil for nil manifest, got %v", lines)
	}
}

// joinPlain strips ANSI from lines and joins them for easy assertion.
func joinPlain(lines []string) string {
	var parts []string
	for _, l := range lines {
		parts = append(parts, stripAnsi(l))
	}
	return strings.Join(parts, "\n")
}

// firstMissing returns the first substring not found in s, or "" if all present.
func firstMissing(s string, subs ...string) string {
	for _, sub := range subs {
		if !strings.Contains(s, sub) {
			return sub
		}
	}
	return ""
}

func TestSourceBorderColor_Project(t *testing.T) {
	focused := SourceBorderColor(SourceProject, true)
	if focused != ColorSrcProject {
		t.Errorf("focused = %v, want %v", focused, ColorSrcProject)
	}
	unfocused := SourceBorderColor(SourceProject, false)
	if unfocused != ColorSrcProjectDim {
		t.Errorf("unfocused = %v, want %v", unfocused, ColorSrcProjectDim)
	}
}
