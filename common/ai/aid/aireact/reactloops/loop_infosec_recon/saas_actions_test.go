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

func TestBoundedSaaSReconActions_RegisterProbePublishAndExit(t *testing.T) {
	var requestCount atomic.Int32
	var requestMethod atomic.Value
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		requestMethod.Store(r.Method)
		w.WriteHeader(http.StatusNoContent)
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

	probeOperator := executeReconAction(t, loop, "probe_api_candidates", map[string]any{
		"limit":           99,
		"concurrency":     99,
		"use_head":        false,
		"timeout_seconds": 99,
	})
	terminated, err = probeOperator.IsTerminated()
	require.NoError(t, err)
	require.True(t, terminated)
	require.Equal(t, int32(1), requestCount.Load())
	require.Equal(t, http.MethodHead, requestMethod.Load())
	require.Len(t, sink.assets, 1)
	require.Equal(t, server.URL+"/health", sink.assets[0].Target)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(sink.assets[0].Payload, &payload))
	require.Equal(t, http.MethodHead, payload["verification_method"])
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

func TestBoundedSaaSReconRejectsRepeatedRegisterAndProbe(t *testing.T) {
	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requestCount.Add(1)
		w.WriteHeader(http.StatusNoContent)
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

	firstProbe := executeReconAction(t, loop, "probe_api_candidates", map[string]any{})
	terminated, err := firstProbe.IsTerminated()
	require.NoError(t, err)
	require.True(t, terminated)

	probeHandler, err := loop.GetActionHandler("probe_api_candidates")
	require.NoError(t, err)
	err = probeHandler.ActionVerifier(loop, buildReconAction(t, "probe_api_candidates", map[string]any{}))
	require.ErrorContains(t, err, "already attempted")
	require.Equal(t, int32(1), requestCount.Load())
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
