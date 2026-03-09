package engine

import (
	"context"
	"errors"
	"fmt"

	"github.com/alphaleonis/cctote/internal/manifest"
)

// ImportClient defines the Claude CLI operations needed by engine import
// functions. This interface is satisfied by *claude.Client.
type ImportClient interface {
	InstallPlugin(ctx context.Context, id string, scope string) error
	UninstallPlugin(ctx context.Context, id string, scope string) error
	SetPluginEnabled(ctx context.Context, id string, enabled bool, scope string) error
	AddMarketplace(ctx context.Context, source string) error
	RemoveMarketplace(ctx context.Context, name string) error
	UpdateMarketplace(ctx context.Context, name string) error
}

// PluginImportResult reports what happened during a plugin import.
type PluginImportResult struct {
	Installed   int
	Reconciled  int
	Uninstalled int
	Skipped     int
	Errors      []error
}

// Err returns a joined error if any operations failed, or nil.
func (r *PluginImportResult) Err() error {
	return errors.Join(r.Errors...)
}

// MarketplaceImportResult reports what happened during a marketplace import.
type MarketplaceImportResult struct {
	Added       int
	Overwritten int
	Removed     int
	Skipped     int
	Errors      []error
}

// Err returns a joined error if any operations failed, or nil.
func (r *MarketplaceImportResult) Err() error {
	return errors.Join(r.Errors...)
}

// ApplyPluginImport executes classified plugin changes via the Claude CLI.
//
// Operations are applied in Remove → Add → Conflict order: uninstall first
// for a clean slate (important in strict mode), then install new plugins,
// then reconcile conflicts. This ordering minimizes exposure to the
// claude plugin install idempotency bugs documented in CLAUDE.md.
//
// Errors are collected across all operations so callers have full visibility
// into partial progress rather than stopping at the first failure.
//
// The desired map provides the target state for each plugin. The current map
// is used for scope-drift warnings reported via hooks.OnWarn. Pass nil for
// current if scope warnings are not needed.
func ApplyPluginImport(
	ctx context.Context,
	client ImportClient,
	plan *ImportPlan,
	desired, current map[string]manifest.Plugin,
	hooks Hooks,
	scope string,
) *PluginImportResult {
	if desired == nil && (len(plan.Add) > 0 || len(plan.Conflict) > 0) {
		return &PluginImportResult{
			Skipped: len(plan.Skip),
			Errors: []error{
				fmt.Errorf("desired map must be non-nil when plan has Add or Conflict items"),
			},
		}
	}

	ph, hasProgress := hooks.(ProgressHooks)

	result := &PluginImportResult{
		Skipped: len(plan.Skip),
	}

	// Uninstall removed plugins first (strict mode clean slate).
	for _, id := range plan.Remove {
		if err := ctx.Err(); err != nil {
			result.Errors = append(result.Errors, err)
			return result
		}
		if hasProgress {
			ph.OnOpStart(SectionPlugin, id, ActionRemoved)
		}
		err := client.UninstallPlugin(ctx, id, scope)
		if hasProgress {
			ph.OnOpDone(SectionPlugin, id, ActionRemoved, err)
		}
		if err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("uninstalling plugin %q: %w", id, err))
			continue
		}
		result.Uninstalled++
	}

	// Install new plugins. When an install fails and the plugin references a
	// marketplace, refresh the marketplace index and retry once — this handles
	// stale marketplace caches without the latency cost of proactive updates.
	updatedMkts := map[string]bool{}
	for _, id := range plan.Add {
		if err := ctx.Err(); err != nil {
			result.Errors = append(result.Errors, err)
			return result
		}
		p := desired[id]
		if hasProgress {
			ph.OnOpStart(SectionPlugin, id, ActionAdded)
		}
		err := client.InstallPlugin(ctx, id, scope)
		if err != nil {
			if mpName := MarketplaceFromPluginID(id); mpName != "" && !updatedMkts[mpName] {
				updatedMkts[mpName] = true
				// Refresh the marketplace index before retrying. If the refresh
				// itself fails, skip the retry — the original install error is
				// more actionable than a marketplace update failure.
				if updateErr := client.UpdateMarketplace(ctx, mpName); updateErr == nil {
					if retryErr := client.InstallPlugin(ctx, id, scope); retryErr != nil {
						// Keep original error; note that retry was attempted.
						// Note: this annotation appears in Error() but not in
						// UserMessage() which extracts RunError.Stderr directly.
						err = fmt.Errorf("%w (retry after marketplace update also failed)", err)
					} else {
						err = nil
					}
				}
			}
		}
		if err != nil {
			if hasProgress {
				ph.OnOpDone(SectionPlugin, id, ActionAdded, err)
			}
			result.Errors = append(result.Errors, fmt.Errorf("installing plugin %q: %w", id, err))
			continue
		}
		// Fresh installs default to enabled; only call SetPluginEnabled when
		// the manifest wants the plugin disabled, to avoid a redundant CLI
		// round-trip and reduce exposure to idempotency bugs.
		if !p.Enabled {
			if err := client.SetPluginEnabled(ctx, id, false, scope); err != nil {
				if hasProgress {
					ph.OnOpDone(SectionPlugin, id, ActionAdded, err)
				}
				result.Errors = append(result.Errors, fmt.Errorf("disabling plugin %q: %w", id, err))
				continue
			}
		}
		if hasProgress {
			ph.OnOpDone(SectionPlugin, id, ActionAdded, nil)
		}
		result.Installed++
	}

	// Reconcile conflicting plugins (enabled-state and scope drift).
	for _, id := range plan.Conflict {
		if err := ctx.Err(); err != nil {
			result.Errors = append(result.Errors, err)
			return result
		}
		p := desired[id]
		if current != nil {
			if cur, ok := current[id]; ok && cur.Scope != p.Scope {
				hooks.OnWarn(fmt.Sprintf("Plugin %q has scope %q locally but %q in manifest — scope cannot be changed via CLI; uninstall and reinstall to fix",
					id, cur.Scope, p.Scope))
			}
		}
		if hasProgress {
			ph.OnOpStart(SectionPlugin, id, ActionUpdated)
		}
		err := client.SetPluginEnabled(ctx, id, p.Enabled, scope)
		if hasProgress {
			ph.OnOpDone(SectionPlugin, id, ActionUpdated, err)
		}
		if err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("reconciling plugin %q: %w", id, err))
			continue
		}
		result.Reconciled++
	}

	return result
}

