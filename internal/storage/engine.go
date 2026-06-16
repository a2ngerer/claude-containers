// internal/storage/engine.go
package storage

import "github.com/a2ngerer/agent-containers/internal/domain"

type ObjectID string

// StorageEngine is the only persistence boundary. The default impl is git-backed,
// but nothing above this interface knows that. Implementations MUST be safe to call
// from a single process at a time (the activate.Lock guards cross-process use).
type StorageEngine interface {
	// content-addressed blobs
	PutObject(content []byte) (ObjectID, error)
	GetObject(id ObjectID) ([]byte, error)

	// persona content trees (a whole persona directory at one point in time)
	WriteTree(dir string) (ObjectID, error)         // snapshot a persona dir, return tree id
	CheckoutTree(id ObjectID, destDir string) error // materialize a tree to destDir

	// snapshots (history)
	WriteSnapshot(s domain.Snapshot) (domain.SnapshotID, error)
	ReadSnapshot(id domain.SnapshotID) (domain.Snapshot, error)
	Timeline(persona string) ([]domain.SnapshotID, error)

	// tags
	SetTag(persona, version string, id domain.SnapshotID) error
	ResolveTag(persona, version string) (domain.SnapshotID, error)
	ListTags(persona string) ([]domain.Tag, error)

	// remotes (sharing)
	AddRemote(name, url string) error
	Push(remote string) error
	Pull(remote string) error
}

// OpenGit opens or initializes the git-backed engine rooted at repoDir.
// Implemented in git.go.
func OpenGit(repoDir string) (StorageEngine, error) {
	return newGitStorageEngine(repoDir)
}
