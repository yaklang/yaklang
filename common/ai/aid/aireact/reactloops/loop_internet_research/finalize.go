package loop_internet_research

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

const maxFinalDocBytes = 50 * 1024

func BuildOnPostIterationHook(invoker aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithOnPostIteraction(func(loop *reactloops.ReActLoop, iteration int, task aicommon.AIStatefulTask, isDone bool, reason any, operator *reactloops.OnPostIterationOperator) {
		if !isDone {
			return
		}

		log.Infof("internet research loop done at iteration %d", iteration)

		isMaxIterations := false
		if reasonErr, ok := reason.(error); ok && strings.Contains(reasonErr.Error(), "max iterations") {
			isMaxIterations = true
			operator.IgnoreError()
		}

		generateAndOutputFinalReport(loop, invoker, isMaxIterations)
	})
}

type smartEvaluation struct {
	Specific   string
	Measurable string
	Achievable string
	Relevant   string
	TimeBound  string
	Overall    string
}

func evaluateSMART(ctx context.Context, invoker aicommon.AIInvokeRuntime, userQuery, searchResults, searchHistory string) *smartEvaluation {
	dNonce := utils.RandStringBytes(4)

	resultPreview := searchResults
	if len(resultPreview) > 4096 {
		resultPreview = resultPreview[:4096] + "\n...(truncated)"
	}

	promptTemplate := `<|SMART_EVAL_{{ .nonce }}|>
Evaluate the following internet research results using the S.M.A.R.T framework.

User Query:
{{ .userQuery }}

Search History:
{{ .searchHistory }}

Collected Results Summary:
{{ .searchResults }}

For each S.M.A.R.T dimension, provide a brief evaluation (1-2 sentences) of the search results:
- Specific: How specific and targeted are the search results relative to the user's question?
- Measurable: Can the information be verified? Are there concrete data points, dates, numbers, or citations?
- Achievable: Did the research achieve its goal of answering the user's question? What percentage of the question was answered?
- Relevant: How relevant is the collected information to the user's actual needs?
- Time-bound: Is the information current and timely? Are the sources up-to-date?
- Overall: A brief overall assessment of the research quality (1 sentence).
<|SMART_EVAL_END_{{ .nonce }}|>
`

	materials, err := utils.RenderTemplate(promptTemplate, map[string]any{
		"nonce":         dNonce,
		"userQuery":     userQuery,
		"searchHistory": searchHistory,
		"searchResults": resultPreview,
	})
	if err != nil {
		log.Warnf("SMART evaluation template render failed: %v", err)
		return nil
	}

	forgeResult, err := invoker.InvokeSpeedPriorityLiteForge(
		ctx,
		"smart-evaluation",
		materials,
		[]aitool.ToolOption{
			aitool.WithStringParam("specific", aitool.WithParam_Description("Specific: how specific are results"), aitool.WithParam_Required(true)),
			aitool.WithStringParam("measurable", aitool.WithParam_Description("Measurable: verifiable data points"), aitool.WithParam_Required(true)),
			aitool.WithStringParam("achievable", aitool.WithParam_Description("Achievable: did research achieve its goal"), aitool.WithParam_Required(true)),
			aitool.WithStringParam("relevant", aitool.WithParam_Description("Relevant: relevance to user needs"), aitool.WithParam_Required(true)),
			aitool.WithStringParam("time_bound", aitool.WithParam_Description("Time-bound: information timeliness"), aitool.WithParam_Required(true)),
			aitool.WithStringParam("overall", aitool.WithParam_Description("Overall assessment in 1 sentence"), aitool.WithParam_Required(true)),
		},
	)
	if err != nil {
		log.Warnf("SMART evaluation LiteForge failed: %v", err)
		return nil
	}
	if forgeResult == nil {
		return nil
	}

	return &smartEvaluation{
		Specific:   strings.TrimSpace(forgeResult.GetString("specific")),
		Measurable: strings.TrimSpace(forgeResult.GetString("measurable")),
		Achievable: strings.TrimSpace(forgeResult.GetString("achievable")),
		Relevant:   strings.TrimSpace(forgeResult.GetString("relevant")),
		TimeBound:  strings.TrimSpace(forgeResult.GetString("time_bound")),
		Overall:    strings.TrimSpace(forgeResult.GetString("overall")),
	}
}

