package aireact

import (
	"context"
	"fmt"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

func (r *ReAct) _invokeToolCall_ReviewWrongParamForTask(
	ctx context.Context,
	task aicommon.AIStatefulTask,
	tool *aitool.Tool,
	old aitool.InvokeParams,
	extraPrompt string,
	requestOptions ...aicommon.AIRequestOption,
) (aitool.InvokeParams, error) {
	feedback := extraPrompt
	if old != nil {
		feedback = fmt.Sprintf("The review rejected the parameters for tool %q: %s. Revise this proposal using the tool schema. Review feedback: %s", tool.Name, old.Dump(), extraPrompt)
	}
	r.AddToTimeline("tool_call_review", feedback)
	params, _, err := r.requestToolCallParamsForTask(ctx, task, tool, feedback, requestOptions...)
	return params, err
}
