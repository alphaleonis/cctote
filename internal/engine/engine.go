// Package engine implements cctote's application layer — the single source of
// truth for business logic shared between CLI and TUI presentation layers.
//
// Operations (export, import, delete) own their invariants: cascade handling,
// marketplace dependency management, profile cleanup, and plan classification.
// Presentation-layer decisions (prompting, overlays, flash messages) are
// delegated to the caller via the [Hooks] interface.
package engine

import "sort"

// Hooks lets the caller control UX-dependent decisions.
// Both CLI and TUI provide their own implementation.
type Hooks interface {
	// OnCascade is called when an operation would cascade to dependents.
	// dependents lists what will also be affected. Return true to proceed.
	OnCascade(item string, dependents []string) (bool, error)

	// OnInfo reports progress/status messages.
	OnInfo(msg string)

	// OnWarn reports non-fatal warnings that the user should be aware of.
	OnWarn(msg string)
}

// ProgressHooks extends Hooks with per-operation progress callbacks.
// Callers that want progress updates type-assert hooks to ProgressHooks;
// existing callers (CLI, tests) are unaffected.
type ProgressHooks interface {
	Hooks
	// OnOpStart is called before an individual CLI operation begins.
	OnOpStart(section SectionKind, name string, action ActionKind)
	// OnOpDone is called after an individual CLI operation completes.
	OnOpDone(section SectionKind, name string, action ActionKind, err error)
}

// ActionKind categorizes what happened to a single item.
type ActionKind string

const (
	ActionAdded    ActionKind = "added"
	ActionUpdated  ActionKind = "updated"
	ActionRemoved  ActionKind = "removed"
	ActionSkipped  ActionKind = "skipped"
	ActionCascaded ActionKind = "cascaded" // removed as a side effect of another operation
)

// ItemAction records what happened to a single item.
type ItemAction struct {
	Section SectionKind
	Name    string
	Action  ActionKind
	Detail  string // optional human-readable detail
}

// Result summarizes the outcome of an engine operation.
type Result struct {
	Actions         []ItemAction
	CleanedProfiles []string // profiles that had references cleaned
}

// Count returns the number of actions matching the given kind.
func (r *Result) Count(action ActionKind) int {
	n := 0
	for _, a := range r.Actions {
		if a.Action == action {
			n++
		}
	}
	return n
}

// Names returns the sorted names of items matching the given action kind.
func (r *Result) Names(action ActionKind) []string {
	var names []string
	for _, a := range r.Actions {
		if a.Action == action {
			names = append(names, a.Name)
		}
	}
	sort.Strings(names)
	return names
}
