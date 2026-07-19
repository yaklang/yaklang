package reactloops

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

const valueFeedbackRecentActionLimit = 32

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
		r.submitValueFeedbackWithTrigger(aicommon.ValueFeedbackTriggerLoopEnd, task, iteration)
		return
	}
	if r.iterationExecutedTool(iteration) {
		r.submitValueFeedbackWithTrigger(aicommon.ValueFeedbackTriggerIterationEnd, task, iteration)
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
	r.submitValueFeedbackWithTrigger(trigger, r.GetCurrentTask(), r.GetCurrentIterationIndex())
}

// submitValueFeedbackWithTrigger 组装并提交一条价值评估记录 (统一装配逻辑).
//
// iteration 是本条记录对应的轮次序号; 核心 trace 使用最近 Timeline 投影.
// Value feedback 是高频轻模型调用，携带完整会话会使输入随会话长度线性增长。
func (r *ReActLoop) submitValueFeedbackWithTrigger(trigger string, task aicommon.AIStatefulTask, iteration int) {
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
		SessionID:        cfg.PersistentSessionId,
		IterationIndex:   iteration,
	}
	// 核心 trace: 只给轻模型最近窗口。原始 Timeline 本身不受影响。
	if cfg.Timeline != nil {
		record.TimelineDump = cfg.Timeline.DumpRecentForPrompt(aicommon.ValueFeedbackRecentTimelineTokens)
	} else {
		record.TimelineDump = aicommon.ShrinkTextBlockByTokens(
			r.GetTimelineDiffWithoutUpdate(),
			aicommon.ValueFeedbackRecentTimelineTokens,
		)
	}

	actions := recentValueFeedbackActions(r.GetAllExistedActionRecord())
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
		record.UserQuery = task.GetUserInput()
	}

	aicommon.SubmitValueFeedback(cfg, record)
}

// SubmitRiskFeedback 在 AI 报出漏洞 (risk) 之后提交一条 risk_feedback 价值评估记录,
// 交由价值评估小模型判定该漏洞是否为误报 (AI 自判路径, source=model_judge). 记录携带
// 完整 timeline dump 作为核心 trace, 以便后端复盘该误报判定的上下文.
//
// 人工确认路径 (未来接入, 本次不实现, 也不改任何前端): 当前端在漏洞报出后提供
// "是真漏洞 / 是误报" 的确认交互时, 应在拿到用户判定后同样组装一条
// aicommon.ValueFeedbackRecord, 设 RiskFeedback.Source = aicommon.RiskFeedbackSourceHuman
// 且 IsFalsePositive 为用户的明确判定 (true/false), 再调 aicommon.SubmitValueFeedback
// 提交. 人工确认是最高价值的误报信号, 后端可据 source 区分 AI 弱标签与人工终判.
//
// 全程 recover + 非阻塞投递, 绝不影响主循环.
func (r *ReActLoop) SubmitRiskFeedback(riskIDs []string, riskType, severity string) {
	defer func() {
		if rec := recover(); rec != nil {
			return
		}
	}()
	if len(riskIDs) == 0 {
		return
	}
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
		TriggerCondition: aicommon.ValueFeedbackTriggerRiskFeedback,
		ExecutionPolicy:  cfg.AgreePolicy,
		SessionID:        cfg.PersistentSessionId,
		IterationIndex:   r.GetCurrentIterationIndex(),
		RiskFeedback: &aicommon.ValueFeedbackRiskFeedback{
			RiskIDs:  riskIDs,
			RiskType: riskType,
			Severity: severity,
			Source:   aicommon.RiskFeedbackSourceModelJudge,
			// IsFalsePositive 留空 (nil): AI 自判路径由小模型在价值评估请求里回填.
		},
	}
	if cfg.Timeline != nil {
		record.TimelineDump = cfg.Timeline.DumpRecentForPrompt(aicommon.ValueFeedbackRecentTimelineTokens)
	}
	if task := r.GetCurrentTask(); !isNilTask(task) {
		record.TaskID = task.GetId()
		record.UserQuery = task.GetUserInput()
	}
	record.WhatHappenedSummary = summarizeValueFeedbackActions(recentValueFeedbackActions(r.GetAllExistedActionRecord()))

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

func recentValueFeedbackActions(actions []*ActionRecord) []*ActionRecord {
	if len(actions) <= valueFeedbackRecentActionLimit {
		return actions
	}
	return actions[len(actions)-valueFeedbackRecentActionLimit:]
}

func isNilTask(task aicommon.AIStatefulTask) bool {
	return task == nil
}
