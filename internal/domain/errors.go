// internal/domain/errors.go
package domain

import "errors"

var (
	ErrPersonaNotFound = errors.New("persona not found")
	ErrPersonaExists   = errors.New("persona already exists")
	ErrNotInitialized  = errors.New("workspace not initialized (run: claude_git init)")
	ErrLocked          = errors.New("environment is locked by another process")
	ErrVerifyMismatch  = errors.New("materialized environment does not match manifest")
	ErrLayerNotFound   = errors.New("extends layer not found")
)
