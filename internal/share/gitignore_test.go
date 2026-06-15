package share

import (
	"os"
	"path/filepath"
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

func TestScanForSecrets_FlagsKeyFilename(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "deploy.key"), []byte("irrelevant body"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "persona.toml"), []byte("name = \"coder\"\n"), 0o644))

	got, err := ScanForSecrets(dir)
	require.NoError(t, err)
	require.Equal(t, []string{"deploy.key"}, got)
}

func TestScanForSecrets_FlagsBeginMarkerContent(t *testing.T) {
	dir := t.TempDir()
	body := "-----BEGIN RSA PRIVATE KEY-----\nMIIEow...\n-----END RSA PRIVATE KEY-----\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "leaked"), []byte(body), 0o600))

	got, err := ScanForSecrets(dir)
	require.NoError(t, err)
	require.Equal(t, []string{"leaked"}, got)
}

func TestScanForSecrets_FlagsTokenPrefixContent(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "notes.md"), []byte("key: sk-ABCDEF0123456789abcdef\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "gh.txt"), []byte("ghp_0123456789abcdefABCDEF0123456789abcd\n"), 0o644))

	got, err := ScanForSecrets(dir)
	require.NoError(t, err)
	require.Equal(t, []string{"gh.txt", "notes.md"}, got)
}

func TestScanForSecrets_CleanRepoReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "personas", "coder"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "personas", "coder", "persona.toml"), []byte("name = \"coder\"\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "personas", "coder", "CLAUDE.md"), []byte("# Coder\nBuild things.\n"), 0o644))

	got, err := ScanForSecrets(dir)
	require.NoError(t, err)
	require.Empty(t, got)
}

func TestScanForSecrets_SkipsDotGit(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".git"), 0o755))
	// a file inside .git that would otherwise match by content must be ignored
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".git", "config"), []byte("sk-deadbeefdeadbeefdeadbeef\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "persona.toml"), []byte("name = \"coder\"\n"), 0o644))

	got, err := ScanForSecrets(dir)
	require.NoError(t, err)
	require.Empty(t, got)
}
