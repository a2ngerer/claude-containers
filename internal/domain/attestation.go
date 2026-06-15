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
