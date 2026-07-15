package loopinfra

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
		"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

const (
	dispatchSubReactJobsLoopKey        = reactloops.DispatchSubReactJobsLoopKey
	dispatchSubReactConcurrencyLoopKey = reactloops.DispatchSubReactConcurrencyLoopKey
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

	jobs, err := reactloops.ParseDispatchJobs(action)
	if err != nil {
		return err
	}

	concurrency := reactloops.ParseConcurrency(action, len(jobs))
	encoded, err := json.Marshal(jobs)
	if err != nil {
		return err
	}
	loop.Set(dispatchSubReactJobsLoopKey, string(encoded))
	loop.Set(dispatchSubReactConcurrencyLoopKey, concurrency)
	return nil
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

	rawJobs := loop.Get(dispatchSubReactJobsLoopKey)
	if strings.TrimSpace(rawJobs) == "" {
		operator.Fail(utils.Error("dispatch_sub_react_agents verifier state missing; retry the action"))
		return
	}
	var jobs []reactloops.DispatchJob
	if err := json.Unmarshal([]byte(rawJobs), &jobs); err != nil {
		operator.Fail(err)
		return
	}

	concurrency := loop.GetInt(dispatchSubReactConcurrencyLoopKey)
	if concurrency <= 0 {
		concurrency = reactloops.ParseConcurrency(action, len(jobs))
	}

	// Create or reuse a sub-agent progress registry on the parent loop so
	// stall heartbeat and verification watchdog can observe sub-agent activity
	// while this action handler blocks waiting for all sub-agents to finish.
	registry := loop.GetSubAgentProgressRegistry()
	if registry == nil {
		registry = reactloops.NewProgressRegistry()
		loop.SetSubAgentProgressRegistry(registry)
	}

	loopInfraStatus(loop, "子 Agent 执行中/ Sub Agents Running...")

	results := reactloops.RunJobsConcurrently(invoker, loop, parentTask, jobs, concurrency, registry)

	reactloops.SortJobResults(results)

	var feedbackLines []string
	successCount := 0
	for _, result := range results {
		if result == nil {
			continue
		}
		if result.Record.Status == "completed" {
			successCount++
		}
		writeSubReactAgentTimelineRecord(invoker, loop, result.Record)
		feedbackLines = append(feedbackLines, result.Feedback)
	}

	summary := fmt.Sprintf(
		"Dispatched %d sub react agents: %d succeeded, %d failed.",
		len(results), successCount, len(results)-successCount,
	)
	invoker.AddToTimeline("[DISPATCH_SUB_REACT_AGENTS_DONE]", summary)
	loopInfraActionFinish(loop, loopInfraNodeSubReactReport, summary)

	operator.Feedback(summary + "\n\n" + strings.Join(feedbackLines, "\n"))
	operator.Continue()
}

func writeSubReactAgentTimelineRecord(
	invoker aicommon.AIInvokeRuntime,
	parentLoop *reactloops.ReActLoop,
	record reactloops.TimelineRecord,
) {
	if invoker == nil {
		return
	}

	payload := record
	if strings.TrimSpace(payload.Result) != "" {
		if parentLoop != nil {
			ref, preview := loopInfraSaveReference(parentLoop, "sub_react_agent_"+record.SubAgentID, payload.Result, 800)
			if ref != "" {
				payload.ResultReference = ref
				payload.Result = preview
			}
		} else {
			payload.Result = utils.ShrinkTextBlock(payload.Result, 800)
		}
	}

	raw, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		log.Warnf("dispatch_sub_react_agents: marshal timeline record failed: %v", err)
		invoker.AddToTimeline(schema.AI_TIMELINE_ITEM_TYPE_SUB_REACT_AGENT_RESULT, utils.InterfaceToString(record))
		return
	}
	invoker.AddToTimeline(schema.AI_TIMELINE_ITEM_TYPE_SUB_REACT_AGENT_RESULT, string(raw))
}

