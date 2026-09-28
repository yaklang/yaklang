package aicommon

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

func TestToolCallerHTTPAttemptObserverUsesConcurrentInvocationIdentity(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var mu sync.Mutex
	var attempts []lowhttp.HTTPAttempt
	ctx = lowhttp.WithHTTPAttemptObserver(ctx, func(start lowhttp.HTTPAttempt) func(lowhttp.HTTPAttempt) {
		return func(end lowhttp.HTTPAttempt) {
			mu.Lock()
			attempts = append(attempts, end)
			mu.Unlock()
		}
	})
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
	cfg := NewTestConfig(ctx, WithAgreeYOLO(), WithWorkdir(t.TempDir()))
	results := make(chan error, 2)
	for _, id := range []string{"traffic-tool-a", "traffic-tool-b"} {
		caller, err := NewToolCaller(ctx,
			WithToolCaller_AICallerConfig(cfg), WithToolCaller_AICaller(cfg),
			WithToolCaller_Task(cfg.DefaultTask), WithToolCaller_Emitter(cfg.Emitter),
			WithToolCaller_CallToolID(id), WithToolCaller_RuntimeId("shared-react-session"))
		require.NoError(t, err)
		params := aitool.InvokeParams{"marker": id}
		if id == "traffic-tool-b" {
			params["runtime_id"] = "caller-supplied-shared-session"
		}
		go func() {
			result, _, callErr := caller.CallToolWithExistedParams(tool, true, params)
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
	mu.Lock()
	defer mu.Unlock()
	require.Len(t, attempts, 2)
	seen := make(map[string]bool)
	for _, attempt := range attempts {
		require.Equal(t, attempt.ToolCallID, attempt.RuntimeID)
		require.Contains(t, string(attempt.Request), "GET /"+attempt.ToolCallID+" HTTP/1.1")
		require.Equal(t, cfg.GetRuntimeId(), attempt.AgentID)
		require.NoError(t, attempt.Error)
		seen[attempt.ToolCallID] = true
	}
	require.Equal(t, map[string]bool{"traffic-tool-a": true, "traffic-tool-b": true}, seen)
}
