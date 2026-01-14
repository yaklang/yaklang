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
		mergedContent = compressKnowledgeResultsWithScore(
			mergedContent,
			userQuery,
			invoker,
			loop,
			maxFinalDocBytes,
		)
	}

	// Get search history for metadata
	searchHistory := loop.Get("search_history")

	// Get next movements summary
	nextMovementsSummary := loop.Get("next_movements_summary")

	// Build final document
	var finalDoc strings.Builder
	finalDoc.WriteString("# 知识增强最终整合文档\n\n")

	// Metadata section
	finalDoc.WriteString("## 元信息\n\n")
	finalDoc.WriteString(fmt.Sprintf("- **用户查询**: %s\n", userQuery))
	finalDoc.WriteString(fmt.Sprintf("- **查询轮数**: %d\n", maxIterations))
	finalDoc.WriteString(fmt.Sprintf("- **生成时间**: %s\n", time.Now().Format("2006-01-02 15:04:05")))
	finalDoc.WriteString(fmt.Sprintf("- **压缩结果数**: %d\n", len(allCompressedResults)))
	finalDoc.WriteString(fmt.Sprintf("- **最终文档大小**: %d 字节\n\n", len(mergedContent)))

	// Search history section
	if searchHistory != "" {
		finalDoc.WriteString("## 查询历史\n\n")
		finalDoc.WriteString("```\n")
		finalDoc.WriteString(searchHistory)
		finalDoc.WriteString("\n```\n\n")
	}

	// Main content section
	finalDoc.WriteString("## 整合后的关键知识内容\n\n")
	finalDoc.WriteString(mergedContent)
	finalDoc.WriteString("\n\n")

	// Next movements summary
	if nextMovementsSummary != "" {
		finalDoc.WriteString("## 下一步建议汇总\n\n")
		finalDoc.WriteString(nextMovementsSummary)
		finalDoc.WriteString("\n\n")
	}

	// Reference files section
	if len(artifactFiles) > 0 {
		finalDoc.WriteString("## 参考文件列表\n\n")
		for i, filename := range artifactFiles {
			finalDoc.WriteString(fmt.Sprintf("%d. `%s`\n", i+1, filename))
		}
		finalDoc.WriteString("\n")
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

	if err := os.WriteFile(finalFilename, []byte(finalDoc.String()), 0644); err != nil {
		log.Warnf("generateFinalKnowledgeDocument: failed to write final document: %v", err)
	} else {
		log.Infof("generateFinalKnowledgeDocument: final document saved to: %s (%d bytes)",
			finalFilename, len(finalDoc.String()))
	}

	// Store final document path in loop context
	loop.Set("final_knowledge_document", finalFilename)
}

// BuildOnPostIterationHook creates the hook for generating final document when loop is done
func BuildOnPostIterationHook(invoker aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithOnPostIteraction(func(loop *reactloops.ReActLoop, iteration int, task aicommon.AIStatefulTask, isDone bool, reason any, operator *reactloops.OnPostIterationOperator) {
		if isDone {
			log.Infof("knowledge enhance loop done at iteration %d, generating final document", iteration)
			generateFinalKnowledgeDocument(loop, invoker)
		}
	})
}
