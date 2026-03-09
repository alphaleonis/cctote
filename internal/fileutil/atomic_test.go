package fileutil

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestAtomicWriteHappyPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.json")
	data := []byte(`{"key": "value"}`)

	if err := AtomicWrite(path, data, 0o644); err != nil {
		t.Fatalf("AtomicWrite: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != string(data) {
		t.Errorf("content = %q, want %q", got, data)
	}
}

func TestAtomicWriteCreatesParentDirs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a", "b", "c", "deep.json")
	data := []byte("nested")

	if err := AtomicWrite(path, data, 0o644); err != nil {
		t.Fatalf("AtomicWrite: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != "nested" {
		t.Errorf("content = %q, want %q", got, "nested")
	}
}

func TestAtomicWriteSetsPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix permission bits are not supported on Windows")
	}

	path := filepath.Join(t.TempDir(), "perms.json")

	if err := AtomicWrite(path, []byte("data"), 0o600); err != nil {
		t.Fatalf("AtomicWrite: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	// Compare only the permission bits (mask off type bits).
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("permissions = %o, want %o", got, 0o600)
	}
}

func TestAtomicWriteOverwritesExisting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "overwrite.json")

	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := AtomicWrite(path, []byte("new"), 0o644); err != nil {
		t.Fatalf("AtomicWrite: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != "new" {
		t.Errorf("content = %q, want %q", got, "new")
	}
}

func TestAtomicWriteEmptyData(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.json")

	if err := AtomicWrite(path, []byte{}, 0o644); err != nil {
		t.Fatalf("AtomicWrite: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty file, got %d bytes", len(got))
	}
}

func TestAtomicWriteNoTempLeftBehind(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "clean.json")

	if err := AtomicWrite(path, []byte("data"), 0o644); err != nil {
		t.Fatalf("AtomicWrite: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 1 {
		names := make([]string, len(entries))
		for i, e := range entries {
			names[i] = e.Name()
		}
		t.Errorf("expected exactly 1 file, got %v", names)
	}
}
