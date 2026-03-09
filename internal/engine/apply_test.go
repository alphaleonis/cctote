package engine

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/alphaleonis/cctote/internal/manifest"
)

// mockClient implements ImportClient for testing.
type mockClient struct {
	calls  []string // "install:id", "uninstall:id", etc.
	failOn map[string]bool
}

func newMockClient() *mockClient {
	return &mockClient{failOn: map[string]bool{}}
}

func (m *mockClient) callKey(verb, id, scope string) string {
	key := verb + ":" + id
	if scope != "" {
		key += "@" + scope
	}
	return key
}

func (m *mockClient) InstallPlugin(_ context.Context, id string, scope string) error {
	key := m.callKey("install", id, scope)
	m.calls = append(m.calls, key)
	if m.failOn["install:"+id] {
		return fmt.Errorf("install failed: %s", id)
	}
	return nil
}

func (m *mockClient) UninstallPlugin(_ context.Context, id string, scope string) error {
	key := m.callKey("uninstall", id, scope)
	m.calls = append(m.calls, key)
	if m.failOn["uninstall:"+id] {
		return fmt.Errorf("uninstall failed: %s", id)
	}
	return nil
}

func (m *mockClient) SetPluginEnabled(_ context.Context, id string, enabled bool, scope string) error {
	verb := "disable"
	if enabled {
		verb = "enable"
	}
	key := m.callKey(verb, id, scope)
	m.calls = append(m.calls, key)
	if m.failOn[verb+":"+id] {
		return fmt.Errorf("%s failed: %s", verb, id)
	}
	return nil
}

func (m *mockClient) AddMarketplace(_ context.Context, source string) error {
	m.calls = append(m.calls, "add-marketplace:"+source)
	if m.failOn["add-marketplace:"+source] {
		return fmt.Errorf("add marketplace failed: %s", source)
	}
	return nil
}

func (m *mockClient) RemoveMarketplace(_ context.Context, name string) error {
	m.calls = append(m.calls, "remove-marketplace:"+name)
	if m.failOn["remove-marketplace:"+name] {
		return fmt.Errorf("remove marketplace failed: %s", name)
	}
	return nil
}

func (m *mockClient) UpdateMarketplace(_ context.Context, name string) error {
	m.calls = append(m.calls, "update-marketplace:"+name)
	if m.failOn["update-marketplace:"+name] {
		return fmt.Errorf("update marketplace failed: %s", name)
	}
	return nil
}

// retryMockClient wraps mockClient but only fails the first install attempt for
// each plugin, simulating a stale marketplace that succeeds after update.
type retryMockClient struct {
	*mockClient
	installAttempts map[string]int
}

func (r *retryMockClient) InstallPlugin(_ context.Context, id string, scope string) error {
	r.calls = append(r.calls, r.callKey("install", id, scope))
	r.installAttempts[id]++
	if r.installAttempts[id] == 1 && r.failOn["install:"+id] {
		return fmt.Errorf("install failed: %s", id)
	}
	return nil
}

// applyHooks returns a mockHooks configured for apply tests (auto-approve cascades).
func applyHooks() *mockHooks {
	return &mockHooks{cascadeOK: true}
}

// --- ApplyPluginImport ---

func TestApplyPluginImport_InstallEnabled(t *testing.T) {
	client := newMockClient()
	plan := &ImportPlan{
		Add: []string{"plugin-a"}, Skip: []string{}, Conflict: []string{}, Remove: []string{},
	}
	desired := map[string]manifest.Plugin{
		"plugin-a": {ID: "plugin-a", Enabled: true},
	}

	result := ApplyPluginImport(context.Background(), client, plan, desired, nil, applyHooks(), "")

	if result.Installed != 1 {
		t.Errorf("Installed = %d, want 1", result.Installed)
	}
	if result.Err() != nil {
		t.Errorf("unexpected error: %v", result.Err())
	}
	// Should NOT call SetPluginEnabled since plugin is enabled (default).
	for _, call := range client.calls {
		if strings.HasPrefix(call, "disable:") || strings.HasPrefix(call, "enable:") {
			t.Errorf("should not call SetPluginEnabled for enabled plugin, got %q", call)
		}
	}
}

