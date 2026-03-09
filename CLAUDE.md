# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

cctote (Alphaleonis Tote Bag for Claude Code) is a Go CLI tool that syncs Claude Code configuration (MCP servers, plugins, marketplaces) across machines. It exports a portable manifest that integrates with chezmoi for dotfile management.

## Commands

```bash
mise run build          # compile binary (VCS version via debug.ReadBuildInfo)
mise run test           # run all tests
mise run lint           # golangci-lint
mise run fmt            # gofmt -w .
mise run coverage       # run tests and enforce per-package coverage thresholds (bash; see note below)
go test ./internal/manifest/ -run TestRoundTrip  # single test
./cctote tui            # launch the TUI after building
```

## Architecture

```
main.go              → thin entry, calls cmd.NewApp(buildinfo.Version()).Execute()
cmd/                 → cobra commands: mcp, plugin, marketplace, profile, diff, config, tui, version
  cmd/chezmoi.go     → chezmoi integration: notifyChezmoi (called by all write commands)
  cmd/helpers.go     → shared CLI helpers: resolveProfile, cliHooks (engine.Hooks impl)
  cmd/diff.go        → diff command: compare manifest vs live Claude Code state
  cmd/marketplace.go → marketplace commands: add/remove/list marketplace sources
internal/engine/     → application layer: export, import, apply, delete, state (sync/diff), profile — shared by CLI and TUI
internal/manifest/   → manifest data model, Load/Save/Update/DefaultPath, FindPlugin
internal/mcp/        → MCP server config: Read/Write/Update McpServers, DefaultPath; project.go for .mcp.json
internal/config/     → app config (cctote.toml): Load/Save/DefaultPath, BoolPtr/BoolVal, Keys/LookupKey
internal/claude/     → Claude CLI client wrapper (plugin/marketplace list/install/remove)
internal/tui/        → bubbletea TUI: sync dashboard with import/export/diff operations
internal/ui/         → shared UI primitives: Writer, Confirm, DiffList, FormatMCPSummary (used by CLI and TUI)
internal/fileutil/   → atomic file writes and file-based locking
internal/cliutil/    → CLI utilities: command runner abstraction
internal/buildinfo/  → version/build metadata
docs/                → specs: cli-spec.md, tui-spec.md
```

The **manifest** (`~/.config/cctote/manifest.json`) is the central data structure — a declarative desired state with no machine-specific runtime data. It carries MCP servers (keyed by name, supporting stdio/http/sse/websocket transports), plugins (id + scope + enabled), and marketplace sources.

Export reads from Claude Code's live config (`~/.claude.json` for MCP servers, `claude plugin list --json` for plugins). Import merges the manifest back into a target machine's config.

## Claude Code Config Layout

Key context for working on export/import:

| Path | What to sync |
|---|---|
| `~/.claude.json` | Only `mcpServers` key — rest is per-machine runtime state (~50 keys) |
| `claude plugin list --json` | Plugin id, scope, enabled — not installPath/timestamps |
| `claude plugin marketplace list --json` | Non-default marketplace sources |
| `<cwd>/.mcp.json` | Project-scope MCP servers (same `mcpServers` wrapper as `~/.claude.json`) |

**Gotchas:** `claude mcp add` is not idempotent (creates `_1` duplicates). `claude plugin install` has scope-crossing idempotency bugs. Guard against both.

## CLI Commands for Scripting

```bash
claude mcp list / add / remove / get
claude plugin list --json / install / enable / disable
claude plugin marketplace add / list
```

## Conventions

- Errors wrap with context: `fmt.Errorf("reading manifest: %w", err)`
- Exception: `internal/claude` mutation methods return bare errors — callers wrap with operation context (see package doc)
- Cobra commands use `RunE` for error propagation
- XDG Base Directory compliance — `DefaultPath()` respects `$XDG_CONFIG_HOME`
- Exception: `mcp.DefaultPath()` does NOT use XDG — Claude Code hardcodes `~/.claude.json`
- Use `json.NewEncoder` with `SetEscapeHTML(false)` when writing ~/.claude.json (URLs contain `&`)
- Manifest has `CurrentVersion = 1` with strict validation on load
- Version injected at build time (CI/GoReleaser) via `-ldflags "-X github.com/alphaleonis/cctote/internal/buildinfo.version=..."`; local builds use VCS info from `debug.ReadBuildInfo()`
- Config bools use `*bool` (not `bool`) to distinguish "unset" from "explicitly false" — use `config.BoolPtr()`/`config.BoolVal()` helpers
- Config commands reload from disk via `config.Load(resolveConfigPath())` — never read stale `appConfig`
- `go-toml/v2` with `omitempty` silently drops zero-value fields — only use `omitempty` on pointer/string types
- Use `manifest.FindPlugin()` for plugin lookups — don't duplicate the helper
- `manifest.Update` holds a file lock — call `hooks.OnCascade` (user prompts) *outside* the lock, capture results *inside* the lock
- `cliHooks` fields: pass `force` and `cascadeMsg` at construction, not via globals
- Engine result's `Actions` slice is the source of truth for what was actually exported/deleted — use it over input sets

## Issue Tracking

This project uses [beans](https://github.com/benpueschel/beans) for issue tracking. Issues are stored in `.beans/` and managed via the `beans` CLI. Use `beans` instead of TodoWrite for all work tracking.

`.beans/` is a separate private git repo (`alphaleonis/cctote-beans`). Bean changes are **not** tracked by the main repo. After creating or updating beans, commit and push inside `.beans/`:

```bash
cd .beans && git add -A && git commit -m "..." && git push && cd ..
```

## Workflow

- Run `mise run lint` before committing and fix any errors in changed code
- Involve the user in plan building — ask for clarifications or decisions (with suggestions) rather than assuming
- When a task has multiple valid approaches, present options with trade-offs before proceeding
- If you discover a pre-existing issue (e.g. a failing test, bug, or broken behavior that predates your current work), immediately create a separate bug bean to track it and alert the user

## Coverage

Per-package statement coverage is enforced via `mise run coverage` (runs `scripts/check-coverage.sh`).

| Package | Minimum |
|---|---|
| `cmd` | 80% |
| `internal/buildinfo` | 60% |
| `internal/claude` | 85% |
| `internal/cliutil` | 90% |
| `internal/config` | 80% |
| `internal/engine` | 85% |
| `internal/fileutil` | 70% |
| `internal/manifest` | 80% |
| `internal/mcp` | 80% |
| `internal/tui` | 30% |
| `internal/ui` | 90% |

- **Cobertura report**: `go test -coverprofile=coverage.out ./... && gocover-cobertura < coverage.out > coverage.xml`
- Ratchet thresholds upward as coverage improves — never lower them
- **Windows**: `mise run coverage` runs a bash script (`scripts/check-coverage.sh`) — use `go test -cover ./pkg/...` to check coverage manually on Windows

## Dependencies

- `cobra` for CLI framework
- `iancoleman/orderedmap` for preserving JSON key order in ~/.claude.json
- `pelletier/go-toml/v2` for app config
- `bubbletea`/`lipgloss`/`bubbles` for TUI mode
- `colorprofile` for terminal color detection
- `clipboard` for TUI copy-to-clipboard
