package aicommon

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"testing"
	"time"

	"github.com/segmentio/ksuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/ai/aid/aiddb"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yak/yaklib"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

func TestHandleHTTPFlowMessage(t *testing.T) {
	exec := yaklib.NewYakitLogExecResult("json-httpflow", `{"runtime_id":"runtime-123","hidden_index":"flow-uuid-123","url":"http://example.com/full-data-should-be-ignored"}`)

	flow, err := handleHTTPFlowMessage(exec)
	require.NoError(t, err)
	require.NotNil(t, flow)
	require.Equal(t, "runtime-123", flow.RuntimeId)
	require.Equal(t, "flow-uuid-123", flow.HiddenIndex)
}

func TestHandleHTTPFlowMessage_IgnoreOtherLevels(t *testing.T) {
	exec := yaklib.NewYakitLogExecResult("json-risk", `{"runtime_id":"runtime-123","hidden_index":"flow-uuid-123"}`)

	flow, err := handleHTTPFlowMessage(exec)
	require.Error(t, err)
	require.Nil(t, flow)
}

func TestHandleFileWriteMessage(t *testing.T) {
	exec := yaklib.NewYakitLogExecResult("file", `{"action":"WRITE","path":"/tmp/demo.txt","action_message":{"mode":"modify"}}`)
	path, ok := handleFileWriteMessage(exec)
	require.True(t, ok)
	require.Equal(t, "/tmp/demo.txt", path)
}

func TestHandleFileWriteMessage_IgnoreNonWrite(t *testing.T) {
	exec := yaklib.NewYakitLogExecResult("file", `{"action":"READ","path":"/tmp/demo.txt"}`)
	path, ok := handleFileWriteMessage(exec)
	require.False(t, ok)
	require.Empty(t, path)

	statusExec := yaklib.NewYakitLogExecResult("file", `{"action":"STATUS","path":"/tmp/demo.txt"}`)
	path, ok = handleFileWriteMessage(statusExec)
	require.False(t, ok)
	require.Empty(t, path)
}

func TestShouldIgnoreExecResultForEmit_FileStatusOnly(t *testing.T) {
	statusExec := yaklib.NewYakitLogExecResult("file", `{"action":"STATUS","path":"/tmp/demo.txt"}`)
	require.True(t, shouldIgnoreExecResultForEmit(statusExec))

	writeExec := yaklib.NewYakitLogExecResult("file", `{"action":"WRITE","path":"/tmp/demo.txt"}`)
	require.False(t, shouldIgnoreExecResultForEmit(writeExec))
}

func TestToolCallerInvoke_PreflightValidationErrorIsReported(t *testing.T) {
	setupToolCallInvokeTestProjectDB(t)
	callToolID := "toolcall-validation-" + ksuid.New().String()
	tc, _ := newToolCallerForCountTest(t, callToolID)

	callbackCalled := false
	tool, err := aitool.New(
		"mock-validation-error",
		aitool.WithStringParam("value", aitool.WithParam_Required(true)),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			callbackCalled = true
			return "unexpected", nil
		}),
	)
	require.NoError(t, err)

	var reportedError string
	stderrOutput := &toolOutputBuffer{}
	result, err := tc.invoke(
		tool,
		aitool.InvokeParams{"value": []any{"invalid"}},
		func(reason any) {},
		func(invokeErr any) { reportedError = fmt.Sprint(invokeErr) },
		&toolOutputBuffer{}, stderrOutput, &toolOutputBuffer{}, &toolOutputBuffer{},
	)

	require.Error(t, err)
	require.NotNil(t, result)
	require.False(t, callbackCalled, "schema validation must stop the callback")
	require.Contains(t, result.Error, "参数验证失败")
	require.Contains(t, result.Error, "修复建议")
	require.Contains(t, result.Error, "请求尚未发送")
	require.Contains(t, reportedError, "参数验证失败")
	require.Contains(t, reportedError, "修复建议")
	require.Contains(t, string(stderrOutput.Snapshot()), "参数验证失败")
	require.Contains(t, string(stderrOutput.Snapshot()), "请求尚未发送")
}

