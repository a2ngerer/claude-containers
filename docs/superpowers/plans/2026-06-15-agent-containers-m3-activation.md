# M3 Activation — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Materialize a composed persona into an isolated `CLAUDE_CONFIG_DIR` outside the workspace, enforce read-only isolation via deny rules and allowlist-only material, verify it fail-closed, and launch (or print) the correct `claude` invocation — so `acon reviewer` activates an uncontaminated environment without ever touching the workspace `.claude/`.

**Architecture:** Three new internal packages sit on top of M2's `compose.Compose`. `materialize` renders a `ResolvedManifest` to disk (skills/subagents copied from the persona repo tree, plus generated `CLAUDE.md`, `settings.json`, `mcp.json`); `enforce` builds the permission set and verifies a materialized dir against the manifest (producing a `domain.Attestation`); `activate` orchestrates compose → lock → materialize → verify (fail-closed) → build-launch and exposes the result to thin `cli` commands. Isolation rests on two orthogonal mechanisms (material withholding + tool denial) and is *verified*, not just displayed.

**Tech Stack:** Go 1.23+, `github.com/spf13/cobra`, `encoding/json` (settings/mcp/lockfile), `github.com/stretchr/testify/require`, std `os`/`filepath`/`syscall`.

**Depends on:** M1 (`domain` types, `storage.StorageEngine` + `OpenGit`, `environment.Environment`/paths), M2 (`compose.Compose` + `compose.ResolvedManifest`).

---

## File Structure

| File | Responsibility |
|---|---|
| `internal/enforce/enforce.go` | `PermissionSet` + `BuildPermissions(enf)`: read-only ⇒ standard write-deny ∪ `enf.ToolsDeny`; allow = `enf.ToolsAllow`. |
| `internal/enforce/enforce_test.go` | Table tests for read-only vs. default mode, dedup, allow passthrough. |
| `internal/materialize/settings.go` | `settings.json` schema structs + `writeSettings(destDir, PermissionSet)`. |
| `internal/materialize/mcp.go` | `writeMCP(destDir, MCPConfig)`: keep persona-local `mcp.json` when `MCP.Config != ""`, else ensure no file. |
| `internal/materialize/materialize.go` | `Materialize(e, rm, destDir)`: clean destDir, copy allowlisted skills/subagents, write `CLAUDE.md` + settings + mcp; idempotent. |
| `internal/materialize/materialize_test.go` | Idempotency (byte-identical on re-run), allowlist-only (no foreign files), CLAUDE.md content. |
| `internal/enforce/verify.go` | `Verify(rm, destDir)`: assert exactly the allowlisted material + expected deny rules; build `domain.Attestation`; `domain.ErrVerifyMismatch` on drift. |
| `internal/enforce/verify_test.go` | Clean pass, smuggled-in build skill ⇒ mismatch, missing deny ⇒ mismatch. |
| `internal/activate/launch.go` | `LaunchSpec` + `BuildLaunch(configDir, rm)`: exact env + argv; omit `--mcp-config`/`--strict-mcp-config` when no MCP. |
| `internal/activate/launch_test.go` | Exact flag set with and without MCP. |
| `internal/activate/lock.go` | `Lock` over `<EnvDir>/lock` holding persona+PID; `Acquire` returns `domain.ErrLocked` on a foreign live lock, re-locks own/stale; `Release`. |
| `internal/activate/lock_test.go` | Acquire on free dir, re-lock by same PID, foreign-live ⇒ ErrLocked, stale (dead PID) re-lock, Release. |
| `internal/activate/activate.go` | `ActivationResult` + `Activate(e, personaRef)`: parse ref → compose → lock → materialize(CacheDir) → verify (fail-closed) → BuildLaunch → SetActive. |
| `internal/activate/activate_test.go` | End-to-end happy path, fail-closed semantics, `:latest` default, not-found. |
| `internal/cli/use.go` | `use <persona>` command, `deactivate` command (same file), `--exec` flag, and the dispatch shim mapping a non-reserved first arg to `use`. |
| `internal/cli/verify.go` | `verify <persona>` command: compose + materialize + Verify; non-zero exit + diff on mismatch. |
| `internal/cli/use_test.go` | Dispatch: reserved subcommand vs. persona-arg routing. |
| `internal/cli/verify_test.go` | `verify` clean output + non-zero error path. |

**New types beyond the architecture contract** (defined in the owning package, flagged again in the closing summary):
- `materialize.settingsFile`, `materialize.settingsPermissions` — private JSON-shape structs for `settings.json` (the contract specifies the file + its `permissions {allow,deny}` content, not the Go marshalling struct).
- `materialize.mcpFile` — private JSON-shape struct for an empty placeholder `mcp.json` (used only when `MCP.Config` names a file the checkout did not provide).
- `activate.Lock` (struct) + `activate.lockState` (private JSON shape of the lockfile body). The contract names `Lock` and `Acquire/Release` but does not fix the struct's fields; defined here.
- `cli.reservedSubcommands` (package `map[string]bool`) + `cli.isReserved(name) bool` + `cli.DispatchArgs([]string) []string` — the dispatch machinery for "non-reserved first arg ⇒ `use`".

---

### Task 0: Package skeletons compile

**Files:**
- Create: `internal/enforce/enforce.go`
- Create: `internal/materialize/materialize.go`
- Create: `internal/activate/activate.go`

- [ ] **Step 1: Create the three package files with only their package clause**

`internal/enforce/enforce.go`:

```go
package enforce
```

`internal/materialize/materialize.go`:

```go
package materialize
```

`internal/activate/activate.go`:

```go
package activate
```

- [ ] **Step 2: Verify the packages build**

Run: `cd /Users/angeral/Repositories/agent-containers && go build ./internal/...`
Expected: no output, exit 0 (empty packages compile).

- [ ] **Step 3: Commit**

```bash
cd /Users/angeral/Repositories/agent-containers
git add internal/enforce/enforce.go internal/materialize/materialize.go internal/activate/activate.go
git commit -m "chore: scaffold M3 activation packages"
```

---

### Task 1: `enforce.BuildPermissions`

**Files:**
- Modify: `internal/enforce/enforce.go`
- Test: `internal/enforce/enforce_test.go`

- [ ] **Step 1: Write the failing test**

`internal/enforce/enforce_test.go`:

```go
package enforce

import (
	"testing"

	"github.com/a2ngerer/agent-containers/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestBuildPermissions(t *testing.T) {
	tests := []struct {
		name      string
		enf       domain.Enforcement
		wantAllow []string
		wantDeny  []string
		wantMode  string
	}{
		{
			name: "read-only adds standard write denials",
			enf: domain.Enforcement{
				PermissionMode: "read-only",
				ToolsAllow:     []string{"Read", "Grep"},
				ToolsDeny:      []string{"Bash(git push:*)"},
			},
			wantAllow: []string{"Read", "Grep"},
			wantDeny:  []string{"Write", "Edit", "NotebookEdit", "Bash(git push:*)"},
			wantMode:  "read-only",
		},
		{
			name: "default mode keeps only explicit denials",
			enf: domain.Enforcement{
				PermissionMode: "default",
				ToolsAllow:     []string{"Read", "Write", "Edit"},
				ToolsDeny:      []string{"Bash(rm:*)"},
			},
			wantAllow: []string{"Read", "Write", "Edit"},
			wantDeny:  []string{"Bash(rm:*)"},
			wantMode:  "default",
		},
		{
			name: "read-only deduplicates an explicit Write denial",
			enf: domain.Enforcement{
				PermissionMode: "read-only",
				ToolsAllow:     nil,
				ToolsDeny:      []string{"Write", "Bash(git commit:*)"},
			},
			wantAllow: nil,
			wantDeny:  []string{"Write", "Edit", "NotebookEdit", "Bash(git commit:*)"},
			wantMode:  "read-only",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildPermissions(tt.enf)
			require.Equal(t, tt.wantAllow, got.Allow)
			require.Equal(t, tt.wantDeny, got.Deny)
			require.Equal(t, tt.wantMode, got.Mode)
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/angeral/Repositories/agent-containers && go test ./internal/enforce/ -run TestBuildPermissions -v`
Expected: FAIL — `undefined: BuildPermissions` / `undefined: PermissionSet`.

- [ ] **Step 3: Write minimal implementation**

`internal/enforce/enforce.go`:

```go
package enforce

import "github.com/a2ngerer/agent-containers/internal/domain"

// PermissionSet is the resolved allow/deny pair plus the permission mode that
// gets serialized into the materialized settings.json.
type PermissionSet struct {
	Allow []string
	Deny  []string
	Mode  string
}

// readOnlyBaseDeny are the write-capable tools that a read-only persona must
// never be able to invoke, regardless of what its manifest lists. A deny at any
// level is final (Claude Code semantics), so these are always present in
// read-only mode.
var readOnlyBaseDeny = []string{"Write", "Edit", "NotebookEdit"}

// BuildPermissions turns a persona's enforcement block into the concrete
// permission set. In "read-only" mode the standard write denials are unioned
// with the manifest's explicit denials (deduplicated, base rules first). Allow
// is passed through verbatim from the manifest.
func BuildPermissions(enf domain.Enforcement) PermissionSet {
	deny := make([]string, 0, len(readOnlyBaseDeny)+len(enf.ToolsDeny))
	seen := make(map[string]bool)

	if enf.PermissionMode == "read-only" {
		for _, d := range readOnlyBaseDeny {
			if !seen[d] {
				seen[d] = true
				deny = append(deny, d)
			}
		}
	}
	for _, d := range enf.ToolsDeny {
		if !seen[d] {
			seen[d] = true
			deny = append(deny, d)
		}
	}
	if len(deny) == 0 {
		deny = nil
	}

	return PermissionSet{
		Allow: enf.ToolsAllow,
		Deny:  deny,
		Mode:  enf.PermissionMode,
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /Users/angeral/Repositories/agent-containers && go test ./internal/enforce/ -run TestBuildPermissions -v`
Expected: PASS — all three subtests.

