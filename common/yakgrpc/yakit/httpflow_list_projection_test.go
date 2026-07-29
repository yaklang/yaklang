package yakit

import (
	"path/filepath"
	"testing"

	"github.com/yaklang/gorm"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/model"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestHTTPFlowListProjectionUsesPersistedTitleWithoutResponse(t *testing.T) {
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