func TestToolCallerInvoke_BatchCheckpointIdentityMismatchIsRejected(t *testing.T) {
	setupToolCallInvokeTestProjectDB(t)
	callToolID := "toolcall-batch-checkpoint-" + ksuid.New().String()
	tc, _ := newToolCallerForCountTest(t, callToolID)
	tc.batchID = "batch-a"
	tc.batchIndex = 2

	invoked := 0
	tool, err := aitool.New(
		"mock-batch-checkpoint",
		aitool.WithStringParam("value", aitool.WithParam_Required(true)),
		aitool.WithSimpleCallback(func(_ aitool.InvokeParams, _ io.Writer, _ io.Writer) (any, error) {
			invoked++
			return "unexpected", nil
		}),
	)
	require.NoError(t, err)
	params := aitool.InvokeParams{"value": "expected"}
	baseRequest := func() map[string]any {
		return map[string]any{
			"batch_id":     "batch-a",
			"call_index":   2,
			"call_tool_id": callToolID,
			"tool_name":    tool.Name,
			"param":        aitool.InvokeParams{"value": "expected"},
		}
	}
	cases := []struct {
		name   string
		mutate func(map[string]any)
	}{
		{name: "batch", mutate: func(req map[string]any) { req["batch_id"] = "batch-b" }},
		{name: "index", mutate: func(req map[string]any) { req["call_index"] = 3 }},
		{name: "call id", mutate: func(req map[string]any) { req["call_tool_id"] = "other-call" }},
		{name: "tool", mutate: func(req map[string]any) { req["tool_name"] = "other-tool" }},
		{name: "params", mutate: func(req map[string]any) { req["param"] = aitool.InvokeParams{"value": "stored"} }},
	}
	for i, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			seq := int64(7300 + i)
			request := baseRequest()
			testCase.mutate(request)
			checkpoint := tc.config.CreateToolCallCheckpoint(seq)
			require.NoError(t, tc.config.SubmitCheckpointRequest(checkpoint, request))
			tc.checkpointSeq = seq

			result, invokeErr := tc.invoke(
				tool,
				cloneToolBatchTestParams(params),
				func(any) {},
				func(any) {},
				&toolOutputBuffer{}, &toolOutputBuffer{}, &toolOutputBuffer{}, &toolOutputBuffer{},
			)
			require.ErrorContains(t, invokeErr, "tool batch checkpoint identity mismatch")
			require.Nil(t, result)
		})
	}
	require.Zero(t, invoked)
}

func cloneToolBatchTestParams(params aitool.InvokeParams) aitool.InvokeParams {
	cloned := make(aitool.InvokeParams, len(params))
	for key, value := range params {
		cloned[key] = value
	}
	return cloned
}

func TestToolCallerInvoke_HTTPFlowCountRefresh(t *testing.T) {
	db := setupToolCallInvokeTestProjectDB(t)
	callToolID := "toolcall-httpflow-" + ksuid.New().String()
	tc, events := newToolCallerForCountTest(t, callToolID)

	tool, err := aitool.New(
		"mock-httpflow-count",
		aitool.WithCallback(func(ctx context.Context, params aitool.InvokeParams, runtimeConfig *aitool.ToolRuntimeConfig, stdout io.Writer, stderr io.Writer) (any, error) {
			return createCountTestRows(3, func() error {
				return db.Create(&schema.HTTPFlow{
					RuntimeId:   runtimeConfig.RuntimeID,
					HiddenIndex: "hidden-" + ksuid.New().String(),
					Url:         "http://example.com/" + ksuid.New().String(),
					Path:        "/",
					Method:      "GET",
					SourceType:  schema.HTTPFlow_SourceType_SCAN,
				}).Error
			})
		}),
	)
	require.NoError(t, err)

	_, err = tc.invoke(tool, aitool.InvokeParams{}, func(reason any) {}, func(err any) {}, &toolOutputBuffer{}, &toolOutputBuffer{}, &toolOutputBuffer{}, &toolOutputBuffer{})
	require.NoError(t, err)

	event := waitForYakitCountValue(t, events, schema.EVENT_TYPE_YAKIT_HTTPFLOW_COUNT, "$.http_flow_count", "3")
	require.Equal(t, callToolID, event.GetContentJSONPath("$.runtime_id"))
}

