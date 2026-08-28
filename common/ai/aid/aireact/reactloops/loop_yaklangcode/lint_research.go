package loop_yaklangcode

import (
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
)

const (
	loopVarLintResearchDone = "lint_research_done"
)

// yaklangCodeMutatingActions must not run while lint is failing until a research action succeeds.
var yaklangCodeMutatingActions = map[string]struct{}{
	"write_code":  {},
	"modify_code": {},
	"insert_code": {},
	"delete_code": {},
}

// yaklangEarlyExitActions are blocked until code passes lint (and self-test when applicable).
var yaklangEarlyExitActions = map[string]struct{}{
	"finish":          {},
	"directly_answer": {},
}

func markYaklangLintResearchDone(loop *reactloops.ReActLoop) {
	if loop == nil {
		return
	}
	loop.Set(loopVarLintResearchDone, "true")
}

func markYaklangLintResearchNeeded(loop *reactloops.ReActLoop) {
	if loop == nil {
		return
	}
	loop.Set(loopVarLintResearchDone, "false")
}

func hasYaklangLintResearchDone(loop interface{ Get(string) string }) bool {
	if loop == nil {
		return false
	}
	return loop.Get(loopVarLintResearchDone) == "true"
}

// needsYaklangLintResearchGate is true when static analysis failed and the model
// has not yet completed a grep/yakdoc research step in this lint-failure cycle.
func needsYaklangLintResearchGate(loop interface{ Get(string) string }) bool {
	if loop == nil {
		return false
	}
	return hasBlockingLintErrors(loop) && !hasYaklangLintResearchDone(loop)
}

type yaklangLoopBox struct {
	loop *reactloops.ReActLoop
}

// newYaklangLoopActionFilter gates actions to reduce empty rounds and whack-a-mole patches:
// - hide finish/directly_answer until full_code exists and lint/self-test pass;
// - after lint failure, hide mutating actions until grep/yakdoc succeeds.
// Lint 失败后始终用 modify_code 小步修复，不再强制 write_code 整段重写。
func newYaklangLoopActionFilter(box *yaklangLoopBox) func(action *reactloops.LoopAction) bool {
	return func(action *reactloops.LoopAction) bool {
		if action == nil || box == nil || box.loop == nil {
			return true
		}
		loop := box.loop
		actionType := action.ActionType

		if needsBlockYaklangEarlyExit(loop) {
			if _, early := yaklangEarlyExitActions[actionType]; early {
				return false
			}
		}

		if needsYaklangLintResearchGate(loop) {
			if _, mutating := yaklangCodeMutatingActions[actionType]; mutating {
				return false
			}
		}
		return true
	}
}

// newYaklangLintResearchActionFilter is an alias kept for tests.
func newYaklangLintResearchActionFilter(box *yaklangLoopBox) func(action *reactloops.LoopAction) bool {
	return newYaklangLoopActionFilter(box)
}

func wrapInitTaskBindLoopBox(
	box *yaklangLoopBox,
	inner func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask, operator *reactloops.InitTaskOperator),
) func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask, operator *reactloops.InitTaskOperator) {
	return func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask, operator *reactloops.InitTaskOperator) {
		if box != nil {
			box.loop = loop
		}
		inner(loop, task, operator)
	}
}

func isYaklangCodeMutatingAction(actionType string) bool {
	_, ok := yaklangCodeMutatingActions[strings.TrimSpace(actionType)]
	return ok
}

// needsBlockYaklangEarlyExit blocks finish/directly_answer until there is code
// that passes lint (and self-test when applicable).
func needsBlockYaklangEarlyExit(loop interface{ Get(string) string }) bool {
	if loop == nil {
		return true
	}
	if strings.TrimSpace(loop.Get("full_code")) == "" {
		return true
	}
	if hasBlockingLintErrors(loop) {
		return true
	}
	if hasFailedSelfTest(loop) {
		return true
	}
	return false
}
