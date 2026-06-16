package storage

import (
	"testing"

	"github.com/a2ngerer/agent-containers/internal/domain"
	"github.com/stretchr/testify/require"
)

// fakeEngine exists only to prove the interface signatures compile exactly as
// the contract specifies. If any method signature drifts, this file fails to build.
type fakeEngine struct{}

func (fakeEngine) PutObject(content []byte) (ObjectID, error)     { return "", nil }
func (fakeEngine) GetObject(id ObjectID) ([]byte, error)          { return nil, nil }
func (fakeEngine) WriteTree(dir string) (ObjectID, error)         { return "", nil }
func (fakeEngine) CheckoutTree(id ObjectID, destDir string) error { return nil }
func (fakeEngine) WriteSnapshot(s domain.Snapshot) (domain.SnapshotID, error) {
	return "", nil
}
func (fakeEngine) ReadSnapshot(id domain.SnapshotID) (domain.Snapshot, error) {
	return domain.Snapshot{}, nil
}
func (fakeEngine) Timeline(persona string) ([]domain.SnapshotID, error) { return nil, nil }
func (fakeEngine) SetTag(persona, version string, id domain.SnapshotID) error {
	return nil
}
func (fakeEngine) ResolveTag(persona, version string) (domain.SnapshotID, error) {
	return "", nil
}
func (fakeEngine) ListTags(persona string) ([]domain.Tag, error) { return nil, nil }
func (fakeEngine) AddRemote(name, url string) error              { return nil }
func (fakeEngine) Push(remote string) error                      { return nil }
func (fakeEngine) Pull(remote string) error                      { return nil }

func TestStorageEngineInterfaceSatisfied(t *testing.T) {
	var e StorageEngine = fakeEngine{}
	require.NotNil(t, e)
}

func TestObjectIDIsString(t *testing.T) {
	id := ObjectID("deadbeef")
	require.Equal(t, "deadbeef", string(id))
}
