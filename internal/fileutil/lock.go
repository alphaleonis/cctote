package fileutil

import (
	"fmt"
	"os"
	"sync"
	"time"
)

const (
	// staleThreshold matches proper-lockfile's default: a lock directory
	// with an mtime older than this is considered abandoned.
	staleThreshold = 10 * time.Second

	// heartbeatInterval is how often the lock holder refreshes mtime.
	// Must be at most staleThreshold/3 to tolerate a missed tick
	// (GC pause, I/O delay) while still appearing fresh to contenders.
	// If you change staleThreshold, update this value to maintain the ratio.
	heartbeatInterval = 3 * time.Second

	// lockRetries is the maximum number of times to retry when the lock
	// is held by a fresh (non-stale) process.
	lockRetries = 3

	// lockRetryDelay is the pause between retries against a fresh lock.
	lockRetryDelay = 200 * time.Millisecond
)

// AcquireLock creates a proper-lockfile compatible directory lock at
// path+".lock". This cooperates with Claude Code's own locking of
// ~/.claude.json. On success it returns a release function that removes
// the lock directory. The caller must always call release (use defer).
//
// The release function intentionally ignores os.Remove errors; a lingering
// lock directory will be reclaimed as stale after 10 seconds by the next caller.
func AcquireLock(path string) (release func(), err error) {
	lockPath := path + ".lock"

	for attempt := 0; ; attempt++ {
		if err := os.Mkdir(lockPath, 0o755); err == nil {
			stop := make(chan struct{})
			done := make(chan struct{})
			go heartbeat(lockPath, stop, done)
			var once sync.Once
			return func() {
				once.Do(func() {
					close(stop)
					<-done // Wait for heartbeat exit before removing lock,
					// so it cannot Chtimes a dir owned by a new acquirer.
					_ = os.Remove(lockPath)
				})
			}, nil
		} else if !os.IsExist(err) {
			return nil, fmt.Errorf("acquiring config lock: %w", err)
		}

		// Lock directory exists — check if it's stale.
		info, statErr := os.Stat(lockPath)
		if statErr != nil {
			if os.IsNotExist(statErr) {
				continue // vanished between mkdir and stat — retry immediately
			}
			return nil, fmt.Errorf("checking config lock: %w", statErr)
		}

		if time.Since(info.ModTime()) > staleThreshold {
			// Stale lock: owner likely crashed. Remove and retry mkdir.
			//
			// TOCTOU note: another process may also detect staleness and
			// race to remove/reclaim. This is safe because os.Mkdir on the
			// next iteration is the atomic arbitrator — only one process's
			// mkdir will succeed. The losing process will see EEXIST and
			// wait or retry normally.
			if err := os.Remove(lockPath); err != nil && !os.IsNotExist(err) {
				return nil, fmt.Errorf("removing stale config lock: %w", err)
			}
			continue // retry mkdir immediately, no sleep or retry count
		}

		// Lock is fresh — held by another process. Give up after retries.
		if attempt >= lockRetries {
			break
		}
		time.Sleep(lockRetryDelay)
	}

	return nil, fmt.Errorf("could not acquire lock on %s: held by another process", path)
}

// heartbeat periodically refreshes the lock directory's mtime so that
// contenders do not mistake a long-held lock for a stale one.
func heartbeat(lockPath string, stop <-chan struct{}, done chan<- struct{}) {
	defer close(done)
	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			now := time.Now()
			// Chtimes may fail if the lock directory was removed externally.
			// If so, stop the heartbeat — refreshing is pointless and the lock
			// will be reclaimed as stale after staleThreshold.
			if err := os.Chtimes(lockPath, now, now); err != nil {
				return
			}
		}
	}
}
