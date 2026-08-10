package ssaapi_test

import (
	"strconv"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// TestOffsetDedup_AnalyzeDuplicates analyzes the nature of duplicate
// offset rows to determine the correct dedup strategy.
func TestOffsetDedup_AnalyzeDuplicates(t *testing.T) {
	programName := uuid.NewString()
	defer ssadb.DeleteProgram(ssadb.GetDB(), programName)

	fs := filesys.NewVirtualFs()
	fs.AddFile("/proj/main.go", `package main
func main() { println(f0() + f1() + f2()) }`)
	for i := 0; i < 40; i++ {
		fs.AddFile("/proj/file"+strconv.Itoa(i)+".go",
			"package main\nfunc f"+strconv.Itoa(i)+"() int { return "+strconv.Itoa(i)+" }")
	}

	_, err := ssaapi.ParseProjectWithFS(fs,
		ssaapi.WithLanguage(ssaconfig.GO),
		ssaconfig.WithCompileConcurrency(1),
		ssaconfig.WithSetProgramName(programName),
	)
	require.NoError(t, err)

	// Count total offsets
	var totalOffsets int64
	ssadb.GetDB().Table("ir_offsets").Where("program_name = ?", programName).Count(&totalOffsets)

	// Count distinct (value_id, file_hash, start_offset, end_offset)
	var distinctFullRows int64
	ssadb.GetDB().Raw(
		"SELECT COUNT(*) FROM (SELECT DISTINCT value_id, file_hash, start_offset, end_offset FROM ir_offsets WHERE program_name = ?) as d",
		programName,
	).Row().Scan(&distinctFullRows)

	// Count value_ids with >1 offset
	var multiValueIDs int64
	ssadb.GetDB().Raw(
		"SELECT COUNT(*) FROM (SELECT value_id FROM ir_offsets WHERE program_name = ? GROUP BY value_id HAVING COUNT(*) > 1) as m",
		programName,
	).Row().Scan(&multiValueIDs)

	// Count truly identical rows (same value_id+file_hash+start+end)
	var identicalRows int64
	ssadb.GetDB().Raw(
		"SELECT COALESCE(SUM(c - 1), 0) FROM (SELECT value_id, file_hash, start_offset, end_offset, COUNT(*) as c FROM ir_offsets WHERE program_name = ? GROUP BY value_id, file_hash, start_offset, end_offset HAVING c > 1) as dup",
		programName,
	).Row().Scan(&identicalRows)

	t.Logf("total_offsets=%d, distinct_full_rows=%d, multi_value_ids=%d, identical_row_excess=%d",
		totalOffsets, distinctFullRows, multiValueIDs, identicalRows)

	// Show sample duplicates: a value_id with multiple offsets, are the ranges same or different?
	type offsetRow struct {
		ValueID     int64
		FileHash    string
		StartOffset int64
		EndOffset   int64
	}
	var rows []offsetRow
	dbRows, err := ssadb.GetDB().Raw(
		`SELECT value_id, file_hash, start_offset, end_offset FROM ir_offsets
		WHERE program_name = ? AND value_id IN (
			SELECT value_id FROM ir_offsets WHERE program_name = ? GROUP BY value_id HAVING COUNT(*) > 1 LIMIT 3
		) ORDER BY value_id, start_offset`,
		programName, programName,
	).Rows()
	require.NoError(t, err)
	defer dbRows.Close()
	for dbRows.Next() {
		var r offsetRow
		dbRows.Scan(&r.ValueID, &r.FileHash, &r.StartOffset, &r.EndOffset)
		rows = append(rows, r)
	}

	for _, r := range rows {
		t.Logf("  value_id=%d file_hash=%s start=%d end=%d", r.ValueID, r.FileHash[:min(8, len(r.FileHash))], r.StartOffset, r.EndOffset)
	}

	// The fix: truly identical rows (same value_id+file_hash+start+end) are bugs.
	// Different ranges for the same value_id are legitimate (same instruction
	// appears at different source locations in different files).
	if identicalRows > 0 {
		t.Logf("FOUND %d identical row duplicates (same value_id+file_hash+start+end) — these are bugs", identicalRows)
	}

	// Assert: no identical rows should exist (they are true duplicates)
	require.Equal(t, int64(0), identicalRows,
		"no truly identical offset rows (same value_id+file_hash+start+end) should exist, found %d excess",
		identicalRows)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
