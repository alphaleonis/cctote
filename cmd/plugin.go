package cmd

import (
	"fmt"
	"sort"
	"strings"

	"github.com/alphaleonis/cctote/internal/engine"
	"github.com/alphaleonis/cctote/internal/manifest"
	"github.com/alphaleonis/cctote/internal/ui"
	"github.com/spf13/cobra"
)

func (a *App) addPluginCommands() {
	pluginCmd := &cobra.Command{
		Use:   "plugin",
		Short: "Manage plugin configurations",
	}

	pluginExportCmd := &cobra.Command{
		Use:   "export [ids...]",
		Short: "Export plugins from Claude Code to the manifest",
		RunE:  a.runPluginExport,
	}

	pluginImportCmd := &cobra.Command{
		Use:   "import [ids...]",
		Short: "Import plugins from the manifest to Claude Code",
		RunE:  a.runPluginImport,
	}

	pluginRemoveCmd := &cobra.Command{
		Use:   "remove <id>",
		Short: "Remove a plugin from the manifest",
		Args:  cobra.ExactArgs(1),
		RunE:  a.runPluginRemove,
	}

	pluginListCmd := &cobra.Command{
		Use:   "list",
		Short: "List plugins",
		RunE:  a.runPluginList,
	}

	addScopeFlag(pluginExportCmd)
	addScopeFlag(pluginImportCmd)
	addScopeFlag(pluginListCmd)

	pluginImportCmd.Flags().Bool("strict", false, "uninstall plugins not in the import set")
	pluginImportCmd.Flags().Bool("dry-run", false, "show what would change without modifying anything")

	pluginListCmd.Flags().Bool("installed", false, "list from Claude Code instead of the manifest")

	pluginCmd.AddCommand(pluginExportCmd, pluginImportCmd, pluginRemoveCmd, pluginListCmd)
	a.root.AddCommand(pluginCmd)
}

// --- plugin export ---

func (a *App) runPluginExport(cmd *cobra.Command, args []string) error {
	w := ui.NewWriter(cmd.ErrOrStderr(), a.jsonOutput)

	scope, err := getScope(cmd)
	if err != nil {
		return err
	}

	if err := a.ensureClaudeAvailable(); err != nil {
		return err
	}

	client := a.newClaudeClient()
	ctx := cmd.Context()

	allPlugins, err := client.ListPlugins(ctx)
	if err != nil {
		return err
	}

	// Filter by scope.
	var installed []manifest.Plugin
	if scope == scopeProject {
		for _, p := range allPlugins {
			if p.Scope == "project" {
				installed = append(installed, p)
			}
		}
	} else {
		// User scope: exclude project-scoped plugins so they don't leak
		// into the manifest. Without this filter, --strict import would
		// attempt to uninstall project-scoped plugins at user scope.
		for _, p := range allPlugins {
			if p.Scope != "project" {
				installed = append(installed, p)
			}
		}
	}

	if len(installed) == 0 {
		source := "Claude Code"
		if scope == scopeProject {
			source = "project"
		}
		w.Info("No plugins found in %s", source)
		if a.jsonOutput {
			return writeJSON(cmd, map[string]any{"added": 0, "updated": 0, "plugins": []string{}})
		}
		return nil
	}

	// Determine which plugins to export.
	toExport := installed
	if len(args) > 0 {
		installedMap := engine.PluginMap(installed)
		toExport = make([]manifest.Plugin, 0, len(args))
		for _, id := range args {
			p, ok := installedMap[id]
			if !ok {
				return fmt.Errorf("plugin %q not found in Claude Code", id)
			}
			toExport = append(toExport, p)
		}
	}

	manPath, err := a.resolveManifestPath()
	if err != nil {
		return err
	}

	// Load existing marketplaces so lazyLoadMarketplaces can skip the CLI call
	// when all referenced marketplaces are already in the manifest.
	var existingMkts map[string]manifest.Marketplace
	if m, loadErr := manifest.Load(manPath); loadErr == nil {
		existingMkts = m.Marketplaces
	}
	claudeMarketplaces, err := lazyLoadMarketplaces(ctx, client, existingMkts, toExport)
	if err != nil {
		return err
	}

	hooks := &cliHooks{cmd: cmd, w: w, section: "marketplace", force: a.forceFlag, cascadeMsg: "Marketplace %q is required by:"}
	result, err := engine.ExportPlugins(manPath, toExport, claudeMarketplaces, hooks)
	if err != nil {
		return err
	}

	a.notifyChezmoi(cmd.Context(), w, cmd.InOrStdin(), manPath)

	added := result.Count(engine.ActionAdded)
	updated := result.Count(engine.ActionUpdated)
	cascaded := result.Names(engine.ActionCascaded)

	if a.jsonOutput {
		var exportedIDs []string
		for _, action := range result.Actions {
			if action.Section == engine.SectionPlugin {
				exportedIDs = append(exportedIDs, action.Name)
			}
		}
		sort.Strings(exportedIDs)
		out := map[string]any{
			"added":   added,
			"updated": updated,
			"plugins": exportedIDs,
		}
		if len(cascaded) > 0 {
			out["autoExportedMarketplaces"] = cascaded
		}
		return writeJSON(cmd, out)
	}

	w.Success("Exported %d plugin(s): %d added, %d updated",
		added+updated, added, updated)
	if len(cascaded) > 0 {
		w.Info("Auto-exported marketplace(s): %s", strings.Join(cascaded, ", "))
	}
	return nil
}

