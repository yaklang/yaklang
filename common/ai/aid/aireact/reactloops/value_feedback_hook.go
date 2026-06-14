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
// 触发条件 (专注高价值数据, 避免逐轮刷量):
//   - 整循环结束 (isDone=true): trigger=loop_end —— 整段轨迹的终态信号, 始终提交.
//   - 每轮结束 (isDone=false): trigger=iteration_end —— 默认不提交; 仅当本轮真正
//     执行了工具 (客观成败=高价值) 时才补一条. 纯思考/直接回答等低价值迭代不触发,
//     其内容会被 loop_end 的完整轨迹覆盖.
//
// 全程 recover + 非阻塞投递, 绝不影响主循环.
//
// 关键词: 价值评估埋点, reactloops onPostIteration, SubmitValueFeedback,
//        iteration_end 高价值门控, loop_end, FocusMode ActionRecord timeline diff

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

// submitValueFeedbackRecord 在 loop_end 始终提交, 在 iteration_end 仅当本轮执行了
// 工具 (高价值客观信号) 时才提交, 减弱逐轮提交带来的低价值噪声与额外开销.
func (r *ReActLoop) submitValueFeedbackRecord(iteration int, task aicommon.AIStatefulTask, isDone bool, reason any) {
	if isDone {
		r.submitValueFeedbackWithTrigger(aicommon.ValueFeedbackTriggerLoopEnd, task)
		return
	}
	if r.iterationExecutedTool(iteration) {
		r.submitValueFeedbackWithTrigger(aicommon.ValueFeedbackTriggerIterationEnd, task)
	}
}

// iterationExecutedTool 判断指定迭代是否真正执行了工具 (ActionRecord.ToolName 非空).
// 工具执行的客观成败是高价值训练信号; 纯思考 / 直接回答 / 规划等迭代不在此触发,
// 留给 loop_end 汇总, 从而专注高价值数据.
func (r *ReActLoop) iterationExecutedTool(iteration int) bool {
	for _, a := range r.GetAllExistedActionRecord() {
		if a == nil {
			continue
		}
		if a.IterationIndex == iteration && a.ToolName != "" {
			return true
		}
	}
	return false
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
