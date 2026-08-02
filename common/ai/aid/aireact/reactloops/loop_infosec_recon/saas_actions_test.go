package loop_infosec_recon

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	_ "github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loopinfra"
	"github.com/yaklang/yaklang/common/schema"
)

func TestBoundedSaaSReconActions_CrawlExtractProbePublishAndExit(t *testing.T) {
	var getCount atomic.Int32
	var headCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			headCount.Add(1)
			w.WriteHeader(http.StatusNoContent)
			return
		}
		getCount.Add(1)
		switch r.URL.Path {
		case "/health":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<html><body><a href="/api/ping">API</a><script src="/app.js"></script></body></html>`))
		case "/app.js":
			w.Header().Set("Content-Type", "application/javascript")
			_, _ = w.Write([]byte(`fetch("/api/users"); const outside = "https://outside.invalid/api/ignored";`))
		default:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true}`))
		}
	}))
	defer server.Close()

	sink := &recordingReconAssetSink{targetURL: server.URL + "/health"}
	loop := newBoundedSaaSReconActionTestLoop(t, sink)

	registerOperator := executeReconAction(t, loop, "recon_register_seed", map[string]any{
		"seed_url":          server.URL + "/health",
		"scope_hosts":       "",
		"max_crawl_depth":   99,
		"probe_concurrency": 99,
	})
	require.True(t, registerOperator.IsContinued())
	terminated, err := registerOperator.IsTerminated()
	require.NoError(t, err)
	require.False(t, terminated)

	pool, err := LoadAPIPool(loop.Get(keyWorkDir))
	require.NoError(t, err)
	require.Len(t, pool.Entries, 1)
	require.Equal(t, server.URL+"/health", pool.Entries[0].NormalizedURL)

	crawlOperator := executeReconAction(t, loop, ToolCrawlJsCollector, map[string]any{
		"start_url": server.URL + "/health",
		"max_depth": 99,
		"urls_max":  999,
	})
	require.True(t, crawlOperator.IsContinued())
	require.Equal(t, "true", loop.Get(keySaaSCrawlCompleted))

	staticOperator := executeReconAction(t, loop, ToolJsStaticExtractAI, map[string]any{
		"paths": "/operator/local/path/must/be/ignored",
	})
	require.True(t, staticOperator.IsContinued())
	require.Equal(t, "true", loop.Get(keySaaSStaticCompleted))

	pool, err = LoadAPIPool(loop.Get(keyWorkDir))
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(pool.Entries), 3)
	require.True(t, containsReconPoolURL(pool, server.URL+"/api/ping"))
	require.True(t, containsReconPoolURL(pool, server.URL+"/api/users"))
	require.False(t, containsReconPoolURL(pool, "https://outside.invalid/api/ignored"))

	probeOperator := executeReconAction(t, loop, "probe_api_candidates", map[string]any{
		"limit":           99,
		"concurrency":     99,
		"use_head":        false,
		"timeout_seconds": 99,
	})
	terminated, err = probeOperator.IsTerminated()
	require.NoError(t, err)
	require.True(t, terminated)
	require.GreaterOrEqual(t, getCount.Load(), int32(3))
	require.GreaterOrEqual(t, headCount.Load(), int32(3))
	require.GreaterOrEqual(t, len(sink.assets), 3)
	asset := findReconAssetByTarget(t, sink.assets, server.URL+"/api/users")
	var payload map[string]any
	require.NoError(t, json.Unmarshal(asset.Payload, &payload))
	require.Equal(t, http.MethodHead, payload["verification_method"])
}

func TestInstallBoundedSaaSReconSeedUsesOnlyServerAuthorization(t *testing.T) {
	target := "https://business.example:8443/health?q=1"
	loop := newBoundedSaaSReconActionTestLoop(t, &recordingReconAssetSink{targetURL: target})
	workDir := loop.Get(keyWorkDir)
	require.NoError(t, ensurePoolFile(workDir))

	seed, err := installBoundedSaaSReconSeed(loop, workDir, target, "")
	require.NoError(t, err)
	require.Equal(t, target, seed)
	require.Equal(t, "business.example", loop.Get(keyScopeHosts))
	require.Equal(t, "true", loop.Get(keySaaSSeedRegistered))

	pool, err := LoadAPIPool(workDir)
	require.NoError(t, err)
	require.Equal(t, target, pool.SeedURL)
	require.Len(t, pool.Entries, 1)
	require.Equal(t, target, pool.Entries[0].NormalizedURL)

	_, err = installBoundedSaaSReconSeed(loop, workDir, "https://other.example/health", "")
	require.ErrorContains(t, err, "server-authorized target")
}

func TestBoundedSaaSReconRegisterVerifier_RejectsScopeWidening(t *testing.T) {
	loop := newBoundedSaaSReconActionTestLoop(t, &recordingReconAssetSink{
		targetURL: "https://business.example/health",
	})
	handler, err := loop.GetActionHandler("recon_register_seed")
	require.NoError(t, err)
	action := buildReconAction(t, "recon_register_seed", map[string]any{
		"seed_url":    "https://business.example/health",
		"scope_hosts": "business.example,other.example",
	})

	err = handler.ActionVerifier(loop, action)
	require.ErrorContains(t, err, "cannot widen")
}

