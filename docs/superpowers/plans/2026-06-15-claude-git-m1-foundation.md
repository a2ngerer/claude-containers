# M1 Foundation — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (- [ ]) syntax for tracking.

**Goal:** Stand up the `claude_git` skeleton so that `claude_git init` binds a workspace end-to-end, seeds a `_base` persona from the existing `.claude/`, and `list`/`status` report it — backed by a git-backed storage engine that round-trips against a real temp repo.

**Architecture:** Three layers per the architecture contract: a pure `domain` package (types only, no I/O), a `storage` package hiding a go-git engine behind the `StorageEngine` interface, and an `environment` package that binds the tool to one workspace via path hashing and `env.toml`. The `cli` package is thin and delegates to `environment`/`probe`; `cmd/claude_git` wires the cobra root.

**Tech Stack:** Go 1.23, `spf13/cobra` + `spf13/viper`, `go-git/go-git/v5` (pure-Go git plumbing), `pelletier/go-toml/v2`, `stretchr/testify/require`.

**Depends on:** nichts (erstes Milestone).

---

## File Structure

| File | Responsibility |
|---|---|
| `go.mod` | Module `github.com/a2ngerer/claude-containers`, Go 1.23, pinned deps (cobra, viper, go-git/v5, go-toml/v2, testify). |
| `cmd/claude_git/main.go` | Entrypoint: build the cobra root via `cli.NewRootCmd()` and `Execute()` it; exit non-zero on error. |
| `internal/domain/persona.go` | `Persona`, `Config`, `SkillSet`, `SubagentSet`, `MCPConfig`, `Metadata`; `Persona.IsLayer()`; TOML load/save helpers with `[enforcement.tools]` allow/deny mapping. |
| `internal/domain/enforcement.go` | `Enforcement` (tools allow/deny carried in `-`-tagged fields). |
| `internal/domain/snapshot.go` | `SnapshotID`, `Snapshot`, `Tag`, `Timeline`. |
| `internal/domain/attestation.go` | `Attestation`, `AttestationLine`. |
| `internal/domain/errors.go` | Sentinel errors (`ErrPersonaNotFound`, `ErrNotInitialized`, ...). |
| `internal/domain/types_test.go` | Field/round-trip tests for snapshot, tag, timeline, enforcement, attestation, sentinels. |
| `internal/domain/persona_test.go` | Round-trip TOML load/save incl. `[enforcement.tools]` mapping and `IsLayer`. |
| `internal/storage/engine.go` | `StorageEngine` interface + `ObjectID`. |
| `internal/storage/git.go` | `GitStorageEngine` (go-git): `OpenGit` + all interface methods; git mapping per contract. |
| `internal/storage/engine_test.go` | Compile-time interface-shape check via an in-test fake. |
| `internal/storage/git_test.go` | Object/tree/snapshot/tag round-trips and remote add against a real temp repo. |
| `internal/environment/paths.go` | `WorkspaceHash`, `ToolHome`, `EnvDir`, `RepoDir`, `CacheDir`. |
| `internal/environment/paths_test.go` | Hash stability, `CLAUDE_GIT_HOME` override, path composition. |
| `internal/environment/environment.go` | `EnvConfig`, `Environment`, `Create`, `Open`, `ListPersonas`, `LoadPersona`, `SavePersona`, `SetActive`. |
| `internal/environment/environment_test.go` | `Create`/`Open` lifecycle, `ErrNotInitialized`, persona CRUD, `SetActive`. |
| `internal/probe/probe.go` | `IsClaudeTracked` via `git ls-files --error-unmatch .claude`. |
| `internal/probe/probe_test.go` | Tracked vs. untracked vs. non-repo workspace. |
| `internal/cli/root.go` | `NewRootCmd()`: cobra root + global flags; wires `init`/`list`/`status`. |
| `internal/cli/stubs.go` | Temporary command stubs so the root compiles before each command file lands (shrinks per task, deleted in Task 11). |
| `internal/cli/init.go` | `claude_git init`: bind workspace, create env dirs/repo/env.toml, import `.claude/` + `CLAUDE.md` as `_base`, write `.claude_git` marker, probe + warn. |
| `internal/cli/list.go` | `claude_git list`/`ls`: print personas with active marker + version. |
| `internal/cli/status.go` | `claude_git status`: print active persona + workspace + repo path. |
| `internal/cli/root_test.go` | Root command shape: `Use`, registered subcommands, `--help`. |
| `internal/cli/init_test.go` | End-to-end `init` over a temp workspace + temp `CLAUDE_GIT_HOME`. |
| `internal/cli/list_test.go` | `list` shows `_base`; errors when not initialized. |
| `internal/cli/status_test.go` | `status` shows workspace/hash/active; reflects `SetActive`; errors when not initialized. |

---

## Task 1: Module bootstrap and cobra entrypoint

**Files:**
- Create: `go.mod`
- Create: `cmd/claude_git/main.go`
- Create: `internal/cli/root.go`
- Create: `internal/cli/stubs.go`
- Test: `internal/cli/root_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/cli/root_test.go`:

```go
package cli

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewRootCmd_Use(t *testing.T) {
	cmd := NewRootCmd()
	require.Equal(t, "claude_git", cmd.Use)
}

func TestNewRootCmd_HasSubcommands(t *testing.T) {
	cmd := NewRootCmd()
	names := map[string]bool{}
	for _, c := range cmd.Commands() {
		names[c.Name()] = true
	}
	require.True(t, names["init"], "init subcommand must be registered")
	require.True(t, names["list"], "list subcommand must be registered")
	require.True(t, names["status"], "status subcommand must be registered")
}

func TestRootCmd_HelpDoesNotError(t *testing.T) {
	cmd := NewRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--help"})
	require.NoError(t, cmd.Execute())
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /Users/angeral/Repositories/claude_git && go test ./internal/cli/
```

Expected: compilation failure — `go.mod` does not exist yet, so the command errors with `go: cannot find main module` (non-zero exit). After `go.mod` exists but before `root.go`, the failure is `undefined: NewRootCmd`.

- [ ] **Step 3: Write minimal implementation**

Create `go.mod`:

```
module github.com/a2ngerer/claude-containers

go 1.23

require (
	github.com/go-git/go-git/v5 v5.12.0
	github.com/pelletier/go-toml/v2 v2.2.3
	github.com/spf13/cobra v1.8.1
	github.com/spf13/viper v1.19.0
	github.com/stretchr/testify v1.9.0
)
```

Create `internal/cli/root.go`:

```go
package cli

import (
	"github.com/spf13/cobra"
)

// NewRootCmd builds the cobra root command with all global flags and the
// M1 subcommands wired in. cmd/claude_git/main.go Execute()s the result.
func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "claude_git",
		Short:         "Version control and isolated, swappable environments for the Claude Code config layer",
		Long:          "claude_git treats CLAUDE.md plus the .claude/ directory as a versioned, swappable, shareable persona — \"Docker for Claude agent environments.\"",
		SilenceUsage:  true,
		SilenceErrors: false,
	}

	// Global flags shared by all subcommands. The workspace defaults to the
	// process working directory; tests and power users override it.
	root.PersistentFlags().String("workspace", "", "workspace path (default: current directory)")

	root.AddCommand(newInitCmd())
	root.AddCommand(newListCmd())
	root.AddCommand(newStatusCmd())

	return root
}
```

Create `cmd/claude_git/main.go`:

```go
package main

import (
	"os"

	"github.com/a2ngerer/claude-containers/internal/cli"
)

func main() {
	if err := cli.NewRootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}
```

The `cli` package references `newInitCmd`/`newListCmd`/`newStatusCmd`, which do not exist yet. Add minimal stubs so the package compiles; Tasks 9–11 replace each body with a real file. Create `internal/cli/stubs.go`:

```go
package cli

import "github.com/spf13/cobra"

// Temporary stubs so the root command compiles in Task 1. Tasks 9-11 replace
// these (init.go, list.go, status.go) with the real implementations and shrink
// this file until it is deleted.
func newInitCmd() *cobra.Command {
	return &cobra.Command{Use: "init", Short: "Bind the current workspace", RunE: func(*cobra.Command, []string) error { return nil }}
}

func newListCmd() *cobra.Command {
	return &cobra.Command{Use: "list", Aliases: []string{"ls"}, Short: "List personas", RunE: func(*cobra.Command, []string) error { return nil }}
}

func newStatusCmd() *cobra.Command {
	return &cobra.Command{Use: "status", Short: "Show active persona and workspace", RunE: func(*cobra.Command, []string) error { return nil }}
}
```

Then fetch and pin dependencies:

```bash
cd /Users/angeral/Repositories/claude_git && go mod tidy
```

- [ ] **Step 4: Run test to verify it passes**

```bash
cd /Users/angeral/Repositories/claude_git && go build ./... && go test ./internal/cli/
```

Expected: PASS (`ok  github.com/a2ngerer/claude-containers/internal/cli`).

- [ ] **Step 5: Commit**

```bash
cd /Users/angeral/Repositories/claude_git && git init -q 2>/dev/null; git add go.mod go.sum cmd/claude_git/main.go internal/cli/root.go internal/cli/stubs.go internal/cli/root_test.go && git commit -m "M1: module bootstrap, cobra root, entrypoint"
```

---

## Task 2: Domain types — enforcement, snapshot, attestation, errors

**Files:**
- Create: `internal/domain/enforcement.go`
- Create: `internal/domain/snapshot.go`
- Create: `internal/domain/attestation.go`
- Create: `internal/domain/errors.go`
- Test: `internal/domain/types_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/domain/types_test.go`:

