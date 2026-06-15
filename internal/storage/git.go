// internal/storage/git.go
package storage

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/angerer/claude_git/internal/domain"
	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/storer"
	"github.com/go-git/go-git/v5/plumbing/transport"
)

const (
	personaRefPrefix = "refs/personas/"
	tagRefPrefix     = "refs/tags/"
)

// GitStorageEngine implements StorageEngine over a bare go-git repository.
// Blobs/trees/commits map onto git objects; persona timelines onto
// refs/personas/<name>; version tags onto refs/tags/<persona>/<version>.
type GitStorageEngine struct {
	repo *git.Repository
}

// newGitStorageEngine is the internal constructor called by OpenGit in engine.go.
func newGitStorageEngine(repoDir string) (StorageEngine, error) {
	repo, err := git.PlainOpen(repoDir)
	if err == nil {
		return &GitStorageEngine{repo: repo}, nil
	}
	if err != git.ErrRepositoryNotExists {
		return nil, fmt.Errorf("open git repo %q: %w", repoDir, err)
	}
	if mkErr := os.MkdirAll(repoDir, 0o755); mkErr != nil {
		return nil, fmt.Errorf("create repo dir %q: %w", repoDir, mkErr)
	}
	repo, err = git.PlainInit(repoDir, true) // bare: no worktree
	if err != nil {
		return nil, fmt.Errorf("init git repo %q: %w", repoDir, err)
	}
	return &GitStorageEngine{repo: repo}, nil
}

// PutObject stores a blob content-addressed in the git object store.
func (g *GitStorageEngine) PutObject(content []byte) (ObjectID, error) {
	obj := g.repo.Storer.NewEncodedObject()
	obj.SetType(plumbing.BlobObject)
	w, err := obj.Writer()
	if err != nil {
		return "", fmt.Errorf("blob writer: %w", err)
	}
	if _, err := w.Write(content); err != nil {
		_ = w.Close()
		return "", fmt.Errorf("write blob: %w", err)
	}
	if err := w.Close(); err != nil {
		return "", fmt.Errorf("close blob writer: %w", err)
	}
	h, err := g.repo.Storer.SetEncodedObject(obj)
	if err != nil {
		return "", fmt.Errorf("store blob: %w", err)
	}
	return ObjectID(h.String()), nil
}

// GetObject retrieves a previously stored blob by its ObjectID.
func (g *GitStorageEngine) GetObject(id ObjectID) ([]byte, error) {
	h := plumbing.NewHash(string(id))
	blob, err := g.repo.BlobObject(h)
	if err != nil {
		return nil, fmt.Errorf("read blob %s: %w", id, err)
	}
	r, err := blob.Reader()
	if err != nil {
		return nil, fmt.Errorf("blob reader %s: %w", id, err)
	}
	defer r.Close()
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("read blob bytes %s: %w", id, err)
	}
	return data, nil
}

// WriteTree snapshots a persona directory recursively into a git tree and
// returns its object id. Subdirectories become nested tree objects.
func (g *GitStorageEngine) WriteTree(dir string) (ObjectID, error) {
	h, err := g.writeTreeRec(dir)
	if err != nil {
		return "", err
	}
	return ObjectID(h.String()), nil
}

func (g *GitStorageEngine) writeTreeRec(dir string) (plumbing.Hash, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return plumbing.ZeroHash, fmt.Errorf("read dir %q: %w", dir, err)
	}
	var treeEntries []object.TreeEntry
	for _, de := range entries {
		full := filepath.Join(dir, de.Name())
		if de.IsDir() {
			sub, err := g.writeTreeRec(full)
			if err != nil {
				return plumbing.ZeroHash, err
			}
			treeEntries = append(treeEntries, object.TreeEntry{
				Name: de.Name(),
				Mode: filemode.Dir,
				Hash: sub,
			})
			continue
		}
		content, err := os.ReadFile(full)
		if err != nil {
			return plumbing.ZeroHash, fmt.Errorf("read file %q: %w", full, err)
		}
		blobID, err := g.PutObject(content)
		if err != nil {
			return plumbing.ZeroHash, err
		}
		treeEntries = append(treeEntries, object.TreeEntry{
			Name: de.Name(),
			Mode: filemode.Regular,
			Hash: plumbing.NewHash(string(blobID)),
		})
	}
	// git requires tree entries sorted by name for a canonical, content-addressed hash.
	sort.Slice(treeEntries, func(i, j int) bool { return treeEntries[i].Name < treeEntries[j].Name })

	tree := &object.Tree{Entries: treeEntries}
	enc := g.repo.Storer.NewEncodedObject()
	if err := tree.Encode(enc); err != nil {
		return plumbing.ZeroHash, fmt.Errorf("encode tree: %w", err)
	}
	h, err := g.repo.Storer.SetEncodedObject(enc)
	if err != nil {
		return plumbing.ZeroHash, fmt.Errorf("store tree: %w", err)
	}
	return h, nil
}

// CheckoutTree walks a stored tree object back onto destDir.
func (g *GitStorageEngine) CheckoutTree(id ObjectID, destDir string) error {
	tree, err := g.repo.TreeObject(plumbing.NewHash(string(id)))
	if err != nil {
		return fmt.Errorf("read tree %s: %w", id, err)
	}
	return g.checkoutTreeRec(tree, destDir)
}

