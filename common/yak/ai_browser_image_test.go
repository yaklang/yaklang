package yak

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-rod/rod/lib/launcher"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/yakscripttools"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/browser"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// Actual Chrome and the shipped Yak script: an operation changes the rendered
// pixels, and both captures reach the tool image channel with distinct hashes.
func TestLiveBrowserScreenshotImageResult(t *testing.T) {
	chrome, ok := launcher.LookPath()
	if !ok {
		t.Skip("Chrome is not installed")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	page := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `<!doctype html><html><body style="background:white"><h1 id="state">Before</h1><button id="save" onclick="document.body.style.background='lightgreen';document.getElementById('state').textContent='Saved'">Save</button></body></html>`)
	}))
	defer page.Close()
	source, err := yakscripttools.GetEmbedFS().ReadFile("yakscriptforai/browser/use_browser.yak")
	require.NoError(t, err)
	spec := yakscripttools.LoadYakScriptToAiTools("image-browser-fixture", string(source))
	require.NotNil(t, spec)
	tools := YakTool2AITool([]*schema.AIYakTool{spec})
	require.Len(t, tools, 1)
	session := "image-test-" + uuid.NewString()
	defer browser.CloseByID(browser.WithID(session))
	invoke := func(params map[string]any) *aitool.ToolResult {
		t.Helper()
		params["session"] = session
		result, err := tools[0].InvokeWithParams(params, aitool.WithContext(ctx))
		require.NoError(t, err)
		status, _ := result.GetExecutionStatus()
		require.NotEqual(t, aitool.ToolExecutionStatusFailed, status, result.String())
		return result
	}
	invoke(map[string]any{"op": "open", "url": page.URL, "headless": "yes", "exe-path": chrome, "timeout": 10})
	before := invoke(map[string]any{"op": "screenshot"})
	require.Len(t, before.Images, 1)
	invoke(map[string]any{"op": "click", "target": "#save"})
	after := invoke(map[string]any{"op": "screenshot"})
	require.Len(t, after.Images, 1)
	require.NotEqual(t, before.Images[0].SHA256, after.Images[0].SHA256)
	for _, result := range []*aitool.ToolResult{before, after} {
		raw, err := result.Images[0].Bytes(ctx)
		require.NoError(t, err)
		require.NotEmpty(t, raw)
	}
	if os.Getenv("YAK_IMAGE_REAL_SMOKE") == "1" {
		public := &ypb.AIModelConfig{Provider: yakit.DefaultAIBalanceProviderConfig(), ModelName: "memfit-vision-free"}
		cb, err := aicommon.LoadAIService("aibalance", append(aispec.BuildOptionsFromConfig(public), aispec.WithContext(ctx), aispec.WithTimeout(20), aispec.WithDisableProviderFallback(true), aispec.WithNetworkDebug(false))...)
		require.NoError(t, err)
		cfg := aicommon.NewConfig(ctx, aicommon.WithDisableCreateDBRuntime(true), aicommon.WithDisableAutoSkills(true), aicommon.WithAICallback(cb))
		raw, err := after.Images[0].Bytes(ctx)
		require.NoError(t, err)
		response, err := cb(cfg, aicommon.NewAIRequest("Read the large heading exactly as displayed in this browser screenshot. Reply only with the heading text.", aicommon.WithAIRequest_Context(ctx), aicommon.WithAIRequest_ImageData(&aicommon.ImageData{Data: raw}), aicommon.WithAIRequest_ExtraSpecOpts(aispec.WithMaxTokens(64), aispec.WithEnableThinking(false))))
		require.NoError(t, err)
		answer, err := io.ReadAll(response.GetOutputStreamReader("browser-effect-smoke", true, cfg.GetEmitter()))
		require.NoError(t, err)
		require.NoError(t, response.GetError())
		require.Contains(t, strings.ToLower(string(answer)), "saved")
		t.Log("real browser + public vision verified the post-click heading")
	}
}
