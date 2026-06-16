// Package activate orchestrates turning a persona into a runnable agent
// environment: compose the manifest, materialize it for a target harness behind
// the harness adapter interface, record the active persona, and build the launch
// spec. Claude Code is the canonical source harness; other harnesses are export
// targets reached through the same adapter contract.
package activate

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/a2ngerer/agent-containers/internal/compose"
	"github.com/a2ngerer/agent-containers/internal/environment"
	"github.com/a2ngerer/agent-containers/internal/harness"
)

// ActivationResult is the outcome of Activate: the target harness, where the env
// was materialized, the translation/attestation report, and the launch spec.
type ActivationResult struct {
	Harness   string
	ConfigDir string
	Report    harness.Report
	Launch    harness.LaunchSpec
}

// Activate composes a persona, locks the environment, materializes it for the
// target harness into the harness-namespaced cache dir, records the active
// persona, and builds the launch spec. Materialization is fail-closed: a
// verification drift (Claude) or translation error aborts before anything is
// recorded or launched. personaRef is "name" or "name:version" (version
// currently informational; default "latest").
func Activate(e *environment.Environment, personaRef, harnessID string) (ActivationResult, error) {
	name, _ := parsePersonaRef(personaRef)

	h, ok := harness.Get(harnessID)
	if !ok {
		return ActivationResult{}, fmt.Errorf("unknown harness %q", harnessID)
	}

	rm, err := compose.Compose(e, name)
	if err != nil {
		return ActivationResult{}, err
	}

	lock, err := Acquire(e, name)
	if err != nil {
		return ActivationResult{}, err
	}
	defer lock.Release()

	configDir := environment.CacheDir(e.Hash, harnessID, name)
	req := harness.Request{
		Manifest:   rm,
		PersonaDir: filepath.Join(environment.RepoDir(e.Hash), "personas", rm.Persona.Name),
		DestDir:    configDir,
	}

	report, err := h.Materialize(req)
	if err != nil {
		return ActivationResult{}, fmt.Errorf("materialize %q for %s: %w", name, harnessID, err)
	}

	if err := e.SetActive(name); err != nil {
		return ActivationResult{}, fmt.Errorf("set active persona: %w", err)
	}

	return ActivationResult{
		Harness:   harnessID,
		ConfigDir: configDir,
		Report:    report,
		Launch:    h.Launch(req),
	}, nil
}

// Export materializes a persona for a target harness into an explicit destDir
// without locking, recording the active persona, or touching the cache. It backs
// `acon export`: the "containerize my Claude setup and take it to another
// harness" flow. The returned launch spec is relative to destDir.
func Export(e *environment.Environment, personaRef, harnessID, destDir string) (harness.Report, harness.LaunchSpec, error) {
	name, _ := parsePersonaRef(personaRef)

	h, ok := harness.Get(harnessID)
	if !ok {
		return harness.Report{}, harness.LaunchSpec{}, fmt.Errorf("unknown harness %q", harnessID)
	}

	rm, err := compose.Compose(e, name)
	if err != nil {
		return harness.Report{}, harness.LaunchSpec{}, err
	}

	req := harness.Request{
		Manifest:   rm,
		PersonaDir: filepath.Join(environment.RepoDir(e.Hash), "personas", rm.Persona.Name),
		DestDir:    destDir,
	}
	report, err := h.Materialize(req)
	if err != nil {
		return harness.Report{}, harness.LaunchSpec{}, fmt.Errorf("export %q for %s: %w", name, harnessID, err)
	}
	return report, h.Launch(req), nil
}

// parsePersonaRef splits "name" or "name:version" into its parts. The version
// defaults to "latest".
func parsePersonaRef(ref string) (name, version string) {
	if i := strings.LastIndex(ref, ":"); i >= 0 {
		return ref[:i], ref[i+1:]
	}
	return ref, "latest"
}
