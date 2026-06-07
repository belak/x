package migrate

import (
	"context"
	"database/sql"
	"testing"
	"testing/fstest"

	"github.com/alecthomas/assert/v2"
	_ "modernc.org/sqlite"
)

func openTestDB(t *testing.T) (*sql.DB, DB) {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	assert.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	return db, NewDriver(db, SQLiteDialect{})
}

func testLayer(name string, fsys fstest.MapFS) Layer {
	return Layer{Name: name, FS: fsys}
}

func TestMigrateAppliesInOrder(t *testing.T) {
	raw, db := openTestDB(t)

	fsys := fstest.MapFS{
		"migrations/0002_second.sql": {Data: []byte("CREATE TABLE b (id INTEGER);")},
		"migrations/0001_first.sql":  {Data: []byte("CREATE TABLE a (id INTEGER);")},
		"migrations/0003_third.sql":  {Data: []byte("CREATE TABLE c (id INTEGER);")},
	}

	m := New(db, WithLayers(testLayer("test", fsys)))
	result, err := m.Migrate(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, 3, len(result.Applied))
	assert.Equal(t, 3, result.Total)

	assert.Equal(t, "test/0001_first", result.Applied[0])
	assert.Equal(t, "test/0002_second", result.Applied[1])
	assert.Equal(t, "test/0003_third", result.Applied[2])

	for _, table := range []string{"a", "b", "c"} {
		_, err := raw.Exec("SELECT 1 FROM " + table) //nolint:gosec // test-only, table names are hardcoded
		assert.NoError(t, err)
	}
}

func TestMigrateIdempotent(t *testing.T) {
	_, db := openTestDB(t)

	fsys := fstest.MapFS{
		"migrations/0001_init.sql": {Data: []byte("CREATE TABLE t (id INTEGER);")},
	}

	m := New(db, WithLayers(testLayer("test", fsys)))

	r1, err := m.Migrate(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, 1, len(r1.Applied))

	r2, err := m.Migrate(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, 0, len(r2.Applied))
	assert.Equal(t, 1, r2.Total)
}

func TestMigrateIncremental(t *testing.T) {
	_, db := openTestDB(t)

	fsys1 := fstest.MapFS{
		"migrations/0001_init.sql": {Data: []byte("CREATE TABLE a (id INTEGER);")},
	}

	m1 := New(db, WithLayers(testLayer("test", fsys1)))
	r1, err := m1.Migrate(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, 1, len(r1.Applied))

	fsys2 := fstest.MapFS{
		"migrations/0001_init.sql":  {Data: []byte("CREATE TABLE a (id INTEGER);")},
		"migrations/0002_add_b.sql": {Data: []byte("CREATE TABLE b (id INTEGER);")},
	}

	m2 := New(db, WithLayers(testLayer("test", fsys2)))
	r2, err := m2.Migrate(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, 1, len(r2.Applied))
	assert.Equal(t, "test/0002_add_b", r2.Applied[0])
	assert.Equal(t, 2, r2.Total)
}

func TestMigrateMultipleLayers(t *testing.T) {
	_, db := openTestDB(t)

	base := fstest.MapFS{
		"migrations/0001_users.sql": {Data: []byte("CREATE TABLE users (id INTEGER);")},
	}
	app := fstest.MapFS{
		"migrations/0002_items.sql": {Data: []byte("CREATE TABLE items (id INTEGER, user_id INTEGER REFERENCES users(id));")},
	}

	m := New(db, WithLayers(testLayer("base", base), testLayer("app", app)))
	result, err := m.Migrate(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, 2, len(result.Applied))
	assert.Equal(t, "base/0001_users", result.Applied[0])
	assert.Equal(t, "app/0002_items", result.Applied[1])
}

func TestMigrateSameFilenameLayerOrder(t *testing.T) {
	_, db := openTestDB(t)

	base := fstest.MapFS{
		"migrations/20250101_initial.sql": {Data: []byte("CREATE TABLE users (id INTEGER PRIMARY KEY);")},
	}
	app := fstest.MapFS{
		"migrations/20250101_initial.sql": {Data: []byte("CREATE TABLE items (id INTEGER, user_id INTEGER REFERENCES users(id));")},
	}

	m := New(db, WithLayers(testLayer("base", base), testLayer("app", app)))
	result, err := m.Migrate(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, 2, len(result.Applied))
	assert.Equal(t, "base/20250101_initial", result.Applied[0])
	assert.Equal(t, "app/20250101_initial", result.Applied[1])
}

func TestMigrateGlobalFilenameSort(t *testing.T) {
	_, db := openTestDB(t)

	alpha := fstest.MapFS{
		"migrations/20240102_b.sql": {Data: []byte("CREATE TABLE b (id INTEGER);")},
	}
	zebra := fstest.MapFS{
		"migrations/20240101_a.sql": {Data: []byte("CREATE TABLE a (id INTEGER);")},
	}

	m := New(db, WithLayers(testLayer("alpha", alpha), testLayer("zebra", zebra)))
	result, err := m.Migrate(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, 2, len(result.Applied))
	assert.Equal(t, "zebra/20240101_a", result.Applied[0])
	assert.Equal(t, "alpha/20240102_b", result.Applied[1])
}

func TestMigrateRollsBackOnError(t *testing.T) {
	raw, db := openTestDB(t)

	fsys := fstest.MapFS{
		"migrations/0001_good.sql": {Data: []byte("CREATE TABLE good (id INTEGER);")},
		"migrations/0002_bad.sql":  {Data: []byte("INVALID SQL SYNTAX HERE;")},
	}

	m := New(db, WithLayers(testLayer("test", fsys)))
	result, err := m.Migrate(context.Background())
	assert.Error(t, err)

	assert.Equal(t, 1, len(result.Applied))

	_, err = raw.Exec("SELECT 1 FROM good")
	assert.NoError(t, err)

	pending, err := m.Pending(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, 1, len(pending))
	assert.Equal(t, "test/0002_bad", pending[0])
}

func TestPending(t *testing.T) {
	_, db := openTestDB(t)

	fsys := fstest.MapFS{
		"migrations/0001_a.sql": {Data: []byte("CREATE TABLE a (id INTEGER);")},
		"migrations/0002_b.sql": {Data: []byte("CREATE TABLE b (id INTEGER);")},
	}

	m := New(db, WithLayers(testLayer("test", fsys)))

	pending, err := m.Pending(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, 2, len(pending))

	_, err = m.Migrate(context.Background())
	assert.NoError(t, err)

	pending, err = m.Pending(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, 0, len(pending))
}

func TestCustomTableAndDirectory(t *testing.T) {
	raw, db := openTestDB(t)

	fsys := fstest.MapFS{
		"db/0001_init.sql": {Data: []byte("CREATE TABLE x (id INTEGER);")},
	}

	m := New(db,
		WithLayers(testLayer("test", fsys)),
		WithTable("my_migrations"),
		WithDirectory("db"),
	)

	result, err := m.Migrate(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, 1, len(result.Applied))

	var count int
	err = raw.QueryRow("SELECT COUNT(*) FROM my_migrations").Scan(&count)
	assert.NoError(t, err)
	assert.Equal(t, 1, count)
}

func TestEmptyMigrations(t *testing.T) {
	_, db := openTestDB(t)

	fsys := fstest.MapFS{
		"migrations/.gitkeep": {Data: []byte("")},
	}

	m := New(db, WithLayers(testLayer("test", fsys)))
	result, err := m.Migrate(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, 0, len(result.Applied))
	assert.Equal(t, 0, result.Total)
}
