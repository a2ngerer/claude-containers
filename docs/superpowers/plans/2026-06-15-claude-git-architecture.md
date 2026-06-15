# claude_git — Architecture Contract (shared reference for all milestone plans)

> **Binding for:** M1 Foundation, M2 Personas & Versioning, M3 Activation, M4 Sharing.
> **Rule:** Plans MUST use the exact module path, package layout, type names, and signatures defined here. Do not redefine or rename. If a plan needs a new type, it adds it in the package indicated below and notes it.
> **Derived from:** `docs/superpowers/specs/2026-06-15-claude-git-design.md`.

---

## 1. Module, toolchain, dependencies

- **Module path:** `github.com/angerer/claude_git` (placeholder — owner adjusts before first push; all import paths below assume it)
- **Go:** 1.23+
- **CLI:** `github.com/spf13/cobra` (commands) + `github.com/spf13/viper` (global config)
- **Git plumbing:** `github.com/go-git/go-git/v5` (pure-Go; no git binary dependency for core ops)
- **TOML:** `github.com/pelletier/go-toml/v2`
- **Tests:** standard `testing` + `github.com/stretchr/testify/require`
- **Errors:** wrap with `fmt.Errorf("...: %w", err)`; sentinel errors (`var ErrX = errors.New(...)`) where callers branch on them.

---

## 2. Package layout (responsibility-split)

```
claude_git/
  go.mod
  cmd/claude_git/main.go        # entrypoint; builds the cobra root and Execute()s it
  internal/
    domain/                     # pure types + pure functions; NO I/O, NO git
      persona.go                # Persona, Config, SkillSet, SubagentSet, MCPConfig, Metadata
      enforcement.go            # Enforcement
      snapshot.go               # SnapshotID, Snapshot, Tag, Timeline
      attestation.go            # Attestation, AttestationLine
      errors.go                 # sentinel errors
    storage/                    # persistence behind an interface
      engine.go                 # StorageEngine interface, ObjectID
      git.go                    # GitStorageEngine (go-git impl)
    environment/                # binding of the tool to one workspace + on-disk paths
      paths.go                  # WorkspaceHash, ToolHome, EnvDir, CacheDir, RepoDir
      environment.go            # Environment, EnvConfig (env.toml), Open/Create
    compose/                    # Layer composition: persona + _base -> ResolvedManifest
      compose.go                # ResolvedManifest, Compose()
    materialize/                # ResolvedManifest -> a CLAUDE_CONFIG_DIR on disk
      materialize.go            # Materialize()
      settings.go               # settings.json generation (permissions)
      mcp.go                    # mcp.json generation
    enforce/                    # build allow/deny + verify a materialized dir
      enforce.go                # PermissionSet, BuildPermissions()
      verify.go                 # Verify()
    activate/                   # orchestrates compose->materialize->verify->attest->launch
      activate.go               # Activate(), ActivationResult
      lock.go                   # Lock, Acquire/Release
      launch.go                 # BuildLaunch() -> env + argv; LaunchSpec
    share/                      # push/pull/clone + secret-safe gitignore
      share.go
      gitignore.go              # DefaultGitignore(), secret patterns
    probe/                      # inspect the workspace's code repo
      probe.go                  # IsClaudeTracked()
    cli/                        # one file per command group; thin, delegates to internal pkgs
      root.go init.go use.go persona.go version.go share.go verify.go
  testdata/                     # fixture .claude/ trees for tests
```

**Layering rule:** `domain` imports nothing internal. `storage`, `environment`, `compose`, `materialize`, `enforce` import `domain`. `activate` imports all of the above. `cli` imports `activate`, `environment`, etc. No import cycles.

---

## 3. Domain types (exact)

