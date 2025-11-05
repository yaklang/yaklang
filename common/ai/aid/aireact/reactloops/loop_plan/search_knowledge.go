package loop_plan

import (
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
)

var searchKnowledge = func(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopActionWithStreamField(
		"search_knowledge",
		`搜索补充知识的工具 - 在RAG数据库等各种来源搜索ai系统本身不确定的知识，帮助任务规划和决策。`,
		[]aitool.ToolOption{
			aitool.WithStringParam("input", aitool.WithParam_Description("用于搜索的输入文本或关键词。")),
		},
		[]*reactloops.LoopStreamField{},
		// Validator
		func(r *reactloops.ReActLoop, action *aicommon.Action) error {
			pattern := action.GetString("input")
			if pattern == "" {
				return utils.Error("search_knowledge requires 'input' parameter")
			}

			return nil
		},
		// Handler
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			allCount := loop.GetInt(PLAN_ENHANCE_COUNT) + 1
			loop.Set(PLAN_ENHANCE_COUNT, allCount)
			if allCount+1 >= loop.GetMaxIterations() {
				loop.RemoveAction("search_knowledge") // 防止无限搜索, 至少需要留一个循环给其他操作
			}
			input := action.GetString("input")
			invoker := loop.GetInvoker()
			ctx := loop.GetConfig().GetContext()
			task := loop.GetCurrentTask()
			if task != nil && !utils.IsNil(task.GetContext()) {
				ctx = task.GetContext()
			}
			enhanceData, err := invoker.EnhanceKnowledgeGetter(ctx, input)
			if err != nil {
				return
			}
			loop.Set(PLAN_ENHANCE_KEY, enhanceData)
		},
	)
}