func (g *GitStorageEngine) checkoutTreeRec(tree *object.Tree, destDir string) error {
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("mkdir %q: %w", destDir, err)
	}
	for _, entry := range tree.Entries {
		target := filepath.Join(destDir, entry.Name)
		if entry.Mode == filemode.Dir {
			sub, err := g.repo.TreeObject(entry.Hash)
			if err != nil {
				return fmt.Errorf("read subtree %s: %w", entry.Hash, err)
			}
			if err := g.checkoutTreeRec(sub, target); err != nil {
				return err
			}
			continue
		}
		blob, err := g.repo.BlobObject(entry.Hash)
		if err != nil {
			return fmt.Errorf("read blob %s: %w", entry.Hash, err)
		}
		r, err := blob.Reader()
		if err != nil {
			return fmt.Errorf("blob reader %s: %w", entry.Hash, err)
		}
		data, err := io.ReadAll(r)
		_ = r.Close()
		if err != nil {
			return fmt.Errorf("read blob bytes %s: %w", entry.Hash, err)
		}
		if err := os.WriteFile(target, data, 0o644); err != nil {
			return fmt.Errorf("write file %q: %w", target, err)
		}
	}
	return nil
}

// WriteSnapshot creates a git commit whose tree is s.TreeID, advancing
// refs/personas/<persona> to it. Parents become commit parents.
func (g *GitStorageEngine) WriteSnapshot(s domain.Snapshot) (domain.SnapshotID, error) {
	sig := object.Signature{
		Name:  s.Author,
		Email: authorEmail(s.Author),
		When:  s.Timestamp,
	}
	commit := &object.Commit{
		Author:    sig,
		Committer: sig,
		Message:   s.Message,
		TreeHash:  plumbing.NewHash(s.TreeID),
	}
	for _, p := range s.Parents {
		commit.ParentHashes = append(commit.ParentHashes, plumbing.NewHash(string(p)))
	}
	enc := g.repo.Storer.NewEncodedObject()
	if err := commit.Encode(enc); err != nil {
		return "", fmt.Errorf("encode commit: %w", err)
	}
	h, err := g.repo.Storer.SetEncodedObject(enc)
	if err != nil {
		return "", fmt.Errorf("store commit: %w", err)
	}
	ref := plumbing.NewHashReference(
		plumbing.ReferenceName(personaRefPrefix+s.Persona),
		h,
	)
	if err := g.repo.Storer.SetReference(ref); err != nil {
		return "", fmt.Errorf("set persona ref %s: %w", s.Persona, err)
	}
	return domain.SnapshotID(h.String()), nil
}

// ReadSnapshot retrieves a snapshot by its ID. The Persona field is resolved
// by scanning persona refs for one that reaches this commit.
func (g *GitStorageEngine) ReadSnapshot(id domain.SnapshotID) (domain.Snapshot, error) {
	commit, err := g.repo.CommitObject(plumbing.NewHash(string(id)))
	if err != nil {
		return domain.Snapshot{}, fmt.Errorf("read commit %s: %w", id, err)
	}
	parents := make([]domain.SnapshotID, 0, len(commit.ParentHashes))
	for _, p := range commit.ParentHashes {
		parents = append(parents, domain.SnapshotID(p.String()))
	}
	return domain.Snapshot{
		ID:        id,
		Persona:   g.personaForCommit(commit.Hash),
		Parents:   parents,
		Message:   commit.Message,
		Author:    commit.Author.Name,
		Timestamp: commit.Author.When,
		TreeID:    commit.TreeHash.String(),
	}, nil
}

// personaForCommit finds which persona ref currently reaches the given commit.
// Best-effort; returns empty string if no persona ref covers it.
// TODO(M2): nondeterministic when multiple personas share an identical tree hash;
// will be resolved by encoding persona name in the commit message/metadata.
func (g *GitStorageEngine) personaForCommit(h plumbing.Hash) string {
	refs, err := g.repo.Storer.IterReferences()
	if err != nil {
		return ""
	}
	defer refs.Close()
	found := ""
	err = refs.ForEach(func(ref *plumbing.Reference) error {
		name := ref.Name().String()
		if !strings.HasPrefix(name, personaRefPrefix) {
			return nil
		}
		persona := strings.TrimPrefix(name, personaRefPrefix)
		iter, err := g.repo.Log(&git.LogOptions{From: ref.Hash()})
		if err != nil {
			return nil
		}
		defer iter.Close()
		_ = iter.ForEach(func(c *object.Commit) error {
			if c.Hash == h {
				found = persona
				return storer.ErrStop
			}
			return nil
		})
		if found != "" {
			return storer.ErrStop
		}
		return nil
	})
	if err != nil && !errors.Is(err, storer.ErrStop) {
		return ""
	}
	return found
}

