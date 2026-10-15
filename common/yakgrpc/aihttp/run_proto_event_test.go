package aihttp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"google.golang.org/protobuf/encoding/protojson"
)

func newInternalTestGateway(t *testing.T, opts ...GatewayOption) *AIAgentHTTPGateway {
	t.Helper()
	defaultOpts := []GatewayOption{
		WithRoutePrefix("/agent"),
		WithHost("127.0.0.1"),
		WithPort(0),
	}
	defaultOpts = append(defaultOpts, opts...)
	gw, err := NewAIAgentHTTPGateway(defaultOpts...)
	require.NoError(t, err)
	t.Cleanup(func() {
		gw.Shutdown()
	})
	return gw
}

func TestMergeStartParamsDoesNotMigrateLegacyContextLimit(t *testing.T) {
	for _, rawSetting := range []string{
		`{"UseDefaultAIConfig":true,"TimelineContentSizeLimit":20}`,
		`{"use_default_ai_config":true,"timeline_content_size_limit":64}`,
	} {
		defaults, ok := decodeSettingPayloadRaw(rawSetting)
		require.True(t, ok)
		require.Positive(t, defaults.TimelineContentSizeLimit, "simulate persisted legacy settings")
		for _, test := range []struct {
			name     string
			request  *ypb.AIStartParams
			expected int64
		}{
			{name: "absent"},
			{name: "legacy_request", request: &ypb.AIStartParams{TimelineContentSizeLimit: 20 * 1024}},
			{name: "explicit_new_limit", request: &ypb.AIStartParams{
				TimelineContentSizeLimit: 20 * 1024, RuntimeContextSizeLimit: 128 * 1024,
			}, expected: 128 * 1024},
		} {
			t.Run(rawSetting+"/"+test.name, func(t *testing.T) {
				params := mergeStartParams(test.request, defaults, "no-limit-migration")
				require.Equal(t, test.expected, params.GetRuntimeContextSizeLimit(),
					"legacy defaults and request fields must never supply the new limit")
			})
		}
	}
}

func TestMergeStartParamsRuntimeContextSizeLimitPrecedence(t *testing.T) {
	for _, test := range []struct {
		name            string
		settingLimit    int64
		request         *ypb.AIStartParams
		disableDefaults bool
		expected        int64
	}{
		{name: "server_default"},
		{name: "setting_default", settingLimit: 128 * 1024, expected: 128 * 1024},
		{name: "request_override", settingLimit: 128 * 1024,
			request: &ypb.AIStartParams{RuntimeContextSizeLimit: 64 * 1024}, expected: 64 * 1024},
		{name: "request_without_setting",
			request: &ypb.AIStartParams{RuntimeContextSizeLimit: 96 * 1024}, expected: 96 * 1024},
		{name: "negative_request_keeps_server_default", settingLimit: 128 * 1024,
			request: &ypb.AIStartParams{RuntimeContextSizeLimit: -1}, expected: -1},
		{name: "defaults_disabled", settingLimit: 128 * 1024, disableDefaults: true},
		{name: "request_enables_defaults", settingLimit: 128 * 1024, disableDefaults: true,
			request: &ypb.AIStartParams{UseDefaultAIConfig: true}, expected: 128 * 1024},
	} {
		t.Run(test.name, func(t *testing.T) {
			defaults := defaultAIAgentChatSettingPayload()
			require.Zero(t, defaults.RuntimeContextSizeLimit)
			defaults.RuntimeContextSizeLimit = test.settingLimit
			defaults.UseDefaultAIConfig = !test.disableDefaults
			params := mergeStartParams(test.request, defaults, "runtime-context-limit")
			require.Equal(t, test.expected, params.GetRuntimeContextSizeLimit())
			if test.request != nil {
				require.NotSame(t, test.request, params, "merging settings must not modify the request")
			}
		})
	}
}

func TestRuntimeContextSettingPatchRoundTrip(t *testing.T) {
	for _, patch := range []string{
		`{"RuntimeContextSizeLimit":131072}`,
		`{"runtime_context_size_limit":131072}`,
	} {
		t.Run(patch, func(t *testing.T) {
			setting, err := mergeSettingPayloadPatch(defaultAIAgentChatSettingPayload(), []byte(patch))
			require.NoError(t, err)
			require.EqualValues(t, 128*1024, setting.RuntimeContextSizeLimit)
			raw, err := json.Marshal(applyAIAgentSettingDefaults(setting))
			require.NoError(t, err)
			restored, ok := decodeSettingPayloadRaw(string(raw))
			require.True(t, ok)
			require.EqualValues(t, 128*1024, restored.RuntimeContextSizeLimit)
			require.EqualValues(t, 128*1024, mergeStartParams(nil, restored, "saved-limit").GetRuntimeContextSizeLimit(),
				"the new setting is passed through directly, without unit conversion")

			legacyPatch, err := mergeSettingPayloadPatch(restored, []byte(`{"TimelineContentSizeLimit":99}`))
			require.NoError(t, err)
			require.EqualValues(t, 128*1024, legacyPatch.RuntimeContextSizeLimit,
				"changing the legacy field must not overwrite the new setting")
			cleared, err := mergeSettingPayloadPatch(legacyPatch, []byte(`{"RuntimeContextSizeLimit":0}`))
			require.NoError(t, err)
			cleared = applyAIAgentSettingDefaults(cleared)
			require.Zero(t, cleared.RuntimeContextSizeLimit)
			require.Zero(t, mergeStartParams(nil, cleared, "server-default").GetRuntimeContextSizeLimit(),
				"resetting to zero must leave the limit to the server, not the legacy setting")
		})
	}
}