// --- plugin import ---

func (a *App) runPluginImport(cmd *cobra.Command, args []string) error {
	w := ui.NewWriter(cmd.ErrOrStderr(), a.jsonOutput)

	scope, err := getScope(cmd)
	if err != nil {
		return err
	}

	strict, _ := cmd.Flags().GetBool("strict")
	dryRun, _ := cmd.Flags().GetBool("dry-run")

	if strict && len(args) > 0 {
		return fmt.Errorf("--strict cannot be used with named plugins; use --strict without names to sync all plugins")
	}

	if err := a.ensureClaudeAvailable(); err != nil {
		return err
	}

	manPath, err := a.resolveManifestPath()
	if err != nil {
		return err
	}

	m, created, err := manifest.LoadOrCreate(manPath)
	if err != nil {
		return err
	}
	if created {
		w.Info("Created manifest at %s", manPath)
	}

	client := a.newClaudeClient()
	ctx := cmd.Context()

	allPlugins, err := client.ListPlugins(ctx)
	if err != nil {
		return err
	}

	// Filter current plugins by scope for comparison.
	var current []manifest.Plugin
	if scope == scopeProject {
		for _, p := range allPlugins {
			if p.Scope == "project" {
				current = append(current, p)
			}
		}
	} else {
		// User scope: exclude project-scoped plugins so --strict doesn't
		// attempt to uninstall them at the wrong scope.
		for _, p := range allPlugins {
			if p.Scope != "project" {
				current = append(current, p)
			}
		}
	}
	currentMap := engine.PluginMap(current)

	// Determine import set.
	importSet := m.Plugins
	if len(args) > 0 {
		manifestMap := engine.PluginMap(m.Plugins)
		importSet = make([]manifest.Plugin, 0, len(args))
		for _, id := range args {
			p, ok := manifestMap[id]
			if !ok {
				return fmt.Errorf("plugin %q not found in manifest", id)
			}
			importSet = append(importSet, p)
		}
	}

	importMap := engine.PluginMap(importSet)

	// Build plan. The engine uses generic Add/Conflict/Remove terminology;
	// for plugins these map to install/reconcile/uninstall.
	plan := engine.ClassifyPluginImport(importSet, current, strict)

	// Marketplace prerequisite check — for new plugins with @marketplace,
	// verify the marketplace is available in Claude Code.
	if err := lazyCheckPluginPrereqs(ctx, client, plan.Add); err != nil {
		return err
	}

	// Dry-run.
	if dryRun {
		return a.printPluginImportPlan(cmd, w, plan.Add, plan.Conflict, plan.Skip, plan.Remove)
	}

	// Confirm strict uninstalls.
	if len(plan.Remove) > 0 {
		w.Warn("The following plugins will be uninstalled (--strict):")
		w.List(plan.Remove)
		if !a.forceFlag {
			ok, promptErr := ui.Confirm(cmd.InOrStdin(), cmd.ErrOrStderr(), "Proceed?", false)
			if promptErr != nil {
				return promptErr
			}
			if !ok {
				w.Abort()
				return nil
			}
		}
	}

	// Execute changes via the engine.
	cliScope := ""
	if scope == scopeProject {
		cliScope = "project"
	}
	hooks := &cliHooks{cmd: cmd, w: w, section: "plugin", force: a.forceFlag}
	result := engine.ApplyPluginImport(ctx, client, plan, importMap, currentMap, hooks, cliScope)

	if a.jsonOutput {
		out := map[string]any{
			"installed":   result.Installed,
			"reconciled":  result.Reconciled,
			"skipped":     result.Skipped,
			"uninstalled": result.Uninstalled,
		}
		if len(result.Errors) > 0 {
			errStrs := make([]string, len(result.Errors))
			for i, e := range result.Errors {
				errStrs[i] = e.Error()
			}
			out["errors"] = errStrs
		}
		return writeJSON(cmd, out)
	}

	w.Success("Imported: %d installed, %d reconciled, %d unchanged, %d uninstalled",
		result.Installed, result.Reconciled, result.Skipped, result.Uninstalled)
	if len(result.Errors) > 0 {
		for _, e := range result.Errors {
			w.Error("%s", e)
		}
		return result.Err()
	}
	return nil
}

