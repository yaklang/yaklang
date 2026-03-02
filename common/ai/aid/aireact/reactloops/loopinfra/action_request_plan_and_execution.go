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
- 存在不确定性，需要先探索再决策
- 任务目标复杂，需要系统化的规划和拆解
- 涉及多个约束条件或风险因素需要综合考量

## plan_request_payload 的定位

plan_request_payload 是为规划系统提供的**问题素材**，而非执行方案本身。规划系统会在内部自主决定如何使用工具、如何拆解步骤、如何编排执行顺序。

因此，plan_request_payload 应聚焦于：
- 用户的核心目标是什么
- 问题的边界和范围在哪里
- 存在哪些不确定性和风险
- 需要从哪些维度去考虑
- 有哪些已知的约束条件

不应在 plan_request_payload 中出现：
- 具体的工具调用步骤（规划系统自己决定）
- 预设的执行顺序（规划系统自己编排）
- 详细的操作方法（规划系统自己选择）

## SMART 维度的问题分析

在编写 plan_request_payload 时，从 SMART 维度对用户目标进行问题分析：

1. **Specific（边界明确）**：用户真正要解决的问题是什么？涉及哪些模块/范围？有哪些不在范围内的内容需要排除？
2. **Measurable（成功标准）**：怎样才算完成？验收条件是什么？有没有可量化的指标？
3. **Achievable（可行性约束）**：当前环境有哪些已知的限制？哪些前置条件不确定（如网络可达性、权限、依赖是否存在）？哪些风险可能导致任务无法按预期执行？
4. **Relevant（核心关联）**：用户的真实意图是什么？任务背后的业务目标是什么？哪些方面是关键路径，哪些是可选的？
5. **Time-bound（优先级）**：哪些部分最紧迫？哪些可以延后处理？任务之间是否存在先后依赖？

## 输出示例

Example 1 - 对目标站点进行安全评估：

	{
		"@action": "request_plan_and_execution",
		"human_readable_thought": "用户要求对目标站点进行安全评估，但目标站点的可达性尚未确认，需要先探明环境状态再制定后续策略",
		"plan_request_payload": "对目标站点 https://target.example.com 进行安全评估。核心目标：识别该站点存在的安全风险并输出评估报告。需要考虑的问题：(1) 该站点当前是否可访问尚不确定，需要在规划中优先验证可达性，并准备不可达时的降级方案；(2) 评估范围应覆盖 Web 应用层面（输入校验、认证机制、信息泄露），不涉及基础设施层；(3) 站点可能存在 WAF 或限流策略，探测手段需要考虑被拦截的情况；(4) 最终产出应是结构化的风险清单，按严重程度分级。"
	}

Example 2 - 梳理并重构遗留代码模块：

	{
		"@action": "request_plan_and_execution",
		"human_readable_thought": "用户要重构一个遗留认证模块，模块的代码结构和依赖关系都不明确，需要先摸底再规划",
		"plan_request_payload": "梳理并重构项目中的认证模块。核心目标：将现有认证逻辑从分散状态整合为统一的、可维护的模块。需要考虑的问题：(1) 认证相关代码分布在哪些文件和模块中尚不清楚，需要先做全面的代码摸底；(2) 现有认证逻辑可能与业务代码耦合较深，重构时需评估解耦的影响范围；(3) 项目是否有测试覆盖不确定，重构后需要验证不破坏现有功能；(4) 成功标准：认证逻辑集中管理，对外提供统一接口，原有功能不受影响。"
	}

Example 3 - 技术调研与方案选型：

	{
		"@action": "request_plan_and_execution",
		"human_readable_thought": "用户需要为项目引入缓存机制，但技术选型和实施方案需要基于项目现状来判断",
		"plan_request_payload": "为当前项目引入缓存机制以优化性能。核心目标：在关键数据访问路径上添加缓存层，降低响应延迟。需要考虑的问题：(1) 当前项目的性能瓶颈在哪里需要先定位，避免盲目加缓存；(2) 缓存方案需要根据项目技术栈来选择（内存缓存 vs 分布式缓存 vs 文件缓存），不同方案的适用场景和引入成本不同；(3) 缓存一致性策略需要结合业务数据的更新频率来决定；(4) 成功标准：关键路径响应时间有可测量的改善，不引入数据不一致问题。"
	}

## 反面示例（禁止）

以下写法不符合 plan_request_payload 的定位：

**过于模糊，缺乏问题分析：**
- "帮我优化一下代码" — 没有说明优化什么、边界在哪、怎样算成功
- "分析项目并给出建议" — 没有说明分析哪些维度、存在什么顾虑

**越俎代庖，替规划系统做执行决策：**
- "步骤1：使用 find_files 搜索文件；步骤2：使用 read_file 阅读代码；步骤3：..." — 具体用什么工具、怎么编排是规划系统的职责，plan_request_payload 只需描述问题和约束
`,
}
