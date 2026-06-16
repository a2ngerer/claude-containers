// Package harness abstracts the agent-harness-specific config layer. Each
// harness (Claude Code, Codex, OpenCode, Gemini, Kimi, Antigravity) has its own
// config layout and activation mechanism; an adapter renders the neutral
// ResolvedManifest into that layout and produces a launch spec.
//
// Claude Code is the canonical SOURCE: personas are authored in Claude's
// vocabulary (instructions, skills, subagents, MCP, permissions) and every other
// adapter is an EXPORT target that translates that manifest into its own format,
// reporting honestly what crossed the boundary intact and what was lost.
package harness

import "github.com/a2ngerer/agent-containers/internal/compose"

// Default is the reference harness id, used when nothing else selects one.
const Default = "claude"

// LaunchSpec is the environment + argv to start (or print) for a harness.
type LaunchSpec struct {
	Env  []string // e.g. ["CODEX_HOME=..."]
	Argv []string // e.g. ["codex", "--cd", "..."]
	// Note is an optional human-facing caveat printed with the launch hint, used
	// by harnesses without a clean config-relocation mechanism.
	Note string
}

// Status classifies how a persona artifact survived translation into a target.
type Status string

const (
	StatusOK         Status = "ok"         // native / carried over unchanged
	StatusTranslated Status = "translated" // re-encoded into the target's format
	StatusDegraded   Status = "degraded"   // only partially expressible
)

// ReportLine is one artifact's translation outcome.
type ReportLine struct {
	Kind   string // instructions | skills | subagents | mcp | permissions
	Detail string
	Status Status
}

// LossyNote records something that could not be represented in the target.
type LossyNote struct {
	Kind   string
	Reason string
}

// Report is the honest translation record produced by Materialize. It is the
// cross-harness analogue of the Claude attestation: it states what crossed the
// boundary intact, what was withheld, and what could not be represented — and
// never claims an enforcement a harness cannot provide.
type Report struct {
	Harness  string
	Persona  string
	Version  string
	Lines    []ReportLine // included / translated artifacts
	Withheld []string     // skills deliberately removed and physically absent
	Denied   []string     // tools denied where the target can enforce it
	Settings []string     // setting sources consulted (Claude)
	Dropped  []LossyNote  // could not be represented in the target (translation loss)
	Verified bool         // true only when the materialized env was drift-verified (Claude)
}

func (r *Report) add(kind, detail string, st Status) {
	r.Lines = append(r.Lines, ReportLine{Kind: kind, Detail: detail, Status: st})
}

func (r *Report) drop(kind, reason string) {
	r.Dropped = append(r.Dropped, LossyNote{Kind: kind, Reason: reason})
}

// Request is the input to a harness adapter.
type Request struct {
	Manifest   compose.ResolvedManifest
	PersonaDir string // <repo>/personas/<name>: source tree for skills/subagents/mcp
	DestDir    string // where to materialize the config
}

// Detection reports whether a harness looks present on the host.
type Detection struct {
	ID         string
	Installed  bool   // binary found on PATH
	BinPath    string // resolved path when Installed
	Configured bool   // a known config dir/file exists for it
}

// Harness renders a neutral persona manifest into one harness's config layout
// and knows how to launch it.
type Harness interface {
	ID() string
	DisplayName() string
	// Materialize renders req into req.DestDir, returning the translation report.
	// It is idempotent (clean-then-build). A verification failure is returned as
	// an error so callers can fail closed.
	Materialize(req Request) (Report, error)
	// Launch returns the env + argv to start this harness against req.DestDir.
	Launch(req Request) LaunchSpec
	// Detect reports whether this harness looks installed/configured on the host.
	Detect() Detection
}

// canonicalOrder fixes the display order of harnesses independent of init()
// ordering. Unknown ids sort after the known ones, alphabetically.
var canonicalOrder = []string{"claude", "codex", "opencode", "gemini", "kimi", "antigravity", "agents"}

var registry = map[string]Harness{}

// Register adds an adapter. Adapters call this from their init(). Duplicate ids
// panic at startup, surfacing a wiring bug immediately.
func Register(h Harness) {
	if _, dup := registry[h.ID()]; dup {
		panic("harness already registered: " + h.ID())
	}
	registry[h.ID()] = h
}

// Get returns the adapter for id.
func Get(id string) (Harness, bool) {
	h, ok := registry[id]
	return h, ok
}

// IDs returns all registered harness ids in canonical order.
func IDs() []string {
	rank := make(map[string]int, len(canonicalOrder))
	for i, id := range canonicalOrder {
		rank[id] = i
	}
	out := make([]string, 0, len(registry))
	for id := range registry {
		out = append(out, id)
	}
	sortByRank(out, rank)
	return out
}

// All returns every registered adapter in canonical order.
func All() []Harness {
	ids := IDs()
	out := make([]Harness, 0, len(ids))
	for _, id := range ids {
		out = append(out, registry[id])
	}
	return out
}

// sortByRank sorts ids by their rank (known ids first in canonical order),
// falling back to lexicographic order for unranked ids. Insertion sort keeps the
// dependency-free package free of an extra import for a list this small.
func sortByRank(ids []string, rank map[string]int) {
	const unranked = 1 << 30
	for i := 1; i < len(ids); i++ {
		for j := i; j > 0; j-- {
			a, b := ids[j-1], ids[j]
			ra, ok := rank[a]
			if !ok {
				ra = unranked
			}
			rb, ok := rank[b]
			if !ok {
				rb = unranked
			}
			if ra < rb || (ra == rb && a <= b) {
				break
			}
			ids[j-1], ids[j] = ids[j], ids[j-1]
		}
	}
}
