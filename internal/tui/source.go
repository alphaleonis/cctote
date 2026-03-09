package tui

import (
	"fmt"
	"image/color"

	"github.com/alphaleonis/cctote/internal/engine"
)

// SourceKind identifies the type of data source a pane is showing.
type SourceKind int

const (
	SourceManifest SourceKind = iota
	SourceProfile
	SourceClaudeCode
	SourceProject
)

// PaneSource identifies what a pane is displaying.
type PaneSource struct {
	Kind        SourceKind
	ProfileName string // only meaningful when Kind == SourceProfile
}

// Label returns a human-readable label for the source.
func (s PaneSource) Label() string {
	switch s.Kind {
	case SourceManifest:
		return "Manifest"
	case SourceProfile:
		return fmt.Sprintf("Profile: %s", s.ProfileName)
	case SourceClaudeCode:
		return "Claude Code"
	case SourceProject:
		return "Project"
	}
	return ""
}

// Icon returns a Nerd Font icon for the source.
func (s PaneSource) Icon() string {
	switch s.Kind {
	case SourceManifest:
		return IconManifest
	case SourceProfile:
		return IconProfile
	case SourceClaudeCode:
		return IconClaudeCode
	case SourceProject:
		return IconProject
	}
	return ""
}

// Equal returns whether two sources are the same.
func (s PaneSource) Equal(o PaneSource) bool {
	return s.Kind == o.Kind && s.ProfileName == o.ProfileName
}

// ExtractSourceData builds a SourceData from the given source and full state.
func ExtractSourceData(src PaneSource, full *FullState) SourceData {
	if full == nil {
		return SourceData{}
	}
	switch src.Kind {
	case SourceManifest:
		if full.Manifest == nil {
			return SourceData{}
		}
		return SourceData{
			MCP:          full.Manifest.MCPServers,
			Plugins:      full.Manifest.Plugins,
			Marketplaces: full.Manifest.Marketplaces,
		}

	case SourceProfile:
		if full.Manifest == nil {
			return SourceData{}
		}
		filteredMCP, filteredPlugins, ok := engine.ResolveProfileLenient(full.Manifest, src.ProfileName)
		if !ok {
			return SourceData{}
		}
		return SourceData{
			MCP:     filteredMCP,
			Plugins: filteredPlugins,
			// Profiles don't include marketplaces.
		}

	case SourceClaudeCode:
		return SourceData{
			MCP:          full.MCPInstalled,
			Plugins:      full.PlugInstalled,
			Marketplaces: full.MktInstalled,
		}

	case SourceProject:
		return SourceData{
			MCP:     full.ProjectMCP,
			Plugins: full.ProjectPlugins,
			// Projects don't include marketplaces.
		}
	}
	return SourceData{}
}

// ResolvedOp identifies the concrete operation when copying between two sources.
type ResolvedOp int

const (
	OpExportToManifest        ResolvedOp = iota // write item to manifest.json
	OpImportToClaude                            // install into Claude Code
	OpAddToProfile                              // add manifest item reference to profile
	OpExportAndAddProfile                       // export to manifest + add profile ref
	OpCopyProfileRef                            // copy ref between profiles
	OpDeleteFromManifest                        // delete from manifest.json
	OpRemoveFromProfile                         // remove profile reference
	OpRemoveFromClaude                          // uninstall from Claude Code live config
	OpDeleteProfile                             // delete an entire profile
	OpExportProjectToManifest                   // project item → manifest.json
	OpImportToProject                           // manifest/Claude Code item → project
	OpDeleteFromProject                         // remove item from project config
	OpInvalid                                   // unsupported combination
)

