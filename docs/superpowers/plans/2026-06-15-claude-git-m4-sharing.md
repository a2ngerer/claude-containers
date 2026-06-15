# M4 Sharing — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (- [ ]) syntax for tracking.

**Goal:** Round-trip the hidden persona repo to any git remote with `push`/`pull`/`clone` while guaranteeing no secrets leak, and onboard team members via `init --from <remote>`.

**Architecture:** A new `internal/share` package layers secret-safe defaults (`gitignore.go`) and remote orchestration (`share.go`) on top of the M1 `storage.StorageEngine` (`AddRemote`/`Push`/`Pull`) and `environment` package. `Push` mandatorily runs `ScanForSecrets` first and aborts on any hit. The `internal/cli/share.go` command group (`push`/`pull`/`clone`) and a small modification to the M1 `internal/cli/init.go` (`--from` branch) expose this to the user; the `.gitignore` is written once at environment creation.

**Tech Stack:** `github.com/spf13/cobra`, `github.com/go-git/go-git/v5`, `github.com/pelletier/go-toml/v2`, `github.com/stretchr/testify/require`, standard `testing`.

**Depends on:** M1 (storage remotes `AddRemote`/`Push`/`Pull` + `environment.Create`/`Open` + `internal/cli/init.go`).

---

## File Structure

| File | Responsibility (one line) |
|---|---|
| `internal/share/gitignore.go` | `DefaultGitignore()` content + `ScanForSecrets(dir)` for tracked secret files / key contents. |
| `internal/share/gitignore_test.go` | Tests: gitignore contains key patterns; scan positive (`*.key`, `-----BEGIN`) and negative (clean repo). |
| `internal/share/share.go` | `Push`/`Pull`/`Clone` wrapping `Store.AddRemote`/`Push`/`Pull`; `Push` scans first and aborts with the suspect list. |
| `internal/share/share_test.go` | Tests: push/pull round-trip against a local bare remote; clone round-trip; push aborts on secret. |
| `internal/cli/share.go` | `push [remote]`, `pull [remote]`, `clone <remote>` cobra commands (thin; delegate to `share`). |
| `internal/cli/share_test.go` | Test: `clone` command writes `repo/`, `env.toml`, and the `.claude_git` marker for the current workspace. |
| `internal/cli/share_e2e_test.go` | End-to-end test: init -> push -> clone round-trip through the CLI. |
| `internal/cli/init_from_test.go` | Test: `init --from <remote>` clones an existing persona repo for onboarding. |
| `internal/cli/init.go` | **MODIFY (M1 file):** add a `--from <remote>` branch that clones an existing persona repo instead of seeding `_base`. |
| `internal/cli/root.go` | **MODIFY (M1 file):** register `push`/`pull`/`clone` on the root command. |
| `internal/environment/environment.go` | **MODIFY (M1 file):** `Create` writes `DefaultGitignore()` into `repo/.gitignore` and commits it as part of repo bootstrap. |
| `internal/environment/clone.go` | New: `CloneInto()` constructor + inlined gitignore + write helpers (avoids the share->environment cycle). |
| `internal/environment/clone_test.go` | Tests: `Create` writes `.gitignore`; `CloneInto` populates+opens; drift guard vs `share.DefaultGitignore`. |

New types beyond the Architecture Contract are flagged at each task and summarized at the end.

---

## Contract anchors used by this milestone (do not redefine)

From `internal/storage/engine.go`:

```go
AddRemote(name, url string) error
Push(remote string) error
Pull(remote string) error
```

From `internal/environment/environment.go`:

```go
func Create(workspace string) (*Environment, error)
func Open(workspace string) (*Environment, error)
type Environment struct {
	Hash      string
	Workspace string
	Store     storage.StorageEngine
	cfg       EnvConfig
}
type EnvConfig struct {
	WorkspacePath string `toml:"workspace_path"`
	ActivePersona string `toml:"active_persona"`
}
```

From `internal/environment/paths.go`:

```go
func WorkspaceHash(absWorkspace string) string
func ToolHome() string
func EnvDir(hash string) string
func RepoDir(hash string) string
```

From `internal/share/gitignore.go` (contract sketch — this milestone implements it):

```go
func DefaultGitignore() string
```

**M1-provided helpers this milestone assumes exist** (from `internal/cli/init.go`, M1). If a name differs in the actual M1 implementation, adapt the call site, not the contract:

- `internal/cli/init.go` defines `newInitCmd() *cobra.Command` whose `RunE` resolves the absolute workspace (`os.Getwd` + `filepath.Abs`), calls `environment.Create(workspace)`, seeds `_base` from the workspace `.claude/`, and writes the `<workspace>/.claude_git` marker (one line = workspace hash).

---

## Task 1 — `DefaultGitignore()`: secret-safe ignore content

The persona repo must never track secrets. `DefaultGitignore()` returns the canonical `.gitignore` body written into every persona repo at creation (Task 5) and re-asserted before push (Task 3 fallback). No secret may be shared (spec §13).

### Step 1.1 — RED: write the failing test

- [ ] **Files:** `internal/share/gitignore_test.go` (new)

```go
package share

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDefaultGitignore_ContainsKeyPatterns(t *testing.T) {
	got := DefaultGitignore()

	mustContain := []string{
		"settings.local.json",
		"*.key",
		"*.pem",
		"*.p12",
		".env",
		".env.*",
		"id_rsa",
		"id_rsa.*",
		"*credential*",
		"*secret*",
		"*.pfx",
		"id_ed25519",
		"id_ecdsa",
		"*.token",
		"*.crt",
		".aws/",
		".ssh/",
	}
	for _, pat := range mustContain {
		require.Contains(t, got, pat, "gitignore must exclude %q", pat)
	}
}

func TestDefaultGitignore_EndsWithNewline(t *testing.T) {
	got := DefaultGitignore()
	require.True(t, strings.HasSuffix(got, "\n"), "gitignore must end with a trailing newline")
}
```

- [ ] **Run:** `cd /Users/angeral/Repositories/claude_git && go test ./internal/share/`
- [ ] **Expected:** compile failure — `undefined: DefaultGitignore`.

### Step 1.2 — GREEN: implement `DefaultGitignore()`

- [ ] **Files:** `internal/share/gitignore.go` (new)

