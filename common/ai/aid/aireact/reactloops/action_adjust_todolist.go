package reactloops

import (
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

const nativeAdjustTodolistActionName = "adjust_todolist"

// Native TODO maintenance uses the same canonical delta and execution path as
// text-mode action sidecars, but exposes it through one dedicated function.
var loopAction_AdjustTodolistNative = &LoopAction{
	ActionType:        nativeAdjustTodolistActionName,
	NativeDescription: "Update the current task's TODO list and focus using prior observations. Call alone or alongside other tools. Changes to close or current must use evidence already available before the call; do not predict a sibling tool's result.",
	NativeOptions:     []aitool.ToolOption{todoDeltaSchemaOption(true, true)},
	NativeOnlyOptions: true,
	ActionHandler: func(_ *ReActLoop, _ *aicommon.Action, operator *LoopActionHandlerOperator) {
		// execOneCall already applied the canonical delta and emitted either the
		// applied state or a validation error. The feedback must not claim success
		// for a no-op or a rejected adjustment.
		operator.Feedback("TODO adjustment handled. Check TODO_DELTA for applied changes or TODO_DELTA_ERROR for corrections; an empty adjustment is a no-op.")
		operator.Continue()
	},
}
