package loopinfra

import (
	"encoding/json"
	"fmt"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

func getSubAgentDepth(loop *reactloops.ReActLoop) int {
	if loop == nil {
		return 0
	}
	return loop.GetInt(reactloops.SubAgentDepthLoopVar)
}

func verifyDispatchSubReactAgents(loop *reactloops.ReActLoop, action *aicommon.Action) error {
	if getSubAgentDepth(loop) > 0 {
		return utils.Error("dispatch_sub_react_agents is only available in top-level agent; sub agents cannot dispatch more sub agents")
	}

	_, err := reactloops.ParseDispatchJobs(action)
	return err
}

func handleDispatchSubReactAgents(
	loop *reactloops.ReActLoop,
	action *aicommon.Action,
	operator *reactloops.LoopActionHandlerOperator,
) {
	invoker := loop.GetInvoker()
	parentTask := operator.GetTask()
	if parentTask == nil {
		parentTask = loop.GetCurrentTask()
	}

	jobs, err := reactloops.ParseDispatchJobs(action)
	if err != nil {
		operator.Fail(err)
		return
	}

	receipt, err := loop.SubmitSubAgents(parentTask, jobs, reactloops.SubAgentOptions{
		TimelineMode:   reactloops.SubAgentTimelineFork,
		ElaborateGoals: true,
	}, fmt.Sprintf("%s:%d", parentTask.GetUUID(), loop.GetCurrentIterationIndex()))
	if err != nil {
		operator.Feedback("子任务派发被拒绝：" + err.Error())
		operator.Continue()
		return
	}
	raw, _ := json.Marshal(receipt)
	invoker.AddToTimeline("sub_agent_dispatch_accepted", string(raw))
	// The receipt is a model protocol. The user-facing stream must describe
	// acceptance, not expose JSON or imply the child work already completed.
	summary := fmt.Sprintf("已派发 %d 个子任务，正在后台执行；主任务将继续处理其他工作。", len(receipt.Jobs))
	loopInfraActionFinish(loop, loopInfraNodeSubReactReport, summary)
	reactloops.EmitStatusI18n(loop, summary,
		fmt.Sprintf("Dispatched %d background tasks; continuing other work.", len(receipt.Jobs)),
		aicommon.WithStatusCode("subagent.dispatched"))
	operator.Feedback(string(raw) + "\n任务已受理，尚未完成。继续协调和验收；无可推进工作时用 wait_sub_react_agents 等待，默认 30 秒，超时不取消子任务。后续模型输入会自动包含结果；新共享证据也可提前唤醒等待。")
	operator.Continue()
}

var loopAction_DispatchSubReactAgents = &reactloops.LoopAction{
	ActionType: schema.AI_REACT_LOOP_ACTION_DISPATCH_SUB_REACT_AGENTS,
	Description: "将范围明确的执行阶段、探索分支或独立工作交给子智能体深入处理，主智能体维护总目标、衔接依赖并验收结果。" +
		"单个子任务可顺序执行多步，无须凑并行任务；同批任务必须独立且无冲突。后序依赖前序时，先派发并验收前序，再用真实结果派发后序。" +
		"任务书须写清输入、约束、范围、交付、证据、未确认项和阻塞；探索另给假设和停止条件，避免重复执行或无依据地扩展分支。" +
		"context_mode 默认 fork，继承派发时的上下文快照；task_only 仅使用显式任务书，不继承父对话、证据或输入附件，须在 goal 中补齐必要事实、约束和路径。两者均保留宿主权限、能力、风险去重和取消范围。" +
		"派发只返回受理标识。继续协调和验收；用 inspect_sub_react_agents 查看进度，wait_sub_react_agents 有界等待（默认 30 秒），cancel_sub_react_agents 显式取消。结果自动进入后续模型输入，等待超时不取消子任务。" +
		"子智能体应及时用 save_evidence 共享影响主线的发现，写入父证据日志并提前唤醒等待；私有执行轨迹不合并。阶段交付后核对总目标和剩余待办，再继续派发或 finish。" +
		"子智能体不能再次派发、开启计划，也不受目标模式门禁约束。",
	Options: []aitool.ToolOption{
		aitool.WithStructArrayParam("dispatches",
			[]aitool.PropertyOption{
				aitool.WithParam_Required(true),
				aitool.WithParam_Description("一个或多个子任务，各自拥有上下文模式和独立时间线，并返回结构化结果。单项可含顺序多步；同批须独立且无冲突，有依赖的阶段须分批派发并先验收前序。"),
			},
			nil,
			aitool.WithStringParam("context_mode",
				aitool.WithParam_EnumString(reactloops.SubAgentContextFork, reactloops.SubAgentContextTaskOnly),
				aitool.WithParam_Description("默认 fork：继承派发时的历史快照，不含之后的父更新。task_only：不继承父对话、证据、输入附件、计划分区或自动记忆召回，须在 goal 中写全任务事实和约束。两者均不改变资源权限和宿主策略。")),
			aitool.WithStringParam("identifier",
				aitool.WithParam_Description("可选的稳定子任务标识，省略时按数组位置生成。"),
			),
			aitool.WithStringParam("goal",
				aitool.WithParam_Required(true),
				aitool.WithParam_Description("独立可执行的阶段或探索目标，含必要输入、用户约束和范围；内部允许顺序多步。后台补全任务书时不得扩大范围。"),
			),
			aitool.WithStringParam("result_contract", aitool.WithParam_Description("交付内容、证据引用、验收标准、未确认项和阻塞条件，供主智能体验收。")),
			aitool.WithStringParam("task_name",
				aitool.WithParam_Description("界面和时间线展示的简短任务名；省略时依次使用 identifier、goal。不要重复整段目标。"),
			),
			aitool.WithStringParam("loop_name",
				aitool.WithParam_Description(fmt.Sprintf("目标循环名称，默认为 %q。", schema.AI_REACT_LOOP_NAME_DEFAULT)),
			),
		),
	},
	StreamFields: []*reactloops.LoopStreamField{
		{
			FieldName: "goal",
			AINodeId:  loopInfraNodeDispatchSubReact,
		},
	},
	ActionVerifier: verifyDispatchSubReactAgents,
	ActionHandler:  handleDispatchSubReactAgents,
}
