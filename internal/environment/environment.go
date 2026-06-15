// internal/environment/environment.go
package environment

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"

	"github.com/a2ngerer/claude-containers/internal/domain"
	"github.com/a2ngerer/claude-containers/internal/storage"
	toml "github.com/pelletier/go-toml/v2"
)

// EnvConfig is the on-disk representation of env.toml.
type EnvConfig struct {
	WorkspacePath string `toml:"workspace_path"`
	ActivePersona string `toml:"active_persona"`
	Author        string `toml:"author"` // set at Create via defaultAuthor(); snapshot author
}

// Environment binds the tool to one workspace.
type Environment struct {
	Hash      string
	Workspace string
	Store     storage.StorageEngine
	cfg       EnvConfig
}

// Author returns the author string stored in env.toml (set once at Create time).
func (e *Environment) Author() string { return e.cfg.Author }

// ActivePersona returns the currently active persona name.
func (e *Environment) ActivePersona() string { return e.cfg.ActivePersona }

// defaultAuthor derives the author identity for a new environment.
// Priority: git config user.name + <user.email>, then OS user login/name.
func defaultAuthor() string {
	name := gitConfigValue("user.name")
	email := gitConfigValue("user.email")
	if name != "" && email != "" {
		return name + " <" + email + ">"
	}
	if name != "" {
		return name
	}
	if u, err := user.Current(); err == nil {
		if u.Name != "" {
			return u.Name
		}
		if u.Username != "" {
			return u.Username
		}
	}
	return "unknown"
}

// gitConfigValue reads a single git config key from the global config.
// Returns "" on any error.
func gitConfigValue(key string) string {
	out, err := exec.Command("git", "config", "--global", key).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func envConfigPath(hash string) string { return filepath.Join(EnvDir(hash), "env.toml") }
func personasDir(hash string) string   { return filepath.Join(RepoDir(hash), "personas") }
func personaDir(hash, name string) string {
	return filepath.Join(personasDir(hash), name)
}

// Create binds a workspace: makes the env dirs, opens the git-backed repo,
// writes env.toml (including a derived author), and returns the Environment.
func Create(workspace string) (*Environment, error) {
	abs, err := filepath.Abs(workspace)
	if err != nil {
		return nil, fmt.Errorf("resolve workspace %q: %w", workspace, err)
	}
	// EvalSymlinks resolves macOS /var -> /private/var so the hash is stable
	// regardless of whether the caller passes a symlinked or canonical path.
	abs, err = filepath.EvalSymlinks(abs)
	if err != nil {
		return nil, fmt.Errorf("resolve symlinks for workspace %q: %w", workspace, err)
	}
	abs = filepath.Clean(abs)
	hash := WorkspaceHash(abs)

	if err := os.MkdirAll(personasDir(hash), 0o755); err != nil {
		return nil, fmt.Errorf("create personas dir: %w", err)
	}
	store, err := storage.OpenGit(RepoDir(hash))
	if err != nil {
		return nil, fmt.Errorf("open storage: %w", err)
	}
	// Write the secret-safe .gitignore eagerly: it must protect from the very
	// first snapshot, not lazily at push time. See clone.go:writeGitignore.
	if err := writeGitignore(RepoDir(hash)); err != nil {
		return nil, err
	}
	cfg := EnvConfig{
		WorkspacePath: abs,
		ActivePersona: "",
		Author:        defaultAuthor(),
	}
	if err := writeEnvConfig(hash, cfg); err != nil {
		return nil, err
	}
	return &Environment{Hash: hash, Workspace: abs, Store: store, cfg: cfg}, nil
}

// Open loads an already-initialized environment for the workspace.
// Returns domain.ErrNotInitialized when env.toml is absent.
func Open(workspace string) (*Environment, error) {
	abs, err := filepath.Abs(workspace)
	if err != nil {
		return nil, fmt.Errorf("resolve workspace %q: %w", workspace, err)
	}
	// EvalSymlinks resolves macOS /var -> /private/var for a stable hash.
	abs, err = filepath.EvalSymlinks(abs)
	if err != nil {
		return nil, fmt.Errorf("resolve symlinks for workspace %q: %w", workspace, err)
	}
	abs = filepath.Clean(abs)
	hash := WorkspaceHash(abs)

	cfg, err := readEnvConfig(hash)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, domain.ErrNotInitialized
		}
		return nil, err
	}
	store, err := storage.OpenGit(RepoDir(hash))
	if err != nil {
		return nil, fmt.Errorf("open storage: %w", err)
	}
	return &Environment{Hash: hash, Workspace: abs, Store: store, cfg: cfg}, nil
}

// ListPersonas returns all personas stored in the environment repo.
func (e *Environment) ListPersonas() ([]domain.Persona, error) {
	entries, err := os.ReadDir(personasDir(e.Hash))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read personas dir: %w", err)
	}
	var out []domain.Persona
	for _, de := range entries {
		if !de.IsDir() {
			continue
		}
		p, err := e.LoadPersona(de.Name())
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

// LoadPersona reads a persona by name. Returns domain.ErrPersonaNotFound when absent.
func (e *Environment) LoadPersona(name string) (domain.Persona, error) {
	path := filepath.Join(personaDir(e.Hash, name), "persona.toml")
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return domain.Persona{}, fmt.Errorf("%q: %w", name, domain.ErrPersonaNotFound)
		}
		return domain.Persona{}, fmt.Errorf("stat persona %q: %w", name, err)
	}
	return domain.LoadPersonaTOML(path)
}

// SavePersona writes a persona to the environment repo.
func (e *Environment) SavePersona(p domain.Persona) error {
	dir := personaDir(e.Hash, p.Name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create persona dir %q: %w", p.Name, err)
	}
	return domain.SavePersonaTOML(p, filepath.Join(dir, "persona.toml"))
}

// SetActive sets the active persona name and persists it to env.toml.
func (e *Environment) SetActive(name string) error {
	e.cfg.ActivePersona = name
	return writeEnvConfig(e.Hash, e.cfg)
}

func readEnvConfig(hash string) (EnvConfig, error) {
	raw, err := os.ReadFile(envConfigPath(hash))
	if err != nil {
		return EnvConfig{}, err
	}
	var cfg EnvConfig
	if err := toml.Unmarshal(raw, &cfg); err != nil {
		return EnvConfig{}, fmt.Errorf("unmarshal env.toml: %w", err)
	}
	return cfg, nil
}

func writeEnvConfig(hash string, cfg EnvConfig) error {
	if err := os.MkdirAll(EnvDir(hash), 0o755); err != nil {
		return fmt.Errorf("create env dir: %w", err)
	}
	out, err := toml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal env.toml: %w", err)
	}
	if err := os.WriteFile(envConfigPath(hash), out, 0o644); err != nil {
		return fmt.Errorf("write env.toml: %w", err)
	}
	return nil
}
