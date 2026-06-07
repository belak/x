// Package migrate provides a minimal database migration runner.
// Migrations are read from one or more named Layer instances (each wrapping
// an fs.FS) and applied in global lexicographic order by filename. Using a
// date-based filename prefix (e.g. 20060102_description.sql) across all
// layers gives a natural chronological ordering regardless of which layer a
// migration comes from.
//
// Applied versions are tracked as "layername/filename" (without .sql) so
// migrations from different layers with the same filename do not collide in
// the tracking table.
//
// Database access and dialect differences are abstracted through the DB
// interface. Use NewDriver to wrap a *sql.DB with a Dialect.
package migrate

import (
	"context"
	"fmt"
	"io/fs"
	"sort"
	"strings"
)

// DB abstracts database operations and dialect-specific SQL. Use
// NewDriver to create one from a *sql.DB and Dialect.
type DB interface {
	Exec(ctx context.Context, query string, args ...any) error
	Query(ctx context.Context, query string, args ...any) (Rows, error)
	Begin(ctx context.Context) (Tx, error)

	CreateTableSQL(table string) string
	InsertVersionSQL(table string) string
	QueryVersionsSQL(table string) string
}

// Rows abstracts result set iteration.
type Rows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
	Close()
}

// Tx abstracts a database transaction.
type Tx interface {
	Exec(ctx context.Context, query string, args ...any) error
	Rollback(ctx context.Context) error
	Commit(ctx context.Context) error
}

// Dialect abstracts dialect-specific SQL differences.
type Dialect interface {
	CreateTableSQL(table string) string
	InsertVersionSQL(table string) string
	QueryVersionsSQL(table string) string
}

// Layer associates a name with an fs.FS containing migration files.
// The name is used to namespace version strings in the tracking table
// (e.g. "auth/20060102_create_users"), so migrations from different
// layers with the same filename do not collide.
type Layer struct {
	Name string
	FS   fs.FS
}

type migrationEntry struct {
	layer Layer
	name  string
}

// Migrator runs migrations against a database.
type Migrator struct {
	db     DB
	layers []Layer
	dir    string
	table  string
}

// Option configures a Migrator.
type Option func(*Migrator)

// WithTable sets the migrations tracking table name (default:
// "schema_migrations").
func WithTable(name string) Option {
	return func(m *Migrator) { m.table = name }
}

// WithDirectory sets the subdirectory within each Layer's FS to read
// migration files from (default: "migrations"). Use "." to read from
// the root.
func WithDirectory(dir string) Option {
	return func(m *Migrator) { m.dir = dir }
}

// WithLayers appends one or more Layer values to the migrator. All
// migrations from all layers are sorted globally by filename before
// being applied, so date-prefixed filenames (e.g. 20060102_name.sql)
// produce a correct chronological order across layers.
func WithLayers(layers ...Layer) Option {
	return func(m *Migrator) { m.layers = append(m.layers, layers...) }
}

// New creates a Migrator. Call WithLayers to register migration layers.
func New(db DB, opts ...Option) *Migrator {
	m := &Migrator{
		db:    db,
		dir:   "migrations",
		table: "schema_migrations",
	}
	for _, o := range opts {
		o(m)
	}
	return m
}

// MigrateResult contains information about a migration run.
type MigrateResult struct {
	Applied []string // versions applied in this run
	Total   int      // total migrations (applied + already present)
}

// Migrate applies all pending migrations in sorted order. Each migration
// runs in its own transaction. Returns the list of newly applied versions.
func (m *Migrator) Migrate(ctx context.Context) (*MigrateResult, error) {
	if err := m.db.Exec(ctx, m.db.CreateTableSQL(m.table)); err != nil {
		return nil, fmt.Errorf("creating migrations table: %w", err)
	}

	applied, err := m.loadApplied(ctx)
	if err != nil {
		return nil, err
	}

	all, err := m.discoverMigrations()
	if err != nil {
		return nil, err
	}

	var pending []migrationEntry
	for _, e := range all {
		if !applied[e.version()] {
			pending = append(pending, e)
		}
	}

	result := &MigrateResult{Total: len(all)}

	for _, e := range pending {
		if err := m.applyMigration(ctx, e); err != nil {
			return result, err
		}
		result.Applied = append(result.Applied, e.version())
	}

	return result, nil
}

// Pending returns the list of migration versions not yet applied.
func (m *Migrator) Pending(ctx context.Context) ([]string, error) {
	if err := m.db.Exec(ctx, m.db.CreateTableSQL(m.table)); err != nil {
		return nil, fmt.Errorf("creating migrations table: %w", err)
	}

	applied, err := m.loadApplied(ctx)
	if err != nil {
		return nil, err
	}

	all, err := m.discoverMigrations()
	if err != nil {
		return nil, err
	}

	var pending []string
	for _, e := range all {
		if !applied[e.version()] {
			pending = append(pending, e.version())
		}
	}
	return pending, nil
}

func (e migrationEntry) version() string {
	return e.layer.Name + "/" + strings.TrimSuffix(e.name, ".sql")
}

func (m *Migrator) loadApplied(ctx context.Context) (map[string]bool, error) {
	rows, err := m.db.Query(ctx, m.db.QueryVersionsSQL(m.table))
	if err != nil {
		return nil, fmt.Errorf("querying applied migrations: %w", err)
	}
	defer rows.Close()

	applied := make(map[string]bool)
	for rows.Next() {
		var version string
		if err := rows.Scan(&version); err != nil {
			return nil, fmt.Errorf("scanning migration version: %w", err)
		}
		applied[version] = true
	}
	return applied, rows.Err()
}

func (m *Migrator) discoverMigrations() ([]migrationEntry, error) {
	var entries []migrationEntry
	for _, layer := range m.layers {
		dirEntries, err := fs.ReadDir(layer.FS, m.dir)
		if err != nil {
			return nil, fmt.Errorf("reading migrations for layer %q from %q: %w", layer.Name, m.dir, err)
		}
		for _, e := range dirEntries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
				entries = append(entries, migrationEntry{layer: layer, name: e.Name()})
			}
		}
	}
	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].name < entries[j].name
	})
	return entries, nil
}

func (m *Migrator) applyMigration(ctx context.Context, e migrationEntry) error {
	path := m.dir + "/" + e.name
	content, err := fs.ReadFile(e.layer.FS, path)
	if err != nil {
		return fmt.Errorf("reading migration %s: %w", e.version(), err)
	}

	tx, err := m.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning transaction for %s: %w", e.version(), err)
	}

	if err := tx.Exec(ctx, string(content)); err != nil {
		_ = tx.Rollback(ctx)
		return fmt.Errorf("executing migration %s: %w", e.version(), err)
	}

	if err := tx.Exec(ctx, m.db.InsertVersionSQL(m.table), e.version()); err != nil {
		_ = tx.Rollback(ctx)
		return fmt.Errorf("recording migration %s: %w", e.version(), err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("committing migration %s: %w", e.version(), err)
	}

	return nil
}