```go
// Package share implements S1 sharing of a persona repo to any git remote,
// with mandatory secret-safe defaults. No secret may ever be committed or pushed.
package share

// DefaultGitignore returns the canonical .gitignore body for a persona repo.
// It is written into repo/.gitignore at environment creation (M1) and excludes
// Claude-local settings, key material, credentials, and common secret patterns.
// A shared persona leaking an API key is an instant trust killer (spec §13).
func DefaultGitignore() string {
	return `# claude_git persona repo — secret-safe defaults (DO NOT relax)
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
}
```

- [ ] **Run:** `cd /Users/angeral/Repositories/claude_git && go test ./internal/share/`
- [ ] **Expected:** `ok  github.com/a2ngerer/claude-containers/internal/share`.

### Step 1.3 — REFACTOR

- [ ] No refactor needed; the function is a single string literal. Confirm formatting:
- [ ] **Run:** `cd /Users/angeral/Repositories/claude_git && gofmt -l internal/share/gitignore.go`
- [ ] **Expected:** empty output (already formatted).

### Step 1.4 — COMMIT

- [ ] **Run:** `cd /Users/angeral/Repositories/claude_git && git add internal/share/gitignore.go internal/share/gitignore_test.go && git commit -m "share: add DefaultGitignore with secret-safe patterns"`

**New types beyond contract:** none (`DefaultGitignore` is in the contract sketch).

---

## Task 2 — `ScanForSecrets(dir)`: detect tracked secret files and key contents

`ScanForSecrets` walks the persona repo and returns suspect paths: files whose name matches a secret pattern, OR files whose content contains an obvious key/token marker (`-----BEGIN`, `sk-`, `ghp_`). This is the runtime guard layered on top of `.gitignore` — `.gitignore` prevents accidental tracking, the scan catches anything already present (e.g. a force-added file or pre-existing repo). Returned paths are relative to `dir`, sorted for determinism.

### Step 2.1 — RED: write the failing test

- [ ] **Files:** `internal/share/gitignore_test.go` (append; also replace the Step 1.1 import block with the merged block shown below)

```go
import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)
```

```go
func TestScanForSecrets_FlagsKeyFilename(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "deploy.key"), []byte("irrelevant body"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "persona.toml"), []byte("name = \"coder\"\n"), 0o644))

	got, err := ScanForSecrets(dir)
	require.NoError(t, err)
	require.Equal(t, []string{"deploy.key"}, got)
}

func TestScanForSecrets_FlagsBeginMarkerContent(t *testing.T) {
	dir := t.TempDir()
	body := "-----BEGIN RSA PRIVATE KEY-----\nMIIEow...\n-----END RSA PRIVATE KEY-----\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "leaked"), []byte(body), 0o600))

	got, err := ScanForSecrets(dir)
	require.NoError(t, err)
	require.Equal(t, []string{"leaked"}, got)
}

func TestScanForSecrets_FlagsTokenPrefixContent(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "notes.md"), []byte("key: sk-ABCDEF0123456789abcdef\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "gh.txt"), []byte("ghp_0123456789abcdefABCDEF0123456789abcd\n"), 0o644))

	got, err := ScanForSecrets(dir)
	require.NoError(t, err)
	require.Equal(t, []string{"gh.txt", "notes.md"}, got)
}

func TestScanForSecrets_CleanRepoReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "personas", "coder"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "personas", "coder", "persona.toml"), []byte("name = \"coder\"\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "personas", "coder", "CLAUDE.md"), []byte("# Coder\nBuild things.\n"), 0o644))

	got, err := ScanForSecrets(dir)
	require.NoError(t, err)
	require.Empty(t, got)
}

func TestScanForSecrets_SkipsDotGit(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".git"), 0o755))
	// a file inside .git that would otherwise match by content must be ignored
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".git", "config"), []byte("sk-deadbeefdeadbeefdeadbeef\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "persona.toml"), []byte("name = \"coder\"\n"), 0o644))

	got, err := ScanForSecrets(dir)
	require.NoError(t, err)
	require.Empty(t, got)
}
```

- [ ] **Run:** `cd /Users/angeral/Repositories/claude_git && go test ./internal/share/`
- [ ] **Expected:** compile failure — `undefined: ScanForSecrets`.

### Step 2.2 — GREEN: implement `ScanForSecrets`

- [ ] **Files:** `internal/share/gitignore.go` (append; merge this import block into the file's single import section directly under the `package share` doc comment — Go forbids two import blocks per file)

```go
import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// secretNameGlobs are filename globs that mark a path as secret-bearing.
// Matched case-insensitively against the base name of each file.
var secretNameGlobs = []string{
	"settings.local.json",
	".env",
	".env.*",
	"*.key",
	"*.pem",
	"*.p12",
	"*.pfx",
	"*.crt",
	"id_rsa",
	"id_rsa.*",
	"id_dsa",
	"id_dsa.*",
	"id_ecdsa",
	"id_ecdsa.*",
	"id_ed25519",
	"id_ed25519.*",
	"*credential*",
	"*secret*",
	"*.token",
	"*.pwd",
	"*password*",
}

// secretContentMarkers are substrings whose presence in a file marks it suspect.
// "-----BEGIN" covers PEM key/cert blocks; "sk-" and "ghp_" cover common API/PAT tokens.
var secretContentMarkers = []string{
	"-----BEGIN",
	"sk-",
	"ghp_",
}

// scanReadLimit caps how many bytes of each file are inspected for content markers.
// Key/token markers appear at the head of real key files; this bounds work on large blobs.
const scanReadLimit = 64 * 1024

// ScanForSecrets walks dir and returns paths (relative to dir, sorted) of tracked
// files that either match a secret filename pattern or contain an obvious key/token
// marker. The .git directory is skipped. A non-empty result MUST block a push.
func ScanForSecrets(dir string) ([]string, error) {
	var suspects []string

	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}

		rel, relErr := filepath.Rel(dir, path)
		if relErr != nil {
			return relErr
		}

		if matchesSecretName(d.Name()) {
			suspects = append(suspects, rel)
			return nil
		}
		hit, contentErr := fileHasSecretMarker(path)
		if contentErr != nil {
			return contentErr
		}
		if hit {
			suspects = append(suspects, rel)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan for secrets in %s: %w", dir, err)
	}

	sort.Strings(suspects)
	return suspects, nil
}

// matchesSecretName reports whether base matches any secret filename glob (case-insensitive).
func matchesSecretName(base string) bool {
	lower := strings.ToLower(base)
	for _, glob := range secretNameGlobs {
		if ok, _ := filepath.Match(strings.ToLower(glob), lower); ok {
			return true
		}
	}
	return false
}

