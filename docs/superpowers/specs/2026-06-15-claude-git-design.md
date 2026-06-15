# claude_git — Design Specification

> **Working title:** `claude_git` (CLI alias `cg`)
> **Status:** Phase 1 design — approved direction, pending user review before implementation planning
> **Date:** 2026-06-15
> **Author:** Alexander Angerer
> **One-liner:** Version control and isolated, swappable environments for the Claude Code agent-configuration layer — *"Docker for Claude agent environments."*

---

## 1. Vision

`claude_git` treats a Claude Code agent's entire behavioral setup — `CLAUDE.md` plus the `.claude/` directory (skills, subagents, settings, hooks, slash-commands, MCP config) — as a **versioned, swappable, shareable artifact**, managed independently of the project's source code.

The mental model is a container engine for agent environments:

| Container engine | claude_git |
|---|---|
| Image (versioned, layered, content-addressed) | **Persona** — a named, versioned, layered configuration bundle |
| Image layer / commit | **Snapshot** — an immutable point in a persona's history |
| Tag (`name:1.2.0`, `name:latest`) | **Version tag** on a snapshot |
| `docker run <image>` | `claude_git <persona>` — activates the environment for the next `claude` start |
| Registry (Docker Hub) | **Sharing** — push/pull persona repositories and individual personas |
| Own layer store (not git) | **Own version model** over a pluggable, git-backed storage engine |

"Container" here is a **metaphor for an isolated, interchangeable environment**, not OS-level sandboxing. Each persona is a self-contained agent environment you swap in when the task changes (build vs. review vs. research). True process/filesystem sandboxing is an optional future stage (§16), not part of the core.

---

## 2. Problem

Claude Code's behavior is fully determined by file-based artifacts in the workspace: `CLAUDE.md` and `.claude/`. This layer is bound 1:1 to the workspace — there is exactly **one** active state per directory. Two concrete frictions follow:

1. **Portability is one-directional.** Copying a config *out* of a workspace is easy; holding *multiple* configs for the *same* workspace and switching between them quickly is not.
2. **No isolation of multiple agent roles in the same project.** The motivating case: a **coding agent** with a curated skill set (mixture of experts) and, strictly separated from it, a **review agent** that deliberately does *not* have those coding skills, so its judgment is not biased ("contaminated") by the builder's perspective.

There is no tool that treats the config layer as a **versioned artifact with history, diff, rollback, and provenance**, decoupled from the code repository.

---

## 3. Market gap (research summary)

Five parallel research agents established the following (sources in §19):

- **Native Claude Code building blocks exist** — subagents (own context window, restricted tools), output styles, `--settings` / `--setting-sources user,project,local` (selective layer loading), `--mcp-config` / `--strict-mcp-config`, `--tools`, `CLAUDE_CONFIG_DIR` (relocates the whole config directory) — but there is **no "persona as a versioned bundle"** and **no versioning** of the config layer.
- Anthropic knows the gap: open feature request **#53458 "Persona Profiles"** describes the switching half almost verbatim, but is unimplemented and carries no versioning. Anthropic is instead building **Agent Teams** (parallel agents, live since Feb 2026) — orchestration, not versioned config.
- **Nearest existing tools:** `claude-profile-switch` (git-backed, but separate *directories* not versioned personas, global via `CLAUDE_CONFIG_DIR`, no diff/merge); Roo Code `.roomodes` (strongest *enforced* persona isolation — a read-only reviewer genuinely cannot write — but no versioning); Continue Hub (versions the config layer, but as a central cloud registry, no branches). Prompt-versioning tools (PromptLayer, Langfuse, PromptHub, LangSmith) are all cloud registries, not applicable to local files.
- **Dotfile managers** provide the mechanics vocabulary: the bare-git trick (separate git-dir coexisting with the code repo) + `git worktree`; "steal" atomic switch + rollback (Nix) and idempotent manifests (dotbot).
- **Demand is real (4/5 overall).** The killer use case — the *uncontaminated reviewer* — scores 5/5 and is even academically grounded: self-recognition causally drives self-preference in LLM evaluators (Panickssery et al., NeurIPS 2024); separation, not just more compute, is what helps (Du et al., ICML 2024). Community vocabulary: *context isolation/pollution/contamination*, *uncontaminated reviewer*, *correlated blind spots*, *"grade its own homework."*

