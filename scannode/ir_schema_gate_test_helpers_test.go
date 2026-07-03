package scannode

import (
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"

	_ "github.com/lib/pq"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb/irschema"
)

// resolveRootDSNForGateTest returns the admin DSN to the Postgres server,
// mirroring irschema's drift test env convention.
func resolveRootDSNForGateTest() string {
	if v := os.Getenv("IR_DB_TEST_DSN"); v != "" {
		return v
	}
	host := os.Getenv("PALM_POSTGRES_HOST")
	if host == "" {
		return ""
	}
	port := os.Getenv("PALM_POSTGRES_PORT")
	user := os.Getenv("PALM_POSTGRES_USER")
	pwd := os.Getenv("PALM_POSTGRES_PASSWORD")
	dbname := os.Getenv("PALM_POSTGRES_DB")
	if port == "" {
		port = "5435"
	}
	if dbname == "" {
		dbname = "palm"
	}
	return fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable", host, port, user, pwd, dbname)
}

// provisionMigratedDBForGateTest creates a fresh database, applies the
// baseline SQL, and stamps ir_schema_migrations at v1 — mirroring the
// state yak-ir-migrator produces. Returns the DSN to the per-test DB.
func provisionMigratedDBForGateTest(t *testing.T, rootDSN, dbName string) string {
	t.Helper()
	ctx := t.Context()

	// Drop + create the per-test DB on the admin connection.
	adminDB, err := sql.Open("postgres", rootDSN)
	if err != nil {
		t.Fatalf("open admin: %v", err)
	}
	defer adminDB.Close()
	if _, err := adminDB.ExecContext(ctx, `DROP DATABASE IF EXISTS `+dbName); err != nil {
		t.Fatalf("drop %s: %v", dbName, err)
	}
	if _, err := adminDB.ExecContext(ctx, `CREATE DATABASE `+dbName); err != nil {
		t.Fatalf("create %s: %v", dbName, err)
	}

	// Open the per-test DB, apply baseline SQL, stamp v1.
	testDSN := replaceDBNameGate(rootDSN, dbName)
	testDB, err := sql.Open("postgres", testDSN)
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	defer testDB.Close()
	base := irschema.LookupMigration(1)
	if base == nil {
		t.Fatal("no baseline migration embedded")
	}
	if _, err := testDB.ExecContext(ctx, base.SQL); err != nil {
		t.Fatalf("apply baseline: %v", err)
	}
	if _, err := testDB.ExecContext(ctx, fmt.Sprintf(
		`CREATE TABLE %s (version BIGINT PRIMARY KEY, checksum TEXT NOT NULL, applied_at TIMESTAMPTZ NOT NULL DEFAULT now(), applied_by TEXT)`,
		irschema.IRSchemaMigrationsTable)); err != nil {
		t.Fatalf("create versions table: %v", err)
	}
	if _, err := testDB.ExecContext(ctx, fmt.Sprintf(
		`INSERT INTO %s (version, checksum) VALUES (1, $1)`,
		irschema.IRSchemaMigrationsTable),
		base.Checksum); err != nil {
		t.Fatalf("stamp version: %v", err)
	}
	return testDSN
}

// replaceDBNameGate swaps the dbname=... token in a keyword=value DSN.
func replaceDBNameGate(dsn, newDB string) string {
	parts := strings.Fields(dsn)
	for i, tok := range parts {
		if strings.HasPrefix(tok, "dbname=") {
			parts[i] = "dbname=" + newDB
			return strings.Join(parts, " ")
		}
	}
	return dsn + " dbname=" + newDB
}