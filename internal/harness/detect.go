package harness

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// probeHost builds a Detection for a harness: it looks up bin on PATH and checks
// whether any of configCandidates (with leading ~ expanded) exists on disk.
func probeHost(id, bin string, configCandidates ...string) Detection {
	d := Detection{ID: id}
	if p, err := exec.LookPath(bin); err == nil {
		d.Installed = true
		d.BinPath = p
	}
	for _, c := range configCandidates {
		if c == "" {
			continue
		}
		if _, err := os.Stat(expandHome(c)); err == nil {
			d.Configured = true
			break
		}
	}
	return d
}

// expandHome resolves a leading ~ to the user's home directory.
func expandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(p, "~"))
		}
	}
	return p
}
