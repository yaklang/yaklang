package irschema

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/consts"
	_ "github.com/yaklang/yaklang/common/yak/ssa/ssadb" // registers SSAProjectTables + patches
	_ "github.com/lib/pq"
)

// TestIRSchemaDrift is the CI maintenance gate. It builds two Postgres 16
// databases from the SAME server and compares their schemas structurally:
//
//	Side A (db_sql): apply 0001_baseline.up.sql via raw database/sql — the
//	  migrator path, what production IR DBs receive.
//	Side B (db_orm): run GORM AutoMigrate + ApplyPatches — the current
//	  code-is-schema path in common/consts/ssa.go, what a developer's
//	  struct tags produce.
//
// If a developer edits a GORM struct tag WITHOUT adding a matching
// NNNN_*.up.sql migration file + bumping CurrentIRSchemaVersion, this test
// fails with a concrete diff telling them exactly which SQL to write.
//
// Env-gated: only runs when TEST_POSTGRES=1. Provisioning is delegated to
// the caller (CI service container or a locally-started `docker run
// postgres:16`). The test reads the connection info from:
//
//	IR_DB_TEST_DSN        — full lib/pq DSN to the admin database (root),
//	                       e.g. "host=127.0.0.1 port=5435 user=palm-user
//	                       password=awesome-palm dbname=palm sslmode=disable".
//	If absent, falls back to PALM_POSTGRES_{HOST,PORT,USER,PASSWORD,DB}
//	                       (same knobs thirdpartyservices.GetPostgresParams uses).
//	If both absent, skips.
//
// This avoids the broken thirdpartyservices helper (which depends on a
// removed docker API type) and works against any PG16 the caller provisions.
func TestIRSchemaDrift(t *testing.T) {
	rootDSN := resolveRootDSN()
	if rootDSN == "" {
		t.Skip("set TEST_POSTGRES=1 and IR_DB_TEST_DSN (or PALM_POSTGRES_* + TEST_POSTGRES=1) to run the IR-schema drift gate")
	}
	if os.Getenv("TEST_POSTGRES") == "" {
		t.Skip("TEST_POSTGRES not set; skipping drift gate")
	}

	ctx := context.Background()

	adminDB, err := sql.Open("postgres", rootDSN)
	if err != nil {
		t.Fatalf("open admin: %v", err)
	}
	defer adminDB.Close()
	if err := adminDB.Ping(); err != nil {
		t.Fatalf("ping admin Postgres: %v (is a PG16 service running at the configured DSN?)", err)
	}

	dbA, dbB := "ir_drift_sql", "ir_drift_orm"
	for _, name := range []string{dbA, dbB} {
		if _, err := adminDB.Exec(`DROP DATABASE IF EXISTS ` + name); err != nil {
			t.Fatalf("drop %s: %v", name, err)
		}
		if _, err := adminDB.Exec(`CREATE DATABASE ` + name); err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
	}

	dsnA := replaceDBName(rootDSN, dbA)
	dsnB := replaceDBName(rootDSN, dbB)

	// Side A: apply baseline SQL via raw database/sql (migrator path).
	base := LookupMigration(1)
	if base == nil {
		t.Fatal("no baseline migration embedded")
	}
	if err := execOn(ctx, dsnA, base.SQL); err != nil {
		t.Fatalf("apply baseline SQL to %s: %v", dbA, err)
	}

	// Side B: GORM AutoMigrate + ApplyPatches (code-is-schema path).
	urlDSN := "postgres://" + userPass(rootDSN) + "@" + hostPort(rootDSN) + "/" + dbB + "?sslmode=disable"
	if err := os.Unsetenv(consts.ENV_SSA_DB_SKIP_MIGRATE); err != nil {
		t.Fatal(err)
	}
	gormDB, err := consts.CreateSSAProjectDatabaseRaw(urlDSN)
	if err != nil {
		t.Fatalf("GORM AutoMigrate to %s: %v", dbB, err)
	}
	_ = gormDB.Close()

	snapA, err := snapshotFromDSN(ctx, dsnA)
	if err != nil {
		t.Fatalf("snapshot %s: %v", dbA, err)
	}
	snapB, err := snapshotFromDSN(ctx, dsnB)
	if err != nil {
		t.Fatalf("snapshot %s: %v", dbB, err)
	}

	if diff := DiffSnapshots(snapA, snapB); diff != "" {
		t.Fatalf("IR schema drift between baseline SQL and GORM AutoMigrate:\n"+
			"Side A = baseline SQL (what the migrator applies to production)\n"+
			"Side B = GORM AutoMigrate (what struct tags produce)\n"+
			"If you edited a GORM struct tag in common/yak/ssa/ssadb/, you MUST add a\n"+
			"matching NNNN_*.up.sql migration file under irschema/migrations/ and bump\n"+
			"CurrentIRSchemaVersion, or production IR DBs will silently diverge.\n%s", diff)
	}

	// Sanity: the embedded baseline snapshot JSON must match the freshly
	// applied baseline SQL. If this fails, regenerate
	// 0001_baseline.snapshot.json (see irschema/README.md).
	exp, err := expectedBaselineSnapshot()
	if err != nil || exp == nil {
		t.Fatalf("embedded baseline snapshot missing/unparseable: %v", err)
	}
	if diff := DiffSnapshots(exp, snapA); diff != "" {
		t.Fatalf("embedded 0001_baseline.snapshot.json drifts from the freshly-applied baseline SQL.\n"+
			"Regenerate the snapshot JSON: see irschema/README.md (snapshot regeneration).\n%s", diff)
	}
}

