package yak

import (
	"context"
	"fmt"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yak/antlr4yak"
)

// YakTool2AIToolWithForgeHandle additionally supports a declared forgeHandle
// entrypoint when no explicit RESULT was produced. This opt-in keeps ordinary
// Yak tool semantics unchanged, including observation-only tools without an
// explicit return value. Imported scripts declaring standalone-tool execute their
// handler themselves and publish through println; this adapter never calls it twice.
// This is an execution convention, not a permissions sandbox.
func YakTool2AIToolWithForgeHandle(aitools []*schema.AIYakTool) []*aitool.Tool {
	return yakTool2AITool(aitools, true)
}

// invokeForgeToolHandler adapts an explicitly selected script convention. It
// does not choose which tools a platform permits or interpret business success.
func invokeForgeToolHandler(ctx context.Context, engine *antlr4yak.Engine, params aitool.InvokeParams) (any, error) {
	if standalone, _ := params["standalone-tool"].(bool); standalone {
		// The script already executed its handler; observations use tool stdout.
		return nil, nil
	}
	if _, ok := engine.GetVar(HOOK_AI_FORGE); ok {
		result, err := engine.SafeCallYakFunction(ctx, HOOK_AI_FORGE, []interface{}{map[string]any(params)})
		if err == nil && result == nil {
			return nil, fmt.Errorf("Forge tool handler produced no result")
		}
		return result, err
	}
	// Native observation-only tools emit through Yakit feedback without a result.
	return nil, nil
}
