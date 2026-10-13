package yakit

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestSSARiskResultIdentityCompatibility(t *testing.T) {
	db := ssadb.GetDB()
	runtimeID := uuid.NewString()
	id := uint64(time.Now().UnixNano())
	legacy := &schema.SSARisk{RuntimeId: runtimeID, ResultID: id, Title: "legacy"}
	withUUID := &schema.SSARisk{RuntimeId: runtimeID, ResultID: id, ResultUUID: uuid.NewString(), Title: "new"}
	require.NoError(t, CreateSSARisk(db, legacy))
	require.NoError(t, CreateSSARisk(db, withUUID))
	t.Cleanup(func() { db.Unscoped().Where("runtime_id = ?", runtimeID).Delete(&schema.SSARisk{}) })

	count := func(filter *ypb.SSARisksFilter) int64 {
		var n int64
		require.NoError(t, FilterSSARisk(db.Model(&schema.SSARisk{}), filter).Count(&n).Error)
		return n
	}
	require.Equal(t, int64(2), count(NewSSARiskFilter(WithSSARiskResultID(id))))
	require.Equal(t, int64(1), count(NewSSARiskFilter(WithSSARiskResultUUID(withUUID.ResultUUID))))
	require.Equal(t, int64(1), count(NewSSARiskFilter(WithSSARiskResultID(id), WithSSARiskResultUUID(withUUID.ResultUUID))))
	require.Equal(t, withUUID.ResultUUID, withUUID.ToGRPCModel().GetResultUUID())
	require.Equal(t, id, withUUID.ToGRPCModel().GetResultID())

	// Numeric deletion still removes both legacy and new rows, with no
	// prerequisite AuditResult row and no UUID lookup.
	require.NoError(t, DeleteSSARiskBySFResult(db, []int64{int64(id)}))
	require.Zero(t, count(NewSSARiskFilter(WithSSARiskFilterTaskID(runtimeID))))
}
