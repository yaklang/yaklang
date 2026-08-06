package yakgrpc

import (
	"context"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/yaklang/gorm"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"google.golang.org/protobuf/proto"
)

func TestQueryHTTPFlowsSystemTimingIsOptInAndBounded(t *testing.T) {
	db, err := gorm.Open("sqlite3", filepath.Join(t.TempDir(), "project.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, db.AutoMigrate(&schema.HTTPFlow{}).Error)
	require.NoError(t, db.Create(&schema.HTTPFlow{
		Hash:          "system-timing-query",
		Url:           "http://example.test/",
		Path:          "/",
		Method:        "GET",
		Request:       strconv.Quote("GET / HTTP/1.1\r\nHost: example.test\r\n\r\n"),
		Response:      strconv.Quote("HTTP/1.1 200 OK\r\nContent-Type: text/html\r\n\r\n<title>system timing</title>"),
		RequestLength: 42,
		BodyLength:    35,
		StatusCode:    200,
		ContentType:   "text/html",
		SourceType:    schema.HTTPFlow_SourceType_MITM,
	}).Error)

	server := &Server{projectDatabase: db}
	request := &ypb.QueryHTTPFlowRequest{
		SourceType: schema.HTTPFlow_SourceType_MITM,
		Pagination: &ypb.Paging{Page: 1, Limit: 10, OrderBy: "id", Order: "desc"},
	}

	withoutTiming, err := server.QueryHTTPFlows(context.Background(), request)
	require.NoError(t, err)
	require.Nil(t, withoutTiming.GetSystemTiming())

	request.IncludeSystemTiming = true
	withTiming, err := server.QueryHTTPFlows(context.Background(), request)
	require.NoError(t, err)
	timing := withTiming.GetSystemTiming()
	require.NotNil(t, timing)
	require.Equal(t, int64(len(withTiming.GetData())), timing.GetReturnedFlowCount())
	require.LessOrEqual(t, len(timing.GetFlowTimings()), yakit.HTTPFlowTimingQuerySampleLimit)
	require.Equal(t, int64(cap(yakit.DBSaveAsyncChannel)), timing.GetAsyncWriteQueueCapacity())
	require.Greater(t, timing.GetResponseReadyAtUnixMs(), int64(0))
	require.True(t, timing.GetCountExecuted())
	require.GreaterOrEqual(t, timing.GetCountDurationUs(), int64(0))
	require.GreaterOrEqual(t, timing.GetDataQueryDurationUs(), int64(0))

	encoded, err := proto.Marshal(withTiming)
	require.NoError(t, err)
	var decoded ypb.QueryHTTPFlowResponse
	require.NoError(t, proto.Unmarshal(encoded, &decoded))
	require.Equal(t, timing.GetAsyncWriteQueueCapacity(), decoded.GetSystemTiming().GetAsyncWriteQueueCapacity())
	require.Equal(t, timing.GetCountExecuted(), decoded.GetSystemTiming().GetCountExecuted())
	require.Equal(t, timing.GetCountDurationUs(), decoded.GetSystemTiming().GetCountDurationUs())
	require.Equal(t, timing.GetDataQueryDurationUs(), decoded.GetSystemTiming().GetDataQueryDurationUs())

	request.SkipTotal = true
	withoutExactTotal, err := server.QueryHTTPFlows(context.Background(), request)
	require.NoError(t, err)
	require.Len(t, withoutExactTotal.GetData(), 1)
	require.Zero(t, withoutExactTotal.GetTotal())
	require.False(t, withoutExactTotal.GetSystemTiming().GetCountExecuted())
	require.Zero(t, withoutExactTotal.GetSystemTiming().GetCountDurationUs())
	require.GreaterOrEqual(t, withoutExactTotal.GetSystemTiming().GetDataQueryDurationUs(), int64(0))
	request.SkipTotal = false

	request.IncludeSystemTiming = false
	request.ExcludeResponseRaw = true
	projected, err := server.QueryHTTPFlows(context.Background(), request)
	require.NoError(t, err)
	require.Len(t, projected.GetData(), 1)
	require.NotEmpty(t, projected.GetData()[0].GetRequest())
	require.Empty(t, projected.GetData()[0].GetResponse())
	require.Equal(t, "system timing", projected.GetData()[0].GetHtmlTitle())
	require.Equal(t, int64(35), projected.GetData()[0].GetBodyLength())

	request.ExcludeRequestRaw = true
	packetProjected, err := server.QueryHTTPFlows(context.Background(), request)
	require.NoError(t, err)
	require.Len(t, packetProjected.GetData(), 1)
	require.Empty(t, packetProjected.GetData()[0].GetRequest())
	require.Empty(t, packetProjected.GetData()[0].GetResponse())
	require.Equal(t, int64(42), packetProjected.GetData()[0].GetRequestLength())
	require.Equal(t, "system timing", packetProjected.GetData()[0].GetHtmlTitle())

	request.ExcludeRequestRaw = false
	request.ExcludeResponseRaw = false
	canonical, err := server.QueryHTTPFlows(context.Background(), request)
	require.NoError(t, err)
	require.NotEmpty(t, canonical.GetData()[0].GetRequest(), "list projection must not contaminate the normal request cache")
	require.NotEmpty(t, canonical.GetData()[0].GetResponse(), "list projection must not contaminate the normal cache")
}
