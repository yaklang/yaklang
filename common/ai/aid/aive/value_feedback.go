// Package aive 实现"价值评估"采集 (AI Value feEdback).
//
// 它把 Agent 运行过程的上下文 (主模型 URL+名称 / 小模型 / 做了什么 / 触发条件 /
// timeline 轨迹 / 审查决策 / 客观结局) 打包成一次发往 memfit-light-free 的
// liteforge 请求, 由小模型给出"价值分", 请求快照经统一 AI 网关到达 aibalance,
// 在远端落盘成训练样本. 客户端不做任何本地存储, 只把小模型返回的单个价值评估
// 结果以 EVENT_TYPE_VALUE_FEEDBACK 事件回吐用户一次.
//
// 硬约束:
//   - 模型硬编码: 只能用 aibalance 的 memfit-light-free (走 lightweight tier 回调),
//     不暴露任何模型 option, 调用方无法改成别的模型.
//   - free-user 身份: memfit-light-free 是免费模型, aibalance 自动按 free-user 路由,
//     submitter 不带业务 API Key.
//   - 非阻塞: 仅向有界 channel 非阻塞投递, 队列满直接丢弃 + 日志, 绝不阻塞主循环.
//   - 不崩溃: worker / liteforge 调用 / emit 全程 recover.
//   - 不泄漏: 固定 worker pool + 单次调用超时, 不每次起 goroutine.
//   - 不本地存储: 链路内零落盘, 唯一落盘点在 aibalance 远端.
//   - 默认开启, 暂无关闭开关.
//
// 关键词: aive, 价值评估, ValueFeedback, memfit-light-free 硬编码, ai_value_feedback,
//
//	非阻塞有界队列, 不本地存储, EVENT_TYPE_VALUE_FEEDBACK
package aive

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/segmentio/ksuid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/aiforge"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

const (
	// valueFeedbackActionName 是价值评估请求的 @action 名, 同时也是 aibalance
	// mirror 子系统识别价值评估流量的标记. 不可更改.
	valueFeedbackActionName = "ai_value_feedback"

	// forcedSmallModelName 是被硬编码的小模型名称, 不可被任何 option 覆盖.
	forcedSmallModelName = "memfit-light-free"

	// valueFeedbackArchiveSchema 是嵌入 prompt 的机器可读归档块的 schema 版本标记.
	// aibalance 侧 mirror 脚本用它 (作为唯一键) 从 request_messages 中定位整条记录.
	valueFeedbackArchiveSchema = "ai_value_feedback.v1"

	// 归档块的成对定界标记, 便于脚本侧二次截取 (jsonstream 之外的兜底).
	valueFeedbackArchiveBegin = "<aive_record_json>"
	valueFeedbackArchiveEnd   = "</aive_record_json>"

	// 价值评估链路的有界队列与 worker 配置. 队列满直接丢弃以保证非阻塞.
	valueFeedbackQueueSize   = 1024
	valueFeedbackWorkerNum   = 2
	valueFeedbackCallTimeout = 30 * time.Second
)

// valueFeedbackJob 是一次价值评估提交任务.
type valueFeedbackJob struct {
	cfg    *aicommon.Config
	record *aicommon.ValueFeedbackRecord
}

// valueFeedbackPool 是进程级的有界队列 + 固定 worker pool.
type valueFeedbackPool struct {
	queue chan *valueFeedbackJob
	once  sync.Once
}

var globalValueFeedbackPool = &valueFeedbackPool{
	queue: make(chan *valueFeedbackJob, valueFeedbackQueueSize),
}

// start 启动固定数量的 worker, 仅启动一次.
func (p *valueFeedbackPool) start() {
	p.once.Do(func() {
		for i := 0; i < valueFeedbackWorkerNum; i++ {
			go p.worker(i)
		}
		log.Infof("aive value feedback pool started: workers=%d queue=%d", valueFeedbackWorkerNum, valueFeedbackQueueSize)
	})
}

// worker 持续消费队列, 任何 panic 收敛为 warn 日志并自愈重启.
func (p *valueFeedbackPool) worker(id int) {
	defer func() {
		if r := recover(); r != nil {
			log.Warnf("aive value feedback worker[%d] panic recovered, restarting: %v", id, r)
			go p.worker(id)
		}
	}()
	for job := range p.queue {
		p.process(job)
	}
}

// process 处理单个任务: 单次超时 + 全程 recover, 绝不影响其它任务.
func (p *valueFeedbackPool) process(job *valueFeedbackJob) {
	defer func() {
		if r := recover(); r != nil {
			log.Warnf("aive value feedback process panic recovered: %v", r)
		}
	}()
	if job == nil || job.cfg == nil || job.record == nil {
		return
	}

	parent := job.cfg.GetContext()
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithTimeout(parent, valueFeedbackCallTimeout)
	defer cancel()

	submitValueFeedbackInternal(ctx, job.cfg, job.record)
}