func TestApplyPluginImport_InstallDisabled(t *testing.T) {
	client := newMockClient()
	plan := &ImportPlan{
		Add: []string{"plugin-a"}, Skip: []string{}, Conflict: []string{}, Remove: []string{},
	}
	desired := map[string]manifest.Plugin{
		"plugin-a": {ID: "plugin-a", Enabled: false},
	}

	result := ApplyPluginImport(context.Background(), client, plan, desired, nil, applyHooks(), "")

	if result.Installed != 1 {
		t.Errorf("Installed = %d, want 1", result.Installed)
	}
	// Should call disable after install.
	if len(client.calls) != 2 || client.calls[0] != "install:plugin-a" || client.calls[1] != "disable:plugin-a" {
		t.Errorf("calls = %v, want [install:plugin-a disable:plugin-a]", client.calls)
	}
}

func TestApplyPluginImport_Reconcile(t *testing.T) {
	client := newMockClient()
	plan := &ImportPlan{
		Add: []string{}, Skip: []string{}, Conflict: []string{"plugin-a"}, Remove: []string{},
	}
	desired := map[string]manifest.Plugin{
		"plugin-a": {ID: "plugin-a", Enabled: true, Scope: "user"},
	}
	current := map[string]manifest.Plugin{
		"plugin-a": {ID: "plugin-a", Enabled: false, Scope: "user"},
	}

	result := ApplyPluginImport(context.Background(), client, plan, desired, current, applyHooks(), "")

	if result.Reconciled != 1 {
		t.Errorf("Reconciled = %d, want 1", result.Reconciled)
	}
	if len(client.calls) != 1 || client.calls[0] != "enable:plugin-a" {
		t.Errorf("calls = %v, want [enable:plugin-a]", client.calls)
	}
}

func TestApplyPluginImport_ScopeDriftWarning(t *testing.T) {
	client := newMockClient()
	hooks := applyHooks()
	plan := &ImportPlan{
		Add: []string{}, Skip: []string{}, Conflict: []string{"plugin-a"}, Remove: []string{},
	}
	desired := map[string]manifest.Plugin{
		"plugin-a": {ID: "plugin-a", Enabled: true, Scope: "project"},
	}
	current := map[string]manifest.Plugin{
		"plugin-a": {ID: "plugin-a", Enabled: false, Scope: "user"},
	}

	ApplyPluginImport(context.Background(), client, plan, desired, current, hooks, "")

	if len(hooks.warnCalls) != 1 {
		t.Fatalf("expected 1 warn message, got %d", len(hooks.warnCalls))
	}
	if !strings.Contains(hooks.warnCalls[0], "scope") {
		t.Errorf("warn message should mention scope: %q", hooks.warnCalls[0])
	}
}

func TestApplyPluginImport_Uninstall(t *testing.T) {
	client := newMockClient()
	plan := &ImportPlan{
		Add: []string{}, Skip: []string{}, Conflict: []string{}, Remove: []string{"plugin-a", "plugin-b"},
	}

	result := ApplyPluginImport(context.Background(), client, plan, nil, nil, applyHooks(), "")

	if result.Uninstalled != 2 {
		t.Errorf("Uninstalled = %d, want 2", result.Uninstalled)
	}
	wantCalls := []string{"uninstall:plugin-a", "uninstall:plugin-b"}
	if len(client.calls) != len(wantCalls) {
		t.Fatalf("calls = %v, want %v", client.calls, wantCalls)
	}
	for i, want := range wantCalls {
		if client.calls[i] != want {
			t.Errorf("call[%d] = %q, want %q", i, client.calls[i], want)
		}
	}
}

func TestApplyPluginImport_SkippedCount(t *testing.T) {
	client := newMockClient()
	plan := &ImportPlan{
		Add: []string{}, Skip: []string{"a", "b", "c"}, Conflict: []string{}, Remove: []string{},
	}

	result := ApplyPluginImport(context.Background(), client, plan, nil, nil, applyHooks(), "")

	if result.Skipped != 3 {
		t.Errorf("Skipped = %d, want 3", result.Skipped)
	}
}

