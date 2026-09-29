package yak

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
)

func TestYakToolPrintlnObservations(t *testing.T) {
	for _, explicit := range []bool{false, true} {
		t.Run(fmt.Sprint(explicit), func(t *testing.T) {
			code := `n, err = println([]byte("hello"), byte(65), byte(1)); assert err == nil; assert n == 11; yakit.Info("feedback-marker")`
			if explicit {
				code += `; RESULT = "explicit-result"`
			}
			source := &schema.AIYakTool{Name: "stdout-test", Content: code, Params: `{"type":"object","properties":{}}`}
			tool := YakTool2AITool([]*schema.AIYakTool{source})[0]
			result, err := tool.ExecuteToolWithCapture(context.Background(), map[string]any{}, aitool.NewToolInvokeConfig())
			require.NoError(t, err)
			require.Contains(t, result.Stdout, "hello 65 1\n")
			require.Contains(t, result.Stdout, "feedback-marker")
			require.Equal(t, 1, strings.Count(result.CombinedOutput, "hello 65 1"))
			if explicit {
				require.Equal(t, "explicit-result", result.Result)
			} else {
				require.Nil(t, result.Result)
			}
		})
	}
}

func TestYakToolPrintlnInvocationIsolation(t *testing.T) {
	source := &schema.AIYakTool{Name: "isolated-stdout", Content: `println(cli.String("marker"))`, Params: `{"type":"object","properties":{"marker":{"type":"string"}}}`}
	tool := YakTool2AITool([]*schema.AIYakTool{source})[0]
	for i := 0; i < 8; i++ {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			t.Parallel()
			marker := "invocation-" + t.Name()
			result, err := tool.ExecuteToolWithCapture(context.Background(), map[string]any{"marker": marker}, aitool.NewToolInvokeConfig())
			require.NoError(t, err)
			require.Equal(t, marker+"\n", result.Stdout)
			require.Nil(t, result.Result)
		})
	}
}
