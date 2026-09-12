package migrate

import "errors"

// ErrNotFound is returned when a queried record does not exist.
var ErrNotFound = errors.New("not found")

// ErrConflict is returned when an insert or update violates a uniqueness constraint.
var ErrConflict = errors.New("conflict")

// ErrNoMigrations is returned when a Layer contains no .sql files.
var ErrNoMigrations = errors.New("no migrations found")
