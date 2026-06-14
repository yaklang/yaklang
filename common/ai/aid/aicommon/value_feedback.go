package aicommon

import (
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
)

// value_feedback.go 是"价值评估"采集的瘦注册缝 (零额外重依赖).
//
// 设计: aicommon 只放纯数据结构 + 注册函数; 真正的价值评估实现 (用 liteforge
// 把上下文打包成一次发往 memfit-light-free 的请求) 放在 aive 包, 由 aive 在
// init() 里调用 RegisterValueFeedbackSubmitter 注册进来. reactloops / aid 等
// 底层只调用 SubmitValueFeedback, 不直接依赖 aive, 从而避免 import 环.
//
// 关键词: ValueFeedbackRecord, RegisterValueFeedbackSubmitter, SubmitValueFeedback,
//        价值评估, ModelEndpoint, 价值评估注册缝

// ModelEndpoint 描述一个模型端点. 价值评估记录只关心模型名称 (不提交 URL),
// 额外保留 provider/service 类型名作为轻量上下文.
type ModelEndpoint struct {
	// ModelName 是模型名称.
	ModelName string `json:"model_name"`
	// ServerName 是 provider/service 类型名 (如 openai / deepseek).
	ServerName string `json:"server_name"`
}

// 价值评估触发条件枚举.
const (
	ValueFeedbackTriggerIterationEnd   = "iteration_end"
	ValueFeedbackTriggerLoopEnd        = "loop_end"
	ValueFeedbackTriggerReviewDecision = "review_decision"
	ValueFeedbackTriggerSpinDetected   = "spin_detected"
	ValueFeedbackTriggerSelfReflection = "self_reflection"
	ValueFeedbackTriggerVerification   = "verification"
)

// ValueFeedbackAction 是一次动作的轻量表示 (避免 aicommon 反向依赖 reactloops
// 的 ActionRecord). 由上层把 ActionRecord 转换成该结构填入.
type ValueFeedbackAction struct {
	ActionType     string `json:"action_type"`
	ActionName     string `json:"action_name"`
	ToolName       string `json:"tool_name,omitempty"`
	IterationIndex int    `json:"iteration_index"`
}

// approval.source 枚举: 区分"谁"做出的决定, 与 execution_policy (配置策略) 解耦.
// 例如 YOLO 策略下某个高风险动作仍可能由人工审批 (source=human), 判断人工反馈
// 的依据是 source=human, 而非 execution_policy.
const (
	ApprovalSourceHuman           = "human"            // 真人工裁决
	ApprovalSourcePolicy          = "policy"            // 策略自动放行 (yolo/auto)
	ApprovalSourceModelJudge      = "model_judge"       // AI 风控/助手判定
	ApprovalSourceRule            = "rule"              // 规则判定
	ApprovalSourceTimeoutFallback = "timeout_fallback"  // 超时兜底放行
)

// approval.decision 枚举: 描述审批的客观结果.
const (
	ApprovalDecisionApprove        = "approve"           // 同意 (未改参数)
	ApprovalDecisionApproveWithEdit = "approve_with_edit" // 同意但修改了参数
	ApprovalDecisionReject         = "reject"            // 拒绝
	ApprovalDecisionCancel         = "cancel"            // 取消 (无最终参数)
	ApprovalDecisionTimeout        = "timeout"           // 超时
	ApprovalDecisionNotRequired    = "not_required"      // 无需审批 (策略自动放行)
)

