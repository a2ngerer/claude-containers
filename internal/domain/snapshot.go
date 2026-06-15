// internal/domain/snapshot.go
package domain

import "time"

type SnapshotID string

type Snapshot struct {
	ID        SnapshotID
	Persona   string
	Parents   []SnapshotID
	Message   string
	Author    string
	Timestamp time.Time
	TreeID    string // storage-backend object id of the persona content tree
}

type Tag struct {
	Persona string
	Version string // semver or "latest"
	Target  SnapshotID
}

type Timeline struct {
	Persona   string
	Snapshots []SnapshotID // newest first
}