// fileHasSecretMarker reports whether the first scanReadLimit bytes of path
// contain any secret content marker.
func fileHasSecretMarker(path string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	buf := make([]byte, scanReadLimit)
	n, err := f.Read(buf)
	if err != nil && n == 0 {
		// EOF on an empty file is not an error condition for the scan.
		return false, nil
	}
	head := string(buf[:n])
	for _, marker := range secretContentMarkers {
		if strings.Contains(head, marker) {
			return true, nil
		}
	}
	return false, nil
}
```

- [ ] **Run:** `cd /Users/angeral/Repositories/claude_git && go test ./internal/share/`
- [ ] **Expected:** `ok  github.com/a2ngerer/claude-containers/internal/share`.

### Step 2.3 — REFACTOR

- [ ] **Run:** `cd /Users/angeral/Repositories/claude_git && gofmt -l internal/share/ && go vet ./internal/share/`
- [ ] **Expected:** empty `gofmt` output; `go vet` reports nothing.

### Step 2.4 — COMMIT

- [ ] **Run:** `cd /Users/angeral/Repositories/claude_git && git add internal/share/gitignore.go internal/share/gitignore_test.go && git commit -m "share: add ScanForSecrets for tracked secret files and key markers"`

**New types beyond contract:** none (only unexported package-level vars/consts: `secretNameGlobs`, `secretContentMarkers`, `scanReadLimit`, and unexported helpers).

---

## Task 3 — `share.Push`: scan-first, then delegate to the store

`Push(e, remote)` is the secret-safe push: it ALWAYS runs `ScanForSecrets` against the persona repo dir first and aborts with `ErrSecretsFound` + the suspect list when anything is found; otherwise it delegates to `e.Store.Push(remote)`. The repo dir is `environment.RepoDir(e.Hash)`.

### Step 3.1 — RED: write the failing test

- [ ] **Files:** `internal/share/share_test.go` (new)

```go
package share

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/a2ngerer/claude-containers/internal/environment"
	"github.com/stretchr/testify/require"
)

// withToolHome points CLAUDE_GIT_HOME at a fresh temp dir for the duration of one test.
func withToolHome(t *testing.T) {
	t.Helper()
	t.Setenv("CLAUDE_GIT_HOME", t.TempDir())
}

func TestPush_AbortsOnSecret(t *testing.T) {
	withToolHome(t)
	ws := t.TempDir()
	env, err := environment.Create(ws)
	require.NoError(t, err)

	// plant a secret file directly in the persona repo
	repo := environment.RepoDir(env.Hash)
	require.NoError(t, os.WriteFile(filepath.Join(repo, "leaked.key"), []byte("x"), 0o600))

	pushErr := Push(env, "origin")
	require.Error(t, pushErr)
	require.True(t, errors.Is(pushErr, ErrSecretsFound), "want ErrSecretsFound, got %v", pushErr)
	require.Contains(t, pushErr.Error(), "leaked.key")
}
```

- [ ] **Run:** `cd /Users/angeral/Repositories/claude_git && go test ./internal/share/ -run TestPush_AbortsOnSecret`
- [ ] **Expected:** compile failure — `undefined: Push` and `undefined: ErrSecretsFound`.

### Step 3.2 — GREEN: implement `share.go` errors + `Push`

- [ ] **Files:** `internal/share/share.go` (new)

```go
package share

import (
	"errors"
	"fmt"
	"strings"

	"github.com/a2ngerer/claude-containers/internal/environment"
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
```

- [ ] **Run:** `cd /Users/angeral/Repositories/claude_git && go test ./internal/share/ -run TestPush_AbortsOnSecret`
- [ ] **Expected:** `ok` — the scan finds `leaked.key`, `Push` returns `ErrSecretsFound` containing `leaked.key`, and `e.Store.Push` is never reached.

### Step 3.3 — REFACTOR

- [ ] **Run:** `cd /Users/angeral/Repositories/claude_git && gofmt -l internal/share/ && go vet ./internal/share/`
- [ ] **Expected:** empty `gofmt`; clean `go vet`.

### Step 3.4 — COMMIT

- [ ] **Run:** `cd /Users/angeral/Repositories/claude_git && git add internal/share/share.go internal/share/share_test.go && git commit -m "share: add Push with mandatory pre-push secret scan"`

**New types beyond contract:** `ErrSecretsFound` (sentinel error) — lives in `internal/share`, the package that owns the push-abort branch. Flagged in the final summary.

---

## Task 4 — `share.Pull` and `share.Clone`

`Pull(e, remote)` is a thin delegate to `e.Store.Pull(remote)` (no scan — pulling foreign content is inspected via `show`/`diff`, not blocked here; spec §13). `Clone(remote, destWorkspace)` creates a fresh environment for `destWorkspace` and populates its repo by cloning the remote, then returns the opened `*environment.Environment`. Clone is the engine behind both the `clone` command (Task 6) and `init --from` (Task 8).

> **Layering note:** `environment` must NOT import `share` (the contract has `share` importing `environment`). Therefore `Clone` delegates to a new sibling constructor `environment.CloneInto`, which performs the actual `git clone` with go-git. **Task 5 adds `environment.CloneInto`.** Implement Task 5 immediately after this task's RED step; until it lands, `go build ./internal/share/` reports `undefined: environment.CloneInto` (expected).

### Step 4.1 — RED: write the failing test

- [ ] **Files:** `internal/share/share_test.go` (append; add `"os/exec"` to the existing import block)

```go
// initBareRemote creates an empty bare git repo and returns its path (usable as a remote URL).
func initBareRemote(t *testing.T) string {
	t.Helper()
	bare := t.TempDir()
	cmd := exec.Command("git", "init", "--bare", bare)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git init --bare: %s", out)
	return bare
}

// commitRepo stages all changes in repoDir and commits them via the git binary.
// Used only in tests to drive a known history against the bare remote.
func commitRepo(t *testing.T, repoDir, msg string) {
	t.Helper()
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = repoDir
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v: %s", args, out)
	}
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "Test")
	run("add", "-A")
	run("commit", "-m", msg)
}