**Conclusion:** The individual pieces exist, scattered. The combination — *an own version model for the local agent-config layer, decoupled from the code VCS, with enforced persona isolation* — exists nowhere. The defensible moat is the **versioning / lifecycle axis**, not switching or orchestration (which Anthropic is actively absorbing).

---

## 4. Design decisions and rationale

These were settled during brainstorming; each records the rationale so Phase 2 does not relitigate them.

### D1 — Product identity: versioned persona system
Not a thin runtime switcher (high obsolescence risk vs. native features) and not a pure lifecycle layer (too far from the original intent). The synthesis: isolated persona activation **plus** real versioning, differentiated on the versioning axis nobody occupies.

### D2 — Activation model: coexisting bundles, materialized outside the workspace
A persona is **not** activated by swapping the workspace `.claude/` (the literal screenshot model). Instead, activation materializes the persona into a config directory **outside** the workspace and points `CLAUDE_CONFIG_DIR` at it. Consequences:
- The workspace `.claude/` (possibly tracked by the code repo) is **never touched** → no collision with the code git. This was verified to be a real trap: `.gitignore` does *not* untrack an already-tracked `.claude/`, and two `.git` dirs over one working tree have zero mutual awareness.
- Multiple personas can be active at once (parallel terminals) → coexistence, which the literal branch model cannot provide.
- The user's flow (`claude` → quit → `claude_git code_reviewer` → next `claude` start runs the new environment) is the primary, sequential UX; parallel coexistence is available but not required.

### D3 — Versioning: own domain model, git-backed pluggable storage
Own concepts and CLI semantics (personas, layers, snapshots, timeline, tags, capability diffs — no user-facing branches). Underneath, a **pluggable storage engine** whose default implementation uses git's content-addressed object store and git remotes, fully hidden from the user. Rationale: best everyday value for students/solo/teams (robust storage, familiar GitHub/GitLab sharing, no new server) while preserving the "our own versioning" experience and leaving the door open for a fully custom object store later, behind the same interface.

### D4 — Enforcement: isolation must be guaranteed, not suggested
Two orthogonal, independently sufficient mechanisms (both confirmed against Claude Code semantics):
1. **Material withholding** — only allowlisted skills/subagents/MCP servers are materialized into the active config dir; what is absent cannot be invoked.
2. **Tool denial** — `permissions.deny` in the generated `settings.json` (a deny at any level is final), reinforced by `--setting-sources` and `--strict-mcp-config`.
A read-only reviewer is thus *physically* unable to build, and *demonstrably* free of the builder's skills.

### D5 — Layering: base layer + persona diffs (composition)
Personas extend a shared `_base` layer rather than being independent snapshots. The decisive reason: **the diff is the documentation of isolation.** "Reviewer = base − build-skills − write" is auditable at a glance, which is exactly the guarantee the product sells; full snapshots would force diffing two directories to prove cleanliness. Composition also avoids drift (change the base once).

### D6 — Container = metaphor, not OS sandbox
The MVP configures a normal Claude Code session. Real sandboxing/orchestration is an explicitly deferred, commercializable future stage (§16); the data model is shaped so it can be added without redesign.

### D7 — Sharing: S1 in MVP, S2 staged, never a hosted registry
Push/pull to any git remote (S1) ships in the MVP, with secret-safe defaults mandatory. Pulling a single persona from a foreign repo (S2) is specified but staged. A hosted registry (S3) is out of scope forever — the Claude Code plugin marketplace already owns distribution; the moat is versioning, not discovery.

### D8 — Language: Go
Single static binary (trivial cross-platform distribution: `go install`, Homebrew, npm-wrapped), excellent pure-Go git plumbing (`go-git`), mature CLI tooling (`cobra`/`viper`). Rust (`gitoxide`/`clap`) is a viable alternative; Go is recommended for faster iteration by a solo maintainer.

---

## 5. Core concepts and data model

