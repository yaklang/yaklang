package loopinfra

import (
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

var loopAction_RequestPlanAndExecution = &reactloops.LoopAction{
	AsyncMode:   true,
	ActionType:  schema.AI_REACT_LOOP_ACTION_REQUEST_PLAN_EXECUTION,
	Description: `Request a detailed plan and execute it step-by-step to achieve the user's goal.`,
	Options: []aitool.ToolOption{
		aitool.WithStringParam(
			"plan_request_payload",
			aitool.WithParam_Description("USE THIS FIELD ONLY IF @action is 'request_plan_and_execution'. Provide a one-sentence summary of the complex task that needs a multi-step plan. This summary will trigger a more advanced planning system. Example: 'Create a marketing plan for a new product launch.'"),
		),
	},
	StreamFields: []*reactloops.LoopStreamField{
		{FieldName: `plan_request_payload`},
	},
	ActionVerifier: func(loop *reactloops.ReActLoop, action *aicommon.Action) error {
		// Check if there's already a plan execution task running
		invoker := loop.GetInvoker()
		if reactInvoker, ok := invoker.(interface {
			GetCurrentPlanExecutionTask() aicommon.AIStatefulTask
		}); ok {
			if reactInvoker.GetCurrentPlanExecutionTask() != nil {
				return utils.Errorf("another plan execution task is already running, please wait for it to complete or use directly_answer to provide the result")
			}
		}

		improveQuery := action.GetString("plan_request_payload")
		if improveQuery == "" {
			improveQuery = action.GetInvokeParams("next_action").GetString("plan_request_payload")
		}
		if improveQuery == "" {
			return utils.Errorf("request_plan_and_execution action must have 'plan_request_payload' field")
		}
		loop.Set("plan_request_payload", improveQuery)
		return nil
	},
	ActionHandler: func(loop *reactloops.ReActLoop, action *aicommon.Action, operator *reactloops.LoopActionHandlerOperator) {
		task := operator.GetTask()

		rewriteQuery := loop.Get("plan_request_payload")
		invoker := loop.GetInvoker()
		invoker.AsyncPlanAndExecute(task.GetContext(), rewriteQuery, func(err error) {
			loop.FinishAsyncTask(task, err)
		})
	},
	OutputExamples: `
# request_plan_and_execution 使用说明

## 适用场景
当任务具有以下特征时，应使用 request_plan_and_execution：
- 需要多步骤协作完成，单次工具调用无法覆盖
- 涉及多个领域或模块的交叉操作
- 需要先收集信息、再分析、再执行的链式流程
- 任务目标复杂，需要拆解为可管理的子任务

## plan_request_payload 编写规范

plan_request_payload 必须对用户的核心目标进行 SMART 框架拆解，而非简单复述用户原话。具体要求：

### SMART 拆解结构

1. **Specific（具体化）**：将用户的模糊目标转化为明确的、可操作的任务描述。指明操作对象、操作方式、预期产出
2. **Measurable（可衡量）**：为任务定义可验证的完成标准，说明如何判断任务是否成功
3. **Achievable（可实现）**：基于当前 agent 的实际能力（已有工具、蓝图、技能）评估可行性，不规划超出能力范围的步骤
4. **Relevant（相关性）**：确保拆解出的每个子目标都直接服务于用户的核心诉求，排除无关操作
5. **Time-bound（有序性）**：明确子任务的执行顺序和依赖关系，关键路径上的任务优先处理

### 能力约束声明

在 plan_request_payload 中必须结合 agent 的实际能力来描述任务：
- 如果需要使用特定工具，在描述中明确提及工具名称
- 如果需要使用蓝图（AI Blueprint），指明蓝图名称
- 如果需要加载技能（Skill），说明所需技能
- 不要规划当前 agent 无法执行的操作，如无对应工具或蓝图支持的步骤

## 输出示例

Example 1 - 代码分析与重构任务：

	{
		"@action": "request_plan_and_execution",
		"human_readable_thought": "用户需要对项目中的认证模块进行安全审计并重构，这涉及代码搜索、漏洞分析、方案设计和代码修改多个阶段，需要系统化规划",
		"plan_request_payload": "对项目认证模块进行安全审计与重构。具体目标：(1) 使用 find_files 和 grep_text 定位所有认证相关代码文件，梳理认证流程调用链；(2) 使用 read_file 逐一审查认证逻辑，识别硬编码凭据、缺失的输入校验、不安全的会话管理等安全风险；(3) 基于审计结果，设计符合 OWASP 标准的重构方案；(4) 使用代码编辑工具逐步实施修改，每步修改后验证功能完整性。完成标准：所有认证端点通过安全检查，无高危漏洞残留。"
	}

Example 2 - 信息收集与报告生成任务：

	{
		"@action": "request_plan_and_execution",
		"human_readable_thought": "用户需要对目标系统进行全面的技术调研并生成报告，需要从多个维度收集和整合信息",
		"plan_request_payload": "对目标系统进行技术调研并生成分析报告。任务拆解：(1) 使用 find_files 扫描项目结构，建立模块依赖关系图；(2) 使用 grep_text 搜索关键配置项和技术栈标识，确认技术选型；(3) 使用 read_file 阅读核心模块代码，分析架构设计模式；(4) 使用 web_search 查阅相关技术的最佳实践和已知问题；(5) 综合以上信息，输出结构化的技术调研报告，包含架构概览、风险评估和优化建议。每个子任务的产出作为下一步的输入，确保信息链完整。"
	}

Example 3 - 简洁但完整的任务拆解：

	{
		"@action": "request_plan_and_execution",
		"human_readable_thought": "用户需要为项目添加完整的 CI/CD 流水线配置，涉及环境分析、配置编写和验证",
		"plan_request_payload": "为当前项目配置 CI/CD 流水线。步骤：(1) 通过 find_files 和 read_file 分析项目类型、构建工具和依赖管理方式；(2) 通过 web_search 获取对应技术栈的 CI/CD 最佳实践；(3) 编写流水线配置文件，覆盖构建、测试、部署三个阶段；(4) 验证配置语法正确性。完成标准：配置文件语法无误，覆盖完整的构建-测试-部署流程。"
	}

## 反面示例（禁止）

以下写法过于模糊，缺乏 SMART 拆解，不应出现在 plan_request_payload 中：
- "帮我优化一下代码" — 未具体化操作对象和优化维度
- "分析项目并给出建议" — 未明确分析范围、方法和产出标准
- "修复所有 bug" — 未界定范围，不可衡量，不可实现
`,
}
