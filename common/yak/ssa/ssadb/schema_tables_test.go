package ssadb

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
)

// A fresh SSA database must hold every table this build registers.
//
// The const pool and the ir_codes const lookup are what the compile writes and
// what a delete removes per program. When the migration silently lost a table,
// the first symptom was a "no such table" deep inside an unrelated query, and
// the delete path paid a retry backoff for it on every program. This test pins
// the invariant to the schema registry instead of to one table name.
func TestFreshSSADatabaseHasEveryRegisteredTable(t *testing.T) {
	dir := t.TempDir()
	db, err := consts.CreateSSAProjectDatabaseRaw(filepath.Join(dir, "schema.db"))
	require.NoError(t, err, "creating a fresh SSA database must not report success after a failed migration")
	t.Cleanup(func() { _ = db.Close() })

	// The tables this build reads and writes; a missing one breaks a code path
	// that has no schema check of its own.
	for _, table := range []string{
		TableIrCodes,
		TableIrConstPool,
		TableIrIndices,
		TableIrNamePool,
		TableIrSources,
		TableIrTypes,
		TableIrOffsets,
		TableIrPrograms,
	} {
		require.True(t, db.HasTable(table), "fresh SSA database is missing table %q", table)
	}
}

// The legacy const table is not part of a new database: this build writes the
// const text to the pool, and only a database produced by an intermediate
// build carries ir_consts_v1. Pin that so a delete keeps treating it as
// optional, and so nobody "fixes" the absence by creating an empty table.
func TestFreshSSADatabaseHasNoLegacyConstTable(t *testing.T) {
	dir := t.TempDir()
	db, err := consts.CreateSSAProjectDatabaseRaw(filepath.Join(dir, "legacy.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	require.False(t, db.HasTable(TableIrConsts),
		"%q is the legacy const table; a new database must not create it", TableIrConsts)
}

// A program delete must not spend the retry backoff on a table that this
// schema does not have. The backoff is 50ms + 100ms per optional table, which
// one syntaxflow package run paid once per test through the test helper.
func TestDeleteProgramDoesNotPayBackoffForMissingTables(t *testing.T) {
	db := setupA3ConstDB(t)

	// A program name that has no rows anywhere: the delete only touches absent
	// rows, so all of the measured time would be backoff.
	started := time.Now()
	require.NoError(t, DeleteProgramIrCode(db, "no-such-program-name"))
	elapsed := time.Since(started)

	require.Less(t, elapsed, 50*time.Millisecond,
		"deleting an absent program took %v; a missing optional table must not be retried", elapsed)
}