```
Environment (one per bound workspace)
 ├── binds a workspace path
 ├── holds a content-addressed Store (git-backed)
 ├── Layers      (reusable building blocks, e.g. _base)
 └── Personas    (named, versioned agent environments)
       ├── Manifest        (declarative: what it includes + enforces)
       ├── Timeline        (ordered Snapshots = history)
       │     └── Snapshot  (immutable state; parent(s), message, author, timestamp)
       └── Version tags    (named pointers to snapshots: reviewer:1.2.0, reviewer:latest)
```

- **Environment** — the binding of `claude_git` to one workspace. Identified by a hash of the absolute workspace path; the underlying persona repository is a standalone, hidden git repo (decoupled from the code repo) that can be pushed/cloned for sharing.
- **Persona** — a named agent environment ("the container/image"). Defined by a manifest (§6) plus its content. Has a linear-by-default history of snapshots and version tags. Examples: `coder`, `reviewer`, `researcher`, `docs-writer`.
- **Layer** — a reusable bundle other personas compose from. `_base` is the conventional shared foundation (global conventions, language rules, paths). Personas declare `extends`.
- **Snapshot** — an immutable, content-addressed state of a persona at a point in time (the "commit"/"layer"). Carries parent reference(s), message, author, timestamp.
- **Version tag** — a SemVer pointer to a snapshot (`reviewer:1.2.0`), plus moving tags (`latest`). Mirrors Docker tags; enables "roll back to the reviewer we reviewed with."
- **Store** — the content-addressed object store (blobs, trees, snapshots) behind a `StorageEngine` interface (§7). Default: git objects; remotes for sync.
- **Materialization** — rendering a persona's composed manifest into a concrete `CLAUDE_CONFIG_DIR` outside the workspace, ready for `claude` to consume.
- **Attestation** — a record of exactly what an activation contained and denied (the visible, auditable cleanliness certificate).

---

## 6. Persona manifest

Declarative; the source of truth for a persona. Stored as `persona.toml` (TOML chosen for comments and human editing; format is an implementation detail behind the model).

```toml
# personas/reviewer/persona.toml
name        = "reviewer"
description = "Uncontaminated reviewer. Sees the diff, not the build skills."
extends     = "_base"                 # composition (D5)

[config]                              # (a) what gets loaded
claude_md       = "CLAUDE.md"         # persona-local top-level instructions
setting_sources = ["user", "project"] # NOT "local" -> coder's local tweaks stay out

[config.skills]
mode    = "allowlist"                 # explicit; never "everything except X"
include = ["security-review", "silent-failure-hunter", "type-design-analyzer"]

[config.subagents]
include = ["code-reviewer", "security-reviewer"]

[config.mcp]
config = "mcp.json"                   # persona-local MCP server set
strict = true                         # --strict-mcp-config: ignore all other MCP sources

[enforcement]                         # (b) what is technically impossible (D4)
permission_mode = "read-only"
tools.allow = ["Read", "Grep", "Glob", "Bash(git diff:*)", "Bash(git log:*)"]
tools.deny  = ["Write", "Edit", "NotebookEdit", "Bash(git commit:*)", "Bash(git push:*)"]

[metadata]                            # for sharing / provenance (§13)
version = "1.2.0"
author  = "alexander.angerer"
```

The `coder` persona is the symmetric case: full build skill set, `Write`/`Edit` allowed, `setting_sources = ["user","project","local"]`.

---

## 7. Architecture

Layered, each layer with one responsibility, communicating through narrow interfaces. This keeps the storage engine swappable (D3) and the activation mechanism testable in isolation.

```
+--------------------------------------------------------------+
|  CLI  (cobra)   claude_git <persona> | new | snapshot | ...   |
+--------------------------------------------------------------+
|  Domain core    Persona, Layer, Snapshot, Timeline, Tag,      |
|                 composition, capability-diff, lifecycle ops   |
+----------------------+-------------------+--------------------+
|  Storage engine      |  Activation       |  Enforcement       |
|  (pluggable)         |  engine           |  generator         |
|  - default: go-git   |  - materialize    |  - settings.json   |
|  - object store      |    CLAUDE_CONFIG_ |    deny/allow      |
|  - remotes (sync)    |    DIR (out of WS)|  - flag builder    |
|                      |  - lockfile       |  - attestation     |
+----------------------+-------------------+--------------------+
|  Sharing             |  Workspace probe                       |
|  - push/pull/clone   |  - detect tracked .claude/ in code repo|
|  - persona package   |  - parallel-session lock               |
|    (S2)              |                                         |
+----------------------+-----------------------------------------+
```

