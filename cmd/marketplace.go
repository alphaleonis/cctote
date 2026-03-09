package cmd

import (
	"fmt"

	"github.com/alphaleonis/cctote/internal/engine"
	"github.com/alphaleonis/cctote/internal/manifest"
	"github.com/alphaleonis/cctote/internal/ui"
	"github.com/spf13/cobra"
)

func (a *App) addMarketplaceCommands() {
	marketplaceCmd := &cobra.Command{
		Use:   "marketplace",
		Short: "Manage marketplace configurations",
	}

	marketplaceExportCmd := &cobra.Command{
		Use:   "export [names...]",
		Short: "Export marketplaces from Claude Code to the manifest",
		RunE:  a.runMarketplaceExport,
	}

	marketplaceImportCmd := &cobra.Command{
		Use:   "import [names...]",
		Short: "Import marketplaces from the manifest to Claude Code",
		RunE:  a.runMarketplaceImport,
	}

	marketplaceRemoveCmd := &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove a marketplace from the manifest",
		Args:  cobra.ExactArgs(1),
		RunE:  a.runMarketplaceRemove,
	}

	marketplaceListCmd := &cobra.Command{
		Use:   "list",
		Short: "List marketplaces",
		RunE:  a.runMarketplaceList,
	}

	marketplaceImportCmd.Flags().Bool("strict", false, "remove marketplaces not in the manifest (cannot be used with named marketplaces)")
	marketplaceImportCmd.Flags().Bool("dry-run", false, "show what would change without modifying anything")
	marketplaceImportCmd.Flags().Bool("overwrite", false, "overwrite differing marketplaces without confirmation")
	marketplaceImportCmd.Flags().Bool("no-overwrite", false, "skip differing marketplaces without confirmation")

	marketplaceListCmd.Flags().Bool("installed", false, "list from Claude Code instead of the manifest")

	marketplaceCmd.AddCommand(marketplaceExportCmd, marketplaceImportCmd, marketplaceRemoveCmd, marketplaceListCmd)
	a.root.AddCommand(marketplaceCmd)
}

// --- marketplace export ---

func (a *App) runMarketplaceExport(cmd *cobra.Command, args []string) error {
	w := ui.NewWriter(cmd.ErrOrStderr(), a.jsonOutput)

	if err := a.ensureClaudeAvailable(); err != nil {
		return err
	}

	client := a.newClaudeClient()
	ctx := cmd.Context()

	installed, err := client.ListMarketplaces(ctx)
	if err != nil {
		return err
	}

	if len(installed) == 0 {
		w.Info("No marketplaces found in Claude Code")
		if a.jsonOutput {
			return writeJSON(cmd, map[string]any{"added": 0, "updated": 0, "marketplaces": []string{}})
		}
		return nil
	}

	// Determine which marketplaces to export.
	toExport := installed
	if len(args) > 0 {
		toExport = make(map[string]manifest.Marketplace, len(args))
		for _, name := range args {
			mp, ok := installed[name]
			if !ok {
				return fmt.Errorf("marketplace %q not found in Claude Code", name)
			}
			toExport[name] = mp
		}
	}

	// Warn about directory sources (non-portable paths).
	for name, mp := range toExport {
		if mp.Source == "directory" {
			w.Warn("Marketplace %q uses a directory source (%s) — path may not be portable across machines", name, mp.Path)
		}
	}

	manPath, err := a.resolveManifestPath()
	if err != nil {
		return err
	}

	hooks := &cliHooks{cmd: cmd, w: w, section: "marketplace", force: a.forceFlag}
	result, err := engine.ExportMarketplaces(manPath, toExport, hooks)
	if err != nil {
		return err
	}

	a.notifyChezmoi(cmd.Context(), w, cmd.InOrStdin(), manPath)

	added := result.Count(engine.ActionAdded)
	updated := result.Count(engine.ActionUpdated)

	if a.jsonOutput {
		return writeJSON(cmd, map[string]any{
			"added":        added,
			"updated":      updated,
			"marketplaces": sortedKeys(toExport),
		})
	}

	w.Success("Exported %d marketplace(s): %d added, %d updated",
		added+updated, added, updated)
	return nil
}

// --- marketplace import ---

