package yakit

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// The scan task row only decorates a scan. When the shared SSA IR database is
// unreachable, ssadb.GetDB() hands out nil for the whole process (the init is
// once-only), and gorm panics on a nil *DB. These lookups must report the
// missing database so the caller can fall back to batch 1 / no level counts.
func TestGetMaxScanBatchNilDatabase(t *testing.T) {
	batch, err := GetMaxScanBatch(nil, []string{"program-a"})
	require.Error(t, err)
	require.Zero(t, batch)
}

func TestGetSSARiskLevelCountNilDatabase(t *testing.T) {
	counts, err := GetSSARiskLevelCount(nil, &ypb.SSARisksFilter{RuntimeID: []string{"task-a"}})
	require.Error(t, err)
	require.Nil(t, counts)
}