**Storage engine interface (sketch):**

```go
type StorageEngine interface {
    PutObject(content []byte) (ObjectID, error)   // content-addressed
    GetObject(id ObjectID) ([]byte, error)
    WriteSnapshot(s Snapshot) (SnapshotID, error)
    ReadSnapshot(id SnapshotID) (Snapshot, error)
    Timeline(persona string) ([]SnapshotID, error)
    SetTag(persona, version string, id SnapshotID) error
    // sharing
    Push(remote string) error
    Pull(remote string) error
    Clone(remote, dest string) error
}
```

The default `GitStorageEngine` maps snapshots to git commits, objects to git blobs/trees, tags to git refs under an internal namespace, and remotes to git remotes — none of which surfaces to the user. A future `NativeStorageEngine` can replace it without touching the domain core.

---

## 8. Isolation and enforcement — the killer use case worked through

The uncontaminated-reviewer flow, end to end:

**One-time setup**
```
cd ~/Repositories/my-project
claude_git init                 # binds workspace, imports current .claude/ into _base
claude_git new coder            # extends _base; full build skills; Write/Edit allowed
claude_git new reviewer         # extends _base; review skills only; Write/Edit DENIED
claude_git snapshot coder    -m "coder: full skill set, TDD agent"
claude_git snapshot reviewer -m "reviewer: stripped, no coding skills, read-only"
```

**Daily loop**
```
claude_git coder                # activates coder env; (re)start claude -> builds the feature
# ... build ...
claude_git reviewer             # activates reviewer env; restart claude
# reviewer sees the SAME code, but its skill set is physically the review-only set,
# and Write/Edit are denied -> the verdict is uncontaminated
```

**How contamination is prevented (D4), and how it is made visible:**

| Contamination vector | Mechanism | Guarantee |
|---|---|---|
| Reviewer uses build skills | allowlist materialization — build skills are not in the config dir | absence = cannot be invoked |
| Reviewer writes/commits code | `permissions.deny: [Write, Edit, ...]` | deny at any level is final |
| Coder's `settings.local.json` leaks in | `--setting-sources user,project` (no `local`) | selective layer loading |
| Project MCP servers leak | `--strict-mcp-config` + persona-local `mcp.json` | all other MCP sources ignored |
| Shared model context between build & review | separate `claude` processes — reviewer starts with an empty context window | process isolation (no shared state) |

The last row is the academically decisive one: the same agent that built the code recognizes and favors its own output (self-preference). A separate persona = a separate process = the reviewer sees the code as foreign input.

**Visible attestation** on activation (also written to history as `attestation.json` for audit):
```
$ claude_git reviewer
  Persona: reviewer  (uncontaminated)   reviewer:1.2.0
  Skills:     3 review-only      [security-review, silent-failure-hunter, type-design-analyzer]
  Withheld:   7 build skills     [superpowers, writing-plans, +5]  (deliberately removed)
  Write/Edit: DENIED (read-only)
  MCP:        strict, 1 server   (no project MCP leak)
  Settings:   user+project       (local layer excluded)

  The reviewer cannot see what the coder built it with. Verdict is uncontaminated.
  -> Start (or restart) Claude Code in this directory to use this environment.
```

`claude_git verify reviewer` re-runs these checks against the materialized dir and exits non-zero on any mismatch — the isolation guarantee is *verified*, not just displayed.

---

## 9. Activation mechanism in detail

What `claude_git <persona>` (alias for `claude_git use <persona>`) does:

