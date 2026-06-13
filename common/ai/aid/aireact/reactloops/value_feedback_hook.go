package reactloops

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

// value_feedback_hook.go 在 reactloops 这一最底层基础设施上自动注入价值评估埋点.
//
// 每个通过 NewReActLoop 创建的 loop 都会在 onPostIteration 回调里组装一条
// ValueFeedbackRecord (FocusMode / ActionRecord / 满意度 / timeline diff), 然后
// 调 aicommon.SubmitValueFeedback 交给已注册的 aive 实现 (未注册时安全 no-op).
// reactloops 不直接依赖 aive, 只走 aicommon 注册缝, 因此无 import 环.
//
// 触发条件:
//   - 每轮结束 (isDone=false): trigger=iteration_end
//   - 整循环结束 (isDone=true): trigger=loop_end
//
// 全程 recover + 非阻塞投递, 绝不影响主循环.
//
// 关键词: 价值评估埋点, reactloops onPostIteration, SubmitValueFeedback,
//        iteration_end, loop_end, FocusMode ActionRecord timeline diff

// buildValueFeedbackPostIteration 返回一个注入到所有 loop 的 onPostIteration 钩子.
func buildValueFeedbackPostIteration() func(loop *ReActLoop, iteration int, task aicommon.AIStatefulTask, isDone bool, reason any, operator *OnPostIterationOperator) {
	return func(loop *ReActLoop, iteration int, task aicommon.AIStatefulTask, isDone bool, reason any, operator *OnPostIterationOperator) {
		defer func() {
			if r := recover(); r != nil {
				// 价值评估埋点失败绝不能影响主循环.
				return
			}
		}()
		loop.submitValueFeedbackRecord(iteration, task, isDone, reason)
	}
}

// submitValueFeedbackRecord 在 iteration_end / loop_end 组装并提交记录.
func (r *ReActLoop) submitValueFeedbackRecord(iteration int, task aicommon.AIStatefulTask, isDone bool, reason any) {
	trigger := aicommon.ValueFeedbackTriggerIterationEnd
	if isDone {
		trigger = aicommon.ValueFeedbackTriggerLoopEnd
	}
	r.submitValueFeedbackWithTrigger(trigger, task)
}

// submitValueFeedbackSignal 在过程信号节点 (SPIN / 反思 / verification) 作为额外
// 触发提交一条价值评估记录. 全程 recover + 非阻塞, 绝不影响主循环.
func (r *ReActLoop) submitValueFeedbackSignal(trigger string) {
	defer func() {
		if rec := recover(); rec != nil {
			return
		}
	}()
	r.submitValueFeedbackWithTrigger(trigger, r.GetCurrentTask())
}

// submitValueFeedbackWithTrigger 组装并提交一条价值评估记录 (统一装配逻辑).
func (r *ReActLoop) submitValueFeedbackWithTrigger(trigger string, task aicommon.AIStatefulTask) {
	cfg, ok := r.config.(*aicommon.Config)
	if !ok || cfg == nil {
		return
	}

	record := &aicommon.ValueFeedbackRecord{
		MainModel: aicommon.ModelEndpoint{
			ModelName:  cfg.AiModelName,
			ServerName: cfg.AiServerName,
		},
		FocusMode:        r.loopName,
		TriggerCondition: trigger,
		ExecutionPolicy:  cfg.AgreePolicy,
		TimelineDiff:     r.GetTimelineDiffWithoutUpdate(),
		SessionID:        cfg.PersistentSessionId,
	}

	actions := r.GetAllExistedActionRecord()
	for _, a := range actions {
		if a == nil {
			continue
		}
		record.Actions = append(record.Actions, aicommon.ValueFeedbackAction{
			ActionType:     a.ActionType,
			ActionName:     a.ActionName,
			ToolName:       a.ToolName,
			IterationIndex: a.IterationIndex,
		})
	}
	record.WhatHappenedSummary = summarizeValueFeedbackActions(actions)

	if sat := r.GetLastSatisfactionRecordFull(); sat != nil {
		finished := sat.Satisfactory
		record.Outcome = &aicommon.ValueFeedbackOutcome{
			TaskFinished: &finished,
			Detail:       sat.Reason,
		}
	}

	if !isNilTask(task) {
		record.TaskID = task.GetId()
	}

	aicommon.SubmitValueFeedback(cfg, record)
}

// summarizeValueFeedbackActions 把动作序列压成 "a -> b -> c(tool)" 形式的摘要.
func summarizeValueFeedbackActions(actions []*ActionRecord) string {
	if len(actions) == 0 {
		return ""
	}
	parts := make([]string, 0, len(actions))
	for _, a := range actions {
		if a == nil {
			continue
		}
		seg := a.ActionType
		if a.ToolName != "" {
			seg = fmt.Sprintf("%s(%s)", a.ActionType, a.ToolName)
		}
		parts = append(parts, seg)
	}
	return strings.Join(parts, " -> ")
}

func isNilTask(task aicommon.AIStatefulTask) bool {
	return task == nil
}
