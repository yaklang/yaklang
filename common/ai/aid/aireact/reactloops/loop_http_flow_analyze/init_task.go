package loop_http_flow_analyze

import (
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
)

func buildInitTask(r aicommon.AIInvokeRuntime) func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask, operator *reactloops.InitTaskOperator) {
	return func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask, operator *reactloops.InitTaskOperator) {
		datas := reactloops.RunAttachedExtraResourcesInit(r, loop, task.GetAttachedDatas())
		var flowIds []int64
		for _, data := range datas {
			switch data.(type) {
			case *aicommon.AttachedHTTPFlowResourceData:
				flowData := data.(*aicommon.AttachedHTTPFlowResourceData)
				flowIds = append(flowIds, flowData.IDs...)
			}
		}

		if len(flowIds) > 0 {
			loop.Set(attachedHTTPFlowIDsKey, flowIds)
		}
	}
}