```go
package domain

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSnapshotFields(t *testing.T) {
	ts := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	s := Snapshot{
		ID:        SnapshotID("abc"),
		Persona:   "reviewer",
		Parents:   []SnapshotID{"parent1"},
		Message:   "init",
		Author:    "alexander.angerer",
		Timestamp: ts,
		TreeID:    "tree123",
	}
	require.Equal(t, SnapshotID("abc"), s.ID)
	require.Equal(t, "reviewer", s.Persona)
	require.Equal(t, []SnapshotID{"parent1"}, s.Parents)
	require.Equal(t, ts, s.Timestamp)
	require.Equal(t, "tree123", s.TreeID)
}

func TestTagAndTimeline(t *testing.T) {
	tag := Tag{Persona: "coder", Version: "1.2.0", Target: SnapshotID("c1")}
	require.Equal(t, "1.2.0", tag.Version)

	tl := Timeline{Persona: "coder", Snapshots: []SnapshotID{"c2", "c1"}}
	require.Equal(t, "coder", tl.Persona)
	require.Len(t, tl.Snapshots, 2)
	require.Equal(t, SnapshotID("c2"), tl.Snapshots[0]) // newest first
}

func TestEnforcementFields(t *testing.T) {
	e := Enforcement{
		PermissionMode: "read-only",
		ToolsAllow:     []string{"Read", "Grep"},
		ToolsDeny:      []string{"Write", "Edit"},
	}
	require.Equal(t, "read-only", e.PermissionMode)
	require.Equal(t, []string{"Read", "Grep"}, e.ToolsAllow)
	require.Equal(t, []string{"Write", "Edit"}, e.ToolsDeny)
}

func TestAttestation(t *testing.T) {
	a := Attestation{
		Persona:    "reviewer",
		Version:    "1.2.0",
		Included:   []AttestationLine{{Kind: "skill", Names: []string{"security-review"}}},
		Withheld:   []AttestationLine{{Kind: "skill", Names: []string{"superpowers"}}},
		Denied:     []string{"Write", "Edit"},
		SettingSrc: []string{"user", "project"},
		Clean:      true,
	}
	require.True(t, a.Clean)
	require.Equal(t, "skill", a.Included[0].Kind)
	require.Equal(t, []string{"security-review"}, a.Included[0].Names)
	require.Equal(t, []string{"Write", "Edit"}, a.Denied)
}

func TestSentinelErrors(t *testing.T) {
	require.True(t, errors.Is(ErrPersonaNotFound, ErrPersonaNotFound))
	require.NotEqual(t, ErrPersonaNotFound.Error(), ErrNotInitialized.Error())
	require.NotEqual(t, ErrLocked.Error(), ErrVerifyMismatch.Error())
	require.Contains(t, ErrNotInitialized.Error(), "claude_git init")
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /Users/angeral/Repositories/claude_git && go test ./internal/domain/
```

Expected: compilation failure — `undefined: Snapshot`, `undefined: Enforcement`, `undefined: Attestation`, `undefined: ErrPersonaNotFound`, etc. Non-zero exit.

- [ ] **Step 3: Write minimal implementation**

Create `internal/domain/enforcement.go`:

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

Create `internal/domain/snapshot.go`:

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

Create `internal/domain/attestation.go`:

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

Create `internal/domain/errors.go`:

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

- [ ] **Step 4: Run test to verify it passes**

```bash
cd /Users/angeral/Repositories/claude_git && go test ./internal/domain/
```

Expected: PASS (`ok  github.com/a2ngerer/claude-containers/internal/domain`).

- [ ] **Step 5: Commit**

```bash
cd /Users/angeral/Repositories/claude_git && git add internal/domain/enforcement.go internal/domain/snapshot.go internal/domain/attestation.go internal/domain/errors.go internal/domain/types_test.go && git commit -m "M1: domain types (enforcement, snapshot, tag, timeline, attestation, errors)"
```

---

## Task 3: Persona type + TOML load/save with `[enforcement.tools]` mapping

**Files:**
- Create: `internal/domain/persona.go`
- Test: `internal/domain/persona_test.go`

The contract gives `Persona`/`Config`/`SkillSet`/`SubagentSet`/`MCPConfig`/`Metadata` exact `toml` tags, but `Enforcement.ToolsAllow`/`ToolsDeny` carry `toml:"-"` because `tools.allow`/`tools.deny` sit under `[enforcement.tools]`, which has no direct struct tag. The load/save helpers bridge this with an on-disk-only intermediate struct.

- [ ] **Step 1: Write the failing test**

Create `internal/domain/persona_test.go`:

```go
package domain

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

const reviewerTOML = `name = "reviewer"
description = "Uncontaminated reviewer."
extends = "_base"

[config]
claude_md = "CLAUDE.md"
setting_sources = ["user", "project"]

[config.skills]
mode = "allowlist"
include = ["security-review", "silent-failure-hunter"]

[config.subagents]
include = ["code-reviewer", "security-reviewer"]

[config.mcp]
config = "mcp.json"
strict = true
requires = ["github"]

[enforcement]
permission_mode = "read-only"

[enforcement.tools]
allow = ["Read", "Grep", "Bash(git diff:*)"]
deny = ["Write", "Edit", "Bash(git commit:*)"]

[metadata]
version = "1.2.0"
author = "alexander.angerer"
`

func TestIsLayer(t *testing.T) {
	require.True(t, Persona{Name: "_base"}.IsLayer())
	require.False(t, Persona{Name: "reviewer"}.IsLayer())
	require.False(t, Persona{Name: ""}.IsLayer())
}

func TestLoadPersonaTOML_MapsEnforcementTools(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "persona.toml")
	require.NoError(t, os.WriteFile(path, []byte(reviewerTOML), 0o644))

	p, err := LoadPersonaTOML(path)
	require.NoError(t, err)

	require.Equal(t, "reviewer", p.Name)
	require.Equal(t, "_base", p.Extends)
	require.Equal(t, "CLAUDE.md", p.Config.ClaudeMD)
	require.Equal(t, []string{"user", "project"}, p.Config.SettingSources)
	require.Equal(t, "allowlist", p.Config.Skills.Mode)
	require.Equal(t, []string{"security-review", "silent-failure-hunter"}, p.Config.Skills.Include)
	require.Equal(t, []string{"code-reviewer", "security-reviewer"}, p.Config.Subagents.Include)
	require.Equal(t, "mcp.json", p.Config.MCP.Config)
	require.True(t, p.Config.MCP.Strict)
	require.Equal(t, []string{"github"}, p.Config.MCP.Requires)
	require.Equal(t, "read-only", p.Enforcement.PermissionMode)
	// the critical mapping: [enforcement.tools] allow/deny -> ToolsAllow/ToolsDeny
	require.Equal(t, []string{"Read", "Grep", "Bash(git diff:*)"}, p.Enforcement.ToolsAllow)
	require.Equal(t, []string{"Write", "Edit", "Bash(git commit:*)"}, p.Enforcement.ToolsDeny)
	require.Equal(t, "1.2.0", p.Metadata.Version)
	require.Equal(t, "alexander.angerer", p.Metadata.Author)
}

func TestSavePersonaTOML_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "persona.toml")
	require.NoError(t, os.WriteFile(path, []byte(reviewerTOML), 0o644))

	orig, err := LoadPersonaTOML(path)
	require.NoError(t, err)

	out := filepath.Join(dir, "out.toml")
	require.NoError(t, SavePersonaTOML(orig, out))

	again, err := LoadPersonaTOML(out)
	require.NoError(t, err)

	require.Equal(t, orig, again)
	require.Equal(t, []string{"Read", "Grep", "Bash(git diff:*)"}, again.Enforcement.ToolsAllow)
	require.Equal(t, []string{"Write", "Edit", "Bash(git commit:*)"}, again.Enforcement.ToolsDeny)
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /Users/angeral/Repositories/claude_git && go test ./internal/domain/ -run 'Persona|IsLayer'
```

Expected: compilation failure — `undefined: Persona`, `undefined: LoadPersonaTOML`, `undefined: SavePersonaTOML`. Non-zero exit.

- [ ] **Step 3: Write minimal implementation**

Create `internal/domain/persona.go`:

```go
// internal/domain/persona.go
package domain

import (
	"fmt"
	"os"

	toml "github.com/pelletier/go-toml/v2"
)

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

// personaWire is the on-disk shape. It mirrors Persona but replaces the
// Enforcement block with one that exposes [enforcement.tools] allow/deny, which
// Persona keeps in toml:"-" fields. Used only inside Load/SavePersonaTOML.
type personaWire struct {
	Name        string          `toml:"name"`
	Description string          `toml:"description"`
	Extends     string          `toml:"extends"`
	Config      Config          `toml:"config"`
	Enforcement enforcementWire `toml:"enforcement"`
	Metadata    Metadata        `toml:"metadata"`
}

type enforcementWire struct {
	PermissionMode string    `toml:"permission_mode"`
	Tools          toolsWire `toml:"tools"`
}

type toolsWire struct {
	Allow []string `toml:"allow"`
	Deny  []string `toml:"deny"`
}

// LoadPersonaTOML reads a persona.toml and maps [enforcement.tools] allow/deny
// into Enforcement.ToolsAllow/ToolsDeny.
func LoadPersonaTOML(path string) (Persona, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Persona{}, fmt.Errorf("read persona toml %q: %w", path, err)
	}
	var w personaWire
	if err := toml.Unmarshal(raw, &w); err != nil {
		return Persona{}, fmt.Errorf("unmarshal persona toml %q: %w", path, err)
	}
	return Persona{
		Name:        w.Name,
		Description: w.Description,
		Extends:     w.Extends,
		Config:      w.Config,
		Enforcement: Enforcement{
			PermissionMode: w.Enforcement.PermissionMode,
			ToolsAllow:     w.Enforcement.Tools.Allow,
			ToolsDeny:      w.Enforcement.Tools.Deny,
		},
		Metadata: w.Metadata,
	}, nil
}

// SavePersonaTOML writes a persona.toml, projecting Enforcement.ToolsAllow/ToolsDeny
// back under [enforcement.tools].
func SavePersonaTOML(p Persona, path string) error {
	w := personaWire{
		Name:        p.Name,
		Description: p.Description,
		Extends:     p.Extends,
		Config:      p.Config,
		Enforcement: enforcementWire{
			PermissionMode: p.Enforcement.PermissionMode,
			Tools: toolsWire{
				Allow: p.Enforcement.ToolsAllow,
				Deny:  p.Enforcement.ToolsDeny,
			},
		},
		Metadata: p.Metadata,
	}
	out, err := toml.Marshal(w)
	if err != nil {
		return fmt.Errorf("marshal persona toml: %w", err)
	}
	if err := os.WriteFile(path, out, 0o644); err != nil {
		return fmt.Errorf("write persona toml %q: %w", path, err)
	}
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
cd /Users/angeral/Repositories/claude_git && go test ./internal/domain/
```

