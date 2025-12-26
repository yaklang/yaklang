package loop_httpflow_analyst

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// buildInitTask creates the initialization task handler for HTTPFlow analysis
func buildInitTask(r aicommon.AIInvokeRuntime) func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask) error {
	return func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask) error {
		emitter := r.GetConfig().GetEmitter()

		log.Infof("httpflow_analyst init: analyzing user requirements")

		// Use LiteForge to analyze user requirements
		promptTemplate := `
你是 HTTP 流量分析专家。分析用户需求，提取分析参数：

【任务1：明确分析目标】
用户想要了解什么？
- 安全威胁分析（异常流量、攻击特征）
- 数据泄露追踪（敏感信息、外发数据）
- 性能分析（慢响应、错误率）
- 行为审计（访问模式、用户行为）
- 资产发现（服务端点、API接口）

【任务2：确定查询范围】
- 时间范围：最近多长时间的数据
- 目标限定：特定域名、IP、路径
- 数据源类型：mitm、scan、全部

【任务3：生成查询计划】
根据分析目标，规划 2-5 个初始查询：
- 统计性查询（count、group by）
- 异常检测查询（错误码、大响应）
- 样本采集查询（典型案例）

<|USER_INPUT_{{ .nonce }}|>
{{ .data }}
<|USER_INPUT_END_{{ .nonce }}|>
`

		renderedPrompt := utils.MustRenderTemplate(
			promptTemplate,
			map[string]any{
				"nonce": utils.RandStringBytes(4),
				"data":  task.GetUserInput(),
			})

		initResult, err := r.InvokeLiteForge(
			task.GetContext(),
			"analyze-httpflow-requirements",
			renderedPrompt,
			[]aitool.ToolOption{
				aitool.WithStringParam("analysis_goal",
					aitool.WithParam_Description("Clear description of what the user wants to analyze"),
					aitool.WithParam_Required(true)),
				aitool.WithStringParam("analysis_type",
					aitool.WithParam_Description("Type of analysis: security/data_leak/performance/audit/discovery"),
					aitool.WithParam_Required(true)),
				aitool.WithStringParam("time_range",
					aitool.WithParam_Description("Time range for analysis, e.g., '24h', '7d', '30d', or 'all'"),
					aitool.WithParam_Required(true)),
				aitool.WithStringArrayParam("target_filters",
					aitool.WithParam_Description("Target filters like domain, IP, path patterns")),
				aitool.WithStringArrayParam("initial_queries",
					aitool.WithParam_Description("2-5 initial query descriptions for the analysis plan"),
					aitool.WithParam_Required(true)),
				aitool.WithStringParam("reason",
					aitool.WithParam_Description("Explain your analysis plan"),
					aitool.WithParam_Required(true)),
			},
			aicommon.WithGeneralConfigStreamableFieldWithNodeId("init-httpflow-analyst", "reason"),
		)

		if err != nil {
			log.Errorf("failed to invoke liteforge for httpflow analysis init: %v", err)
			return utils.Errorf("failed to analyze requirements: %v", err)
		}

		analysisGoal := initResult.GetString("analysis_goal")
		analysisType := initResult.GetString("analysis_type")
		timeRange := initResult.GetString("time_range")
		targetFilters := initResult.GetStringSlice("target_filters")
		initialQueries := initResult.GetStringSlice("initial_queries")
		reason := initResult.GetString("reason")

		log.Infof("httpflow_analyst init: goal=%s, type=%s, time_range=%s",
			analysisGoal, analysisType, timeRange)

		// Build query scope
		queryScope := fmt.Sprintf(`
## 查询范围（Scope）
- **分析类型**: %s
- **时间范围**: %s
- **目标过滤**: %v
- **数据源**: HTTPFlow 数据库
`, analysisType, timeRange, targetFilters)

		// Store analysis context in loop state
		loop.Set("analysis_goal", analysisGoal)
		loop.Set("analysis_type", analysisType)
		loop.Set("time_range", timeRange)
		loop.Set("query_scope", queryScope)
		loop.Set("evidence_pack", "")   // Will be populated during analysis
		loop.Set("claims_index", "")    // Will track all claims
		loop.Set("query_history", "")   // Track executed queries
		loop.Set("report_sections", "") // Accumulated report content

		// Set output directory for artifacts
		outputDir := r.GetConfig().GetConfigString("httpflow_analyst_output_directory")
		if outputDir == "" {
			// Default to temp directory
			outputDir = filepath.Join(os.TempDir(), "httpflow_analysis_reports")
		}
		loop.Set("output_directory", outputDir)
		os.MkdirAll(outputDir, 0755)
		log.Infof("httpflow_analyst: artifact output directory set to: %s", outputDir)

		// Emit analysis context
		emitter.EmitThoughtStream(task.GetIndex(), "HTTPFlow Analysis initialized:\n"+
			"- Goal: "+analysisGoal+"\n"+
			"- Type: "+analysisType+"\n"+
			"- Time Range: "+timeRange+"\n"+
			"- Initial Queries: "+fmt.Sprintf("%v", initialQueries)+"\n"+
			"- Analysis: "+reason)

		r.AddToTimeline("analysis_init", utils.MustRenderTemplate(`
HTTPFlow Analysis Initialized:
- Goal: {{ .goal }}
- Type: {{ .analysisType }}
- Time Range: {{ .timeRange }}
- Filters: {{ .filters }}
- Initial Queries: {{ .queries }}
- Reason: {{ .reason }}
- Timestamp: {{ .timestamp }}
`, map[string]any{
			"goal":         analysisGoal,
			"analysisType": analysisType,
			"timeRange":    timeRange,
			"filters":      targetFilters,
			"queries":      initialQueries,
			"reason":       reason,
			"timestamp":    time.Now().Format("2006-01-02 15:04:05"),
		}))

		return nil
	}
}