// ValueFeedbackApproval 描述一次审批决策 (记录事实, 不预先下训练标签).
// 关键设计: execution_policy (配置策略) 与 approval.source (本次决定来源) 分离.
type ValueFeedbackApproval struct {
	// Required 表示本次是否真的需要人工/模型介入.
	Required bool `json:"required"`
	// Source 取 human/policy/model_judge/rule/timeout_fallback (谁做的决定).
	Source string `json:"source"`
	// Decision 取 approve/approve_with_edit/reject/cancel/timeout/not_required.
	Decision string `json:"decision"`
	// Suggestion 是审批响应里用户/AI 选择的原始建议项 (如 continue / wrong_tool /
	// wrong_params / direct_answer / incomplete / adjust_plan / cancel), 保留原始
	// 取值以便下游不丢失语义 (Decision 是它归一化后的结论).
	Suggestion string `json:"suggestion,omitempty"`
	// Reason 是机器可读的决定原因 (如 auto_approve_by_yolo_policy).
	Reason string `json:"reason,omitempty"`
	// Question 是审批问题摘要.
	Question string `json:"question,omitempty"`
	// OriginalValue 是审批前的原始提议参数.
	OriginalValue map[string]any `json:"original_value,omitempty"`
	// FinalValue 是审批后的最终参数.
	FinalValue map[string]any `json:"final_value,omitempty"`
	// Changed 表示参数是否在审批中被实际修改 (original 与 final 比对得出).
	Changed bool `json:"changed"`
	// Comment 是审批人留下的备注 (如有).
	Comment string `json:"comment,omitempty"`
	// ReviewerIDHash 是审批人不可逆指纹 (当前管线暂无审批人身份, 一般为空).
	ReviewerIDHash string `json:"reviewer_id_hash,omitempty"`
	// ReviewLatencyMs 是从发起审批到做出决定的毫秒时延.
	ReviewLatencyMs int64 `json:"review_latency_ms,omitempty"`
	// DecidedAtMs 是做出决定的毫秒时间戳.
	DecidedAtMs int64 `json:"decided_at_ms,omitempty"`
}

// ValueFeedbackOutcome 描述可被程序确定性判定的客观结局.
type ValueFeedbackOutcome struct {
	ToolSuccess  *bool  `json:"tool_success,omitempty"`
	RiskSaved    *bool  `json:"risk_saved,omitempty"`
	CompilePass  *bool  `json:"compile_pass,omitempty"`
	TaskFinished *bool  `json:"task_finished,omitempty"`
	Detail       string `json:"detail,omitempty"`
}

// ValueFeedbackRecord 是一次价值评估提交的完整上下文 (纯数据).
//
// ID 与 Signature 在提交时由 aive 计算: ID 唯一标识本次提交; Signature 对稳定
// 字段 (主模型 URL+名称 / 小模型 / focus mode / trigger / what_happened /
// timeline_dump) 做 SHA256, 用于去重与完整性校验.
type ValueFeedbackRecord struct {
	ID        string `json:"id"`
	Signature string `json:"signature"`

	MainModel  ModelEndpoint `json:"main_model"`
	SmallModel ModelEndpoint `json:"small_model"`

	FocusMode        string `json:"focus_mode"`
	TriggerCondition string `json:"trigger_condition"`

	WhatHappenedSummary string                `json:"what_happened_summary"`
	Actions             []ValueFeedbackAction `json:"actions,omitempty"`

	ExecutionPolicy AgreePolicyType        `json:"execution_policy,omitempty"`
	Approval        *ValueFeedbackApproval `json:"approval,omitempty"`

	TimelineDump string `json:"timeline_dump,omitempty"`
	TimelineDiff string `json:"timeline_diff,omitempty"`

	Outcome *ValueFeedbackOutcome `json:"outcome,omitempty"`

	SessionID string `json:"session_id,omitempty"`
	TaskID    string `json:"task_id,omitempty"`
	Timestamp int64  `json:"timestamp"`
}

// ValueFeedbackSubmitter 由 aive 实现并注册. 实现必须是非阻塞的 (内部有界队列),
// 绝不能阻塞或 panic 到调用方.
type ValueFeedbackSubmitter func(cfg *Config, record *ValueFeedbackRecord)

var (
	valueFeedbackSubmitter   ValueFeedbackSubmitter
	valueFeedbackSubmitterMu sync.RWMutex
)

// RegisterValueFeedbackSubmitter 由 aive 在 init() 中调用注册价值评估实现.
// 默认开启: 注册后即生效, 暂不提供关闭开关.
func RegisterValueFeedbackSubmitter(submitter ValueFeedbackSubmitter) {
	valueFeedbackSubmitterMu.Lock()
	defer valueFeedbackSubmitterMu.Unlock()
	valueFeedbackSubmitter = submitter
}

// SubmitValueFeedback 把一次价值评估上下文交给已注册的实现.
// 未注册时安全 no-op; 任何 panic 被本函数兜底收敛, 绝不影响主流程.
func SubmitValueFeedback(cfg *Config, record *ValueFeedbackRecord) {
	if cfg == nil || record == nil {
		return
	}
	valueFeedbackSubmitterMu.RLock()
	submitter := valueFeedbackSubmitter
	valueFeedbackSubmitterMu.RUnlock()
	if submitter == nil {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			log.Warnf("SubmitValueFeedback recovered panic: %v", r)
		}
	}()
	submitter(cfg, record)
}

