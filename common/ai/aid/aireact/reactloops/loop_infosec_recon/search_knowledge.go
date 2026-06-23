package loop_infosec_recon

import (
	"fmt"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
)

const (
	keyInfosecEnhanceCount = "infosec_enhance_count"
	keyInfosecEnhanceData  = "infosec_enhance_data"
)

var searchKnowledgeInfosec = func(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopActionWithStreamField(
		"search_knowledge",
		"Search RAG / knowledge base for techniques, tools, or patterns relevant to recon and API discovery.",
		[]aitool.ToolOption{
			aitool.WithStringParam("input", aitool.WithParam_Description("Search keywords or question.")),
		},
		[]*reactloops.LoopStreamField{},
		func(loop *reactloops.ReActLoop, action *aicommon.Action) error {
			if action.GetString("input") == "" {
				return utils.Error("search_knowledge requires input")
			}
			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			n := loop.GetInt(keyInfosecEnhanceCount) + 1
			loop.Set(keyInfosecEnhanceCount, n)
			if n+1 >= loop.GetMaxIterations() {
				loop.RemoveAction("search_knowledge")
			}
			input := action.GetString("input")
			reactloops.EmitActionLog(loop, infosecReconToolNodeID, fmt.Sprintf("开始: search_knowledge / Start: search_knowledge (%s)", input))
			reactloops.EmitStatus(loop, "知识检索中 / Searching knowledge base...")

			invoker := loop.GetInvoker()
			ctx := loop.GetConfig().GetContext()
			task := loop.GetCurrentTask()
			if task != nil && !utils.IsNil(task.GetContext()) {
				ctx = task.GetContext()
			}
			enhanceData, err := invoker.EnhanceKnowledgeGetterEx(ctx, input, nil)
			if err != nil {
				op.Feedback("search_knowledge failed: " + err.Error())
				op.Continue()
				return
			}
			loop.Set(keyInfosecEnhanceData, enhanceData)
			appendInfosecReconLog(loop, "=== search_knowledge ===\n"+utils.ShrinkString(enhanceData, 8000))
			op.Feedback("search_knowledge completed")
			reactloops.EmitStatus(loop, "完成 / Complete")
			reactloops.EmitActionLog(loop, infosecReconToolNodeID, fmt.Sprintf("完成: search_knowledge (%d bytes) / Done: search_knowledge (%d bytes)", len(enhanceData), len(enhanceData)))
			op.Continue()
		},
	)
}