// printPluginImportPlan renders the dry-run plan. This is intentionally
// separate from printImportPlan (mcp.go) because the JSON keys reflect
// different semantics: plugins have install/reconcile/uninstall (CLI ops)
// while MCP uses add/conflict/remove (file-based merge ops).
func (a *App) printPluginImportPlan(cmd *cobra.Command, w *ui.Writer, install, reconcile, skip, uninstall []string) error {
	if a.jsonOutput {
		return writeJSON(cmd, map[string]any{
			"install":   install,
			"reconcile": reconcile,
			"skip":      skip,
			"uninstall": uninstall,
		})
	}

	// DiffConflict (yellow ~) is reused for reconcile (enabled-state drift).
	// It's not a true conflict requiring user decision — reconcile always applies.
	w.DiffList(ui.DiffAdd, install)
	w.DiffList(ui.DiffConflict, reconcile)
	w.DiffList(ui.DiffSkip, skip)
	w.DiffList(ui.DiffRemove, uninstall)
	if len(install)+len(reconcile)+len(skip)+len(uninstall) == 0 {
		w.NothingToDo()
	}
	return nil
}

// --- plugin remove ---

func (a *App) runPluginRemove(cmd *cobra.Command, args []string) error {
	w := ui.NewWriter(cmd.ErrOrStderr(), a.jsonOutput)
	id := args[0]

	manPath, err := a.resolveManifestPath()
	if err != nil {
		return err
	}

	hooks := &cliHooks{cmd: cmd, w: w, section: "plugin", force: a.forceFlag}
	result, err := engine.DeletePlugin(manPath, id, hooks)
	if err != nil {
		return err
	}
	if result == nil {
		w.Abort()
		return nil
	}

	a.notifyChezmoi(cmd.Context(), w, cmd.InOrStdin(), manPath)

	if a.jsonOutput {
		out := map[string]any{"removed": id}
		if len(result.CleanedProfiles) > 0 {
			out["cleanedProfiles"] = result.CleanedProfiles
		}
		return writeJSON(cmd, out)
	}

	w.Success("Removed %q from manifest", id)
	if len(result.CleanedProfiles) > 0 {
		w.Info("Cleaned from profile(s):")
		w.List(result.CleanedProfiles)
	}
	return nil
}

// --- plugin list ---

func (a *App) runPluginList(cmd *cobra.Command, args []string) error {
	installed, _ := cmd.Flags().GetBool("installed")
	w := ui.NewWriter(cmd.ErrOrStderr(), a.jsonOutput)

	scope, err := getScope(cmd)
	if err != nil {
		return err
	}

	if scope == scopeProject && !installed {
		return fmt.Errorf("--scope project requires --installed (the manifest has no scope concept)")
	}

	var plugins []manifest.Plugin

	if installed {
		if err := a.ensureClaudeAvailable(); err != nil {
			return err
		}
		client := a.newClaudeClient()
		allPlugins, err := client.ListPlugins(cmd.Context())
		if err != nil {
			return err
		}
		if scope == scopeProject {
			for _, p := range allPlugins {
				if p.Scope == "project" {
					plugins = append(plugins, p)
				}
			}
		} else {
			// User scope: exclude project-scoped plugins for consistency
			// with export/import filtering.
			for _, p := range allPlugins {
				if p.Scope != "project" {
					plugins = append(plugins, p)
				}
			}
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
		plugins = m.Plugins
	}

	if a.jsonOutput {
		return writeJSON(cmd, plugins)
	}

	if len(plugins) == 0 {
		source := "manifest"
		if installed {
			source = "Claude Code"
			if scope == scopeProject {
				source = "project"
			}
		}
		w.Info("No plugins in %s.", source)
		return nil
	}

	rows := make([][]string, 0, len(plugins))
	for _, p := range plugins {
		enabled := "no"
		if p.Enabled {
			enabled = "yes"
		}
		rows = append(rows, []string{p.ID, p.Scope, enabled})
	}
	w.Table(cmd.OutOrStdout(), []string{"ID", "SCOPE", "ENABLED"}, rows)
	return nil
}
