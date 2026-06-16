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

// ToolHome returns the tool's root directory. ACON_HOME overrides;
// otherwise ~/.acon.
func ToolHome() string {
	if h := os.Getenv("ACON_HOME"); h != "" {
		return h
	}
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".acon")
}

// EnvDir is the per-workspace environment directory.
func EnvDir(hash string) string { return filepath.Join(ToolHome(), "environments", hash) }

// RepoDir is the hidden git repo (StorageEngine backend) for an environment.
func RepoDir(hash string) string { return filepath.Join(EnvDir(hash), "repo") }

// CacheDir is the materialized CLAUDE_CONFIG_DIR for one persona (ephemeral).
func CacheDir(hash, persona string) string {
	return filepath.Join(ToolHome(), "cache", hash, persona)
}
