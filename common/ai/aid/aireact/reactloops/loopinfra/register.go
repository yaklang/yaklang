package loopinfra

import (
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
)

func init() {
	reactloops.RegisterAction(loopAction_toolRequireAndCall)
	reactloops.RegisterAction(loopAction_directlyCallTool)
	reactloops.RegisterAction(loopAction_AskForClarification)
	reactloops.RegisterAction(loopAction_EnhanceKnowledgeAnswer)
	reactloops.RegisterAction(loopAction_RequestPlanAndExecution)
	reactloops.RegisterAction(loopAction_RequestPlanAlias)
	reactloops.RegisterAction(loopAction_RequireAIBlueprintForge)
	reactloops.RegisterAction(loopAction_toolCompose)
	reactloops.RegisterAction(loopAction_LoadingSkills)
	reactloops.RegisterAction(loopAction_ChangeSkillViewOffset)
	reactloops.RegisterAction(loopAction_LoadSkillResources)
	reactloops.RegisterAction(loopAction_SearchCapabilities)
	reactloops.RegisterAction(loopAction_LoadCapability)
	reactloops.RegisterAction(loopAction_QueryMCPServers)
	reactloops.RegisterAction(loopAction_QueryMCPTools)
	// adjust_todolist 是主循环 TODO 显式维护通道; 写入全局动作表后由
	// NewReActLoop 默认 inject 给所有 loop, verification 保持只读.
	// 关键词: adjust_todolist 全局注册, 默认所有 loop 可见
	reactloops.RegisterAction(loopAction_AdjustTodolist)
	reactloops.RegisterAction(loopAction_ListAsyncTasks)
	reactloops.RegisterAction(loopAction_DispatchSubReactAgents)
}
