package ssadb

import (
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
)

// The pool key covers the kind, so values that render the same text restore
// with their own raw Go type instead of collapsing into one row.
func TestIrConstPoolDistinguishesKinds(t *testing.T) {
	db := setupA3ConstDB(t)
	prog := "const-kinds"

	type want struct {
		kind IrConstKind
		text string
		raw  any
	}
	cases := []want{
		{IrConstKindNil, "nil", nil},
		{IrConstKindString, "nil", "nil"},
		{IrConstKindInt64, "1", int64(1)},
		{IrConstKindString, "1", "1"},
		{IrConstKindBool, "true", true},
		{IrConstKindString, "true", "true"},
		{IrConstKindString, "", ""},
	}

	ids := make(map[string]int64, len(cases))
	for _, c := range cases {
		id := InternIrConst(prog, c.kind, c.text)
		require.NotZero(t, id, "kind=%d text=%q", c.kind, c.text)
		ids[fmt.Sprintf("%d/%s", c.kind, c.text)] = id
	}
	require.NotEqual(t, ids["0/nil"], ids["4/nil"], "nil and \"nil\" must not share a pool row")
	require.NotEqual(t, ids["2/1"], ids["4/1"], "int 1 and \"1\" must not share a pool row")
	require.NotEqual(t, ids["1/true"], ids["4/true"], "bool true and \"true\" must not share a pool row")

	require.NoError(t, GetIrConstPool(prog).Flush(db))
	for _, c := range cases {
		id := ids[fmt.Sprintf("%d/%s", c.kind, c.text)]
		value := GetIrConstValue(db, prog, id)
		require.NotNil(t, value, "kind=%d text=%q", c.kind, c.text)
		require.Equal(t, c.kind, value.Kind, "kind=%d text=%q", c.kind, c.text)
		require.Equal(t, c.raw, value.Raw(), "kind=%d text=%q", c.kind, c.text)
	}
}

// flushIrConstPools has to serve rows assembled outside the codec. It reads
// const_value instead of guessing a string kind: "nil" and "1" are ambiguous
// once the raw value is gone.
func TestFlushIrConstPoolsReadsKindFromExtra(t *testing.T) {
	db := setupA3ConstDB(t)
	prog := "const-fallback"

	rows := []*IrCode{
		{CodeID: 1, ProgramName: prog, Opcode: irCodeConstOpcode, ConstType: irCodeConstTypeNormal,
			String: "nil", ExtraInformation: `{"const_value":null}`},
		{CodeID: 2, ProgramName: prog, Opcode: irCodeConstOpcode, ConstType: irCodeConstTypeNormal,
			String: "nil", ExtraInformation: `{"const_value":"nil"}`},
		{CodeID: 3, ProgramName: prog, Opcode: irCodeConstOpcode, ConstType: irCodeConstTypeNormal,
			String: "1", ExtraInformation: `{"const_value":1}`},
		{CodeID: 4, ProgramName: prog, Opcode: irCodeConstOpcode, ConstType: irCodeConstTypeNormal,
			String: "1", ExtraInformation: `{"const_value":"1"}`},
		{CodeID: 5, ProgramName: prog, Opcode: irCodeConstOpcode, ConstType: irCodeConstTypeNormal,
			String: "true", ExtraInformation: `{"const_value":true}`},
		{CodeID: 6, ProgramName: prog, Opcode: irCodeConstOpcode, ConstType: irCodeConstTypeNormal,
			String: "", ExtraInformation: `{"const_value":""}`},
	}
	require.NoError(t, SaveIrCodeBatch(db, rows))

	expected := map[int64]any{
		1: nil,
		2: "nil",
		3: int64(1),
		4: "1",
		5: true,
		6: "",
	}
	for codeID, raw := range expected {
		var stored IrCode
		require.NoError(t, db.Where("program_name = ? AND code_id = ?", prog, codeID).First(&stored).Error)
		require.NotZero(t, stored.ConstID, "code_id=%d must be pooled from const_value", codeID)
		value := GetIrConstValue(db, prog, stored.ConstID)
		require.NotNil(t, value, "code_id=%d", codeID)
		require.Equal(t, raw, value.Raw(), "code_id=%d", codeID)
	}

	// Without const_value the kind is unknown. The row must stay unpooled
	// instead of being stored as a string.
	unknown := &IrCode{CodeID: 7, ProgramName: prog, Opcode: irCodeConstOpcode,
		ConstType: irCodeConstTypeNormal, String: "unknown"}
	require.NoError(t, SaveIrCodeBatch(db, []*IrCode{unknown}))
	var stored IrCode
	require.NoError(t, db.Where("program_name = ? AND code_id = ?", prog, 7).First(&stored).Error)
	require.Zero(t, stored.ConstID, "a row without const_value must not be guessed as a string")
}

