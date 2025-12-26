package loop_httpflow_analyst

import (
	"bytes"
	_ "embed"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

//go:embed prompts/persistent_instruction.txt
var instruction string

//go:embed prompts/reflection_output_example.txt
var outputExample string

//go:embed prompts/reactive_data.txt
var reactiveData string

func init() {
	err := reactloops.RegisterLoopFactory(
		schema.AI_REACT_LOOP_NAME_HTTPFLOW_ANALYST,
		func(r aicommon.AIInvokeRuntime, opts ...reactloops.ReActLoopOption) (*reactloops.ReActLoop, error) {
			config := r.GetConfig()

			// Create preset options for httpflow_analyst loop
			preset := []reactloops.ReActLoopOption{
				reactloops.WithAllowRAG(true),
				reactloops.WithAllowToolCall(true),
				reactloops.WithInitTask(buildInitTask(r)),
				reactloops.WithMaxIterations(int(config.GetMaxIterationCount())),
				reactloops.WithAllowUserInteract(config.GetAllowUserInteraction()),
				reactloops.WithPersistentInstruction(instruction),
				reactloops.WithReflectionOutputExample(outputExample),
				reactloops.WithReactiveDataBuilder(func(loop *reactloops.ReActLoop, feedbacker *bytes.Buffer, nonce string) (string, error) {
					// Get analysis state from loop
					analysisGoal := loop.Get("analysis_goal")
					queryScope := loop.Get("query_scope")
					evidencePack := loop.Get("evidence_pack")
					claimsIndex := loop.Get("claims_index")

					feedbacks := feedbacker.String()
					feedbacks = strings.TrimSpace(feedbacks)

					renderMap := map[string]any{
						"AnalysisGoal":     analysisGoal,
						"QueryScope":       queryScope,
						"EvidencePack":     evidencePack,
						"ClaimsIndex":      claimsIndex,
						"Nonce":            nonce,
						"FeedbackMessages": feedbacks,
					}
					return utils.RenderTemplate(reactiveData, renderMap)
				}),
				// Register actions for HTTPFlow analysis
				queryHTTPFlowAction(r),      // Query HTTPFlow database
				aggregateEvidenceAction(r),  // Build evidence pack from query results
				writeReportAction(r),        // Write report section based on evidence
				concludeAnalysisAction(r),   // Finalize report and save artifact
			}
			preset = append(preset, opts...)
			return reactloops.NewReActLoop(schema.AI_REACT_LOOP_NAME_HTTPFLOW_ANALYST, r, preset...)
		},
		// Register metadata for better AI understanding
		reactloops.WithLoopDescription("Enter focused mode for HTTPFlow/HTTP History analysis. Perform evidence-based analysis of HTTP traffic data with traceable conclusions."),
		reactloops.WithLoopUsagePrompt("Use when user requests HTTP traffic analysis, security report generation, or flow investigation. Provides query_httpflow, aggregate_evidence, write_report, and conclude_analysis tools for evidence-based reporting."),
		reactloops.WithLoopOutputExample(`
* When user requests HTTP flow analysis:
  {"@action": "httpflow_analyst", "human_readable_thought": "I need to analyze HTTP traffic data to find patterns, anomalies, or specific information"}
`),
	)
	if err != nil {
		log.Errorf("register reactloop: %v failed", schema.AI_REACT_LOOP_NAME_HTTPFLOW_ANALYST)
	}
}