// ResolveOp determines the operation for copying from one source to another.
//
// Operation matrix (from row → to column):
//
//	               To:  Manifest                 ClaudeCode    Profile              Project
//	From:
//	  Manifest          invalid                  import        add-ref              import-to-project
//	  ClaudeCode        export                   invalid       export+add-profile   import-to-project
//	  Profile           invalid*                 import        copy-ref             import-to-project
//	  Project           export-project-to-man    import        export+add-profile   invalid
//
// * Profile→Manifest is invalid because profile items are references
// to manifest entries — the underlying data already exists in the manifest.
func ResolveOp(from, to PaneSource) ResolvedOp {
	switch {
	// Claude Code → Manifest
	case from.Kind == SourceClaudeCode && to.Kind == SourceManifest:
		return OpExportToManifest
	// Manifest → Claude Code
	case from.Kind == SourceManifest && to.Kind == SourceClaudeCode:
		return OpImportToClaude
	// Claude Code → Profile (export to manifest + add profile ref)
	case from.Kind == SourceClaudeCode && to.Kind == SourceProfile:
		return OpExportAndAddProfile
	// Manifest → Profile (add profile ref, item already in manifest)
	case from.Kind == SourceManifest && to.Kind == SourceProfile:
		return OpAddToProfile
	// Profile → Claude Code (import from manifest)
	case from.Kind == SourceProfile && to.Kind == SourceClaudeCode:
		return OpImportToClaude
	// Profile → Manifest (no-op: profile items are already in manifest)
	case from.Kind == SourceProfile && to.Kind == SourceManifest:
		return OpInvalid
	// Profile → Profile (copy ref)
	case from.Kind == SourceProfile && to.Kind == SourceProfile:
		return OpCopyProfileRef
	// Project → Manifest
	case from.Kind == SourceProject && to.Kind == SourceManifest:
		return OpExportProjectToManifest
	// Project → Claude Code
	case from.Kind == SourceProject && to.Kind == SourceClaudeCode:
		return OpImportToClaude
	// Project → Profile (export to manifest + add profile ref)
	case from.Kind == SourceProject && to.Kind == SourceProfile:
		return OpExportAndAddProfile
	// Same kind → same kind
	case from.Kind == to.Kind:
		return OpInvalid
	// Manifest/ClaudeCode/Profile → Project (catch-all after same-kind guard)
	case to.Kind == SourceProject:
		return OpImportToProject
	}
	return OpInvalid
}

// ResolveDeleteOp determines the operation for deleting from the focused source.
func ResolveDeleteOp(focused PaneSource) ResolvedOp {
	switch focused.Kind {
	case SourceManifest:
		return OpDeleteFromManifest
	case SourceProfile:
		return OpRemoveFromProfile
	case SourceClaudeCode:
		return OpRemoveFromClaude // MCP via direct file write, plugins/marketplaces via CLI
	case SourceProject:
		return OpDeleteFromProject
	}
	return OpInvalid
}

// Label returns a human-readable action label.
func (op ResolvedOp) Label() string {
	switch op {
	case OpExportToManifest:
		return "Export"
	case OpImportToClaude:
		return "Import"
	case OpAddToProfile:
		return "Add to profile"
	case OpExportAndAddProfile:
		return "Export & add to profile"
	case OpCopyProfileRef:
		return "Copy to profile"
	case OpDeleteFromManifest:
		return "Delete"
	case OpRemoveFromProfile:
		return "Remove"
	case OpRemoveFromClaude:
		return "Uninstall"
	case OpDeleteProfile:
		return "Delete profile"
	case OpExportProjectToManifest:
		return "Export"
	case OpImportToProject:
		return "Copy to project"
	case OpDeleteFromProject:
		return "Remove"
	}
	return "Copy"
}

// PastTense returns the past-tense label for flash messages.
func (op ResolvedOp) PastTense() string {
	switch op {
	case OpExportToManifest:
		return "Exported"
	case OpImportToClaude:
		return "Imported"
	case OpAddToProfile:
		return "Added to profile"
	case OpExportAndAddProfile:
		return "Exported & added"
	case OpCopyProfileRef:
		return "Copied to profile"
	case OpDeleteFromManifest:
		return "Deleted"
	case OpRemoveFromProfile:
		return "Removed"
	case OpRemoveFromClaude:
		return "Uninstalled"
	case OpDeleteProfile:
		return "Deleted profile"
	case OpExportProjectToManifest:
		return "Exported"
	case OpImportToProject:
		return "Copied to project"
	case OpDeleteFromProject:
		return "Removed"
	}
	return "Copied"
}