Expected: PASS (`ok  github.com/a2ngerer/claude-containers/internal/domain`) — all domain tests, including the round-trip.

- [ ] **Step 5: Commit**

```bash
cd /Users/angeral/Repositories/claude_git && git add internal/domain/persona.go internal/domain/persona_test.go && git commit -m "M1: Persona type + TOML load/save with [enforcement.tools] mapping"
```

---

## Task 4: StorageEngine interface + ObjectID

**Files:**
- Create: `internal/storage/engine.go`
- Test: `internal/storage/engine_test.go`

This task pins the interface and `ObjectID` only; the concrete `GitStorageEngine` lands in Task 5. The test asserts the interface shape via a compile-time assignment of a minimal in-test fake — guaranteeing the signatures match the contract exactly before the real impl is written.

- [ ] **Step 1: Write the failing test**

Create `internal/storage/engine_test.go`:

```go
package storage

import (
	"testing"

	"github.com/a2ngerer/claude-containers/internal/domain"
	"github.com/stretchr/testify/require"
)

// fakeEngine exists only to prove the interface signatures compile exactly as
// the contract specifies. If any method signature drifts, this file fails to build.
type fakeEngine struct{}

func (fakeEngine) PutObject(content []byte) (ObjectID, error)     { return "", nil }
func (fakeEngine) GetObject(id ObjectID) ([]byte, error)          { return nil, nil }
func (fakeEngine) WriteTree(dir string) (ObjectID, error)         { return "", nil }
func (fakeEngine) CheckoutTree(id ObjectID, destDir string) error { return nil }
func (fakeEngine) WriteSnapshot(s domain.Snapshot) (domain.SnapshotID, error) {
	return "", nil
}
func (fakeEngine) ReadSnapshot(id domain.SnapshotID) (domain.Snapshot, error) {
	return domain.Snapshot{}, nil
}
func (fakeEngine) Timeline(persona string) ([]domain.SnapshotID, error) { return nil, nil }
func (fakeEngine) SetTag(persona, version string, id domain.SnapshotID) error {
	return nil
}
func (fakeEngine) ResolveTag(persona, version string) (domain.SnapshotID, error) {
	return "", nil
}
func (fakeEngine) ListTags(persona string) ([]domain.Tag, error) { return nil, nil }
func (fakeEngine) AddRemote(name, url string) error              { return nil }
func (fakeEngine) Push(remote string) error                      { return nil }
func (fakeEngine) Pull(remote string) error                      { return nil }

func TestStorageEngineInterfaceSatisfied(t *testing.T) {
	var e StorageEngine = fakeEngine{}
	require.NotNil(t, e)
}

func TestObjectIDIsString(t *testing.T) {
	id := ObjectID("deadbeef")
	require.Equal(t, "deadbeef", string(id))
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /Users/angeral/Repositories/claude_git && go test ./internal/storage/
```

Expected: compilation failure — `undefined: StorageEngine`, `undefined: ObjectID`. Non-zero exit.

- [ ] **Step 3: Write minimal implementation**

Create `internal/storage/engine.go`:

```go
// internal/storage/engine.go
package storage

import "github.com/a2ngerer/claude-containers/internal/domain"

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
```

- [ ] **Step 4: Run test to verify it passes**

```bash
cd /Users/angeral/Repositories/claude_git && go test ./internal/storage/
```

Expected: PASS (`ok  github.com/a2ngerer/claude-containers/internal/storage`).

- [ ] **Step 5: Commit**

```bash
cd /Users/angeral/Repositories/claude_git && git add internal/storage/engine.go internal/storage/engine_test.go && git commit -m "M1: StorageEngine interface + ObjectID"
```

---

## Task 5: GitStorageEngine (go-git) — objects, trees, snapshots, tags, remotes

**Files:**
- Create: `internal/storage/git.go`
- Test: `internal/storage/git_test.go`

Git mapping per contract: persona content tree → git tree; `WriteSnapshot` → git commit under `refs/personas/<name>`; `SetTag` → `refs/tags/<persona>/<version>`; remotes → git remotes. Users never see refs. The engine is built on a bare repository so there is no worktree to manage; trees and blobs are written directly through the object storer, and `CheckoutTree` walks a tree object back onto disk.

- [ ] **Step 1: Write the failing test**

Create `internal/storage/git_test.go`:

```go
package storage

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/a2ngerer/claude-containers/internal/domain"
	"github.com/stretchr/testify/require"
)

func newTestEngine(t *testing.T) StorageEngine {
	t.Helper()
	repoDir := filepath.Join(t.TempDir(), "repo")
	e, err := OpenGit(repoDir)
	require.NoError(t, err)
	return e
}

func TestOpenGit_IsIdempotent(t *testing.T) {
	repoDir := filepath.Join(t.TempDir(), "repo")
	_, err := OpenGit(repoDir)
	require.NoError(t, err)
	_, err = OpenGit(repoDir) // second open must not fail
	require.NoError(t, err)
}

func TestPutGetObject_RoundTrip(t *testing.T) {
	e := newTestEngine(t)
	content := []byte("hello persona world")
	id, err := e.PutObject(content)
	require.NoError(t, err)
	require.NotEmpty(t, string(id))

	got, err := e.GetObject(id)
	require.NoError(t, err)
	require.Equal(t, content, got)
}

func TestPutObject_ContentAddressed(t *testing.T) {
	e := newTestEngine(t)
	a, err := e.PutObject([]byte("same"))
	require.NoError(t, err)
	b, err := e.PutObject([]byte("same"))
	require.NoError(t, err)
	require.Equal(t, a, b, "identical content must yield identical id")
}

func TestWriteCheckoutTree_RoundTrip(t *testing.T) {
	e := newTestEngine(t)

	src := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(src, "persona.toml"), []byte("name = \"x\"\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(src, "CLAUDE.md"), []byte("# instructions\n"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(src, "skills", "review"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(src, "skills", "review", "SKILL.md"), []byte("skill body\n"), 0o644))

	treeID, err := e.WriteTree(src)
	require.NoError(t, err)
	require.NotEmpty(t, string(treeID))

	dest := filepath.Join(t.TempDir(), "out")
	require.NoError(t, e.CheckoutTree(treeID, dest))

	for _, rel := range []string{"persona.toml", "CLAUDE.md", filepath.Join("skills", "review", "SKILL.md")} {
		want, err := os.ReadFile(filepath.Join(src, rel))
		require.NoError(t, err)
		got, err := os.ReadFile(filepath.Join(dest, rel))
		require.NoError(t, err, "missing checked-out file %s", rel)
		require.Equal(t, want, got, "content mismatch for %s", rel)
	}
}

func TestWriteReadSnapshot_RoundTrip(t *testing.T) {
	e := newTestEngine(t)

	src := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(src, "persona.toml"), []byte("name = \"coder\"\n"), 0o644))
	treeID, err := e.WriteTree(src)
	require.NoError(t, err)

	when := time.Date(2026, 6, 15, 10, 30, 0, 0, time.UTC)
	snap := domain.Snapshot{
		Persona:   "coder",
		Message:   "initial coder snapshot",
		Author:    "alexander.angerer",
		Timestamp: when,
		TreeID:    string(treeID),
	}
	id, err := e.WriteSnapshot(snap)
	require.NoError(t, err)
	require.NotEmpty(t, string(id))

	got, err := e.ReadSnapshot(id)
	require.NoError(t, err)
	require.Equal(t, "coder", got.Persona)
	require.Equal(t, "initial coder snapshot", got.Message)
	require.Equal(t, "alexander.angerer", got.Author)
	require.Equal(t, when.UTC(), got.Timestamp.UTC())
	require.Equal(t, string(treeID), got.TreeID)
	require.Equal(t, id, got.ID)
}

func TestTimeline_NewestFirst(t *testing.T) {
	e := newTestEngine(t)
	src := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(src, "persona.toml"), []byte("name = \"coder\"\n"), 0o644))
	treeID, err := e.WriteTree(src)
	require.NoError(t, err)

	first, err := e.WriteSnapshot(domain.Snapshot{Persona: "coder", Message: "first", Author: "a", Timestamp: time.Now().UTC(), TreeID: string(treeID)})
	require.NoError(t, err)
	second, err := e.WriteSnapshot(domain.Snapshot{Persona: "coder", Message: "second", Parents: []domain.SnapshotID{first}, Author: "a", Timestamp: time.Now().UTC().Add(time.Second), TreeID: string(treeID)})
	require.NoError(t, err)

	tl, err := e.Timeline("coder")
	require.NoError(t, err)
	require.Len(t, tl, 2)
	require.Equal(t, second, tl[0], "newest snapshot first")
	require.Equal(t, first, tl[1])
}

func TestTags_SetResolveList(t *testing.T) {
	e := newTestEngine(t)
	src := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(src, "persona.toml"), []byte("name = \"reviewer\"\n"), 0o644))
	treeID, err := e.WriteTree(src)
	require.NoError(t, err)
	snapID, err := e.WriteSnapshot(domain.Snapshot{Persona: "reviewer", Message: "m", Author: "a", Timestamp: time.Now().UTC(), TreeID: string(treeID)})
	require.NoError(t, err)

	require.NoError(t, e.SetTag("reviewer", "1.2.0", snapID))
	require.NoError(t, e.SetTag("reviewer", "latest", snapID))

	resolved, err := e.ResolveTag("reviewer", "1.2.0")
	require.NoError(t, err)
	require.Equal(t, snapID, resolved)

	tags, err := e.ListTags("reviewer")
	require.NoError(t, err)
	versions := make([]string, 0, len(tags))
	for _, tg := range tags {
		require.Equal(t, "reviewer", tg.Persona)
		require.Equal(t, snapID, tg.Target)
		versions = append(versions, tg.Version)
	}
	sort.Strings(versions)
	require.Equal(t, []string{"1.2.0", "latest"}, versions)
}

func TestAddRemote_Persists(t *testing.T) {
	e := newTestEngine(t)
	require.NoError(t, e.AddRemote("origin", "https://example.com/personas.git"))
	// adding the same remote name again must error (go-git ErrRemoteExists)
	require.Error(t, e.AddRemote("origin", "https://example.com/personas.git"))
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /Users/angeral/Repositories/claude_git && go test ./internal/storage/ -run Git
```

