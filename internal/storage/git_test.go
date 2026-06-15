package storage

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/angerer/claude_git/internal/domain"
	"github.com/stretchr/testify/require"
)

func newTestEngine(t *testing.T) StorageEngine {
	t.Helper()
	repoDir := filepath.Join(t.TempDir(), "repo")
	e, err := OpenGit(repoDir)
	require.NoError(t, err)
	return e
}

func TestOpenGit_IsIdempotent(t *testing.T) {
	repoDir := filepath.Join(t.TempDir(), "repo")
	_, err := OpenGit(repoDir)
	require.NoError(t, err)
	_, err = OpenGit(repoDir) // second open must not fail
	require.NoError(t, err)
}

func TestPutGetObject_RoundTrip(t *testing.T) {
	e := newTestEngine(t)
	content := []byte("hello persona world")
	id, err := e.PutObject(content)
	require.NoError(t, err)
	require.NotEmpty(t, string(id))

	got, err := e.GetObject(id)
	require.NoError(t, err)
	require.Equal(t, content, got)
}

func TestPutObject_ContentAddressed(t *testing.T) {
	e := newTestEngine(t)
	a, err := e.PutObject([]byte("same"))
	require.NoError(t, err)
	b, err := e.PutObject([]byte("same"))
	require.NoError(t, err)
	require.Equal(t, a, b, "identical content must yield identical id")
}

func TestWriteCheckoutTree_RoundTrip(t *testing.T) {
	e := newTestEngine(t)

	src := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(src, "persona.toml"), []byte("name = \"x\"\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(src, "CLAUDE.md"), []byte("# instructions\n"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(src, "skills", "review"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(src, "skills", "review", "SKILL.md"), []byte("skill body\n"), 0o644))

	treeID, err := e.WriteTree(src)
	require.NoError(t, err)
	require.NotEmpty(t, string(treeID))

	dest := filepath.Join(t.TempDir(), "out")
	require.NoError(t, e.CheckoutTree(treeID, dest))

	for _, rel := range []string{"persona.toml", "CLAUDE.md", filepath.Join("skills", "review", "SKILL.md")} {
		want, err := os.ReadFile(filepath.Join(src, rel))
		require.NoError(t, err)
		got, err := os.ReadFile(filepath.Join(dest, rel))
		require.NoError(t, err, "missing checked-out file %s", rel)
		require.Equal(t, want, got, "content mismatch for %s", rel)
	}
}

func TestWriteReadSnapshot_RoundTrip(t *testing.T) {
	e := newTestEngine(t)

	src := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(src, "persona.toml"), []byte("name = \"coder\"\n"), 0o644))
	treeID, err := e.WriteTree(src)
	require.NoError(t, err)

	when := time.Date(2026, 6, 15, 10, 30, 0, 0, time.UTC)
	snap := domain.Snapshot{
		Persona:   "coder",
		Message:   "initial coder snapshot",
		Author:    "alexander.angerer",
		Timestamp: when,
		TreeID:    string(treeID),
	}
	id, err := e.WriteSnapshot(snap)
	require.NoError(t, err)
	require.NotEmpty(t, string(id))

	got, err := e.ReadSnapshot(id)
	require.NoError(t, err)
	require.Equal(t, "coder", got.Persona)
	require.Equal(t, "initial coder snapshot", got.Message)
	require.Equal(t, "alexander.angerer", got.Author)
	require.Equal(t, when.UTC(), got.Timestamp.UTC())
	require.Equal(t, string(treeID), got.TreeID)
	require.Equal(t, id, got.ID)
}

func TestTimeline_NewestFirst(t *testing.T) {
	e := newTestEngine(t)
	src := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(src, "persona.toml"), []byte("name = \"coder\"\n"), 0o644))
	treeID, err := e.WriteTree(src)
	require.NoError(t, err)

	first, err := e.WriteSnapshot(domain.Snapshot{Persona: "coder", Message: "first", Author: "a", Timestamp: time.Now().UTC(), TreeID: string(treeID)})
	require.NoError(t, err)
	second, err := e.WriteSnapshot(domain.Snapshot{Persona: "coder", Message: "second", Parents: []domain.SnapshotID{first}, Author: "a", Timestamp: time.Now().UTC().Add(time.Second), TreeID: string(treeID)})
	require.NoError(t, err)

	tl, err := e.Timeline("coder")
	require.NoError(t, err)
	require.Len(t, tl, 2)
	require.Equal(t, second, tl[0], "newest snapshot first")
	require.Equal(t, first, tl[1])
}

func TestTags_SetResolveList(t *testing.T) {
	e := newTestEngine(t)
	src := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(src, "persona.toml"), []byte("name = \"reviewer\"\n"), 0o644))
	treeID, err := e.WriteTree(src)
	require.NoError(t, err)
	snapID, err := e.WriteSnapshot(domain.Snapshot{Persona: "reviewer", Message: "m", Author: "a", Timestamp: time.Now().UTC(), TreeID: string(treeID)})
	require.NoError(t, err)

	require.NoError(t, e.SetTag("reviewer", "1.2.0", snapID))
	require.NoError(t, e.SetTag("reviewer", "latest", snapID))

	resolved, err := e.ResolveTag("reviewer", "1.2.0")
	require.NoError(t, err)
	require.Equal(t, snapID, resolved)

	tags, err := e.ListTags("reviewer")
	require.NoError(t, err)
	versions := make([]string, 0, len(tags))
	for _, tg := range tags {
		require.Equal(t, "reviewer", tg.Persona)
		require.Equal(t, snapID, tg.Target)
		versions = append(versions, tg.Version)
	}
	sort.Strings(versions)
	require.Equal(t, []string{"1.2.0", "latest"}, versions)
}

func TestTimeline_NonexistentPersona_ReturnsErrPersonaNotFound(t *testing.T) {
	e := newTestEngine(t)
	_, err := e.Timeline("nonexistent")
	require.Error(t, err)
	require.True(t, errors.Is(err, domain.ErrPersonaNotFound), "expected ErrPersonaNotFound, got: %v", err)
}

func TestResolveTag_NonexistentPersonaVersion_ReturnsErrPersonaNotFound(t *testing.T) {
	e := newTestEngine(t)
	_, err := e.ResolveTag("nonexistent", "1.0.0")
	require.Error(t, err)
	require.True(t, errors.Is(err, domain.ErrPersonaNotFound), "expected ErrPersonaNotFound, got: %v", err)
}

func TestAddRemote_Persists(t *testing.T) {
	e := newTestEngine(t)
	require.NoError(t, e.AddRemote("origin", "https://example.com/personas.git"))
	// adding the same remote name again must error (go-git ErrRemoteExists)
	require.Error(t, e.AddRemote("origin", "https://example.com/personas.git"))
}
