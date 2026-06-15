package enforce

import "github.com/a2ngerer/claude-containers/internal/domain"

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
