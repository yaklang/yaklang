package yak

import (
	"context"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
)

func TestForgeToolAdapterCallsOnceAndPreservesDefault(t *testing.T) {
	code := `standalone = cli.Bool("standalone-tool", cli.setDefault(false))
count = 0
forgeHandle = func(params) { count++; assert count == 1; return sprintf("called:%v", count) }
if standalone { yakit.Info(forgeHandle({})) }`
	source := &schema.AIYakTool{Name: "handler-test", Content: code, Params: `{"type":"object","properties":{"standalone-tool":{"type":"boolean"}}}`}
	for _, standalone := range []bool{false, true} {
		tool := YakTool2AIToolWithForgeHandle([]*schema.AIYakTool{source})[0]
		result, err := tool.ExecuteToolWithCapture(context.Background(), map[string]any{"standalone-tool": standalone}, aitool.NewToolInvokeConfig())
		if err != nil {
			t.Fatal(err)
		}
		if (standalone && (result.Result != nil || strings.Count(result.Stdout, "called:1") != 1)) || (!standalone && result.Result != "called:1") {
			t.Fatalf("standalone=%v result=%#v", standalone, result)
		}
	}
	ordinary := YakTool2AITool([]*schema.AIYakTool{source})[0]
	result, err := ordinary.ExecuteToolWithCapture(context.Background(), map[string]any{}, aitool.NewToolInvokeConfig())
	if err != nil {
		t.Fatal(err)
	}
	if result.Result != nil {
		t.Fatalf("default converter invoked handler: %#v", result)
	}
	source.Content = `RESULT = "explicit"; forgeHandle = func(params) { panic("must not call") }`
	result, err = YakTool2AIToolWithForgeHandle([]*schema.AIYakTool{source})[0].ExecuteToolWithCapture(context.Background(), map[string]any{}, aitool.NewToolInvokeConfig())
	if err != nil || result.Result != "explicit" {
		t.Fatalf("RESULT precedence: %#v %v", result, err)
	}
	source.Content = `yakit.Info("warning only"); forgeHandle = func(params) { panic("must not execute again") }`
	result, err = YakTool2AIToolWithForgeHandle([]*schema.AIYakTool{source})[0].ExecuteToolWithCapture(context.Background(), map[string]any{"standalone-tool": true}, aitool.NewToolInvokeConfig())
	if err != nil || result.Result != nil || !strings.Contains(result.Stdout, "warning only") {
		t.Fatalf("standalone observations must remain valid without a return value: %#v %v", result, err)
	}
}