func TestPushPull_RoundTrip(t *testing.T) {
	withToolHome(t)
	bare := initBareRemote(t)

	// producer environment: create, add remote, push
	wsA := t.TempDir()
	envA, err := environment.Create(wsA)
	require.NoError(t, err)

	// drop a recognizable, non-secret file into the repo so we can detect it after clone
	marker := filepath.Join(environment.RepoDir(envA.Hash), "personas", "_base", "MARKER.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(marker), 0o755))
	require.NoError(t, os.WriteFile(marker, []byte("round-trip-token\n"), 0o644))
	commitRepo(t, environment.RepoDir(envA.Hash), "add marker")

	require.NoError(t, envA.Store.AddRemote("origin", bare))
	require.NoError(t, Push(envA, "origin"))

	// consumer environment: clone from the same bare remote
	wsB := t.TempDir()
	envB, err := Clone(bare, wsB)
	require.NoError(t, err)

	gotMarker := filepath.Join(environment.RepoDir(envB.Hash), "personas", "_base", "MARKER.md")
	data, err := os.ReadFile(gotMarker)
	require.NoError(t, err)
	require.Equal(t, "round-trip-token\n", string(data))
}

func TestPull_FetchesNewCommits(t *testing.T) {
	withToolHome(t)
	bare := initBareRemote(t)

	wsA := t.TempDir()
	envA, err := environment.Create(wsA)
	require.NoError(t, err)
	require.NoError(t, envA.Store.AddRemote("origin", bare))
	require.NoError(t, Push(envA, "origin"))

	wsB := t.TempDir()
	envB, err := Clone(bare, wsB)
	require.NoError(t, err)

	// A adds a new file and pushes it
	newFile := filepath.Join(environment.RepoDir(envA.Hash), "personas", "_base", "SECOND.md")
	require.NoError(t, os.WriteFile(newFile, []byte("second\n"), 0o644))
	commitRepo(t, environment.RepoDir(envA.Hash), "add second")
	require.NoError(t, Push(envA, "origin"))

	// B pulls and must now see SECOND.md
	require.NoError(t, Pull(envB, "origin"))
	_, statErr := os.Stat(filepath.Join(environment.RepoDir(envB.Hash), "personas", "_base", "SECOND.md"))
	require.NoError(t, statErr)
}
```

- [ ] **Run:** `cd /Users/angeral/Repositories/claude_git && go test ./internal/share/ -run 'RoundTrip|Pull_Fetches'`
- [ ] **Expected:** compile failure — `undefined: Clone`, `undefined: Pull`.

### Step 4.2 — GREEN: implement `Pull` and `Clone`

- [ ] **Files:** `internal/share/share.go` (append; the import block stays exactly as in Task 3 — no `go-git` here, because `environment.CloneInto` owns the clone)

```go
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
// It delegates to environment.CloneInto, which clones the remote into RepoDir(hash),
// writes env.toml, and sets the <workspace>/.claude_git marker. The returned environment
// is ready for `list`, `use`, etc. Used by the `clone` command and by `init --from`.
func Clone(remote, destWorkspace string) (*environment.Environment, error) {
	env, err := environment.CloneInto(destWorkspace, remote)
	if err != nil {
		return nil, fmt.Errorf("clone %q into workspace %q: %w", remote, destWorkspace, err)
	}
	return env, nil
}
```

- [ ] **Run (after Task 5 lands):** `cd /Users/angeral/Repositories/claude_git && go test ./internal/share/ -run 'RoundTrip|Pull_Fetches'`
- [ ] **Expected:** `ok` once `environment.CloneInto` exists (Task 5).

### Step 4.3 — REFACTOR

- [ ] **Run (after Task 5):** `cd /Users/angeral/Repositories/claude_git && gofmt -l internal/share/ && go vet ./internal/share/`
- [ ] **Expected:** empty `gofmt`; clean `go vet`.

### Step 4.4 — COMMIT

- [ ] **Run:** `cd /Users/angeral/Repositories/claude_git && git add internal/share/share.go internal/share/share_test.go && git commit -m "share: add Pull and Clone (delegating to environment.CloneInto)"`

**New types beyond contract:** none in `share` (functions only). The new constructor `environment.CloneInto` is introduced in Task 5 and flagged there.

---

## Task 5 — `environment.CloneInto` + `.gitignore` at creation (M1 modifications)

This task touches the M1 `internal/environment` package: it adds `CloneInto` (the constructor `share.Clone` and `init --from` depend on) and makes `Create` write `repo/.gitignore` from the secret-safe defaults.

**Import-direction caveat:** `environment` must not import `share`. `share.DefaultGitignore()`'s content is therefore inlined as a package-level constant `defaultGitignore` in `environment`. A guard test asserts the two byte-strings stay equal so they never drift.

### Step 5.1 — RED: write the failing tests

- [ ] **Files:** `internal/environment/clone_test.go` (new)

```go
package environment_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/a2ngerer/claude-containers/internal/environment"
	"github.com/a2ngerer/claude-containers/internal/share"
	"github.com/stretchr/testify/require"
)

func initBareRemoteEnv(t *testing.T) string {
	t.Helper()
	bare := t.TempDir()
	cmd := exec.Command("git", "init", "--bare", bare)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git init --bare: %s", out)
	return bare
}

func TestCreate_WritesGitignore(t *testing.T) {
	t.Setenv("CLAUDE_GIT_HOME", t.TempDir())
	ws := t.TempDir()

	env, err := environment.Create(ws)
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(environment.RepoDir(env.Hash), ".gitignore"))
	require.NoError(t, err)
	require.Equal(t, share.DefaultGitignore(), string(data),
		"repo/.gitignore must equal share.DefaultGitignore()")
}

func TestCloneInto_PopulatesAndOpens(t *testing.T) {
	t.Setenv("CLAUDE_GIT_HOME", t.TempDir())

	// build a source repo and push it to a bare remote
	src := t.TempDir()
	srcEnv, err := environment.Create(src)
	require.NoError(t, err)

	bare := initBareRemoteEnv(t)
	require.NoError(t, srcEnv.Store.AddRemote("origin", bare))
	require.NoError(t, share.Push(srcEnv, "origin"))

	// clone into a fresh workspace
	dst := t.TempDir()
	dstEnv, err := environment.CloneInto(dst, bare)
	require.NoError(t, err)

	// env.toml exists
	_, statErr := os.Stat(filepath.Join(environment.EnvDir(dstEnv.Hash), "env.toml"))
	require.NoError(t, statErr)

	// repo was populated (has .gitignore from the source)
	_, statErr = os.Stat(filepath.Join(environment.RepoDir(dstEnv.Hash), ".gitignore"))
	require.NoError(t, statErr)

	// workspace marker exists and contains the hash
	marker, readErr := os.ReadFile(filepath.Join(dst, ".claude_git"))
	require.NoError(t, readErr)
	require.Equal(t, dstEnv.Hash, string(marker))
}

func TestGitignoreInSync(t *testing.T) {
	// The inlined environment.defaultGitignore must stay byte-identical to
	// share.DefaultGitignore(). Compared via Create's output (the only public surface).
	t.Setenv("CLAUDE_GIT_HOME", t.TempDir())
	ws := t.TempDir()
	env, err := environment.Create(ws)
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(environment.RepoDir(env.Hash), ".gitignore"))
	require.NoError(t, err)
	require.Equal(t, share.DefaultGitignore(), string(data),
		"environment.defaultGitignore drifted from share.DefaultGitignore() — keep them identical")
}
```

- [ ] **Run:** `cd /Users/angeral/Repositories/claude_git && go test ./internal/environment/ -run 'WritesGitignore|CloneInto|GitignoreInSync'`
- [ ] **Expected:** failures — `TestCreate_WritesGitignore`/`TestGitignoreInSync` fail (M1 `Create` writes no `.gitignore` yet) and `TestCloneInto_PopulatesAndOpens` fails to compile (`undefined: environment.CloneInto`).

### Step 5.2 — GREEN: add the gitignore constant + write helpers + `CloneInto`

- [ ] **Files:** `internal/environment/clone.go` (new — keeps the M1 `environment.go` minimally touched)

```go
package environment

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/go-git/go-git/v5"
	toml "github.com/pelletier/go-toml/v2"
)

