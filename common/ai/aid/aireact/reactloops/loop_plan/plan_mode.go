package loop_plan

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

const (
	PLAN_MODE_KEY        = "plan_mode"
	PLAN_MODE_REASON_KEY = "plan_mode_reason"
	PLAN_MODE_SIMPLE     = "simple"
	PLAN_MODE_DEEP       = "deep"
)

func isOpenPlanMode(loop *reactloops.ReActLoop) bool {
	return strings.TrimSpace(loop.Get(PLAN_MODE_KEY)) == ""
}

func isSimplePlanMode(loop *reactloops.ReActLoop) bool {
	return loop.Get(PLAN_MODE_KEY) == PLAN_MODE_SIMPLE
}

func isDeepPlanMode(loop *reactloops.ReActLoop) bool {
	return loop.Get(PLAN_MODE_KEY) == PLAN_MODE_DEEP
}

func shouldEnterDeepPlanModeFromAction(actionType string) bool {
	switch actionType {
	case "finish_exploration", "output_facts", "begin_deep_planning":
		return true
	}
	for _, name := range infoGatheringActions {
		if actionType == name {
			return true
		}
	}
	return false
}

func trackPlanModeFromAction(loop *reactloops.ReActLoop, action *reactloops.ActionRecord) {
	if action == nil || !isOpenPlanMode(loop) {
		return
	}
	switch action.ActionType {
	case "generate_direct_plan":
		reason := ""
		if action.ActionParams != nil {
			reason = strings.TrimSpace(utils.InterfaceToString(action.ActionParams["human_readable_thought"]))
		}
		enterSimplePlanMode(loop, reason)
	case "begin_deep_planning":
		// handled in action handler
	default:
		if shouldEnterDeepPlanModeFromAction(action.ActionType) {
			enterDeepPlanMode(loop, fmt.Sprintf("chose exploration via %s", action.ActionType))
		}
	}
}

func enterSimplePlanMode(loop *reactloops.ReActLoop, reason string) {
	if isSimplePlanMode(loop) {
		return
	}
	loop.Set(PLAN_MODE_KEY, PLAN_MODE_SIMPLE)
	if reason != "" {
		loop.Set(PLAN_MODE_REASON_KEY, reason)
	}
	disableSimpleModeExplorationActions(loop)
	emitPlanModeChosen(loop, PLAN_MODE_SIMPLE, reason)
	log.Infof("plan loop: entered simple direct planning mode")
}

func enterDeepPlanMode(loop *reactloops.ReActLoop, reason string) {
	if isDeepPlanMode(loop) {
		return
	}
	loop.Set(PLAN_MODE_KEY, PLAN_MODE_DEEP)
	if reason != "" {
		loop.Set(PLAN_MODE_REASON_KEY, reason)
	}
	loop.RemoveAction("generate_direct_plan")
	emitPlanModeChosen(loop, PLAN_MODE_DEEP, reason)
	log.Infof("plan loop: entered deep exploration planning mode: %s", reason)
}

func emitPlanModeChosen(loop *reactloops.ReActLoop, mode, reason string) {
	invoker := loop.GetInvoker()
	if invoker == nil {
		return
	}
	msg := fmt.Sprintf("Planning mode: %s.", mode)
	if strings.TrimSpace(reason) != "" {
		msg += " " + strings.TrimSpace(reason)
	}
	invoker.AddToTimeline("plan_mode_chosen", msg)
}

func disableSimpleModeExplorationActions(loop *reactloops.ReActLoop) {
	for _, name := range infoGatheringActions {
		loop.RemoveAction(name)
	}
	loop.RemoveAction("finish_exploration")
	loop.RemoveAction("output_facts")
}

func restoreDeepPlanningActions(loop *reactloops.ReActLoop, r aicommon.AIInvokeRuntime) {
	options := []reactloops.ReActLoopOption{
		finishExploration(r),
		outputFactsAction(r),
		searchKnowledge(r),
		readFileAction(r),
		findFilesAction(r),
		grepTextAction(r),
		webSearchAction(r),
		scanPortAction(r),
		simpleCrawlerAction(r),
	}
	for _, opt := range options {
		opt(loop)
	}
	loop.RemoveAction("generate_direct_plan")
}

func bootstrapFactsFromUserInput(userInput string) string {
	userInput = strings.TrimSpace(userInput)
	if userInput == "" {
		return ""
	}
	return normalizeFactsDocument(fmt.Sprintf("## 用户需求\n\n%s", userInput))
}