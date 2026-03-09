package cmd

import (
	"fmt"
	"strings"

	"github.com/alphaleonis/cctote/internal/engine"
	"github.com/alphaleonis/cctote/internal/manifest"
	"github.com/alphaleonis/cctote/internal/mcp"
	"github.com/alphaleonis/cctote/internal/ui"
	"github.com/spf13/cobra"
)

func (a *App) addMcpCommands() {
	mcpCmd := &cobra.Command{
		Use:   "mcp",
		Short: "Manage MCP server configurations",
	}

	mcpExportCmd := &cobra.Command{
		Use:   "export [names...]",
		Short: "Export MCP servers from Claude Code to the manifest",
		RunE:  a.runMcpExport,
	}

	mcpImportCmd := &cobra.Command{
		Use:   "import [names...]",
		Short: "Import MCP servers from the manifest to Claude Code",
		RunE:  a.runMcpImport,
	}

	mcpRemoveCmd := &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove an MCP server from the manifest",
		Args:  cobra.ExactArgs(1),
		RunE:  a.runMcpRemove,
	}

	mcpListCmd := &cobra.Command{
		Use:   "list",
		Short: "List MCP servers",
		RunE:  a.runMcpList,
	}

	addScopeFlag(mcpExportCmd)
	addScopeFlag(mcpImportCmd)
	addScopeFlag(mcpListCmd)

	mcpImportCmd.Flags().Bool("strict", false, "remove MCP servers not in the manifest (cannot be used with named servers)")
	mcpImportCmd.Flags().Bool("dry-run", false, "show what would change without modifying anything")
	mcpImportCmd.Flags().Bool("overwrite", false, "overwrite differing MCP servers without confirmation")
	mcpImportCmd.Flags().Bool("no-overwrite", false, "skip differing MCP servers without confirmation")

	mcpListCmd.Flags().Bool("installed", false, "list from Claude Code instead of the manifest")

	mcpCmd.AddCommand(mcpExportCmd, mcpImportCmd, mcpRemoveCmd, mcpListCmd)
	a.root.AddCommand(mcpCmd)
}

// --- mcp export ---

func (a *App) runMcpExport(cmd *cobra.Command, args []string) error {
	w := ui.NewWriter(cmd.ErrOrStderr(), a.jsonOutput)

	scope, err := getScope(cmd)
	if err != nil {
		return err
	}

	var installed map[string]manifest.MCPServer
	var sourceLabel string

	if scope == scopeProject {
		installed, err = mcp.ReadProjectMcpServers(a.resolveProjectMcpPath())
		sourceLabel = ".mcp.json"
	} else {
		var claudePath string
		claudePath, err = mcp.DefaultPath()
		if err != nil {
			return err
		}
		installed, err = mcp.ReadMcpServers(claudePath)
		sourceLabel = claudePath
	}
	if err != nil {
		return err
	}

	if len(installed) == 0 {
		w.Info("No MCP servers found in %s", sourceLabel)
		if a.jsonOutput {
			return writeJSON(cmd, map[string]any{"added": 0, "updated": 0, "mcpServers": []string{}})
		}
		return nil
	}

	// Determine which MCP servers to export.
	toExport := installed
	if len(args) > 0 {
		toExport = make(map[string]manifest.MCPServer, len(args))
		for _, name := range args {
			srv, ok := installed[name]
			if !ok {
				return fmt.Errorf("MCP server %q not found in Claude Code config", name)
			}
			toExport[name] = srv
		}
	}

	manPath, err := a.resolveManifestPath()
	if err != nil {
		return err
	}

	hooks := &cliHooks{cmd: cmd, w: w, section: "MCP server", force: a.forceFlag}
	result, err := engine.ExportMCPServers(manPath, toExport, hooks)
	if err != nil {
		return err
	}

	a.notifyChezmoi(cmd.Context(), w, cmd.InOrStdin(), manPath)

	added := result.Count(engine.ActionAdded)
	updated := result.Count(engine.ActionUpdated)

	if a.jsonOutput {
		return writeJSON(cmd, map[string]any{
			"added":      added,
			"updated":    updated,
			"mcpServers": sortedKeys(toExport),
		})
	}

	w.Success("Exported %d MCP server(s): %d added, %d updated",
		added+updated, added, updated)
	return nil
}

// --- mcp import ---

