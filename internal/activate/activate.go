package activate

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/a2ngerer/claude-containers/internal/compose"
	"github.com/a2ngerer/claude-containers/internal/domain"
	"github.com/a2ngerer/claude-containers/internal/enforce"
	"github.com/a2ngerer/claude-containers/internal/environment"
	"github.com/a2ngerer/claude-containers/internal/materialize"
)

// verifyFn is the verify function used by Activate. It is a package-level
// variable so tests can override it to simulate ErrVerifyMismatch without
// having to manipulate on-disk state in ways the normal flow prevents.
var verifyFn = defaultVerify

func defaultVerify(rm compose.ResolvedManifest, personaDir, destDir string) (domain.Attestation, error) {
	return enforce.Verify(rm, personaDir, destDir)
}

// ActivationResult is the outcome of Activate: where the env was materialized,
// the attestation proving its cleanliness, and the launch spec to run/print.
type ActivationResult struct {
	ConfigDir   string
	Attestation domain.Attestation
	Launch      LaunchSpec
}

// Activate composes a persona, locks the environment, materializes it into the
// cache config dir, verifies isolation (fail-closed), enriches the attestation,
// records the active persona, and builds the launch spec. personaRef is "name"
// or "name:version" (version currently informational; default "latest").
func Activate(e *environment.Environment, personaRef string) (ActivationResult, error) {
	name, _ := parsePersonaRef(personaRef)

	rm, err := compose.Compose(e, name)
	if err != nil {
		return ActivationResult{}, err
	}

	lock, err := Acquire(e, name)
	if err != nil {
		return ActivationResult{}, err
	}
	defer lock.Release()

	configDir := environment.CacheDir(e.Hash, name)
	if err := materialize.Materialize(e, rm, configDir); err != nil {
		return ActivationResult{}, fmt.Errorf("materialize %q: %w", name, err)
	}

	personaDir := filepath.Join(environment.RepoDir(e.Hash), "personas", rm.Persona.Name)
	att, err := verifyFn(rm, personaDir, configDir)
	if err != nil {
		if errors.Is(err, domain.ErrVerifyMismatch) {
			// fail closed: do not launch a compromised environment
			return ActivationResult{}, err
		}
		return ActivationResult{}, fmt.Errorf("verify %q: %w", name, err)
	}

	att.Withheld = withheldSkills(e, rm)

	if err := e.SetActive(name); err != nil {
		return ActivationResult{}, fmt.Errorf("set active persona: %w", err)
	}

	return ActivationResult{
		ConfigDir:   configDir,
		Attestation: att,
		Launch:      BuildLaunch(configDir, rm),
	}, nil
}

// parsePersonaRef splits "name" or "name:version" into its parts. The version
// defaults to "latest".
func parsePersonaRef(ref string) (name, version string) {
	if i := strings.LastIndex(ref, ":"); i >= 0 {
		return ref[:i], ref[i+1:]
	}
	return ref, "latest"
}

// withheldSkills returns the build narrative: skills physically present in the
// persona repo tree but excluded from the allowlist. This powers the
// "deliberately removed" line in the attestation.
func withheldSkills(e *environment.Environment, rm compose.ResolvedManifest) []domain.AttestationLine {
	skillsDir := filepath.Join(environment.RepoDir(e.Hash), "personas", rm.Persona.Name, "skills")
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		return nil
	}
	allowed := make(map[string]bool, len(rm.Skills))
	for _, s := range rm.Skills {
		allowed[s] = true
	}
	var withheld []string
	for _, ent := range entries {
		if ent.IsDir() && !allowed[ent.Name()] {
			withheld = append(withheld, ent.Name())
		}
	}
	if len(withheld) == 0 {
		return nil
	}
	sort.Strings(withheld)
	return []domain.AttestationLine{{Kind: "skill", Names: withheld}}
}