func TestApplyPluginImport_ContinuesAfterError(t *testing.T) {
	client := newMockClient()
	client.failOn["install:bad"] = true

	plan := &ImportPlan{
		Add: []string{"good-a", "bad", "good-b"}, Skip: []string{}, Conflict: []string{}, Remove: []string{},
	}
	desired := map[string]manifest.Plugin{
		"good-a": {ID: "good-a", Enabled: true},
		"bad":    {ID: "bad", Enabled: true},
		"good-b": {ID: "good-b", Enabled: true},
	}

	result := ApplyPluginImport(context.Background(), client, plan, desired, nil, applyHooks(), "")

	if result.Installed != 2 {
		t.Errorf("Installed = %d, want 2", result.Installed)
	}
	if len(result.Errors) != 1 {
		t.Errorf("Errors = %d, want 1", len(result.Errors))
	}
	// All 3 installs should be attempted.
	installCount := 0
	for _, call := range client.calls {
		if strings.HasPrefix(call, "install:") {
			installCount++
		}
	}
	if installCount != 3 {
		t.Errorf("install attempts = %d, want 3", installCount)
	}
}

func TestApplyPluginImport_SkipsDisableAfterFailedInstall(t *testing.T) {
	client := newMockClient()
	client.failOn["install:bad"] = true

	plan := &ImportPlan{
		Add: []string{"bad"}, Skip: []string{}, Conflict: []string{}, Remove: []string{},
	}
	desired := map[string]manifest.Plugin{
		"bad": {ID: "bad", Enabled: false},
	}

	ApplyPluginImport(context.Background(), client, plan, desired, nil, applyHooks(), "")

	for _, call := range client.calls {
		if strings.HasPrefix(call, "disable:") || strings.HasPrefix(call, "enable:") {
			t.Errorf("should not call SetPluginEnabled after failed install, got %q", call)
		}
	}
}

func TestApplyPluginImport_DisableFailureDoesNotCountAsInstalled(t *testing.T) {
	client := newMockClient()
	client.failOn["disable:plugin-a"] = true

	plan := &ImportPlan{
		Add: []string{"plugin-a"}, Skip: []string{}, Conflict: []string{}, Remove: []string{},
	}
	desired := map[string]manifest.Plugin{
		"plugin-a": {ID: "plugin-a", Enabled: false},
	}

	result := ApplyPluginImport(context.Background(), client, plan, desired, nil, applyHooks(), "")

	// A failed disable means the plugin is NOT fully reconciled — don't count it.
	if result.Installed != 0 {
		t.Errorf("Installed = %d, want 0 (disable failed)", result.Installed)
	}
	if len(result.Errors) != 1 {
		t.Errorf("Errors = %d, want 1", len(result.Errors))
	}
}

func TestApplyPluginImport_ForwardsScope(t *testing.T) {
	client := newMockClient()
	plan := &ImportPlan{
		Add: []string{"plugin-a"}, Skip: []string{}, Conflict: []string{"plugin-b"}, Remove: []string{"plugin-c"},
	}
	desired := map[string]manifest.Plugin{
		"plugin-a": {ID: "plugin-a", Enabled: true},
		"plugin-b": {ID: "plugin-b", Enabled: false},
	}
	current := map[string]manifest.Plugin{
		"plugin-b": {ID: "plugin-b", Enabled: true, Scope: "project"},
	}

	ApplyPluginImport(context.Background(), client, plan, desired, current, applyHooks(), "project")

	// Every client call should include the scope.
	wantCalls := []string{
		"uninstall:plugin-c@project",
		"install:plugin-a@project",
		"disable:plugin-b@project",
	}
	if len(client.calls) != len(wantCalls) {
		t.Fatalf("calls = %v, want %v", client.calls, wantCalls)
	}
	for i, want := range wantCalls {
		if client.calls[i] != want {
			t.Errorf("call[%d] = %q, want %q", i, client.calls[i], want)
		}
	}
}