// ApplyMarketplaceImport executes classified marketplace changes via the
// Claude CLI.
//
// Items in plan.Add are added, items in overwrite are removed then re-added
// (no update operation exists), and items in plan.Remove are removed. The
// overwrite slice should contain the subset of plan.Conflict that the caller
// decided to overwrite — conflict resolution (interactive prompting, flags)
// stays with the caller.
//
// Errors are collected across all operations for partial-progress visibility.
func ApplyMarketplaceImport(
	ctx context.Context,
	client ImportClient,
	plan *ImportPlan,
	desired map[string]manifest.Marketplace,
	overwrite []string,
	hooks Hooks,
) *MarketplaceImportResult {
	// Skipped = unchanged items (plan.Skip) + conflicts the caller chose not
	// to overwrite. The overwrite slice must be a subset of plan.Conflict.
	skipped := len(plan.Skip) + len(plan.Conflict) - len(overwrite)
	if skipped < 0 {
		skipped = 0
	}
	result := &MarketplaceImportResult{
		Skipped: skipped,
	}

	ph, hasProgress := hooks.(ProgressHooks)

	// Add new marketplaces.
	for _, name := range plan.Add {
		if err := ctx.Err(); err != nil {
			result.Errors = append(result.Errors, err)
			return result
		}
		source, err := desired[name].SourceLocatorE()
		if err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("marketplace %q: %w", name, err))
			continue
		}
		if hasProgress {
			ph.OnOpStart(SectionMarketplace, name, ActionAdded)
		}
		err = client.AddMarketplace(ctx, source)
		if hasProgress {
			ph.OnOpDone(SectionMarketplace, name, ActionAdded, err)
		}
		if err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("marketplace %q: %w", name, err))
			continue
		}
		result.Added++
	}

	// Overwrite conflicting marketplaces (remove + re-add).
	for _, name := range overwrite {
		if err := ctx.Err(); err != nil {
			result.Errors = append(result.Errors, err)
			return result
		}
		source, err := desired[name].SourceLocatorE()
		if err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("marketplace %q: %w", name, err))
			continue
		}
		if hasProgress {
			ph.OnOpStart(SectionMarketplace, name, ActionUpdated)
		}
		if err := client.RemoveMarketplace(ctx, name); err != nil {
			if hasProgress {
				ph.OnOpDone(SectionMarketplace, name, ActionUpdated, err)
			}
			result.Errors = append(result.Errors, fmt.Errorf("marketplace %q: remove for overwrite failed: %w", name, err))
			continue
		}
		err = client.AddMarketplace(ctx, source)
		if hasProgress {
			ph.OnOpDone(SectionMarketplace, name, ActionUpdated, err)
		}
		if err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("marketplace %q: re-add after removal failed (manual re-add may be needed): %w", name, err))
			continue
		}
		result.Overwritten++
	}

	// Remove marketplaces (strict mode).
	for _, name := range plan.Remove {
		if err := ctx.Err(); err != nil {
			result.Errors = append(result.Errors, err)
			return result
		}
		if hasProgress {
			ph.OnOpStart(SectionMarketplace, name, ActionRemoved)
		}
		err := client.RemoveMarketplace(ctx, name)
		if hasProgress {
			ph.OnOpDone(SectionMarketplace, name, ActionRemoved, err)
		}
		if err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("marketplace %q: %w", name, err))
			continue
		}
		result.Removed++
	}

	return result
}
