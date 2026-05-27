package loop_ssa_risk_overview

import (
	"fmt"
	"io"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	sfu "github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/syntaxflow_utils"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

func buildPostIterationHook(invoker aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithOnPostIteraction(func(loop *reactloops.ReActLoop, iteration int, task aicommon.AIStatefulTask, isDone bool, reason any, operator *reactloops.OnPostIterationOperator) {
		if !isDone {
			collectIterationFindingsFromAction(loop)
			return
		}
		if hasFinalAnswerDelivered(loop) || hasDirectlyAnswered(loop) || getLastAction(loop) == "directly_answer" {
			return
		}
		if strings.TrimSpace(loop.Get("ssa_risk_overview_preface")) == "" {
			return
		}

		contextMaterials := collectFinalizeContext(loop, task, reason)
		if contextMaterials == "" {
			return
		}
		deliverOverviewFinalize(loop, invoker, contextMaterials, iteration)
		if reasonErr, ok := reason.(error); ok && strings.Contains(reasonErr.Error(), "max iterations") {
			operator.IgnoreError()
		}
	})
}

func collectFinalizeContext(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask, reason any) string {
	var ctx strings.Builder
	ctx.WriteString("# SSA Risk Overview Context\n\n")

	if task != nil {
		if u := strings.TrimSpace(task.GetUserInput()); u != "" {
			ctx.WriteString("## User Query\n\n")
			ctx.WriteString(u)
			ctx.WriteString("\n\n")
		}
	}

	if hint := strings.TrimSpace(loop.Get("ssa_risk_total_hint")); hint != "" {
		ctx.WriteString(fmt.Sprintf("## Approximate total: %s\n\n", hint))
	}

	if preface := strings.TrimSpace(loop.Get("ssa_risk_overview_preface")); preface != "" {
		ctx.WriteString("## Risk list preface (DB sample)\n\n")
		ctx.WriteString(utils.ShrinkTextBlock(preface, 12000))
		ctx.WriteString("\n\n")
	}

	if analysis := strings.TrimSpace(loop.Get(sfu.LoopVarSSARiskOverviewAnalysisSummary)); analysis != "" {
		ctx.WriteString("## Batch analyze summary\n\n")
		ctx.WriteString(analysis)
		ctx.WriteString("\n\n")
	}

	if findings := strings.TrimSpace(loop.Get(overviewFindingsKey)); findings != "" {
		ctx.WriteString("## Accumulated overview findings\n\n")
		ctx.WriteString(findings)
		ctx.WriteString("\n\n")
	}

	if actions := strings.TrimSpace(buildRecentActionsPrompt(loop)); actions != "" {
		ctx.WriteString("## Recent actions\n\n")
		ctx.WriteString(actions)
		ctx.WriteString("\n\n")
	}

	ctx.WriteString("## Exit reason\n\n")
	if reasonErr, ok := reason.(error); ok && reasonErr != nil {
		ctx.WriteString(reasonErr.Error())
	} else {
		ctx.WriteString("Loop ended without directly_answer.")
	}
	return strings.TrimSpace(ctx.String())
}

func deliverOverviewFinalize(loop *reactloops.ReActLoop, invoker aicommon.AIInvokeRuntime, contextMaterials string, iteration int) {
	userQuery := ""
	if task := loop.GetCurrentTask(); task != nil {
		userQuery = strings.TrimSpace(task.GetUserInput())
	}
	nonce := utils.RandStringBytes(8)
	summaryPrompt := utils.MustRenderTemplate(`
<|INSTRUCTION_{{ .Nonce }}|>
You are an IRify SSA static-analysis triage expert. Based on the context below, write ONE consolidated report for the user.

Requirements:
1. Do NOT paste the full risk id list again — summarize patterns (severity, rule families, programs).
2. Answer the user's question with evidence from the preface/sample and any batch-analyze section.
3. If batch analyze is missing but user asked for deep review, say what was done and suggest analyze_filtered_risks.
4. Use clear Markdown; same language as the user query.

User query: {{ .UserQuery }}
<|INSTRUCTION_END_{{ .Nonce }}|>

<|CONTEXT_{{ .Nonce }}|>
{{ .ContextMaterials }}
<|CONTEXT_END_{{ .Nonce }}|>
`, map[string]any{
		"Nonce":            nonce,
		"UserQuery":        userQuery,
		"ContextMaterials": contextMaterials,
	})

	taskID := ""
	if task := loop.GetCurrentTask(); task != nil {
		taskID = task.GetId()
	}

	action, err := invoker.InvokeSpeedPriorityLiteForge(
		loop.GetConfig().GetContext(),
		"ssa_risk_overview_finalize_summary",
		summaryPrompt,
		[]aitool.ToolOption{
			aitool.WithStringParam("summary",
				aitool.WithParam_Description("Consolidated SSA risk overview report in Markdown"),
				aitool.WithParam_Required(true),
			),
		},
		aicommon.WithGeneralConfigStreamableFieldEmitterCallback([]string{"summary"}, func(key string, r io.Reader, emitter *aicommon.Emitter) {
			if emitter == nil {
				io.Copy(io.Discard, r)
				return
			}
			if event, _ := emitter.EmitStreamEventWithContentType(
				"re-act-loop-answer-payload",
				utils.JSONStringReader(r),
				taskID,
				aicommon.TypeTextMarkdown,
				func() {},
			); event != nil {
				emitter.EmitTextReferenceMaterial(event.GetStreamEventWriterId(), contextMaterials)
			}
		}),
	)
	if err != nil {
		log.Warnf("ssa_risk_overview finalize: lite forge failed: %v", err)
		deliverRawOverviewFallback(loop, invoker, contextMaterials, iteration)
		return
	}
	summary := strings.TrimSpace(action.GetString("summary"))
	if summary == "" {
		deliverRawOverviewFallback(loop, invoker, contextMaterials, iteration)
		return
	}
	// summary already streamed via WithGeneralConfigStreamableFieldEmitterCallback above.
	markFinalAnswerDelivered(loop)
	recordMetaAction(loop, "finalize_summary", "loop exit without directly_answer", utils.ShrinkTextBlock(summary, 240))
	invoker.AddToTimeline("ssa_risk_overview_finalized",
		fmt.Sprintf("SSA risk overview finalized after %d iterations (lite-forge summary).", iteration))
}

func deliverRawOverviewFallback(loop *reactloops.ReActLoop, invoker aicommon.AIInvokeRuntime, contextMaterials string, iteration int) {
	if hasFinalAnswerDelivered(loop) {
		return
	}
	taskID := ""
	if task := loop.GetCurrentTask(); task != nil {
		taskID = task.GetId()
	}
	if emitter := loop.GetEmitter(); emitter != nil {
		_, _ = emitter.EmitTextMarkdownStreamEvent(
			"re-act-loop-answer-payload",
			strings.NewReader(contextMaterials),
			taskID,
			func() {},
		)
	} else {
		invoker.EmitResultAfterStream(contextMaterials)
	}
	markFinalAnswerDelivered(loop)
	invoker.AddToTimeline("ssa_risk_overview_finalized_raw",
		fmt.Sprintf("SSA risk overview finalized with raw context after %d iterations.", iteration))
}
