package ssadb

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
)

// TestUniqueConstraint_IrCodesProgramCodeId proves that ir_codes has a
// UNIQUE constraint on (program_name, code_id), preventing duplicate INSERTs.
func TestUniqueConstraint_IrCodesProgramCodeId(t *testing.T) {
	// Use a fresh DB to avoid conflicts with existing test data
	dbPath := filepath.Join(t.TempDir(), "test-unique.db")
	db, err := consts.CreateSSAProjectDatabaseRaw(dbPath)
	require.NoError(t, err, "failed to create fresh DB")
	defer db.Close()

	// Run the patch which creates the unique index
	patchIrCodeIndex(db)

	// Check that the unique index exists
	var indexCount int64
	db.Raw(
		`SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND tbl_name='ir_codes' AND name='ux_ir_codes_program_code'`,
	).Row().Scan(&indexCount)
	require.Equal(t, int64(1), indexCount,
		"ir_codes must have UNIQUE INDEX ux_ir_codes_program_code ON (program_name, code_id)")

	// Try to insert duplicate (program_name, code_id)
	progName := "test-unique-constraint-1"

	// First insert should succeed
	ir1 := &IrCode{
		ProgramName: progName,
		CodeID:      42,
		Opcode:      1,
		Name:        "first",
	}
	require.NoError(t, db.Create(ir1).Error, "first insert should succeed")

	// Second insert with same (program_name, code_id) should FAIL
	ir2 := &IrCode{
		ProgramName: progName,
		CodeID:      42,
		Opcode:      1,
		Name:        "second",
	}
	err2 := db.Create(ir2).Error
	require.Error(t, err2, "second insert with same (program_name, code_id) must fail with UNIQUE constraint violation")
}

// TestUniqueConstraint_DifferentProgramsAllowed proves that different programs
// can have the same code_id without conflict.
func TestUniqueConstraint_DifferentProgramsAllowed(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test-diff-prog.db")
	db, err := consts.CreateSSAProjectDatabaseRaw(dbPath)
	require.NoError(t, err, "failed to create fresh DB")
	defer db.Close()
	patchIrCodeIndex(db)

	progA := "test-unique-constraint-a"
	progB := "test-unique-constraint-b"

	irA := &IrCode{ProgramName: progA, CodeID: 99, Opcode: 1, Name: "a"}
	irB := &IrCode{ProgramName: progB, CodeID: 99, Opcode: 1, Name: "b"}

	require.NoError(t, db.Create(irA).Error, "program A insert should succeed")
	require.NoError(t, db.Create(irB).Error, "program B insert with same code_id should succeed (different program_name)")
}
