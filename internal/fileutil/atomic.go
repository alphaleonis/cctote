// Package fileutil provides shared file operation utilities.
package fileutil

import (
	"fmt"
	"os"
	"path/filepath"
)

// AtomicWrite writes data to path atomically using a temp-file-plus-rename
// strategy. A crash mid-write will not leave the target file truncated or
// corrupt. Creates parent directories if they don't exist.
//
// On Go 1.21+ os.Rename uses MoveFileEx with MOVEFILE_REPLACE_EXISTING on
// Windows, so cross-platform atomic replace works.
func AtomicWrite(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating directory: %w", err)
	}

	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp.*")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpName := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("writing temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("closing temp file: %w", err)
	}

	// Preserve permissions: CreateTemp uses 0600; apply the requested mode.
	if err := os.Chmod(tmpName, perm); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("setting file permissions: %w", err)
	}

	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("replacing file: %w", err)
	}

	return nil
}
