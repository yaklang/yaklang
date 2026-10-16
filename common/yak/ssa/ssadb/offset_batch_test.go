package ssadb

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
)

func TestOffsetPreloadMatchesPerVariableReads(t *testing.T) {
	db := setupA2LoaderDB(t)
	require.NoError(t, db.AutoMigrate(&IrOffset{}).Error)
	prog := t.Name()
	invalidateOffsetCache(prog)
	t.Cleanup(func() { invalidateOffsetCache(prog) })
	rows := []*IrOffset{
		{ProgramName: prog, ValueID: 7, VariableName: "x", FileHash: "a", StartOffset: 10, EndOffset: 20},
		{ProgramName: prog, ValueID: 7, VariableName: "x", FileHash: "b", StartOffset: 30, EndOffset: 40},
		{ProgramName: prog, ValueID: 7, VariableName: "alias", FileHash: "a", StartOffset: 10, EndOffset: 20},
		{ProgramName: prog, ValueID: 8, VariableName: "y", FileHash: "c", StartOffset: 1, EndOffset: 2},
		{ProgramName: prog + "-other", ValueID: 7, VariableName: "x", FileHash: "z", StartOffset: 5, EndOffset: 6},
		{ProgramName: prog, ValueID: 7, VariableName: "x", FileHash: "gone", StartOffset: 7, EndOffset: 8},
	}
	require.NoError(t, db.CreateInBatches(rows, 10).Error)
	require.NoError(t, db.Delete(rows[len(rows)-1]).Error)

	before := NativeOffsetQueries()
	require.NoError(t, PreloadOffsetsByValueIDs(db, " "+prog+" ", []int64{7, 8, 7, 0, -1}))
	require.Equal(t, before+1, NativeOffsetQueries(), "one IN query should load every requested value")

	for _, tc := range []struct {
		name  string
		value int64
	}{
		{"x", 7}, {"alias", 7}, {"absent", 7}, {"y", 8},
	} {
		var want []*IrOffset
		require.NoError(t, db.New().Model(&IrOffset{}).Where("program_name = ? AND variable_name = ? AND value_id = ?", prog, tc.name, tc.value).Find(&want).Error)
		got := GetOffsetByVariable(tc.name, tc.value, prog)
		require.Equal(t, want, got)
	}
	require.Equal(t, before+1, NativeOffsetQueries(), "preloaded values and known-absent names must not query again")
	var other []*IrOffset
	require.NoError(t, db.New().Model(&IrOffset{}).Where("program_name = ? AND variable_name = ? AND value_id = ?", prog+"-other", "x", 7).Find(&other).Error)
	require.Equal(t, other, GetOffsetByVariable("x", 7, prog+"-other"))
}

func TestOffsetPreloadDoesNotCacheEmptyOrShareRows(t *testing.T) {
	db := setupA2LoaderDB(t)
	require.NoError(t, db.AutoMigrate(&IrOffset{}).Error)
	prog := t.Name()
	invalidateOffsetCache(prog)
	t.Cleanup(func() { invalidateOffsetCache(prog) })
	before := NativeOffsetQueries()
	require.NoError(t, PreloadOffsetsByValueIDs(db, prog, []int64{7}))
	require.Equal(t, before+1, NativeOffsetQueries())
	require.NoError(t, db.Create(&IrOffset{ProgramName: prog, ValueID: 7, VariableName: "x", FileHash: "a", EndOffset: 20}).Error)
	got := GetOffsetByVariable("x", 7, prog)
	require.Len(t, got, 1)
	require.Equal(t, int64(20), got[0].EndOffset)
	require.Greater(t, NativeOffsetQueries(), before+1, "an empty preload must not become a negative cache entry")
	require.NoError(t, PreloadOffsetsByValueIDs(db, prog, []int64{7}))
	cached := GetOffsetByVariable("x", 7, prog)
	require.Len(t, cached, 1)
	cached[0].EndOffset = 99
	again := GetOffsetByVariable("x", 7, prog)
	require.Len(t, again, 1)
	require.Equal(t, int64(20), again[0].EndOffset, "callers must not share cached offset rows")
}

func TestOffsetPreloadInvalidatesAfterBatchSave(t *testing.T) {
	db := setupA2LoaderDB(t)
	require.NoError(t, db.AutoMigrate(&IrOffset{}).Error)
	prog := t.Name()
	invalidateOffsetCache(prog)
	t.Cleanup(func() { invalidateOffsetCache(prog) })
	require.NoError(t, db.Create(&IrOffset{ProgramName: prog, ValueID: 7, VariableName: "x", FileHash: "a", EndOffset: 20}).Error)
	require.NoError(t, PreloadOffsetsByValueIDs(db, prog, []int64{7}))
	require.Len(t, GetOffsetByVariable("x", 7, prog), 1)
	require.NoError(t, SaveIrOffsetBatch(db, []*IrOffset{{
		ProgramName: prog, ValueID: 7, VariableName: "x", FileHash: "b", StartOffset: 30, EndOffset: 40,
	}}))
	got := GetOffsetByVariable("x", 7, prog)
	require.Len(t, got, 2)
	require.Equal(t, int64(40), got[1].EndOffset)
}

func TestOffsetPreloadLegacySchemaFallsBackPerVariable(t *testing.T) {
	oldDB := GetDB()
	db, err := gorm.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	db.DB().SetMaxOpenConns(1)
	SetDB(db)
	t.Cleanup(func() { SetDB(oldDB); require.NoError(t, db.Close()) })
	prog := t.Name()
	invalidateOffsetCache(prog)
	require.NoError(t, db.Exec(`CREATE TABLE ir_offsets (id INTEGER PRIMARY KEY, program_name TEXT, value_id INTEGER, variable_name TEXT, file_hash TEXT, start_offset INTEGER, end_offset INTEGER, deleted_at DATETIME)`).Error)
	require.NoError(t, db.Exec(`INSERT INTO ir_offsets(id,program_name,value_id,variable_name,file_hash,start_offset,end_offset) VALUES(1,?,7,'x','a',10,20)`, prog).Error)
	require.Error(t, PreloadOffsetsByValueIDs(db, prog, []int64{7}))
	rows := GetOffsetByVariable("x", 7, prog)
	require.Len(t, rows, 1)
	require.Equal(t, int64(20), rows[0].EndOffset)
}