// Timeline returns all snapshot IDs for a persona, newest first.
func (g *GitStorageEngine) Timeline(persona string) ([]domain.SnapshotID, error) {
	ref, err := g.repo.Storer.Reference(
		plumbing.ReferenceName(personaRefPrefix + persona),
	)
	if err != nil {
		if errors.Is(err, plumbing.ErrReferenceNotFound) {
			return nil, fmt.Errorf("no timeline for persona %q: %w", persona, domain.ErrPersonaNotFound)
		}
		return nil, fmt.Errorf("resolve persona ref %s: %w", persona, err)
	}
	iter, err := g.repo.Log(&git.LogOptions{From: ref.Hash()})
	if err != nil {
		return nil, fmt.Errorf("log persona %s: %w", persona, err)
	}
	defer iter.Close()
	var ids []domain.SnapshotID
	if err := iter.ForEach(func(c *object.Commit) error {
		ids = append(ids, domain.SnapshotID(c.Hash.String()))
		return nil
	}); err != nil {
		return nil, fmt.Errorf("walk persona log %s: %w", persona, err)
	}
	return ids, nil // go-git Log yields newest first
}

// SetTag creates or updates a lightweight tag at refs/tags/<persona>/<version>.
func (g *GitStorageEngine) SetTag(persona, version string, id domain.SnapshotID) error {
	ref := plumbing.NewHashReference(
		plumbing.ReferenceName(tagRefPrefix+persona+"/"+version),
		plumbing.NewHash(string(id)),
	)
	if err := g.repo.Storer.SetReference(ref); err != nil {
		return fmt.Errorf("set tag %s/%s: %w", persona, version, err)
	}
	return nil
}

// ResolveTag looks up refs/tags/<persona>/<version> and returns the target SnapshotID.
func (g *GitStorageEngine) ResolveTag(persona, version string) (domain.SnapshotID, error) {
	ref, err := g.repo.Storer.Reference(
		plumbing.ReferenceName(tagRefPrefix + persona + "/" + version),
	)
	if err != nil {
		if errors.Is(err, plumbing.ErrReferenceNotFound) {
			return "", fmt.Errorf("version %q not found for persona %q: %w", version, persona, domain.ErrPersonaNotFound)
		}
		return "", fmt.Errorf("resolve tag %s/%s: %w", persona, version, err)
	}
	return domain.SnapshotID(ref.Hash().String()), nil
}

// ListTags returns all tags for a persona by scanning refs/tags/<persona>/.
func (g *GitStorageEngine) ListTags(persona string) ([]domain.Tag, error) {
	refs, err := g.repo.Storer.IterReferences()
	if err != nil {
		return nil, fmt.Errorf("iter refs: %w", err)
	}
	defer refs.Close()
	prefix := tagRefPrefix + persona + "/"
	var tags []domain.Tag
	if err := refs.ForEach(func(ref *plumbing.Reference) error {
		name := ref.Name().String()
		if !strings.HasPrefix(name, prefix) {
			return nil
		}
		tags = append(tags, domain.Tag{
			Persona: persona,
			Version: strings.TrimPrefix(name, prefix),
			Target:  domain.SnapshotID(ref.Hash().String()),
		})
		return nil
	}); err != nil {
		return nil, fmt.Errorf("walk tag refs %s: %w", persona, err)
	}
	return tags, nil
}

// AddRemote registers a new remote in the git config.
// Returns an error (wrapping git.ErrRemoteExists) if the name is already taken.
func (g *GitStorageEngine) AddRemote(name, url string) error {
	_, err := g.repo.CreateRemote(&config.RemoteConfig{
		Name: name,
		URLs: []string{url},
	})
	if err != nil {
		return fmt.Errorf("add remote %s: %w", name, err)
	}
	return nil
}

// Push pushes all persona refs and tags to the named remote.
func (g *GitStorageEngine) Push(remote string) error {
	err := g.repo.Push(&git.PushOptions{
		RemoteName: remote,
		RefSpecs: []config.RefSpec{
			"refs/personas/*:refs/personas/*",
			"refs/tags/*:refs/tags/*",
		},
	})
	if err != nil && err != git.NoErrAlreadyUpToDate {
		return fmt.Errorf("push to %s: %w", remote, err)
	}
	return nil
}

// Pull fetches all persona refs and tags from the named remote.
func (g *GitStorageEngine) Pull(remote string) error {
	err := g.repo.Fetch(&git.FetchOptions{
		RemoteName: remote,
		RefSpecs: []config.RefSpec{
			"refs/personas/*:refs/personas/*",
			"refs/tags/*:refs/tags/*",
		},
	})
	// A brand-new remote (e.g. an empty repo created for `init --from`) has
	// nothing to fetch yet: that is a no-op, not a failure.
	if err != nil &&
		!errors.Is(err, git.NoErrAlreadyUpToDate) &&
		!errors.Is(err, transport.ErrEmptyRemoteRepository) {
		return fmt.Errorf("pull from %s: %w", remote, err)
	}
	return nil
}

// authorEmail synthesizes a stable placeholder email so commit objects are
// well-formed even when only an author name is known.
func authorEmail(author string) string {
	a := strings.TrimSpace(author)
	if a == "" {
		a = "unknown"
	}
	return strings.ReplaceAll(a, " ", ".") + "@claude-git.local"
}
