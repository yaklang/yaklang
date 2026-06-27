package loop_vuln_verify

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
)

func buildOnPostIterationHook(invoker aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithOnPostIteraction(func(
		loop *reactloops.ReActLoop,
		iteration int,
		task aicommon.AIStatefulTask,
		isDone bool,
		reason any,
		op *reactloops.OnPostIterationOperator,
	) {
		// Auto-clear the "SSA Risk ID only" flag after the AI has had two iterations
		// to call require_tool and directly_call_tool for ssa-risk. Without this the
		// AI keeps seeing the "[阻断]" warning and repeats the ssa-risk call indefinitely.
		if !isDone && loop.Get(keySSARiskIDOnly) == "true" && iteration >= 2 {
			loop.Set(keySSARiskIDOnly, "")
		}

		if !isDone {
			return
		}
		// If the verdict was already delivered via directly_answer, nothing to do.
		if loop.Get(keyVerdictDelivered) == "true" {
			return
		}

		deliverFallbackReport(loop, invoker, task, reason)

		// Treat max-iterations exhaustion as a non-fatal outcome.
		if reasonErr, ok := reason.(error); ok &&
			strings.Contains(reasonErr.Error(), "max iterations") {
			op.IgnoreError()
		}
	})
}

// deliverFallbackReport is called when the main loop ends without producing a verdict.
// It uses LiteForge to synthesise a summary from collected evidence, or emits a
// minimal placeholder when no evidence was gathered.
func deliverFallbackReport(
	loop *reactloops.ReActLoop,
	invoker aicommon.AIInvokeRuntime,
	task aicommon.AIStatefulTask,
	reason any,
) {
	evidenceJSON := loop.Get(keyEvidenceJSON)
	finding := loop.Get(keyFindingDescription)
	target := loop.Get(keyTargetInfo)
	reproducibility := loop.Get(keyReproducibilityVerdict)
	reachability := loop.Get(keyReachabilityStatus)

	hasEvidence := evidenceJSON != "" && evidenceJSON != "[]"

	// Check whether the framework's own satisfaction checker fired and produced
	// a positive result (e.g. keyword match in HTTP response triggered operator.Exit
	// before the AI could call record_evidence).
	satRecord := loop.GetLastSatisfactionRecordFull()
	satConfirmed := satRecord != nil && satRecord.Satisfactory

	var promptParts []string
	promptParts = append(promptParts,
		"请根据以下漏洞验证会话信息生成一份 Markdown 格式的验证报告，",
		"包含：Verdict（CONFIRMED/NOT_CONFIRMED/INCONCLUSIVE）、Evidence Summary、结论依据。",
		"",
		fmt.Sprintf("**Finding**: %s", finding),
		fmt.Sprintf("**Target**: %s", target),
		fmt.Sprintf("**Reproducibility Assessment**: %s", reproducibility),
		fmt.Sprintf("**Reachability**: %s", reachability),
	)

	if hasEvidence {
		promptParts = append(promptParts,
			"",
			"**Collected Evidence (JSON)**:",
			"```json",
			evidenceJSON,
			"```",
		)
	} else if satConfirmed {
		// The loop was exited by the satisfaction checker, which means the AI's
		// tool call produced sufficient evidence for the framework to consider the
		// task complete. Use the satisfaction record as evidence.
		promptParts = append(promptParts,
			"",
			"**Note**: The verification loop was terminated by the framework's automatic",
			"satisfaction check after a tool call produced confirmatory output.",
			"The AI did not explicitly call record_evidence, but the framework determined",
			"the task was complete based on the following observation:",
			"",
			fmt.Sprintf("**Satisfaction Checker Reasoning**: %s", satRecord.Reason),
		)
		if satRecord.Evidence != "" {
			promptParts = append(promptParts,
				"",
				fmt.Sprintf("**Framework Evidence**: %s", satRecord.Evidence),
			)
		}
		promptParts = append(promptParts,
			"",
			"**IMPORTANT — Verdict guidance**:",
			"- The satisfaction checker concluded the task IS complete (Satisfactory=true).",
			"- Use CONFIRMED if the satisfaction reasoning clearly indicates successful exploitation.",
			"- Use INCONCLUSIVE only if the reasoning is ambiguous.",
		)
	} else {
		promptParts = append(promptParts,
			"",
			"**Note**: No evidence was formally recorded via record_evidence during this session.",
			"The AI may have made tool calls and observed results, but did not call record_evidence to capture them.",
			"",
			"**IMPORTANT — Verdict guidance when evidence is empty**:",
			"- Do NOT use NOT_CONFIRMED. NOT_CONFIRMED means 'tested and definitively does not work'.",
			"- Use INCONCLUSIVE — it means 'verification was incomplete or evidence was lost'.",
			"- In the Evidence Summary, note that evidence may have been observed but not recorded.",
		)
	}

	if reasonErr, ok := reason.(error); ok {
		promptParts = append(promptParts,
			"",
			fmt.Sprintf("**Session ended because**: %v", reasonErr),
		)
	}

	prompt := strings.Join(promptParts, "\n")

	// Do NOT use WithGeneralConfigStreamableFieldWithNodeId here: that option
	// streams the field with no content-type, which causes the frontend to render
	// it as plain text. We emit the final result exclusively via
	// EmitTextMarkdownStreamEvent below so the correct content-type is applied.
	result, err := invoker.InvokeQualityPriorityLiteForge(
		task.GetContext(),
		"vuln-verify-fallback-report",
		prompt,
		[]aitool.ToolOption{
			aitool.WithStringParam("report",
				aitool.WithParam_Required(true),
				aitool.WithParam_Description("完整的漏洞验证报告，Markdown 格式"),
			),
		},
	)
	if err != nil {
		log.Errorf("[VulnVerify] fallback report generation failed: %v", err)
		emitMinimalFallback(loop, invoker, satRecord)
		return
	}

	report := result.GetString("report")
	if report == "" {
		emitMinimalFallback(loop, invoker, satRecord)
		return
	}

	loop.Set(keyVerdictDelivered, "true")
	invoker.EmitFileArtifactWithExt("vuln_verify_fallback_report", ".md", report)
	// Stream markdown to UI first, then register as the final result.
	taskIndex := ""
	if task != nil {
		taskIndex = task.GetId()
	}
	if emitter := loop.GetEmitter(); emitter != nil {
		if _, err := emitter.EmitTextMarkdownStreamEvent(
			"re-act-loop-answer-payload",
			strings.NewReader(report),
			taskIndex,
			func() {},
		); err != nil {
			log.Warnf("[VulnVerify] fallback report markdown stream failed: %v", err)
		}
	}
	invoker.EmitResultAfterStream(report)
}

