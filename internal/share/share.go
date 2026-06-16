package share

import (
	"errors"
	"fmt"
	"strings"

	"github.com/a2ngerer/agent-containers/internal/environment"
)

// ErrSecretsFound is returned by Push when ScanForSecrets reports suspect paths.
// The error message lists every suspect path so the user can remove them.
var ErrSecretsFound = errors.New("refusing to push: secrets detected in persona repo")

// Push scans the persona repo for secrets and, only if clean, pushes it to remote.
// It MUST run the scan before any network operation: a single leaked key is a trust killer.
func Push(e *environment.Environment, remote string) error {
	repoDir := environment.RepoDir(e.Hash)

	suspects, err := ScanForSecrets(repoDir)
	if err != nil {
		return fmt.Errorf("pre-push secret scan: %w", err)
	}
	if len(suspects) > 0 {
		return fmt.Errorf("%w:\n  %s\nremove these files (or add them to .gitignore and untrack) before pushing",
			ErrSecretsFound, strings.Join(suspects, "\n  "))
	}

	if err := e.Store.Push(remote); err != nil {
		return fmt.Errorf("push to %q: %w", remote, err)
	}
	return nil
}

// Pull fetches and integrates remote changes into the persona repo.
// Unlike Push, Pull does not scan: foreign content is reviewed via show/diff before
// activation, never silently trusted, but pulling itself is not a leak vector.
func Pull(e *environment.Environment, remote string) error {
	if err := e.Store.Pull(remote); err != nil {
		return fmt.Errorf("pull from %q: %w", remote, err)
	}
	return nil
}

// Clone onboards an existing persona repo into a NEW environment bound to destWorkspace.
// It delegates to environment.CloneInto, which initialises a bare repo at RepoDir(hash),
// fetches from remote, writes env.toml, and sets the <workspace>/.acon marker.
// The returned environment is ready for list, use, etc. Used by the clone command and
// by init --from.
func Clone(remote, destWorkspace string) (*environment.Environment, error) {
	env, err := environment.CloneInto(destWorkspace, remote)
	if err != nil {
		return nil, fmt.Errorf("clone %q into workspace %q: %w", remote, destWorkspace, err)
	}
	return env, nil
}