// defaultGitignore is the byte-identical copy of share.DefaultGitignore().
// It is inlined here to avoid an import cycle (share imports environment, not vice
// versa). The guard test TestGitignoreInSync asserts the two never drift.
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

// writeGitignore writes repo/.gitignore with the secret-safe defaults.
// Called by Create after the repo is initialized (see Step 5.3 wiring).
func writeGitignore(repoDir string) error {
	path := filepath.Join(repoDir, ".gitignore")
	if err := os.WriteFile(path, []byte(defaultGitignore), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// CloneInto onboards an existing persona repo into a NEW environment bound to workspace.
// It computes the hash, makes the env dir, clones remote into RepoDir, writes env.toml
// (no active persona), writes the <workspace>/.claude_git marker, and returns the opened
// environment. Used by share.Clone and by `claude_git init --from`.
func CloneInto(workspace, remote string) (*Environment, error) {
	abs, err := filepath.Abs(workspace)
	if err != nil {
		return nil, fmt.Errorf("resolve workspace path: %w", err)
	}
	hash := WorkspaceHash(abs)

	if err := os.MkdirAll(EnvDir(hash), 0o755); err != nil {
		return nil, fmt.Errorf("create env dir: %w", err)
	}

	repoDir := RepoDir(hash)
	if _, statErr := os.Stat(filepath.Join(repoDir, ".git")); statErr == nil {
		return nil, fmt.Errorf("repo already exists at %s", repoDir)
	}
	if _, cloneErr := git.PlainClone(repoDir, false, &git.CloneOptions{URL: remote}); cloneErr != nil {
		return nil, fmt.Errorf("git clone %q -> %q: %w", remote, repoDir, cloneErr)
	}

	cfg := EnvConfig{WorkspacePath: abs, ActivePersona: ""}
	if err := writeEnvConfig(hash, cfg); err != nil {
		return nil, err
	}
	if err := writeMarker(abs, hash); err != nil {
		return nil, err
	}

	return Open(abs)
}

// writeEnvConfig marshals cfg to <envdir>/env.toml.
func writeEnvConfig(hash string, cfg EnvConfig) error {
	data, err := toml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal env.toml: %w", err)
	}
	path := filepath.Join(EnvDir(hash), "env.toml")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// writeMarker writes the one-line <workspace>/.claude_git marker (= workspace hash).
func writeMarker(absWorkspace, hash string) error {
	path := filepath.Join(absWorkspace, ".claude_git")
	if err := os.WriteFile(path, []byte(hash), 0o644); err != nil {
		return fmt.Errorf("write workspace marker %s: %w", path, err)
	}
	return nil
}
```

- [ ] **CRITICAL note on duplicate helpers:** M1's `environment.Create` already writes `env.toml` and the `<workspace>/.claude_git` marker. If M1 exposes reusable helpers (e.g. `writeEnvConfig`/`writeMarker` or similar), DELETE the duplicate definitions above and call M1's. The duplicates are included only so this plan compiles standalone. If M1 already defines a function with the same name, `go build` reports `writeEnvConfig redeclared in this block` (or `writeMarker redeclared`) — in that case keep M1's and remove the copy here. `toml.Marshal` of `EnvConfig` must match M1's env.toml field names (`workspace_path`, `active_persona`) exactly — it does, per the contract.

### Step 5.3 — GREEN: wire `.gitignore` into M1 `Create`

- [ ] **Files:** `internal/environment/environment.go` (MODIFY — M1 file)
- [ ] **Exact modify instruction:** locate the body of `func Create(workspace string) (*Environment, error)`. After the statement that initializes the git repo into `RepoDir(hash)` (M1 calls `storage.OpenGit(RepoDir(hash))` or `git.PlainInit(RepoDir(hash), false)`), and before the function returns the `*Environment`, insert:

```go
	if err := writeGitignore(RepoDir(hash)); err != nil {
		return nil, err
	}
```

- [ ] **Decision (recorded):** the `.gitignore` is written eagerly at environment creation, NOT lazily at first push. Reason: it must protect from the very first commit `Create`/`init` makes (M1 commits the seeded `_base`). A lazy write before push would already be too late if a secret-named file was committed during `init`. `share.Push` additionally re-asserts via `ScanForSecrets` as a runtime backstop — defense in depth, but creation-time is the primary guarantee.
- [ ] **Note:** if M1's `Create` returns very early (no later statements), place the `writeGitignore` call right after the repo-init line; the `RepoDir(hash)` dir already exists at that point because `OpenGit`/`PlainInit` created it.

### Step 5.4 — Run + verify

- [ ] **Run:** `cd /Users/angeral/Repositories/claude_git && go test ./internal/environment/ ./internal/share/`
- [ ] **Expected:** `ok` for both packages; `TestCreate_WritesGitignore`, `TestCloneInto_PopulatesAndOpens`, `TestGitignoreInSync` pass, and the Task 4 `share` round-trip tests now pass because `environment.CloneInto` exists.

### Step 5.5 — REFACTOR

- [ ] **Run:** `cd /Users/angeral/Repositories/claude_git && gofmt -l internal/environment/ internal/share/ && go vet ./internal/environment/ ./internal/share/`
- [ ] **Expected:** empty `gofmt`; clean `go vet`.

### Step 5.6 — COMMIT

- [ ] **Run:** `cd /Users/angeral/Repositories/claude_git && git add internal/environment/clone.go internal/environment/clone_test.go internal/environment/environment.go && git commit -m "environment: add CloneInto and write secret-safe .gitignore on create"`

**New types beyond contract:** `environment.CloneInto(workspace, remote string) (*Environment, error)` (new exported constructor in `internal/environment`); unexported helpers `writeGitignore`, `writeEnvConfig`, `writeMarker`, and the `defaultGitignore` constant (the latter three may be removed if M1 already provides equivalents). Flagged in the final summary.

---

## Task 6 — `internal/cli/share.go`: `push`, `pull`, `clone` commands

Thin cobra commands per the contract convention (parse args, call one `share` function, format output). `push`/`pull` operate on the current workspace's environment (`environment.Open(cwd)`); `clone` creates a new environment for the current workspace.

### Step 6.1 — RED: write the failing test

- [ ] **Files:** `internal/cli/share_test.go` (new)

```go
package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/a2ngerer/claude-containers/internal/environment"
	"github.com/stretchr/testify/require"
)

// chdir switches into dir for the duration of one test, restoring cwd afterward.
func chdir(t *testing.T, dir string) {
	t.Helper()
	prev, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { _ = os.Chdir(prev) })
}

