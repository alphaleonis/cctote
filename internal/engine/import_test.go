package engine

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/alphaleonis/cctote/internal/manifest"
	"github.com/alphaleonis/cctote/internal/mcp"
)

// --- ClassifyMCPImport ---

func TestClassifyMCPImport_AddSkipConflict(t *testing.T) {
	desired := map[string]manifest.MCPServer{
		"new":       {Command: "new-cmd"},
		"same":      {Command: "same-cmd"},
		"different": {Command: "desired-cmd"},
	}
	current := map[string]manifest.MCPServer{
		"same":      {Command: "same-cmd"},
		"different": {Command: "current-cmd"},
		"extra":     {Command: "extra-cmd"},
	}

	plan := ClassifyMCPImport(desired, current, false)

	assertSlice(t, "Add", plan.Add, []string{"new"})
	assertSlice(t, "Skip", plan.Skip, []string{"same"})
	assertSlice(t, "Conflict", plan.Conflict, []string{"different"})
	assertSlice(t, "Remove", plan.Remove, []string{})
}

func TestClassifyMCPImport_Strict(t *testing.T) {
	desired := map[string]manifest.MCPServer{
		"keep": {Command: "cmd"},
	}
	current := map[string]manifest.MCPServer{
		"keep":    {Command: "cmd"},
		"remove1": {Command: "r1"},
		"remove2": {Command: "r2"},
	}

	plan := ClassifyMCPImport(desired, current, true)

	assertSlice(t, "Skip", plan.Skip, []string{"keep"})
	assertSlice(t, "Remove", plan.Remove, []string{"remove1", "remove2"})
}

func TestClassifyMCPImport_Empty(t *testing.T) {
	plan := ClassifyMCPImport(nil, nil, false)

	assertSlice(t, "Add", plan.Add, []string{})
	assertSlice(t, "Skip", plan.Skip, []string{})
	assertSlice(t, "Conflict", plan.Conflict, []string{})
	assertSlice(t, "Remove", plan.Remove, []string{})
}

// --- ClassifyPluginImport ---

func TestClassifyPluginImport(t *testing.T) {
	desired := []manifest.Plugin{
		{ID: "install-me", Scope: "global", Enabled: true},
		{ID: "already-ok", Scope: "global", Enabled: true},
		{ID: "reconcile-me", Scope: "global", Enabled: false},
	}
	current := []manifest.Plugin{
		{ID: "already-ok", Scope: "global", Enabled: true},
		{ID: "reconcile-me", Scope: "global", Enabled: true},
		{ID: "orphan", Scope: "project", Enabled: true},
	}

	plan := ClassifyPluginImport(desired, current, false)

	assertSlice(t, "Add", plan.Add, []string{"install-me"})
	assertSlice(t, "Skip", plan.Skip, []string{"already-ok"})
	assertSlice(t, "Conflict", plan.Conflict, []string{"reconcile-me"})
	assertSlice(t, "Remove", plan.Remove, []string{})
}

func TestClassifyPluginImport_Strict(t *testing.T) {
	desired := []manifest.Plugin{
		{ID: "keep", Scope: "global", Enabled: true},
	}
	current := []manifest.Plugin{
		{ID: "keep", Scope: "global", Enabled: true},
		{ID: "uninstall-me", Scope: "global", Enabled: true},
	}

	plan := ClassifyPluginImport(desired, current, true)

	assertSlice(t, "Skip", plan.Skip, []string{"keep"})
	assertSlice(t, "Remove", plan.Remove, []string{"uninstall-me"})
}

// --- ClassifyMarketplaceImport ---

func TestClassifyMarketplaceImport(t *testing.T) {
	desired := map[string]manifest.Marketplace{
		"new":       {Source: "github", Repo: "new/repo"},
		"same":      {Source: "github", Repo: "same/repo"},
		"different": {Source: "github", Repo: "desired/repo"},
	}
	current := map[string]manifest.Marketplace{
		"same":      {Source: "github", Repo: "same/repo"},
		"different": {Source: "github", Repo: "current/repo"},
		"extra":     {Source: "git", URL: "https://extra.example.com"},
	}

	plan := ClassifyMarketplaceImport(desired, current, false)

	assertSlice(t, "Add", plan.Add, []string{"new"})
	assertSlice(t, "Skip", plan.Skip, []string{"same"})
	assertSlice(t, "Conflict", plan.Conflict, []string{"different"})
	assertSlice(t, "Remove", plan.Remove, []string{})
}

func TestClassifyMarketplaceImport_Strict(t *testing.T) {
	desired := map[string]manifest.Marketplace{
		"keep": {Source: "github", Repo: "keep/repo"},
	}
	current := map[string]manifest.Marketplace{
		"keep":   {Source: "github", Repo: "keep/repo"},
		"remove": {Source: "git", URL: "https://remove.example.com"},
	}

	plan := ClassifyMarketplaceImport(desired, current, true)

	assertSlice(t, "Skip", plan.Skip, []string{"keep"})
	assertSlice(t, "Remove", plan.Remove, []string{"remove"})
}