// createConstPoolIndex adds the unique key SaveInBatches upserts on. A bare
// AutoMigrate does not create it; only ensureIrConstIndexes does.
func createConstPoolIndex(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.Exec(
		`CREATE UNIQUE INDEX IF NOT EXISTS "`+irConstPoolProgramIDIndex+`" ON "`+
			TableIrConstPool+`" ("program_name", "const_id");`,
	).Error)
}

// The pool is filled by the concurrent marshal workers and a value only joins
// seen after its row is written. Interning the same literal many times before
// the first flush must still queue exactly one row: PostgreSQL rejects an
// INSERT ... ON CONFLICT statement that carries the same conflict key twice,
// which fails the whole IR batch.
func TestIrConstPoolConcurrentInternKeepsOnePendingRow(t *testing.T) {
	prog := "const-dedupe-" + t.Name()
	pool := GetIrConstPool(prog)
	t.Cleanup(func() { DropIrConstPool(prog) })

	const workers, rounds = 16, 64
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < rounds; j++ {
				pool.Intern(IrConstKindString, "same-literal")
			}
		}()
	}
	wg.Wait()

	pool.mu.Lock()
	pending := len(pool.pending)
	pool.mu.Unlock()
	require.Equal(t, 1, pending, "one literal must queue one const pool row per flush cycle")

	db, err := gorm.Open("sqlite3", filepath.Join(t.TempDir(), "const.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, db.AutoMigrate(&IrConst{}).Error)
	createConstPoolIndex(t, db)

	require.NoError(t, pool.Flush(db))
	id := IrConstPoolID(IrConstKindString, "same-literal")
	var count int64
	require.NoError(t, db.Model(&IrConst{}).
		Where("program_name = ? AND const_id = ?", prog, id).
		Count(&count).Error)
	require.Equal(t, int64(1), count)
}

// A failed flush puts its rows back for the retry. A worker that interns the
// same value again after the failure already has an entry; merging must not
// queue a second row, or the retry repeats the duplicate-key failure.
func TestIrConstPoolFailedFlushRetryKeepsOneRow(t *testing.T) {
	path := filepath.Join(t.TempDir(), "const.db")
	db, err := gorm.Open("sqlite3", path)
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&IrConst{}).Error)
	createConstPoolIndex(t, db)
	sqlDB := db.DB()
	require.NoError(t, sqlDB.Close())

	prog := "const-retry-" + t.Name()
	pool := GetIrConstPool(prog)
	t.Cleanup(func() { DropIrConstPool(prog) })

	id := pool.Intern(IrConstKindString, "retry-literal")
	require.Error(t, pool.Flush(db), "a closed database must fail the batch")

	require.Equal(t, id, pool.Intern(IrConstKindString, "retry-literal"))
	pool.mu.Lock()
	pending := len(pool.pending)
	pool.mu.Unlock()
	require.Equal(t, 1, pending, "the retry must keep one row for the value")

	retry, err := gorm.Open("sqlite3", path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = retry.Close() })
	require.NoError(t, pool.Flush(retry))

	var count int64
	require.NoError(t, retry.Model(&IrConst{}).
		Where("program_name = ? AND const_id = ?", prog, id).
		Count(&count).Error)
	require.Equal(t, int64(1), count)
}
