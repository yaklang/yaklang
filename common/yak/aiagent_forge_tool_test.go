package yak

import (
	"context"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
	"strings"
	"sync"
	"testing"
)

func TestForgeToolOutputBoundedAndConcurrent(t *testing.T) {
	var output forgeToolOutput
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); output.Write([]byte(strings.Repeat("x", maxForgeToolOutputBytes/4))) }()
	}
	wg.Wait()
	if output.buf.Len() != maxForgeToolOutputBytes {
		t.Fatalf("buffer size=%d", output.buf.Len())
	}
	if _, err := output.result(); err == nil {
		t.Fatal("oversized output accepted")
	}
	var empty forgeToolOutput
	if _, err := empty.result(); err == nil {
		t.Fatal("empty output accepted")
	}
}

func TestForgeToolAdapterCallsOnceAndPreservesDefault(t *testing.T) {
	code := `standalone = cli.Bool("standalone-tool", cli.setDefault(false))
count = 0
forgeHandle = func(params) { count++; return sprintf("called:%v", count) }
if standalone { println(forgeHandle({})) }`
	source := &schema.AIYakTool{Name: "handler-test", Content: code, Params: `{"type":"object","properties":{"standalone-tool":{"type":"boolean"}}}`}
	for _, standalone := range []bool{false, true} {
		tool := YakTool2AIToolWithForgeHandle([]*schema.AIYakTool{source})[0]
		result, err := tool.ExecuteToolWithCapture(context.Background(), map[string]any{"standalone-tool": standalone}, aitool.NewToolInvokeConfig())
		if err != nil {
			t.Fatal(err)
		}
		if result.Result != "called:1" {
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
	source.Content = `yakit.Info("warning only"); forgeHandle = func(params) { return "unused" }`
	_, err = YakTool2AIToolWithForgeHandle([]*schema.AIYakTool{source})[0].ExecuteToolWithCapture(context.Background(), map[string]any{"standalone-tool": true}, aitool.NewToolInvokeConfig())
	if err == nil {
		t.Fatal("warning-only output must not count as a standalone result")
	}
}
