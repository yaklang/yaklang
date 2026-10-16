package ssadb

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
)

func TestOffsetNativeReadMatchesGORM(t *testing.T) {
	db := setupA2LoaderDB(t)
	require.NoError(t, db.AutoMigrate(&IrOffset{}).Error)
	rows := []*IrOffset{
		{ProgramName: "p1", ValueID: 7, VariableName: "x", FileHash: "a", StartOffset: 10, EndOffset: 20},
		{ProgramName: "p1", ValueID: 7, VariableName: "x", FileHash: "b", StartOffset: 30, EndOffset: 40},
		{ProgramName: "p1", ValueID: 7, VariableName: "alias", FileHash: "a", StartOffset: 10, EndOffset: 20},
		{ProgramName: "p1", ValueID: 7, VariableName: "", FileHash: "a", StartOffset: 10, EndOffset: 20},
		{ProgramName: "p2", ValueID: 7, VariableName: "x", FileHash: "b", StartOffset: 50, EndOffset: 60},
		{ProgramName: "p1", ValueID: 7, VariableName: "x", FileHash: "deleted", StartOffset: 1, EndOffset: 2},
	}
	require.NoError(t, db.CreateInBatches(rows, 10).Error)
	require.NoError(t, db.Delete(rows[len(rows)-1]).Error)
	count := countGORMReadQueries(db)
	for _, tc := range []struct{ name, variable, program string }{
		{"multiple-ranges", "x", "p1"},
		{"alias", "alias", "p1"},
		{"unnamed", "", "p1"},
		{"other-program", "x", "p2"},
		{"trim-program", "x", " p1 "},
		{"all-programs", "x", ""},
		{"miss", "absent", "p1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var want []*IrOffset
			q := db.Model(&IrOffset{}).Where("variable_name = ? and value_id = ?", tc.variable, 7)
			if program := strings.TrimSpace(tc.program); program != "" {
				q = q.Where("program_name = ?", program)
			}
			require.NoError(t, q.Find(&want).Error)
			before := count.Load()
			got := GetOffsetByVariable(tc.variable, 7, tc.program)
			require.Equal(t, want, got, "all offsets and their order/metadata must match the previous reader")
			require.Equal(t, before, count.Load(), "ordinary offset reads should bypass GORM's query/scan machinery")
		})
	}
}

func TestOffsetNativeReadDoesNotCacheMissOrShareRows(t *testing.T) {
	db := setupA2LoaderDB(t)
	require.NoError(t, db.AutoMigrate(&IrOffset{}).Error)
	require.Empty(t, GetOffsetByVariable("x", 7, t.Name()))
	require.NoError(t, db.Create(&IrOffset{ProgramName: t.Name(), ValueID: 7, VariableName: "x", FileHash: "a", EndOffset: 20}).Error)
	first := GetOffsetByVariable("x", 7, t.Name())
	require.Len(t, first, 1)
	first[0].EndOffset = 99
	second := GetOffsetByVariable("x", 7, t.Name())
	require.Len(t, second, 1)
	require.Equal(t, int64(20), second[0].EndOffset, "query callers must not share mutable offset row objects")
}

func TestOffsetNativeFailureStillReadsLegacySchema(t *testing.T) {
	oldDB := GetDB()
	db, err := gorm.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	db.DB().SetMaxOpenConns(1)
	SetDB(db)
	t.Cleanup(func() { SetDB(oldDB); require.NoError(t, db.Close()) })
	// The old/minimal table lacks timestamps needed by the native reader.
	require.NoError(t, db.Exec(`CREATE TABLE ir_offsets (id INTEGER PRIMARY KEY, program_name TEXT, value_id INTEGER, variable_name TEXT, file_hash TEXT, start_offset INTEGER, end_offset INTEGER, deleted_at DATETIME)`).Error)
	require.NoError(t, db.Exec(`INSERT INTO ir_offsets(id,program_name,value_id,variable_name,file_hash,start_offset,end_offset) VALUES(1,?,7,'x','a',10,20)`, t.Name()).Error)
	count := countGORMReadQueries(db)
	rows := GetOffsetByVariable("x", 7, t.Name())
	require.Len(t, rows, 1)
	require.Equal(t, int64(20), rows[0].EndOffset)
	require.Equal(t, int64(1), count.Load(), "actual native errors retain legacy-compatible GORM fallback")
}
