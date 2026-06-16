# M2 Personas & Versioning — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (- [ ]) syntax for tracking.

**Goal:** Deliver the full persona lifecycle (create/show/edit/remove + two embedded templates) and the versioning axis (snapshot/log/capability-diff/rollback/tag) on top of the M1 domain, storage, and environment layers.

**Architecture:** A pure `compose` layer resolves `_base` + persona diffs into a `ResolvedManifest` (scalar override, skills/subagents union, `replace`-mode escape hatch, concatenated `CLAUDE.md`). Thin `cli` command groups (`persona.go`, `version.go`) parse flags and delegate to `compose`, `environment`, and the `storage.StorageEngine`; all versioning maps onto `WriteTree`/`WriteSnapshot`/`Timeline`/`SetTag`. The capability diff is computed in `compose` from two `ResolvedManifest`s and is the key user-facing feature — never a raw file diff.

**Tech Stack:** Go 1.23+, `spf13/cobra`, `pelletier/go-toml/v2`, `go-git/go-git/v5` (via M1 storage), `stretchr/testify/require`.

**Depends on:** M1 (domain types, `storage.StorageEngine` + `OpenGit`, `environment.Create`/`Open`/`LoadPersona`/`SavePersona`/`ListPersonas`/`SetActive`, `environment.CacheDir`/`RepoDir` paths all present and tested).

---

## File Structure

| File | Responsibility |
|---|---|
| `internal/compose/compose.go` | **Create.** `ResolvedManifest` + `Compose()`; merge `_base`+persona (scalar override, skills/subagents union or replace, CLAUDE.md concat). |
| `internal/compose/diff.go` | **Create.** `CapabilityDiff` + `Diff()`; structural delta (skills/subagents/tool allow+deny only-in-A / only-in-B) between two `ResolvedManifest`s. |
| `internal/compose/compose_test.go` | **Create.** Tests for union, scalar override, replace-mode, CLAUDE.md concat, extends-layer-not-found. |
| `internal/compose/diff_test.go` | **Create.** Tests for the capability diff (skills/subagents/tool deltas, symmetric). |
| `internal/cli/templates.go` | **Create.** Embedded `coder` + `reviewer` persona templates (`persona.toml` + `CLAUDE.md` content) and a lookup helper. |
| `internal/cli/persona.go` | **Create.** `new` (`--from` / `--extends` / `--template`), `show`, `edit`, `rm` cobra commands. |
| `internal/cli/version.go` | **Create.** `snapshot` (alias `commit`), `log`, `diff`, `rollback`, `tag` cobra commands. |
| `internal/cli/personahelpers.go` | **Create.** Shared helpers: scaffold a persona dir, resolve a persona-or-snapshot ref to a `ResolvedManifest`, format a timeline, format a capability diff. |
| `internal/cli/persona_test.go` | **Create.** Tests for scaffold (`new`/`--template`/`--from`), `show`, `rm`. |
| `internal/cli/version_test.go` | **Create.** Tests for snapshot/log roundtrip, rollback, tag, capability-diff command. |
| `internal/cli/root.go` | **Modify.** Register the new command groups on the root command (M1 created `root.go`). |

**New types beyond the contract** (all in the contract-designated packages, flagged in the closing summary):
`compose.CapabilityDiff` (in `compose/`, the diff result struct — the contract names `capability-diff` as a `domain core` concern but defines no struct; placing the result type next to `Compose` keeps `domain` I/O-free and pure).

---

## Conventions for every task

- Module path: `github.com/a2ngerer/agent-containers`.
- Run all commands from the repo root `/Users/angeral/Repositories/agent-containers`.
- Tests isolate the tool home: every test sets `t.Setenv("ACON_HOME", t.TempDir())` and seeds an environment via `environment.Create(t.TempDir())` (M1 API).
- Use `require` (fail-fast), never `assert`.
- One commit per task; commit message in English; no `Co-Authored-By` trailer.
- TDD order per task: (1) write failing test → (2) run, see it fail → (3) minimal impl → (4) run, see it pass → (5) commit.

---

## Task 1 — `compose.ResolvedManifest` + `Compose()`: scalar override + skills/subagents union

Composition of `_base` + persona diff. Scalars (`SettingSources`, `Enforcement`, `MCP`) come from the persona layer; skills/subagents `Include` lists are merged as a **union** of base + persona; `ClaudeMD` is base content + `"\n\n"` + persona content.

**Files:**
- Create: `internal/compose/compose.go`
- Test: `internal/compose/compose_test.go`

### Steps

- [ ] **1.1 — Write the failing test.** Create `internal/compose/compose_test.go`:

```go
package compose_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/a2ngerer/agent-containers/internal/compose"
	"github.com/a2ngerer/agent-containers/internal/domain"
	"github.com/a2ngerer/agent-containers/internal/environment"
	"github.com/stretchr/testify/require"
)

// seedEnv creates an isolated tool home + a bound environment with a _base layer
// and writes the given personas into the repo. Returns the open environment.
func seedEnv(t *testing.T, personas ...domain.Persona) *environment.Environment {
	t.Helper()
	t.Setenv("ACON_HOME", t.TempDir())
	ws := t.TempDir()
	env, err := environment.Create(ws)
	require.NoError(t, err)
	for _, p := range personas {
		require.NoError(t, env.SavePersona(p))
		writeClaudeMD(t, env, p.Name, "MD:"+p.Name)
	}
	return env
}

// writeClaudeMD writes a CLAUDE.md into a persona dir in the repo.
func writeClaudeMD(t *testing.T, env *environment.Environment, persona, body string) {
	t.Helper()
	dir := filepath.Join(environment.RepoDir(env.Hash), "personas", persona)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte(body), 0o644))
}

func baseLayer() domain.Persona {
	return domain.Persona{
		Name:    "_base",
		Extends: "",
		Config: domain.Config{
			ClaudeMD:       "CLAUDE.md",
			SettingSources: []string{"user", "project", "local"},
			Skills:         domain.SkillSet{Mode: "allowlist", Include: []string{"base-skill"}},
			Subagents:      domain.SubagentSet{Include: []string{"base-agent"}},
		},
	}
}

func TestCompose_UnionAndScalarOverride(t *testing.T) {
	base := baseLayer()
	leaf := domain.Persona{
		Name:    "coder",
		Extends: "_base",
		Config: domain.Config{
			ClaudeMD:       "CLAUDE.md",
			SettingSources: []string{"user", "project"}, // overrides base
			Skills:         domain.SkillSet{Mode: "allowlist", Include: []string{"build-skill"}},
			Subagents:      domain.SubagentSet{Include: []string{"coder-agent"}},
			MCP:            domain.MCPConfig{Config: "mcp.json", Strict: true},
		},
		Enforcement: domain.Enforcement{PermissionMode: "default", ToolsAllow: []string{"Write"}},
	}
	env := seedEnv(t, base, leaf)

	rm, err := compose.Compose(env, "coder")
	require.NoError(t, err)

	// skills/subagents are the UNION of base + persona
	require.ElementsMatch(t, []string{"base-skill", "build-skill"}, rm.Skills)
	require.ElementsMatch(t, []string{"base-agent", "coder-agent"}, rm.Subagents)
	// scalars come from the persona layer
	require.Equal(t, []string{"user", "project"}, rm.SettingSrc)
	require.Equal(t, "default", rm.Enforcement.PermissionMode)
	require.Equal(t, []string{"Write"}, rm.Enforcement.ToolsAllow)
	require.Equal(t, "mcp.json", rm.MCP.Config)
	require.True(t, rm.MCP.Strict)
	// CLAUDE.md = base body + "\n\n" + persona body
	require.Equal(t, "MD:_base\n\nMD:coder", rm.ClaudeMD)
}
```

- [ ] **1.2 — Run, see it fail.**

```bash
cd /Users/angeral/Repositories/agent-containers && go test ./internal/compose/ 2>&1 | head -20
```

Expected: build failure — `package compose ... no Go files` / `undefined: compose.Compose`.

- [ ] **1.3 — Minimal implementation.** Create `internal/compose/compose.go`:

```go
// Package compose resolves a persona together with its extends-layer into a
// flat ResolvedManifest. It is pure with respect to domain types but reads
// persona content (CLAUDE.md) from the environment's repo.
package compose

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/a2ngerer/agent-containers/internal/domain"
	"github.com/a2ngerer/agent-containers/internal/environment"
)

// ResolvedManifest is the composed, effective configuration of a leaf persona.
type ResolvedManifest struct {
	Persona     domain.Persona // the leaf persona (post-merge effective values)
	Skills      []string       // resolved skill dir names to include
	Subagents   []string       // resolved subagent basenames
	ClaudeMD    string         // composed CLAUDE.md content (base + persona)
	SettingSrc  []string
	Enforcement domain.Enforcement
	MCP         domain.MCPConfig
}

// Compose resolves personaName against its extends layer and returns the
// effective manifest. Resolution order is base -> persona: scalars are taken
// from the persona layer; skills/subagents include-lists are unioned unless
// the persona sets Skills.Mode == "replace".
func Compose(e *environment.Environment, personaName string) (ResolvedManifest, error) {
	leaf, err := e.LoadPersona(personaName)
	if err != nil {
		return ResolvedManifest{}, fmt.Errorf("load persona %q: %w", personaName, err)
	}

	skills := append([]string(nil), leaf.Config.Skills.Include...)
	subagents := append([]string(nil), leaf.Config.Subagents.Include...)
	claudeMD, err := readClaudeMD(e, leaf)
	if err != nil {
		return ResolvedManifest{}, err
	}

	if leaf.Extends != "" {
		base, err := e.LoadPersona(leaf.Extends)
		if err != nil {
			return ResolvedManifest{}, fmt.Errorf("%w: %s", domain.ErrLayerNotFound, leaf.Extends)
		}
		if leaf.Config.Skills.Mode != "replace" {
			skills = union(base.Config.Skills.Include, skills)
		}
		subagents = union(base.Config.Subagents.Include, subagents)
		baseMD, err := readClaudeMD(e, base)
		if err != nil {
			return ResolvedManifest{}, err
		}
		claudeMD = baseMD + "\n\n" + claudeMD
	}

	return ResolvedManifest{
		Persona:     leaf,
		Skills:      skills,
		Subagents:   subagents,
		ClaudeMD:    claudeMD,
		SettingSrc:  leaf.Config.SettingSources,
		Enforcement: leaf.Enforcement,
		MCP:         leaf.Config.MCP,
	}, nil
}

// readClaudeMD reads the persona-local CLAUDE.md (Config.ClaudeMD path,
// default "CLAUDE.md"). A missing file yields empty content, not an error.
func readClaudeMD(e *environment.Environment, p domain.Persona) (string, error) {
	name := p.Config.ClaudeMD
	if name == "" {
		name = "CLAUDE.md"
	}
	path := filepath.Join(environment.RepoDir(e.Hash), "personas", p.Name, name)
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read CLAUDE.md for %q: %w", p.Name, err)
	}
	return string(b), nil
}

// union returns a + (b minus duplicates), preserving a's order then b's.
func union(a, b []string) []string {
	seen := make(map[string]struct{}, len(a)+len(b))
	out := make([]string, 0, len(a)+len(b))
	for _, s := range append(append([]string(nil), a...), b...) {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}
```