- [ ] **Step 5: Commit**

```bash
cd /Users/angeral/Repositories/agent-containers
git add internal/enforce/enforce.go internal/enforce/enforce_test.go
git commit -m "feat(enforce): BuildPermissions with read-only base denials"
```

---

### Task 2: `materialize` settings.json writer

**Files:**
- Create: `internal/materialize/settings.go`
- Test: `internal/materialize/settings_test.go`

- [ ] **Step 1: Write the failing test**

`internal/materialize/settings_test.go`:

```go
package materialize

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/a2ngerer/agent-containers/internal/enforce"
	"github.com/stretchr/testify/require"
)

func TestWriteSettings(t *testing.T) {
	dir := t.TempDir()
	ps := enforce.PermissionSet{
		Allow: []string{"Read", "Grep"},
		Deny:  []string{"Write", "Edit", "NotebookEdit"},
		Mode:  "read-only",
	}

	require.NoError(t, writeSettings(dir, ps))

	raw, err := os.ReadFile(filepath.Join(dir, "settings.json"))
	require.NoError(t, err)

	var got settingsFile
	require.NoError(t, json.Unmarshal(raw, &got))
	require.Equal(t, []string{"Read", "Grep"}, got.Permissions.Allow)
	require.Equal(t, []string{"Write", "Edit", "NotebookEdit"}, got.Permissions.Deny)
	require.Equal(t, "read-only", got.PermissionMode)

	// Deterministic, pretty-printed, trailing newline.
	require.Equal(t, byte('\n'), raw[len(raw)-1])
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/angeral/Repositories/agent-containers && go test ./internal/materialize/ -run TestWriteSettings -v`
Expected: FAIL — `undefined: writeSettings` / `undefined: settingsFile`.

- [ ] **Step 3: Write minimal implementation**

`internal/materialize/settings.go`:

```go
package materialize

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/a2ngerer/agent-containers/internal/enforce"
)

// settingsPermissions mirrors the "permissions" object Claude Code reads from
// settings.json. allow/deny use omitempty so an empty persona produces a clean
// object rather than null arrays.
type settingsPermissions struct {
	Allow []string `json:"allow,omitempty"`
	Deny  []string `json:"deny,omitempty"`
}

// settingsFile is the on-disk shape of the generated settings.json. permissionMode
// carries the persona's mode ("read-only" | "default") for visibility and verify.
type settingsFile struct {
	Permissions    settingsPermissions `json:"permissions"`
	PermissionMode string              `json:"permissionMode,omitempty"`
}

// writeSettings serializes the permission set into destDir/settings.json with a
// deterministic two-space indent and a trailing newline (so two runs are byte
// identical).
func writeSettings(destDir string, ps enforce.PermissionSet) error {
	sf := settingsFile{
		Permissions: settingsPermissions{
			Allow: ps.Allow,
			Deny:  ps.Deny,
		},
		PermissionMode: ps.Mode,
	}
	data, err := json.MarshalIndent(sf, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal settings: %w", err)
	}
	data = append(data, '\n')
	path := filepath.Join(destDir, "settings.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write settings.json: %w", err)
	}
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /Users/angeral/Repositories/agent-containers && go test ./internal/materialize/ -run TestWriteSettings -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd /Users/angeral/Repositories/agent-containers
git add internal/materialize/settings.go internal/materialize/settings_test.go
git commit -m "feat(materialize): deterministic settings.json writer"
```

---

### Task 3: `materialize` mcp.json writer

**Files:**
- Create: `internal/materialize/mcp.go`
- Test: `internal/materialize/mcp_test.go`

The persona-local `mcp.json` lives in the persona's repo directory and is copied into destDir by `Materialize` (Task 4) when the persona ships one. `writeMCP` performs the *post-copy reconciliation*: if `MCP.Config == ""` it guarantees no `mcp.json` leaks into the config dir (removes a stray one); if `MCP.Config != ""` it ensures the named file exists, writing an empty `{"mcpServers":{}}` placeholder only when the copy did not provide one — so the launch flag `--mcp-config <dir>/mcp.json` never dangles.

- [ ] **Step 1: Write the failing test**

`internal/materialize/mcp_test.go`:

```go
package materialize

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/a2ngerer/agent-containers/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestWriteMCP_NoConfigRemovesStrayFile(t *testing.T) {
	dir := t.TempDir()
	stray := filepath.Join(dir, "mcp.json")
	require.NoError(t, os.WriteFile(stray, []byte("{}"), 0o644))

	require.NoError(t, writeMCP(dir, domain.MCPConfig{Config: ""}))

	_, err := os.Stat(stray)
	require.True(t, os.IsNotExist(err), "mcp.json must not exist when Config is empty")
}

func TestWriteMCP_ConfigKeepsExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mcp.json")
	original := []byte(`{"mcpServers":{"github":{}}}`)
	require.NoError(t, os.WriteFile(path, original, 0o644))

	require.NoError(t, writeMCP(dir, domain.MCPConfig{Config: "mcp.json"}))

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, original, got, "existing persona mcp.json must be left untouched")
}

func TestWriteMCP_ConfigWritesPlaceholderWhenMissing(t *testing.T) {
	dir := t.TempDir()

	require.NoError(t, writeMCP(dir, domain.MCPConfig{Config: "mcp.json"}))

	got, err := os.ReadFile(filepath.Join(dir, "mcp.json"))
	require.NoError(t, err)
	require.JSONEq(t, `{"mcpServers":{}}`, string(got))
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/angeral/Repositories/agent-containers && go test ./internal/materialize/ -run TestWriteMCP -v`
Expected: FAIL — `undefined: writeMCP`.

- [ ] **Step 3: Write minimal implementation**

`internal/materialize/mcp.go`:

```go
package materialize

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/a2ngerer/agent-containers/internal/domain"
)

// mcpFile is the minimal Claude Code mcp.json shape used only for the empty
// placeholder. A real persona-local mcp.json is copied verbatim and never
// re-encoded through this struct.
type mcpFile struct {
	MCPServers map[string]any `json:"mcpServers"`
}

// writeMCP reconciles destDir/mcp.json with the persona's MCP config.
//
//   - Config == "" : guarantee no mcp.json is present (remove a stray file so no
//     project MCP config can leak into the isolated config dir).
//   - Config != "" : ensure the named file exists; if the copy did not provide
//     one, write an empty {"mcpServers":{}} placeholder so the launch flag
//     --mcp-config <dir>/mcp.json never dangles.
func writeMCP(destDir string, mcp domain.MCPConfig) error {
	path := filepath.Join(destDir, "mcp.json")

	if mcp.Config == "" {
		if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("remove stray mcp.json: %w", err)
		}
		return nil
	}

	if _, err := os.Stat(path); err == nil {
		return nil // persona shipped its own mcp.json; leave it untouched
	} else if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("stat mcp.json: %w", err)
	}

	data, err := json.Marshal(mcpFile{MCPServers: map[string]any{}})
	if err != nil {
		return fmt.Errorf("marshal mcp placeholder: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write mcp.json placeholder: %w", err)
	}
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /Users/angeral/Repositories/agent-containers && go test ./internal/materialize/ -run TestWriteMCP -v`
Expected: PASS — all three subtests.

- [ ] **Step 5: Commit**

```bash
cd /Users/angeral/Repositories/agent-containers
git add internal/materialize/mcp.go internal/materialize/mcp_test.go
git commit -m "feat(materialize): reconcile mcp.json (none-vs-strict)"
```

---

### Task 4: `materialize.Materialize` (idempotent render)

**Files:**
- Modify: `internal/materialize/materialize.go`
- Test: `internal/materialize/materialize_test.go`

`Materialize` is the contract entry point `func Materialize(e *environment.Environment, rm compose.ResolvedManifest, destDir string) error`. It must be idempotent: a second run yields a byte-identical dir. Strategy is **clean-then-build**: remove destDir wholesale, recreate it, copy only allowlisted skills/subagents from the persona repo tree, then write `CLAUDE.md`, `settings.json`, `mcp.json`. Skills live under `<RepoDir>/personas/<persona>/skills/<name>/` and subagents under `<RepoDir>/personas/<persona>/agents/<name>.md` (on-disk layout, contract §6). `rm.Skills`/`rm.Subagents` are authoritative (compose already merged `_base`), and the copy reads from the persona dir on disk.

The test seeds a fake persona repo tree under a temp `ACON_HOME`, builds a `ResolvedManifest` by hand, and asserts allowlist-only + idempotency.

- [ ] **Step 1: Write the failing test**

`internal/materialize/materialize_test.go`:

