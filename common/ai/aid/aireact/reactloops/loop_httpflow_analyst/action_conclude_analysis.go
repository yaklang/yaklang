package loop_httpflow_analyst

import (
	"encoding/json"
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

// AnalysisReport is the final report structure
type AnalysisReport struct {
	Title            string           `json:"title"`
	GeneratedAt      string           `json:"generated_at"`
	Scope            string           `json:"scope"`
	ExecutiveSummary string           `json:"executive_summary"`
	Sections         string           `json:"sections"`
	Conclusions      []string         `json:"conclusions"`
	Recommendations  []string         `json:"recommendations"`
	EvidencePack     *EvidencePack    `json:"evidence_pack"`
	Provenance       ReportProvenance `json:"provenance"`
}

type ReportProvenance struct {
	DataSource    string   `json:"data_source"`
	QueryCount    int      `json:"query_count"`
	EvidenceCount int      `json:"evidence_count"`
	TimeWindow    string   `json:"time_window"`
	Filters       []string `json:"filters"`
	GeneratedBy   string   `json:"generated_by"`
}

// concludeAnalysisAction creates the action for finalizing the analysis and saving the report
var concludeAnalysisAction = func(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopActionWithStreamField(
		"conclude_analysis",
		`Conclude Analysis - 完成分析并生成最终报告

【功能说明】
完成 HTTPFlow 分析，生成可追溯的最终报告。
报告将保存为 Markdown 文件，包含完整的证据链和溯源信息。

【参数说明】
- report_title (必需): 报告标题
- executive_summary (必需): 执行摘要（概述关键发现）
- conclusions (必需): 主要结论列表（每条必须有证据支持）
- recommendations (可选): 建议列表
- risk_level (可选): 风险等级评估 critical/high/medium/low/info
- output_filename (可选): 输出文件名，默认自动生成

【使用时机】
- 所有必要查询完成后
- 证据包已经构建完整
- 准备输出最终分析报告时

【输出格式】
报告包含：
1. 执行摘要
2. 分析范围与方法
3. 各章节内容（已通过 write_report 写入）
4. 结论列表（带证据引用）
5. 建议
6. 证据溯源信息`,
		[]aitool.ToolOption{
			aitool.WithStringParam("report_title",
				aitool.WithParam_Required(true),
				aitool.WithParam_Description("Report title")),
			aitool.WithStringParam("executive_summary",
				aitool.WithParam_Required(true),
				aitool.WithParam_Description("Executive summary of key findings")),
			aitool.WithStringArrayParam("conclusions",
				aitool.WithParam_Required(true),
				aitool.WithParam_Description("List of main conclusions (must be evidence-backed)")),
			aitool.WithStringArrayParam("recommendations",
				aitool.WithParam_Description("List of recommendations")),
			aitool.WithStringParam("risk_level",
				aitool.WithParam_Enum("critical", "high", "medium", "low", "info"),
				aitool.WithParam_Description("Overall risk level assessment")),
			aitool.WithStringParam("output_filename",
				aitool.WithParam_Description("Output filename, auto-generated if not specified")),
		},
		[]*reactloops.LoopStreamField{
			{
				FieldName: "executive_summary",
				AINodeId:  "analysis-conclusion",
			},
		},
		// Validator
		func(loop *reactloops.ReActLoop, action *aicommon.Action) error {
			reportTitle := action.GetString("report_title")
			executiveSummary := action.GetString("executive_summary")
			conclusions := action.GetStringSlice("conclusions")

			if reportTitle == "" {
				return utils.Error("conclude_analysis requires 'report_title' parameter")
			}
			if executiveSummary == "" {
				return utils.Error("conclude_analysis requires 'executive_summary' parameter")
			}
			if len(conclusions) == 0 {
				return utils.Error("conclude_analysis requires at least one conclusion")
			}

			return nil
		},
		// Handler
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			reportTitle := action.GetString("report_title")
			executiveSummary := action.GetString("executive_summary")
			conclusions := action.GetStringSlice("conclusions")
			recommendations := action.GetStringSlice("recommendations")
			riskLevel := action.GetString("risk_level")
			outputFilename := action.GetString("output_filename")

			invoker := loop.GetInvoker()
			emitter := loop.GetEmitter()

			// Get accumulated data
			analysisGoal := loop.Get("analysis_goal")
			queryScope := loop.Get("query_scope")
			reportSections := loop.Get("report_sections")
			claimsIndex := loop.Get("claims_index")
			queryHistory := loop.Get("query_history")
			outputDir := loop.Get("output_directory")

			if outputDir == "" {
				outputDir = os.TempDir()
			}

			// Load evidence pack
			var evidencePack *EvidencePack
			evidenceFile := filepath.Join(outputDir, "evidence_pack.json")
			if data, err := os.ReadFile(evidenceFile); err == nil {
				evidencePack = &EvidencePack{}
				json.Unmarshal(data, evidencePack)
			}

			// Build the final report
			var reportBuilder strings.Builder

			// Title
			reportBuilder.WriteString(fmt.Sprintf("# %s\n\n", reportTitle))
			reportBuilder.WriteString(fmt.Sprintf("**生成时间**: %s\n\n", time.Now().Format("2006-01-02 15:04:05")))

			// Risk level badge
			if riskLevel != "" {
				riskEmoji := map[string]string{
					"critical": "🔴",
					"high":     "🟠",
					"medium":   "🟡",
					"low":      "🟢",
					"info":     "🔵",
				}[riskLevel]
				reportBuilder.WriteString(fmt.Sprintf("**风险等级**: %s %s\n\n", riskEmoji, strings.ToUpper(riskLevel)))
			}

			// Executive Summary
			reportBuilder.WriteString("## 执行摘要\n\n")
			reportBuilder.WriteString(executiveSummary)
			reportBuilder.WriteString("\n\n")

			// Analysis Scope & Method
			reportBuilder.WriteString("## 分析范围与方法\n\n")
			reportBuilder.WriteString(fmt.Sprintf("**分析目标**: %s\n\n", analysisGoal))
			reportBuilder.WriteString(queryScope)
			reportBuilder.WriteString("\n")

			// Main sections (already written)
			if reportSections != "" {
				reportBuilder.WriteString(reportSections)
			}

			// Conclusions
			reportBuilder.WriteString("\n## 结论\n\n")
			for i, conclusion := range conclusions {
				reportBuilder.WriteString(fmt.Sprintf("%d. %s\n", i+1, conclusion))
			}
			reportBuilder.WriteString("\n")

			// Recommendations
			if len(recommendations) > 0 {
				reportBuilder.WriteString("## 建议\n\n")
				for i, rec := range recommendations {
					reportBuilder.WriteString(fmt.Sprintf("%d. %s\n", i+1, rec))
				}
				reportBuilder.WriteString("\n")
			}

			// Evidence Index
			reportBuilder.WriteString("## 证据索引\n\n")
			if claimsIndex != "" {
				reportBuilder.WriteString("```\n")
				reportBuilder.WriteString(claimsIndex)
				reportBuilder.WriteString("\n```\n\n")
			}

			// Query History (Provenance)
			reportBuilder.WriteString("## 查询历史（溯源）\n\n")
			if queryHistory != "" {
				reportBuilder.WriteString("```\n")
				reportBuilder.WriteString(queryHistory)
				reportBuilder.WriteString("\n```\n\n")
			}

			// Provenance footer
			reportBuilder.WriteString("---\n\n")
			reportBuilder.WriteString("### 报告溯源信息\n\n")
			reportBuilder.WriteString(fmt.Sprintf("- **数据源**: HTTPFlow Database\n"))
			if evidencePack != nil {
				reportBuilder.WriteString(fmt.Sprintf("- **证据数量**: %d 条\n", len(evidencePack.Items)))
			}
			reportBuilder.WriteString(fmt.Sprintf("- **生成工具**: HTTPFlow Analyst (AI-Powered)\n"))
			reportBuilder.WriteString(fmt.Sprintf("- **生成时间**: %s\n", time.Now().Format("2006-01-02 15:04:05 MST")))
			reportBuilder.WriteString("\n*本报告由 AI 基于证据包自动生成，所有结论均可通过证据索引中的 FlowID 进行复核。*\n")

			reportContent := reportBuilder.String()

			// Generate filename
			if outputFilename == "" {
				safeTitle := strings.ReplaceAll(reportTitle, " ", "_")
				safeTitle = strings.ReplaceAll(safeTitle, "/", "_")
				safeTitle = strings.ReplaceAll(safeTitle, "\\", "_")
				if len(safeTitle) > 40 {
					safeTitle = safeTitle[:40]
				}
				outputFilename = fmt.Sprintf("httpflow_analysis_%s_%s.md",
					time.Now().Format("20060102_150405"), safeTitle)
			} else {
				// Extract only the filename part if a full path was provided
				outputFilename = filepath.Base(outputFilename)
				// Ensure it ends with .md
				if !strings.HasSuffix(outputFilename, ".md") {
					outputFilename = outputFilename + ".md"
				}
			}

			// Save report
			reportPath := filepath.Join(outputDir, outputFilename)
			if err := os.WriteFile(reportPath, []byte(reportContent), 0644); err != nil {
				log.Errorf("failed to save analysis report: %v", err)
				op.Fail(fmt.Sprintf("Failed to save report: %v", err))
				return
			}

			log.Infof("HTTPFlow analysis report saved to: %s", reportPath)

			// Store the report path
			loop.Set("final_report_path", reportPath)

			// Also save as JSON for programmatic access
			jsonReport := AnalysisReport{
				Title:            reportTitle,
				GeneratedAt:      time.Now().Format("2006-01-02 15:04:05"),
				Scope:            queryScope,
				ExecutiveSummary: executiveSummary,
				Sections:         reportSections,
				Conclusions:      conclusions,
				Recommendations:  recommendations,
				EvidencePack:     evidencePack,
				Provenance: ReportProvenance{
					DataSource:  "HTTPFlow Database",
					GeneratedBy: "HTTPFlow Analyst (AI-Powered)",
				},
			}

			jsonPath := strings.TrimSuffix(reportPath, ".md") + ".json"
			if jsonData, err := json.MarshalIndent(jsonReport, "", "  "); err == nil {
				os.WriteFile(jsonPath, jsonData, 0644)
				log.Infof("HTTPFlow analysis JSON saved to: %s", jsonPath)
			}

			// Emit completion
			completionMsg := fmt.Sprintf(`
## 分析完成

**报告已生成**: %s

### 摘要
%s

### 结论数量
%d 条

### 建议数量
%d 条

*完整报告已保存，可直接查看 Markdown 文件。*
`, reportPath, truncateString(executiveSummary, 200), len(conclusions), len(recommendations))

			emitter.EmitThoughtStream("analysis_complete", completionMsg)
			invoker.AddToTimeline("analysis_complete", fmt.Sprintf("Report saved: %s", reportPath))

			log.Infof("HTTPFlow analysis completed: %d conclusions, %d recommendations", len(conclusions), len(recommendations))

			// Exit the loop
			op.Exit()
		},
	)
}