- [ ] **1.4 — Run, see it pass.**

```bash
cd /Users/angeral/Repositories/agent-containers && go test ./internal/compose/ 2>&1 | tail -5
```

Expected: `ok  	github.com/a2ngerer/agent-containers/internal/compose`.

- [ ] **1.5 — Commit.**

```bash
cd /Users/angeral/Repositories/agent-containers && git add internal/compose/compose.go internal/compose/compose_test.go && git commit -m "feat(compose): resolve persona + base layer with scalar override and skills/subagents union"
```

---

## Task 2 — Compose: `replace` mode and CLAUDE.md concat edge cases

When the persona sets `Skills.Mode == "replace"`, the resolved skills are **only** the persona list (base skills dropped); subagents still union. When `Extends == ""`, no base content is prepended.

**Files:**
- Modify: `internal/compose/compose_test.go`
- (No production change expected — Task 1 already implements `replace`; this task proves it and the layer-not-found path.)

### Steps

- [ ] **2.1 — Write the failing test.** Append to `internal/compose/compose_test.go`:

```go
func TestCompose_ReplaceModeDropsBaseSkills(t *testing.T) {
	base := baseLayer()
	leaf := domain.Persona{
		Name:    "reviewer",
		Extends: "_base",
		Config: domain.Config{
			ClaudeMD:       "CLAUDE.md",
			SettingSources: []string{"user", "project"},
			Skills:         domain.SkillSet{Mode: "replace", Include: []string{"security-review"}},
			Subagents:      domain.SubagentSet{Include: []string{"code-reviewer"}},
		},
	}
	env := seedEnv(t, base, leaf)

	rm, err := compose.Compose(env, "reviewer")
	require.NoError(t, err)

	// replace mode: ONLY the persona's skills, base-skill dropped
	require.Equal(t, []string{"security-review"}, rm.Skills)
	// subagents still union
	require.ElementsMatch(t, []string{"base-agent", "code-reviewer"}, rm.Subagents)
}

func TestCompose_NoExtendsHasNoBasePrefix(t *testing.T) {
	standalone := domain.Persona{
		Name:    "solo",
		Extends: "",
		Config: domain.Config{
			ClaudeMD: "CLAUDE.md",
			Skills:   domain.SkillSet{Mode: "allowlist", Include: []string{"only-skill"}},
		},
	}
	env := seedEnv(t, standalone)

	rm, err := compose.Compose(env, "solo")
	require.NoError(t, err)

	require.Equal(t, []string{"only-skill"}, rm.Skills)
	require.Equal(t, "MD:solo", rm.ClaudeMD) // no "\n\n" prefix
}

func TestCompose_ExtendsLayerNotFound(t *testing.T) {
	leaf := domain.Persona{
		Name:    "broken",
		Extends: "_ghost",
		Config:  domain.Config{ClaudeMD: "CLAUDE.md"},
	}
	env := seedEnv(t, leaf)

	_, err := compose.Compose(env, "broken")
	require.ErrorIs(t, err, domain.ErrLayerNotFound)
}
```

- [ ] **2.2 — Run, see it fail (or pass).**

```bash
cd /Users/angeral/Repositories/agent-containers && go test ./internal/compose/ -run 'Replace|NoExtends|LayerNotFound' 2>&1 | tail -15
```

Expected: `replace`/`NoExtends` pass immediately (Task 1 already implements both). `ExtendsLayerNotFound` could fail only if M1's `LoadPersona` returns an error that does not wrap into `domain.ErrLayerNotFound` — Task 1's mapping `fmt.Errorf("%w: %s", domain.ErrLayerNotFound, ...)` handles this. Confirm the failure (if any) names the missing layer.

- [ ] **2.3 — Minimal implementation (only if 2.2 fails).** If `TestCompose_ExtendsLayerNotFound` fails, the cause is the wrap in `compose.go` not being reached — re-check that the `if leaf.Extends != ""` block returns `fmt.Errorf("%w: %s", domain.ErrLayerNotFound, leaf.Extends)` on the inner `LoadPersona` error. If `replace` failed, re-check the `if leaf.Config.Skills.Mode != "replace"` guard. No code beyond Task 1 is anticipated.

- [ ] **2.4 — Run, see it pass.**

```bash
cd /Users/angeral/Repositories/agent-containers && go test ./internal/compose/ 2>&1 | tail -5
```

Expected: `ok  	github.com/a2ngerer/agent-containers/internal/compose`.

- [ ] **2.5 — Commit.**

```bash
cd /Users/angeral/Repositories/agent-containers && git add internal/compose/compose_test.go && git commit -m "test(compose): cover replace-mode, no-extends, and layer-not-found composition paths"
```

---

## Task 3 — Capability diff: `CapabilityDiff` + `Diff()`

The capability diff is the key versioning feature. It takes two `ResolvedManifest`s and reports the structural delta: skills only in A, skills only in B, subagents only in A/B, tool-allow delta, tool-deny delta. This is **not** a raw file diff.

**Files:**
- Create: `internal/compose/diff.go`
- Test: `internal/compose/diff_test.go`

### Steps

- [ ] **3.1 — Write the failing test.** Create `internal/compose/diff_test.go`:

```go
package compose_test

import (
	"testing"

	"github.com/a2ngerer/agent-containers/internal/compose"
	"github.com/a2ngerer/agent-containers/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestDiff_CapabilityDelta(t *testing.T) {
	a := compose.ResolvedManifest{
		Persona:   domain.Persona{Name: "coder"},
		Skills:    []string{"build-skill", "shared-skill"},
		Subagents: []string{"coder-agent", "shared-agent"},
		Enforcement: domain.Enforcement{
			ToolsAllow: []string{"Write", "Edit", "Read"},
			ToolsDeny:  []string{},
		},
	}
	b := compose.ResolvedManifest{
		Persona:   domain.Persona{Name: "reviewer"},
		Skills:    []string{"security-review", "shared-skill"},
		Subagents: []string{"shared-agent"},
		Enforcement: domain.Enforcement{
			ToolsAllow: []string{"Read"},
			ToolsDeny:  []string{"Write", "Edit"},
		},
	}

	d := compose.Diff(a, b)

	require.Equal(t, "coder", d.NameA)
	require.Equal(t, "reviewer", d.NameB)
	require.Equal(t, []string{"build-skill"}, d.SkillsOnlyA)
	require.Equal(t, []string{"security-review"}, d.SkillsOnlyB)
	require.Equal(t, []string{"coder-agent"}, d.SubagentsOnlyA)
	require.Empty(t, d.SubagentsOnlyB)
	require.Equal(t, []string{"Edit", "Write"}, d.AllowOnlyA) // sorted
	require.Empty(t, d.AllowOnlyB)
	require.Empty(t, d.DenyOnlyA)
	require.Equal(t, []string{"Edit", "Write"}, d.DenyOnlyB)
}
```

- [ ] **3.2 — Run, see it fail.**

```bash
cd /Users/angeral/Repositories/agent-containers && go test ./internal/compose/ -run TestDiff 2>&1 | tail -15
```

Expected: `undefined: compose.Diff` / `undefined: compose.CapabilityDiff`.

- [ ] **3.3 — Minimal implementation.** Create `internal/compose/diff.go`:

```go
package compose

import "sort"

// CapabilityDiff is the structural delta between two ResolvedManifests.
// It answers "what can A do that B cannot, and vice versa" — the auditable
// isolation guarantee the product sells. It is NOT a textual file diff.
type CapabilityDiff struct {
	NameA, NameB   string
	SkillsOnlyA    []string
	SkillsOnlyB    []string
	SubagentsOnlyA []string
	SubagentsOnlyB []string
	AllowOnlyA     []string
	AllowOnlyB     []string
	DenyOnlyA      []string
	DenyOnlyB      []string
}

// Diff computes the capability delta between manifests a and b.
func Diff(a, b ResolvedManifest) CapabilityDiff {
	return CapabilityDiff{
		NameA:          a.Persona.Name,
		NameB:          b.Persona.Name,
		SkillsOnlyA:    onlyIn(a.Skills, b.Skills),
		SkillsOnlyB:    onlyIn(b.Skills, a.Skills),
		SubagentsOnlyA: onlyIn(a.Subagents, b.Subagents),
		SubagentsOnlyB: onlyIn(b.Subagents, a.Subagents),
		AllowOnlyA:     onlyIn(a.Enforcement.ToolsAllow, b.Enforcement.ToolsAllow),
		AllowOnlyB:     onlyIn(b.Enforcement.ToolsAllow, a.Enforcement.ToolsAllow),
		DenyOnlyA:      onlyIn(a.Enforcement.ToolsDeny, b.Enforcement.ToolsDeny),
		DenyOnlyB:      onlyIn(b.Enforcement.ToolsDeny, a.Enforcement.ToolsDeny),
	}
}

// onlyIn returns the sorted set of elements present in a but not in b.
func onlyIn(a, b []string) []string {
	inB := make(map[string]struct{}, len(b))
	for _, s := range b {
		inB[s] = struct{}{}
	}
	var out []string
	for _, s := range a {
		if _, ok := inB[s]; !ok {
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}
```

- [ ] **3.4 — Run, see it pass.**

```bash
cd /Users/angeral/Repositories/agent-containers && go test ./internal/compose/ 2>&1 | tail -5
```

Expected: `ok  	github.com/a2ngerer/agent-containers/internal/compose`.

- [ ] **3.5 — Commit.**

