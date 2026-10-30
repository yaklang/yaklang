package reactloops

import (
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
		aitool.WithStringParam("retain_timeline", aitool.WithParam_Required(required), aitool.WithParam_Description("仅 compress_context 使用：保留的普通历史 ID 和闭区间，例如 3-8,12,20-26,37,41-45。空字符串表示全部概括。")),
		aitool.WithStringArrayParam("retire_skill_names", aitool.WithParam_Required(required), aitool.WithParam_Description("仅 compress_context 使用：整份淘汰的技能名。空数组表示全部保留。")),
		aitool.WithStringArrayParam("retire_evidence_ids", aitool.WithParam_Required(required), aitool.WithParam_Description("仅 compress_context 使用：淘汰的证据 id。空数组表示全部保留。")),
	}
}

func (r *ReActLoop) verifyContextCompressionAction(action *aicommon.Action) error {
	if r == nil {
		return fmt.Errorf("compress_context requires a loop")
	}
	if action == nil {
		return fmt.Errorf("context compression requires a compress_context action")
	}
	if _, present := action.LookupCanonicalParam("todo_delta"); present {
		return fmt.Errorf("compress_context must not modify TODO; omit todo_delta")
	}
	if rejected := r.GetActionExecutionValue(action, todoAdjustmentErrorState); rejected != nil {
		return fmt.Errorf("compress_context must not modify TODO; omit todo_delta: %v", rejected)
	}
	if raw, ok := action.LookupCanonicalParam("summary"); !ok {
		return fmt.Errorf("compress_context requires summary")
	} else if _, ok := raw.(string); !ok {
		return fmt.Errorf("compress_context summary must be a string")
	}
	if raw, ok := action.LookupCanonicalParam("retain_timeline"); !ok {
		return fmt.Errorf("compress_context requires retain_timeline")
	} else if _, ok := raw.(string); !ok {
		return fmt.Errorf("compress_context retain_timeline must be a string")
	}
	for _, name := range []string{"retire_skill_names", "retire_evidence_ids"} {
		if _, err := contextCompressionStringArray(action, name); err != nil {
			return err
		}
	}

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
		operator.Fail(err)
		return
	}
	if _, err := timeline.ApplyCompressionSelection(selection, operator.requestPrompt, operator.trajectoryResponse); err != nil {
		operator.Fail(err)
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
