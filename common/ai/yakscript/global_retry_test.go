package yakscript

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/consts"
)

func setGlobalRetry(t *testing.T, enabled bool) {
	old := consts.GetTieredAIConfig()
	consts.SetTieredAIConfig(&consts.TieredAIConfig{RetryForever: enabled})
	t.Cleanup(func() { consts.SetTieredAIConfig(old) })
}

func TestGlobalRetryYak(t *testing.T) {
	setGlobalRetry(t, true)
	calls := 0
	withTestExecutor(t, func(context.Context, string, *Request, func(string) error) (any, error) {
		calls++
		if calls == 1 {
			return nil, &aispec.ModelCallFailure{Kind: "protocol", Cause: errors.New("incomplete upstream response")}
		}
		return "ready", nil
	})
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	c := testScriptClient(t, t.Name(), aispec.WithContext(ctx))
	result, err := aispec.ChatWithGlobalRetry(c, "fixture")
	require.NoError(t, err)
	require.Equal(t, "ready", result)
	require.Equal(t, 2, calls)
}

func TestGlobalRetryYakDiscardsPartialEmit(t *testing.T) {
	setGlobalRetry(t, true)
	calls := 0
	var output strings.Builder
	withTestExecutor(t, func(_ context.Context, _ string, _ *Request, emit func(string) error) (any, error) {
		calls++
		if calls < 2 {
			emit("discard")
			return nil, &aispec.ModelCallFailure{Kind: "protocol", Cause: errors.New("truncated")}
		}
		emit("ready")
		return nil, nil
	})
	c := testScriptClient(t, t.Name(), aispec.WithStreamHandler(func(r io.Reader) { io.Copy(&output, r) }))
	result, err := aispec.ChatWithGlobalRetry(c, "fixture")
	require.NoError(t, err)
	require.Equal(t, "ready", result)
	require.Equal(t, "ready", output.String())
	require.Equal(t, 2, calls)
}

func TestGlobalRetryOffStopsAfterOneAttempt(t *testing.T) {
	setGlobalRetry(t, false)
	calls := 0
	withTestExecutor(t, func(_ context.Context, _ string, req *Request, _ func(string) error) (any, error) {
		calls++
		return nil, ModelError(req, "network", 0, "", "temporary", "")
	})
	c := testScriptClient(t, t.Name())
	_, err := aispec.ChatWithGlobalRetry(c, "fixture")
	require.Error(t, err)
	require.Equal(t, 1, calls)
}

func testScriptClient(t *testing.T, name string, options ...aispec.AIConfigOption) *GatewayClient {
	t.Helper()
	c := &GatewayClient{}
	c.LoadOption(append([]aispec.AIConfigOption{aispec.WithType(ProviderType), aispec.WithScript("fixture"), aispec.WithTimeout(.05)}, options...)...)
	return c
}
func withTestExecutor(t *testing.T, run Executor) {
	previous := executor.Load()
	RegisterExecutor(run)
	t.Cleanup(func() { executor.Store(previous) })
}
