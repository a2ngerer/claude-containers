package share

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDefaultGitignore_ContainsKeyPatterns(t *testing.T) {
	got := DefaultGitignore()

	mustContain := []string{
		"settings.local.json",
		"*.key",
		"*.pem",
		"*.p12",
		".env",
		".env.*",
		"id_rsa",
		"id_rsa.*",
		"*credential*",
		"*secret*",
		"*.pfx",
		"id_ed25519",
		"id_ecdsa",
		"*.token",
		"*.crt",
		".aws/",
		".ssh/",
	}
	for _, pat := range mustContain {
		require.Contains(t, got, pat, "gitignore must exclude %q", pat)
	}
}

func TestDefaultGitignore_EndsWithNewline(t *testing.T) {
	got := DefaultGitignore()
	require.True(t, strings.HasSuffix(got, "\n"), "gitignore must end with a trailing newline")
}