```bash
cd /Users/angeral/Repositories/agent-containers && git add internal/compose/diff.go internal/compose/diff_test.go && git commit -m "feat(compose): add capability diff between two resolved manifests"
```

---

## Task 4 — Embedded persona templates (`coder`, `reviewer`)

Two shipped default-persona templates so users do not start from zero. `coder` = full build skills, `Write`/`Edit` allowed, `setting_sources = user,project,local`. `reviewer` = review-only skills, `permission_mode = read-only`, denies `Write`/`Edit`/`NotebookEdit`, `setting_sources = user,project`. Each template carries a `persona.toml` body and a `CLAUDE.md` body.

**Files:**
- Create: `internal/cli/templates.go`
- Test: `internal/cli/persona_test.go` (template-lookup portion)

### Steps

- [ ] **4.1 — Write the failing test.** Create `internal/cli/persona_test.go`:

```go
package cli

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPersonaTemplate_CoderAndReviewer(t *testing.T) {
	coder, ok := personaTemplate("coder")
	require.True(t, ok)
	require.Contains(t, coder.TOML, `name        = "coder"`)
	require.Contains(t, coder.TOML, `setting_sources = ["user", "project", "local"]`)
	require.Contains(t, coder.TOML, `"Write"`)
	require.NotEmpty(t, coder.ClaudeMD)

	rev, ok := personaTemplate("reviewer")
	require.True(t, ok)
	require.Contains(t, rev.TOML, `name        = "reviewer"`)
	require.Contains(t, rev.TOML, `permission_mode = "read-only"`)
	require.Contains(t, rev.TOML, `setting_sources = ["user", "project"]`)
	// reviewer denies write tools
	deny := rev.TOML[strings.Index(rev.TOML, "deny"):]
	require.Contains(t, deny, `"Write"`)
	require.Contains(t, deny, `"Edit"`)
	require.Contains(t, deny, `"NotebookEdit"`)

	_, ok = personaTemplate("nope")
	require.False(t, ok)
}
```

- [ ] **4.2 — Run, see it fail.**

```bash
cd /Users/angeral/Repositories/agent-containers && go test ./internal/cli/ -run TestPersonaTemplate 2>&1 | tail -15
```

Expected: `undefined: personaTemplate`.

- [ ] **4.3 — Minimal implementation.** Create `internal/cli/templates.go`:

```go
package cli

// personaScaffold is the file content a persona template renders into a new
// persona directory: a persona.toml body and a starter CLAUDE.md body.
type personaScaffold struct {
	TOML     string
	ClaudeMD string
}

const coderTOML = `name        = "coder"
description = "Full build persona. Curated skill set, Write/Edit allowed."
extends     = "_base"

[config]
claude_md       = "CLAUDE.md"
setting_sources = ["user", "project", "local"]

[config.skills]
mode    = "allowlist"
include = ["superpowers", "writing-plans", "executing-plans", "test", "debug"]

[config.subagents]
include = ["code-architect", "tdd-guide"]

[config.mcp]
config = "mcp.json"
strict = false

[enforcement]
permission_mode = "default"

[enforcement.tools]
allow = ["Read", "Grep", "Glob", "Write", "Edit", "Bash"]
deny  = []

[metadata]
version = "0.1.0"
author  = ""
`

const coderMD = `# coder

Full build environment. You may create and edit files, run the build, and use
the curated build skill set. Follow the project's TDD workflow.
`

const reviewerTOML = `name        = "reviewer"
description = "Uncontaminated reviewer. Sees the diff, not the build skills."
extends     = "_base"

[config]
claude_md       = "CLAUDE.md"
setting_sources = ["user", "project"]

[config.skills]
mode    = "replace"
include = ["security-review", "silent-failure-hunter", "type-design-analyzer"]

[config.subagents]
include = ["code-reviewer", "security-reviewer"]

[config.mcp]
config = "mcp.json"
strict = true

[enforcement]
permission_mode = "read-only"

[enforcement.tools]
allow = ["Read", "Grep", "Glob", "Bash(git diff:*)", "Bash(git log:*)"]
deny  = ["Write", "Edit", "NotebookEdit", "Bash(git commit:*)", "Bash(git push:*)"]

[metadata]
version = "0.1.0"
author  = ""
`

const reviewerMD = `# reviewer

Read-only review environment. You cannot write or edit files. Review the diff
on its own merits; you do not have the builder's skills and must not assume its
intent.
`

// personaTemplate returns the embedded scaffold for a known template name.
func personaTemplate(name string) (personaScaffold, bool) {
	switch name {
	case "coder":
		return personaScaffold{TOML: coderTOML, ClaudeMD: coderMD}, true
	case "reviewer":
		return personaScaffold{TOML: reviewerTOML, ClaudeMD: reviewerMD}, true
	default:
		return personaScaffold{}, false
	}
}
```

- [ ] **4.4 — Run, see it pass.**

```bash
cd /Users/angeral/Repositories/agent-containers && go test ./internal/cli/ -run TestPersonaTemplate 2>&1 | tail -5
```

Expected: `ok  	github.com/a2ngerer/agent-containers/internal/cli`.

- [ ] **4.5 — Commit.**

```bash
cd /Users/angeral/Repositories/agent-containers && git add internal/cli/templates.go internal/cli/persona_test.go && git commit -m "feat(cli): embed coder and reviewer persona templates"
```

---

## Task 5 — Scaffold helper: write a persona dir to the repo

`scaffoldPersona` writes `persona.toml` (parsed into `domain.Persona`, then saved via `env.SavePersona` for canonical encoding) plus `CLAUDE.md` into `personas/<name>/` inside the repo. Refuses to overwrite an existing persona (`domain.ErrPersonaExists`).

**Files:**
- Create: `internal/cli/personahelpers.go`
- Test: `internal/cli/persona_test.go` (append)

### Steps

- [ ] **5.1 — Write the failing test.** Extend the import block of `internal/cli/persona_test.go` to:

```go
import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/a2ngerer/agent-containers/internal/domain"
	"github.com/a2ngerer/agent-containers/internal/environment"
	"github.com/stretchr/testify/require"
)
```

Then append:

```go
func newTestEnv(t *testing.T) *environment.Environment {
	t.Helper()
	t.Setenv("ACON_HOME", t.TempDir())
	env, err := environment.Create(t.TempDir())
	require.NoError(t, err)
	return env
}

func TestScaffoldPersona_WritesTOMLandMD(t *testing.T) {
	env := newTestEnv(t)
	sc, _ := personaTemplate("reviewer")

	err := scaffoldPersona(env, "rev1", sc)
	require.NoError(t, err)

	dir := filepath.Join(environment.RepoDir(env.Hash), "personas", "rev1")
	md, err := os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
	require.NoError(t, err)
	require.Equal(t, sc.ClaudeMD, string(md))

	p, err := env.LoadPersona("rev1")
	require.NoError(t, err)
	require.Equal(t, "rev1", p.Name) // name overridden to the new name
	require.Equal(t, "read-only", p.Enforcement.PermissionMode)
	require.ElementsMatch(t, []string{"Write", "Edit", "NotebookEdit", "Bash(git commit:*)", "Bash(git push:*)"}, p.Enforcement.ToolsDeny)

	// second scaffold with the same name is rejected
	err = scaffoldPersona(env, "rev1", sc)
	require.ErrorIs(t, err, domain.ErrPersonaExists)
}
```

- [ ] **5.2 — Run, see it fail.**

```bash
cd /Users/angeral/Repositories/agent-containers && go test ./internal/cli/ -run TestScaffoldPersona 2>&1 | tail -15
```

Expected: `undefined: scaffoldPersona`.

- [ ] **5.3 — Minimal implementation.** Create `internal/cli/personahelpers.go`:

```go
package cli

import (
	"fmt"
	"os"
	"path/filepath"

	toml "github.com/pelletier/go-toml/v2"

	"github.com/a2ngerer/agent-containers/internal/domain"
	"github.com/a2ngerer/agent-containers/internal/environment"
)

// tomlPersona mirrors the [enforcement.tools] sub-table that domain.Enforcement
// keeps out of struct tags (ToolsAllow/ToolsDeny are toml:"-"). Used only for
// loading and encoding persona.toml against the domain type.
type tomlPersona struct {
	domain.Persona
	Enforcement struct {
		domain.Enforcement
		Tools struct {
			Allow []string `toml:"allow"`
			Deny  []string `toml:"deny"`
		} `toml:"tools"`
	} `toml:"enforcement"`
}

// parsePersonaTOML decodes a persona.toml body into a domain.Persona, mapping
// the [enforcement.tools] allow/deny lists into ToolsAllow/ToolsDeny.
func parsePersonaTOML(body []byte) (domain.Persona, error) {
	var tp tomlPersona
	if err := toml.Unmarshal(body, &tp); err != nil {
		return domain.Persona{}, fmt.Errorf("parse persona toml: %w", err)
	}
	p := tp.Persona
	p.Enforcement = tp.Enforcement.Enforcement
	p.Enforcement.ToolsAllow = tp.Enforcement.Tools.Allow
	p.Enforcement.ToolsDeny = tp.Enforcement.Tools.Deny
	return p, nil
}

// scaffoldPersona materializes a persona template into personas/<name>/ in the
// repo. The persona's name is forced to `name`. Refuses to overwrite.
func scaffoldPersona(e *environment.Environment, name string, sc personaScaffold) error {
	if _, err := e.LoadPersona(name); err == nil {
		return fmt.Errorf("%q: %w", name, domain.ErrPersonaExists)
	}
	p, err := parsePersonaTOML([]byte(sc.TOML))
	if err != nil {
		return err
	}
	p.Name = name
	if err := e.SavePersona(p); err != nil {
		return fmt.Errorf("save persona %q: %w", name, err)
	}
	dir := filepath.Join(environment.RepoDir(e.Hash), "personas", name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir persona dir: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte(sc.ClaudeMD), 0o644); err != nil {
		return fmt.Errorf("write CLAUDE.md: %w", err)
	}
	return nil
}
```

> **Implementer note:** `e.SavePersona` (M1) writes the canonical `persona.toml`. The contract lists `ToolsAllow`/`ToolsDeny` as load-mapped fields under `[enforcement.tools]`, so M1's encoder is expected to emit that sub-table. If the round-trip in this test loses `ToolsDeny`, extend M1's `SavePersona` to serialize `[enforcement.tools]` (it already owns persona persistence). Do not weaken the test.