// ReviewFocusMode_* 标注审批发生在哪条 review 通路 (写入 record.FocusMode), 便于
// 下游按通路过滤监控. 所有走 DoWaitAgree 的审批通路都应被监控.
const (
	ReviewFocusModeTool    = "tool_review"
	ReviewFocusModePlan    = "plan_review"
	ReviewFocusModeTask    = "task_review"
	ReviewFocusModeAIForge = "aiforge_review"
	ReviewFocusModeGeneric = "review"
)

// SubmitToolReviewValueFeedback 在工具审批路径 (review_decision) 记录一次审批决策,
// 这是最高价值的人工纠正信号. originalParams 为审批前提议参数, finalParams 为审批后
// (review 应用) 的最终参数, changed 精确由二者比对得出.
func (c *Config) SubmitToolReviewValueFeedback(ep *Endpoint, reviewQuestion string, originalParams, finalParams aitool.InvokeParams) {
	c.submitReviewValueFeedback(ep, ReviewFocusModeTool, reviewQuestion, originalParams, finalParams, true)
}

// SubmitReviewValueFeedbackFromEndpoint 是通用审批监控入口 (plan/task/aiforge 等),
// 直接从 endpoint 取 review materials 作为 original_value、取最终 params 作为
// final_value. 这些通路无法精确判定参数是否被编辑, 故不下 changed/approve_with_edit
// 结论, 仍记录 source/required/decision 以区分人工与策略自动放行.
//
// 关键: 监控的判定依据是 approval.source (谁做的决定), 与 execution_policy 解耦.
func (c *Config) SubmitReviewValueFeedbackFromEndpoint(ep *Endpoint, focusMode string, reviewQuestion string) {
	if c == nil || ep == nil {
		return
	}
	original := aitool.InvokeParams(ep.GetReviewMaterials())
	final := ep.GetParams()
	c.submitReviewValueFeedback(ep, focusMode, reviewQuestion, original, final, false)
}

// submitReviewValueFeedback 是审批价值评估的统一装配逻辑. 生产侧只记录"事实":
// execution_policy (配置策略) 与 approval (本次决定来源/结果/原始与最终参数) 解耦,
// 不预先下训练标签. trackChanged=true 时才精确比对参数是否被编辑 (仅工具审批可靠).
// 全程 recover + 非阻塞, 绝不影响主流程.
func (c *Config) submitReviewValueFeedback(ep *Endpoint, focusMode, reviewQuestion string, originalParams, finalParams aitool.InvokeParams, trackChanged bool) {
	if c == nil || ep == nil {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			log.Warnf("submitReviewValueFeedback recovered panic: %v", r)
		}
	}()

	// 审批运行时真相 (谁做的决定 / 是否需要介入 / 原因) 由 DoWaitAgree 写入 endpoint.
	// 缺省视为策略自动放行, 避免 nil 时误判为人工.
	source := ApprovalSourcePolicy
	required := false
	reason := ""
	decidedAtMs := time.Now().UnixMilli()
	if meta := ep.GetApprovalMeta(); meta != nil {
		if meta.Source != "" {
			source = meta.Source
		}
		required = meta.Required
		reason = meta.Reason
		if meta.DecidedAtMs > 0 {
			decidedAtMs = meta.DecidedAtMs
		}
	}

	// 审批响应 (含用户/AI 选择的 suggestion) 始终在 endpoint.GetParams() 上, 无论
	// 工具/计划/任务通路; 工具通路的 finalParams 是 review 应用后的工具参数, 不含
	// suggestion, 故 suggestion 必须单独从这里取.
	reviewResp := ep.GetParams()
	suggestion := ""
	if reviewResp != nil {
		suggestion = reviewResp.GetString("suggestion")
	}
	decisionFromSuggestion, suggestionChange := classifyReviewSuggestion(suggestion)

	changed := suggestionChange
	if trackChanged && !invokeParamsEqual(originalParams, finalParams) {
		changed = true
	}

	// decision 归一化: 优先反映真实人工/AI 决定 (suggestion), 再被"无需审批"与
	// "取消 (空响应)"覆盖.
	decision := decisionFromSuggestion
	switch {
	case !required:
		// 策略/低风险自动放行: 无需审批.
		decision = ApprovalDecisionNotRequired
	case len(reviewResp) == 0:
		// 需要审批但响应为空 (endpoint 被无参释放): 视为用户取消.
		decision = ApprovalDecisionCancel
	case changed && decision == ApprovalDecisionApprove:
		// suggestion 未显式表达编辑, 但参数确被改动 (工具通路精确比对得出).
		decision = ApprovalDecisionApproveWithEdit
	}

	if focusMode == "" {
		focusMode = ReviewFocusModeGeneric
	}

	approval := &ValueFeedbackApproval{
		Required:      required,
		Source:        source,
		Decision:      decision,
		Suggestion:    suggestion,
		Reason:        reason,
		Question:      reviewQuestion,
		OriginalValue: map[string]any(originalParams),
		FinalValue:    map[string]any(finalParams),
		Changed:       changed,
		DecidedAtMs:   decidedAtMs,
	}
	// 备注/纠偏文本 (extra_prompt 等) 在审批响应里, 用 reviewResp 抽取.
	if cmt := extractApprovalComment(reviewResp); cmt != "" {
		approval.Comment = cmt
	}
	if createdAt := ep.GetCreatedAtMs(); createdAt > 0 && decidedAtMs >= createdAt {
		approval.ReviewLatencyMs = decidedAtMs - createdAt
	}

	record := &ValueFeedbackRecord{
		MainModel: ModelEndpoint{
			ModelName:  c.AiModelName,
			ServerName: c.AiServerName,
		},
		FocusMode:        focusMode,
		TriggerCondition: ValueFeedbackTriggerReviewDecision,
		ExecutionPolicy:  c.AgreePolicy,
		Approval:         approval,
		SessionID:        c.PersistentSessionId,
	}
	if c.Timeline != nil {
		record.TimelineDump = c.Timeline.Dump()
	}
	SubmitValueFeedback(c, record)
}

