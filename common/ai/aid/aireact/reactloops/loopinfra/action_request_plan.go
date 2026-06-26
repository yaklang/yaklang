package loopinfra

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_plan"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

const requestPlanDescription = `Request a multi-step plan for a complex task. Your responsibility ends at submitting the plan need—the planning system produces a plan for user review. Do not execute plan steps yourself; the user approves whether and when to run it. To revise an existing plan, call this action again; in plan_request_payload describe only the new plan requirements (not as a modification diff). Do not use directly_answer to acknowledge plan changes.`

var requestPlanOutputExamples = `
# request_plan_and_execution 使用说明

## 你的职责边界（重要）

- **你只负责生成 plan**：调用 request_plan / request_plan_and_execution 并填写 plan_request_payload，由规划系统产出 plan。
- **你不负责执行 plan**：plan 中的步骤不会因为你调用了本 action 就自动执行；是否在用户审查通过后执行、何时执行，**由用户决定**。
- **修订 plan 时也一样**：再次 request_plan 即可，仍只产出 plan，不代为执行。

## 适用场景
当任务具有以下特征时，应使用 request_plan_and_execution：
- 需要多步骤协作完成，单次工具调用无法覆盖
- 存在不确定性，需要先探索再决策
- 任务目标复杂，需要系统化的规划和拆解
- 涉及多个约束条件或风险因素需要综合考量
- **用户要调整已有 plan 的步骤或目标**（即使上一份 plan 已生成且用户尚未批准执行）

## 修改已有 plan（重要）

当用户要求调整已生成的 plan（例如替换子任务目标、增删步骤、改变依赖顺序）时：

- **正确做法**：再次调用 request_plan / request_plan_and_execution；规划系统会重新生成一份 plan，新 plan 替代旧 plan。
- **plan_request_payload 写法**：像首次规划一样，**直接描述新的 plan 需求**（目标、步骤、约束、交付标准）。**不要**在 payload 里写「修订」「修改」「覆盖原 plan」「把 A 改为 B」「保持第 X 步不变」等元叙述——用户改需求的意图由你触发 request_plan 这一动作即可表达，规划系统不需要 diff 说明。
- **错误做法**：用 directly_answer 回复「已收到需求变更」并等待用户再次确认——这不会更新 plan，用户也看不到新的规划结果。
- **无需用户二次确认**：收到调整指令后应直接重新 request_plan，而不是连续多轮确认。

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
- 修订/修改/覆盖/替换原 plan 等元叙述（调整场景也按全新需求描述，不要写 diff）
- 暗示你会或应该立即执行 plan 步骤的表述（执行由用户在审查后决定）

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

Example 4 - 用户改需求后重新规划（payload 只写新 plan，不写「修改」）：

	{
		"@action": "request_plan_and_execution",
		"human_readable_thought": "用户调整了漏洞验证重点，直接重新 request_plan 产出新 plan，执行仍由用户审查后决定",
		"plan_request_payload": "用户需求是:对授权测试目标 https://app.example.com 制定 Web 安全评估 plan，本阶段仅产出 plan、不直接发包验证。计划包含两个串行阶段：1) 梳理对外可访问入口、参数面与认证边界；2) 针对搜索类输入点做反射型 XSS 探测并留存请求响应证据。规划时需考虑 WAF/限流、登录态是否影响测试面、误报与人工复核边界。交付标准为产出完整、可审查的 plan。"
	}

## 反面示例（禁止）

以下写法不符合 plan_request_payload 的定位：

**过于模糊，缺乏问题分析：**
- "帮我优化一下代码" — 没有说明优化什么、边界在哪、怎样算成功
- "分析项目并给出建议" — 没有说明分析哪些维度、存在什么顾虑

**越俎代庖，替规划系统做执行决策：**
- "步骤1：使用 find_files 搜索文件；步骤2：使用 read_file 阅读代码；步骤3：..." — 具体用什么工具、怎么编排是规划系统的职责，plan_request_payload 只需描述问题和约束

**用 directly_answer 假装已调整 plan：**
- 用户说「把 plan 第二步从 SQL 注入改成 XSS」时，只回复「已收到需求变更，请选择路径 1 或 2」——必须用 request_plan 重新生成 plan，否则用户看不到更新后的规划

**在 payload 里写 diff 式元叙述：**
- "修订评估 plan：保持第一步信息收集不变，将第二步由 SQL 注入改为 XSS，覆盖原方案" — 应直接写新 plan 需求，例如先梳理入口再对搜索参数做 XSS 验证，不要提「修订」「覆盖」「改为」

**误以为 request_plan 会立即执行：**
- "生成 plan 后立刻对 /api/login 发包验证 SQL 注入" — request_plan 只产出 plan；是否执行由用户审查后决定，不要在 payload 里要求你或系统马上执行
`

var loopAction_RequestPlanAndExecution = &reactloops.LoopAction{
	AsyncMode:   false,
	ActionType:  schema.AI_REACT_LOOP_ACTION_REQUEST_PLAN_EXECUTION,
	Description: requestPlanDescription,
	Options: []aitool.ToolOption{
		aitool.WithStringParam(
			"plan_request_payload",
			aitool.WithParam_Description("USE THIS FIELD ONLY IF @action is 'request_plan_and_execution'. Summarize the plan need for the planning system: goals, scope, constraints, and success criteria. This triggers plan generation only—not execution; the user reviews and decides whether to run the plan. When revising an existing plan, describe the new plan requirements directly (no modification diff). Example: '用户需求是:...; 执行过程中需要考虑:...; 可能用到的工具方向有:...; 这个任务交付标准为:....'"),
		),
	},
	StreamFields: []*reactloops.LoopStreamField{
		{FieldName: `plan_request_payload`, AINodeId: "plan"},
	},
	ActionVerifier: func(loop *reactloops.ReActLoop, action *aicommon.Action) error {
		return verifyPlanRequestPayload(loop, action, schema.AI_REACT_LOOP_ACTION_REQUEST_PLAN_EXECUTION)
	},
	ActionHandler:  handleRequestPlanAction,
	OutputExamples: requestPlanOutputExamples,
}