func TestToolCallerInvoke_RiskCountRefresh(t *testing.T) {
	db := setupToolCallInvokeTestProjectDB(t)
	callToolID := "toolcall-risk-" + ksuid.New().String()
	tc, events := newToolCallerForCountTest(t, callToolID)

	tool, err := aitool.New(
		"mock-risk-count",
		aitool.WithCallback(func(ctx context.Context, params aitool.InvokeParams, runtimeConfig *aitool.ToolRuntimeConfig, stdout io.Writer, stderr io.Writer) (any, error) {
			return createCountTestRows(3, func() error {
				return db.Create(&schema.Risk{
					RuntimeId: runtimeConfig.RuntimeID,
					Title:     "risk-" + ksuid.New().String(),
					RiskType:  "info",
					Severity:  "low",
					Url:       "http://example.com/" + ksuid.New().String(),
					IP:        "127.0.0.1",
				}).Error
			})
		}),
	)
	require.NoError(t, err)

	_, err = tc.invoke(tool, aitool.InvokeParams{}, func(reason any) {}, func(err any) {}, &toolOutputBuffer{}, &toolOutputBuffer{}, &toolOutputBuffer{}, &toolOutputBuffer{})
	require.NoError(t, err)

	event := waitForYakitCountValue(t, events, schema.EVENT_TYPE_YAKIT_RISK_COUNT, "$.risk_count", "3")
	require.Equal(t, callToolID, event.GetContentJSONPath("$.runtime_id"))
}

func setupToolCallInvokeTestProjectDB(t *testing.T) *gorm.DB {
	t.Helper()

	originProjectDBPath := consts.GetCurrentProjectDatabasePath()
	projectDBPath := filepath.Join(t.TempDir(), "toolcall-invoke-test.db")
	require.NoError(t, consts.SetGormProjectDatabase(projectDBPath))
	db := consts.GetGormProjectDatabase()
	t.Cleanup(func() {
		require.NoError(t, consts.SetGormProjectDatabase(originProjectDBPath))
		require.NoError(t, db.Close())
	})
	require.NoError(t, db.AutoMigrate(
		&schema.HTTPFlow{},
		&schema.Risk{},
		&schema.AiOutputEvent{},
		&schema.AiCheckpoint{},
	).Error)
	return db
}

func newToolCallerForCountTest(t *testing.T, callToolID string) (*ToolCaller, <-chan *schema.AiOutputEvent) {
	t.Helper()

	events := make(chan *schema.AiOutputEvent, 64)
	cfg := NewTestConfig(context.Background(), WithID("cfg-"+ksuid.New().String()), WithEventHandler(func(e *schema.AiOutputEvent) {
		events <- e
	}))

	tc, err := NewToolCaller(
		context.Background(),
		WithToolCaller_AICallerConfig(cfg),
		WithToolCaller_Emitter(cfg.Emitter),
		WithToolCaller_CallToolID(callToolID),
		WithToolCaller_RuntimeId(callToolID),
	)
	require.NoError(t, err)
	return tc, events
}

func createCountTestRows(total int, create func() error) (any, error) {
	for i := 0; i < total; i++ {
		if err := create(); err != nil {
			return nil, err
		}
	}
	return map[string]any{"created": total}, nil
}

func waitForYakitCountValue(t *testing.T, ch <-chan *schema.AiOutputEvent, eventType schema.EventType, valuePath, want string) *schema.AiOutputEvent {
	t.Helper()

	deadline := time.After(2 * time.Second)
	for {
		select {
		case event := <-ch:
			if event != nil && event.Type == eventType && event.GetContentJSONPath(valuePath) == want {
				return event
			}
		case <-deadline:
			t.Fatalf("timeout waiting for %s event with %s=%s", eventType, valuePath, want)
		}
	}
}

func TestResilienceSubForgeCheckpointReplay(t *testing.T) {
	setupToolCallInvokeTestProjectDB(t)
	tc, _ := newToolCallerForCountTest(t, "sub-forge-checkpoint-"+ksuid.New().String())
	invocations := 0
	tool, err := aitool.New("sub_forge_mock", aitool.WithStringParam("query"),
		aitool.WithSimpleCallback(func(_ aitool.InvokeParams, _, _ io.Writer) (any, error) {
			invocations++
			return "child result", nil
		}))
	require.NoError(t, err)
	tool.RejectInterruptedReplay = true
	params := aitool.InvokeParams{"query": "inventory"}
	run := func() (*aitool.ToolResult, error) {
		return tc.invoke(tool, cloneToolBatchTestParams(params), func(any) {}, func(any) {},
			&toolOutputBuffer{}, &toolOutputBuffer{}, &toolOutputBuffer{}, &toolOutputBuffer{})
	}
	tc.checkpointSeq = 8090
	cp := tc.config.CreateToolCallCheckpoint(tc.checkpointSeq)
	require.NoError(t, tc.config.SubmitCheckpointRequest(cp, map[string]any{"tool_name": tool.Name, "param": params, "callback_started": true}))
	result, err := run()
	require.ErrorContains(t, err, "outcome is unknown")
	require.Nil(t, result)
	require.Zero(t, invocations)

	tc.checkpointSeq++
	result, err = run()
	require.NoError(t, err)
	require.True(t, result.Success)
	replayed, err := run()
	require.NoError(t, err)
	require.True(t, replayed.Success)
	require.Equal(t, 1, invocations)
	params["query"] = "different request"
	_, err = run()
	require.ErrorContains(t, err, "identity mismatch")
	require.Equal(t, 1, invocations)
}