- [ ] **5.4 — Run, see it pass.**

```bash
cd /Users/angeral/Repositories/agent-containers && go test ./internal/cli/ -run TestScaffoldPersona 2>&1 | tail -5
```

Expected: `ok  	github.com/a2ngerer/agent-containers/internal/cli`.

- [ ] **5.5 — Commit.**

```bash
cd /Users/angeral/Repositories/agent-containers && git add internal/cli/personahelpers.go internal/cli/persona_test.go && git commit -m "feat(cli): add scaffoldPersona + persona TOML parsing helper"
```

---

## Task 6 — `new` command (`--template` / `--extends` / `--from`)

`new <name>` scaffolds `personas/<name>/persona.toml` + `CLAUDE.md`. Default `extends = _base`. `--template coder|reviewer` uses an embedded template. `--from <persona>` copies an existing persona as the starting point. `--extends <layer>` overrides the extends layer. `--template` and `--from` are mutually exclusive.

**Files:**
- Create: `internal/cli/persona.go`
- Modify: `internal/cli/personahelpers.go` (add `copyPersonaScaffold` + `encodePersonaTOML`)
- Test: `internal/cli/persona_test.go` (append)

### Steps

- [ ] **6.1 — Write the failing test.** Add `"bytes"` to the import block of `internal/cli/persona_test.go`, then append:

```go
func runNew(t *testing.T, env *environment.Environment, args ...string) (string, error) {
	t.Helper()
	cmd := newNewCmd(func() (*environment.Environment, error) { return env, nil })
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

func TestNewCmd_DefaultExtendsBase(t *testing.T) {
	env := newTestEnv(t)
	_, err := runNew(t, env, "blank")
	require.NoError(t, err)

	p, err := env.LoadPersona("blank")
	require.NoError(t, err)
	require.Equal(t, "_base", p.Extends)
}

func TestNewCmd_TemplateCoder(t *testing.T) {
	env := newTestEnv(t)
	_, err := runNew(t, env, "c1", "--template", "coder")
	require.NoError(t, err)

	p, err := env.LoadPersona("c1")
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"Read", "Grep", "Glob", "Write", "Edit", "Bash"}, p.Enforcement.ToolsAllow)
	require.Equal(t, []string{"user", "project", "local"}, p.Config.SettingSources)
}

func TestNewCmd_ExtendsOverride(t *testing.T) {
	env := newTestEnv(t)
	_, err := runNew(t, env, "x1", "--template", "reviewer", "--extends", "_custom")
	require.NoError(t, err)

	p, err := env.LoadPersona("x1")
	require.NoError(t, err)
	require.Equal(t, "_custom", p.Extends)
}

func TestNewCmd_FromCopiesPersona(t *testing.T) {
	env := newTestEnv(t)
	_, err := runNew(t, env, "src", "--template", "reviewer")
	require.NoError(t, err)

	_, err = runNew(t, env, "dst", "--from", "src")
	require.NoError(t, err)

	p, err := env.LoadPersona("dst")
	require.NoError(t, err)
	require.Equal(t, "read-only", p.Enforcement.PermissionMode) // copied from src
}

func TestNewCmd_TemplateAndFromConflict(t *testing.T) {
	env := newTestEnv(t)
	_, err := runNew(t, env, "bad", "--template", "coder", "--from", "src")
	require.Error(t, err)
}
```

- [ ] **6.2 — Run, see it fail.**

```bash
cd /Users/angeral/Repositories/agent-containers && go test ./internal/cli/ -run TestNewCmd 2>&1 | tail -15
```

Expected: `undefined: newNewCmd`.

- [ ] **6.3 — Minimal implementation.** First add to `internal/cli/personahelpers.go`:

```go
// copyPersonaScaffold builds a personaScaffold from an existing persona's
// saved manifest and its on-disk CLAUDE.md, for `new --from`.
func copyPersonaScaffold(e *environment.Environment, src string) (personaScaffold, error) {
	p, err := e.LoadPersona(src)
	if err != nil {
		return personaScaffold{}, fmt.Errorf("load source persona %q: %w", src, err)
	}
	md := ""
	name := p.Config.ClaudeMD
	if name == "" {
		name = "CLAUDE.md"
	}
	b, err := os.ReadFile(filepath.Join(environment.RepoDir(e.Hash), "personas", src, name))
	if err == nil {
		md = string(b)
	}
	body, err := encodePersonaTOML(p)
	if err != nil {
		return personaScaffold{}, err
	}
	return personaScaffold{TOML: body, ClaudeMD: md}, nil
}

// encodePersonaTOML serializes a domain.Persona to a persona.toml body,
// including the [enforcement.tools] sub-table.
func encodePersonaTOML(p domain.Persona) (string, error) {
	var tp tomlPersona
	tp.Persona = p
	tp.Enforcement.Enforcement = p.Enforcement
	tp.Enforcement.Tools.Allow = p.Enforcement.ToolsAllow
	tp.Enforcement.Tools.Deny = p.Enforcement.ToolsDeny
	b, err := toml.Marshal(tp)
	if err != nil {
		return "", fmt.Errorf("encode persona toml: %w", err)
	}
	return string(b), nil
}
```

Then create `internal/cli/persona.go`:

```go
// Package cli holds the thin cobra command groups. Commands parse flags and
// delegate to internal packages; no business logic lives here.
package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/a2ngerer/agent-containers/internal/environment"
)

// envOpener resolves the bound environment for the current workspace. The CLI
// root injects the real opener; tests inject a stub.
type envOpener func() (*environment.Environment, error)

// newNewCmd builds the `new` command.
func newNewCmd(open envOpener) *cobra.Command {
	var template, from, extends string
	cmd := &cobra.Command{
		Use:   "new <name>",
		Short: "Create a new persona",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if template != "" && from != "" {
				return fmt.Errorf("--template and --from are mutually exclusive")
			}
			env, err := open()
			if err != nil {
				return err
			}

			var sc personaScaffold
			switch {
			case from != "":
				sc, err = copyPersonaScaffold(env, from)
				if err != nil {
					return err
				}
			case template != "":
				var ok bool
				sc, ok = personaTemplate(template)
				if !ok {
					return fmt.Errorf("unknown template %q (have: coder, reviewer)", template)
				}
			default:
				sc = blankScaffold(name, extends)
			}

			if err := scaffoldPersona(env, name, sc); err != nil {
				return err
			}
			// apply the chosen extends layer (also the default "_base") after scaffolding
			p, err := env.LoadPersona(name)
			if err != nil {
				return err
			}
			p.Extends = extends
			if err := env.SavePersona(p); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Created persona %q\n", name)
			return nil
		},
	}
	cmd.Flags().StringVar(&template, "template", "", "scaffold from an embedded template (coder|reviewer)")
	cmd.Flags().StringVar(&from, "from", "", "copy an existing persona as the starting point")
	cmd.Flags().StringVar(&extends, "extends", "_base", "layer this persona extends")
	return cmd
}

// blankScaffold returns a minimal persona.toml + CLAUDE.md for `new` without a
// template or source.
func blankScaffold(name, extends string) personaScaffold {
	if extends == "" {
		extends = "_base"
	}
	body := fmt.Sprintf(`name        = %q
description = ""
extends     = %q

[config]
claude_md       = "CLAUDE.md"
setting_sources = ["user", "project"]

[config.skills]
mode    = "allowlist"
include = []

[config.subagents]
include = []

[config.mcp]
config = ""
strict = false

[enforcement]
permission_mode = "default"

[enforcement.tools]
allow = ["Read", "Grep", "Glob"]
deny  = []

[metadata]
version = "0.1.0"
author  = ""
`, name, extends)
	return personaScaffold{TOML: body, ClaudeMD: "# " + name + "\n"}
}
```

> The `--extends` default is `"_base"`, so the post-scaffold block always re-saves the persona with the chosen (or default) layer. That is intentional and idempotent.

- [ ] **6.4 — Run, see it pass.**

```bash
cd /Users/angeral/Repositories/agent-containers && go test ./internal/cli/ -run TestNewCmd 2>&1 | tail -5
```

Expected: `ok  	github.com/a2ngerer/agent-containers/internal/cli`.

- [ ] **6.5 — Commit.**

```bash
cd /Users/angeral/Repositories/agent-containers && git add internal/cli/persona.go internal/cli/personahelpers.go internal/cli/persona_test.go && git commit -m "feat(cli): add 'new' command with --template, --from, --extends"
```

---

## Task 7 — `show` command (composed manifest + capability preview)

`show <persona>` composes the persona and prints: name/version, active skills, active subagents, allowed tools, denied tools, withheld base skills (skills present in the base layer but dropped by `replace` mode), and effective setting sources.

**Files:**
- Modify: `internal/cli/persona.go` (add `newShowCmd`)
- Modify: `internal/cli/personahelpers.go` (add `formatShow`, `withheldBaseSkills`, `joinOrNone`)
- Test: `internal/cli/persona_test.go` (append)

### Steps

- [ ] **7.1 — Write the failing test.** Append to `internal/cli/persona_test.go`:

```go
func runShow(t *testing.T, env *environment.Environment, args ...string) (string, error) {
	t.Helper()
	cmd := newShowCmd(func() (*environment.Environment, error) { return env, nil })
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

func TestShowCmd_RendersCapabilities(t *testing.T) {
	env := newTestEnv(t)
	// seed a _base layer with a build skill that the reviewer will withhold
	base := domain.Persona{
		Name:   "_base",
		Config: domain.Config{ClaudeMD: "CLAUDE.md", Skills: domain.SkillSet{Mode: "allowlist", Include: []string{"superpowers"}}},
	}
	require.NoError(t, env.SavePersona(base))
	_, err := runNew(t, env, "reviewer", "--template", "reviewer")
	require.NoError(t, err)

	out, err := runShow(t, env, "reviewer")
	require.NoError(t, err)

	require.Contains(t, out, "Persona: reviewer")
	require.Contains(t, out, "security-review") // active skill
	require.Contains(t, out, "code-reviewer")   // active subagent
	require.Contains(t, out, "Write")           // denied tool listed
	require.Contains(t, out, "Withheld")        // withheld section present
	require.Contains(t, out, "superpowers")     // base skill withheld by replace mode
	require.Contains(t, out, "user, project")   // setting sources
}
```

- [ ] **7.2 — Run, see it fail.**