// execGit runs a git command in dir and returns combined output.
func execGit(dir string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	return cmd.CombinedOutput()
}

// initBareRemoteCLI creates an empty bare git repo and returns its path.
func initBareRemoteCLI(t *testing.T) string {
	t.Helper()
	bare := t.TempDir()
	out, err := execGit("", "init", "--bare", bare)
	require.NoError(t, err, "git init --bare: %s", out)
	return bare
}

func TestCloneCmd_SetsUpEnvironmentForCwd(t *testing.T) {
	t.Setenv("CLAUDE_GIT_HOME", t.TempDir())

	// produce a source repo and push to a bare remote
	src := t.TempDir()
	srcEnv, err := environment.Create(src)
	require.NoError(t, err)
	bare := initBareRemoteCLI(t)
	require.NoError(t, srcEnv.Store.AddRemote("origin", bare))
	require.NoError(t, srcEnv.Store.Push("origin"))

	// run `clone <bare>` with cwd = a fresh workspace
	dst := t.TempDir()
	chdir(t, dst)

	cmd := newCloneCmd()
	cmd.SetArgs([]string{bare})
	require.NoError(t, cmd.Execute())

	// the new environment exists for dst: repo + env.toml + marker
	hash := environment.WorkspaceHash(dst)
	_, statErr := os.Stat(filepath.Join(environment.RepoDir(hash), ".git"))
	require.NoError(t, statErr)
	_, statErr = os.Stat(filepath.Join(environment.EnvDir(hash), "env.toml"))
	require.NoError(t, statErr)
	marker, readErr := os.ReadFile(filepath.Join(dst, ".claude_git"))
	require.NoError(t, readErr)
	require.Equal(t, hash, string(marker))
}
```

- [ ] **Note on cwd-based hash:** the test compares against `environment.WorkspaceHash(dst)` using the same path the command sees via `os.Getwd`. macOS `t.TempDir()` may return a `/var` path symlinked to `/private/var`; `WorkspaceHash` cleans but does not resolve symlinks, so both sides hash the same string. The comparison stays valid regardless of the hash impl because it uses the same function the command uses.
- [ ] **Run:** `cd /Users/angeral/Repositories/claude_git && go test ./internal/cli/ -run TestCloneCmd`
- [ ] **Expected:** compile failure — `undefined: newCloneCmd`.

### Step 6.2 — GREEN: implement the command group

- [ ] **Files:** `internal/cli/share.go` (new)

```go
package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/a2ngerer/claude-containers/internal/environment"
	"github.com/a2ngerer/claude-containers/internal/share"
	"github.com/spf13/cobra"
)

// cwdWorkspace returns the absolute path of the current working directory.
func cwdWorkspace() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get working directory: %w", err)
	}
	return filepath.Abs(wd)
}

// newPushCmd: `claude_git push [remote]` — secret-scan, then push (default remote "origin").
func newPushCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "push [remote]",
		Short: "Push the persona repo to a remote (aborts if secrets are detected)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			remote := "origin"
			if len(args) == 1 {
				remote = args[0]
			}
			ws, err := cwdWorkspace()
			if err != nil {
				return err
			}
			env, err := environment.Open(ws)
			if err != nil {
				return err
			}
			if err := share.Push(env, remote); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Pushed persona repo to %q.\n", remote)
			return nil
		},
	}
}

// newPullCmd: `claude_git pull [remote]` — fetch + integrate remote changes.
func newPullCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "pull [remote]",
		Short: "Pull persona repo changes from a remote",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			remote := "origin"
			if len(args) == 1 {
				remote = args[0]
			}
			ws, err := cwdWorkspace()
			if err != nil {
				return err
			}
			env, err := environment.Open(ws)
			if err != nil {
				return err
			}
			if err := share.Pull(env, remote); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Pulled persona repo from %q.\n", remote)
			return nil
		},
	}
}

// newCloneCmd: `claude_git clone <remote>` — clone an existing persona repo into a
// new environment bound to the current workspace (team onboarding into an empty dir).
func newCloneCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "clone <remote>",
		Short: "Clone an existing persona repo into a new environment for this workspace",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			remote := args[0]
			ws, err := cwdWorkspace()
			if err != nil {
				return err
			}
			env, err := share.Clone(remote, ws)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(),
				"Cloned %q into a new environment for %s.\nRun `claude_git list` to see available personas.\n",
				remote, env.Workspace)
			return nil
		},
	}
}
```

- [ ] **Run:** `cd /Users/angeral/Repositories/claude_git && go test ./internal/cli/ -run TestCloneCmd`
- [ ] **Expected:** `ok` — the clone command sets up `repo/.git`, `env.toml`, and the marker for the cwd workspace.

### Step 6.3 — REFACTOR + register on root

- [ ] **Files:** `internal/cli/root.go` (MODIFY — M1 file)
- [ ] **Exact modify instruction:** in the function that builds the root command (M1 names it `NewRootCmd()` or `newRootCmd()`), where other subcommands are attached via `root.AddCommand(...)`, append:

```go
	root.AddCommand(newPushCmd(), newPullCmd(), newCloneCmd())
```

- [ ] **Note:** if M1's root builder uses a different variable name than `root` for the `*cobra.Command`, use that variable. Do not add business logic to `root.go`.
- [ ] **Run:** `cd /Users/angeral/Repositories/claude_git && gofmt -l internal/cli/ && go vet ./internal/cli/ && go build ./...`
- [ ] **Expected:** empty `gofmt`; clean `go vet`; successful build.

### Step 6.4 — COMMIT

- [ ] **Run:** `cd /Users/angeral/Repositories/claude_git && git add internal/cli/share.go internal/cli/share_test.go internal/cli/root.go && git commit -m "cli: add push, pull, clone commands"`

**New types beyond contract:** none (cobra command constructors + unexported `cwdWorkspace` helper).

---

## Task 7 — End-to-end push/clone round-trip through the CLI

A single integration test exercising the full user path: create environment in workspace A, add a persona file, `push` to a bare remote, `clone` into workspace B, then assert the persona content arrived and no secret was committed. This is acceptance criterion §18.7 ("`push`/`clone` round-trip the persona repo to a remote with no secrets committed").

### Step 7.1 — RED: write the failing test

- [ ] **Files:** `internal/cli/share_e2e_test.go` (new)

```go
package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/a2ngerer/claude-containers/internal/environment"
	"github.com/stretchr/testify/require"
)