func TestApplyPluginImport_RetriesAfterMarketplaceUpdate(t *testing.T) {
	client := newMockClient()
	// First install of marketplace plugins fails, triggering a marketplace update + retry.
	installAttempts := map[string]int{}
	client.failOn["install:plugin-a@my-mkt"] = true

	// Use a custom client that tracks call count.
	tracker := &retryMockClient{mockClient: client, installAttempts: installAttempts}

	plan := &ImportPlan{
		Add:  []string{"plugin-a@my-mkt", "plugin-b@my-mkt", "plugin-c"},
		Skip: []string{}, Conflict: []string{}, Remove: []string{},
	}
	desired := map[string]manifest.Plugin{
		"plugin-a@my-mkt": {ID: "plugin-a@my-mkt", Enabled: true},
		"plugin-b@my-mkt": {ID: "plugin-b@my-mkt", Enabled: true},
		"plugin-c":        {ID: "plugin-c", Enabled: true},
	}

	result := ApplyPluginImport(context.Background(), tracker, plan, desired, nil, applyHooks(), "")

	// plugin-a@my-mkt: first install fails → update my-mkt → retry install → succeeds.
	// plugin-b@my-mkt: install succeeds (my-mkt already updated, no retry needed).
	// plugin-c: no marketplace, install succeeds.
	if result.Installed != 3 {
		t.Errorf("Installed = %d, want 3", result.Installed)
	}
	if result.Err() != nil {
		t.Errorf("unexpected error: %v", result.Err())
	}

	// Marketplace should be updated exactly once (deduplicated).
	updateCount := 0
	for _, call := range tracker.calls {
		if call == "update-marketplace:my-mkt" {
			updateCount++
		}
	}
	if updateCount != 1 {
		t.Errorf("update-marketplace:my-mkt called %d times, want 1", updateCount)
	}

	// plugin-c should NOT trigger any marketplace update.
	for _, call := range tracker.calls {
		if call == "update-marketplace:" {
			t.Error("should not update marketplace for plugins without marketplace suffix")
		}
	}
}

func TestApplyPluginImport_NoRetryWithoutMarketplace(t *testing.T) {
	client := newMockClient()
	client.failOn["install:plugin-c"] = true

	plan := &ImportPlan{
		Add: []string{"plugin-c"}, Skip: []string{}, Conflict: []string{}, Remove: []string{},
	}
	desired := map[string]manifest.Plugin{
		"plugin-c": {ID: "plugin-c", Enabled: true},
	}

	result := ApplyPluginImport(context.Background(), client, plan, desired, nil, applyHooks(), "")

	if result.Installed != 0 {
		t.Errorf("Installed = %d, want 0", result.Installed)
	}
	if len(result.Errors) != 1 {
		t.Errorf("Errors = %d, want 1", len(result.Errors))
	}
	// No marketplace update should be attempted.
	for _, call := range client.calls {
		if strings.HasPrefix(call, "update-marketplace:") {
			t.Errorf("unexpected marketplace update: %s", call)
		}
	}
}

func TestApplyPluginImport_RetryFailsReportsOriginalError(t *testing.T) {
	// Both install attempts fail — the error should be reported.
	client := newMockClient()
	client.failOn["install:plugin-a@bad-mkt"] = true

	plan := &ImportPlan{
		Add: []string{"plugin-a@bad-mkt"}, Skip: []string{}, Conflict: []string{}, Remove: []string{},
	}
	desired := map[string]manifest.Plugin{
		"plugin-a@bad-mkt": {ID: "plugin-a@bad-mkt", Enabled: true},
	}

	result := ApplyPluginImport(context.Background(), client, plan, desired, nil, applyHooks(), "")

	if result.Installed != 0 {
		t.Errorf("Installed = %d, want 0", result.Installed)
	}
	if len(result.Errors) != 1 {
		t.Fatalf("Errors = %d, want 1", len(result.Errors))
	}
	if !strings.Contains(result.Errors[0].Error(), "plugin-a@bad-mkt") {
		t.Errorf("error should mention plugin: %v", result.Errors[0])
	}
}

// --- ApplyMarketplaceImport ---

