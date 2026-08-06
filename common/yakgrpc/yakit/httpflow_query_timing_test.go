package yakit

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/yaklang/gorm"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestQueryHTTPFlowRecordsCountAndDataQueryDurations(t *testing.T) {
	db, err := gorm.Open("sqlite3", filepath.Join(t.TempDir(), "project.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, db.AutoMigrate(&schema.HTTPFlow{}).Error)

	for index := 0; index < 3; index++ {
		path := fmt.Sprintf("/%d", index)
		require.NoError(t, db.Create(&schema.HTTPFlow{
			Hash:       fmt.Sprintf("query-timing-%d", index),
			Url:        "http://example.test" + path,
			Path:       path,
			Method:     "GET",
			SourceType: schema.HTTPFlow_SourceType_MITM,
		}).Error)
	}

	for _, offsetID := range []int64{0, 1} {
		paging, flows, queryErr := QueryHTTPFlow(db, &ypb.QueryHTTPFlowRequest{
			SourceType: schema.HTTPFlow_SourceType_MITM,
			OffsetId:   offsetID,
			Pagination: &ypb.Paging{
				Page:    1,
				Limit:   2,
				OrderBy: "id",
				Order:   "asc",
			},
		})
		require.NoError(t, queryErr)
		require.Len(t, flows, 2)
		require.True(t, paging.CountExecuted)
		require.Positive(t, paging.CountQueryDuration)
		require.Positive(t, paging.DataQueryDuration)
	}

	paging, flows, queryErr := QueryHTTPFlow(db, &ypb.QueryHTTPFlowRequest{
		SourceType: schema.HTTPFlow_SourceType_MITM,
		AfterId:    1,
		SkipTotal:  true,
		Pagination: &ypb.Paging{
			Page:    1,
			Limit:   2,
			OrderBy: "id",
			Order:   "asc",
		},
	})
	require.NoError(t, queryErr)
	require.Len(t, flows, 2)
	require.False(t, paging.CountExecuted)
	require.Zero(t, paging.CountQueryDuration)
	require.Positive(t, paging.DataQueryDuration)
	require.Zero(t, paging.TotalRecord)
}
