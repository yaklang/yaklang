package loopinfra

import (
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
)

func init() {
	reactloops.RegisterAction(loopAction_toolRequireAndCall)
	reactloops.RegisterAction(loopAction_AskForClarification)
	reactloops.RegisterAction(loopAction_EnhanceKnowledgeAnswer)
	reactloops.RegisterAction(loopAction_RequestPlanAndExecution)
	reactloops.RegisterAction(loopAction_RequireAIBlueprintForge)
}