```
1. resolve   persona (+ optional :version tag); compose with its layers (_base + diffs)
2. lock      acquire the environment lock (records active persona + PID); warn on conflict
3. materialize
             render the composed manifest into
             ~/.claude_git/cache/<workspace-hash>/<persona>/         <- becomes CLAUDE_CONFIG_DIR
             - copy only allowlisted skills/subagents
             - generate settings.json with enforcement deny/allow rules
             - generate persona-local mcp.json
             - write CLAUDE.md (composed)
4. verify    assert the materialized dir matches the manifest exactly (fail closed)
5. attest    print the attestation; record it to history
6. launch    either:
             (a) print the env + flags to start claude with (default, safe), or
             (b) exec claude directly with:
                 CLAUDE_CONFIG_DIR=<dir> \
                 claude --setting-sources user,project \
                        --strict-mcp-config --mcp-config <dir>/mcp.json \
                        --allowedTools "<allow set>" \
                        --append-system-prompt @<dir>/CLAUDE.md
```

**The user never types the flag soup** — the persona encapsulates the correct, reproducible combination. This is the concrete value over native flags: the building blocks exist, but nobody assembles `CLAUDE_CONFIG_DIR` + `--setting-sources` + `--strict-mcp-config` + `--allowedTools` correctly by hand on every start.

**CLI dispatch rule:** reserved subcommands (`init`, `new`, `use`, `list`/`ls`, `status`, `snapshot`, `log`, `diff`, `rollback`, `tag`, `show`, `verify`, `edit`, `rm`, `push`, `pull`, `clone`, `pull-persona`) are recognized as such; any other first argument is interpreted as a persona name (`claude_git reviewer` ≡ `claude_git use reviewer`). Unknown names produce a "did you mean" with the persona list.

**Edge cases (must be handled):**
- *Tracked `.claude/` in the code repo:* because activation never writes into the workspace, this is harmless. `init` still probes (`git ls-files --error-unmatch .claude`) and, if tracked, advises (with confirmation) on optionally untracking it — never acts on the code repo automatically.
- *Parallel sessions, divergent personas:* allowed because each persona has its own `CLAUDE_CONFIG_DIR`. The lockfile guards concurrent `claude_git` mutations, not concurrent `claude` sessions.
- *Activation does not affect a running session:* Claude Code reads config at start. After `claude_git <persona>`, the tool explicitly instructs the user to (re)start Claude Code.

---

## 10. On-disk layout

```
~/.claude_git/                                  # tool home
  config.toml                                   # global config (default editor, author, ...)
  environments/
    <workspace-hash>/                           # one per bound workspace
      env.toml                                  # absolute workspace path, active persona, settings
      repo/                                     # hidden, standalone git repo (the StorageEngine backend)
        personas/ _base/ coder/ reviewer/       # manifests + content
        (git objects/refs internal, never shown)
  cache/
    <workspace-hash>/
      <persona>/                                # materialized CLAUDE_CONFIG_DIR (ephemeral, rebuildable)

<workspace>/.claude_git                         # optional marker file linking workspace -> environment
                                                # (enables team onboarding via `init --from <remote>`)
```

The workspace's own `.claude/` and `CLAUDE.md` are read at `init` (to seed `_base`) and otherwise left untouched.

---

## 11. CLI specification

**Setup**
- `claude_git init [--from <remote>]` — bind the current workspace; seed `_base` from the existing `.claude/`, or clone an existing persona repo for team onboarding.

**Activation (core UX)**
- `claude_git <persona>[:<version>]` / `claude_git use <persona>` — compose, materialize, attest, and launch (or print launch command).
- `claude_git deactivate` — clear the active persona for this workspace.

**Persona management**
- `claude_git new <name> [--from <persona>] [--extends <layer>]`
- `claude_git list` / `ls` — personas + active marker + current version
- `claude_git status` — active persona, dirty state, lock holder
- `claude_git show <persona>` — composed manifest + attestation preview
- `claude_git edit <persona>` — open the manifest
- `claude_git rm <persona>`

**Versioning**
- `claude_git snapshot [<persona>] [-m <msg>]` — commit current persona state (alias: `commit`)
- `claude_git log [<persona>]` — history/timeline
- `claude_git diff [<a> <b>]` — **capability diff** between snapshots or personas ("coder has X, reviewer does not")
- `claude_git rollback <persona> <snapshot|version>`
- `claude_git tag <persona> <version>` — SemVer tag a snapshot
- `claude_git verify <persona>` — re-check materialized isolation; non-zero on mismatch

