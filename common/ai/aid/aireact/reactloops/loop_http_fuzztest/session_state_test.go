package loop_http_fuzztest

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

func mustQuoteUserInputHistory(t *testing.T, history []schema.AIAgentUserInputRecord) string {
	t.Helper()
	quoted, err := schema.QuoteUserInputHistory(history)
	require.NoError(t, err)
	return quoted
}

func newHTTPFuzzSessionStateTestRuntime(t *testing.T) (*aicommon.Config, string, string) {
	t.Helper()
	runtimeID := uuid.NewString()
	sessionID := uuid.NewString()
	cfg := aicommon.NewConfig(
		context.Background(),
		aicommon.WithID(runtimeID),
		aicommon.WithPersistentSessionId(sessionID),
		aicommon.WithDisableAutoSkills(true),
	)
	require.NoError(t, cfg.GetDB().AutoMigrate(&schema.AIAgentRuntime{}).Error)
	require.NoError(t, cfg.GetDB().Create(&schema.AIAgentRuntime{
		Uuid:              runtimeID,
		PersistentSession: sessionID,
		Name:              "http-fuzz-session-state-test",
	}).Error)
	t.Cleanup(func() {
		require.NoError(t, cfg.GetDB().Unscoped().Where("uuid = ?", runtimeID).Delete(&schema.AIAgentRuntime{}).Error)
	})
	return cfg, runtimeID, sessionID
}

func TestPersistHTTPFuzzSessionContextUsesTimeline(t *testing.T) {
	cfg, runtimeID, _ := newHTTPFuzzSessionStateTestRuntime(t)
	require.NoError(t, cfg.GetDB().Model(&schema.AIAgentRuntime{}).
		Where("uuid = ?", runtimeID).
		Update("quoted_user_input", mustQuoteUserInputHistory(t,
			[]schema.AIAgentUserInputRecord{{
				Round:     1,
				Timestamp: time.Now(),
				UserInput: "REAL_USER_INPUT",
			}},
		)).Error)
	cfg.SetUserInputHistory([]schema.AIAgentUserInputRecord{{
		Round:     1,
		Timestamp: time.Now(),
		UserInput: "REAL_USER_INPUT",
	}})

	invoker := newFuzzFinalizeTestInvoker(t)
	invoker.SetConfig(cfg)
	loop := newFuzzFinalizeTestLoop(t, invoker)
	loop.Set("original_request", "GET /items?id=1 HTTP/1.1\r\nHost: example.com\r\n\r\n")
	loop.Set("original_request_summary", "URL: http://example.com/items?id=1 BODY: [(0) bytes]")
	loop.Set("current_request_summary", "URL: http://example.com/items?id=1 BODY: [(0) bytes]")
	loop.Set("representative_request", "GET /items?id=1 HTTP/1.1\r\nHost: example.com\r\n\r\n")
	loop.Set("representative_response", "HTTP/1.1 200 OK\r\n\r\n"+string(make([]byte, 4096)))
	loop.Set("representative_httpflow_hidden_index", "flow-dedicated")
	loop.Set("diff_result_analysis", "analysis remains internal")
	markLoopHTTPFuzzLastAction(loop, "fuzz_get_params")

	persistLoopHTTPFuzzSessionContext(loop, "unit_test")

	history := cfg.GetUserInputHistory()
	require.Len(t, history, 1)
	require.Equal(t, "REAL_USER_INPUT", history[0].UserInput)
	persistedContext, ok := findFuzzFinalizeTimelineContent(invoker.timelineRecords, "http_fuzztest_session_context")
	require.True(t, ok)
	require.Contains(t, persistedContext, loopHTTPFuzzTimelineContextMarker)
	require.Contains(t, persistedContext, `"representative_hidden_index":"flow-dedicated"`)
	require.Contains(t, cfg.GetTimeline().Dump(), loopHTTPFuzzTimelineContextMarker)

	runtime, err := yakit.GetAgentRuntime(cfg.GetDB(), runtimeID)
	require.NoError(t, err)
	require.Len(t, runtime.GetUserInputHistory(), 1)
	require.Equal(t, "REAL_USER_INPUT", runtime.GetUserInputHistory()[0].UserInput)

	restoredInvoker := newFuzzFinalizeTestInvoker(t)
	restoredInvoker.SetConfig(cfg)
	restoredLoop := newFuzzFinalizeTestLoop(t, restoredInvoker)
	require.True(t, restoreLoopHTTPFuzzSessionContext(restoredLoop, restoredInvoker))
	require.Equal(t, "flow-dedicated", restoredLoop.Get("representative_httpflow_hidden_index"))
	require.Contains(t, restoredLoop.Get("original_request"), "GET /items?id=1 HTTP/1.1")
	require.Contains(t, restoredLoop.Get("original_request"), "Host: example.com")
}
