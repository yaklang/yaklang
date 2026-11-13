package yakit

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestSaveLowHTTPFlow_AIFuzzPluginSource(t *testing.T) {
	db := consts.GetGormProjectDatabase()
	runtimeID := uuid.NewString()
	t.Cleanup(func() {
		db.Where("runtime_id = ?", runtimeID).Unscoped().Delete(&schema.HTTPFlow{})
	})

	var savedFlow *schema.HTTPFlow
	SaveLowHTTPFlow(&lowhttp.LowhttpResponse{
		RawRequest: []byte(fmt.Sprintf("GET /%s HTTP/1.1\r\nHost: example.com\r\n\r\n", runtimeID)),
		RawPacket:  []byte("HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nok"),
		TraceInfo:  &lowhttp.LowhttpTraceInfo{},
		Url:        "http://example.com/" + runtimeID,
		RemoteAddr: "127.0.0.1:80",
		Source:     "reactloop_http_fuzztest",
		RuntimeId:  runtimeID,
		AfterSaveHTTPFlowHandler: []func(*schema.HTTPFlow){
			func(flow *schema.HTTPFlow) { savedFlow = flow },
		},
	}, true)

	require.NotNil(t, savedFlow)
	require.Equal(t, schema.HTTPFlow_SourceType_SCAN, savedFlow.SourceType)
	require.Equal(t, runtimeID, savedFlow.RuntimeId)

	paging, flows, err := QueryHTTPFlow(db, &ypb.QueryHTTPFlowRequest{
		SourceType: schema.HTTPFlow_SourceType_SCAN,
		RuntimeId:  runtimeID,
		Pagination: &ypb.Paging{Page: 1, Limit: 10},
	})
	require.NoError(t, err)
	require.Equal(t, 1, paging.TotalRecord)
	require.Len(t, flows, 1)
	require.Equal(t, savedFlow.ID, flows[0].ID)
}

func TestQueryHTTPFlow_PluginSourceIncludesLegacyAIFuzz(t *testing.T) {
	db, err := gorm.Open("sqlite3", filepath.Join(t.TempDir(), "project.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	require.NoError(t, db.AutoMigrate(&schema.HTTPFlow{}).Error)

	for _, fixture := range []struct {
		path      string
		source    string
		runtimeID string
	}{
		{"/plugin", "scan", "runtime-a"},
		{"/legacy-ai", "reactloop_http_fuzztest", "runtime-a"},
		{"/other-task-ai", "reactloop_http_fuzztest", "runtime-b"},
		{"/mitm", "mitm", "runtime-a"},
		{"/crawler", "basic-crawler", "runtime-a"},
		{"/browser", "ai-browser-http", "runtime-a"},
		{"/custom", "custom-producer", "runtime-a"},
	} {
		require.NoError(t, db.Create(&schema.HTTPFlow{
			Url:        "http://example.test" + fixture.path,
			Path:       fixture.path,
			Method:     "GET",
			SourceType: fixture.source,
			RuntimeId:  fixture.runtimeID,
		}).Error)
	}

	for _, tc := range []struct {
		name       string
		source     string
		runtimeID  string
		runtimeIDs []string
		paths      []string
	}{
		{name: "history", source: "scan", paths: []string{"/plugin", "/legacy-ai", "/other-task-ai"}},
		{name: "ai details", source: "scan", runtimeID: "runtime-a", paths: []string{"/plugin", "/legacy-ai"}},
		{name: "multiple ai tasks", source: "scan", runtimeIDs: []string{"runtime-a", "runtime-b"}, paths: []string{"/plugin", "/legacy-ai", "/other-task-ai"}},
		{name: "multiple sources", source: "mitm,scan", paths: []string{"/mitm", "/plugin", "/legacy-ai", "/other-task-ai"}},
		{name: "explicit legacy source", source: "reactloop_http_fuzztest", paths: []string{"/legacy-ai", "/other-task-ai"}},
		{name: "mitm", source: "mitm", paths: []string{"/mitm"}},
		{name: "crawler", source: "basic-crawler", paths: []string{"/crawler"}},
		{name: "named browser", source: "ai-browser-http", paths: []string{"/browser"}},
		{name: "custom producer", source: "custom-producer", paths: []string{"/custom"}},
		{name: "all sources", paths: []string{"/plugin", "/legacy-ai", "/other-task-ai", "/mitm", "/crawler", "/browser", "/custom"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			query := &ypb.QueryHTTPFlowRequest{
				SourceType: tc.source,
				RuntimeId:  tc.runtimeID,
				RuntimeIDs: tc.runtimeIDs,
				Pagination: &ypb.Paging{Page: 1, Limit: 10},
			}
			paging, flows, err := QueryHTTPFlow(db, query)
			require.NoError(t, err)
			require.Equal(t, len(tc.paths), paging.TotalRecord)
			paths := make([]string, 0, len(flows))
			for _, flow := range flows {
				paths = append(paths, flow.Path)
			}
			require.ElementsMatch(t, tc.paths, paths)
			require.Equal(t, tc.source, query.SourceType)
		})
	}
}