```go
package materialize

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/a2ngerer/agent-containers/internal/compose"
	"github.com/a2ngerer/agent-containers/internal/domain"
	"github.com/a2ngerer/agent-containers/internal/environment"
	"github.com/stretchr/testify/require"
)

// seedRepo creates a minimal persona repo tree the way init/snapshot would, so
// Materialize has skills/agents to copy. Returns the opened environment.
func seedRepo(t *testing.T, persona string) *environment.Environment {
	t.Helper()
	home := t.TempDir()
	t.Setenv("ACON_HOME", home)
	ws := t.TempDir()

	e, err := environment.Create(ws)
	require.NoError(t, err)

	pdir := filepath.Join(environment.RepoDir(e.Hash), "personas", persona)
	// allowlisted skill
	require.NoError(t, os.MkdirAll(filepath.Join(pdir, "skills", "security-review"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(pdir, "skills", "security-review", "SKILL.md"),
		[]byte("# security-review\n"), 0o644))
	// a skill that is NOT in the allowlist -> must never be materialized
	require.NoError(t, os.MkdirAll(filepath.Join(pdir, "skills", "writing-plans"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(pdir, "skills", "writing-plans", "SKILL.md"),
		[]byte("# writing-plans\n"), 0o644))
	// allowlisted subagent
	require.NoError(t, os.MkdirAll(filepath.Join(pdir, "agents"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(pdir, "agents", "code-reviewer.md"),
		[]byte("# code-reviewer\n"), 0o644))

	return e
}

func reviewerManifest() compose.ResolvedManifest {
	return compose.ResolvedManifest{
		Persona: domain.Persona{
			Name:     "reviewer",
			Metadata: domain.Metadata{Version: "1.2.0"},
		},
		Skills:     []string{"security-review"},
		Subagents:  []string{"code-reviewer"},
		ClaudeMD:   "# reviewer\nUncontaminated.\n",
		SettingSrc: []string{"user", "project"},
		Enforcement: domain.Enforcement{
			PermissionMode: "read-only",
			ToolsAllow:     []string{"Read", "Grep"},
			ToolsDeny:      []string{"Bash(git commit:*)"},
		},
		MCP: domain.MCPConfig{Config: "", Strict: true},
	}
}

func TestMaterialize_AllowlistOnly(t *testing.T) {
	e := seedRepo(t, "reviewer")
	rm := reviewerManifest()
	dest := filepath.Join(t.TempDir(), "cfg")

	require.NoError(t, Materialize(e, rm, dest))

	// allowlisted skill present
	require.FileExists(t, filepath.Join(dest, "skills", "security-review", "SKILL.md"))
	// withheld skill absent
	_, err := os.Stat(filepath.Join(dest, "skills", "writing-plans"))
	require.True(t, os.IsNotExist(err), "non-allowlisted skill must not be materialized")
	// subagent present
	require.FileExists(t, filepath.Join(dest, "agents", "code-reviewer.md"))
	// generated files
	require.FileExists(t, filepath.Join(dest, "CLAUDE.md"))
	require.FileExists(t, filepath.Join(dest, "settings.json"))
	// MCP off -> no mcp.json
	_, err = os.Stat(filepath.Join(dest, "mcp.json"))
	require.True(t, os.IsNotExist(err))

	md, err := os.ReadFile(filepath.Join(dest, "CLAUDE.md"))
	require.NoError(t, err)
	require.Equal(t, "# reviewer\nUncontaminated.\n", string(md))
}

func TestMaterialize_Idempotent(t *testing.T) {
	e := seedRepo(t, "reviewer")
	rm := reviewerManifest()
	dest := filepath.Join(t.TempDir(), "cfg")

	require.NoError(t, Materialize(e, rm, dest))
	first := snapshotDir(t, dest)

	require.NoError(t, Materialize(e, rm, dest))
	second := snapshotDir(t, dest)

	require.Equal(t, first, second, "second materialize must be byte-identical")
}

// snapshotDir returns a deterministic map of relative path -> file content for
// every regular file under root.
func snapshotDir(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		out[rel] = string(data)
		return nil
	})
	require.NoError(t, err)
	return out
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/angeral/Repositories/agent-containers && go test ./internal/materialize/ -run TestMaterialize -v`
Expected: FAIL — `undefined: Materialize`.

- [ ] **Step 3: Write minimal implementation**

`internal/materialize/materialize.go`:

```go
package materialize

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/a2ngerer/agent-containers/internal/compose"
	"github.com/a2ngerer/agent-containers/internal/enforce"
	"github.com/a2ngerer/agent-containers/internal/environment"
)

// Materialize renders rm into destDir (a CLAUDE_CONFIG_DIR outside the
// workspace). It copies only the allowlisted skills/subagents from the persona
// repo, writes the composed CLAUDE.md, the enforcement settings.json, and
// reconciles mcp.json. It is idempotent: running it twice yields a byte-identical
// destDir (clean-then-build).
func Materialize(e *environment.Environment, rm compose.ResolvedManifest, destDir string) error {
	if err := os.RemoveAll(destDir); err != nil {
		return fmt.Errorf("clean dest dir: %w", err)
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("create dest dir: %w", err)
	}

	personaDir := filepath.Join(environment.RepoDir(e.Hash), "personas", rm.Persona.Name)

	if err := copySkills(personaDir, destDir, rm.Skills); err != nil {
		return err
	}
	if err := copySubagents(personaDir, destDir, rm.Subagents); err != nil {
		return err
	}

	if err := os.WriteFile(filepath.Join(destDir, "CLAUDE.md"), []byte(rm.ClaudeMD), 0o644); err != nil {
		return fmt.Errorf("write CLAUDE.md: %w", err)
	}

	ps := enforce.BuildPermissions(rm.Enforcement)
	if err := writeSettings(destDir, ps); err != nil {
		return err
	}

	if err := writeMCP(destDir, rm.MCP); err != nil {
		return err
	}

	return nil
}

// copySkills copies each allowlisted skill directory from <personaDir>/skills/<name>
// to <destDir>/skills/<name>. A missing source skill is an error (the manifest
// promised it).
func copySkills(personaDir, destDir string, skills []string) error {
	for _, name := range skills {
		src := filepath.Join(personaDir, "skills", name)
		dst := filepath.Join(destDir, "skills", name)
		if err := copyTree(src, dst); err != nil {
			return fmt.Errorf("copy skill %q: %w", name, err)
		}
	}
	return nil
}

// copySubagents copies each allowlisted subagent file <personaDir>/agents/<name>.md
// to <destDir>/agents/<name>.md.
func copySubagents(personaDir, destDir string, subagents []string) error {
	if len(subagents) == 0 {
		return nil
	}
	if err := os.MkdirAll(filepath.Join(destDir, "agents"), 0o755); err != nil {
		return fmt.Errorf("create agents dir: %w", err)
	}
	for _, name := range subagents {
		src := filepath.Join(personaDir, "agents", name+".md")
		dst := filepath.Join(destDir, "agents", name+".md")
		if err := copyFile(src, dst); err != nil {
			return fmt.Errorf("copy subagent %q: %w", name, err)
		}
	}
	return nil
}

// copyTree recursively copies the directory at src into dst, preserving the
// relative structure. File mode is normalized (0644 files, 0755 dirs) so two
// materializations are byte/metadata identical.
func copyTree(src, dst string) error {
	return filepath.Walk(src, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		return copyFile(p, target)
	})
}

// copyFile copies a single regular file, creating parent dirs as needed and
// normalizing the mode to 0644.
func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /Users/angeral/Repositories/agent-containers && go test ./internal/materialize/ -v`
Expected: PASS — `TestWriteSettings`, `TestWriteMCP*`, `TestMaterialize_AllowlistOnly`, `TestMaterialize_Idempotent`.

- [ ] **Step 5: Commit**

```bash
cd /Users/angeral/Repositories/agent-containers
git add internal/materialize/materialize.go internal/materialize/materialize_test.go
git commit -m "feat(materialize): idempotent allowlist-only render to config dir"
```

---

### Task 5: `enforce.Verify` (fail-closed attestation)

**Files:**
- Create: `internal/enforce/verify.go`
- Test: `internal/enforce/verify_test.go`

`Verify(rm compose.ResolvedManifest, destDir string) (domain.Attestation, error)` re-reads the materialized dir and asserts: (1) the set of skill directories and subagent files present equals **exactly** the allowlist (no extras, none missing); (2) `settings.json` contains every deny rule `BuildPermissions` would produce. On any mismatch it returns `domain.ErrVerifyMismatch` (wrapped with detail) and an attestation with `Clean=false`. On success it builds the full `domain.Attestation` (Included/Denied/SettingSrc/Clean=true). The richer `Withheld` narrative needs repo access and is filled in `activate` (Task 8); `Verify` leaves it empty.

- [ ] **Step 1: Write the failing test**

`internal/enforce/verify_test.go`:

