package yak

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/yakscripttools"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yak/yaklang"
)

func TestYakAIDetachInvocationBinding(t *testing.T) {
	for _, id := range []string{"first", "second"} {
		engine := yaklang.New()
		bindAIDetachToEngine(engine, &aitool.ToolRuntimeConfig{Detach: func() (string, error) { return id, nil }})
		require.NoError(t, engine.SafeEval(context.Background(), `result = ai.Detach()~; assert(result == "`+id+`")`))
	}
	engine := yaklang.New()
	bindAIDetachToEngine(engine, nil)
	require.NoError(t, engine.SafeEval(context.Background(), `id, err = ai.Detach(); assert(err != nil)`))
	engine = yaklang.New()
	require.NoError(t, engine.SafeEval(context.Background(), `id, err = ai.Detach(); assert(err != nil)`), "an invocation must not mutate global ai exports")
}

func TestYakToolDefaultAsyncMetadata(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		source := `__DESC__ = "metadata test"` + "\n"
		if enabled {
			source += "__ASYNC__ = true\n"
		}
		spec := yakscripttools.LoadYakScriptToAiTools("default_async_metadata", source+`RESULT = "done"`)
		require.NotNil(t, spec)
		require.Equal(t, enabled, spec.DefaultAsync)
		tools := YakTool2AITool([]*schema.AIYakTool{spec})
		require.Len(t, tools, 1)
		require.Equal(t, enabled, tools[0].DefaultAsync)
	}
}
