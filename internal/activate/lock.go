package activate

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"

	"github.com/angerer/claude_git/internal/domain"
	"github.com/angerer/claude_git/internal/environment"
)

// lockState is the JSON body of the environment lockfile.
type lockState struct {
	Persona string `json:"persona"`
	PID     int    `json:"pid"`
}

// Lock is a held environment lock. It guards cross-process claude_git mutations
// for one workspace; it does NOT guard concurrent claude sessions (those are
// isolated by separate CLAUDE_CONFIG_DIRs).
type Lock struct {
	path  string
	state lockState
}

// Acquire takes the environment lock for persona. It succeeds when the lock is
// free, already held by this process, or held by a dead (stale) process. It
// returns domain.ErrLocked when a different, live process holds it.
func Acquire(e *environment.Environment, persona string) (*Lock, error) {
	path := filepath.Join(environment.EnvDir(e.Hash), "lock")

	if existing, ok, err := readLock(path); err != nil {
		return nil, err
	} else if ok {
		foreign := existing.PID != os.Getpid()
		if foreign && pidAlive(existing.PID) {
			return nil, fmt.Errorf("%w: held by %s (pid %d)", domain.ErrLocked, existing.Persona, existing.PID)
		}
		// own lock or stale lock -> fall through and overwrite
	}

	st := lockState{Persona: persona, PID: os.Getpid()}
	if err := writeLock(path, st); err != nil {
		return nil, err
	}
	return &Lock{path: path, state: st}, nil
}

// Release removes the lockfile if it is still owned by this lock's PID.
func (l *Lock) Release() error {
	current, ok, err := readLock(l.path)
	if err != nil {
		return err
	}
	if !ok || current.PID != l.state.PID {
		return nil // someone else owns it now; leave it alone
	}
	if err := os.Remove(l.path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("release lock: %w", err)
	}
	return nil
}

// readLock reads the lockfile. ok=false means no lockfile is present.
func readLock(path string) (lockState, bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return lockState{}, false, nil
	}
	if err != nil {
		return lockState{}, false, fmt.Errorf("read lock: %w", err)
	}
	var st lockState
	if err := json.Unmarshal(data, &st); err != nil {
		return lockState{}, false, fmt.Errorf("parse lock: %w", err)
	}
	return st, true, nil
}

func writeLock(path string, st lockState) error {
	data, err := json.Marshal(st)
	if err != nil {
		return fmt.Errorf("marshal lock: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create env dir: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write lock: %w", err)
	}
	return nil
}

// pidAlive reports whether a process with the given PID currently exists.
// Signal 0 performs error checking without actually sending a signal.
func pidAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	if err == nil {
		return true
	}
	// EPERM means the process exists but we may not signal it -> alive.
	return errors.Is(err, syscall.EPERM)
}