func TestApplyMarketplaceImport_Add(t *testing.T) {
	client := newMockClient()
	plan := &ImportPlan{
		Add: []string{"mkt-a"}, Skip: []string{}, Conflict: []string{}, Remove: []string{},
	}
	desired := map[string]manifest.Marketplace{
		"mkt-a": {Source: "github", Repo: "owner/repo"},
	}

	result := ApplyMarketplaceImport(context.Background(), client, plan, desired, nil, applyHooks())

	if result.Added != 1 {
		t.Errorf("Added = %d, want 1", result.Added)
	}
	if len(client.calls) != 1 || client.calls[0] != "add-marketplace:owner/repo" {
		t.Errorf("calls = %v, want [add-marketplace:owner/repo]", client.calls)
	}
}

func TestApplyMarketplaceImport_Overwrite(t *testing.T) {
	client := newMockClient()
	plan := &ImportPlan{
		Add: []string{}, Skip: []string{}, Conflict: []string{"mkt-a"}, Remove: []string{},
	}
	desired := map[string]manifest.Marketplace{
		"mkt-a": {Source: "github", Repo: "owner/new-repo"},
	}

	result := ApplyMarketplaceImport(context.Background(), client, plan, desired, []string{"mkt-a"}, applyHooks())

	if result.Overwritten != 1 {
		t.Errorf("Overwritten = %d, want 1", result.Overwritten)
	}
	if result.Skipped != 0 {
		t.Errorf("Skipped = %d, want 0", result.Skipped)
	}
	// Should remove then re-add.
	if len(client.calls) != 2 {
		t.Fatalf("calls = %v, want 2 calls", client.calls)
	}
	if client.calls[0] != "remove-marketplace:mkt-a" {
		t.Errorf("first call = %q, want remove-marketplace:mkt-a", client.calls[0])
	}
	if client.calls[1] != "add-marketplace:owner/new-repo" {
		t.Errorf("second call = %q, want add-marketplace:owner/new-repo", client.calls[1])
	}
}

func TestApplyMarketplaceImport_SkipConflict(t *testing.T) {
	client := newMockClient()
	plan := &ImportPlan{
		Add: []string{}, Skip: []string{"unchanged"}, Conflict: []string{"conflicting"}, Remove: []string{},
	}
	desired := map[string]manifest.Marketplace{
		"conflicting": {Source: "github", Repo: "owner/repo"},
	}

	// No overwrite — conflict is skipped.
	result := ApplyMarketplaceImport(context.Background(), client, plan, desired, nil, applyHooks())

	if result.Skipped != 2 { // 1 Skip + 1 Conflict not overwritten
		t.Errorf("Skipped = %d, want 2", result.Skipped)
	}
	if len(client.calls) != 0 {
		t.Errorf("calls = %v, want none", client.calls)
	}
}

func TestApplyMarketplaceImport_Remove(t *testing.T) {
	client := newMockClient()
	plan := &ImportPlan{
		Add: []string{}, Skip: []string{}, Conflict: []string{}, Remove: []string{"old-mkt"},
	}

	result := ApplyMarketplaceImport(context.Background(), client, plan, nil, nil, applyHooks())

	if result.Removed != 1 {
		t.Errorf("Removed = %d, want 1", result.Removed)
	}
	if len(client.calls) != 1 || client.calls[0] != "remove-marketplace:old-mkt" {
		t.Errorf("calls = %v, want [remove-marketplace:old-mkt]", client.calls)
	}
}

func TestApplyMarketplaceImport_ContinuesAfterError(t *testing.T) {
	client := newMockClient()
	client.failOn["add-marketplace:owner/bad"] = true

	plan := &ImportPlan{
		Add: []string{"good", "bad"}, Skip: []string{}, Conflict: []string{}, Remove: []string{},
	}
	desired := map[string]manifest.Marketplace{
		"good": {Source: "github", Repo: "owner/good"},
		"bad":  {Source: "github", Repo: "owner/bad"},
	}

	result := ApplyMarketplaceImport(context.Background(), client, plan, desired, nil, applyHooks())

	if result.Added != 1 {
		t.Errorf("Added = %d, want 1", result.Added)
	}
	if len(result.Errors) != 1 {
		t.Errorf("Errors = %d, want 1", len(result.Errors))
	}
}

