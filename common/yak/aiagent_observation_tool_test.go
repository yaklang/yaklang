package yak

import (
	"context"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
)

func TestForgeToolAdapterPreservesObservationOnlyTools(t *testing.T) {
	source := &schema.AIYakTool{
		Name: "observation-only", Params: `{"type":"object","properties":{}}`,
		Content: `yakit.Info("OBSERVATION_MARKER: successful read-only response")`,
	}
	for _, convert := range []struct {
		name string
		fn   func([]*schema.AIYakTool) []*aitool.Tool
	}{{"ordinary", YakTool2AITool}, {"forge", YakTool2AIToolWithForgeHandle}} {
		t.Run(convert.name, func(t *testing.T) {
			result, err := convert.fn([]*schema.AIYakTool{source})[0].ExecuteToolWithCapture(context.Background(), map[string]any{}, aitool.NewToolInvokeConfig())
			if err != nil {
				t.Fatal(err)
			}
			if result.Result != nil || !strings.Contains(result.Stdout, "OBSERVATION_MARKER") {
				t.Fatalf("observation semantics changed: %#v", result)
			}
		})
	}
}

func TestForgeToolAdapterDoesNotHideExecutionErrors(t *testing.T) {
	source := &schema.AIYakTool{Name: "failing-observation", Params: `{"type":"object","properties":{}}`, Content: `yakit.Info("before failure"); panic("expected script failure")`}
	_, err := YakTool2AIToolWithForgeHandle([]*schema.AIYakTool{source})[0].ExecuteToolWithCapture(context.Background(), map[string]any{}, aitool.NewToolInvokeConfig())
	if err == nil {
		t.Fatal("script failure was converted to successful observations")
	}
}