func TestPushClone_E2E_RoundTrip(t *testing.T) {
	t.Setenv("CLAUDE_GIT_HOME", t.TempDir())
	bare := initBareRemoteCLI(t)

	// --- producer: create env in workspace A, add a persona file, push ---
	wsA := t.TempDir()
	envA, err := environment.Create(wsA)
	require.NoError(t, err)

	coderDir := filepath.Join(environment.RepoDir(envA.Hash), "personas", "coder")
	require.NoError(t, os.MkdirAll(coderDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(coderDir, "persona.toml"),
		[]byte("name = \"coder\"\ndescription = \"builder\"\n"), 0o644))
	out, gitErr := execGit(environment.RepoDir(envA.Hash), "add", "-A")
	require.NoError(t, gitErr, "git add: %s", out)
	out, gitErr = execGit(environment.RepoDir(envA.Hash), "-c", "user.email=t@e.com", "-c", "user.name=T", "commit", "-m", "add coder")
	require.NoError(t, gitErr, "git commit: %s", out)

	require.NoError(t, envA.Store.AddRemote("origin", bare))

	chdir(t, wsA)
	pushCmd := newPushCmd()
	pushCmd.SetArgs([]string{"origin"})
	require.NoError(t, pushCmd.Execute())

	// --- consumer: clone into workspace B ---
	wsB := t.TempDir()
	chdir(t, wsB)
	cloneCmd := newCloneCmd()
	cloneCmd.SetArgs([]string{bare})
	require.NoError(t, cloneCmd.Execute())

	// the coder persona arrived in B
	hashB := environment.WorkspaceHash(wsB)
	data, readErr := os.ReadFile(filepath.Join(environment.RepoDir(hashB), "personas", "coder", "persona.toml"))
	require.NoError(t, readErr)
	require.Contains(t, string(data), "name = \"coder\"")

	// no secret was committed: .gitignore is present
	_, statErr := os.Stat(filepath.Join(environment.RepoDir(hashB), ".gitignore"))
	require.NoError(t, statErr)
}
```

- [ ] **Run:** `cd /Users/angeral/Repositories/claude_git && go test ./internal/cli/ -run TestPushClone_E2E`
- [ ] **Expected:** PASS (all helpers and commands exist from Task 6). The assertion targets the explicitly committed `coder` file, so the test is robust to M1's auto-commit policy.

### Step 7.2 — REFACTOR

- [ ] **Run:** `cd /Users/angeral/Repositories/claude_git && gofmt -l internal/cli/ && go vet ./internal/cli/`
- [ ] **Expected:** empty `gofmt`; clean `go vet`.

### Step 7.3 — COMMIT

- [ ] **Run:** `cd /Users/angeral/Repositories/claude_git && git add internal/cli/share_e2e_test.go && git commit -m "cli: add end-to-end push/clone round-trip test"`

**New types beyond contract:** none.

---

## Task 8 — MODIFY `internal/cli/init.go`: `--from <remote>` branch

Extend the M1 `init` command with a `--from <remote>` flag. When set, `init` does NOT seed `_base` from the local `.claude/`; instead it clones an existing persona repo for the current workspace (team onboarding) by delegating to `share.Clone`. This is the symmetric onboarding path to `clone`, but lives on `init` per the CLI spec (§11: `claude_git init [--from <remote>]`).

### Step 8.1 — RED: write the failing test

- [ ] **Files:** `internal/cli/init_from_test.go` (new)

```go
package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/a2ngerer/claude-containers/internal/environment"
	"github.com/stretchr/testify/require"
)

