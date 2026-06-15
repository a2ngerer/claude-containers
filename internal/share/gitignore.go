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
