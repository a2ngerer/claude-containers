package domain

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSnapshotFields(t *testing.T) {
	ts := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	s := Snapshot{
		ID:        SnapshotID("abc"),
		Persona:   "reviewer",
		Parents:   []SnapshotID{"parent1"},
		Message:   "init",
		Author:    "alexander.angerer",
		Timestamp: ts,
		TreeID:    "tree123",
	}
	require.Equal(t, SnapshotID("abc"), s.ID)
	require.Equal(t, "reviewer", s.Persona)
	require.Equal(t, []SnapshotID{"parent1"}, s.Parents)
	require.Equal(t, ts, s.Timestamp)
	require.Equal(t, "tree123", s.TreeID)
}

func TestTagAndTimeline(t *testing.T) {
	tag := Tag{Persona: "coder", Version: "1.2.0", Target: SnapshotID("c1")}
	require.Equal(t, "1.2.0", tag.Version)

	tl := Timeline{Persona: "coder", Snapshots: []SnapshotID{"c2", "c1"}}
	require.Equal(t, "coder", tl.Persona)
	require.Len(t, tl.Snapshots, 2)
	require.Equal(t, SnapshotID("c2"), tl.Snapshots[0]) // newest first
}

func TestEnforcementFields(t *testing.T) {
	e := Enforcement{
		PermissionMode: "read-only",
		ToolsAllow:     []string{"Read", "Grep"},
		ToolsDeny:      []string{"Write", "Edit"},
	}
	require.Equal(t, "read-only", e.PermissionMode)
	require.Equal(t, []string{"Read", "Grep"}, e.ToolsAllow)
	require.Equal(t, []string{"Write", "Edit"}, e.ToolsDeny)
}

func TestAttestation(t *testing.T) {
	a := Attestation{
		Persona:    "reviewer",
		Version:    "1.2.0",
		Included:   []AttestationLine{{Kind: "skill", Names: []string{"security-review"}}},
		Withheld:   []AttestationLine{{Kind: "skill", Names: []string{"superpowers"}}},
		Denied:     []string{"Write", "Edit"},
		SettingSrc: []string{"user", "project"},
		Clean:      true,
	}
	require.True(t, a.Clean)
	require.Equal(t, "skill", a.Included[0].Kind)
	require.Equal(t, []string{"security-review"}, a.Included[0].Names)
	require.Equal(t, []string{"Write", "Edit"}, a.Denied)
}

func TestSentinelErrors(t *testing.T) {
	require.True(t, errors.Is(ErrPersonaNotFound, ErrPersonaNotFound))
	require.NotEqual(t, ErrPersonaNotFound.Error(), ErrNotInitialized.Error())
	require.NotEqual(t, ErrLocked.Error(), ErrVerifyMismatch.Error())
	require.Contains(t, ErrNotInitialized.Error(), "claude_git init")
}
