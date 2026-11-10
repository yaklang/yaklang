package aireact

import (
	"fmt"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
)

// The tool result is committed before automatic vision. Both its image
// references and the resulting text therefore have an ordinary Timeline owner.
func (r *ReAct) observeToolImageResult(task aicommon.AIStatefulTask, result *aitool.ToolResult) {
	if result == nil || len(result.Images) == 0 {
		return
	}
	ctx := r.config.GetContext()
	question := "Describe visible evidence from this tool result."
	if task != nil {
		question = task.GetUserInput()
		if task.GetContext() != nil {
			ctx = task.GetContext()
		}
	}
	question += fmt.Sprintf("\nTool: %s\nExpected observation: %s", result.Name, result.CallExpectations)
	if loop := r.GetCurrentLoop(); loop != nil {
		if err := loop.ObserveToolImages(ctx, result, question); err != nil {
			log.Warnf("tool image observation failed: %v", err)
		}
		return
	}
	native, _, err := r.config.ImageInputRoute(false)
	if err != nil || native {
		return
	}
	for i, img := range result.Images {
		key := fmt.Sprintf("tool:%s:%d", result.ToolCallID, i)
		if err := r.config.RecordImageObservation(ctx, key, img, question); err != nil {
			log.Warnf("tool image observation failed: %v", err)
		}
	}
}
