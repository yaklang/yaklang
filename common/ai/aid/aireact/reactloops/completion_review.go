package reactloops

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

const completionReviewPrompt = `[COMPLETION REVIEW REQUIRED]

TODO 空列表不是完成证据。结束前对照 CURRENT-TASK 和已有 Observation 做一次完成审计：
1. 用户的每个验收目标是否都有实际产出和验证证据；不得把读过、分析过或单次阴性当作整个目标已完成。
2. 近期工具结果、timeline、evidence 和已投递答复是否出现范围内、会影响验收或后续动作、却从未 add 的具体对象或替代实验。有则先用新 id add，再执行；散文里的“次要发现”不算维护 TODO。
3. 终态项的 reason/refs 是否支持原目标，正文是否还承诺未做的下一步；deferred 的外部阻塞是否真实、恢复条件是否已满足。发现缺口时用新 id 建立延续 TODO，不得重写历史或缩小原目标来清空列表。

有缺口就选择能产生证据的正常动作，并携带 todo_delta；检查点不要求额外工具调用、固定轮数或无关扩展。
确无剩余工作时，finish 必须携带 completion_review：goal_evidence 逐项关联验收目标与 Observation/产物，discovery_audit 说明新发现如何进入 TODO 或为何依据用户范围排除，closure_audit 核对终态证据及真实外部阻塞。不能只写“已完成”“无 TODO”或复述本检查表。
“主产出已交付”“深度优先”“置信已够”“性价比低”都不能替代审计。仍有外部阻塞时明确报告未完成，不能宣称全部成功。
检查只对当前工作状态有效；后续动作、新证据、用户输入或 TODO 变更之后，finish 必须重新接受检查。`

func completionReviewOption() aitool.ToolOption {
	return aitool.WithStructParam("completion_review", []aitool.PropertyOption{
		aitool.WithParam_Description("Only for finish after the host completion checkpoint. Evidence-backed audit of the current work state, not a completion checkbox. If the audit finds executable gaps, continue work instead of finish."),
	},
		aitool.WithStringParam("goal_evidence", aitool.WithParam_Required(true), aitool.WithParam_Description("Map each current-task acceptance goal to actual observations or deliverables; state any externally blocked, unfinished goal.")),
		aitool.WithStringParam("discovery_audit", aitool.WithParam_Required(true), aitool.WithParam_Description("Account for relevant objects found in tool results, evidence and answers, including their TODO IDs or concrete user-scope exclusion. Do not infer coverage from an empty list.")),
		aitool.WithStringParam("closure_audit", aitool.WithParam_Required(true), aitool.WithParam_Description("Check closed TODO reasons/refs against their original scope, unresolved next steps and deferred recovery conditions. Explain the evidence, not just the terminal status.")),
	)
}

// completionStateKey deliberately excludes finish attempts, model thoughts and
// checkpoint timeline notes: they are not work and must not cause an endless
// audit loop. Every completed non-finish action invalidates the review separately.
// Canonical snapshots catch TODO changes on finish itself and external updates;
// evidence and user input also catch changes outside the normal action handler.
func (r *ReActLoop) completionStateKey() [32]byte {
	task := r.GetCurrentTask()
	input := ""
	if task != nil {
		input = task.GetUserInput()
	}
	open, current, closed := r.config.SnapshotCanonicalTodos(aicommon.BuildVerificationTodoScope(task))
	raw, _ := json.Marshal([]any{input, open, current, closed, r.config.GetSessionEvidenceRendered()})
	return sha256.Sum256(raw)
}

func (r *ReActLoop) invalidateCompletionReview(task aicommon.AIStatefulTask) {
	key := todoCheckpointScopeKey(aicommon.BuildVerificationTodoScope(task))
	r.todoCheckpointMu.Lock()
	delete(r.completionReviewStates, key)
	r.todoCheckpointMu.Unlock()
}

func validateCompletionReviewFields(action *aicommon.Action) error {
	if action == nil {
		return fmt.Errorf("finish requires completion_review after the completion checkpoint")
	}
	review := action.GetInvokeParams("completion_review")
	for _, field := range []string{"goal_evidence", "discovery_audit", "closure_audit"} {
		value, ok := review[field].(string)
		if !ok || strings.TrimSpace(value) == "" {
			return fmt.Errorf("finish requires a non-empty completion_review.%s; audit existing observations or add and execute missing work", field)
		}
	}
	return nil
}

// Once the checkpoint has been delivered, malformed reviews use the existing
// bounded action-validation retry path instead of spinning the ReAct loop.
func verifyCompletionReviewAction(r *ReActLoop, action *aicommon.Action) error {
	if action != nil && action.GetString("_todo_delta_error") != "" {
		// Invalid maintenance must not repeatedly invalidate the checkpoint and
		// evade the bounded validation retries with no work-budget consumption.
		return fmt.Errorf("cannot finish with invalid TODO maintenance: %s", action.GetString("_todo_delta_error"))
	}
	key := todoCheckpointScopeKey(aicommon.BuildVerificationTodoScope(r.GetCurrentTask()))
	state := r.completionStateKey()
	r.todoCheckpointMu.Lock()
	previous, reviewed := r.completionReviewStates[key]
	r.todoCheckpointMu.Unlock()
	if reviewed && previous == state {
		return validateCompletionReviewFields(action)
	}
	return nil
}

// checkCompletionReview is a host-enforced opportunity to discover untracked
// work even when no TODO was ever created. It validates audit structure, not the
// truth of model claims; evidence quality still requires semantic review.
func (r *ReActLoop) checkCompletionReview(action *aicommon.Action) (string, bool) {
	key := todoCheckpointScopeKey(aicommon.BuildVerificationTodoScope(r.GetCurrentTask()))
	state := r.completionStateKey()
	r.todoCheckpointMu.Lock()
	previous, reviewed := r.completionReviewStates[key]
	if r.completionReviewStates == nil {
		r.completionReviewStates = make(map[string][32]byte)
	}
	r.completionReviewStates[key] = state
	r.todoCheckpointMu.Unlock()
	if !reviewed || previous != state {
		return completionReviewPrompt, false
	}
	if err := validateCompletionReviewFields(action); err != nil {
		return err.Error() + "\n" + completionReviewPrompt, false
	}
	return "", true
}
