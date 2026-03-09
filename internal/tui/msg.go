package tui

import "github.com/alphaleonis/cctote/internal/engine"

// StateLoadedMsg is sent when the initial state load completes.
type StateLoadedMsg struct {
	State    *FullState
	Err      error
	Warnings []string // non-fatal issues (e.g., Claude CLI unavailable)
}

// StateRefreshedMsg is sent when a refresh (R key) completes.
type StateRefreshedMsg struct {
	State    *FullState
	Err      error
	Warnings []string
}

// CopyItem identifies a single item to copy.
type CopyItem struct {
	Section SectionKind
	Name    string
}

// ItemResult holds the outcome for a single item in a batch.
type ItemResult struct {
	Name string
	Err  error
}

// CopyResultMsg is sent when a copy operation completes.
type CopyResultMsg struct {
	Items   []CopyItem
	Op      ResolvedOp
	Results []ItemResult
}

// FlashStyle controls the color of a flash message.
type FlashStyle int

const (
	FlashSuccess FlashStyle = iota
	FlashError
	FlashInfo
)

// FlashMsg sets a new flash message on the status line.
type FlashMsg struct {
	Text  string
	Style FlashStyle
}

// FlashExpiredMsg clears a flash if its ID matches.
type FlashExpiredMsg struct {
	ID int
}

// ConfirmAcceptedMsg is sent when the user confirms a copy operation.
type ConfirmAcceptedMsg struct {
	Items []CopyItem
	Op    ResolvedOp
	From  PaneSource
	To    PaneSource
}

// ConfirmCancelledMsg is sent when the user cancels the confirmation overlay.
type ConfirmCancelledMsg struct{}

// SourceSelectedMsg is sent when the user picks a source from the picker.
type SourceSelectedMsg struct {
	Side   Side
	Source PaneSource
}

// BulkApplyMsg is sent when the user confirms a bulk apply operation.
type BulkApplyMsg struct {
	Source PaneSource
	Target PaneSource
	Strict bool
}

// BulkApplyResultMsg is sent when a bulk apply operation completes.
type BulkApplyResultMsg struct {
	Warnings      []string
	Err           error
	ManifestDirty bool // true if the operation modified the manifest
}

// ProgressUpdateMsg reports an individual operation starting or completing.
type ProgressUpdateMsg struct {
	Section engine.SectionKind
	Name    string
	Action  engine.ActionKind
	Done    bool  // false=starting, true=completed
	Err     error // non-nil on completion with error
	Current int   // 1-based operation number
	Total   int
}

// ProgressFinishedMsg signals that the entire bulk apply operation is done.
type ProgressFinishedMsg struct {
	Warnings      []string
	Err           error
	ManifestDirty bool
}

// ProgressDismissedMsg is sent when the user closes the progress overlay.
type ProgressDismissedMsg struct{}

// ProfileCreateMsg is sent when the user confirms creating a profile.
type ProfileCreateMsg struct {
	Name string
}

// ProfileCreateResultMsg is sent when a profile create operation completes.
type ProfileCreateResultMsg struct {
	Name string
	Err  error
}

// ProfileDeleteResultMsg is sent when a profile delete operation completes.
type ProfileDeleteResultMsg struct {
	Name string
	Err  error
}

// ConfigSavedMsg is sent when a config value is saved from the overlay.
type ConfigSavedMsg struct {
	Key   string
	Value string
}

// ToggleResultMsg is sent when a plugin toggle operation completes.
type ToggleResultMsg struct {
	Name          string
	Err           error
	ManifestDirty bool
}

// MktUpdateResultMsg is sent when a marketplace update operation completes.
type MktUpdateResultMsg struct {
	Name string
	Err  error
}

// ChezmoiDoneMsg is sent when the chezmoi re-add operation completes.
type ChezmoiDoneMsg struct {
	Err error
}