```go
package enforce

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/a2ngerer/agent-containers/internal/compose"
	"github.com/a2ngerer/agent-containers/internal/domain"
	"github.com/stretchr/testify/require"
)

// buildMaterializedDir writes a minimal materialized config dir that matches rm:
// the allowlisted skills/subagents plus a settings.json carrying the expected
// deny rules.
func buildMaterializedDir(t *testing.T, rm compose.ResolvedManifest) string {
	t.Helper()
	dir := t.TempDir()
	for _, s := range rm.Skills {
		require.NoError(t, os.MkdirAll(filepath.Join(dir, "skills", s), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "skills", s, "SKILL.md"), []byte("x"), 0o644))
	}
	for _, a := range rm.Subagents {
		require.NoError(t, os.MkdirAll(filepath.Join(dir, "agents"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "agents", a+".md"), []byte("x"), 0o644))
	}
	ps := BuildPermissions(rm.Enforcement)
	sf := map[string]any{
		"permissions":    map[string]any{"allow": ps.Allow, "deny": ps.Deny},
		"permissionMode": ps.Mode,
	}
	data, err := json.MarshalIndent(sf, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "settings.json"), data, 0o644))
	return dir
}

func reviewerRM() compose.ResolvedManifest {
	return compose.ResolvedManifest{
		Persona:    domain.Persona{Name: "reviewer", Metadata: domain.Metadata{Version: "1.2.0"}},
		Skills:     []string{"security-review"},
		Subagents:  []string{"code-reviewer"},
		SettingSrc: []string{"user", "project"},
		Enforcement: domain.Enforcement{
			PermissionMode: "read-only",
			ToolsAllow:     []string{"Read", "Grep"},
			ToolsDeny:      []string{"Bash(git commit:*)"},
		},
	}
}

func TestVerify_Clean(t *testing.T) {
	rm := reviewerRM()
	dir := buildMaterializedDir(t, rm)

	att, err := Verify(rm, dir)
	require.NoError(t, err)
	require.True(t, att.Clean)
	require.Equal(t, "reviewer", att.Persona)
	require.Equal(t, "1.2.0", att.Version)
	require.Equal(t, []string{"user", "project"}, att.SettingSrc)
	require.Contains(t, att.Denied, "Write")
	require.Contains(t, att.Denied, "Bash(git commit:*)")

	// Included carries the skill + subagent names.
	var gotSkills, gotSubagents []string
	for _, line := range att.Included {
		switch line.Kind {
		case "skill":
			gotSkills = line.Names
		case "subagent":
			gotSubagents = line.Names
		}
	}
	require.Equal(t, []string{"security-review"}, gotSkills)
	require.Equal(t, []string{"code-reviewer"}, gotSubagents)
}

func TestVerify_SmuggledSkillMismatch(t *testing.T) {
	rm := reviewerRM()
	dir := buildMaterializedDir(t, rm)
	// Smuggle a build skill into the materialized dir.
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "skills", "writing-plans"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "skills", "writing-plans", "SKILL.md"), []byte("x"), 0o644))

	att, err := Verify(rm, dir)
	require.ErrorIs(t, err, domain.ErrVerifyMismatch)
	require.False(t, att.Clean)
}

func TestVerify_MissingDenyMismatch(t *testing.T) {
	rm := reviewerRM()
	dir := buildMaterializedDir(t, rm)
	// Rewrite settings.json without the mandatory Write deny.
	sf := map[string]any{
		"permissions":    map[string]any{"allow": []string{"Read"}, "deny": []string{"Edit", "NotebookEdit"}},
		"permissionMode": "read-only",
	}
	data, _ := json.MarshalIndent(sf, "", "  ")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "settings.json"), data, 0o644))

	att, err := Verify(rm, dir)
	require.True(t, errors.Is(err, domain.ErrVerifyMismatch))
	require.False(t, att.Clean)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/angeral/Repositories/agent-containers && go test ./internal/enforce/ -run TestVerify -v`
Expected: FAIL — `undefined: Verify`.

- [ ] **Step 3: Write minimal implementation**

`internal/enforce/verify.go`:

```go
package enforce

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/a2ngerer/agent-containers/internal/compose"
	"github.com/a2ngerer/agent-containers/internal/domain"
)

// Verify asserts that destDir contains exactly the allowlisted skills/subagents
// from rm and that its settings.json carries every deny rule BuildPermissions
// would produce. It returns a domain.Attestation describing the activation. On
// any drift it returns domain.ErrVerifyMismatch (wrapped) and Clean=false.
func Verify(rm compose.ResolvedManifest, destDir string) (domain.Attestation, error) {
	ps := BuildPermissions(rm.Enforcement)

	att := domain.Attestation{
		Persona:    rm.Persona.Name,
		Version:    rm.Persona.Metadata.Version,
		Denied:     ps.Deny,
		SettingSrc: rm.SettingSrc,
		Clean:      false,
	}
	if len(rm.Skills) > 0 {
		att.Included = append(att.Included, domain.AttestationLine{Kind: "skill", Names: rm.Skills})
	}
	if len(rm.Subagents) > 0 {
		att.Included = append(att.Included, domain.AttestationLine{Kind: "subagent", Names: rm.Subagents})
	}
	if rm.MCP.Config != "" {
		att.Included = append(att.Included, domain.AttestationLine{Kind: "mcp", Names: []string{rm.MCP.Config}})
	}

	var problems []string

	// (1) skills present on disk must equal the allowlist exactly.
	gotSkills, err := listDir(filepath.Join(destDir, "skills"))
	if err != nil {
		return att, fmt.Errorf("read materialized skills: %w", err)
	}
	if diff := setDiff(rm.Skills, gotSkills); diff != "" {
		problems = append(problems, "skills "+diff)
	}

	// (2) subagents present on disk must equal the allowlist exactly.
	gotSubagents, err := listSubagents(filepath.Join(destDir, "agents"))
	if err != nil {
		return att, fmt.Errorf("read materialized agents: %w", err)
	}
	if diff := setDiff(rm.Subagents, gotSubagents); diff != "" {
		problems = append(problems, "subagents "+diff)
	}

	// (3) settings.json must contain every expected deny rule.
	gotDeny, err := readDeny(filepath.Join(destDir, "settings.json"))
	if err != nil {
		return att, fmt.Errorf("read materialized settings: %w", err)
	}
	if missing := missingFrom(ps.Deny, gotDeny); len(missing) > 0 {
		problems = append(problems, "missing deny rules: "+strings.Join(missing, ", "))
	}

	if len(problems) > 0 {
		return att, fmt.Errorf("%w: %s", domain.ErrVerifyMismatch, strings.Join(problems, "; "))
	}

	att.Clean = true
	return att, nil
}

// listDir returns the sorted names of immediate sub-entries of dir. A missing
// dir yields an empty slice (no skills/agents materialized).
func listDir(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	return names, nil
}

// listSubagents returns sorted subagent basenames (without the .md suffix) found
// in dir.
func listSubagents(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		names = append(names, strings.TrimSuffix(e.Name(), ".md"))
	}
	sort.Strings(names)
	return names, nil
}

// readDeny extracts permissions.deny from a settings.json file.
func readDeny(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var sf struct {
		Permissions struct {
			Deny []string `json:"deny"`
		} `json:"permissions"`
	}
	if err := json.Unmarshal(data, &sf); err != nil {
		return nil, fmt.Errorf("parse settings.json: %w", err)
	}
	return sf.Permissions.Deny, nil
}

// setDiff returns "" if want and got contain the same elements (order-independent),
// otherwise a human-readable description of the discrepancy.
func setDiff(want, got []string) string {
	wantSet := toSet(want)
	gotSet := toSet(got)
	var extra, missing []string
	for g := range gotSet {
		if !wantSet[g] {
			extra = append(extra, g)
		}
	}
	for w := range wantSet {
		if !gotSet[w] {
			missing = append(missing, w)
		}
	}
	if len(extra) == 0 && len(missing) == 0 {
		return ""
	}
	sort.Strings(extra)
	sort.Strings(missing)
	parts := []string{}
	if len(extra) > 0 {
		parts = append(parts, "unexpected: "+strings.Join(extra, ", "))
	}
	if len(missing) > 0 {
		parts = append(parts, "missing: "+strings.Join(missing, ", "))
	}
	return strings.Join(parts, "; ")
}

// missingFrom returns the elements of want that are absent from got.
func missingFrom(want, got []string) []string {
	gotSet := toSet(got)
	var missing []string
	for _, w := range want {
		if !gotSet[w] {
			missing = append(missing, w)
		}
	}
	return missing
}

func toSet(s []string) map[string]bool {
	m := make(map[string]bool, len(s))
	for _, v := range s {
		m[v] = true
	}
	return m
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /Users/angeral/Repositories/agent-containers && go test ./internal/enforce/ -v`
Expected: PASS — `TestBuildPermissions`, `TestVerify_Clean`, `TestVerify_SmuggledSkillMismatch`, `TestVerify_MissingDenyMismatch`.

- [ ] **Step 5: Commit**

```bash
cd /Users/angeral/Repositories/agent-containers
git add internal/enforce/verify.go internal/enforce/verify_test.go
git commit -m "feat(enforce): Verify materialized dir fail-closed with attestation"
```

---

### Task 6: `activate.BuildLaunch` (exact flag assembly)

**Files:**
- Create: `internal/activate/launch.go`
- Test: `internal/activate/launch_test.go`

`BuildLaunch(configDir string, rm compose.ResolvedManifest) LaunchSpec` produces env + argv. Per the milestone scope the argv is:

```
claude
  --setting-sources <csv(rm.SettingSrc)>
  --strict-mcp-config            (only when MCP configured)
  --mcp-config <configDir>/mcp.json   (only when MCP configured)
  --allowedTools <join(allow)>
  --append-system-prompt @<configDir>/CLAUDE.md
```

The allow set is `enforce.BuildPermissions(rm.Enforcement).Allow`. MCP flags are emitted iff `rm.MCP.Config != ""`. Env is always `["CLAUDE_CONFIG_DIR=<configDir>"]`.

- [ ] **Step 1: Write the failing test**

`internal/activate/launch_test.go`:

```go
package activate

import (
	"testing"

	"github.com/a2ngerer/agent-containers/internal/compose"
	"github.com/a2ngerer/agent-containers/internal/domain"
	"github.com/stretchr/testify/require"
)

func baseRM() compose.ResolvedManifest {
	return compose.ResolvedManifest{
		Persona:    domain.Persona{Name: "reviewer"},
		SettingSrc: []string{"user", "project"},
		Enforcement: domain.Enforcement{
			PermissionMode: "read-only",
			ToolsAllow:     []string{"Read", "Grep", "Glob"},
		},
	}
}

func TestBuildLaunch_NoMCP(t *testing.T) {
	rm := baseRM()
	rm.MCP = domain.MCPConfig{Config: ""}

	spec := BuildLaunch("/tmp/cfg", rm)

	require.Equal(t, []string{"CLAUDE_CONFIG_DIR=/tmp/cfg"}, spec.Env)
	require.Equal(t, []string{
		"claude",
		"--setting-sources", "user,project",
		"--allowedTools", "Read,Grep,Glob",
		"--append-system-prompt", "@/tmp/cfg/CLAUDE.md",
	}, spec.Argv)
	require.NotContains(t, spec.Argv, "--mcp-config")
	require.NotContains(t, spec.Argv, "--strict-mcp-config")
}

func TestBuildLaunch_WithMCP(t *testing.T) {
	rm := baseRM()
	rm.MCP = domain.MCPConfig{Config: "mcp.json", Strict: true}

	spec := BuildLaunch("/tmp/cfg", rm)

	require.Equal(t, []string{
		"claude",
		"--setting-sources", "user,project",
		"--strict-mcp-config",
		"--mcp-config", "/tmp/cfg/mcp.json",
		"--allowedTools", "Read,Grep,Glob",
		"--append-system-prompt", "@/tmp/cfg/CLAUDE.md",
	}, spec.Argv)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/angeral/Repositories/agent-containers && go test ./internal/activate/ -run TestBuildLaunch -v`