**Sharing**
- `claude_git push [<remote>]` / `claude_git pull [<remote>]` — sync the persona repo (S1)
- `claude_git clone <remote>` — clone a whole persona set
- `claude_git pull-persona <source>#<persona>` — pull a single persona (S2, staged)

MVP subset is defined in §15.

---

## 12. Layering and composition

- Resolution order: `_base` → persona diff. Later layers override scalars; skill/subagent allowlists are unions unless a layer marks `mode = "replace"`.
- Materialization merges the composed manifest into a flat config dir; Claude Code only ever sees the finished directory, never a "diff."
- Composition is what makes isolation auditable (D5): `claude_git diff coder reviewer` shows the capability delta directly.

---

## 13. Sharing and security

**S1 (MVP):** the persona repo is a normal (hidden) git repo → `push`/`pull`/`clone` to any GitHub/GitLab remote. Decoupled from the code repo by construction.

**S2 (staged): persona package format** — do not reinvent; map onto existing standards.
```toml
# persona.toml already carries name/version/author/description (§6)
# package adds provenance on pull:
[provenance]
source = "github.com/user/repo#security-reviewer"
commit = "<sha>"
```
- Pull copies referenced files into the local persona store; on skill/subagent name collision, **default = reject + show diff**, with explicit `--rename`/`--overwrite` opt-ins. Never silently overwrite.
- MCP dependencies are **declared only** (`mcp.requires = ["github", "semgrep"]`); never ship MCP configs or secrets. On pull, warn if a required server is missing.

**Security (mandatory in MVP):**
- Secret-safe `.gitignore` defaults: `settings.local.json`, credential files, anything matching common secret patterns are never committed/shared. A shared persona leaking an API key is an instant trust killer.
- Foreign personas are executable instruction sets → a prompt-injection vector. `show`/`diff` before activation; never auto-activate a freshly pulled persona.

---

## 14. Boundary vs. native features (be honest)

| Situation | Use |
|---|---|
| Sub-task within the same effort, shared context wanted | **native subagent** — lighter, no process boundary |
| Live orchestration of parallel agents | **Agent Teams** — that is what they are for |
| Contamination-freedom is the *goal*; reproducible, versioned, shareable role | **claude_git persona** |
| "What did the reviewer config look like last week, and why did it change?" | **claude_git** (history/diff/rollback) — nothing native does this |

`claude_git` complements native isolation; it does not compete on orchestration. Its defensible value is **persistent, versioned, auditable** config — the axis Anthropic is not building.

---

## 15. MVP scope (Phase 2 target)

**In:**
- `init`, `new`, `use`/`<persona>`, `list`, `status`, `show`, `snapshot`, `log`, `diff`, `rollback`, `tag`, `verify`, `deactivate`, `edit`, `rm`
- Composition (`_base` + persona diffs)
- Materialization to `CLAUDE_CONFIG_DIR` + flag generation + attestation
- Enforcement (deny/allow, allowlist, setting-sources, strict-mcp-config)
- Git-backed storage engine (default), workspace probe, lockfile
- S1 sharing (`push`/`pull`/`clone`) with secret-safe defaults
- Two shipped default personas (`coder`, `reviewer`) so users do not start from zero

**Deferred:** S2 persona-pull; native (non-git) storage engine; true sandbox containers (§16); TUI/GUI; team policy features.

---

## 16. Roadmap

- **Phase 2 — MVP** (§15): the working CLI for solo use.
- **Phase 3 — Sharing & teams:** S2 persona packages; team baseline personas with auditable history ("roll back to the reviewer version we reviewed with" — the one thing the plugin marketplace cannot do); CI integration (`claude_git verify` as a gate).
- **Phase 4 — Containers (commercializable):** optional true sandbox isolation — each persona runs in its own filesystem/process sandbox with resource limits. Builds on the existing persona model; turns the metaphor into enforced runtime isolation. This is the "sellable" stage.
- **Optional:** native storage engine (no git dependency); editor/TUI integration; persona analytics (does the isolated reviewer measurably catch more — ties to the self-bias literature, the research-adjacent angle).

---

## 17. Risks and mitigations

