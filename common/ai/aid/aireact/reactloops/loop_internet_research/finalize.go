package loop_internet_research

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

const maxFinalDocBytes = 50 * 1024

func generateFinalResearchDocument(loop *reactloops.ReActLoop, invoker aicommon.AIInvokeRuntime) {
	userQuery := loop.Get("user_query")
	finalSummary := loop.Get("final_summary")
	maxIterations := loop.GetCurrentIterationIndex()

	var allCompressedResults []string
	var artifactFiles []string

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

	if len(allCompressedResults) == 0 {
		log.Infof("generateFinalResearchDocument: no compressed results to aggregate")
		return
	}

	log.Infof("generateFinalResearchDocument: aggregating %d compressed results from %d artifact files",
		len(allCompressedResults), len(artifactFiles))

	mergedContent := strings.Join(allCompressedResults, "\n\n---\n\n")

	if len(mergedContent) > maxFinalDocBytes {
		log.Infof("generateFinalResearchDocument: merged content too large (%d bytes), compressing to %d bytes",
			len(mergedContent), maxFinalDocBytes)
		ctx := loop.GetConfig().GetContext()
		compressedContent, err := invoker.CompressLongTextWithDestination(ctx, mergedContent, userQuery, int64(maxFinalDocBytes))
		if err != nil {
			log.Warnf("generateFinalResearchDocument: failed to compress merged content: %v", err)
		} else {
			mergedContent = compressedContent
		}
	}

	searchHistory := loop.Get("search_history")
	searchCountStr := loop.Get("search_count")
	nextMovementsSummary := loop.Get("next_movements_summary")

	var finalDoc strings.Builder
	finalDoc.WriteString("# Internet Research Report\n\n")

	finalDoc.WriteString("## User Query\n\n")
	finalDoc.WriteString(userQuery)
	finalDoc.WriteString("\n\n")

	if finalSummary != "" {
		finalDoc.WriteString("## Summary\n\n")
		finalDoc.WriteString(finalSummary)
		finalDoc.WriteString("\n\n")
	}

	finalDoc.WriteString("## Research Overview\n\n")
	finalDoc.WriteString(fmt.Sprintf("- **Time**: %s\n", time.Now().Format("2006-01-02 15:04:05")))
	finalDoc.WriteString(fmt.Sprintf("- **Search Count**: %s\n", searchCountStr))
	finalDoc.WriteString(fmt.Sprintf("- **Documents Generated**: %d\n", len(artifactFiles)))
	finalDoc.WriteString(fmt.Sprintf("- **Compressed Results**: %d\n", len(allCompressedResults)))
	finalDoc.WriteString(fmt.Sprintf("- **Final Document Size**: %d bytes\n\n", len(mergedContent)))

	if searchHistory != "" {
		finalDoc.WriteString("## Search History\n\n")
		finalDoc.WriteString("```\n")
		finalDoc.WriteString(searchHistory)
		finalDoc.WriteString("\n```\n\n")
	}

	finalDoc.WriteString("## Detailed Research Content\n\n")
	finalDoc.WriteString(mergedContent)
	finalDoc.WriteString("\n\n")

	if nextMovementsSummary != "" {
		finalDoc.WriteString("## Research Process Notes\n\n")
		finalDoc.WriteString("<details>\n<summary>Click to expand</summary>\n\n")
		finalDoc.WriteString(nextMovementsSummary)
		finalDoc.WriteString("\n\n</details>\n\n")
	}

	if len(artifactFiles) > 0 {
		finalDoc.WriteString("## Reference Files\n\n")
		for i, filename := range artifactFiles {
			finalDoc.WriteString(fmt.Sprintf("%d. `%s`\n", i+1, filename))
		}
		finalDoc.WriteString("\n")
	}

	finalContent := finalDoc.String()
	if len(finalContent) > maxFinalDocBytes {
		log.Warnf("generateFinalResearchDocument: final report too large (%d bytes), truncating", len(finalContent))
		finalContent = finalContent[:maxFinalDocBytes-100] + "\n\n...(report truncated, see detailed files)"
	}

	finalFilename := invoker.EmitFileArtifactWithExt(
		fmt.Sprintf("internet_research_final_%s", utils.DatetimePretty2()),
		".md",
		"",
	)

	emitter := loop.GetEmitter()
	if emitter != nil {
		emitter.EmitPinFilename(finalFilename)
	}

	if err := os.WriteFile(finalFilename, []byte(finalContent), 0644); err != nil {
		log.Warnf("generateFinalResearchDocument: failed to write final document: %v", err)
	} else {
		log.Infof("generateFinalResearchDocument: final document saved to: %s (%d bytes)",
			finalFilename, len(finalContent))
	}

	invoker.AddToTimeline("internet_research_finished", fmt.Sprintf("Final report saved to: %s\nSummary: %s", finalFilename, finalSummary))
	loop.Set("final_research_document", finalFilename)
}

