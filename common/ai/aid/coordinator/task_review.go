package coordinator

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/coordinator_legacy"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// Preserve automatic review preferences without reviving a separate AI task-
// review loop. Only manual (or an unknown policy) requires a human task endpoint.
func usesManualTaskReview(policy aicommon.AgreePolicyType) bool {
	switch policy {
	case aicommon.AgreePolicyYOLO, aicommon.AgreePolicyAuto, aicommon.AgreePolicyAI, aicommon.AgreePolicyAIAuto:
		return false
	default:
		return true
	}
}

// Only the existing selector DTO is reused from the compatibility catalog.
// No legacy coordinator, task execution or replanning runtime is instantiated.
func (s *Session) ReviewExecutionTask(ctx context.Context, a Attempt) error {
	ep := s.Epm.CreateEndpointWithEventType(schema.EVENT_TYPE_TASK_REVIEW_REQUIRE)
	ep.SetDefaultSuggestionContinue()
	s.mu.Lock()
	if s.taskReviewEndpoints == nil {
		s.taskReviewEndpoints = map[string]bool{}
	}
	s.taskReviewEndpoints[ep.GetId()] = true
	s.mu.Unlock()
	state := s.Snapshot()
	payload := map[string]any{"id": ep.GetId(), "selectors": coordinator_legacy.TaskReviewSuggestions, "task": map[string]any{"task_id": a.Task.ID, "index": a.Task.Index, "name": a.Task.Name, "goal": a.Task.Goal, "attempt_id": a.ID, "summary": a.Result.Summary, "short_summary": a.Result.Summary, "long_summary": a.Result.Summary}, "short_summary": a.Result.Summary, "long_summary": a.Result.Summary, "result": a.Result, "progress": map[string]any{"phase": "NotCompleted", "total_tasks": len(state.Attempts)}, "pending_tasks": []any{}}
	ep.SetReviewMaterials(payload)
	if err := s.SubmitCheckpointRequest(ep.GetCheckpoint(), payload); err != nil {
		return fmt.Errorf("persist task review checkpoint: %w", err)
	}
	s.EmitInteractiveJSON(ep.GetId(), schema.EVENT_TYPE_TASK_REVIEW_REQUIRE, "review-require", payload)
	s.DoWaitAgreeWithPolicy(ctx, aicommon.AgreePolicyManual, ep)
	params := ep.GetParams()
	s.ReleaseInteractiveEvent(ep.GetId(), params)
	if err := ctx.Err(); err != nil {
		return err
	}
	if params == nil {
		return fmt.Errorf("task review returned no decision")
	}
	s.CallAfterReview(ep.GetSeq(), "审查当前任务的实际执行结果", params)
	suggestion := params.GetString("suggestion")
	reason := params.GetString("extra_prompt")
	if reason == "" {
		reason = params.GetString("reason")
	}
	if suggestion == "continue" {
		return s.controller.applyReview(a.Task.ID, a.ID, "accept", "用户通过当前任务审核。", true)
	}
	if suggestion == "cancel" || suggestion == "end" || suggestion == "close" {
		if reason == "" {
			reason = "用户明确停止此任务。"
		}
		if err := s.controller.applyReview(a.Task.ID, a.ID, "cancel", reason, true); err != nil {
			return err
		}
		return s.controller.CancelTasks([]string{a.Task.ID}, reason)
	}
	if suggestion != "deeply_think" && suggestion != "inaccurate" && suggestion != "adjust_plan" {
		return fmt.Errorf("unknown task review decision %q", suggestion)
	}
	if reason == "" {
		reason = "用户要求进一步核对当前任务（" + suggestion + "），未提供额外原因。"
	}
	if err := s.controller.applyReview(a.Task.ID, a.ID, "reject", reason, true); err != nil {
		return err
	}
	if patch, exists := params["tasks_patch"]; exists {
		_, err := s.controller.ModifyPlan(ctx, map[string]any{"tasks_patch": patch})
		return err
	}
	if suggestion == "inaccurate" {
		_, err := s.controller.ModifyPlan(ctx, map[string]any{"tasks_patch": []any{map[string]any{"operator": "update", "task_id": a.Task.ID, "changes": map[string]any{"goal": a.Task.Goal + "\n用户审核反馈：" + reason + "\n重新验证事实、工具参数和证据来源，不沿用未验收结论。"}}}})
		return err
	}
	s.recordReviewRevision(ep.GetId(), "用户要求加深或调整任务："+reason, params)
	return nil
}

// User ingress remains the existing Timeline path. A task's normal manual
// approval is handled by its manager, so it must not force a model re-review.
func (s *Session) notifyUserInput(e *ypb.AIInputEvent) {
	if e.IsInteractiveMessage {
		s.mu.Lock()
		taskReview := s.taskReviewEndpoints[e.InteractiveId] || s.planReviewEndpoints[e.InteractiveId]
		s.mu.Unlock()
		if taskReview {
			return
		}
	}
	s.controller.Wake()
}

func (s *Session) recordReviewRevision(id, summary string, params aitool.InvokeParams) {
	s.mu.Lock()
	if s.recordedReviewFeedback == nil {
		s.recordedReviewFeedback = map[string]bool{}
	}
	if s.recordedReviewFeedback[id] {
		s.mu.Unlock()
		return
	}
	s.recordedReviewFeedback[id] = true
	s.mu.Unlock()
	reason := params.GetString("extra_prompt")
	existing := false
	for _, entry := range s.GetUserInputHistory() {
		if reason != "" && strings.Contains(entry.UserInput, reason) && strings.Contains(entry.UserInput, params.GetString("suggestion")) {
			existing = true
		}
	}
	if !existing {
		_, _ = s.AppendUserInputHistory(summary, time.Now())
	}
	c := s.controller
	c.mu.Lock()
	// Known review endpoints bypass the generic ingress wake. This one message
	// belongs to this endpoint only; never overwrite an unrelated user request.
	c.state.UserRevision++
	c.enqueueLocked("user_message", "", 0, summary, []string{"review:" + id}, true)
	c.mu.Unlock()
	c.publish()
}

// Plan rejection keeps PLAN editable; closing/stopping has a distinct outcome.
func (s *Session) planRevisionFeedback(id string, params aitool.InvokeParams) error {
	if params.GetString("suggestion") == "close" || params.GetString("suggestion") == "cancel" {
		s.cancel()
		return context.Canceled
	}
	reason := params.GetString("extra_prompt")
	if reason == "" {
		reason = params.GetString("reason")
	}
	summary := "用户要求修订当前计划（" + params.GetString("suggestion") + "），请检查并完善方案。"
	if reason != "" {
		summary += " 用户说明：" + reason
	}
	s.recordReviewRevision(id, summary, params)
	return fmt.Errorf("plan revision requested: %s", params)
}