// --- ApplyMCPImport ---

func TestApplyMCPImport_AddOverwriteRemove(t *testing.T) {
	dir := t.TempDir()
	claudePath := filepath.Join(dir, ".claude.json")

	// Seed with existing servers.
	current := map[string]manifest.MCPServer{
		"keep":      {Command: "keep-cmd"},
		"overwrite": {Command: "old-cmd"},
		"remove":    {Command: "remove-cmd"},
	}
	if err := mcp.WriteMcpServers(claudePath, current); err != nil {
		t.Fatal(err)
	}

	desired := map[string]manifest.MCPServer{
		"new":       {Command: "new-cmd"},
		"overwrite": {Command: "new-overwrite-cmd"},
	}

	err := ApplyMCPImport(claudePath, desired,
		[]string{"new"},
		[]string{"overwrite"},
		[]string{"remove"},
	)
	if err != nil {
		t.Fatal(err)
	}

	result, err := mcp.ReadMcpServers(claudePath)
	if err != nil {
		t.Fatal(err)
	}

	if _, ok := result["new"]; !ok {
		t.Error("'new' should be added")
	}
	if result["overwrite"].Command != "new-overwrite-cmd" {
		t.Error("'overwrite' should be updated")
	}
	if _, ok := result["remove"]; ok {
		t.Error("'remove' should be deleted")
	}
	if _, ok := result["keep"]; !ok {
		t.Error("'keep' should be preserved")
	}
	if result["keep"].Command != "keep-cmd" {
		t.Errorf("keep.Command = %q, want %q", result["keep"].Command, "keep-cmd")
	}
}

func TestApplyMCPImport_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	claudePath := filepath.Join(dir, ".claude.json")

	desired := map[string]manifest.MCPServer{
		"srv": {Command: "cmd"},
	}

	err := ApplyMCPImport(claudePath, desired, []string{"srv"}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	result, err := mcp.ReadMcpServers(claudePath)
	if err != nil {
		t.Fatal(err)
	}
	if result["srv"].Command != "cmd" {
		t.Errorf("srv command = %q, want %q", result["srv"].Command, "cmd")
	}
}

// --- ApplyMCPImportToProject ---

func TestApplyMCPImportToProject_AddOverwriteRemove(t *testing.T) {
	dir := t.TempDir()
	mcpPath := filepath.Join(dir, ".mcp.json")

	// Seed with existing servers.
	if err := mcp.UpdateProjectMcpServers(mcpPath, func(servers map[string]manifest.MCPServer) error {
		servers["keep"] = manifest.MCPServer{Command: "keep-cmd"}
		servers["overwrite"] = manifest.MCPServer{Command: "old-cmd"}
		servers["remove"] = manifest.MCPServer{Command: "remove-cmd"}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	desired := map[string]manifest.MCPServer{
		"new":       {Command: "new-cmd"},
		"overwrite": {Command: "new-overwrite-cmd"},
	}

	err := ApplyMCPImportToProject(mcpPath, desired,
		[]string{"new"},
		[]string{"overwrite"},
		[]string{"remove"},
	)
	if err != nil {
		t.Fatal(err)
	}

	result, err := mcp.ReadProjectMcpServers(mcpPath)
	if err != nil {
		t.Fatal(err)
	}

	if _, ok := result["new"]; !ok {
		t.Error("'new' should be added")
	}
	if result["overwrite"].Command != "new-overwrite-cmd" {
		t.Error("'overwrite' should be updated")
	}
	if _, ok := result["remove"]; ok {
		t.Error("'remove' should be deleted")
	}
	if _, ok := result["keep"]; !ok {
		t.Error("'keep' should be preserved")
	}
	if result["keep"].Command != "keep-cmd" {
		t.Errorf("keep.Command = %q, want %q", result["keep"].Command, "keep-cmd")
	}
}

func TestApplyMCPImportToProject_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	mcpPath := filepath.Join(dir, ".mcp.json")

	desired := map[string]manifest.MCPServer{
		"srv": {Command: "cmd"},
	}

	err := ApplyMCPImportToProject(mcpPath, desired, []string{"srv"}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	result, err := mcp.ReadProjectMcpServers(mcpPath)
	if err != nil {
		t.Fatal(err)
	}
	if result["srv"].Command != "cmd" {
		t.Errorf("srv command = %q, want %q", result["srv"].Command, "cmd")
	}
}

// --- test helpers ---

// assertSlice compares two string slices after sorting both, so tests are
// resilient to map-iteration order in Classify* functions.
func assertSlice(t *testing.T, label string, got, want []string) {
	t.Helper()
	g := slices.Clone(got)
	w := slices.Clone(want)
	slices.Sort(g)
	slices.Sort(w)
	if len(g) != len(w) {
		t.Errorf("%s: got %v, want %v", label, g, w)
		return
	}
	for i := range g {
		if g[i] != w[i] {
			t.Errorf("%s[%d]: got %q, want %q", label, i, g[i], w[i])
		}
	}
}
