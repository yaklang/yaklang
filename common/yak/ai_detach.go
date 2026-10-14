package yak

import (
	"fmt"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/yak/antlr4yak"
)

// Bind a private library copy for each invocation; mutating ai.Exports would
// let concurrent scripts detach one another's tools.
func bindAIDetachToEngine(engine *antlr4yak.Engine, runtime *aitool.ToolRuntimeConfig) {
	exports := make(map[string]any)
	if value, ok := engine.GetVar("ai"); ok {
		if original, ok := value.(map[string]any); ok {
			for key, value := range original {
				exports[key] = value
			}
		}
	}
	exports["Detach"] = func() (string, error) {
		if runtime == nil || runtime.Detach == nil {
			return "", fmt.Errorf("ai.Detach is only available inside a detachable AI tool invocation")
		}
		return runtime.Detach()
	}
	engine.SetVars(map[string]any{"ai": exports})
}