var loopAction_DispatchSubReactAgents = &reactloops.LoopAction{
	ActionType: schema.AI_REACT_LOOP_ACTION_DISPATCH_SUB_REACT_AGENTS,
	Description: "Dispatch multiple INDEPENDENT sub ReAct agents in parallel. Each sub agent inherits the current timeline snapshot as context, " +
		"runs in an isolated timeline fork, and returns one structured result record back to the parent agent. " +
		"Sub agents cannot dispatch more sub agents, open plans, or be subject to goal-mode limits.\n" +
		"WHAT THIS IS FOR: parallelizing genuinely independent workstreams that can run at the same time without talking to each other (e.g. scan host A and scan host B).\n" +
		"WHAT THIS IS NOT FOR: (1) offloading a single sequential task you should do yourself — if the whole task is one chain of steps, do NOT dispatch it; (2) dumping every imaginable subtask into one call to avoid thinking. Only dispatch subtasks you have actually confirmed are independent.\n" +
		"DEPENDENCY RULE: every sub agent in ONE dispatch MUST be mutually independent — none may depend on another's input or result. If B depends on A's result, do NOT batch them: dispatch A (or do A yourself) now, wait for its result to land in the timeline, then in a LATER loop iteration dispatch B once the prior result is available. Group only no-dependency subtasks into the same dispatch.\n" +
		"GOAL QUALITY: give each sub agent a crisp, self-contained goal and a result_contract whenever possible, so it can finish and return a structured result without re-reading your reasoning.",
	Options: []aitool.ToolOption{
		aitool.WithStructArrayParam("dispatches",
			[]aitool.PropertyOption{
				aitool.WithParam_Required(true),
				aitool.WithParam_Description("Sub agent jobs to dispatch in parallel. Each item runs in an isolated timeline fork and returns one structured result back to the parent. " +
					"All jobs in one dispatch MUST be mutually independent — none may depend on another job's input or result. Dependent sub agents must be split across separate loop iterations: dispatch the first batch, wait for completion, then dispatch the dependent batch in the next iteration."),
			},
			nil,
			aitool.WithStringParam("identifier",
				aitool.WithParam_Description("Optional stable label for this sub agent. Auto-generated from array index when omitted."),
			),
			aitool.WithStringParam("goal",
				aitool.WithParam_Required(true),
				aitool.WithParam_Description("Short one-line intent for this sub agent — a noun phrase or single sentence, STRICTLY within 15 characters (English) / 15 字以内 (Chinese). Keep it brief; a complete, self-contained goal and result contract are elaborated automatically before the sub agent runs — do not write the full goal here."),
			),
			aitool.WithStringParam("task_name",
				aitool.WithParam_Description("Short, human-readable name for this sub agent's task, shown as the task title in the UI and timeline. Falls back to identifier, then goal when omitted. Prefer a concise noun phrase here rather than reusing the full goal sentence."),
			),
			aitool.WithStringParam("loop_name",
				aitool.WithParam_Description(fmt.Sprintf("Target ReAct loop name. Defaults to %q.", schema.AI_REACT_LOOP_NAME_DEFAULT)),
			),
		),
		aitool.WithIntegerParam(
			"concurrency",
			aitool.WithParam_Description(fmt.Sprintf("Parallelism for sub agent execution. Default min(len(dispatches), %d), max %d.", reactloops.DefaultDispatchConcurrency, reactloops.MaxDispatchConcurrency)),
		),
	},
	StreamFields: []*reactloops.LoopStreamField{
		{
			FieldName: "goal",
			AINodeId:  loopInfraNodeDispatchSubReact,
		},
		{
			FieldName: "concurrency",
			AINodeId:  loopInfraNodeDispatchConcurrency,
			IsSystem:  true,
		},
	},
	ActionVerifier: verifyDispatchSubReactAgents,
	ActionHandler:  handleDispatchSubReactAgents,
}
