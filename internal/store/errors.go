package store

import "errors"

// ErrNotFound is returned when a row addressed by id does not exist.
var ErrNotFound = errors.New("store: not found")