func TestApplyMarketplaceImport_InvalidSource(t *testing.T) {
	client := newMockClient()
	plan := &ImportPlan{
		Add: []string{"bad-source"}, Skip: []string{}, Conflict: []string{}, Remove: []string{},
	}
	desired := map[string]manifest.Marketplace{
		"bad-source": {Source: "unknown"},
	}

	result := ApplyMarketplaceImport(context.Background(), client, plan, desired, nil, applyHooks())

	if len(result.Errors) != 1 {
		t.Fatalf("Errors = %d, want 1", len(result.Errors))
	}
	if !strings.Contains(result.Errors[0].Error(), "unsupported") {
		t.Errorf("error should mention unsupported source: %v", result.Errors[0])
	}
}

func TestApplyMarketplaceImport_OverwriteReAddFails(t *testing.T) {
	client := newMockClient()
	client.failOn["add-marketplace:owner/repo"] = true

	plan := &ImportPlan{
		Add: []string{}, Skip: []string{}, Conflict: []string{"mkt-a"}, Remove: []string{},
	}
	desired := map[string]manifest.Marketplace{
		"mkt-a": {Source: "github", Repo: "owner/repo"},
	}

	result := ApplyMarketplaceImport(context.Background(), client, plan, desired, []string{"mkt-a"}, applyHooks())

	// Remove succeeded but re-add failed — marketplace is now removed with no replacement.
	if result.Overwritten != 0 {
		t.Errorf("Overwritten = %d, want 0 (re-add failed)", result.Overwritten)
	}
	if len(result.Errors) != 1 {
		t.Fatalf("Errors = %d, want 1", len(result.Errors))
	}
	if !strings.Contains(result.Errors[0].Error(), "manual re-add") {
		t.Errorf("error should mention manual re-add: %v", result.Errors[0])
	}
}

// --- ProgressHooks ---

type progressCall struct {
	section SectionKind
	name    string
	action  ActionKind
	done    bool
	err     error
}

type mockProgressHooks struct {
	*mockHooks
	calls []progressCall
}

func (h *mockProgressHooks) OnOpStart(section SectionKind, name string, action ActionKind) {
	h.calls = append(h.calls, progressCall{section: section, name: name, action: action, done: false})
}

func (h *mockProgressHooks) OnOpDone(section SectionKind, name string, action ActionKind, err error) {
	h.calls = append(h.calls, progressCall{section: section, name: name, action: action, done: true, err: err})
}

func TestApplyPluginImport_ProgressHooks(t *testing.T) {
	client := newMockClient()
	plan := &ImportPlan{
		Remove:   []string{"old-plugin"},
		Add:      []string{"new-plugin"},
		Conflict: []string{"conflict-plugin"},
		Skip:     []string{},
	}
	desired := map[string]manifest.Plugin{
		"new-plugin":      {ID: "new-plugin", Enabled: true},
		"conflict-plugin": {ID: "conflict-plugin", Enabled: true},
	}
	current := map[string]manifest.Plugin{
		"conflict-plugin": {ID: "conflict-plugin", Enabled: false, Scope: "user"},
	}

	hooks := &mockProgressHooks{mockHooks: applyHooks()}
	ApplyPluginImport(context.Background(), client, plan, desired, current, hooks, "")

	// Expect: start+done for each of Remove(1) + Add(1) + Conflict(1) = 6 calls
	if len(hooks.calls) != 6 {
		t.Fatalf("progress calls = %d, want 6", len(hooks.calls))
	}
	// Verify ordering: remove start, remove done, add start, add done, conflict start, conflict done
	wantNames := []string{"old-plugin", "old-plugin", "new-plugin", "new-plugin", "conflict-plugin", "conflict-plugin"}
	wantDone := []bool{false, true, false, true, false, true}
	wantActions := []ActionKind{ActionRemoved, ActionRemoved, ActionAdded, ActionAdded, ActionUpdated, ActionUpdated}
	for i, call := range hooks.calls {
		if call.section != SectionPlugin {
			t.Errorf("call[%d].section = %v, want SectionPlugin", i, call.section)
		}
		if call.name != wantNames[i] {
			t.Errorf("call[%d].name = %q, want %q", i, call.name, wantNames[i])
		}
		if call.done != wantDone[i] {
			t.Errorf("call[%d].done = %v, want %v", i, call.done, wantDone[i])
		}
		if call.action != wantActions[i] {
			t.Errorf("call[%d].action = %q, want %q", i, call.action, wantActions[i])
		}
	}
}