// loopAction_RequestPlanAlias keeps request_plan resolvable for older prompts.
var loopAction_RequestPlanAlias = &reactloops.LoopAction{
	AsyncMode:      loopAction_RequestPlanAndExecution.AsyncMode,
	ActionType:     schema.AI_REACT_LOOP_ACTION_REQUEST_PLAN,
	Description:    loopAction_RequestPlanAndExecution.Description,
	Options:        loopAction_RequestPlanAndExecution.Options,
	StreamFields:   loopAction_RequestPlanAndExecution.StreamFields,
	OutputExamples: loopAction_RequestPlanAndExecution.OutputExamples,
	ActionVerifier: func(loop *reactloops.ReActLoop, action *aicommon.Action) error {
		return verifyPlanRequestPayload(loop, action, schema.AI_REACT_LOOP_ACTION_REQUEST_PLAN)
	},
	ActionHandler: handleRequestPlanAction,
}

func handleRequestPlanAction(loop *reactloops.ReActLoop, action *aicommon.Action, operator *reactloops.LoopActionHandlerOperator) {
	task := operator.GetTask()
	rewriteQuery := loop.Get("plan_request_payload")
	invoker := loop.GetInvoker()

	if strings.TrimSpace(rewriteQuery) == "" {
		rewriteQuery = task.GetUserInput()
	}

	if isDetachedPlanEnabled(invoker) {
		handleDetachedRequestPlan(loop, invoker, task, rewriteQuery, operator)
		return
	}
	handleLegacyAsyncPlanAndExecute(loop, invoker, task, rewriteQuery, operator)
}

func isDetachedPlanEnabled(invoker aicommon.AIInvokeRuntime) bool {
	if invoker == nil {
		return false
	}
	cfg := invoker.GetConfig()
	if cfg == nil {
		return false
	}
	getter, ok := cfg.(interface{ GetEnableDetachedPlan() bool })
	if !ok {
		return false
	}
	return getter.GetEnableDetachedPlan()
}

func handleDetachedRequestPlan(
	loop *reactloops.ReActLoop,
	invoker aicommon.AIInvokeRuntime,
	task aicommon.AIStatefulTask,
	rewriteQuery string,
	operator *reactloops.LoopActionHandlerOperator,
) {
	planTask := aicommon.NewStatefulTaskBase(
		task.GetId()+"_plan",
		rewriteQuery,
		task.GetContext(),
		task.GetEmitter(),
	)

	appendPlanPrompt := func(tagName, prompt string) string {
		if strings.TrimSpace(prompt) == "" {
			return ""
		}
		nonce := utils.RandStringBytes(8)
		return fmt.Sprintf(
			"\n<|%s_%s|>\n"+
				"%s\n"+
				"<|%s_END_%s|>\n",
			tagName, nonce, prompt, tagName, nonce)
	}

	var planPrompt string
	if globalConfig := yakit.GetCachedAIGlobalConfig(); globalConfig != nil && globalConfig.GetAIPlanPrompt() != "" {
		planPrompt += appendPlanPrompt("AI_PLAN", globalConfig.GetAIPlanPrompt())
	}
	cfg := invoker.GetConfig()
	if cfg != nil {
		if userPlanPrompt := cfg.GetConfigString("plan_prompt"); userPlanPrompt != "" {
			planPrompt += appendPlanPrompt("USER_PLAN", userPlanPrompt)
		}
		if planPrompt != "" {
			cfg.SetConfig(loop_plan.PLAN_PROMPT_KEY, planPrompt)
		}
	}

	var planLoop *reactloops.ReActLoop
	opts := []any{
		reactloops.WithOnLoopInstanceCreated(func(l *reactloops.ReActLoop) {
			planLoop = l
		}),
	}

	_, err := invoker.ExecuteLoopTaskIF(schema.AI_REACT_LOOP_NAME_PLAN, planTask, opts...)
	if err != nil {
		operator.Fail(err)
		return
	}

	if planLoop == nil {
		operator.Fail(utils.Error("plan loop instance not created"))
		return
	}

	planData := planLoop.Get(loop_plan.PLAN_DATA_KEY)
	if planData == "" {
		operator.Fail(utils.Error("plan loop finished without producing plan data"))
		return
	}

	planInput := &aicommon.ExecutePlanInput{
		PlanPayload:  rewriteQuery,
		PlanData:     planData,
		PlanFacts:    planLoop.Get(loop_plan.PLAN_FACTS_KEY),
		PlanDocument: planLoop.Get(loop_plan.PLAN_DOCUMENT_KEY),
	}

	if _, err = invoker.PublishDetachedPlan(task.GetContext(), planInput, task.GetId()); err != nil {
		operator.Fail(err)
		return
	}

	operator.Exit()
}

func handleLegacyAsyncPlanAndExecute(
	loop *reactloops.ReActLoop,
	invoker aicommon.AIInvokeRuntime,
	task aicommon.AIStatefulTask,
	rewriteQuery string,
	operator *reactloops.LoopActionHandlerOperator,
) {
	operator.RequestAsyncMode()
	task.SetAsyncMode(true)
	invoker.AsyncPlanAndExecute(task.GetContext(), rewriteQuery, func(err error) {
		loop.FinishAsyncTask(task, err)
	})
}

