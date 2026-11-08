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
	operator.Feedback(string(raw) + "\nAccepted, not completed. Continue useful coordination and acceptance work. If nothing useful remains, wait_sub_react_agents defaults to 30s; an observation timeout does not cancel workers. Results arrive in a later model input.")
	operator.Continue()
}

var loopAction_DispatchSubReactAgents = &reactloops.LoopAction{
	ActionType: schema.AI_REACT_LOOP_ACTION_DISPATCH_SUB_REACT_AGENTS,
	Description: "Delegate one or more bounded tasks to sub ReAct agents for deep multi-step execution, a phase of a sequential workflow, exploration paths, or independent parallel work. Prefer delegation for substantial execution so the parent can maintain the main objective, coordinate dependencies and review results. Choose context_mode per job: fork (default) inherits a submission-time history snapshot; task_only starts from the explicit task brief without parent conversation, evidence or input attachments. " +
		"Use fork when prior investigation is needed; use task_only for self-contained searches or independent reviews, including required constraints, inputs and file paths in goal. Both modes keep host instructions, capabilities, shared risk deduplication and cancellation scope. " +
		"This action returns accepted job IDs immediately; continue useful coordination and acceptance work while children run. " +
		"Use inspect_sub_react_agents for progress, wait_sub_react_agents for a bounded wait (default 30 seconds), or cancel_sub_react_agents. New results enter the next model input automatically. " +
		"Children should use save_evidence to publish decision-relevant discoveries to the parent evidence journal while still working; new shared evidence can return a waiting parent early. Private traces are not merged. " +
		"Sub agents cannot dispatch more sub agents, open plans, or be subject to goal-mode limits.\n" +
		"PHASE DELEGATION: a single child may execute a complete phase with sequential steps. Review its result before dispatching a dependent later phase. Multiple parallel tasks are not required.\n" +
		"EXPLORATION: delegate promising investigation paths with concrete hypotheses, authorized scope, evidence requirements and stopping conditions. The parent maintains the main line, compares findings and selects follow-up work; do not launch every speculative branch or duplicate delegated execution.\n" +
		"DEPENDENCY RULE: every sub agent in ONE dispatch MUST be mutually independent and non-conflicting — none may depend on another's input or result. If B depends on A's result, do NOT batch them: dispatch A now, review its result, then in a LATER loop iteration dispatch B with the actual prerequisite evidence. Sequential steps within one child are allowed.\n" +
		"GOAL QUALITY: give each sub agent a crisp, self-contained goal and result_contract including required inputs, user constraints, deliverables, evidence references, unresolved questions and blockers. The parent reviews the result against the overall user goal before accepting it, continuing delegation, or requesting finish.",
	Options: []aitool.ToolOption{
		aitool.WithStructArrayParam("dispatches",
			[]aitool.PropertyOption{
				aitool.WithParam_Required(true),
				aitool.WithParam_Description("One or more delegated jobs. A single job may contain a multi-step sequential phase or bounded exploration path. Each item has its own context_mode and independent timeline, and returns one structured result back to the parent. " +
					"All jobs in one dispatch MUST be mutually independent and non-conflicting — none may depend on another job's input or result. Dependent phases must be split across separate loop iterations: dispatch the prerequisite, review its result, then dispatch the next phase using that result."),
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
				aitool.WithParam_Description("Self-contained execution phase or exploration goal, required inputs, user constraints and scope. Sequential steps within this task are allowed. Goal elaboration runs in the background and must preserve this scope."),
			),
			aitool.WithStringParam("result_contract", aitool.WithParam_Description("Deliverables, evidence references, acceptance criteria, unresolved questions and blockers for the parent's review.")),
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