Expected: compilation failure — `undefined: OpenGit`. Non-zero exit.

- [ ] **Step 3: Write minimal implementation**

Create `internal/storage/git.go`:

```go
// internal/storage/git.go
package storage

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/a2ngerer/claude-containers/internal/domain"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/storer"
)

const (
	personaRefPrefix = "refs/personas/"
	tagRefPrefix     = "refs/tags/"
)

// GitStorageEngine implements StorageEngine over a bare go-git repository.
// Blobs/trees/commits map onto git objects; persona timelines onto
// refs/personas/<name>; version tags onto refs/tags/<persona>/<version>.
type GitStorageEngine struct {
	repo *git.Repository
}

// OpenGit opens or initializes the git-backed engine rooted at repoDir.
func OpenGit(repoDir string) (StorageEngine, error) {
	repo, err := git.PlainOpen(repoDir)
	if err == nil {
		return &GitStorageEngine{repo: repo}, nil
	}
	if err != git.ErrRepositoryNotExists {
		return nil, fmt.Errorf("open git repo %q: %w", repoDir, err)
	}
	if mkErr := os.MkdirAll(repoDir, 0o755); mkErr != nil {
		return nil, fmt.Errorf("create repo dir %q: %w", repoDir, mkErr)
	}
	repo, err = git.PlainInit(repoDir, true) // bare: no worktree
	if err != nil {
		return nil, fmt.Errorf("init git repo %q: %w", repoDir, err)
	}
	return &GitStorageEngine{repo: repo}, nil
}

func (g *GitStorageEngine) PutObject(content []byte) (ObjectID, error) {
	obj := g.repo.Storer.NewEncodedObject()
	obj.SetType(plumbing.BlobObject)
	w, err := obj.Writer()
	if err != nil {
		return "", fmt.Errorf("blob writer: %w", err)
	}
	if _, err := w.Write(content); err != nil {
		_ = w.Close()
		return "", fmt.Errorf("write blob: %w", err)
	}
	if err := w.Close(); err != nil {
		return "", fmt.Errorf("close blob writer: %w", err)
	}
	h, err := g.repo.Storer.SetEncodedObject(obj)
	if err != nil {
		return "", fmt.Errorf("store blob: %w", err)
	}
	return ObjectID(h.String()), nil
}

func (g *GitStorageEngine) GetObject(id ObjectID) ([]byte, error) {
	h := plumbing.NewHash(string(id))
	blob, err := g.repo.BlobObject(h)
	if err != nil {
		return nil, fmt.Errorf("read blob %s: %w", id, err)
	}
	r, err := blob.Reader()
	if err != nil {
		return nil, fmt.Errorf("blob reader %s: %w", id, err)
	}
	defer r.Close()
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("read blob bytes %s: %w", id, err)
	}
	return data, nil
}

// WriteTree snapshots a persona directory recursively into a git tree and
// returns its object id. Subdirectories become nested tree objects.
func (g *GitStorageEngine) WriteTree(dir string) (ObjectID, error) {
	h, err := g.writeTreeRec(dir)
	if err != nil {
		return "", err
	}
	return ObjectID(h.String()), nil
}

func (g *GitStorageEngine) writeTreeRec(dir string) (plumbing.Hash, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return plumbing.ZeroHash, fmt.Errorf("read dir %q: %w", dir, err)
	}
	var treeEntries []object.TreeEntry
	for _, de := range entries {
		full := filepath.Join(dir, de.Name())
		if de.IsDir() {
			sub, err := g.writeTreeRec(full)
			if err != nil {
				return plumbing.ZeroHash, err
			}
			treeEntries = append(treeEntries, object.TreeEntry{Name: de.Name(), Mode: filemode.Dir, Hash: sub})
			continue
		}
		content, err := os.ReadFile(full)
		if err != nil {
			return plumbing.ZeroHash, fmt.Errorf("read file %q: %w", full, err)
		}
		id, err := g.PutObject(content)
		if err != nil {
			return plumbing.ZeroHash, err
		}
		treeEntries = append(treeEntries, object.TreeEntry{Name: de.Name(), Mode: filemode.Regular, Hash: plumbing.NewHash(string(id))})
	}
	// git requires tree entries sorted by name for a canonical, content-addressed hash.
	sort.Slice(treeEntries, func(i, j int) bool { return treeEntries[i].Name < treeEntries[j].Name })

	tree := &object.Tree{Entries: treeEntries}
	enc := g.repo.Storer.NewEncodedObject()
	if err := tree.Encode(enc); err != nil {
		return plumbing.ZeroHash, fmt.Errorf("encode tree: %w", err)
	}
	h, err := g.repo.Storer.SetEncodedObject(enc)
	if err != nil {
		return plumbing.ZeroHash, fmt.Errorf("store tree: %w", err)
	}
	return h, nil
}

// CheckoutTree walks a stored tree object back onto destDir.
func (g *GitStorageEngine) CheckoutTree(id ObjectID, destDir string) error {
	tree, err := g.repo.TreeObject(plumbing.NewHash(string(id)))
	if err != nil {
		return fmt.Errorf("read tree %s: %w", id, err)
	}
	return g.checkoutTreeRec(tree, destDir)
}

func (g *GitStorageEngine) checkoutTreeRec(tree *object.Tree, destDir string) error {
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("mkdir %q: %w", destDir, err)
	}
	for _, entry := range tree.Entries {
		target := filepath.Join(destDir, entry.Name)
		if entry.Mode == filemode.Dir {
			sub, err := g.repo.TreeObject(entry.Hash)
			if err != nil {
				return fmt.Errorf("read subtree %s: %w", entry.Hash, err)
			}
			if err := g.checkoutTreeRec(sub, target); err != nil {
				return err
			}
			continue
		}
		blob, err := g.repo.BlobObject(entry.Hash)
		if err != nil {
			return fmt.Errorf("read blob %s: %w", entry.Hash, err)
		}
		r, err := blob.Reader()
		if err != nil {
			return fmt.Errorf("blob reader %s: %w", entry.Hash, err)
		}
		data, err := io.ReadAll(r)
		_ = r.Close()
		if err != nil {
			return fmt.Errorf("read blob bytes %s: %w", entry.Hash, err)
		}
		if err := os.WriteFile(target, data, 0o644); err != nil {
			return fmt.Errorf("write file %q: %w", target, err)
		}
	}
	return nil
}

// WriteSnapshot creates a git commit whose tree is s.TreeID, advancing
// refs/personas/<persona> to it. Parents become commit parents.
func (g *GitStorageEngine) WriteSnapshot(s domain.Snapshot) (domain.SnapshotID, error) {
	sig := object.Signature{Name: s.Author, Email: authorEmail(s.Author), When: s.Timestamp}
	commit := &object.Commit{
		Author:    sig,
		Committer: sig,
		Message:   s.Message,
		TreeHash:  plumbing.NewHash(s.TreeID),
	}
	for _, p := range s.Parents {
		commit.ParentHashes = append(commit.ParentHashes, plumbing.NewHash(string(p)))
	}
	enc := g.repo.Storer.NewEncodedObject()
	if err := commit.Encode(enc); err != nil {
		return "", fmt.Errorf("encode commit: %w", err)
	}
	h, err := g.repo.Storer.SetEncodedObject(enc)
	if err != nil {
		return "", fmt.Errorf("store commit: %w", err)
	}
	ref := plumbing.NewHashReference(plumbing.ReferenceName(personaRefPrefix+s.Persona), h)
	if err := g.repo.Storer.SetReference(ref); err != nil {
		return "", fmt.Errorf("set persona ref %s: %w", s.Persona, err)
	}
	return domain.SnapshotID(h.String()), nil
}

func (g *GitStorageEngine) ReadSnapshot(id domain.SnapshotID) (domain.Snapshot, error) {
	commit, err := g.repo.CommitObject(plumbing.NewHash(string(id)))
	if err != nil {
		return domain.Snapshot{}, fmt.Errorf("read commit %s: %w", id, err)
	}
	parents := make([]domain.SnapshotID, 0, len(commit.ParentHashes))
	for _, p := range commit.ParentHashes {
		parents = append(parents, domain.SnapshotID(p.String()))
	}
	return domain.Snapshot{
		ID:        id,
		Persona:   g.personaForCommit(commit.Hash),
		Parents:   parents,
		Message:   commit.Message,
		Author:    commit.Author.Name,
		Timestamp: commit.Author.When,
		TreeID:    commit.TreeHash.String(),
	}, nil
}

// personaForCommit finds which persona ref currently reaches the given commit.
// Best-effort; empty string if no persona ref covers it.
func (g *GitStorageEngine) personaForCommit(h plumbing.Hash) string {
	refs, err := g.repo.Storer.IterReferences()
	if err != nil {
		return ""
	}
	defer refs.Close()
	found := ""
	_ = refs.ForEach(func(ref *plumbing.Reference) error {
		name := ref.Name().String()
		if !strings.HasPrefix(name, personaRefPrefix) {
			return nil
		}
		persona := strings.TrimPrefix(name, personaRefPrefix)
		iter, err := g.repo.Log(&git.LogOptions{From: ref.Hash()})
		if err != nil {
			return nil
		}
		defer iter.Close()
		_ = iter.ForEach(func(c *object.Commit) error {
			if c.Hash == h {
				found = persona
				return storer.ErrStop
			}
			return nil
		})
		return nil
	})
	return found
}

func (g *GitStorageEngine) Timeline(persona string) ([]domain.SnapshotID, error) {
	ref, err := g.repo.Storer.Reference(plumbing.ReferenceName(personaRefPrefix + persona))
	if err != nil {
		return nil, fmt.Errorf("resolve persona ref %s: %w", persona, err)
	}
	iter, err := g.repo.Log(&git.LogOptions{From: ref.Hash()})
	if err != nil {
		return nil, fmt.Errorf("log persona %s: %w", persona, err)
	}
	defer iter.Close()
	var ids []domain.SnapshotID
	err = iter.ForEach(func(c *object.Commit) error {
		ids = append(ids, domain.SnapshotID(c.Hash.String()))
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk persona log %s: %w", persona, err)
	}
	return ids, nil // go-git Log yields newest first
}

func (g *GitStorageEngine) SetTag(persona, version string, id domain.SnapshotID) error {
	ref := plumbing.NewHashReference(
		plumbing.ReferenceName(tagRefPrefix+persona+"/"+version),
		plumbing.NewHash(string(id)),
	)
	if err := g.repo.Storer.SetReference(ref); err != nil {
		return fmt.Errorf("set tag %s/%s: %w", persona, version, err)
	}
	return nil
}

func (g *GitStorageEngine) ResolveTag(persona, version string) (domain.SnapshotID, error) {
	ref, err := g.repo.Storer.Reference(plumbing.ReferenceName(tagRefPrefix + persona + "/" + version))
	if err != nil {
		return "", fmt.Errorf("resolve tag %s/%s: %w", persona, version, err)
	}
	return domain.SnapshotID(ref.Hash().String()), nil
}

func (g *GitStorageEngine) ListTags(persona string) ([]domain.Tag, error) {
	refs, err := g.repo.Storer.IterReferences()
	if err != nil {
		return nil, fmt.Errorf("iter refs: %w", err)
	}
	defer refs.Close()
	prefix := tagRefPrefix + persona + "/"
	var tags []domain.Tag
	err = refs.ForEach(func(ref *plumbing.Reference) error {
		name := ref.Name().String()
		if !strings.HasPrefix(name, prefix) {
			return nil
		}
		tags = append(tags, domain.Tag{
			Persona: persona,
			Version: strings.TrimPrefix(name, prefix),
			Target:  domain.SnapshotID(ref.Hash().String()),
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk tag refs %s: %w", persona, err)
	}
	return tags, nil
}

func (g *GitStorageEngine) AddRemote(name, url string) error {
	_, err := g.repo.CreateRemote(&config.RemoteConfig{Name: name, URLs: []string{url}})
	if err != nil {
		return fmt.Errorf("add remote %s: %w", name, err)
	}
	return nil
}

func (g *GitStorageEngine) Push(remote string) error {
	err := g.repo.Push(&git.PushOptions{
		RemoteName: remote,
		RefSpecs: []config.RefSpec{
			config.RefSpec("refs/personas/*:refs/personas/*"),
			config.RefSpec("refs/tags/*:refs/tags/*"),
		},
	})
	if err != nil && err != git.NoErrAlreadyUpToDate {
		return fmt.Errorf("push to %s: %w", remote, err)
	}
	return nil
}

func (g *GitStorageEngine) Pull(remote string) error {
	err := g.repo.Fetch(&git.FetchOptions{
		RemoteName: remote,
		RefSpecs: []config.RefSpec{
			config.RefSpec("refs/personas/*:refs/personas/*"),
			config.RefSpec("refs/tags/*:refs/tags/*"),
		},
	})
	if err != nil && err != git.NoErrAlreadyUpToDate {
		return fmt.Errorf("pull from %s: %w", remote, err)
	}
	return nil
}

// authorEmail synthesizes a stable placeholder email so commit objects are
// well-formed even when only an author name is known.
func authorEmail(author string) string {
	a := strings.TrimSpace(author)
	if a == "" {
		a = "unknown"
	}
	return strings.ReplaceAll(a, " ", ".") + "@claude-git.local"
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
cd /Users/angeral/Repositories/claude_git && go test ./internal/storage/
```