func BuildOnPostIterationHook(invoker aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithOnPostIteraction(func(loop *reactloops.ReActLoop, iteration int, task aicommon.AIStatefulTask, isDone bool, reason any, operator *reactloops.OnPostIterationOperator) {
		if isDone {
			log.Infof("internet research loop done at iteration %d", iteration)

			if reasonErr, ok := reason.(error); ok && strings.Contains(reasonErr.Error(), "max iterations") {
				log.Infof("internet research loop ended due to max iterations, generating insufficient data report")
				generateInsufficientDataReport(loop, invoker)
				operator.IgnoreError()
			} else {
				generateFinalResearchDocument(loop, invoker)
			}
		}
	})
}

func generateInsufficientDataReport(loop *reactloops.ReActLoop, invoker aicommon.AIInvokeRuntime) {
	userQuery := loop.Get("user_query")
	searchHistory := loop.Get("search_history")
	searchResultsSummary := loop.Get("search_results_summary")
	searchCountStr := loop.Get("search_count")
	maxIterations := loop.GetCurrentIterationIndex()

	var allCompressedResults []string
	var artifactFiles []string

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

	var report strings.Builder
	report.WriteString("# Internet Research Report (Insufficient Data)\n\n")

	report.WriteString("## User Query\n\n")
	report.WriteString(userQuery)
	report.WriteString("\n\n")

	report.WriteString("## Research Status\n\n")
	report.WriteString("**Note**: Multiple searches were attempted but insufficient relevant information was found.\n\n")

	report.WriteString("### Overview\n\n")
	report.WriteString(fmt.Sprintf("- **Time**: %s\n", time.Now().Format("2006-01-02 15:04:05")))
	report.WriteString(fmt.Sprintf("- **Search Count**: %s\n", searchCountStr))
	report.WriteString(fmt.Sprintf("- **Max Iterations Reached**: Yes (%d)\n", maxIterations))
	report.WriteString(fmt.Sprintf("- **Documents Found**: %d\n\n", len(artifactFiles)))

	if searchHistory != "" {
		report.WriteString("### Search History\n\n")
		report.WriteString("```\n")
		report.WriteString(searchHistory)
		report.WriteString("\n```\n\n")
	}

	if searchResultsSummary != "" || len(allCompressedResults) > 0 {
		report.WriteString("### Partial Information Found\n\n")

		if searchResultsSummary != "" {
			report.WriteString(searchResultsSummary)
		} else if len(allCompressedResults) > 0 {
			mergedContent := strings.Join(allCompressedResults, "\n\n---\n\n")
			const maxPartialBytes = 20 * 1024
			if len(mergedContent) > maxPartialBytes {
				mergedContent = mergedContent[:maxPartialBytes-100] + "\n\n...(content truncated)"
			}
			report.WriteString(mergedContent)
		}

		report.WriteString("\n\n> **Note**: The above information may be incomplete.\n\n")
	} else {
		report.WriteString("### Search Results\n\n")
		report.WriteString("No directly relevant information was found.\n\n")
	}

	if len(artifactFiles) > 0 {
		report.WriteString("## Reference Files\n\n")
		for i, filename := range artifactFiles {
			report.WriteString(fmt.Sprintf("%d. `%s`\n", i+1, filename))
		}
		report.WriteString("\n")
	}

	report.WriteString("## Suggestions\n\n")
	report.WriteString("1. Try using different keywords or search queries\n")
	report.WriteString("2. Consider searching in a different language\n")
	report.WriteString("3. Try breaking down the question into smaller, more specific queries\n")

	finalContent := report.String()
	if len(finalContent) > maxFinalDocBytes {
		finalContent = finalContent[:maxFinalDocBytes-100] + "\n\n...(report truncated)"
	}

	finalFilename := invoker.EmitFileArtifactWithExt(
		fmt.Sprintf("internet_research_insufficient_%s", utils.DatetimePretty2()),
		".md",
		"",
	)

	emitter := loop.GetEmitter()
	if emitter != nil {
		emitter.EmitPinFilename(finalFilename)
	}

	if err := os.WriteFile(finalFilename, []byte(finalContent), 0644); err != nil {
		log.Warnf("generateInsufficientDataReport: failed to write report: %v", err)
	} else {
		log.Infof("generateInsufficientDataReport: report saved to: %s (%d bytes)",
			finalFilename, len(finalContent))
	}

	invoker.AddToTimeline("internet_research_insufficient", fmt.Sprintf("Insufficient data report saved to: %s", finalFilename))
	loop.Set("final_research_document", finalFilename)
	loop.Set("research_status", "insufficient")
}