```bash
cd /Users/angeral/Repositories/agent-containers && go test ./internal/cli/ -run TestShowCmd 2>&1 | tail -15
```

Expected: `undefined: newShowCmd`.

- [ ] **7.3 — Minimal implementation.** Extend the import block of `internal/cli/personahelpers.go` to add `"sort"`, `"strings"`, and `"github.com/a2ngerer/agent-containers/internal/compose"`, then add:

```go
// withheldBaseSkills returns base-layer skills that the composed manifest does
// NOT include — i.e. dropped by Skills.Mode == "replace". Empty otherwise.
func withheldBaseSkills(e *environment.Environment, rm compose.ResolvedManifest) []string {
	if rm.Persona.Extends == "" {
		return nil
	}
	base, err := e.LoadPersona(rm.Persona.Extends)
	if err != nil {
		return nil
	}
	active := make(map[string]struct{}, len(rm.Skills))
	for _, s := range rm.Skills {
		active[s] = struct{}{}
	}
	var withheld []string
	for _, s := range base.Config.Skills.Include {
		if _, ok := active[s]; !ok {
			withheld = append(withheld, s)
		}
	}
	sort.Strings(withheld)
	return withheld
}

// formatShow renders the composed manifest as a human capability preview.
func formatShow(e *environment.Environment, rm compose.ResolvedManifest) string {
	var b strings.Builder
	p := rm.Persona
	ver := p.Metadata.Version
	if ver == "" {
		ver = "0.0.0"
	}
	fmt.Fprintf(&b, "Persona: %s   %s:%s\n", p.Name, p.Name, ver)
	if p.Description != "" {
		fmt.Fprintf(&b, "  %s\n", p.Description)
	}
	fmt.Fprintf(&b, "  Skills:    %s\n", joinOrNone(rm.Skills))
	fmt.Fprintf(&b, "  Subagents: %s\n", joinOrNone(rm.Subagents))
	fmt.Fprintf(&b, "  Allow:     %s\n", joinOrNone(rm.Enforcement.ToolsAllow))
	fmt.Fprintf(&b, "  Denied:    %s\n", joinOrNone(rm.Enforcement.ToolsDeny))
	if wh := withheldBaseSkills(e, rm); len(wh) > 0 {
		fmt.Fprintf(&b, "  Withheld:  %s   (dropped by replace mode)\n", strings.Join(wh, ", "))
	}
	fmt.Fprintf(&b, "  Settings:  %s\n", joinOrNone(rm.SettingSrc))
	mode := rm.Enforcement.PermissionMode
	if mode == "" {
		mode = "default"
	}
	fmt.Fprintf(&b, "  Mode:      %s\n", mode)
	return b.String()
}

func joinOrNone(xs []string) string {
	if len(xs) == 0 {
		return "(none)"
	}
	return strings.Join(xs, ", ")
}
```

Then extend the import block of `internal/cli/persona.go` to add `"github.com/a2ngerer/agent-containers/internal/compose"`, and add:

```go
// newShowCmd builds the `show` command.
func newShowCmd(open envOpener) *cobra.Command {
	return &cobra.Command{
		Use:   "show <persona>",
		Short: "Show a persona's composed manifest and capability preview",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			env, err := open()
			if err != nil {
				return err
			}
			rm, err := compose.Compose(env, args[0])
			if err != nil {
				return err
			}
			fmt.Fprint(cmd.OutOrStdout(), formatShow(env, rm))
			return nil
		},
	}
}
```

- [ ] **7.4 — Run, see it pass.**

```bash
cd /Users/angeral/Repositories/agent-containers && go test ./internal/cli/ -run TestShowCmd 2>&1 | tail -5
```

Expected: `ok  	github.com/a2ngerer/agent-containers/internal/cli`.

- [ ] **7.5 — Commit.**

```bash
cd /Users/angeral/Repositories/agent-containers && git add internal/cli/persona.go internal/cli/personahelpers.go internal/cli/persona_test.go && git commit -m "feat(cli): add 'show' command rendering composed manifest + withheld skills"
```

---

## Task 8 — `edit` + `rm` commands

`edit <persona>` opens `persona.toml` in `$EDITOR` (falling back to `vi`). `rm <persona>` removes the persona directory from the repo. `rm` refuses if the persona is currently the active persona for the environment.

**Files:**
- Modify: `internal/cli/persona.go` (add `newEditCmd`, `newRmCmd`)
- Modify: `internal/cli/personahelpers.go` (add `removePersona`, `personaTOMLPath`)
- Modify: `internal/environment/environment.go` (add `ActivePersona()` accessor if M1 lacks it)
- Test: `internal/cli/persona_test.go` (append)

### Steps

- [ ] **8.1 — Write the failing test.** Append to `internal/cli/persona_test.go`:

```go
func runRm(t *testing.T, env *environment.Environment, args ...string) (string, error) {
	t.Helper()
	cmd := newRmCmd(func() (*environment.Environment, error) { return env, nil })
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

func TestRmCmd_RemovesPersona(t *testing.T) {
	env := newTestEnv(t)
	_, err := runNew(t, env, "tmp", "--template", "coder")
	require.NoError(t, err)

	_, err = runRm(t, env, "tmp")
	require.NoError(t, err)

	_, err = env.LoadPersona("tmp")
	require.ErrorIs(t, err, domain.ErrPersonaNotFound)

	dir := filepath.Join(environment.RepoDir(env.Hash), "personas", "tmp")
	_, statErr := os.Stat(dir)
	require.True(t, os.IsNotExist(statErr))
}

func TestRmCmd_RefusesActivePersona(t *testing.T) {
	env := newTestEnv(t)
	_, err := runNew(t, env, "act", "--template", "coder")
	require.NoError(t, err)
	require.NoError(t, env.SetActive("act"))

	_, err = runRm(t, env, "act")
	require.Error(t, err)
	require.Contains(t, err.Error(), "active")
}

func TestPersonaTOMLPath(t *testing.T) {
	env := newTestEnv(t)
	got := personaTOMLPath(env, "p")
	want := filepath.Join(environment.RepoDir(env.Hash), "personas", "p", "persona.toml")
	require.Equal(t, want, got)
}
```

- [ ] **8.2 — Run, see it fail.**

```bash
cd /Users/angeral/Repositories/agent-containers && go test ./internal/cli/ -run 'TestRmCmd|TestPersonaTOMLPath' 2>&1 | tail -15
```

Expected: `undefined: newRmCmd` / `undefined: personaTOMLPath` (and possibly `env.ActivePersona undefined`).

- [ ] **8.3 — Minimal implementation.** First ensure the accessor exists. Check M1:

```bash
cd /Users/angeral/Repositories/agent-containers && grep -n "func (e \*Environment) ActivePersona" internal/environment/environment.go || echo "MISSING -> add accessor"
```

If missing, add to `internal/environment/environment.go`:

```go
// ActivePersona returns the persona currently activated for this environment
// (empty if none).
func (e *Environment) ActivePersona() string { return e.cfg.ActivePersona }
```

Then add to `internal/cli/personahelpers.go`:

```go
// personaTOMLPath returns the on-disk path of a persona's persona.toml.
func personaTOMLPath(e *environment.Environment, name string) string {
	return filepath.Join(environment.RepoDir(e.Hash), "personas", name, "persona.toml")
}

// removePersona deletes a persona directory from the repo. It refuses to remove
// the active persona.
func removePersona(e *environment.Environment, name string) error {
	if _, err := e.LoadPersona(name); err != nil {
		return err
	}
	if e.ActivePersona() == name {
		return fmt.Errorf("cannot remove %q: it is the active persona (run: acon deactivate)", name)
	}
	dir := filepath.Join(environment.RepoDir(e.Hash), "personas", name)
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("remove persona dir: %w", err)
	}
	return nil
}
```

Then extend the import block of `internal/cli/persona.go` to add `"os"` and `"os/exec"`, and add:

```go
// newEditCmd builds the `edit` command.
func newEditCmd(open envOpener) *cobra.Command {
	return &cobra.Command{
		Use:   "edit <persona>",
		Short: "Open a persona's persona.toml in $EDITOR",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			env, err := open()
			if err != nil {
				return err
			}
			if _, err := env.LoadPersona(args[0]); err != nil {
				return err
			}
			editor := os.Getenv("EDITOR")
			if editor == "" {
				editor = "vi"
			}
			path := personaTOMLPath(env, args[0])
			ed := exec.Command(editor, path)
			ed.Stdin, ed.Stdout, ed.Stderr = os.Stdin, os.Stdout, os.Stderr
			return ed.Run()
		},
	}
}

// newRmCmd builds the `rm` command.
func newRmCmd(open envOpener) *cobra.Command {
	return &cobra.Command{
		Use:   "rm <persona>",
		Short: "Remove a persona",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			env, err := open()
			if err != nil {
				return err
			}
			if err := removePersona(env, args[0]); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Removed persona %q\n", args[0])
			return nil
		},
	}
}
```

- [ ] **8.4 — Run, see it pass.**

```bash
cd /Users/angeral/Repositories/agent-containers && go test ./internal/cli/ -run 'TestRmCmd|TestPersonaTOMLPath' 2>&1 | tail -5
```

Expected: `ok  	github.com/a2ngerer/agent-containers/internal/cli`.

- [ ] **8.5 — Commit.**

```bash
cd /Users/angeral/Repositories/agent-containers && git add internal/cli/persona.go internal/cli/personahelpers.go internal/environment/environment.go internal/cli/persona_test.go && git commit -m "feat(cli): add 'edit' and 'rm' persona commands"
```

---

## Task 9 — `snapshot` command (alias `commit`)

`snapshot [persona] [-m msg]` writes the persona directory as a content tree (`Store.WriteTree`) and records a `domain.Snapshot` (`Store.WriteSnapshot`) with the tree id, message, author, and an RFC 3339 timestamp. The persona argument defaults to the active persona. Empty `-m` defaults to `"snapshot <persona>"`.

**Files:**
- Create: `internal/cli/version.go`
- Modify: `internal/cli/personahelpers.go` (add `takeSnapshot`, `resolveActivePersona`, `shortID`)
- Modify: `internal/environment/environment.go` (add `Author()` accessor if M1 lacks it)
- Test: `internal/cli/version_test.go`

### Steps

- [ ] **9.1 — Write the failing test.** Create `internal/cli/version_test.go`:

