// internal/probe/probe.go
package probe

import (
	"errors"
	"fmt"
	"os/exec"
)

// IsClaudeTracked reports whether the workspace's code git repo already tracks
// the .claude directory. It runs `git ls-files --error-unmatch .claude`:
// exit 0 means tracked; a non-zero exit (untracked path, or not a git repo)
// means not tracked. Only a failure to execute git at all is returned as error.
func IsClaudeTracked(workspace string) (bool, error) {
	cmd := exec.Command("git", "ls-files", "--error-unmatch", ".claude")
	cmd.Dir = workspace
	err := cmd.Run()
	if err == nil {
		return true, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		// git ran and reported "not tracked" (or "not a repository").
		return false, nil
	}
	return false, fmt.Errorf("run git ls-files in %q: %w", workspace, err)
}
