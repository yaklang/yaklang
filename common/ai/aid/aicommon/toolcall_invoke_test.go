package aicommon

import (
	"context"
	"io"
	"path/filepath"
	"testing"
	"time"

	"github.com/jinzhu/gorm"
	"github.com/segmentio/ksuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yak/yaklib"
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
	t.Cleanup(func() {
		require.NoError(t, consts.SetGormProjectDatabase(originProjectDBPath))
	})

	db := consts.GetGormProjectDatabase()
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
		WithToolCaller_AICaller(&ProxyAICaller{callFunc: func(request *AIRequest) (*AIResponse, error) {
			return &AIResponse{}, nil
		}}),
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