func (a *App) runMarketplaceImport(cmd *cobra.Command, args []string) error {
	w := ui.NewWriter(cmd.ErrOrStderr(), a.jsonOutput)

	overwrite, _ := cmd.Flags().GetBool("overwrite")
	noOverwrite, _ := cmd.Flags().GetBool("no-overwrite")
	strict, _ := cmd.Flags().GetBool("strict")
	dryRun, _ := cmd.Flags().GetBool("dry-run")

	if strict && len(args) > 0 {
		return fmt.Errorf("--strict cannot be used with named marketplaces; use --strict without names to sync all marketplaces")
	}

	if a.forceFlag {
		overwrite = true
	}
	if overwrite && noOverwrite {
		return fmt.Errorf("--overwrite and --no-overwrite are mutually exclusive (--force implies --overwrite)")
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

	current, err := client.ListMarketplaces(ctx)
	if err != nil {
		return err
	}

	// Determine import set.
	importSet := m.Marketplaces
	if len(args) > 0 {
		importSet = make(map[string]manifest.Marketplace, len(args))
		for _, name := range args {
			mp, ok := m.Marketplaces[name]
			if !ok {
				return fmt.Errorf("marketplace %q not found in manifest", name)
			}
			importSet[name] = mp
		}
	}

	// Build the plan.
	plan := engine.ClassifyMarketplaceImport(importSet, current, strict)

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
			// skip
		} else {
			prompt := fmt.Sprintf("Marketplace %q differs — overwrite?", name)
			ok, err := ui.Confirm(cmd.InOrStdin(), cmd.ErrOrStderr(), prompt, false)
			if err != nil {
				return err
			}
			if ok {
				overwriteNames = append(overwriteNames, name)
			}
		}
	}

	// Confirm strict removals.
	if len(plan.Remove) > 0 {
		w.Warn("The following marketplaces will be removed (--strict):")
		w.List(plan.Remove)
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

	// Execute changes via the engine.
	hooks := &cliHooks{cmd: cmd, w: w, section: "marketplace", force: a.forceFlag}
	result := engine.ApplyMarketplaceImport(ctx, client, plan, importSet, overwriteNames, hooks)

	if a.jsonOutput {
		out := map[string]any{
			"added":       result.Added,
			"overwritten": result.Overwritten,
			"skipped":     result.Skipped,
			"removed":     result.Removed,
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

	w.Success("Imported: %d added, %d overwritten, %d unchanged, %d removed",
		result.Added, result.Overwritten, result.Skipped, result.Removed)
	if len(result.Errors) > 0 {
		for _, e := range result.Errors {
			w.Error("%s", e)
		}
		return result.Err()
	}
	return nil
}

// --- marketplace remove ---

func (a *App) runMarketplaceRemove(cmd *cobra.Command, args []string) error {
	w := ui.NewWriter(cmd.ErrOrStderr(), a.jsonOutput)
	name := args[0]

	manPath, err := a.resolveManifestPath()
	if err != nil {
		return err
	}

	hooks := &cliHooks{cmd: cmd, w: w, section: "marketplace", force: a.forceFlag}
	result, err := engine.DeleteMarketplace(manPath, name, hooks)
	if err != nil {
		return err
	}
	if result == nil {
		w.Abort()
		return nil
	}

	a.notifyChezmoi(cmd.Context(), w, cmd.InOrStdin(), manPath)

	cascadedPlugins := result.Names(engine.ActionCascaded)

	if a.jsonOutput {
		out := map[string]any{"removed": name}
		if len(cascadedPlugins) > 0 {
			out["removedPlugins"] = cascadedPlugins
		}
		return writeJSON(cmd, out)
	}

	w.Success("Removed %q from manifest", name)
	if len(cascadedPlugins) > 0 {
		w.Info("Removed %d plugin(s):", len(cascadedPlugins))
		w.List(cascadedPlugins)
	}
	return nil
}

// --- marketplace list ---

func (a *App) runMarketplaceList(cmd *cobra.Command, args []string) error {
	installed, _ := cmd.Flags().GetBool("installed")
	w := ui.NewWriter(cmd.ErrOrStderr(), a.jsonOutput)

	var marketplaces map[string]manifest.Marketplace

	if installed {
		if err := a.ensureClaudeAvailable(); err != nil {
			return err
		}
		client := a.newClaudeClient()
		var err error
		marketplaces, err = client.ListMarketplaces(cmd.Context())
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
		marketplaces = m.Marketplaces
	}

	if a.jsonOutput {
		return writeJSON(cmd, marketplaces)
	}

	if len(marketplaces) == 0 {
		source := "manifest"
		if installed {
			source = "Claude Code"
		}
		w.Info("No marketplaces in %s.", source)
		return nil
	}

	names := sortedKeys(marketplaces)
	rows := make([][]string, 0, len(names))
	for _, name := range names {
		mp := marketplaces[name]
		rows = append(rows, []string{name, mp.Source, mp.SourceLocator()})
	}
	w.Table(cmd.OutOrStdout(), []string{"NAME", "SOURCE", "DETAIL"}, rows)
	return nil
}
