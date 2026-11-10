package yak

import (
	"bytes"
	"context"
	"encoding/base64"
	"image"
	"image/png"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/yakscripttools"
	"github.com/yaklang/yaklang/common/schema"
	analyzeresult "github.com/yaklang/yaklang/common/yak/static_analyzer/result"
	"github.com/yaklang/yaklang/common/yak/yaklang"
)

// Exercise the same RESULT export path as the browser script, without requiring
// Chrome. The runtime must preserve the Go media type across the Yak VM.
func TestYakToolImageResultChannel(t *testing.T) {
	var raw bytes.Buffer
	require.NoError(t, png.Encode(&raw, image.NewRGBA(image.Rect(0, 0, 19, 7))))
	encoded := base64.StdEncoding.EncodeToString(raw.Bytes())
	source := `__DESC__ = "image result fixture"
RESULT = aiagent.ImageResult(codec.DecodeBase64("` + encoded + `")~, "image/png", {"operation": "screenshot"})~
`
	spec := yakscripttools.LoadYakScriptToAiTools("image-result-fixture", source)
	require.NotNil(t, spec)
	tools := YakTool2AITool([]*schema.AIYakTool{spec})
	require.Len(t, tools, 1)
	result, err := tools[0].InvokeWithParams(map[string]any{}, aitool.WithContext(context.Background()))
	require.NoError(t, err)
	require.Len(t, result.Images, 1)
	require.Equal(t, 19, result.Images[0].Width)
	data, err := result.Images[0].Bytes(context.Background())
	require.NoError(t, err)
	require.Equal(t, raw.Bytes(), data)
	require.NotContains(t, result.String(), encoded)
}

func TestYakImageResultBindingPreservesExportsAndCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	engine := yaklang.New()
	called := false
	engine.SetVars(map[string]any{"aiagent": map[string]any{"sentinel": func() { called = true }}})
	bindAIImageResultToEngine(engine, ctx)
	require.NoError(t, engine.SafeEval(context.Background(), `aiagent.sentinel(); _, err = aiagent.ImageResult(b"invalid", "image/png", nil); assert(err != nil)`))
	require.True(t, called)
	fresh := yaklang.New()
	require.NoError(t, fresh.SafeEval(context.Background(), `assert(aiagent.ImageResult != nil)`))
}

func TestBrowserScreenshotScriptStaticAnalysis(t *testing.T) {
	source, err := yakscripttools.GetEmbedFS().ReadFile("yakscriptforai/browser/use_browser.yak")
	require.NoError(t, err)
	for _, diagnostic := range StaticAnalyze(string(source)) {
		require.NotEqual(t, analyzeresult.Error, diagnostic.Severity, diagnostic.String())
	}
}