// classifyReviewSuggestion 把审批响应里的原始 suggestion 归一化为 decision, 并给出
// 该 suggestion 是否隐含"修改了产出" (impliesChange). 这是最高价值的人工纠正信号:
// 工具通路的 wrong_tool/wrong_params/direct_answer, 计划/任务通路的 incomplete/
// adjust_plan/deeply_think 等, 都代表人工对 AI 产出的否定或修正, 不能被笼统记成 approve.
func classifyReviewSuggestion(suggestion string) (decision string, impliesChange bool) {
	s := strings.ToLower(strings.TrimSpace(suggestion))
	switch s {
	case "", "continue", "agree", "approve", "yes", "ok", "finish":
		return ApprovalDecisionApprove, false
	case "cancel", "enough-cancel", "abort", "stop":
		return ApprovalDecisionCancel, false
	case "reject", "no", "deny", "wrong_tool", "direct_answer":
		// 工具被否决 (换工具 / 改为直接回答): 对 AI 选择的强负向信号.
		return ApprovalDecisionReject, true
	case "wrong_params":
		// 参数被纠正: 同意用工具但人工改了参数.
		return ApprovalDecisionApproveWithEdit, true
	case "incomplete", "adjust_plan", "deeply_think", "create-subtask",
		"create_subtask", "freedom-review", "redo", "retry":
		// 计划/任务被要求修订: 人工驱动的纠偏.
		return ApprovalDecisionApproveWithEdit, true
	default:
		// 未知 suggestion: 保守按 approve, 但原始值已存入 approval.suggestion 不丢失.
		return ApprovalDecisionApprove, false
	}
}

// extractApprovalComment 从审批最终参数里尽力抽取一条人类备注 (如有).
func extractApprovalComment(params aitool.InvokeParams) string {
	if params == nil {
		return ""
	}
	for _, k := range []string{"comment", "extra_prompt", "note", "remark"} {
		if v, ok := params[k]; ok {
			if s, ok := v.(string); ok && s != "" {
				return s
			}
		}
	}
	return ""
}

// invokeParamsEqual 浅比较两个审批参数是否一致 (用 JSON 规范化后比对, 容忍 key 顺序).
func invokeParamsEqual(a, b aitool.InvokeParams) bool {
	ra, ea := json.Marshal(a)
	rb, eb := json.Marshal(b)
	if ea != nil || eb != nil {
		return false
	}
	return string(ra) == string(rb)
}
