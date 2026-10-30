package test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp/httpctx"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

// Exercise the actual ordinary HTTP tool and SQLite persistence. A and B hit
// the same URLs concurrently, including batch expansion, redirects and retries.
func TestDoHTTPRequestBrowserIdentityPersistsToSQLite(t *testing.T) {
	db, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&schema.HTTPFlow{}).Error)
	previous := consts.CaptureProjectDatabaseBinding()
	consts.BindProjectDatabase(db, "")
	wasSync := consts.GLOBAL_DB_SAVE_SYNC.IsSet()
	consts.GLOBAL_DB_SAVE_SYNC.Set()
	yakit.RegisterLowHTTPSaveCallback()
	t.Cleanup(func() {
		consts.BindProjectDatabaseWithReader(previous.Database, previous.ReadDatabase, previous.Path)
		if !wasSync {
			consts.GLOBAL_DB_SAVE_SYNC.UnSet()
		}
		_ = db.Close()
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/retry" && r.Header.Get("Content-Type") == "application/json":
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"username is required"}`))
		case r.URL.Path == "/redirect/1" || r.URL.Path == "/redirect/2":
			http.Redirect(w, r, "/done", http.StatusFound)
		default:
			_, _ = w.Write([]byte("ok"))
		}
	}))
	t.Cleanup(server.Close)
	tool := getDoHTTPRequestTool(t)
	_, hasBrowserRef := tool.InputSchema.Properties.Get("browser_ref")
	require.True(t, hasBrowserRef)
	valid, validationErrors := tool.ValidateParams(aitool.InvokeParams{"url": server.URL, "device_id": "device-a"})
	require.True(t, valid, "bound infrastructure identity must pass tool validation: %v", validationErrors)
	var wg sync.WaitGroup
	for _, badge := range []string{"A", "B"} {
		wg.Add(1)
		go func(badge string) {
			defer wg.Done()
			ctx := httpctx.WithBrowserInstance(context.Background(), httpctx.BrowserInstance{
				Manager: "ytray", InstanceID: "instance-" + badge, Badge: badge,
			})
			for _, params := range []aitool.InvokeParams{
				{"url": server.URL + "/redirect/{{int(1-2)}}", "fuzztag": true, "concurrent": 2},
				{"url": server.URL + "/retry", "method": "POST", "content-type": "application/json", "body": `{"username":"fixture"}`},
			} {
				params["headers"] = map[string]any{"Cookie": "fixture=" + badge}
				result, err := tool.InvokeWithParams(params, aitool.WithContext(ctx),
					aitool.WithRuntimeConfig(&aitool.ToolRuntimeConfig{RuntimeID: "browser-fixture-" + badge}))
				require.NoError(t, err)
				cleanupHTTPArtifacts(t, utils.InterfaceToGeneralMap(result.Data.(*aitool.ToolExecutionResult).Result))
			}
		}(badge)
	}
	wg.Wait()
	for badge, color := range map[string]string{"A": schema.FLOW_COLOR_ORANGE, "B": schema.FLOW_COLOR_BLUE} {
		var flows []schema.HTTPFlow
		require.NoError(t, db.Where("runtime_id = ?", "browser-fixture-"+badge).Find(&flows).Error)
		require.Len(t, flows, 6, "two redirect hops per batch item plus the form retry")
		for _, flow := range flows {
			require.Contains(t, flow.Tags, fmt.Sprintf("[浏览器 %s]", badge))
			require.Contains(t, flow.Tags, color)
			require.Equal(t, "scan", flow.SourceType)
			if !strings.HasSuffix(flow.Url, "/done") {
				require.Contains(t, flow.GetRequest(), "fixture="+badge)
			}
		}
	}
	result, err := tool.InvokeWithParams(aitool.InvokeParams{
		"url": server.URL + "/done", "headers": map[string]any{"X-Browser-Instance": "A"},
	}, aitool.WithRuntimeConfig(&aitool.ToolRuntimeConfig{RuntimeID: "ordinary-fixture"}))
	require.NoError(t, err)
	cleanupHTTPArtifacts(t, utils.InterfaceToGeneralMap(result.Data.(*aitool.ToolExecutionResult).Result))
	var ordinary []schema.HTTPFlow
	require.NoError(t, db.Where("runtime_id = ?", "ordinary-fixture").Find(&ordinary).Error)
	require.Len(t, ordinary, 1)
	require.Empty(t, ordinary[0].Tags, "business HTTP headers must not supply a browser identity")
}