Expected: PASS (`ok  github.com/a2ngerer/claude-containers/internal/storage`) — all object/tree/snapshot/timeline/tag/remote round-trips green against the temp repo.

- [ ] **Step 5: Commit**

```bash
cd /Users/angeral/Repositories/claude_git && git add internal/storage/git.go internal/storage/git_test.go && git commit -m "M1: GitStorageEngine (go-git) - objects, trees, snapshots, tags, remotes"
```

---

## Task 6: Environment paths

**Files:**
- Create: `internal/environment/paths.go`
- Test: `internal/environment/paths_test.go`

`WorkspaceHash` = sha1-hex of `filepath.Clean(absolute path)`. `ToolHome` respects `CLAUDE_GIT_HOME`, else `~/.claude_git`. The remaining helpers compose paths.

- [ ] **Step 1: Write the failing test**

Create `internal/environment/paths_test.go`:

```go
package environment

import (
	"crypto/sha1"
	"encoding/hex"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWorkspaceHash_StableAndCleaned(t *testing.T) {
	base := "/Users/x/proj"
	h1 := WorkspaceHash(base)
	h2 := WorkspaceHash("/Users/x/proj/") // trailing slash -> cleaned to same
	h3 := WorkspaceHash("/Users/x/proj/../proj")
	require.Equal(t, h1, h2)
	require.Equal(t, h1, h3)

	sum := sha1.Sum([]byte(filepath.Clean(base)))
	require.Equal(t, hex.EncodeToString(sum[:]), h1)
	require.Len(t, h1, 40)
}

func TestWorkspaceHash_DifferentPathsDiffer(t *testing.T) {
	require.NotEqual(t, WorkspaceHash("/a/b"), WorkspaceHash("/a/c"))
}

func TestToolHome_RespectsEnv(t *testing.T) {
	t.Setenv("CLAUDE_GIT_HOME", "/tmp/cg-home")
	require.Equal(t, "/tmp/cg-home", ToolHome())
}

func TestToolHome_DefaultUnderHome(t *testing.T) {
	t.Setenv("CLAUDE_GIT_HOME", "")
	t.Setenv("HOME", "/tmp/fake-home")
	require.Equal(t, filepath.Join("/tmp/fake-home", ".claude_git"), ToolHome())
}

func TestDerivedPaths(t *testing.T) {
	t.Setenv("CLAUDE_GIT_HOME", "/tmp/cg")
	hash := "abc123"
	require.Equal(t, filepath.Join("/tmp/cg", "environments", hash), EnvDir(hash))
	require.Equal(t, filepath.Join("/tmp/cg", "environments", hash, "repo"), RepoDir(hash))
	require.Equal(t, filepath.Join("/tmp/cg", "cache", hash, "reviewer"), CacheDir(hash, "reviewer"))
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /Users/angeral/Repositories/claude_git && go test ./internal/environment/
```

Expected: compilation failure — `undefined: WorkspaceHash`, `undefined: ToolHome`, etc. Non-zero exit.

- [ ] **Step 3: Write minimal implementation**

Create `internal/environment/paths.go`:

```go
// internal/environment/paths.go
package environment

import (
	"crypto/sha1"
	"encoding/hex"
	"os"
	"path/filepath"
)

// WorkspaceHash returns the sha1 hex of the cleaned absolute workspace path.
// It identifies an environment uniquely per workspace directory.
func WorkspaceHash(absWorkspace string) string {
	sum := sha1.Sum([]byte(filepath.Clean(absWorkspace)))
	return hex.EncodeToString(sum[:])
}

// ToolHome returns the tool's root directory. CLAUDE_GIT_HOME overrides;
// otherwise ~/.claude_git.
func ToolHome() string {
	if h := os.Getenv("CLAUDE_GIT_HOME"); h != "" {
		return h
	}
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".claude_git")
}

// EnvDir is the per-workspace environment directory.
func EnvDir(hash string) string { return filepath.Join(ToolHome(), "environments", hash) }

// RepoDir is the hidden git repo (StorageEngine backend) for an environment.
func RepoDir(hash string) string { return filepath.Join(EnvDir(hash), "repo") }

// CacheDir is the materialized CLAUDE_CONFIG_DIR for one persona (ephemeral).
func CacheDir(hash, persona string) string {
	return filepath.Join(ToolHome(), "cache", hash, persona)
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
cd /Users/angeral/Repositories/claude_git && go test ./internal/environment/
```

Expected: PASS (`ok  github.com/a2ngerer/claude-containers/internal/environment`).

- [ ] **Step 5: Commit**

```bash
cd /Users/angeral/Repositories/claude_git && git add internal/environment/paths.go internal/environment/paths_test.go && git commit -m "M1: environment paths (WorkspaceHash, ToolHome, EnvDir, RepoDir, CacheDir)"
```

---

## Task 7: Environment — Create/Open + persona CRUD + SetActive

**Files:**
- Create: `internal/environment/environment.go`
- Test: `internal/environment/environment_test.go`

`Create` makes the env dirs, opens the repo through `storage.OpenGit`, and writes `env.toml`. `Open` returns `domain.ErrNotInitialized` if `env.toml` is absent. Personas live as `repo/personas/<name>/persona.toml`. `env.toml` and `persona.toml` are TOML; `EnvConfig` holds the workspace path and the active persona.

- [ ] **Step 1: Write the failing test**

Create `internal/environment/environment_test.go`:

```go
package environment

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/a2ngerer/claude-containers/internal/domain"
	"github.com/stretchr/testify/require"
)

func setupHome(t *testing.T) {
	t.Helper()
	t.Setenv("CLAUDE_GIT_HOME", t.TempDir())
}

func TestCreate_MakesDirsAndConfig(t *testing.T) {
	setupHome(t)
	ws := t.TempDir()

	env, err := Create(ws)
	require.NoError(t, err)
	require.Equal(t, WorkspaceHash(filepath.Clean(ws)), env.Hash)
	require.Equal(t, filepath.Clean(ws), env.Workspace)
	require.NotNil(t, env.Store)

	// re-open must succeed and report the same workspace
	reopened, err := Open(ws)
	require.NoError(t, err)
	require.Equal(t, env.Hash, reopened.Hash)
	require.Equal(t, filepath.Clean(ws), reopened.Workspace)
}

func TestOpen_NotInitialized(t *testing.T) {
	setupHome(t)
	ws := t.TempDir()
	_, err := Open(ws)
	require.True(t, errors.Is(err, domain.ErrNotInitialized))
}

func TestPersonaCRUD(t *testing.T) {
	setupHome(t)
	ws := t.TempDir()
	env, err := Create(ws)
	require.NoError(t, err)

	p := domain.Persona{
		Name:        "_base",
		Description: "shared base",
		Enforcement: domain.Enforcement{
			PermissionMode: "default",
			ToolsAllow:     []string{"Read"},
			ToolsDeny:      []string{"Write"},
		},
		Metadata: domain.Metadata{Version: "0.1.0", Author: "alexander.angerer"},
	}
	require.NoError(t, env.SavePersona(p))

	got, err := env.LoadPersona("_base")
	require.NoError(t, err)
	require.Equal(t, "_base", got.Name)
	require.Equal(t, []string{"Read"}, got.Enforcement.ToolsAllow)
	require.Equal(t, []string{"Write"}, got.Enforcement.ToolsDeny)

	list, err := env.ListPersonas()
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.Equal(t, "_base", list[0].Name)
}

func TestLoadPersona_NotFound(t *testing.T) {
	setupHome(t)
	ws := t.TempDir()
	env, err := Create(ws)
	require.NoError(t, err)
	_, err = env.LoadPersona("ghost")
	require.True(t, errors.Is(err, domain.ErrPersonaNotFound))
}

func TestSetActive_Persists(t *testing.T) {
	setupHome(t)
	ws := t.TempDir()
	env, err := Create(ws)
	require.NoError(t, err)
	require.NoError(t, env.SavePersona(domain.Persona{Name: "coder"}))
	require.NoError(t, env.SetActive("coder"))

	reopened, err := Open(ws)
	require.NoError(t, err)
	require.Equal(t, "coder", reopened.ActivePersona())
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /Users/angeral/Repositories/claude_git && go test ./internal/environment/ -run 'Create|Open|Persona|SetActive'
```

