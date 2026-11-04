package reactloops

import (
	"context"
	"errors"
	"fmt"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/promptloader"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

var compressContextDynamicText = promptloader.MustLoad("ai/aid/aireact/reactloops/prompts/timeline_compression.txt")

// Keep the action contract in both schemas so switching prompt mode does not
// rewrite the cacheable prefix. The model may call it on any turn; a
// compression prompt only narrows the advertised action for that response.
var loopAction_CompressContext = &LoopAction{
	ActionType:        aicommon.ContextCompressionAction,
	NativeOnlyOptions: true,
	Description: "Compress the current context. " +
		"Summarize ordinary history, retain whole entries by timeline ID or ID range, " +
		"and retire unused skills by name or evidence by its visible id. " +
		"A valid summary is sufficient; malformed optional selections are ignored. " +
		"Do not execute business actions, delete tool schemas, or modify user input or TODO.",
	Options:        contextCompressionOptions(false),
	NativeOptions:  contextCompressionOptions(true),
	ActionVerifier: (*ReActLoop).verifyContextCompressionAction,
	ActionHandler:  handleCompressContext,
}

// The text schema combines fields from all actions; compression-only fields
// must not become required for ordinary actions. Native schemas are per action.
func contextCompressionOptions(required bool) []aitool.ToolOption {
	return []aitool.ToolOption{
		aitool.WithStringParam("summary", aitool.WithParam_Required(required), aitool.WithParam_Description("当前动作的摘要；compress_context 时承接未保留的普通历史，并在淘汰技能或证据时追加说明。")),
		aitool.WithStringParam("retain_timeline", aitool.WithParam_Description("可选：保留的普通历史 ID 和闭区间，例如 3-8,12,20-26,37,41-45。缺失、无效或未知 ID 会被忽略。")),
		aitool.WithStringArrayParam("retire_skill_names", aitool.WithParam_Description("可选：整份淘汰的技能名。缺失、无效或未知名称会被忽略。")),
		aitool.WithStringArrayParam("retire_evidence_ids", aitool.WithParam_Description("可选：淘汰的证据 id。缺失、无效或未知 id 会被忽略。")),
	}
}

func (r *ReActLoop) verifyContextCompressionAction(action *aicommon.Action) error {
	if r == nil {
		return fmt.Errorf("compress_context requires a loop")
	}
	if action == nil {
		return fmt.Errorf("context compression requires a compress_context action")
	}
	// Compression accepts only a summary as essential. Other parameters are
	// best-effort selections and never make the action unusable.
	if _, ok := action.LookupCanonicalParam("summary"); !ok {
		return fmt.Errorf("compress_context requires summary")
	}
	// A compression turn must never carry TODO maintenance. Drop it before the
	// shared TODO application path so the valid summary can still be committed.
	action.DeleteParam("todo_delta")
	return nil
}

func handleCompressContext(loop *ReActLoop, action *aicommon.Action, operator *LoopActionHandlerOperator) {
	if loop == nil || operator == nil {
		if operator != nil {
			operator.Fail(fmt.Errorf("context compression requires a loop"))
		}
		return
	}
	timeline := timelineFromConfig(loop.config)
	if timeline == nil {
		operator.Fail(fmt.Errorf("context compression requires a timeline"))
		return
	}
	selection, err := loop.prepareContextCompression(action, contextCompressionRequest{
		throughID: operator.requestTimelineThroughID, itemIDs: operator.requestTimelineItemIDs,
	})
	if err != nil {
		var rejected *contextCompressionSelectionError
		if errors.As(err, &rejected) {
			operator.contextCompressionError = err
			operator.Continue()
			return
		}
		loop.emitContextCompressionRuntimeWarning("reading timeline candidates", err)
		operator.Continue()
		return
	}
	if _, err := timeline.ApplyCompressionSelection(selection, operator.requestPrompt, operator.trajectoryResponse); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			operator.Fail(err)
			return
		}
		loop.emitContextCompressionRuntimeWarning("committing the timeline compression result", err)
		operator.Continue()
		return
	}
	operator.Continue()
}

func timelineFromConfig(config aicommon.AICallerConfigIf) *aicommon.Timeline {
	provider, ok := config.(interface{ GetTimeline() *aicommon.Timeline })
	if !ok {
		return nil
	}
	return provider.GetTimeline()
}