Expected: FAIL — `undefined: BuildLaunch` / `undefined: LaunchSpec`.

- [ ] **Step 3: Write minimal implementation**

`internal/activate/launch.go`:

```go
package activate

import (
	"path/filepath"
	"strings"

	"github.com/a2ngerer/agent-containers/internal/compose"
	"github.com/a2ngerer/agent-containers/internal/enforce"
)

// LaunchSpec is the environment + argv to start (or print) for claude.
type LaunchSpec struct {
	Env  []string // e.g. ["CLAUDE_CONFIG_DIR=..."]
	Argv []string // e.g. ["claude","--setting-sources","user,project", ...]
}

// BuildLaunch assembles the exact, reproducible claude invocation for a
// materialized config dir. MCP flags (--strict-mcp-config, --mcp-config) are
// emitted only when the persona configures an MCP file; otherwise they are
// omitted entirely so no MCP source is consulted.
func BuildLaunch(configDir string, rm compose.ResolvedManifest) LaunchSpec {
	allow := enforce.BuildPermissions(rm.Enforcement).Allow

	argv := []string{
		"claude",
		"--setting-sources", strings.Join(rm.SettingSrc, ","),
	}
	if rm.MCP.Config != "" {
		argv = append(argv,
			"--strict-mcp-config",
			"--mcp-config", filepath.Join(configDir, "mcp.json"),
		)
	}
	argv = append(argv,
		"--allowedTools", strings.Join(allow, ","),
		"--append-system-prompt", "@"+filepath.Join(configDir, "CLAUDE.md"),
	)

	return LaunchSpec{
		Env:  []string{"CLAUDE_CONFIG_DIR=" + configDir},
		Argv: argv,
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /Users/angeral/Repositories/agent-containers && go test ./internal/activate/ -run TestBuildLaunch -v`
Expected: PASS — both subtests.

- [ ] **Step 5: Commit**

```bash
cd /Users/angeral/Repositories/agent-containers
git add internal/activate/launch.go internal/activate/launch_test.go
git commit -m "feat(activate): BuildLaunch with conditional MCP flags"
```

---

### Task 7: `activate.Lock` (cross-process lockfile)

**Files:**
- Create: `internal/activate/lock.go`
- Test: `internal/activate/lock_test.go`

The lock lives at `<EnvDir(hash)>/lock` and holds `{persona, pid}` as JSON. `Acquire`:
- if no lockfile → write ours, return a `*Lock`.
- if a lockfile exists held by **a different live PID** → return `domain.ErrLocked`.
- if held by **our own PID**, or by a **dead PID** (stale) → overwrite and acquire.

`Release` removes the lockfile if (and only if) it is still ours. Liveness uses `syscall.Kill(pid, 0)` (signal 0 probes existence without sending a signal).

- [ ] **Step 1: Write the failing test**

`internal/activate/lock_test.go`:

```go
package activate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/a2ngerer/agent-containers/internal/domain"
	"github.com/a2ngerer/agent-containers/internal/environment"
	"github.com/stretchr/testify/require"
)

func lockEnv(t *testing.T) *environment.Environment {
	t.Helper()
	home := t.TempDir()
	t.Setenv("ACON_HOME", home)
	ws := t.TempDir()
	e, err := environment.Create(ws)
	require.NoError(t, err)
	return e
}

func lockPath(e *environment.Environment) string {
	return filepath.Join(environment.EnvDir(e.Hash), "lock")
}

func TestLock_AcquireOnFreeDir(t *testing.T) {
	e := lockEnv(t)

	lk, err := Acquire(e, "reviewer")
	require.NoError(t, err)
	require.NotNil(t, lk)

	data, err := os.ReadFile(lockPath(e))
	require.NoError(t, err)
	var st lockState
	require.NoError(t, json.Unmarshal(data, &st))
	require.Equal(t, "reviewer", st.Persona)
	require.Equal(t, os.Getpid(), st.PID)
}

func TestLock_ReLockBySamePID(t *testing.T) {
	e := lockEnv(t)
	_, err := Acquire(e, "coder")
	require.NoError(t, err)

	// Same process re-acquiring (e.g. switching persona) must succeed.
	lk, err := Acquire(e, "reviewer")
	require.NoError(t, err)
	require.NotNil(t, lk)
}

func TestLock_ForeignLivePIDIsLocked(t *testing.T) {
	e := lockEnv(t)
	// PID 1 is always alive on POSIX and is not us.
	writeForeignLock(t, e, "coder", 1)

	_, err := Acquire(e, "reviewer")
	require.ErrorIs(t, err, domain.ErrLocked)
}

func TestLock_StalePIDReLock(t *testing.T) {
	e := lockEnv(t)
	// A PID essentially guaranteed not to exist -> stale lock.
	writeForeignLock(t, e, "coder", 2147483640)

	lk, err := Acquire(e, "reviewer")
	require.NoError(t, err)
	require.NotNil(t, lk)
}

func TestLock_Release(t *testing.T) {
	e := lockEnv(t)
	lk, err := Acquire(e, "reviewer")
	require.NoError(t, err)

	require.NoError(t, lk.Release())
	_, err = os.Stat(lockPath(e))
	require.True(t, os.IsNotExist(err))
}

func writeForeignLock(t *testing.T, e *environment.Environment, persona string, pid int) {
	t.Helper()
	data, err := json.Marshal(lockState{Persona: persona, PID: pid})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(lockPath(e), data, 0o644))
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/angeral/Repositories/agent-containers && go test ./internal/activate/ -run TestLock -v`
Expected: FAIL — `undefined: Acquire` / `undefined: lockState`.

- [ ] **Step 3: Write minimal implementation**

`internal/activate/lock.go`:

```go
package activate

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"

	"github.com/a2ngerer/agent-containers/internal/domain"
	"github.com/a2ngerer/agent-containers/internal/environment"
)

// lockState is the JSON body of the environment lockfile.
type lockState struct {
	Persona string `json:"persona"`
	PID     int    `json:"pid"`
}

// Lock is a held environment lock. It guards cross-process acon mutations
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
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /Users/angeral/Repositories/agent-containers && go test ./internal/activate/ -run TestLock -v`
Expected: PASS — all five subtests.

- [ ] **Step 5: Commit**

```bash
cd /Users/angeral/Repositories/agent-containers
git add internal/activate/lock.go internal/activate/lock_test.go
git commit -m "feat(activate): cross-process lock with stale-PID reclaim"
```

---

### Task 8: `activate.Activate` (orchestration, fail-closed)

**Files:**
- Modify: `internal/activate/activate.go`
- Test: `internal/activate/activate_test.go`

`Activate(e *environment.Environment, personaRef string) (ActivationResult, error)`:
1. parse `personaRef` (`name` or `name:version`, version defaults to `latest`).
2. `compose.Compose(e, name)`.
3. `Acquire(e, name)` (lock); `defer Release` — the lock guards the mutation, not the later `claude` session.
4. `Materialize(e, rm, environment.CacheDir(e.Hash, name))`.
5. `enforce.Verify(rm, configDir)` — on `domain.ErrVerifyMismatch`, return the error and **do not** launch (fail closed).
6. enrich the attestation's `Withheld` with build skills physically present in the persona repo but not in the allowlist.
7. `e.SetActive(name)`.
8. `BuildLaunch(configDir, rm)` into the result.

The test seeds a real `_base` + `reviewer` persona on disk (with `persona.toml`) so M2's `compose.Compose` resolves, then asserts the happy path, the `:latest` default, and not-found. The fail-closed *mechanism* is unit-tested directly in `enforce.Verify` (Task 5); here the happy path proves `Activate` consumes `Verify` and only launches on `Clean`.

- [ ] **Step 1: Write the failing test**

`internal/activate/activate_test.go`:

```go
package activate

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/a2ngerer/agent-containers/internal/domain"
	"github.com/a2ngerer/agent-containers/internal/environment"
	"github.com/stretchr/testify/require"
)

// seedReviewerEnv writes a _base layer and a reviewer persona on disk so
// compose.Compose can resolve "reviewer". Returns the opened environment.
func seedReviewerEnv(t *testing.T) *environment.Environment {
	t.Helper()
	home := t.TempDir()
	t.Setenv("ACON_HOME", home)
	ws := t.TempDir()
	e, err := environment.Create(ws)
	require.NoError(t, err)

	repo := environment.RepoDir(e.Hash)

	// _base layer
	base := filepath.Join(repo, "personas", "_base")
	require.NoError(t, os.MkdirAll(base, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(base, "persona.toml"),
		[]byte("name = \"_base\"\n\n[config]\nclaude_md = \"CLAUDE.md\"\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(base, "CLAUDE.md"), []byte("# base\n"), 0o644))

	// reviewer persona
	rev := filepath.Join(repo, "personas", "reviewer")
	require.NoError(t, os.MkdirAll(filepath.Join(rev, "skills", "security-review"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(rev, "skills", "security-review", "SKILL.md"), []byte("# sr\n"), 0o644))
	// a withheld build skill physically present but not allowlisted
	require.NoError(t, os.MkdirAll(filepath.Join(rev, "skills", "writing-plans"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(rev, "skills", "writing-plans", "SKILL.md"), []byte("# wp\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(rev, "CLAUDE.md"), []byte("# reviewer\n"), 0o644))

	manifest := `name = "reviewer"