func execOn(ctx context.Context, dsn, sqlText string) error {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return err
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, sqlText)
	return err
}

func snapshotFromDSN(ctx context.Context, dsn string) (*SchemaSnapshot, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	return Snapshot(ctx, db)
}

// resolveRootDSN returns the admin database DSN, preferring IR_DB_TEST_DSN,
// then synthesizing from PALM_POSTGRES_* knobs, else "".
func resolveRootDSN() string {
	if v := os.Getenv("IR_DB_TEST_DSN"); v != "" {
		return v
	}
	host := os.Getenv("PALM_POSTGRES_HOST")
	port := os.Getenv("PALM_POSTGRES_PORT")
	user := os.Getenv("PALM_POSTGRES_USER")
	pwd := os.Getenv("PALM_POSTGRES_PASSWORD")
	dbname := os.Getenv("PALM_POSTGRES_DB")
	if host == "" {
		return ""
	}
	if port == "" {
		port = "5435"
	}
	if dbname == "" {
		dbname = "palm"
	}
	return fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable", host, port, user, pwd, dbname)
}

// replaceDBName swaps the dbname=... token in a keyword=value DSN.
func replaceDBName(dsn, newDB string) string {
	parts := strings.Fields(dsn)
	for i, tok := range parts {
		if strings.HasPrefix(tok, "dbname=") {
			parts[i] = "dbname=" + newDB
			return strings.Join(parts, " ")
		}
	}
	return dsn + " dbname=" + newDB
}

func dsnField(dsn, key string) string {
	for _, tok := range strings.Fields(dsn) {
		if strings.HasPrefix(tok, key+"=") {
			return strings.TrimPrefix(tok, key+"=")
		}
	}
	return ""
}
func userPass(dsn string) string { return dsnField(dsn, "user") + ":" + dsnField(dsn, "password") }
func hostPort(dsn string) string { return dsnField(dsn, "host") + ":" + dsnField(dsn, "port") }

// TestEmbeddedMigrationsContiguous is a non-Postgres sanity check that the
// embedded migration files are contiguous starting at 1.
func TestEmbeddedMigrationsContiguous(t *testing.T) {
	ms := EmbeddedMigrations()
	if len(ms) == 0 {
		t.Fatal("no migrations embedded")
	}
	for i, m := range ms {
		if m.Version != int64(i+1) {
			t.Fatalf("migration at index %d has version %d, expected %d (versions must be contiguous from 1)", i, m.Version, i+1)
		}
		if m.Checksum == "" {
			t.Fatalf("migration version %d has empty checksum", m.Version)
		}
	}
}

// TestBaselineSQLStableChecksum confirms the embedded checksum matches the
// SQL file on disk (catches a stale embed cache or hand-edited SQL without a
// rebuild). Non-Postgres.
func TestBaselineSQLStableChecksum(t *testing.T) {
	base := LookupMigration(1)
	if base == nil {
		t.Fatal("no baseline migration")
	}
	raw, err := os.ReadFile(filepath.Join("migrations", "0001_baseline.up.sql"))
	if err != nil {
		t.Fatalf("read baseline sql: %v", err)
	}
	if recomputed := sha256Hex(raw); recomputed != base.Checksum {
		t.Fatalf("baseline SQL checksum mismatch: embedded=%s file-on-disk=%s (rebuild the package after editing the SQL)", base.Checksum, recomputed)
	}
	t.Logf("baseline v1 checksum: %s", base.Checksum)
}

// Compile-time assertion that *gorm.DB satisfies the irschema Check signature
// (guards against an accidental API drift if gorm is upgraded).
var _ = func(_ *gorm.DB) {}