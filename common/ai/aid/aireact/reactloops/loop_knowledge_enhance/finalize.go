package loop_knowledge_enhance

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

// maxFinalDocBytes is the maximum size for the final aggregated knowledge document
const maxFinalDocBytes = 50 * 1024 // 50KB

// generateFinalKnowledgeDocument aggregates all compressed knowledge from rounds
// into a single document limited to 50KB
func generateFinalKnowledgeDocument(loop *reactloops.ReActLoop, invoker aicommon.AIInvokeRuntime) {
	userQuery := loop.Get("user_query")
	finalSummary := loop.Get("final_summary") // 从 evaluateNextMovements 获取的总结
	maxIterations := loop.GetCurrentIterationIndex()

	// Collect all compressed results and artifact files
	var allCompressedResults []string
	var artifactFiles []string

	// Iterate through all possible rounds and queries
	for iteration := 1; iteration <= maxIterations+1; iteration++ {
		for queryIdx := 1; queryIdx <= 20; queryIdx++ { // Support up to 20 queries per iteration
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
		log.Infof("generateFinalKnowledgeDocument: no compressed results to aggregate")
		return
	}

	log.Infof("generateFinalKnowledgeDocument: aggregating %d compressed results from %d artifact files",
		len(allCompressedResults), len(artifactFiles))

	// Merge all results
	mergedContent := strings.Join(allCompressedResults, "\n\n---\n\n")

	// If total size exceeds 50KB, compress again
	if len(mergedContent) > maxFinalDocBytes {
		log.Infof("generateFinalKnowledgeDocument: merged content too large (%d bytes), compressing to %d bytes",
			len(mergedContent), maxFinalDocBytes)
		ctx := loop.GetConfig().GetContext()
		compressedContent, err := invoker.CompressLongTextWithDestination(ctx, mergedContent, userQuery, int64(maxFinalDocBytes))
		if err != nil {
			log.Warnf("generateFinalKnowledgeDocument: failed to compress merged content: %v", err)
			// keep original if compression fails
		} else {
			mergedContent = compressedContent
		}
	}

	// Get search history for metadata
	searchHistory := loop.Get("search_history")
	searchCountStr := loop.Get("search_count")

	// Get next movements summary
	nextMovementsSummary := loop.Get("next_movements_summary")

	// Build final document
	var finalDoc strings.Builder
	finalDoc.WriteString("# 知识增强查询报告\n\n")

	// User query section
	finalDoc.WriteString("## 用户问题\n\n")
	finalDoc.WriteString(userQuery)
	finalDoc.WriteString("\n\n")

	// Summary section (from evaluateNextMovements)
	if finalSummary != "" {
		finalDoc.WriteString("## 总体回答\n\n")
		finalDoc.WriteString(finalSummary)
		finalDoc.WriteString("\n\n")
	}

	// Metadata section
	finalDoc.WriteString("## 查询概况\n\n")
	finalDoc.WriteString(fmt.Sprintf("- **查询时间**: %s\n", time.Now().Format("2006-01-02 15:04:05")))
	finalDoc.WriteString(fmt.Sprintf("- **搜索轮次**: %s 次\n", searchCountStr))
	finalDoc.WriteString(fmt.Sprintf("- **生成的文档数**: %d\n", len(artifactFiles)))
	finalDoc.WriteString(fmt.Sprintf("- **压缩结果数**: %d\n", len(allCompressedResults)))
	finalDoc.WriteString(fmt.Sprintf("- **最终文档大小**: %d 字节\n\n", len(mergedContent)))

	// Search history section
	if searchHistory != "" {
		finalDoc.WriteString("## 搜索历史\n\n")
		finalDoc.WriteString("```\n")
		finalDoc.WriteString(searchHistory)
		finalDoc.WriteString("\n```\n\n")
	}

	// Main content section
	finalDoc.WriteString("## 详细知识内容\n\n")
	finalDoc.WriteString(mergedContent)
	finalDoc.WriteString("\n\n")

	// Next movements summary (for reference)
	if nextMovementsSummary != "" {
		finalDoc.WriteString("## 搜索过程中的建议记录\n\n")
		finalDoc.WriteString("<details>\n<summary>点击展开</summary>\n\n")
		finalDoc.WriteString(nextMovementsSummary)
		finalDoc.WriteString("\n\n</details>\n\n")
	}

	// Reference files section
	if len(artifactFiles) > 0 {
		finalDoc.WriteString("## 参考文件\n\n")
		for i, filename := range artifactFiles {
			finalDoc.WriteString(fmt.Sprintf("%d. `%s`\n", i+1, filename))
		}
		finalDoc.WriteString("\n")
	}

	// Ensure total size doesn't exceed limit
	finalContent := finalDoc.String()
	log.Infof("final content: \n%v", string(finalContent))
	const maxTotalBytes = 50 * 1024
	if len(finalContent) > maxTotalBytes {
		log.Warnf("generateFinalKnowledgeDocument: final report too large (%d bytes), truncating", len(finalContent))
		finalContent = finalContent[:maxTotalBytes-100] + "\n\n...(报告已截断，请查看详细文件)"
	}

	// Save final document
	finalFilename := invoker.EmitFileArtifactWithExt(
		fmt.Sprintf("knowledge_enhance_final_%s", utils.DatetimePretty2()),
		".md",
		"",
	)

	emitter := loop.GetEmitter()
	if emitter != nil {
		emitter.EmitPinFilename(finalFilename)
	}

	if err := os.WriteFile(finalFilename, []byte(finalContent), 0644); err != nil {
		log.Warnf("generateFinalKnowledgeDocument: failed to write final document: %v", err)
	} else {
		log.Infof("generateFinalKnowledgeDocument: final document saved to: %s (%d bytes)",
			finalFilename, len(finalContent))
	}

	// Record to timeline
	invoker.AddToTimeline("knowledge_search_finished", fmt.Sprintf("Final report saved to: %s\nSummary: %s", finalFilename, finalSummary))

	// Store final document path in loop context
	loop.Set("final_knowledge_document", finalFilename)
}

// BuildOnPostIterationHook creates the hook for generating final document when loop is done
func BuildOnPostIterationHook(invoker aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithOnPostIteraction(func(loop *reactloops.ReActLoop, iteration int, task aicommon.AIStatefulTask, isDone bool, reason any, operator *reactloops.OnPostIterationOperator) {
		if isDone {
			log.Infof("knowledge enhance loop done at iteration %d", iteration)

			// 检查是否因为超出迭代次数而结束
			if reasonErr, ok := reason.(error); ok && strings.Contains(reasonErr.Error(), "max iterations") {
				log.Infof("knowledge enhance loop ended due to max iterations, generating insufficient data report")
				// 生成"资料不足"报告
				generateInsufficientDataReport(loop, invoker)
				// 忽略错误，不让专注模式报错退出
				operator.IgnoreError()
			} else {
				// 正常结束，生成完整报告
				generateFinalKnowledgeDocument(loop, invoker)
			}
		}
	})
}

// generateInsufficientDataReport 生成资料不足报告
// 当循环因超出最大迭代次数而结束时调用
func generateInsufficientDataReport(loop *reactloops.ReActLoop, invoker aicommon.AIInvokeRuntime) {
	userQuery := loop.Get("user_query")
	searchHistory := loop.Get("search_history")
	searchResultsSummary := loop.Get("search_results_summary")
	searchCountStr := loop.Get("search_count")
	maxIterations := loop.GetCurrentIterationIndex()

	// Collect all compressed results and artifact files
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
	report.WriteString("# 知识查询报告（资料不足）\n\n")

	// User query section
	report.WriteString("## 用户问题\n\n")
	report.WriteString(userQuery)
	report.WriteString("\n\n")

	// Search status section
	report.WriteString("## 搜索情况\n\n")
	report.WriteString("⚠️ **注意**：已尝试多次搜索，但未能找到足够的相关资料来完整回答问题。\n\n")

	// Metadata
	report.WriteString("### 搜索概况\n\n")
	report.WriteString(fmt.Sprintf("- **查询时间**: %s\n", time.Now().Format("2006-01-02 15:04:05")))
	report.WriteString(fmt.Sprintf("- **搜索次数**: %s 次\n", searchCountStr))
	report.WriteString(fmt.Sprintf("- **达到最大迭代次数**: 是 (%d 次)\n", maxIterations))
	report.WriteString(fmt.Sprintf("- **找到的文档数**: %d\n\n", len(artifactFiles)))

	// Search history
	if searchHistory != "" {
		report.WriteString("### 搜索历史\n\n")
		report.WriteString("```\n")
		report.WriteString(searchHistory)
		report.WriteString("\n```\n\n")
	}

	// Partial results
	if searchResultsSummary != "" || len(allCompressedResults) > 0 {
		report.WriteString("### 已找到的部分信息\n\n")

		if searchResultsSummary != "" {
			report.WriteString(searchResultsSummary)
		} else if len(allCompressedResults) > 0 {
			// Merge all compressed results (limit to 20KB)
			mergedContent := strings.Join(allCompressedResults, "\n\n---\n\n")
			const maxPartialBytes = 20 * 1024
			if len(mergedContent) > maxPartialBytes {
				mergedContent = mergedContent[:maxPartialBytes-100] + "\n\n...(内容已截断)"
			}
			report.WriteString(mergedContent)
		}

		report.WriteString("\n\n> **注意**：以上信息可能不足以完整回答问题，仅供参考。\n\n")
	} else {
		report.WriteString("### 搜索结果\n\n")
		report.WriteString("未找到与问题直接相关的信息。\n\n")
	}

	// Reference files
	if len(artifactFiles) > 0 {
		report.WriteString("## 参考文件\n\n")
		for i, filename := range artifactFiles {
			report.WriteString(fmt.Sprintf("%d. `%s`\n", i+1, filename))
		}
		report.WriteString("\n")
	}

	// Suggestions
	report.WriteString("## 建议\n\n")
	report.WriteString("1. 尝试使用不同的关键词或查询方式\n")
	report.WriteString("2. 检查知识库是否包含相关主题的内容\n")
	report.WriteString("3. 考虑扩大搜索范围或添加更多知识库\n")

	// Ensure total size doesn't exceed limit
	finalContent := report.String()
	const maxTotalBytes = 50 * 1024
	if len(finalContent) > maxTotalBytes {
		log.Warnf("generateInsufficientDataReport: report too large (%d bytes), truncating", len(finalContent))
		finalContent = finalContent[:maxTotalBytes-100] + "\n\n...(报告已截断)"
	}

	// Save report
	finalFilename := invoker.EmitFileArtifactWithExt(
		fmt.Sprintf("knowledge_enhance_insufficient_%s", utils.DatetimePretty2()),
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

	// Record to timeline
	invoker.AddToTimeline("knowledge_search_insufficient", fmt.Sprintf("Insufficient data report saved to: %s", finalFilename))

	// Store report path in loop context
	loop.Set("final_knowledge_document", finalFilename)
	loop.Set("knowledge_search_status", "insufficient")
}
