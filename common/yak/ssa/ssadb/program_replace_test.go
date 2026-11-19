package ssadb

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
)

func TestFinalizeStagedProgramCheckedReplacesTarget(t *testing.T) {
	db := newProgramDeleteTestDB(t, "")
	for _, program := range []string{"replace-target", "replace-staged"} {
		require.NoError(t, db.Create(&IrProgram{ProgramName: program}).Error)
		seedProgramDeleteRow(t, db, TableIrCodes, "program_name", program)
		seedProgramDeleteRow(t, db, TableIrIndices, "program_name", program)
		seedProgramDeleteRow(t, db, TableIrNamePool, "program_name", program)
		seedProgramDeleteRow(t, db, TableIrSources, "program_name", program)
	}
	seedProgramDeleteRow(t, db, TableAuditResults, "program_name", "replace-target")
	seedProgramDeleteRow(t, db, schema.TableSSARisks, "program_name", "replace-target")
	seedProgramDeleteRow(t, db, schema.TableSyntaxFlowScanTask, "programs", "replace-target")

	require.NoError(t, FinalizeStagedProgramChecked(db, "replace-staged", "replace-target", true))

	require.Equal(t, 1, countProgramDeleteRows(t, db, TableIrPrograms, "program_name", "replace-target"))
	require.Zero(t, countProgramDeleteRows(t, db, TableIrPrograms, "program_name", "replace-staged"))
	require.Equal(t, 1, countProgramDeleteRows(t, db, TableIrCodes, "program_name", "replace-target"))
	require.Zero(t, countProgramDeleteRows(t, db, TableIrCodes, "program_name", "replace-staged"))
	require.Zero(t, countProgramDeleteRows(t, db, TableAuditResults, "program_name", "replace-target"))
	require.Zero(t, countProgramDeleteRows(t, db, schema.TableSSARisks, "program_name", "replace-target"))
	require.Zero(t, countProgramDeleteRows(t, db, schema.TableSyntaxFlowScanTask, "programs", "replace-target"))
}

func TestFinalizeStagedProgramCheckedRefusesExistingTargetWithoutReplace(t *testing.T) {
	db := newProgramDeleteTestDB(t, "")
	for _, program := range []string{"keep-target", "keep-staged"} {
		require.NoError(t, db.Create(&IrProgram{ProgramName: program}).Error)
		seedProgramDeleteRow(t, db, TableIrCodes, "program_name", program)
	}

	err := FinalizeStagedProgramChecked(db, "keep-staged", "keep-target", false)
	require.ErrorContains(t, err, "already exists")
	require.Equal(t, 1, countProgramDeleteRows(t, db, TableIrPrograms, "program_name", "keep-target"))
	require.Equal(t, 1, countProgramDeleteRows(t, db, TableIrPrograms, "program_name", "keep-staged"))
	require.Equal(t, 1, countProgramDeleteRows(t, db, TableIrCodes, "program_name", "keep-target"))
}

func TestFinalizeStagedProgramCheckedCreatesMissingTarget(t *testing.T) {
	db := newProgramDeleteTestDB(t, "")
	require.NoError(t, db.Create(&IrProgram{ProgramName: "fresh-staged"}).Error)
	seedProgramDeleteRow(t, db, TableIrCodes, "program_name", "fresh-staged")

	require.NoError(t, FinalizeStagedProgramChecked(db, "fresh-staged", "fresh-target", false))
	require.Equal(t, 1, countProgramDeleteRows(t, db, TableIrPrograms, "program_name", "fresh-target"))
	require.Zero(t, countProgramDeleteRows(t, db, TableIrPrograms, "program_name", "fresh-staged"))
	require.Equal(t, 1, countProgramDeleteRows(t, db, TableIrCodes, "program_name", "fresh-target"))
}

// A missing child table must roll back the rows already deleted from target.
// Otherwise a failed replacement would leave target present but emptied.
func TestFinalizeStagedProgramCheckedRollsBackOnFailure(t *testing.T) {
	db := newProgramDeleteTestDB(t, TableIrIndices)
	for _, program := range []string{"rollback-target", "rollback-staged"} {
		require.NoError(t, db.Create(&IrProgram{ProgramName: program}).Error)
		seedProgramDeleteRow(t, db, TableIrCodes, "program_name", program)
	}

	err := FinalizeStagedProgramChecked(db, "rollback-staged", "rollback-target", true)
	require.ErrorContains(t, err, TableIrIndices)
	require.Equal(t, 1, countProgramDeleteRows(t, db, TableIrPrograms, "program_name", "rollback-target"))
	require.Equal(t, 1, countProgramDeleteRows(t, db, TableIrPrograms, "program_name", "rollback-staged"))
	require.Equal(t, 1, countProgramDeleteRows(t, db, TableIrCodes, "program_name", "rollback-target"))
	require.Equal(t, 1, countProgramDeleteRows(t, db, TableIrCodes, "program_name", "rollback-staged"))
}

func TestFinalizeStagedProgramCheckedRejectsMissingStaged(t *testing.T) {
	db := newProgramDeleteTestDB(t, "")
	require.NoError(t, db.Create(&IrProgram{ProgramName: "only-target"}).Error)

	err := FinalizeStagedProgramChecked(db, "absent-staged", "only-target", true)
	require.ErrorContains(t, err, "does not exist")
	require.Equal(t, 1, countProgramDeleteRows(t, db, TableIrPrograms, "program_name", "only-target"))
}