```go
// internal/domain/persona.go
package domain

type Persona struct {
	Name        string      `toml:"name"`
	Description string      `toml:"description"`
	Extends     string      `toml:"extends"` // layer name, e.g. "_base"; "" = none
	Config      Config      `toml:"config"`
	Enforcement Enforcement `toml:"enforcement"`
	Metadata    Metadata    `toml:"metadata"`
}

type Config struct {
	ClaudeMD       string      `toml:"claude_md"`       // path relative to persona dir
	SettingSources []string    `toml:"setting_sources"` // subset of {"user","project","local"}
	Skills         SkillSet    `toml:"skills"`
	Subagents      SubagentSet `toml:"subagents"`
	MCP            MCPConfig   `toml:"mcp"`
}

type SkillSet struct {
	Mode    string   `toml:"mode"`    // "allowlist" (default) | "replace"
	Include []string `toml:"include"` // skill directory names
}

type SubagentSet struct {
	Include []string `toml:"include"` // subagent file basenames (without .md)
}

type MCPConfig struct {
	Config   string   `toml:"config"`   // path to persona-local mcp.json ("" = none)
	Strict   bool     `toml:"strict"`   // -> --strict-mcp-config
	Requires []string `toml:"requires"` // declared server names (sharing only; never configs)
}

type Metadata struct {
	Version string `toml:"version"` // semver, e.g. "1.2.0"
	Author  string `toml:"author"`
}

// IsLayer reports whether this is a composable layer (name starts with "_").
func (p Persona) IsLayer() bool { return len(p.Name) > 0 && p.Name[0] == '_' }
```

```go
// internal/domain/enforcement.go
package domain

type Enforcement struct {
	PermissionMode string   `toml:"permission_mode"` // "read-only" | "default"
	ToolsAllow     []string `toml:"-"`               // loaded from [enforcement.tools] allow
	ToolsDeny      []string `toml:"-"`               // loaded from [enforcement.tools] deny
}
// NOTE: tools.allow/deny live under [enforcement.tools]; loaders map them into these fields.
```

```go
// internal/domain/snapshot.go
package domain

import "time"

type SnapshotID string

type Snapshot struct {
	ID        SnapshotID
	Persona   string
	Parents   []SnapshotID
	Message   string
	Author    string
	Timestamp time.Time
	TreeID    string // storage-backend object id of the persona content tree
}

type Tag struct {
	Persona string
	Version string // semver or "latest"
	Target  SnapshotID
}

type Timeline struct {
	Persona   string
	Snapshots []SnapshotID // newest first
}
```

```go
// internal/domain/attestation.go
package domain

type Attestation struct {
	Persona    string
	Version    string
	Included   []AttestationLine // skills/subagents/mcp present
	Withheld   []AttestationLine // deliberately excluded (for the diff narrative)
	Denied     []string          // denied tools, e.g. ["Write","Edit"]
	SettingSrc []string          // effective setting sources
	Clean      bool              // true iff verify passed
}

type AttestationLine struct {
	Kind  string // "skill" | "subagent" | "mcp"
	Names []string
}
```

```go
// internal/domain/errors.go
package domain

import "errors"

var (
	ErrPersonaNotFound = errors.New("persona not found")
	ErrPersonaExists   = errors.New("persona already exists")
	ErrNotInitialized  = errors.New("workspace not initialized (run: claude_git init)")
	ErrLocked          = errors.New("environment is locked by another process")
	ErrVerifyMismatch  = errors.New("materialized environment does not match manifest")
	ErrLayerNotFound   = errors.New("extends layer not found")
)
```

---

## 4. Storage interface (exact)

```go
// internal/storage/engine.go
package storage

import "github.com/angerer/claude_git/internal/domain"

type ObjectID string

// StorageEngine is the only persistence boundary. The default impl is git-backed,
// but nothing above this interface knows that. Implementations MUST be safe to call
// from a single process at a time (the activate.Lock guards cross-process use).
type StorageEngine interface {
	// content-addressed blobs
	PutObject(content []byte) (ObjectID, error)
	GetObject(id ObjectID) ([]byte, error)

	// persona content trees (a whole persona directory at one point in time)
	WriteTree(dir string) (ObjectID, error)         // snapshot a persona dir, return tree id
	CheckoutTree(id ObjectID, destDir string) error // materialize a tree to destDir

	// snapshots (history)
	WriteSnapshot(s domain.Snapshot) (domain.SnapshotID, error)
	ReadSnapshot(id domain.SnapshotID) (domain.Snapshot, error)
	Timeline(persona string) ([]domain.SnapshotID, error)

	// tags
	SetTag(persona, version string, id domain.SnapshotID) error
	ResolveTag(persona, version string) (domain.SnapshotID, error)
	ListTags(persona string) ([]domain.Tag, error)

	// remotes (sharing)
	AddRemote(name, url string) error
	Push(remote string) error
	Pull(remote string) error
}

// OpenGit opens or initializes the git-backed engine rooted at repoDir.
func OpenGit(repoDir string) (StorageEngine, error) // implemented in git.go
```

