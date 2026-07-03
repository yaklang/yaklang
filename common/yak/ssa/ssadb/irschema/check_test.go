package irschema

import (
	"context"
	"testing"

	"github.com/jinzhu/gorm"
	_ "github.com/mattn/go-sqlite3"
)

// newSQLiteDB returns an in-memory SQLite gorm DB. SQLite is sufficient for
// unit-testing the Check/Migrate state machine (ir_schema_migrations table,
// version recording, adoption logic); the Postgres-specific structural
// snapshot comparison lives in drift_test.go (env-gated, PG16).
func newSQLiteDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	// gorm v1 sqlite uses "sqlite3" driver; clean up on test end.
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// TestCheck_UnmanagedDB: ir_schema_migrations absent → CurrentVersion=0,
// Compatible=false, MissingMigrations lists all embedded versions.
func TestCheck_UnmanagedDB(t *testing.T) {
	db := newSQLiteDB(t)
	res, err := Check(context.Background(), db)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if res.CurrentVersion != 0 {
		t.Errorf("CurrentVersion = %d, want 0", res.CurrentVersion)
	}
	if res.Compatible {
		t.Errorf("Compatible = true on unmanaged DB, want false")
	}
	if len(res.MissingMigrations) == 0 {
		t.Errorf("MissingMigrations empty on unmanaged DB, want all embedded versions")
	}
	if res.ExpectedVersion != CurrentIRSchemaVersion {
		t.Errorf("ExpectedVersion = %d, want %d", res.ExpectedVersion, CurrentIRSchemaVersion)
	}
}

// TestCheck_ManagedDB: after Migrate stamps v1, Check reports Compatible=true.
// NOTE: this uses SQLite, which the baseline SQL (Postgres-flavored) cannot
// be applied to. So this test only exercises the stamp path (no baseline DDL)
// by pre-creating ir_schema_migrations manually. The full fresh-migrate
// path is covered by drift_test.go against real Postgres.
func TestCheck_ManagedDB(t *testing.T) {
	db := newSQLiteDB(t)
	ctx := context.Background()
	// Manually create ir_schema_migrations (as a migrator stamp would).
	if _, err := db.DB().Exec(
		`CREATE TABLE ` + IRSchemaMigrationsTable + ` (version BIGINT PRIMARY KEY, checksum TEXT NOT NULL, applied_at TEXT, applied_by TEXT)`); err != nil {
		t.Fatalf("create versions table: %v", err)
	}
	if _, err := db.DB().Exec(
		`INSERT INTO `+IRSchemaMigrationsTable+` (version, checksum) VALUES (1, 'fake-checksum-for-test')`); err != nil {
		t.Fatalf("insert version: %v", err)
	}
	res, err := Check(ctx, db)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if res.CurrentVersion != 1 {
		t.Errorf("CurrentVersion = %d, want 1", res.CurrentVersion)
	}
	if !res.Compatible {
		t.Errorf("Compatible = false at v1, want true")
	}
	if len(res.MissingMigrations) != 0 {
		t.Errorf("MissingMigrations = %v, want empty", res.MissingMigrations)
	}
}

// TestCheck_TooNew: DB at version 2 while binary expects 1 → Compatible=false
// (too-new path, Phase 3 forward-compat).
func TestCheck_TooNew(t *testing.T) {
	db := newSQLiteDB(t)
	ctx := context.Background()
	db.DB().Exec(`CREATE TABLE ` + IRSchemaMigrationsTable + ` (version BIGINT PRIMARY KEY, checksum TEXT NOT NULL, applied_at TEXT, applied_by TEXT)`)
	db.DB().Exec(`INSERT INTO ` + IRSchemaMigrationsTable + ` (version, checksum) VALUES (2, 'x')`)
	res, err := Check(ctx, db)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if res.CurrentVersion != 2 {
		t.Errorf("CurrentVersion = %d, want 2", res.CurrentVersion)
	}
	if res.Compatible {
		t.Errorf("Compatible = true at v2 (binary expects %d), want false", CurrentIRSchemaVersion)
	}
}

// TestCheck_TooOld: DB at version below MinSupported → Compatible=false.
// With CurrentIRSchemaVersion=1, MinSupported=0, so version 0 (unmanaged) is
// the too-old case, already covered by TestCheck_UnmanagedDB. This test
// documents the boundary: when MinSupported bumps above 0 in a future
// version, this test will need a DB at version (MinSupported-1).
func TestCheck_TooOld(t *testing.T) {
	if MinSupportedIRSchemaVersion <= 0 {
		t.Skip("MinSupported is 0; too-old boundary is the unmanaged case (covered by TestCheck_UnmanagedDB)")
	}
	db := newSQLiteDB(t)
	ctx := context.Background()
	db.DB().Exec(`CREATE TABLE ` + IRSchemaMigrationsTable + ` (version BIGINT PRIMARY KEY, checksum TEXT NOT NULL, applied_at TEXT, applied_by TEXT)`)
	db.DB().Exec(`INSERT INTO ` + IRSchemaMigrationsTable + ` (version, checksum) VALUES (?, 'x')`, MinSupportedIRSchemaVersion-1)
	res, _ := Check(ctx, db)
	if res.Compatible {
		t.Errorf("Compatible = true at version below MinSupported, want false")
	}
}