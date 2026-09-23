package ssadb

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/schema"
)

func TestRetryProgramDelete(t *testing.T) {
	attempts := 0
	err := retryProgramDelete("ir_codes", func() error {
		attempts++
		if attempts < programDeleteAttempts {
			return errors.New("temporary failure")
		}
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, programDeleteAttempts, attempts)

	attempts = 0
	failure := errors.New("persistent failure")
	err = retryProgramDelete("ir_codes", func() error {
		attempts++
		return failure
	})
	require.ErrorIs(t, err, failure)
	require.ErrorContains(t, err, "ir_codes")
	require.Equal(t, programDeleteAttempts, attempts)
}

func TestDeleteProgramIrCodeContinuesAfterTableFailure(t *testing.T) {
	db := newProgramDeleteTestDB(t, TableIrCodes)
	program := "ir-code-partial"
	seedProgramDeleteRow(t, db, TableIrIndices, "program_name", program)
	seedProgramDeleteRow(t, db, TableAuditResults, "program_name", program)
	require.NoError(t, db.Exec(`DROP TABLE "`+TableAuditEdges+`"`).Error)

	err := DeleteProgramIrCode(db, program)
	require.ErrorContains(t, err, TableIrCodes)
	require.ErrorContains(t, err, TableAuditEdges)
	require.Zero(t, countProgramDeleteRows(t, db, TableIrIndices, "program_name", program))
	require.Zero(t, countProgramDeleteRows(t, db, TableAuditResults, "program_name", program))
}

func TestDeleteProgramCheckedKeepsIdentityForRetry(t *testing.T) {
	db := newProgramDeleteTestDB(t, TableIrCodes)
	program := "full-delete-partial"
	require.NoError(t, db.Create(&IrProgram{ProgramName: program}).Error)
	seedProgramDeleteRow(t, db, TableIrIndices, "program_name", program)
	seedProgramDeleteRow(t, db, TableAuditResults, "program_name", program)
	seedProgramDeleteRow(t, db, schema.TableSSARisks, "program_name", program)
	seedProgramDeleteRow(t, db, schema.TableSyntaxFlowScanTask, "programs", program)

	err := DeleteProgramChecked(db, program)
	require.ErrorContains(t, err, TableIrCodes)
	for _, table := range []string{TableIrIndices, TableAuditResults, schema.TableSSARisks} {
		require.Zero(t, countProgramDeleteRows(t, db, table, "program_name", program), table)
	}
	require.Zero(t, countProgramDeleteRows(t, db, schema.TableSyntaxFlowScanTask, "programs", program))
	require.Equal(t, 1, countProgramDeleteRows(t, db, TableIrPrograms, "program_name", program), "keep program identity so node retries can finish cleanup")

	require.NoError(t, db.Exec(`CREATE TABLE "`+TableIrCodes+`" (program_name TEXT, programs TEXT, folder_path TEXT, file_name TEXT)`).Error)
	require.NoError(t, DeleteProgramChecked(db, program))
	require.Zero(t, countProgramDeleteRows(t, db, TableIrPrograms, "program_name", program))
}

func newProgramDeleteTestDB(t *testing.T, missing string) *gorm.DB {
	t.Helper()
	db, err := gorm.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	db.DB().SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&IrProgram{}).Error)
	for _, table := range []string{
		TableIrCodes, TableIrIndices, TableIrNamePool, TableIrSources, TableIrTypes, TableIrOffsets,
		TableAuditResults, TableAuditNodes, TableAuditEdges,
		schema.TableSSARisks, schema.TableSyntaxFlowScanTask,
	} {
		if table == missing {
			continue
		}
		require.NoError(t, db.Exec(`CREATE TABLE "`+table+`" (program_name TEXT, programs TEXT, folder_path TEXT, file_name TEXT)`).Error, table)
	}
	return db
}

func seedProgramDeleteRow(t *testing.T, db *gorm.DB, table, column, program string) {
	t.Helper()
	require.NoError(t, db.Exec(`INSERT INTO "`+table+`" ("`+column+`") VALUES (?)`, program).Error)
}

func countProgramDeleteRows(t *testing.T, db *gorm.DB, table, column, program string) int {
	t.Helper()
	var count int
	err := db.Raw(`SELECT count(*) FROM "`+table+`" WHERE "`+column+`" = ?`, program).Row().Scan(&count)
	require.NoError(t, err)
	return count
}
