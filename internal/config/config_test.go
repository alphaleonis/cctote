package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func boolPtr(b bool) *bool { return &b }

func strPtr(s string) *string { return &s }

func TestRoundTrip(t *testing.T) {
	c := &Config{
		ManifestPath: "/custom/path/manifest.json",
		Chezmoi: ChezmoiConfig{
			Enabled:   boolPtr(true),
			AutoReAdd: strPtr("always"),
		},
	}

	path := filepath.Join(t.TempDir(), "cctote.toml")
	if err := Save(path, c); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if got.ManifestPath != c.ManifestPath {
		t.Errorf("ManifestPath = %q, want %q", got.ManifestPath, c.ManifestPath)
	}
	if got.Chezmoi.Enabled == nil || *got.Chezmoi.Enabled != true {
		t.Errorf("Chezmoi.Enabled = %v, want true", got.Chezmoi.Enabled)
	}
	if got.Chezmoi.AutoReAdd == nil || *got.Chezmoi.AutoReAdd != "always" {
		t.Errorf("Chezmoi.AutoReAdd = %v, want \"always\"", got.Chezmoi.AutoReAdd)
	}
}

func TestRoundTripDefaults(t *testing.T) {
	c := &Config{}

	path := filepath.Join(t.TempDir(), "cctote.toml")
	if err := Save(path, c); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if got.ManifestPath != "" {
		t.Errorf("ManifestPath = %q, want empty", got.ManifestPath)
	}
	if got.Chezmoi.Enabled != nil {
		t.Errorf("Chezmoi.Enabled = %v, want nil", got.Chezmoi.Enabled)
	}
	if got.Chezmoi.AutoReAdd != nil {
		t.Errorf("Chezmoi.AutoReAdd = %v, want nil", got.Chezmoi.AutoReAdd)
	}
}