| Risk | Mitigation |
|---|---|
| **Claude Code format drift** (settings keys, `.claude/` layout, subagent format change) breaks the tool | Thin adapter over CC's config surface; pin tested CC versions; integration tests that materialize + launch against a real `claude`; fail closed on unknown keys |
| **Anthropic ships #53458 / extends Agent Teams**, absorbing the switching value | Lean into the versioning/lifecycle moat (D1, §14) which Anthropic is unlikely to build; position as complementary |
| **Overlap with `claude-profile-switch`** | Differentiator is the version model (snapshots/diff/rollback/tags) + enforced, composable personas + decoupling from the code repo — none of which it does |
| **Single-maintainer maintenance load** | Go single binary, small surface, strong test coverage; storage behind an interface to isolate the riskiest part |
| **Silent isolation breakage** (a build skill slips into the reviewer) | `verify` step on every `use` (fail closed) + attestation as verification, not decoration |
| **Data loss in the config store** | Use git's hardened object store as default backend; never hand-roll the store for the MVP |

---

## 18. Acceptance criteria (MVP)

1. `init` in a fresh repo seeds `_base` from existing `.claude/` without modifying the workspace.
2. `new coder` / `new reviewer`, then `snapshot` each — both appear in `list` with versions.
3. `claude_git reviewer` materializes a config dir outside the workspace, prints an attestation, and the workspace `.claude/` is byte-for-byte unchanged.
4. A Claude Code session started after `claude_git reviewer` cannot invoke build skills and cannot `Write`/`Edit` (verified manually + by `verify`).
5. `diff coder reviewer` shows the capability delta.
6. `rollback reviewer <older>` restores a prior persona state; `log` reflects it.
7. `push`/`clone` round-trip the persona repo to a remote with no secrets committed.
8. Two terminals can run `coder` and `reviewer` concurrently without corrupting each other's state.

---

## 19. Sources (selected)

- Claude Code: [Settings](https://code.claude.com/docs/en/settings) · [CLI reference](https://code.claude.com/docs/en/cli-reference) · [Subagents](https://code.claude.com/docs/en/sub-agents) · [Output styles](https://code.claude.com/docs/en/output-styles) · [Permissions](https://code.claude.com/docs/en/agent-sdk/permissions)
- Feature requests: [#53458 Persona Profiles](https://github.com/anthropics/claude-code/issues/53458) · [#261 profiles](https://github.com/anthropics/claude-code/issues/261)
- Nearest tools: [claude-profile-switch](https://github.com/guibes/claude-profile-switch) · [Roo Code custom modes](https://docs.roocode.com/features/custom-modes) · [Continue Hub](https://docs.continue.dev/hub/assistants/intro)
- Mechanics: [git-worktree](https://git-scm.com/docs/git-worktree) · [gitignore](https://git-scm.com/docs/gitignore) · [Atlassian dotfiles (bare repo)](https://www.atlassian.com/git/tutorials/dotfiles) · [go-git](https://github.com/go-git/go-git)
- Standard: [AGENTS.md](https://agents.md/)
- Bias grounding: [Panickssery et al., NeurIPS 2024 (self-preference)](https://proceedings.neurips.cc/paper_files/paper/2024/hash/7f1f0218e45f5414c79c0679633e47bc-Abstract-Conference.html) · [Du et al., ICML 2024 (multi-agent debate)](https://arxiv.org/abs/2305.14325) · [Anthropic — Building Effective Agents](https://www.anthropic.com/research/building-effective-agents)
- Demand: ["Same Claude, Different Roles"](https://dev.to/lazydev_oh/same-claude-different-roles-my-5-agent-dev-team-3jlc) · [dot-claude-sync](https://dev.to/ugo/share-claude-created-documents-across-worktrees-with-dot-claude-sync-cpc)

---

## 20. Glossary

- **Persona** — a named, versioned, isolated agent environment (the "container/image").
- **Layer** — a reusable bundle personas compose from (`_base`).
- **Snapshot** — an immutable point in a persona's history.
- **Materialization** — rendering a persona into a `CLAUDE_CONFIG_DIR` outside the workspace.
- **Attestation** — the verifiable record of what an activation included and denied.
- **Uncontaminated reviewer** — a review persona that provably lacks the builder's skills and context.
