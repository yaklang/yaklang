package aicommon

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

func TestToolCallerHTTPAttemptObserverPreservesRuntimeAndSessionContexts(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "observed")
	}))
	defer server.Close()
	host := strings.TrimPrefix(server.URL, "http://")
	started := make(chan struct{}, 2)
	release := make(chan struct{})
	tool, err := aitool.New("concurrent_traffic_tool",
		aitool.WithStringParam("marker", aitool.WithParam_Required(true)),
		aitool.WithDangerousNoNeedUserReview(true),
		aitool.WithNoRuntimeCallback(func(callCtx context.Context, params aitool.InvokeParams, _, _ io.Writer) (any, error) {
			started <- struct{}{}
			select {
			case <-release:
			case <-callCtx.Done():
				return nil, callCtx.Err()
			}
			packet := fmt.Sprintf("GET /%s HTTP/1.1\r\nHost: %s\r\nConnection: close\r\n\r\n", params.GetString("marker"), host)
			response, err := lowhttp.HTTP(lowhttp.WithPacketBytes([]byte(packet)), lowhttp.WithRuntimeId(params.GetString("runtime_id")), lowhttp.WithSaveHTTPFlow(false), lowhttp.WithContext(callCtx))
			if err != nil {
				return nil, err
			}
			return string(response.RawPacket), nil
		}),
	)
	require.NoError(t, err)
	results := make(chan error, 2)
	attempts := make([]chan lowhttp.HTTPAttempt, 2)
	for index, marker := range []string{"session-a", "session-b"} {
		observed := make(chan lowhttp.HTTPAttempt, 2)
		attempts[index] = observed
		callCtx := lowhttp.WithHTTPAttemptObserver(ctx, func(lowhttp.HTTPAttempt) func(lowhttp.HTTPAttempt) {
			return func(end lowhttp.HTTPAttempt) { observed <- end }
		})
		cfg := NewTestConfig(callCtx, WithAgreeYOLO(), WithWorkdir(t.TempDir()))
		caller, err := NewToolCaller(callCtx,
			WithToolCaller_AICallerConfig(cfg),
			WithToolCaller_Task(cfg.DefaultTask), WithToolCaller_Emitter(cfg.Emitter),
			WithToolCaller_CallToolID("call-"+marker), WithToolCaller_RuntimeId("shared-runtime"))
		require.NoError(t, err)
		params := aitool.InvokeParams{"marker": marker}
		if index == 1 {
			params["runtime_id"] = "caller-supplied-runtime"
		}
		go func() {
			result, _, callErr := caller.CallToolWithExistedParams(tool, params)
			if callErr == nil && (result == nil || !result.Success) {
				callErr = fmt.Errorf("tool callback did not succeed: %v", result)
			}
			results <- callErr
		}()
	}
	for i := 0; i < 2; i++ {
		select {
		case <-started:
		case <-ctx.Done():
			t.Fatal("both tool calls must overlap before issuing HTTP")
		}
	}
	close(release)
	for i := 0; i < 2; i++ {
		select {
		case err := <-results:
			require.NoError(t, err)
		case <-ctx.Done():
			t.Fatal("tool invocation did not finish")
		}
	}
	for index, marker := range []string{"session-a", "session-b"} {
		require.Len(t, attempts[index], 1)
		attempt := <-attempts[index]
		require.Contains(t, string(attempt.Request), "GET /"+marker+" HTTP/1.1")
		wantRuntime := "shared-runtime"
		if index == 1 {
			wantRuntime = "caller-supplied-runtime"
		}
		require.Equal(t, wantRuntime, attempt.RuntimeID)
		require.NoError(t, attempt.Error)
	}
}
