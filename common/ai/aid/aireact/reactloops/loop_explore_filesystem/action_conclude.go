package loop_explore_filesystem

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// concludeExplorationAction creates the action for concluding the exploration and providing final summary
var concludeExplorationAction = func(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopActionWithStreamField(
		"conclude_exploration",
		`Conclude Exploration - 总结探索发现并给出结论

【功能说明】
当探索任务完成时，使用此 action 来：
1. 总结所有发现的有价值代码片段
2. 回答用户的探索问题
3. 提供代码结构分析和建议

【参数说明】
- conclusion (必需): 详细的结论说明
- key_findings (必需): 关键发现的列表
- code_references (可选): 相关代码引用列表
- recommendations (可选): 建议和后续步骤

【使用时机】
- 已找到足够信息回答用户问题
- 探索目标已达成
- 需要总结多次 grep 的发现`,
		[]aitool.ToolOption{
			aitool.WithStringParam("conclusion",
				aitool.WithParam_Required(true),
				aitool.WithParam_Description("Detailed conclusion answering the user's exploration question")),
			aitool.WithStringArrayParam("key_findings",
				aitool.WithParam_Required(true),
				aitool.WithParam_Description("List of key findings from the exploration")),
			aitool.WithStringArrayParam("code_references",
				aitool.WithParam_Description("List of relevant code file paths and line references")),
			aitool.WithStringArrayParam("recommendations",
				aitool.WithParam_Description("Suggestions for further exploration or next steps")),
		},
		[]*reactloops.LoopStreamField{
			{
				FieldName: "conclusion",
				AINodeId:  "exploration-conclusion",
			},
		},
		// Validator
		func(loop *reactloops.ReActLoop, action *aicommon.Action) error {
			conclusion := action.GetString("conclusion")
			keyFindings := action.GetStringSlice("key_findings")

			if conclusion == "" {
				return utils.Error("conclude_exploration requires 'conclusion' parameter")
			}
			if len(keyFindings) == 0 {
				return utils.Error("conclude_exploration requires at least one key finding")
			}

			return nil
		},
		// Handler
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			conclusion := action.GetString("conclusion")
			keyFindings := action.GetStringSlice("key_findings")
			codeReferences := action.GetStringSlice("code_references")
			recommendations := action.GetStringSlice("recommendations")

			invoker := loop.GetInvoker()
			emitter := loop.GetEmitter()

			// Build final summary
			var summaryBuilder strings.Builder

			summaryBuilder.WriteString("# 文件系统探索结论\n\n")

			// Add exploration context
			targetPath := loop.Get("target_path")
			explorationGoal := loop.Get("exploration_goal")
			summaryBuilder.WriteString(fmt.Sprintf("## 探索上下文\n"))
			summaryBuilder.WriteString(fmt.Sprintf("- **目标路径**: %s\n", targetPath))
			summaryBuilder.WriteString(fmt.Sprintf("- **探索目标**: %s\n\n", explorationGoal))

			// Add conclusion
			summaryBuilder.WriteString(fmt.Sprintf("## 结论\n%s\n\n", conclusion))

			// Add key findings
			summaryBuilder.WriteString("## 关键发现\n")
			for i, finding := range keyFindings {
				summaryBuilder.WriteString(fmt.Sprintf("%d. %s\n", i+1, finding))
			}
			summaryBuilder.WriteString("\n")

			// Add code references if any
			if len(codeReferences) > 0 {
				summaryBuilder.WriteString("## 代码引用\n")
				for _, ref := range codeReferences {
					summaryBuilder.WriteString(fmt.Sprintf("- `%s`\n", ref))
				}
				summaryBuilder.WriteString("\n")
			}

			// Add recommendations if any
			if len(recommendations) > 0 {
				summaryBuilder.WriteString("## 建议\n")
				for _, rec := range recommendations {
					summaryBuilder.WriteString(fmt.Sprintf("- %s\n", rec))
				}
			}

			// Add generation timestamp
			summaryBuilder.WriteString("\n---\n")
			summaryBuilder.WriteString(fmt.Sprintf("*Generated at: %s*\n", time.Now().Format("2006-01-02 15:04:05")))

			summary := summaryBuilder.String()

			// Store final summary
			loop.Set("exploration_summary", summary)

			// Save to artifact file if output directory is configured
			outputDir := loop.Get("output_directory")
			if outputDir != "" {
				// Ensure output directory exists
				if err := os.MkdirAll(outputDir, 0755); err != nil {
					log.Warnf("failed to create output directory %s: %v", outputDir, err)
				} else {
					// Generate filename based on timestamp and exploration goal
					timestamp := time.Now().Format("20060102_150405")
					safeGoal := strings.ReplaceAll(explorationGoal, " ", "_")
					safeGoal = strings.ReplaceAll(safeGoal, "/", "_")
					if len(safeGoal) > 50 {
						safeGoal = safeGoal[:50]
					}
					filename := fmt.Sprintf("exploration_%s_%s.md", timestamp, safeGoal)
					outputPath := filepath.Join(outputDir, filename)

					// Write markdown file
					if err := os.WriteFile(outputPath, []byte(summary), 0644); err != nil {
						log.Warnf("failed to write exploration summary to %s: %v", outputPath, err)
					} else {
						log.Infof("exploration summary saved to: %s", outputPath)
						loop.Set("artifact_path", outputPath)
						invoker.AddToTimeline("artifact_saved", fmt.Sprintf("Exploration summary saved to: %s", outputPath))
					}
				}
			}

			// Emit summary
			emitter.EmitThoughtStream("exploration_conclusion", summary)
			invoker.AddToTimeline("exploration_conclusion", summary)

			log.Infof("exploration concluded with %d key findings", len(keyFindings))

			// Exit the loop - exploration is complete
			op.Exit()
		},
	)
}