extends = "_base"

[config]
claude_md = "CLAUDE.md"
setting_sources = ["user", "project"]

[config.skills]
mode = "allowlist"
include = ["security-review"]

[config.mcp]
config = ""
strict = true

[enforcement]
permission_mode = "read-only"
tools.allow = ["Read", "Grep"]
tools.deny = ["Bash(git commit:*)"]

[metadata]
version = "1.2.0"
author = "tester"
`
	require.NoError(t, os.WriteFile(filepath.Join(rev, "persona.toml"), []byte(manifest), 0o644))
	return e
}

func TestActivate_HappyPath(t *testing.T) {
	e := seedReviewerEnv(t)

	res, err := Activate(e, "reviewer")
	require.NoError(t, err)
	require.True(t, res.Attestation.Clean)
	require.Equal(t, environment.CacheDir(e.Hash, "reviewer"), res.ConfigDir)

	// config dir materialized outside the workspace
	require.NotContains(t, res.ConfigDir, e.Workspace)
	require.FileExists(t, filepath.Join(res.ConfigDir, "settings.json"))
	require.FileExists(t, filepath.Join(res.ConfigDir, "skills", "security-review", "SKILL.md"))

	// withheld narrative includes the non-allowlisted build skill
	var withheld []string
	for _, line := range res.Attestation.Withheld {
		if line.Kind == "skill" {
			withheld = line.Names
		}
	}
	require.Contains(t, withheld, "writing-plans")

	// launch is built
	require.Equal(t, []string{"CLAUDE_CONFIG_DIR=" + res.ConfigDir}, res.Launch.Env)
	require.Equal(t, "claude", res.Launch.Argv[0])

	// active persona recorded
	e2, err := environment.Open(e.Workspace)
	require.NoError(t, err)
	require.NotNil(t, e2)
}

func TestActivate_DefaultsToLatestVersion(t *testing.T) {
	e := seedReviewerEnv(t)
	// "reviewer" with no :version must resolve and activate without error.
	_, err := Activate(e, "reviewer")
	require.NoError(t, err)
}

func TestActivate_NotFound(t *testing.T) {
	e := seedReviewerEnv(t)
	_, err := Activate(e, "ghost")
	require.Error(t, err)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/angeral/Repositories/agent-containers && go test ./internal/activate/ -run TestActivate -v`
Expected: FAIL — `undefined: Activate` / `undefined: ActivationResult`.

- [ ] **Step 3: Write minimal implementation**

`internal/activate/activate.go`:

```go
package activate

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/a2ngerer/agent-containers/internal/compose"
	"github.com/a2ngerer/agent-containers/internal/domain"
	"github.com/a2ngerer/agent-containers/internal/enforce"
	"github.com/a2ngerer/agent-containers/internal/environment"
	"github.com/a2ngerer/agent-containers/internal/materialize"
)

// ActivationResult is the outcome of Activate: where the env was materialized,
// the attestation proving its cleanliness, and the launch spec to run/print.
type ActivationResult struct {
	ConfigDir   string
	Attestation domain.Attestation
	Launch      LaunchSpec
}

// Activate composes a persona, locks the environment, materializes it into the
// cache config dir, verifies isolation (fail-closed), enriches the attestation,
// records the active persona, and builds the launch spec. personaRef is "name"
// or "name:version" (version currently informational; default "latest").
func Activate(e *environment.Environment, personaRef string) (ActivationResult, error) {
	name, _ := parsePersonaRef(personaRef)

	rm, err := compose.Compose(e, name)
	if err != nil {
		return ActivationResult{}, err
	}

	lock, err := Acquire(e, name)
	if err != nil {
		return ActivationResult{}, err
	}
	defer lock.Release()

	configDir := environment.CacheDir(e.Hash, name)
	if err := materialize.Materialize(e, rm, configDir); err != nil {
		return ActivationResult{}, fmt.Errorf("materialize %q: %w", name, err)
	}

	att, err := enforce.Verify(rm, configDir)
	if err != nil {
		if errors.Is(err, domain.ErrVerifyMismatch) {
			// fail closed: do not launch a compromised environment
			return ActivationResult{}, err
		}
		return ActivationResult{}, fmt.Errorf("verify %q: %w", name, err)
	}

	att.Withheld = withheldSkills(e, rm)

	if err := e.SetActive(name); err != nil {
		return ActivationResult{}, fmt.Errorf("set active persona: %w", err)
	}

	return ActivationResult{
		ConfigDir:   configDir,
		Attestation: att,
		Launch:      BuildLaunch(configDir, rm),
	}, nil
}

// parsePersonaRef splits "name" or "name:version" into its parts. The version
// defaults to "latest".
func parsePersonaRef(ref string) (name, version string) {
	if i := strings.LastIndex(ref, ":"); i >= 0 {
		return ref[:i], ref[i+1:]
	}
	return ref, "latest"
}

// withheldSkills returns the build narrative: skills physically present in the
// persona repo tree but excluded from the allowlist. This powers the
// "deliberately removed" line in the attestation.
func withheldSkills(e *environment.Environment, rm compose.ResolvedManifest) []domain.AttestationLine {
	skillsDir := filepath.Join(environment.RepoDir(e.Hash), "personas", rm.Persona.Name, "skills")
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		return nil
	}
	allowed := make(map[string]bool, len(rm.Skills))
	for _, s := range rm.Skills {
		allowed[s] = true
	}
	var withheld []string
	for _, ent := range entries {
		if ent.IsDir() && !allowed[ent.Name()] {
			withheld = append(withheld, ent.Name())
		}
	}
	if len(withheld) == 0 {
		return nil
	}
	sort.Strings(withheld)
	return []domain.AttestationLine{{Kind: "skill", Names: withheld}}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /Users/angeral/Repositories/agent-containers && go test ./internal/activate/ -v`
Expected: PASS — `TestBuildLaunch*`, `TestLock*`, `TestActivate_HappyPath`, `TestActivate_DefaultsToLatestVersion`, `TestActivate_NotFound`.

- [ ] **Step 5: Commit**

```bash
cd /Users/angeral/Repositories/agent-containers
git add internal/activate/activate.go internal/activate/activate_test.go
git commit -m "feat(activate): orchestrate compose->materialize->verify->launch fail-closed"
```

---

### Task 9: CLI `use` + `deactivate` + dispatch shim

**Files:**
- Create: `internal/cli/use.go`
- Test: `internal/cli/use_test.go`
- Modify: `internal/cli/root.go` (register commands), `cmd/acon/main.go` (apply dispatch)

`use.go` contributes the `use <persona>` command, the `deactivate` command, and the dispatch shim `DispatchArgs([]string) []string` that rewrites a non-reserved first token to `use <token>`. Reserved names come from `reservedSubcommands`. The pure functions `DispatchArgs`/`isReserved` are the only unit-tested part (no dependence on how the root command is constructed in M1/M2).

- [ ] **Step 1: Write the failing test**

`internal/cli/use_test.go`:

```go
package cli

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDispatchArgs(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{
			name: "reserved subcommand is left untouched",
			in:   []string{"snapshot", "reviewer", "-m", "msg"},
			want: []string{"snapshot", "reviewer", "-m", "msg"},
		},
		{
			name: "bare persona name is rewritten to use",
			in:   []string{"reviewer"},
			want: []string{"use", "reviewer"},
		},
		{
			name: "persona with version is rewritten to use",
			in:   []string{"reviewer:1.2.0"},
			want: []string{"use", "reviewer:1.2.0"},
		},
		{
			name: "no args is left untouched",
			in:   []string{},
			want: []string{},
		},
		{
			name: "leading flag is left untouched",
			in:   []string{"--help"},
			want: []string{"--help"},
		},
		{
			name: "init stays init",
			in:   []string{"init"},
			want: []string{"init"},
		},
		{
			name: "verify stays verify",
			in:   []string{"verify", "reviewer"},
			want: []string{"verify", "reviewer"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, DispatchArgs(tt.in))
		})
	}
}

func TestIsReserved(t *testing.T) {
	require.True(t, isReserved("snapshot"))
	require.True(t, isReserved("use"))
	require.True(t, isReserved("deactivate"))
	require.True(t, isReserved("verify"))
	require.False(t, isReserved("reviewer"))
	require.False(t, isReserved("coder"))
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/angeral/Repositories/agent-containers && go test ./internal/cli/ -run 'TestDispatchArgs|TestIsReserved' -v`
Expected: FAIL — `undefined: DispatchArgs` / `undefined: isReserved`.

- [ ] **Step 3: Write minimal implementation**

`internal/cli/use.go`:

```go
package cli

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"github.com/a2ngerer/agent-containers/internal/activate"
	"github.com/a2ngerer/agent-containers/internal/domain"
	"github.com/a2ngerer/agent-containers/internal/environment"
	"github.com/spf13/cobra"
)

// reservedSubcommands is the authoritative set of command names that must NOT be
// interpreted as a persona name by the dispatch shim. Keep in sync with the
// registered cobra commands (spec §9, §11).
var reservedSubcommands = map[string]bool{
	"init": true, "new": true, "use": true, "list": true, "ls": true,
	"status": true, "snapshot": true, "commit": true, "log": true,
	"diff": true, "rollback": true, "tag": true, "show": true,
	"verify": true, "edit": true, "rm": true, "deactivate": true,
	"push": true, "pull": true, "clone": true, "pull-persona": true,
	"help": true, "completion": true,
}

// isReserved reports whether name is a reserved subcommand.
func isReserved(name string) bool { return reservedSubcommands[name] }

// DispatchArgs implements the CLI dispatch rule: if the first argument is not a
// reserved subcommand and not a flag, it is treated as a persona name and the
// args are rewritten to "use <name>". All other inputs pass through unchanged.
func DispatchArgs(args []string) []string {
	if len(args) == 0 {
		return args
	}
	first := args[0]
	if strings.HasPrefix(first, "-") || isReserved(first) {
		return args
	}
	return append([]string{"use"}, args...)
}

// newUseCmd builds `acon use <persona>`: compose, materialize, attest, and
// either print the launch command (default, safe) or exec claude (--exec).
func newUseCmd() *cobra.Command {
	var execDirect bool
	cmd := &cobra.Command{
		Use:   "use <persona>",
		Short: "Activate a persona for this workspace",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ws, err := os.Getwd()
			if err != nil {
				return err
			}
			env, err := environment.Open(ws)
			if err != nil {
				return err
			}
			res, err := activate.Activate(env, args[0])
			if err != nil {
				return err
			}
			printAttestation(cmd, res.Attestation)
			if execDirect {
				return execLaunch(res.Launch)
			}
			printLaunchHint(cmd, res.Launch)
			return nil
		},
	}
	cmd.Flags().BoolVar(&execDirect, "exec", false, "exec claude directly instead of printing the command")
	return cmd
}

// newDeactivateCmd builds `acon deactivate`: clear the active persona.
func newDeactivateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "deactivate",
		Short: "Clear the active persona for this workspace",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ws, err := os.Getwd()
			if err != nil {
				return err
			}
			env, err := environment.Open(ws)
			if err != nil {
				return err
			}
			if err := env.SetActive(""); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "Active persona cleared.")
			return nil
		},
	}
}

// printAttestation renders the human-readable cleanliness certificate.
func printAttestation(cmd *cobra.Command, att domain.Attestation) {
	out := cmd.OutOrStdout()
	clean := "uncontaminated"
	if !att.Clean {
		clean = "UNVERIFIED"
	}
	fmt.Fprintf(out, "Persona: %s  (%s)   %s:%s\n", att.Persona, clean, att.Persona, att.Version)
	for _, line := range att.Included {
		fmt.Fprintf(out, "  %-9s %s\n", line.Kind+":", strings.Join(line.Names, ", "))
	}
	for _, line := range att.Withheld {
		fmt.Fprintf(out, "  Withheld  %s [%s]  (deliberately removed)\n", line.Kind, strings.Join(line.Names, ", "))
	}
	if len(att.Denied) > 0 {
		fmt.Fprintf(out, "  Denied:   %s\n", strings.Join(att.Denied, ", "))
	}
	if len(att.SettingSrc) > 0 {
		fmt.Fprintf(out, "  Settings: %s\n", strings.Join(att.SettingSrc, "+"))
	}
}

// printLaunchHint prints the env + command the user runs to start claude, plus
// the restart instruction.
func printLaunchHint(cmd *cobra.Command, spec activate.LaunchSpec) {
	out := cmd.OutOrStdout()
	fmt.Fprintln(out, "\nTo use this environment, start (or restart) Claude Code with:")
	fmt.Fprintf(out, "  %s %s\n", strings.Join(spec.Env, " "), strings.Join(spec.Argv, " "))
	fmt.Fprintln(out, "-> Start (or restart) Claude Code in this directory to use this environment.")
}

// execLaunch replaces the current process with claude (--exec path).
func execLaunch(spec activate.LaunchSpec) error {
	path, err := exec.LookPath("claude")
	if err != nil {
		return fmt.Errorf("claude not found on PATH: %w", err)
	}
	env := append(os.Environ(), spec.Env...)
	return syscall.Exec(path, spec.Argv, env)
}
```

Register the commands. In `internal/cli/root.go`, add to the root command construction (alongside the M1/M2 registrations):

```go
root.AddCommand(newUseCmd())
root.AddCommand(newDeactivateCmd())
```

Apply the dispatch shim. In `cmd/acon/main.go`, rewrite args before execution:

```go
package main

import (
	"fmt"
	"os"

	"github.com/a2ngerer/agent-containers/internal/cli"
)

func main() {
	root := cli.NewRootCmd()
	root.SetArgs(cli.DispatchArgs(os.Args[1:]))
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
```

> If M1/M2 expose `cli.Execute()` rather than `cli.NewRootCmd()`, route the rewrite inside `Execute()` instead: build the root command, then `root.SetArgs(DispatchArgs(os.Args[1:]))` before `root.Execute()`. The unit test below only exercises the pure `DispatchArgs`/`isReserved`, so it is independent of which entrypoint shape M1/M2 chose. Do not duplicate root-command construction — reuse the existing constructor.

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /Users/angeral/Repositories/agent-containers && go test ./internal/cli/ -run 'TestDispatchArgs|TestIsReserved' -v && go build ./...`
Expected: PASS for both tests; `go build ./...` exit 0.

- [ ] **Step 5: Commit**

```bash
cd /Users/angeral/Repositories/agent-containers
git add internal/cli/use.go internal/cli/use_test.go internal/cli/root.go cmd/acon/main.go
git commit -m "feat(cli): use/deactivate commands + persona dispatch shim"
```

---

### Task 10: CLI `verify <persona>` (non-zero on mismatch)

**Files:**
- Create: `internal/cli/verify.go`
- Test: `internal/cli/verify_test.go`
- Modify: `internal/cli/root.go` (register `verify`)

`verify <persona>` re-composes, re-materializes into the cache dir, runs `enforce.Verify`, and prints the attestation. On `domain.ErrVerifyMismatch` (or any error) it returns the error, so cobra's `RunE` produces a non-zero exit via `main`; the message carries the diff already embedded by `Verify`. The test drives the command object directly over a seeded workspace.

- [ ] **Step 1: Write the failing test**

`internal/cli/verify_test.go`:

```go
package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/a2ngerer/agent-containers/internal/environment"
	"github.com/stretchr/testify/require"
)

