package ssadb

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDBOpStatsAccumulateByKind(t *testing.T) {
	ResetDBOpStats()
	SetDBOpDialect("postgres")

	RecordDBOp(DBOpQuery, 10*time.Millisecond, false)
	RecordDBOp(DBOpQuery, 30*time.Millisecond, false)
	RecordDBOp(DBOpCreate, 5*time.Millisecond, true)
	RecordDBOp(DBOpUpdate, 8*time.Millisecond, false)
	RecordDBOp(DBOpDelete, 2*time.Millisecond, false)

	first := SnapshotDBOpStats()
	require.Equal(t, "postgres", first.Dialect)
	require.Equal(t, int64(2), first.Ops[DBOpQuery].Count)
	require.Equal(t, int64(40), first.Ops[DBOpQuery].TotalMs)
	require.Equal(t, int64(20), first.Ops[DBOpQuery].AvgMs)
	require.Equal(t, int64(1), first.Ops[DBOpCreate].Count)
	require.Equal(t, int64(1), first.Ops[DBOpCreate].ErrorCount)
	require.Equal(t, int64(1), first.Ops[DBOpUpdate].Count)
	require.Equal(t, int64(1), first.Ops[DBOpDelete].Count)
	require.Equal(t, int64(5), first.TotalCount)
	require.Equal(t, int64(55), first.TotalMs)
	require.Equal(t, int64(1), first.ErrorCount)

	RecordDBOp(DBOpQuery, 20*time.Millisecond, false)
	second := SnapshotDBOpStats()
	delta := DeltaDBOpStats(first, second)
	require.Equal(t, int64(1), delta.Ops[DBOpQuery].Count)
	require.Equal(t, int64(0), delta.Ops[DBOpCreate].Count)
	require.Equal(t, int64(20), delta.Ops[DBOpQuery].TotalMs)
	require.Equal(t, int64(20), delta.Ops[DBOpQuery].AvgMs)
	require.Equal(t, int64(1), delta.TotalCount)
}
