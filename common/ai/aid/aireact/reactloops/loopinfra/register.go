package loopinfra

import (
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/schema"
)

const (
	LoopAction_RequireTool         = schema.AI_REACT_LOOP_ACTION_REQUIRE_TOOL
	LoopAction_AskForClarification = schema.AI_REACT_LOOP_ACTION_ASK_FOR_CLARIFICATION
)

func init() {
	reactloops.RegisterAction(loopAction_toolRequireAndCall.ActionType, loopAction_toolRequireAndCall)
	reactloops.RegisterAction(loopAction_AskForClarification.ActionType, loopAction_AskForClarification)
}
