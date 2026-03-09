package fileutil

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestAcquireAndRelease(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.json")

	release, err := AcquireLock(path)
	if err != nil {
		t.Fatalf("AcquireLock: %v", err)
	}

	lockPath := path + ".lock"
	info, err := os.Stat(lockPath)
	if err != nil {
		t.Fatalf("lock directory should exist: %v", err)
	}
	if !info.IsDir() {
		t.Error("lock should be a directory")
	}

	release()

	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Error("lock directory should be removed after release")
	}
}

func TestAcquireStaleLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.json")
	lockPath := path + ".lock"

	// Create a stale lock (mtime well beyond the threshold).
	if err := os.Mkdir(lockPath, 0o755); err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-1 * time.Minute)
	if err := os.Chtimes(lockPath, past, past); err != nil {
		t.Fatal(err)
	}

	release, err := AcquireLock(path)
	if err != nil {
		t.Fatalf("should reclaim stale lock: %v", err)
	}
	defer release()

	// Verify we own the lock.
	if _, err := os.Stat(lockPath); err != nil {
		t.Fatalf("lock directory should exist: %v", err)
	}
}

func TestAcquireFreshLockFails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.json")
	lockPath := path + ".lock"

	// Create a fresh lock (held by another process).
	if err := os.Mkdir(lockPath, 0o755); err != nil {
		t.Fatal(err)
	}

	// NOTE: This test takes ~600ms due to lock retry delays.
	release, err := AcquireLock(path)
	if release != nil {
		defer release()
	}
	if err == nil {
		t.Fatal("should fail when lock is held")
	}
}

func TestHeartbeatKeepsLockFresh(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow heartbeat test in -short mode")
	}

	path := filepath.Join(t.TempDir(), "test.json")
	lockPath := path + ".lock"

	release, err := AcquireLock(path)
	if err != nil {
		t.Fatalf("AcquireLock: %v", err)
	}
	defer release()

	info, err := os.Stat(lockPath)
	if err != nil {
		t.Fatalf("stat lock: %v", err)
	}
	initialMtime := info.ModTime()

	// Poll for up to 2x the heartbeat interval to tolerate scheduling jitter.
	deadline := time.Now().Add(2 * heartbeatInterval)
	var currentMtime time.Time
	for time.Now().Before(deadline) {
		info, err = os.Stat(lockPath)
		if err != nil {
			t.Fatalf("stat lock during poll: %v", err)
		}
		currentMtime = info.ModTime()
		if currentMtime.After(initialMtime) {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if !currentMtime.After(initialMtime) {
		t.Errorf("mtime was not refreshed within deadline: initial=%v current=%v", initialMtime, currentMtime)
	}
}

func TestLongHeldLockNotReclaimable(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow lock contention test in -short mode")
	}

	path := filepath.Join(t.TempDir(), "test.json")

	release, err := AcquireLock(path)
	if err != nil {
		t.Fatalf("AcquireLock: %v", err)
	}
	defer release()

	// Hold the lock well past the stale threshold. The heartbeat keeps
	// it fresh, so a contender should fail to acquire.
	time.Sleep(staleThreshold + 2*time.Second)

	// Precondition: verify heartbeat actually kept the lock fresh.
	info, statErr := os.Stat(path + ".lock")
	if statErr != nil {
		t.Fatalf("stat lock before contender: %v", statErr)
	}
	if time.Since(info.ModTime()) > staleThreshold {
		t.Fatal("precondition failed: heartbeat did not keep lock fresh")
	}

	contenderRelease, err := AcquireLock(path)
	if contenderRelease != nil {
		contenderRelease()
		t.Fatal("contender should not have acquired the lock")
	}
	if err == nil {
		t.Fatal("contender should get an error")
	}
}

func TestHeartbeatStopsOnRelease(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.json")
	lockPath := path + ".lock"

	goroutinesBefore := runtime.NumGoroutine()

	release, err := AcquireLock(path)
	if err != nil {
		t.Fatalf("AcquireLock: %v", err)
	}

	// release() waits for the heartbeat goroutine to exit (<-done),
	// so the goroutine is guaranteed gone after this call returns.
	release()

	// Lock directory should be removed.
	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Error("lock directory should be removed after release")
	}

	// release() already drained the heartbeat goroutine via <-done.
	// Allow +2 tolerance for runtime-internal goroutine fluctuations.
	goroutinesAfter := runtime.NumGoroutine()
	if goroutinesAfter > goroutinesBefore+2 {
		t.Errorf("possible goroutine leak: before=%d after=%d", goroutinesBefore, goroutinesAfter)
	}
}

func TestReleaseThenReacquire(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.json")

	release1, err := AcquireLock(path)
	if err != nil {
		t.Fatalf("first AcquireLock: %v", err)
	}
	release1()

	release2, err := AcquireLock(path)
	if err != nil {
		t.Fatalf("second AcquireLock after release: %v", err)
	}
	defer release2()

	lockPath := path + ".lock"
	if _, err := os.Stat(lockPath); err != nil {
		t.Fatalf("lock directory should exist: %v", err)
	}

	// Verify the re-acquired lock is genuinely held.
	thirdRelease, thirdErr := AcquireLock(path)
	if thirdRelease != nil {
		thirdRelease()
		t.Fatal("third acquire should fail while second lock is held")
	}
	if thirdErr == nil {
		t.Fatal("expected error acquiring against held lock")
	}
}

func TestHeartbeatExitsOnLockRemoval(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.json")
	lockPath := path + ".lock"

	release, err := AcquireLock(path)
	if err != nil {
		t.Fatalf("AcquireLock: %v", err)
	}
	defer release()

	// Simulate external removal of the lock directory.
	if err := os.Remove(lockPath); err != nil {
		t.Fatalf("removing lock dir: %v", err)
	}

	// Wait for at least one heartbeat tick to detect the removal.
	time.Sleep(heartbeatInterval + time.Second)

	// A contender should be able to acquire since the directory is gone
	// and the heartbeat has stopped (no longer recreating it via Chtimes).
	contenderRelease, err := AcquireLock(path)
	if err != nil {
		t.Fatalf("contender should acquire after external removal: %v", err)
	}
	contenderRelease()
}
