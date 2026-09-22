package yakit

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/model"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestHTTPFlowListProjectionUsesPersistedTitleWithoutResponse(t *testing.T) {
	prev := consts.GetHTTPFlowListInlineMaxContentLength()
	consts.SetHTTPFlowListInlineMaxContentLength(0)
	t.Cleanup(func() { consts.SetHTTPFlowListInlineMaxContentLength(prev) })

	db, err := gorm.Open("sqlite3", filepath.Join(t.TempDir(), "project.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, db.AutoMigrate(&schema.HTTPFlow{}).Error)

	request := []byte("GET /new HTTP/1.1\r\nHost: example.test\r\n\r\n")
	response := []byte("HTTP/1.1 200 OK\r\nContent-Type: text/html\r\n\r\n<html><title>persisted title</title></html>")
	flow, err := CreateHTTPFlowFromHTTPWithBodySavedFromRaw(
		false,
		request,
		response,
		schema.HTTPFlow_SourceType_MITM,
		"http://example.test/new",
		"127.0.0.1:80",
	)
	require.NoError(t, err)
	require.True(t, flow.HtmlTitle.Valid)
	require.Equal(t, "persisted title", flow.HtmlTitle.String)
	require.NoError(t, db.Create(flow).Error)

	_, rows, err := QueryHTTPFlow(db, projectedHTTPFlowQuery(int64(flow.ID)))
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.True(t, rows[0].HtmlTitle.Valid)
	require.Equal(t, "persisted title", rows[0].HtmlTitle.String)
	require.Empty(t, rows[0].Response, "persisted-title rows must not load response into Go")

	projected, err := model.ToHTTPFlowGRPCModelWithoutResponseRaw(rows[0], false)
	require.NoError(t, err)
	require.Equal(t, "persisted title", projected.GetHtmlTitle())
	require.Empty(t, projected.GetResponse())
	require.NotEmpty(t, projected.GetRequest())

	packetQuery := projectedHTTPFlowQuery(int64(flow.ID))
	packetQuery.ExcludeRequestRaw = true
	_, packetRows, err := QueryHTTPFlow(db, packetQuery)
	require.NoError(t, err)
	require.Len(t, packetRows, 1)
	require.Empty(t, packetRows[0].Request, "request projection must not load request into Go")
	require.Empty(t, packetRows[0].Response, "response projection must not load response into Go")
	require.Equal(t, flow.RequestLength, packetRows[0].RequestLength)
	require.Equal(t, flow.BodyLength, packetRows[0].BodyLength)

	packetProjected, err := model.ToHTTPFlowGRPCModelWithListProjection(packetRows[0], false, true, true)
	require.NoError(t, err)
	require.Empty(t, packetProjected.GetRequest())
	require.Empty(t, packetProjected.GetResponse())
	require.Equal(t, flow.RequestLength, packetProjected.GetRequestLength())
	require.Equal(t, "persisted title", packetProjected.GetHtmlTitle())

	// The canonical query contract is unchanged and still returns both packets.
	canonicalQuery := projectedHTTPFlowQuery(int64(flow.ID))
	canonicalQuery.ExcludeResponseRaw = false
	canonicalQuery.ExcludeRequestRaw = false
	_, canonicalRows, err := QueryHTTPFlow(db, canonicalQuery)
	require.NoError(t, err)
	require.Len(t, canonicalRows, 1)
	require.NotEmpty(t, canonicalRows[0].Request)
	require.NotEmpty(t, canonicalRows[0].Response)
}

func TestHTTPFlowListProjectionFallsBackForLegacyNullTitle(t *testing.T) {
	prev := consts.GetHTTPFlowListInlineMaxContentLength()
	consts.SetHTTPFlowListInlineMaxContentLength(0)
	t.Cleanup(func() { consts.SetHTTPFlowListInlineMaxContentLength(prev) })

	db, err := gorm.Open("sqlite3", filepath.Join(t.TempDir(), "project.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, db.AutoMigrate(&schema.HTTPFlow{}).Error)

	flow, err := CreateHTTPFlowFromHTTPWithBodySavedFromRaw(
		false,
		[]byte("GET /legacy HTTP/1.1\r\nHost: example.test\r\n\r\n"),
		[]byte("HTTP/1.1 200 OK\r\nContent-Type: text/html\r\n\r\n<title>legacy title</title>"),
		schema.HTTPFlow_SourceType_MITM,
		"http://example.test/legacy",
		"127.0.0.1:80",
	)
	require.NoError(t, err)
	require.NoError(t, db.Create(flow).Error)
	require.NoError(t, db.Exec("UPDATE http_flows SET html_title = NULL WHERE id = ?", flow.ID).Error)

	_, rows, err := QueryHTTPFlow(db, projectedHTTPFlowQuery(int64(flow.ID)))
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.False(t, rows[0].HtmlTitle.Valid)
	require.NotEmpty(t, rows[0].Response, "legacy rows need response for title fallback")

	projected, err := model.ToHTTPFlowGRPCModelWithoutResponseRaw(rows[0], false)
	require.NoError(t, err)
	require.Equal(t, "legacy title", projected.GetHtmlTitle())
	require.Empty(t, projected.GetResponse())
}

func TestHTTPFlowListProjectionMarksEmptyTitleAsComputed(t *testing.T) {
	prev := consts.GetHTTPFlowListInlineMaxContentLength()
	consts.SetHTTPFlowListInlineMaxContentLength(0)
	t.Cleanup(func() { consts.SetHTTPFlowListInlineMaxContentLength(prev) })

	db, err := gorm.Open("sqlite3", filepath.Join(t.TempDir(), "project.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, db.AutoMigrate(&schema.HTTPFlow{}).Error)

	flow, err := CreateHTTPFlowFromHTTPWithBodySavedFromRaw(
		false,
		[]byte("GET /json HTTP/1.1\r\nHost: example.test\r\n\r\n"),
		[]byte("HTTP/1.1 200 OK\r\nContent-Type: application/json\r\n\r\n{\"ok\":true}"),
		schema.HTTPFlow_SourceType_MITM,
		"http://example.test/json",
		"127.0.0.1:80",
	)
	require.NoError(t, err)
	require.True(t, flow.HtmlTitle.Valid)
	require.Empty(t, flow.HtmlTitle.String)
	require.NoError(t, db.Create(flow).Error)

	_, rows, err := QueryHTTPFlow(db, projectedHTTPFlowQuery(int64(flow.ID)))
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.True(t, rows[0].HtmlTitle.Valid)
	require.Empty(t, rows[0].Response, "computed no-title rows must not fall back to response")
}

func TestHTTPFlowListProjectionInlinesSmallPacketsWithinBudget(t *testing.T) {
	prev := consts.GetHTTPFlowListInlineMaxContentLength()
	consts.SetHTTPFlowListInlineMaxContentLength(consts.DefaultHTTPFlowListInlineMaxContentLength)
	t.Cleanup(func() { consts.SetHTTPFlowListInlineMaxContentLength(prev) })

	db, err := gorm.Open("sqlite3", filepath.Join(t.TempDir(), "project.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, db.AutoMigrate(&schema.HTTPFlow{}).Error)

	request := []byte("GET /inline HTTP/1.1\r\nHost: example.test\r\n\r\n")
	response := []byte("HTTP/1.1 200 OK\r\nContent-Type: text/html\r\n\r\n<html><title>inline title</title></html>")
	flow, err := CreateHTTPFlowFromHTTPWithBodySavedFromRaw(
		false,
		request,
		response,
		schema.HTTPFlow_SourceType_MITM,
		"http://example.test/inline",
		"127.0.0.1:80",
	)
	require.NoError(t, err)
	require.NoError(t, db.Create(flow).Error)

	query := projectedHTTPFlowQuery(int64(flow.ID))
	query.ExcludeRequestRaw = true
	query.ExcludeResponseRaw = true
	_, rows, err := QueryHTTPFlow(db, query)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.NotEmpty(t, rows[0].Request, "small request must stay inline under the 300K budget")
	require.NotEmpty(t, rows[0].Response, "small response must stay inline under the 300K budget")

	projected, err := model.ToHTTPFlowGRPCModelWithListProjection(rows[0], false, true, true)
	require.NoError(t, err)
	require.NotEmpty(t, projected.GetRequest())
	require.NotEmpty(t, projected.GetResponse())
	require.Equal(t, "inline title", projected.GetHtmlTitle())

	consts.SetHTTPFlowListInlineMaxContentLength(8)
	_, oversizeRows, err := QueryHTTPFlow(db, query)
	require.NoError(t, err)
	require.Len(t, oversizeRows, 1)
	require.Empty(t, oversizeRows[0].Request, "packets above the inline budget must stay empty")
	require.Empty(t, oversizeRows[0].Response, "packets above the inline budget must stay empty")
}

func TestHTTPFlowListProjectionInlinesSmallPacketsWithoutExcludeFlags(t *testing.T) {
	prev := consts.GetHTTPFlowListInlineMaxContentLength()
	consts.SetHTTPFlowListInlineMaxContentLength(consts.DefaultHTTPFlowListInlineMaxContentLength)
	t.Cleanup(func() { consts.SetHTTPFlowListInlineMaxContentLength(prev) })

	db, err := gorm.Open("sqlite3", filepath.Join(t.TempDir(), "project.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, db.AutoMigrate(&schema.HTTPFlow{}).Error)

	request := []byte("GET /frontend HTTP/1.1\r\nHost: example.test\r\n\r\n")
	response := []byte("HTTP/1.1 200 OK\r\nContent-Type: text/html\r\n\r\n<html><title>no exclude</title></html>")
	flow, err := CreateHTTPFlowFromHTTPWithBodySavedFromRaw(
		false,
		request,
		response,
		schema.HTTPFlow_SourceType_MITM,
		"http://example.test/frontend",
		"127.0.0.1:80",
	)
	require.NoError(t, err)
	require.NoError(t, db.Create(flow).Error)

	query := projectedHTTPFlowQuery(int64(flow.ID))
	query.ExcludeRequestRaw = false
	query.ExcludeResponseRaw = false
	_, rows, err := QueryHTTPFlow(db, query)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.NotEmpty(t, rows[0].Request, "current Yakit MITM list no longer sends Exclude*; small packets must still inline")
	require.NotEmpty(t, rows[0].Response, "current Yakit MITM list no longer sends Exclude*; small packets must still inline")

	projected, err := model.ToHTTPFlowGRPCModel(rows[0], false)
	require.NoError(t, err)
	require.NotEmpty(t, projected.GetRequest())
	require.NotEmpty(t, projected.GetResponse())

	consts.SetHTTPFlowListInlineMaxContentLength(8)
	_, oversizeRows, err := QueryHTTPFlow(db, query)
	require.NoError(t, err)
	require.Len(t, oversizeRows, 1)
	require.Empty(t, oversizeRows[0].Request, "over-budget packets must stay empty even without Exclude*")
	require.Empty(t, oversizeRows[0].Response, "over-budget packets must stay empty even without Exclude*")
}

func projectedHTTPFlowQuery(id int64) *ypb.QueryHTTPFlowRequest {
	return &ypb.QueryHTTPFlowRequest{
		IncludeId:          []int64{id},
		Full:               false,
		ExcludeResponseRaw: true,
		Pagination: &ypb.Paging{
			Page:    1,
			Limit:   10,
			OrderBy: "id",
			Order:   "desc",
		},
	}
}
