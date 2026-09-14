package store

import "errors"

// ErrNotFound is returned when a row addressed by id does not exist.
var ErrNotFound = errors.New("store: not found")

// ErrInvalidTransition is returned by SetRunStatus when the run is already
// in a terminal status (done|failed|canceled|skipped): once a run reaches
// one of those, its status must never be overwritten.
var ErrInvalidTransition = errors.New("store: invalid run status transition")
