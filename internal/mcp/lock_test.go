package mcp

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/alphaleonis/cctote/internal/manifest"
)

func TestWriteMcpServersAcquiresLock(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".claude.json")
	lockPath := path + ".lock"

	// Create a fresh lock to simulate contention.
	if err := os.Mkdir(lockPath, 0o755); err != nil {
		t.Fatal(err)
	}

	err := WriteMcpServers(path, nil)
	if err == nil {
		t.Fatal("WriteMcpServers should fail when lock is held")
	}

	// Remove the lock, verify write succeeds.
	if err := os.Remove(lockPath); err != nil {
		t.Fatalf("removing lock for test: %v", err)
	}

	if err := WriteMcpServers(path, map[string]manifest.MCPServer{
		"test": {Command: "echo"},
	}); err != nil {
		t.Fatalf("WriteMcpServers should succeed after lock released: %v", err)
	}

	// Lock should be cleaned up.
	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Error("lock directory should be removed after WriteMcpServers")
	}
}