func evaluateInsufficientReason(ctx context.Context, invoker aicommon.AIInvokeRuntime, userQuery, searchResults, searchHistory string) string {
	dNonce := utils.RandStringBytes(4)

	resultPreview := searchResults
	if len(resultPreview) > 2048 {
		resultPreview = resultPreview[:2048] + "\n...(truncated)"
	}

	promptTemplate := `<|INSUFFICIENT_EVAL_{{ .nonce }}|>
The following internet research did not yield sufficient results to fully answer the user's question.

User Query:
{{ .userQuery }}

Search History:
{{ .searchHistory }}

Partial Results (if any):
{{ .searchResults }}

Please analyze why the search results are insufficient. Consider:
1. What specific aspects of the user's question remain unanswered?
2. What was found vs what was expected?
3. Possible reasons (topic too niche, information not publicly available, wrong search strategy, etc.)

Provide a concise analysis (3-5 sentences) explaining why the collected information does not meet the user's needs.
<|INSUFFICIENT_EVAL_END_{{ .nonce }}|>
`

	materials, err := utils.RenderTemplate(promptTemplate, map[string]any{
		"nonce":         dNonce,
		"userQuery":     userQuery,
		"searchHistory": searchHistory,
		"searchResults": resultPreview,
	})
	if err != nil {
		return ""
	}

	forgeResult, err := invoker.InvokeSpeedPriorityLiteForge(
		ctx,
		"insufficient-reason-analysis",
		materials,
		[]aitool.ToolOption{
			aitool.WithStringParam("analysis", aitool.WithParam_Description("Analysis of why results are insufficient"), aitool.WithParam_Required(true)),
		},
	)
	if err != nil || forgeResult == nil {
		return ""
	}

	return strings.TrimSpace(forgeResult.GetString("analysis"))
}

func collectResearchData(loop *reactloops.ReActLoop) (allCompressedResults []string, artifactFiles []string) {
	maxIterations := loop.GetCurrentIterationIndex()
	if maxIterations <= 0 {
		maxIterations = 5
	}

	for iteration := 1; iteration <= maxIterations+1; iteration++ {
		for queryIdx := 1; queryIdx <= 20; queryIdx++ {
			compressedResult := loop.Get(fmt.Sprintf("compressed_result_round_%d_%d", iteration, queryIdx))
			artifactFile := loop.Get(fmt.Sprintf("artifact_round_%d_%d", iteration, queryIdx))
			if compressedResult != "" {
				allCompressedResults = append(allCompressedResults, compressedResult)
			}
			if artifactFile != "" {
				artifactFiles = append(artifactFiles, artifactFile)
			}
		}
	}
	return
}