Expected: compilation failure — `undefined: Create`, `undefined: Open`, `undefined: Environment`. Non-zero exit.

- [ ] **Step 3: Write minimal implementation**

Create `internal/environment/environment.go`:

```go
// internal/environment/environment.go
package environment

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/a2ngerer/claude-containers/internal/domain"
	"github.com/a2ngerer/claude-containers/internal/storage"
	toml "github.com/pelletier/go-toml/v2"
)

type EnvConfig struct {
	WorkspacePath string `toml:"workspace_path"`
	ActivePersona string `toml:"active_persona"`
}

type Environment struct {
	Hash      string
	Workspace string
	Store     storage.StorageEngine
	cfg       EnvConfig
}

func envConfigPath(hash string) string { return filepath.Join(EnvDir(hash), "env.toml") }
func personasDir(hash string) string   { return filepath.Join(RepoDir(hash), "personas") }
func personaDir(hash, name string) string {
	return filepath.Join(personasDir(hash), name)
}

// Create binds a workspace: makes the env dirs, opens the git-backed repo,
// and writes env.toml.
func Create(workspace string) (*Environment, error) {
	abs, err := filepath.Abs(workspace)
	if err != nil {
		return nil, fmt.Errorf("resolve workspace %q: %w", workspace, err)
	}
	abs = filepath.Clean(abs)
	hash := WorkspaceHash(abs)

	if err := os.MkdirAll(personasDir(hash), 0o755); err != nil {
		return nil, fmt.Errorf("create personas dir: %w", err)
	}
	store, err := storage.OpenGit(RepoDir(hash))
	if err != nil {
		return nil, fmt.Errorf("open storage: %w", err)
	}
	cfg := EnvConfig{WorkspacePath: abs, ActivePersona: ""}
	if err := writeEnvConfig(hash, cfg); err != nil {
		return nil, err
	}
	return &Environment{Hash: hash, Workspace: abs, Store: store, cfg: cfg}, nil
}

// Open loads an already-initialized environment for the workspace. Returns
// domain.ErrNotInitialized when env.toml is absent.
func Open(workspace string) (*Environment, error) {
	abs, err := filepath.Abs(workspace)
	if err != nil {
		return nil, fmt.Errorf("resolve workspace %q: %w", workspace, err)
	}
	abs = filepath.Clean(abs)
	hash := WorkspaceHash(abs)

	cfg, err := readEnvConfig(hash)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, domain.ErrNotInitialized
		}
		return nil, err
	}
	store, err := storage.OpenGit(RepoDir(hash))
	if err != nil {
		return nil, fmt.Errorf("open storage: %w", err)
	}
	return &Environment{Hash: hash, Workspace: abs, Store: store, cfg: cfg}, nil
}

func (e *Environment) ActivePersona() string { return e.cfg.ActivePersona }

func (e *Environment) ListPersonas() ([]domain.Persona, error) {
	entries, err := os.ReadDir(personasDir(e.Hash))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read personas dir: %w", err)
	}
	var out []domain.Persona
	for _, de := range entries {
		if !de.IsDir() {
			continue
		}
		p, err := e.LoadPersona(de.Name())
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

func (e *Environment) LoadPersona(name string) (domain.Persona, error) {
	path := filepath.Join(personaDir(e.Hash, name), "persona.toml")
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return domain.Persona{}, fmt.Errorf("%q: %w", name, domain.ErrPersonaNotFound)
	}
	return domain.LoadPersonaTOML(path)
}

func (e *Environment) SavePersona(p domain.Persona) error {
	dir := personaDir(e.Hash, p.Name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create persona dir %q: %w", p.Name, err)
	}
	return domain.SavePersonaTOML(p, filepath.Join(dir, "persona.toml"))
}

func (e *Environment) SetActive(name string) error {
	e.cfg.ActivePersona = name
	return writeEnvConfig(e.Hash, e.cfg)
}

func readEnvConfig(hash string) (EnvConfig, error) {
	raw, err := os.ReadFile(envConfigPath(hash))
	if err != nil {
		return EnvConfig{}, err
	}
	var cfg EnvConfig
	if err := toml.Unmarshal(raw, &cfg); err != nil {
		return EnvConfig{}, fmt.Errorf("unmarshal env.toml: %w", err)
	}
	return cfg, nil
}

func writeEnvConfig(hash string, cfg EnvConfig) error {
	if err := os.MkdirAll(EnvDir(hash), 0o755); err != nil {
		return fmt.Errorf("create env dir: %w", err)
	}
	out, err := toml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal env.toml: %w", err)
	}
	if err := os.WriteFile(envConfigPath(hash), out, 0o644); err != nil {
		return fmt.Errorf("write env.toml: %w", err)
	}
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
cd /Users/angeral/Repositories/claude_git && go test ./internal/environment/
```

Expected: PASS (`ok  github.com/a2ngerer/claude-containers/internal/environment`) — paths and environment tests both green.

- [ ] **Step 5: Commit**

```bash
cd /Users/angeral/Repositories/claude_git && git add internal/environment/environment.go internal/environment/environment_test.go && git commit -m "M1: Environment Create/Open, persona CRUD, SetActive"
```

---

## Task 8: Workspace probe (IsClaudeTracked)

**Files:**
- Create: `internal/probe/probe.go`
- Test: `internal/probe/probe_test.go`

`IsClaudeTracked` runs `git ls-files --error-unmatch .claude` in the workspace via `os/exec`. Exit 0 → tracked; non-zero (the path is untracked, or it is not a git repo) → not tracked. Only a genuine execution failure (git binary missing) is surfaced as an error.

- [ ] **Step 1: Write the failing test**

Create `internal/probe/probe_test.go`:

```go
package probe

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func gitAvailable(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not available")
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v failed: %s", args, string(out))
}

func TestIsClaudeTracked_TrackedRepo(t *testing.T) {
	gitAvailable(t)
	ws := t.TempDir()
	runGit(t, ws, "init")
	runGit(t, ws, "config", "user.email", "t@example.com")
	runGit(t, ws, "config", "user.name", "Tester")
	require.NoError(t, os.MkdirAll(filepath.Join(ws, ".claude"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(ws, ".claude", "settings.json"), []byte("{}"), 0o644))
	runGit(t, ws, "add", ".claude")
	runGit(t, ws, "commit", "-m", "track claude")

	tracked, err := IsClaudeTracked(ws)
	require.NoError(t, err)
	require.True(t, tracked)
}

func TestIsClaudeTracked_UntrackedInRepo(t *testing.T) {
	gitAvailable(t)
	ws := t.TempDir()
	runGit(t, ws, "init")
	require.NoError(t, os.MkdirAll(filepath.Join(ws, ".claude"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(ws, ".claude", "settings.json"), []byte("{}"), 0o644))

	tracked, err := IsClaudeTracked(ws)
	require.NoError(t, err)
	require.False(t, tracked)
}

func TestIsClaudeTracked_NotAGitRepo(t *testing.T) {
	gitAvailable(t)
	ws := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(ws, ".claude"), 0o755))

	tracked, err := IsClaudeTracked(ws)
	require.NoError(t, err)
	require.False(t, tracked)
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /Users/angeral/Repositories/claude_git && go test ./internal/probe/
```

Expected: compilation failure — `undefined: IsClaudeTracked`. Non-zero exit.

- [ ] **Step 3: Write minimal implementation**

Create `internal/probe/probe.go`:

```go
// internal/probe/probe.go
package probe

import (
	"errors"
	"fmt"
	"os/exec"
)

// IsClaudeTracked reports whether the workspace's code git repo already tracks
// the .claude directory. It runs `git ls-files --error-unmatch .claude`:
// exit 0 means tracked; a non-zero exit (untracked path, or not a git repo)
// means not tracked. Only a failure to execute git at all is returned as error.
func IsClaudeTracked(workspace string) (bool, error) {
	cmd := exec.Command("git", "ls-files", "--error-unmatch", ".claude")
	cmd.Dir = workspace
	err := cmd.Run()
	if err == nil {
		return true, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		// git ran and reported "not tracked" (or "not a repository").
		return false, nil
	}
	return false, fmt.Errorf("run git ls-files in %q: %w", workspace, err)
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
cd /Users/angeral/Repositories/claude_git && go test ./internal/probe/
```

Expected: PASS (`ok  github.com/a2ngerer/claude-containers/internal/probe`).

- [ ] **Step 5: Commit**

```bash
cd /Users/angeral/Repositories/claude_git && git add internal/probe/probe.go internal/probe/probe_test.go && git commit -m "M1: workspace probe (IsClaudeTracked)"
```

---

## Task 9: `init` command — bind workspace, import `_base`, marker, probe warning

**Files:**
- Create: `internal/cli/init.go` (replaces the `newInitCmd` stub)
- Modify: `internal/cli/stubs.go` (delete the `newInitCmd` stub)
- Test: `internal/cli/init_test.go`

