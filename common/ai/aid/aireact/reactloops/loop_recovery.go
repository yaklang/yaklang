package reactloops

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

const (
	recoverableFailureLimit       = 3
	recoverableTimelineEntry      = "REACT_RECOVERABLE_ERROR"
	reactRecoveryWarningNodeID    = "react_recovery_warning"
	maxRecoverableErrorTextLength = 4096
)

// recoverableTimelineEvent 描述一条可恢复错误在 Timeline 中的上下文。
// 业务入口只填字段，统一由 recordRecoverableTimelineEvent 负责写入。
type recoverableTimelineEvent struct {
	phase             string
	action            string
	stage             string
	iteration         int
	err               error
	retryCount        int
	sideEffectUnknown bool
	nextStep          string
}

func isLoopControlTermination(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

// noteRecoverableFailure 按 action 和错误内容累计相同失败。
// 达到预算后，调用方可以禁用当前 action，避免同类错误形成热循环。
func (r *ReActLoop) noteRecoverableFailure(actionName string, err error) (attempt int, exhausted bool) {
	if r == nil || err == nil {
		return 1, false
	}
	hash := fnv.New64a()
	_, _ = hash.Write([]byte(actionName))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write([]byte(strings.TrimSpace(err.Error())))
	key := fmt.Sprintf("%s:%x", actionName, hash.Sum64())

	r.recoverableFailureMu.Lock()
	defer r.recoverableFailureMu.Unlock()
	if r.recoverableFailures == nil {
		r.recoverableFailures = make(map[string]int)
	}
	r.recoverableFailures[key]++
	attempt = r.recoverableFailures[key]
	return attempt, attempt >= recoverableFailureLimit
}

// recordRecoverableTimelineEvent 构造并写入标准的可恢复错误 Timeline 条目。
// 返回截断后的错误文本，供需要额外日志或 warning 的调用方复用。
func (r *ReActLoop) recordRecoverableTimelineEvent(event recoverableTimelineEvent) string {
	if r == nil || event.err == nil {
		return ""
	}

	errorText := utils.ShrinkString(
		strings.TrimSpace(event.err.Error()),
		maxRecoverableErrorTextLength,
	)
	lines := []string{
		"phase=" + event.phase,
		"action=" + event.action,
	}
	if event.stage != "" {
		lines = append(lines, "stage="+event.stage)
	}
	if event.iteration > 0 {
		lines = append(lines, fmt.Sprintf("iteration=%d", event.iteration))
	}
	lines = append(lines, "error="+errorText)
	if event.sideEffectUnknown {
		lines = append(lines, "side_effect_started=unknown")
	}
	if event.retryCount > 0 {
		lines = append(lines, fmt.Sprintf("retry_count=%d", event.retryCount))
	}
	if event.nextStep != "" {
		lines = append(lines, "next_step="+event.nextStep)
	}

	if invoker := r.GetInvoker(); invoker != nil {
		invoker.AddToTimeline(recoverableTimelineEntry, strings.Join(lines, "\n"))
	}
	return errorText
}

// recoverActionExecutionFailure 将普通 Action 失败写入 Timeline，
// 并返回 loopActionsRecover 交给 exec 继续下一轮。
func (r *ReActLoop) recoverActionExecutionFailure(
	operator *LoopActionHandlerOperator,
	call LoopCall,
	iteration int,
	err error,
) loopActionsResult {
	if err == nil {
		err = utils.Error("action execution failed")
	}
	actionName := "<unknown>"
	if call.Action != nil && strings.TrimSpace(call.Action.Name()) != "" {
		actionName = call.Action.Name()
	}

	attempt, exhausted := r.noteRecoverableFailure(actionName, err)
	nextStep := "Inspect the current state before retrying. Change the action, parameters, or target when the same failure is likely to repeat."
	if exhausted {
		nextStep = "The same failure repeated too many times. Do not retry this action with the same parameters; inspect the state and choose a different action or target."
		if operator != nil && actionName != "<unknown>" {
			operator.RemoveNextAction(actionName)
		}
	}

	errorText := r.recordRecoverableTimelineEvent(recoverableTimelineEvent{
		phase:             "execute_ai_output",
		action:            actionName,
		iteration:         iteration,
		err:               err,
		retryCount:        attempt,
		sideEffectUnknown: true,
		nextStep:          nextStep,
	})
	if errorText != "" {
		r.emitRecoveryWarning(fmt.Sprintf(
			"Action %s failed during execution and was converted into a recoverable error (attempt %d): %s",
			actionName,
			attempt,
			errorText,
		))
	}
	return loopActionsResult{
		operator:       operator,
		result:         loopActionsRecover,
		recoveredError: err,
	}
}

// recordRecoverableTurnError 记录单轮执行中可恢复的错误。
// 该函数只负责写入 Timeline，实际 continue 由 exec 统一决定。
func (r *ReActLoop) recordRecoverableTurnError(iteration int, phase, action string, err error) {
	if err == nil {
		return
	}
	attempt, exhausted := r.noteRecoverableFailure(action, err)
	nextStep := "Continue from the current Timeline state. Change the next action or parameters when the same failure is likely to repeat."
	if exhausted {
		nextStep = "The same failure repeated too many times. Avoid repeating the same call; inspect the state and choose a different path."
	}

	errorText := r.recordRecoverableTimelineEvent(recoverableTimelineEvent{
		phase:      phase,
		action:     action,
		iteration:  iteration,
		err:        err,
		retryCount: attempt,
		nextStep:   nextStep,
	})
	if errorText != "" {
		r.emitRecoveryWarning(fmt.Sprintf(
			"Loop stage %s could not be completed and was converted into a recoverable error (attempt %d): %s",
			phase,
			attempt,
			errorText,
		))
	}
}

// recordContextCompressionFailure 记录 Prompt 构建阶段的压缩失败。
// 压缩没有修改 Timeline 时，提示模型下一轮重新判断是否还需要压缩。
func (r *ReActLoop) recordContextCompressionFailure(stage string, err error) {
	if err == nil {
		err = utils.Error("context compression did not complete")
	}
	r.recordRecoverableTimelineEvent(recoverableTimelineEvent{
		phase:    "generate_prompt",
		action:   aicommon.ContextCompressionAction,
		stage:    stage,
		err:      err,
		nextStep: "The timeline was not modified by this compression attempt. Re-evaluate whether compression is still needed before retrying.",
	})
}

// emitRecoveryWarning 同时写日志和普通流 warning。
// 这里不能使用系统流，否则前端不会向用户展示。
func (r *ReActLoop) emitRecoveryWarning(message string) {
	if r == nil {
		return
	}
	log.Warnf("%s", message)
	emitter := r.GetEmitter()
	if emitter == nil {
		return
	}
	taskIndex := ""
	if task := r.GetCurrentTask(); task != nil {
		taskIndex = task.GetId()
	}
	if _, err := emitter.EmitDefaultStreamEvent(
		reactRecoveryWarningNodeID,
		strings.NewReader(message),
		taskIndex,
	); err != nil {
		log.Warnf("emit recoverable action warning: %v", err)
	}
}

// warnOptionalOutputFailure 处理展示类回调失败。
// 这类错误不能影响已经解析出的 canonical AI 响应和 Action。
func (r *ReActLoop) warnOptionalOutputFailure(stage string, err error) {
	if err == nil {
		return
	}
	r.emitRecoveryWarning(fmt.Sprintf(
		"Optional AI output stage %q failed; canonical response processing continues: %v",
		stage,
		err,
	))
}

// resetRecoverableLoopFailureCounters 在成功执行 Action 后清空连续失败预算。
func resetRecoverableLoopFailureCounters(r *ReActLoop) {
	if r == nil {
		return
	}
	r.recoverableFailureMu.Lock()
	r.recoverableFailures = nil
	r.recoverableFailureMu.Unlock()
}
