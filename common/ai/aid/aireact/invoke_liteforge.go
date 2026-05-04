package aireact

import (
	"context"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/aiforge"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (r *ReAct) invokeLiteForgeWithCallback(cb aicommon.AICallbackType, ctx context.Context, actionName string, prompt string, outputs []aitool.ToolOption, opts ...aicommon.GeneralKVConfigOption) (*aicommon.Action, error) {
	var rawOutputs []any
	for _, output := range outputs {
		var rawOpt any = output
		rawOutputs = append(rawOutputs, rawOpt)
	}

	gconfig := aicommon.NewGeneralKVConfig(opts...)

	fopts := []aiforge.LiteForgeOption{
		aiforge.WithLiteForge_Prompt(prompt),
		aiforge.WithLiteForge_OutputSchemaRaw(
			actionName,
			aitool.NewObjectSchemaWithActionName(actionName, rawOutputs...),
		),
	}
	// 关键词: aicache, PROMPT_SECTION, StaticInstruction, invokeLiteForgeWithCallback
	// 调用方可以通过 aicommon.WithLiteForgeStaticInstruction 携带系统侧静态指令.
	// P0-B1 之后该字段进入 semi-dynamic 段 (而非 high-static), 跨同一 forge
	// 调用稳定哈希; 真正动态的内容应由 prompt 参数 (进入 dynamic 段) 承载.
	if staticInstruction := gconfig.GetLiteForgeStaticInstruction(); staticInstruction != "" {
		fopts = append(fopts, aiforge.WithLiteForge_StaticInstruction(staticInstruction))
	}
	for _, i := range gconfig.GetStreamableFields() {
		fopts = append(fopts, aiforge.WithLiteForge_StreamableFieldWithAINodeId(i.AINodeId(), i.FieldKey()))
	}
	// add user-defined field stream callbacks from GeneralKVConfig
	for _, item := range gconfig.GetStreamableFieldCallbacks() {
		if item.Callback != nil {
			fopts = append(fopts, aiforge.WithLiteForge_FieldStreamEmitterCallback(item.FieldKeys, aiforge.FieldStreamEmitterCallback(item.Callback)))
		}
	}
	fopts = append(fopts, aiforge.WithLiteForge_Emitter(r.config.Emitter))

	if !utils.IsNil(cb) {
		fopts = append(fopts, aiforge.WithExtendLiteForge_AIOption(aicommon.WithFastAICallback(cb)))
	}

	f, err := aiforge.NewLiteForge(actionName, fopts...)
	if err != nil {
		return nil, utils.Wrap(err, "create liteforge failed")
	}
	execCb := cb
	if utils.IsNil(execCb) {
		execCb = r.config.OriginalAICallback
	}
	// 关键词: invokeLiteForgeWithCallback, ai.usageCallback 透传, WithUserUsageCallback
	// 子 coordinator 走 WithFastAICallback path, 必须把父 ReAct 的 user UsageCallback
	// 一并继承, 否则 raw chat 末帧 token usage 不会触达 ai.usageCallback(...).
	execOpts := []aicommon.ConfigOption{
		aicommon.WithAgreeYOLO(),
		aicommon.WithFastAICallback(execCb),
		aicommon.WithPersistentSessionId(r.config.PersistentSessionId),
		aicommon.WithDisableCreateDBRuntime(true), // disable create db runtime because ReAct loop will create it before invoking liteforge, and creating it again in liteforge may
	}
	if userUsageCb := r.config.GetUserUsageCallback(); userUsageCb != nil {
		execOpts = append(execOpts, aicommon.WithUserUsageCallback(userUsageCb))
	}
	forgeResult, err := f.Execute(ctx, []*ypb.ExecParamItem{
		{Key: "query", Value: prompt},
	},
		execOpts...)
	if err != nil {
		return nil, utils.Wrap(err, "invoke liteforge failed")
	}
	return forgeResult.Action, nil
}

func (r *ReAct) InvokeSpeedPriorityLiteForge(
	ctx context.Context, actionName string, prompt string,
	outputs []aitool.ToolOption, opts ...aicommon.GeneralKVConfigOption,
) (*aicommon.Action, error) {
	return r.invokeLiteForgeWithCallback(r.config.SpeedPriorityAICallback, ctx, actionName, prompt, outputs, opts...)
}

func (r *ReAct) InvokeQualityPriorityLiteForge(
	ctx context.Context, actionName string, prompt string,
	outputs []aitool.ToolOption, opts ...aicommon.GeneralKVConfigOption,
) (*aicommon.Action, error) {
	return r.invokeLiteForgeWithCallback(r.config.QualityPriorityAICallback, ctx, actionName, prompt, outputs, opts...)
}

func (r *ReAct) InvokeLiteForge(ctx context.Context, actionName string, prompt string, outputs []aitool.ToolOption, opts ...aicommon.GeneralKVConfigOption) (*aicommon.Action, error) {
	return r.InvokeQualityPriorityLiteForge(ctx, actionName, prompt, outputs, opts...)
}
