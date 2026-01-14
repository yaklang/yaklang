package loop_knowledge_enhance

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// finishKnowledgeSearchAction 允许 AI 主动结束知识搜索并生成总结
var finishKnowledgeSearchAction = func(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"finish_knowledge_search",
		"完成知识收集并生成总体回答。当认为已收集足够信息时调用此action。",
		[]aitool.ToolOption{
			aitool.WithStringParam("summary", aitool.WithParam_Description("对所收集知识的整体总结，回答用户的原始问题"), aitool.WithParam_Required(true)),
			aitool.WithStringParam("key_findings", aitool.WithParam_Description("关键发现要点，用分号分隔")),
			aitool.WithStringParam("recommendations", aitool.WithParam_Description("基于收集的知识给出的建议")),
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action) error {
			loop.LoadingStatus("验证完成参数 - validating finish parameters")
			summary := action.GetString("summary")
			if summary == "" {
				return utils.Error("summary is required to finish knowledge search")
			}
			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			loop.LoadingStatus("生成最终报告 - generating final report")

			summary := action.GetString("summary")
			keyFindings := action.GetString("key_findings")
			recommendations := action.GetString("recommendations")

			invoker := loop.GetInvoker()
			emitter := loop.GetEmitter()

			userQuery := loop.Get("user_query")
			searchHistory := loop.Get("search_history")
			allResults := loop.Get("all_compressed_results")
			nextMovementsSummary := loop.Get("next_movements_summary")
			searchCountStr := loop.Get("search_count")

			// 获取所有 artifact 文件
			iteration := loop.GetCurrentIterationIndex()
			if iteration <= 0 {
				iteration = 1
			}

			var artifactFiles []string
			searchCount := 0
			if searchCountStr != "" {
				if c, err := strconv.Atoi(searchCountStr); err == nil {
					searchCount = c
				}
			}

			for i := 1; i <= iteration; i++ {
				for j := 1; j <= searchCount+1; j++ {
					artifactFile := loop.Get(fmt.Sprintf("artifact_round_%d_%d", i, j))
					if artifactFile != "" {
						artifactFiles = append(artifactFiles, artifactFile)
					}
				}
			}

			// 构建最终报告
			var reportBuilder strings.Builder

			reportBuilder.WriteString(fmt.Sprintf(`# 知识增强查询报告

## 用户问题

%s

## 查询概况

- 查询时间: %s
- 搜索轮次: %d 次
- 生成的文档数: %d

## 搜索历史

%s

`, userQuery, time.Now().Format("2006-01-02 15:04:05"), searchCount, len(artifactFiles), searchHistory))

			// 添加总结
			reportBuilder.WriteString("## 总体回答\n\n")
			reportBuilder.WriteString(summary)
			reportBuilder.WriteString("\n\n")

			// 添加关键发现
			if keyFindings != "" {
				reportBuilder.WriteString("## 关键发现\n\n")
				findings := strings.Split(keyFindings, ";")
				for i, f := range findings {
					f = strings.TrimSpace(f)
					if f != "" {
						reportBuilder.WriteString(fmt.Sprintf("%d. %s\n", i+1, f))
					}
				}
				reportBuilder.WriteString("\n")
			}

			// 添加建议
			if recommendations != "" {
				reportBuilder.WriteString("## 建议\n\n")
				reportBuilder.WriteString(recommendations)
				reportBuilder.WriteString("\n\n")
			}

			// 添加详细知识内容（如果不超过 50KB）
			if allResults != "" {
				const maxDetailBytes = 40 * 1024 // 预留空间给其他内容
				detailResults := allResults
				if len(detailResults) > maxDetailBytes {
					// 使用压缩
					log.Infof("all results too large (%d bytes), compressing to 40KB", len(detailResults))
					detailResults = compressKnowledgeResultsWithScore(detailResults, userQuery, invoker, loop, maxDetailBytes)
				}
				reportBuilder.WriteString("## 详细知识内容\n\n")
				reportBuilder.WriteString(detailResults)
				reportBuilder.WriteString("\n\n")
			}

			// 添加下一步建议历史（仅供参考）
			if nextMovementsSummary != "" {
				reportBuilder.WriteString("## 搜索过程中的建议记录\n\n")
				reportBuilder.WriteString("<details>\n<summary>点击展开</summary>\n\n")
				reportBuilder.WriteString(nextMovementsSummary)
				reportBuilder.WriteString("\n\n</details>\n\n")
			}

			// 添加参考文件列表
			if len(artifactFiles) > 0 {
				reportBuilder.WriteString("## 参考文件\n\n")
				for _, f := range artifactFiles {
					reportBuilder.WriteString(fmt.Sprintf("- `%s`\n", f))
				}
				reportBuilder.WriteString("\n")
			}

			finalReport := reportBuilder.String()

			// 确保总大小不超过 50KB
			const maxTotalBytes = 50 * 1024
			if len(finalReport) > maxTotalBytes {
				log.Warnf("final report too large (%d bytes), truncating", len(finalReport))
				finalReport = finalReport[:maxTotalBytes-100] + "\n\n...(报告已截断，请查看详细文件)"
			}

			// 保存最终报告
			finalFilename := invoker.EmitFileArtifactWithExt(
				fmt.Sprintf("knowledge_enhance_final_%s", utils.DatetimePretty2()),
				".md",
				"",
			)
			emitter.EmitPinFilename(finalFilename)

			if err := os.WriteFile(finalFilename, []byte(finalReport), 0644); err != nil {
				log.Warnf("failed to write final report: %v", err)
			} else {
				log.Infof("final knowledge report saved to: %s", finalFilename)
			}

			// 记录到 timeline
			invoker.AddToTimeline("knowledge_search_finished", fmt.Sprintf("Final report saved to: %s\nSummary: %s", finalFilename, summary))

			// 输出反馈
			feedback := fmt.Sprintf(`=== 知识收集完成 ===

最终报告已保存到: %s
报告大小: %d bytes

## 总结

%s

知识增强查询已完成。
`, finalFilename, len(finalReport), summary)

			op.Feedback(feedback)
			op.Exit() // 正式退出循环
		},
	)
}
