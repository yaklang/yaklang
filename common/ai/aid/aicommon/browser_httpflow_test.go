package aicommon

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils/lowhttp/httpctx"
)

func TestAttachedBrowserHTTPRequestRouting(t *testing.T) {
	task := NewStatefulTaskBase("http-test", "compare A and B", context.Background(), nil, true)
	task.SetAttachedDatas([]*AttachedResource{
		NewAttachedResource(AttachedResourceTypeBrowser, AttachedResourceKeyBrowserDevice, `{"deviceId":"device-a","reference":"A"}`),
		NewAttachedResource(AttachedResourceTypeBrowser, AttachedResourceKeyBrowserDevice, `{"deviceId":"device-b","reference":"B"}`),
	})
	params, err := BindAttachedBrowserToolParams(task, "do_http_request", aitool.InvokeParams{
		"browser_ref": "@B", "device_id": "spoofed", "url": "http://example.test/user?id=1",
	})
	require.NoError(t, err)
	require.Equal(t, "device-b", params.GetString("device_id"))
	require.Equal(t, "@B", params.GetString("browser_ref"), "keep the selector when reviewing/editing ordinary HTTP parameters")

	_, err = BindAttachedBrowserToolParams(task, "do_http_request", aitool.InvokeParams{"device_id": "spoofed"})
	require.ErrorContains(t, err, "browser_ref is required")
	_, err = BindAttachedBrowserToolParams(task, "do_http_request", aitool.InvokeParams{"browser_ref": "C"})
	require.ErrorContains(t, err, "does not match an attached browser")

	params, err = BindAttachedBrowserToolParams(task, "do_http_request", aitool.InvokeParams{"browser_ref": "none", "device_id": "spoofed"})
	require.NoError(t, err)
	require.Empty(t, params)

	task.SetAttachedDatas(nil)
	params, err = BindAttachedBrowserToolParams(task, "do_http_request", aitool.InvokeParams{"browser_ref": "B", "device_id": "spoofed"})
	require.NoError(t, err)
	require.NotContains(t, params, "device_id")
	require.Equal(t, "B", params.GetString("browser_ref"))

	task.SetAttachedDatas([]*AttachedResource{
		NewAttachedResource(AttachedResourceTypeBrowser, AttachedResourceKeyBrowserDevice, `{"deviceId":"device-a","reference":"A"}`),
	})
	_, err = BindAttachedBrowserToolParams(task, "do_http_request", aitool.InvokeParams{"browser_ref": "B"})
	require.ErrorContains(t, err, "does not match an attached browser")
	params, err = BindAttachedBrowserToolParams(task, "do_http_request", aitool.InvokeParams{})
	require.NoError(t, err)
	require.Equal(t, "device-a", params.GetString("device_id"))
}

func TestToolCallerBindsBrowserHTTPFlowIdentity(t *testing.T) {
	ctx := context.Background()
	resolver := func(deviceID, browserRef string) (httpctx.BrowserInstance, error) {
		badge := map[string]string{"device-a": "A", "device-b": "B"}[deviceID]
		require.NotEmpty(t, badge)
		require.Equal(t, badge, strings.TrimPrefix(browserRef, "@"))
		return httpctx.BrowserInstance{Manager: "ytray", InstanceID: deviceID, Badge: badge}, nil
	}
	cfg := NewTestConfig(ctx, WithWorkdir(t.TempDir()), WithBrowserHTTPFlowIdentityResolver(resolver))
	for _, badge := range []string{"A", "B"} {
		t.Run(badge, func(t *testing.T) {
			invoked := false
			tool, err := aitool.New("do_http_request",
				aitool.WithDangerousNoNeedUserReview(true),
				aitool.WithNoRuntimeCallback(func(ctx context.Context, _ aitool.InvokeParams, _, _ io.Writer) (any, error) {
					req, err := http.NewRequestWithContext(ctx, "GET", "http://example.test/same-url", nil)
					require.NoError(t, err)
					identity, ok := httpctx.GetBrowserInstance(req)
					require.True(t, ok)
					require.Equal(t, badge, identity.Badge)
					invoked = true
					return "ok", nil
				}),
			)
			require.NoError(t, err)
			task := NewStatefulTaskBase("http-"+badge, "compare A and B", ctx, cfg.Emitter, true)
			task.SetAttachedDatas([]*AttachedResource{
				NewAttachedResource(AttachedResourceTypeBrowser, AttachedResourceKeyBrowserDevice, `{"deviceId":"device-a","reference":"A"}`),
				NewAttachedResource(AttachedResourceTypeBrowser, AttachedResourceKeyBrowserDevice, `{"deviceId":"device-b","reference":"B"}`),
			})
			caller, err := NewToolCaller(ctx, WithToolCaller_AICallerConfig(cfg), WithToolCaller_Task(task),
				WithToolCaller_Emitter(cfg.Emitter), WithToolCaller_RuntimeId("http-"+badge))
			require.NoError(t, err)
			result, _, err := caller.CallToolWithExistedParams(tool, aitool.InvokeParams{"browser_ref": badge})
			require.NoError(t, err)
			require.True(t, result.Success)
			require.True(t, invoked)
		})
	}
	_, ok := httpctx.GetBrowserInstance((&http.Request{}).WithContext(ctx))
	require.False(t, ok, "one invocation must not change its parent's identity")

	child := NewConfig(ctx, ConvertConfigToOptions(cfg)...)
	require.NotNil(t, child.GetBrowserHTTPFlowIdentityResolver(), "plan/forge child configs must retain Bridge routing")
}

func TestBrowserHTTPFlowContextRequiresBridgeForSelection(t *testing.T) {
	cfg := NewTestConfig(context.Background(), WithWorkdir(t.TempDir()))
	_, err := bindBrowserHTTPFlowContext(context.Background(), cfg, "do_http_request", aitool.InvokeParams{"browser_ref": "A"})
	require.ErrorContains(t, err, "authenticated browser Bridge")
	ctx, err := bindBrowserHTTPFlowContext(context.Background(), cfg, "do_http_request", nil)
	require.NoError(t, err)
	require.NotNil(t, ctx, "ordinary HTTP requests remain available without browsers")
}