func TestResilienceSubForgeCheckpointAcrossWorkerAttempts(t *testing.T) {
	setupToolCallInvokeTestProjectDB(t)
	tc, _ := newToolCallerForCountTest(t, "sub-forge-attempt-"+ksuid.New().String())
	callbacks := 0
	tool, err := aitool.New("sub_forge_recover", aitool.WithStringParam("query"),
		aitool.WithSimpleCallback(func(_ aitool.InvokeParams, _, _ io.Writer) (any, error) {
			callbacks++
			return fmt.Sprintf("child result %d", callbacks), nil
		}))
	require.NoError(t, err)
	tool.RejectInterruptedReplay = true
	newTask := func(id string) *AIStatefulTaskBase {
		return NewStatefulTaskBase(id, "worker", tc.ctx, tc.emitter, true)
	}
	run := func(query string) (*aitool.ToolResult, error) {
		return tc.invoke(tool, aitool.InvokeParams{"query": query}, func(any) {}, func(any) {},
			&toolOutputBuffer{}, &toolOutputBuffer{}, &toolOutputBuffer{}, &toolOutputBuffer{})
	}

	first := newTask("persistent-task")
	tc.task = first
	result, err := run("completed")
	require.NoError(t, err)
	require.True(t, result.Success)
	require.Equal(t, 1, callbacks)
	result, err = run("completed")
	require.NoError(t, err)
	require.True(t, result.Success)
	require.Equal(t, 2, callbacks)

	cp := tc.config.CreateToolCallCheckpoint(9011)
	require.NoError(t, tc.config.SubmitCheckpointRequest(cp, map[string]any{
		"tool_name": tool.Name, "param": aitool.InvokeParams{"query": "interrupted"},
		"callback_started": true, "replay_task_id": first.GetId(), "replay_attempt_id": first.GetUUID(),
	}))
	cp = tc.config.CreateToolCallCheckpoint(9012)
	require.NoError(t, tc.config.SubmitCheckpointRequest(cp, map[string]any{
		"tool_name": tool.Name, "param": aitool.InvokeParams{"query": "completed"},
		"callback_started": true, "replay_task_id": first.GetId(), "replay_attempt_id": first.GetUUID(),
		"replay_ordinal": 3,
	}))
	next := newTask(first.GetId())
	tc.task = next
	require.NotEqual(t, first.GetUUID(), next.GetUUID())
	_, err = run("interrupted")
	require.ErrorContains(t, err, "unknown outcome")
	for ordinal := 1; ordinal <= 2; ordinal++ {
		replayed, err := run("completed")
		require.NoError(t, err)
		require.True(t, replayed.Success)
		require.Contains(t, replayed.String(), fmt.Sprintf("child result %d", ordinal))
	}
	require.Equal(t, 2, callbacks)
	_, err = run("completed")
	require.ErrorContains(t, err, "unknown outcome")
	require.Equal(t, 2, callbacks)
	require.NoError(t, tc.config.GetDB().Delete(cp).Error)
	result, err = run("completed")
	require.NoError(t, err)
	require.True(t, result.Success)
	require.Contains(t, result.String(), "child result 3")

	tc.task = newTask("different-task")
	result, err = run("completed")
	require.NoError(t, err)
	require.True(t, result.Success)
	require.Equal(t, 4, callbacks)
}

