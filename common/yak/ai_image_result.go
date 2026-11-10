package yak

import (
	"context"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/yak/antlr4yak"
)

func imageResult(raw []byte, mimeType string, result any) (*aitool.MediaResult, error) {
	return aitool.ImageResult(context.Background(), raw, mimeType, result)
}

// Bind the image decoder to the actual invocation lifetime. Scripts need not
// obtain a context through variables or change the process-global module map.
func bindAIImageResultToEngine(engine *antlr4yak.Engine, ctx context.Context) {
	exports := make(map[string]any)
	if value, ok := engine.GetVar("aiagent"); ok {
		if original, ok := value.(map[string]any); ok {
			for key, value := range original {
				exports[key] = value
			}
		}
	}
	exports["ImageResult"] = func(raw []byte, mimeType string, result any) (*aitool.MediaResult, error) {
		return aitool.ImageResult(ctx, raw, mimeType, result)
	}
	engine.SetVars(map[string]any{"aiagent": exports})
}