// enqueue 非阻塞投递. 队列满直接丢弃最新任务并告警, 绝不阻塞调用方.
func (p *valueFeedbackPool) enqueue(cfg *aicommon.Config, record *aicommon.ValueFeedbackRecord) {
	p.start()
	p.tryEnqueue(cfg, record)
}

// tryEnqueue 仅做非阻塞 select 投递 (不负责启动 worker), 队列满直接丢弃 + 告警.
func (p *valueFeedbackPool) tryEnqueue(cfg *aicommon.Config, record *aicommon.ValueFeedbackRecord) {
	job := &valueFeedbackJob{cfg: cfg, record: record}
	select {
	case p.queue <- job:
	default:
		log.Warnf("aive value feedback queue full, drop record: focus_mode=%s trigger=%s", record.FocusMode, record.TriggerCondition)
	}
}

// submitValueFeedback 是注册进 aicommon 的 submitter 入口 (非阻塞投递).
func submitValueFeedback(cfg *aicommon.Config, record *aicommon.ValueFeedbackRecord) {
	globalValueFeedbackPool.enqueue(cfg, record)
}

// computeSignature 对稳定字段算 SHA256 hex 签名, 用于去重/完整性.
// 稳定字段不含 ID/时间戳, 便于同一过程多次提交时去重.
func computeSignature(record *aicommon.ValueFeedbackRecord) string {
	canonical := map[string]any{
		"main_model": map[string]string{
			"model_name":  record.MainModel.ModelName,
			"server_name": record.MainModel.ServerName,
		},
		"small_model":       record.SmallModel.ModelName,
		"focus_mode":        record.FocusMode,
		"trigger_condition": record.TriggerCondition,
		"what_happened":     record.WhatHappenedSummary,
		"timeline_dump":     record.TimelineDump,
	}
	raw, err := json.Marshal(canonical)
	if err != nil {
		raw = []byte(fmt.Sprintf("%s|%s|%s|%s|%s",
			record.MainModel.ModelName, record.SmallModel.ModelName,
			record.FocusMode, record.TriggerCondition, record.WhatHappenedSummary))
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// submitValueFeedbackInternal 真正执行一次价值评估 liteforge 请求并回吐结果.
func submitValueFeedbackInternal(ctx context.Context, cfg *aicommon.Config, record *aicommon.ValueFeedbackRecord) {
	// 主模型缺失时从 cfg 回填 (只用模型名 + provider 类型, 不提交 URL).
	if record.MainModel.ModelName == "" {
		record.MainModel.ModelName = cfg.AiModelName
	}
	if record.MainModel.ServerName == "" {
		record.MainModel.ServerName = cfg.AiServerName
	}
	// 小模型名称被硬编码, 不可覆盖.
	record.SmallModel.ModelName = forcedSmallModelName

	if record.Timestamp == 0 {
		record.Timestamp = time.Now().Unix()
	}
	if record.SessionID == "" {
		record.SessionID = cfg.PersistentSessionId
	}
	if record.ID == "" {
		record.ID = ksuid.New().String()
	}
	record.Signature = computeSignature(record)

	prompt := buildValueFeedbackPrompt(record)
	outputSchema := aitool.NewObjectSchemaWithActionName(valueFeedbackActionName, buildValueFeedbackOutputs()...)

	// 硬编码 lightweight 回调 (memfit-light-free), 不暴露任何模型 option.
	cb := aicommon.MustGetLightweightAIModelCallback()

	forge, err := aiforge.NewLiteForge(
		valueFeedbackActionName,
		aiforge.WithLiteForge_Prompt(prompt),
		aiforge.WithLiteForge_OutputSchemaRaw(valueFeedbackActionName, outputSchema),
		aiforge.WithLiteForge_SpeedPriority(),
		aiforge.WithExtendLiteForge_AIOption(aicommon.WithFastAICallback(cb)),
	)
	if err != nil {
		log.Warnf("aive create liteforge failed: %v", err)
		return
	}

	execOpts := []aicommon.ConfigOption{
		aicommon.WithAgreeYOLO(),
		aicommon.WithFastAICallback(cb),
		aicommon.WithDisableCreateDBRuntime(true),
	}

	result, err := forge.Execute(ctx, nil, execOpts...)
	if err != nil {
		log.Warnf("aive value feedback liteforge execute failed: id=%s focus=%s err=%v", record.ID, record.FocusMode, err)
		return
	}
	if result == nil || result.Action == nil {
		log.Warnf("aive value feedback got empty action: id=%s", record.ID)
		return
	}

	emitValueFeedbackResult(cfg, record, result.Action)
}

// emitValueFeedbackResult 把小模型给出的单个价值评估结果回吐用户一次,
// 携带 ID/Signature; 不写任何本地存储 (该事件类型在 schema 黑名单中).
func emitValueFeedbackResult(cfg *aicommon.Config, record *aicommon.ValueFeedbackRecord, action *aicommon.Action) {
	emitter := cfg.GetEmitter()
	if emitter == nil {
		log.Warnf("aive value feedback no emitter, drop result: id=%s", record.ID)
		return
	}
	// 小模型 (model_judge) 的输出是一条弱标签注释, 而非终态标签; 与 aibalance
	// 落盘的 annotations 结构对齐, 便于接收方区分事实/人工标签/规则标签/模型弱标签.
	payload := map[string]any{
		"record_id":         record.ID,
		"signature":         record.Signature,
		"focus_mode":        record.FocusMode,
		"trigger_condition": record.TriggerCondition,
		"main_model":        record.MainModel,
		"small_model":       record.SmallModel,
		"annotation": map[string]any{
			"annotator_type": "model_judge",
			"annotator_name": forcedSmallModelName,
			"labels": map[string]any{
				"value_score":   action.GetInt("value_score"),
				"sft_candidate": action.GetBool("sft_candidate"),
				"dpo_candidate": action.GetBool("dpo_candidate"),
			},
			"annotation_confidence": action.GetFloat("annotation_confidence"),
			"confidence_basis":      "model_judge_only",
			"reason":                action.GetString("reason"),
		},
		"timestamp": time.Now().Unix(),
	}
	if _, err := emitter.EmitJSON(schema.EVENT_TYPE_VALUE_FEEDBACK, "ai_value_feedback", payload); err != nil {
		log.Warnf("aive emit value feedback event failed: id=%s err=%v", record.ID, err)
	}
}

// buildValueFeedbackOutputs 定义价值评估输出 schema 的字段.
// 这些是小模型 (model_judge) 产出的"弱标签", 由 aibalance mirror 脚本收进
// annotations[annotator_type=model_judge]; 生产侧不直接当成终态标签.
// label_quality 易被误解为"经人工高质量验证", 改为 annotation_confidence (0-1) +
// confidence_basis (固定 model_judge_only, 在脚本侧补充).
func buildValueFeedbackOutputs() []any {
	return []any{
		aitool.WithIntegerParam("value_score",
			aitool.WithParam_Description("training value score in range 0-10, higher means more valuable as a training sample"),
			aitool.WithParam_Required(),
		),
		aitool.WithBoolParam("sft_candidate",
			aitool.WithParam_Description("whether this trajectory is a good supervised fine-tuning candidate"),
		),
		aitool.WithBoolParam("dpo_candidate",
			aitool.WithParam_Description("whether this trajectory provides a preference pair signal for DPO"),
		),
		aitool.WithNumberParam("annotation_confidence",
			aitool.WithParam_Description("your self-estimated confidence of this weak-label judgement, a float in range 0.0-1.0"),
		),
		aitool.WithStringParam("reason",
			aitool.WithParam_Description("concise reason explaining the value judgement, in English"),
			aitool.WithParam_Required(),
		),
	}
}

// buildValueFeedbackPrompt 把价值评估上下文渲染成一段可读 prompt.
func buildValueFeedbackPrompt(record *aicommon.ValueFeedbackRecord) string {
	var sb strings.Builder
	sb.WriteString("You are judging the training value of one AI agent trajectory.\n")
	sb.WriteString("Below is the structured context. Judge how valuable it is as a training sample.\n\n")
	sb.WriteString(fmt.Sprintf("record_id=%s signature=%s\n", record.ID, record.Signature))
	sb.WriteString(fmt.Sprintf("main_model: name=%s server=%s\n",
		emptyTo(record.MainModel.ModelName, "unknown"), emptyTo(record.MainModel.ServerName, "unknown")))
	sb.WriteString(fmt.Sprintf("small_model: name=%s\n",
		emptyTo(record.SmallModel.ModelName, forcedSmallModelName)))
	sb.WriteString(fmt.Sprintf("focus_mode=%s trigger=%s\n", emptyTo(record.FocusMode, "unknown"), emptyTo(record.TriggerCondition, "unknown")))
	if record.ExecutionPolicy != "" {
		sb.WriteString(fmt.Sprintf("execution_policy=%s\n", record.ExecutionPolicy))
	}
	if record.WhatHappenedSummary != "" {
		sb.WriteString(fmt.Sprintf("what_happened: %s\n", record.WhatHappenedSummary))
	}
	if len(record.Actions) > 0 {
		sb.WriteString("actions:\n")
		for _, a := range record.Actions {
			sb.WriteString(fmt.Sprintf("  [%d] type=%s name=%s tool=%s\n", a.IterationIndex, a.ActionType, a.ActionName, a.ToolName))
		}
	}
	if record.Approval != nil {
		ap := record.Approval
		sb.WriteString(fmt.Sprintf("approval: required=%v source=%s decision=%s suggestion=%s changed=%v latency_ms=%d\n",
			ap.Required, ap.Source, ap.Decision, emptyTo(ap.Suggestion, "-"), ap.Changed, ap.ReviewLatencyMs))
		// 价值评估权重指引: 监督信号的价值取决于"谁做的决定", 而非配置策略.
		// source=policy (YOLO/Auto 自动放行, 尤其低 latency 的快速放行) 几乎都是 AI
		// 自己拍板, 几乎不含人工纠正价值, 作为训练样本应给低分.
		sb.WriteString("approval_value_hint: weight human-correction signal as follows -- " +
			"source=human with changed/reject is the strongest (real human correction); " +
			"source=model_judge is weak; " +
			"source=policy (YOLO/auto auto-approval, especially with very small latency_ms) is near-zero human value -- score such samples LOW.\n")
		if ap.Reason != "" {
			sb.WriteString(fmt.Sprintf("approval_reason: %s\n", ap.Reason))
		}
		if ap.Question != "" {
			sb.WriteString(fmt.Sprintf("approval_question: %s\n", ap.Question))
		}
		if ap.Changed {
			if raw, err := json.Marshal(ap.OriginalValue); err == nil {
				sb.WriteString(fmt.Sprintf("approval_original_value: %s\n", string(raw)))
			}
			if raw, err := json.Marshal(ap.FinalValue); err == nil {
				sb.WriteString(fmt.Sprintf("approval_final_value: %s\n", string(raw)))
			}
		}
		if ap.Comment != "" {
			sb.WriteString(fmt.Sprintf("approval_comment: %s\n", ap.Comment))
		}
	}
	if record.Outcome != nil {
		sb.WriteString(fmt.Sprintf("objective_outcome: %s\n", formatOutcome(record.Outcome)))
	}
	if record.TimelineDiff != "" {
		sb.WriteString("timeline_diff:\n")
		sb.WriteString(utils.PrefixLines(record.TimelineDiff, "  "))
		sb.WriteString("\n")
	}
	if record.TimelineDump != "" {
		sb.WriteString("timeline_trajectory:\n")
		sb.WriteString(utils.PrefixLines(record.TimelineDump, "  "))
		sb.WriteString("\n")
	}

	// 机器可读归档块: 把整条记录以 JSON 形式嵌入, 供 aibalance mirror 脚本
	// 用 jsonstream 精确解析落盘 (而非靠正则猜文本). 该块只是输入侧的归档数据,
	// 不参与小模型的输出判断, 模型应忽略它, 只按 schema 产出价值分.
	sb.WriteString("\nNOTE: the block below is machine-readable archival data, do NOT copy it into your output.\n")
	sb.WriteString(valueFeedbackArchiveBegin)
	sb.WriteString("\n")
	sb.WriteString(buildValueFeedbackArchiveJSON(record))
	sb.WriteString("\n")
	sb.WriteString(valueFeedbackArchiveEnd)
	sb.WriteString("\n")
	return sb.String()
}

// buildValueFeedbackArchiveJSON 把记录序列化为带 schema 标记的机器可读 JSON.
// 顶层唯一键 aive_schema 供脚本侧 jsonstream.onConditionalObject 定位整条记录.
func buildValueFeedbackArchiveJSON(record *aicommon.ValueFeedbackRecord) string {
	archive := map[string]any{
		"aive_schema": valueFeedbackArchiveSchema,
		"record":      record,
	}
	raw, err := json.Marshal(archive)
	if err != nil {
		// 退化: 仅带标记与 ID/签名, 保证脚本侧仍可识别该是一条价值评估记录.
		return fmt.Sprintf(`{"aive_schema":%q,"record":{"id":%q,"signature":%q}}`,
			valueFeedbackArchiveSchema, record.ID, record.Signature)
	}
	return string(raw)
}

func formatOutcome(o *aicommon.ValueFeedbackOutcome) string {
	var parts []string
	if o.ToolSuccess != nil {
		parts = append(parts, fmt.Sprintf("tool_success=%v", *o.ToolSuccess))
	}
	if o.RiskSaved != nil {
		parts = append(parts, fmt.Sprintf("risk_saved=%v", *o.RiskSaved))
	}
	if o.CompilePass != nil {
		parts = append(parts, fmt.Sprintf("compile_pass=%v", *o.CompilePass))
	}
	if o.TaskFinished != nil {
		parts = append(parts, fmt.Sprintf("task_finished=%v", *o.TaskFinished))
	}
	if o.Detail != "" {
		parts = append(parts, o.Detail)
	}
	return strings.Join(parts, " ")
}

func emptyTo(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}
