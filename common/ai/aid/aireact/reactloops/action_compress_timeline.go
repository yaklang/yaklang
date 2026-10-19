package reactloops

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/promptloader"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
)

const compressTimelineActionName = aicommon.TimelineCompressionAction

var compressTimelineDynamicText = promptloader.MustLoad("ai/aid/aireact/reactloops/prompts/timeline_compression.txt")

// loopAction_CompressTimeline is executed by the loop when the timeline crosses
// its threshold. It is not offered in the normal action schema: the loop calls
// it directly, then the handler folds history and guides one compression turn.
var loopAction_CompressTimeline = &LoopAction{
	ActionType: compressTimelineActionName,
	Description: "Compress the current timeline before the next decision. " +
		"Summarize ordinary history, keep entries whose visible anchors are still needed, " +
		"and retire unused skills by name or evidence by the id shown in the prompt. " +
		"Do not delete tool schemas or user input.",
	Options: []aitool.ToolOption{
		aitool.WithStringParam("summary", aitool.WithParam_Required(true), aitool.WithParam_Description("承接未保留的普通历史，并在淘汰技能或证据时追加本次淘汰说明。")),
		aitool.WithStringArrayParam("retain_timeline", aitool.WithParam_Required(true), aitool.WithParam_Description("原样保留的普通历史记号，必须复制动态区列出的整行。空数组表示全部概括。")),
		aitool.WithStringArrayParam("retire_skill_names", aitool.WithParam_Required(true), aitool.WithParam_Description("整份淘汰的技能名，与 === Skill: 名字 === 一致。空数组表示不淘汰技能。")),
		aitool.WithStringArrayParam("retire_evidence_ids", aitool.WithParam_Required(true), aitool.WithParam_Description("淘汰的证据 id，与已知观测里的 id 一致。空数组表示不淘汰证据。")),
	},
	ActionHandler: handleCompressTimeline,
}

func handleCompressTimeline(loop *ReActLoop, _ *aicommon.Action, operator *LoopActionHandlerOperator) {
	if loop == nil || loop.config == nil {
		if operator != nil {
			operator.Continue()
		}
		return
	}
	timeline := timelineFromConfig(loop.config)
	if timeline == nil {
		operator.Continue()
		return
	}
	ctx := loop.config.GetContext()
	if task := loop.GetCurrentTask(); task != nil && task.GetContext() != nil {
		ctx = task.GetContext()
	}
	turn := loop.compressionTurn
	var lastStage aicommon.TimelineCompressionStage
	decision, err := timeline.BeginCompressionDecision(aicommon.TimelineCompressionOptions{
		Context:       ctx,
		PromptOptions: loop.mainTimelinePromptOptions(),
		OnProgress: func(stage aicommon.TimelineCompressionStage) {
			if stage == lastStage || ctx.Err() != nil {
				return
			}
			lastStage = stage
			if stage != aicommon.TimelineCompressionWaiting {
				return
			}
			loop.UserStatus("正在等待已有的上下文优化完成…", "Waiting for the ongoing context optimization…",
				aicommon.WithStatusCode("context.optimization.waiting"))
		},
		RetainedContext: map[string]string{
			"user_query":          turn.UserInput,
			"frozen_user_context": turn.FrozenUserContext,
			"todo":                turn.Todo,
		},
	})
	if err != nil {
		if ctx.Err() != nil {
			operator.Fail(ctx.Err())
			return
		}
		log.Warnf("timeline compression did not start; keeping the current context: %v", err)
		operator.Continue()
		return
	}
	if decision == nil {
		operator.Continue()
		return
	}
	var committed bool
	defer func() {
		if !committed {
			decision.Abort(err)
		}
	}()
	if err = loop.completeCompressionTurn(decision); err != nil {
		if ctx.Err() != nil {
			operator.Fail(ctx.Err())
			return
		}
		log.Warnf("timeline compression action finished without a commit; keeping the current context: %v", err)
		operator.Feedback("这次没有完成上下文压缩，继续使用当前上下文。")
		operator.Continue()
		return
	}
	committed = true
	operator.Feedback("上下文已压缩，继续当前任务。")
	operator.Continue()
}

func (r *ReActLoop) completeCompressionTurn(decision *aicommon.CompressionDecision) error {
	if r == nil || r.invoker == nil || r.config == nil || decision == nil {
		return fmt.Errorf("timeline compression requires the main-loop prompt builder")
	}
	base := strings.TrimSpace(r.compressionTurn.Prompt)
	if base == "" {
		return fmt.Errorf("timeline compression requires the main-loop prompt")
	}
	var tail strings.Builder
	if text := strings.TrimSpace(compressTimelineDynamicText); text != "" {
		tail.WriteString("\n\n")
		tail.WriteString(text)
	}
	if guide := strings.TrimSpace(decision.View().DynamicGuide); guide != "" {
		tail.WriteString("\n\n")
		tail.WriteString(guide)
	}
	prompt := base + tail.String()
	limit := aicommon.TimelineCompressionMaxInputTokens
	if aicommon.TokenCountExceeds(prompt, limit) {
		return fmt.Errorf("压缩请求超过输入上限 %d", limit)
	}
	ctx := r.config.GetContext()
	if task := r.GetCurrentTask(); task != nil && task.GetContext() != nil {
		ctx = task.GetContext()
	}
	// Parse, validate and commit inside the transaction. A rejected selection is
	// a post-handler error, so the transaction retries with its own correction
	// prompt instead of a second compression loop.
	return aicommon.CallAITransaction(r.config, prompt, r.config.CallQualityPriorityAI, func(resp *aicommon.AIResponse) error {
		parsed, err := aicommon.ExtractValidActionFromStream(ctx,
			resp.GetOutputStreamReader("timeline-compression", true, r.config.GetEmitter()),
			compressTimelineActionName)
		if err != nil {
			return err
		}
		if err := decision.Validate(parsed); err != nil {
			return err
		}
		_, err = decision.Commit()
		return err
	}, aicommon.WithAIRequest_CallerLabel(aicommon.CallerLabelTimelineCompress), aicommon.WithAIRequest_Context(ctx))
}

func timelineFromConfig(config aicommon.AICallerConfigIf) *aicommon.Timeline {
	provider, ok := config.(interface{ GetTimeline() *aicommon.Timeline })
	if !ok {
		return nil
	}
	return provider.GetTimeline()
}