func TestApplyPluginImport_ProgressHooksOnError(t *testing.T) {
	client := newMockClient()
	client.failOn["install:bad-plugin"] = true
	plan := &ImportPlan{
		Add:      []string{"bad-plugin"},
		Skip:     []string{},
		Conflict: []string{},
		Remove:   []string{},
	}
	desired := map[string]manifest.Plugin{
		"bad-plugin": {ID: "bad-plugin", Enabled: true},
	}

	hooks := &mockProgressHooks{mockHooks: applyHooks()}
	ApplyPluginImport(context.Background(), client, plan, desired, nil, hooks, "")

	// OnOpStart + OnOpDone (with error)
	if len(hooks.calls) != 2 {
		t.Fatalf("progress calls = %d, want 2", len(hooks.calls))
	}
	if hooks.calls[1].err == nil {
		t.Error("expected error on done call")
	}
}

func TestApplyPluginImport_ContextCancellation(t *testing.T) {
	client := newMockClient()
	plan := &ImportPlan{
		Remove:   []string{"a", "b", "c"},
		Add:      []string{},
		Skip:     []string{},
		Conflict: []string{},
	}

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel before starting — all ops should be skipped.
	cancel()

	result := ApplyPluginImport(ctx, client, plan, nil, nil, applyHooks(), "")

	if len(result.Errors) != 1 {
		t.Fatalf("Errors = %d, want 1 (context canceled)", len(result.Errors))
	}
	if !strings.Contains(result.Errors[0].Error(), "canceled") {
		t.Errorf("error should mention canceled: %v", result.Errors[0])
	}
	if len(client.calls) != 0 {
		t.Errorf("no client calls expected, got %v", client.calls)
	}
}

func TestApplyMarketplaceImport_ProgressHooks(t *testing.T) {
	client := newMockClient()
	plan := &ImportPlan{
		Add:      []string{"new-mkt"},
		Skip:     []string{},
		Conflict: []string{"conflict-mkt"},
		Remove:   []string{"old-mkt"},
	}
	desired := map[string]manifest.Marketplace{
		"new-mkt":      {Source: "github", Repo: "owner/new"},
		"conflict-mkt": {Source: "github", Repo: "owner/conflict"},
	}

	hooks := &mockProgressHooks{mockHooks: applyHooks()}
	ApplyMarketplaceImport(context.Background(), client, plan, desired, []string{"conflict-mkt"}, hooks)

	// Add(1) + Overwrite(1) + Remove(1) = 3 operations, each with start+done = 6 calls
	if len(hooks.calls) != 6 {
		t.Fatalf("progress calls = %d, want 6", len(hooks.calls))
	}
}

func TestApplyMarketplaceImport_ContextCancellation(t *testing.T) {
	client := newMockClient()
	plan := &ImportPlan{
		Add:      []string{"mkt-a", "mkt-b"},
		Skip:     []string{},
		Conflict: []string{},
		Remove:   []string{},
	}
	desired := map[string]manifest.Marketplace{
		"mkt-a": {Source: "github", Repo: "owner/a"},
		"mkt-b": {Source: "github", Repo: "owner/b"},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result := ApplyMarketplaceImport(ctx, client, plan, desired, nil, applyHooks())

	if len(result.Errors) != 1 {
		t.Fatalf("Errors = %d, want 1 (context canceled)", len(result.Errors))
	}
	if len(client.calls) != 0 {
		t.Errorf("no client calls expected, got %v", client.calls)
	}
}
