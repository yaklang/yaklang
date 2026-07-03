package irschema

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jinzhu/gorm"
)

// CheckResult is returned by Check. It is purely informational; scannode
// only branches on Compatible.
type CheckResult struct {
	// CurrentVersion is the schema version the DB is at, read from
	// ir_schema_migrations. It is 0 when the table is absent (unmanaged DB
	// — the migrator has never run, or this is a pre-governance DB).
	CurrentVersion int64 `json:"current_version"`
	// ExpectedVersion is CurrentIRSchemaVersion (what this binary can produce).
	ExpectedVersion int64 `json:"expected_version"`
	// MinSupported is MinSupportedIRSchemaVersion (oldest this binary reads).
	MinSupported int64 `json:"min_supported"`
	// Compatible is true iff CurrentVersion is within [MinSupported, ExpectedVersion].
	Compatible bool `json:"compatible"`
	// MissingMigrations lists embedded versions strictly greater than
	// CurrentVersion (i.e. not yet applied). Empty when up to date.
	MissingMigrations []int64 `json:"missing_migrations,omitempty"`
}

// ErrCheckReadFailed is returned when Check cannot query ir_schema_migrations
// for a reason other than the table being absent.
var ErrCheckReadFailed = errors.New("irschema: failed to read ir_schema_migrations")

// IsMigrationsTableMissing reports whether err indicates the ir_schema_migrations
// table does not exist. Check returns (CheckResult{CurrentVersion:0,...}, nil)
// in that case, not an error — an unmanaged DB is a normal state pre-migrator.
// This helper is exported for tests and the migrator's adoption logic.
func IsMigrationsTableMissing(err error) bool {
	if err == nil {
		return false
	}
	// Postgres: relation "ir_schema_migrations" does not exist (SQLSTATE 42P01).
	msg := err.Error()
	return strings.Contains(msg, fmt.Sprintf("%q does not exist", IRSchemaMigrationsTable)) ||
		strings.Contains(msg, fmt.Sprintf("\"%s\" does not exist", IRSchemaMigrationsTable)) ||
		strings.Contains(msg, "SQLSTATE 42P01")
}

// tableExists returns true if the named table exists in the public schema.
// It uses gorm's HasTable, which is cross-dialect (Postgres / SQLite / MySQL)
// and cheap. Safe under a DML-only role: HasTable issues a
// `SELECT ... FROM information_schema.tables` (Postgres, world-readable) or
// the equivalent `sqlite_master` lookup (SQLite).
func tableExists(ctx context.Context, db *gorm.DB, table string) (bool, error) {
	// gorm v1 HasTable takes the bare table name and resolves the dialect.
	return db.HasTable(table), nil
}

// anyIRTablesPresent reports whether any of the well-known SSA IR tables exist.
// It is the adoption gate: a DB with ir_schema_migrations absent AND no IR
// tables is fresh (apply baseline); a DB with ir_schema_migrations absent AND
// IR tables present is a pre-governance AutoMigrated DB (adopt/stamp).
func anyIRTablesPresent(ctx context.Context, db *gorm.DB) (bool, error) {
	known := []string{
		"ir_codes", "ir_programs", "ir_sources", "audit_results", "ssa_risks",
	}
	for _, t := range known {
		ok, err := tableExists(ctx, db, t)
		if err != nil {
			return false, err
		}
		if ok {
			return true, nil
		}
	}
	return false, nil
}

// Check is the read-only version gate. It is safe to call from a scannode
// holding only DML privileges on the IR DB: it only reads information_schema
// (world-readable) and ir_schema_migrations (owned by the migrator, but
// SELECT on it is granted to ir_dml_user by the Phase 2 grant scripts).
//
// If ir_schema_migrations does not exist, Check returns a CheckResult with
// CurrentVersion=0 and Compatible=false (an unmanaged DB). It does NOT
// return an error in that case — the absence is a legitimate, observable
// state, and the caller (scannode) simply fails the task fast.
func Check(ctx context.Context, db *gorm.DB) (CheckResult, error) {
	res := CheckResult{
		ExpectedVersion: CurrentIRSchemaVersion,
		MinSupported:    MinSupportedIRSchemaVersion,
	}

	hasTable, err := tableExists(ctx, db, IRSchemaMigrationsTable)
	if err != nil {
		return res, err
	}
	if !hasTable {
		// Unmanaged DB. CurrentVersion stays 0; list every embedded version
		// as missing so the migrator's adoption logic has the full set.
		for _, m := range EmbeddedMigrations() {
			res.MissingMigrations = append(res.MissingMigrations, m.Version)
		}
		res.Compatible = false
		return res, nil
	}

	// Managed DB: read the highest applied version.
	row := db.DB().QueryRowContext(ctx,
		`SELECT coalesce(max(version), 0) FROM `+IRSchemaMigrationsTable,
	)
	if err := row.Scan(&res.CurrentVersion); err != nil {
		return res, fmt.Errorf("%w: read max(version): %v", ErrCheckReadFailed, err)
	}

	for _, m := range EmbeddedMigrations() {
		if m.Version > res.CurrentVersion {
			res.MissingMigrations = append(res.MissingMigrations, m.Version)
		}
	}
	res.Compatible = res.CurrentVersion >= res.MinSupported && res.CurrentVersion <= res.ExpectedVersion
	return res, nil
}
