package loop_plan

import (
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
)

var generate = func(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopActionWithStreamField(
		"plan",
		"Generate a detailed plan based on the user's requirements or enhance the existing plan if one is already present.",
		[]aitool.ToolOption{
			aitool.WithStringParam("main_task", aitool.WithParam_Description("对用户原始需求进行提炼和重述，形成一个**清晰、具体、且可直接执行的主任务**。应以**动词开头**，明确指出核心行动，例如：'创建一个营销活动计划' 或 '分析用户流失数据'。")),
			aitool.WithStringParam("main_task_goal", aitool.WithParam_Description("定义主任务的最终目标及衡量其完成的明确标准。**必须清晰、无歧义地阐述以下三点**：1）**完成状态**：任务推进到何种程度可被视为已完成？2）**成功指标**：用哪些具体的、可量化的指标来评估任务是否成功达成目标？3）**交付成果**：任务完成后，预期的最终产出或交付物是什么？目标是提供一个**可验证的、客观的完成基准**。")),
			aitool.WithStructArrayParam(
				"tasks",
				[]aitool.PropertyOption{
					aitool.WithParam_Description("将主任务拆解为一系列**具体、可执行的小任务**，每个小任务应包含以下要素：1）**任务描述**：简明扼要地说明任务内容和预期结果；2）**优先级**：根据任务的重要性和紧急程度进行排序（高、中、低）；3）**依赖关系**：明确指出该任务是否依赖于其他任务的完成。确保所有小任务共同支持主任务的达成，并且每个任务都是独立且可操作的。"),
				},
				nil,
				aitool.WithStringParam("subtask_name", aitool.WithParam_Description("子任务的简洁、概括性名称。**强烈推荐采用‘动词+名词’的格式**（例如：'设计用户调研问卷'、'部署测试环境'），以便清晰表达子任务的核心动作和对象。**长度建议控制在20个汉字（或等效字符数）以内**，以方便在任务管理和沟通中引用与追踪。")),
				aitool.WithStringParam("subtask_goal", aitool.WithParam_Description("定义该子任务的具体目标和衡量其完成的明确标准。**必须清晰、无歧义地阐述以下三点**：1）**完成条件**：在什么具体情况下可以认定此子任务已完成？2）**交付物/输出要求**：此子任务完成后，应产出哪些具体的成果或达到哪些明确的输出标准？3）**成功指标（若适用）**：如果可能，提供可量化的指标来衡量子任务的完成质量。**目标是确保每个子任务都有一个明确、可验证的终点。** 例如，应描述为‘生成包含至少三个设计方案的初步设计稿’，而非‘进行初步设计’。避免使用如‘进一步分析’、‘收集相关信息’等缺乏明确完成标志的模糊描述")),
			),
		},
		[]*reactloops.LoopStreamField{
			{
				FieldName: "subtask_name",
				AINodeId:  "plan",
			},
		},
		func(l *reactloops.ReActLoop, action *aicommon.Action) error {
			mainName := action.GetString("main_task")
			mainGoal := action.GetString("main_task_goal")
			taskList := action.GetInvokeParamsArray("tasks")
			if mainGoal == "" || mainName == "" || len(taskList) == 0 {
				return utils.Errorf("main_task parameter is missing")
			}
			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			loop.Set(PLAN_DATA_KEY, string(utils.Jsonify(action.GetParams())))
			// todo 或许 review可以放在这里？
			op.Exit()
		},
	)
}