func TestHandlePushEventRespondsWithAIOutputEvent(t *testing.T) {
	gw := newInternalTestGateway(t)
	runID := "run-push-accepted"
	gw.runManager.Create(runID, nil)

	body, err := protojson.MarshalOptions{UseProtoNames: true}.Marshal(&ypb.AIInputEvent{
		IsFreeInput: true,
		FreeInput:   "hello",
	})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/agent/run/"+runID+"/events/push", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	gw.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var resp ypb.AIOutputEvent
	require.NoError(t, protojson.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, "accepted", resp.GetType())
	require.True(t, resp.GetIsSystem())
	require.True(t, resp.GetIsResult())
	require.NotEmpty(t, resp.GetEventUUID())
	require.NotZero(t, resp.GetTimestamp())
}

func TestHandlePushEventRejectsLegacyPayload(t *testing.T) {
	gw := newInternalTestGateway(t)
	runID := "run-push-legacy"
	gw.runManager.Create(runID, nil)

	legacyBody := []byte(`{"type":"free_input","free_input":"hello"}`)
	req := httptest.NewRequest(http.MethodPost, "/agent/run/"+runID+"/events/push", bytes.NewReader(legacyBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	gw.ServeHTTP(w, req)
	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandleCancelRunRespondsWithAIOutputEvent(t *testing.T) {
	gw := newInternalTestGateway(t)
	runID := "run-cancelled"
	gw.runManager.Create(runID, nil)

	body, err := protojson.MarshalOptions{UseProtoNames: true}.Marshal(&ypb.AIInputEvent{})
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/agent/run/"+runID+"/cancel", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	gw.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var resp ypb.AIOutputEvent
	require.NoError(t, protojson.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, string(RunStatusCancelled), resp.GetType())
	require.True(t, resp.GetIsSystem())
	require.True(t, resp.GetIsResult())
	require.NotZero(t, resp.GetTimestamp())
	_, ok := gw.runManager.Get(runID)
	require.False(t, ok)
}

func TestEnsureReusableSessionRecreatesCancelledSession(t *testing.T) {
	gw := newInternalTestGateway(t)
	runID := "run-recreate-after-cancel"

	original := gw.runManager.Create(runID, nil)
	original.Cancel()

	recreated, created, err := gw.ensureReusableSession(runID)
	require.NoError(t, err)
	require.True(t, created)
	require.NotSame(t, original, recreated)
	require.NoError(t, recreated.ctx.Err())
}

func TestHandleSSEEventsRespondsWithAIOutputEvent(t *testing.T) {
	gw := newInternalTestGateway(t)
	runID := "run-sse-completed"
	session := gw.runManager.Create(runID, nil)
	session.Complete(nil)

	req := httptest.NewRequest(http.MethodGet, "/agent/run/"+runID+"/events", nil)
	req.Header.Set("Origin", "http://localhost:4173")
	w := httptest.NewRecorder()

	gw.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "http://localhost:4173", w.Header().Get("Access-Control-Allow-Origin"))
	require.Equal(t, "true", w.Header().Get("Access-Control-Allow-Credentials"))

	var payloads []string
	scanner := bufio.NewScanner(strings.NewReader(w.Body.String()))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") {
			payloads = append(payloads, strings.TrimPrefix(line, "data: "))
		}
	}
	require.NoError(t, scanner.Err())
	require.Len(t, payloads, 2)

	var ready ypb.AIOutputEvent
	require.NoError(t, protojson.Unmarshal([]byte(payloads[0]), &ready))
	require.Equal(t, "listener_ready", ready.GetType())
	require.True(t, ready.GetIsSystem())

	var done ypb.AIOutputEvent
	require.NoError(t, protojson.Unmarshal([]byte(payloads[1]), &done))
	require.Equal(t, string(RunStatusCompleted), done.GetType())
	require.True(t, done.GetIsResult())
}
