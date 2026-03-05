package loop_plan

import (
	"fmt"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

func appendReconResults(loop *reactloops.ReActLoop, content string) {
	old := loop.Get(PLAN_RECON_RESULTS_KEY)
	if old == "" {
		loop.Set(PLAN_RECON_RESULTS_KEY, content)
	} else {
		loop.Set(PLAN_RECON_RESULTS_KEY, old+"\n\n"+content)
	}
}

// makeReconRequireAction creates a recon LoopAction that delegates to ExecuteToolRequiredAndCall.
//
// No extra ToolOption is declared — the AI's intent is conveyed through the always-present
// "human_readable_thought" field in the shared schema. The handler writes the thought
// to the timeline so the require-phase AI can use it as context for parameter generation.
// This avoids parameter-name collisions that would occur if multiple recon actions all
// declared a "request" field (OrderedMap.Set overwrites earlier entries with the same key).
func makeReconRequireAction(
	actionName string,
	targetToolName string,
	desc string,
) func(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return func(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
		return reactloops.WithRegisterLoopAction(
			actionName,
			desc,
			nil, // rely on human_readable_thought; avoids duplicate-param-name overwrites in buildSchema
			nil,
			func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
				invoker := loop.GetInvoker()
				ctx := loop.GetConfig().GetContext()
				task := loop.GetCurrentTask()
				if task != nil && !utils.IsNil(task.GetContext()) {
					ctx = task.GetContext()
				}

				thought := action.GetString("human_readable_thought")
				if thought != "" {
					invoker.AddToTimeline(actionName+"_intent", thought)
				}
				loop.LoadingStatus(fmt.Sprintf("executing %s: %s", targetToolName, utils.ShrinkString(thought, 80)))

				result, directly, err := invoker.ExecuteToolRequiredAndCall(ctx, targetToolName)
				if err != nil {
					log.Warnf("%s tool call failed: %v", targetToolName, err)
					failMsg := fmt.Sprintf(
						"%s FAILED: %v. "+
							"You may try %s again with a different approach, "+
							"or use other information gathering tools as alternatives.",
						actionName, err, actionName)
					invoker.AddToTimeline(actionName+"_failed", failMsg)
					op.Feedback(failMsg)
					op.Continue()
					return
				}

				if directly {
					invoker.AddToTimeline(actionName+"_skipped", "user chose to skip tool execution")
					op.Feedback(fmt.Sprintf("%s was skipped by user intervention.", actionName))
					op.Continue()
					return
				}

				if result == nil {
					emptyMsg := fmt.Sprintf("%s returned no result. Consider trying with different parameters.", actionName)
					invoker.AddToTimeline(actionName+"_empty", emptyMsg)
					op.Feedback(emptyMsg)
					op.Continue()
					return
				}

				content := utils.InterfaceToString(result.Data)
				if result.Error != "" {
					invoker.AddToTimeline(actionName+"_error", result.Error)
				}

				if content == "" {
					emptyMsg := fmt.Sprintf("%s returned empty content. The target may be unreachable or no results found.", actionName)
					invoker.AddToTimeline(actionName+"_empty", emptyMsg)
					op.Feedback(emptyMsg)
					op.Continue()
					return
				}

				thoughtHint := utils.ShrinkString(thought, 120)
				entry := fmt.Sprintf("=== %s: %s ===\n%s", actionName, thoughtHint, utils.ShrinkString(content, 4096))
				appendReconResults(loop, entry)

				invoker.AddToTimeline(actionName+"_result",
					fmt.Sprintf("[%s] %s\n\n%s", targetToolName, thoughtHint, utils.ShrinkString(content, 2048)))

				op.Feedback(fmt.Sprintf("%s completed: '%s'", actionName, thoughtHint))
				op.Continue()
			},
		)
	}
}

var scanPortAction = makeReconRequireAction(
	"scan_port", "scan_port",
	"Scan target host ports to discover open services and their fingerprints. "+
		"Useful for understanding the target's network exposure before planning penetration testing or security assessment tasks. "+
		"Describe the target hosts, ports of interest, and scan preferences in the 'request' field.",
)

var simpleCrawlerAction = makeReconRequireAction(
	"simple_crawler", "simple_crawler",
	"Crawl a target web application to discover URLs, pages, and entry points without using a browser. "+
		"Useful for mapping the attack surface of web applications before planning security testing tasks. "+
		"Describe the target URLs, crawl depth, and any preferences in the 'request' field.",
)