func (a *App) runMcpImport(cmd *cobra.Command, args []string) error {
	w := ui.NewWriter(cmd.ErrOrStderr(), a.jsonOutput)

	scope, err := getScope(cmd)
	if err != nil {
		return err
	}

	overwrite, _ := cmd.Flags().GetBool("overwrite")
	noOverwrite, _ := cmd.Flags().GetBool("no-overwrite")
	strict, _ := cmd.Flags().GetBool("strict")
	dryRun, _ := cmd.Flags().GetBool("dry-run")

	// --strict is incompatible with named arguments: --strict means "the
	// manifest is the complete desired state", which conflicts with partial
	// selection. Without this guard, --strict + names would remove ALL
	// servers not in the named subset — a destructive footgun.
	if strict && len(args) > 0 {
		return fmt.Errorf("--strict cannot be used with named MCP servers; use --strict without names to sync all servers")
	}

	// Flag validation: --force implies --overwrite (skip all prompts and
	// apply changes). Catch the --force --no-overwrite contradiction.
	if a.forceFlag {
		overwrite = true
	}
	if overwrite && noOverwrite {
		return fmt.Errorf("--overwrite and --no-overwrite are mutually exclusive (--force implies --overwrite)")
	}

	manPath, err := a.resolveManifestPath()
	if err != nil {
		return err
	}

	m, _, err := manifest.LoadOrCreate(manPath)
	if err != nil {
		return err
	}

	// Read the current state from the target scope.
	var current map[string]manifest.MCPServer
	var targetPath string

	if scope == scopeProject {
		targetPath = a.resolveProjectMcpPath()
		current, err = mcp.ReadProjectMcpServers(targetPath)
	} else {
		targetPath, err = mcp.DefaultPath()
		if err != nil {
			return err
		}
		current, err = mcp.ReadMcpServers(targetPath)
	}
	if err != nil {
		return err
	}

	// Determine import set.
	importSet := m.MCPServers
	if len(args) > 0 {
		importSet = make(map[string]manifest.MCPServer, len(args))
		for _, name := range args {
			srv, ok := m.MCPServers[name]
			if !ok {
				return fmt.Errorf("MCP server %q not found in manifest", name)
			}
			importSet[name] = srv
		}
	}

	// Build the plan.
	plan := engine.ClassifyMCPImport(importSet, current, strict)

	// Dry-run: print plan and exit.
	if dryRun {
		return a.printImportPlan(cmd, w, plan.Add, plan.Skip, plan.Conflict, plan.Remove)
	}

	// Resolve conflicts.
	var overwriteNames []string
	for _, name := range plan.Conflict {
		if overwrite {
			overwriteNames = append(overwriteNames, name)
		} else if noOverwrite {
			// skip — do nothing
		} else {
			prompt := fmt.Sprintf("MCP server %q differs — overwrite?", name)
			if srv, ok := importSet[name]; ok {
				for _, line := range ui.FormatMCPSummary(srv) {
					w.Faint("  %s", line)
				}
			}
			ok, err := ui.Confirm(cmd.InOrStdin(), cmd.ErrOrStderr(), prompt, false)
			if err != nil {
				return err
			}
			if ok {
				overwriteNames = append(overwriteNames, name)
			}
		}
	}

	// Warn about env vars that look like secrets in servers being added/overwritten.
	mcpHooks := &cliHooks{cmd: cmd, w: w, section: "MCP server", force: a.forceFlag}
	engine.WarnSecretEnvVars(importSet, append(plan.Add, overwriteNames...), mcpHooks)

	// Confirm strict removals. Always show what will be removed; only skip
	// the confirmation prompt when --force is set.
	if len(plan.Remove) > 0 {
		w.Warn("The following MCP servers will be removed (--strict):")
		for _, name := range plan.Remove {
			w.List([]string{name})
			if srv, ok := current[name]; ok {
				for _, line := range ui.FormatMCPSummary(srv) {
					w.Faint("      %s", line)
				}
			}
		}
		if !a.forceFlag {
			ok, err := ui.Confirm(cmd.InOrStdin(), cmd.ErrOrStderr(), "Proceed?", false)
			if err != nil {
				return err
			}
			if !ok {
				w.Abort()
				return nil
			}
		}
	}

	// Apply changes to the target scope.
	if scope == scopeProject {
		engine.WarnProjectEnvVars(importSet, append(plan.Add, overwriteNames...), mcpHooks)
		// Write directly to .mcp.json instead of `claude mcp add -s project`
		// because Claude CLI's mcp add is not idempotent — it creates _1 suffix
		// duplicates. Direct file write gives clean upsert semantics.
		if err := applyMCPImportToProject(targetPath, importSet, plan.Add, overwriteNames, plan.Remove); err != nil {
			return err
		}
	} else {
		// Apply changes atomically. ApplyMCPImport re-reads the config under
		// a file lock, so concurrent changes by Claude Code are preserved.
		if err := engine.ApplyMCPImport(targetPath, importSet, plan.Add, overwriteNames, plan.Remove); err != nil {
			return err
		}
	}

	skipped := len(plan.Conflict) - len(overwriteNames)
	if a.jsonOutput {
		return writeJSON(cmd, map[string]any{
			"added":       len(plan.Add),
			"overwritten": len(overwriteNames),
			"skipped":     len(plan.Skip) + skipped,
			"removed":     len(plan.Remove),
		})
	}

	w.Success("Imported: %d added, %d overwritten, %d unchanged, %d removed",
		len(plan.Add), len(overwriteNames), len(plan.Skip)+skipped, len(plan.Remove))
	return nil
}

