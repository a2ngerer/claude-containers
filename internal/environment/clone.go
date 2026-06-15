// internal/environment/clone.go
package environment

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/angerer/claude_git/internal/storage"
)

// defaultGitignore is a byte-identical copy of share.DefaultGitignore(). It is
// inlined here to avoid an import cycle (share imports environment, never the
// reverse). TestGitignoreInSync asserts the two strings never drift apart.
const defaultGitignore = `# claude_git persona repo — secret-safe defaults (DO NOT relax)
# Claude Code local layer (coder's machine-local tweaks must never be shared)
settings.local.json

# environment / dotenv
.env
.env.*

# private keys and certificates
*.key
*.pem
*.p12
*.pfx
*.crt
id_rsa
id_rsa.*
id_dsa
id_dsa.*
id_ecdsa
id_ecdsa.*
id_ed25519
id_ed25519.*

# credential / secret / token bearing files
*credential*
*secret*
*.token
*.pwd
*password*

# cloud / SSH config dirs that may carry secrets
.aws/
.ssh/
.gnupg/
`

// writeGitignore writes repo/.gitignore with the secret-safe defaults. Both
// Create and CloneInto call it, so every environment repo carries the guard from
// the very first snapshot. share.Push re-asserts via ScanForSecrets as a runtime
// backstop, but creation-time is the primary guarantee.
func writeGitignore(repoDir string) error {
	path := filepath.Join(repoDir, ".gitignore")
	if err := os.WriteFile(path, []byte(defaultGitignore), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// writeMarker writes the one-line <workspace>/.claude_git marker (= workspace
// hash). The format matches the CLI init path so both entry points agree.
func writeMarker(absWorkspace, hash string) error {
	path := filepath.Join(absWorkspace, ".claude_git")
	if err := os.WriteFile(path, []byte(hash+"\n"), 0o644); err != nil {
		return fmt.Errorf("write workspace marker %s: %w", path, err)
	}
	return nil
}

// CloneInto onboards an existing persona repo into a NEW environment bound to
// workspace. It initialises the git-backed store at RepoDir(hash), fetches the
// remote's persona and tag refs, writes the secret-safe .gitignore and env.toml,
// and drops the <workspace>/.claude_git marker. The returned environment is ready
// for list, use, etc. Used by share.Clone and by `claude_git init --from`.
//
// The repo is fetched, not git-cloned: persona timelines live under
// refs/personas/* and tags under refs/tags/*, which a default clone (refs/heads/*)
// would not pull. AddRemote+Pull uses exactly those refspecs.
func CloneInto(workspace, remote string) (*Environment, error) {
	abs, err := filepath.Abs(workspace)
	if err != nil {
		return nil, fmt.Errorf("resolve workspace %q: %w", workspace, err)
	}
	// EvalSymlinks resolves macOS /var -> /private/var so the hash matches the
	// one Open()/init compute for the same workspace.
	abs, err = filepath.EvalSymlinks(abs)
	if err != nil {
		return nil, fmt.Errorf("resolve symlinks for workspace %q: %w", workspace, err)
	}
	abs = filepath.Clean(abs)
	hash := WorkspaceHash(abs)

	if _, statErr := os.Stat(envConfigPath(hash)); statErr == nil {
		return nil, fmt.Errorf("workspace %q is already initialized", abs)
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return nil, fmt.Errorf("stat env config: %w", statErr)
	}

	if err := os.MkdirAll(EnvDir(hash), 0o755); err != nil {
		return nil, fmt.Errorf("create env dir: %w", err)
	}

	store, err := storage.OpenGit(RepoDir(hash))
	if err != nil {
		return nil, fmt.Errorf("open storage: %w", err)
	}
	if err := store.AddRemote("origin", remote); err != nil {
		return nil, fmt.Errorf("add remote %q: %w", remote, err)
	}
	if err := store.Pull("origin"); err != nil {
		return nil, fmt.Errorf("fetch persona refs from %q: %w", remote, err)
	}
	if err := writeGitignore(RepoDir(hash)); err != nil {
		return nil, err
	}

	cfg := EnvConfig{
		WorkspacePath: abs,
		ActivePersona: "",
		Author:        defaultAuthor(),
	}
	if err := writeEnvConfig(hash, cfg); err != nil {
		return nil, err
	}
	if err := writeMarker(abs, hash); err != nil {
		return nil, err
	}

	return &Environment{Hash: hash, Workspace: abs, Store: store, cfg: cfg}, nil
}