```go
package cli

import (
	"bytes"
	"testing"

	"github.com/a2ngerer/agent-containers/internal/environment"
	"github.com/stretchr/testify/require"
)

func newVerTestEnv(t *testing.T) *environment.Environment {
	t.Helper()
	t.Setenv("ACON_HOME", t.TempDir())
	env, err := environment.Create(t.TempDir())
	require.NoError(t, err)
	return env
}

func runSnapshot(t *testing.T, env *environment.Environment, args ...string) (string, error) {
	t.Helper()
	cmd := newSnapshotCmd(func() (*environment.Environment, error) { return env, nil })
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

func TestSnapshotCmd_WritesToTimeline(t *testing.T) {
	env := newVerTestEnv(t)
	_, err := runNew(t, env, "coder", "--template", "coder")
	require.NoError(t, err)

	out, err := runSnapshot(t, env, "coder", "-m", "first")
	require.NoError(t, err)
	require.Contains(t, out, "Snapshot")

	ids, err := env.Store.Timeline("coder")
	require.NoError(t, err)
	require.Len(t, ids, 1)

	snap, err := env.Store.ReadSnapshot(ids[0])
	require.NoError(t, err)
	require.Equal(t, "coder", snap.Persona)
	require.Equal(t, "first", snap.Message)
	require.NotEmpty(t, snap.TreeID)
	require.False(t, snap.Timestamp.IsZero())
}

func TestSnapshotCmd_DefaultsToActivePersona(t *testing.T) {
	env := newVerTestEnv(t)
	_, err := runNew(t, env, "coder", "--template", "coder")
	require.NoError(t, err)
	require.NoError(t, env.SetActive("coder"))

	_, err = runSnapshot(t, env) // no persona arg
	require.NoError(t, err)

	ids, err := env.Store.Timeline("coder")
	require.NoError(t, err)
	require.Len(t, ids, 1)
}
```

- [ ] **9.2 — Run, see it fail.**

```bash
cd /Users/angeral/Repositories/agent-containers && go test ./internal/cli/ -run TestSnapshotCmd 2>&1 | tail -15
```

Expected: `undefined: newSnapshotCmd`.

- [ ] **9.3 — Minimal implementation.** First ensure the author accessor exists. Check M1:

```bash
cd /Users/angeral/Repositories/agent-containers && grep -n "func (e \*Environment) Author" internal/environment/environment.go || echo "MISSING -> add accessor"
```

If missing, add to `internal/environment/environment.go` (return the configured author, or `""`):

```go
// Author returns the configured commit author for this environment ("" if unset).
func (e *Environment) Author() string { return e.cfg.Author }
```

> If M1's `EnvConfig` has no `Author` field, return `""` from a literal accessor instead: `func (e *Environment) Author() string { return "" }`. The author is metadata only and does not affect any test assertion.

Then extend the import block of `internal/cli/personahelpers.go` to add `"time"` (keep `domain`, already imported), and add:

```go
// resolveActivePersona returns the persona name from args[0] if present,
// otherwise the environment's active persona.
func resolveActivePersona(e *environment.Environment, args []string) (string, error) {
	if len(args) > 0 && args[0] != "" {
		return args[0], nil
	}
	if a := e.ActivePersona(); a != "" {
		return a, nil
	}
	return "", fmt.Errorf("no persona given and no active persona set")
}

// takeSnapshot writes the persona dir as a tree and records a snapshot.
// It returns the new snapshot id.
func takeSnapshot(e *environment.Environment, persona, msg string) (domain.SnapshotID, error) {
	if _, err := e.LoadPersona(persona); err != nil {
		return "", err
	}
	if msg == "" {
		msg = "snapshot " + persona
	}
	dir := filepath.Join(environment.RepoDir(e.Hash), "personas", persona)
	tree, err := e.Store.WriteTree(dir)
	if err != nil {
		return "", fmt.Errorf("write tree: %w", err)
	}
	prev, _ := e.Store.Timeline(persona) // newest first; may be empty
	var parents []domain.SnapshotID
	if len(prev) > 0 {
		parents = []domain.SnapshotID{prev[0]}
	}
	snap := domain.Snapshot{
		Persona:   persona,
		Parents:   parents,
		Message:   msg,
		Author:    e.Author(),
		Timestamp: time.Now().UTC(),
		TreeID:    string(tree),
	}
	id, err := e.Store.WriteSnapshot(snap)
	if err != nil {
		return "", fmt.Errorf("write snapshot: %w", err)
	}
	return id, nil
}

// shortID truncates an id for display.
func shortID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}
```

> The snapshot timestamp is `time.Now().UTC()`; the storage engine serializes it as RFC 3339 per the contract (§6). `log` formats it back via `time.RFC3339` (Task 10).

Then create `internal/cli/version.go`:

```go
package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/a2ngerer/agent-containers/internal/environment"
)

// newSnapshotCmd builds the `snapshot` command (alias `commit`).
func newSnapshotCmd(open envOpener) *cobra.Command {
	var msg string
	cmd := &cobra.Command{
		Use:     "snapshot [persona]",
		Aliases: []string{"commit"},
		Short:   "Record an immutable snapshot of a persona",
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			env, err := open()
			if err != nil {
				return err
			}
			persona, err := resolveActivePersona(env, args)
			if err != nil {
				return err
			}
			id, err := takeSnapshot(env, persona, msg)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Snapshot %s recorded for %q\n", shortID(string(id)), persona)
			return nil
		},
	}
	cmd.Flags().StringVarP(&msg, "message", "m", "", "snapshot message")
	return cmd
}
```

> `version.go` imports `environment` so that later commands (rollback, tag) in the same file can reference it; `snapshot` itself does not call it directly. If `go vet` flags the import as unused at this task, defer adding it until Task 10 (which uses `environment.RepoDir` indirectly) — but `log` in Task 10 lives in this file and `rollback` in Task 12 uses `environment.RepoDir`, so keep the import.

- [ ] **9.4 — Run, see it pass.**

```bash
cd /Users/angeral/Repositories/agent-containers && go test ./internal/cli/ -run TestSnapshotCmd 2>&1 | tail -5
```

Expected: `ok  	github.com/a2ngerer/agent-containers/internal/cli`.

- [ ] **9.5 — Commit.**

```bash
cd /Users/angeral/Repositories/agent-containers && git add internal/cli/version.go internal/cli/personahelpers.go internal/environment/environment.go internal/cli/version_test.go && git commit -m "feat(cli): add 'snapshot' command writing tree + snapshot to the timeline"
```

---

## Task 10 — `log` command

`log [persona]` reads the timeline (`Store.Timeline`), reads each snapshot, and prints one line per entry (newest first): short id, RFC 3339 timestamp, message. Persona defaults to active.

**Files:**
- Modify: `internal/cli/version.go` (add `newLogCmd`)
- Modify: `internal/cli/personahelpers.go` (add `formatTimeline`)
- Test: `internal/cli/version_test.go` (append)

### Steps

- [ ] **10.1 — Write the failing test.** Append to `internal/cli/version_test.go`:

```go
func runLog(t *testing.T, env *environment.Environment, args ...string) (string, error) {
	t.Helper()
	cmd := newLogCmd(func() (*environment.Environment, error) { return env, nil })
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

func TestLogCmd_Roundtrip(t *testing.T) {
	env := newVerTestEnv(t)
	_, err := runNew(t, env, "coder", "--template", "coder")
	require.NoError(t, err)
	_, err = runSnapshot(t, env, "coder", "-m", "one")
	require.NoError(t, err)
	_, err = runSnapshot(t, env, "coder", "-m", "two")
	require.NoError(t, err)

	out, err := runLog(t, env, "coder")
	require.NoError(t, err)

	// newest first: "two" appears before "one"
	idxTwo := indexOf(out, "two")
	idxOne := indexOf(out, "one")
	require.NotEqual(t, -1, idxTwo)
	require.NotEqual(t, -1, idxOne)
	require.Less(t, idxTwo, idxOne)
}

// indexOf is a tiny local substring finder to avoid importing strings here.
func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
```

- [ ] **10.2 — Run, see it fail.**

```bash
cd /Users/angeral/Repositories/agent-containers && go test ./internal/cli/ -run TestLogCmd 2>&1 | tail -15
```

Expected: `undefined: newLogCmd`.

- [ ] **10.3 — Minimal implementation.** Add to `internal/cli/personahelpers.go` (uses `strings`, `time` — already imported from Tasks 7/9):

```go
// formatTimeline renders a persona's snapshots newest-first, one line each.
func formatTimeline(e *environment.Environment, persona string) (string, error) {
	ids, err := e.Store.Timeline(persona)
	if err != nil {
		return "", fmt.Errorf("timeline for %q: %w", persona, err)
	}
	if len(ids) == 0 {
		return fmt.Sprintf("No snapshots for %q yet.\n", persona), nil
	}
	var b strings.Builder
	for _, id := range ids {
		s, err := e.Store.ReadSnapshot(id)
		if err != nil {
			return "", fmt.Errorf("read snapshot %s: %w", id, err)
		}
		fmt.Fprintf(&b, "%s  %s  %s\n", shortID(string(id)), s.Timestamp.UTC().Format(time.RFC3339), s.Message)
	}
	return b.String(), nil
}
```

Then add to `internal/cli/version.go`:

```go
// newLogCmd builds the `log` command.
func newLogCmd(open envOpener) *cobra.Command {
	return &cobra.Command{
		Use:   "log [persona]",
		Short: "Show a persona's snapshot history",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			env, err := open()
			if err != nil {
				return err
			}
			persona, err := resolveActivePersona(env, args)
			if err != nil {
				return err
			}
			out, err := formatTimeline(env, persona)
			if err != nil {
				return err
			}
			fmt.Fprint(cmd.OutOrStdout(), out)
			return nil
		},
	}
}
```

- [ ] **10.4 — Run, see it pass.**

```bash
cd /Users/angeral/Repositories/agent-containers && go test ./internal/cli/ -run TestLogCmd 2>&1 | tail -5
```

Expected: `ok  	github.com/a2ngerer/agent-containers/internal/cli`.

- [ ] **10.5 — Commit.**

```bash
cd /Users/angeral/Repositories/agent-containers && git add internal/cli/version.go internal/cli/personahelpers.go internal/cli/version_test.go && git commit -m "feat(cli): add 'log' command formatting the persona timeline"
```

