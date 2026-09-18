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
		operator.Feedback("Sub-agent submission rejected: " + err.Error())
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
	operator.Feedback(string(raw) + "\nAccepted, not completed. Continue independent work. If nothing useful remains, wait_sub_react_agents defaults to 30s; an observation timeout does not cancel workers. Results arrive in a later model input.")
	operator.Continue()
}

var loopAction_DispatchSubReactAgents = &reactloops.LoopAction{
	ActionType: schema.AI_REACT_LOOP_ACTION_DISPATCH_SUB_REACT_AGENTS,
	Description: "Dispatch multiple INDEPENDENT sub ReAct agents in parallel. Choose context_mode per job: fork (default) inherits a submission-time history snapshot; task_only starts from the explicit task brief without parent conversation, evidence or input attachments. " +
		"Use fork when prior investigation is needed; use task_only for self-contained searches or independent reviews, including required constraints, inputs and file paths in goal. Both modes keep host instructions, capabilities, shared risk deduplication and cancellation scope. " +
		"This action returns accepted job IDs immediately; continue independent work while children run. " +
		"Use inspect_sub_react_agents for progress, wait_sub_react_agents for a bounded wait (default 30 seconds), or cancel_sub_react_agents. New results enter the next model input automatically. " +
		"Sub agents cannot dispatch more sub agents, open plans, or be subject to goal-mode limits.\n" +
		"WHAT THIS IS FOR: parallelizing genuinely independent workstreams that can run at the same time without talking to each other (e.g. scan host A and scan host B).\n" +
		"WHAT THIS IS NOT FOR: (1) offloading a single sequential task you should do yourself — if the whole task is one chain of steps, do NOT dispatch it; (2) dumping every imaginable subtask into one call to avoid thinking. Only dispatch subtasks you have actually confirmed are independent.\n" +
		"DEPENDENCY RULE: every sub agent in ONE dispatch MUST be mutually independent — none may depend on another's input or result. If B depends on A's result, do NOT batch them: dispatch A (or do A yourself) now, wait for its result to land in the timeline, then in a LATER loop iteration dispatch B once the prior result is available. Group only no-dependency subtasks into the same dispatch.\n" +
		"GOAL QUALITY: give each sub agent a crisp, self-contained goal and a result_contract whenever possible, so it can finish and return a structured result without re-reading your reasoning.",
	Options: []aitool.ToolOption{
		aitool.WithStructArrayParam("dispatches",
			[]aitool.PropertyOption{
				aitool.WithParam_Required(true),
				aitool.WithParam_Description("Sub agent jobs to dispatch in parallel. Each item has its own context_mode and independent timeline, and returns one structured result back to the parent. " +
					"All jobs in one dispatch MUST be mutually independent — none may depend on another job's input or result. Dependent sub agents must be split across separate loop iterations: dispatch the first batch, wait for completion, then dispatch the dependent batch in the next iteration."),
			},
			nil,
			aitool.WithStringParam("context_mode",
				aitool.WithParam_EnumString(reactloops.SubAgentContextFork, reactloops.SubAgentContextTaskOnly),
				aitool.WithParam_Description("Default fork: inherit the history snapshot captured at dispatch, never later parent updates. task_only: no parent conversation/evidence/input attachments, plan partitions or automatic memory recall; provide all necessary task facts and constraints in goal. Resource permissions and host policy are unchanged.")),
			aitool.WithStringParam("identifier",
				aitool.WithParam_Description("Optional stable label for this sub agent. Auto-generated from array index when omitted."),
			),
			aitool.WithStringParam("goal",
				aitool.WithParam_Required(true),
				aitool.WithParam_Description("Self-contained goal, required inputs and constraints. Goal elaboration runs in the background and must preserve this scope."),
			),
			aitool.WithStringParam("result_contract", aitool.WithParam_Description("Required evidence, output format and acceptance criteria.")),
			aitool.WithStringParam("task_name",
				aitool.WithParam_Description("Short, human-readable name for this sub agent's task, shown as the task title in the UI and timeline. Falls back to identifier, then goal when omitted. Prefer a concise noun phrase here rather than reusing the full goal sentence."),
			),
			aitool.WithStringParam("loop_name",
				aitool.WithParam_Description(fmt.Sprintf("Target ReAct loop name. Defaults to %q.", schema.AI_REACT_LOOP_NAME_DEFAULT)),
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