`init` resolves the workspace, calls `environment.Create`, imports the existing `.claude/` directory and `CLAUDE.md` into the `_base` persona (content copied into `repo/personas/_base/`, plus a `_base` persona.toml), writes the `.claude_git` marker (one line = workspace hash) into the workspace, and probes for a tracked `.claude/` — printing a loud warning if found. Activation never writes into the workspace; the marker is the only file `init` creates there.

- [ ] **Step 1: Write the failing test**

Create `internal/cli/init_test.go`:

```go
package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/a2ngerer/claude-containers/internal/environment"
	"github.com/stretchr/testify/require"
)

func runCLI(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := NewRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

func seedWorkspace(t *testing.T) string {
	t.Helper()
	ws := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(ws, ".claude", "skills", "demo"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(ws, ".claude", "settings.json"), []byte(`{"x":1}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(ws, ".claude", "skills", "demo", "SKILL.md"), []byte("demo skill\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(ws, "CLAUDE.md"), []byte("# project rules\n"), 0o644))
	return ws
}

func TestInit_BindsWorkspaceAndImportsBase(t *testing.T) {
	t.Setenv("CLAUDE_GIT_HOME", t.TempDir())
	ws := seedWorkspace(t)

	out, err := runCLI(t, "init", "--workspace", ws)
	require.NoError(t, err)
	require.Contains(t, out, "Initialized")

	// marker file written into the workspace, one line = hash
	marker, err := os.ReadFile(filepath.Join(ws, ".claude_git"))
	require.NoError(t, err)
	require.Equal(t, environment.WorkspaceHash(filepath.Clean(ws)), strings.TrimSpace(string(marker)))

	// _base persona created with imported content
	hash := environment.WorkspaceHash(filepath.Clean(ws))
	baseDir := filepath.Join(environment.RepoDir(hash), "personas", "_base")
	require.FileExists(t, filepath.Join(baseDir, "persona.toml"))
	require.FileExists(t, filepath.Join(baseDir, "CLAUDE.md"))
	require.FileExists(t, filepath.Join(baseDir, ".claude", "settings.json"))
	require.FileExists(t, filepath.Join(baseDir, ".claude", "skills", "demo", "SKILL.md"))

	// _base appears via the environment API
	env, err := environment.Open(ws)
	require.NoError(t, err)
	personas, err := env.ListPersonas()
	require.NoError(t, err)
	require.Len(t, personas, 1)
	require.Equal(t, "_base", personas[0].Name)
	require.True(t, personas[0].IsLayer())

	// workspace .claude/ left untouched (still exactly what we seeded)
	orig, err := os.ReadFile(filepath.Join(ws, ".claude", "settings.json"))
	require.NoError(t, err)
	require.Equal(t, `{"x":1}`, string(orig))
}

func TestInit_NoExistingClaude(t *testing.T) {
	t.Setenv("CLAUDE_GIT_HOME", t.TempDir())
	ws := t.TempDir()

	out, err := runCLI(t, "init", "--workspace", ws)
	require.NoError(t, err)
	require.Contains(t, out, "Initialized")

	hash := environment.WorkspaceHash(filepath.Clean(ws))
	baseDir := filepath.Join(environment.RepoDir(hash), "personas", "_base")
	require.FileExists(t, filepath.Join(baseDir, "persona.toml"))
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /Users/angeral/Repositories/claude_git && go test ./internal/cli/ -run Init
```

Expected: behaviour failure — the Task-1 stub `newInitCmd` does nothing, so `Initialized` is never printed and the marker/`_base` assertions fail. (If `init.go` is added before the stub is removed, the failure is instead `newInitCmd redeclared in this block`.) Non-zero exit.

- [ ] **Step 3: Write minimal implementation**

Remove the `newInitCmd` stub from `internal/cli/stubs.go` so it now contains only `newListCmd` and `newStatusCmd`:

```go
package cli

import "github.com/spf13/cobra"

// Temporary stubs so the root command compiles. Tasks 10-11 replace
// these (list.go, status.go) with the real implementations.
func newListCmd() *cobra.Command {
	return &cobra.Command{Use: "list", Aliases: []string{"ls"}, Short: "List personas", RunE: func(*cobra.Command, []string) error { return nil }}
}

func newStatusCmd() *cobra.Command {
	return &cobra.Command{Use: "status", Short: "Show active persona and workspace", RunE: func(*cobra.Command, []string) error { return nil }}
}
```

Create `internal/cli/init.go`:

```go
// internal/cli/init.go
package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/a2ngerer/claude-containers/internal/domain"
	"github.com/a2ngerer/claude-containers/internal/environment"
	"github.com/a2ngerer/claude-containers/internal/probe"
	"github.com/spf13/cobra"
)

func newInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Bind the current workspace and seed the _base persona",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ws, err := resolveWorkspace(cmd)
			if err != nil {
				return err
			}
			return runInit(cmd.OutOrStdout(), ws)
		},
	}
}

// resolveWorkspace reads the --workspace flag, defaulting to the process CWD.
func resolveWorkspace(cmd *cobra.Command) (string, error) {
	ws, _ := cmd.Flags().GetString("workspace")
	if ws == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("determine working directory: %w", err)
		}
		ws = cwd
	}
	abs, err := filepath.Abs(ws)
	if err != nil {
		return "", fmt.Errorf("resolve workspace %q: %w", ws, err)
	}
	return filepath.Clean(abs), nil
}

func runInit(out io.Writer, workspace string) error {
	env, err := environment.Create(workspace)
	if err != nil {
		return fmt.Errorf("create environment: %w", err)
	}

	if err := importBase(env, workspace); err != nil {
		return err
	}

	// marker file: one line = workspace hash
	marker := filepath.Join(workspace, ".claude_git")
	if err := os.WriteFile(marker, []byte(env.Hash+"\n"), 0o644); err != nil {
		return fmt.Errorf("write marker: %w", err)
	}

	fmt.Fprintf(out, "Initialized claude_git environment for %s\n", workspace)
	fmt.Fprintf(out, "  hash:   %s\n", env.Hash)
	fmt.Fprintf(out, "  base:   _base persona seeded from existing .claude/ and CLAUDE.md\n")

	tracked, err := probe.IsClaudeTracked(workspace)
	if err != nil {
		return fmt.Errorf("probe workspace: %w", err)
	}
	if tracked {
		fmt.Fprintln(out, "")
		fmt.Fprintln(out, "WARNING: .claude/ is tracked by this workspace's code repository.")
		fmt.Fprintln(out, "  claude_git never writes into the workspace, so this is harmless to claude_git,")
		fmt.Fprintln(out, "  but you may want to untrack it (git rm -r --cached .claude) to avoid committing")
		fmt.Fprintln(out, "  agent config into your code history. claude_git will not touch your code repo.")
	}
	return nil
}

// importBase seeds the _base persona: it copies the workspace .claude/ tree and
// CLAUDE.md into the persona dir and writes a minimal _base persona.toml.
func importBase(env *environment.Environment, workspace string) error {
	baseDir := filepath.Join(environment.RepoDir(env.Hash), "personas", "_base")
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return fmt.Errorf("create _base dir: %w", err)
	}

	srcClaude := filepath.Join(workspace, ".claude")
	if info, err := os.Stat(srcClaude); err == nil && info.IsDir() {
		if err := copyTree(srcClaude, filepath.Join(baseDir, ".claude")); err != nil {
			return fmt.Errorf("import .claude: %w", err)
		}
	}

	srcMD := filepath.Join(workspace, "CLAUDE.md")
	if _, err := os.Stat(srcMD); err == nil {
		if err := copyFile(srcMD, filepath.Join(baseDir, "CLAUDE.md")); err != nil {
			return fmt.Errorf("import CLAUDE.md: %w", err)
		}
	}

	base := domain.Persona{
		Name:        "_base",
		Description: "Shared base layer imported from the workspace .claude/ and CLAUDE.md.",
		Extends:     "",
		Config: domain.Config{
			ClaudeMD:       "CLAUDE.md",
			SettingSources: []string{"user", "project"},
			Skills:         domain.SkillSet{Mode: "allowlist"},
		},
		Enforcement: domain.Enforcement{PermissionMode: "default"},
		Metadata:    domain.Metadata{Version: "0.1.0", Author: defaultAuthor()},
	}
	return domain.SavePersonaTOML(base, filepath.Join(baseDir, "persona.toml"))
}

func defaultAuthor() string {
	if u := os.Getenv("USER"); u != "" {
		return u
	}
	return "unknown"
}

func copyTree(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		return copyFile(path, target)
	})
}

func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
cd /Users/angeral/Repositories/claude_git && go build ./... && go test ./internal/cli/ -run Init
```

Expected: PASS (`ok  github.com/a2ngerer/claude-containers/internal/cli`).

- [ ] **Step 5: Commit**

```bash
cd /Users/angeral/Repositories/claude_git && git add internal/cli/init.go internal/cli/stubs.go internal/cli/init_test.go && git commit -m "M1: init command - bind workspace, import _base, marker, probe warning"
```

---

## Task 10: `list` command — personas with active marker + version

**Files:**
- Create: `internal/cli/list.go` (replaces the `newListCmd` stub)
- Modify: `internal/cli/stubs.go` (delete the `newListCmd` stub)
- Test: `internal/cli/list_test.go`

`list` opens the environment and prints one line per persona: an active marker (`*`/space), the name, and its version. `ErrNotInitialized` surfaces as a clear error.

- [ ] **Step 1: Write the failing test**

Create `internal/cli/list_test.go`:

```go
package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestList_ShowsBaseAfterInit(t *testing.T) {
	t.Setenv("CLAUDE_GIT_HOME", t.TempDir())
	ws := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(ws, "CLAUDE.md"), []byte("# rules\n"), 0o644))

	_, err := runCLI(t, "init", "--workspace", ws)
	require.NoError(t, err)

	out, err := runCLI(t, "list", "--workspace", ws)
	require.NoError(t, err)
	require.Contains(t, out, "_base")
	require.Contains(t, out, "0.1.0")
}