func generateAndOutputFinalReport(loop *reactloops.ReActLoop, invoker aicommon.AIInvokeRuntime, isMaxIterations bool) {
	userQuery := loop.Get("user_query")
	finalSummary := loop.Get("final_summary")
	searchHistory := loop.Get("search_history")
	searchResultsSummary := loop.Get("search_results_summary")
	searchCountStr := loop.Get("search_count")

	allCompressedResults, artifactFiles := collectResearchData(loop)
	hasResults := len(allCompressedResults) > 0 && searchResultsSummary != ""

	ctx := loop.GetConfig().GetContext()

	var report strings.Builder

	if hasResults {
		report.WriteString("# Internet Research Report\n\n")

		report.WriteString("## User Query\n\n")
		report.WriteString(userQuery)
		report.WriteString("\n\n")

		report.WriteString("## Research Overview\n\n")
		report.WriteString(fmt.Sprintf("- **Search Count**: %s\n", searchCountStr))
		report.WriteString(fmt.Sprintf("- **Documents Generated**: %d\n", len(artifactFiles)))
		report.WriteString(fmt.Sprintf("- **Time**: %s\n", time.Now().Format("2006-01-02 15:04:05")))
		if isMaxIterations {
			report.WriteString("- **Status**: Reached maximum iteration limit\n")
		} else {
			report.WriteString("- **Status**: Completed\n")
		}
		report.WriteString("\n")

		if finalSummary != "" {
			report.WriteString("## Key Findings\n\n")
			report.WriteString(finalSummary)
			report.WriteString("\n\n")
		}

		if searchHistory != "" {
			report.WriteString("## Search History\n\n")
			report.WriteString("```\n")
			report.WriteString(searchHistory)
			report.WriteString("\n```\n\n")
		}

		mergedContent := strings.Join(allCompressedResults, "\n\n---\n\n")
		if len(mergedContent) > maxFinalDocBytes/2 {
			compressed, err := invoker.CompressLongTextWithDestination(ctx, mergedContent, userQuery, int64(maxFinalDocBytes/2))
			if err == nil {
				mergedContent = compressed
			}
		}

		report.WriteString("## Sources & Content\n\n")
		report.WriteString(mergedContent)
		report.WriteString("\n\n")

		smart := evaluateSMART(ctx, invoker, userQuery, searchResultsSummary, searchHistory)
		report.WriteString("## S.M.A.R.T Evaluation\n\n")
		if smart != nil {
			report.WriteString("| Dimension | Evaluation |\n")
			report.WriteString("|-----------|------------|\n")
			report.WriteString(fmt.Sprintf("| **S**pecific | %s |\n", smart.Specific))
			report.WriteString(fmt.Sprintf("| **M**easurable | %s |\n", smart.Measurable))
			report.WriteString(fmt.Sprintf("| **A**chievable | %s |\n", smart.Achievable))
			report.WriteString(fmt.Sprintf("| **R**elevant | %s |\n", smart.Relevant))
			report.WriteString(fmt.Sprintf("| **T**ime-bound | %s |\n", smart.TimeBound))
			report.WriteString(fmt.Sprintf("\n**Overall**: %s\n\n", smart.Overall))
		} else {
			report.WriteString("S.M.A.R.T evaluation could not be generated.\n\n")
		}

		if len(artifactFiles) > 0 {
			report.WriteString("## Reference Files\n\n")
			for i, filename := range artifactFiles {
				report.WriteString(fmt.Sprintf("%d. `%s`\n", i+1, filename))
			}
			report.WriteString("\n")
		}

	} else {
		report.WriteString("# Internet Research Report (Insufficient Results)\n\n")

		report.WriteString("## User Query\n\n")
		report.WriteString(userQuery)
		report.WriteString("\n\n")

		report.WriteString("## Research Overview\n\n")
		report.WriteString(fmt.Sprintf("- **Search Count**: %s\n", searchCountStr))
		report.WriteString(fmt.Sprintf("- **Time**: %s\n", time.Now().Format("2006-01-02 15:04:05")))
		report.WriteString("- **Status**: Insufficient relevant results\n\n")

		if searchHistory != "" {
			report.WriteString("## Search History\n\n")
			report.WriteString("```\n")
			report.WriteString(searchHistory)
			report.WriteString("\n```\n\n")
		}

		if searchResultsSummary != "" || len(allCompressedResults) > 0 {
			report.WriteString("## Partial Results Found\n\n")
			if searchResultsSummary != "" {
				report.WriteString(searchResultsSummary)
			} else {
				mergedContent := strings.Join(allCompressedResults, "\n\n---\n\n")
				if len(mergedContent) > 10*1024 {
					mergedContent = mergedContent[:10*1024] + "\n...(truncated)"
				}
				report.WriteString(mergedContent)
			}
			report.WriteString("\n\n")
		} else {
			report.WriteString("## Search Results\n\n")
			report.WriteString("No relevant information was found for the given query.\n\n")
		}

		report.WriteString("## Analysis: Why Results Are Insufficient\n\n")
		insufficientReason := evaluateInsufficientReason(ctx, invoker, userQuery, searchResultsSummary, searchHistory)
		if insufficientReason != "" {
			report.WriteString(insufficientReason)
		} else {
			report.WriteString("The search results did not contain sufficient information directly relevant to the user's question. ")
			report.WriteString("This may be due to the topic being too niche, the information not being publicly available, ")
			report.WriteString("or the search keywords not matching the available content.\n")
		}
		report.WriteString("\n\n")

		report.WriteString("## Suggestions\n\n")
		report.WriteString("1. Try using different keywords or phrasing\n")
		report.WriteString("2. Consider searching in a different language\n")
		report.WriteString("3. Break down the question into smaller, more specific queries\n")
		report.WriteString("4. Check if the information is available through specialized databases\n\n")

		if len(artifactFiles) > 0 {
			report.WriteString("## Reference Files\n\n")
			for i, filename := range artifactFiles {
				report.WriteString(fmt.Sprintf("%d. `%s`\n", i+1, filename))
			}
			report.WriteString("\n")
		}
	}

	finalContent := report.String()
	if len(finalContent) > maxFinalDocBytes {
		finalContent = finalContent[:maxFinalDocBytes-100] + "\n\n...(report truncated, see detailed files)"
	}

	log.Infof("internet research final report size: %d bytes, hasResults: %v", len(finalContent), hasResults)

	finalFilename := invoker.EmitFileArtifactWithExt(
		fmt.Sprintf("internet_research_report_%s", utils.DatetimePretty2()),
		".md",
		"",
	)

	emitter := loop.GetEmitter()
	if emitter != nil {
		emitter.EmitPinFilename(finalFilename)
	}

	if err := os.WriteFile(finalFilename, []byte(finalContent), 0644); err != nil {
		log.Warnf("failed to write final research report: %v", err)
	} else {
		log.Infof("final research report saved to: %s (%d bytes)", finalFilename, len(finalContent))
	}

	task := loop.GetCurrentTask()
	var taskCtx context.Context
	if task != nil && !utils.IsNil(task.GetContext()) {
		taskCtx = task.GetContext()
	} else {
		taskCtx = ctx
	}

	result, err := invoker.DirectlyAnswer(taskCtx, finalContent, nil)
	if err != nil {
		log.Warnf("failed to directly answer with research report: %v", err)
	} else {
		log.Infof("research report delivered to user, response: %s", utils.ShrinkTextBlock(result, 512))
	}

	statusStr := "completed"
	if !hasResults {
		statusStr = "insufficient"
	}
	invoker.AddToTimeline("internet_research_report",
		fmt.Sprintf("Internet research %s. Report saved to: %s", statusStr, finalFilename))

	loop.Set("final_research_document", finalFilename)
	loop.Set("research_status", statusStr)
}