---

## Task 11 — `diff` command (capability diff of two personas)

`diff [a] [b]` resolves two persona refs to `ResolvedManifest`s, computes the `compose.Diff`, and renders the capability delta (skills only in A / only in B, subagent delta, tool allow/deny delta). With one arg, compares against the active persona; with zero args, errors with usage.

**Files:**
- Modify: `internal/cli/version.go` (add `newDiffCmd`)
- Modify: `internal/cli/personahelpers.go` (add `resolveManifestRef`, `formatCapabilityDiff`)
- Test: `internal/cli/version_test.go` (append)

### Steps

- [ ] **11.1 — Write the failing test.** Add `"github.com/a2ngerer/agent-containers/internal/domain"` to the import block of `internal/cli/version_test.go`, then append:

```go
func runDiff(t *testing.T, env *environment.Environment, args ...string) (string, error) {
	t.Helper()
	cmd := newDiffCmd(func() (*environment.Environment, error) { return env, nil })
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

func TestDiffCmd_CapabilityDelta(t *testing.T) {
	env := newVerTestEnv(t)
	base := domain.Persona{
		Name:   "_base",
		Config: domain.Config{ClaudeMD: "CLAUDE.md", Skills: domain.SkillSet{Mode: "allowlist", Include: []string{"shared"}}},
	}
	require.NoError(t, env.SavePersona(base))
	_, err := runNew(t, env, "coder", "--template", "coder")
	require.NoError(t, err)
	_, err = runNew(t, env, "reviewer", "--template", "reviewer")
	require.NoError(t, err)

	out, err := runDiff(t, env, "coder", "reviewer")
	require.NoError(t, err)

	require.Contains(t, out, "coder")
	require.Contains(t, out, "reviewer")
	require.Contains(t, out, "security-review") // only in reviewer
	require.Contains(t, out, "Write")           // allow-only-in-coder OR deny-only-in-reviewer
}
```

- [ ] **11.2 — Run, see it fail.**

```bash
cd /Users/angeral/Repositories/agent-containers && go test ./internal/cli/ -run TestDiffCmd 2>&1 | tail -15
```

Expected: `undefined: newDiffCmd`.

- [ ] **11.3 — Minimal implementation.** Add to `internal/cli/personahelpers.go` (uses `compose`, `strings` — already imported):

```go
// resolveManifestRef composes a persona ref into a ResolvedManifest. For M2 the
// ref is a bare persona name; ":version" / snapshot refs are resolved by the
// versioning commands directly and are not handled here.
func resolveManifestRef(e *environment.Environment, ref string) (compose.ResolvedManifest, error) {
	return compose.Compose(e, ref)
}

// formatCapabilityDiff renders a CapabilityDiff for the terminal.
func formatCapabilityDiff(d compose.CapabilityDiff) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Capability diff: %s  vs  %s\n", d.NameA, d.NameB)
	section := func(label string, onlyA, onlyB []string) {
		fmt.Fprintf(&b, "  %s\n", label)
		fmt.Fprintf(&b, "    only in %s: %s\n", d.NameA, joinOrNone(onlyA))
		fmt.Fprintf(&b, "    only in %s: %s\n", d.NameB, joinOrNone(onlyB))
	}
	section("Skills", d.SkillsOnlyA, d.SkillsOnlyB)
	section("Subagents", d.SubagentsOnlyA, d.SubagentsOnlyB)
	section("Tools allowed", d.AllowOnlyA, d.AllowOnlyB)
	section("Tools denied", d.DenyOnlyA, d.DenyOnlyB)
	return b.String()
}
```

Then add to `internal/cli/version.go` — extend its import block to add `"github.com/a2ngerer/agent-containers/internal/compose"`, then add:

```go
// newDiffCmd builds the `diff` command (capability diff).
func newDiffCmd(open envOpener) *cobra.Command {
	return &cobra.Command{
		Use:   "diff [a] [b]",
		Short: "Capability diff between two personas",
		Args:  cobra.MaximumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			env, err := open()
			if err != nil {
				return err
			}
			var refA, refB string
			switch len(args) {
			case 2:
				refA, refB = args[0], args[1]
			case 1:
				active := env.ActivePersona()
				if active == "" {
					return fmt.Errorf("one argument given but no active persona to compare against")
				}
				refA, refB = active, args[0]
			default:
				return fmt.Errorf("diff requires two persona names (or one, compared to the active persona)")
			}
			a, err := resolveManifestRef(env, refA)
			if err != nil {
				return err
			}
			b, err := resolveManifestRef(env, refB)
			if err != nil {
				return err
			}
			fmt.Fprint(cmd.OutOrStdout(), formatCapabilityDiff(compose.Diff(a, b)))
			return nil
		},
	}
}
```

- [ ] **11.4 — Run, see it pass.**

```bash
cd /Users/angeral/Repositories/agent-containers && go test ./internal/cli/ -run TestDiffCmd 2>&1 | tail -5
```

Expected: `ok  	github.com/a2ngerer/agent-containers/internal/cli`.

- [ ] **11.5 — Commit.**

```bash
cd /Users/angeral/Repositories/agent-containers && git add internal/cli/version.go internal/cli/personahelpers.go internal/cli/version_test.go && git commit -m "feat(cli): add 'diff' command rendering capability delta between personas"
```

---

## Task 12 — `rollback` command

`rollback <persona> <snapshot|version>` resolves the target snapshot (by short id from the timeline, or via `Store.ResolveTag` for a version), checks the target tree out into the persona dir (`Store.CheckoutTree`), then records a new snapshot `"rollback to <ref>"`. History moves forward; nothing is rewritten.

**Files:**
- Modify: `internal/cli/version.go` (add `newRollbackCmd`, `domainObjectID`)
- Modify: `internal/cli/personahelpers.go` (add `resolveSnapshotRef`)
- Test: `internal/cli/version_test.go` (append)

### Steps

- [ ] **12.1 — Write the failing test.** Add `"os"` and `"path/filepath"` to the import block of `internal/cli/version_test.go`, then append:

```go
func runRollback(t *testing.T, env *environment.Environment, args ...string) (string, error) {
	t.Helper()
	cmd := newRollbackCmd(func() (*environment.Environment, error) { return env, nil })
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

func TestRollbackCmd_RestoresPriorTree(t *testing.T) {
	env := newVerTestEnv(t)
	_, err := runNew(t, env, "coder", "--template", "coder")
	require.NoError(t, err)

	// snapshot v1 with a marker file
	mdPath := filepath.Join(environment.RepoDir(env.Hash), "personas", "coder", "CLAUDE.md")
	require.NoError(t, os.WriteFile(mdPath, []byte("V1"), 0o644))
	_, err = runSnapshot(t, env, "coder", "-m", "v1")
	require.NoError(t, err)
	ids, err := env.Store.Timeline("coder")
	require.NoError(t, err)
	v1 := shortID(string(ids[0]))

	// change the file and snapshot v2
	require.NoError(t, os.WriteFile(mdPath, []byte("V2"), 0o644))
	_, err = runSnapshot(t, env, "coder", "-m", "v2")
	require.NoError(t, err)

	// rollback to v1
	_, err = runRollback(t, env, "coder", v1)
	require.NoError(t, err)

	// the working persona dir now holds V1 again
	got, err := os.ReadFile(mdPath)
	require.NoError(t, err)
	require.Equal(t, "V1", string(got))

	// a new "rollback to" snapshot was appended (now 3 total)
	ids, err = env.Store.Timeline("coder")
	require.NoError(t, err)
	require.Len(t, ids, 3)
	top, err := env.Store.ReadSnapshot(ids[0])
	require.NoError(t, err)
	require.Contains(t, top.Message, "rollback to")
}
```

- [ ] **12.2 — Run, see it fail.**

```bash
cd /Users/angeral/Repositories/agent-containers && go test ./internal/cli/ -run TestRollbackCmd 2>&1 | tail -15
```

Expected: `undefined: newRollbackCmd`.

- [ ] **12.3 — Minimal implementation.** Add to `internal/cli/personahelpers.go` (uses `strings`, `domain` — already imported):

```go
// resolveSnapshotRef resolves ref (a short/full snapshot id or a version tag)
// to a concrete SnapshotID within a persona's history.
func resolveSnapshotRef(e *environment.Environment, persona, ref string) (domain.SnapshotID, error) {
	// try version tag first
	if id, err := e.Store.ResolveTag(persona, ref); err == nil {
		return id, nil
	}
	// fall back to (short) id match against the timeline
	ids, err := e.Store.Timeline(persona)
	if err != nil {
		return "", fmt.Errorf("timeline for %q: %w", persona, err)
	}
	for _, id := range ids {
		s := string(id)
		if s == ref || strings.HasPrefix(s, ref) {
			return id, nil
		}
	}
	return "", fmt.Errorf("snapshot or version %q not found for persona %q", ref, persona)
}
```

Then add to `internal/cli/version.go` — extend its import block to add `"path/filepath"` and `"github.com/a2ngerer/agent-containers/internal/storage"`, then add:

```go
// domainObjectID converts a stored tree id string to a storage.ObjectID.
func domainObjectID(treeID string) storage.ObjectID { return storage.ObjectID(treeID) }

// newRollbackCmd builds the `rollback` command.
func newRollbackCmd(open envOpener) *cobra.Command {
	return &cobra.Command{
		Use:   "rollback <persona> <snapshot|version>",
		Short: "Restore a persona to a prior snapshot, recording a new snapshot",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			env, err := open()
			if err != nil {
				return err
			}
			persona, ref := args[0], args[1]
			target, err := resolveSnapshotRef(env, persona, ref)
			if err != nil {
				return err
			}
			snap, err := env.Store.ReadSnapshot(target)
			if err != nil {
				return err
			}
			dir := filepath.Join(environment.RepoDir(env.Hash), "personas", persona)
			if err := env.Store.CheckoutTree(domainObjectID(snap.TreeID), dir); err != nil {
				return fmt.Errorf("checkout tree: %w", err)
			}
			id, err := takeSnapshot(env, persona, "rollback to "+ref)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Rolled back %q to %s (new snapshot %s)\n", persona, ref, shortID(string(id)))
			return nil
		},
	}
}
```

- [ ] **12.4 — Run, see it pass.**

```bash
cd /Users/angeral/Repositories/agent-containers && go test ./internal/cli/ -run TestRollbackCmd 2>&1 | tail -5
```