func TestBoundedSaaSReconRegisterVerifier_LocksExactServerTarget(t *testing.T) {
	loop := newBoundedSaaSReconActionTestLoop(t, &recordingReconAssetSink{
		targetURL: "https://business.example:8443/health?q=1",
	})
	handler, err := loop.GetActionHandler("recon_register_seed")
	require.NoError(t, err)

	for _, proposed := range []string{
		"http://business.example:8443/health?q=1",
		"https://other.example:8443/health?q=1",
		"https://business.example/health?q=1",
		"https://business.example:8443/other?q=1",
		"https://business.example:8443/health?q=2",
	} {
		action := buildReconAction(t, "recon_register_seed", map[string]any{
			"seed_url": proposed,
		})
		err := handler.ActionVerifier(loop, action)
		require.ErrorContains(t, err, "server-authorized target")
	}
}

func TestBoundedSaaSReconRejectsRepeatedPipelineStages(t *testing.T) {
	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><body>ok</body></html>`))
	}))
	defer server.Close()

	target := server.URL + "/health"
	loop := newBoundedSaaSReconActionTestLoop(t, &recordingReconAssetSink{targetURL: target})
	firstRegister := executeReconAction(t, loop, "recon_register_seed", map[string]any{"seed_url": target})
	require.True(t, firstRegister.IsContinued())

	registerHandler, err := loop.GetActionHandler("recon_register_seed")
	require.NoError(t, err)
	err = registerHandler.ActionVerifier(loop, buildReconAction(t, "recon_register_seed", map[string]any{"seed_url": target}))
	require.ErrorContains(t, err, "already registered")

	firstCrawl := executeReconAction(t, loop, ToolCrawlJsCollector, map[string]any{})
	require.True(t, firstCrawl.IsContinued())
	crawlHandler, err := loop.GetActionHandler(ToolCrawlJsCollector)
	require.NoError(t, err)
	err = crawlHandler.ActionVerifier(loop, buildReconAction(t, ToolCrawlJsCollector, map[string]any{}))
	require.ErrorContains(t, err, "already attempted")

	firstStatic := executeReconAction(t, loop, ToolJsStaticExtractAI, map[string]any{})
	require.True(t, firstStatic.IsContinued())
	staticHandler, err := loop.GetActionHandler(ToolJsStaticExtractAI)
	require.NoError(t, err)
	err = staticHandler.ActionVerifier(loop, buildReconAction(t, ToolJsStaticExtractAI, map[string]any{}))
	require.ErrorContains(t, err, "already attempted")

	firstProbe := executeReconAction(t, loop, "probe_api_candidates", map[string]any{})
	terminated, err := firstProbe.IsTerminated()
	require.NoError(t, err)
	require.True(t, terminated)

	probeHandler, err := loop.GetActionHandler("probe_api_candidates")
	require.NoError(t, err)
	err = probeHandler.ActionVerifier(loop, buildReconAction(t, "probe_api_candidates", map[string]any{}))
	require.ErrorContains(t, err, "already attempted")
	require.Greater(t, requestCount.Load(), int32(1))
}

func containsReconPoolURL(pool *APIPool, target string) bool {
	for _, entry := range pool.Entries {
		if entry.NormalizedURL == target {
			return true
		}
	}
	return false
}

func findReconAssetByTarget(t *testing.T, assets []aicommon.AssetResult, target string) aicommon.AssetResult {
	t.Helper()
	for _, asset := range assets {
		if asset.Target == target {
			return asset
		}
	}
	t.Fatalf("asset %s not found", target)
	return aicommon.AssetResult{}
}

func newBoundedSaaSReconActionTestLoop(
	t *testing.T,
	sink aicommon.ResultSink,
) *reactloops.ReActLoop {
	t.Helper()
	ctx := context.Background()
	workdir := t.TempDir()
	config := aicommon.NewConfig(
		ctx,
		aicommon.WithResultSink(sink),
		aicommon.WithWorkdir(workdir),
		aicommon.WithDisablePerception(true),
	)
	invoker := mock.NewMockInvoker(ctx)
	invoker.SetConfig(config)
	loop, err := reactloops.CreateLoopByName(
		schema.AI_REACT_LOOP_NAME_INFOSEC_RECON,
		invoker,
	)
	require.NoError(t, err)
	loop.Set(keyWorkDir, workdir)
	return loop
}

func executeReconAction(
	t *testing.T,
	loop *reactloops.ReActLoop,
	actionName string,
	params map[string]any,
) *reactloops.LoopActionHandlerOperator {
	t.Helper()
	handler, err := loop.GetActionHandler(actionName)
	require.NoError(t, err)
	action := buildReconAction(t, actionName, params)
	if handler.ActionVerifier != nil {
		require.NoError(t, handler.ActionVerifier(loop, action))
	}
	task := aicommon.NewStatefulTaskBase(
		"bounded-saas-recon-test",
		"verify one declared business service URL",
		context.Background(),
		loop.GetEmitter(),
	)
	operator := reactloops.NewActionHandlerOperator(task)
	handler.ActionHandler(loop, action, operator)
	return operator
}

func buildReconAction(
	t *testing.T,
	actionName string,
	params map[string]any,
) *aicommon.Action {
	t.Helper()
	payload := map[string]any{"@action": actionName}
	for key, value := range params {
		payload[key] = value
	}
	raw, err := json.Marshal(payload)
	require.NoError(t, err)
	action, err := aicommon.ExtractAction(string(raw), actionName)
	require.NoError(t, err)
	return action
}
