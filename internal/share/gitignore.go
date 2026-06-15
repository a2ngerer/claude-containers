// Package share implements S1 sharing of a persona repo to any git remote,
// with mandatory secret-safe defaults. No secret may ever be committed or pushed.
package share

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