func emitMinimalFallback(loop *reactloops.ReActLoop, invoker aicommon.AIInvokeRuntime, satRecord *reactloops.SatisfactionRecord) {
	finding := loop.Get(keyFindingDescription)
	if finding == "" {
		finding = "(unknown)"
	}
	count := loop.Get(keyEvidenceCount)
	if count == "" {
		count = "0"
	}

	verdict := "INCONCLUSIVE"
	evidenceNote := "The verification session ended without producing a conclusive result."

	if satRecord != nil && satRecord.Satisfactory {
		verdict = "CONFIRMED"
		evidenceNote = "The framework's automatic satisfaction check detected confirmatory evidence.\n\n" +
			"**Satisfaction Checker Reasoning**: " + satRecord.Reason
		if satRecord.Evidence != "" {
			evidenceNote += "\n\n**Evidence**: " + satRecord.Evidence
		}
	}

	report := fmt.Sprintf(
		"# Vulnerability Verification Report\n\n"+
			"## Verdict: %s\n\n"+
			"%s\n\n"+
			"**Finding**: %s\n\n"+
			"**Evidence collected**: %s piece(s)\n\n"+
			"Please review the session timeline for details.",
		verdict, evidenceNote, finding, count)

	loop.Set(keyVerdictDelivered, "true")
	taskIndex := ""
	if t := loop.GetCurrentTask(); t != nil {
		taskIndex = t.GetId()
	}
	if emitter := loop.GetEmitter(); emitter != nil {
		if _, err := emitter.EmitTextMarkdownStreamEvent(
			"re-act-loop-answer-payload",
			strings.NewReader(report),
			taskIndex,
			func() {},
		); err != nil {
			log.Warnf("[VulnVerify] minimal fallback markdown stream failed: %v", err)
		}
	}
	invoker.EmitResultAfterStream(report)
}