// seedReviewer creates a workspace + reviewer persona and chdirs into the
// workspace so the command's os.Getwd() resolves to it.
func seedReviewer(t *testing.T) *environment.Environment {
	t.Helper()
	home := t.TempDir()
	t.Setenv("ACON_HOME", home)
	ws := t.TempDir()
	e, err := environment.Create(ws)
	require.NoError(t, err)

	repo := environment.RepoDir(e.Hash)
	base := filepath.Join(repo, "personas", "_base")
	require.NoError(t, os.MkdirAll(base, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(base, "persona.toml"),
		[]byte("name = \"_base\"\n\n[config]\nclaude_md = \"CLAUDE.md\"\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(base, "CLAUDE.md"), []byte("# base\n"), 0o644))

	rev := filepath.Join(repo, "personas", "reviewer")
	require.NoError(t, os.MkdirAll(filepath.Join(rev, "skills", "security-review"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(rev, "skills", "security-review", "SKILL.md"), []byte("# sr\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(rev, "CLAUDE.md"), []byte("# reviewer\n"), 0o644))
	manifest := `name = "reviewer"
extends = "_base"

[config]
claude_md = "CLAUDE.md"
setting_sources = ["user", "project"]

[config.skills]
mode = "allowlist"
include = ["security-review"]

[config.mcp]
config = ""
strict = true

[enforcement]
permission_mode = "read-only"
tools.allow = ["Read", "Grep"]
tools.deny = ["Bash(git commit:*)"]

[metadata]
version = "1.2.0"
author = "tester"
`
	require.NoError(t, os.WriteFile(filepath.Join(rev, "persona.toml"), []byte(manifest), 0o644))

	wd, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Chdir(wd) })
	require.NoError(t, os.Chdir(ws))
	return e
}

func TestVerifyCmd_Clean(t *testing.T) {
	seedReviewer(t)

	cmd := newVerifyCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"reviewer"})

	require.NoError(t, cmd.Execute())
	require.Contains(t, out.String(), "reviewer")
	require.Contains(t, out.String(), "uncontaminated")
}

func TestVerifyCmd_UnknownPersonaErrors(t *testing.T) {
	seedReviewer(t)

	cmd := newVerifyCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	cmd.SetArgs([]string{"ghost"})

	err := cmd.Execute()
	require.Error(t, err)
}
```

> The smuggled-skill mismatch path is fully exercised one layer down in `enforce.Verify` (Task 5, `TestVerify_SmuggledSkillMismatch`). It cannot be reproduced through the `verify` command because `verify` re-materializes (clean-then-build) before checking, which removes any smuggled file. The deterministic CLI-level assertion is therefore the non-zero error on an unresolvable persona; the isolation-breach detection is verified where it actually runs.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/angeral/Repositories/agent-containers && go test ./internal/cli/ -run TestVerifyCmd -v`
Expected: FAIL — `undefined: newVerifyCmd`.

- [ ] **Step 3: Write minimal implementation**

`internal/cli/verify.go`:

```go
package cli

import (
	"fmt"
	"os"

	"github.com/a2ngerer/agent-containers/internal/compose"
	"github.com/a2ngerer/agent-containers/internal/enforce"
	"github.com/a2ngerer/agent-containers/internal/environment"
	"github.com/a2ngerer/agent-containers/internal/materialize"
	"github.com/spf13/cobra"
)

// newVerifyCmd builds `acon verify <persona>`: re-materialize the persona
// into its cache config dir and assert isolation. Exits non-zero (RunE error) on
// any mismatch; the error message carries the diff produced by enforce.Verify.
func newVerifyCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "verify <persona>",
		Short: "Re-check that a persona's materialized environment matches its manifest",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ws, err := os.Getwd()
			if err != nil {
				return err
			}
			env, err := environment.Open(ws)
			if err != nil {
				return err
			}
			rm, err := compose.Compose(env, args[0])
			if err != nil {
				return err
			}
			configDir := environment.CacheDir(env.Hash, args[0])
			if err := materialize.Materialize(env, rm, configDir); err != nil {
				return fmt.Errorf("materialize %q: %w", args[0], err)
			}
			att, err := enforce.Verify(rm, configDir)
			if err != nil {
				// non-zero exit with the embedded diff
				return err
			}
			printAttestation(cmd, att)
			fmt.Fprintln(cmd.OutOrStdout(), "\nVerified: materialized environment matches the manifest.")
			return nil
		},
	}
}
```

Register it in `internal/cli/root.go`:

```go
root.AddCommand(newVerifyCmd())
```

> If M1/M2 already registered a `verify` command stub, replace that registration with `newVerifyCmd()` (do not add a second `verify`). The reserved-subcommand list in `use.go` already includes `"verify"`, so dispatch is consistent either way.

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /Users/angeral/Repositories/agent-containers && go test ./internal/cli/ -run TestVerifyCmd -v`
Expected: PASS — `TestVerifyCmd_Clean`, `TestVerifyCmd_UnknownPersonaErrors`.

- [ ] **Step 5: Commit**

```bash
cd /Users/angeral/Repositories/agent-containers
git add internal/cli/verify.go internal/cli/verify_test.go internal/cli/root.go
git commit -m "feat(cli): verify command, non-zero exit on isolation mismatch"
```

---

### Task 11: Full-suite green + end-to-end smoke

**Files:**
- None created; this task validates the whole M3 surface together.

- [ ] **Step 1: Run the entire test suite**

Run: `cd /Users/angeral/Repositories/agent-containers && go test ./... -count=1`
Expected: `ok` for every package, including `internal/enforce`, `internal/materialize`, `internal/activate`, `internal/cli`. No failures.

- [ ] **Step 2: Vet the tree**

Run: `cd /Users/angeral/Repositories/agent-containers && go vet ./...`
Expected: no output, exit 0.

- [ ] **Step 3: Build the binary and smoke the dispatch + attestation**

Run:

```bash
cd /Users/angeral/Repositories/agent-containers
go build -o /tmp/acon ./cmd/acon
export ACON_HOME="$(mktemp -d)"
WS="$(mktemp -d)"
cd "$WS"
/tmp/acon init
/tmp/acon new reviewer --extends _base
```

> If `init`/`new` from M1/M2 are not yet wired into the binary in this branch, the unit + integration tests in Steps 1-2 already prove M3's behavior; skip the seeding and Steps 3-4. The smoke below assumes a reviewer persona exists with at least one allowlisted skill.

```bash
# Dispatch: bare persona name == use reviewer (prints attestation + launch hint,
# does NOT modify the workspace .claude/).
/tmp/acon reviewer
echo "--- workspace untouched check ---"
ls -la "$WS/.claude" 2>/dev/null || echo "no workspace .claude written by activation (expected)"
```

Expected: the `reviewer` invocation prints a `Persona: reviewer (uncontaminated) reviewer:<version>` attestation block followed by a `CLAUDE_CONFIG_DIR=...` launch command and the restart hint; the activation writes nothing into the workspace `.claude/`.

- [ ] **Step 4: Confirm the config dir is outside the workspace**

Run:

```bash
/tmp/acon reviewer | grep -o 'CLAUDE_CONFIG_DIR=[^ ]*'
```

Expected: a path under `$ACON_HOME/cache/<hash>/reviewer` — i.e. NOT under `$WS`. This is the materialized-isolation guarantee (spec §8, acceptance criterion 3).

- [ ] **Step 5: Commit (allow-empty marker for the validation task)**

```bash
cd /Users/angeral/Repositories/agent-containers
git add -A
git commit -m "test(m3): full-suite green + activation smoke" --allow-empty
```

---

## Self-Review (against spec §8/§9 and the architecture contract)

**Spec coverage:**
- §8 material withholding → Task 4 (`Materialize` copies only `rm.Skills`/`rm.Subagents`) + Task 5 (`Verify` rejects extras). Covered.
- §8 tool denial → Task 1 (`BuildPermissions` read-only base deny) + Task 2 (`settings.json`). Covered.
- §8 visible attestation + `verify` non-zero on mismatch → Task 5 (`Attestation`), Task 9 (`printAttestation`), Task 10 (`verify` CLI). Covered.
- §9 activation order (resolve → lock → materialize → verify → attest → launch) → Task 8 (`Activate`), fail-closed. Covered.
- §9 launch flags (CLAUDE_CONFIG_DIR + --setting-sources + --strict-mcp-config + --mcp-config + --allowedTools + --append-system-prompt) → Task 6 (`BuildLaunch`), MCP flags conditional. Covered.
- §9 dispatch rule (reserved vs. persona-arg) → Task 9 (`DispatchArgs`/`reservedSubcommands`). Covered.
- §9 edge case "activation never writes the workspace" → Task 4 writes only to `destDir` (CacheDir); Task 11 Step 3 asserts the workspace is untouched. Covered.
- §9 parallel sessions allowed, lock guards mutations only → Task 7 (`Lock` per-env, stale reclaim). Covered.
- Contract fail-closed (`activate` aborts on `ErrVerifyMismatch`) → Task 8. Covered.
- Contract idempotent materialization → Task 4 (`TestMaterialize_Idempotent`). Covered.

**Type consistency:** `Materialize`, `BuildPermissions`, `PermissionSet`, `Verify`, `Attestation`, `Activate`, `ActivationResult`, `LaunchSpec`, `BuildLaunch` match contract §5 signatures exactly. `compose.ResolvedManifest` fields (`Persona`, `Skills`, `Subagents`, `ClaudeMD`, `SettingSrc`, `Enforcement`, `MCP`) used as defined in contract §5. `domain` sentinels (`ErrVerifyMismatch`, `ErrLocked`) used as defined in contract §3. Paths (`RepoDir`, `CacheDir`, `EnvDir`) used per contract §5.

**Placeholder scan:** no "TBD/TODO/similar to Task N"; every code step is complete and compilable. The two integration seams to M1/M2 (entrypoint shape in Task 9; a possible pre-existing `verify` stub in Task 10) ship complete code plus an explicit reuse instruction — no dead helpers.

---

## Closing Summary

**Tasks:** 12 (Task 0 scaffold + Tasks 1–11). One commit per task. Tasks 1–10 follow the TDD 5-step cycle (write failing test → confirm fail → minimal implementation → confirm pass → commit); Tasks 0 and 11 are scaffold/validation.

**Packages (3 new + 1 extended):**
- `internal/enforce/` (new): `enforce.go`, `verify.go`.
- `internal/materialize/` (new): `materialize.go`, `settings.go`, `mcp.go`.
- `internal/activate/` (new): `activate.go`, `lock.go`, `launch.go`.
- `internal/cli/` (extended): `use.go` (+ `deactivate` + dispatch), `verify.go`; registrations in `root.go`; arg-rewrite in `cmd/acon/main.go`.

**Files:** 8 new source + 7 new test = 15 new files, plus edits to `internal/cli/root.go` and `cmd/acon/main.go`.

**Commands delivered:** `acon use <persona>` (plus bare `acon <persona>` via dispatch, `--exec` for direct `syscall.Exec`), `acon deactivate`, `acon verify <persona>`.

**Key tests:** `BuildPermissions` read-only base deny + dedup; `Materialize` allowlist-only + byte-identical idempotency; `Verify` clean / smuggled-build-skill ⇒ `ErrVerifyMismatch` / missing-deny ⇒ mismatch; `BuildLaunch` exact flags with and without MCP (`--mcp-config` omitted when no MCP); `Lock` free / own-reLock / foreign-live ⇒ `ErrLocked` / stale-reclaim / release; `Activate` happy path + `:latest` default + not-found; CLI `DispatchArgs` reserved-vs-persona routing; `verify` clean output + non-zero error path.

**New types beyond the architecture contract** (each defined in its owning package, none crossing a layering boundary, none shadowing a contract type):
1. `materialize.settingsFile` + `materialize.settingsPermissions` (private) — Go shape of the generated `settings.json`. The contract specifies the file's existence and its `permissions {allow,deny}` + `permissionMode` content (via `BuildPermissions`), but not the marshalling struct.
2. `materialize.mcpFile` (private) — minimal `{"mcpServers":{}}` shape for the defensive placeholder when `MCP.Config` names a file the copy did not provide.
3. `activate.Lock` (exported struct: `{path, state}`) + `activate.lockState` (private: `{persona, pid}` JSON lockfile body at `<EnvDir>/lock`). The contract names `Lock` and `Acquire`/`Release` but does not fix the struct fields or the lockfile format; defined here.
4. `cli.reservedSubcommands` (package var `map[string]bool`) + `cli.isReserved(string) bool` + `cli.DispatchArgs([]string) []string` — the dispatch machinery for the "non-reserved first arg ⇒ `use`" rule (spec §9). The contract describes the behavior but names no symbol; `DispatchArgs`/`isReserved` are exposed as pure, testable functions.

**Timestamps:** none introduced by M3 directly; the lockfile carries `{persona, pid}` only. Any history/attestation persistence that records time uses `time.RFC3339` per contract §6 (owned by M2's snapshot/attestation writers, not re-implemented here).