// IsDangerous returns whether the operation is destructive and should default
// to a danger-styled confirmation button.
func (op ResolvedOp) IsDangerous() bool {
	return op == OpDeleteFromManifest || op == OpRemoveFromClaude || op == OpDeleteProfile || op == OpDeleteFromProject
}

// TargetsProfile returns whether the operation writes to a profile.
func (op ResolvedOp) TargetsProfile() bool {
	return op == OpAddToProfile || op == OpExportAndAddProfile || op == OpCopyProfileRef
}

// ProgressLabel returns a present-tense gerund for the progress dialog title.
func (op ResolvedOp) ProgressLabel() string {
	switch op {
	case OpImportToClaude:
		return "Importing"
	case OpRemoveFromClaude:
		return "Uninstalling"
	case OpImportToProject:
		return "Copying to project:"
	case OpDeleteFromProject:
		return "Removing from project:"
	case OpExportToManifest, OpExportProjectToManifest:
		return "Exporting"
	case OpDeleteFromManifest:
		return "Deleting"
	case OpRemoveFromProfile:
		return "Removing"
	case OpAddToProfile:
		return "Adding to profile:"
	case OpExportAndAddProfile:
		return "Exporting & adding:"
	case OpCopyProfileRef:
		return "Copying to profile:"
	}
	return "Processing"
}

// ActionKind returns the appropriate engine.ActionKind for progress reporting.
func (op ResolvedOp) ActionKind() engine.ActionKind {
	switch op {
	case OpDeleteFromManifest, OpRemoveFromProfile, OpRemoveFromClaude, OpDeleteFromProject:
		return engine.ActionRemoved
	case OpExportToManifest, OpExportProjectToManifest, OpExportAndAddProfile:
		return engine.ActionUpdated
	default:
		// Import and install operations (OpImportToClaude, OpImportToProject, etc.)
		// are treated as "added" for progress reporting.
		return engine.ActionAdded
	}
}

// InvokesCLI returns whether the operation calls the Claude CLI for per-item
// mutations (plugin install/uninstall, marketplace add/remove). Operations
// that only write files (manifest, MCP config) return false.
func (op ResolvedOp) InvokesCLI() bool {
	switch op {
	case OpImportToClaude, OpRemoveFromClaude, OpImportToProject, OpDeleteFromProject:
		return true
	}
	return false
}

// ModifiesManifest returns whether the operation writes to the manifest file.
func (op ResolvedOp) ModifiesManifest() bool {
	switch op {
	case OpExportToManifest, OpExportProjectToManifest,
		OpAddToProfile, OpExportAndAddProfile,
		OpCopyProfileRef, OpDeleteFromManifest, OpRemoveFromProfile,
		OpDeleteProfile:
		return true
	}
	return false
}

// SourceBorderColor returns the border color for a source kind.
func SourceBorderColor(kind SourceKind, focused bool) color.Color {
	if focused {
		switch kind {
		case SourceManifest:
			return ColorSrcManifest
		case SourceProfile:
			return ColorSrcProfile
		case SourceClaudeCode:
			return ColorSrcClaudeCode
		case SourceProject:
			return ColorSrcProject
		}
		return ColorFocusBdr
	}
	switch kind {
	case SourceManifest:
		return ColorSrcManifestDim
	case SourceProfile:
		return ColorSrcProfileDim
	case SourceClaudeCode:
		return ColorSrcClaudeCodeDim
	case SourceProject:
		return ColorSrcProjectDim
	}
	return ColorBorder
}
