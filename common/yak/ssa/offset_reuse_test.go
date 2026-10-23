package ssa

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

var offsetKeyComparisonSink bool

func TestOffsetDedupKeyDoesNotAllocateStrings(t *testing.T) {
	key := offsetDedupKey(123456, "source-content-hash", 12345, 54321)
	n := testing.AllocsPerRun(100, func() {
		offsetKeyComparisonSink = key == offsetDedupKey(123456, "source-content-hash", 12345, 54321)
	})
	require.Zero(t, n, "checking a saved offset should not allocate ID strings or a joined key")
	require.NotEqual(t, key, offsetDedupKey(123457, "source-content-hash", 12345, 54321))
	require.NotEqual(t, key, offsetDedupKey(123456, "other-source", 12345, 54321))
	require.NotEqual(t, key, offsetDedupKey(123456, "source-content-hash", 12346, 54321))
	require.NotEqual(t, key, offsetDedupKey(123456, "source-content-hash", 12345, 54322))
}

func TestOffsetDedupPreservesVariableAliases(t *testing.T) {
	p := newResidentDeltaProgram(t)
	defer closeUnsavedFlushFixture(t, p)()
	r := memedit.NewRangeFromOffsets(memedit.NewMemEditor("alias"), 0, 5)
	for i := 0; i < 3; i++ {
		for _, name := range []string{"left", "right"} {
			row := ssadb.CreateOffset(r, p.Name)
			row.ValueID, row.VariableName = 10, name
			p.Cache.indexes.saveOffsetDedup(row)
		}
	}
	require.NoError(t, p.Cache.indexes.Flush())
	var rows []ssadb.IrOffset
	require.NoError(t, ssadb.GetDB().Where("program_name = ? AND value_id = ?", p.Name, 10).Find(&rows).Error)
	require.Equal(t, 2, len(rows), "the same value and position may be referenced by different variable names")
}