func TestLoadMissingFileReturnsDefaults(t *testing.T) {
	got, err := Load(filepath.Join(t.TempDir(), "nonexistent.toml"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if got.ManifestPath != "" {
		t.Errorf("ManifestPath = %q, want empty", got.ManifestPath)
	}
	if got.Chezmoi.Enabled != nil {
		t.Errorf("Chezmoi.Enabled = %v, want nil", got.Chezmoi.Enabled)
	}
}

func TestLoadMalformedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.toml")
	if err := os.WriteFile(path, []byte("not valid [[[ toml"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for malformed TOML")
	}

	want := "parsing config"
	if got := err.Error(); !contains(got, want) {
		t.Errorf("error = %q, want it to contain %q", got, want)
	}
}

func TestSaveCreatesDirectories(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a", "b", "c", "cctote.toml")
	if err := Save(path, &Config{ManifestPath: "/test"}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.ManifestPath != "/test" {
		t.Errorf("ManifestPath = %q, want %q", got.ManifestPath, "/test")
	}
}

func TestRoundTripExplicitValues(t *testing.T) {
	// Explicit values must survive a Save→Load round-trip with the fields
	// present in the TOML file, so "explicitly set" is distinguishable from "never set".
	f := false
	never := "never"
	c := &Config{
		ManifestPath: "/test",
		Chezmoi: ChezmoiConfig{
			Enabled:   &f,
			AutoReAdd: &never,
		},
	}

	path := filepath.Join(t.TempDir(), "cctote.toml")
	if err := Save(path, c); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// The TOML file must contain the fields explicitly.
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	tomlStr := string(raw)
	if !strings.Contains(tomlStr, "enabled") {
		t.Errorf("expected 'enabled' in TOML file, got:\n%s", tomlStr)
	}
	if !strings.Contains(tomlStr, "auto_re_add") {
		t.Errorf("expected 'auto_re_add' in TOML file, got:\n%s", tomlStr)
	}

	// Load back and verify explicit values are preserved (not nil).
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Chezmoi.Enabled == nil {
		t.Error("Chezmoi.Enabled = nil after round-trip, want *false")
	} else if *got.Chezmoi.Enabled != false {
		t.Errorf("Chezmoi.Enabled = %v, want false", *got.Chezmoi.Enabled)
	}
	if got.Chezmoi.AutoReAdd == nil {
		t.Error("Chezmoi.AutoReAdd = nil after round-trip, want *\"never\"")
	} else if *got.Chezmoi.AutoReAdd != "never" {
		t.Errorf("Chezmoi.AutoReAdd = %v, want \"never\"", *got.Chezmoi.AutoReAdd)
	}
}

func TestUnsetFieldsAreNil(t *testing.T) {
	// A missing config file should produce nil pointers (not zero values).
	got, err := Load(filepath.Join(t.TempDir(), "nonexistent.toml"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Chezmoi.Enabled != nil {
		t.Errorf("Chezmoi.Enabled = %v, want nil for missing config", got.Chezmoi.Enabled)
	}
	if got.Chezmoi.AutoReAdd != nil {
		t.Errorf("Chezmoi.AutoReAdd = %v, want nil for missing config", got.Chezmoi.AutoReAdd)
	}
}

func TestDefaultPathXDGOverride(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/custom/config")

	got, err := DefaultPath()
	if err != nil {
		t.Fatalf("DefaultPath: %v", err)
	}

	want := filepath.Join("/custom/config", "cctote", "cctote.toml")
	if got != want {
		t.Errorf("DefaultPath() = %q, want %q", got, want)
	}
}

func TestDefaultPathFallback(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")

	got, err := DefaultPath()
	if err != nil {
		t.Fatalf("DefaultPath: %v", err)
	}

	home, _ := os.UserHomeDir()
	want := filepath.Join(home, ".config", "cctote", "cctote.toml")
	if got != want {
		t.Errorf("DefaultPath() = %q, want %q", got, want)
	}
}

// --- keys.go tests ---

// wantKeys is the single source of truth for expected config keys.
// Update this table when adding or removing keys in keys.go.
var wantKeys = []struct {
	name    string
	keyType KeyType
}{
	{"manifest_path", KeyTypeString},
	{"chezmoi.enabled", KeyTypeBool},
	{"chezmoi.auto_re_add", KeyTypeEnum},
}

func TestKeysRegistry(t *testing.T) {
	all := Keys()
	if len(all) != len(wantKeys) {
		t.Fatalf("len(Keys()) = %d, want %d", len(all), len(wantKeys))
	}

	for i, wk := range wantKeys {
		// Verify Keys() order and content.
		if all[i].Name != wk.name {
			t.Errorf("Keys()[%d].Name = %q, want %q", i, all[i].Name, wk.name)
		}
		if all[i].Type != wk.keyType {
			t.Errorf("Keys()[%d].Type = %q, want %q", i, all[i].Type, wk.keyType)
		}

		// Verify LookupKey roundtrip.
		k, err := LookupKey(wk.name)
		if err != nil {
			t.Fatalf("LookupKey(%q): %v", wk.name, err)
		}
		if k.Name != wk.name {
			t.Errorf("LookupKey(%q).Name = %q", wk.name, k.Name)
		}
	}
}

func TestLookupKeyUnknown(t *testing.T) {
	_, err := LookupKey("nonexistent_key")
	if err == nil {
		t.Fatal("expected error for unknown key")
	}
	if !contains(err.Error(), "unknown config key") {
		t.Errorf("error = %q, want it to contain 'unknown config key'", err.Error())
	}
}

func TestKeyDefGetSetReset(t *testing.T) {
	c := &Config{}
	path := "/custom/manifest.json"

	k, err := LookupKey("manifest_path")
	if err != nil {
		t.Fatalf("LookupKey: %v", err)
	}
	if err := k.Set(c, path); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if got := c.ManifestPath; got != path {
		t.Errorf("after Set, ManifestPath = %q, want %q", got, path)
	}
	if !k.IsSet(c) {
		t.Error("IsSet = false after Set, want true")
	}
	if got := k.Get(c); got != path {
		t.Errorf("Get = %q, want %q", got, path)
	}

	k.Reset(c)
	if k.IsSet(c) {
		t.Error("IsSet = true after Reset, want false")
	}
}

func TestKeyDefBoolSetInvalid(t *testing.T) {
	c := &Config{}
	k, err := LookupKey("chezmoi.enabled")
	if err != nil {
		t.Fatalf("LookupKey: %v", err)
	}
	if err := k.Set(c, "not_a_bool"); err == nil {
		t.Fatal("expected error for invalid bool value")
	}
}

func TestBoolPtrAndBoolVal(t *testing.T) {
	tests := []struct {
		name    string
		input   *bool
		wantVal bool
	}{
		{"nil returns false", nil, false},
		{"true returns true", BoolPtr(true), true},
		{"false returns false", BoolPtr(false), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BoolVal(tt.input)
			if got != tt.wantVal {
				t.Errorf("BoolVal(%v) = %v, want %v", tt.input, got, tt.wantVal)
			}
		})
	}

	// BoolPtr round-trip: pointer identity must differ, value must match.
	pTrue := BoolPtr(true)
	pFalse := BoolPtr(false)
	if *pTrue != true {
		t.Errorf("BoolPtr(true) = %v, want true", *pTrue)
	}
	if *pFalse != false {
		t.Errorf("BoolPtr(false) = %v, want false", *pFalse)
	}
	if pTrue == pFalse {
		t.Error("BoolPtr(true) and BoolPtr(false) should be different pointers")
	}
}

func TestKeyDefGetFallbackDefault(t *testing.T) {
	// When manifest_path is not set, Get should return the default path.
	c := &Config{} // ManifestPath == ""
	k, err := LookupKey("manifest_path")
	if err != nil {
		t.Fatalf("LookupKey: %v", err)
	}

	got := k.Get(c)
	def := k.Default()
	if got != def {
		t.Errorf("Get on unset config = %q, want default %q", got, def)
	}
	if got == "" {
		t.Error("expected a non-empty default manifest path")
	}
}

func TestKeyDefBoolGetSetResetCycle(t *testing.T) {
	c := &Config{}
	k, err := LookupKey("chezmoi.enabled")
	if err != nil {
		t.Fatalf("LookupKey: %v", err)
	}

	// Initially unset.
	if k.IsSet(c) {
		t.Error("IsSet = true on fresh config, want false")
	}
	if got := k.Get(c); got != "false" {
		t.Errorf("Get on unset = %q, want %q", got, "false")
	}

	// Set to true.
	if err := k.Set(c, "true"); err != nil {
		t.Fatalf("Set(true): %v", err)
	}
	if !k.IsSet(c) {
		t.Error("IsSet = false after Set, want true")
	}
	if got := k.Get(c); got != "true" {
		t.Errorf("Get after Set(true) = %q, want %q", got, "true")
	}

	// Reset.
	k.Reset(c)
	if k.IsSet(c) {
		t.Error("IsSet = true after Reset, want false")
	}
}

func TestKeyDefEnumGetSetResetCycle(t *testing.T) {
	c := &Config{}
	k, err := LookupKey("chezmoi.auto_re_add")
	if err != nil {
		t.Fatalf("LookupKey: %v", err)
	}

	// Initially unset — default is "never".
	if k.IsSet(c) {
		t.Error("IsSet = true on fresh config, want false")
	}
	if got := k.Get(c); got != "never" {
		t.Errorf("Get on unset = %q, want %q", got, "never")
	}

	// Set to each valid value.
	for _, val := range []string{"never", "ask", "always"} {
		if err := k.Set(c, val); err != nil {
			t.Fatalf("Set(%q): %v", val, err)
		}
		if !k.IsSet(c) {
			t.Errorf("IsSet = false after Set(%q), want true", val)
		}
		if got := k.Get(c); got != val {
			t.Errorf("Get after Set(%q) = %q", val, got)
		}
	}

	// Invalid value.
	if err := k.Set(c, "invalid"); err == nil {
		t.Error("expected error for invalid enum value")
	}

	// Reset.
	k.Reset(c)
	if k.IsSet(c) {
		t.Error("IsSet = true after Reset, want false")
	}
	if got := k.Get(c); got != "never" {
		t.Errorf("Get after Reset = %q, want %q", got, "never")
	}
}

func TestAutoReAddMode(t *testing.T) {
	if got := AutoReAddMode(nil); got != "never" {
		t.Errorf("AutoReAddMode(nil) = %q, want %q", got, "never")
	}
	ask := "ask"
	if got := AutoReAddMode(&ask); got != "ask" {
		t.Errorf("AutoReAddMode(&\"ask\") = %q, want %q", got, "ask")
	}
}

func TestStrPtr(t *testing.T) {
	p := StrPtr("test")
	if *p != "test" {
		t.Errorf("StrPtr(\"test\") = %q, want %q", *p, "test")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