**Git mapping (for the impl, M1):** persona content tree → git tree; `WriteSnapshot` → git commit (one ref per persona under `refs/personas/<name>`); `SetTag` → `refs/tags/<persona>/<version>`; remotes → git remotes. Users never see refs.

---

## 5. Key package APIs (exact signatures the plans implement)

```go
// internal/environment/paths.go
func WorkspaceHash(absWorkspace string) string // sha1 hex of filepath.Clean(abs)
func ToolHome() string                         // ~/.claude_git (respects CLAUDE_GIT_HOME)
func EnvDir(hash string) string                // <home>/environments/<hash>
func RepoDir(hash string) string               // <home>/environments/<hash>/repo
func CacheDir(hash, persona string) string     // <home>/cache/<hash>/<persona>

// internal/environment/environment.go
type EnvConfig struct {
	WorkspacePath string `toml:"workspace_path"`
	ActivePersona string `toml:"active_persona"`
	Author        string `toml:"author"` // set at Create via defaultAuthor(); snapshot author. Getter: (e *Environment) Author()
}
type Environment struct {
	Hash      string
	Workspace string
	Store     storage.StorageEngine
	cfg       EnvConfig
}
func Create(workspace string) (*Environment, error) // init: makes dirs, repo, env.toml
func Open(workspace string) (*Environment, error)   // returns domain.ErrNotInitialized if absent
func (e *Environment) ListPersonas() ([]domain.Persona, error)
func (e *Environment) LoadPersona(name string) (domain.Persona, error)
func (e *Environment) SavePersona(p domain.Persona) error
func (e *Environment) SetActive(name string) error

// internal/compose/compose.go
type ResolvedManifest struct {
	Persona     domain.Persona // the leaf persona (post-merge effective values)
	Skills      []string       // resolved skill dir names to include
	Subagents   []string       // resolved subagent basenames
	ClaudeMD    string         // composed CLAUDE.md content (base + persona)
	SettingSrc  []string
	Enforcement domain.Enforcement
	MCP         domain.MCPConfig
}
func Compose(e *environment.Environment, personaName string) (ResolvedManifest, error)

// internal/materialize/materialize.go
// Renders rm into destDir (a CLAUDE_CONFIG_DIR). Copies allowlisted skills/subagents
// from the persona repo, writes CLAUDE.md, settings.json, mcp.json. Idempotent.
func Materialize(e *environment.Environment, rm compose.ResolvedManifest, destDir string) error

// internal/enforce/enforce.go
type PermissionSet struct {
	Allow []string
	Deny  []string
	Mode  string
}
func BuildPermissions(enf domain.Enforcement) PermissionSet

// internal/enforce/verify.go
// Verify asserts destDir contains exactly the allowlisted material and the deny rules.
func Verify(rm compose.ResolvedManifest, destDir string) (domain.Attestation, error)

// internal/activate/activate.go
type ActivationResult struct {
	ConfigDir   string
	Attestation domain.Attestation
	Launch      LaunchSpec
}
func Activate(e *environment.Environment, personaRef string) (ActivationResult, error)

// internal/activate/launch.go
type LaunchSpec struct {
	Env  []string // e.g. ["CLAUDE_CONFIG_DIR=..."]
	Argv []string // e.g. ["claude","--setting-sources","user,project", ...]
}
func BuildLaunch(configDir string, rm compose.ResolvedManifest) LaunchSpec

// internal/probe/probe.go
func IsClaudeTracked(workspace string) (bool, error) // git ls-files --error-unmatch .claude

// internal/share/gitignore.go
func DefaultGitignore() string // settings.local.json, *.key, .env, etc.
```

