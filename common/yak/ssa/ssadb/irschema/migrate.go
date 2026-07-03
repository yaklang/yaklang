package irschema

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/log"
)

// Errors surfaced by Migrate.
var (
	// ErrIRSchemaAlreadyAtVersion: the DB is already at toVersion (or
	// higher). Returned with exit code 3 by yak-ir-migrator so deploy
	// runbooks can treat it as a no-op success.
	ErrIRSchemaAlreadyAtVersion = errors.New("irschema: DB already at requested version")

	// ErrIRSchemaAdoptDrift: the DB has IR tables but no ir_schema_migrations
	// row, AND its actual schema does not match the baseline. The operator
	// must reconcile (manual ALTER, or --force-adopt to accept the current
	// state as v1). yak-ir-migrator exits 2 and prints the drift diff JSON.
	ErrIRSchemaAdoptDrift = errors.New("irschema: existing IR DB drifts from baseline; refusing to adopt")
)

// MigrateOptions tunes a Migrate call.
type MigrateOptions struct {
	// ToVersion caps the migration target. 0 means "up to MaxEmbeddedVersion()".
	ToVersion int64
	// ForceAdopt overrides ErrIRSchemaAdoptDrift: the current schema is
	// accepted as-is and stamped at the baseline version WITHOUT applying
	// the baseline SQL. Destructive: it lies about what the schema contains.
	// Reserved for disaster recovery; logs a loud warning.
	ForceAdopt bool
	// AppliedBy is recorded in ir_schema_migrations.applied_by. If empty,
	// "$HOSTNAME:yak-ir-migrator" is used.
	AppliedBy string
}

// Migrate runs DDL on the IR DB to bring it up to opts.ToVersion (or the max
// embedded version if 0). It is the ONLY function in this package that issues
// DDL, and it is only ever called from cmd/yak-ir-migrator (the standalone
// binary). scannode never calls it.
//
// Concurrency & locks: Migrate pins a single *sql.Conn from the GORM pool
// (db.DB().Conn) and runs the entire bootstrap + apply through that one conn,
// so the Postgres advisory lock and the DDL share a session. Inside a
// bootstrap transaction it takes pg_advisory_xact_lock(IRDBMigrationLockKey)
// (transaction-scoped: auto-released on commit/rollback, no leak even on
// panic). Two migrator instances starting against the same IR DB serialize
// cleanly.
//
// Adoption (RFC §baseline step + production-readiness for pre-governance DBs):
//
//	case A: ir_schema_migrations absent AND no IR tables   → apply baseline, stamp v1
//	case B: ir_schema_migrations absent AND IR tables exist + schema matches baseline → stamp v1 (NO DDL)
//	case C: ir_schema_migrations absent AND IR tables exist + schema drifts           → ErrIRSchemaAdoptDrift (unless ForceAdopt)
//	case D: ir_schema_migrations present                                                → apply only pending migrations > current
func Migrate(ctx context.Context, db *gorm.DB, opts MigrateOptions) (int64, error) {
	// Build the migration list once so a checksum/build error surfaces early.
	_ = EmbeddedMigrations()
	if cachedErr != nil {
		return 0, fmt.Errorf("irschema: embedded migrations invalid: %w", cachedErr)
	}
	target := opts.ToVersion
	if target == 0 {
		target = MaxEmbeddedVersion()
	}
	if target > MaxEmbeddedVersion() {
		return 0, fmt.Errorf("irschema: target version %d exceeds max embedded %d", target, MaxEmbeddedVersion())
	}
	appliedBy := strings.TrimSpace(opts.AppliedBy)
	if appliedBy == "" {
		hostname, _ := os.Hostname()
		appliedBy = fmt.Sprintf("%s:yak-ir-migrator", hostname)
	}

	// Pin a single connection for the whole migration so the advisory lock
	// and the DDL live on the same session.
	conn, err := db.DB().Conn(ctx)
	if err != nil {
		return 0, fmt.Errorf("irschema: acquire conn: %w", err)
	}
	defer conn.Close()

	// The bootstrap transaction: takes the advisory-xact-lock and (re)checks
	// the current state. All adoption decisions happen inside this txn.
	// bootstrapWorked is true when bootstrap itself applied the baseline or
	// stamped an existing DB (so the loop below has nothing to apply, yet the
	// run was still productive and should report "migrated", not "already at
	// version").
	currentVersion, bootstrapWorked, err := beginWithLockAndAdopt(ctx, conn, opts, appliedBy)
	if err != nil {
		return currentVersion, err
	}

	appliedAnything := bootstrapWorked

	// Apply pending migrations strictly in order. Each migration with NoTx=false
	// runs in its own txn together with its ir_schema_migrations insert, so a
	// failed migration rolls back cleanly and its version row is never recorded.
	for _, m := range EmbeddedMigrations() {
		if m.Version <= currentVersion {
			continue
		}
		if m.Version > target {
			break
		}
		if err := applyOneMigration(ctx, conn, m, appliedBy); err != nil {
			return currentVersion, fmt.Errorf("irschema: apply version %d: %w", m.Version, err)
		}
		log.Infof("irschema: applied migration version=%d checksum=%s notx=%v", m.Version, m.Checksum[:12], m.NoTx)
		currentVersion = m.Version
		appliedAnything = true
	}

	if currentVersion >= target && !appliedAnything {
		// The DB was already at the target before Migrate ran (no baseline
		// applied, no pending migrations). Genuine no-op.
		return currentVersion, ErrIRSchemaAlreadyAtVersion
	}
	return currentVersion, nil
}

