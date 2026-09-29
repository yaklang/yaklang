package yak

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
)

func TestYakToolPrintObservations(t *testing.T) {
	for _, explicit := range []bool{false, true} {
		t.Run(fmt.Sprint(explicit), func(t *testing.T) {
			code := `n, err = print("prefix:"); assert err == nil; assert n == 7; n, err = printf("%s=%02d|", "value", 3); assert err == nil; assert n == 9; n, err = println([]byte("hello"), byte(65), byte(1)); assert err == nil; assert n == 11; yakit.Info("feedback-marker"); dump("dump-marker", true)`
			if explicit {
				code += `; RESULT = "explicit-result"`
			}
			source := &schema.AIYakTool{Name: "stdout-test", Content: code, Params: `{"type":"object","properties":{}}`}
			tool := YakTool2AITool([]*schema.AIYakTool{source})[0]
			result, err := tool.ExecuteToolWithCapture(context.Background(), map[string]any{}, aitool.NewToolInvokeConfig())
			require.NoError(t, err)
			require.Contains(t, result.Stdout, "prefix:value=03|hello 65 1\n")
			require.Contains(t, result.Stdout, "feedback-marker")
			require.Contains(t, result.Stdout, spew.Sdump("dump-marker", true))
			require.Equal(t, 1, strings.Count(result.CombinedOutput, "dump-marker"))
			require.Equal(t, 1, strings.Count(result.CombinedOutput, "hello 65 1"))
			if explicit {
				require.Equal(t, "explicit-result", result.Result)
			} else {
				require.Nil(t, result.Result)
			}
		})
	}
}

func TestYakToolPrintInvocationIsolation(t *testing.T) {
	source := &schema.AIYakTool{Name: "isolated-stdout", Content: `marker = cli.String("marker"); print(marker); printf("|%s|", marker); println(marker); dump(marker)`, Params: `{"type":"object","properties":{"marker":{"type":"string"}}}`}
	tool := YakTool2AITool([]*schema.AIYakTool{source})[0]
	for i := 0; i < 8; i++ {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			t.Parallel()
			marker := "invocation-" + t.Name()
			result, err := tool.ExecuteToolWithCapture(context.Background(), map[string]any{"marker": marker}, aitool.NewToolInvokeConfig())
			require.NoError(t, err)
			require.Equal(t, marker+"|"+marker+"|"+marker+"\n"+spew.Sdump(marker), result.Stdout)
			require.Nil(t, result.Result)
		})
	}
}