func TestResilienceSubForgeCheckpointRetriesBusyAdmission(t *testing.T) {
	setupToolCallInvokeTestProjectDB(t)
	tc, _ := newToolCallerForCountTest(t, "sub-forge-busy-"+ksuid.New().String())
	busy := true
	var cancelWhileBusy context.CancelFunc
	callbacks := 0
	tool, err := aitool.New("sub_forge_retry_busy", aitool.WithStringParam("query"),
		aitool.WithSimpleCallback(func(_ aitool.InvokeParams, _, _ io.Writer) (any, error) {
			if busy {
				if cancelWhileBusy != nil {
					cancelWhileBusy()
				}
				return nil, ErrSubForgeBusy
			}
			callbacks++
			return "child result", nil
		}))
	require.NoError(t, err)
	tool.RejectInterruptedReplay = true
	tool.RequireForeground = true
	run := func() (*aitool.ToolResult, error) {
		tc.task = NewStatefulTaskBase("persistent-task", "worker", tc.ctx, tc.emitter, true)
		return tc.invoke(tool, aitool.InvokeParams{"query": "same request"}, func(any) {}, func(any) {},
			&toolOutputBuffer{}, &toolOutputBuffer{}, &toolOutputBuffer{}, &toolOutputBuffer{})
	}

	_, err = run()
	require.ErrorIs(t, err, ErrSubForgeBusy)
	require.Zero(t, callbacks)

	originalCtx := tc.ctx
	tc.ctx, cancelWhileBusy = context.WithCancel(originalCtx)
	_, err = run()
	require.ErrorIs(t, err, context.Canceled)
	require.ErrorIs(t, err, ErrSubForgeBusy)
	require.Zero(t, callbacks)

	tc.ctx = originalCtx
	busy = false
	result, err := run()
	require.NoError(t, err)
	require.True(t, result.Success)
	require.Equal(t, 1, callbacks)
	result, err = run()
	require.NoError(t, err)
	require.Contains(t, result.String(), "child result")
	require.Equal(t, 1, callbacks)
}

// Cancel at the durable start marker, before InvokeWithParams can enter the
// callback. This exercises the cancellation window without timing or sleeps.
type cancelAtToolStartCheckpoint struct {
	AICallerConfigIf
	cancel context.CancelFunc
}

func (c *cancelAtToolStartCheckpoint) SubmitCheckpointRequest(checkpoint *schema.AiCheckpoint, request any) error {
	if err := c.AICallerConfigIf.SubmitCheckpointRequest(checkpoint, request); err != nil {
		return err
	}
	if params, ok := request.(map[string]any); ok && params["callback_started"] == true && c.cancel != nil {
		c.cancel()
		c.cancel = nil
	}
	return nil
}

func TestResilienceSubForgeCheckpointRetriesCancellationBeforeCallback(t *testing.T) {
	setupToolCallInvokeTestProjectDB(t)
	tc, _ := newToolCallerForCountTest(t, "sub-forge-pre-cancel-"+ksuid.New().String())
	callbacks := 0
	tool, err := aitool.New("sub_forge_pre_cancel", aitool.WithStringParam("query"),
		aitool.WithSimpleCallback(func(_ aitool.InvokeParams, _, _ io.Writer) (any, error) {
			callbacks++
			return "child result", nil
		}))
	require.NoError(t, err)
	tool.RejectInterruptedReplay = true
	tool.RequireForeground = true
	originalCtx := tc.ctx
	ctx, cancel := context.WithCancel(originalCtx)
	defer cancel()
	tc.ctx = ctx
	tc.config = &cancelAtToolStartCheckpoint{AICallerConfigIf: tc.config, cancel: cancel}
	tc.checkpointSeq = 9100
	run := func() (*aitool.ToolResult, error) {
		tc.task = NewStatefulTaskBase("persistent-task", "worker", tc.ctx, tc.emitter, true)
		return tc.invoke(tool, aitool.InvokeParams{"query": "same request"}, func(any) {}, func(any) {},
			&toolOutputBuffer{}, &toolOutputBuffer{}, &toolOutputBuffer{}, &toolOutputBuffer{})
	}

	_, err = run()
	require.ErrorIs(t, err, context.Canceled)
	require.Zero(t, callbacks, "cancellation must happen before callback entry")
	checkpoint, ok := yakit.GetToolCallCheckpoint(tc.config.GetDB(), tc.config.GetRuntimeId(), tc.checkpointSeq)
	require.True(t, ok)
	require.False(t, checkpoint.Finished)
	require.False(t, aiddb.AiCheckPointGetRequestParams(checkpoint).GetBool("callback_started"), "a known pre-start cancellation must remain retryable")
	require.ErrorIs(t, err, aitool.ErrInvocationNotStarted)

	// A new worker attempt and sequence must execute once, then replay the result.
	tc.ctx = originalCtx
	tc.checkpointSeq++
	result, err := run()
	require.NoError(t, err)
	require.True(t, result.Success)
	require.Equal(t, 1, callbacks)
	tc.checkpointSeq++
	result, err = run()
	require.NoError(t, err)
	require.True(t, result.Success)
	require.Contains(t, result.String(), "child result")
	require.Equal(t, 1, callbacks)
}