func TestInitFrom_ClonesExistingRepo(t *testing.T) {
	t.Setenv("CLAUDE_GIT_HOME", t.TempDir())

	// produce a source repo with a reviewer persona, push to bare remote
	src := t.TempDir()
	srcEnv, err := environment.Create(src)
	require.NoError(t, err)
	revDir := filepath.Join(environment.RepoDir(srcEnv.Hash), "personas", "reviewer")
	require.NoError(t, os.MkdirAll(revDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(revDir, "persona.toml"),
		[]byte("name = \"reviewer\"\n"), 0o644))
	out, gitErr := execGit(environment.RepoDir(srcEnv.Hash), "add", "-A")
	require.NoError(t, gitErr, "git add: %s", out)
	out, gitErr = execGit(environment.RepoDir(srcEnv.Hash), "-c", "user.email=t@e.com", "-c", "user.name=T", "commit", "-m", "add reviewer")
	require.NoError(t, gitErr, "git commit: %s", out)

	bare := initBareRemoteCLI(t)
	require.NoError(t, srcEnv.Store.AddRemote("origin", bare))
	require.NoError(t, srcEnv.Store.Push("origin"))

	// onboard: `init --from <bare>` in a fresh workspace
	dst := t.TempDir()
	chdir(t, dst)
	cmd := newInitCmd()
	cmd.SetArgs([]string{"--from", bare})
	require.NoError(t, cmd.Execute())

	// the reviewer persona was cloned in, and the marker is set
	hash := environment.WorkspaceHash(dst)
	data, readErr := os.ReadFile(filepath.Join(environment.RepoDir(hash), "personas", "reviewer", "persona.toml"))
	require.NoError(t, readErr)
	require.Contains(t, string(data), "name = \"reviewer\"")

	marker, mErr := os.ReadFile(filepath.Join(dst, ".claude_git"))
	require.NoError(t, mErr)
	require.Equal(t, hash, string(marker))
}
```

- [ ] **Run:** `cd /Users/angeral/Repositories/claude_git && go test ./internal/cli/ -run TestInitFrom`
- [ ] **Expected:** FAIL — M1 `init` has no `--from` flag/branch; it seeds an empty `_base` instead of cloning, so `personas/reviewer/persona.toml` is absent (or the unknown-flag error surfaces).

### Step 8.2 — GREEN: add the `--from` flag and branch to M1 `init.go`

- [ ] **Files:** `internal/cli/init.go` (MODIFY — M1 file)
- [ ] **Exact modify instruction (this is the init.go modify location for `--from`):**

  1. **Function:** `newInitCmd() *cobra.Command` (the M1 constructor returning the `init` command).
  2. **Flag declaration:** after the `cmd` (the `*cobra.Command`) is built and before it is returned, bind a string flag:

     ```go
     var fromRemote string
     cmd.Flags().StringVar(&fromRemote, "from", "",
         "clone an existing persona repo from this git remote instead of seeding _base")
     ```

     (If M1 builds the command as a struct literal `cmd := &cobra.Command{...}`, place this immediately after that literal. `fromRemote` must be declared in the same scope as `RunE` so the closure captures it.)
  3. **Branch in `RunE`:** as the FIRST statements inside the existing `RunE: func(cmd *cobra.Command, args []string) error { ... }`, before M1's "resolve workspace + `environment.Create` + seed `_base`" logic, insert the short-circuit:

     ```go
     if fromRemote != "" {
         ws, err := cwdWorkspace()
         if err != nil {
             return err
         }
         env, err := share.Clone(fromRemote, ws)
         if err != nil {
             return err
         }
         fmt.Fprintf(cmd.OutOrStdout(),
             "Onboarded from %q into a new environment for %s.\nRun `claude_git list` to see available personas.\n",
             fromRemote, env.Workspace)
         return nil
     }
     // ... M1's existing seed-from-local-.claude logic continues here unchanged ...
     ```

  4. **Imports:** ensure `internal/cli/init.go` imports `"fmt"` and `"github.com/a2ngerer/claude-containers/internal/share"`. `cwdWorkspace` is already defined in `internal/cli/share.go` (same package), so no extra import is needed for it.

- [ ] **Decision (recorded):** the `--from` branch fully REPLACES the seeding path — it does NOT both clone and seed. Cloning brings the team's `_base` and personas verbatim; re-seeding from the onboarding machine's local `.claude/` would pollute the shared baseline. The two are mutually exclusive by design (spec §11: "seed `_base` from the existing `.claude/`, **or** clone an existing persona repo").
- [ ] **Run:** `cd /Users/angeral/Repositories/claude_git && go test ./internal/cli/ -run TestInitFrom`
- [ ] **Expected:** `ok` — `init --from <bare>` clones the reviewer persona and sets the marker.

### Step 8.3 — REFACTOR

- [ ] **Run:** `cd /Users/angeral/Repositories/claude_git && gofmt -l internal/cli/ && go vet ./internal/cli/ && go build ./...`
- [ ] **Expected:** empty `gofmt`; clean `go vet`; successful build.

### Step 8.4 — COMMIT

- [ ] **Run:** `cd /Users/angeral/Repositories/claude_git && git add internal/cli/init.go internal/cli/init_from_test.go && git commit -m "cli: add init --from for team onboarding via clone"`

**New types beyond contract:** none (a new `--from` flag and a branch on the existing M1 command).

---

## Task 9 — Full-suite green + vet + import-cycle gate

Confirm the whole milestone compiles, all tests pass, and the package layering (no `environment` → `share` cycle) holds.

### Step 9.1 — Full build + test + vet

- [ ] **Run:** `cd /Users/angeral/Repositories/claude_git && go build ./... && go test ./... && go vet ./...`
- [ ] **Expected:** build succeeds; `ok` for `internal/share`, `internal/environment`, `internal/cli`; `go vet` reports nothing.

### Step 9.2 — Import-cycle guard

- [ ] **Run:** `cd /Users/angeral/Repositories/claude_git && go list -deps ./internal/environment/ | grep -c 'github.com/a2ngerer/claude-containers/internal/share' || true`
- [ ] **Expected:** `0` — `environment` must not depend on `share` (the gitignore is inlined; the drift test in Task 5 keeps them in sync).

### Step 9.3 — COMMIT (if any formatting fixups were needed)

- [ ] **Run:** `cd /Users/angeral/Repositories/claude_git && git add -A && git commit -m "share: M4 milestone green (build, test, vet)" || echo "nothing to commit"`

---

## Final summary

**Tasks:** 9 (Tasks 1–4 build `internal/share`; Task 5 modifies `internal/environment`; Tasks 6–8 add CLI; Task 9 is the green gate).

**Files created:**
- `internal/share/gitignore.go`, `internal/share/gitignore_test.go`
- `internal/share/share.go`, `internal/share/share_test.go`
- `internal/environment/clone.go`, `internal/environment/clone_test.go`
- `internal/cli/share.go`, `internal/cli/share_test.go`, `internal/cli/share_e2e_test.go`
- `internal/cli/init_from_test.go`

**Files modified (M1):**
- `internal/environment/environment.go` — `Create` now calls `writeGitignore(RepoDir(hash))` after repo init.
- `internal/cli/init.go` — `--from <remote>` flag + branch (details below).
- `internal/cli/root.go` — registers `newPushCmd`, `newPullCmd`, `newCloneCmd`.

**Packages:** `internal/share` (new), `internal/environment` (extended), `internal/cli` (extended).

**Commands:** `claude_git push [remote]`, `claude_git pull [remote]`, `claude_git clone <remote>`, `claude_git init --from <remote>`.

**New types/symbols beyond the Architecture Contract:**
- `internal/share`: `ScanForSecrets(dir string) ([]string, error)` (named in the milestone scope, not in the contract sketch); `Push(e *environment.Environment, remote string) error`; `Pull(e *environment.Environment, remote string) error`; `Clone(remote, destWorkspace string) (*environment.Environment, error)`; sentinel `var ErrSecretsFound`. (`DefaultGitignore()` IS in the contract sketch — not new.)
- `internal/environment`: `CloneInto(workspace, remote string) (*Environment, error)` (new exported constructor); unexported `writeGitignore`, `writeEnvConfig`, `writeMarker`, const `defaultGitignore`. If M1 already defines `writeEnvConfig`/`writeMarker` equivalents, reuse M1's and delete the duplicates (called out in Task 5.2).
- `internal/cli`: command constructors `newPushCmd`, `newPullCmd`, `newCloneCmd`; unexported `cwdWorkspace`.

**Exact `init.go` modify location for `--from`:** In `internal/cli/init.go`, function `newInitCmd() *cobra.Command`:
1. Declare `var fromRemote string` and bind `cmd.Flags().StringVar(&fromRemote, "from", "", "clone an existing persona repo from this git remote instead of seeding _base")` after the command is built, in the same scope as `RunE`.
2. As the FIRST statement inside `RunE`, add the short-circuit: `if fromRemote != "" { ws, err := cwdWorkspace(); ...; env, err := share.Clone(fromRemote, ws); ...; return nil }` — it clones via `share.Clone` and returns before M1's local-`.claude/` seeding path.
3. Add imports `"fmt"` and `github.com/a2ngerer/claude-containers/internal/share` to `init.go`.
The clone branch and the seed branch are mutually exclusive (spec §11: seed `_base` **or** clone).

**`.gitignore` placement decision:** written eagerly in `environment.Create` (Task 5.3), NOT lazily at first push — it must protect the very first commit `init` makes. `share.Push` re-asserts via `ScanForSecrets` as a runtime backstop (defense in depth).