// beginWithLockAndAdopt runs inside a short transaction that:
//  1. acquires pg_advisory_xact_lock so concurrent migrators serialize,
//  2. bootstraps ir_schema_migrations if absent,
//  3. runs the baseline adoption logic (cases A/B/C above),
//  4. returns the current applied version after adoption.
//
// The transaction commits before applying any non-baseline pending migrations,
// so a long DDL migration does not hold the advisory lock for the whole run
// (the per-migration transactions re-serialize on the version row). For
// Phase 1 the entire migration set is the baseline, so this distinction is
// moot.
func beginWithLockAndAdopt(ctx context.Context, conn *sql.Conn, opts MigrateOptions, appliedBy string) (int64, bool, error) {
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return 0, false, fmt.Errorf("irschema: begin bootstrap txn: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	// 1. advisory lock (transaction-scoped: auto-released at commit/rollback).
	if _, err := tx.ExecContext(ctx,
		`SELECT pg_advisory_xact_lock($1)`, IRDBMigrationLockKey,
	); err != nil {
		return 0, false, fmt.Errorf("irschema: acquire advisory lock: %w", err)
	}

	// 2. ir_schema_migrations presence.
	hasVersions, err := tableExistsTx(ctx, tx, IRSchemaMigrationsTable)
	if err != nil {
		return 0, false, err
	}

	worked := false
	if !hasVersions {
		// Adoption gate: fresh DB vs pre-governance DB?
		hasIR, err := anyIRTablesPresentTx(ctx, tx)
		if err != nil {
			return 0, false, err
		}
		if hasIR {
			// Case B/C: existing tables, no version row. Compare against baseline.
			match, diff, err := baselineMatchesActual(ctx, tx)
			if err != nil {
				return 0, false, fmt.Errorf("irschema: baseline comparison: %w", err)
			}
			if !match {
				if !opts.ForceAdopt {
					if diff == "" {
						diff = "no embedded baseline snapshot; existing DB cannot be auto-verified. " +
							"Run `yak-ir-migrator --dump-expected` against a known-good IR DB to capture the " +
							"baseline snapshot, or pass --force-adopt after manual schema review."
					}
					return 0, false, fmt.Errorf("%w: %s", ErrIRSchemaAdoptDrift, diff)
				}
				log.Warnf("irschema: --force-adopt set; stamping v1 without structural verification (DESTRUCTIVE). Inspect the DB manually before relying on this.")
			}
			// Stamp: bootstrap the version table, record v1, do NOT re-run baseline DDL.
			if err := bootstrapVersionsTable(ctx, tx); err != nil {
				return 0, false, err
			}
			if err := recordVersion(ctx, tx, 1, baselineChecksum(), appliedBy); err != nil {
				return 0, false, err
			}
			log.Infof("irschema: adopted existing IR DB as v1 (stamp, no DDL; force=%v)", opts.ForceAdopt)
			worked = true
		} else {
			// Case A: fresh DB. Apply baseline SQL, then bootstrap + stamp.
			base := LookupMigration(1)
			if base == nil {
				return 0, false, fmt.Errorf("irschema: no baseline migration (version 1) embedded")
			}
			if _, err := tx.ExecContext(ctx, base.SQL); err != nil {
				return 0, false, fmt.Errorf("irschema: apply baseline SQL: %w", err)
			}
			if err := bootstrapVersionsTable(ctx, tx); err != nil {
				return 0, false, err
			}
			if err := recordVersion(ctx, tx, 1, base.Checksum, appliedBy); err != nil {
				return 0, false, err
			}
			log.Infof("irschema: applied baseline v1 to fresh DB (checksum=%s)", base.Checksum[:12])
			worked = true
		}
	}

	// Read back the current version after (possibly) adoption.
	var current int64
	row := tx.QueryRowContext(ctx, `SELECT coalesce(max(version), 0) FROM `+IRSchemaMigrationsTable)
	if err := row.Scan(&current); err != nil {
		return 0, false, fmt.Errorf("irschema: read current version: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, false, fmt.Errorf("irschema: commit bootstrap txn: %w", err)
	}
	committed = true
	return current, worked, nil
}

func applyOneMigration(ctx context.Context, conn *sql.Conn, m Migration, appliedBy string) error {
	if m.NoTx {
		// Autocommit path (CREATE INDEX CONCURRENTLY, etc.).
		if _, err := conn.ExecContext(ctx, m.SQL); err != nil {
			return err
		}
		// Record version row in a separate short transaction.
		tx, err := conn.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer func() { _ = tx.Rollback() }()
		if err := recordVersion(ctx, tx, m.Version, m.Checksum, appliedBy); err != nil {
			return err
		}
		return tx.Commit()
	}
	// Transactional path: DDL + version row in the same txn.
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	if _, err := tx.ExecContext(ctx, m.SQL); err != nil {
		return err
	}
	if err := recordVersion(ctx, tx, m.Version, m.Checksum, appliedBy); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	committed = true
	return nil
}

func bootstrapVersionsTable(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS `+IRSchemaMigrationsTable+` (
    version    BIGINT PRIMARY KEY,
    checksum   TEXT NOT NULL,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    applied_by TEXT
)`)
	return err
}

func recordVersion(ctx context.Context, tx *sql.Tx, version int64, checksum, appliedBy string) error {
	_, err := tx.ExecContext(ctx,
		`INSERT INTO `+IRSchemaMigrationsTable+` (version, checksum, applied_by) VALUES ($1, $2, $3)
		 ON CONFLICT (version) DO UPDATE SET checksum = EXCLUDED.checksum, applied_at = now(), applied_by = EXCLUDED.applied_by`,
		version, checksum, appliedBy,
	)
	return err
}

func tableExistsTx(ctx context.Context, tx *sql.Tx, table string) (bool, error) {
	var n int
	err := tx.QueryRowContext(ctx,
		`SELECT count(*) FROM information_schema.tables WHERE table_schema='public' AND table_name=$1`,
		table,
	).Scan(&n)
	return n > 0, err
}

func anyIRTablesPresentTx(ctx context.Context, tx *sql.Tx) (bool, error) {
	known := []string{"ir_codes", "ir_programs", "ir_sources", "audit_results", "ssa_risks"}
	for _, t := range known {
		var n int
		if err := tx.QueryRowContext(ctx,
			`SELECT count(*) FROM information_schema.tables WHERE table_schema='public' AND table_name=$1`,
			t,
		).Scan(&n); err != nil {
			return false, err
		}
		if n > 0 {
			return true, nil
		}
	}
	return false, nil
}

func baselineChecksum() string {
	if b := LookupMigration(1); b != nil {
		return b.Checksum
	}
	return ""
}

// baselineMatchesActual compares a pre-governance DB's actual IR schema against
// the baseline snapshot (the schema the baseline SQL would produce). It runs
// inside the bootstrap transaction, so it uses the tx's underlying connection.
//
// The expected snapshot is loaded from the embedded
// migrations/0001_baseline.snapshot.json (captured when the baseline SQL was
// frozen, verified against a live PG16 IR DB by drift_test.go). Returns
// (match=true, "", nil) when the two are structurally equal.
func baselineMatchesActual(ctx context.Context, tx *sql.Tx) (bool, string, error) {
	actual, err := snapshotTx(ctx, tx)
	if err != nil {
		return false, "", err
	}
	expected, err := expectedBaselineSnapshot()
	if err != nil {
		return false, "", err
	}
	if expected == nil {
		// No snapshot available — conservative refusal. Operator must pass
		// --force-adopt after manual review.
		log.Warnf("irschema: no embedded baseline snapshot; refusing auto-adopt")
		return false, "no embedded baseline snapshot", nil
	}
	diff := DiffSnapshots(expected, actual)
	return diff == "", diff, nil
}