`personaRef` syntax: `name` or `name:version` (version defaults to `latest`).

---

## 6. On-disk layout (authoritative; see spec §10)

```
$CLAUDE_GIT_HOME (default ~/.claude_git)/
  config.toml
  environments/<workspace-hash>/
    env.toml                 # EnvConfig
    repo/                    # git repo = StorageEngine backend (hidden from user)
      personas/_base/ coder/ reviewer/    # persona.toml + CLAUDE.md + skills/ agents/ mcp.json
  cache/<workspace-hash>/<persona>/        # materialized CLAUDE_CONFIG_DIR (ephemeral)
<workspace>/.claude_git                    # marker: one line = workspace-hash
```

`env.toml`, `persona.toml` use TOML. Timestamps: RFC 3339 (`time.RFC3339`).

---

## 7. Conventions

- **Testing:** table-driven where natural; `t.TempDir()` for filesystem tests; set `CLAUDE_GIT_HOME` to a temp dir per test to isolate the tool home. Use `require` (fail-fast), not `assert`.
- **No global state** beyond viper config; pass `*Environment` explicitly.
- **CLI files are thin:** parse flags, call one internal function, format output. No business logic in `cli/`.
- **Output:** human output to stdout; errors to stderr; non-zero exit on failure. `--json` flag deferred (not in MVP).
- **Determinism:** materialization is idempotent — running it twice yields a byte-identical dir.
- **Fail closed:** `activate` runs `verify` and aborts (no launch) on `domain.ErrVerifyMismatch`.

---

## 8. Build order & integration seams (reconciliation)

Added after the milestone-plan self-review. Build the milestones **in order M1 → M2 → M3 → M4**; each compiles and its tests pass before the next starts. The following seams cross plan boundaries and are resolved here authoritatively (plans defer to this section):

1. **CLI entrypoint = `cli.NewRootCmd() *cobra.Command` (M1).** `cmd/claude_git/main.go` builds it and `Execute()`s it. M2/M3/M4 attach their commands additively in the existing `root.AddCommand(...)` block inside `NewRootCmd()` — never replace it, never introduce a second root builder.

2. **Bare-arg dispatch modifies `main.go` (M3).** M3 adds `cli.DispatchArgs([]string) []string` and `cli.reservedSubcommands`. The final `main.go` form is:
   ```go
   root := cli.NewRootCmd()
   root.SetArgs(cli.DispatchArgs(os.Args[1:])) // maps `claude_git <persona>` -> `claude_git use <persona>` for non-reserved first args
   if err := root.Execute(); err != nil { os.Exit(1) }
   ```
   This is the one M3-owned edit to the M1 file; do it when building M3.

3. **`Environment.Author` seam (M1 owns, M2 consumes).** `EnvConfig` carries `Author` (see §5). `environment.Create` sets `cfg.Author = defaultAuthor()` at init and persists it to `env.toml`. M2's `func (e *Environment) Author() string { return e.cfg.Author }` then compiles unchanged. `defaultAuthor()` lives in `internal/environment` (M1).

4. **`.gitignore` seam (M4 modifies M1's `Create`).** `environment.Create` writes `repo/.gitignore` from a package-level `defaultGitignore` constant in `internal/environment` (inlined to avoid an `environment`→`share` import cycle). `share.DefaultGitignore()` returns the same bytes; a drift-guard test keeps them identical. When building M4, extend M1's `Create` test to assert the `.gitignore` exists. Reuse M1's `writeEnvConfig`/`writeMarker` if present; delete M4's duplicates.

5. **Command registration is additive.** Each of M2/M3/M4 appends its `newXxxCmd()` calls to the single `AddCommand` block; order does not matter. No command name collides (verified against §11 of the spec).

**Net:** one real fix (seam 3, now in §5) and four coordination notes. No other cross-plan type or signature mismatch was found in self-review.
