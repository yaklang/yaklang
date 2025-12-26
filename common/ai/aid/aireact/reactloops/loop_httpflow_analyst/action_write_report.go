package loop_httpflow_analyst

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// writeReportAction creates the action for writing report sections based on evidence
var writeReportAction = func(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopActionWithStreamField(
		"write_report",
		`Write Report Section - 基于证据写报告章节

【功能说明】
基于已收集的证据包写报告章节。严格遵循"基于证据的编译"原则：
- 只能引用证据包里的统计与样本
- 证据包以外的内容只能作为"假设/待验证"
- 每条结论都要带证据引用

【参数说明】
- section_title (必需): 章节标题
- section_content (必需): 章节正文内容
- evidence_refs (必需): 引用的证据 ID 列表
- scope_declaration (必需): 本节的范围声明（时间窗、数据源、过滤条件等）
- findings (可选): 本节的关键发现列表
- needs_more_query (可选): 是否需要更多查询，如果是则提供查询建议

【使用时机】
- 收集足够证据后，需要写报告章节时
- 需要总结特定维度的分析结果时`,
		[]aitool.ToolOption{
			aitool.WithStringParam("section_title",
				aitool.WithParam_Required(true),
				aitool.WithParam_Description("Section title")),
			aitool.WithStringParam("section_content",
				aitool.WithParam_Required(true),
				aitool.WithParam_Description("Section content with evidence references")),
			aitool.WithStringArrayParam("evidence_refs",
				aitool.WithParam_Required(true),
				aitool.WithParam_Description("List of evidence IDs referenced in this section")),
			aitool.WithStringParam("scope_declaration",
				aitool.WithParam_Required(true),
				aitool.WithParam_Description("Scope declaration: time range, data source, filters")),
			aitool.WithStringArrayParam("findings",
				aitool.WithParam_Description("Key findings from this section")),
			aitool.WithStringParam("needs_more_query",
				aitool.WithParam_Description("If more queries needed, describe what")),
		},
		[]*reactloops.LoopStreamField{
			{
				FieldName: "section_content",
				AINodeId:  "report-section-content",
			},
		},
		// Validator
		func(loop *reactloops.ReActLoop, action *aicommon.Action) error {
			sectionTitle := action.GetString("section_title")
			sectionContent := action.GetString("section_content")
			evidenceRefs := action.GetStringSlice("evidence_refs")
			scopeDeclaration := action.GetString("scope_declaration")

			if sectionTitle == "" {
				return utils.Error("write_report requires 'section_title' parameter")
			}
			if sectionContent == "" {
				return utils.Error("write_report requires 'section_content' parameter")
			}
			if len(evidenceRefs) == 0 {
				return utils.Error("write_report requires at least one evidence reference")
			}
			if scopeDeclaration == "" {
				return utils.Error("write_report requires 'scope_declaration' parameter")
			}

			return nil
		},
		// Handler
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			sectionTitle := action.GetString("section_title")
			sectionContent := action.GetString("section_content")
			evidenceRefs := action.GetStringSlice("evidence_refs")
			scopeDeclaration := action.GetString("scope_declaration")
			findings := action.GetStringSlice("findings")
			needsMoreQuery := action.GetString("needs_more_query")

			invoker := loop.GetInvoker()
			emitter := loop.GetEmitter()

			// Build section with scope declaration
			var sectionBuilder strings.Builder
			sectionBuilder.WriteString(fmt.Sprintf("\n## %s\n\n", sectionTitle))
			sectionBuilder.WriteString(fmt.Sprintf("> **分析范围**: %s\n\n", scopeDeclaration))
			sectionBuilder.WriteString(sectionContent)
			sectionBuilder.WriteString("\n\n")

			// Add findings
			if len(findings) > 0 {
				sectionBuilder.WriteString("### 关键发现\n")
				for i, finding := range findings {
					sectionBuilder.WriteString(fmt.Sprintf("%d. %s\n", i+1, finding))
				}
				sectionBuilder.WriteString("\n")
			}

			// Add evidence references
			sectionBuilder.WriteString(fmt.Sprintf("*证据引用: %s*\n", strings.Join(evidenceRefs, ", ")))

			sectionText := sectionBuilder.String()

			// Append to report sections
			reportSections := loop.Get("report_sections")
			loop.Set("report_sections", reportSections+sectionText)

			log.Infof("report section written: %s (refs: %v)", sectionTitle, evidenceRefs)

			// Emit section
			emitter.EmitThoughtStream("report_section", sectionText)
			invoker.AddToTimeline("report_section", fmt.Sprintf("Section: %s\nEvidence: %s\nFindings: %d",
				sectionTitle, strings.Join(evidenceRefs, ", "), len(findings)))

			// If needs more query, add to feedback
			if needsMoreQuery != "" {
				log.Infof("section suggests additional query: %s", needsMoreQuery)
				emitter.EmitThoughtStream("query_suggestion", fmt.Sprintf("建议补充查询: %s", needsMoreQuery))
			}
		},
	)
}