Expected: `ok  	github.com/a2ngerer/agent-containers/internal/cli`.

- [ ] **12.5 — Commit.**

```bash
cd /Users/angeral/Repositories/agent-containers && git add internal/cli/version.go internal/cli/personahelpers.go internal/cli/version_test.go && git commit -m "feat(cli): add 'rollback' command restoring a prior tree and snapshotting it"
```

---

## Task 13 — `tag` command

`tag <persona> <version>` points a SemVer version at the persona's newest snapshot via `Store.SetTag`. Errors if the persona has no snapshots yet.

**Files:**
- Modify: `internal/cli/version.go` (add `newTagCmd`)
- Test: `internal/cli/version_test.go` (append)

### Steps

- [ ] **13.1 — Write the failing test.** Append to `internal/cli/version_test.go`:

```go
func runTag(t *testing.T, env *environment.Environment, args ...string) (string, error) {
	t.Helper()
	cmd := newTagCmd(func() (*environment.Environment, error) { return env, nil })
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

func TestTagCmd_TagsNewestSnapshot(t *testing.T) {
	env := newVerTestEnv(t)
	_, err := runNew(t, env, "coder", "--template", "coder")
	require.NoError(t, err)
	_, err = runSnapshot(t, env, "coder", "-m", "v1")
	require.NoError(t, err)

	_, err = runTag(t, env, "coder", "1.0.0")
	require.NoError(t, err)

	ids, err := env.Store.Timeline("coder")
	require.NoError(t, err)
	resolved, err := env.Store.ResolveTag("coder", "1.0.0")
	require.NoError(t, err)
	require.Equal(t, ids[0], resolved)
}

func TestTagCmd_NoSnapshots(t *testing.T) {
	env := newVerTestEnv(t)
	_, err := runNew(t, env, "coder", "--template", "coder")
	require.NoError(t, err)

	_, err = runTag(t, env, "coder", "1.0.0")
	require.Error(t, err)
	require.Contains(t, err.Error(), "no snapshots")
}
```

- [ ] **13.2 — Run, see it fail.**

```bash
cd /Users/angeral/Repositories/agent-containers && go test ./internal/cli/ -run TestTagCmd 2>&1 | tail -15
```

Expected: `undefined: newTagCmd`.

- [ ] **13.3 — Minimal implementation.** Add to `internal/cli/version.go`:

```go
// newTagCmd builds the `tag` command.
func newTagCmd(open envOpener) *cobra.Command {
	return &cobra.Command{
		Use:   "tag <persona> <version>",
		Short: "Tag a persona's newest snapshot with a SemVer version",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			env, err := open()
			if err != nil {
				return err
			}
			persona, version := args[0], args[1]
			ids, err := env.Store.Timeline(persona)
			if err != nil {
				return err
			}
			if len(ids) == 0 {
				return fmt.Errorf("cannot tag %q: no snapshots yet (run: acon snapshot %s)", persona, persona)
			}
			if err := env.Store.SetTag(persona, version, ids[0]); err != nil {
				return fmt.Errorf("set tag: %w", err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Tagged %q snapshot %s as %s:%s\n", persona, shortID(string(ids[0])), persona, version)
			return nil
		},
	}
}
```

- [ ] **13.4 — Run, see it pass.**

```bash
cd /Users/angeral/Repositories/agent-containers && go test ./internal/cli/ -run TestTagCmd 2>&1 | tail -5
```

Expected: `ok  	github.com/a2ngerer/agent-containers/internal/cli`.

- [ ] **13.5 — Commit.**

```bash
cd /Users/angeral/Repositories/agent-containers && git add internal/cli/version.go internal/cli/version_test.go && git commit -m "feat(cli): add 'tag' command pointing a version at the newest snapshot"
```

---

## Task 14 — Register command groups on root + full-suite gate

Wire the persona and version commands onto the cobra root so the binary exposes them. The root injects a real `envOpener` that calls `environment.Open` for the current working directory. Then run the full suite + `go vet` as the milestone gate.

**Files:**
- Modify: `internal/cli/root.go`
- Test: `internal/cli/root_test.go`

### Steps

- [ ] **14.1 — Read the current root to find the registration seam.**

```bash
cd /Users/angeral/Repositories/agent-containers && sed -n '1,80p' internal/cli/root.go
```

Expected: an M1 root builder (e.g. `func NewRootCmd() *cobra.Command` adding `init`/`use`/`list`). Identify the function that adds subcommands and whether M1 already exposes an opener for the cwd (e.g. `openCWD`).

- [ ] **14.2 — Write the failing test.** Create `internal/cli/root_test.go`:

```go
package cli

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRoot_RegistersM2Commands(t *testing.T) {
	root := NewRootCmd()
	have := map[string]bool{}
	for _, c := range root.Commands() {
		have[c.Name()] = true
	}
	for _, name := range []string{"new", "show", "edit", "rm", "snapshot", "log", "diff", "rollback", "tag"} {
		require.True(t, have[name], "root must register %q", name)
	}
}
```

> If M1's root builder is not named `NewRootCmd`, change the call in this test to match the existing exported builder (do not rename M1's function).

- [ ] **14.3 — Run, see it fail.**

```bash
cd /Users/angeral/Repositories/agent-containers && go test ./internal/cli/ -run TestRoot_RegistersM2Commands 2>&1 | tail -15
```

Expected: failure — the M2 command names are not yet registered.

- [ ] **14.4 — Minimal implementation.** In `internal/cli/root.go`, inside the root builder where subcommands are added, register the new groups using the same opener the M1 commands use. If M1 exposes `openCWD`, reuse it; otherwise add it next to the root builder:

```go
// openCWD opens the environment bound to the current working directory.
func openCWD() (*environment.Environment, error) {
	wd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	return environment.Open(wd)
}
```

Then add (adapting to the existing `AddCommand` block):

```go
	root.AddCommand(
		newNewCmd(openCWD),
		newShowCmd(openCWD),
		newEditCmd(openCWD),
		newRmCmd(openCWD),
		newSnapshotCmd(openCWD),
		newLogCmd(openCWD),
		newDiffCmd(openCWD),
		newRollbackCmd(openCWD),
		newTagCmd(openCWD),
	)
```

Ensure `internal/cli/root.go` imports `"os"` and `"github.com/a2ngerer/agent-containers/internal/environment"` (add if missing). If M1 already defines `openCWD`, delete the duplicate above and use the existing one.

- [ ] **14.5 — Run the full milestone gate.**

```bash
cd /Users/angeral/Repositories/agent-containers && go build ./... && go vet ./... && go test ./... 2>&1 | tail -20
```

Expected: clean build, no vet findings, and `ok` for `internal/compose` and `internal/cli` (plus any M1 packages). No `FAIL` lines.

- [ ] **14.6 — Commit.**

```bash
cd /Users/angeral/Repositories/agent-containers && git add internal/cli/root.go internal/cli/root_test.go && git commit -m "feat(cli): register M2 persona and versioning commands on the root command"
```

---

## Closing summary

**Tasks:** 14 (each one TDD cycle, one commit).

**Files created (11):**
- `internal/compose/compose.go`, `internal/compose/compose_test.go`
- `internal/compose/diff.go`, `internal/compose/diff_test.go`
- `internal/cli/templates.go`
- `internal/cli/persona.go`
- `internal/cli/version.go`
- `internal/cli/personahelpers.go`
- `internal/cli/persona_test.go`, `internal/cli/version_test.go`, `internal/cli/root_test.go`

**Files modified (2):** `internal/cli/root.go` (register M2 command groups), `internal/environment/environment.go` (add read-only `ActivePersona()` / `Author()` accessors if M1 lacks them).

**Packages touched:** `internal/compose` (new in M2), `internal/cli` (extended), `internal/environment` (accessors only).

**Commands delivered:** `new` (`--template coder|reviewer`, `--from`, `--extends`), `show`, `edit`, `rm`, `snapshot` (alias `commit`, `-m`), `log`, `diff` (capability diff), `rollback`, `tag`.

**New types/functions beyond the architecture contract** (all in the contract-designated package; the contract names these features as `domain core` concerns but defines no concrete result types, and they require no I/O changes to the contract's interfaces):

1. `compose.CapabilityDiff` (struct) + `compose.Diff(a, b ResolvedManifest) CapabilityDiff` — the capability-diff result type and pure function. Placed in `compose/` to keep `domain` dependency-free; `ResolvedManifest` already lives there.
2. CLI-internal helpers in `internal/cli` (all unexported, no contract surface): `personaScaffold`, `personaTemplate`, `tomlPersona`, `parsePersonaTOML`, `encodePersonaTOML`, `scaffoldPersona`, `copyPersonaScaffold`, `blankScaffold`, `removePersona`, `personaTOMLPath`, `resolveActivePersona`, `takeSnapshot`, `formatTimeline`, `formatShow`, `withheldBaseSkills`, `joinOrNone`, `resolveManifestRef`, `formatCapabilityDiff`, `resolveSnapshotRef`, `domainObjectID`, `shortID`, `envOpener`, `openCWD`, plus the `newXCmd` builders.

**Trivial accessors that must exist on `environment.Environment`** (read-only getters over the already-defined private `cfg` field; add to `internal/environment/environment.go` if M1 did not already expose them — flagged because Tasks 8–11 depend on them):
- `func (e *Environment) ActivePersona() string` → `e.cfg.ActivePersona`
- `func (e *Environment) Author() string` → `e.cfg.Author` (or literal `""` if `EnvConfig` has no author field)

These are pure getters, not new domain concepts, and do not alter any contract signature.

**Test coverage:** Compose union/override/replace/no-extends/layer-not-found (Tasks 1–2); capability diff structural delta (Task 3, Task 11); template lookup (Task 4); scaffold + overwrite-guard (Task 5); `new` flag matrix incl. mutual exclusion (Task 6); `show` capability + withheld preview (Task 7); `rm` active-guard (Task 8); snapshot/log roundtrip with newest-first ordering and RFC 3339 timestamps (Tasks 9–10); rollback restores prior tree + appends a forward snapshot (Task 12); tag resolves to newest snapshot and rejects empty history (Task 13); root registration gate + `go build`/`go vet`/full `go test` (Task 14). All filesystem tests isolate via `t.Setenv("ACON_HOME", t.TempDir())` and seed an environment through `environment.Create` (M1 API).