func TestList_NotInitialized(t *testing.T) {
	t.Setenv("CLAUDE_GIT_HOME", t.TempDir())
	ws := t.TempDir()
	_, err := runCLI(t, "list", "--workspace", ws)
	require.Error(t, err)
	require.Contains(t, err.Error(), "not initialized")
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /Users/angeral/Repositories/claude_git && go test ./internal/cli/ -run List
```

Expected: behaviour failure — the stub `newListCmd` prints nothing, so `_base`/`0.1.0` are absent and `TestList_NotInitialized` gets no error. (If `list.go` is added before the stub is removed: `newListCmd redeclared`.) Non-zero exit.

- [ ] **Step 3: Write minimal implementation**

Remove `newListCmd` from `internal/cli/stubs.go`, leaving only `newStatusCmd`:

```go
package cli

import "github.com/spf13/cobra"

// Temporary stub so the root command compiles. Task 11 replaces this (status.go).
func newStatusCmd() *cobra.Command {
	return &cobra.Command{Use: "status", Short: "Show active persona and workspace", RunE: func(*cobra.Command, []string) error { return nil }}
}
```

Create `internal/cli/list.go`:

```go
// internal/cli/list.go
package cli

import (
	"fmt"
	"io"

	"github.com/a2ngerer/claude-containers/internal/environment"
	"github.com/spf13/cobra"
)

func newListCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List personas in this workspace's environment",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ws, err := resolveWorkspace(cmd)
			if err != nil {
				return err
			}
			return runList(cmd.OutOrStdout(), ws)
		},
	}
}

func runList(out io.Writer, workspace string) error {
	env, err := environment.Open(workspace)
	if err != nil {
		return err
	}
	personas, err := env.ListPersonas()
	if err != nil {
		return fmt.Errorf("list personas: %w", err)
	}
	if len(personas) == 0 {
		fmt.Fprintln(out, "no personas yet")
		return nil
	}
	active := env.ActivePersona()
	for _, p := range personas {
		marker := " "
		if p.Name == active {
			marker = "*"
		}
		version := p.Metadata.Version
		if version == "" {
			version = "-"
		}
		fmt.Fprintf(out, "%s %-20s %s\n", marker, p.Name, version)
	}
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
cd /Users/angeral/Repositories/claude_git && go build ./... && go test ./internal/cli/ -run List
```

Expected: PASS (`ok  github.com/a2ngerer/claude-containers/internal/cli`).

- [ ] **Step 5: Commit**

```bash
cd /Users/angeral/Repositories/claude_git && git add internal/cli/list.go internal/cli/stubs.go internal/cli/list_test.go && git commit -m "M1: list command - personas with active marker and version"
```

---

## Task 11: `status` command — active persona + workspace

**Files:**
- Create: `internal/cli/status.go` (replaces the `newStatusCmd` stub)
- Delete: `internal/cli/stubs.go` (all three commands now have real files)
- Test: `internal/cli/status_test.go`

`status` opens the environment and prints the workspace path, the environment hash, the repo dir, and the active persona (or `none`). It surfaces `ErrNotInitialized` clearly.

- [ ] **Step 1: Write the failing test**

Create `internal/cli/status_test.go`:

```go
package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/a2ngerer/claude-containers/internal/environment"
	"github.com/stretchr/testify/require"
)

func TestStatus_AfterInit(t *testing.T) {
	t.Setenv("CLAUDE_GIT_HOME", t.TempDir())
	ws := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(ws, "CLAUDE.md"), []byte("# rules\n"), 0o644))

	_, err := runCLI(t, "init", "--workspace", ws)
	require.NoError(t, err)

	out, err := runCLI(t, "status", "--workspace", ws)
	require.NoError(t, err)
	require.Contains(t, out, filepath.Clean(ws))
	require.Contains(t, out, environment.WorkspaceHash(filepath.Clean(ws)))
	require.Contains(t, out, "none") // no active persona yet
}

func TestStatus_ReflectsActivePersona(t *testing.T) {
	t.Setenv("CLAUDE_GIT_HOME", t.TempDir())
	ws := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(ws, "CLAUDE.md"), []byte("# rules\n"), 0o644))
	_, err := runCLI(t, "init", "--workspace", ws)
	require.NoError(t, err)

	env, err := environment.Open(ws)
	require.NoError(t, err)
	require.NoError(t, env.SetActive("_base"))

	out, err := runCLI(t, "status", "--workspace", ws)
	require.NoError(t, err)
	require.Contains(t, out, "_base")
}

func TestStatus_NotInitialized(t *testing.T) {
	t.Setenv("CLAUDE_GIT_HOME", t.TempDir())
	ws := t.TempDir()
	_, err := runCLI(t, "status", "--workspace", ws)
	require.Error(t, err)
	require.Contains(t, err.Error(), "not initialized")
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /Users/angeral/Repositories/claude_git && go test ./internal/cli/ -run Status
```

Expected: behaviour failure — the stub `newStatusCmd` prints nothing and never errors. (If `status.go` is added before the stub is removed: `newStatusCmd redeclared`.) Non-zero exit.

- [ ] **Step 3: Write minimal implementation**

Delete `internal/cli/stubs.go` (all three commands now have real files):

```bash
cd /Users/angeral/Repositories/claude_git && rm internal/cli/stubs.go
```

Create `internal/cli/status.go`:

```go
// internal/cli/status.go
package cli

import (
	"fmt"
	"io"

	"github.com/a2ngerer/claude-containers/internal/environment"
	"github.com/spf13/cobra"
)

func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show the active persona and bound workspace",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ws, err := resolveWorkspace(cmd)
			if err != nil {
				return err
			}
			return runStatus(cmd.OutOrStdout(), ws)
		},
	}
}

func runStatus(out io.Writer, workspace string) error {
	env, err := environment.Open(workspace)
	if err != nil {
		return err
	}
	active := env.ActivePersona()
	if active == "" {
		active = "none"
	}
	fmt.Fprintf(out, "workspace: %s\n", env.Workspace)
	fmt.Fprintf(out, "hash:      %s\n", env.Hash)
	fmt.Fprintf(out, "repo:      %s\n", environment.RepoDir(env.Hash))
	fmt.Fprintf(out, "active:    %s\n", active)
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
cd /Users/angeral/Repositories/claude_git && go build ./... && go test ./internal/cli/
```

Expected: PASS (`ok  github.com/a2ngerer/claude-containers/internal/cli`) — all CLI tests (root, init, list, status) green.

- [ ] **Step 5: Commit**

```bash
cd /Users/angeral/Repositories/claude_git && git add internal/cli/status.go internal/cli/status_test.go && git rm internal/cli/stubs.go 2>/dev/null; git add -A && git commit -m "M1: status command - active persona and workspace"
```

---

## Task 12: Full-suite green gate + end-to-end smoke

**Files:**
- Test: (no new files) — run the whole suite and a real `init`/`list`/`status` round-trip.

This task is the milestone exit check from the contract: `go build ./... && go test ./...` green, and `claude_git init` works end-to-end with `list` showing `_base` and `status` showing the active persona + workspace.

- [ ] **Step 1: Write the failing test**

No new test file. The "failing test" here is the full suite plus a manual smoke run; if any earlier task regressed, this surfaces it. Run:

```bash
cd /Users/angeral/Repositories/claude_git && go vet ./...
```

Expected initially: clean. If `go vet` reports anything, fix it before proceeding (this is the point of the gate).

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /Users/angeral/Repositories/claude_git && go build ./... && go test ./...
```

Expected: all packages PASS. If any package fails, that is the signal to fix it in this task before the milestone closes:

```
ok  github.com/a2ngerer/claude-containers/internal/cli
ok  github.com/a2ngerer/claude-containers/internal/domain
ok  github.com/a2ngerer/claude-containers/internal/environment
ok  github.com/a2ngerer/claude-containers/internal/probe
ok  github.com/a2ngerer/claude-containers/internal/storage
```

- [ ] **Step 3: Write minimal implementation**

End-to-end smoke against a real temp workspace and temp tool home (no code changes; this proves the binary works as a whole):

```bash
cd /Users/angeral/Repositories/claude_git && \
  go build -o /tmp/cg ./cmd/claude_git && \
  export CLAUDE_GIT_HOME="$(mktemp -d)" && \
  SMOKE_WS="$(mktemp -d)" && \
  mkdir -p "$SMOKE_WS/.claude/skills/demo" && \
  printf '{"x":1}' > "$SMOKE_WS/.claude/settings.json" && \
  printf '# project rules\n' > "$SMOKE_WS/CLAUDE.md" && \
  /tmp/cg init --workspace "$SMOKE_WS" && \
  echo "--- list ---" && /tmp/cg list --workspace "$SMOKE_WS" && \
  echo "--- status ---" && /tmp/cg status --workspace "$SMOKE_WS" && \
  echo "--- workspace settings (must be untouched) ---" && cat "$SMOKE_WS/.claude/settings.json"
```

Expected output (paths/hash vary): an `Initialized claude_git environment ...` block, a `list` line containing `_base` and `0.1.0`, a `status` block with `workspace:`/`hash:`/`repo:`/`active:    none`, and a final line `{"x":1}` proving the workspace `.claude/` was not modified (only the `.claude_git` marker was added).

- [ ] **Step 4: Run test to verify it passes**

```bash
cd /Users/angeral/Repositories/claude_git && go build ./... && go test ./... && go vet ./...
```

Expected: PASS across all packages and a clean `go vet`.

- [ ] **Step 5: Commit**

```bash
cd /Users/angeral/Repositories/claude_git && git add -A && git commit -m "M1: full-suite green gate; init/list/status end-to-end smoke verified" --allow-empty
```

---

## Milestone exit criteria (M1)

- `go build ./...` and `go test ./...` are green; `go vet ./...` is clean.
- `claude_git init` binds a workspace: creates `~/.claude_git/environments/<hash>/{env.toml,repo/}`, imports the existing `.claude/` + `CLAUDE.md` into a `_base` persona, writes the one-line `.claude_git` marker, and warns loudly (without acting) if `.claude/` is tracked by the code repo.
- `claude_git list` shows `_base` with its version; `claude_git status` shows the active persona, workspace, hash, and repo path.
- `GitStorageEngine` is exercised against a real temp repo (`t.TempDir()`, `CLAUDE_GIT_HOME` on temp) for objects, trees, snapshots, timeline, tags, and remote registration.
- The workspace's own `.claude/` and `CLAUDE.md` are read at init but otherwise left byte-for-byte untouched.
