// Package irschema governs the SSA IR DB schema version.
//
// irschema is the single source of truth for the IR DB DDL:
//
//   - It embeds the incremental migration SQL files (migrations/*.up.sql).
//   - It exposes the current schema version constant compiled into the binary.
//   - It provides Check (read-only, safe under a DML-only DB role) and Migrate
//     (DDL, only ever run by the standalone yak-ir-migrator binary, never by
//     scannode) APIs.
//
// scannode consumes only Check; legion-control shells out to yak-ir-migrator
// (which calls Migrate). The control plane never imports this package directly
// — it stays decoupled from the yaklang engine tree.
package irschema

// CurrentIRSchemaVersion is the schema version this binary expects / produces.
// Bump this constant AND add a new NNNN_*.up.sql migration file whenever the
// GORM struct tags in common/yak/ssa/ssadb change. The drift test
// (drift_test.go, env-gated TEST_POSTGRES=1) fails CI if a struct change is
// not accompanied by a matching migration file + version bump.
const CurrentIRSchemaVersion int64 = 1

// MinSupportedIRSchemaVersion is the oldest IR DB version this binary can
// safely read from (Phase 3 forward-compatibility window, N-1).
//
// A scannode built from this binary accepts a DB at version N or N-1. A DB
// newer than CurrentIRSchemaVersion is rejected (too-new), so an old node
// never silently misinterprets columns a newer binary introduced.
const MinSupportedIRSchemaVersion int64 = CurrentIRSchemaVersion - 1

// IRDBMigrationLockKey is the fixed Postgres advisory-xact-lock key used by
// Migrate to serialize concurrent migrator runs. It is registered here so no
// other subsystem accidentally reuses the same key. Two migrator instances
// starting against the same IR DB serialize cleanly: the second blocks on
// pg_advisory_xact_lock until the first commits its bootstrap transaction.
const IRDBMigrationLockKey int64 = 72400001

// IRSchemaMigrationsTable is the name of the version-tracking table owned by
// the migrator. It is NOT registered in the GORM SSAProjectTables registry —
// it is bootstrapped by Migrate itself to avoid the chicken-and-egg of GORM
// AutoMigrate creating the table that records AutoMigrate's own version.
const IRSchemaMigrationsTable = "ir_schema_migrations"