// applyMCPImportToProject writes MCP servers to .mcp.json using the
// locked read-modify-write pattern.
func applyMCPImportToProject(path string, desired map[string]manifest.MCPServer, add, overwrite, remove []string) error {
	return mcp.UpdateProjectMcpServers(path, func(servers map[string]manifest.MCPServer) error {
		for _, name := range add {
			servers[name] = desired[name]
		}
		for _, name := range overwrite {
			servers[name] = desired[name]
		}
		for _, name := range remove {
			delete(servers, name)
		}
		return nil
	})
}

func (a *App) printImportPlan(cmd *cobra.Command, w *ui.Writer, add, skip, conflict, remove []string) error {
	if a.jsonOutput {
		return writeJSON(cmd, map[string]any{
			"add":      add,
			"skip":     skip,
			"conflict": conflict,
			"remove":   remove,
		})
	}

	w.DiffList(ui.DiffAdd, add)
	w.DiffList(ui.DiffSkip, skip)
	w.DiffList(ui.DiffConflict, conflict)
	w.DiffList(ui.DiffRemove, remove)
	if len(add)+len(skip)+len(conflict)+len(remove) == 0 {
		w.NothingToDo()
	}
	return nil
}

// --- mcp remove ---

func (a *App) runMcpRemove(cmd *cobra.Command, args []string) error {
	w := ui.NewWriter(cmd.ErrOrStderr(), a.jsonOutput)
	name := args[0]

	manPath, err := a.resolveManifestPath()
	if err != nil {
		return err
	}

	hooks := &cliHooks{cmd: cmd, w: w, section: "MCP server", force: a.forceFlag}
	result, err := engine.DeleteMCP(manPath, name, hooks)
	if err != nil {
		return err
	}
	if result == nil {
		w.Abort()
		return nil
	}

	a.notifyChezmoi(cmd.Context(), w, cmd.InOrStdin(), manPath)

	if a.jsonOutput {
		out := map[string]any{"removed": name}
		if len(result.CleanedProfiles) > 0 {
			out["cleanedProfiles"] = result.CleanedProfiles
		}
		return writeJSON(cmd, out)
	}

	w.Success("Removed %q from manifest", name)
	if len(result.CleanedProfiles) > 0 {
		w.Info("Cleaned from profile(s):")
		w.List(result.CleanedProfiles)
	}
	return nil
}

// --- mcp list ---

func (a *App) runMcpList(cmd *cobra.Command, args []string) error {
	installed, _ := cmd.Flags().GetBool("installed")
	uw := ui.NewWriter(cmd.ErrOrStderr(), a.jsonOutput)

	scope, err := getScope(cmd)
	if err != nil {
		return err
	}

	if scope == scopeProject && !installed {
		return fmt.Errorf("--scope project requires --installed (the manifest has no scope concept)")
	}

	var servers map[string]manifest.MCPServer

	if installed {
		if scope == scopeProject {
			servers, err = mcp.ReadProjectMcpServers(a.resolveProjectMcpPath())
		} else {
			var claudePath string
			claudePath, err = mcp.DefaultPath()
			if err != nil {
				return err
			}
			servers, err = mcp.ReadMcpServers(claudePath)
		}
		if err != nil {
			return err
		}
	} else {
		manPath, err := a.resolveManifestPath()
		if err != nil {
			return err
		}
		m, _, err := manifest.LoadOrCreate(manPath)
		if err != nil {
			return err
		}
		servers = m.MCPServers
	}

	if a.jsonOutput {
		return writeJSON(cmd, servers)
	}

	if len(servers) == 0 {
		source := "manifest"
		if installed {
			source = "Claude Code"
			if scope == scopeProject {
				source = ".mcp.json"
			}
		}
		uw.Info("No MCP servers in %s.", source)
		return nil
	}

	names := sortedKeys(servers)
	rows := make([][]string, 0, len(names))
	for _, name := range names {
		srv := servers[name]
		transport := srv.Type
		if transport == "" {
			transport = "stdio"
		}
		rows = append(rows, []string{name, transport, serverDetail(srv)})
	}
	uw.Table(cmd.OutOrStdout(), []string{"NAME", "TRANSPORT", "DETAIL"}, rows)
	return nil
}

// --- helpers ---

func serverDetail(srv manifest.MCPServer) string {
	switch {
	case srv.URL != "":
		return srv.URL
	case srv.Command != "":
		if len(srv.Args) > 0 {
			return srv.Command + " " + strings.Join(srv.Args, " ")
		}
		return srv.Command
	default:
		return ""
	}
}
